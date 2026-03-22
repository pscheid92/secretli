import { hkdf } from "@noble/hashes/hkdf.js";
import { sha512 } from "@noble/hashes/sha2.js";

const enc = new TextEncoder();

import { base64UrlDecode, base64UrlEncode } from "../base64";
import { KeySet } from "../encryption";

describe("KeySet", () => {
  describe("generateRandom", () => {
    it("creates a keyset with non-empty encoded fields", async () => {
      const ks = await KeySet.generateRandom();
      const encoded = ks.getEncoded();
      expect(encoded.shareSecret).toBeTruthy();
      expect(encoded.publicID).toBeTruthy();
      expect(encoded.retrievalToken).toBeTruthy();
      expect(encoded.deletionToken).toBeTruthy();
    });

    it("generates unique keysets each time", async () => {
      const a = await KeySet.generateRandom();
      const b = await KeySet.generateRandom();
      expect(a.getEncoded().shareSecret).not.toBe(b.getEncoded().shareSecret);
    });
  });

  describe("encryptBlob/decryptBlob", () => {
    it("round-trips text as bytes", async () => {
      const ks = await KeySet.generateRandom();
      const plaintext = "Hello, secret world!";
      const data = new TextEncoder().encode(plaintext);
      const blob = await ks.encryptBlob(data);
      const decrypted = await ks.decryptBlob(blob);
      expect(new TextDecoder().decode(decrypted)).toBe(plaintext);
    });

    it("round-trips empty data", async () => {
      const ks = await KeySet.generateRandom();
      const blob = await ks.encryptBlob(new Uint8Array(0));
      const decrypted = await ks.decryptBlob(blob);
      expect(decrypted.length).toBe(0);
    });

    it("round-trips binary data", async () => {
      const ks = await KeySet.generateRandom();
      const data = crypto.getRandomValues(new Uint8Array(1024));
      const blob = await ks.encryptBlob(data);
      const decrypted = await ks.decryptBlob(blob);
      expect(decrypted).toEqual(data);
    });

    it("produces blob larger than input (version + nonce + auth tag)", async () => {
      const ks = await KeySet.generateRandom();
      const data = new Uint8Array(100);
      const blob = await ks.encryptBlob(data);
      // 1 byte version + 24 byte nonce + 100 byte data + 16 byte Poly1305 tag = 141
      expect(blob.size).toBe(141);
    });
  });

  describe("encryptMeta/decryptMeta", () => {
    it("round-trips text metadata", async () => {
      const ks = await KeySet.generateRandom();
      const meta = { type: "text" as const, password_protected: false };
      const envelope = await ks.encryptMeta(meta);
      const decrypted = await ks.decryptMeta(envelope);
      expect(decrypted).toEqual(meta);
    });

    it("round-trips file metadata with filename", async () => {
      const ks = await KeySet.generateRandom();
      const meta = { type: "file" as const, password_protected: true, filename: "report.pdf" };
      const envelope = await ks.encryptMeta(meta);
      const decrypted = await ks.decryptMeta(envelope);
      expect(decrypted).toEqual(meta);
    });

    it("produces v2$nonce$ciphertext format", async () => {
      const ks = await KeySet.generateRandom();
      const envelope = await ks.encryptMeta({ type: "text", password_protected: false });
      const parts = envelope.split("$");
      expect(parts).toHaveLength(3);
      expect(parts[0]).toBe("v2");
    });

    it("rejects invalid envelope format", async () => {
      const ks = await KeySet.generateRandom();
      await expect(ks.decryptMeta("bad-format")).rejects.toThrow(
        "invalid metadata envelope format",
      );
    });
  });

  describe("fromShareSecret", () => {
    it("derives the same encryption key from the share secret", async () => {
      const original = await KeySet.generateRandom();
      const encoded = original.getEncoded();

      const restored = await KeySet.fromShareSecret(encoded.shareSecret);
      const restoredEncoded = restored.getEncoded();

      expect(restoredEncoded.publicID).toBe(encoded.publicID);
      expect(restoredEncoded.retrievalToken).toBe(encoded.retrievalToken);
    });

    it("can decrypt blob encrypted by the original keyset", async () => {
      const original = await KeySet.generateRandom();
      const plaintext = "secret message";
      const data = new TextEncoder().encode(plaintext);
      const blob = await original.encryptBlob(data);

      const restored = await KeySet.fromShareSecret(original.getEncoded().shareSecret);
      const decrypted = await restored.decryptBlob(blob);
      expect(new TextDecoder().decode(decrypted)).toBe(plaintext);
    });

    it("can decrypt metadata encrypted by the original keyset", async () => {
      const original = await KeySet.generateRandom();
      const meta = { type: "file" as const, password_protected: false, filename: "test.txt" };
      const envelope = await original.encryptMeta(meta);

      const restored = await KeySet.fromShareSecret(original.getEncoded().shareSecret);
      const decrypted = await restored.decryptMeta(envelope);
      expect(decrypted).toEqual(meta);
    });
  });

  describe("fromShareSecret with password (v2 Argon2id)", () => {
    it("derives keys with password and can decrypt own data", async () => {
      const original = await KeySet.generateRandom();
      const shareSecret = original.getEncoded().shareSecret;

      const withPw = await KeySet.fromShareSecret(shareSecret, "my-password");
      const data = new TextEncoder().encode("password-protected secret");
      const blob = await withPw.encryptBlob(data);

      const restored = await KeySet.fromShareSecret(shareSecret, "my-password");
      const decrypted = await restored.decryptBlob(blob);
      expect(new TextDecoder().decode(decrypted)).toBe("password-protected secret");
    });

    it("derives different keys with different passwords", async () => {
      const original = await KeySet.generateRandom();
      const shareSecret = original.getEncoded().shareSecret;

      const ks1 = await KeySet.fromShareSecret(shareSecret, "password-a");
      const ks2 = await KeySet.fromShareSecret(shareSecret, "password-b");

      expect(ks1.getEncoded().publicID).not.toBe(ks2.getEncoded().publicID);
    });

    it("password-derived keys differ from no-password keys", async () => {
      const original = await KeySet.generateRandom();
      const shareSecret = original.getEncoded().shareSecret;

      const noPw = await KeySet.fromShareSecret(shareSecret);
      const withPw = await KeySet.fromShareSecret(shareSecret, "some-password");

      expect(noPw.getEncoded().publicID).not.toBe(withPw.getEncoded().publicID);
    });

    it("wrong password cannot decrypt data", async () => {
      const original = await KeySet.generateRandom();
      const shareSecret = original.getEncoded().shareSecret;

      const withPw = await KeySet.fromShareSecret(shareSecret, "correct-password");
      const blob = await withPw.encryptBlob(new TextEncoder().encode("secret"));

      const wrongPw = await KeySet.fromShareSecret(shareSecret, "wrong-password");
      await expect(wrongPw.decryptBlob(blob)).rejects.toThrow();
    });
  });

  describe("password-protected workflow", () => {
    it("metadata encrypted with base key, data with password key", async () => {
      const keySet = await KeySet.generateRandom();
      const shareSecret = keySet.getEncoded().shareSecret;

      // Create: metadata with base key, data with password key
      const meta = { type: "text" as const, password_protected: true };
      const encryptedMeta = await keySet.encryptMeta(meta);
      const passwordKeySet = await KeySet.fromShareSecret(shareSecret, "the-password");
      const blob = await passwordKeySet.encryptBlob(new TextEncoder().encode("secret data"));

      // Retrieve: decrypt metadata with base key (no password needed)
      const restored = await KeySet.fromShareSecret(shareSecret);
      const decryptedMeta = await restored.decryptMeta(encryptedMeta);
      expect(decryptedMeta.password_protected).toBe(true);

      // Decrypt data with password key
      const restoredPw = await KeySet.fromShareSecret(shareSecret, "the-password");
      const decryptedData = await restoredPw.decryptBlob(blob);
      expect(new TextDecoder().decode(decryptedData)).toBe("secret data");

      // Wrong password fails
      const wrongPw = await KeySet.fromShareSecret(shareSecret, "wrong");
      await expect(wrongPw.decryptBlob(blob)).rejects.toThrow();
    });
  });

  describe("AAD enforcement", () => {
    it("rejects decryption with a different KeySet (different publicID)", async () => {
      const ksA = await KeySet.generateRandom();
      const ksB = await KeySet.generateRandom();

      const meta = { type: "text" as const, password_protected: false };
      const envelope = await ksA.encryptMeta(meta);
      await expect(ksB.decryptMeta(envelope)).rejects.toThrow();
    });

    it("rejects blob decryption with a different KeySet", async () => {
      const ksA = await KeySet.generateRandom();
      const ksB = await KeySet.generateRandom();

      const blob = await ksA.encryptBlob(new TextEncoder().encode("data"));
      await expect(ksB.decryptBlob(blob)).rejects.toThrow();
    });

    it("rejects meta ciphertext used as blob (wrong AAD purpose)", async () => {
      const ks = await KeySet.generateRandom();
      const meta = { type: "text" as const, password_protected: false };
      const envelope = await ks.encryptMeta(meta);

      // Extract nonce and ciphertext from the v2 envelope
      const parts = envelope.split("$");
      const nonce = base64UrlDecode(parts[1]);
      const ciphertext = base64UrlDecode(parts[2]);

      // Construct a v2 blob with the meta's nonce and ciphertext
      const fakeBlob = new Blob([
        new Uint8Array([0x02]),
        new Uint8Array(nonce),
        new Uint8Array(ciphertext),
      ]);

      // Decryption should fail because AAD is "blob" not "meta"
      await expect(ks.decryptBlob(fakeBlob)).rejects.toThrow();
    });
  });

  describe("v1 backwards compatibility", () => {
    // Helper to create v1-formatted encrypted meta using Web Crypto AES-GCM
    async function createV1Meta(
      encryptionKey: Uint8Array,
      meta: { type: string; password_protected: boolean },
    ): Promise<string> {
      const nonce = crypto.getRandomValues(new Uint8Array(12));
      const plaintext = new TextEncoder().encode(JSON.stringify(meta));
      const keyBuf = new ArrayBuffer(encryptionKey.length);
      new Uint8Array(keyBuf).set(encryptionKey);
      const key = await crypto.subtle.importKey("raw", keyBuf, "AES-GCM", false, ["encrypt"]);
      const ciphertext = await crypto.subtle.encrypt(
        { name: "AES-GCM", iv: nonce },
        key,
        plaintext,
      );
      return `v1$${base64UrlEncode(nonce)}$${base64UrlEncode(new Uint8Array(ciphertext))}`;
    }

    // Helper to create v1-formatted encrypted blob using Web Crypto AES-GCM
    async function createV1Blob(encryptionKey: Uint8Array, data: Uint8Array): Promise<Blob> {
      const nonce = crypto.getRandomValues(new Uint8Array(12));
      const keyBuf = new ArrayBuffer(encryptionKey.length);
      new Uint8Array(keyBuf).set(encryptionKey);
      const key = await crypto.subtle.importKey("raw", keyBuf, "AES-GCM", false, ["encrypt"]);
      const dataBuf = data.buffer.slice(
        data.byteOffset,
        data.byteOffset + data.byteLength,
      ) as ArrayBuffer;
      const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, key, dataBuf);
      return new Blob([nonce, ciphertext]);
    }

    it("decrypts v1-encrypted meta", async () => {
      const shareSecret = crypto.getRandomValues(new Uint8Array(32));
      const encryptionKey = hkdf(
        sha512,
        shareSecret,
        undefined,
        enc.encode("share_item_encryption_key"),
        32,
      );

      const meta = { type: "text" as const, password_protected: false };
      const v1Envelope = await createV1Meta(encryptionKey, meta);
      expect(v1Envelope.startsWith("v1$")).toBe(true);

      const ks = await KeySet.fromShareSecret(base64UrlEncode(shareSecret));
      const decrypted = await ks.decryptMeta(v1Envelope);
      expect(decrypted).toEqual(meta);
    });

    it("decrypts v1-encrypted blob", async () => {
      const shareSecret = crypto.getRandomValues(new Uint8Array(32));
      const encryptionKey = hkdf(
        sha512,
        shareSecret,
        undefined,
        enc.encode("share_item_encryption_key"),
        32,
      );

      const data = new TextEncoder().encode("v1 encrypted data");
      const v1Blob = await createV1Blob(encryptionKey, data);

      const ks = await KeySet.fromShareSecret(base64UrlEncode(shareSecret));
      const decrypted = await ks.decryptBlob(v1Blob);
      expect(new TextDecoder().decode(decrypted)).toBe("v1 encrypted data");
    });

    it("decrypts v1 password-protected data with kdfVersion v1", async () => {
      const shareSecret = crypto.getRandomValues(new Uint8Array(32));
      const password = "test-password";

      // Derive key material using PBKDF2 (the v1 way)
      const passwordKey = await crypto.subtle.importKey(
        "raw",
        new TextEncoder().encode(password),
        "PBKDF2",
        false,
        ["deriveBits"],
      );
      const shareSecretBuf = new ArrayBuffer(shareSecret.length);
      new Uint8Array(shareSecretBuf).set(shareSecret);
      const masterBits = await crypto.subtle.deriveBits(
        {
          name: "PBKDF2",
          hash: "SHA-512",
          salt: shareSecretBuf,
          iterations: 210_000,
        },
        passwordKey,
        256,
      );
      const keyMaterial = new Uint8Array(masterBits);
      const encryptionKey = hkdf(
        sha512,
        keyMaterial,
        undefined,
        enc.encode("share_item_encryption_key"),
        32,
      );

      // Create v1 blob encrypted with the PBKDF2-derived key
      const data = new TextEncoder().encode("password-protected v1 data");
      const v1Blob = await createV1Blob(encryptionKey, data);

      // Decrypt using the new code with kdfVersion: "v1"
      const encoded = base64UrlEncode(shareSecret);
      const ks = await KeySet.fromShareSecret(encoded, password, { kdfVersion: "v1" });
      const decrypted = await ks.decryptBlob(v1Blob);
      expect(new TextDecoder().decode(decrypted)).toBe("password-protected v1 data");
    });
  });
});
