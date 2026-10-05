import type { ReactNode } from "react";

/** One line worth noticing, marked with the amber dot instead of a box. */
export default function Note({ children }: { children: ReactNode }) {
  return (
    <p className="flex items-start gap-3 text-body text-muted">
      <span aria-hidden="true" className="mt-2.25 h-1.5 w-1.5 shrink-0 rounded-full bg-accent" />
      <span>{children}</span>
    </p>
  );
}
