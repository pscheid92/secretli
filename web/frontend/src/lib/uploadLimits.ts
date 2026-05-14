import { estimateBundleEncryptedSize } from "./bundle";
import { estimateChunkedEncryptedSize } from "./chunkedBundle";
import { ENCRYPTED_BLOB_OVERHEAD_BYTES } from "./encryption";

export const MAX_ENCRYPTED_UPLOAD_BYTES = 1024 * 1024 * 1024;
export const MAX_UPLOAD_LABEL = "1 GiB";
export const MAX_FILE_UPLOAD_BYTES = MAX_ENCRYPTED_UPLOAD_BYTES - ENCRYPTED_BLOB_OVERHEAD_BYTES;
export const DEFAULT_CHUNKED_UPLOAD_THRESHOLD_BYTES = 64 * 1024 * 1024;

export function encryptedUploadSize(plaintextBytes: number): number {
  return plaintextBytes + ENCRYPTED_BLOB_OVERHEAD_BYTES;
}

export function fitsEncryptedUploadLimit(plaintextBytes: number): boolean {
  return encryptedUploadSize(plaintextBytes) <= MAX_ENCRYPTED_UPLOAD_BYTES;
}

export function fitsBundleUploadLimit(fileSizes: number[]): boolean {
  return estimatedSelectedFileUploadSize(fileSizes) <= MAX_ENCRYPTED_UPLOAD_BYTES;
}

export function shouldUseChunkedUpload(fileSizes: number[]): boolean {
  return estimateBundleEncryptedSize(fileSizes) >= chunkedUploadThresholdBytes();
}

export function estimatedSelectedFileUploadSize(fileSizes: number[]): number {
  return shouldUseChunkedUpload(fileSizes)
    ? estimateChunkedEncryptedSize(fileSizes)
    : estimateBundleEncryptedSize(fileSizes);
}

export function chunkedUploadThresholdBytes(): number {
  const override =
    typeof window !== "undefined"
      ? (window as Window & { __SECRETLI_CHUNKED_UPLOAD_THRESHOLD_BYTES?: number })
          .__SECRETLI_CHUNKED_UPLOAD_THRESHOLD_BYTES
      : undefined;
  if (typeof override === "number" && Number.isFinite(override) && override >= 0) {
    return override;
  }

  const envValue = Number(import.meta.env.VITE_CHUNKED_UPLOAD_THRESHOLD_BYTES);
  if (Number.isFinite(envValue) && envValue >= 0) {
    return envValue;
  }

  return DEFAULT_CHUNKED_UPLOAD_THRESHOLD_BYTES;
}
