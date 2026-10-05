/**
 * Runs short-code transfers over the HTTP relay. Pages load this module only
 * when someone sends or receives with a code, which keeps the CPace code and
 * the word list out of the initial bundle.
 */
import {
  ApiError,
  awaitTransferAnswer,
  awaitTransferDelivery,
  claimTransfer,
  closeTransfer,
  isTransientStatus,
  MAX_TRANSIENT_ATTEMPTS,
  openTransfer,
  postTransferAnswer,
  postTransferDelivery,
  retryDelayMs,
} from "./api";
import { base64UrlDecode, base64UrlEncode } from "./base64";
import {
  CodeMismatchError,
  createOffer,
  type ReceiverRelay,
  receiveLink,
  type SenderRelay,
  sendLink,
  TransferEndedError,
  type TransferParty,
} from "./transfer";
import { formatCode, parseCode, randomWords } from "./transferWords";

/** The typed code could not be read; nothing was sent to the server. */
export class CodeFormatError extends Error {
  readonly word?: string;

  constructor(word?: string) {
    super(word ? `unknown code word: ${word}` : "malformed code");
    this.name = "CodeFormatError";
    this.word = word;
  }
}

/** The session id is the transfer id, so it is as random as a share secret. */
const SESSION_ID_BYTES = 32;

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      reject(signal.reason);
    });
  });
}

function endedError(err: unknown): TransferEndedError | null {
  if (!(err instanceof ApiError)) return null;
  if (err.status === 410) return new TransferEndedError(String(err.details?.reason ?? "expired"));
  // Cleanup deletes a transfer shortly after it ends.
  if (err.status === 404) return new TransferEndedError("expired");
  return null;
}

/**
 * Runs a relay request until it yields a value: a long-poll answers null
 * when its window passed, and a transient failure is retried. Writes are
 * idempotent on the relay, so retrying them is safe too.
 */
async function untilDone<T>(attempt: () => Promise<T | null>, signal?: AbortSignal): Promise<T> {
  let failures = 0;
  for (;;) {
    signal?.throwIfAborted();
    try {
      const value = await attempt();
      failures = 0;
      if (value !== null) return value;
    } catch (err) {
      const ended = endedError(err);
      if (ended) throw ended;
      const transient = err instanceof ApiError && isTransientStatus(err.status);
      if (!transient || ++failures >= MAX_TRANSIENT_ATTEMPTS) throw err;
      await delay(retryDelayMs(failures, err.retryAfter), signal);
    }
  }
}

/** Writes once, retrying transient failures. */
async function write(post: () => Promise<void>, signal?: AbortSignal): Promise<void> {
  await untilDone(async () => {
    await post();
    return true;
  }, signal);
}

/** The sender's view of the relay, over HTTP long-polling. */
export function httpSenderRelay(
  transferID: string,
  token: string,
  signal?: AbortSignal,
): SenderRelay {
  return {
    async awaitAnswer() {
      const answer = await untilDone(() => awaitTransferAnswer(transferID, token, signal), signal);
      return {
        share: base64UrlDecode(answer.share),
        confirmation: base64UrlDecode(answer.confirmation),
      };
    },
    deliver: (sealed) =>
      write(() => postTransferDelivery(transferID, token, base64UrlEncode(sealed), signal), signal),
    close: (reason) => closeTransfer(transferID, token, reason),
  };
}

/** The receiver's view of the relay, over HTTP long-polling. */
export function httpReceiverRelay(
  transferID: string,
  token: string,
  signal?: AbortSignal,
): ReceiverRelay {
  return {
    answer: (answer) =>
      write(
        () =>
          postTransferAnswer(
            transferID,
            token,
            {
              share: base64UrlEncode(answer.share),
              confirmation: base64UrlEncode(answer.confirmation),
            },
            signal,
          ),
        signal,
      ),
    async awaitDelivery() {
      const sealed = await untilDone(
        () => awaitTransferDelivery(transferID, token, signal),
        signal,
      );
      return base64UrlDecode(sealed);
    },
    close: (reason) => closeTransfer(transferID, token, reason),
  };
}

export interface SendingTransfer {
  /** The code to show, like "7-acid-rocket". */
  readonly code: string;
  readonly expiresAt: number;
  /** Settles when the link was handed over, or with the reason it was not. */
  readonly done: Promise<void>;
  /** Stops waiting and ends the transfer, also while the page unloads. */
  cancel(): void;
}

/**
 * Whether the transfer is over on the relay already: delivered (which ends
 * it), closed as a mismatch by the protocol, or ended by the other side.
 * Closing it again would only send a misleading "cancelled".
 */
function transferEnded(err?: unknown): boolean {
  return err === undefined || err instanceof CodeMismatchError || err instanceof TransferEndedError;
}

export async function startSending(link: string): Promise<SendingTransfer> {
  const sid = crypto.getRandomValues(new Uint8Array(SESSION_ID_BYTES));
  const transferID = base64UrlEncode(sid);
  const party: TransferParty = { words: randomWords(), sid, origin: window.location.origin };
  // The offer depends on the session id, so the sender picks the id and
  // creates the transfer with its offer in one request.
  const offer = createOffer(party);
  const opened = await openTransfer(transferID, base64UrlEncode(offer.share));

  const controller = new AbortController();
  const relay = httpSenderRelay(transferID, opened.sender_token, controller.signal);
  let ended = false;
  const done = sendLink(relay, party, offer, link);
  done.then(
    () => {
      ended = true;
    },
    (err) => {
      // A cancelled wait rejects too; whoever awaits done handles the rest.
      ended = transferEnded(err);
    },
  );

  return {
    code: formatCode(opened.nameplate, party.words),
    expiresAt: Date.parse(opened.expires_at),
    done,
    cancel() {
      if (ended || controller.signal.aborted) return;
      controller.abort();
      closeTransfer(transferID, opened.sender_token, "cancelled", true).catch(() => {});
    },
  };
}

/** Claims the transfer behind a typed code and returns the link it carries. */
export async function receiveWithCode(input: string, signal?: AbortSignal): Promise<string> {
  const parsed = parseCode(input);
  if (!parsed.ok)
    throw new CodeFormatError(parsed.error === "unknown-word" ? parsed.word : undefined);

  const claimed = await claimTransfer(parsed.nameplate);
  const cancel = () => {
    closeTransfer(claimed.transfer_id, claimed.receiver_token, "cancelled", true).catch(() => {});
  };
  // Tell the sender right away if the receiver gives up while waiting.
  signal?.addEventListener("abort", cancel, { once: true });
  const relay = httpReceiverRelay(claimed.transfer_id, claimed.receiver_token, signal);
  try {
    return await receiveLink(
      relay,
      {
        words: parsed.words,
        sid: base64UrlDecode(claimed.transfer_id),
        origin: window.location.origin,
      },
      base64UrlDecode(claimed.offer),
    );
  } catch (err) {
    // Any other failure leaves the transfer claimed; release the sender now
    // instead of letting it wait for the expiry.
    if (!transferEnded(err) && !signal?.aborted) cancel();
    throw err;
  } finally {
    signal?.removeEventListener("abort", cancel);
  }
}

function rateLimited(err: unknown): boolean {
  return err instanceof ApiError && err.status === 429;
}

/** What the sender sees when a transfer fails. */
export function describeSendError(err: unknown): string {
  if (err instanceof CodeMismatchError) return "The code didn't match. Start again for a new code.";
  if (err instanceof TransferEndedError && err.reason === "expired") {
    return "Nobody entered the code in time. Start again for a new code.";
  }
  if (err instanceof TransferEndedError && err.reason === "cancelled") {
    return "The other device stopped the transfer. Start again for a new code.";
  }
  if (err instanceof ApiError && err.status === 503) {
    return "Too many transfers right now. Try again in a minute.";
  }
  if (rateLimited(err)) return "Too many attempts. Wait a minute and try again.";
  return "The transfer failed. Start again for a new code.";
}

/** What the receiver sees when a code doesn't work. */
export function describeReceiveError(err: unknown): string {
  if (err instanceof CodeFormatError) {
    return err.word
      ? `"${err.word}" isn't a code word. Check the spelling.`
      : "Codes look like 7-acid-rocket: a number and two words.";
  }
  if (err instanceof ApiError && err.status === 404) {
    return "No transfer with that number. Check the code, or ask for a new one.";
  }
  if (err instanceof ApiError && err.status === 409)
    return "That code was already used. Ask for a new one.";
  if (err instanceof CodeMismatchError) return "The code didn't match. Ask for a new code.";
  if (err instanceof TransferEndedError) {
    return err.reason === "cancelled"
      ? "The sender stopped the transfer."
      : "The code expired. Ask for a new one.";
  }
  if (rateLimited(err)) return "Too many attempts. Wait a minute and try again.";
  return "The transfer failed. Ask for a new code.";
}
