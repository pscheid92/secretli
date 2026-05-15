export class ApiError extends Error {
  readonly status: number;
  readonly requestId?: string;

  constructor(status: number, message: string, requestId?: string) {
    super(requestId ? `${message} (request id: ${requestId})` : message);
    this.name = "ApiError";
    this.status = status;
    this.requestId = requestId;
  }
}

async function request<T>(url: string, init: RequestInit): Promise<T> {
  const { requestID, init: requestInit } = withRequestID(init);
  let res: Response;
  try {
    res = await fetch(url, requestInit);
  } catch {
    throw new ApiError(0, "Network error — please check your connection", requestID);
  }

  if (!res.ok) {
    throw await apiErrorFromResponse(res, requestID);
  }

  if (res.status === 204) return undefined as T;
  return res.json();
}

// --- Create ---

export interface CreateSecretParams {
  public_id: string;
  metadata_token: string;
  blob_token: string;
  deletion_token: string;
  encrypted_meta: string;
  expiration: string;
  burn_after_read: boolean;
}

export interface CreateSecretResponse {
  expires_at: string;
}

export function createSecret(
  params: CreateSecretParams,
  blob: Blob,
): Promise<CreateSecretResponse> {
  const form = new FormData();
  form.append("public_id", params.public_id);
  form.append("metadata_token", params.metadata_token);
  form.append("blob_token", params.blob_token);
  form.append("deletion_token", params.deletion_token);
  form.append("encrypted_meta", params.encrypted_meta);
  form.append("expiration", params.expiration);
  form.append("burn_after_read", String(params.burn_after_read));
  form.append("file", blob);

  return request("/api/v1/secrets", {
    method: "POST",
    body: form,
  });
}

// --- Retrieve (streams blob) ---

export interface RetrieveSecretResponse {
  blob: Blob;
  burnAfterRead: boolean;
}

export async function retrieveSecret(
  publicID: string,
  blobToken: string,
): Promise<RetrieveSecretResponse> {
  const { requestID, init } = withRequestID({
    method: "POST",
    headers: { "X-Blob-Token": blobToken },
  });
  let res: Response;
  try {
    res = await fetch(`/api/v1/secrets/${publicID}`, init);
  } catch {
    throw new ApiError(0, "Network error — please check your connection", requestID);
  }

  if (!res.ok) {
    throw await apiErrorFromResponse(res, requestID);
  }

  const blob = await res.blob();
  return {
    blob,
    burnAfterRead: res.headers.get("X-Burn-After-Read") === "true",
  };
}

// --- Retrieval sessions and ranges ---

export interface RetrievalSessionResponse {
  session_token: string;
  blob_size: number;
  expires_at: string;
  burn_after_read: boolean;
}

export async function startRetrievalSession(
  publicID: string,
  blobToken: string,
): Promise<RetrievalSessionResponse> {
  return request(`/api/v1/secrets/${publicID}/retrieval-session`, {
    method: "POST",
    headers: { "X-Blob-Token": blobToken },
  });
}

export async function retrieveSecretRange(
  publicID: string,
  sessionToken: string,
  start: number,
  end: number,
): Promise<Uint8Array> {
  const { requestID, init } = withRequestID({
    method: "GET",
    headers: {
      Authorization: `Bearer ${sessionToken}`,
      Range: `bytes=${start}-${end}`,
    },
  });
  let res: Response;
  try {
    res = await fetch(`/api/v1/secrets/${publicID}/blob`, init);
  } catch {
    throw new ApiError(0, "Network error — please check your connection", requestID);
  }

  if (!res.ok) {
    throw await apiErrorFromResponse(res, requestID);
  }
  if (res.status !== 206) {
    throw new ApiError(
      res.status,
      `Expected partial content response (${res.status})`,
      requestIDFromResponse(res, requestID),
    );
  }

  return new Uint8Array(await res.arrayBuffer());
}

async function apiErrorFromResponse(res: Response, fallbackRequestID: string): Promise<ApiError> {
  const body = await res.json().catch(() => null);
  const message = body?.error ?? `Request failed (${res.status})`;
  return new ApiError(res.status, message, requestIDFromResponse(res, fallbackRequestID));
}

function withRequestID(init: RequestInit): { requestID: string; init: RequestInit } {
  const requestID = newRequestID();
  const headers = new Headers(init.headers);
  headers.set("X-Request-ID", requestID);
  return {
    requestID,
    init: { credentials: "same-origin", ...init, headers },
  };
}

function requestIDFromResponse(res: Response, fallbackRequestID: string): string {
  return res.headers.get("X-Request-ID") || fallbackRequestID;
}

function newRequestID(): string {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `req-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

// --- Metadata ---

export interface SecretMetadataResponse {
  encrypted_meta: string;
  blob_size: number;
  burn_after_read: boolean;
  expires_at: string;
  created_at: string;
}

export function getSecretMetadata(
  publicID: string,
  metadataToken: string,
): Promise<SecretMetadataResponse> {
  return request(`/api/v1/secrets/${publicID}/meta`, {
    method: "GET",
    headers: { "X-Metadata-Token": metadataToken },
  });
}

// --- Delete ---

export function deleteSecret(
  publicID: string,
  metadataToken: string,
  deletionToken: string,
): Promise<void> {
  return request(`/api/v1/secrets/${publicID}`, {
    method: "DELETE",
    headers: {
      "X-Metadata-Token": metadataToken,
      "X-Deletion-Token": deletionToken,
    },
  });
}

// --- Upload sessions ---

export interface UploadSessionPart {
  readonly part_number: number;
  readonly offset: number;
  readonly size: number;
  readonly sha256: string;
  readonly etag?: string;
}

export interface StartUploadSessionParams {
  readonly public_id: string;
  readonly metadata_token: string;
  readonly blob_token: string;
  readonly deletion_token: string;
  readonly encrypted_meta: string;
  readonly expiration: string;
  readonly burn_after_read: boolean;
  readonly blob_size: number;
}

export interface UploadSessionStatus {
  readonly session_id: string;
  readonly upload_token?: string;
  readonly public_id: string;
  readonly part_size: number;
  readonly blob_size: number;
  readonly expires_at: string;
  readonly upload_expires_at: string;
  readonly state: "pending" | "completed" | "aborted";
  readonly uploaded_parts: UploadSessionPart[];
}

export interface StartUploadSessionResponse extends UploadSessionStatus {
  readonly upload_token: string;
}

export interface CompleteUploadSessionResponse {
  readonly expires_at: string;
}

export function startUploadSession(
  params: StartUploadSessionParams,
): Promise<StartUploadSessionResponse> {
  return request("/api/v1/secrets/uploads", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  });
}

export function getUploadSession(
  sessionID: string,
  uploadToken: string,
): Promise<UploadSessionStatus> {
  return request(`/api/v1/secrets/uploads/${sessionID}`, {
    method: "GET",
    headers: { Authorization: `Bearer ${uploadToken}` },
  });
}

export function completeUploadSession(
  sessionID: string,
  uploadToken: string,
): Promise<CompleteUploadSessionResponse> {
  return request(`/api/v1/secrets/uploads/${sessionID}/complete`, {
    method: "POST",
    headers: { Authorization: `Bearer ${uploadToken}` },
  });
}

export function abortUploadSession(sessionID: string, uploadToken: string): Promise<void> {
  return request(`/api/v1/secrets/uploads/${sessionID}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${uploadToken}` },
  });
}

export function uploadSessionPart(
  sessionID: string,
  uploadToken: string,
  partNumber: number,
  offset: number,
  bytes: Blob,
  sha256: string,
): Promise<UploadSessionPart> {
  return request(`/api/v1/secrets/uploads/${sessionID}/parts/${partNumber}`, {
    method: "PUT",
    headers: {
      Authorization: `Bearer ${uploadToken}`,
      "Content-Type": "application/octet-stream",
      "X-Part-Offset": String(offset),
      "X-Part-Size": String(bytes.size),
      "X-Part-SHA256": sha256,
    },
    body: bytes,
  });
}
