import { Link } from "react-router";
import { textButtonClass } from "../components/ui/styles";
import { usePageTitle } from "../hooks/usePageTitle";

export default function NotFoundPage() {
  usePageTitle("Page not found");
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <p className="font-display text-7xl font-bold text-zinc-500 mb-4">404</p>
      <h1 className="text-sm text-zinc-600 dark:text-zinc-100 mb-6">This page doesn't exist.</h1>
      <Link to="/" className={textButtonClass("muted")}>
        ← Go home
      </Link>
    </div>
  );
}
