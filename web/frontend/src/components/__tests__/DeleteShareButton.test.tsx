import { fireEvent, render, screen } from "@testing-library/react";
import DeleteShareButton from "../retrieve/DeleteShareButton";

describe("DeleteShareButton", () => {
  it("asks for confirmation before deleting", () => {
    const onDelete = vi.fn();
    render(<DeleteShareButton deleting={false} onDelete={onDelete} />);

    fireEvent.click(screen.getByRole("button", { name: "Delete share" }));

    expect(onDelete).not.toHaveBeenCalled();
    expect(screen.getByRole("group", { name: "Delete share" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Delete permanently" }));
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("goes back without deleting when the confirmation is cancelled", () => {
    const onDelete = vi.fn();
    render(<DeleteShareButton deleting={false} onDelete={onDelete} />);

    fireEvent.click(screen.getByRole("button", { name: "Delete share" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onDelete).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Delete share" })).toBeTruthy();
  });

  it("disables both buttons while the deletion runs", () => {
    render(<DeleteShareButton deleting onDelete={vi.fn()} />);

    expect(screen.getByRole("button", { name: "Deleting..." }).hasAttribute("disabled")).toBe(true);
    expect(screen.getByRole("button", { name: "Cancel" }).hasAttribute("disabled")).toBe(true);
  });
});
