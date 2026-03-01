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
        className={`cursor-pointer rounded-lg border-2 border-dashed p-8 h-[168px] flex flex-col items-center justify-center text-center transition-all duration-150 focus:outline-none focus:border-amber-400 ${
          dragOver
            ? "border-amber-400 bg-amber-400/5 dark:bg-amber-400/5"
            : selectedFiles.length > 0
              ? "border-zinc-300 dark:border-zinc-500 bg-white dark:bg-zinc-900"
              : "border-zinc-200 dark:border-zinc-600 hover:border-zinc-300 dark:hover:border-zinc-600"
        }`}
      >
        <input
          ref={inputRef}
          type="file"
          multiple
          onChange={handleChange}
          className="hidden"
        />

        {selectedFiles.length === 0 ? (
          <div className="space-y-3">
            <div className="mx-auto w-10 h-10 rounded-full border border-zinc-200 dark:border-zinc-600 flex items-center justify-center">
              <svg
                className="h-5 w-5 text-zinc-500 dark:text-zinc-100"
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
            </div>
            <div>
              <p className="text-sm text-zinc-600 dark:text-zinc-100">
                Drop files here, or <span className="text-amber-500 dark:text-amber-400">click to select</span>
              </p>
              <p className="text-xs text-zinc-500 dark:text-zinc-100 mt-1">Max 100MB total</p>
            </div>
          </div>
        ) : selectedFiles.length === 1 ? (
          <div className="space-y-1">
            <div className="mx-auto w-10 h-10 rounded-full border border-zinc-200 dark:border-zinc-500 flex items-center justify-center mb-3">
              <svg className="h-5 w-5 text-zinc-600 dark:text-zinc-100" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 002.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 00-1.123-.08m-5.801 0c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 00.75-.75 2.25 2.25 0 00-.1-.664m-5.8 0A2.251 2.251 0 0113.5 2.25H15c1.012 0 1.867.668 2.15 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m0 0H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V9.375c0-.621-.504-1.125-1.125-1.125H8.25zM6.75 12h.008v.008H6.75V12zm0 3h.008v.008H6.75V15zm0 3h.008v.008H6.75V18z" />
              </svg>
            </div>
            <p className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{selectedFiles[0].name}</p>
            <p className="text-xs text-zinc-500 dark:text-zinc-100">{formatSize(selectedFiles[0].size)}</p>
            <p className="text-xs text-amber-500 dark:text-amber-400 mt-2">Click or drop to change</p>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="divide-y divide-zinc-200 dark:divide-zinc-600 text-left mx-auto max-w-xs">
              {selectedFiles.map((f, i) => (
                <div key={i} className="flex justify-between py-1.5 text-xs">
                  <span className="font-medium text-zinc-700 dark:text-zinc-100 truncate pr-4">{f.name}</span>
                  <span className="text-zinc-500 dark:text-zinc-100 flex-shrink-0">{formatSize(f.size)}</span>
                </div>
              ))}
              <div className="flex justify-between pt-1.5 text-xs text-zinc-500 dark:text-zinc-100">
                <span>{selectedFiles.length} files</span>
                <span>{formatSize(selectedFiles.reduce((s, f) => s + f.size, 0))}</span>
              </div>
            </div>
            <p className="text-xs text-amber-500 dark:text-amber-400">Click or drop to change</p>
          </div>
        )}
      </div>
      {error && <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{error}</p>}
    </div>
  );
}
