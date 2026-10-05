import { fireEvent, render, screen } from "@testing-library/react";
import { toast } from "sonner";
import LinkPrompt from "../retrieve/LinkPrompt";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const SECRET = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";

function submitLink(text: string) {
  fireEvent.change(screen.getByRole("textbox"), { target: { value: text } });
  fireEvent.click(screen.getByRole("button", { name: "Open Share" }));
}

describe("LinkPrompt", () => {
  beforeEach(() => {
    vi.mocked(toast.error).mockClear();
  });

  it("refuses a share link for another host and names that host", () => {
    render(<LinkPrompt />);

    submitLink(`https://other.example/s#${SECRET}`);

    expect(toast.error).toHaveBeenCalledWith(
      "That link is for other.example. Open it there instead.",
    );
    expect(window.location.hash).toBe("");
  });

  it("refuses text that is not a share link", () => {
    render(<LinkPrompt />);

    submitLink("https://other.example/login");

    expect(toast.error).toHaveBeenCalledWith("Please enter a valid Secretli link.");
    expect(window.location.hash).toBe("");
  });
});
