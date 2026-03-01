import { useState } from "react";
import ExpirationPicker from "./ExpirationPicker";
import Spinner from "./Spinner";

export interface SecretFormData {
  text: string;
  expiration: string;
  burnAfterRead: boolean;
  password: string;
}

interface SecretFormProps {
  onSubmit: (data: SecretFormData) => void;
  loading: boolean;
}

export default function SecretForm({ onSubmit, loading }: SecretFormProps) {
  const [text, setText] = useState("");
  const [expiration, setExpiration] = useState("1d");
  const [burnAfterRead, setBurnAfterRead] = useState(false);
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!text.trim()) {
      setError("Please enter a secret to share.");
      return;
    }
    if (showPassword && !password) {
      setError("Please enter a password or disable password protection.");
      return;
    }
    setError("");
    onSubmit({ text, expiration, burnAfterRead, password });
  }

  return (
    <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-5 shadow-sm">
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label
            htmlFor="secret-text"
            className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
          >
            Secret
          </label>
          <textarea
            id="secret-text"
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              setError("");
            }}
            placeholder="Enter your secret text..."
            rows={6}
            className="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-800 dark:text-white px-3.5 py-2.5 text-sm placeholder:text-gray-400 focus:bg-white dark:focus:bg-gray-800 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 transition-colors duration-150"
          />
          {error && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{error}</p>}
        </div>

        <div className="flex flex-wrap items-center gap-4">
          <div>
            <label
              htmlFor="expiration"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
            >
              Expires in
            </label>
            <ExpirationPicker value={expiration} onChange={setExpiration} />
          </div>

          <label className="flex items-center gap-2 pt-5 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
            <input
              type="checkbox"
              checked={burnAfterRead}
              onChange={(e) => setBurnAfterRead(e.target.checked)}
              className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            Burn after reading
          </label>
        </div>

        <div>
          <button
            type="button"
            onClick={() => setShowPassword(!showPassword)}
            className="text-sm font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 transition-colors duration-150"
          >
            {showPassword ? "Remove password protection" : "Add password protection"}
          </button>
          {showPassword && (
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter a password..."
              className="mt-2 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-800 dark:text-white px-3.5 py-2.5 text-sm placeholder:text-gray-400 focus:bg-white dark:focus:bg-gray-800 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 transition-colors duration-150"
            />
          )}
        </div>

        <button
          type="submit"
          disabled={loading}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2.5 font-semibold text-white shadow-md hover:bg-blue-700 hover:shadow-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-150"
        >
          {loading && <Spinner size="sm" />}
          {loading ? "Encrypting..." : "Share Secret"}
        </button>
      </form>
    </div>
  );
}
