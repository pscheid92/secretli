import { useCallback, useRef, useState } from "react";
import { formatSize } from "../lib/format";

const MAX_TOTAL_SIZE = 100 * 1024 * 1024; // 100MB

interface FileUploadProps {
  onSelect: (files: File[]) => void;
}

export default function FileUpload({ onSelect }: FileUploadProps) {
  const [dragOver, setDragOver] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const handleFiles = useCallback(
    (files: File[]) => {
      setError("");
      const total = files.reduce((sum, f) => sum + f.size, 0);
      if (total > MAX_TOTAL_SIZE) {
        setError("Total file size exceeds 100MB limit.");
        return;
      }
      setSelectedFiles(files);
      onSelect(files);
    },
    [onSelect],
  );

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    setDragOver(false);
    const files = Array.from(e.dataTransfer.files);
    if (files.length > 0) handleFiles(files);
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? []);
    if (files.length > 0) handleFiles(files);
  }

  return (
    <div>
      <div
        role="button"
        tabIndex={0}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") inputRef.current?.click();
        }}
        className={`cursor-pointer rounded-xl border-2 border-dashed p-10 text-center transition-all duration-150 ${
          dragOver
            ? "border-blue-400 bg-blue-50 dark:bg-blue-950 scale-[1.01]"
            : "border-gray-300 dark:border-gray-600 hover:border-blue-400 dark:hover:border-blue-500"
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          multiple
          onChange={handleChange}
          className="hidden"
        />
        {selectedFiles.length === 1 ? (
          <div className="space-y-1">
            <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
              {selectedFiles[0].name}
            </p>
            <p className="text-xs text-gray-500 dark:text-gray-400">
              {formatSize(selectedFiles[0].size)}
            </p>
            <p className="text-xs text-blue-600 dark:text-blue-400">
              Click or drop to change file
            </p>
          </div>
        ) : selectedFiles.length > 1 ? (
          <div className="space-y-2">
            <table className="mx-auto text-sm">
              <tbody>
                {selectedFiles.map((f, i) => (
                  <tr key={i}>
                    <td className="pr-4 text-left font-medium text-gray-900 dark:text-gray-100">
                      {f.name}
                    </td>
                    <td className="text-right text-gray-500 dark:text-gray-400">
                      {formatSize(f.size)}
                    </td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="border-t border-gray-200 dark:border-gray-700">
                  <td className="pr-4 pt-1 text-left text-xs text-gray-500 dark:text-gray-400">
                    {selectedFiles.length} files
                  </td>
                  <td className="pt-1 text-right text-xs text-gray-500 dark:text-gray-400">
                    {formatSize(selectedFiles.reduce((s, f) => s + f.size, 0))}
                  </td>
                </tr>
              </tfoot>
            </table>
            <p className="text-xs text-blue-600 dark:text-blue-400">
              Click or drop to change files
            </p>
          </div>
        ) : (
          <div className="space-y-2">
            <svg
              className="mx-auto h-10 w-10 text-gray-400 dark:text-gray-500"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={1.5}
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5"
              />
            </svg>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              Drop files here, or click to select
            </p>
            <p className="text-xs text-gray-400 dark:text-gray-500">Max 100MB total</p>
          </div>
        )}
      </div>
      {error && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
