import {
  BUNDLE_HEADER_LENGTH,
  type BundleFile,
  createEncryptedBundle,
  DEFAULT_BUNDLE_CHUNK_SIZE,
  DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES,
  decryptBundleFiles,
  readBundleManifest,
} from "../bundle";
import { KeySet } from "../encryption";

async function blobBytes(blob: Blob): Promise<Uint8Array> {
  return new Uint8Array(await blob.arrayBuffer());
}

describe("encrypted bundles", () => {
  it("rejects empty bundles", async () => {
    const keySet = await KeySet.generateRandom();

    await expect(createEncryptedBundle([], keySet)).rejects.toThrow("at least one file");
  });

  it("round-trips a single file through manifest and chunk ranges", async () => {
    const keySet = await KeySet.generateRandom();
    const file = new File(["hello bundle"], "notes.txt", { type: "text/plain" });
    const { blob } = await createEncryptedBundle([file], keySet);
    const bytes = await blobBytes(blob);
    const fetchRange = async (start: number, end: number) => bytes.slice(start, end + 1);

    const { manifest } = await readBundleManifest(fetchRange, keySet);
    expect(manifest.files).toHaveLength(1);
    expect(manifest.files[0].path).toBe("notes.txt");

    const [{ blob: decrypted }] = await decryptBundleFiles(manifest.files, keySet, fetchRange);
    await expect(decrypted.text()).resolves.toBe("hello bundle");
  });

  it("uses 4 MiB chunks for large bundle files", async () => {
    const keySet = await KeySet.generateRandom();
    const payload = new Uint8Array(DEFAULT_BUNDLE_CHUNK_SIZE + 1);
    const file = new File([payload], "large.bin");
    const { blob } = await createEncryptedBundle([file], keySet);
    const bytes = await blobBytes(blob);
    const fetchRange = async (start: number, end: number) => bytes.slice(start, end + 1);

    const { manifest } = await readBundleManifest(fetchRange, keySet);

    expect(DEFAULT_BUNDLE_CHUNK_SIZE).toBe(4 * 1024 * 1024);
    expect(manifest.chunkSize).toBe(DEFAULT_BUNDLE_CHUNK_SIZE);
    expect(manifest.files[0].chunks.map((chunk) => chunk.plaintextSize)).toEqual([
      DEFAULT_BUNDLE_CHUNK_SIZE,
      1,
    ]);
  });

  it("coalesces consecutive chunks into bounded range fetches", async () => {
    const keySet = await KeySet.generateRandom();
    const payload = new Uint8Array(DEFAULT_BUNDLE_CHUNK_SIZE * 4 + 1);
    payload[0] = 1;
    payload[DEFAULT_BUNDLE_CHUNK_SIZE] = 2;
    payload[payload.length - 1] = 3;
    const file = new File([payload], "large.bin");
    const { blob } = await createEncryptedBundle([file], keySet);
    const bytes = await blobBytes(blob);
    const fetchRange = async (start: number, end: number) => bytes.slice(start, end + 1);
    const { manifest } = await readBundleManifest(fetchRange, keySet);
    const fileManifest = manifest.files[0];
    const ranges: Array<[number, number]> = [];
    const recordingRange = async (start: number, end: number) => {
      ranges.push([start, end]);
      return bytes.slice(start, end + 1);
    };

    const [{ blob: decrypted }] = await decryptBundleFiles([fileManifest], keySet, recordingRange);
    const decryptedBytes = await blobBytes(decrypted);

    expect(fileManifest.chunks).toHaveLength(5);
    expect(ranges).toEqual([
      [
        fileManifest.chunks[0].offset,
        fileManifest.chunks[3].offset + fileManifest.chunks[3].length - 1,
      ],
      [
        fileManifest.chunks[4].offset,
        fileManifest.chunks[4].offset + fileManifest.chunks[4].length - 1,
      ],
    ]);
    expect(decryptedBytes[0]).toBe(1);
    expect(decryptedBytes[DEFAULT_BUNDLE_CHUNK_SIZE]).toBe(2);
    expect(decryptedBytes[decryptedBytes.length - 1]).toBe(3);
  });

  it("uses the download-all coalescing option", async () => {
    const ranges: Array<[number, number]> = [];
    const file: BundleFile = {
      index: 0,
      path: "alpha.bin",
      name: "alpha.bin",
      type: "application/octet-stream",
      size: 6,
      chunks: Array.from({ length: 6 }, (_, index) => ({
        index,
        offset: BUNDLE_HEADER_LENGTH + index,
        length: 1,
        plaintextSize: 1,
      })),
    };
    const fakeKeySet = {
      decryptBundlePart: () => new Uint8Array([1]),
    } as unknown as KeySet;
    const recordingRange = async (start: number, end: number) => {
      ranges.push([start, end]);
      return new Uint8Array(end - start + 1);
    };

    const decrypted = await decryptBundleFiles([file], fakeKeySet, recordingRange, {
      maxCoalescedPlaintextBytes: 4,
    });

    expect(DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES).toBe(64 * 1024 * 1024);
    expect(ranges).toEqual([
      [BUNDLE_HEADER_LENGTH, BUNDLE_HEADER_LENGTH + 3],
      [BUNDLE_HEADER_LENGTH + 4, BUNDLE_HEADER_LENGTH + 5],
    ]);
    await expect(decrypted[0].blob.arrayBuffer()).resolves.toHaveProperty("byteLength", 6);
  });

  it("rejects a manifest range outside the reported bundle size", async () => {
    const keySet = await KeySet.generateRandom();
    const file = new File(["hello bundle"], "notes.txt", { type: "text/plain" });
    const { blob } = await createEncryptedBundle([file], keySet);
    const bytes = await blobBytes(blob);
    const fetchRange = async (start: number, end: number) => bytes.slice(start, end + 1);

    await expect(readBundleManifest(fetchRange, keySet, bytes.length - 1)).rejects.toThrow(
      "invalid bundle header",
    );
  });

  it("round-trips multiple files without storing a zip", async () => {
    const keySet = await KeySet.generateRandom();
    const files = [
      new File(["alpha"], "a.txt", { type: "text/plain" }),
      new File(["bravo"], "b.txt", { type: "text/plain" }),
    ];
    const { blob } = await createEncryptedBundle(files, keySet);
    const bytes = await blobBytes(blob);
    const fetchRange = async (start: number, end: number) => bytes.slice(start, end + 1);

    const { manifest } = await readBundleManifest(fetchRange, keySet);
    expect(manifest.files.map((file) => file.path)).toEqual(["a.txt", "b.txt"]);

    const decrypted = await decryptBundleFiles(manifest.files, keySet, fetchRange);
    await expect(decrypted[0].blob.text()).resolves.toBe("alpha");
    await expect(decrypted[1].blob.text()).resolves.toBe("bravo");
  });

  it("rejects tampered chunks", async () => {
    const keySet = await KeySet.generateRandom();
    const file = new File(["hello bundle"], "notes.txt");
    const { blob } = await createEncryptedBundle([file], keySet);
    const bytes = await blobBytes(blob);
    const fetchRange = async (start: number, end: number) => bytes.slice(start, end + 1);
    const { manifest } = await readBundleManifest(fetchRange, keySet);
    const chunk = manifest.files[0].chunks[0];
    const tampered = bytes.slice();
    tampered[chunk.offset] ^= 1;
    const tamperedRange = async (start: number, end: number) => tampered.slice(start, end + 1);

    await expect(decryptBundleFiles([manifest.files[0]], keySet, tamperedRange)).rejects.toThrow();
  });
});
