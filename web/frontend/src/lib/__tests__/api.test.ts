import {
  ApiError,
  abortUploadSession,
  completeUploadSession,
  deleteSecret,
  getSecretMetadata,
  getUploadSession,
  isTransientStatus,
  MAX_TRANSIENT_ATTEMPTS,
  retrieveSecretRange,
  retryDelayMs,
  type StartUploadSessionParams,
  startRetrievalSession,
  startUploadSession,
  uploadSessionPart,
} from "../api";

describe("ApiError", () => {
  it("has correct name, status, and message", () => {
    const err = new ApiError(404, "Not found");
    expect(err).toBeInstanceOf(Error);
    expect(err.name).toBe("ApiError");
    expect(err.status).toBe(404);
    expect(err.message).toBe("Not found");
  });

  it("has zero status for network errors", () => {
    const err = new ApiError(0, "Network error");
    expect(err.status).toBe(0);
  });
});

describe("request helper (via deleteSecret)", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("throws ApiError with server error message", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }),
    );

    try {
      await deleteSecret("pub-id", "meta-tok", "del-tok");
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(401);
      expect((e as ApiError).message).toMatch(/^unauthorized \(request id: .+\)$/);
      expect((e as ApiError).requestId).toBeTruthy();
    }
  });

  it("throws ApiError with fallback message when body is not JSON", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("Internal Server Error", { status: 500 }),
    );

    try {
      await deleteSecret("pub-id", "meta-tok", "del-tok");
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(500);
      expect((e as ApiError).message).toMatch(/^Request failed \(500\) \(request id: .+\)$/);
      expect((e as ApiError).requestId).toBeTruthy();
    }
  });

  it("throws ApiError on network failure", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("Failed to fetch"));

    try {
      await deleteSecret("pub-id", "meta-tok", "del-tok");
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(0);
      expect((e as ApiError).message).toContain("Network error");
      expect((e as ApiError).requestId).toBeTruthy();
    }
  });

  it("returns undefined for 204 responses", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));

    const result = await deleteSecret("pub-id", "meta-tok", "del-tok");
    expect(result).toBeUndefined();
  });
});

describe("retrieval sessions", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("starts a retrieval session with blob token header", async () => {
    const response = {
      session_token: "session-token",
      blob_size: 123,
      expires_at: "2026-05-11T12:00:00Z",
      burn_after_read: true,
    };
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(JSON.stringify(response), { status: 201 }));

    await expect(startRetrievalSession("pub-id", "blob-token")).resolves.toEqual(response);
    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets/pub-id/retrieval-session");
    expect(call[1]).toEqual(expect.objectContaining({ method: "POST" }));
    expectHeader(call[1], "X-Blob-Token", "blob-token");
    expectHeader(call[1], "X-Request-ID");
  });

  it("retrieves a byte range with bearer session and range headers", async () => {
    const bytes = new Uint8Array([1, 2, 3]);
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(bytes, {
        status: 206,
        headers: { "Content-Range": "bytes 5-7/20" },
      }),
    );

    const result = await retrieveSecretRange("pub-id", "session-token", 5, 7);
    expect(result).toEqual(bytes);
    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets/pub-id/blob");
    expect(call[1]).toEqual(expect.objectContaining({ method: "GET" }));
    expectHeader(call[1], "Authorization", "Bearer session-token");
    expectHeader(call[1], "Range", "bytes=5-7");
    expectHeader(call[1], "X-Request-ID");
  });

  it("rejects non-partial range responses", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(new Uint8Array([1, 2, 3])));

    await expect(retrieveSecretRange("pub-id", "session-token", 5, 7)).rejects.toMatchObject({
      status: 200,
      requestId: expect.any(String),
    });
  });

  it("retries a range request after a transient failure", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValueOnce(new TypeError("connection reset"))
      .mockResolvedValueOnce(new Response("oops", { status: 503, headers: { "Retry-After": "0" } }))
      .mockResolvedValueOnce(new Response(new Uint8Array([9, 8, 7]), { status: 206 }));

    const result = await retrieveSecretRange("pub-id", "session-token", 0, 2);
    expect(result).toEqual(new Uint8Array([9, 8, 7]));
    expect(fetchSpy).toHaveBeenCalledTimes(3);
  });

  it("does not retry a range request that failed with a client error", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ error: "invalid session" }), { status: 403 }),
      );

    await expect(retrieveSecretRange("pub-id", "session-token", 0, 2)).rejects.toMatchObject({
      status: 403,
    });
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it("gives up after the last transient attempt", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response("busy", { status: 429, headers: { "Retry-After": "0" } }));

    await expect(retrieveSecretRange("pub-id", "session-token", 0, 2)).rejects.toMatchObject({
      status: 429,
    });
    expect(fetchSpy).toHaveBeenCalledTimes(MAX_TRANSIENT_ATTEMPTS);
  });

  it("classifies transient statuses and honours Retry-After", () => {
    expect([0, 429, 500, 503].every(isTransientStatus)).toBe(true);
    expect([400, 403, 404, 409, 413].some(isTransientStatus)).toBe(false);
    expect(retryDelayMs(1)).toBe(250);
    expect(retryDelayMs(2)).toBe(500);
    expect(retryDelayMs(1, "2")).toBe(2000);
    expect(retryDelayMs(1, "9999")).toBe(30_000);
    expect(retryDelayMs(3, "garbage")).toBe(750);
  });
});

describe("getSecretMetadata", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns metadata with encrypted_meta", async () => {
    const mockResponse = {
      encrypted_meta: "v2$nonce$cipher",
      blob_size: 2048,
      burn_after_read: false,
      expires_at: "2026-03-01T00:00:00Z",
      created_at: "2026-02-28T00:00:00Z",
    };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockResponse), { status: 200 }),
    );

    const result = await getSecretMetadata("pub-id", "meta-token");
    expect(result).toEqual(mockResponse);
  });
});

describe("upload sessions", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("starts an upload session through the v1 API", async () => {
    const params: StartUploadSessionParams = {
      public_id: "pub-id",
      metadata_token: "meta-token",
      blob_token: "blob-token",
      deletion_token: "delete-token",
      encrypted_meta: "v2$nonce$cipher",
      expiration: "1d",
      burn_after_read: false,
      blob_size: 70 * 1024 * 1024,
    };
    const response = {
      session_id: "session-id",
      upload_token: "upload-token",
      public_id: params.public_id,
      part_size: 32 * 1024 * 1024,
      blob_size: params.blob_size,
      expires_at: "2026-05-15T12:00:00Z",
      upload_expires_at: "2026-05-16T12:00:00Z",
      state: "pending",
      uploaded_parts: [],
    };
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(JSON.stringify(response), { status: 201 }));

    await expect(startUploadSession(params)).resolves.toEqual(response);

    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets/uploads");
    expect(call[1]).toEqual(
      expect.objectContaining({
        method: "POST",
      }),
    );
    expectHeader(call[1], "Content-Type", "application/json");
    expectHeader(call[1], "X-Request-ID");
    expect(JSON.parse(call[1]?.body as string)).toEqual(params);
  });

  it("retrieves upload session status with bearer auth", async () => {
    const response = {
      session_id: "session-id",
      public_id: "pub-id",
      part_size: 32 * 1024 * 1024,
      blob_size: 70 * 1024 * 1024,
      expires_at: "2026-05-15T12:00:00Z",
      upload_expires_at: "2026-05-16T12:00:00Z",
      state: "pending",
      uploaded_parts: [{ part_number: 1, offset: 0, size: 5, sha256: "abc", etag: "etag-1" }],
    };
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }));

    await expect(getUploadSession("session-id", "upload-token")).resolves.toEqual(response);
    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets/uploads/session-id");
    expect(call[1]).toEqual(expect.objectContaining({ method: "GET" }));
    expectHeader(call[1], "Authorization", "Bearer upload-token");
    expectHeader(call[1], "X-Request-ID");
  });

  it("uploads a multipart part with offset, size, and hash headers", async () => {
    const response = { part_number: 2, offset: 5, size: 3, sha256: "hash", etag: "etag-2" };
    const blob = new Blob(["abc"]);
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(JSON.stringify(response), { status: 200 }));

    await expect(
      uploadSessionPart("session-id", "upload-token", 2, 5, blob, "hash"),
    ).resolves.toEqual(response);

    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets/uploads/session-id/parts/2");
    expect(call[1]).toEqual(
      expect.objectContaining({
        method: "PUT",
        body: blob,
      }),
    );
    expectHeader(call[1], "Authorization", "Bearer upload-token");
    expectHeader(call[1], "Content-Type", "application/octet-stream");
    expectHeader(call[1], "X-Part-Offset", "5");
    expectHeader(call[1], "X-Part-Size", "3");
    expectHeader(call[1], "X-Part-SHA256", "hash");
    expectHeader(call[1], "X-Request-ID");
  });

  it("completes and aborts upload sessions through the v1 API", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ expires_at: "2026-05-15T12:00:00Z" }), { status: 201 }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(completeUploadSession("session-id", "upload-token")).resolves.toEqual({
      expires_at: "2026-05-15T12:00:00Z",
    });
    await expect(abortUploadSession("session-id", "upload-token")).resolves.toBeUndefined();

    expect(fetchSpy.mock.calls[0][0]).toBe("/api/v1/secrets/uploads/session-id/complete");
    expect(fetchSpy.mock.calls[0][1]).toEqual(
      expect.objectContaining({
        method: "POST",
      }),
    );
    expectHeader(fetchSpy.mock.calls[0][1], "Authorization", "Bearer upload-token");
    expectHeader(fetchSpy.mock.calls[0][1], "X-Request-ID");
    expect(fetchSpy.mock.calls[1][0]).toBe("/api/v1/secrets/uploads/session-id");
    expect(fetchSpy.mock.calls[1][1]).toEqual(
      expect.objectContaining({
        method: "DELETE",
      }),
    );
    expectHeader(fetchSpy.mock.calls[1][1], "Authorization", "Bearer upload-token");
    expectHeader(fetchSpy.mock.calls[1][1], "X-Request-ID");
  });
});

describe("deleteSecret", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends DELETE with token headers", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 204 }));

    const result = await deleteSecret("pub-id", "meta-tok", "del-tok");
    expect(result).toBeUndefined();

    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets/pub-id");
    expect(call[1]).toEqual(expect.objectContaining({ method: "DELETE" }));
    expectHeader(call[1], "X-Metadata-Token", "meta-tok");
    expectHeader(call[1], "X-Deletion-Token", "del-tok");
    expectHeader(call[1], "X-Request-ID");
  });
});

function expectHeader(init: RequestInit | undefined, name: string, value?: string) {
  expect(init?.headers).toBeInstanceOf(Headers);
  const headers = init?.headers as Headers;
  if (value === undefined) {
    expect(headers.get(name)).toBeTruthy();
    return;
  }
  expect(headers.get(name)).toBe(value);
}
