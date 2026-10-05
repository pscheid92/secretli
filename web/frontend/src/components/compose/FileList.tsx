import { formatSize } from "../../lib/format";
import IconButton from "../ui/IconButton";
import { CloseIcon, FileIcon } from "../ui/icons";

interface FileListProps {
  files: File[];
  disabled?: boolean;
  onRemove: (index: number) => void;
}

/** The files waiting in the composer, and what they add up to. */
export default function FileList({ files, disabled, onRemove }: FileListProps) {
  const total = files.reduce((sum, file) => sum + file.size, 0);
  return (
    <div className="px-2.5 pt-2.5">
      <ul className="m-0 list-none p-0">
        {files.map((file, index) => (
          <li
            // biome-ignore lint/suspicious/noArrayIndexKey: rows are stateless and the index disambiguates files sharing name and size
            key={`${file.name}-${file.size}-${index}`}
            className="flex min-h-13 items-center gap-3 pr-0.5 pl-3.5"
          >
            <span className="flex text-faint">
              <FileIcon />
            </span>
            <span className="min-w-0 flex-1 truncate font-mono text-sm text-ink" title={file.name}>
              {file.name}
            </span>
            <span className="text-[13px] tabular-nums text-faint">{formatSize(file.size)}</span>
            <IconButton
              label={`Remove ${file.name}`}
              disabled={disabled}
              onClick={() => onRemove(index)}
            >
              <CloseIcon />
            </IconButton>
          </li>
        ))}
      </ul>
      <p className="px-3.5 pt-1.5 pb-3 text-[13px] text-faint">
        {files.length} {files.length === 1 ? "file" : "files"} · {formatSize(total)} · encrypted
        before upload
      </p>
    </div>
  );
}
