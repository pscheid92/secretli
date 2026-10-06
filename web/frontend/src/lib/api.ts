export class ApiError extends Error {
  readonly status: number;
  readonly requestId?: string;
  /** The server's Retry-After header, for callers that retry. */
  readonly retryAfter?: string;
  /** Structured details from the error body, such as why a transfer ended. */
  readonly details?: Readonly<Record<string, unknown>>;

  constructor(
    status: number,
    message: string,
    requestId?: string,
    retryAfter?: string,
    details?: Readonly<Record<string, unknown>>,
  ) {
    super(requestId ? `${message} (request id: ${requestId})` : message);
    this.name = "ApiError";
    this.status = status;
    this.requestId = requestId;
    this.retryAfter = retryAfter;
    this.details = details;
  }
}

export const MAX_TRANSIENT_ATTEMPTS = 3;
const MAX_RETRY_AFTER_MS = 30_000;

/**
 * Whether a failed request is worth retrying: network failures, server
 * errors and rate limiting. Client errors (4xx) are deterministic and must
 * surface immediately.
 */
export function isTransientStatus(status: number): boolean {
  return status === 0 || status === 429 || status >= 500;
}

/**
 * Delay before retrying a transient failure, honouring Retry-After (seconds)
 * from rate-limit responses up to a cap.
 */
export function retryDelayMs(attempt: number, retryAfterHeader?: string | null): number {
  if (retryAfterHeader) {
    const seconds = Number.parseInt(retryAfterHeader, 10);
    if (Number.isFinite(seconds) && seconds >= 0) {
      return Math.min(seconds * 1000, MAX_RETRY_AFTER_MS);
    }
  }
  return 250 * attempt;
}

function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

async function doFetch(url: string, init: RequestInit, requestID: string): Promise<Response> {
  try {
    return await fetch(url, init);
  } catch (err) {
    if (isAbortError(err)) throw err;
    throw new ApiError(0, "Network error — please check your connection", requestID);
  }
}

/**
 * Reads a response body. A connection lost after the headers arrived rejects
 * with a TypeError; report it as the network failure it is, so callers retry
 * it instead of treating it as an unexpected error.
 */
async function readBody<T>(read: () => Promise<T>, requestID: string): Promise<T> {
  try {
    return await read();
  } catch (err) {
    if (err instanceof TypeError) {
      throw new ApiError(0, "Network error — please check your connection", requestID);
    }
    throw err;
  }
}

async function request<T>(url: string, init: RequestInit): Promise<T> {
  const { requestID, init: requestInit } = withRequestID(init);
  const res = await doFetch(url, requestInit, requestID);

  if (!res.ok) {
    throw await apiErrorFromResponse(res, requestID);
  }

  if (res.status === 204) return undefined as T;
  return readBody(() => res.json(), requestID);
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// --- Retrieval sessions and ranges ---

export interface RetrievalSessionResponse {
  session_token: string;
  blob_size: number;
  expires_at: string;
  burn_after_read: boolean;
}

/**
 * Opens a secret for reading. The owner's deletion token, when there is one,
 * tells the server that the owner is looking rather than a recipient getting
 * it.
 */
export async function startRetrievalSession(
  publicID: string,
  blobToken: string,
  deletionToken?: string,
): Promise<RetrievalSessionResponse> {
  return request(`/api/v1/secrets/${publicID}/retrieval-session`, {
    method: "POST",
    headers: deletionToken
      ? { "X-Blob-Token": blobToken, "X-Deletion-Token": deletionToken }
      : { "X-Blob-Token": blobToken },
  });
}

export async function retrieveSecretRange(
  publicID: string,
  sessionToken: string,
  start: number,
  end: number,
  signal?: AbortSignal,
): Promise<Uint8Array> {
  // A gigabyte download is hundreds of range requests; one transient failure
  // must not throw the whole transfer away.
  let lastError: ApiError | undefined;
  for (let attempt = 1; attempt <= MAX_TRANSIENT_ATTEMPTS; attempt++) {
    const { requestID, init } = withRequestID({
      method: "GET",
      headers: {
        Authorization: `Bearer ${sessionToken}`,
        Range: `bytes=${start}-${end}`,
      },
      signal,
    });
    let res: Response;
    try {
      res = await doFetch(`/api/v1/secrets/${publicID}/blob`, init, requestID);
    } catch (err) {
      if (!(err instanceof ApiError)) throw err;
      lastError = err;
      if (attempt < MAX_TRANSIENT_ATTEMPTS) await delay(retryDelayMs(attempt));
      continue;
    }

    if (!res.ok) {
      const apiErr = await apiErrorFromResponse(res, requestID);
      if (!isTransientStatus(res.status)) throw apiErr;
      lastError = apiErr;
      if (attempt < MAX_TRANSIENT_ATTEMPTS) {
        await delay(retryDelayMs(attempt, res.headers.get("Retry-After")));
      }
      continue;
    }
    if (res.status !== 206) {
      throw new ApiError(
        res.status,
        `Expected partial content response (${res.status})`,
        requestIDFromResponse(res, requestID),
      );
    }

    try {
      return new Uint8Array(await readBody(() => res.arrayBuffer(), requestID));
    } catch (err) {
      if (!(err instanceof ApiError)) throw err;
      lastError = err;
      if (attempt < MAX_TRANSIENT_ATTEMPTS) await delay(retryDelayMs(attempt));
    }
  }
  throw lastError ?? new ApiError(0, "Network error — please check your connection");
}

async function apiErrorFromResponse(res: Response, fallbackRequestID: string): Promise<ApiError> {
  const body = await res.json().catch(() => null);
  const message = body?.error ?? `Request failed (${res.status})`;
  return new ApiError(
    res.status,
    message,
    requestIDFromResponse(res, fallbackRequestID),
    res.headers.get("Retry-After") ?? undefined,
    body?.details,
  );
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
  /** When a recipient first opened a reusable secret, if one has. */
  opened_at?: string;
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

export type SecretOutcome = "opened" | "expired" | "deleted";

/**
 * What became of a secret that is gone: the details of the 410 the metadata
 * endpoint answers with for as long as the server remembers.
 */
export interface SecretGone {
  readonly outcome: SecretOutcome;
  readonly burn_after_read: boolean;
  /** When it happened; for an expired secret, its expiry. */
  readonly ended_at: string;
  /** When a recipient first opened it, if anyone did. */
  readonly first_opened_at?: string;
  /** A one-time secret the owner opened themselves, so nobody else got it. */
  readonly opened_by_owner: boolean;
}

const OUTCOMES: readonly string[] = ["opened", "expired", "deleted"];

/** The story behind a 410 from the metadata endpoint, or null for any other error. */
export function secretGoneFromError(err: unknown): SecretGone | null {
  if (!(err instanceof ApiError) || err.status !== 410 || !err.details) return null;
  const { outcome, ended_at, first_opened_at, burn_after_read, opened_by_owner } = err.details;
  if (typeof outcome !== "string" || !OUTCOMES.includes(outcome) || typeof ended_at !== "string") {
    return null;
  }
  return {
    outcome: outcome as SecretOutcome,
    burn_after_read: Boolean(burn_after_read),
    ended_at,
    first_opened_at: typeof first_opened_at === "string" ? first_opened_at : undefined,
    opened_by_owner: Boolean(opened_by_owner),
  };
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

export interface StartUploadSessionResponse {
  readonly session_id: string;
  readonly upload_token: string;
  readonly public_id: string;
  readonly part_size: number;
  readonly blob_size: number;
  readonly expires_at: string;
  readonly upload_expires_at: string;
  readonly state: "pending" | "completed" | "aborted";
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
  signal?: AbortSignal,
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
    signal,
  });
}

// --- Build version ---

/** The commit the running server was built from, or "dev" for local builds. */
export async function getVersion(): Promise<string> {
  const { version } = await request<{ version: string }>("/api/v1/version", { method: "GET" });
  return version;
}

// --- Short-code transfers ---
//
// A transfer runs in three legs: the sender creates it with its PAKE share
// (the offer), the receiver claims it and posts its answer, and the sender
// posts the delivery. Each leg is written once with POST; the other side
// waits for it with a GET long-poll that answers 204 when its window passed.

export type TransferCloseReasonName = "cancelled" | "mismatch";

export interface OpenTransferResponse {
  readonly nameplate: number;
  readonly sender_token: string;
  readonly expires_at: string;
}

export interface ClaimTransferResponse {
  readonly transfer_id: string;
  readonly receiver_token: string;
  /** The sender's PAKE share, unpadded base64url. */
  readonly offer: string;
  readonly expires_at: string;
}

/** The receiver's PAKE share and confirmation tag, unpadded base64url. */
export interface TransferAnswerBody {
  readonly share: string;
  readonly confirmation: string;
}

/**
 * Transfer requests skip the HTTP cache. Both sides use the same leg URLs,
 * and when both run in one browser, its cache makes a request wait for the
 * other tab's pending request on that URL: up to 20 s per step.
 */
function transferRequest<T>(url: string, init: RequestInit): Promise<T> {
  return request(url, { ...init, cache: "no-store" });
}

function bearer(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` };
}

/** Creates a transfer under the id the sender picked, with its offer. */
export function openTransfer(transferID: string, offer: string): Promise<OpenTransferResponse> {
  return transferRequest("/api/v1/transfers", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ transfer_id: transferID, offer }),
  });
}

export function claimTransfer(nameplate: number): Promise<ClaimTransferResponse> {
  return transferRequest("/api/v1/transfers/claim", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ nameplate }),
  });
}

export function postTransferAnswer(
  transferID: string,
  token: string,
  answer: TransferAnswerBody,
  signal?: AbortSignal,
): Promise<void> {
  return transferRequest(`/api/v1/transfers/${transferID}/answer`, {
    method: "POST",
    headers: { ...bearer(token), "Content-Type": "application/json" },
    body: JSON.stringify(answer),
    signal,
  });
}

/** One long-poll for the answer, or null when the window passed without it. */
export async function awaitTransferAnswer(
  transferID: string,
  token: string,
  signal?: AbortSignal,
): Promise<TransferAnswerBody | null> {
  const body = await transferRequest<TransferAnswerBody | undefined>(
    `/api/v1/transfers/${transferID}/answer`,
    { method: "GET", headers: bearer(token), signal },
  );
  return body ?? null;
}

export function postTransferDelivery(
  transferID: string,
  token: string,
  sealed: string,
  signal?: AbortSignal,
): Promise<void> {
  return transferRequest(`/api/v1/transfers/${transferID}/delivery`, {
    method: "POST",
    headers: { ...bearer(token), "Content-Type": "application/json" },
    body: JSON.stringify({ sealed }),
    signal,
  });
}

/** One long-poll for the sealed link, or null when the window passed without it. */
export async function awaitTransferDelivery(
  transferID: string,
  token: string,
  signal?: AbortSignal,
): Promise<string | null> {
  const body = await transferRequest<{ sealed: string } | undefined>(
    `/api/v1/transfers/${transferID}/delivery`,
    { method: "GET", headers: bearer(token), signal },
  );
  return body?.sealed ?? null;
}

/** Ends a transfer early. keepalive lets the request finish while the page unloads. */
export function closeTransfer(
  transferID: string,
  token: string,
  reason: TransferCloseReasonName,
  keepalive = false,
): Promise<void> {
  return transferRequest(`/api/v1/transfers/${transferID}?reason=${reason}`, {
    method: "DELETE",
    headers: bearer(token),
    keepalive,
  });
}
