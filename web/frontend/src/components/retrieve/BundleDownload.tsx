import { useEffect, useState } from "react";
import { useLeaveWarning } from "../../hooks/useLeaveWarning";
import { type BundleManifest, type DecryptedBundleFile, manifestTotalSize } from "../../lib/bundle";
import { saveBlob } from "../../lib/download";
import { formatSize } from "../../lib/format";
import Button from "../ui/Button";
import { ArrowRightIcon, DownloadIcon, FileIcon } from "../ui/icons";
import Note from "../ui/Note";
import PageTitle from "../ui/PageTitle";
import ProgressRow from "../ui/ProgressRow";
import { textButtonClass } from "../ui/styles";
import TextButton from "../ui/TextButton";
import DeleteShareButton from "./DeleteShareButton";

/** 0..1 of the files decrypted so far, with a label like "4.0 MB / 10.0 MB". */
export interface DownloadProgress {
  fraction: number;
  label: string;
}

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
  progress: DownloadProgress | null;
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
  // A one-time share is gone from the server: until the files are saved,
  // this page holds the only way to get them.
  const onlyCopyHere = burnAfterRead && !downloadedFiles && !expired;
  useLeaveWarning(onlyCopyHere);

  const note = downloadedFiles
    ? isMulti
      ? "Saved. If your browser blocked one of them, use Save next to that file."
      : "Saved. Use Save to save it again."
    : expired
      ? burnAfterRead
        ? "The download window closed. This one-time secret can't be opened again."
        : "The download window closed. Open the link again to start a new one."
      : `${formatCountdown(secondsLeft)} left to download${
          burnAfterRead ? ", and it opens only once: stay on this page until it's done." : "."
        }`;

  return (
    <div className="space-y-7">
      <PageTitle
        lead={`${manifest.files.length} ${isMulti ? "files" : "file"} · ${formatSize(totalSize)}`}
      >
        {isMulti ? "Here are your files" : "Here's your file"}
      </PageTitle>

      <div className="overflow-hidden rounded-[20px] border border-line bg-surface shadow-card">
        <ul className="m-0 list-none px-2.5 pt-2.5 pb-1.5">
          {manifest.files.map((file) => {
            const downloaded = downloadedFiles?.find((entry) => entry.file.index === file.index);
            return (
              <li
                key={`${file.index}-${file.path}`}
                data-testid={`bundle-file-${file.index}`}
                className="flex min-h-13 items-center gap-3 pr-2 pl-3.5"
              >
                <span className="flex text-faint">
                  <FileIcon />
                </span>
                <span
                  className="min-w-0 flex-1 truncate font-mono text-sm text-ink"
                  title={file.path}
                >
                  {file.path}
                </span>
                <span className="text-[13px] tabular-nums text-faint">{formatSize(file.size)}</span>
                {downloaded && (
                  <TextButton onClick={() => saveBlob(downloaded.blob, downloaded.file.name)}>
                    Save
                  </TextButton>
                )}
              </li>
            );
          })}
        </ul>
        <div className="flex flex-wrap items-center gap-0.5 border-t border-line p-2">
          <Button onClick={onDownloadAll} disabled={!canDownload} className="pl-4">
            <DownloadIcon />
            {downloading
              ? "Preparing…"
              : downloadedFiles
                ? "Save again"
                : isMulti
                  ? "Download files"
                  : "Download file"}
          </Button>
        </div>
        {downloading && (
          <ProgressRow
            label={progress ? `Decrypting · ${progress.label}` : "Reading the encrypted data…"}
            fraction={progress?.fraction}
          />
        )}
      </div>

      <Note>{note}</Note>

      {canDelete && <DeleteShareButton deleting={deleting} onDelete={onDelete} />}

      <a href="/share" className={textButtonClass("muted")}>
        Share a secret of your own
        <ArrowRightIcon />
      </a>
    </div>
  );
}
