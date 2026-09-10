import {
  ApiError,
  abortUploadSession,
  completeUploadSession,
  getUploadSession,
  isTransientStatus,
  MAX_TRANSIENT_ATTEMPTS,
  retryDelayMs,
  type StartUploadSessionResponse,
  startUploadSession,
  type UploadSessionPart,
  uploadSessionPart,
} from "./api";
import {
  BUNDLE_RECORD_OVERHEAD_BYTES,
  BUNDLE_V2_FOOTER_LENGTH,
  type BundleFile,
  type BundleManifest,
  buildBundleV2Footer,
  bundleManifestAadV2,
  bundleNameForFiles,
  bundleRecordAad,
  planEncryptedBundleV2,
  sha256Hex,
} from "./bundle";
import { type EncodedKeySet, KeySet } from "./encryption";

export const LARGE_BUNDLE_MULTIPART_THRESHOLD_BYTES = 64 * 1024 * 1024;
export const MULTIPART_UPLOAD_PART_SIZE = 32 * 1024 * 1024;
export const MULTIPART_UPLOAD_CONCURRENCY = 3;
export const S3_MIN_MULTIPART_PART_SIZE = 5 * 1024 * 1024;

const RESUME_DB = "secretli-upload-resume";
const RESUME_STORE = "sessions";
const RESUME_KEY = "latest-file-bundle";

export class UploadCancelledError extends Error {
  constructor() {
    super("upload cancelled");
    this.name = "UploadCancelledError";
  }
}

export interface MultipartBundleUploadParams {
  readonly files: File[];
  readonly baseKeySet: KeySet;
  readonly bundleKeySet: KeySet;
  readonly password?: string;
  readonly passwordProtected: boolean;
  readonly expiration: string;
  readonly burnAfterRead: boolean;
  readonly onProgress?: (progress: MultipartUploadProgress) => void;
  /** Cancels the upload; the server session is aborted and no state is kept. */
  readonly signal?: AbortSignal;
}

export interface MultipartUploadProgress {
  readonly uploadedBytes: number;
  readonly totalBytes: number;
  readonly uploadedParts: number;
  readonly totalParts?: number;
}

export interface MultipartBundleUploadResult {
  readonly expires_at: string;
  readonly manifest: BundleManifest;
  readonly encoded: EncodedKeySet;
  readonly deletionToken: string;
}

/**
 * What survives a page reload. Deliberately contains no key material: it is
 * only enough to abort a session that can no longer be resumed, so the server
 * does not keep the partial upload around for the full session TTL.
 */
interface PersistedSession {
  readonly key: string;
  readonly session_id: string;
  readonly upload_token: string;
  readonly upload_expires_at: string;
}

/**
 * Resume state for the current tab. The share secret and deletion token
 * never leave memory, so an abandoned upload cannot leave the decryption key
 * of a (possibly published) share on disk.
 */
interface ActiveUpload {
  readonly session_id: string;
  readonly upload_token: string;
  readonly share_secret: string;
  readonly deletion_token: string;
  readonly blob_token: string;
  readonly password_protected: boolean;
  readonly bundle_name: string;
  readonly fingerprints: string[];
  readonly blob_size: number;
  readonly part_size: number;
  readonly upload_expires_at: string;
  readonly nonce_salt: string;
}

let activeUpload: ActiveUpload | null = null;

/** Test hook: forget the in-memory resume state. */
export function resetMultipartUploadStateForTests(): void {
  activeUpload = null;
}

export async function uploadMultipartBundle(
  params: MultipartBundleUploadParams,
): Promise<MultipartBundleUploadResult> {
  if (params.files.length === 0) {
    throw new Error("bundle must contain at least one file");
  }
  throwIfCancelled(params.signal);

  const bundleName = bundleNameForFiles(params.files);
  const plan = planEncryptedBundleV2(params.files, bundleName);
  const fingerprints = fingerprintFiles(params.files);

  const resumed = await chooseResumeState(params, fingerprints, plan.totalSize, bundleName);
  const session = resumed
    ? await resumeUploadSession(resumed)
    : await createUploadSession(params, plan.totalSize, bundleName, fingerprints);
  const active = activeUpload;
  if (!active || active.session_id !== session.session_id) {
    throw new Error("upload session state is missing");
  }
  const baseKeySet = resumed
    ? await KeySet.fromShareSecret(active.share_secret)
    : params.baseKeySet;
  const bundleKeySet = resumed
    ? await KeySet.fromShareSecret(active.share_secret, params.password)
    : params.bundleKeySet;
  const encoded = baseKeySet.getEncoded();

  const cancel = async (): Promise<never> => {
    await cancelMultipartBundleUpload(session.session_id, session.upload_token);
    throw new UploadCancelledError();
  };

  try {
    const manifest = await encryptAndUploadParts(params, plan, session, bundleKeySet, active);
    throwIfCancelled(params.signal);
    const response = await completeUploadSession(session.session_id, session.upload_token);
    activeUpload = null;
    await clearPersistedSession();
    return {
      expires_at: response.expires_at,
      manifest,
      encoded,
      deletionToken: encoded.deletionToken,
    };
  } catch (err) {
    if (params.signal?.aborted || err instanceof UploadCancelledError) {
      return cancel();
    }
    if (err instanceof ApiError && err.status === 409) {
      // The server rejected the parts or the session (for example storage
      // discarded the multipart upload). The session cannot be resumed.
      activeUpload = null;
      await clearPersistedSession();
    }
    throw err;
  }
}

async function encryptAndUploadParts(
  params: MultipartBundleUploadParams,
  plan: ReturnType<typeof planEncryptedBundleV2>,
  session: StartUploadSessionResponse,
  bundleKeySet: KeySet,
  active: ActiveUpload,
): Promise<BundleManifest> {
  const manifestFiles = mutableManifestFiles(plan.manifest.files);
  const uploadedParts = new Map(session.uploaded_parts.map((part) => [part.part_number, part]));
  const uploader = new UploadQueue(MULTIPART_UPLOAD_CONCURRENCY);
  let uploadedBytes = session.uploaded_parts.reduce((sum, part) => sum + part.size, 0);
  let uploadedPartCount = session.uploaded_parts.length;

  const reportProgress = () => {
    params.onProgress?.({
      uploadedBytes,
      totalBytes: plan.totalSize,
      uploadedParts: uploadedPartCount,
    });
  };
  reportProgress();

  let partNumber = 1;
  let currentOffset = 0;
  let currentSize = 0;
  let currentParts: ArrayBuffer[] = [];

  const flushPart = async (isFinal: boolean) => {
    if (currentParts.length === 0) {
      return;
    }
    if (!isFinal && currentSize < S3_MIN_MULTIPART_PART_SIZE) {
      return;
    }

    const part = new Blob(currentParts, { type: "application/octet-stream" });
    const offset = currentOffset;
    const number = partNumber;
    const sha256 = await sha256Blob(part);
    currentParts = [];
    currentSize = 0;
    partNumber++;

    const existing = uploadedParts.get(number);
    if (existing) {
      if (existing.offset !== offset || existing.size !== part.size || existing.sha256 !== sha256) {
        // The recorded part cannot be ours; the session is poisoned and must
        // not be offered for resume again.
        activeUpload = null;
        await clearPersistedSession();
        throw new Error("uploaded part does not match resumable state");
      }
      return;
    }

    throwIfCancelled(params.signal);
    await uploader.schedule(async () => {
      const uploaded = await uploadPartWithRetry(
        session.session_id,
        session.upload_token,
        number,
        offset,
        part,
        sha256,
        params.signal,
      );
      uploadedParts.set(number, uploaded);
      uploadedBytes += uploaded.size;
      uploadedPartCount++;
      reportProgress();
    });
  };

  for (const record of plan.records) {
    throwIfCancelled(params.signal);
    if (
      currentParts.length > 0 &&
      currentSize + record.length > session.part_size &&
      currentSize >= S3_MIN_MULTIPART_PART_SIZE
    ) {
      await flushPart(false);
      currentOffset = record.offset;
    }

    const file = params.files[record.fileIndex];
    const plaintext = new Uint8Array(await file.slice(record.start, record.end).arrayBuffer());
    if (plaintext.length !== record.plaintextSize) {
      throw new Error("bundle file changed during encryption");
    }
    const encrypted = bundleKeySet.encryptBundlePartDeterministic(
      plaintext,
      bundleRecordAad(record.fileIndex, record.chunkIndex, record.plaintextSize),
      recordNonceLabel(
        active.nonce_salt,
        record.fileIndex,
        record.chunkIndex,
        record.plaintextSize,
      ),
    );
    if (encrypted.length !== record.length) {
      throw new Error("bundle record size mismatch");
    }
    manifestFiles[record.fileIndex].chunks[record.chunkIndex] = {
      ...manifestFiles[record.fileIndex].chunks[record.chunkIndex],
      sha256: await sha256Hex(encrypted),
    };
    currentParts.push(toArrayBuffer(encrypted));
    currentSize += encrypted.length;
  }

  const manifest: BundleManifest = { ...plan.manifest, files: manifestFiles };
  const manifestPlaintext = new TextEncoder().encode(JSON.stringify(manifest));
  if (manifestPlaintext.length + BUNDLE_RECORD_OVERHEAD_BYTES !== plan.encryptedManifestLength) {
    throw new Error("bundle manifest size mismatch");
  }
  const encryptedManifest = bundleKeySet.encryptBundlePartDeterministic(
    manifestPlaintext,
    bundleManifestAadV2(),
    `manifest:v2:${active.nonce_salt}`,
  );
  const footer = buildBundleV2Footer({
    version: 2,
    footerLength: BUNDLE_V2_FOOTER_LENGTH,
    manifestLength: encryptedManifest.length,
    manifestSha256: await sha256Hex(encryptedManifest),
  });
  currentParts.push(toArrayBuffer(encryptedManifest), toArrayBuffer(footer));
  currentSize += encryptedManifest.length + footer.length;

  await flushPart(true);
  await uploader.drain();
  return manifest;
}

/**
 * Aborts the server session and forgets all resume state. Safe to call for a
 * session that is already gone.
 */
export async function cancelMultipartBundleUpload(sessionID: string, uploadToken: string) {
  activeUpload = null;
  await clearPersistedSession();
  try {
    await abortUploadSession(sessionID, uploadToken);
  } catch (err) {
    if (!(err instanceof ApiError) || (err.status !== 404 && err.status !== 409)) {
      throw err;
    }
  }
}

async function createUploadSession(
  params: MultipartBundleUploadParams,
  blobSize: number,
  bundleName: string,
  fingerprints: string[],
): Promise<StartUploadSessionResponse> {
  const encoded = params.baseKeySet.getEncoded();
  const encryptedMeta = await params.baseKeySet.encryptMeta({
    type: "bundle",
    password_protected: params.passwordProtected,
    bundle_name: bundleName,
  });
  const blobToken = params.bundleKeySet.getEncoded().blobToken;
  const session = await startUploadSession({
    public_id: encoded.publicID,
    metadata_token: encoded.metadataToken,
    blob_token: blobToken,
    deletion_token: encoded.deletionToken,
    encrypted_meta: encryptedMeta,
    expiration: params.expiration,
    burn_after_read: params.burnAfterRead,
    blob_size: blobSize,
  });

  activeUpload = {
    session_id: session.session_id,
    upload_token: session.upload_token,
    share_secret: encoded.shareSecret,
    deletion_token: encoded.deletionToken,
    blob_token: blobToken,
    password_protected: params.passwordProtected,
    bundle_name: bundleName,
    fingerprints,
    blob_size: blobSize,
    part_size: session.part_size,
    upload_expires_at: session.upload_expires_at,
    nonce_salt: newNonceSalt(),
  };
  await persistSession({
    key: RESUME_KEY,
    session_id: session.session_id,
    upload_token: session.upload_token,
    upload_expires_at: session.upload_expires_at,
  });

  return session;
}

/**
 * Picks the in-memory session to resume when the same files are submitted
 * again in this tab. Anything left over from a previous page load has no key
 * material any more and is aborted on the server instead.
 */
async function chooseResumeState(
  params: MultipartBundleUploadParams,
  fingerprints: string[],
  blobSize: number,
  bundleName: string,
): Promise<ActiveUpload | null> {
  const persisted = await readPersistedSession();
  if (persisted && (!activeUpload || activeUpload.session_id !== persisted.session_id)) {
    await abortStaleSession(persisted);
  }

  const state = activeUpload;
  if (!state) {
    return null;
  }
  if (
    state.blob_size !== blobSize ||
    state.bundle_name !== bundleName ||
    state.password_protected !== params.passwordProtected ||
    state.upload_expires_at <= new Date().toISOString() ||
    !sameStrings(state.fingerprints, fingerprints)
  ) {
    // Different files or an expired session: it can never complete.
    await abortStaleSession(state);
    return null;
  }

  const bundleKeySet = await KeySet.fromShareSecret(state.share_secret, params.password);
  if (bundleKeySet.getEncoded().blobToken !== state.blob_token) {
    await abortStaleSession(state);
    return null;
  }
  return state;
}

async function abortStaleSession(session: { session_id: string; upload_token: string }) {
  try {
    await cancelMultipartBundleUpload(session.session_id, session.upload_token);
  } catch {
    // Best effort: the server reaps expired sessions on its own.
  }
}

async function resumeUploadSession(state: ActiveUpload): Promise<StartUploadSessionResponse> {
  const status = await getUploadSession(state.session_id, state.upload_token);
  if (status.state !== "pending") {
    activeUpload = null;
    await clearPersistedSession();
    throw new Error("upload session is no longer pending");
  }
  return { ...status, upload_token: state.upload_token };
}

async function uploadPartWithRetry(
  sessionID: string,
  uploadToken: string,
  partNumber: number,
  offset: number,
  part: Blob,
  sha256: string,
  signal?: AbortSignal,
): Promise<UploadSessionPart> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= MAX_TRANSIENT_ATTEMPTS; attempt++) {
    throwIfCancelled(signal);
    try {
      return await uploadSessionPart(
        sessionID,
        uploadToken,
        partNumber,
        offset,
        part,
        sha256,
        signal,
      );
    } catch (err) {
      if (!(err instanceof ApiError) || !isTransientStatus(err.status)) {
        throw err;
      }
      lastError = err;
      if (attempt === MAX_TRANSIENT_ATTEMPTS) break;
      await delay(retryDelayMs(attempt, err.status === 429 ? "1" : undefined));
    }
  }
  throw lastError;
}

class UploadQueue {
  private readonly inFlight = new Set<Promise<void>>();
  private readonly concurrency: number;

  constructor(concurrency: number) {
    this.concurrency = concurrency;
  }

  async schedule(task: () => Promise<void>) {
    while (this.inFlight.size >= this.concurrency) {
      await Promise.race(this.inFlight);
    }
    const promise = task().finally(() => {
      this.inFlight.delete(promise);
    });
    this.inFlight.add(promise);
  }

  async drain() {
    await Promise.all(this.inFlight);
  }
}

function throwIfCancelled(signal?: AbortSignal) {
  if (signal?.aborted) {
    throw new UploadCancelledError();
  }
}

function recordNonceLabel(
  salt: string,
  fileIndex: number,
  chunkIndex: number,
  plaintextSize: number,
): string {
  // The salt is fresh per upload attempt, so a resumed session re-derives the
  // same nonces (needed for idempotent part hashes) while a new session with
  // the same key can never reuse a nonce for different plaintext.
  return `record:${salt}:${fileIndex}:${chunkIndex}:${plaintextSize}`;
}

function newNonceSalt(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

async function sha256Blob(blob: Blob): Promise<string> {
  return sha256Hex(new Uint8Array(await blob.arrayBuffer()));
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function mutableManifestFiles(files: readonly BundleFile[]): BundleFile[] {
  return files.map((file) => ({
    ...file,
    chunks: file.chunks.map((chunk) => ({ ...chunk })),
  }));
}

function fingerprintFiles(files: File[]): string[] {
  return files.map((file) => {
    const path = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name;
    return [path, file.name, file.type, file.size, file.lastModified].join(" ");
  });
}

function sameStrings(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function readPersistedSession(): Promise<PersistedSession | null> {
  const db = await openResumeDB();
  if (!db) return null;
  return new Promise((resolve) => {
    const tx = db.transaction(RESUME_STORE, "readonly");
    const req = tx.objectStore(RESUME_STORE).get(RESUME_KEY);
    req.onsuccess = () => resolve((req.result as PersistedSession | undefined) ?? null);
    req.onerror = () => resolve(null);
    tx.oncomplete = () => db.close();
  });
}

async function persistSession(state: PersistedSession): Promise<void> {
  const db = await openResumeDB();
  if (!db) return;
  return new Promise((resolve) => {
    const tx = db.transaction(RESUME_STORE, "readwrite");
    tx.objectStore(RESUME_STORE).put(state);
    tx.oncomplete = () => {
      db.close();
      resolve();
    };
    tx.onerror = () => {
      db.close();
      resolve();
    };
  });
}

async function clearPersistedSession(): Promise<void> {
  const db = await openResumeDB();
  if (!db) return;
  return new Promise((resolve) => {
    const tx = db.transaction(RESUME_STORE, "readwrite");
    tx.objectStore(RESUME_STORE).delete(RESUME_KEY);
    tx.oncomplete = () => {
      db.close();
      resolve();
    };
    tx.onerror = () => {
      db.close();
      resolve();
    };
  });
}

async function openResumeDB(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === "undefined") {
    return null;
  }
  return new Promise((resolve) => {
    const req = indexedDB.open(RESUME_DB, 2);
    req.onupgradeneeded = () => {
      // Version 1 stored key material; drop it and start clean.
      if (req.result.objectStoreNames.contains(RESUME_STORE)) {
        req.result.deleteObjectStore(RESUME_STORE);
      }
      req.result.createObjectStore(RESUME_STORE, { keyPath: "key" });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => resolve(null);
  });
}
