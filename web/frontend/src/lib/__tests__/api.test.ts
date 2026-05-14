import {
  ApiError,
  type CreateSecretParams,
  createSecret,
  deleteSecret,
  getSecretMetadata,
  retrieveSecret,
  retrieveSecretRange,
  startRetrievalSession,
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
