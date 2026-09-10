import Spinner from "./Spinner";

export interface TransferStep {
  label: string;
  state: "done" | "active" | "pending";
}

export interface TransferProgress {
  /** 0..1 */
  fraction: number;
  label: string;
}

interface TransferStatusProps {
  title: string;
  steps: TransferStep[];
  progress?: TransferProgress;
  onCancel?: () => void;
  cancelLabel?: string;
}

export default function TransferStatus({
  title,
  steps,
  progress,
  onCancel,
  cancelLabel = "Cancel",
}: TransferStatusProps) {
  const percent = progress ? Math.round(Math.min(Math.max(progress.fraction, 0), 1) * 100) : null;
  return (
    <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-900/40 dark:bg-amber-900/10">
      <div className="flex items-center gap-2">
        <Spinner size="sm" className="text-amber-500" />
        <span className="text-sm font-medium text-zinc-800 dark:text-zinc-100">{title}</span>
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="ml-auto text-xs font-semibold text-zinc-600 transition-colors duration-150 hover:text-red-600 dark:text-zinc-300 dark:hover:text-red-400"
          >
            {cancelLabel}
          </button>
        )}
      </div>
      {progress && percent !== null && (
        <div className="mt-3">
          <div
            role="progressbar"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={percent}
            aria-label={title}
            className="h-1.5 w-full overflow-hidden rounded-full bg-amber-200/60 dark:bg-amber-900/40"
          >
            <div
              className="h-full rounded-full bg-amber-400 transition-[width] duration-200"
              style={{ width: `${percent}%` }}
            />
          </div>
          <div className="mt-1.5 flex justify-between text-xs tabular-nums text-zinc-600 dark:text-zinc-300">
            <span>{progress.label}</span>
            <span>{percent}%</span>
          </div>
        </div>
      )}
      <div className="mt-3 grid gap-2 sm:grid-cols-3">
        {steps.map((step) => (
          <div key={step.label} className="flex items-center gap-2 text-xs">
            <span
              className={`h-2 w-2 flex-shrink-0 rounded-full ${
                step.state === "done"
                  ? "bg-emerald-400"
                  : step.state === "active"
                    ? "bg-amber-400"
                    : "bg-zinc-300 dark:bg-zinc-700"
              }`}
            />
            <span
              className={
                step.state === "pending"
                  ? "text-zinc-500 dark:text-zinc-400"
                  : "text-zinc-700 dark:text-zinc-100"
              }
            >
              {step.label}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
