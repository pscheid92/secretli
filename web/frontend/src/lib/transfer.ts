/**
 * Short-code transfer of a share link in three legs through the relay: the
 * sender's CPace share (the offer), the receiver's share with a tag proving
 * it derived the same key (the answer), and only then the link, sealed for
 * the receiver (the delivery). The relay sees public shares, a tag and
 * fixed-size ciphertext; the words never leave the two devices.
 */
import { xchacha20poly1305 } from "@noble/ciphers/chacha.js";
import { equalBytes } from "@noble/ciphers/utils.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { hmac } from "@noble/hashes/hmac.js";
import { sha512 } from "@noble/hashes/sha2.js";
import {
  CPaceError,
  calculateGenerator,
  cpaceIsk,
  cpaceShare,
  sampleScalar,
  scalarMultVfy,
} from "./cpace";
import { transferPassword } from "./transferWords";

/** Why a side ends a transfer early; delivering ends it as done. */
export type TransferCloseReason = "cancelled" | "mismatch";

/** The receiver's CPace share and its proof of the same key. */
export interface TransferAnswer {
  readonly share: Uint8Array;
  readonly confirmation: Uint8Array;
}

/** The relay as the sender uses it, once the transfer holds its offer. */
export interface SenderRelay {
  /** Resolves with the receiver's answer once it exists. */
  awaitAnswer(): Promise<TransferAnswer>;
  /** Stores the sealed link, which ends the transfer as done. */
  deliver(sealed: Uint8Array): Promise<void>;
  close(reason: TransferCloseReason): Promise<void>;
}

/** The relay as the receiver uses it, after claiming the transfer. */
export interface ReceiverRelay {
  answer(answer: TransferAnswer): Promise<void>;
  /** Resolves with the sealed link once the sender delivered it. */
  awaitDelivery(): Promise<Uint8Array>;
  close(reason: TransferCloseReason): Promise<void>;
}

/** What both sides must agree on; only the words are secret. */
export interface TransferParty {
  readonly words: readonly [string, string];
  /** The transfer id, which the sender picks; also the CPace session id. */
  readonly sid: Uint8Array;
  readonly origin: string;
}

/** The sender's first leg, kept until the answer arrives. */
export interface TransferOffer {
  readonly share: Uint8Array;
  readonly scalar: bigint;
}

/** The other side ended the transfer, or it expired. */
export class TransferEndedError extends Error {
  readonly reason: string;

  constructor(reason: string) {
    super(`transfer ended: ${reason}`);
    this.name = "TransferEndedError";
    this.reason = reason;
  }
}

/** The two sides used different codes. */
export class CodeMismatchError extends Error {
  constructor() {
    super("the code did not match");
    this.name = "CodeMismatchError";
  }
}

const encoder = new TextEncoder();
const AD_SENDER = encoder.encode("sender");
const AD_RECEIVER = encoder.encode("receiver");
const CONFIRM_INFO = encoder.encode("secretli transfer v1 confirm");
const PAYLOAD_INFO = encoder.encode("secretli transfer v1 payload");
const PAYLOAD_AAD_SUFFIX = encoder.encode("payload");

/** The link is padded to this size, so its length does not reach the relay. */
export const PAYLOAD_PLAINTEXT_BYTES = 512;
const LENGTH_PREFIX_BYTES = 2;
const MAX_LINK_BYTES = PAYLOAD_PLAINTEXT_BYTES - LENGTH_PREFIX_BYTES;
const NONCE_BYTES = 24;
const TAG_BYTES = 16;
export const SEALED_PAYLOAD_BYTES = NONCE_BYTES + PAYLOAD_PLAINTEXT_BYTES + TAG_BYTES;
const CONFIRMATION_TAG_BYTES = 32;

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

/** CPace's channel identifier: binds a run to this protocol version and site. */
export function channelIdentifier(origin: string): Uint8Array {
  return encoder.encode(`secretli-transfer-v1 ${origin}`);
}

export interface TransferKeys {
  readonly confirm: Uint8Array;
  readonly payload: Uint8Array;
}

export function deriveTransferKeys(isk: Uint8Array, sid: Uint8Array): TransferKeys {
  return {
    confirm: hkdf(sha512, isk, sid, CONFIRM_INFO, 32),
    payload: hkdf(sha512, isk, sid, PAYLOAD_INFO, 32),
  };
}

/** The receiver's proof that it derived the same key. */
export function confirmationTag(key: Uint8Array, ya: Uint8Array, yb: Uint8Array): Uint8Array {
  return hmac(sha512, key, concat(AD_RECEIVER, ya, yb)).slice(0, CONFIRMATION_TAG_BYTES);
}

function payloadAad(sid: Uint8Array): Uint8Array {
  return concat(sid, PAYLOAD_AAD_SUFFIX);
}

/** Seals the link as nonce || ciphertext, always the same size. */
export function sealLink(key: Uint8Array, sid: Uint8Array, link: string): Uint8Array {
  const bytes = encoder.encode(link);
  if (bytes.length > MAX_LINK_BYTES) throw new Error("link is too long to transfer");
  const plaintext = new Uint8Array(PAYLOAD_PLAINTEXT_BYTES);
  plaintext[0] = bytes.length >> 8;
  plaintext[1] = bytes.length & 0xff;
  plaintext.set(bytes, LENGTH_PREFIX_BYTES);
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_BYTES));
  return concat(nonce, xchacha20poly1305(key, nonce, payloadAad(sid)).encrypt(plaintext));
}

/** Opens a sealed link; any failure means the keys differed. */
export function openLink(key: Uint8Array, sid: Uint8Array, sealed: Uint8Array): string {
  if (sealed.length !== SEALED_PAYLOAD_BYTES) throw new CodeMismatchError();
  let plaintext: Uint8Array;
  try {
    const nonce = sealed.slice(0, NONCE_BYTES);
    plaintext = xchacha20poly1305(key, nonce, payloadAad(sid)).decrypt(sealed.slice(NONCE_BYTES));
  } catch {
    throw new CodeMismatchError();
  }
  const length = (plaintext[0] << 8) | plaintext[1];
  if (length > MAX_LINK_BYTES) throw new CodeMismatchError();
  return new TextDecoder().decode(
    plaintext.slice(LENGTH_PREFIX_BYTES, LENGTH_PREFIX_BYTES + length),
  );
}

/** Closes the relay without letting a failed close hide the real error. */
async function closeQuietly(
  relay: SenderRelay | ReceiverRelay,
  reason: TransferCloseReason,
): Promise<void> {
  try {
    await relay.close(reason);
  } catch {
    // The transfer may already be gone; the caller reports its own error.
  }
}

function generatorFor(party: TransferParty) {
  return calculateGenerator(
    transferPassword(party.words),
    channelIdentifier(party.origin),
    party.sid,
  );
}

/**
 * K from the other side's share, closing the transfer as a mismatch when the
 * share fails the draft's checks.
 */
async function sharedSecret(
  relay: SenderRelay | ReceiverRelay,
  scalar: bigint,
  peerShare: Uint8Array,
): Promise<Uint8Array> {
  try {
    return scalarMultVfy(scalar, peerShare);
  } catch (err) {
    if (!(err instanceof CPaceError)) throw err;
    await closeQuietly(relay, "mismatch");
    throw new CodeMismatchError();
  }
}

/** The sender's first leg: its CPace share, computed before the transfer exists. */
export function createOffer(party: TransferParty): TransferOffer {
  const scalar = sampleScalar();
  return { share: cpaceShare(generatorFor(party), scalar), scalar };
}

/**
 * Sender role: hands the link over only after the receiver's confirmation
 * tag proves it typed the same code.
 */
export async function sendLink(
  relay: SenderRelay,
  party: TransferParty,
  offer: TransferOffer,
  link: string,
): Promise<void> {
  const answer = await relay.awaitAnswer();
  const k = await sharedSecret(relay, offer.scalar, answer.share);
  const isk = cpaceIsk(party.sid, k, offer.share, AD_SENDER, answer.share, AD_RECEIVER);
  const keys = deriveTransferKeys(isk, party.sid);
  if (!equalBytes(answer.confirmation, confirmationTag(keys.confirm, offer.share, answer.share))) {
    await closeQuietly(relay, "mismatch");
    throw new CodeMismatchError();
  }
  await relay.deliver(sealLink(keys.payload, party.sid, link));
}

/** Receiver role: answers the offer, proving knowledge of the code, then opens the link. */
export async function receiveLink(
  relay: ReceiverRelay,
  party: TransferParty,
  offer: Uint8Array,
): Promise<string> {
  const scalar = sampleScalar();
  const share = cpaceShare(generatorFor(party), scalar);
  const k = await sharedSecret(relay, scalar, offer);
  const isk = cpaceIsk(party.sid, k, offer, AD_SENDER, share, AD_RECEIVER);
  const keys = deriveTransferKeys(isk, party.sid);
  await relay.answer({ share, confirmation: confirmationTag(keys.confirm, offer, share) });

  let sealed: Uint8Array;
  try {
    sealed = await relay.awaitDelivery();
  } catch (err) {
    if (err instanceof TransferEndedError && err.reason === "mismatch")
      throw new CodeMismatchError();
    throw err;
  }
  return openLink(keys.payload, party.sid, sealed);
}
