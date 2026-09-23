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

/**
 * Publishes a text share on a fake server: metadata and blob are really
 * encrypted, so the page decrypts them exactly as it would in production.
 */
async function publishTextShare(options: { burnAfterRead?: boolean; password?: string } = {}) {
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

  window.location.hash = `#${shareSecret}`;
}

/** Makes the next range read fail the way a range read does after its retries. */
function failNextRangeRead(error: Error) {
  api.retrieveSecretRange.mockImplementationOnce(async () => {
    throw error;
  });
}

async function openPasswordPrompt() {
  fireEvent.click(await screen.findByRole("button", { name: "Unlock Share" }));
}

async function submitPassword(password: string) {
  const input = await screen.findByPlaceholderText("Enter password...");
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

    fireEvent.click(await screen.findByRole("button", { name: "Reveal & Burn" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());

    // The page stays put, and trying again reuses the session that burned the
    // share instead of starting one the server would refuse.
    fireEvent.click(await screen.findByRole("button", { name: "Reveal & Burn" }));
    expect(await screen.findByText(SECRET_TEXT)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(1);
  });

  it("starts a new session once the kept one has expired", async () => {
    await publishTextShare();
    failNextRangeRead(new ApiError(403, "invalid retrieval session"));
    render(<RetrievePage />);

    fireEvent.click(await screen.findByRole("button", { name: "Reveal Text" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The download window has expired. Please try again.",
      ),
    );

    fireEvent.click(await screen.findByRole("button", { name: "Reveal Text" }));
    expect(await screen.findByText(SECRET_TEXT)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(2);
  });

  it("reports a wrong password only when the server rejects the blob token", async () => {
    await publishTextShare({ password: "correct horse" });
    render(<RetrievePage />);

    await openPasswordPrompt();
    await submitPassword("wrong horse");
    expect(await screen.findByText("Wrong password. Please try again.")).toBeTruthy();
    expect(api.retrieveSecretRange).not.toHaveBeenCalled();
  });

  it("does not blame the password when reading fails after it was accepted", async () => {
    await publishTextShare({ burnAfterRead: true, password: "correct horse" });
    failNextRangeRead(new ApiError(0, "Network error — please check your connection"));
    render(<RetrievePage />);

    await openPasswordPrompt();
    await submitPassword("correct horse");
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Network error — please check your connection"),
    );
    expect(screen.queryByText("Wrong password. Please try again.")).toBeNull();

    await submitPassword("correct horse");
    expect(await screen.findByText(SECRET_TEXT)).toBeTruthy();
    expect(api.startRetrievalSession).toHaveBeenCalledTimes(1);
  });
});
