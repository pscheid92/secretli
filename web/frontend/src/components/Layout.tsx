import { useState } from "react";
import { Link, Outlet, useLocation } from "react-router";
import { useAuth } from "../context/AuthContext";
import { useTheme } from "../hooks/useTheme";

function ThemeIcon({ theme }: { theme: string }) {
  if (theme === "dark") {
    return (
      <svg
        className="h-5 w-5"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        strokeWidth={2}
        aria-hidden="true"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"
        />
      </svg>
    );
  }
  if (theme === "light") {
    return (
      <svg
        className="h-5 w-5"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        strokeWidth={2}
        aria-hidden="true"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"
        />
      </svg>
    );
  }
  // system
  return (
    <svg
      className="h-5 w-5"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      strokeWidth={2}
      aria-hidden="true"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"
      />
    </svg>
  );
}

function navLinkClass(currentPath: string, linkPath: string): string {
  const isActive = currentPath === linkPath;
  return [
    "pb-0.5 transition-colors duration-150",
    isActive
      ? "text-blue-600 dark:text-blue-400 border-b-2 border-blue-600 dark:border-blue-400"
      : "hover:text-blue-600 dark:hover:text-blue-400",
  ].join(" ");
}

export default function Layout() {
  const { user, logout } = useAuth();
  const { theme, cycle } = useTheme();
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();

  return (
    <div className="min-h-screen flex flex-col bg-gray-50 dark:bg-gray-950 text-gray-900 dark:text-gray-100">
      <header className="bg-white/80 dark:bg-gray-900/80 backdrop-blur-md sticky top-0 z-50 border-b border-gray-200 dark:border-gray-800">
        <nav className="mx-auto flex max-w-4xl items-center justify-between px-6 py-4">
          <Link
            to="/"
            className="text-xl font-bold tracking-tight bg-gradient-to-r from-blue-600 to-violet-600 bg-clip-text text-transparent"
          >
            Secretli
          </Link>

          {/* Desktop nav */}
          <div className="hidden md:flex items-center gap-6 text-sm font-medium">
            <Link to="/share" className={navLinkClass(location.pathname, "/share")}>
              Text
            </Link>
            <Link to="/file" className={navLinkClass(location.pathname, "/file")}>
              File
            </Link>
            {user ? (
              <>
                <Link to="/history" className={navLinkClass(location.pathname, "/history")}>
                  History
                </Link>
                <span className="text-gray-400">{user.display_name || user.email}</span>
                <button
                  type="button"
                  onClick={() => logout()}
                  className="text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400 transition-colors duration-150"
                >
                  Logout
                </button>
              </>
            ) : (
              <Link to="/login" className={navLinkClass(location.pathname, "/login")}>
                Login
              </Link>
            )}
            <button
              type="button"
              onClick={cycle}
              className="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200 transition-colors duration-150"
              title={`Theme: ${theme}`}
            >
              <ThemeIcon theme={theme} />
            </button>
          </div>

          {/* Mobile buttons */}
          <div className="flex md:hidden items-center gap-3">
            <button
              type="button"
              onClick={cycle}
              className="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800 transition-colors duration-150"
              title={`Theme: ${theme}`}
            >
              <ThemeIcon theme={theme} />
            </button>
            <button
              type="button"
              onClick={() => setMenuOpen(!menuOpen)}
              className="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800 transition-colors duration-150"
            >
              <svg
                className="h-6 w-6"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
                aria-hidden="true"
              >
                {menuOpen ? (
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                ) : (
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
                )}
              </svg>
            </button>
          </div>
        </nav>

        {/* Mobile menu dropdown */}
        {menuOpen && (
          <div className="md:hidden border-t border-gray-200 dark:border-gray-800 bg-white/80 dark:bg-gray-900/80 backdrop-blur-md px-6 py-3 space-y-1 text-sm font-medium">
            <Link
              to="/share"
              onClick={() => setMenuOpen(false)}
              className="block rounded-lg px-2 py-2 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors duration-150"
            >
              Text
            </Link>
            <Link
              to="/file"
              onClick={() => setMenuOpen(false)}
              className="block rounded-lg px-2 py-2 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors duration-150"
            >
              File
            </Link>
            {user ? (
              <>
                <Link
                  to="/history"
                  onClick={() => setMenuOpen(false)}
                  className="block rounded-lg px-2 py-2 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors duration-150"
                >
                  History
                </Link>
                <span className="block px-2 py-2 text-gray-400">
                  {user.display_name || user.email}
                </span>
                <button
                  type="button"
                  onClick={() => {
                    logout();
                    setMenuOpen(false);
                  }}
                  className="block w-full text-left rounded-lg px-2 py-2 text-gray-500 hover:bg-gray-100 hover:text-red-600 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-red-400 transition-colors duration-150"
                >
                  Logout
                </button>
              </>
            ) : (
              <Link
                to="/login"
                onClick={() => setMenuOpen(false)}
                className="block rounded-lg px-2 py-2 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors duration-150"
              >
                Login
              </Link>
            )}
          </div>
        )}
      </header>

      <main className="mx-auto w-full max-w-4xl flex-1 px-6 py-8">
        <Outlet />
      </main>

      <footer className="py-6 text-center text-xs text-gray-400">
        Secretli &mdash; Zero-knowledge secret sharing
      </footer>
    </div>
  );
}
