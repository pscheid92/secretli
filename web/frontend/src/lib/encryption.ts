import { base64UrlDecode, base64UrlEncode } from "./base64";

export interface EncodedKeySet {
  readonly shareSecret: string;
  readonly publicID: string;
  readonly retrievalToken: string;
  readonly deletionToken: string;
}

export interface EncryptedData {
  readonly nonce: string;
  readonly encrypted_data: string;
}

function toBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

export class KeySet {
  private readonly shareSecret: Uint8Array;
  private readonly encryptionKey: Uint8Array;
  private readonly publicID: Uint8Array;
  private readonly retrievalToken: Uint8Array;
  private readonly deletionToken: Uint8Array;

  private constructor(
    shareSecret: Uint8Array,
    encryptionKey: Uint8Array,
    publicID: Uint8Array,
    retrievalToken: Uint8Array,
    deletionToken: Uint8Array,
  ) {
    this.shareSecret = shareSecret;
    this.encryptionKey = encryptionKey;
    this.publicID = publicID;
    this.retrievalToken = retrievalToken;
    this.deletionToken = deletionToken;
  }

  static async generateRandom(): Promise<KeySet> {
    const shareSecret = crypto.getRandomValues(new Uint8Array(32));
    const deletionToken = crypto.getRandomValues(new Uint8Array(16));
    const { encryptionKey, publicID, retrievalToken } = await deriveKeys(shareSecret);
    return new KeySet(shareSecret, encryptionKey, publicID, retrievalToken, deletionToken);
  }

  static async fromShareSecret(encoded: string, password?: string): Promise<KeySet> {
    const shareSecretBytes = base64UrlDecode(encoded);
    let keyMaterial: Uint8Array = shareSecretBytes;

    if (password) {
      const passwordKey = await crypto.subtle.importKey(
        "raw",
        new TextEncoder().encode(password),
        "PBKDF2",
        false,
        ["deriveBits"],
      );
      const masterBits = await crypto.subtle.deriveBits(
        {
          name: "PBKDF2",
          hash: "SHA-512",
          salt: toBuffer(shareSecretBytes),
          iterations: 100_000,
        },
        passwordKey,
        256,
      );
      keyMaterial = new Uint8Array(masterBits);
    }

    const { encryptionKey, publicID, retrievalToken } = await deriveKeys(keyMaterial);
    const deletionToken = new Uint8Array(0);
    return new KeySet(shareSecretBytes, encryptionKey, publicID, retrievalToken, deletionToken);
  }

  async encrypt(plaintext: string): Promise<EncryptedData> {
    const nonce = crypto.getRandomValues(new Uint8Array(12));
    const encoded = new TextEncoder().encode(plaintext);

    const key = await crypto.subtle.importKey(
      "raw",
      toBuffer(this.encryptionKey),
      "AES-GCM",
      false,
      ["encrypt"],
    );
    const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, key, encoded);

    return {
      nonce: base64UrlEncode(nonce),
      encrypted_data: base64UrlEncode(new Uint8Array(ciphertext)),
    };
  }

  async decrypt(nonce: string, encryptedData: string): Promise<string> {
    const nonceBytes = base64UrlDecode(nonce);
    const cipherBytes = base64UrlDecode(encryptedData);

    const key = await crypto.subtle.importKey(
      "raw",
      toBuffer(this.encryptionKey),
      "AES-GCM",
      false,
      ["decrypt"],
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: toBuffer(nonceBytes) },
      key,
      toBuffer(cipherBytes),
    );

    return new TextDecoder().decode(decrypted);
  }

  async encryptFile(data: Uint8Array): Promise<{ nonce: string; encryptedBlob: Blob }> {
    const nonce = crypto.getRandomValues(new Uint8Array(12));

    const key = await crypto.subtle.importKey(
      "raw",
      toBuffer(this.encryptionKey),
      "AES-GCM",
      false,
      ["encrypt"],
    );
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv: nonce },
      key,
      toBuffer(data),
    );

    return {
      nonce: base64UrlEncode(nonce),
      encryptedBlob: new Blob([ciphertext]),
    };
  }

  async decryptFile(nonce: string, encryptedBlob: Blob): Promise<Uint8Array> {
    const nonceBytes = base64UrlDecode(nonce);
    const cipherBytes = new Uint8Array(await encryptedBlob.arrayBuffer());

    const key = await crypto.subtle.importKey(
      "raw",
      toBuffer(this.encryptionKey),
      "AES-GCM",
      false,
      ["decrypt"],
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: toBuffer(nonceBytes) },
      key,
      toBuffer(cipherBytes),
    );

    return new Uint8Array(decrypted);
  }

  async encryptFilename(filename: string): Promise<string> {
    const nonce = crypto.getRandomValues(new Uint8Array(12));
    const encoded = new TextEncoder().encode(filename);

    const key = await crypto.subtle.importKey(
      "raw",
      toBuffer(this.encryptionKey),
      "AES-GCM",
      false,
      ["encrypt"],
    );
    const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, key, encoded);

    return `${base64UrlEncode(nonce)}:${base64UrlEncode(new Uint8Array(ciphertext))}`;
  }

  async decryptFilename(encrypted: string): Promise<string> {
    const [nonceStr, ciphertextStr] = encrypted.split(":");
    const nonceBytes = base64UrlDecode(nonceStr);
    const cipherBytes = base64UrlDecode(ciphertextStr);

    const key = await crypto.subtle.importKey(
      "raw",
      toBuffer(this.encryptionKey),
      "AES-GCM",
      false,
      ["decrypt"],
    );
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: toBuffer(nonceBytes) },
      key,
      toBuffer(cipherBytes),
    );

    return new TextDecoder().decode(decrypted);
  }

  getEncoded(): EncodedKeySet {
    return {
      shareSecret: base64UrlEncode(this.shareSecret),
      publicID: base64UrlEncode(this.publicID),
      retrievalToken: base64UrlEncode(this.retrievalToken),
      deletionToken: base64UrlEncode(this.deletionToken),
    };
  }
}

async function deriveKeys(keyBytes: Uint8Array): Promise<{
  encryptionKey: Uint8Array;
  publicID: Uint8Array;
  retrievalToken: Uint8Array;
}> {
  const baseKey = await crypto.subtle.importKey("raw", toBuffer(keyBytes), "HKDF", false, [
    "deriveBits",
  ]);

  const encryptionKey = new Uint8Array(
    await deriveSubkey(baseKey, "share_item_encryption_key", 32),
  );
  const publicID = new Uint8Array(await deriveSubkey(baseKey, "share_item_uuid", 16));
  const retrievalToken = new Uint8Array(await deriveSubkey(baseKey, "share_item_token", 16));

  return { encryptionKey, publicID, retrievalToken };
}

async function deriveSubkey(key: CryptoKey, info: string, length: number): Promise<ArrayBuffer> {
  return crypto.subtle.deriveBits(
    {
      name: "HKDF",
      hash: "SHA-512",
      info: new TextEncoder().encode(info).buffer as ArrayBuffer,
      salt: new ArrayBuffer(0),
    },
    key,
    length * 8,
  );
}
