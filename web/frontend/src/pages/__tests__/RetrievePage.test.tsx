import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { toast } from "sonner";
import { createEncryptedBundle } from "../../lib/__tests__/bundleFixture";
import { ApiError, type RetrievalSessionResponse } from "../../lib/api";
import { KeySet } from "../../lib/encryption";
import RetrievePage from "../RetrievePage";

const api = vi.hoisted(() => ({
  getSecretMetadata: vi.fn(),
  startRetrievalSession: vi.fn(),
  retrieveSecretRange: vi.fn(),
}));

vi.mock("../../lib/api", async (importOriginal) => {
  const original = await importOriginal<typeof import("../../lib/api")>();
  return { ...original, ...api };
});

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const SECRET_TEXT = "the launch code is 0000";

// The page derives the password key with scrypt on the main thread, which is
// slow on CI runners; waits that follow a password submit get more time.
const AFTER_PASSWORD = { timeout: 15_000 };

/**
 * Publishes a text share on a fake server: metadata and blob are really
 * encrypted, so the page decrypts them exactly as it would in production.
 */
async function publishTextShare(
  options: {
    burnAfterRead?: boolean;
    password?: string;
    ownerLink?: boolean;
    openedAt?: string;
  } = {},
) {
  const baseKeySet = await KeySet.generateRandom();
  const shareSecret = baseKeySet.getEncoded().shareSecret;
  const blobKeySet = options.password
    ? await KeySet.fromShareSecret(shareSecret, options.password)
    : baseKeySet;
  const { blob } = await createEncryptedBundle(
    [new File([SECRET_TEXT], "secret.txt", { type: "text/plain" })],
    blobKeySet,
  );
  const bytes = new Uint8Array(await blob.arrayBuffer());

  api.getSecretMetadata.mockResolvedValue({
    encrypted_meta: await baseKeySet.encryptMeta({
      type: "text",
      password_protected: options.password !== undefined,
    }),
    blob_size: bytes.length,
    burn_after_read: options.burnAfterRead ?? false,
    expires_at: new Date(Date.now() + 3600_000).toISOString(),
    created_at: new Date().toISOString(),
    ...(options.openedAt ? { opened_at: options.openedAt } : {}),
  });
  api.startRetrievalSession.mockImplementation(
    async (_publicID: string, blobToken: string): Promise<RetrievalSessionResponse> => {
      if (blobToken !== blobKeySet.getEncoded().blobToken) {
        throw new ApiError(403, "invalid blob token");
      }
      return {
        session_token: "session-token",
        blob_size: bytes.length,
        expires_at: new Date(Date.now() + 900_000).toISOString(),
        burn_after_read: options.burnAfterRead ?? false,
      };
    },
  );
  api.retrieveSecretRange.mockImplementation(async (_id, _token, start: number, end: number) =>
    bytes.slice(start, end + 1),
  );

  // An owner link carries the 43-character deletion token after "!".
  window.location.hash = options.ownerLink
    ? `#${shareSecret}!${"D".repeat(43)}`
    : `#${shareSecret}`;
}

/** Makes the next range read fail the way a range read does after its retries. */
function failNextRangeRead(error: Error) {
  api.retrieveSecretRange.mockImplementationOnce(async () => {
    throw error;
  });
}

async function submitPassword(password: string) {
  const input = await screen.findByLabelText("Password");
  fireEvent.change(input, { target: { value: password } });
  fireEvent.submit(input.closest("form") as HTMLFormElement);
}

describe("RetrievePage", () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset();
    vi.mocked(toast.error).mockClear();
  });

  it("retries a burn-after-read share with the session it already started", async () => {
    await publishTextShare({ burnAfterRead: true });
    failNextRangeRead(new ApiError(503, "storage unavailable"));
    render(<RetrievePage />);

    fireEvent.click(await screen.findByRole("button", { name: "Reveal secret" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());

    // The page stays put, and trying again reuses the session that burned the
    // share instead of starting one the server would refuse.
    fireEvent.click(await screen.findByRole("button", { name: "Reveal secret" }));
    expect(await screen.findByText(SECRET_TEXT)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(1);
  });

  it("starts a new session once the kept one has expired", async () => {
    await publishTextShare();
    failNextRangeRead(new ApiError(403, "invalid retrieval session"));
    render(<RetrievePage />);

    fireEvent.click(await screen.findByRole("button", { name: "Reveal secret" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The download window has expired. Please try again.",
      ),
    );

    fireEvent.click(await screen.findByRole("button", { name: "Reveal secret" }));
    expect(await screen.findByText(SECRET_TEXT)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(2);
  });

  it("reports a wrong password only when the server rejects the blob token", async () => {
    await publishTextShare({ password: "correct horse" });
    render(<RetrievePage />);

    await submitPassword("wrong horse");
    expect(await screen.findByText("Wrong password. Try again.", {}, AFTER_PASSWORD)).toBeTruthy();
    expect(api.retrieveSecretRange).not.toHaveBeenCalled();

    // Selected, so typing again replaces the mistyped password.
    const input = screen.getByLabelText("Password") as HTMLInputElement;
    await waitFor(() => expect(document.activeElement).toBe(input));
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, "wrong horse".length]);
  }, 30_000);

  it("does not blame the password when reading fails after it was accepted", async () => {
    await publishTextShare({ burnAfterRead: true, password: "correct horse" });
    failNextRangeRead(new ApiError(0, "Network error — please check your connection"));
    render(<RetrievePage />);

    await submitPassword("correct horse");
    await waitFor(
      () =>
        expect(toast.error).toHaveBeenCalledWith("Network error — please check your connection"),
      AFTER_PASSWORD,
    );
    expect(screen.queryByText("Wrong password. Try again.")).toBeNull();

    await submitPassword("correct horse");
    expect(await screen.findByText(SECRET_TEXT, {}, AFTER_PASSWORD)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(1);
  }, 45_000);

  it("keeps the accepted session when a mistyped password follows it", async () => {
    await publishTextShare({ burnAfterRead: true, password: "correct horse" });
    failNextRangeRead(new ApiError(0, "Network error — please check your connection"));
    render(<RetrievePage />);

    await submitPassword("correct horse");
    await waitFor(() => expect(toast.error).toHaveBeenCalled(), AFTER_PASSWORD);

    // The burned share would answer a new session with 404; the typo must
    // neither reach the server nor throw the kept session away.
    await submitPassword("correct hose");
    expect(await screen.findByText("Wrong password. Try again.", {}, AFTER_PASSWORD)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(1);

    await submitPassword("correct horse");
    expect(await screen.findByText(SECRET_TEXT, {}, AFTER_PASSWORD)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(1);
  }, 60_000);

  it("warns owners that revealing their one-time share takes it from the recipient", async () => {
    await publishTextShare({ burnAfterRead: true, ownerLink: true });
    render(<RetrievePage />);

    expect(await screen.findByText(/This is your owner link/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete it now" })).toBeTruthy();
  });

  it("gives recipients the usual one-time warning", async () => {
    await publishTextShare({ burnAfterRead: true });
    render(<RetrievePage />);

    expect(await screen.findByText(/It opens once, then it's gone/)).toBeTruthy();
    expect(screen.queryByText(/This is your owner link/)).toBeNull();
  });

  it("says a link is damaged instead of asking the server about it", async () => {
    // Cut off while copying: one character short.
    window.location.hash = `#${"A".repeat(42)}`;
    render(<RetrievePage />);

    expect(
      await screen.findByText(
        "This link is incomplete or damaged. Check that you copied all of it.",
      ),
    ).toBeTruthy();
    expect(api.getSecretMetadata).not.toHaveBeenCalled();
  });

  it("names every reason a share can be gone, including a one-time share already opened", async () => {
    await publishTextShare();
    api.getSecretMetadata.mockImplementation(async () => {
      throw new ApiError(404, "secret not found");
    });
    render(<RetrievePage />);

    expect(await screen.findByRole("heading", { name: "This secret is gone" })).toBeTruthy();
    expect(screen.getByText(/It was opened already, or it expired\./)).toBeTruthy();
  });

  it("ends on an error page when a burn-after-read session expires", async () => {
    await publishTextShare({ burnAfterRead: true });
    failNextRangeRead(new ApiError(403, "invalid retrieval session"));
    render(<RetrievePage />);

    fireEvent.click(await screen.findByRole("button", { name: "Reveal secret" }));

    // Trying again could only get a 404, so do not invite it.
    expect(await screen.findByRole("heading", { name: "The download window closed" })).toBeTruthy();
    expect(toast.error).not.toHaveBeenCalled();
  });

  /** The 410 the metadata endpoint answers with once the server only remembers a secret. */
  function gone(details: Record<string, unknown>) {
    return new ApiError(410, "secret is gone", undefined, undefined, details);
  }

  async function openLink(ownerLink: boolean) {
    const shareSecret = (await KeySet.generateRandom()).getEncoded().shareSecret;
    window.location.hash = ownerLink ? `#${shareSecret}!${"D".repeat(43)}` : `#${shareSecret}`;
  }

  it("tells the owner when their one-time secret was opened", async () => {
    const openedAt = new Date().toISOString();
    api.getSecretMetadata.mockRejectedValue(
      gone({
        outcome: "opened",
        burn_after_read: true,
        ended_at: openedAt,
        first_opened_at: openedAt,
        opened_by_owner: false,
      }),
    );
    await openLink(true);
    render(<RetrievePage />);

    expect(await screen.findByRole("heading", { name: "Your secret was opened" })).toBeTruthy();
    expect(screen.getByText(/^Opened today at /)).toBeTruthy();
  });

  it("warns a recipient when someone already opened the one-time secret", async () => {
    const openedAt = new Date().toISOString();
    api.getSecretMetadata.mockRejectedValue(
      gone({
        outcome: "opened",
        burn_after_read: true,
        ended_at: openedAt,
        first_opened_at: openedAt,
        opened_by_owner: false,
      }),
    );
    await openLink(false);
    render(<RetrievePage />);

    expect(
      await screen.findByRole("heading", { name: "This secret was already opened" }),
    ).toBeTruthy();
    expect(screen.getByText(/tell the sender/)).toBeTruthy();
  });

  it("tells the owner their secret expired unopened", async () => {
    api.getSecretMetadata.mockRejectedValue(
      gone({
        outcome: "expired",
        burn_after_read: true,
        ended_at: new Date().toISOString(),
        opened_by_owner: false,
      }),
    );
    await openLink(true);
    render(<RetrievePage />);

    expect(
      await screen.findByRole("heading", { name: "Your secret expired unopened" }),
    ).toBeTruthy();
  });

  it("tells a recipient that the sender deleted the secret", async () => {
    api.getSecretMetadata.mockRejectedValue(
      gone({
        outcome: "deleted",
        burn_after_read: false,
        ended_at: new Date().toISOString(),
        opened_by_owner: false,
      }),
    );
    await openLink(false);
    render(<RetrievePage />);

    expect(await screen.findByRole("heading", { name: "This secret was deleted" })).toBeTruthy();
    expect(screen.getByText(/^The sender deleted it today at /)).toBeTruthy();
  });

  it("finds out what happened when the secret goes away between showing and opening it", async () => {
    await publishTextShare({ burnAfterRead: true });
    render(<RetrievePage />);
    await screen.findByRole("button", { name: "Reveal secret" });
    const openedAt = new Date().toISOString();
    api.startRetrievalSession.mockRejectedValue(new ApiError(404, "secret not found"));
    api.getSecretMetadata.mockRejectedValueOnce(
      gone({
        outcome: "opened",
        burn_after_read: true,
        ended_at: openedAt,
        first_opened_at: openedAt,
        opened_by_owner: false,
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Reveal secret" }));

    expect(
      await screen.findByRole("heading", { name: "This secret was already opened" }),
    ).toBeTruthy();
  });

  it("tells the owner of a reusable secret when it was first opened", async () => {
    await publishTextShare({ ownerLink: true, openedAt: new Date().toISOString() });
    render(<RetrievePage />);

    expect(await screen.findByText(/It was first opened today at /)).toBeTruthy();
  });

  it("sends the deletion token along when the owner opens their own secret", async () => {
    await publishTextShare({ ownerLink: true });
    render(<RetrievePage />);

    fireEvent.click(await screen.findByRole("button", { name: "Reveal secret" }));

    expect(await screen.findByText(SECRET_TEXT)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledWith(
      expect.any(String),
      expect.any(String),
      "D".repeat(43),
    );
  });
});
