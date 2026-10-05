import {
  CodeMismatchError,
  createOffer,
  deriveTransferKeys,
  openLink,
  type ReceiverRelay,
  receiveLink,
  SEALED_PAYLOAD_BYTES,
  type SenderRelay,
  sealLink,
  sendLink,
  type TransferAnswer,
  type TransferCloseReason,
  TransferEndedError,
  type TransferParty,
} from "../transfer";

const LINK = "https://secretli.example/s#AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";
const SID = crypto.getRandomValues(new Uint8Array(32));
const ORIGIN = "https://secretli.example";

/**
 * Both sides' views of one in-memory transfer, with the server's rules:
 * legs written once, delivering ends the transfer, a close the other side
 * sees, and waits that block until their leg exists.
 */
function relayPair() {
  let answer: TransferAnswer | null = null;
  let delivery: Uint8Array | null = null;
  let closed: string | null = null;
  let wake: Array<() => void> = [];
  const notify = () => {
    const waiting = wake;
    wake = [];
    for (const resolve of waiting) resolve();
  };
  // A written leg is handed out even after the transfer closed.
  async function waitFor<T>(read: () => T | null): Promise<T> {
    for (;;) {
      const value = read();
      if (value) return value;
      if (closed) throw new TransferEndedError(closed);
      await new Promise<void>((resolve) => wake.push(resolve));
    }
  }
  const close = async (reason: TransferCloseReason) => {
    closed ??= reason;
    notify();
  };

  const sender: SenderRelay = {
    awaitAnswer: () => waitFor(() => answer),
    async deliver(sealed) {
      if (closed) throw new TransferEndedError(closed);
      if (delivery) throw new Error("delivered twice");
      delivery = sealed;
      closed = "done";
      notify();
    },
    close,
  };
  const receiver: ReceiverRelay = {
    async answer(value) {
      if (closed) throw new TransferEndedError(closed);
      if (answer) throw new Error("answered twice");
      answer = value;
      notify();
    },
    awaitDelivery: () => waitFor(() => delivery),
    close,
  };

  return { sender, receiver, closedWith: () => closed, delivered: () => delivery !== null };
}

function party(words: [string, string], overrides: Partial<TransferParty> = {}): TransferParty {
  return { words, sid: SID, origin: ORIGIN, ...overrides };
}

/** Runs both roles: the sender with its words, the receiver with its own. */
function handOver(
  relay: ReturnType<typeof relayPair>,
  sender: TransferParty,
  receiver: TransferParty,
) {
  const offer = createOffer(sender);
  return Promise.allSettled([
    sendLink(relay.sender, sender, offer, LINK),
    receiveLink(relay.receiver, receiver, offer.share),
  ]);
}

describe("short-code transfer", () => {
  it("hands the link to a receiver who typed the same words", async () => {
    const relay = relayPair();

    const [sent, received] = await handOver(
      relay,
      party(["acid", "rocket"]),
      party(["acid", "rocket"]),
    );

    expect(sent.status).toBe("fulfilled");
    expect(received.status === "fulfilled" && received.value).toBe(LINK);
    expect(relay.closedWith()).toBe("done");
  });

  it("never delivers when the words differ, and both sides learn it", async () => {
    const relay = relayPair();

    const [sent, received] = await handOver(
      relay,
      party(["acid", "rocket"]),
      party(["acid", "robe"]),
    );

    expect(sent.status === "rejected" && sent.reason).toBeInstanceOf(CodeMismatchError);
    expect(received.status === "rejected" && received.reason).toBeInstanceOf(CodeMismatchError);
    expect(relay.closedWith()).toBe("mismatch");
    expect(relay.delivered()).toBe(false);
  });

  it("is bound to the site: a run for another origin does not match", async () => {
    const relay = relayPair();

    const [sent] = await handOver(
      relay,
      party(["acid", "rocket"]),
      party(["acid", "rocket"], { origin: "https://evil.example" }),
    );

    expect(sent.status === "rejected" && sent.reason).toBeInstanceOf(CodeMismatchError);
  });

  it("is bound to the transfer: another session id does not match", async () => {
    const relay = relayPair();

    const [sent] = await handOver(
      relay,
      party(["acid", "rocket"]),
      party(["acid", "rocket"], { sid: crypto.getRandomValues(new Uint8Array(32)) }),
    );

    expect(sent.status === "rejected" && sent.reason).toBeInstanceOf(CodeMismatchError);
  });

  it("refuses an offer that is no valid share, and ends the transfer", async () => {
    const relay = relayPair();

    const receiving = receiveLink(
      relay.receiver,
      party(["acid", "rocket"]),
      new Uint8Array(32).fill(255),
    );

    await expect(receiving).rejects.toBeInstanceOf(CodeMismatchError);
    expect(relay.closedWith()).toBe("mismatch");
  });

  it("reports a transfer the sender cancelled", async () => {
    const relay = relayPair();
    const offer = createOffer(party(["acid", "rocket"]));
    const receiving = receiveLink(relay.receiver, party(["acid", "rocket"]), offer.share);

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
