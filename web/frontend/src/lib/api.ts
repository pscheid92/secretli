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
