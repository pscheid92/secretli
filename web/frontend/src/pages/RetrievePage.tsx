import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import Spinner from "../components/Spinner";
import {
  ApiError,
  type SecretMetadataResponse,
  deleteSecret,
  downloadFile,
  getSecretMetadata,
  retrieveSecret,
} from "../lib/api";
import { KeySet } from "../lib/encryption";

type State =
  | { stage: "loading" }
  | {
      stage: "confirm";
      shareSecret: string;
      deletionToken: string;
      metadata: SecretMetadataResponse;
    }
  | {
      stage: "password";
      shareSecret: string;
      deletionToken: string;
      secretType: string;
      nonce: string;
      encryptedData: string;
    }
  | { stage: "decrypted"; text: string; deletionToken: string; shareSecret: string }
  | { stage: "downloading" }
  | { stage: "file-ready"; filename: string; deletionToken: string; shareSecret: string }
  | { stage: "deleted" }
  | { stage: "error"; message: string };

export default function RetrievePage() {
  const [state, setState] = useState<State>({ stage: "loading" });
  const [password, setPassword] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [revealing, setRevealing] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleFileDownload = useCallback(
    async (
      keySet: KeySet,
      publicID: string,
      retrievalToken: string,
      shareSecret: string,
      deletionToken: string,
    ) => {
      setState({ stage: "downloading" });

      const fileResponse = await downloadFile(publicID, retrievalToken);

      const decryptedBytes = await keySet.decryptFile(fileResponse.nonce, fileResponse.blob);
      let filename = "download";
      if (fileResponse.encryptedFilename) {
        filename = await keySet.decryptFilename(fileResponse.encryptedFilename);
      }

      const url = URL.createObjectURL(
        new Blob([
          decryptedBytes.buffer.slice(
            decryptedBytes.byteOffset,
            decryptedBytes.byteOffset + decryptedBytes.byteLength,
          ) as ArrayBuffer,
        ]),
      );
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      setState({ stage: "file-ready", filename, deletionToken, shareSecret });
    },
    [],
  );

  const fetchMetadata = useCallback(async () => {
    const hash = window.location.hash.slice(1);
    if (!hash) {
      setState({ stage: "error", message: "No secret key found in the URL." });
      return;
    }

    const delimiterIndex = hash.indexOf("!");
    const shareSecret = delimiterIndex >= 0 ? hash.slice(0, delimiterIndex) : hash;
    const deletionToken = delimiterIndex >= 0 ? hash.slice(delimiterIndex + 1) : "";

    try {
      const keySet = await KeySet.fromShareSecret(shareSecret);
      const encoded = keySet.getEncoded();

      const metadata = await getSecretMetadata(encoded.publicID, encoded.retrievalToken);

      setState({ stage: "confirm", shareSecret, deletionToken, metadata });
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 404) {
          setState({ stage: "error", message: "This secret has expired or does not exist." });
        } else if (err.status === 403) {
          setState({ stage: "error", message: "Invalid retrieval token." });
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

    const { shareSecret, deletionToken } = state;
    setRevealing(true);

    try {
      const keySet = await KeySet.fromShareSecret(shareSecret);
      const encoded = keySet.getEncoded();

      const response = await retrieveSecret(encoded.publicID, encoded.retrievalToken);

      if (response.password_protected) {
        setState({
          stage: "password",
          shareSecret,
          deletionToken,
          secretType: response.secret_type,
          nonce: response.nonce,
          encryptedData: response.encrypted_data,
        });
        return;
      }

      if (response.secret_type === "file") {
        await handleFileDownload(
          keySet,
          encoded.publicID,
          encoded.retrievalToken,
          shareSecret,
          deletionToken,
        );
        return;
      }

      const text = await keySet.decrypt(response.nonce, response.encrypted_data);
      setState({ stage: "decrypted", text, deletionToken, shareSecret });
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 404) {
          setState({ stage: "error", message: "This secret has expired or does not exist." });
        } else {
          setState({ stage: "error", message: err.message });
        }
      } else {
        setState({ stage: "error", message: "An unexpected error occurred." });
      }
    } finally {
      setRevealing(false);
    }
  }

  async function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (state.stage !== "password") return;

    setPasswordLoading(true);
    setPasswordError("");

    try {
      const keySet = await KeySet.fromShareSecret(state.shareSecret, password);
      const encoded = keySet.getEncoded();

      if (state.secretType === "file") {
        await handleFileDownload(
          keySet,
          encoded.publicID,
          encoded.retrievalToken,
          state.shareSecret,
          state.deletionToken,
        );
        return;
      }

      const text = await keySet.decrypt(state.nonce, state.encryptedData);
      setState({
        stage: "decrypted",
        text,
        deletionToken: state.deletionToken,
        shareSecret: state.shareSecret,
      });
    } catch {
      setPasswordError("Wrong password. Please try again.");
    } finally {
      setPasswordLoading(false);
    }
  }

  async function handleDelete() {
    if (state.stage !== "decrypted" && state.stage !== "file-ready") return;
    if (!state.deletionToken) return;

    setDeleting(true);
    try {
      const keySet = await KeySet.fromShareSecret(state.shareSecret);
      const encoded = keySet.getEncoded();
      await deleteSecret(encoded.publicID, encoded.retrievalToken, state.deletionToken);
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
    toast.success("Secret copied to clipboard");
  }

  function formatTimestamp(iso: string): string {
    return new Date(iso).toLocaleString();
  }

  if (state.stage === "loading") {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-12">
        <Spinner size="lg" className="text-blue-600" />
        <p className="text-gray-500 dark:text-gray-400">Loading secret info...</p>
      </div>
    );
  }

  if (state.stage === "downloading") {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-12">
        <Spinner size="lg" className="text-blue-600" />
        <p className="text-gray-500 dark:text-gray-400">Downloading and decrypting file...</p>
      </div>
    );
  }

  if (state.stage === "error") {
    return (
      <div className="space-y-4">
        <div className="rounded-xl border-l-4 border-red-500 bg-white dark:bg-gray-900 p-5 shadow-sm">
          <h2 className="text-lg font-semibold text-red-800 dark:text-red-300">
            Unable to retrieve secret
          </h2>
          <p className="mt-1 text-sm text-red-700 dark:text-red-400">{state.message}</p>
        </div>
        <a
          href="/share"
          className="inline-block text-sm font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 transition-colors duration-150"
        >
          Share a new secret
        </a>
      </div>
    );
  }

  if (state.stage === "confirm") {
    const { metadata } = state;
    const isFile = metadata.secret_type === "file";

    return (
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-bold dark:text-white">Secret Ready</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            Review the details below, then reveal the secret.
          </p>
        </div>

        <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-5 shadow-sm divide-y divide-gray-100 dark:divide-gray-800">
          <div className="flex justify-between py-2.5 first:pt-0 text-sm">
            <span className="text-gray-500 dark:text-gray-400">Type</span>
            <span className="font-medium text-gray-900 dark:text-gray-100 capitalize">
              {metadata.secret_type}
            </span>
          </div>
          <div className="flex justify-between py-2.5 text-sm">
            <span className="text-gray-500 dark:text-gray-400">Created</span>
            <span className="text-gray-900 dark:text-gray-100">
              {formatTimestamp(metadata.created_at)}
            </span>
          </div>
          <div className="flex justify-between py-2.5 text-sm">
            <span className="text-gray-500 dark:text-gray-400">Expires</span>
            <span className="text-gray-900 dark:text-gray-100">
              {formatTimestamp(metadata.expires_at)}
            </span>
          </div>
          {metadata.password_protected && (
            <div className="flex justify-between py-2.5 last:pb-0 text-sm">
              <span className="text-gray-500 dark:text-gray-400">Password</span>
              <span className="text-amber-600 dark:text-amber-400 font-medium">Required</span>
            </div>
          )}
        </div>

        {metadata.burn_after_read && (
          <div className="rounded-xl border-l-4 border-amber-500 bg-white dark:bg-gray-900 p-5 shadow-sm">
            <p className="text-sm font-medium text-amber-800 dark:text-amber-300">
              This secret will be permanently destroyed after viewing.
            </p>
          </div>
        )}

        <button
          type="button"
          onClick={handleReveal}
          disabled={revealing}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2.5 font-semibold text-white shadow-md hover:bg-blue-700 hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-150"
        >
          {revealing && <Spinner size="sm" />}
          {revealing
            ? "Revealing..."
            : isFile
              ? "Download & Decrypt"
              : "Reveal Secret"}
        </button>
      </div>
    );
  }

  if (state.stage === "password") {
    return (
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-bold dark:text-white">Password Required</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            This secret is password-protected. Enter the password to decrypt it.
          </p>
        </div>
        <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-5 shadow-sm">
          <form onSubmit={handlePasswordSubmit} className="space-y-3">
            <input
              type="password"
              value={password}
              onChange={(e) => {
                setPassword(e.target.value);
                setPasswordError("");
              }}
              placeholder="Enter password..."
              autoFocus
              className="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-800 dark:text-white px-3.5 py-2.5 text-sm placeholder:text-gray-400 focus:bg-white dark:focus:bg-gray-800 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 transition-colors duration-150"
            />
            {passwordError && (
              <p className="text-sm text-red-600 dark:text-red-400">{passwordError}</p>
            )}
            <button
              type="submit"
              disabled={passwordLoading}
              className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2.5 font-semibold text-white shadow-md hover:bg-blue-700 hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-150"
            >
              {passwordLoading && <Spinner size="sm" />}
              {passwordLoading ? "Decrypting..." : "Decrypt"}
            </button>
          </form>
        </div>
      </div>
    );
  }

  if (state.stage === "deleted") {
    return (
      <div className="space-y-4">
        <div className="rounded-xl border-l-4 border-green-500 bg-white dark:bg-gray-900 p-5 shadow-sm">
          <h2 className="text-lg font-semibold text-green-800 dark:text-green-300">
            Secret deleted
          </h2>
          <p className="mt-1 text-sm text-green-700 dark:text-green-400">
            The secret has been permanently destroyed.
          </p>
        </div>
        <a
          href="/share"
          className="inline-block text-sm font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 transition-colors duration-150"
        >
          Share a new secret
        </a>
      </div>
    );
  }

  if (state.stage === "file-ready") {
    return (
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-bold dark:text-white">File Downloaded</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            The file has been decrypted and saved to your downloads.
          </p>
        </div>
        <div className="rounded-xl border-l-4 border-green-500 bg-white dark:bg-gray-900 p-5 shadow-sm">
          <p className="text-sm font-medium text-green-800 dark:text-green-300">{state.filename}</p>
        </div>
        {state.deletionToken && (
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            className="flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2.5 font-semibold text-white shadow-md hover:bg-red-700 hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-150"
          >
            {deleting && <Spinner size="sm" />}
            {deleting ? "Deleting..." : "Delete this secret"}
          </button>
        )}
      </div>
    );
  }

  // stage === 'decrypted'
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-bold dark:text-white">Secret</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          This secret was shared with you. Copy the contents below.
        </p>
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-5 shadow-sm">
        <div className="flex items-start justify-between gap-2">
          <pre className="flex-1 whitespace-pre-wrap break-words text-sm text-gray-900 dark:text-gray-100">
            {state.text}
          </pre>
          <button
            type="button"
            onClick={copyDecryptedText}
            className="shrink-0 rounded-lg bg-gray-100 dark:bg-gray-700 px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors duration-150"
          >
            Copy
          </button>
        </div>
      </div>

      {state.deletionToken && (
        <button
          type="button"
          onClick={handleDelete}
          disabled={deleting}
          className="flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2.5 font-semibold text-white shadow-md hover:bg-red-700 hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-150"
        >
          {deleting && <Spinner size="sm" />}
          {deleting ? "Deleting..." : "Delete this secret"}
        </button>
      )}
    </div>
  );
}
