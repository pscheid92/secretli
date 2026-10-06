import { existsSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { base64UrlEncode } from "../base64";
import { decryptBundleFiles, readBundleManifest } from "../bundle";
import { KeySet, type SecretMeta } from "../encryption";
import { createEncryptedBundle } from "./bundleFixture";

/**
 * The Go implementation under internal/share and this one must read each
 * other's output. Each side commits a vectors file the other side's tests
 * decrypt. This side's file is written by
 *
 *   WRITE_VECTORS=1 pnpm vitest run src/lib/__tests__/vectors.test.ts
 *
 * and Go's by `go test ./internal/share -run TestWritesGoVectors -args -write-vectors`.
 */
const TESTDATA = path.resolve(__dirname, "../../../../../internal/share/testdata");

interface VectorFile {
  name: string;
  type: string;
  content_base64: string;
}

interface Vectors {
  share_secret: string;
  password: string;
  derived: {
    public_id: string;
    metadata_token: string;
    blob_token: string;
    password_blob_token: string;
  };
  meta: SecretMeta;
  encrypted_meta: string;
  files: VectorFile[];
  bundle_name: string;
  /** The whole encrypted bundle, sealed with the password-derived blob keys. */
  bundle_base64: string;
}

const SCRYPT = { timeout: 30_000 };

function toBase64(bytes: Uint8Array): string {
  return Buffer.from(bytes).toString("base64");
}

function fromBase64(text: string): Uint8Array {
  return new Uint8Array(Buffer.from(text, "base64"));
}

describe("cross-implementation vectors", () => {
  it("writes this implementation's vectors when asked", SCRYPT, async () => {
    if (!process.env.WRITE_VECTORS) return;

    const secret = new Uint8Array(32);
    for (let i = 0; i < secret.length; i++) secret[i] = i + 1;
    const shareSecret = base64UrlEncode(secret);
    const password = "correct horse battery staple";
    const base = await KeySet.fromShareSecret(shareSecret);
    const blob = await KeySet.fromShareSecret(shareSecret, password);

    const contents = [
      new TextEncoder().encode("hello from typescript\n"),
      new Uint8Array(0),
      new Uint8Array(Array.from({ length: 256 }, (_, i) => i)),
    ];
    const files: VectorFile[] = [
      { name: "hello.txt", type: "text/plain", content_base64: toBase64(contents[0]) },
      { name: "empty.bin", type: "application/octet-stream", content_base64: "" },
      {
        name: "bytes.bin",
        type: "application/octet-stream",
        content_base64: toBase64(contents[2]),
      },
    ];
    const fileObjects = files.map((f, i) => new File([contents[i]], f.name, { type: f.type }));
    const { blob: bundle, manifest } = await createEncryptedBundle(fileObjects, blob);
    const meta: SecretMeta = {
      type: "bundle",
      password_protected: true,
      bundle_name: manifest.bundleName,
    };
    const encoded = base.getEncoded();
    const vectors: Vectors = {
      share_secret: shareSecret,
      password,
      derived: {
        public_id: encoded.publicID,
        metadata_token: encoded.metadataToken,
        blob_token: encoded.blobToken,
        password_blob_token: blob.getEncoded().blobToken,
      },
      meta,
      encrypted_meta: await base.encryptMeta(meta),
      files,
      bundle_name: manifest.bundleName,
      bundle_base64: toBase64(new Uint8Array(await bundle.arrayBuffer())),
    };
    writeFileSync(path.join(TESTDATA, "ts-vectors.json"), `${JSON.stringify(vectors, null, 2)}\n`);
  });

  it("reads what the Go implementation wrote", SCRYPT, async () => {
    const file = path.join(TESTDATA, "go-vectors.json");
    if (!existsSync(file)) {
      throw new Error(`${file} is missing; see the comment at the top of this test`);
    }
    const v: Vectors = JSON.parse(readFileSync(file, "utf8"));

    const base = await KeySet.fromShareSecret(v.share_secret);
    const encoded = base.getEncoded();
    expect(encoded.publicID).toBe(v.derived.public_id);
    expect(encoded.metadataToken).toBe(v.derived.metadata_token);
    expect(encoded.blobToken).toBe(v.derived.blob_token);
    const blob = await KeySet.fromShareSecret(v.share_secret, v.password);
    expect(blob.getEncoded().blobToken).toBe(v.derived.password_blob_token);

    await expect(base.decryptMeta(v.encrypted_meta)).resolves.toEqual(v.meta);

    const bytes = fromBase64(v.bundle_base64);
    const fetchRange = async (start: number, end: number) => bytes.slice(start, end + 1);
    const { manifest } = await readBundleManifest(fetchRange, blob, bytes.length);
    expect(manifest.bundleName).toBe(v.bundle_name);
    expect(manifest.files.map((f) => [f.name, f.type])).toEqual(
      v.files.map((f) => [f.name, f.type]),
    );
    const decrypted = await decryptBundleFiles(manifest.files, blob, fetchRange);
    for (const [i, { blob: content }] of decrypted.entries()) {
      expect(new Uint8Array(await content.arrayBuffer())).toEqual(
        fromBase64(v.files[i].content_base64),
      );
    }
  });
});
