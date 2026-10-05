import { useEffect, useSyncExternalStore } from "react";

// How many components currently have something to lose on leaving.
let activeWarnings = 0;
const listeners = new Set<() => void>();

function notify() {
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function warnBeforeUnload(event: BeforeUnloadEvent) {
  event.preventDefault();
  // Older browsers only show the dialog when returnValue is set.
  event.returnValue = "";
}

/**
 * While active, leaving the page asks first: the browser shows its own
 * "Leave site?" dialog. Use it while something exists only on this page,
 * like a revealed one-time secret or a running upload.
 */
export function useLeaveWarning(active: boolean) {
  useEffect(() => {
    if (!active) return;
    if (activeWarnings++ === 0) window.addEventListener("beforeunload", warnBeforeUnload);
    notify();
    return () => {
      if (--activeWarnings === 0) window.removeEventListener("beforeunload", warnBeforeUnload);
      notify();
    };
  }, [active]);
}

/**
 * Whether a leave warning is active. In-app links switch to full page loads
 * meanwhile: a client-side route change would skip the browser's dialog.
 */
export function useLeaveWarningActive(): boolean {
  return useSyncExternalStore(subscribe, () => activeWarnings > 0);
}
