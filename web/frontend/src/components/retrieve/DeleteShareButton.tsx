import { useState } from "react";
import Spinner from "../Spinner";
import Button from "../ui/Button";
import { TrashIcon } from "../ui/icons";
import TextButton from "../ui/TextButton";

interface DeleteShareButtonProps {
  deleting: boolean;
  disabled?: boolean;
  onDelete: () => void;
}

/** Deletes the secret for everyone. A second click confirms: it can't be undone. */
export default function DeleteShareButton({
  deleting,
  disabled,
  onDelete,
}: DeleteShareButtonProps) {
  const [confirming, setConfirming] = useState(false);

  if (!confirming && !deleting) {
    return (
      <TextButton tone="danger" onClick={() => setConfirming(true)} disabled={disabled}>
        <TrashIcon />
        Delete it now
      </TextButton>
    );
  }

  return (
    <div
      role="group"
      aria-label="Delete it now"
      className="flex flex-wrap items-center gap-x-4 gap-y-2 motion-safe:animate-fade"
    >
      <p className="min-w-60 flex-1 text-sm text-ink">
        Delete it for good? The link stops working right away.
      </p>
      <div className="flex gap-1">
        <Button variant="danger-outline" onClick={onDelete} disabled={deleting || disabled}>
          {deleting && <Spinner size="sm" />}
          {deleting ? "Deleting…" : "Delete permanently"}
        </Button>
        <Button variant="quiet" onClick={() => setConfirming(false)} disabled={deleting} autoFocus>
          Cancel
        </Button>
      </div>
    </div>
  );
}
