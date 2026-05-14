import {
  completeChunkedUpload,
  createChunkedUpload,
  getChunkedUploadStatus,
  type UploadedChunkStatus,
  uploadEncryptedChunk,
  uploadEncryptedManifest,
} from "./api";
import {
  buildChunkedBundlePlan,
  buildChunkedManifest,
  ChunkedEncryptionPool,
  type ChunkUploadRecord,
  chunkAad,
  encryptChunkedManifest,
  estimateChunkedEncryptedSize,
  sha256Hex,
} from "./chunkedBundle";
import type { EncodedKeySet, KeySet } from "./encryption";

const DB_NAME = "secretli-chunked-uploads";
const DB_VERSION = 1;
const STORE_NAME = "uploads";
const PARALLEL_CHUNK_UPLOADS = 3;
const MAX_UPLOAD_ATTEMPTS = 3;

export interface FileFingerprint {
  readonly path: string;
  readonly name: string;
  readonly type: string;
  readonly size: number;
  readonly lastModified: number;
}

export interface ChunkedUploadResumeState {
  readonly publicID: string;
  readonly uploadToken: string;
  readonly uploadExpiresAt: string;
  readonly shareSecret: string;
  readonly deletionToken: string;
  readonly passwordProtected: boolean;
  readonly expiration: string;
  readonly burnAfterRead: boolean;
  readonly chunkSize: number;
  readonly chunkCount: number;
  readonly encryptedTotalSize: number;
  readonly fileFingerprints: FileFingerprint[];
  readonly chunks: ChunkUploadRecord[];
  readonly manifest?: {
    readonly encryptedSize: number;
    readonly sha256: string;
  };
  readonly updatedAt: string;
}

export interface ChunkedUploadProgress {
  readonly phase: "encrypting" | "uploading" | "manifest" | "complete";
  readonly uploadedBytes: number;
  readonly totalBytes: number;
}

export interface UploadChunkedShareOptions {
  readonly files: File[];
  readonly baseKeySet: KeySet;
  readonly encryptKeySet: KeySet;
  readonly encryptedMeta: string;
  readonly expiration: string;
  readonly burnAfterRead: boolean;
  readonly passwordProtected: boolean;
  readonly resumeState?: ChunkedUploadResumeState | null;
  readonly onProgress?: (progress: ChunkedUploadProgress) => void;
}

export interface UploadChunkedShareResult {
  readonly encoded: EncodedKeySet;
  readonly expiresAt: string;
}

export async function uploadChunkedShare(
  options: UploadChunkedShareOptions,
): Promise<UploadChunkedShareResult> {
  const plan = buildChunkedBundlePlan(options.files);
  const encoded = options.baseKeySet.getEncoded();
  const blobEncoded = options.encryptKeySet.getEncoded();
  const fingerprints = fileFingerprints(options.files);
  const estimatedTotal = estimateChunkedEncryptedSize(options.files.map((file) => file.size));

  let state = options.resumeState ?? null;
  if (state) {
    if (
      state.publicID !== encoded.publicID ||
      !fingerprintsMatch(state.fileFingerprints, fingerprints)
    ) {
      throw new Error("Selected files do not match the pending upload.");
    }
  } else {
    const init = await createChunkedUpload({
      public_id: encoded.publicID,
      metadata_token: encoded.metadataToken,
      blob_token: blobEncoded.blobToken,
      deletion_token: encoded.deletionToken,
      encrypted_meta: options.encryptedMeta,
      expiration: options.expiration,
      burn_after_read: options.burnAfterRead,
      chunk_size: plan.chunkSize,
      chunk_count: plan.chunks.length,
      encrypted_total_size: estimatedTotal,
    });
    state = {
      publicID: encoded.publicID,
      uploadToken: init.upload_token,
      uploadExpiresAt: init.upload_expires_at,
      shareSecret: encoded.shareSecret,
      deletionToken: encoded.deletionToken,
      passwordProtected: options.passwordProtected,
      expiration: options.expiration,
      burnAfterRead: options.burnAfterRead,
      chunkSize: init.chunk_size,
      chunkCount: plan.chunks.length,
      encryptedTotalSize: estimatedTotal,
      fileFingerprints: fingerprints,
      chunks: [],
      updatedAt: new Date().toISOString(),
    };
    await saveChunkedUploadState(state);
  }

  const uploadToken = state.uploadToken;
  const serverState = await getChunkedUploadStatus(encoded.publicID, uploadToken);
  const uploadedRecords = new Map<number, ChunkUploadRecord>();
  for (const chunk of serverState.chunks) {
    uploadedRecords.set(chunk.index, {
      globalIndex: chunk.index,
      encryptedSize: chunk.encrypted_size,
      sha256: chunk.sha256,
    });
  }

  let uploadedBytes = Array.from(uploadedRecords.values()).reduce(
    (sum, record) => sum + record.encryptedSize,
    0,
  );
  let totalBytes = estimatedTotal;
  const inFlight = new Map<number, number>();
  const pool = new ChunkedEncryptionPool(options.encryptKeySet.getChunkedEncryptionMaterial());

  const emitProgress = (phase: ChunkedUploadProgress["phase"]) => {
    const inFlightBytes = Array.from(inFlight.values()).reduce((sum, bytes) => sum + bytes, 0);
    options.onProgress?.({
      phase,
      uploadedBytes: uploadedBytes + inFlightBytes,
      totalBytes,
    });
  };

  try {
    await runLimited(plan.chunks, PARALLEL_CHUNK_UPLOADS, async (chunk) => {
      if (uploadedRecords.has(chunk.globalIndex)) {
        emitProgress("uploading");
        return;
      }

      emitProgress("encrypting");
      const file = options.files[chunk.fileIndex];
      const plaintext = new Uint8Array(await file.slice(chunk.start, chunk.end).arrayBuffer());
      const encrypted = await pool.encrypt(
        plaintext,
        chunkAad(chunk.fileIndex, chunk.fileChunkIndex, chunk.globalIndex, chunk.plaintextSize),
      );
      const hash = await sha256Hex(encrypted);
      emitProgress("uploading");

      const uploaded = await uploadChunkWithRetry(
        encoded.publicID,
        uploadToken,
        chunk.globalIndex,
        encrypted,
        hash,
        (loaded) => {
          inFlight.set(chunk.globalIndex, loaded);
          emitProgress("uploading");
        },
      );

      inFlight.delete(chunk.globalIndex);
      uploadedBytes += uploaded.encrypted_size;
      const record = {
        globalIndex: chunk.globalIndex,
        encryptedSize: uploaded.encrypted_size,
        sha256: uploaded.sha256,
      };
      uploadedRecords.set(chunk.globalIndex, record);
      await saveChunkedUploadState({
        ...state,
        chunks: Array.from(uploadedRecords.values()).sort((a, b) => a.globalIndex - b.globalIndex),
        updatedAt: new Date().toISOString(),
      });
      emitProgress("uploading");
    });

    let manifestStatus = serverState.manifest;
    if (!manifestStatus) {
      const records = Array.from(uploadedRecords.values()).sort(
        (a, b) => a.globalIndex - b.globalIndex,
      );
      const manifest = buildChunkedManifest(plan, records);
      const encryptedManifest = encryptChunkedManifest(manifest, options.encryptKeySet);
      const manifestHash = await sha256Hex(encryptedManifest);
      totalBytes = uploadedBytes + encryptedManifest.byteLength;
      emitProgress("manifest");

      manifestStatus = await uploadManifestWithRetry(
        encoded.publicID,
        uploadToken,
        encryptedManifest,
        manifestHash,
        (loaded) => {
          inFlight.set(-1, loaded);
          emitProgress("manifest");
        },
      );
      inFlight.delete(-1);
      uploadedBytes += manifestStatus.encrypted_size;
      await saveChunkedUploadState({
        ...state,
        chunks: Array.from(uploadedRecords.values()).sort((a, b) => a.globalIndex - b.globalIndex),
        manifest: {
          encryptedSize: manifestStatus.encrypted_size,
          sha256: manifestStatus.sha256,
        },
        updatedAt: new Date().toISOString(),
      });
    }

    emitProgress("complete");
    const completed = await completeChunkedUpload(encoded.publicID, uploadToken);
    await deleteChunkedUploadState(encoded.publicID);
    return { encoded, expiresAt: completed.expires_at };
  } finally {
    pool.destroy();
  }
}

export function fileFingerprints(files: File[]): FileFingerprint[] {
  return files.map((file) => ({
    path: (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name,
    name: file.name,
    type: file.type || "application/octet-stream",
    size: file.size,
    lastModified: file.lastModified,
  }));
}

export function fingerprintsMatch(a: readonly FileFingerprint[], b: readonly FileFingerprint[]) {
  if (a.length !== b.length) return false;
  return a.every((fingerprint, index) => {
    const other = b[index];
    return (
      other &&
      fingerprint.path === other.path &&
      fingerprint.name === other.name &&
      fingerprint.type === other.type &&
      fingerprint.size === other.size &&
      fingerprint.lastModified === other.lastModified
    );
  });
}

export async function saveChunkedUploadState(state: ChunkedUploadResumeState): Promise<void> {
  const db = await openUploadDB();
  if (!db) return;
  await idbRequest(
    db.transaction(STORE_NAME, "readwrite").objectStore(STORE_NAME).put(state, state.publicID),
  );
}

export async function deleteChunkedUploadState(publicID: string): Promise<void> {
  const db = await openUploadDB();
  if (!db) return;
  await idbRequest(
    db.transaction(STORE_NAME, "readwrite").objectStore(STORE_NAME).delete(publicID),
  );
}

export async function listPendingChunkedUploadStates(): Promise<ChunkedUploadResumeState[]> {
  const db = await openUploadDB();
  if (!db) return [];
  const states = await idbRequest<ChunkedUploadResumeState[]>(
    db.transaction(STORE_NAME, "readonly").objectStore(STORE_NAME).getAll(),
  );
  const now = Date.now();
  return states
    .filter((state) => Date.parse(state.uploadExpiresAt) > now)
    .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
}

async function uploadChunkWithRetry(
  publicID: string,
  uploadToken: string,
  index: number,
  bytes: Uint8Array,
  sha256: string,
  onProgress: (loaded: number) => void,
): Promise<UploadedChunkStatus> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= MAX_UPLOAD_ATTEMPTS; attempt++) {
    try {
      return await uploadEncryptedChunk(publicID, uploadToken, index, bytes, sha256, (loaded) =>
        onProgress(loaded),
      );
    } catch (error) {
      lastError = error;
      const status = await getChunkedUploadStatus(publicID, uploadToken).catch(() => null);
      const uploaded = status?.chunks.find((chunk) => chunk.index === index);
      if (uploaded?.sha256 === sha256 && uploaded.encrypted_size === bytes.byteLength) {
        return uploaded;
      }
      if (attempt < MAX_UPLOAD_ATTEMPTS) {
        await delay(250 * 2 ** (attempt - 1));
      }
    }
  }
  throw lastError;
}

async function uploadManifestWithRetry(
  publicID: string,
  uploadToken: string,
  bytes: Uint8Array,
  sha256: string,
  onProgress: (loaded: number) => void,
): Promise<UploadedChunkStatus> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= MAX_UPLOAD_ATTEMPTS; attempt++) {
    try {
      return await uploadEncryptedManifest(publicID, uploadToken, bytes, sha256, (loaded) =>
        onProgress(loaded),
      );
    } catch (error) {
      lastError = error;
      const status = await getChunkedUploadStatus(publicID, uploadToken).catch(() => null);
      if (
        status?.manifest?.sha256 === sha256 &&
        status.manifest.encrypted_size === bytes.byteLength
      ) {
        return { index: -1, encrypted_size: status.manifest.encrypted_size, sha256 };
      }
      if (attempt < MAX_UPLOAD_ATTEMPTS) {
        await delay(250 * 2 ** (attempt - 1));
      }
    }
  }
  throw lastError;
}

async function runLimited<T>(
  items: readonly T[],
  concurrency: number,
  worker: (item: T) => Promise<void>,
) {
  let cursor = 0;
  const runners = Array.from({ length: Math.min(concurrency, items.length) }, async () => {
    while (cursor < items.length) {
      const item = items[cursor++];
      await worker(item);
    }
  });
  await Promise.all(runners);
}

function delay(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function openUploadDB(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === "undefined") return null;

  const request = indexedDB.open(DB_NAME, DB_VERSION);
  request.onupgradeneeded = () => {
    const db = request.result;
    if (!db.objectStoreNames.contains(STORE_NAME)) {
      db.createObjectStore(STORE_NAME);
    }
  };

  return idbRequest<IDBDatabase>(request).catch(() => null);
}

function idbRequest<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}
