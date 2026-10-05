import { render, screen } from "@testing-library/react";
import { useLeaveWarning, useLeaveWarningActive } from "../useLeaveWarning";

function Guard({ active }: { active: boolean }) {
  useLeaveWarning(active);
  return null;
}

function Links() {
  return <p>{useLeaveWarningActive() ? "full page loads" : "client-side"}</p>;
}

/** Whether leaving the page now would ask first. */
function leavingAsks(): boolean {
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

describe("useLeaveWarning", () => {
  it("asks before leaving only while active", () => {
    const { rerender, unmount } = render(<Guard active={false} />);
    expect(leavingAsks()).toBe(false);

    rerender(<Guard active />);
    expect(leavingAsks()).toBe(true);

    unmount();
    expect(leavingAsks()).toBe(false);
  });

  it("keeps asking while any of several warnings is active", () => {
    const first = render(<Guard active />);
    const second = render(<Guard active />);

    first.unmount();
    expect(leavingAsks()).toBe(true);

    second.unmount();
    expect(leavingAsks()).toBe(false);
  });

  it("switches in-app links to full page loads meanwhile", () => {
    const { rerender } = render(
      <>
        <Guard active={false} />
        <Links />
      </>,
    );
    expect(screen.getByText("client-side")).toBeTruthy();

    rerender(
      <>
        <Guard active />
        <Links />
      </>,
    );
    expect(screen.getByText("full page loads")).toBeTruthy();
  });
});
