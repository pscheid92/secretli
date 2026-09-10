import { useEffect, useState } from "react";
import { type BundleManifest, type DecryptedBundleFile, manifestTotalSize } from "../../lib/bundle";
import { saveBlob } from "../../lib/download";
import { formatSize } from "../../lib/format";
import Spinner from "../Spinner";
import TransferStatus, { type TransferProgress } from "../TransferStatus";
import DeleteShareButton from "./DeleteShareButton";

function secondsUntil(iso: string): number {
  return Math.max(0, Math.floor((new Date(iso).getTime() - Date.now()) / 1000));
}

function useSecondsUntil(iso: string): number {
  const [seconds, setSeconds] = useState(() => secondsUntil(iso));
  useEffect(() => {
    setSeconds(secondsUntil(iso));
    const timer = window.setInterval(() => setSeconds(secondsUntil(iso)), 1000);
    return () => window.clearInterval(timer);
  }, [iso]);
  return seconds;
}

function formatCountdown(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return `${minutes}:${rest.toString().padStart(2, "0")}`;
}

interface BundleDownloadProps {
  manifest: BundleManifest;
  sessionExpiresAt: string;
  burnAfterRead: boolean;
  downloading: boolean;
  progress: TransferProgress | null;
  downloadedFiles: DecryptedBundleFile[] | null;
  canDelete: boolean;
  deleting: boolean;
  onDownloadAll: () => void;
  onDelete: () => void;
}

/** The file list and download controls once the manifest has been read. */
export default function BundleDownload({
  manifest,
  sessionExpiresAt,
  burnAfterRead,
  downloading,
  progress,
  downloadedFiles,
  canDelete,
  deleting,
  onDownloadAll,
  onDelete,
}: BundleDownloadProps) {
  const isMulti = manifest.files.length > 1;
  const totalSize = manifestTotalSize(manifest);
  const secondsLeft = useSecondsUntil(sessionExpiresAt);
  // Once the blobs are in memory the server session no longer matters.
  const expired = secondsLeft === 0 && !downloadedFiles;
  const canDownload = !downloading && !expired;

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
          {manifest.files.map((file) => {
            const downloaded = downloadedFiles?.find((entry) => entry.file.index === file.index);
            return (
              <div
                key={`${file.index}-${file.path}`}
                data-testid={`bundle-file-${file.index}`}
                className="grid grid-cols-[1fr_auto] items-center gap-4 px-4 py-3"
              >
                <span className="min-w-0 truncate font-mono text-sm font-medium text-zinc-700 dark:text-zinc-100">
                  {file.path}
                </span>
                <span className="flex items-center gap-3">
                  <span className="text-xs tabular-nums text-zinc-500 dark:text-zinc-400">
                    {formatSize(file.size)}
                  </span>
                  {downloaded && (
                    <button
                      type="button"
                      onClick={() => saveBlob(downloaded.blob, downloaded.file.name)}
                      className="text-xs font-semibold text-amber-600 transition-colors duration-150 hover:text-amber-500 dark:text-amber-400 dark:hover:text-amber-300"
                    >
                      Save
                    </button>
                  )}
                </span>
              </div>
            );
          })}
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
            {!downloadedFiles && (
              <div className="py-3">
                <div className="text-xs text-zinc-500 dark:text-zinc-400">Download window</div>
                <div
                  className={`mt-1 text-sm font-medium tabular-nums ${
                    expired
                      ? "text-red-600 dark:text-red-400"
                      : secondsLeft < 120
                        ? "text-amber-600 dark:text-amber-400"
                        : "text-zinc-900 dark:text-zinc-100"
                  }`}
                >
                  {expired ? "Expired" : `${formatCountdown(secondsLeft)} remaining`}
                </div>
              </div>
            )}
          </div>
        </section>

        {expired && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-xs text-red-700 dark:border-red-900/40 dark:bg-red-900/10 dark:text-red-400">
            {burnAfterRead
              ? "The download window has closed. This share was consumed when it was opened and can no longer be downloaded."
              : "The download window has closed. Open the link again to start a new one."}
          </div>
        )}

        {downloading && (
          <TransferStatus
            title={isMulti ? "Preparing files" : "Preparing file"}
            progress={progress ?? undefined}
            steps={[
              { label: "Reading encrypted data", state: "active" },
              { label: "Decrypting", state: "active" },
              { label: "Saving", state: "pending" },
            ]}
          />
        )}
        {isMulti && (
          <p className="text-xs leading-relaxed text-zinc-500 dark:text-zinc-400">
            Your browser will save {manifest.files.length} files. If it blocks some of them, use the
            Save buttons next to each file.
          </p>
        )}
        <button
          type="button"
          onClick={onDownloadAll}
          disabled={!canDownload}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-semibold text-zinc-950 transition-all duration-150 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {downloading && <Spinner size="sm" className="text-zinc-700" />}
          {downloading
            ? "Preparing..."
            : downloadedFiles
              ? isMulti
                ? "Save Files Again"
                : "Save File Again"
              : isMulti
                ? "Download Files"
                : "Download File"}
        </button>
        {canDelete && <DeleteShareButton deleting={deleting} onDelete={onDelete} />}
      </aside>
    </div>
  );
}
