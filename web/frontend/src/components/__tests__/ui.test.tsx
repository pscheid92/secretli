import { render, screen } from "@testing-library/react";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
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

  it("keep extra classes next to the shared ones", () => {
    render(<Button className="mt-4">Reveal</Button>);

    const classes = screen.getByRole("button", { name: "Reveal" }).className;
    expect(classes).toContain("bg-amber-400");
    expect(classes).toContain("mt-4");
  });
});
