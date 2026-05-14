import {
  buildChunkedBundlePlan,
  buildChunkedManifest,
  chunkAad,
  decryptChunkedFiles,
  decryptChunkedManifest,
  encryptChunkedManifest,
  sha256Hex,
} from "../chunkedBundle";
import { KeySet } from "../encryption";

async function encryptPlanChunks(files: File[], keySet: KeySet, chunkSize = 4) {
  const plan = buildChunkedBundlePlan(files, chunkSize);
  const encryptedChunks = new Map<number, Uint8Array>();
  const records = [];

  for (const chunk of plan.chunks) {
    const file = files[chunk.fileIndex];
    const plaintext = new Uint8Array(await file.slice(chunk.start, chunk.end).arrayBuffer());
    const encrypted = keySet.encryptChunkedPart(
      plaintext,
      chunkAad(chunk.fileIndex, chunk.fileChunkIndex, chunk.globalIndex, chunk.plaintextSize),
    );
    encryptedChunks.set(chunk.globalIndex, encrypted);
    records.push({
      globalIndex: chunk.globalIndex,
      encryptedSize: encrypted.length,
      sha256: await sha256Hex(encrypted),
    });
  }

  const manifest = buildChunkedManifest(plan, records);
  const encryptedManifest = encryptChunkedManifest(manifest, keySet);
  return { manifest, encryptedManifest, encryptedChunks };
}

describe("chunked encrypted bundles", () => {
  it("round-trips a single file", async () => {
    const keySet = await KeySet.generateRandom();
    const file = new File(["hello chunked bundle"], "notes.txt", { type: "text/plain" });
    const { encryptedManifest, encryptedChunks } = await encryptPlanChunks([file], keySet);

    const manifest = decryptChunkedManifest(encryptedManifest, keySet);
    expect(manifest.storage_version).toBe("chunked-v1");
    expect(manifest.files[0].path).toBe("notes.txt");

    const [{ blob }] = await decryptChunkedFiles(manifest, keySet, async (index) => {
      const chunk = encryptedChunks.get(index);
      if (!chunk) throw new Error("missing test chunk");
      return chunk;
    });
    await expect(blob.text()).resolves.toBe("hello chunked bundle");
  });

  it("round-trips multiple files and zero-byte files", async () => {
    const keySet = await KeySet.generateRandom();
    const files = [
      new File(["alpha"], "a.txt", { type: "text/plain" }),
      new File([], "empty.txt", { type: "text/plain" }),
      new File(["bravo"], "b.txt", { type: "text/plain" }),
    ];
    const { encryptedManifest, encryptedChunks } = await encryptPlanChunks(files, keySet, 3);
    const manifest = decryptChunkedManifest(encryptedManifest, keySet);

    expect(manifest.files.map((file) => file.path)).toEqual(["a.txt", "empty.txt", "b.txt"]);
    expect(manifest.files[1].chunks).toHaveLength(0);

    const decrypted = await decryptChunkedFiles(manifest, keySet, async (index) => {
      const chunk = encryptedChunks.get(index);
      if (!chunk) throw new Error("missing test chunk");
      return chunk;
    });

    await expect(decrypted[0].blob.text()).resolves.toBe("alpha");
    await expect(decrypted[1].blob.arrayBuffer()).resolves.toHaveProperty("byteLength", 0);
    await expect(decrypted[2].blob.text()).resolves.toBe("bravo");
  });

  it("rejects tampered manifests", async () => {
    const keySet = await KeySet.generateRandom();
    const { encryptedManifest } = await encryptPlanChunks([new File(["alpha"], "a.txt")], keySet);
    const tampered = encryptedManifest.slice();
    tampered[tampered.length - 1] ^= 1;

    expect(() => decryptChunkedManifest(tampered, keySet)).toThrow();
  });

  it("rejects tampered chunks before saving files", async () => {
    const keySet = await KeySet.generateRandom();
    const { encryptedManifest, encryptedChunks } = await encryptPlanChunks(
      [new File(["alpha"], "a.txt")],
      keySet,
    );
    const manifest = decryptChunkedManifest(encryptedManifest, keySet);
    const tampered = encryptedChunks.get(0)?.slice();
    if (!tampered) throw new Error("missing test chunk");
    tampered[0] ^= 1;

    await expect(decryptChunkedFiles(manifest, keySet, async () => tampered)).rejects.toThrow(
      "chunk hash mismatch",
    );
  });
});
