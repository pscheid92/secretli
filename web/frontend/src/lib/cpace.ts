/**
 * CPace over ristretto255 with SHA-512 (CPACE-RISTR255-SHA512), as specified
 * in draft-irtf-cfrg-cpace-21, initiator-responder setting. Function names
 * follow the draft; the test vectors of its appendix B.3 pin every step.
 */
import { ristretto255, ristretto255_hasher } from "@noble/curves/ed25519.js";
import { sha512 } from "@noble/hashes/sha2.js";

type Point = typeof ristretto255.Point.BASE;

const encoder = new TextEncoder();
const DSI = encoder.encode("CPaceRistretto255");
const DSI_ISK = encoder.encode("CPaceRistretto255_ISK");
/** SHA-512's input block size, which the generator string pads to. */
const SHA512_BLOCK_BYTES = 128;
/** Ristretto255 scalars have 252 significant bits. */
const SCALAR_BYTES = 32;

/** A received share was invalid or led to the neutral element: abort. */
export class CPaceError extends Error {
  constructor() {
    super("invalid CPace share");
    this.name = "CPaceError";
  }
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

/** Prepends the LEB128-encoded length. */
export function prependLen(data: Uint8Array): Uint8Array {
  const length: number[] = [];
  let n = data.length;
  do {
    length.push(n < 128 ? n : (n & 0x7f) | 0x80);
    n >>>= 7;
  } while (n > 0);
  return concat(Uint8Array.from(length), data);
}

export function lvCat(...parts: Uint8Array[]): Uint8Array {
  return concat(...parts.map(prependLen));
}

export function generatorString(prs: Uint8Array, ci: Uint8Array, sid: Uint8Array): Uint8Array {
  const zpad = Math.max(
    0,
    SHA512_BLOCK_BYTES - 1 - prependLen(prs).length - prependLen(DSI).length,
  );
  return lvCat(DSI, prs, new Uint8Array(zpad), ci, sid);
}

/** The password-dependent generator: element derivation over the hashed generator string. */
export function calculateGenerator(prs: Uint8Array, ci: Uint8Array, sid: Uint8Array): Point {
  const derive = ristretto255_hasher.deriveToCurve;
  if (!derive) throw new Error("ristretto255 element derivation unavailable");
  return derive(sha512(generatorString(prs, ci, sid)));
}

export function scalarFromBytesLE(bytes: Uint8Array): bigint {
  let n = 0n;
  for (let i = bytes.length - 1; i >= 0; i--) {
    n = (n << 8n) | BigInt(bytes[i]);
  }
  return n;
}

/** The draft's recommended sampling: 32 random bytes, bits above 252 cleared. */
export function sampleScalar(): bigint {
  for (;;) {
    const bytes = crypto.getRandomValues(new Uint8Array(SCALAR_BYTES));
    bytes[SCALAR_BYTES - 1] &= 0x0f;
    const scalar = scalarFromBytesLE(bytes);
    if (scalar !== 0n) return scalar;
  }
}

/** This side's share Y = y·g, as the 32 bytes that go over the wire. */
export function cpaceShare(generator: Point, scalar: bigint): Uint8Array {
  return generator.multiply(scalar).toBytes();
}

/**
 * scalar_mult_vfy: y·X for a received encoding X. Throws CPaceError when X
 * does not decode or the result is the neutral element.
 */
export function scalarMultVfy(scalar: bigint, encoded: Uint8Array): Uint8Array {
  let point: Point;
  try {
    point = ristretto255.Point.fromBytes(encoded);
  } catch {
    throw new CPaceError();
  }
  const product = point.multiply(scalar);
  if (product.is0()) throw new CPaceError();
  return product.toBytes();
}

export function transcriptIr(
  ya: Uint8Array,
  ada: Uint8Array,
  yb: Uint8Array,
  adb: Uint8Array,
): Uint8Array {
  return concat(lvCat(ya, ada), lvCat(yb, adb));
}

/** The intermediate session key both sides share when they used the same password. */
export function cpaceIsk(
  sid: Uint8Array,
  k: Uint8Array,
  ya: Uint8Array,
  ada: Uint8Array,
  yb: Uint8Array,
  adb: Uint8Array,
): Uint8Array {
  return sha512(concat(lvCat(DSI_ISK, sid, k), transcriptIr(ya, ada, yb, adb)));
}
