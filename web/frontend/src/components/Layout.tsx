import { Link, Outlet } from 'react-router'
import { useAuth } from '../context/AuthContext'

export default function Layout() {
  const { user, logout } = useAuth()

  return (
    <div className="min-h-screen flex flex-col bg-gray-50 text-gray-900">
      <header className="border-b border-gray-200 bg-white">
        <nav className="mx-auto flex max-w-4xl items-center justify-between px-6 py-4">
          <Link to="/" className="text-xl font-bold tracking-tight">
            Secretli
          </Link>
          <div className="flex items-center gap-6 text-sm font-medium">
            <Link to="/" className="hover:text-blue-600">
              Share
            </Link>
            <Link to="/file" className="hover:text-blue-600">
              File
            </Link>
            {user ? (
              <>
                <Link to="/history" className="hover:text-blue-600">
                  History
                </Link>
                <span className="text-gray-400">{user.display_name || user.email}</span>
                <button
                  onClick={() => logout()}
                  className="text-gray-500 hover:text-red-600"
                >
                  Logout
                </button>
              </>
            ) : (
              <Link to="/login" className="hover:text-blue-600">
                Login
              </Link>
            )}
          </div>
        </nav>
      </header>

      <main className="mx-auto w-full max-w-4xl flex-1 px-6 py-8">
        <Outlet />
      </main>

      <footer className="border-t border-gray-200 bg-white py-4 text-center text-xs text-gray-400">
        Secretli &mdash; Zero-knowledge secret sharing
      </footer>
    </div>
  )
}
