import { xchacha20poly1305 } from "@noble/ciphers/chacha.js";
import { type ChunkedEncryptionMaterial, chunkedAad, type KeySet } from "./encryption";

export const CHUNKED_STORAGE_VERSION = "chunked-v1";
export const DEFAULT_CHUNKED_PLAINTEXT_CHUNK_SIZE = 16 * 1024 * 1024;
export const CHUNKED_RECORD_OVERHEAD_BYTES = 24 + 16;
export const MAX_CHUNKED_MANIFEST_BYTES = 8 * 1024 * 1024;

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();
const NONCE_LENGTH = 24;

export interface ChunkedPlanChunk {
  readonly globalIndex: number;
  readonly fileIndex: number;
  readonly fileChunkIndex: number;
  readonly start: number;
  readonly end: number;
  readonly plaintextSize: number;
}

export interface ChunkedPlanFile {
  readonly index: number;
  readonly path: string;
  readonly name: string;
  readonly type: string;
  readonly size: number;
  readonly chunks: ChunkedPlanChunk[];
}

export interface ChunkedBundlePlan {
  readonly bundleName: string;
  readonly chunkSize: number;
  readonly files: ChunkedPlanFile[];
  readonly chunks: ChunkedPlanChunk[];
}

export interface ChunkedManifestChunk {
  readonly index: number;
  readonly global_index: number;
  readonly plaintext_size: number;
  readonly encrypted_size: number;
  readonly sha256: string;
}

export interface ChunkedManifestFile {
  readonly index: number;
  readonly path: string;
  readonly name: string;
  readonly type: string;
  readonly size: number;
  readonly chunks: ChunkedManifestChunk[];
}

export interface ChunkedManifest {
  readonly version: 1;
  readonly storage_version: "chunked-v1";
  readonly bundleName: string;
  readonly chunkSize: number;
  readonly chunkCount: number;
  readonly files: ChunkedManifestFile[];
}

export interface ChunkUploadRecord {
  readonly globalIndex: number;
  readonly encryptedSize: number;
  readonly sha256: string;
}

export interface DecryptedChunkedFile {
  readonly file: ChunkedManifestFile;
  readonly blob: Blob;
}

export type ChunkFetcher = (index: number) => Promise<Uint8Array>;

export function buildChunkedBundlePlan(
  files: File[],
  chunkSize = DEFAULT_CHUNKED_PLAINTEXT_CHUNK_SIZE,
  bundleName = defaultBundleName(files),
): ChunkedBundlePlan {
  if (files.length === 0) {
    throw new Error("bundle must contain at least one file");
  }
  if (!Number.isSafeInteger(chunkSize) || chunkSize <= 0) {
    throw new Error("invalid chunk size");
  }

  let globalIndex = 0;
  const planFiles: ChunkedPlanFile[] = [];
  const chunks: ChunkedPlanChunk[] = [];

  for (const [fileIndex, file] of files.entries()) {
    const fileChunks: ChunkedPlanChunk[] = [];
    for (
      let start = 0, fileChunkIndex = 0;
      start < file.size;
      start += chunkSize, fileChunkIndex++
    ) {
      const end = Math.min(start + chunkSize, file.size);
      const chunk = {
        globalIndex,
        fileIndex,
        fileChunkIndex,
        start,
        end,
        plaintextSize: end - start,
      };
      fileChunks.push(chunk);
      chunks.push(chunk);
      globalIndex++;
    }

    planFiles.push({
      index: fileIndex,
      path: filePath(file),
      name: file.name,
      type: file.type || "application/octet-stream",
      size: file.size,
      chunks: fileChunks,
    });
  }

  return { bundleName, chunkSize, files: planFiles, chunks };
}

export function buildChunkedManifest(
  plan: ChunkedBundlePlan,
  records: readonly ChunkUploadRecord[],
): ChunkedManifest {
  const recordsByIndex = new Map(records.map((record) => [record.globalIndex, record]));
  const files = plan.files.map((file) => ({
    index: file.index,
    path: file.path,
    name: file.name,
    type: file.type,
    size: file.size,
    chunks: file.chunks.map((chunk) => {
      const record = recordsByIndex.get(chunk.globalIndex);
      if (!record) {
        throw new Error("missing encrypted chunk record");
      }
      return {
        index: chunk.fileChunkIndex,
        global_index: chunk.globalIndex,
        plaintext_size: chunk.plaintextSize,
        encrypted_size: record.encryptedSize,
        sha256: record.sha256,
      };
    }),
  }));

  return {
    version: 1,
    storage_version: CHUNKED_STORAGE_VERSION,
    bundleName: plan.bundleName,
    chunkSize: plan.chunkSize,
    chunkCount: plan.chunks.length,
    files,
  };
}

export function encryptChunkedManifest(manifest: ChunkedManifest, keySet: KeySet): Uint8Array {
  const plaintext = textEncoder.encode(JSON.stringify(manifest));
  if (plaintext.length > MAX_CHUNKED_MANIFEST_BYTES) {
    throw new Error("chunked manifest is too large");
  }
  return keySet.encryptChunkedPart(plaintext, manifestAad());
}

export function decryptChunkedManifest(
  encryptedManifest: Uint8Array,
  keySet: KeySet,
): ChunkedManifest {
  const plaintext = keySet.decryptChunkedPart(encryptedManifest, manifestAad());
  const manifest = JSON.parse(textDecoder.decode(plaintext)) as ChunkedManifest;
  validateChunkedManifest(manifest);
  return manifest;
}

export async function decryptChunkedFiles(
  manifest: ChunkedManifest,
  keySet: KeySet,
  fetchChunk: ChunkFetcher,
): Promise<DecryptedChunkedFile[]> {
  validateChunkedManifest(manifest);

  const chunkRefs = manifest.files
    .flatMap((file) => file.chunks.map((chunk) => ({ file, chunk })))
    .sort((a, b) => a.chunk.global_index - b.chunk.global_index);
  const encryptedChunks = new Map<number, Uint8Array>();

  for (const { chunk } of chunkRefs) {
    const encrypted = await fetchChunk(chunk.global_index);
    if (encrypted.length !== chunk.encrypted_size) {
      throw new Error("chunk size mismatch");
    }
    const actualHash = await sha256Hex(encrypted);
    if (actualHash !== chunk.sha256) {
      throw new Error("chunk hash mismatch");
    }
    encryptedChunks.set(chunk.global_index, encrypted);
  }

  const partsByFile = manifest.files.map(() => [] as BlobPart[]);
  for (const { file, chunk } of chunkRefs) {
    const encrypted = encryptedChunks.get(chunk.global_index);
    if (!encrypted) {
      throw new Error("missing encrypted chunk");
    }
    const plaintext = keySet.decryptChunkedPart(
      encrypted,
      chunkAad(file.index, chunk.index, chunk.global_index, chunk.plaintext_size),
    );
    if (plaintext.length !== chunk.plaintext_size) {
      throw new Error("chunk plaintext size mismatch");
    }
    partsByFile[file.index].push(toArrayBuffer(plaintext));
  }

  return manifest.files.map((file, index) => ({
    file,
    blob: new Blob(partsByFile[index], { type: file.type || "application/octet-stream" }),
  }));
}

export function chunkAad(
  fileIndex: number,
  fileChunkIndex: number,
  globalChunkIndex: number,
  plaintextSize: number,
): Uint8Array {
  return textEncoder.encode(
    `chunk:${fileIndex}:${fileChunkIndex}:${globalChunkIndex}:${plaintextSize}`,
  );
}

export function manifestAad(): Uint8Array {
  return textEncoder.encode("manifest:1");
}

export function estimateChunkedEncryptedSize(
  fileSizes: number[],
  chunkSize = DEFAULT_CHUNKED_PLAINTEXT_CHUNK_SIZE,
): number {
  const chunkCount = fileSizes.reduce(
    (count, size) => count + (size > 0 ? Math.ceil(size / chunkSize) : 0),
    0,
  );
  const plaintextBytes = fileSizes.reduce((sum, size) => sum + size, 0);
  return (
    plaintextBytes +
    chunkCount * CHUNKED_RECORD_OVERHEAD_BYTES +
    MAX_CHUNKED_MANIFEST_BYTES +
    CHUNKED_RECORD_OVERHEAD_BYTES
  );
}

export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", toArrayBuffer(bytes));
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

export function defaultEncryptionWorkerCount(): number {
  const hardwareConcurrency = navigator.hardwareConcurrency || 1;
  return Math.min(3, Math.max(1, hardwareConcurrency - 1));
}

export class ChunkedEncryptionPool {
  private readonly material: ChunkedEncryptionMaterial;
  private workers: Worker[] = [];
  private queue: Array<{
    plaintext: Uint8Array;
    aadSuffix: Uint8Array;
    resolve: (encrypted: Uint8Array) => void;
    reject: (error: Error) => void;
  }> = [];
  private idleWorkers: Worker[] = [];
  private nextID = 1;
  private fallback = false;
  private pending = new Map<
    number,
    {
      resolve: (encrypted: Uint8Array) => void;
      reject: (error: Error) => void;
      worker: Worker;
    }
  >();

  constructor(material: ChunkedEncryptionMaterial, workerCount = defaultEncryptionWorkerCount()) {
    this.material = material;
    if (typeof Worker === "undefined") {
      this.fallback = true;
      return;
    }

    try {
      for (let i = 0; i < workerCount; i++) {
        const worker = new Worker(new URL("./chunkedCrypto.worker.ts", import.meta.url), {
          type: "module",
        });
        worker.onmessage = (event) => this.handleWorkerMessage(event);
        worker.onerror = () => {
          this.failWorker(worker, new Error("chunk encryption worker failed"));
        };
        this.workers.push(worker);
        this.idleWorkers.push(worker);
      }
      this.fallback = this.workers.length === 0;
    } catch {
      this.destroy();
      this.fallback = true;
    }
  }

  encrypt(plaintext: Uint8Array, aadSuffix: Uint8Array): Promise<Uint8Array> {
    if (this.fallback) {
      return Promise.resolve(encryptWithMaterial(this.material, plaintext, aadSuffix));
    }

    return new Promise((resolve, reject) => {
      this.queue.push({ plaintext, aadSuffix, resolve, reject });
      this.pump();
    });
  }

  destroy() {
    for (const worker of this.workers) {
      worker.terminate();
    }
    this.workers = [];
    this.idleWorkers = [];
    this.pending.clear();
    this.queue = [];
  }

  private pump() {
    while (this.queue.length > 0 && this.idleWorkers.length > 0) {
      const worker = this.idleWorkers.pop();
      const task = this.queue.shift();
      if (!worker || !task) return;

      const id = this.nextID++;
      this.pending.set(id, { resolve: task.resolve, reject: task.reject, worker });
      worker.postMessage({
        id,
        key: this.material.blobKey,
        publicID: this.material.publicID,
        plaintext: task.plaintext,
        aadSuffix: task.aadSuffix,
      });
    }
  }

  private handleWorkerMessage(event: MessageEvent) {
    const { id, encrypted, error } = event.data as {
      id: number;
      encrypted?: Uint8Array;
      error?: string;
    };
    const task = this.pending.get(id);
    if (!task) return;

    this.pending.delete(id);
    this.idleWorkers.push(task.worker);
    if (error) {
      task.reject(new Error(error));
    } else if (encrypted) {
      task.resolve(encrypted);
    } else {
      task.reject(new Error("chunk encryption worker returned no data"));
    }
    this.pump();
  }

  private failWorker(worker: Worker, error: Error) {
    for (const [id, task] of this.pending) {
      if (task.worker === worker) {
        this.pending.delete(id);
        task.reject(error);
      }
    }
    this.workers = this.workers.filter((item) => item !== worker);
    this.idleWorkers = this.idleWorkers.filter((item) => item !== worker);
    worker.terminate();
    if (this.workers.length === 0) {
      this.fallback = true;
    }
  }
}

function encryptWithMaterial(
  material: ChunkedEncryptionMaterial,
  plaintext: Uint8Array,
  aadSuffix: Uint8Array,
): Uint8Array {
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_LENGTH));
  const cipher = xchacha20poly1305(
    material.blobKey,
    nonce,
    chunkedAad(material.publicID, aadSuffix),
  );
  const ciphertext = cipher.encrypt(plaintext);
  const encrypted = new Uint8Array(nonce.length + ciphertext.length);
  encrypted.set(nonce, 0);
  encrypted.set(ciphertext, nonce.length);
  return encrypted;
}

function validateChunkedManifest(manifest: ChunkedManifest) {
  if (
    manifest.version !== 1 ||
    manifest.storage_version !== CHUNKED_STORAGE_VERSION ||
    !manifest.bundleName ||
    !Number.isSafeInteger(manifest.chunkSize) ||
    manifest.chunkSize <= 0 ||
    !Number.isSafeInteger(manifest.chunkCount) ||
    manifest.chunkCount < 0 ||
    !Array.isArray(manifest.files) ||
    manifest.files.length === 0
  ) {
    throw new Error("invalid chunked manifest");
  }

  const seenChunks = new Set<number>();
  for (const [fileIndex, file] of manifest.files.entries()) {
    if (
      file.index !== fileIndex ||
      !file.path ||
      !file.name ||
      !Number.isSafeInteger(file.size) ||
      file.size < 0 ||
      !Array.isArray(file.chunks)
    ) {
      throw new Error("invalid chunked manifest");
    }

    let fileSize = 0;
    for (const [chunkIndex, chunk] of file.chunks.entries()) {
      if (
        chunk.index !== chunkIndex ||
        !Number.isSafeInteger(chunk.global_index) ||
        chunk.global_index < 0 ||
        !Number.isSafeInteger(chunk.plaintext_size) ||
        chunk.plaintext_size <= 0 ||
        chunk.plaintext_size > manifest.chunkSize ||
        !Number.isSafeInteger(chunk.encrypted_size) ||
        chunk.encrypted_size !== chunk.plaintext_size + CHUNKED_RECORD_OVERHEAD_BYTES ||
        !/^[0-9a-f]{64}$/.test(chunk.sha256) ||
        seenChunks.has(chunk.global_index)
      ) {
        throw new Error("invalid chunked manifest");
      }
      seenChunks.add(chunk.global_index);
      fileSize += chunk.plaintext_size;
    }

    if (fileSize !== file.size || (file.size === 0 && file.chunks.length !== 0)) {
      throw new Error("invalid chunked manifest");
    }
  }

  if (seenChunks.size !== manifest.chunkCount) {
    throw new Error("invalid chunked manifest");
  }
}

function filePath(file: File): string {
  const maybeRelative = (file as File & { webkitRelativePath?: string }).webkitRelativePath;
  return maybeRelative || file.name;
}

function defaultBundleName(files: File[]): string {
  if (files.length === 1) {
    return files[0]?.name || "Secretli file";
  }
  return `Secretli bundle (${files.length} files)`;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}
