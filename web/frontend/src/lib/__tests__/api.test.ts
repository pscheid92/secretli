import {
  ApiError,
  createSecret,
  retrieveSecret,
  uploadFile,
  downloadFile,
  deleteSecret,
  type CreateSecretParams,
  type UploadFileMetadata,
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

  it("sends POST with JSON body and returns expires_at", async () => {
    const params: CreateSecretParams = {
      public_id: "pub123",
      retrieval_token: "ret-tok",
      deletion_token: "del-tok",
      nonce: "nonce123",
      encrypted_data: "data123",
      expiration: "5m",
      burn_after_read: true,
      password_protected: false,
    };

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ expires_at: "2026-02-26T00:05:00Z" }), { status: 200 }),
    );

    const result = await createSecret(params);
    expect(result.expires_at).toBe("2026-02-26T00:05:00Z");

    expect(fetchSpy).toHaveBeenCalledWith("/api/v1/secrets", expect.objectContaining({
      method: "POST",
      body: JSON.stringify(params),
    }));
  });
});

describe("retrieveSecret", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends POST with retrieval token header", async () => {
    const mockResponse = {
      nonce: "nonce123",
      encrypted_data: "data123",
      secret_type: "text",
      burn_after_read: false,
      password_protected: true,
    };
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify(mockResponse), { status: 200 }),
    );

    const result = await retrieveSecret("pub-id", "ret-token");
    expect(result).toEqual(mockResponse);

    expect(fetchSpy).toHaveBeenCalledWith("/api/v1/secrets/pub-id", expect.objectContaining({
      method: "POST",
    }));
  });

  it("throws ApiError on 404", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "secret not found" }), { status: 404 }),
    );

    await expect(retrieveSecret("missing", "tok")).rejects.toThrow(ApiError);
  });
});

describe("uploadFile", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends multipart form with metadata and file", async () => {
    const metadata: UploadFileMetadata = {
      public_id: "file-pub",
      retrieval_token: "ret-tok",
      deletion_token: "del-tok",
      nonce: "nonce",
      expiration: "1d",
      burn_after_read: false,
      password_protected: false,
      encrypted_filename: "enc-name",
    };
    const blob = new Blob(["encrypted-data"]);

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ expires_at: "2026-02-27T00:00:00Z" }), { status: 200 }),
    );

    const result = await uploadFile(metadata, blob);
    expect(result.expires_at).toBe("2026-02-27T00:00:00Z");

    const call = fetchSpy.mock.calls[0];
    expect(call[0]).toBe("/api/v1/secrets/file");
    const body = call[1]?.body as FormData;
    expect(body).toBeInstanceOf(FormData);
    expect(body.get("metadata")).toBe(JSON.stringify(metadata));
    expect(body.get("file")).toBeInstanceOf(Blob);
  });
});

describe("downloadFile", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("extracts response headers into result", async () => {
    const headers = new Headers({
      "X-Encrypted-Filename": "enc-fname",
      "X-Nonce": "file-nonce",
      "X-Burn-After-Read": "true",
      "X-Password-Protected": "false",
    });
    const fileBlob = new Blob(["file-data"]);
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      headers,
      blob: () => Promise.resolve(fileBlob),
      json: () => Promise.resolve({}),
    } as unknown as Response);

    const result = await downloadFile("pub-id", "ret-tok");
    expect(result.encryptedFilename).toBe("enc-fname");
    expect(result.nonce).toBe("file-nonce");
    expect(result.burnAfterRead).toBe(true);
    expect(result.passwordProtected).toBe(false);

    const text = await result.blob.text();
    expect(text).toBe("file-data");
  });

  it("defaults missing headers to empty/false", async () => {
    const fileBlob = new Blob(["data"]);
    vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      status: 200,
      headers: new Headers(),
      blob: () => Promise.resolve(fileBlob),
      json: () => Promise.resolve({}),
    } as unknown as Response);

    const result = await downloadFile("pub-id", "ret-tok");
    expect(result.encryptedFilename).toBe("");
    expect(result.nonce).toBe("");
    expect(result.burnAfterRead).toBe(false);
    expect(result.passwordProtected).toBe(false);
  });

  it("throws ApiError on server error", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ error: "not found" }), { status: 404 }),
    );

    await expect(downloadFile("missing", "tok")).rejects.toThrow(ApiError);
  });

  it("throws ApiError on network failure", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("Network error"));

    try {
      await downloadFile("pub", "tok");
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(0);
    }
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
