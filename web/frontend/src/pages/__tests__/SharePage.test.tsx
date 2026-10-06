import { fireEvent, render, screen } from "@testing-library/react";
import SharePage from "../SharePage";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() } }));

describe("SharePage", () => {
  afterEach(() => {
    window.history.replaceState(null, "", "/share");
  });

  it("starts with the text shared into the app, and clears the address", () => {
    window.history.replaceState(null, "", "/share?text=hunter2");

    render(<SharePage />);

    expect((screen.getByLabelText("Secret") as HTMLTextAreaElement).value).toBe("hunter2");
    expect(window.location.search).toBe("");
    expect(
      (screen.getByRole("button", { name: /Create link/ }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("starts empty otherwise", () => {
    render(<SharePage />);

    expect((screen.getByLabelText("Secret") as HTMLTextAreaElement).value).toBe("");
    fireEvent.change(screen.getByLabelText("Secret"), { target: { value: "typed" } });
    expect((screen.getByLabelText("Secret") as HTMLTextAreaElement).value).toBe("typed");
  });
});
