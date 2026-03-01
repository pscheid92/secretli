import { toast } from 'sonner'

interface SecretResultProps {
  url: string
  expiresAt: string
  burnAfterRead: boolean
}

export default function SecretResult({ url, expiresAt, burnAfterRead }: SecretResultProps) {
  async function copyToClipboard() {
    await navigator.clipboard.writeText(url)
    toast.success('Link copied to clipboard')
  }

  return (
    <div className="space-y-4 rounded-md border border-green-200 bg-green-50 dark:border-green-800 dark:bg-green-950 p-4">
      <h2 className="text-lg font-semibold text-green-800 dark:text-green-300">Secret created!</h2>

      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Share this link</label>
        <div className="flex gap-2">
          <input
            type="text"
            readOnly
            value={url}
            className="flex-1 rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 dark:text-white px-3 py-2 text-sm shadow-sm"
          />
          <button
            type="button"
            onClick={copyToClipboard}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
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
  )
}
