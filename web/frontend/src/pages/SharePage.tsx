import { useRef, useState } from "react";
import { toast } from "sonner";
import Composer, { type ComposeData, type ComposeProgress } from "../components/compose/Composer";
import SecretResult from "../components/SecretResult";
import { ArrowLeftIcon } from "../components/ui/icons";
import PageTitle from "../components/ui/PageTitle";
import TextButton from "../components/ui/TextButton";
import { useLeaveWarning } from "../hooks/useLeaveWarning";
import { usePageTitle } from "../hooks/usePageTitle";
import { ApiError, deleteSecret } from "../lib/api";
import { KeySet } from "../lib/encryption";
import { formatSize } from "../lib/format";
import { UploadCancelledError, uploadMultipartBundle } from "../lib/multipartBundleUpload";
import {
  fitsBundleManifestLimit,
  fitsBundleUploadLimit,
  MAX_UPLOAD_LABEL,
} from "../lib/uploadLimits";

// Text is stored as a single-file bundle so there is exactly one on-the-wire
// format; the name is inside the encrypted manifest and never reaches the server.
const TEXT_SECRET_FILENAME = "secret.txt";

interface ShareResult {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  passwordProtected: boolean;
  deletionToken: string;
  publicID: string;
  metadataToken: string;
}

type View = { kind: "compose" } | { kind: "result"; result: ShareResult } | { kind: "deleted" };

export default function SharePage() {
  const [view, setView] = useState<View>({ kind: "compose" });
  const [busy, setBusy] = useState<ComposeProgress | null>(null);
  const [deleting, setDeleting] = useState(false);
  // Each new secret gets a fresh composer.
  const [draft, setDraft] = useState(0);
  const abortRef = useRef<AbortController | null>(null);
  usePageTitle(
    view.kind === "result"
      ? "Link ready"
      : view.kind === "deleted"
        ? "Secret deleted"
        : "Share a secret",
  );

  // A navigation away from an in-flight upload silently discards it.
  useLeaveWarning(busy !== null);

  async function handleSubmit(data: ComposeData) {
    const files =
      data.kind === "files"
        ? data.files
        : [
            new File([new TextEncoder().encode(data.text)], TEXT_SECRET_FILENAME, {
              type: "text/plain",
            }),
          ];
    if (!fitsBundleUploadLimit(files.map((file) => file.size))) {
      toast.error(`Together these files exceed the ${MAX_UPLOAD_LABEL} limit.`);
      return;
    }
    if (!fitsBundleManifestLimit(files)) {
      toast.error("Too many files for one link. Zip them first, or split them up.");
      return;
    }

    const controller = new AbortController();
    abortRef.current = controller;
    setBusy({ stage: "encrypting" });

    try {
      const keySet = await KeySet.generateRandom();
      const hasPassword = data.password.length > 0;

      let encryptKeySet = keySet;
      if (hasPassword) {
        const encoded = keySet.getEncoded();
        encryptKeySet = await KeySet.fromShareSecret(encoded.shareSecret, data.password);
      }

      // Text and files are encrypted record by record and streamed as multipart
      // parts alike, so there is a single upload path. Only a real upload is
      // worth a cancel button; a text is gone before anyone could press it.
      const cancellable = data.kind === "files";
      setBusy({ stage: "uploading", onCancel: cancellable ? () => controller.abort() : undefined });
      const response = await uploadMultipartBundle({
        files,
        secretType: data.kind === "files" ? "bundle" : "text",
        baseKeySet: keySet,
        bundleKeySet: encryptKeySet,
        passwordProtected: hasPassword,
        expiration: data.expiration,
        burnAfterRead: data.burnAfterRead,
        signal: controller.signal,
        onProgress: ({ uploadedBytes, totalBytes }) => {
          if (!cancellable) return;
          setBusy({
            stage: "uploading",
            fraction: totalBytes > 0 ? uploadedBytes / totalBytes : 0,
            label: `${formatSize(uploadedBytes)} / ${formatSize(totalBytes)}`,
            onCancel: () => controller.abort(),
          });
        },
      });

      setView({
        kind: "result",
        result: {
          url: `${window.location.origin}/s#${response.encoded.shareSecret}`,
          expiresAt: response.expires_at,
          burnAfterRead: data.burnAfterRead,
          passwordProtected: hasPassword,
          deletionToken: response.deletionToken,
          publicID: response.encoded.publicID,
          metadataToken: response.encoded.metadataToken,
        },
      });
    } catch (err) {
      if (err instanceof UploadCancelledError) {
        toast.info("Upload cancelled.");
      } else if (err instanceof ApiError) {
        toast.error(err.message);
      } else if (err instanceof Error && err.message === "bundle manifest is too large") {
        toast.error("Too many files for one link. Zip them first, or split them up.");
      } else {
        toast.error("An unexpected error occurred. Please try again.");
      }
    } finally {
      abortRef.current = null;
      setBusy(null);
    }
  }

  async function handleDelete(result: ShareResult) {
    setDeleting(true);
    try {
      await deleteSecret(result.publicID, result.metadataToken, result.deletionToken);
      setView({ kind: "deleted" });
      toast.success("Secret deleted");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Failed to delete the secret.");
    } finally {
      setDeleting(false);
    }
  }

  function startOver() {
    setDraft(draft + 1);
    setView({ kind: "compose" });
  }

  const startOverLink = (
    <TextButton tone="muted" onClick={startOver}>
      <ArrowLeftIcon />
      Share another secret
    </TextButton>
  );

  if (view.kind === "result") {
    return (
      <div className="space-y-7">
        <SecretResult
          url={view.result.url}
          expiresAt={view.result.expiresAt}
          burnAfterRead={view.result.burnAfterRead}
          passwordProtected={view.result.passwordProtected}
          deletionToken={view.result.deletionToken}
          deleting={deleting}
          onDelete={() => handleDelete(view.result)}
        />
        {startOverLink}
      </div>
    );
  }

  if (view.kind === "deleted") {
    return (
      <div className="space-y-8">
        <PageTitle lead="The link doesn't open anything any more.">Secret deleted</PageTitle>
        {startOverLink}
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <PageTitle lead="Encrypted in your browser. Gone once it's read.">Share a secret</PageTitle>
      <Composer key={draft} onSubmit={handleSubmit} busy={busy} />
    </div>
  );
}
