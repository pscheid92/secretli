import Spinner from "./Spinner";

export interface TransferStep {
  label: string;
  state: "done" | "active" | "pending";
}

export default function TransferStatus({
  title,
  steps,
  detail,
  progress,
}: {
  title: string;
  steps: TransferStep[];
  detail?: string;
  progress?: number;
}) {
  return (
    <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-900/40 dark:bg-amber-900/10">
      <div className="flex items-center gap-2">
        <Spinner size="sm" className="text-amber-500" />
        <span className="text-sm font-medium text-zinc-800 dark:text-zinc-100">{title}</span>
      </div>
      {typeof progress === "number" && (
        <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-amber-100 dark:bg-amber-950">
          <div
            className="h-full rounded-full bg-amber-400 transition-all duration-150"
            style={{ width: `${Math.max(0, Math.min(100, progress))}%` }}
          />
        </div>
      )}
      {detail && <p className="mt-2 text-xs text-zinc-600 dark:text-zinc-300">{detail}</p>}
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
