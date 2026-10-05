import { fireEvent, render, screen } from "@testing-library/react";
import ExpirationPicker from "../ExpirationPicker";

describe("ExpirationPicker", () => {
  it("names the options in words and marks the chosen one as pressed", () => {
    render(<ExpirationPicker value="1d" onChange={vi.fn()} />);

    expect(screen.getByRole("group", { name: "Expires in" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "1 day" }).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByRole("button", { name: "5 minutes" }).getAttribute("aria-pressed")).toBe(
      "false",
    );
  });

  it("reports the option that was picked", () => {
    const onChange = vi.fn();
    render(<ExpirationPicker value="1d" onChange={onChange} />);

    fireEvent.click(screen.getByRole("button", { name: "4 hours" }));

    expect(onChange).toHaveBeenCalledWith("4h");
  });
});
