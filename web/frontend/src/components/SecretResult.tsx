import { toast } from "sonner";

interface SecretResultProps {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
}

export default function SecretResult({ url, expiresAt, burnAfterRead }: SecretResultProps) {
  async function copyToClipboard() {
    await navigator.clipboard.writeText(url);
    toast.success("Link copied to clipboard");
  }

  return (
    <div className="rounded-xl border border-green-200 dark:border-green-800 bg-white dark:bg-gray-900 p-5 shadow-sm space-y-4">
      <h2 className="text-lg font-semibold text-green-800 dark:text-green-300">Secret created!</h2>

      <div>
        <label
          htmlFor="share-url"
          className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
        >
          Share this link
        </label>
        <div className="flex gap-2">
          <input
            id="share-url"
            type="text"
            readOnly
            value={url}
            className="flex-1 rounded-lg border border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-800 dark:text-white px-3.5 py-2.5 font-mono text-xs focus:bg-white dark:focus:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-500/20 transition-colors duration-150"
          />
          <button
            type="button"
            onClick={copyToClipboard}
            className="rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-semibold text-white shadow-md hover:bg-blue-700 hover:shadow-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 transition-all duration-150"
          >
            Copy
          </button>
        </div>
      </div>

      <div className="text-sm text-gray-600 dark:text-gray-400 space-y-1">
        <p>Expires: {new Date(expiresAt).toLocaleString()}</p>
        {burnAfterRead && (
          <p className="text-amber-700 dark:text-amber-400 font-medium">
            This secret will be destroyed after being viewed once.
          </p>
        )}
      </div>
    </div>
  );
}
