import Spinner from "../Spinner";

interface DeleteShareButtonProps {
  deleting: boolean;
  disabled?: boolean;
  onDelete: () => void;
}

export default function DeleteShareButton({
  deleting,
  disabled,
  onDelete,
}: DeleteShareButtonProps) {
  return (
    <button
      type="button"
      onClick={onDelete}
      disabled={deleting || disabled}
      className="flex items-center gap-2 rounded-lg border border-red-200 px-4 py-2.5 text-sm text-red-600 transition-all duration-150 hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-red-500/20 disabled:cursor-not-allowed disabled:opacity-40 dark:border-red-900/40 dark:text-red-400 dark:hover:bg-red-900/10"
    >
      {deleting && <Spinner size="sm" className="text-red-500" />}
      {deleting ? "Deleting..." : "Delete share"}
    </button>
  );
}
