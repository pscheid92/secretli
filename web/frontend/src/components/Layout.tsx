import { useState } from "react";
import { Link, Outlet, useLocation } from "react-router";
import { useTheme } from "../hooks/useTheme";

function ThemeIcon({ theme }: { theme: string }) {
  if (theme === "dark") {
    return (
      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5} aria-hidden="true">
        <path strokeLinecap="round" strokeLinejoin="round" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
      </svg>
    );
  }
  if (theme === "light") {
    return (
      <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5} aria-hidden="true">
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
      </svg>
    );
  }
  return (
    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5} aria-hidden="true">
      <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
    </svg>
  );
}

export default function Layout() {
  const { theme, cycle } = useTheme();
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();

  const isShareActive = location.pathname === "/share" || location.pathname === "/file";
  const isRetrieveActive = location.pathname === "/s";

  return (
    <div className="min-h-screen flex flex-col bg-zinc-50 dark:bg-zinc-950 text-zinc-800 dark:text-zinc-100">
      <header className="sticky top-0 z-50 border-b border-zinc-200 dark:border-zinc-500/50 bg-white/90 dark:bg-zinc-950/90 backdrop-blur-md">
        <nav className="mx-auto flex max-w-2xl items-center justify-between px-4 sm:px-6 py-4">
          <Link to="/" className="flex items-center gap-2 group">
            <span className="font-display text-sm font-semibold tracking-[0.2em] uppercase text-zinc-800 dark:text-zinc-100">
              Secretli
            </span>
            <span className="w-1.5 h-1.5 rounded-full bg-amber-400" />
          </Link>

          {/* Desktop nav */}
          <div className="hidden md:flex items-center gap-8">
            <Link
              to="/share"
              className={`text-sm tracking-wide transition-colors duration-150 ${
                isShareActive
                  ? "text-amber-500 dark:text-amber-400"
                  : "text-zinc-700 dark:text-zinc-100 hover:text-zinc-900 dark:hover:text-white"
              }`}
            >
              Share
            </Link>
            <Link
              to="/s"
              className={`text-sm tracking-wide transition-colors duration-150 ${
                isRetrieveActive
                  ? "text-amber-500 dark:text-amber-400"
                  : "text-zinc-700 dark:text-zinc-100 hover:text-zinc-900 dark:hover:text-white"
              }`}
            >
              Retrieve
            </Link>
            <button
              type="button"
              onClick={cycle}
              title={`Theme: ${theme}`}
              className="text-zinc-600 dark:text-zinc-100 hover:text-zinc-900 dark:hover:text-white transition-colors duration-150"
            >
              <ThemeIcon theme={theme} />
            </button>
          </div>

          {/* Mobile */}
          <div className="flex md:hidden items-center gap-3">
            <button
              type="button"
              onClick={cycle}
              title={`Theme: ${theme}`}
              className="text-zinc-600 dark:text-zinc-100 hover:text-zinc-900 dark:hover:text-white transition-colors duration-150"
            >
              <ThemeIcon theme={theme} />
            </button>
            <button
              type="button"
              onClick={() => setMenuOpen(!menuOpen)}
              className="text-zinc-600 dark:text-zinc-100 hover:text-zinc-900 dark:hover:text-white transition-colors duration-150"
            >
              <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5} aria-hidden="true">
                {menuOpen ? (
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                ) : (
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
                )}
              </svg>
            </button>
          </div>
        </nav>

        {menuOpen && (
          <div className="md:hidden border-t border-zinc-200 dark:border-zinc-500/50 bg-white/90 dark:bg-zinc-950/90 backdrop-blur-md px-4 sm:px-6 py-3 space-y-0.5">
            <Link
              to="/share"
              onClick={() => setMenuOpen(false)}
              className="block rounded-md px-2 py-2.5 text-sm text-zinc-600 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 hover:bg-zinc-100 dark:hover:bg-zinc-900 transition-colors duration-150"
            >
              Share
            </Link>
            <Link
              to="/s"
              onClick={() => setMenuOpen(false)}
              className="block rounded-md px-2 py-2.5 text-sm text-zinc-600 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 hover:bg-zinc-100 dark:hover:bg-zinc-900 transition-colors duration-150"
            >
              Retrieve
            </Link>
          </div>
        )}
      </header>

      <main className="mx-auto w-full max-w-2xl flex-1 px-4 sm:px-6 py-10">
        <Outlet />
      </main>

      <footer className="py-8 text-center">
        <p className="text-xs tracking-[0.15em] uppercase text-zinc-600 dark:text-zinc-100">
          Zero-knowledge secret sharing
        </p>
      </footer>
    </div>
  );
}
