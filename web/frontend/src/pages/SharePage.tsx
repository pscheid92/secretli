import { useState } from 'react'
import SecretForm, { type SecretFormData } from '../components/SecretForm'
import SecretResult from '../components/SecretResult'
import { KeySet } from '../lib/encryption'
import { createSecret, ApiError } from '../lib/api'

interface ShareResult {
  url: string
  expiresAt: string
  burnAfterRead: boolean
}

export default function SharePage() {
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<ShareResult | null>(null)
  const [error, setError] = useState('')

  async function handleSubmit(data: SecretFormData) {
    setLoading(true)
    setError('')
    setResult(null)

    try {
      const keySet = await KeySet.generateRandom()
      const hasPassword = data.password.length > 0

      // If password-protected, re-derive keys with the password for encryption
      let encryptKeySet = keySet
      if (hasPassword) {
        const encoded = keySet.getEncoded()
        encryptKeySet = await KeySet.fromShareSecret(encoded.shareSecret, data.password)
      }

      const encrypted = await encryptKeySet.encrypt(data.text)
      const encoded = keySet.getEncoded()

      const response = await createSecret({
        public_id: hasPassword ? encryptKeySet.getEncoded().publicID : encoded.publicID,
        retrieval_token: hasPassword ? encryptKeySet.getEncoded().retrievalToken : encoded.retrievalToken,
        deletion_token: encoded.deletionToken,
        nonce: encrypted.nonce,
        encrypted_data: encrypted.encrypted_data,
        expiration: data.expiration,
        burn_after_read: data.burnAfterRead,
        password_protected: hasPassword,
      })

      const shareUrl = `${window.location.origin}/s#${encoded.shareSecret}!${encoded.deletionToken}`

      setResult({
        url: shareUrl,
        expiresAt: response.expires_at,
        burnAfterRead: data.burnAfterRead,
      })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('An unexpected error occurred. Please try again.')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Share a Secret</h1>
        <p className="mt-1 text-sm text-gray-500">
          Your secret is encrypted in your browser before being sent to the server. Only the link holder can decrypt it.
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
            onClick={() => setResult(null)}
            className="text-sm text-blue-600 hover:text-blue-800"
          >
            Share another secret
          </button>
        </div>
      ) : (
        <>
          {error && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
              {error}
            </div>
          )}
          <SecretForm onSubmit={handleSubmit} loading={loading} />
        </>
      )}
    </div>
  )
}
