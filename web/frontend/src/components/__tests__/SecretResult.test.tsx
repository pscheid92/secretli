import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { toast } from "sonner";
import SecretResult from "../SecretResult";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() } }));

const SHARE_URL = "https://secretli.example/s#AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";
const DELETION_TOKEN = "ZyXwVuTsRqPoNmLkJiHgFeDcBa9876543210_-ZyXwV";
const OWNER_URL = `${SHARE_URL}!${DELETION_TOKEN}`;

function renderResult(
  options: { burnAfterRead?: boolean; passwordProtected?: boolean; onDelete?: () => void } = {},
) {
  render(
    <SecretResult
      url={SHARE_URL}
      expiresAt={new Date(Date.now() + 3600_000).toISOString()}
      burnAfterRead={options.burnAfterRead ?? true}
      passwordProtected={options.passwordProtected ?? false}
      deletionToken={DELETION_TOKEN}
      deleting={false}
      onDelete={options.onDelete ?? vi.fn()}
    />,
  );
}

function stubNavigator(name: "clipboard" | "share", value: unknown) {
  Object.defineProperty(navigator, name, { value, configurable: true });
}

function openOwnerLink() {
  fireEvent.click(screen.getByRole("button", { name: "Owner link" }));
}

describe("SecretResult", () => {
  beforeEach(() => {
    vi.mocked(toast.error).mockClear();
    // jsdom does not implement scrolling.
    Element.prototype.scrollIntoView = vi.fn();
  });

  afterEach(() => {
    Reflect.deleteProperty(navigator, "clipboard");
    Reflect.deleteProperty(navigator, "share");
  });

  it("moves focus to the title that replaced the form", () => {
    renderResult();

    expect(document.activeElement).toBe(
      screen.getByRole("heading", { name: "Your link is ready" }),
    );
  });

  it("sums the link up under the title", () => {
    renderResult({ passwordProtected: true });

    expect(
      screen.getByText(/^Opens once · expires (today|tomorrow) at .+ · password required$/),
    ).toBeTruthy();
  });

  it("copies the recipient link and says so on the button", async () => {
    const writeText = vi.fn(async () => {});
    stubNavigator("clipboard", { writeText });
    renderResult();

    fireEvent.click(screen.getByRole("button", { name: "Copy link" }));

    expect(await screen.findByRole("button", { name: "Copied" })).toBeTruthy();
    expect(writeText).toHaveBeenCalledWith(SHARE_URL);
  });

  it("explains how to copy by hand when the clipboard refuses", async () => {
    stubNavigator("clipboard", {
      writeText: vi.fn(async () => {
        throw new DOMException("denied", "NotAllowedError");
      }),
    });
    renderResult();

    fireEvent.click(screen.getByRole("button", { name: "Copy link" }));

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
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

  it("reminds the sender to send a password another way", () => {
    renderResult({ passwordProtected: true });

    expect(screen.getByText(/Send the password another way/)).toBeTruthy();
  });

  it("doesn't mention a password the link doesn't have", () => {
    renderResult();

    expect(screen.queryByText(/Send the password another way/)).toBeNull();
    expect(screen.queryByText(/password required/)).toBeNull();
  });

  it("keeps the owner link folded away, then explains and copies it", async () => {
    const writeText = vi.fn(async () => {});
    stubNavigator("clipboard", { writeText });
    renderResult();

    expect(screen.queryByTestId("owner-link")).toBeNull();
    openOwnerLink();

    expect(screen.getByText(/Keep this link to yourself/)).toBeTruthy();
    expect(screen.getByTestId("owner-link").textContent).toBe(OWNER_URL);
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledWith(OWNER_URL));
  });

  it("asks before deleting the secret, then deletes it", () => {
    const onDelete = vi.fn();
    renderResult({ onDelete });
    openOwnerLink();

    fireEvent.click(screen.getByRole("button", { name: "Delete it now" }));
    expect(onDelete).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Delete permanently" }));

    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  describe("QR code", () => {
    it("shows the recipient link as an SVG QR code and scrolls to it", async () => {
      renderResult();

      fireEvent.click(screen.getByRole("button", { name: "QR code" }));

      const image = await screen.findByAltText("QR code for share link");
      expect(image.getAttribute("src")).toMatch(/^data:image\/svg\+xml;/);
      expect(screen.getByText(/anyone who sees the code can open the secret/)).toBeTruthy();
      expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({ block: "nearest" });
    });

    it("hides the code again", async () => {
      renderResult();
      fireEvent.click(screen.getByRole("button", { name: "QR code" }));
      await screen.findByAltText("QR code for share link");

      fireEvent.click(screen.getByRole("button", { name: "QR code" }));

      expect(screen.queryByAltText("QR code for share link")).toBeNull();
    });
  });
});
