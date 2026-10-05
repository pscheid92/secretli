import type { ReactNode } from "react";
import Spinner from "../Spinner";

interface ProgressRowProps {
  /** What is happening, as a short line. */
  label: string;
  /** 0..1, once the work reports how far it is. */
  fraction?: number;
  /** A way out, like a cancel button. */
  action?: ReactNode;
}

/** The bottom row of a card while it is busy: a thin bar, a spinner and a line. */
export default function ProgressRow({ label, fraction, action }: ProgressRowProps) {
  const percent =
    fraction === undefined ? null : Math.round(Math.min(Math.max(fraction, 0), 1) * 100);
  return (
    <div className="relative border-t border-line">
      {percent !== null && (
        <div
          role="progressbar"
          aria-label={label}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={percent}
          className="absolute inset-x-0 top-0 h-0.5 overflow-hidden bg-line"
        >
          <div
            className="h-full bg-accent transition-[width] duration-200"
            style={{ width: `${percent}%` }}
          />
        </div>
      )}
      <div className="flex flex-wrap items-center gap-3 px-6 py-3 text-sm text-muted">
        <Spinner size="sm" className="text-accent" />
        <span role="status" className="flex-1">
          {label}
        </span>
        {action}
      </div>
    </div>
  );
}
