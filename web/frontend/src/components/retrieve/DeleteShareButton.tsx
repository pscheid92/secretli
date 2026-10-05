import { useState } from "react";
import Spinner from "../Spinner";
import Button from "../ui/Button";
import { TrashIcon } from "../ui/icons";
import TextButton from "../ui/TextButton";

interface DeleteShareButtonProps {
  deleting: boolean;
  disabled?: boolean;
  /** What the first button says; it also names the confirmation. */
  label?: string;
  /** The confirmation's question. */
  question?: string;
  onDelete: () => void;
}

/** Deletes the share for everyone. A second click confirms: it can't be undone. */
export default function DeleteShareButton({
  deleting,
  disabled,
  label = "Delete share",
  question = "Delete this share for everyone? Its links stop working, and this can't be undone.",
  onDelete,
}: DeleteShareButtonProps) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming && !deleting) {
    return (
      <TextButton tone="danger" onClick={() => setConfirming(true)} disabled={disabled}>
        <TrashIcon />
        {label}
      </TextButton>
    );
  }

  return (
    <div role="group" aria-label={label} className="flex flex-wrap items-center gap-x-4 gap-y-2">
      <p className="min-w-60 flex-1 text-sm text-ink">{question}</p>
      <div className="flex gap-1">
        <Button variant="danger-outline" onClick={onDelete} disabled={deleting || disabled}>
          {deleting && <Spinner size="sm" />}
          {deleting ? "Deleting..." : "Delete permanently"}
        </Button>
        <Button variant="quiet" onClick={() => setConfirming(false)} disabled={deleting} autoFocus>
          Cancel
        </Button>
      </div>
    </div>
  );
}
