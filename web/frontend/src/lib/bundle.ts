import type { KeySet } from "./encryption";

export const BUNDLE_V2_FOOTER_LENGTH = 64;
export const DEFAULT_BUNDLE_CHUNK_SIZE = 4 * 1024 * 1024;
export const BUNDLE_RECORD_OVERHEAD_BYTES = 24 + 16;
export const MAX_BUNDLE_COALESCED_PLAINTEXT_BYTES = 16 * 1024 * 1024;
export const DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES = 64 * 1024 * 1024;
export const MAX_BUNDLE_MANIFEST_BYTES = 256 * 1024;

const BUNDLE_V2_MAGIC = new Uint8Array([0x53, 0x4c, 0x42, 0x4e, 0x44, 0x4c, 0x32, 0x00]);
const BUNDLE_V2_VERSION = 2;
const SHA256_HEX_LENGTH = 64;
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export interface BundleChunk {
  readonly index: number;
  readonly offset: number;
  readonly length: number;
  readonly plaintextSize: number;
  readonly sha256?: string;
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
  readonly version: 2;
  readonly bundleName: string;
  readonly chunkSize: number;
  readonly files: BundleFile[];
}

export interface BundleV2Footer {
  readonly version: 2;
  readonly footerLength: number;
  readonly manifestLength: number;
  readonly manifestSha256: string;
}

export interface BundleV2RecordPlan {
  readonly fileIndex: number;
  readonly chunkIndex: number;
  readonly start: number;
  readonly end: number;
  readonly offset: number;
  readonly length: number;
  readonly plaintextSize: number;
}

export interface BundleV2Plan {
  readonly bundleName: string;
  readonly files: File[];
  readonly manifest: BundleManifest;
  readonly records: BundleV2RecordPlan[];
  readonly dataSize: number;
  readonly encryptedManifestLength: number;
  readonly totalSize: number;
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
  return createEncryptedBundleV2(files, keySet, bundleName);
}

export async function createEncryptedBundleV2(
  files: File[],
  keySet: KeySet,
  bundleName = defaultBundleName(files),
): Promise<{ blob: Blob; manifest: BundleManifest; footer: BundleV2Footer }> {
  const plan = planEncryptedBundleV2(files, bundleName);
  const { parts, manifest, encryptedManifest, footer } = await encryptBundleV2Plan(plan, keySet);

  return {
    blob: new Blob(
      [...parts, toArrayBuffer(encryptedManifest), toArrayBuffer(buildBundleV2Footer(footer))],
      {
        type: "application/octet-stream",
      },
    ),
    manifest,
    footer,
  };
}

export function planEncryptedBundleV2(
  files: File[],
  bundleName = defaultBundleName(files),
): BundleV2Plan {
  if (files.length === 0) {
    throw new Error("bundle must contain at least one file");
  }

  let offset = 0;
  const records: BundleV2RecordPlan[] = [];
  const bundleFiles: BundleFile[] = [];

  for (const [fileIndex, file] of files.entries()) {
    const chunks: BundleChunk[] = [];
    for (
      let start = 0, chunkIndex = 0;
      start < file.size;
      start += DEFAULT_BUNDLE_CHUNK_SIZE, chunkIndex++
    ) {
      const end = Math.min(start + DEFAULT_BUNDLE_CHUNK_SIZE, file.size);
      const plaintextSize = end - start;
      const length = plaintextSize + BUNDLE_RECORD_OVERHEAD_BYTES;
      records.push({ fileIndex, chunkIndex, start, end, offset, length, plaintextSize });
      chunks.push({
        index: chunkIndex,
        offset,
        length,
        plaintextSize,
        sha256: placeholderRecordSHA256(),
      });
      offset += length;
    }

    bundleFiles.push({
      index: fileIndex,
      path: filePath(file),
      name: file.name,
      type: file.type || "application/octet-stream",
      size: file.size,
      chunks,
    });
  }

  const manifest: BundleManifest = {
    version: 2,
    bundleName,
    chunkSize: DEFAULT_BUNDLE_CHUNK_SIZE,
    files: bundleFiles,
  };
  const manifestPlaintext = textEncoder.encode(JSON.stringify(manifest));
  if (manifestPlaintext.length > MAX_BUNDLE_MANIFEST_BYTES) {
    throw new Error("bundle manifest is too large");
  }

  const encryptedManifestLength = manifestPlaintext.length + BUNDLE_RECORD_OVERHEAD_BYTES;
  return {
    bundleName,
    files,
    manifest,
    records,
    dataSize: offset,
    encryptedManifestLength,
    totalSize: offset + encryptedManifestLength + BUNDLE_V2_FOOTER_LENGTH,
  };
}

export async function encryptBundleV2Plan(
  plan: BundleV2Plan,
  keySet: KeySet,
): Promise<{
  parts: ArrayBuffer[];
  manifest: BundleManifest;
  encryptedManifest: Uint8Array;
  footer: BundleV2Footer;
}> {
  const parts: ArrayBuffer[] = [];
  const files = mutableManifestFiles(plan.manifest.files);

  for (const record of plan.records) {
    const file = plan.files[record.fileIndex];
    const plaintext = new Uint8Array(await file.slice(record.start, record.end).arrayBuffer());
    if (plaintext.length !== record.plaintextSize) {
      throw new Error("bundle file changed during encryption");
    }

    const encrypted = keySet.encryptBundlePartDeterministic(
      plaintext,
      chunkAad(record.fileIndex, record.chunkIndex, record.plaintextSize),
      `record:${record.fileIndex}:${record.chunkIndex}:${record.plaintextSize}`,
    );
    if (encrypted.length !== record.length) {
      throw new Error("bundle record size mismatch");
    }
    files[record.fileIndex].chunks[record.chunkIndex] = {
      ...files[record.fileIndex].chunks[record.chunkIndex],
      sha256: await sha256Hex(encrypted),
    };
    parts.push(toArrayBuffer(encrypted));
  }

  const manifest: BundleManifest = { ...plan.manifest, files };
  const manifestPlaintext = textEncoder.encode(JSON.stringify(manifest));
  if (manifestPlaintext.length > MAX_BUNDLE_MANIFEST_BYTES) {
    throw new Error("bundle manifest is too large");
  }

  const encryptedManifest = keySet.encryptBundlePartDeterministic(
    manifestPlaintext,
    manifestAadV2(),
    "manifest:v2",
  );
  if (encryptedManifest.length !== plan.encryptedManifestLength) {
    throw new Error("bundle manifest size mismatch");
  }

  const footer: BundleV2Footer = {
    version: 2,
    footerLength: BUNDLE_V2_FOOTER_LENGTH,
    manifestLength: encryptedManifest.length,
    manifestSha256: await sha256Hex(encryptedManifest),
  };

  return { parts, manifest, encryptedManifest, footer };
}

export async function readBundleManifest(
  fetchRange: BundleRangeFetcher,
  keySet: KeySet,
  bundleSize: number,
): Promise<{ footer: BundleV2Footer; manifest: BundleManifest }> {
  if (bundleSize < BUNDLE_V2_FOOTER_LENGTH) {
    throw new Error("invalid bundle footer");
  }

  const footerStart = bundleSize - BUNDLE_V2_FOOTER_LENGTH;
  const footerBytes = await fetchRange(footerStart, bundleSize - 1);
  return readBundleManifestV2(fetchRange, keySet, bundleSize, footerBytes);
}

export async function readBundleManifestV2(
  fetchRange: BundleRangeFetcher,
  keySet: KeySet,
  bundleSize: number,
  footerBytes?: Uint8Array,
): Promise<{ footer: BundleV2Footer; manifest: BundleManifest }> {
  if (bundleSize < BUNDLE_V2_FOOTER_LENGTH) {
    throw new Error("invalid bundle footer");
  }

  const footer =
    footerBytes === undefined
      ? parseBundleV2Footer(await fetchRange(bundleSize - BUNDLE_V2_FOOTER_LENGTH, bundleSize - 1))
      : parseBundleV2Footer(footerBytes);
  if (footer.manifestLength > MAX_BUNDLE_MANIFEST_BYTES + BUNDLE_RECORD_OVERHEAD_BYTES) {
    throw new Error("bundle manifest is too large");
  }

  const manifestOffset = bundleSize - BUNDLE_V2_FOOTER_LENGTH - footer.manifestLength;
  if (manifestOffset < 0) {
    throw new Error("invalid bundle footer");
  }
  const encryptedManifest = await fetchRange(
    manifestOffset,
    manifestOffset + footer.manifestLength - 1,
  );
  if ((await sha256Hex(encryptedManifest)) !== footer.manifestSha256) {
    throw new Error("invalid bundle manifest hash");
  }

  const manifestPlaintext = keySet.decryptBundlePart(encryptedManifest, manifestAadV2());
  const manifest = JSON.parse(textDecoder.decode(manifestPlaintext)) as BundleManifest;
  validateManifestV2(manifest, manifestOffset, bundleSize);
  return { footer, manifest };
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
      if (chunk.sha256 !== undefined && (await sha256Hex(encrypted)) !== chunk.sha256) {
        throw new Error("bundle chunk hash mismatch");
      }
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
    plaintextBytes +
    chunkCount * BUNDLE_RECORD_OVERHEAD_BYTES +
    MAX_BUNDLE_MANIFEST_BYTES +
    BUNDLE_RECORD_OVERHEAD_BYTES +
    BUNDLE_V2_FOOTER_LENGTH
  );
}

export function bundleNameForFiles(files: File[]): string {
  return defaultBundleName(files);
}

export function bundleRecordAad(
  fileIndex: number,
  chunkIndex: number,
  plaintextSize: number,
): Uint8Array {
  return chunkAad(fileIndex, chunkIndex, plaintextSize);
}

export function bundleManifestAadV2(): Uint8Array {
  return manifestAadV2();
}

export function parseBundleV2Footer(bytes: Uint8Array): BundleV2Footer {
  if (bytes.length !== BUNDLE_V2_FOOTER_LENGTH || !hasBundleV2FooterMagic(bytes)) {
    throw new Error("invalid bundle footer");
  }
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const version = view.getUint32(8, false);
  const footerLength = view.getUint32(12, false);
  const manifestLength = getUint64(view, 16);
  const manifestSha256 = bytesToHex(bytes.slice(24, 56));
  if (
    version !== BUNDLE_V2_VERSION ||
    footerLength !== BUNDLE_V2_FOOTER_LENGTH ||
    manifestLength <= BUNDLE_RECORD_OVERHEAD_BYTES
  ) {
    throw new Error("invalid bundle footer");
  }
  return { version: 2, footerLength, manifestLength, manifestSha256 };
}

export function buildBundleV2Footer(footer: BundleV2Footer): Uint8Array {
  if (footer.version !== 2 || footer.footerLength !== BUNDLE_V2_FOOTER_LENGTH) {
    throw new Error("invalid bundle footer");
  }
  if (!isHexSHA256(footer.manifestSha256)) {
    throw new Error("invalid bundle footer");
  }

  const bytes = new Uint8Array(BUNDLE_V2_FOOTER_LENGTH);
  bytes.set(BUNDLE_V2_MAGIC, 0);
  const view = new DataView(bytes.buffer);
  view.setUint32(8, BUNDLE_V2_VERSION, false);
  view.setUint32(12, BUNDLE_V2_FOOTER_LENGTH, false);
  setUint64(view, 16, footer.manifestLength);
  bytes.set(hexToBytes(footer.manifestSha256), 24);
  return bytes;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function validateManifestV2(manifest: BundleManifest, manifestOffset: number, bundleSize: number) {
  if (
    manifest.version !== 2 ||
    manifest.chunkSize !== DEFAULT_BUNDLE_CHUNK_SIZE ||
    !manifest.bundleName ||
    !Array.isArray(manifest.files) ||
    manifest.files.length === 0 ||
    manifestOffset < 0 ||
    manifestOffset + BUNDLE_V2_FOOTER_LENGTH > bundleSize
  ) {
    throw new Error("invalid bundle manifest");
  }

  const ranges: Array<{ start: number; end: number }> = [];
  for (const [fileIndex, file] of manifest.files.entries()) {
    validateBundleFile(file, fileIndex);

    let fileSize = 0;
    for (const [chunkIndex, chunk] of file.chunks.entries()) {
      if (
        chunk.index !== chunkIndex ||
        chunk.offset < 0 ||
        !validBundleChunkShape(chunk, manifest.chunkSize) ||
        !isHexSHA256(chunk.sha256)
      ) {
        throw new Error("invalid bundle manifest");
      }
      const end = rangeEnd(chunk.offset, chunk.length);
      if (end >= manifestOffset) {
        throw new Error("invalid bundle manifest");
      }
      ranges.push({ start: chunk.offset, end });
      fileSize += chunk.plaintextSize;
    }

    validateBundleFileSize(file, fileSize);
  }

  validateContiguousRanges(ranges, manifestOffset);
}

function validateBundleFile(file: BundleFile, expectedIndex: number) {
  if (
    file.index !== expectedIndex ||
    !file.path ||
    !file.name ||
    !Number.isSafeInteger(file.size) ||
    file.size < 0 ||
    !Array.isArray(file.chunks)
  ) {
    throw new Error("invalid bundle manifest");
  }
}

function validBundleChunkShape(chunk: BundleChunk, chunkSize: number): boolean {
  return (
    Number.isSafeInteger(chunk.offset) &&
    Number.isSafeInteger(chunk.length) &&
    Number.isSafeInteger(chunk.plaintextSize) &&
    chunk.plaintextSize > 0 &&
    chunk.plaintextSize <= chunkSize &&
    chunk.length === chunk.plaintextSize + BUNDLE_RECORD_OVERHEAD_BYTES
  );
}

function validateBundleFileSize(file: BundleFile, fileSize: number) {
  if (fileSize !== file.size || (file.size === 0 && file.chunks.length !== 0)) {
    throw new Error("invalid bundle manifest");
  }
}

function validateContiguousRanges(
  ranges: Array<{ start: number; end: number }>,
  manifestOffset: number,
) {
  ranges.sort((a, b) => a.start - b.start);
  let expectedOffset = 0;
  for (const range of ranges) {
    if (range.start !== expectedOffset) {
      throw new Error("invalid bundle manifest");
    }
    expectedOffset = range.end + 1;
  }
  if (expectedOffset !== manifestOffset) {
    throw new Error("invalid bundle manifest");
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

function manifestAadV2(): Uint8Array {
  return textEncoder.encode("manifest:v2");
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

function mutableManifestFiles(files: readonly BundleFile[]): BundleFile[] {
  return files.map((file) => ({
    ...file,
    chunks: file.chunks.map((chunk) => ({ ...chunk })),
  }));
}

function placeholderRecordSHA256(): string {
  return "0".repeat(SHA256_HEX_LENGTH);
}

function hasBundleV2FooterMagic(bytes: Uint8Array): boolean {
  if (bytes.length !== BUNDLE_V2_FOOTER_LENGTH) {
    return false;
  }
  for (let i = 0; i < BUNDLE_V2_MAGIC.length; i++) {
    if (bytes[i] !== BUNDLE_V2_MAGIC[i]) {
      return false;
    }
  }
  return true;
}

export async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", toArrayBuffer(bytes));
  return bytesToHex(new Uint8Array(digest));
}

function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function hexToBytes(hex: string): Uint8Array {
  if (!isHexSHA256(hex)) {
    throw new Error("invalid hex string");
  }
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return bytes;
}

function isHexSHA256(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
}
