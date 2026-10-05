import { fireEvent, render, screen, within } from "@testing-library/react";
import SecretResult from "../SecretResult";

const SHARE_URL = "https://secretli.example/s#AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";

function renderResult() {
  render(
    <SecretResult
      url={SHARE_URL}
      expiresAt="2026-10-06T12:00:00Z"
      burnAfterRead={false}
      deletionToken="ZyXwVuTsRqPoNmLkJiHgFeDcBa9876543210_-ZyXwV"
    />,
  );
  return screen.getByRole("button", { name: "Show large QR code" });
}

describe("SecretResult large QR code", () => {
  it("shows the recipient link as a large SVG QR code", async () => {
    fireEvent.click(renderResult());

    const dialog = screen.getByRole("dialog", { name: "Large QR code" });
    const image = await within(dialog).findByAltText("QR code for share link");
    expect(image.getAttribute("src")).toMatch(/^data:image\/svg\+xml;/);
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Close" }));
  });

  it("closes on Escape and returns focus to the trigger", () => {
    const trigger = renderResult();
    fireEvent.click(trigger);

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("closes on a click outside the code but not on the code's caption", () => {
    fireEvent.click(renderResult());
    const dialog = screen.getByRole("dialog");

    fireEvent.click(within(dialog).getByText(/Anyone who can see it/));
    expect(screen.queryByRole("dialog")).not.toBeNull();

    fireEvent.click(dialog);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
