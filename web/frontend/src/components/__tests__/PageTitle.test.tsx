import { render, screen } from "@testing-library/react";
import { createRef } from "react";
import PageTitle from "../ui/PageTitle";

describe("PageTitle", () => {
  it("names the heading without the decorative full stop", () => {
    render(<PageTitle lead="Send this link to the recipient.">Share is ready</PageTitle>);

    expect(screen.getByRole("heading", { level: 1, name: "Share is ready" })).toBeTruthy();
    expect(screen.getByText("Send this link to the recipient.")).toBeTruthy();
  });

  it("can take focus when a page asks for it", () => {
    const ref = createRef<HTMLHeadingElement>();
    render(
      <PageTitle ref={ref} tabIndex={-1}>
        Share is ready
      </PageTitle>,
    );

    ref.current?.focus();
    expect(document.activeElement).toBe(screen.getByRole("heading", { level: 1 }));
  });
});
