import {
  ApiError,
  createSecret,
  retrieveSecret,
  deleteSecret,
  getSecretMetadata,
  type CreateSecretParams,
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
      await deleteSecret("pub-id", "ret-tok", "del-tok");
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
      await deleteSecret("pub-id", "ret-tok", "del-tok");
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
      await deleteSecret("pub-id", "ret-tok", "del-tok");
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(0);
      expect((e as ApiError).message).toContain("Network error");
    }
  });

  it("returns undefined for 204 responses", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 204 }),
    );

    const result = await deleteSecret("pub-id", "ret-tok", "del-tok");
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
      retrieval_token: "ret-tok",
      deletion_token: "del-tok",
      encrypted_meta: "v1$nonce$ciphertext",
      expiration: "5m",
      burn_after_read: true,
    };
    const blob = new Blob(["encrypted-data"]);

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ expires_at: "2026-02-26T00:05:00Z" }), { status: 200 }),
    );

    const result = await createSecret(params, blob);
    expect(result.expires_at).toBe("2026-02-26T00:05:00Z");

    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets");
    const body = call[1]?.body as FormData;
    expect(body).toBeInstanceOf(FormData);
    expect(body.get("metadata")).toBe(JSON.stringify(params));
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

    const result = await retrieveSecret("pub-id", "ret-token");
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

    const result = await retrieveSecret("pub-id", "ret-token");
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

describe("getSecretMetadata", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns metadata with encrypted_meta", async () => {
    const mockResponse = {
      encrypted_meta: "v1$nonce$cipher",
      blob_size: 2048,
      burn_after_read: false,
      expires_at: "2026-03-01T00:00:00Z",
      created_at: "2026-02-28T00:00:00Z",
    };
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockResponse), { status: 200 }),
    );

    const result = await getSecretMetadata("pub-id", "ret-token");
    expect(result).toEqual(mockResponse);
  });
});

describe("deleteSecret", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends DELETE with token headers", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 204 }),
    );

    const result = await deleteSecret("pub-id", "ret-tok", "del-tok");
    expect(result).toBeUndefined();

    expect(fetchSpy).toHaveBeenCalledWith("/api/v1/secrets/pub-id", expect.objectContaining({
      method: "DELETE",
    }));
  });
});
