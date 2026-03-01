import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../context/AuthContext'
import { getUserSecrets, type SecretSummary } from '../lib/api'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function StatusBadge({ secret }: { secret: SecretSummary }) {
  const now = new Date()
  const expires = new Date(secret.expires_at)

  if (expires < now) {
    return <span className="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-500">Expired</span>
  }
  if (secret.retrieved_at) {
    return <span className="rounded bg-green-100 px-2 py-0.5 text-xs text-green-700">Retrieved</span>
  }
  return <span className="rounded bg-blue-100 px-2 py-0.5 text-xs text-blue-700">Pending</span>
}

export default function HistoryPage() {
  const { user, isLoading: authLoading } = useAuth()
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const perPage = 20

  const { data, isLoading } = useQuery({
    queryKey: ['user-secrets', page, perPage],
    queryFn: () => getUserSecrets(page, perPage),
    enabled: !!user,
  })

  if (!authLoading && !user) {
    navigate('/login', { replace: true })
    return null
  }

  const secrets = data?.secrets ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / perPage)

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Secret History</h1>
        <p className="mt-1 text-sm text-gray-500">
          Secrets you've shared while signed in.
        </p>
      </div>

      {isLoading || authLoading ? (
        <p className="text-sm text-gray-400">Loading...</p>
      ) : secrets.length === 0 ? (
        <p className="text-sm text-gray-400">No secrets yet. Share one to get started.</p>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-200 text-xs uppercase text-gray-500">
                <tr>
                  <th className="px-3 py-2">Label</th>
                  <th className="px-3 py-2">Type</th>
                  <th className="px-3 py-2">Created</th>
                  <th className="px-3 py-2">Expires</th>
                  <th className="px-3 py-2">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {secrets.map((s) => (
                  <tr key={s.public_id} className="hover:bg-gray-50">
                    <td className="px-3 py-2 font-medium">
                      {s.label || <span className="text-gray-300">untitled</span>}
                    </td>
                    <td className="px-3 py-2 capitalize">{s.secret_type}</td>
                    <td className="px-3 py-2 text-gray-500">{formatDate(s.created_at)}</td>
                    <td className="px-3 py-2 text-gray-500">{formatDate(s.expires_at)}</td>
                    <td className="px-3 py-2">
                      <StatusBadge secret={s} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-between text-sm">
              <button
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
                className="rounded border border-gray-300 px-3 py-1 disabled:opacity-50"
              >
                Previous
              </button>
              <span className="text-gray-500">
                Page {page} of {totalPages}
              </span>
              <button
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
                className="rounded border border-gray-300 px-3 py-1 disabled:opacity-50"
              >
                Next
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
