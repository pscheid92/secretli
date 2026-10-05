import { useState } from "react";
import Spinner from "../Spinner";
import Button from "../ui/Button";

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
      <Button variant="danger-outline" onClick={() => setConfirming(true)} disabled={disabled}>
        Delete share
      </Button>
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
        <Button variant="danger" onClick={onDelete} disabled={deleting || disabled}>
          {deleting && <Spinner size="sm" />}
          {deleting ? "Deleting..." : "Delete permanently"}
        </Button>
        <Button
          variant="secondary"
          onClick={() => setConfirming(false)}
          disabled={deleting}
          autoFocus
        >
          Cancel
        </Button>
      </div>
    </div>
  );
}
