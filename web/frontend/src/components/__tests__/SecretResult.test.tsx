import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { toast } from "sonner";
import SecretResult from "../SecretResult";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const SHARE_URL = "https://secretli.example/s#AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";
const DELETION_TOKEN = "ZyXwVuTsRqPoNmLkJiHgFeDcBa9876543210_-ZyXwV";

function renderResult(options: { burnAfterRead?: boolean; passwordProtected?: boolean } = {}) {
  render(
    <SecretResult
      url={SHARE_URL}
      expiresAt="2026-10-06T12:00:00Z"
      burnAfterRead={options.burnAfterRead ?? false}
      passwordProtected={options.passwordProtected ?? false}
      deletionToken={DELETION_TOKEN}
    />,
  );
}

function stubNavigator(name: "clipboard" | "share", value: unknown) {
  Object.defineProperty(navigator, name, { value, configurable: true });
}

describe("SecretResult", () => {
  beforeEach(() => {
    vi.mocked(toast.error).mockClear();
    vi.mocked(toast.success).mockClear();
    // jsdom does not implement scrolling.
    Element.prototype.scrollIntoView = vi.fn();
  });

  afterEach(() => {
    Reflect.deleteProperty(navigator, "clipboard");
    Reflect.deleteProperty(navigator, "share");
  });

  it("moves focus to the result that replaced the form", () => {
    renderResult();

    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Share is ready" }));
  });

  it("copies the recipient link and says so on the button", async () => {
    const writeText = vi.fn(async () => {});
    stubNavigator("clipboard", { writeText });
    renderResult();

    fireEvent.click(screen.getByRole("button", { name: "Copy link" }));

    expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
    expect(writeText).toHaveBeenCalledWith(SHARE_URL);
  });

  it("selects the link for copying by hand when the clipboard refuses", async () => {
    stubNavigator("clipboard", {
      writeText: vi.fn(async () => {
        throw new DOMException("denied", "NotAllowedError");
      }),
    });
    renderResult();

    fireEvent.click(screen.getByRole("button", { name: "Copy link" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(document.activeElement).toBe(screen.getByLabelText("Recipient link"));
    expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy();
  });

  it("offers the system share sheet only where the browser has one", () => {
    renderResult();

    expect(screen.queryByRole("button", { name: "Share…" })).toBeNull();
  });

  it("hands the link to the share sheet, and a closed sheet is no error", async () => {
    const share = vi.fn(async () => {
      throw new DOMException("closed", "AbortError");
    });
    stubNavigator("share", share);
    renderResult();

    fireEvent.click(screen.getByRole("button", { name: "Share…" }));

    await waitFor(() => expect(share).toHaveBeenCalledWith({ url: SHARE_URL }));
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("reminds the sender to send a password separately", () => {
    renderResult({ passwordProtected: true });

    expect(screen.getByText(/Send the password separately/)).toBeTruthy();
    expect(screen.getByText("Required")).toBeTruthy();
  });

  it("doesn't mention a password the share doesn't have", () => {
    renderResult();

    expect(screen.queryByText(/Send the password separately/)).toBeNull();
  });

  it("explains the owner link and copies it", async () => {
    const writeText = vi.fn(async () => {});
    stubNavigator("clipboard", { writeText });
    renderResult();

    expect(screen.getByText(/Delete the share early with this link/)).toBeTruthy();
    expect((screen.getByLabelText("Owner link") as HTMLInputElement).value).toBe(
      `${SHARE_URL}!${DELETION_TOKEN}`,
    );
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(`${SHARE_URL}!${DELETION_TOKEN}`));
  });

  describe("QR code", () => {
    it("shows the recipient link as an SVG QR code across the box and scrolls to it", async () => {
      renderResult();

      fireEvent.click(screen.getByRole("button", { name: "Show QR code" }));

      const image = await screen.findByAltText("QR code for share link");
      expect(image.getAttribute("src")).toMatch(/^data:image\/svg\+xml;/);
      expect(screen.getByText("Anyone who can see this code can open the share.")).toBeTruthy();
      expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: "nearest" });
    });

    it("hides the code again", async () => {
      renderResult();
      fireEvent.click(screen.getByRole("button", { name: "Show QR code" }));
      await screen.findByAltText("QR code for share link");

      fireEvent.click(screen.getByRole("button", { name: "Hide QR code" }));

      expect(screen.queryByAltText("QR code for share link")).toBeNull();
    });
  });
});
