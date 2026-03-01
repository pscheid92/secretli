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

  describe("text encrypt/decrypt", () => {
    it("round-trips plaintext", async () => {
      const ks = await KeySet.generateRandom();
      const plaintext = "Hello, secret world!";
      const { nonce, encrypted_data } = await ks.encrypt(plaintext);
      const decrypted = await ks.decrypt(nonce, encrypted_data);
      expect(decrypted).toBe(plaintext);
    });

    it("round-trips empty string", async () => {
      const ks = await KeySet.generateRandom();
      const { nonce, encrypted_data } = await ks.encrypt("");
      const decrypted = await ks.decrypt(nonce, encrypted_data);
      expect(decrypted).toBe("");
    });

    it("round-trips unicode", async () => {
      const ks = await KeySet.generateRandom();
      const plaintext = "emoji: \u{1F512}\u{1F511} and CJK: \u4F60\u597D";
      const { nonce, encrypted_data } = await ks.encrypt(plaintext);
      const decrypted = await ks.decrypt(nonce, encrypted_data);
      expect(decrypted).toBe(plaintext);
    });
  });

  describe("file encrypt/decrypt", () => {
    it("round-trips binary data", async () => {
      const ks = await KeySet.generateRandom();
      const data = crypto.getRandomValues(new Uint8Array(1024));
      const { nonce, encryptedBlob } = await ks.encryptFile(data);
      const decrypted = await ks.decryptFile(nonce, encryptedBlob);
      expect(decrypted).toEqual(data);
    });
  });

  describe("filename encrypt/decrypt", () => {
    it("round-trips a filename", async () => {
      const ks = await KeySet.generateRandom();
      const filename = "my-secret-doc.pdf";
      const encrypted = await ks.encryptFilename(filename);
      expect(encrypted).toContain(":");
      const decrypted = await ks.decryptFilename(encrypted);
      expect(decrypted).toBe(filename);
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

    it("can decrypt data encrypted by the original keyset", async () => {
      const original = await KeySet.generateRandom();
      const plaintext = "secret message";
      const { nonce, encrypted_data } = await original.encrypt(plaintext);

      const restored = await KeySet.fromShareSecret(original.getEncoded().shareSecret);
      const decrypted = await restored.decrypt(nonce, encrypted_data);
      expect(decrypted).toBe(plaintext);
    });
  });

  describe("fromShareSecret with password", () => {
    it("derives keys with password and can decrypt own data", async () => {
      const original = await KeySet.generateRandom();
      const shareSecret = original.getEncoded().shareSecret;

      const withPw = await KeySet.fromShareSecret(shareSecret, "my-password");
      const plaintext = "password-protected secret";
      const { nonce, encrypted_data } = await withPw.encrypt(plaintext);

      const restored = await KeySet.fromShareSecret(shareSecret, "my-password");
      const decrypted = await restored.decrypt(nonce, encrypted_data);
      expect(decrypted).toBe(plaintext);
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
      const { nonce, encrypted_data } = await withPw.encrypt("secret");

      const wrongPw = await KeySet.fromShareSecret(shareSecret, "wrong-password");
      await expect(wrongPw.decrypt(nonce, encrypted_data)).rejects.toThrow();
    });
  });
});
