import { xchacha20poly1305 } from "@noble/ciphers/chacha.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { scrypt } from "@noble/hashes/scrypt.js";
import { sha512 } from "@noble/hashes/sha2.js";
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

const ENVELOPE_VERSION = "v2";
const V1_NONCE_LENGTH = 12;
const V2_NONCE_LENGTH = 24;
const BLOB_V2_TAG = 0x02;

function toBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function buildAad(publicID: Uint8Array, purpose: "meta" | "blob"): Uint8Array {
  const suffix = new TextEncoder().encode(purpose);
  const aad = new Uint8Array(publicID.length + suffix.length);
  aad.set(publicID, 0);
  aad.set(suffix, publicID.length);
  return aad;
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
    const { encryptionKey, publicID, retrievalToken } = deriveKeys(shareSecret);
    return new KeySet(shareSecret, encryptionKey, publicID, retrievalToken, deletionToken);
  }

  static async fromShareSecret(
    encoded: string,
    password?: string,
    options?: { kdfVersion?: "v1" | "v2" },
  ): Promise<KeySet> {
    const shareSecretBytes = base64UrlDecode(encoded);
    let keyMaterial: Uint8Array = shareSecretBytes;

    if (password) {
      const version = options?.kdfVersion ?? "v2";
      if (version === "v1") {
        // Legacy PBKDF2 via Web Crypto
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
      } else {
        // v2: scrypt (preferred over Argon2id for smaller JS/native performance gap)
        keyMaterial = scrypt(new TextEncoder().encode(password), shareSecretBytes, {
          N: 2 ** 14,
          r: 8,
          p: 1,
          dkLen: 32,
        });
      }
    }

    const { encryptionKey, publicID, retrievalToken } = deriveKeys(keyMaterial);
    const deletionToken = new Uint8Array(0);
    return new KeySet(shareSecretBytes, encryptionKey, publicID, retrievalToken, deletionToken);
  }

  /**
   * Encrypt a metadata object into the envelope format: v2$base64url(nonce)$base64url(ciphertext)
   */
  async encryptMeta(meta: SecretMeta): Promise<string> {
    const nonce = crypto.getRandomValues(new Uint8Array(V2_NONCE_LENGTH));
    const plaintext = new TextEncoder().encode(JSON.stringify(meta));
    const aad = buildAad(this.publicID, "meta");
    const cipher = xchacha20poly1305(this.encryptionKey, nonce, aad);
    const ciphertext = cipher.encrypt(plaintext);

    return `${ENVELOPE_VERSION}$${base64UrlEncode(nonce)}$${base64UrlEncode(ciphertext)}`;
  }

  /**
   * Decrypt an envelope string back to a metadata object. Supports v1 and v2 formats.
   */
  async decryptMeta(envelope: string): Promise<SecretMeta> {
    const parts = envelope.split("$");
    if (parts.length !== 3) {
      throw new Error("invalid metadata envelope format");
    }

    const version = parts[0];
    const nonce = base64UrlDecode(parts[1]);
    const ciphertext = base64UrlDecode(parts[2]);

    let plaintext: Uint8Array;

    if (version === "v1") {
      // Legacy AES-256-GCM path (no AAD)
      const key = await crypto.subtle.importKey(
        "raw",
        toBuffer(this.encryptionKey),
        "AES-GCM",
        false,
        ["decrypt"],
      );
      const decrypted = await crypto.subtle.decrypt(
        { name: "AES-GCM", iv: toBuffer(nonce) },
        key,
        toBuffer(ciphertext),
      );
      plaintext = new Uint8Array(decrypted);
    } else if (version === "v2") {
      // XChaCha20-Poly1305 with AAD
      const aad = buildAad(this.publicID, "meta");
      const cipher = xchacha20poly1305(this.encryptionKey, nonce, aad);
      plaintext = cipher.decrypt(ciphertext);
    } else {
      throw new Error("invalid metadata envelope format");
    }

    return JSON.parse(new TextDecoder().decode(plaintext));
  }

  /**
   * Encrypt binary data into a Blob: [0x02 version byte][24-byte nonce][ciphertext with poly1305 tag]
   */
  async encryptBlob(data: Uint8Array): Promise<Blob> {
    const nonce = crypto.getRandomValues(new Uint8Array(V2_NONCE_LENGTH));
    const aad = buildAad(this.publicID, "blob");
    const cipher = xchacha20poly1305(this.encryptionKey, nonce, aad);
    const ciphertext = cipher.encrypt(data);

    return new Blob([new Uint8Array([BLOB_V2_TAG]), nonce, new Uint8Array(ciphertext)]);
  }

  /**
   * Decrypt a blob back to plaintext bytes. Supports v1 and v2 formats.
   */
  async decryptBlob(blob: Blob): Promise<Uint8Array> {
    const bytes = new Uint8Array(await blob.arrayBuffer());

    if (bytes[0] === BLOB_V2_TAG) {
      try {
        // v2: [0x02][24-byte nonce][ciphertext with poly1305 tag]
        const nonce = bytes.slice(1, 1 + V2_NONCE_LENGTH);
        const ciphertext = bytes.slice(1 + V2_NONCE_LENGTH);
        const aad = buildAad(this.publicID, "blob");
        const cipher = xchacha20poly1305(this.encryptionKey, nonce, aad);
        return cipher.decrypt(ciphertext);
      } catch {
        // Fall through to v1 — handles the rare case where a v1 nonce starts with 0x02
      }
    }

    // v1: [12-byte nonce][AES-GCM ciphertext]
    const nonce = bytes.slice(0, V1_NONCE_LENGTH);
    const ciphertext = bytes.slice(V1_NONCE_LENGTH);
    const key = await crypto.subtle.importKey(
      "raw",
      toBuffer(this.encryptionKey),
      "AES-GCM",
      false,
      ["decrypt"],
    );
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
}

const encoder = new TextEncoder();

function deriveKeys(keyBytes: Uint8Array): {
  encryptionKey: Uint8Array;
  publicID: Uint8Array;
  retrievalToken: Uint8Array;
} {
  const encryptionKey = hkdf(
    sha512,
    keyBytes,
    undefined,
    encoder.encode("share_item_encryption_key"),
    32,
  );
  const publicID = hkdf(sha512, keyBytes, undefined, encoder.encode("share_item_uuid"), 16);
  const retrievalToken = hkdf(sha512, keyBytes, undefined, encoder.encode("share_item_token"), 16);
  return { encryptionKey, publicID, retrievalToken };
}
