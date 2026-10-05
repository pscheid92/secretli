import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import EnterCode from "../retrieve/EnterCode";

const receiveWithCode = vi.fn();
vi.mock("../../lib/transferSession", () => ({
  receiveWithCode: (code: string, signal: AbortSignal) => receiveWithCode(code, signal),
  describeReceiveError: (err: Error) => `explained: ${err.message}`,
}));

const SECRET = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789-_AbCdE";

function submit(code: string) {
  fireEvent.change(screen.getByLabelText(/Type the code/), { target: { value: code } });
  fireEvent.click(screen.getByRole("button", { name: "Receive share" }));
}

describe("EnterCode", () => {
  beforeEach(() => {
    receiveWithCode.mockReset();
  });

  it("opens the share the code delivered", async () => {
    receiveWithCode.mockResolvedValue(`${window.location.origin}/s#${SECRET}`);
    const onReceived = vi.fn();
    render(<EnterCode onReceived={onReceived} onCancel={vi.fn()} />);

    submit("7-aci-roc");

    await waitFor(() => expect(onReceived).toHaveBeenCalledWith(SECRET));
    expect(receiveWithCode).toHaveBeenCalledWith("7-aci-roc", expect.any(AbortSignal));
  });

  it("explains a code that didn't work and lets the user try again", async () => {
    receiveWithCode.mockImplementation(async () => {
      throw new Error("already used");
    });
    render(<EnterCode onReceived={vi.fn()} onCancel={vi.fn()} />);

    submit("7-acid-rocket");

    await screen.findByText("explained: already used");
    expect(
      (screen.getByRole("button", { name: "Receive share" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("refuses a delivered link for another site", async () => {
    receiveWithCode.mockResolvedValue(`https://other.example/s#${SECRET}`);
    const onReceived = vi.fn();
    render(<EnterCode onReceived={onReceived} onCancel={vi.fn()} />);

    submit("7-acid-rocket");

    await screen.findByText("The received link isn't a share on this site.");
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

    unmount();

    expect(signal?.aborted).toBe(true);
  });
});
