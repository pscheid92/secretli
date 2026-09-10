import Spinner from "../Spinner";

export function RetrieveLoading() {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-20">
      <Spinner size="lg" className="text-amber-400" />
      <p className="text-sm text-zinc-600 dark:text-zinc-100">Checking share...</p>
    </div>
  );
}

export function RetrieveError({ message }: { message: string }) {
  return (
    <div className="space-y-5">
      <div className="rounded-lg border border-red-200 dark:border-red-900/40 bg-red-50 dark:bg-red-900/10 p-5">
        <div className="flex items-start gap-3">
          <svg
            className="h-4 w-4 text-red-500 flex-shrink-0 mt-0.5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
            aria-hidden="true"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <div>
            <p className="text-sm font-medium text-red-700 dark:text-red-400">
              Unable to open share
            </p>
            <p className="text-sm text-red-600 dark:text-red-500 mt-0.5">{message}</p>
          </div>
        </div>
      </div>
      <a
        href="/share"
        className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
      >
        ← Create a new share
      </a>
    </div>
  );
}

export function ShareDeleted() {
  return (
    <div className="space-y-5">
      <div className="flex items-center gap-2.5">
        <div className="w-2 h-2 rounded-full bg-emerald-400" />
        <span className="text-sm font-medium text-zinc-600 dark:text-zinc-100">Share deleted</span>
      </div>
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 px-4 py-4">
        <p className="text-sm text-zinc-600 dark:text-zinc-100">
          The share has been permanently destroyed.
        </p>
      </div>
      <a
        href="/"
        className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
      >
        ← Create a new share
      </a>
    </div>
  );
}
