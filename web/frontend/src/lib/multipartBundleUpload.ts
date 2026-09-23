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
  BUNDLE_FOOTER_LENGTH,
  type BundleManifest,
  type BundlePlan,
  buildBundleFooter,
  bundleManifestAad,
  bundleNameForFiles,
  bundleRecordAad,
  planBundle,
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
  const plan = planBundle(params.files, bundleName);
  const session = await createUploadSession(params, plan.totalSize, bundleName);
  const encoded = params.baseKeySet.getEncoded();

  try {
    const manifest = await encryptAndUploadParts(params, plan, session);
    throwIfCancelled(params.signal);
    // The server answers a repeated complete with the same result, so a
    // transient failure here must not throw the whole upload away.
    const response = await withTransientRetry(
      () => completeUploadSession(session.session_id, session.upload_token),
      params.signal,
    );
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
  plan: BundlePlan,
  session: StartUploadSessionResponse,
): Promise<BundleManifest> {
  const uploader = new UploadQueue(MULTIPART_UPLOAD_CONCURRENCY, params.signal);
  try {
    return await encryptAndQueueParts(params, plan, session, uploader);
  } finally {
    // After a failure (including one while encrypting), parts still in flight
    // are pointless; after success nothing is left running.
    uploader.close();
  }
}

async function encryptAndQueueParts(
  params: MultipartBundleUploadParams,
  plan: BundlePlan,
  session: StartUploadSessionResponse,
  uploader: UploadQueue,
): Promise<BundleManifest> {
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
    await uploader.schedule(async (signal) => {
      const uploaded = await uploadPartWithRetry(
        session.session_id,
        session.upload_token,
        number,
        offset,
        part,
        sha256,
        signal,
      );
      uploadedBytes += uploaded.size;
      uploadedPartCount++;
      reportProgress();
    });
  };

  for (const record of plan.records) {
    throwIfCancelled(params.signal);
    // Stop encrypting as soon as any part has failed for good.
    uploader.throwIfFailed();
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
    currentParts.push(toArrayBuffer(encrypted));
    currentSize += encrypted.length;
  }

  // The manifest is fully known at plan time, so the declared blob size the
  // server validated against still holds.
  const manifest = plan.manifest;
  const encryptedManifest = params.bundleKeySet.encryptBundlePart(
    new TextEncoder().encode(JSON.stringify(manifest)),
    bundleManifestAad(),
  );
  if (encryptedManifest.length !== plan.encryptedManifestLength) {
    throw new Error("bundle manifest size mismatch");
  }
  const footer = buildBundleFooter({
    version: 2,
    footerLength: BUNDLE_FOOTER_LENGTH,
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

function uploadPartWithRetry(
  sessionID: string,
  uploadToken: string,
  partNumber: number,
  offset: number,
  part: Blob,
  sha256: string,
  signal: AbortSignal,
): Promise<UploadSessionPart> {
  return withTransientRetry(
    () => uploadSessionPart(sessionID, uploadToken, partNumber, offset, part, sha256, signal),
    signal,
  );
}

/**
 * Retries network failures, rate limiting and server errors, waiting as long
 * as the server asks (capped by retryDelayMs). Client errors are final.
 */
async function withTransientRetry<T>(request: () => Promise<T>, signal?: AbortSignal): Promise<T> {
  for (let attempt = 1; ; attempt++) {
    throwIfCancelled(signal);
    try {
      return await request();
    } catch (err) {
      if (
        !(err instanceof ApiError) ||
        !isTransientStatus(err.status) ||
        attempt === MAX_TRANSIENT_ATTEMPTS
      ) {
        throw err;
      }
      await delay(retryDelayMs(attempt, err.retryAfter), signal);
    }
  }
}

/**
 * Runs part uploads with bounded concurrency. The first failure is kept and
 * aborts the uploads still running; the caller sees it on its next schedule(),
 * throwIfFailed() or drain(), so a failed part can never go unnoticed.
 */
class UploadQueue {
  private readonly inFlight = new Set<Promise<void>>();
  private readonly concurrency: number;
  private readonly controller = new AbortController();
  private failure: { error: unknown } | undefined;

  /** Cancelling the upload aborts every part still in flight. */
  constructor(concurrency: number, cancel?: AbortSignal) {
    this.concurrency = concurrency;
    if (cancel?.aborted) {
      this.controller.abort();
    }
    // Tied to the queue's own signal, so the listener goes away with it.
    cancel?.addEventListener("abort", () => this.controller.abort(), {
      once: true,
      signal: this.controller.signal,
    });
  }

  /** Aborts whatever is still running and detaches from the cancel signal. */
  close() {
    this.controller.abort();
  }

  async schedule(task: (signal: AbortSignal) => Promise<void>) {
    this.throwIfFailed();
    while (this.inFlight.size >= this.concurrency) {
      await Promise.race(this.inFlight);
      this.throwIfFailed();
    }
    // Queued promises never reject: a failure is recorded instead, so it is
    // reported exactly once and never becomes an unhandled rejection.
    const promise = task(this.controller.signal)
      .catch((error: unknown) => this.fail(error))
      .finally(() => {
        this.inFlight.delete(promise);
      });
    this.inFlight.add(promise);
  }

  throwIfFailed() {
    if (this.failure) {
      throw this.failure.error;
    }
  }

  async drain() {
    await Promise.all(this.inFlight);
    this.throwIfFailed();
  }

  private fail(error: unknown) {
    // Only the first failure matters; the rest are the aborts it caused.
    if (this.failure) return;
    this.failure = { error };
    this.controller.abort();
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

/** Waits, unless the signal fires first: a cancelled upload stops waiting. */
function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new UploadCancelledError());
      return;
    }
    const onAbort = () => {
      clearTimeout(timer);
      reject(new UploadCancelledError());
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}
