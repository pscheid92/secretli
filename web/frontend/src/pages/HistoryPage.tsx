import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import Spinner from "../components/Spinner";
import { useAuth } from "../context/AuthContext";
import { getUserSecrets, type SecretSummary } from "../lib/api";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function StatusBadge({ secret }: { secret: SecretSummary }) {
  const now = new Date();
  const expires = new Date(secret.expires_at);

  if (expires < now) {
    return (
      <span className="rounded-full bg-gray-100 dark:bg-gray-700 px-2.5 py-0.5 text-xs font-medium text-gray-500 dark:text-gray-400">
        Expired
      </span>
    );
  }
  if (secret.retrieved_at) {
    return (
      <span className="rounded-full bg-green-100 dark:bg-green-900 px-2.5 py-0.5 text-xs font-medium text-green-700 dark:text-green-400">
        Retrieved
      </span>
    );
  }
  return (
    <span className="rounded-full bg-blue-100 dark:bg-blue-900 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:text-blue-400">
      Pending
    </span>
  );
}

export default function HistoryPage() {
  const { user, isLoading: authLoading } = useAuth();
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const perPage = 20;

  const { data, isLoading } = useQuery({
    queryKey: ["user-secrets", page, perPage],
    queryFn: () => getUserSecrets(page, perPage),
    enabled: !!user,
  });

  if (!authLoading && !user) {
    navigate("/login", { replace: true });
    return null;
  }

  const secrets = data?.secrets ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / perPage);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold dark:text-white">Secret History</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Secrets you've shared while signed in.
        </p>
      </div>

      {isLoading || authLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="lg" className="text-blue-600" />
        </div>
      ) : secrets.length === 0 ? (
        <p className="text-sm text-gray-400">No secrets yet. Share one to get started.</p>
      ) : (
        <>
          {/* Desktop table */}
          <div className="hidden md:block rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 shadow-sm overflow-hidden">
            <table className="w-full text-left text-sm">
              <thead className="bg-gray-50 dark:bg-gray-800/50 text-xs uppercase text-gray-500 dark:text-gray-400">
                <tr>
                  <th className="px-4 py-3">Label</th>
                  <th className="px-4 py-3">Type</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3">Expires</th>
                  <th className="px-4 py-3">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                {secrets.map((s) => (
                  <tr key={s.public_id} className="hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors">
                    <td className="px-4 py-3 font-medium">
                      {s.label || (
                        <span className="text-gray-300 dark:text-gray-600">untitled</span>
                      )}
                    </td>
                    <td className="px-4 py-3 capitalize">{s.secret_type}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                      {formatDate(s.created_at)}
                    </td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                      {formatDate(s.expires_at)}
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge secret={s} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile cards */}
          <div className="md:hidden space-y-3">
            {secrets.map((s) => (
              <div
                key={s.public_id}
                className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-4 shadow-sm space-y-1"
              >
                <div className="flex items-center justify-between">
                  <span className="font-medium text-sm">
                    {s.label || <span className="text-gray-300 dark:text-gray-600">untitled</span>}
                  </span>
                  <StatusBadge secret={s} />
                </div>
                <div className="flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
                  <span className="capitalize">{s.secret_type}</span>
                  <span>{formatDate(s.created_at)}</span>
                </div>
              </div>
            ))}
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-between text-sm">
              <button
                type="button"
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
                className="rounded-lg border border-gray-300 dark:border-gray-600 px-3 py-1.5 hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-50 dark:text-gray-300 transition-colors duration-150"
              >
                Previous
              </button>
              <span className="text-gray-500 dark:text-gray-400">
                Page {page} of {totalPages}
              </span>
              <button
                type="button"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
                className="rounded-lg border border-gray-300 dark:border-gray-600 px-3 py-1.5 hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-50 dark:text-gray-300 transition-colors duration-150"
              >
                Next
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
