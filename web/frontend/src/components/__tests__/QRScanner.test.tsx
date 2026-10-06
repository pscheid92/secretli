import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import QRScanner from "../retrieve/QRScanner";

const SECRET = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";

let getUserMedia: ReturnType<typeof vi.fn>;
let detect: ReturnType<typeof vi.fn>;
let stopTrack: ReturnType<typeof vi.fn>;

function fakeStream(): MediaStream {
  const track = { stop: stopTrack, getSettings: () => ({}) };
  return { getTracks: () => [track], getVideoTracks: () => [track] } as unknown as MediaStream;
}

function detectCode(rawValue: string) {
  detect.mockResolvedValue([{ rawValue }]);
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", { value: hidden, configurable: true });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  stopTrack = vi.fn();
  getUserMedia = vi.fn(async () => fakeStream());
  detect = vi.fn(async () => []);
  Object.defineProperty(navigator, "mediaDevices", {
    value: { getUserMedia },
    configurable: true,
  });
  (globalThis as { BarcodeDetector?: unknown }).BarcodeDetector = class {
    static getSupportedFormats = async () => ["qr_code"];
    detect = detect;
  };
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  vi.spyOn(HTMLMediaElement.prototype, "readyState", "get").mockReturnValue(
    HTMLMediaElement.HAVE_ENOUGH_DATA,
  );
});

afterEach(() => {
  Reflect.deleteProperty(navigator, "mediaDevices");
  Reflect.deleteProperty(document, "hidden");
  delete (globalThis as { BarcodeDetector?: unknown }).BarcodeDetector;
  vi.restoreAllMocks();
});

describe("QRScanner", () => {
  it("opens a scanned share link for this site and turns the camera off", async () => {
    detectCode(`${window.location.origin}/s#${SECRET}`);
    const onScan = vi.fn();

    render(<QRScanner onScan={onScan} onCancel={vi.fn()} />);

    await waitFor(() => expect(onScan).toHaveBeenCalledWith(SECRET));
    expect(onScan).toHaveBeenCalledTimes(1);
    expect(stopTrack).toHaveBeenCalled();
    expect(screen.getByText("Opening the secret…")).toBeTruthy();
  });

  it("keeps scanning and names the host of a share link for another site", async () => {
    detectCode(`https://other.example/s#${SECRET}`);
    const onScan = vi.fn();

    render(<QRScanner onScan={onScan} onCancel={vi.fn()} />);

    await screen.findByText("This code is for other.example, not this site.");
    await waitFor(() => expect(detect.mock.calls.length).toBeGreaterThan(1));
    expect(onScan).not.toHaveBeenCalled();
    expect(stopTrack).not.toHaveBeenCalled();
  });

  it("says when a code is not a Secretli link", async () => {
    detectCode("https://phish.example/login");

    render(<QRScanner onScan={vi.fn()} onCancel={vi.fn()} />);

    await screen.findByText("That code isn't a Secretli link.");
  });

  it.each([
    ["NotAllowedError", "Camera access is blocked."],
    ["NotFoundError", "No camera found."],
    ["NotReadableError", "The camera couldn't be started."],
  ])("explains a camera that rejects with %s and offers a retry", async (name, message) => {
    getUserMedia.mockRejectedValueOnce(new DOMException("camera", name));

    render(<QRScanner onScan={vi.fn()} onCancel={vi.fn()} />);

    await screen.findByText(new RegExp(message.replace(/[.?]/g, "\\$&")));
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(getUserMedia).toHaveBeenCalledTimes(2));
  });

  it("releases the camera when the tab is hidden and resumes on request", async () => {
    render(<QRScanner onScan={vi.fn()} onCancel={vi.fn()} />);
    await screen.findByText("Point the camera at the QR code on the other screen.");

    act(() => setHidden(true));

    expect(stopTrack).toHaveBeenCalled();
    await screen.findByText("Scanning paused while this tab was in the background.");

    act(() => setHidden(false));
    fireEvent.click(screen.getByRole("button", { name: "Resume" }));
    await waitFor(() => expect(getUserMedia).toHaveBeenCalledTimes(2));
  });

  it("releases the camera when it goes away", async () => {
    const { unmount } = render(<QRScanner onScan={vi.fn()} onCancel={vi.fn()} />);
    await screen.findByText("Point the camera at the QR code on the other screen.");

    unmount();

    expect(stopTrack).toHaveBeenCalled();
  });

  it("releases a camera that opens after the scanner went away", async () => {
    let resolveCamera: (stream: MediaStream) => void = () => {};
    getUserMedia.mockReturnValueOnce(
      new Promise<MediaStream>((resolve) => {
        resolveCamera = resolve;
      }),
    );
    const { unmount } = render(<QRScanner onScan={vi.fn()} onCancel={vi.fn()} />);

    unmount();
    resolveCamera(fakeStream());

    await waitFor(() => expect(stopTrack).toHaveBeenCalled());
  });
});
