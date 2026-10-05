import { fireEvent, render, screen } from "@testing-library/react";
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
}

describe("SecretResult QR code", () => {
  beforeEach(() => {
    // jsdom does not implement scrolling.
    Element.prototype.scrollIntoView = vi.fn();
  });

  it("shows the recipient link as an SVG QR code across the box and scrolls to it", async () => {
    renderResult();

    fireEvent.click(screen.getByRole("button", { name: "Show QR code" }));

    const image = await screen.findByAltText("QR code for share link");
    expect(image.getAttribute("src")).toMatch(/^data:image\/svg\+xml;/);
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
