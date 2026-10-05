import { ApiError } from "../api";
import { CodeMismatchError, TransferEndedError } from "../transfer";
import {
  CodeFormatError,
  describeReceiveError,
  describeSendError,
  httpTransferRelay,
  receiveWithCode,
} from "../transferSession";

const TRANSFER_ID = "t".repeat(43);
const TOKEN = "k".repeat(43);

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status });
}

describe("httpTransferRelay", () => {
  afterEach(() => vi.restoreAllMocks());

  it("keeps long-polling through empty windows until the message arrives", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(json(200, { data: "AQID" }));

    const data = await httpTransferRelay(TRANSFER_ID, TOKEN).get("share");

    expect(data).toEqual(new Uint8Array([1, 2, 3]));
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock.mock.calls[0][0]).toBe(`/api/v1/transfers/${TRANSFER_ID}/messages/share`);
  });

  it("reports why the transfer ended", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      json(410, { error: "transfer has ended", details: { reason: "mismatch" } }),
    );

    await expect(httpTransferRelay(TRANSFER_ID, TOKEN).get("payload")).rejects.toEqual(
      new TransferEndedError("mismatch"),
    );
  });

  it("treats a transfer that is already deleted as expired", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(json(404, { error: "transfer not found" }));

    await expect(
      httpTransferRelay(TRANSFER_ID, TOKEN).put("share", new Uint8Array([1])),
    ).rejects.toEqual(new TransferEndedError("expired"));
  });

  it("retries a transient failure while polling", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(json(503, { error: "unavailable" }))
      .mockResolvedValueOnce(json(200, { data: "AQ" }));

    await expect(httpTransferRelay(TRANSFER_ID, TOKEN).get("share")).resolves.toEqual(
      new Uint8Array([1]),
    );
  });

  it("sends messages as unpadded base64url with the bearer token", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 204 }));

    await httpTransferRelay(TRANSFER_ID, TOKEN).put("confirm", new Uint8Array([251, 255]));

    const [, init] = fetchMock.mock.calls[0];
    expect(init?.method).toBe("PUT");
    expect(new Headers(init?.headers).get("Authorization")).toBe(`Bearer ${TOKEN}`);
    expect(JSON.parse(String(init?.body))).toEqual({ data: "-_8" });
  });
});

describe("receiveWithCode", () => {
  afterEach(() => vi.restoreAllMocks());

  it("rejects a malformed code without contacting the server", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch");

    await expect(receiveWithCode("7-acid")).rejects.toBeInstanceOf(CodeFormatError);
    await expect(receiveWithCode("7-acid-rokcet")).rejects.toEqual(new CodeFormatError("rokcet"));
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("error messages", () => {
  it.each([
    [new CodeMismatchError(), "The code didn't match. Start again for a new code."],
    [
      new TransferEndedError("expired"),
      "Nobody entered the code in time. Start again for a new code.",
    ],
    [
      new TransferEndedError("cancelled"),
      "The other device stopped the transfer. Start again for a new code.",
    ],
    [new ApiError(503, "busy"), "Too many transfers right now. Try again in a minute."],
    [new Error("boom"), "The transfer failed. Start again for a new code."],
  ])("tells the sender about %s", (err, message) => {
    expect(describeSendError(err)).toBe(message);
  });

  it.each([
    [new CodeFormatError(), "Codes look like 7-acid-rocket: a number and two words."],
    [new CodeFormatError("rokcet"), '"rokcet" isn\'t a code word. Check the spelling.'],
    [
      new ApiError(404, "missing"),
      "No transfer with that number. Check the code, or ask for a new one.",
    ],
    [new ApiError(409, "claimed"), "That code was already used. Ask for a new one."],
    [new CodeMismatchError(), "The code didn't match. Ask for a new code."],
    [new TransferEndedError("cancelled"), "The sender stopped the transfer."],
    [new TransferEndedError("expired"), "The code expired. Ask for a new one."],
    [new ApiError(429, "slow down"), "Too many attempts. Wait a minute and try again."],
  ])("tells the receiver about %s", (err, message) => {
    expect(describeReceiveError(err)).toBe(message);
  });
});
