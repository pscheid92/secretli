/** Reads QR codes from a playing camera video. */
export interface QRDecoder {
  decode(video: HTMLVideoElement): Promise<string[]>;
}

/** Why the camera could not be used. */
export type CameraFailure = "no-camera" | "camera-failed" | "blocked";

/** About 8 decodes a second: plenty for a QR code held still, and easy on the CPU. */
export const SCAN_INTERVAL_MS = 125;

/** jsQR's cost grows with the pixel count; a QR code filling part of the frame needs no more. */
const MAX_DECODE_WIDTH = 800;

// BarcodeDetector is not in TypeScript's DOM types yet.
interface BarcodeDetectorInstance {
  detect(source: HTMLVideoElement): Promise<{ rawValue: string }[]>;
}
interface BarcodeDetectorConstructor {
  new (options: { formats: string[] }): BarcodeDetectorInstance;
  getSupportedFormats(): Promise<string[]>;
}

/** Camera access needs a secure context; without it there is nothing to offer. */
export function canScan(): boolean {
  return (
    window.isSecureContext === true && typeof navigator.mediaDevices?.getUserMedia === "function"
  );
}

export function openCamera(): Promise<MediaStream> {
  // Laptops ignore facingMode and use the webcam; phones pick the back camera.
  return navigator.mediaDevices.getUserMedia({
    video: { facingMode: "environment", width: { ideal: 1280 }, height: { ideal: 720 } },
    audio: false,
  });
}

/** Turns the camera (and its indicator light) off. */
export function stopCamera(stream: MediaStream): void {
  for (const track of stream.getTracks()) {
    track.stop();
  }
}

export function cameraFailure(err: unknown): CameraFailure {
  const name = err instanceof DOMException ? err.name : "";
  if (name === "NotAllowedError" || name === "SecurityError") return "blocked";
  if (name === "NotFoundError" || name === "OverconstrainedError") return "no-camera";
  return "camera-failed";
}

/**
 * Uses the browser's built-in detector where there is one (Chrome and Edge on
 * macOS, ChromeOS and Android) and loads jsQR everywhere else.
 */
export async function createDecoder(): Promise<QRDecoder> {
  const BuiltIn = (globalThis as { BarcodeDetector?: BarcodeDetectorConstructor }).BarcodeDetector;
  if (BuiltIn) {
    try {
      if ((await BuiltIn.getSupportedFormats()).includes("qr_code")) {
        const detector = new BuiltIn({ formats: ["qr_code"] });
        return {
          decode: async (video) => (await detector.detect(video)).map((code) => code.rawValue),
        };
      }
    } catch {
      // Fall through to jsQR.
    }
  }

  const { default: jsQR } = await import("jsqr");
  const canvas = document.createElement("canvas");
  const context = canvas.getContext("2d", { willReadFrequently: true });
  if (!context) {
    throw new Error("canvas 2D context unavailable");
  }
  return {
    decode: async (video) => {
      const scale = Math.min(1, MAX_DECODE_WIDTH / video.videoWidth);
      const width = Math.round(video.videoWidth * scale);
      const height = Math.round(video.videoHeight * scale);
      if (width === 0 || height === 0) return [];
      if (canvas.width !== width || canvas.height !== height) {
        canvas.width = width;
        canvas.height = height;
      }
      context.drawImage(video, 0, 0, width, height);
      const { data } = context.getImageData(0, 0, width, height);
      // Secretli's codes are always dark on white; skipping the inverted pass halves the work.
      const code = jsQR(data, width, height, { inversionAttempts: "dontInvert" });
      return code ? [code.data] : [];
    },
  };
}

/**
 * Decodes frames until `onCodes` returns true or the returned function is
 * called. Frames that fail to decode are skipped.
 */
export function scanFrames(
  video: HTMLVideoElement,
  decoder: QRDecoder,
  onCodes: (texts: string[]) => boolean,
): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function tick() {
    if (video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
      try {
        const texts = await decoder.decode(video);
        if (stopped) return;
        if (texts.length > 0 && onCodes(texts)) return;
      } catch {
        if (stopped) return;
      }
    }
    timer = setTimeout(tick, SCAN_INTERVAL_MS);
  }

  timer = setTimeout(tick, 0);
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}
