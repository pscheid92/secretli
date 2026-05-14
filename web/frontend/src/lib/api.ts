export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(url: string, init: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(url, { credentials: "same-origin", ...init });
  } catch {
    throw new ApiError(0, "Network error — please check your connection");
  }

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const message = body?.error ?? `Request failed (${res.status})`;
    throw new ApiError(res.status, message);
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

// --- Chunked v2 upload ---

export interface CreateChunkedUploadParams extends CreateSecretParams {
  chunk_size: number;
  chunk_count: number;
  encrypted_total_size: number;
}

export interface CreateChunkedUploadResponse {
  public_id: string;
  upload_token: string;
  upload_expires_at: string;
  chunk_size: number;
}

export interface UploadedChunkStatus {
  index: number;
  encrypted_size: number;
  sha256: string;
}

export interface UploadedManifestStatus {
  encrypted_size: number;
  sha256: string;
}

export interface ChunkedUploadStatus {
  public_id: string;
  upload_expires_at: string;
  chunk_size: number;
  chunk_count: number;
  encrypted_total_size: number;
  chunks: UploadedChunkStatus[];
  manifest: UploadedManifestStatus | null;
}

export function createChunkedUpload(
  params: CreateChunkedUploadParams,
): Promise<CreateChunkedUploadResponse> {
  return request("/api/v2/secrets/uploads", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  });
}

export function getChunkedUploadStatus(
  publicID: string,
  uploadToken: string,
): Promise<ChunkedUploadStatus> {
  return request(`/api/v2/secrets/${publicID}/upload`, {
    method: "GET",
    headers: { Authorization: `Bearer ${uploadToken}` },
  });
}

export function uploadEncryptedChunk(
  publicID: string,
  uploadToken: string,
  index: number,
  bytes: Uint8Array,
  sha256: string,
  onProgress?: (loaded: number, total: number) => void,
): Promise<UploadedChunkStatus> {
  return uploadEncryptedObject(
    `/api/v2/secrets/${publicID}/chunks/${index}`,
    uploadToken,
    bytes,
    sha256,
    onProgress,
  );
}

export function uploadEncryptedManifest(
  publicID: string,
  uploadToken: string,
  bytes: Uint8Array,
  sha256: string,
  onProgress?: (loaded: number, total: number) => void,
): Promise<UploadedChunkStatus> {
  return uploadEncryptedObject(
    `/api/v2/secrets/${publicID}/manifest`,
    uploadToken,
    bytes,
    sha256,
    onProgress,
  );
}

export function completeChunkedUpload(
  publicID: string,
  uploadToken: string,
): Promise<CreateSecretResponse> {
  return request(`/api/v2/secrets/${publicID}/complete`, {
    method: "POST",
    headers: { Authorization: `Bearer ${uploadToken}` },
  });
}

export function cancelChunkedUpload(publicID: string, uploadToken: string): Promise<void> {
  return request(`/api/v2/secrets/${publicID}/upload`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${uploadToken}` },
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
  let res: Response;
  try {
    res = await fetch(`/api/v1/secrets/${publicID}`, {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Blob-Token": blobToken },
    });
  } catch {
    throw new ApiError(0, "Network error — please check your connection");
  }

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const message = body?.error ?? `Request failed (${res.status})`;
    throw new ApiError(res.status, message);
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
  storage_version?: "single-v1" | "chunked-v1";
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
  let res: Response;
  try {
    res = await fetch(`/api/v1/secrets/${publicID}/blob`, {
      method: "GET",
      credentials: "same-origin",
      headers: {
        Authorization: `Bearer ${sessionToken}`,
        Range: `bytes=${start}-${end}`,
      },
    });
  } catch {
    throw new ApiError(0, "Network error — please check your connection");
  }

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const message = body?.error ?? `Request failed (${res.status})`;
    throw new ApiError(res.status, message);
  }
  if (res.status !== 206) {
    throw new ApiError(res.status, `Expected partial content response (${res.status})`);
  }

  return new Uint8Array(await res.arrayBuffer());
}

export async function startChunkedRetrievalSession(
  publicID: string,
  blobToken: string,
): Promise<RetrievalSessionResponse> {
  return request(`/api/v2/secrets/${publicID}/retrieval-session`, {
    method: "POST",
    headers: { "X-Blob-Token": blobToken },
  });
}

export async function retrieveChunkedManifest(
  publicID: string,
  sessionToken: string,
): Promise<Uint8Array> {
  return retrieveChunkedObject(`/api/v2/secrets/${publicID}/manifest`, sessionToken);
}

export async function retrieveChunkedChunk(
  publicID: string,
  sessionToken: string,
  index: number,
): Promise<Uint8Array> {
  return retrieveChunkedObject(`/api/v2/secrets/${publicID}/chunks/${index}`, sessionToken);
}

// --- Metadata ---

export interface SecretMetadataResponse {
  encrypted_meta: string;
  blob_size: number;
  burn_after_read: boolean;
  expires_at: string;
  created_at: string;
  storage_version?: "single-v1" | "chunked-v1";
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

function uploadEncryptedObject(
  url: string,
  uploadToken: string,
  bytes: Uint8Array,
  sha256: string,
  onProgress?: (loaded: number, total: number) => void,
): Promise<UploadedChunkStatus> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url);
    xhr.withCredentials = true;
    xhr.responseType = "json";
    xhr.setRequestHeader("Authorization", `Bearer ${uploadToken}`);
    xhr.setRequestHeader("Content-Type", "application/octet-stream");
    xhr.setRequestHeader("X-Encrypted-SHA256", sha256);
    xhr.upload.onprogress = (event) => {
      onProgress?.(event.loaded, event.lengthComputable ? event.total : bytes.byteLength);
    };
    xhr.onerror = () => reject(new ApiError(0, "Network error — please check your connection"));
    xhr.onload = () => {
      if (xhr.status < 200 || xhr.status >= 300) {
        const response =
          typeof xhr.response === "object" && xhr.response !== null ? xhr.response : null;
        const message =
          (response as { error?: string } | null)?.error ?? `Request failed (${xhr.status})`;
        reject(new ApiError(xhr.status, message));
        return;
      }
      resolve(xhr.response as UploadedChunkStatus);
    };
    xhr.send(new Blob([arrayBufferFromBytes(bytes)]));
  });
}

async function retrieveChunkedObject(url: string, sessionToken: string): Promise<Uint8Array> {
  let res: Response;
  try {
    res = await fetch(url, {
      method: "GET",
      credentials: "same-origin",
      headers: { Authorization: `Bearer ${sessionToken}` },
    });
  } catch {
    throw new ApiError(0, "Network error — please check your connection");
  }

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const message = body?.error ?? `Request failed (${res.status})`;
    throw new ApiError(res.status, message);
  }

  return new Uint8Array(await res.arrayBuffer());
}

function arrayBufferFromBytes(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}
