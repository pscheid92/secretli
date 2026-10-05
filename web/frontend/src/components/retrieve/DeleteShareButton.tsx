import { useState } from "react";
import Spinner from "../Spinner";

interface DeleteShareButtonProps {
  deleting: boolean;
  disabled?: boolean;
  onDelete: () => void;
}

/** Deletes the share for everyone. A second click confirms: it can't be undone. */
export default function DeleteShareButton({
  deleting,
  disabled,
  onDelete,
}: DeleteShareButtonProps) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming && !deleting) {
    return (
      <button
        type="button"
        onClick={() => setConfirming(true)}
        disabled={disabled}
        className="flex items-center gap-2 rounded-lg border border-red-200 px-4 py-2.5 text-sm text-red-600 transition-all duration-150 hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-red-500/20 disabled:cursor-not-allowed disabled:opacity-40 dark:border-red-900/40 dark:text-red-400 dark:hover:bg-red-900/10"
      >
        Delete share
      </button>
    );
  }

  return (
    <div
      role="group"
      aria-label="Delete share"
      className="space-y-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 dark:border-red-900/40 dark:bg-red-900/10"
    >
      <p className="text-sm text-red-700 dark:text-red-400">
        Delete this share for everyone? Its links stop working, and this can't be undone.
      </p>
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          onClick={onDelete}
          disabled={deleting || disabled}
          className="flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors duration-150 hover:bg-red-500 focus:outline-none focus:ring-2 focus:ring-red-500/40 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {deleting && <Spinner size="sm" />}
          {deleting ? "Deleting..." : "Delete permanently"}
        </button>
        <button
          type="button"
          onClick={() => setConfirming(false)}
          disabled={deleting}
          autoFocus
          className="rounded-lg border border-zinc-200 bg-white px-4 py-2 text-sm font-medium text-zinc-700 transition-colors duration-150 hover:border-zinc-400 hover:text-zinc-900 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-zinc-500/50 dark:bg-zinc-900 dark:text-zinc-100 dark:hover:text-white"
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
