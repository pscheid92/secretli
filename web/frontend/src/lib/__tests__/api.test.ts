import {
  ApiError,
  abortUploadSession,
  type CreateSecretParams,
  completeUploadSession,
  createSecret,
  deleteSecret,
  getSecretMetadata,
  getUploadSession,
  retrieveSecret,
  retrieveSecretRange,
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
      expect((e as ApiError).message).toBe("unauthorized");
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
      expect((e as ApiError).message).toBe("Request failed (500)");
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
    }
  });

  it("returns undefined for 204 responses", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 204 }));

    const result = await deleteSecret("pub-id", "meta-tok", "del-tok");
    expect(result).toBeUndefined();
  });
});

describe("createSecret", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends multipart form with metadata and file blob", async () => {
    const params: CreateSecretParams = {
      public_id: "pub123",
      metadata_token: "meta-tok",
      blob_token: "blob-tok",
      deletion_token: "del-tok",
      encrypted_meta: "v2$nonce$ciphertext",
      expiration: "5m",
      burn_after_read: true,
    };
    const blob = new Blob(["encrypted-data"]);

    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ expires_at: "2026-02-26T00:05:00Z" }), { status: 200 }),
      );

    const result = await createSecret(params, blob);
    expect(result.expires_at).toBe("2026-02-26T00:05:00Z");

    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets");
    const body = call[1]?.body as FormData;
    expect(body).toBeInstanceOf(FormData);
    expect(body.get("public_id")).toBe(params.public_id);
    expect(body.get("metadata_token")).toBe(params.metadata_token);
    expect(body.get("blob_token")).toBe(params.blob_token);
    expect(body.get("deletion_token")).toBe(params.deletion_token);
    expect(body.get("encrypted_meta")).toBe(params.encrypted_meta);
    expect(body.get("expiration")).toBe(params.expiration);
    expect(body.get("burn_after_read")).toBe(String(params.burn_after_read));
    expect(body.get("file")).toBeInstanceOf(Blob);
  });
});

describe("retrieveSecret", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns blob and burn_after_read header", async () => {
    const headers = new Headers({
      "X-Burn-After-Read": "true",
    });
    const fileBlob = new Blob(["encrypted-blob-data"]);
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      headers,
      blob: () => Promise.resolve(fileBlob),
    } as unknown as Response);

    const result = await retrieveSecret("pub-id", "blob-token");
    expect(result.burnAfterRead).toBe(true);

    const text = await result.blob.text();
    expect(text).toBe("encrypted-blob-data");
  });

  it("defaults burn_after_read to false when header missing", async () => {
    const fileBlob = new Blob(["data"]);
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers(),
      blob: () => Promise.resolve(fileBlob),
    } as unknown as Response);

    const result = await retrieveSecret("pub-id", "blob-token");
    expect(result.burnAfterRead).toBe(false);
  });

  it("throws ApiError on 404", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "secret not found" }), { status: 404 }),
    );

    await expect(retrieveSecret("missing", "tok")).rejects.toThrow(ApiError);
  });

  it("throws ApiError on network failure", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("Network error"));

    try {
      await retrieveSecret("pub", "tok");
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(0);
    }
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
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/secrets/pub-id/retrieval-session",
      expect.objectContaining({
        method: "POST",
        headers: { "X-Blob-Token": "blob-token" },
      }),
    );
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
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/secrets/pub-id/blob",
      expect.objectContaining({
        method: "GET",
        headers: {
          Authorization: "Bearer session-token",
          Range: "bytes=5-7",
        },
      }),
    );
  });

  it("rejects non-partial range responses", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(new Uint8Array([1, 2, 3])));

    await expect(retrieveSecretRange("pub-id", "session-token", 5, 7)).rejects.toMatchObject({
      status: 200,
      message: "Expected partial content response (200)",
    });
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
        headers: { "Content-Type": "application/json" },
      }),
    );
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
    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/secrets/uploads/session-id",
      expect.objectContaining({
        method: "GET",
        headers: { Authorization: "Bearer upload-token" },
      }),
    );
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
        headers: {
          Authorization: "Bearer upload-token",
          "Content-Type": "application/octet-stream",
          "X-Part-Offset": "5",
          "X-Part-Size": "3",
          "X-Part-SHA256": "hash",
        },
        body: blob,
      }),
    );
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
        headers: { Authorization: "Bearer upload-token" },
      }),
    );
    expect(fetchSpy.mock.calls[1][0]).toBe("/api/v1/secrets/uploads/session-id");
    expect(fetchSpy.mock.calls[1][1]).toEqual(
      expect.objectContaining({
        method: "DELETE",
        headers: { Authorization: "Bearer upload-token" },
      }),
    );
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

    expect(fetchSpy).toHaveBeenCalledWith(
      "/api/v1/secrets/pub-id",
      expect.objectContaining({
        method: "DELETE",
        headers: {
          "X-Metadata-Token": "meta-tok",
          "X-Deletion-Token": "del-tok",
        },
      }),
    );
  });
});
