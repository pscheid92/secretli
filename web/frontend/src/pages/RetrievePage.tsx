import { useCallback, useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import SecretTypeIcon from "../components/SecretTypeIcon";
import Spinner from "../components/Spinner";
import TransferStatus from "../components/TransferStatus";
import {
  ApiError,
  deleteSecret,
  getSecretMetadata,
  retrieveSecret,
  retrieveSecretRange,
  type SecretMetadataResponse,
  startRetrievalSession,
} from "../lib/api";
import { type BundleManifest, decryptBundleFiles, readBundleManifest } from "../lib/bundle";
import { KeySet, type SecretMeta } from "../lib/encryption";
import { formatRelativeTime, formatSize } from "../lib/format";

interface DecryptedMeta {
  serverMeta: SecretMetadataResponse;
  clientMeta: SecretMeta;
}

type State =
  | { stage: "prompt" }
  | { stage: "loading" }
  | { stage: "confirm"; shareSecret: string; deletionToken: string; meta: DecryptedMeta }
  | { stage: "password"; shareSecret: string; deletionToken: string; meta: DecryptedMeta }
  | { stage: "decrypted"; text: string; shareSecret: string; deletionToken: string }
  | {
      stage: "bundle-ready";
      manifest: BundleManifest;
      keySet: KeySet;
      publicID: string;
      sessionToken: string;
      shareSecret: string;
      deletionToken: string;
    }
  | { stage: "deleted" }
  | { stage: "error"; message: string };

function MetaRow({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className="flex items-center justify-between py-2.5 text-sm">
      <span className="text-zinc-600 dark:text-zinc-100">{label}</span>
      <span
        className={
          accent
            ? "text-amber-600 dark:text-amber-400 font-medium"
            : "text-zinc-600 dark:text-zinc-100"
        }
      >
        {value}
      </span>
    </div>
  );
}

function manifestTotalSize(manifest: BundleManifest): number {
  return manifest.files.reduce((sum, file) => sum + file.size, 0);
}

export default function RetrievePage() {
  const hash = window.location.hash.slice(1);
  const [state, setState] = useState<State>(hash ? { stage: "loading" } : { stage: "prompt" });
  const [linkInput, setLinkInput] = useState("");
  const {
    register: registerPassword,
    handleSubmit: handlePasswordFormSubmit,
    formState: { errors: passwordErrors },
    setError: setPasswordFormError,
  } = useForm<{ password: string }>({ defaultValues: { password: "" } });
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [revealing, setRevealing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [downloadingBundle, setDownloadingBundle] = useState(false);

  const fetchMetadata = useCallback(async () => {
    const hash = window.location.hash.slice(1);
    if (!hash) {
      setState({ stage: "prompt" });
      return;
    }

    const delimiterIndex = hash.indexOf("!");
    const shareSecret = delimiterIndex >= 0 ? hash.slice(0, delimiterIndex) : hash;
    const deletionToken = delimiterIndex >= 0 ? hash.slice(delimiterIndex + 1) : "";

    try {
      const keySet = await KeySet.fromShareSecret(shareSecret);
      const encoded = keySet.getEncoded();

      const serverMeta = await getSecretMetadata(encoded.publicID, encoded.metadataToken);

      // Decrypt the client-encrypted metadata
      const clientMeta = await keySet.decryptMeta(serverMeta.encrypted_meta);

      setState({
        stage: "confirm",
        shareSecret,
        deletionToken,
        meta: { serverMeta, clientMeta },
      });
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 404) {
          setState({ stage: "error", message: "This share has expired or does not exist." });
        } else if (err.status === 403) {
          setState({ stage: "error", message: "Invalid metadata token." });
        } else {
          setState({ stage: "error", message: err.message });
        }
      } else {
        setState({ stage: "error", message: "An unexpected error occurred." });
      }
    }
  }, []);

  useEffect(() => {
    fetchMetadata();
  }, [fetchMetadata]);

  async function handleReveal() {
    if (state.stage !== "confirm") return;

    const { shareSecret, deletionToken, meta } = state;
    setRevealing(true);

    try {
      if (meta.clientMeta.password_protected) {
        setState({ stage: "password", shareSecret, deletionToken, meta });
        return;
      }

      const keySet = await KeySet.fromShareSecret(shareSecret);
      await revealWithKeySet(keySet, shareSecret, deletionToken, meta.clientMeta);
    } catch (err) {
      handleRevealError(err);
    } finally {
      setRevealing(false);
    }
  }

  async function handlePasswordSubmit(data: { password: string }) {
    if (state.stage !== "password") return;

    setPasswordLoading(true);

    try {
      const keySet = await KeySet.fromShareSecret(state.shareSecret, data.password);
      await revealWithKeySet(keySet, state.shareSecret, state.deletionToken, state.meta.clientMeta);
    } catch {
      setPasswordFormError("password", { message: "Wrong password. Please try again." });
    } finally {
      setPasswordLoading(false);
    }
  }

  async function revealWithKeySet(
    keySet: KeySet,
    shareSecret: string,
    deletionToken: string,
    clientMeta: SecretMeta,
  ) {
    // Public ID is always derived from the share secret; blob access may be password-derived.
    const baseKeySet = await KeySet.fromShareSecret(shareSecret);
    const baseEncoded = baseKeySet.getEncoded();
    const blobEncoded = keySet.getEncoded();

    if (clientMeta.type === "bundle") {
      const session = await startRetrievalSession(baseEncoded.publicID, blobEncoded.blobToken);
      const fetchRange = (start: number, end: number) =>
        retrieveSecretRange(baseEncoded.publicID, session.session_token, start, end);
      const { manifest } = await readBundleManifest(fetchRange, keySet, session.blob_size);
      setState({
        stage: "bundle-ready",
        manifest,
        keySet,
        publicID: baseEncoded.publicID,
        sessionToken: session.session_token,
        shareSecret,
        deletionToken,
      });
      return;
    }

    if (clientMeta.type !== "text") {
      setState({
        stage: "error",
        message: "This link uses an unsupported file format.",
      });
      return;
    }

    const response = await retrieveSecret(baseEncoded.publicID, blobEncoded.blobToken);
    const decrypted = await keySet.decryptBlob(response.blob);
    const text = new TextDecoder().decode(decrypted);
    setState({ stage: "decrypted", text, shareSecret, deletionToken });
  }

  function handleRevealError(err: unknown) {
    if (err instanceof ApiError) {
      if (err.status === 404) {
        setState({ stage: "error", message: "This share has expired or does not exist." });
      } else {
        setState({ stage: "error", message: err.message });
      }
    } else {
      setState({ stage: "error", message: "An unexpected error occurred." });
    }
  }

  async function handleDelete() {
    if (state.stage !== "decrypted" && state.stage !== "bundle-ready") {
      return;
    }
    if (!state.deletionToken) return;

    setDeleting(true);
    try {
      const keySet = await KeySet.fromShareSecret(state.shareSecret);
      const encoded = keySet.getEncoded();
      await deleteSecret(encoded.publicID, encoded.metadataToken, state.deletionToken);
      setState({ stage: "deleted" });
      toast.success("Share deleted");
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message);
      } else {
        toast.error("Failed to delete share.");
      }
    } finally {
      setDeleting(false);
    }
  }

  async function copyDecryptedText() {
    if (state.stage !== "decrypted") return;
    await navigator.clipboard.writeText(state.text);
    toast.success("Copied to clipboard");
  }

  // -- Prompt --

  if (state.stage === "prompt") {
    function handleLinkSubmit(e: React.FormEvent) {
      e.preventDefault();
      try {
        const url = new URL(linkInput.trim());
        const fragment = url.hash.slice(1);
        if (!fragment) {
          toast.error("That link doesn't contain a share key.");
          return;
        }
        window.location.href = `${window.location.pathname}#${fragment}`;
        window.location.reload();
      } catch {
        toast.error("Please enter a valid Secretli link.");
      }
    }

    return (
      <div className="mx-auto max-w-2xl space-y-6">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Open a Share
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            Paste a Secretli link to decrypt it in this browser.
          </p>
        </div>
        <form onSubmit={handleLinkSubmit} className="space-y-4">
          <input
            id="secret-link"
            type="text"
            value={linkInput}
            onChange={(e) => setLinkInput(e.target.value)}
            placeholder="https://secretli.example/s#..."
            autoFocus
            className="w-full rounded-lg border border-zinc-200 dark:border-zinc-500/50 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-4 py-3 text-sm font-mono placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
          />
          <button
            type="submit"
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 transition-all duration-150"
          >
            Open Share
          </button>
        </form>
      </div>
    );
  }

  // -- Loading --

  if (state.stage === "loading") {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-20">
        <Spinner size="lg" className="text-amber-400" />
        <p className="text-sm text-zinc-600 dark:text-zinc-100">Checking share...</p>
      </div>
    );
  }

  // -- Error --

  if (state.stage === "error") {
    return (
      <div className="space-y-5">
        <div className="rounded-lg border border-red-200 dark:border-red-900/40 bg-red-50 dark:bg-red-900/10 p-5">
          <div className="flex items-start gap-3">
            <svg
              className="h-4 w-4 text-red-500 flex-shrink-0 mt-0.5"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
              />
            </svg>
            <div>
              <p className="text-sm font-medium text-red-700 dark:text-red-400">
                Unable to open share
              </p>
              <p className="text-sm text-red-600 dark:text-red-500 mt-0.5">{state.message}</p>
            </div>
          </div>
        </div>
        <a
          href="/share"
          className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
        >
          ← Create a new share
        </a>
      </div>
    );
  }

  // -- Confirm --

  if (state.stage === "confirm") {
    const { serverMeta, clientMeta } = state.meta;
    const isBundle = clientMeta.type === "bundle";
    const revealLabel = clientMeta.password_protected
      ? "Unlock Share"
      : serverMeta.burn_after_read
        ? isBundle
          ? "Prepare Download & Burn"
          : "Reveal & Burn"
        : isBundle
          ? "Prepare Download"
          : "Reveal Text";

    return (
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start">
        <section className="space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
          <div>
            <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
              {isBundle ? "File Share" : "Text Share"}
            </h1>
            <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
              Review the details before this browser decrypts the content.
            </p>
          </div>

          <div className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 dark:divide-zinc-700 dark:border-zinc-700">
            <div className="flex items-center gap-2.5 px-4 py-3">
              <SecretTypeIcon
                type={clientMeta.type}
                className="h-4 w-4 text-zinc-500 dark:text-zinc-100"
              />
              <span className="text-sm font-medium text-zinc-700 dark:text-zinc-100">
                {isBundle ? "Files" : "Text"}
              </span>
            </div>
            <div className="px-4">
              <MetaRow label="Created" value={formatRelativeTime(serverMeta.created_at)} />
            </div>
            <div className="px-4">
              <MetaRow label="Expires" value={formatRelativeTime(serverMeta.expires_at)} />
            </div>
            {isBundle && serverMeta.blob_size > 0 && (
              <div className="px-4">
                <MetaRow label="Size" value={formatSize(serverMeta.blob_size)} />
              </div>
            )}
            {clientMeta.password_protected && (
              <div className="px-4">
                <MetaRow label="Password" value="Required" accent />
              </div>
            )}
          </div>

          {isBundle && (
            <p className="text-xs leading-relaxed text-zinc-500 dark:text-zinc-400">
              File names and sizes are hidden until the encrypted manifest is opened.
            </p>
          )}
        </section>

        <aside className="space-y-4 lg:sticky lg:top-24">
          {serverMeta.burn_after_read && (
            <div className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-900/40 dark:bg-amber-900/10">
              <svg
                className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
              <p className="text-xs text-amber-700 dark:text-amber-400">
                This share will be permanently consumed when reveal starts. If the download is
                interrupted after that, the link may not work again.
              </p>
            </div>
          )}
          <section className="rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
              Next step
            </h2>
            <button
              type="button"
              onClick={handleReveal}
              disabled={revealing}
              className="mt-4 flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-semibold text-zinc-950 transition-all duration-150 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {revealing && <Spinner size="sm" className="text-zinc-700" />}
              {revealing ? "Decrypting..." : revealLabel}
            </button>
          </section>
        </aside>
      </div>
    );
  }

  // -- Password --

  if (state.stage === "password") {
    const isBurnAfterRead = state.meta.serverMeta.burn_after_read;
    const isBundle = state.meta.clientMeta.type === "bundle";
    const submitLabel = isBurnAfterRead
      ? isBundle
        ? "Prepare Download & Burn"
        : "Reveal & Burn"
      : isBundle
        ? "Prepare Download"
        : "Reveal Text";

    return (
      <div className="mx-auto max-w-xl space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Unlock Share
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            Enter the password to decrypt the protected content.
          </p>
        </div>
        <form onSubmit={handlePasswordFormSubmit(handlePasswordSubmit)} className="space-y-4">
          <input
            type="password"
            {...registerPassword("password", { required: "Password is required" })}
            placeholder="Enter password..."
            autoFocus
            autoComplete="off"
            data-gramm="false"
            data-gramm_editor="false"
            data-enable-grammarly="false"
            data-1p-ignore
            className="w-full rounded-lg border border-zinc-200 dark:border-zinc-500/50 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-4 py-3 text-sm placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
          />
          {passwordErrors.password && (
            <p className="text-xs text-red-500 dark:text-red-400">
              {passwordErrors.password.message}
            </p>
          )}
          {isBurnAfterRead && (
            <div className="flex items-start gap-3 rounded-lg border border-amber-200 dark:border-amber-900/40 bg-amber-50 dark:bg-amber-900/10 px-4 py-3">
              <svg
                className="h-4 w-4 text-amber-500 flex-shrink-0 mt-0.5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
                />
              </svg>
              <p className="text-xs text-amber-700 dark:text-amber-400">
                Submitting the password starts retrieval and permanently consumes this share.
              </p>
            </div>
          )}
          <button
            type="submit"
            disabled={passwordLoading}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
          >
            {passwordLoading && <Spinner size="sm" className="text-zinc-700" />}
            {passwordLoading ? "Decrypting..." : submitLabel}
          </button>
        </form>
      </div>
    );
  }

  // -- Deleted --

  if (state.stage === "deleted") {
    return (
      <div className="space-y-5">
        <div className="flex items-center gap-2.5">
          <div className="w-2 h-2 rounded-full bg-emerald-400" />
          <span className="text-sm font-medium text-zinc-600 dark:text-zinc-100">
            Share deleted
          </span>
        </div>
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 px-4 py-4">
          <p className="text-sm text-zinc-600 dark:text-zinc-100">
            The share has been permanently destroyed.
          </p>
        </div>
        <a
          href="/"
          className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
        >
          ← Create a new share
        </a>
      </div>
    );
  }

  // -- Bundle ready --

  if (state.stage === "bundle-ready") {
    const { keySet, manifest, publicID, sessionToken } = state;
    const isMulti = manifest.files.length > 1;
    const totalSize = manifestTotalSize(manifest);

    function saveBlob(blob: Blob, name: string) {
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = name;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    }

    async function downloadAll() {
      setDownloadingBundle(true);
      try {
        const fetchRange = (start: number, end: number) =>
          retrieveSecretRange(publicID, sessionToken, start, end);
        for (const { file, blob } of await decryptBundleFiles(manifest.files, keySet, fetchRange, {
          maxCoalescedPlaintextBytes: totalSize,
        })) {
          saveBlob(blob, file.name);
        }
      } catch {
        toast.error("Failed to download files.");
      } finally {
        setDownloadingBundle(false);
      }
    }

    return (
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start">
        <section className="space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
          <div>
            <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
              {isMulti ? "Download Files" : "Download File"}
            </h1>
            <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
              {manifest.files.length} {isMulti ? "files" : "file"} · {formatSize(totalSize)}
            </p>
          </div>

          <div className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 dark:divide-zinc-700 dark:border-zinc-700">
            {manifest.files.map((file) => (
              <div
                key={`${file.index}-${file.path}`}
                data-testid={`bundle-file-${file.index}`}
                className="grid grid-cols-[1fr_auto] items-center gap-4 px-4 py-3"
              >
                <span className="min-w-0 truncate font-mono text-sm font-medium text-zinc-700 dark:text-zinc-100">
                  {file.path}
                </span>
                <span className="text-xs tabular-nums text-zinc-500 dark:text-zinc-400">
                  {formatSize(file.size)}
                </span>
              </div>
            ))}
          </div>
        </section>

        <aside className="space-y-4 lg:sticky lg:top-24">
          <section className="rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
              Bundle
            </h2>
            <div className="mt-2 divide-y divide-zinc-200 dark:divide-zinc-700">
              <div className="py-3">
                <div className="text-xs text-zinc-500 dark:text-zinc-400">Name</div>
                <div className="mt-1 truncate text-sm font-medium text-zinc-900 dark:text-zinc-100">
                  {manifest.bundleName}
                </div>
              </div>
              <div className="py-3">
                <div className="text-xs text-zinc-500 dark:text-zinc-400">Files</div>
                <div className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                  {manifest.files.length}
                </div>
              </div>
              <div className="py-3">
                <div className="text-xs text-zinc-500 dark:text-zinc-400">Total size</div>
                <div className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                  {formatSize(totalSize)}
                </div>
              </div>
            </div>
          </section>

          {downloadingBundle && (
            <TransferStatus
              title={isMulti ? "Preparing files" : "Preparing file"}
              steps={[
                { label: "Reading encrypted data", state: "active" },
                { label: "Decrypting", state: "pending" },
                { label: "Saving", state: "pending" },
              ]}
            />
          )}
          {isMulti && (
            <p className="text-xs leading-relaxed text-zinc-500 dark:text-zinc-400">
              Your browser will save {manifest.files.length} files.
            </p>
          )}
          <button
            type="button"
            onClick={downloadAll}
            disabled={downloadingBundle}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-semibold text-zinc-950 transition-all duration-150 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {downloadingBundle && <Spinner size="sm" className="text-zinc-700" />}
            {downloadingBundle ? "Preparing..." : isMulti ? "Download Files" : "Download File"}
          </button>
          {state.deletionToken && (
            <button
              type="button"
              onClick={handleDelete}
              disabled={deleting}
              className="flex items-center gap-2 rounded-lg border border-red-200 px-4 py-2.5 text-sm text-red-600 transition-all duration-150 hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-red-500/20 disabled:cursor-not-allowed disabled:opacity-40 dark:border-red-900/40 dark:text-red-400 dark:hover:bg-red-900/10"
            >
              {deleting && <Spinner size="sm" className="text-red-500" />}
              {deleting ? "Deleting..." : "Delete share"}
            </button>
          )}
        </aside>
      </div>
    );
  }

  // -- Decrypted text --

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start">
      <section className="space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Decrypted Text
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            Decrypted in this browser. Copy the content below.
          </p>
        </div>

        <div className="overflow-hidden rounded-lg border border-zinc-200 dark:border-zinc-700">
          <div className="flex items-center justify-between border-b border-zinc-200 bg-zinc-50 px-4 py-2 dark:border-zinc-700 dark:bg-zinc-950">
            <span className="text-xs uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
              Plaintext
            </span>
            <button
              type="button"
              onClick={copyDecryptedText}
              className="text-xs font-semibold text-amber-600 transition-colors duration-150 hover:text-amber-500 dark:text-amber-400 dark:hover:text-amber-300"
            >
              Copy
            </button>
          </div>
          <pre className="min-h-40 whitespace-pre-wrap break-words bg-white px-4 py-4 text-sm leading-relaxed text-zinc-800 dark:bg-zinc-950 dark:text-zinc-100">
            {(state as { stage: "decrypted"; text: string }).text}
          </pre>
        </div>
      </section>

      {(state as { stage: "decrypted"; deletionToken: string }).deletionToken && (
        <aside className="lg:sticky lg:top-24">
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            className="flex items-center gap-2 rounded-lg border border-red-200 px-4 py-2.5 text-sm text-red-600 transition-all duration-150 hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-red-500/20 disabled:cursor-not-allowed disabled:opacity-40 dark:border-red-900/40 dark:text-red-400 dark:hover:bg-red-900/10"
          >
            {deleting && <Spinner size="sm" className="text-red-500" />}
            {deleting ? "Deleting..." : "Delete share"}
          </button>
        </aside>
      )}
    </div>
  );
}
