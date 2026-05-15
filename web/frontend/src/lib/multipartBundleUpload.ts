import {
  abortUploadSession,
  completeUploadSession,
  getUploadSession,
  type StartUploadSessionResponse,
  startUploadSession,
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
} from "./bundleV2";
import { type EncodedKeySet, KeySet } from "./encryption";

export const LARGE_BUNDLE_MULTIPART_THRESHOLD_BYTES = 64 * 1024 * 1024;
export const MULTIPART_UPLOAD_PART_SIZE = 32 * 1024 * 1024;
export const MULTIPART_UPLOAD_CONCURRENCY = 3;
export const S3_MIN_MULTIPART_PART_SIZE = 5 * 1024 * 1024;

const RESUME_DB = "secretli-upload-resume";
const RESUME_STORE = "sessions";
const RESUME_KEY = "latest-file-bundle";
const MAX_UPLOAD_ATTEMPTS = 3;

export interface MultipartBundleUploadParams {
  readonly files: File[];
  readonly baseKeySet: KeySet;
  readonly bundleKeySet: KeySet;
  readonly password?: string;
  readonly passwordProtected: boolean;
  readonly expiration: string;
  readonly burnAfterRead: boolean;
  readonly onProgress?: (progress: MultipartUploadProgress) => void;
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

interface ResumeState {
  readonly key: string;
  readonly session_id: string;
  readonly upload_token: string;
  readonly public_id: string;
  readonly share_secret: string;
  readonly metadata_token: string;
  readonly blob_token: string;
  readonly deletion_token: string;
  readonly encrypted_meta: string;
  readonly expiration: string;
  readonly burn_after_read: boolean;
  readonly password_protected: boolean;
  readonly bundle_name: string;
  readonly fingerprints: string[];
  readonly blob_size: number;
  readonly part_size: number;
  readonly upload_expires_at: string;
  readonly uploaded_parts: ResumePart[];
}

interface ResumePart {
  readonly part_number: number;
  readonly offset: number;
  readonly size: number;
  readonly sha256: string;
}

export async function uploadMultipartBundle(
  params: MultipartBundleUploadParams,
): Promise<MultipartBundleUploadResult> {
  if (params.files.length === 0) {
    throw new Error("bundle must contain at least one file");
  }

  const bundleName = bundleNameForFiles(params.files);
  const plan = planEncryptedBundleV2(params.files, bundleName);
  const fingerprints = fingerprintFiles(params.files);
  const resumed = await chooseResumeState(params, fingerprints, plan.totalSize, bundleName);
  const session = resumed
    ? await resumeUploadSession(resumed)
    : await createUploadSession(params, plan.totalSize, bundleName, fingerprints);
  const baseKeySet = resumed
    ? await KeySet.fromShareSecret(resumed.share_secret)
    : params.baseKeySet;
  const bundleKeySet = resumed
    ? await KeySet.fromShareSecret(resumed.share_secret, params.password)
    : params.bundleKeySet;
  const encoded = baseKeySet.getEncoded();

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
        throw new Error("uploaded part does not match resumable state");
      }
      return;
    }

    await uploader.schedule(async () => {
      const uploaded = await uploadPartWithRetry(
        session.session_id,
        session.upload_token,
        number,
        offset,
        part,
        sha256,
      );
      uploadedParts.set(number, uploaded);
      uploadedBytes += uploaded.size;
      uploadedPartCount++;
      await updateResumeParts(session.session_id, Array.from(uploadedParts.values()));
      reportProgress();
    });
  };

  for (const record of plan.records) {
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
      `record:${record.fileIndex}:${record.chunkIndex}:${record.plaintextSize}`,
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
    "manifest:v2",
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

  const response = await completeUploadSession(session.session_id, session.upload_token);
  await clearResumeState();
  return {
    expires_at: response.expires_at,
    manifest,
    encoded,
    deletionToken: encoded.deletionToken,
  };
}

export async function cancelMultipartBundleUpload(sessionID: string, uploadToken: string) {
  await abortUploadSession(sessionID, uploadToken);
  await clearResumeState();
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
  const session = await startUploadSession({
    public_id: encoded.publicID,
    metadata_token: encoded.metadataToken,
    blob_token: params.bundleKeySet.getEncoded().blobToken,
    deletion_token: encoded.deletionToken,
    encrypted_meta: encryptedMeta,
    expiration: params.expiration,
    burn_after_read: params.burnAfterRead,
    blob_size: blobSize,
  });

  await storeResumeState({
    key: RESUME_KEY,
    session_id: session.session_id,
    upload_token: session.upload_token,
    public_id: encoded.publicID,
    share_secret: encoded.shareSecret,
    metadata_token: encoded.metadataToken,
    blob_token: params.bundleKeySet.getEncoded().blobToken,
    deletion_token: encoded.deletionToken,
    encrypted_meta: encryptedMeta,
    expiration: params.expiration,
    burn_after_read: params.burnAfterRead,
    password_protected: params.passwordProtected,
    bundle_name: bundleName,
    fingerprints,
    blob_size: blobSize,
    part_size: session.part_size,
    upload_expires_at: session.upload_expires_at,
    uploaded_parts: [],
  });

  return session;
}

async function chooseResumeState(
  params: MultipartBundleUploadParams,
  fingerprints: string[],
  blobSize: number,
  bundleName: string,
): Promise<ResumeState | null> {
  const state = await readResumeState();
  if (
    !state ||
    state.blob_size !== blobSize ||
    state.bundle_name !== bundleName ||
    state.password_protected !== params.passwordProtected ||
    state.upload_expires_at <= new Date().toISOString() ||
    !sameStrings(state.fingerprints, fingerprints)
  ) {
    return null;
  }

  const bundleKeySet = await KeySet.fromShareSecret(state.share_secret, params.password);
  if (bundleKeySet.getEncoded().blobToken !== state.blob_token) {
    return null;
  }

  if (typeof window !== "undefined" && !window.confirm("Resume the unfinished file upload?")) {
    return null;
  }
  return state;
}

async function resumeUploadSession(state: ResumeState): Promise<StartUploadSessionResponse> {
  const status = await getUploadSession(state.session_id, state.upload_token);
  return {
    ...status,
    upload_token: state.upload_token,
    uploaded_parts: status.uploaded_parts.length > 0 ? status.uploaded_parts : state.uploaded_parts,
  };
}

async function uploadPartWithRetry(
  sessionID: string,
  uploadToken: string,
  partNumber: number,
  offset: number,
  part: Blob,
  sha256: string,
) {
  let lastError: unknown;
  for (let attempt = 1; attempt <= MAX_UPLOAD_ATTEMPTS; attempt++) {
    try {
      return await uploadSessionPart(sessionID, uploadToken, partNumber, offset, part, sha256);
    } catch (err) {
      lastError = err;
      if (attempt === MAX_UPLOAD_ATTEMPTS) break;
      await delay(250 * attempt);
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
    return [path, file.name, file.type, file.size, file.lastModified].join("\u0000");
  });
}

function sameStrings(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function readResumeState(): Promise<ResumeState | null> {
  const db = await openResumeDB();
  if (!db) return null;
  return new Promise((resolve) => {
    const tx = db.transaction(RESUME_STORE, "readonly");
    const req = tx.objectStore(RESUME_STORE).get(RESUME_KEY);
    req.onsuccess = () => resolve((req.result as ResumeState | undefined) ?? null);
    req.onerror = () => resolve(null);
    tx.oncomplete = () => db.close();
  });
}

async function storeResumeState(state: ResumeState): Promise<void> {
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

async function updateResumeParts(sessionID: string, parts: ResumePart[]) {
  const state = await readResumeState();
  if (!state || state.session_id !== sessionID) return;
  await storeResumeState({ ...state, uploaded_parts: parts });
}

async function clearResumeState(): Promise<void> {
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
    const req = indexedDB.open(RESUME_DB, 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore(RESUME_STORE, { keyPath: "key" });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => resolve(null);
  });
}
