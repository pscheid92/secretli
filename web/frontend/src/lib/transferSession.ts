/**
 * Runs short-code transfers over the HTTP relay. Pages load this module only
 * when someone sends or receives with a code, which keeps the CPace code and
 * the word list out of the initial bundle.
 */
import {
  ApiError,
  claimTransfer,
  closeTransfer,
  isTransientStatus,
  MAX_TRANSIENT_ATTEMPTS,
  openTransfer,
  pollTransferMessage,
  putTransferMessage,
  retryDelayMs,
} from "./api";
import { base64UrlDecode, base64UrlEncode } from "./base64";
import {
  CodeMismatchError,
  receiveLink,
  sendLink,
  TransferEndedError,
  type TransferRelay,
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

/** One side's view of the relay, over HTTP long-polling. */
export function httpTransferRelay(
  transferID: string,
  token: string,
  signal?: AbortSignal,
): TransferRelay {
  return {
    async put(phase, data) {
      try {
        await putTransferMessage(transferID, token, phase, base64UrlEncode(data), signal);
      } catch (err) {
        throw endedError(err) ?? err;
      }
    },
    async get(phase) {
      let failures = 0;
      for (;;) {
        signal?.throwIfAborted();
        try {
          const data = await pollTransferMessage(transferID, token, phase, signal);
          failures = 0;
          if (data !== null) return base64UrlDecode(data);
        } catch (err) {
          const ended = endedError(err);
          if (ended) throw ended;
          const transient = err instanceof ApiError && isTransientStatus(err.status);
          if (!transient || ++failures >= MAX_TRANSIENT_ATTEMPTS) throw err;
          await delay(retryDelayMs(failures, err.retryAfter), signal);
        }
      }
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

export async function startSending(link: string): Promise<SendingTransfer> {
  const opened = await openTransfer();
  const words = randomWords();
  const controller = new AbortController();
  const relay = httpTransferRelay(opened.transfer_id, opened.sender_token, controller.signal);
  const done = sendLink(
    relay,
    { words, sid: base64UrlDecode(opened.transfer_id), origin: window.location.origin },
    link,
  );
  // A cancelled wait rejects; whoever awaits done handles the other errors.
  done.catch(() => {});

  return {
    code: formatCode(opened.nameplate, words),
    expiresAt: Date.parse(opened.expires_at),
    done,
    cancel() {
      if (controller.signal.aborted) return;
      controller.abort();
      closeTransfer(opened.transfer_id, opened.sender_token, "cancelled", true).catch(() => {});
    },
  };
}

/** Claims the transfer behind a typed code and returns the link it carries. */
export async function receiveWithCode(input: string, signal?: AbortSignal): Promise<string> {
  const parsed = parseCode(input);
  if (!parsed.ok)
    throw new CodeFormatError(parsed.error === "unknown-word" ? parsed.word : undefined);

  const claimed = await claimTransfer(parsed.nameplate);
  // Tell the sender right away if the receiver gives up.
  signal?.addEventListener(
    "abort",
    () => {
      closeTransfer(claimed.transfer_id, claimed.receiver_token, "cancelled", true).catch(() => {});
    },
    { once: true },
  );
  const relay = httpTransferRelay(claimed.transfer_id, claimed.receiver_token, signal);
  return receiveLink(relay, {
    words: parsed.words,
    sid: base64UrlDecode(claimed.transfer_id),
    origin: window.location.origin,
  });
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
