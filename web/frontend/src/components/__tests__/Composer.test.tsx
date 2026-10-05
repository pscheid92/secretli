import { fireEvent, render, screen } from "@testing-library/react";
import { MAX_ENCRYPTED_UPLOAD_BYTES } from "../../lib/uploadLimits";
import Composer from "../compose/Composer";

function fileInput(): HTMLInputElement {
  const input = document.querySelector('input[type="file"]');
  if (!(input instanceof HTMLInputElement)) throw new Error("file input not found");
  return input;
}

function fileWithSize(name: string, size: number): File {
  const file = new File(["x"], name, { type: "application/octet-stream" });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

const createButton = () => screen.getByRole("button", { name: /Create link/ });
const secretBox = () => screen.getByLabelText("Secret") as HTMLTextAreaElement;

describe("Composer", () => {
  it("makes a one-time link from the text, once there is some", () => {
    const onSubmit = vi.fn();
    render(<Composer onSubmit={onSubmit} busy={null} />);

    expect(createButton().hasAttribute("disabled")).toBe(true);
    fireEvent.change(secretBox(), { target: { value: "hunter2" } });
    fireEvent.click(createButton());

    expect(onSubmit).toHaveBeenCalledWith({
      kind: "text",
      text: "hunter2",
      expiration: "1d",
      burnAfterRead: true,
      password: "",
    });
  });

  it("submits with ⌘ or Ctrl and Enter from the text box", () => {
    const onSubmit = vi.fn();
    render(<Composer onSubmit={onSubmit} busy={null} />);

    fireEvent.change(secretBox(), { target: { value: "hunter2" } });
    fireEvent.keyDown(secretBox(), { key: "Enter", metaKey: true });
    fireEvent.keyDown(secretBox(), { key: "Enter", ctrlKey: true });
    fireEvent.keyDown(secretBox(), { key: "Enter" });

    expect(onSubmit).toHaveBeenCalledTimes(2);
  });

  it("swaps the text box for the attached files, and brings it back when they go", () => {
    const onSubmit = vi.fn();
    render(<Composer onSubmit={onSubmit} busy={null} />);
    fireEvent.change(secretBox(), { target: { value: "a note" } });
    const file = new File(["x"], "notes.txt", { type: "text/plain" });

    fireEvent.change(fileInput(), { target: { files: [file] } });

    expect(screen.queryByLabelText("Secret")).toBeNull();
    expect(screen.getByText("notes.txt")).toBeTruthy();
    expect(screen.getByText(/1 file · /)).toBeTruthy();
    fireEvent.click(createButton());
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "files", files: [file] }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Remove notes.txt" }));

    expect(secretBox().value).toBe("a note");
  });

  it("adds files once, even when the same file is picked again", () => {
    render(<Composer onSubmit={vi.fn()} busy={null} />);
    const file = new File(["x"], "notes.txt", { type: "text/plain" });

    fireEvent.change(fileInput(), { target: { files: [file] } });
    fireEvent.change(fileInput(), { target: { files: [file, new File(["y"], "more.txt")] } });

    expect(screen.getAllByText("notes.txt")).toHaveLength(1);
    expect(screen.getByText(/2 files · /)).toBeTruthy();
  });

  it("refuses files beyond the limits and says why", () => {
    render(<Composer onSubmit={vi.fn()} busy={null} />);

    fireEvent.change(fileInput(), {
      target: { files: [fileWithSize("huge.bin", MAX_ENCRYPTED_UPLOAD_BYTES)] },
    });

    expect(screen.getByRole("alert").textContent).toMatch(/exceed the 1 GiB limit/);
    expect(screen.getByLabelText("Secret")).toBeTruthy();

    const many = Array.from({ length: 3000 }, (_, i) => new File(["x"], `many-${i}.bin`));
    fireEvent.change(fileInput(), { target: { files: many } });

    expect(screen.getByRole("alert").textContent).toMatch(/Too many files/);
  });

  it("needs a password once that option is on", () => {
    const onSubmit = vi.fn();
    render(<Composer onSubmit={onSubmit} busy={null} />);
    fireEvent.change(secretBox(), { target: { value: "hunter2" } });

    fireEvent.click(screen.getByRole("button", { name: "Password" }));
    fireEvent.click(createButton());

    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText("Add a password, or turn it off.")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByLabelText("Password"));

    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "s3cret" } });
    fireEvent.click(createButton());

    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ password: "s3cret" }));
  });

  it("forgets the password when the option is turned off again", () => {
    const onSubmit = vi.fn();
    render(<Composer onSubmit={onSubmit} busy={null} />);
    fireEvent.change(secretBox(), { target: { value: "hunter2" } });
    fireEvent.click(screen.getByRole("button", { name: "Password" }));
    fireEvent.change(screen.getByLabelText("Password"), { target: { value: "s3cret" } });

    fireEvent.click(screen.getByRole("button", { name: "Password" }));
    fireEvent.click(createButton());

    expect(screen.queryByLabelText("Password")).toBeNull();
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ password: "" }));
  });

  it("changes the expiry and the opens setting from the bar", () => {
    const onSubmit = vi.fn();
    render(<Composer onSubmit={onSubmit} busy={null} />);
    fireEvent.change(secretBox(), { target: { value: "hunter2" } });

    fireEvent.click(screen.getByRole("button", { name: "Expires in 1 day" }));
    fireEvent.click(screen.getByRole("button", { name: "4 hours" }));
    expect(screen.getByRole("button", { name: "Expires in 4 hours" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Opens once" }));
    fireEvent.click(screen.getByRole("button", { name: /Until it expires/ }));
    expect(screen.getByRole("button", { name: "Opens until it expires" })).toBeTruthy();

    fireEvent.click(createButton());
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ expiration: "4h", burnAfterRead: false }),
    );
  });

  it("closes a menu with Escape, and when something else is clicked", () => {
    render(<Composer onSubmit={vi.fn()} busy={null} />);

    fireEvent.click(screen.getByRole("button", { name: "Expires in 1 day" }));
    expect(screen.getByRole("button", { name: "5 minutes" })).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("button", { name: "5 minutes" }), { key: "Escape" });
    expect(screen.queryByRole("button", { name: "5 minutes" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Expires in 1 day" }));

    fireEvent.click(screen.getByRole("button", { name: "Expires in 1 day" }));
    fireEvent.mouseDown(document.body);
    expect(screen.queryByRole("button", { name: "5 minutes" })).toBeNull();
  });

  it("shows what the page is doing, and lets an upload be cancelled", () => {
    const onCancel = vi.fn();
    render(
      <Composer
        onSubmit={vi.fn()}
        busy={{ stage: "uploading", fraction: 0.4, label: "4.0 MB / 10.0 MB", onCancel }}
      />,
    );

    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("40");
    expect(screen.getByRole("status").textContent).toContain("4.0 MB / 10.0 MB");
    expect(screen.getByRole("button", { name: /Creating link/ }).hasAttribute("disabled")).toBe(
      true,
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel upload" }));

    expect(onCancel).toHaveBeenCalled();
  });
});
