import { fireEvent, render, screen } from "@testing-library/react";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import PasswordInput from "../ui/PasswordInput";
import TextButton from "../ui/TextButton";

describe("shared buttons", () => {
  it("never submit a form unless asked to", () => {
    render(
      <form>
        <Button>Plain</Button>
        <TextButton>Copy</TextButton>
        <Button type="submit">Send</Button>
      </form>,
    );

    expect(screen.getByRole("button", { name: "Plain" }).getAttribute("type")).toBe("button");
    expect(screen.getByRole("button", { name: "Copy" }).getAttribute("type")).toBe("button");
    expect(screen.getByRole("button", { name: "Send" }).getAttribute("type")).toBe("submit");
  });

  it("give an icon button its label as name and tooltip", () => {
    render(
      <IconButton label="Switch theme">
        <svg aria-hidden="true" />
      </IconButton>,
    );

    expect(screen.getByRole("button", { name: "Switch theme" }).getAttribute("title")).toBe(
      "Switch theme",
    );
  });

  it("let a password field show what was typed, and hide it again", () => {
    render(<PasswordInput aria-label="Password" defaultValue="hunter2" />);
    const field = screen.getByLabelText("Password") as HTMLInputElement;
    const toggle = screen.getByRole("button", { name: "Show" });

    expect(field.type).toBe("password");
    fireEvent.click(toggle);
    expect(field.type).toBe("text");
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(toggle);
    expect(field.type).toBe("password");
  });

  it("keep extra classes next to the shared ones", () => {
    render(<Button className="mt-4">Reveal</Button>);

    const classes = screen.getByRole("button", { name: "Reveal" }).className;
    expect(classes).toContain("bg-accent");
    expect(classes).toContain("mt-4");
  });
});
