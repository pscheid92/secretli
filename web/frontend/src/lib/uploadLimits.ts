import { estimateBundleEncryptedSize } from "./bundle";
import { ENCRYPTED_BLOB_OVERHEAD_BYTES } from "./encryption";

export const MAX_ENCRYPTED_UPLOAD_BYTES = 1024 * 1024 * 1024;
export const MAX_UPLOAD_LABEL = "1 GiB";
export const MAX_FILE_UPLOAD_BYTES = MAX_ENCRYPTED_UPLOAD_BYTES - ENCRYPTED_BLOB_OVERHEAD_BYTES;

export function encryptedUploadSize(plaintextBytes: number): number {
  return plaintextBytes + ENCRYPTED_BLOB_OVERHEAD_BYTES;
}

export function fitsEncryptedUploadLimit(plaintextBytes: number): boolean {
  return encryptedUploadSize(plaintextBytes) <= MAX_ENCRYPTED_UPLOAD_BYTES;
}

export function fitsBundleUploadLimit(fileSizes: number[]): boolean {
  return estimateBundleEncryptedSize(fileSizes) <= MAX_ENCRYPTED_UPLOAD_BYTES;
}
