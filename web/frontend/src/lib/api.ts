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

export interface CreateSecretParams {
  public_id: string;
  retrieval_token: string;
  deletion_token: string;
  nonce: string;
  encrypted_data: string;
  expiration: string;
  burn_after_read: boolean;
  password_protected: boolean;
}

export interface CreateSecretResponse {
  expires_at: string;
}

export interface RetrieveSecretResponse {
  nonce: string;
  encrypted_data: string;
  secret_type: string;
  burn_after_read: boolean;
  password_protected: boolean;
}

export function createSecret(params: CreateSecretParams): Promise<CreateSecretResponse> {
  return request("/api/v1/secrets", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  });
}

export function retrieveSecret(
  publicID: string,
  retrievalToken: string,
): Promise<RetrieveSecretResponse> {
  return request(`/api/v1/secrets/${publicID}`, {
    method: "POST",
    headers: { "X-Retrieval-Token": retrievalToken },
  });
}

export interface UploadFileMetadata {
  public_id: string;
  retrieval_token: string;
  deletion_token: string;
  nonce: string;
  expiration: string;
  burn_after_read: boolean;
  password_protected: boolean;
  encrypted_filename: string;
}

export interface UploadFileResponse {
  expires_at: string;
}

export async function uploadFile(
  metadata: UploadFileMetadata,
  encryptedBlob: Blob,
): Promise<UploadFileResponse> {
  const form = new FormData();
  form.append("metadata", JSON.stringify(metadata));
  form.append("file", encryptedBlob);

  return request("/api/v1/secrets/file", {
    method: "POST",
    body: form,
  });
}

export interface DownloadFileResponse {
  blob: Blob;
  encryptedFilename: string;
  nonce: string;
  burnAfterRead: boolean;
  passwordProtected: boolean;
}

export async function downloadFile(
  publicID: string,
  retrievalToken: string,
): Promise<DownloadFileResponse> {
  let res: Response;
  try {
    res = await fetch(`/api/v1/secrets/${publicID}/file`, {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Retrieval-Token": retrievalToken },
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
    encryptedFilename: res.headers.get("X-Encrypted-Filename") ?? "",
    nonce: res.headers.get("X-Nonce") ?? "",
    burnAfterRead: res.headers.get("X-Burn-After-Read") === "true",
    passwordProtected: res.headers.get("X-Password-Protected") === "true",
  };
}

// --- Metadata ---

export interface SecretMetadataResponse {
  secret_type: string;
  burn_after_read: boolean;
  password_protected: boolean;
  expires_at: string;
  created_at: string;
  file_size?: number;
}

export function getSecretMetadata(
  publicID: string,
  retrievalToken: string,
): Promise<SecretMetadataResponse> {
  return request(`/api/v1/secrets/${publicID}/meta`, {
    method: "GET",
    headers: { "X-Retrieval-Token": retrievalToken },
  });
}

// --- Auth ---

export interface User {
  id: number;
  email: string;
  display_name: string;
  created_at: string;
}

export function register(email: string, password: string, displayName: string): Promise<User> {
  return request("/api/v1/auth/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, display_name: displayName }),
  });
}

export function login(email: string, password: string): Promise<User> {
  return request("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
}

export function logout(): Promise<void> {
  return request("/api/v1/auth/logout", { method: "POST" });
}

export function getCurrentUser(): Promise<User> {
  return request("/api/v1/auth/me", { method: "GET" });
}

// --- User Secrets ---

export interface SecretSummary {
  public_id: string;
  label: string;
  secret_type: string;
  burn_after_read: boolean;
  password_protected: boolean;
  expires_at: string;
  created_at: string;
  retrieved_at: string | null;
}

export interface UserSecretsResponse {
  secrets: SecretSummary[];
  page: number;
  per_page: number;
  total: number;
}

export function getUserSecrets(page = 1, perPage = 20): Promise<UserSecretsResponse> {
  return request(`/api/v1/user/secrets?page=${page}&per_page=${perPage}`, {
    method: "GET",
  });
}

export function deleteSecret(
  publicID: string,
  retrievalToken: string,
  deletionToken: string,
): Promise<void> {
  return request(`/api/v1/secrets/${publicID}`, {
    method: "DELETE",
    headers: {
      "X-Retrieval-Token": retrievalToken,
      "X-Deletion-Token": deletionToken,
    },
  });
}
