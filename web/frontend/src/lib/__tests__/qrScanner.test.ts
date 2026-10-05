import jsQR from "jsqr";
import {
  cameraFailure,
  canScan,
  createDecoder,
  type QRDecoder,
  SCAN_INTERVAL_MS,
  scanFrames,
} from "../qrScanner";

vi.mock("jsqr", () => ({ default: vi.fn() }));

function fakeVideo(readyState: number = HTMLMediaElement.HAVE_ENOUGH_DATA): HTMLVideoElement {
  return { readyState, videoWidth: 1280, videoHeight: 720 } as HTMLVideoElement;
}

function decoderReturning(...results: (string[] | Error)[]): QRDecoder & { calls: number } {
  const decoder = {
    calls: 0,
    decode: async () => {
      const result = results[Math.min(decoder.calls, results.length - 1)];
      decoder.calls++;
      if (result instanceof Error) throw result;
      return result;
    },
  };
  return decoder;
}

describe("cameraFailure", () => {
  it.each([
    ["NotAllowedError", "blocked"],
    ["SecurityError", "blocked"],
    ["NotFoundError", "no-camera"],
    ["OverconstrainedError", "no-camera"],
    ["NotReadableError", "camera-failed"],
    ["AbortError", "camera-failed"],
  ])("maps %s to %s", (name, expected) => {
    expect(cameraFailure(new DOMException("camera", name))).toBe(expected);
  });

  it("treats anything else as a camera that failed to start", () => {
    expect(cameraFailure(new Error("boom"))).toBe("camera-failed");
  });
});

describe("canScan", () => {
  it("is false without camera access", () => {
    expect(canScan()).toBe(false);
  });
});

describe("scanFrames", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("decodes until a code is accepted, then stops", async () => {
    const decoder = decoderReturning([], ["not it"], ["the link"], ["after"]);
    const seen: string[][] = [];
    scanFrames(fakeVideo(), decoder, (texts) => {
      seen.push(texts);
      return texts[0] === "the link";
    });

    await vi.advanceTimersByTimeAsync(SCAN_INTERVAL_MS * 10);

    expect(seen).toEqual([["not it"], ["the link"]]);
    expect(decoder.calls).toBe(3);
  });

  it("waits until the video has a frame", async () => {
    const decoder = decoderReturning([]);
    scanFrames(fakeVideo(HTMLMediaElement.HAVE_NOTHING), decoder, () => true);

    await vi.advanceTimersByTimeAsync(SCAN_INTERVAL_MS * 4);

    expect(decoder.calls).toBe(0);
  });

  it("skips frames that fail to decode", async () => {
    const decoder = decoderReturning(new Error("bad frame"), ["the link"]);
    const onCodes = vi.fn(() => true);
    scanFrames(fakeVideo(), decoder, onCodes);

    await vi.advanceTimersByTimeAsync(SCAN_INTERVAL_MS * 4);

    expect(onCodes).toHaveBeenCalledWith(["the link"]);
  });

  it("stops decoding when stopped", async () => {
    const decoder = decoderReturning([]);
    const stop = scanFrames(fakeVideo(), decoder, () => false);
    await vi.advanceTimersByTimeAsync(SCAN_INTERVAL_MS * 2);
    const callsBeforeStop = decoder.calls;

    stop();
    await vi.advanceTimersByTimeAsync(SCAN_INTERVAL_MS * 4);

    expect(decoder.calls).toBe(callsBeforeStop);
  });
});

describe("createDecoder", () => {
  afterEach(() => {
    delete (globalThis as { BarcodeDetector?: unknown }).BarcodeDetector;
    vi.restoreAllMocks();
  });

  function installBarcodeDetector(formats: string[]) {
    const detect = vi.fn(async () => [{ rawValue: "from the built-in detector" }]);
    (globalThis as { BarcodeDetector?: unknown }).BarcodeDetector = class {
      static getSupportedFormats = async () => formats;
      detect = detect;
    };
    return detect;
  }

  it("uses the built-in detector when it reads QR codes", async () => {
    const detect = installBarcodeDetector(["ean_13", "qr_code"]);

    const decoder = await createDecoder();

    await expect(decoder.decode(fakeVideo())).resolves.toEqual(["from the built-in detector"]);
    expect(detect).toHaveBeenCalled();
    expect(jsQR).not.toHaveBeenCalled();
  });

  it("falls back to jsQR on a downscaled frame when the built-in detector lacks QR", async () => {
    installBarcodeDetector(["ean_13"]);
    const context = {
      drawImage: vi.fn(),
      getImageData: vi.fn((_x: number, _y: number, w: number, h: number) => ({
        data: new Uint8ClampedArray(w * h * 4),
      })),
    };
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(context as never);
    vi.mocked(jsQR).mockReturnValue({ data: "from jsQR" } as ReturnType<typeof jsQR>);

    const decoder = await createDecoder();

    await expect(decoder.decode(fakeVideo())).resolves.toEqual(["from jsQR"]);
    expect(jsQR).toHaveBeenCalledWith(expect.any(Uint8ClampedArray), 800, 450, {
      inversionAttempts: "dontInvert",
    });
  });
});
