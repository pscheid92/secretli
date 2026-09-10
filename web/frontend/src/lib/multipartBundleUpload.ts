import {
  ApiError,
  abortUploadSession,
  completeUploadSession,
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
  type BundleV2Plan,
  buildBundleV2Footer,
  bundleManifestAadV2,
  bundleNameForFiles,
  bundleRecordAad,
  planEncryptedBundleV2,
  sha256Hex,
} from "./bundle";
import type { EncodedKeySet, KeySet } from "./encryption";

export const MULTIPART_UPLOAD_PART_SIZE = 32 * 1024 * 1024;
export const MULTIPART_UPLOAD_CONCURRENCY = 3;
export const S3_MIN_MULTIPART_PART_SIZE = 5 * 1024 * 1024;

export class UploadCancelledError extends Error {
  constructor() {
    super("upload cancelled");
    this.name = "UploadCancelledError";
  }
}

export interface MultipartBundleUploadParams {
  readonly files: File[];
  /** Recorded in the encrypted metadata so the reader knows how to present it. */
  readonly secretType: "text" | "bundle";
  readonly baseKeySet: KeySet;
  readonly bundleKeySet: KeySet;
  readonly passwordProtected: boolean;
  readonly expiration: string;
  readonly burnAfterRead: boolean;
  readonly onProgress?: (progress: MultipartUploadProgress) => void;
  /** Cancels the upload; the server session is aborted. */
  readonly signal?: AbortSignal;
}

export interface MultipartUploadProgress {
  readonly uploadedBytes: number;
  readonly totalBytes: number;
  readonly uploadedParts: number;
}

export interface MultipartBundleUploadResult {
  readonly expires_at: string;
  readonly manifest: BundleManifest;
  readonly encoded: EncodedKeySet;
  readonly deletionToken: string;
}

/**
 * Encrypts the files record by record and streams them to the server as S3
 * multipart parts, so neither side ever holds the whole bundle in memory.
 * A failed upload is simply started again with fresh keys; nothing about an
 * attempt is persisted.
 */
export async function uploadMultipartBundle(
  params: MultipartBundleUploadParams,
): Promise<MultipartBundleUploadResult> {
  if (params.files.length === 0) {
    throw new Error("bundle must contain at least one file");
  }
  throwIfCancelled(params.signal);

  const bundleName = bundleNameForFiles(params.files);
  const plan = planEncryptedBundleV2(params.files, bundleName);
  const session = await createUploadSession(params, plan.totalSize, bundleName);
  const encoded = params.baseKeySet.getEncoded();

  try {
    const manifest = await encryptAndUploadParts(params, plan, session);
    throwIfCancelled(params.signal);
    const response = await completeUploadSession(session.session_id, session.upload_token);
    return {
      expires_at: response.expires_at,
      manifest,
      encoded,
      deletionToken: encoded.deletionToken,
    };
  } catch (err) {
    // Whatever went wrong, the partial upload is useless: release it.
    await abortQuietly(session.session_id, session.upload_token);
    if (params.signal?.aborted || err instanceof UploadCancelledError) {
      throw new UploadCancelledError();
    }
    throw err;
  }
}

async function encryptAndUploadParts(
  params: MultipartBundleUploadParams,
  plan: BundleV2Plan,
  session: StartUploadSessionResponse,
): Promise<BundleManifest> {
  const manifestFiles = mutableManifestFiles(plan.manifest.files);
  const uploader = new UploadQueue(MULTIPART_UPLOAD_CONCURRENCY);
  let uploadedBytes = 0;
  let uploadedPartCount = 0;

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
    const encrypted = params.bundleKeySet.encryptBundlePart(
      plaintext,
      bundleRecordAad(record.fileIndex, record.chunkIndex, record.plaintextSize),
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
  const encryptedManifest = params.bundleKeySet.encryptBundlePart(
    manifestPlaintext,
    bundleManifestAadV2(),
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

async function createUploadSession(
  params: MultipartBundleUploadParams,
  blobSize: number,
  bundleName: string,
): Promise<StartUploadSessionResponse> {
  const encoded = params.baseKeySet.getEncoded();
  const encryptedMeta = await params.baseKeySet.encryptMeta({
    type: params.secretType,
    password_protected: params.passwordProtected,
    bundle_name: bundleName,
  });
  return startUploadSession({
    public_id: encoded.publicID,
    metadata_token: encoded.metadataToken,
    blob_token: params.bundleKeySet.getEncoded().blobToken,
    deletion_token: encoded.deletionToken,
    encrypted_meta: encryptedMeta,
    expiration: params.expiration,
    burn_after_read: params.burnAfterRead,
    blob_size: blobSize,
  });
}

/** Best-effort abort; the server reaps expired sessions on its own anyway. */
async function abortQuietly(sessionID: string, uploadToken: string) {
  try {
    await abortUploadSession(sessionID, uploadToken);
  } catch {
    // ignore
  }
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

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
