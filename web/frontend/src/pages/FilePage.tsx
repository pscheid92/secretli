import { useState } from 'react'
import { toast } from 'sonner'
import FileUpload from '../components/FileUpload'
import ExpirationPicker from '../components/ExpirationPicker'
import SecretResult from '../components/SecretResult'
import Spinner from '../components/Spinner'
import { KeySet } from '../lib/encryption'
import { uploadFile, ApiError } from '../lib/api'

interface FileResult {
  url: string
  expiresAt: string
  burnAfterRead: boolean
}

export default function FilePage() {
  const [file, setFile] = useState<File | null>(null)
  const [expiration, setExpiration] = useState('1d')
  const [burnAfterRead, setBurnAfterRead] = useState(false)
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<FileResult | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!file) {
      toast.error('Please select a file.')
      return
    }

    setLoading(true)

    try {
      const keySet = await KeySet.generateRandom()
      const hasPassword = password.length > 0

      let encryptKeySet = keySet
      if (hasPassword) {
        const encoded = keySet.getEncoded()
        encryptKeySet = await KeySet.fromShareSecret(encoded.shareSecret, password)
      }

      const fileBytes = new Uint8Array(await file.arrayBuffer())
      const { nonce, encryptedBlob } = await encryptKeySet.encryptFile(fileBytes)
      const encryptedFilename = await encryptKeySet.encryptFilename(file.name)

      const encoded = keySet.getEncoded()

      const response = await uploadFile(
        {
          public_id: hasPassword ? encryptKeySet.getEncoded().publicID : encoded.publicID,
          retrieval_token: hasPassword ? encryptKeySet.getEncoded().retrievalToken : encoded.retrievalToken,
          deletion_token: encoded.deletionToken,
          nonce,
          expiration,
          burn_after_read: burnAfterRead,
          password_protected: hasPassword,
          encrypted_filename: encryptedFilename,
        },
        encryptedBlob,
      )

      const shareUrl = `${window.location.origin}/s#${encoded.shareSecret}!${encoded.deletionToken}`

      setResult({
        url: shareUrl,
        expiresAt: response.expires_at,
        burnAfterRead,
      })
      toast.success('File uploaded')
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message)
      } else {
        toast.error('An unexpected error occurred. Please try again.')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold dark:text-white">Share a File</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Your file is encrypted in your browser before being uploaded. Only the link holder can decrypt it.
        </p>
      </div>

      {result ? (
        <div className="space-y-4">
          <SecretResult
            url={result.url}
            expiresAt={result.expiresAt}
            burnAfterRead={result.burnAfterRead}
          />
          <button
            type="button"
            onClick={() => { setResult(null); setFile(null) }}
            className="text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
          >
            Share another file
          </button>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          <FileUpload onSelect={setFile} />

          <div className="flex flex-wrap items-center gap-4">
            <div>
              <label htmlFor="expiration" className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
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
              className="text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
            >
              {showPassword ? 'Remove password protection' : 'Add password protection'}
            </button>
            {showPassword && (
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter a password..."
                className="mt-2 w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-800 dark:text-white px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              />
            )}
          </div>

          <button
            type="submit"
            disabled={loading || !file}
            className="flex w-full items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading && <Spinner size="sm" />}
            {loading ? 'Encrypting & uploading...' : 'Share File'}
          </button>
        </form>
      )}
    </div>
  )
}
