import { xchacha20poly1305 } from "@noble/ciphers/chacha.js";
import { chunkedAad } from "./encryption";

const NONCE_LENGTH = 24;
const workerSelf = self as unknown as {
  onmessage: ((event: MessageEvent) => void) | null;
  postMessage: (message: unknown, transfer?: Transferable[]) => void;
};

workerSelf.onmessage = (event: MessageEvent) => {
  const { id, key, publicID, plaintext, aadSuffix } = event.data as {
    id: number;
    key: Uint8Array;
    publicID: Uint8Array;
    plaintext: Uint8Array;
    aadSuffix: Uint8Array;
  };

  try {
    const nonce = crypto.getRandomValues(new Uint8Array(NONCE_LENGTH));
    const cipher = xchacha20poly1305(key, nonce, chunkedAad(publicID, aadSuffix));
    const ciphertext = cipher.encrypt(plaintext);
    const encrypted = new Uint8Array(nonce.length + ciphertext.length);
    encrypted.set(nonce, 0);
    encrypted.set(ciphertext, nonce.length);
    workerSelf.postMessage({ id, encrypted }, [encrypted.buffer as ArrayBuffer]);
  } catch (error) {
    workerSelf.postMessage({
      id,
      error: error instanceof Error ? error.message : "chunk encryption failed",
    });
  }
};
