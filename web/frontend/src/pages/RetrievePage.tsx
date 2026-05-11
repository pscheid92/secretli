import { useCallback, useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import SecretTypeIcon from "../components/SecretTypeIcon";
import Spinner from "../components/Spinner";
import {
  ApiError,
  deleteSecret,
  getSecretMetadata,
  retrieveSecret,
  retrieveSecretRange,
  type SecretMetadataResponse,
  startRetrievalSession,
} from "../lib/api";
import {
  type BundleFile,
  type BundleManifest,
  DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES,
  decryptBundleFile,
  decryptBundleFiles,
  readBundleManifest,
} from "../lib/bundle";
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
  | { stage: "downloading" }
  | {
      stage: "file-ready";
      files: Array<{ name: string; blob: Blob }>;
      shareSecret: string;
      deletionToken: string;
    }
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
          setState({ stage: "error", message: "This secret has expired or does not exist." });
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

    const response = await retrieveSecret(baseEncoded.publicID, blobEncoded.blobToken);
    const decrypted = await keySet.decryptBlob(response.blob);

    if (clientMeta.type === "file") {
      const filename = clientMeta.filename ?? "download";
      const rawBuffer = decrypted.buffer.slice(
        decrypted.byteOffset,
        decrypted.byteOffset + decrypted.byteLength,
      ) as ArrayBuffer;

      if (filename === "multiple.zip") {
        const JSZip = (await import("jszip")).default;
        const zip = await JSZip.loadAsync(rawBuffer);
        const files: Array<{ name: string; blob: Blob }> = [];
        for (const [name, entry] of Object.entries(zip.files)) {
          if (entry.dir) continue;
          const data = await entry.async("blob");
          files.push({ name, blob: data });
        }
        setState({ stage: "file-ready", files, shareSecret, deletionToken });
      } else {
        const blob = new Blob([rawBuffer]);
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);

        setState({
          stage: "file-ready",
          files: [{ name: filename, blob }],
          shareSecret,
          deletionToken,
        });
      }
    } else {
      const text = new TextDecoder().decode(decrypted);
      setState({ stage: "decrypted", text, shareSecret, deletionToken });
    }
  }

  function handleRevealError(err: unknown) {
    if (err instanceof ApiError) {
      if (err.status === 404) {
        setState({ stage: "error", message: "This secret has expired or does not exist." });
      } else {
        setState({ stage: "error", message: err.message });
      }
    } else {
      setState({ stage: "error", message: "An unexpected error occurred." });
    }
  }

  async function handleDelete() {
    if (
      state.stage !== "decrypted" &&
      state.stage !== "file-ready" &&
      state.stage !== "bundle-ready"
    ) {
      return;
    }
    if (!state.deletionToken) return;

    setDeleting(true);
    try {
      const keySet = await KeySet.fromShareSecret(state.shareSecret);
      const encoded = keySet.getEncoded();
      await deleteSecret(encoded.publicID, encoded.metadataToken, state.deletionToken);
      setState({ stage: "deleted" });
      toast.success("Secret deleted");
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message);
      } else {
        toast.error("Failed to delete secret.");
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
          toast.error("That link doesn't contain a secret key.");
          return;
        }
        window.location.href = `${window.location.pathname}#${fragment}`;
        window.location.reload();
      } catch {
        toast.error("Please enter a valid Secretli link.");
      }
    }

    return (
      <div className="space-y-6">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Retrieve a Secret
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            Paste the link you received to decrypt it.
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
            Retrieve Secret
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
        <p className="text-sm text-zinc-600 dark:text-zinc-100">Loading...</p>
      </div>
    );
  }

  // -- Downloading --

  if (state.stage === "downloading") {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-20">
        <Spinner size="lg" className="text-amber-400" />
        <p className="text-sm text-zinc-600 dark:text-zinc-100">Downloading and decrypting...</p>
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
                Unable to retrieve secret
              </p>
              <p className="text-sm text-red-600 dark:text-red-500 mt-0.5">{state.message}</p>
            </div>
          </div>
        </div>
        <a
          href="/share"
          className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
        >
          ← Share a new secret
        </a>
      </div>
    );
  }

  // -- Confirm --

  if (state.stage === "confirm") {
    const { serverMeta, clientMeta } = state.meta;
    const isFile = clientMeta.type === "file" || clientMeta.type === "bundle";
    const revealLabel = clientMeta.password_protected
      ? "Enter Password"
      : serverMeta.burn_after_read
        ? isFile
          ? "Download & Burn"
          : "Reveal & Burn"
        : isFile
          ? "Download & Decrypt"
          : "Reveal Secret";

    return (
      <div className="space-y-5">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Secret Ready
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            Review the details, then reveal.
          </p>
        </div>

        <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 divide-y divide-zinc-200 dark:divide-zinc-500/50 overflow-hidden">
          <div className="flex items-center gap-2.5 px-4 py-3">
            <SecretTypeIcon
              type={clientMeta.type}
              className="h-4 w-4 text-zinc-500 dark:text-zinc-100"
            />
            <span className="text-sm font-medium text-zinc-600 dark:text-zinc-100">
              {clientMeta.type === "bundle" ? "File bundle" : isFile ? "File" : "Text"} secret
            </span>
          </div>
          <div className="px-4">
            <MetaRow label="Created" value={formatRelativeTime(serverMeta.created_at)} />
          </div>
          <div className="px-4">
            <MetaRow label="Expires" value={formatRelativeTime(serverMeta.expires_at)} />
          </div>
          {isFile && serverMeta.blob_size > 0 && (
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

        {serverMeta.burn_after_read && (
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
              This secret will be permanently consumed when reveal starts. If the download is
              interrupted after that, the link may not work again.
            </p>
          </div>
        )}

        <button
          type="button"
          onClick={handleReveal}
          disabled={revealing}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
        >
          {revealing && <Spinner size="sm" className="text-zinc-700" />}
          {revealing ? "Decrypting..." : revealLabel}
        </button>
      </div>
    );
  }

  // -- Password --

  if (state.stage === "password") {
    const isBurnAfterRead = state.meta.serverMeta.burn_after_read;
    const isFile = state.meta.clientMeta.type === "file" || state.meta.clientMeta.type === "bundle";
    const submitLabel = isBurnAfterRead
      ? isFile
        ? "Download & Burn"
        : "Reveal & Burn"
      : "Decrypt";

    return (
      <div className="space-y-5">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Password Required
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            This secret is password-protected.
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
                Submitting the password starts retrieval and permanently consumes this secret.
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
            Secret deleted
          </span>
        </div>
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 px-4 py-4">
          <p className="text-sm text-zinc-600 dark:text-zinc-100">
            The secret has been permanently destroyed.
          </p>
        </div>
        <a
          href="/"
          className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
        >
          ← Share a new secret
        </a>
      </div>
    );
  }

  // -- Bundle ready --

  if (state.stage === "bundle-ready") {
    const { keySet, manifest, publicID, sessionToken } = state;
    const isMulti = manifest.files.length > 1;

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

    async function downloadFile(file: BundleFile) {
      setDownloadingBundle(true);
      try {
        const fetchRange = (start: number, end: number) =>
          retrieveSecretRange(publicID, sessionToken, start, end);
        const blob = await decryptBundleFile(file, keySet, fetchRange);
        saveBlob(blob, file.name);
      } catch {
        toast.error("Failed to download file.");
      } finally {
        setDownloadingBundle(false);
      }
    }

    async function downloadAll() {
      setDownloadingBundle(true);
      try {
        const fetchRange = (start: number, end: number) =>
          retrieveSecretRange(publicID, sessionToken, start, end);
        for (const { file, blob } of await decryptBundleFiles(manifest.files, keySet, fetchRange, {
          maxCoalescedPlaintextBytes: DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES,
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
      <div className="space-y-5">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            {isMulti ? "Files Ready" : "File Ready"}
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">{manifest.bundleName}</p>
        </div>
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 divide-y divide-zinc-200 dark:divide-zinc-500/50 overflow-hidden">
          {manifest.files.map((file) => (
            <div
              key={`${file.index}-${file.path}`}
              data-testid={`bundle-file-${file.index}`}
              className="flex items-center gap-3 px-4 py-3"
            >
              <div className="w-2 h-2 rounded-full bg-emerald-400 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <span className="text-sm font-medium text-zinc-600 dark:text-zinc-100 font-mono truncate block">
                  {file.path}
                </span>
                <span className="text-xs text-zinc-500 dark:text-zinc-400">
                  {formatSize(file.size)}
                </span>
              </div>
              <button
                type="button"
                onClick={() => downloadFile(file)}
                disabled={downloadingBundle}
                className="flex-shrink-0 text-xs font-medium text-amber-600 dark:text-amber-400 hover:text-amber-500 dark:hover:text-amber-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors duration-150"
              >
                Download
              </button>
            </div>
          ))}
        </div>
        {isMulti && (
          <button
            type="button"
            onClick={downloadAll}
            disabled={downloadingBundle}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
          >
            {downloadingBundle && <Spinner size="sm" className="text-zinc-700" />}
            {downloadingBundle ? "Downloading..." : "Download All"}
          </button>
        )}
        {state.deletionToken && (
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            className="flex items-center gap-2 rounded-lg border border-red-200 dark:border-red-900/40 px-4 py-2.5 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/10 focus:outline-none focus:ring-2 focus:ring-red-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
          >
            {deleting && <Spinner size="sm" className="text-red-500" />}
            {deleting ? "Deleting..." : "Delete this secret"}
          </button>
        )}
      </div>
    );
  }

  // -- File ready --

  if (state.stage === "file-ready") {
    const { files } = state;
    const isMulti = files.length > 1;

    function downloadFile(file: { name: string; blob: Blob }) {
      const url = URL.createObjectURL(file.blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = file.name;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    }

    function downloadAll() {
      for (const file of files) {
        downloadFile(file);
      }
    }

    return (
      <div className="space-y-5">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            {isMulti ? "Files Decrypted" : "File Downloaded"}
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            {isMulti
              ? "Decrypted successfully. Download individual files below."
              : "Decrypted and saved to your downloads."}
          </p>
        </div>
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 divide-y divide-zinc-200 dark:divide-zinc-500/50 overflow-hidden">
          {state.files.map((file) => (
            <div key={file.name} className="flex items-center gap-3 px-4 py-3">
              <div className="w-2 h-2 rounded-full bg-emerald-400 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <span className="text-sm font-medium text-zinc-600 dark:text-zinc-100 font-mono truncate block">
                  {file.name}
                </span>
                <span className="text-xs text-zinc-500 dark:text-zinc-400">
                  {formatSize(file.blob.size)}
                </span>
              </div>
              <button
                type="button"
                onClick={() => downloadFile(file)}
                className="flex-shrink-0 text-xs font-medium text-amber-600 dark:text-amber-400 hover:text-amber-500 dark:hover:text-amber-300 transition-colors duration-150"
              >
                Download
              </button>
            </div>
          ))}
        </div>
        {isMulti && (
          <button
            type="button"
            onClick={downloadAll}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 transition-all duration-150"
          >
            Download All
          </button>
        )}
        {state.deletionToken && (
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            className="flex items-center gap-2 rounded-lg border border-red-200 dark:border-red-900/40 px-4 py-2.5 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/10 focus:outline-none focus:ring-2 focus:ring-red-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
          >
            {deleting && <Spinner size="sm" className="text-red-500" />}
            {deleting ? "Deleting..." : "Delete this secret"}
          </button>
        )}
      </div>
    );
  }

  // -- Decrypted text --

  return (
    <div className="space-y-5">
      <div>
        <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
          Secret
        </h1>
        <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
          Decrypted successfully. Copy the contents below.
        </p>
      </div>

      <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 overflow-hidden">
        <div className="flex items-center justify-between px-4 py-2 bg-zinc-50 dark:bg-zinc-900/50 border-b border-zinc-200 dark:border-zinc-500/50">
          <span className="text-xs tracking-widest uppercase text-zinc-500 dark:text-zinc-100">
            Plaintext
          </span>
          <button
            type="button"
            onClick={copyDecryptedText}
            className="text-xs font-medium text-amber-600 dark:text-amber-400 hover:text-amber-500 dark:hover:text-amber-300 transition-colors duration-150"
          >
            Copy
          </button>
        </div>
        <pre className="px-4 py-4 whitespace-pre-wrap break-words text-sm text-zinc-800 dark:text-zinc-100 bg-white dark:bg-zinc-800 leading-relaxed">
          {(state as { stage: "decrypted"; text: string }).text}
        </pre>
      </div>

      {(state as { stage: "decrypted"; deletionToken: string }).deletionToken && (
        <button
          type="button"
          onClick={handleDelete}
          disabled={deleting}
          className="flex items-center gap-2 rounded-lg border border-red-200 dark:border-red-900/40 px-4 py-2.5 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/10 focus:outline-none focus:ring-2 focus:ring-red-500/20 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
        >
          {deleting && <Spinner size="sm" className="text-red-500" />}
          {deleting ? "Deleting..." : "Delete this secret"}
        </button>
      )}
    </div>
  );
}
