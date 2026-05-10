import { ENCRYPTED_BLOB_OVERHEAD_BYTES } from "./encryption";

export const MAX_ENCRYPTED_UPLOAD_BYTES = 100 * 1024 * 1024;
export const MAX_UPLOAD_LABEL = "100 MB";
export const MAX_FILE_UPLOAD_BYTES = MAX_ENCRYPTED_UPLOAD_BYTES - ENCRYPTED_BLOB_OVERHEAD_BYTES;

export function encryptedUploadSize(plaintextBytes: number): number {
  return plaintextBytes + ENCRYPTED_BLOB_OVERHEAD_BYTES;
}

export function fitsEncryptedUploadLimit(plaintextBytes: number): boolean {
  return encryptedUploadSize(plaintextBytes) <= MAX_ENCRYPTED_UPLOAD_BYTES;
}
