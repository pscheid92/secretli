import type { KeySet } from "./encryption";

export const BUNDLE_HEADER_LENGTH = 64;
export const DEFAULT_BUNDLE_CHUNK_SIZE = 4 * 1024 * 1024;
export const BUNDLE_RECORD_OVERHEAD_BYTES = 24 + 16;
export const MAX_BUNDLE_COALESCED_PLAINTEXT_BYTES = 16 * 1024 * 1024;
export const DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES = 64 * 1024 * 1024;
export const MAX_BUNDLE_MANIFEST_BYTES = 256 * 1024;

const BUNDLE_MAGIC = new Uint8Array([0x53, 0x4c, 0x42, 0x4e, 0x44, 0x4c, 0x31, 0x00]);
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export interface BundleChunk {
  readonly index: number;
  readonly offset: number;
  readonly length: number;
  readonly plaintextSize: number;
}

export interface BundleFile {
  readonly index: number;
  readonly path: string;
  readonly name: string;
  readonly type: string;
  readonly size: number;
  readonly chunks: BundleChunk[];
}

export interface BundleManifest {
  readonly version: 1;
  readonly bundleName: string;
  readonly chunkSize: number;
  readonly files: BundleFile[];
}

export interface BundleHeader {
  readonly chunkSize: number;
  readonly manifestOffset: number;
  readonly manifestLength: number;
}

export interface DecryptedBundleFile {
  readonly file: BundleFile;
  readonly blob: Blob;
}

export interface DecryptBundleFilesOptions {
  readonly maxCoalescedPlaintextBytes?: number;
}

export type BundleRangeFetcher = (start: number, end: number) => Promise<Uint8Array>;

interface BundleChunkGroup {
  readonly chunks: BundleChunkRef[];
  readonly offset: number;
  length: number;
  plaintextSize: number;
}

interface BundleChunkRef {
  readonly file: BundleFile;
  readonly chunk: BundleChunk;
}

export async function createEncryptedBundle(
  files: File[],
  keySet: KeySet,
  bundleName = defaultBundleName(files),
): Promise<{ blob: Blob; manifest: BundleManifest }> {
  if (files.length === 0) {
    throw new Error("bundle must contain at least one file");
  }

  let offset = BUNDLE_HEADER_LENGTH;
  const parts: BlobPart[] = [];
  const bundleFiles: BundleFile[] = [];

  for (const [fileIndex, file] of files.entries()) {
    const path = filePath(file);
    const chunks: BundleChunk[] = [];

    for (
      let start = 0, chunkIndex = 0;
      start < file.size;
      start += DEFAULT_BUNDLE_CHUNK_SIZE, chunkIndex++
    ) {
      const end = Math.min(start + DEFAULT_BUNDLE_CHUNK_SIZE, file.size);
      const plaintext = new Uint8Array(await file.slice(start, end).arrayBuffer());
      const encrypted = keySet.encryptBundlePart(
        plaintext,
        chunkAad(fileIndex, chunkIndex, plaintext.length),
      );
      parts.push(toArrayBuffer(encrypted));
      chunks.push({
        index: chunkIndex,
        offset,
        length: encrypted.length,
        plaintextSize: plaintext.length,
      });
      offset += encrypted.length;
    }

    bundleFiles.push({
      index: fileIndex,
      path,
      name: file.name,
      type: file.type || "application/octet-stream",
      size: file.size,
      chunks,
    });
  }

  const manifest: BundleManifest = {
    version: 1,
    bundleName,
    chunkSize: DEFAULT_BUNDLE_CHUNK_SIZE,
    files: bundleFiles,
  };
  const manifestPlaintext = textEncoder.encode(JSON.stringify(manifest));
  if (manifestPlaintext.length > MAX_BUNDLE_MANIFEST_BYTES) {
    throw new Error("bundle manifest is too large");
  }

  const encryptedManifest = keySet.encryptBundlePart(manifestPlaintext, manifestAad());
  const header = buildBundleHeader({
    chunkSize: DEFAULT_BUNDLE_CHUNK_SIZE,
    manifestOffset: offset,
    manifestLength: encryptedManifest.length,
  });

  return {
    blob: new Blob([toArrayBuffer(header), ...parts, toArrayBuffer(encryptedManifest)], {
      type: "application/octet-stream",
    }),
    manifest,
  };
}

export async function readBundleManifest(
  fetchRange: BundleRangeFetcher,
  keySet: KeySet,
  bundleSize?: number,
): Promise<{ header: BundleHeader; manifest: BundleManifest }> {
  const header = parseBundleHeader(await fetchRange(0, BUNDLE_HEADER_LENGTH - 1));
  if (header.manifestLength > MAX_BUNDLE_MANIFEST_BYTES + BUNDLE_RECORD_OVERHEAD_BYTES) {
    throw new Error("bundle manifest is too large");
  }
  const manifestEnd = rangeEnd(header.manifestOffset, header.manifestLength);
  if (bundleSize !== undefined && manifestEnd >= bundleSize) {
    throw new Error("invalid bundle header");
  }
  const encryptedManifest = await fetchRange(header.manifestOffset, manifestEnd);
  const manifestPlaintext = keySet.decryptBundlePart(encryptedManifest, manifestAad());
  const manifest = JSON.parse(textDecoder.decode(manifestPlaintext)) as BundleManifest;
  validateManifest(manifest, header, bundleSize);
  return { header, manifest };
}

export async function decryptBundleFile(
  file: BundleFile,
  keySet: KeySet,
  fetchRange: BundleRangeFetcher,
): Promise<Blob> {
  const [decrypted] = await decryptBundleFiles([file], keySet, fetchRange);
  return decrypted.blob;
}

export async function decryptBundleFiles(
  files: readonly BundleFile[],
  keySet: KeySet,
  fetchRange: BundleRangeFetcher,
  options: DecryptBundleFilesOptions = {},
): Promise<DecryptedBundleFile[]> {
  const partsByFile = files.map(() => [] as BlobPart[]);
  const fileIndexes = new Map(files.map((file, index) => [file.index, index]));
  const refs = files
    .flatMap((file) => file.chunks.map((chunk) => ({ file, chunk })))
    .sort((a, b) => a.chunk.offset - b.chunk.offset);
  const maxCoalescedPlaintextBytes =
    options.maxCoalescedPlaintextBytes ?? MAX_BUNDLE_COALESCED_PLAINTEXT_BYTES;

  for (const group of coalesceChunks(refs, maxCoalescedPlaintextBytes)) {
    const encryptedGroup = await fetchRange(group.offset, rangeEnd(group.offset, group.length));
    if (encryptedGroup.length !== group.length) {
      throw new Error("bundle range size mismatch");
    }

    let cursor = 0;
    for (const { file, chunk } of group.chunks) {
      const encrypted = encryptedGroup.subarray(cursor, cursor + chunk.length);
      const plaintext = keySet.decryptBundlePart(
        encrypted,
        chunkAad(file.index, chunk.index, chunk.plaintextSize),
      );
      if (plaintext.length !== chunk.plaintextSize) {
        throw new Error("bundle chunk size mismatch");
      }
      const fileIndex = fileIndexes.get(file.index);
      if (fileIndex === undefined) {
        throw new Error("invalid bundle file");
      }
      partsByFile[fileIndex].push(toArrayBuffer(plaintext));
      cursor += chunk.length;
    }
  }

  return files.map((file, index) => ({
    file,
    blob: new Blob(partsByFile[index], { type: file.type || "application/octet-stream" }),
  }));
}

export function estimateBundleEncryptedSize(fileSizes: number[]): number {
  const chunkCount = fileSizes.reduce(
    (count, size) => count + Math.ceil(size / DEFAULT_BUNDLE_CHUNK_SIZE),
    0,
  );
  const plaintextBytes = fileSizes.reduce((sum, size) => sum + size, 0);
  return (
    BUNDLE_HEADER_LENGTH +
    plaintextBytes +
    chunkCount * BUNDLE_RECORD_OVERHEAD_BYTES +
    MAX_BUNDLE_MANIFEST_BYTES +
    BUNDLE_RECORD_OVERHEAD_BYTES
  );
}

function buildBundleHeader(header: BundleHeader): Uint8Array {
  const bytes = new Uint8Array(BUNDLE_HEADER_LENGTH);
  bytes.set(BUNDLE_MAGIC, 0);
  const view = new DataView(bytes.buffer);
  view.setUint32(8, BUNDLE_HEADER_LENGTH, false);
  view.setUint32(12, header.chunkSize, false);
  setUint64(view, 16, header.manifestOffset);
  setUint64(view, 24, header.manifestLength);
  return bytes;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function parseBundleHeader(bytes: Uint8Array): BundleHeader {
  if (bytes.length !== BUNDLE_HEADER_LENGTH) {
    throw new Error("invalid bundle header");
  }
  for (let i = 0; i < BUNDLE_MAGIC.length; i++) {
    if (bytes[i] !== BUNDLE_MAGIC[i]) {
      throw new Error("invalid bundle header");
    }
  }
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (view.getUint32(8, false) !== BUNDLE_HEADER_LENGTH) {
    throw new Error("invalid bundle header");
  }
  const chunkSize = view.getUint32(12, false);
  const manifestOffset = getUint64(view, 16);
  const manifestLength = getUint64(view, 24);
  if (chunkSize <= 0 || manifestOffset < BUNDLE_HEADER_LENGTH || manifestLength <= 0) {
    throw new Error("invalid bundle header");
  }
  return { chunkSize, manifestOffset, manifestLength };
}

function validateManifest(manifest: BundleManifest, header: BundleHeader, bundleSize?: number) {
  if (
    manifest.version !== 1 ||
    manifest.chunkSize !== header.chunkSize ||
    !manifest.bundleName ||
    !Array.isArray(manifest.files) ||
    manifest.files.length === 0
  ) {
    throw new Error("invalid bundle manifest");
  }

  const ranges: Array<{ start: number; end: number }> = [];
  for (const [fileIndex, file] of manifest.files.entries()) {
    if (
      file.index !== fileIndex ||
      !file.path ||
      !file.name ||
      !Number.isSafeInteger(file.size) ||
      file.size < 0 ||
      !Array.isArray(file.chunks)
    ) {
      throw new Error("invalid bundle manifest");
    }

    let fileSize = 0;
    for (const [chunkIndex, chunk] of file.chunks.entries()) {
      if (
        chunk.index !== chunkIndex ||
        chunk.offset < BUNDLE_HEADER_LENGTH ||
        !Number.isSafeInteger(chunk.offset) ||
        !Number.isSafeInteger(chunk.length) ||
        !Number.isSafeInteger(chunk.plaintextSize) ||
        chunk.plaintextSize <= 0 ||
        chunk.plaintextSize > header.chunkSize ||
        chunk.length !== chunk.plaintextSize + BUNDLE_RECORD_OVERHEAD_BYTES
      ) {
        throw new Error("invalid bundle manifest");
      }
      const end = rangeEnd(chunk.offset, chunk.length);
      if (end >= header.manifestOffset || (bundleSize !== undefined && end >= bundleSize)) {
        throw new Error("invalid bundle manifest");
      }
      ranges.push({ start: chunk.offset, end });
      fileSize += chunk.plaintextSize;
    }

    if (fileSize !== file.size || (file.size === 0 && file.chunks.length !== 0)) {
      throw new Error("invalid bundle manifest");
    }
  }

  ranges.sort((a, b) => a.start - b.start);
  for (let i = 1; i < ranges.length; i++) {
    if (ranges[i].start <= ranges[i - 1].end) {
      throw new Error("invalid bundle manifest");
    }
  }
}

function coalesceChunks(
  chunks: readonly BundleChunkRef[],
  maxPlaintextBytes: number,
): BundleChunkGroup[] {
  const groups: BundleChunkGroup[] = [];

  for (const chunkRef of chunks) {
    const { chunk } = chunkRef;
    const previous = groups.at(-1);
    if (
      !previous ||
      chunk.offset !== previous.offset + previous.length ||
      previous.plaintextSize + chunk.plaintextSize > maxPlaintextBytes
    ) {
      groups.push({
        chunks: [chunkRef],
        offset: chunk.offset,
        length: chunk.length,
        plaintextSize: chunk.plaintextSize,
      });
      continue;
    }

    previous.chunks.push(chunkRef);
    previous.length += chunk.length;
    previous.plaintextSize += chunk.plaintextSize;
  }

  return groups;
}

function setUint64(view: DataView, offset: number, value: number) {
  if (!Number.isSafeInteger(value) || value < 0) {
    throw new Error("invalid bundle integer");
  }
  view.setBigUint64(offset, BigInt(value), false);
}

function getUint64(view: DataView, offset: number): number {
  const value = Number(view.getBigUint64(offset, false));
  if (!Number.isSafeInteger(value)) {
    throw new Error("invalid bundle integer");
  }
  return value;
}

function rangeEnd(offset: number, length: number): number {
  if (!Number.isSafeInteger(offset) || !Number.isSafeInteger(length) || length <= 0) {
    throw new Error("invalid bundle integer");
  }
  const end = offset + length - 1;
  if (!Number.isSafeInteger(end) || end < offset) {
    throw new Error("invalid bundle integer");
  }
  return end;
}

function manifestAad(): Uint8Array {
  return textEncoder.encode("manifest");
}

function chunkAad(fileIndex: number, chunkIndex: number, plaintextSize: number): Uint8Array {
  return textEncoder.encode(`chunk:${fileIndex}:${chunkIndex}:${plaintextSize}`);
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
