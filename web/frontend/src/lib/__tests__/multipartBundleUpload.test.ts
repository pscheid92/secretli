import { ApiError, type UploadSessionPart, type UploadSessionStatus } from "../api";
import { readBundleManifest } from "../bundle";
import { KeySet } from "../encryption";
import {
  resetMultipartUploadStateForTests,
  S3_MIN_MULTIPART_PART_SIZE,
  UploadCancelledError,
  uploadMultipartBundle,
} from "../multipartBundleUpload";

const api = vi.hoisted(() => ({
  startUploadSession: vi.fn(),
  getUploadSession: vi.fn(),
  uploadSessionPart: vi.fn(),
  completeUploadSession: vi.fn(),
  abortUploadSession: vi.fn(),
}));

vi.mock("../api", async (importOriginal) => {
  const original = await importOriginal<typeof import("../api")>();
  return { ...original, ...api };
});

const MIB = 1024 * 1024;
// Small part size so a 13 MiB file yields more than one part in tests.
const TEST_PART_SIZE = 6 * MIB;

interface FakeServer {
  status: UploadSessionStatus;
  parts: Map<number, { part: UploadSessionPart; bytes: Uint8Array }>;
  sessionCounter: number;
}

function installFakeServer(): FakeServer {
  const server: FakeServer = {
    status: {
      session_id: "",
      public_id: "",
      part_size: TEST_PART_SIZE,
      blob_size: 0,
      expires_at: new Date(Date.now() + 3600_000).toISOString(),
      upload_expires_at: new Date(Date.now() + 3600_000).toISOString(),
      state: "pending",
      uploaded_parts: [],
    },
    parts: new Map(),
    sessionCounter: 0,
  };

  api.startUploadSession.mockImplementation(async (params) => {
    server.sessionCounter++;
    server.parts.clear();
    server.status = {
      ...server.status,
      session_id: `session-${server.sessionCounter}`,
      public_id: params.public_id,
      blob_size: params.blob_size,
      state: "pending",
      uploaded_parts: [],
    };
    return { ...server.status, upload_token: `token-${server.sessionCounter}` };
  });
  api.getUploadSession.mockImplementation(async () => ({
    ...server.status,
    uploaded_parts: Array.from(server.parts.values(), (entry) => entry.part),
  }));
  api.uploadSessionPart.mockImplementation(
    async (_session, _token, partNumber, offset, bytes: Blob, sha256) => {
      const part: UploadSessionPart = {
        part_number: partNumber,
        offset,
        size: bytes.size,
        sha256,
        etag: `etag-${partNumber}`,
      };
      server.parts.set(partNumber, { part, bytes: new Uint8Array(await bytes.arrayBuffer()) });
      return part;
    },
  );
  api.completeUploadSession.mockImplementation(async () => {
    server.status = { ...server.status, state: "completed" };
    return { expires_at: server.status.expires_at };
  });
  api.abortUploadSession.mockImplementation(async () => {
    server.status = { ...server.status, state: "aborted" };
  });
  return server;
}

function patternedFile(size: number, name = "payload.bin"): File {
  const bytes = new Uint8Array(size);
  for (let i = 0; i < size; i++) bytes[i] = (i * 7 + 3) & 0xff;
  return new File([bytes], name, { type: "application/octet-stream", lastModified: 1_700_000_000 });
}

function sameBytes(a: Uint8Array, b: Uint8Array): boolean {
  // Deep equality on multi-megabyte typed arrays is far too slow in the
  // matcher; compare buffers directly.
  return Buffer.from(a.buffer, a.byteOffset, a.byteLength).equals(
    Buffer.from(b.buffer, b.byteOffset, b.byteLength),
  );
}

function assembledBlob(server: FakeServer): Uint8Array {
  const parts = Array.from(server.parts.values()).sort(
    (a, b) => a.part.part_number - b.part.part_number,
  );
  const total = parts.reduce((sum, entry) => sum + entry.bytes.length, 0);
  const out = new Uint8Array(total);
  let cursor = 0;
  for (const entry of parts) {
    expect(entry.part.offset).toBe(cursor);
    out.set(entry.bytes, cursor);
    cursor += entry.bytes.length;
  }
  return out;
}

async function baseParams(files: File[], password?: string) {
  const baseKeySet = await KeySet.generateRandom();
  const bundleKeySet = password
    ? await KeySet.fromShareSecret(baseKeySet.getEncoded().shareSecret, password)
    : baseKeySet;
  return {
    files,
    baseKeySet,
    bundleKeySet,
    password,
    passwordProtected: password !== undefined,
    expiration: "1d",
    burnAfterRead: false,
  };
}

describe("uploadMultipartBundle", () => {
  beforeEach(() => {
    resetMultipartUploadStateForTests();
    for (const fn of Object.values(api)) fn.mockReset();
  });

  it("splits the encrypted bundle into contiguous parts that respect the S3 minimum", async () => {
    const server = installFakeServer();
    const file = patternedFile(13 * MIB);
    const params = await baseParams([file]);
    const progress: number[] = [];

    const result = await uploadMultipartBundle({
      ...params,
      onProgress: (p) => progress.push(p.uploadedBytes),
    });

    expect(api.startUploadSession).toHaveBeenCalledTimes(1);
    expect(api.completeUploadSession).toHaveBeenCalledTimes(1);
    const parts = Array.from(server.parts.values()).map((entry) => entry.part);
    expect(parts.length).toBeGreaterThan(1);
    for (const part of parts.slice(0, -1)) {
      expect(part.size).toBeGreaterThanOrEqual(S3_MIN_MULTIPART_PART_SIZE);
    }
    const blob = assembledBlob(server);
    expect(blob.length).toBe(server.status.blob_size);
    expect(progress.at(-1)).toBe(blob.length);

    // The assembled object is a valid bundle that decrypts to the original file.
    const fetchRange = async (start: number, end: number) => blob.slice(start, end + 1);
    const { manifest } = await readBundleManifest(fetchRange, params.bundleKeySet, blob.length);
    expect(manifest.files[0].name).toBe("payload.bin");
    expect(manifest.files[0].size).toBe(13 * MIB);
    expect(result.encoded.shareSecret).toBe(params.baseKeySet.getEncoded().shareSecret);
    expect(result.deletionToken).toBe(params.baseKeySet.getEncoded().deletionToken);
  }, 30_000);

  it("retries transient part failures but not client errors", async () => {
    installFakeServer();
    const file = patternedFile(64);
    const params = await baseParams([file]);

    const original = api.uploadSessionPart.getMockImplementation();
    let calls = 0;
    api.uploadSessionPart.mockImplementation(async (...args) => {
      calls++;
      if (calls === 1) throw new ApiError(503, "storage unavailable");
      return original?.(...args);
    });
    await expect(uploadMultipartBundle(params)).resolves.toBeTruthy();
    expect(calls).toBe(2);

    resetMultipartUploadStateForTests();
    installFakeServer();
    api.uploadSessionPart.mockClear();
    api.uploadSessionPart.mockImplementation(async () => {
      throw new ApiError(400, "part rejected");
    });
    await expect(
      uploadMultipartBundle(await baseParams([patternedFile(64)])),
    ).rejects.toMatchObject({
      status: 400,
    });
    expect(api.uploadSessionPart).toHaveBeenCalledTimes(1);
  });

  it("aborts the server session and forgets state when cancelled", async () => {
    installFakeServer();
    const file = patternedFile(64);
    const params = await baseParams([file]);
    const controller = new AbortController();
    const original = api.uploadSessionPart.getMockImplementation();
    api.uploadSessionPart.mockImplementation(async (...args) => {
      controller.abort();
      return original?.(...args);
    });

    await expect(
      uploadMultipartBundle({ ...params, signal: controller.signal }),
    ).rejects.toBeInstanceOf(UploadCancelledError);
    expect(api.abortUploadSession).toHaveBeenCalledWith("session-1", "token-1");
    expect(api.completeUploadSession).not.toHaveBeenCalled();

    // A fresh attempt starts a brand new session.
    api.uploadSessionPart.mockImplementation(original ?? (async () => ({}) as UploadSessionPart));
    await uploadMultipartBundle(await baseParams([file]));
    expect(api.startUploadSession).toHaveBeenCalledTimes(2);
  });

  it("resumes the same session in this tab and skips already uploaded parts", async () => {
    const server = installFakeServer();
    const file = patternedFile(13 * MIB);
    const params = await baseParams([file]);

    api.completeUploadSession.mockRejectedValueOnce(new ApiError(503, "try later"));
    await expect(uploadMultipartBundle(params)).rejects.toMatchObject({ status: 503 });
    const uploadedBefore = api.uploadSessionPart.mock.calls.length;
    expect(uploadedBefore).toBeGreaterThan(0);
    const firstBlob = assembledBlob(server);

    // Second attempt: same files, the keys are re-derived from the in-memory
    // share secret, so a different (fresh) base keyset must not matter.
    const retry = await baseParams([file]);
    const result = await uploadMultipartBundle(retry);

    expect(api.startUploadSession).toHaveBeenCalledTimes(1);
    expect(api.getUploadSession).toHaveBeenCalledTimes(1);
    expect(api.uploadSessionPart).toHaveBeenCalledTimes(uploadedBefore);
    expect(api.completeUploadSession).toHaveBeenCalledTimes(2);
    expect(result.encoded.shareSecret).toBe(params.baseKeySet.getEncoded().shareSecret);
    expect(sameBytes(assembledBlob(server), firstBlob)).toBe(true);
  }, 30_000);

  it("starts over when the recorded parts no longer match", async () => {
    const server = installFakeServer();
    const file = patternedFile(64);
    const params = await baseParams([file]);

    api.completeUploadSession.mockRejectedValueOnce(new ApiError(503, "try later"));
    await expect(uploadMultipartBundle(params)).rejects.toMatchObject({ status: 503 });

    // Poison the server-side record for part 1.
    const entry = server.parts.get(1);
    if (!entry) throw new Error("expected part 1");
    server.parts.set(1, { ...entry, part: { ...entry.part, sha256: "0".repeat(64) } });

    await expect(uploadMultipartBundle(await baseParams([file]))).rejects.toThrow(
      "does not match resumable state",
    );
    expect(api.abortUploadSession).not.toHaveBeenCalled();

    // The poisoned session is forgotten; the next attempt creates a new one.
    await uploadMultipartBundle(await baseParams([file]));
    expect(api.startUploadSession).toHaveBeenCalledTimes(2);
  });

  it("does not resume when the session was rejected with 409", async () => {
    installFakeServer();
    const file = patternedFile(64);
    api.completeUploadSession.mockRejectedValueOnce(new ApiError(409, "parts rejected"));
    await expect(uploadMultipartBundle(await baseParams([file]))).rejects.toMatchObject({
      status: 409,
    });

    await uploadMultipartBundle(await baseParams([file]));
    expect(api.startUploadSession).toHaveBeenCalledTimes(2);
  });

  it("uses a fresh nonce salt per upload so identical content never repeats a nonce", async () => {
    const server = installFakeServer();
    const file = patternedFile(64);
    const keys = await baseParams([file]);

    await uploadMultipartBundle(keys);
    const first = assembledBlob(server);

    // Same keys, same file, new session after the first completed.
    await uploadMultipartBundle(keys);
    const second = assembledBlob(server);

    expect(api.startUploadSession).toHaveBeenCalledTimes(2);
    expect(first.length).toBe(second.length);
    expect(sameBytes(first, second)).toBe(false);
    // Both still decrypt with the same key.
    for (const blob of [first, second]) {
      const fetchRange = async (start: number, end: number) => blob.slice(start, end + 1);
      const { manifest } = await readBundleManifest(fetchRange, keys.bundleKeySet, blob.length);
      expect(manifest.files[0].size).toBe(64);
    }
  });

  it("derives the blob token from the password when resuming", async () => {
    installFakeServer();
    const file = patternedFile(64);
    const params = await baseParams([file], "hunter2");
    api.completeUploadSession.mockRejectedValueOnce(new ApiError(503, "try later"));
    await expect(uploadMultipartBundle(params)).rejects.toMatchObject({ status: 503 });

    // A different password cannot resume the session: it is aborted instead.
    await uploadMultipartBundle(await baseParams([file], "other"));
    expect(api.abortUploadSession).toHaveBeenCalledWith("session-1", "token-1");
    expect(api.startUploadSession).toHaveBeenCalledTimes(2);
  });
});
