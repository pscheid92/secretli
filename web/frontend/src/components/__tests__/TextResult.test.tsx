import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import TextResult from "../retrieve/TextResult";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

/** Whether leaving the page now would ask first. */
function leavingAsks(): boolean {
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

function renderText(burnAfterRead: boolean) {
  render(
    <TextResult
      text="the launch code is 0000"
      burnAfterRead={burnAfterRead}
      canDelete={false}
      deleting={false}
      onDelete={vi.fn()}
    />,
  );
}

describe("TextResult", () => {
  afterEach(() => {
    Reflect.deleteProperty(navigator, "clipboard");
  });

  it("says a one-time text is the only copy and asks before leaving until it was copied", async () => {
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: vi.fn(async () => {}) },
      configurable: true,
    });
    renderText(true);

    expect(screen.getByText(/When you leave this page, it's gone for good/)).toBeTruthy();
    expect(leavingAsks()).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Copy secret" }));

    await waitFor(() => expect(leavingAsks()).toBe(false));
  });

  it("counts copying by hand", () => {
    renderText(true);

    fireEvent(document, new Event("copy"));

    expect(leavingAsks()).toBe(false);
  });

  it("lets a reusable text go without asking", () => {
    renderText(false);

    expect(screen.queryByText(/When you leave this page, it's gone for good/)).toBeNull();
    expect(leavingAsks()).toBe(false);
  });
});
