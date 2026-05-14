import { ENCRYPTED_BLOB_OVERHEAD_BYTES } from "../encryption";
import {
  encryptedUploadSize,
  fitsBundleUploadLimit,
  fitsEncryptedUploadLimit,
  MAX_ENCRYPTED_UPLOAD_BYTES,
  MAX_FILE_UPLOAD_BYTES,
  shouldUseChunkedUpload,
} from "../uploadLimits";

describe("upload limits", () => {
  it("reserves encrypted blob overhead from the client file limit", () => {
    expect(MAX_FILE_UPLOAD_BYTES + ENCRYPTED_BLOB_OVERHEAD_BYTES).toBe(MAX_ENCRYPTED_UPLOAD_BYTES);
  });

  it("accepts the largest plaintext size that fits after encryption", () => {
    expect(encryptedUploadSize(MAX_FILE_UPLOAD_BYTES)).toBe(MAX_ENCRYPTED_UPLOAD_BYTES);
    expect(fitsEncryptedUploadLimit(MAX_FILE_UPLOAD_BYTES)).toBe(true);
  });

  it("rejects one byte over the plaintext boundary", () => {
    expect(encryptedUploadSize(MAX_FILE_UPLOAD_BYTES + 1)).toBe(MAX_ENCRYPTED_UPLOAD_BYTES + 1);
    expect(fitsEncryptedUploadLimit(MAX_FILE_UPLOAD_BYTES + 1)).toBe(false);
  });

  it("reserves bundle chunk and manifest overhead", () => {
    expect(fitsBundleUploadLimit([1024])).toBe(true);
    expect(fitsBundleUploadLimit([MAX_ENCRYPTED_UPLOAD_BYTES])).toBe(false);
  });

  it("routes file bundles to chunked upload at the 64 MiB boundary", () => {
    expect(shouldUseChunkedUpload([63 * 1024 * 1024])).toBe(false);
    expect(shouldUseChunkedUpload([64 * 1024 * 1024])).toBe(true);
  });
});
