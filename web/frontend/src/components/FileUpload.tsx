import { useCallback, useRef, useState } from "react";
import { formatSize } from "../lib/format";
import {
  fitsBundleUploadLimit,
  MAX_FILE_UPLOAD_BYTES,
  MAX_UPLOAD_LABEL,
} from "../lib/uploadLimits";

interface FileUploadProps {
  onSelect: (files: File[]) => void;
}

function FileIcon({ name }: { name: string }) {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  let label: string;
  let color: string;

  if (ext === "pdf") {
    label = "PDF";
    color = "text-red-400";
  } else if (["jpg", "jpeg", "png", "gif", "webp", "svg", "bmp"].includes(ext)) {
    label = "IMG";
    color = "text-blue-400";
  } else if (["doc", "docx"].includes(ext)) {
    label = "DOC";
    color = "text-blue-500";
  } else if (["xls", "xlsx", "csv"].includes(ext)) {
    label = "XLS";
    color = "text-green-500";
  } else if (["zip", "tar", "gz", "rar", "7z"].includes(ext)) {
    label = "ZIP";
    color = "text-amber-500";
  } else if (["mp4", "mov", "avi", "mkv"].includes(ext)) {
    label = "VID";
    color = "text-purple-400";
  } else if (["mp3", "wav", "flac", "aac"].includes(ext)) {
    label = "AUD";
    color = "text-pink-400";
  } else {
    label = ext.toUpperCase().slice(0, 3) || "FILE";
    color = "text-zinc-400";
  }

  return (
    <span className={`text-[10px] font-semibold uppercase ${color} w-7 text-center flex-shrink-0`}>
      {label}
    </span>
  );
}

export default function FileUpload({ onSelect }: FileUploadProps) {
  const [dragOver, setDragOver] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  const updateFiles = useCallback(
    (files: File[]) => {
      setError("");
      const total = files.reduce((sum, f) => sum + f.size, 0);
      if (total > MAX_FILE_UPLOAD_BYTES || !fitsBundleUploadLimit(files.map((file) => file.size))) {
        setError(`Total file size exceeds the ${MAX_UPLOAD_LABEL} upload limit.`);
        return;
      }
      setSelectedFiles(files);
      onSelect(files);
    },
    [onSelect],
  );

  const addFiles = useCallback(
    (newFiles: File[]) => {
      const merged = [...selectedFiles];
      for (const f of newFiles) {
        if (!merged.some((m) => m.name === f.name && m.size === f.size)) {
          merged.push(f);
        }
      }
      updateFiles(merged);
    },
    [selectedFiles, updateFiles],
  );

  const removeFile = useCallback(
    (index: number) => {
      const next = selectedFiles.filter((_, i) => i !== index);
      setError("");
      setSelectedFiles(next);
      onSelect(next);
    },
    [selectedFiles, onSelect],
  );

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    setDragOver(false);
    const files = Array.from(e.dataTransfer.files);
    if (files.length > 0) {
      selectedFiles.length > 0 ? addFiles(files) : updateFiles(files);
    }
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files ?? []);
    if (files.length > 0) {
      selectedFiles.length > 0 ? addFiles(files) : updateFiles(files);
    }
    e.target.value = "";
  }

  const totalSize = selectedFiles.reduce((s, f) => s + f.size, 0);
  const usagePercent = Math.min((totalSize / MAX_FILE_UPLOAD_BYTES) * 100, 100);

  return (
    <div>
      <input ref={inputRef} type="file" multiple onChange={handleChange} className="hidden" />

      {selectedFiles.length === 0 ? (
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
          className={`cursor-pointer rounded-lg border-2 border-dashed p-8 min-h-[168px] flex flex-col items-center justify-center text-center transition-all duration-150 focus:outline-none focus:border-amber-400 ${
            dragOver
              ? "border-amber-400 bg-amber-400/5"
              : "border-zinc-200 dark:border-zinc-600 hover:border-zinc-300 dark:hover:border-zinc-600"
          }`}
        >
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
                Drop files here, or{" "}
                <span className="text-amber-500 dark:text-amber-400">click to select</span>
              </p>
              <p className="text-xs text-zinc-500 dark:text-zinc-100 mt-1">
                Max {MAX_UPLOAD_LABEL} total
              </p>
            </div>
          </div>
        </div>
      ) : (
        <div
          role="region"
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={handleDrop}
          className={`rounded-lg border-2 border-dashed p-4 transition-all duration-150 ${
            dragOver
              ? "border-amber-400 bg-amber-400/5"
              : "border-zinc-300 dark:border-zinc-500 bg-white dark:bg-zinc-900"
          }`}
        >
          <div className="space-y-0.5">
            {selectedFiles.map((f, i) => (
              <div
                key={`${f.name}-${f.size}-${i}`}
                className="flex items-center gap-2 py-1.5 px-1 rounded text-xs group hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors duration-100"
              >
                <FileIcon name={f.name} />
                <span
                  className="font-medium text-zinc-700 dark:text-zinc-100 truncate flex-1"
                  title={f.name}
                >
                  {f.name}
                </span>
                <span className="text-zinc-500 dark:text-zinc-400 flex-shrink-0 tabular-nums">
                  {formatSize(f.size)}
                </span>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    removeFile(i);
                  }}
                  className="ml-0.5 p-0.5 rounded text-zinc-400 hover:text-red-400 hover:bg-red-400/10 transition-colors duration-150 opacity-0 group-hover:opacity-100 focus:opacity-100"
                  aria-label={`Remove ${f.name}`}
                >
                  <svg
                    className="h-3.5 w-3.5"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    strokeWidth={2}
                    aria-hidden="true"
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            ))}
          </div>

          {/* Size usage bar */}
          <div className="mt-3 pt-3 border-t border-zinc-200 dark:border-zinc-600">
            <div className="flex justify-between text-xs text-zinc-500 dark:text-zinc-400 mb-1.5">
              <span>
                {selectedFiles.length} {selectedFiles.length === 1 ? "file" : "files"}
              </span>
              <span>
                {formatSize(totalSize)} / {MAX_UPLOAD_LABEL}
              </span>
            </div>
            <div className="h-1 rounded-full bg-zinc-200 dark:bg-zinc-700 overflow-hidden">
              <div
                className={`h-full rounded-full transition-all duration-300 ${
                  usagePercent > 90 ? "bg-red-400" : "bg-amber-400"
                }`}
                style={{ width: `${usagePercent}%` }}
              />
            </div>
          </div>

          {/* Add more files */}
          <div className="mt-3 text-center">
            <button
              type="button"
              onClick={() => inputRef.current?.click()}
              className="text-xs text-amber-500 dark:text-amber-400 hover:text-amber-600 dark:hover:text-amber-300 transition-colors duration-150"
            >
              + Add more files
            </button>
            <span className="text-xs text-zinc-400 dark:text-zinc-500 ml-2">or drop here</span>
          </div>
        </div>
      )}

      {error && <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{error}</p>}
    </div>
  );
}
