import { useState, useRef, useCallback } from 'react'

const MAX_FILE_SIZE = 100 * 1024 * 1024 // 100MB

interface FileUploadProps {
  onSelect: (file: File) => void
}

export default function FileUpload({ onSelect }: FileUploadProps) {
  const [dragOver, setDragOver] = useState(false)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const handleFile = useCallback((file: File) => {
    setError('')
    if (file.size > MAX_FILE_SIZE) {
      setError('File exceeds 100MB limit.')
      return
    }
    setSelectedFile(file)
    onSelect(file)
  }, [onSelect])

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    setDragOver(false)
    const file = e.dataTransfer.files[0]
    if (file) handleFile(file)
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file) handleFile(file)
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  return (
    <div>
      <div
        onDragOver={(e) => { e.preventDefault(); setDragOver(true) }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        className={`cursor-pointer rounded-md border-2 border-dashed p-8 text-center transition-colors ${
          dragOver
            ? 'border-blue-400 bg-blue-50 dark:bg-blue-950'
            : 'border-gray-300 dark:border-gray-600 hover:border-gray-400 dark:hover:border-gray-500'
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          onChange={handleChange}
          className="hidden"
        />
        {selectedFile ? (
          <div className="space-y-1">
            <p className="text-sm font-medium text-gray-900 dark:text-gray-100">{selectedFile.name}</p>
            <p className="text-xs text-gray-500 dark:text-gray-400">{formatSize(selectedFile.size)}</p>
            <p className="text-xs text-blue-600 dark:text-blue-400">Click or drop to change file</p>
          </div>
        ) : (
          <div className="space-y-1">
            <p className="text-sm text-gray-600 dark:text-gray-400">Drop a file here, or click to select</p>
            <p className="text-xs text-gray-400 dark:text-gray-500">Max 100MB</p>
          </div>
        )}
      </div>
      {error && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{error}</p>}
    </div>
  )
}
