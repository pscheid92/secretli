import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import Spinner from '../components/Spinner'
import { KeySet } from '../lib/encryption'
import { retrieveSecret, downloadFile, deleteSecret, ApiError } from '../lib/api'

type State =
  | { stage: 'loading' }
  | { stage: 'password'; shareSecret: string; deletionToken: string; secretType: string; nonce: string; encryptedData: string }
  | { stage: 'decrypted'; text: string; deletionToken: string; shareSecret: string }
  | { stage: 'downloading' }
  | { stage: 'file-ready'; filename: string; deletionToken: string; shareSecret: string }
  | { stage: 'deleted' }
  | { stage: 'error'; message: string }

export default function RetrievePage() {
  const [state, setState] = useState<State>({ stage: 'loading' })
  const [password, setPassword] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [passwordLoading, setPasswordLoading] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const retrieve = useCallback(async () => {
    const hash = window.location.hash.slice(1)
    if (!hash) {
      setState({ stage: 'error', message: 'No secret key found in the URL.' })
      return
    }

    const delimiterIndex = hash.indexOf('!')
    const shareSecret = delimiterIndex >= 0 ? hash.slice(0, delimiterIndex) : hash
    const deletionToken = delimiterIndex >= 0 ? hash.slice(delimiterIndex + 1) : ''

    try {
      const keySet = await KeySet.fromShareSecret(shareSecret)
      const encoded = keySet.getEncoded()

      const response = await retrieveSecret(encoded.publicID, encoded.retrievalToken)

      if (response.password_protected) {
        setState({
          stage: 'password',
          shareSecret,
          deletionToken,
          secretType: response.secret_type,
          nonce: response.nonce,
          encryptedData: response.encrypted_data,
        })
        return
      }

      if (response.secret_type === 'file') {
        await handleFileDownload(keySet, encoded.publicID, encoded.retrievalToken, shareSecret, deletionToken)
        return
      }

      const text = await keySet.decrypt(response.nonce, response.encrypted_data)
      setState({ stage: 'decrypted', text, deletionToken, shareSecret })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 404) {
          setState({ stage: 'error', message: 'This secret has expired or does not exist.' })
        } else if (err.status === 403) {
          setState({ stage: 'error', message: 'Invalid retrieval token.' })
        } else {
          setState({ stage: 'error', message: err.message })
        }
      } else {
        setState({ stage: 'error', message: 'An unexpected error occurred.' })
      }
    }
  }, [])

  async function handleFileDownload(
    keySet: KeySet,
    publicID: string,
    retrievalToken: string,
    shareSecret: string,
    deletionToken: string,
  ) {
    setState({ stage: 'downloading' })

    const fileResponse = await downloadFile(publicID, retrievalToken)

    const decryptedBytes = await keySet.decryptFile(fileResponse.nonce, fileResponse.blob)
    let filename = 'download'
    if (fileResponse.encryptedFilename) {
      filename = await keySet.decryptFilename(fileResponse.encryptedFilename)
    }

    const url = URL.createObjectURL(new Blob([decryptedBytes.buffer.slice(decryptedBytes.byteOffset, decryptedBytes.byteOffset + decryptedBytes.byteLength) as ArrayBuffer]))
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)

    setState({ stage: 'file-ready', filename, deletionToken, shareSecret })
  }

  useEffect(() => { retrieve() }, [retrieve])

  async function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (state.stage !== 'password') return

    setPasswordLoading(true)
    setPasswordError('')

    try {
      const keySet = await KeySet.fromShareSecret(state.shareSecret, password)
      const encoded = keySet.getEncoded()

      if (state.secretType === 'file') {
        await handleFileDownload(keySet, encoded.publicID, encoded.retrievalToken, state.shareSecret, state.deletionToken)
        return
      }

      const text = await keySet.decrypt(state.nonce, state.encryptedData)
      setState({ stage: 'decrypted', text, deletionToken: state.deletionToken, shareSecret: state.shareSecret })
    } catch {
      setPasswordError('Wrong password. Please try again.')
    } finally {
      setPasswordLoading(false)
    }
  }

  async function handleDelete() {
    if (state.stage !== 'decrypted' && state.stage !== 'file-ready') return
    if (!state.deletionToken) return

    setDeleting(true)
    try {
      const keySet = await KeySet.fromShareSecret(state.shareSecret)
      const encoded = keySet.getEncoded()
      await deleteSecret(encoded.publicID, encoded.retrievalToken, state.deletionToken)
      setState({ stage: 'deleted' })
      toast.success('Secret deleted')
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message)
      } else {
        toast.error('Failed to delete secret.')
      }
    } finally {
      setDeleting(false)
    }
  }

  async function copyDecryptedText() {
    if (state.stage !== 'decrypted') return
    await navigator.clipboard.writeText(state.text)
    toast.success('Secret copied to clipboard')
  }

  if (state.stage === 'loading') {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-12">
        <Spinner size="lg" className="text-blue-600" />
        <p className="text-gray-500 dark:text-gray-400">Decrypting secret...</p>
      </div>
    )
  }

  if (state.stage === 'downloading') {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-12">
        <Spinner size="lg" className="text-blue-600" />
        <p className="text-gray-500 dark:text-gray-400">Downloading and decrypting file...</p>
      </div>
    )
  }

  if (state.stage === 'error') {
    return (
      <div className="space-y-4">
        <div className="rounded-md border border-red-200 dark:border-red-800 bg-red-50 dark:bg-red-950 p-4">
          <h2 className="text-lg font-semibold text-red-800 dark:text-red-300">Unable to retrieve secret</h2>
          <p className="mt-1 text-sm text-red-700 dark:text-red-400">{state.message}</p>
        </div>
        <a href="/" className="inline-block text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300">
          Share a new secret
        </a>
      </div>
    )
  }

  if (state.stage === 'password') {
    return (
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-bold dark:text-white">Password Required</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            This secret is password-protected. Enter the password to decrypt it.
          </p>
        </div>
        <form onSubmit={handlePasswordSubmit} className="space-y-3">
          <input
            type="password"
            value={password}
            onChange={(e) => { setPassword(e.target.value); setPasswordError('') }}
            placeholder="Enter password..."
            autoFocus
            className="w-full rounded-md border border-gray-300 dark:border-gray-600 dark:bg-gray-800 dark:text-white px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          {passwordError && <p className="text-sm text-red-600 dark:text-red-400">{passwordError}</p>}
          <button
            type="submit"
            disabled={passwordLoading}
            className="flex w-full items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {passwordLoading && <Spinner size="sm" />}
            {passwordLoading ? 'Decrypting...' : 'Decrypt'}
          </button>
        </form>
      </div>
    )
  }

  if (state.stage === 'deleted') {
    return (
      <div className="space-y-4">
        <div className="rounded-md border border-green-200 dark:border-green-800 bg-green-50 dark:bg-green-950 p-4">
          <h2 className="text-lg font-semibold text-green-800 dark:text-green-300">Secret deleted</h2>
          <p className="mt-1 text-sm text-green-700 dark:text-green-400">The secret has been permanently destroyed.</p>
        </div>
        <a href="/" className="inline-block text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300">
          Share a new secret
        </a>
      </div>
    )
  }

  if (state.stage === 'file-ready') {
    return (
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-bold dark:text-white">File Downloaded</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
            The file has been decrypted and saved to your downloads.
          </p>
        </div>
        <div className="rounded-md border border-green-200 dark:border-green-800 bg-green-50 dark:bg-green-950 p-4">
          <p className="text-sm font-medium text-green-800 dark:text-green-300">{state.filename}</p>
        </div>
        {state.deletionToken && (
          <button
            type="button"
            onClick={handleDelete}
            disabled={deleting}
            className="flex items-center gap-2 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {deleting && <Spinner size="sm" />}
            {deleting ? 'Deleting...' : 'Delete this secret'}
          </button>
        )}
      </div>
    )
  }

  // stage === 'decrypted'
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-bold dark:text-white">Secret</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          This secret was shared with you. Copy the contents below.
        </p>
      </div>

      <div className="rounded-md border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
        <div className="flex items-start justify-between gap-2">
          <pre className="flex-1 whitespace-pre-wrap break-words text-sm text-gray-900 dark:text-gray-100">{state.text}</pre>
          <button
            type="button"
            onClick={copyDecryptedText}
            className="shrink-0 rounded-md bg-gray-100 dark:bg-gray-700 px-3 py-1 text-xs font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-600"
          >
            Copy
          </button>
        </div>
      </div>

      {state.deletionToken && (
        <button
          type="button"
          onClick={handleDelete}
          disabled={deleting}
          className="flex items-center gap-2 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {deleting && <Spinner size="sm" />}
          {deleting ? 'Deleting...' : 'Delete this secret'}
        </button>
      )}
    </div>
  )
}
