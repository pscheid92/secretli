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
  retrieval_token: string;
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
  form.append("metadata", JSON.stringify(params));
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
  retrievalToken: string,
): Promise<RetrieveSecretResponse> {
  let res: Response;
  try {
    res = await fetch(`/api/v1/secrets/${publicID}`, {
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
  retrievalToken: string,
): Promise<SecretMetadataResponse> {
  return request(`/api/v1/secrets/${publicID}/meta`, {
    method: "GET",
    headers: { "X-Retrieval-Token": retrievalToken },
  });
}

// --- Delete ---

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
