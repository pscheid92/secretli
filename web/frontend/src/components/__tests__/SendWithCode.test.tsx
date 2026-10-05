import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import SendWithCode from "../SendWithCode";

const startSending = vi.fn();
vi.mock("../../lib/transferSession", () => ({
  startSending: (link: string) => startSending(link),
  describeSendError: () => "The code didn't match. Start again for a new code.",
}));

function deferred() {
  let resolve: () => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  promise.catch(() => {});
  return { promise, resolve, reject };
}

function startsWith(done: Promise<void>, cancel = vi.fn()) {
  startSending.mockResolvedValueOnce({
    code: "7-acid-rocket",
    expiresAt: Date.now() + 10 * 60_000,
    done,
    cancel,
  });
  return cancel;
}

describe("SendWithCode", () => {
  beforeEach(() => {
    startSending.mockReset();
  });

  it("shows the code for the link and reports a delivered transfer", async () => {
    const done = deferred();
    startsWith(done.promise);

    render(<SendWithCode url="https://secretli.example/s#key" onClose={vi.fn()} />);

    await screen.findByText("7-acid-rocket");
    expect(startSending).toHaveBeenCalledWith("https://secretli.example/s#key");
    // The short address that opens code entry on the other device.
    expect(screen.getByText(`${window.location.host}/c`)).toBeTruthy();
    expect(screen.getByText(/expires in 10:00/)).toBeTruthy();

    await act(async () => done.resolve());
    await screen.findByText("Sent. The other device is opening the share.");
  });

  it("explains a failure and starts again with a new code", async () => {
    const done = deferred();
    startsWith(done.promise);
    render(<SendWithCode url="https://secretli.example/s#key" onClose={vi.fn()} />);
    await screen.findByText("7-acid-rocket");

    await act(async () => done.reject(new Error("mismatch")));
    await screen.findByText("The code didn't match. Start again for a new code.");

    startsWith(new Promise(() => {}));
    fireEvent.click(screen.getByRole("button", { name: "New code" }));
    await waitFor(() => expect(startSending).toHaveBeenCalledTimes(2));
  });

  it("ends the transfer when it is closed", async () => {
    const cancel = startsWith(new Promise(() => {}));
    const onClose = vi.fn();
    const { unmount } = render(
      <SendWithCode url="https://secretli.example/s#key" onClose={onClose} />,
    );
    await screen.findByText("7-acid-rocket");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
    unmount();

    expect(cancel).toHaveBeenCalled();
  });

  it("ends the transfer when the page is left", async () => {
    const cancel = startsWith(new Promise(() => {}));
    render(<SendWithCode url="https://secretli.example/s#key" onClose={vi.fn()} />);
    await screen.findByText("7-acid-rocket");

    window.dispatchEvent(new Event("pagehide"));

    expect(cancel).toHaveBeenCalled();
  });
});
