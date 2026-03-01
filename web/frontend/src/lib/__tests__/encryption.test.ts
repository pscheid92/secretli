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

    it("produces blob larger than input (nonce + auth tag)", async () => {
      const ks = await KeySet.generateRandom();
      const data = new Uint8Array(100);
      const blob = await ks.encryptBlob(data);
      // 12 byte nonce + 100 byte data + 16 byte GCM auth tag = 128
      expect(blob.size).toBe(128);
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

    it("produces v1$nonce$ciphertext format", async () => {
      const ks = await KeySet.generateRandom();
      const envelope = await ks.encryptMeta({ type: "text", password_protected: false });
      const parts = envelope.split("$");
      expect(parts).toHaveLength(3);
      expect(parts[0]).toBe("v1");
    });

    it("rejects invalid envelope format", async () => {
      const ks = await KeySet.generateRandom();
      await expect(ks.decryptMeta("bad-format")).rejects.toThrow("invalid metadata envelope format");
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

  describe("fromShareSecret with password", () => {
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
      // This simulates the actual create/retrieve workflow
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
});
