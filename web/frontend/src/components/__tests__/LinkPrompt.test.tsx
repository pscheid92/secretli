import { fireEvent, render, screen } from "@testing-library/react";
import { toast } from "sonner";
import LinkPrompt from "../retrieve/LinkPrompt";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const SECRET = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";

function submitLink(text: string) {
  fireEvent.change(screen.getByRole("textbox"), { target: { value: text } });
  fireEvent.click(screen.getByRole("button", { name: "Open Share" }));
}

describe("LinkPrompt", () => {
  beforeEach(() => {
    vi.mocked(toast.error).mockClear();
  });

  afterEach(() => {
    window.history.replaceState(null, "", "/s");
  });

  it("opens a share link for this site by putting its fragment in the address", () => {
    render(<LinkPrompt />);

    submitLink(`${window.location.origin}/s#${SECRET}`);

    expect(toast.error).not.toHaveBeenCalled();
    expect(window.location.hash).toBe(`#${SECRET}`);
  });

  it("refuses a share link for another host and names that host", () => {
    render(<LinkPrompt />);

    submitLink(`https://other.example/s#${SECRET}`);

    expect(toast.error).toHaveBeenCalledWith(
      "That link is for other.example. Open it there instead.",
    );
    expect(window.location.hash).toBe("");
  });

  it("refuses text that is not a share link", () => {
    render(<LinkPrompt />);

    submitLink("https://other.example/login");

    expect(toast.error).toHaveBeenCalledWith("Please enter a valid Secretli link.");
    expect(window.location.hash).toBe("");
  });

  it("offers entering a transfer code, also without a camera", () => {
    render(<LinkPrompt />);

    fireEvent.click(screen.getByRole("button", { name: "Enter a code" }));

    expect(screen.getByRole("region", { name: "Enter a code" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Enter a code" })).toBeNull();
  });

  describe("QR scanning", () => {
    afterEach(() => {
      Reflect.deleteProperty(navigator, "mediaDevices");
      Reflect.deleteProperty(window, "isSecureContext");
    });

    it("is not offered without camera access", () => {
      render(<LinkPrompt />);

      expect(screen.queryByRole("button", { name: "Scan QR code" })).toBeNull();
    });

    it("opens the scanner when camera access is available", () => {
      Object.defineProperty(window, "isSecureContext", { value: true, configurable: true });
      Object.defineProperty(navigator, "mediaDevices", {
        // Never settles: this test only checks that the scanner appears.
        value: { getUserMedia: () => new Promise(() => {}) },
        configurable: true,
      });
      render(<LinkPrompt />);

      fireEvent.click(screen.getByRole("button", { name: "Scan QR code" }));

      expect(screen.getByRole("region", { name: "QR code scanner" })).toBeTruthy();
      expect(screen.getByText("Starting the camera…")).toBeTruthy();
    });
  });
});
