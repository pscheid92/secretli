import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import BundleDownload from "../components/retrieve/BundleDownload";
import LinkPrompt from "../components/retrieve/LinkPrompt";
import PasswordPrompt from "../components/retrieve/PasswordPrompt";
import {
  RetrieveError,
  RetrieveLoading,
  ShareDeleted,
} from "../components/retrieve/RetrieveStatus";
import ShareDetails from "../components/retrieve/ShareDetails";
import TextResult from "../components/retrieve/TextResult";
import type { TransferProgress } from "../components/TransferStatus";
import {
  ApiError,
  deleteSecret,
  getSecretMetadata,
  retrieveSecretRange,
  type SecretMetadataResponse,
  startRetrievalSession,
} from "../lib/api";
import {
  type BundleManifest,
  cachingRangeFetcher,
  type DecryptedBundleFile,
  DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES,
  decryptBundleFiles,
  manifestTotalSize,
  readBundleManifest,
} from "../lib/bundle";
import { saveFilesSequentially } from "../lib/download";
import { KeySet, type SecretMeta } from "../lib/encryption";
import { formatSize } from "../lib/format";

/**
 * Everything derived from the URL fragment. The base key set is derived once
 * and reused; only the blob key depends on a password.
 */
interface ShareIdentity {
  readonly baseKeySet: KeySet;
  readonly deletionToken: string;
  /** Needed to re-derive the blob key from a password. */
  readonly shareSecret: string;
}

interface DecryptedMeta {
  serverMeta: SecretMetadataResponse;
  clientMeta: SecretMeta;
}

type State =
  | { stage: "prompt" }
  | { stage: "loading" }
  | { stage: "confirm"; identity: ShareIdentity; meta: DecryptedMeta }
  | { stage: "password"; identity: ShareIdentity; meta: DecryptedMeta }
  | { stage: "decrypted"; identity: ShareIdentity; text: string }
  | {
      stage: "bundle-ready";
      identity: ShareIdentity;
      manifest: BundleManifest;
      blobKeySet: KeySet;
      publicID: string;
      sessionToken: string;
      sessionExpiresAt: string;
      burnAfterRead: boolean;
    }
  | { stage: "deleted" }
  | { stage: "error"; message: string };

/** Strips the share secret from the address bar so it does not linger in history. */
function stripFragmentFromLocation() {
  if (!window.location.hash) return;
  window.history.replaceState(null, "", window.location.pathname + window.location.search);
}

export default function RetrievePage() {
  // Read the fragment exactly once: it is removed from the address bar as soon
  // as it has been parsed, and effects may run more than once in development.
  const initialHashRef = useRef(window.location.hash.slice(1));
  const [state, setState] = useState<State>(
    initialHashRef.current ? { stage: "loading" } : { stage: "prompt" },
  );
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [revealing, setRevealing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [downloadingBundle, setDownloadingBundle] = useState(false);
  const [downloadProgress, setDownloadProgress] = useState<TransferProgress | null>(null);
  const [downloadedFiles, setDownloadedFiles] = useState<DecryptedBundleFile[] | null>(null);

  const fetchMetadata = useCallback(async () => {
    const hash = initialHashRef.current;
    if (!hash) {
      setState({ stage: "prompt" });
      return;
    }
    stripFragmentFromLocation();

    const delimiterIndex = hash.indexOf("!");
    const shareSecret = delimiterIndex >= 0 ? hash.slice(0, delimiterIndex) : hash;
    const deletionToken = delimiterIndex >= 0 ? hash.slice(delimiterIndex + 1) : "";

    try {
      const baseKeySet = await KeySet.fromShareSecret(shareSecret);
      const encoded = baseKeySet.getEncoded();
      const serverMeta = await getSecretMetadata(encoded.publicID, encoded.metadataToken);
      const clientMeta = await baseKeySet.decryptMeta(serverMeta.encrypted_meta);

      setState({
        stage: "confirm",
        identity: { baseKeySet, deletionToken, shareSecret },
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
    const { identity, meta } = state;

    if (meta.clientMeta.password_protected) {
      setState({ stage: "password", identity, meta });
      return;
    }

    setRevealing(true);
    try {
      await reveal(identity, identity.baseKeySet, meta.clientMeta);
    } catch (err) {
      handleRevealError(err);
    } finally {
      setRevealing(false);
    }
  }

  /** Returns a field error message, or null once the share has been revealed. */
  async function handlePasswordSubmit(password: string): Promise<string | null> {
    if (state.stage !== "password") return null;
    const { identity, meta } = state;

    setPasswordLoading(true);
    try {
      const blobKeySet = await KeySet.fromShareSecret(identity.shareSecret, password);
      await reveal(identity, blobKeySet, meta.clientMeta);
      return null;
    } catch (err) {
      // A wrong password derives a wrong blob token (403) or fails to decrypt.
      // Anything else (burned, expired, rate limited, offline) must not be
      // reported as a password problem.
      if (err instanceof ApiError && err.status !== 403) {
        handleRevealError(err);
        return null;
      }
      return "Wrong password. Please try again.";
    } finally {
      setPasswordLoading(false);
    }
  }

  async function reveal(identity: ShareIdentity, blobKeySet: KeySet, clientMeta: SecretMeta) {
    if (clientMeta.type !== "text" && clientMeta.type !== "bundle") {
      setState({ stage: "error", message: "This link uses an unsupported format." });
      return;
    }

    // The public ID always comes from the share secret; blob access may be
    // password-derived.
    const publicID = identity.baseKeySet.getEncoded().publicID;
    const session = await startRetrievalSession(publicID, blobKeySet.getEncoded().blobToken);
    const fetchRange = await cachingRangeFetcher(
      (start: number, end: number) =>
        retrieveSecretRange(publicID, session.session_token, start, end),
      session.blob_size,
    );

    // Text and files share one storage format, so both start the same way.
    const { manifest } = await readBundleManifest(fetchRange, blobKeySet, session.blob_size);

    if (clientMeta.type === "text") {
      const [only] = await decryptBundleFiles(manifest.files, blobKeySet, fetchRange);
      setState({ stage: "decrypted", identity, text: await only.blob.text() });
      return;
    }

    setDownloadedFiles(null);
    setState({
      stage: "bundle-ready",
      identity,
      manifest,
      blobKeySet,
      publicID,
      sessionToken: session.session_token,
      sessionExpiresAt: session.expires_at,
      burnAfterRead: session.burn_after_read,
    });
  }

  function handleRevealError(err: unknown) {
    if (!(err instanceof ApiError)) {
      setState({ stage: "error", message: "An unexpected error occurred." });
      return;
    }
    if (err.status === 404) {
      setState({ stage: "error", message: "This share has expired or does not exist." });
    } else if (err.status === 403) {
      setState({ stage: "error", message: "This link cannot unlock the share." });
    } else if (err.status === 429) {
      toast.error("Too many attempts. Please wait a minute and try again.");
    } else if (err.status === 0) {
      toast.error(err.message);
    } else {
      setState({ stage: "error", message: err.message });
    }
  }

  async function handleDelete(identity: ShareIdentity) {
    if (!identity.deletionToken) return;

    setDeleting(true);
    try {
      const encoded = identity.baseKeySet.getEncoded();
      await deleteSecret(encoded.publicID, encoded.metadataToken, identity.deletionToken);
      setState({ stage: "deleted" });
      toast.success("Share deleted");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Failed to delete share.");
    } finally {
      setDeleting(false);
    }
  }

  async function downloadAll() {
    if (state.stage !== "bundle-ready") return;
    const { blobKeySet, manifest, publicID, sessionToken } = state;
    const totalSize = manifestTotalSize(manifest);

    // Already decrypted once (for example a browser blocked some of the
    // saves): just hand the blobs to the browser again.
    if (downloadedFiles) {
      await saveDecrypted(downloadedFiles);
      return;
    }

    setDownloadingBundle(true);
    setDownloadProgress({ fraction: 0, label: `0 B / ${formatSize(totalSize)}` });
    try {
      const fetchRange = (start: number, end: number) =>
        retrieveSecretRange(publicID, sessionToken, start, end);
      const files = await decryptBundleFiles(manifest.files, blobKeySet, fetchRange, {
        maxCoalescedPlaintextBytes: DOWNLOAD_ALL_BUNDLE_COALESCED_PLAINTEXT_BYTES,
        onProgress: ({ decryptedBytes }) => {
          const done = Math.min(decryptedBytes, totalSize);
          setDownloadProgress({
            fraction: totalSize > 0 ? done / totalSize : 1,
            label: `${formatSize(done)} / ${formatSize(totalSize)}`,
          });
        },
      });
      setDownloadedFiles(files);
      await saveDecrypted(files);
    } catch (err) {
      if (err instanceof ApiError && err.status === 403) {
        toast.error("The download window has expired. Open the link again to restart.");
      } else {
        toast.error(err instanceof ApiError ? err.message : "Failed to download files.");
      }
    } finally {
      setDownloadingBundle(false);
      setDownloadProgress(null);
    }
  }

  function saveDecrypted(files: DecryptedBundleFile[]) {
    return saveFilesSequentially(files.map(({ file, blob }) => ({ name: file.name, blob })));
  }

  switch (state.stage) {
    case "prompt":
      return <LinkPrompt />;
    case "loading":
      return <RetrieveLoading />;
    case "error":
      return <RetrieveError message={state.message} />;
    case "deleted":
      return <ShareDeleted />;
    case "confirm":
      return (
        <ShareDetails
          serverMeta={state.meta.serverMeta}
          clientMeta={state.meta.clientMeta}
          revealing={revealing}
          deleting={deleting}
          canDelete={Boolean(state.identity.deletionToken)}
          onReveal={handleReveal}
          onDelete={() => handleDelete(state.identity)}
        />
      );
    case "password":
      return (
        <PasswordPrompt
          clientMeta={state.meta.clientMeta}
          burnAfterRead={state.meta.serverMeta.burn_after_read}
          loading={passwordLoading}
          onSubmit={handlePasswordSubmit}
        />
      );
    case "decrypted":
      return (
        <TextResult
          text={state.text}
          canDelete={Boolean(state.identity.deletionToken)}
          deleting={deleting}
          onDelete={() => handleDelete(state.identity)}
        />
      );
    case "bundle-ready":
      return (
        <BundleDownload
          manifest={state.manifest}
          sessionExpiresAt={state.sessionExpiresAt}
          burnAfterRead={state.burnAfterRead}
          downloading={downloadingBundle}
          progress={downloadProgress}
          downloadedFiles={downloadedFiles}
          canDelete={Boolean(state.identity.deletionToken)}
          deleting={deleting}
          onDownloadAll={downloadAll}
          onDelete={() => handleDelete(state.identity)}
        />
      );
  }
}
