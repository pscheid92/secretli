import {
  CodeMismatchError,
  deriveTransferKeys,
  openLink,
  receiveLink,
  SEALED_PAYLOAD_BYTES,
  sealLink,
  sendLink,
  type TransferCloseReason,
  TransferEndedError,
  type TransferParty,
  type TransferPhase,
  type TransferRelay,
} from "../transfer";

const LINK = "https://secretli.example/s#AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";
const SID = crypto.getRandomValues(new Uint8Array(32));
const ORIGIN = "https://secretli.example";

/**
 * Two relay views over one in-memory mailbox, with the server's rules:
 * write-once messages, a close the other side sees, and reads that wait.
 */
function relayPair() {
  const messages = new Map<string, Uint8Array>();
  let closed: TransferCloseReason | null = null;
  const writes: string[] = [];
  let wake: Array<() => void> = [];
  const notify = () => {
    const waiting = wake;
    wake = [];
    for (const resolve of waiting) resolve();
  };

  const side = (own: string, other: string): TransferRelay => ({
    async put(phase: TransferPhase, data: Uint8Array) {
      if (closed) throw new TransferEndedError(closed);
      const key = `${own}:${phase}`;
      if (messages.has(key)) throw new Error(`${key} written twice`);
      messages.set(key, data);
      writes.push(key);
      notify();
    },
    async get(phase: TransferPhase) {
      for (;;) {
        const message = messages.get(`${other}:${phase}`);
        if (message) return message;
        if (closed) throw new TransferEndedError(closed);
        await new Promise<void>((resolve) => wake.push(resolve));
      }
    },
    async close(reason: TransferCloseReason) {
      closed ??= reason;
      notify();
    },
  });

  return {
    sender: side("sender", "receiver"),
    receiver: side("receiver", "sender"),
    closedWith: () => closed,
    writes,
  };
}

function party(words: [string, string], overrides: Partial<TransferParty> = {}): TransferParty {
  return { words, sid: SID, origin: ORIGIN, ...overrides };
}

describe("short-code transfer", () => {
  it("hands the link to a receiver who typed the same words", async () => {
    const relay = relayPair();

    const [, received] = await Promise.all([
      sendLink(relay.sender, party(["acid", "rocket"]), LINK),
      receiveLink(relay.receiver, party(["acid", "rocket"])),
    ]);

    expect(received).toBe(LINK);
    expect(relay.closedWith()).toBe("done");
  });

  it("never sends the payload when the words differ, and both sides learn it", async () => {
    const relay = relayPair();

    const [sent, received] = await Promise.allSettled([
      sendLink(relay.sender, party(["acid", "rocket"]), LINK),
      receiveLink(relay.receiver, party(["acid", "robe"])),
    ]);

    expect(sent.status === "rejected" && sent.reason).toBeInstanceOf(CodeMismatchError);
    expect(received.status === "rejected" && received.reason).toBeInstanceOf(CodeMismatchError);
    expect(relay.closedWith()).toBe("mismatch");
    expect(relay.writes).not.toContain("sender:payload");
  });

  it("is bound to the site: a run for another origin does not match", async () => {
    const relay = relayPair();

    const [sent] = await Promise.allSettled([
      sendLink(relay.sender, party(["acid", "rocket"]), LINK),
      receiveLink(relay.receiver, party(["acid", "rocket"], { origin: "https://evil.example" })),
    ]);

    expect(sent.status === "rejected" && sent.reason).toBeInstanceOf(CodeMismatchError);
  });

  it("is bound to the transfer: another session id does not match", async () => {
    const relay = relayPair();
    const otherSid = crypto.getRandomValues(new Uint8Array(32));

    const [sent] = await Promise.allSettled([
      sendLink(relay.sender, party(["acid", "rocket"]), LINK),
      receiveLink(relay.receiver, party(["acid", "rocket"], { sid: otherSid })),
    ]);

    expect(sent.status === "rejected" && sent.reason).toBeInstanceOf(CodeMismatchError);
  });

  it("reports a transfer the sender cancelled", async () => {
    const relay = relayPair();
    const receiving = receiveLink(relay.receiver, party(["acid", "rocket"]));

    await relay.sender.close("cancelled");

    await expect(receiving).rejects.toEqual(new TransferEndedError("cancelled"));
  });
});

describe("sealed link payload", () => {
  const keys = deriveTransferKeys(crypto.getRandomValues(new Uint8Array(64)), SID);

  it("round-trips and always has the same size", () => {
    const short = sealLink(keys.payload, SID, "https://a.example/s#x");
    const long = sealLink(keys.payload, SID, LINK);

    expect(short.length).toBe(SEALED_PAYLOAD_BYTES);
    expect(long.length).toBe(SEALED_PAYLOAD_BYTES);
    expect(openLink(keys.payload, SID, long)).toBe(LINK);
  });

  it("rejects a tampered payload, another session and a wrong size", () => {
    const sealed = sealLink(keys.payload, SID, LINK);
    const tampered = sealed.slice();
    tampered[40] ^= 1;

    expect(() => openLink(keys.payload, SID, tampered)).toThrow(CodeMismatchError);
    expect(() => openLink(keys.payload, new Uint8Array(32), sealed)).toThrow(CodeMismatchError);
    expect(() => openLink(keys.payload, SID, sealed.slice(1))).toThrow(CodeMismatchError);
  });

  it("refuses a link longer than the padded payload", () => {
    expect(() => sealLink(keys.payload, SID, "x".repeat(511))).toThrow(/too long/);
  });

  it("derives separate confirmation and payload keys", () => {
    expect(keys.confirm).not.toEqual(keys.payload);
  });
});
