import { base64UrlDecode, base64UrlEncode } from "./base64";

export interface EncodedKeySet {
  readonly shareSecret: string;
  readonly publicID: string;
  readonly retrievalToken: string;
  readonly deletionToken: string;
}

export interface SecretMeta {
  readonly type: "text" | "file";
  readonly password_protected: boolean;
  readonly filename?: string;
}

const ENVELOPE_VERSION = "v1";
const NONCE_LENGTH = 12;

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
          iterations: 210_000,
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

  /**
   * Encrypt a metadata object into the envelope format: v1$base64url(nonce)$base64url(ciphertext)
   */
  async encryptMeta(meta: SecretMeta): Promise<string> {
    const nonce = crypto.getRandomValues(new Uint8Array(NONCE_LENGTH));
    const plaintext = new TextEncoder().encode(JSON.stringify(meta));

    const key = await this.importKey("encrypt");
    const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, key, plaintext);

    return `${ENVELOPE_VERSION}$${base64UrlEncode(nonce)}$${base64UrlEncode(new Uint8Array(ciphertext))}`;
  }

  /**
   * Decrypt an envelope string (v1$nonce$ciphertext) back to a metadata object.
   */
  async decryptMeta(envelope: string): Promise<SecretMeta> {
    const parts = envelope.split("$");
    if (parts.length !== 3 || parts[0] !== ENVELOPE_VERSION) {
      throw new Error("invalid metadata envelope format");
    }

    const nonce = base64UrlDecode(parts[1]);
    const ciphertext = base64UrlDecode(parts[2]);

    const key = await this.importKey("decrypt");
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: toBuffer(nonce) },
      key,
      toBuffer(ciphertext),
    );

    return JSON.parse(new TextDecoder().decode(decrypted));
  }

  /**
   * Encrypt binary data into a Blob with the nonce prepended: [12-byte nonce][ciphertext]
   */
  async encryptBlob(data: Uint8Array): Promise<Blob> {
    const nonce = crypto.getRandomValues(new Uint8Array(NONCE_LENGTH));

    const key = await this.importKey("encrypt");
    const ciphertext = await crypto.subtle.encrypt(
      { name: "AES-GCM", iv: nonce },
      key,
      toBuffer(data),
    );

    return new Blob([nonce, ciphertext]);
  }

  /**
   * Decrypt a nonce-prefixed blob back to plaintext bytes.
   */
  async decryptBlob(blob: Blob): Promise<Uint8Array> {
    const bytes = new Uint8Array(await blob.arrayBuffer());
    const nonce = bytes.slice(0, NONCE_LENGTH);
    const ciphertext = bytes.slice(NONCE_LENGTH);

    const key = await this.importKey("decrypt");
    const decrypted = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: toBuffer(nonce) },
      key,
      toBuffer(ciphertext),
    );

    return new Uint8Array(decrypted);
  }

  getEncoded(): EncodedKeySet {
    return {
      shareSecret: base64UrlEncode(this.shareSecret),
      publicID: base64UrlEncode(this.publicID),
      retrievalToken: base64UrlEncode(this.retrievalToken),
      deletionToken: base64UrlEncode(this.deletionToken),
    };
  }

  private async importKey(usage: "encrypt" | "decrypt"): Promise<CryptoKey> {
    return crypto.subtle.importKey("raw", toBuffer(this.encryptionKey), "AES-GCM", false, [usage]);
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
