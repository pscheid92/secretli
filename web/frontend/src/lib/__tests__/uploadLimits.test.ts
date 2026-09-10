import {
  fitsBundleManifestLimit,
  fitsBundleUploadLimit,
  MAX_ENCRYPTED_UPLOAD_BYTES,
} from "../uploadLimits";

describe("upload limits", () => {
  it("accepts a bundle that fits after encryption", () => {
    expect(fitsBundleUploadLimit([1024])).toBe(true);
  });

  it("reserves bundle chunk and manifest overhead", () => {
    // A file of exactly the limit cannot fit once records, manifest and footer
    // are added around it.
    expect(fitsBundleUploadLimit([MAX_ENCRYPTED_UPLOAD_BYTES])).toBe(false);
  });

  it("detects file counts whose manifest overflows the cap", () => {
    const few = Array.from({ length: 3 }, (_, i) => new File(["x"], `f-${i}.bin`));
    const many = Array.from({ length: 3000 }, (_, i) => new File(["x"], `f-${i}.bin`));
    expect(fitsBundleManifestLimit([])).toBe(true);
    expect(fitsBundleManifestLimit(few)).toBe(true);
    expect(fitsBundleManifestLimit(many)).toBe(false);
  });
});
