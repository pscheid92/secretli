import { fireEvent, render, screen } from "@testing-library/react";
import { MAX_FILE_UPLOAD_BYTES, MAX_UPLOAD_LABEL } from "../../lib/uploadLimits";
import FileUpload from "../FileUpload";

function fileWithSize(size: number): File {
  const file = new File(["x"], "payload.bin", { type: "application/octet-stream" });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

function fileInput(container: HTMLElement): HTMLInputElement {
  const input = container.querySelector('input[type="file"]');
  if (!(input instanceof HTMLInputElement)) {
    throw new Error("file input not found");
  }
  return input;
}

describe("FileUpload", () => {
  it("accepts files that fit after bundle encryption", () => {
    const onSelect = vi.fn();
    const file = fileWithSize(1024);
    const { container } = render(<FileUpload onSelect={onSelect} />);

    fireEvent.change(fileInput(container), { target: { files: [file] } });

    expect(onSelect).toHaveBeenCalledWith([file]);
    expect(screen.queryByText(/upload limit/i)).toBeNull();
  });

  it("rejects a file that would exceed the encrypted backend limit", () => {
    const onSelect = vi.fn();
    const { container } = render(<FileUpload onSelect={onSelect} />);

    fireEvent.change(fileInput(container), {
      target: { files: [fileWithSize(MAX_FILE_UPLOAD_BYTES + 1)] },
    });

    expect(onSelect).not.toHaveBeenCalled();
    expect(
      screen.getByText(`Total file size exceeds the ${MAX_UPLOAD_LABEL} upload limit.`),
    ).toBeTruthy();
  });
});
