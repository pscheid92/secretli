import { Link } from "react-router";

export default function NotFoundPage() {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <p className="font-display text-7xl font-bold text-zinc-200 dark:text-zinc-800 mb-4">404</p>
      <p className="text-sm text-zinc-600 dark:text-zinc-100 mb-6">This page doesn't exist.</p>
      <Link
        to="/"
        className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
      >
        ← Go home
      </Link>
    </div>
  );
}
