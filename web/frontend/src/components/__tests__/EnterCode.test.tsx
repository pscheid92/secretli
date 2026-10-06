import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import EnterCode from "../retrieve/EnterCode";

const receiveWithCode = vi.fn();
vi.mock("../../lib/transferSession", () => ({
  receiveWithCode: (code: string, signal: AbortSignal) => receiveWithCode(code, signal),
  describeReceiveError: (err: Error) => `explained: ${err.message}`,
}));

const SECRET = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";

const numberField = () => screen.getByLabelText("Number") as HTMLInputElement;
const firstWord = () => screen.getByLabelText("First word") as HTMLInputElement;
const secondWord = () => screen.getByLabelText("Second word") as HTMLInputElement;

/** Types a whole code into the first field, as pasting it would. */
function submit(code: string) {
  fireEvent.change(numberField(), { target: { value: code } });
  fireEvent.click(screen.getByRole("button", { name: "Receive" }));
}

describe("EnterCode", () => {
  beforeEach(() => {
    receiveWithCode.mockReset();
  });

  it("spreads a pasted code over the three fields and sends it as typed", async () => {
    receiveWithCode.mockResolvedValue(`${window.location.origin}/s#${SECRET}`);
    const onReceived = vi.fn();
    render(<EnterCode onReceived={onReceived} onCancel={vi.fn()} />);

    submit("7-aci-roc");

    expect(numberField().value).toBe("7");
    expect(firstWord().value).toBe("aci");
    expect(secondWord().value).toBe("roc");
    await waitFor(() => expect(onReceived).toHaveBeenCalledWith(SECRET));
    expect(receiveWithCode).toHaveBeenCalledWith("7-aci-roc", expect.any(AbortSignal));
  });

  it("moves on at a dash, and completes a word from three letters", async () => {
    render(<EnterCode onReceived={vi.fn()} onCancel={vi.fn()} />);

    fireEvent.change(numberField(), { target: { value: "7-" } });
    expect(numberField().value).toBe("7");
    expect(document.activeElement).toBe(firstWord());

    fireEvent.change(firstWord(), { target: { value: "aci" } });
    await waitFor(() => expect(screen.getByTestId("ghost-first").textContent).toBe("d"));

    fireEvent.change(firstWord(), { target: { value: "aci-" } });
    expect(firstWord().value).toBe("acid");
    expect(document.activeElement).toBe(secondWord());

    fireEvent.change(secondWord(), { target: { value: "ROC" } });
    fireEvent.blur(secondWord());
    expect(secondWord().value).toBe("rocket");
  });

  it("steps back with Backspace in an empty field", () => {
    render(<EnterCode onReceived={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.change(numberField(), { target: { value: "7-acid-" } });
    expect(document.activeElement).toBe(secondWord());

    fireEvent.keyDown(secondWord(), { key: "Backspace" });

    expect(document.activeElement).toBe(firstWord());
    expect(firstWord().value).toBe("acid");
  });

  it("waits for all three parts before it can receive", () => {
    render(<EnterCode onReceived={vi.fn()} onCancel={vi.fn()} />);
    const receive = screen.getByRole("button", { name: "Receive" }) as HTMLButtonElement;

    expect(receive.disabled).toBe(true);
    fireEvent.change(numberField(), { target: { value: "7-acid" } });
    expect(receive.disabled).toBe(true);
    fireEvent.change(secondWord(), { target: { value: "rocket" } });
    expect(receive.disabled).toBe(false);
  });

  it("explains a code that didn't work and lets the user try again", async () => {
    receiveWithCode.mockImplementation(async () => {
      throw new Error("already used");
    });
    render(<EnterCode onReceived={vi.fn()} onCancel={vi.fn()} />);

    submit("7-acid-rocket");

    await screen.findByText("explained: already used");
    expect((screen.getByRole("button", { name: "Receive" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("refuses a delivered link for another site", async () => {
    receiveWithCode.mockResolvedValue(`https://other.example/s#${SECRET}`);
    const onReceived = vi.fn();
    render(<EnterCode onReceived={onReceived} onCancel={vi.fn()} />);

    submit("7-acid-rocket");

    await screen.findByText("That code delivered a link for another site.");
    expect(onReceived).not.toHaveBeenCalled();
  });

  it("aborts a running transfer when it goes away", async () => {
    let signal: AbortSignal | undefined;
    receiveWithCode.mockImplementation((_code: string, s: AbortSignal) => {
      signal = s;
      return new Promise(() => {});
    });
    const { unmount } = render(<EnterCode onReceived={vi.fn()} onCancel={vi.fn()} />);
    submit("7-acid-rocket");
    await waitFor(() => expect(signal).toBeDefined());
    expect(screen.getByText("Connecting to the other device…")).toBeTruthy();

    unmount();

    expect(signal?.aborted).toBe(true);
  });
});
