import { useState } from 'react'
import { Link, Outlet } from 'react-router'
import { useAuth } from '../context/AuthContext'
import { useTheme } from '../hooks/useTheme'

function ThemeIcon({ theme }: { theme: string }) {
  if (theme === 'dark') {
    return (
      <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
      </svg>
    )
  }
  if (theme === 'light') {
    return (
      <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
      </svg>
    )
  }
  // system
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
    </svg>
  )
}

export default function Layout() {
  const { user, logout } = useAuth()
  const { theme, cycle } = useTheme()
  const [menuOpen, setMenuOpen] = useState(false)

  return (
    <div className="min-h-screen flex flex-col bg-gray-50 dark:bg-gray-950 text-gray-900 dark:text-gray-100">
      <header className="border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
        <nav className="mx-auto flex max-w-4xl items-center justify-between px-6 py-4">
          <Link to="/" className="text-xl font-bold tracking-tight">
            Secretli
          </Link>

          {/* Desktop nav */}
          <div className="hidden md:flex items-center gap-6 text-sm font-medium">
            <Link to="/" className="hover:text-blue-600 dark:hover:text-blue-400">Share</Link>
            <Link to="/file" className="hover:text-blue-600 dark:hover:text-blue-400">File</Link>
            {user ? (
              <>
                <Link to="/history" className="hover:text-blue-600 dark:hover:text-blue-400">History</Link>
                <span className="text-gray-400">{user.display_name || user.email}</span>
                <button onClick={() => logout()} className="text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400">Logout</button>
              </>
            ) : (
              <Link to="/login" className="hover:text-blue-600 dark:hover:text-blue-400">Login</Link>
            )}
            <button onClick={cycle} className="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" title={`Theme: ${theme}`}>
              <ThemeIcon theme={theme} />
            </button>
          </div>

          {/* Mobile buttons */}
          <div className="flex md:hidden items-center gap-3">
            <button onClick={cycle} className="text-gray-500 dark:text-gray-400" title={`Theme: ${theme}`}>
              <ThemeIcon theme={theme} />
            </button>
            <button onClick={() => setMenuOpen(!menuOpen)} className="text-gray-500 dark:text-gray-400">
              <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
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
          <div className="md:hidden border-t border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 px-6 py-3 space-y-2 text-sm font-medium">
            <Link to="/" onClick={() => setMenuOpen(false)} className="block py-1 hover:text-blue-600 dark:hover:text-blue-400">Share</Link>
            <Link to="/file" onClick={() => setMenuOpen(false)} className="block py-1 hover:text-blue-600 dark:hover:text-blue-400">File</Link>
            {user ? (
              <>
                <Link to="/history" onClick={() => setMenuOpen(false)} className="block py-1 hover:text-blue-600 dark:hover:text-blue-400">History</Link>
                <span className="block py-1 text-gray-400">{user.display_name || user.email}</span>
                <button onClick={() => { logout(); setMenuOpen(false) }} className="block py-1 text-gray-500 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400">Logout</button>
              </>
            ) : (
              <Link to="/login" onClick={() => setMenuOpen(false)} className="block py-1 hover:text-blue-600 dark:hover:text-blue-400">Login</Link>
            )}
          </div>
        )}
      </header>

      <main className="mx-auto w-full max-w-4xl flex-1 px-6 py-8">
        <Outlet />
      </main>

      <footer className="border-t border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 py-4 text-center text-xs text-gray-400">
        Secretli &mdash; Zero-knowledge secret sharing
      </footer>
    </div>
  )
}
