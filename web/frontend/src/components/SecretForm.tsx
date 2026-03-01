import { useState } from 'react'
import ExpirationPicker from './ExpirationPicker'

export interface SecretFormData {
  text: string
  expiration: string
  burnAfterRead: boolean
  password: string
}

interface SecretFormProps {
  onSubmit: (data: SecretFormData) => void
  loading: boolean
}

export default function SecretForm({ onSubmit, loading }: SecretFormProps) {
  const [text, setText] = useState('')
  const [expiration, setExpiration] = useState('1d')
  const [burnAfterRead, setBurnAfterRead] = useState(false)
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!text.trim()) {
      setError('Please enter a secret to share.')
      return
    }
    setError('')
    onSubmit({ text, expiration, burnAfterRead, password })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label htmlFor="secret-text" className="block text-sm font-medium text-gray-700 mb-1">
          Secret
        </label>
        <textarea
          id="secret-text"
          value={text}
          onChange={(e) => { setText(e.target.value); setError('') }}
          placeholder="Enter your secret text..."
          rows={6}
          className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
        {error && <p className="mt-1 text-sm text-red-600">{error}</p>}
      </div>

      <div className="flex flex-wrap items-center gap-4">
        <div>
          <label htmlFor="expiration" className="block text-sm font-medium text-gray-700 mb-1">
            Expires in
          </label>
          <ExpirationPicker value={expiration} onChange={setExpiration} />
        </div>

        <label className="flex items-center gap-2 pt-5 text-sm text-gray-700 cursor-pointer">
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
          className="text-sm text-blue-600 hover:text-blue-800"
        >
          {showPassword ? 'Remove password protection' : 'Add password protection'}
        </button>
        {showPassword && (
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Enter a password..."
            className="mt-2 w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        )}
      </div>

      <button
        type="submit"
        disabled={loading}
        className="w-full rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {loading ? 'Encrypting...' : 'Share Secret'}
      </button>
    </form>
  )
}
