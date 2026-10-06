import {
  type FormEvent,
  type KeyboardEvent,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { formatExpiration } from "../../lib/expiration";
import {
  fitsBundleManifestLimit,
  fitsBundleUploadLimit,
  MAX_UPLOAD_LABEL,
} from "../../lib/uploadLimits";
import Spinner from "../Spinner";
import Button from "../ui/Button";
import IconButton from "../ui/IconButton";
import { CheckIcon, PaperclipIcon, PlusIcon } from "../ui/icons";
import ProgressRow from "../ui/ProgressRow";
import { FOCUS } from "../ui/styles";
import TextButton from "../ui/TextButton";
import ChipMenu, { CHIP } from "./ChipMenu";
import FileList from "./FileList";

export type ComposeData = {
  expiration: string;
  burnAfterRead: boolean;
  /** Empty when the link needs no password. */
  password: string;
} & ({ kind: "text"; text: string } | { kind: "files"; files: File[] });

/** What the page is doing with the last submission, while the composer waits. */
export interface ComposeProgress {
  stage: "encrypting" | "uploading";
  /** 0..1, once the upload reports it. */
  fraction?: number;
  label?: string;
  onCancel?: () => void;
}

interface ComposerProps {
  onSubmit: (data: ComposeData) => void;
  busy: ComposeProgress | null;
}

const EXPIRATIONS = [
  ["5m", "5 min"],
  ["10m", "10 min"],
  ["15m", "15 min"],
  ["1h", "1 hour"],
  ["4h", "4 hours"],
  ["12h", "12 hours"],
  ["1d", "1 day"],
  ["3d", "3 days"],
  ["7d", "7 days"],
] as const;

const OPENS = [
  { once: true, label: "Once", detail: "The first visit shows it, then it is deleted." },
  { once: false, label: "Until it expires", detail: "Anyone with the link can open it again." },
] as const;

const OPTION = `rounded-[10px] text-left font-medium text-ink transition-colors duration-150 hover:bg-hover ${FOCUS}`;
const SELECTED = "ring-[1.5px] ring-ink ring-inset";

// navigator.platform is deprecated, but still the one value every browser fills in.
const SHORTCUT = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘ Enter" : "Ctrl+Enter";

/** Files already in the list are not added twice. */
function merge(current: File[], added: File[]): File[] {
  const next = [...current];
  for (const file of added) {
    if (!next.some((f) => f.name === file.name && f.size === file.size)) next.push(file);
  }
  return next;
}

/**
 * One box for a secret: text until files are attached, then the files. The
 * settings sit in the bar underneath, so nothing has to be visited before
 * the link can be made. Files can be dropped anywhere on the page.
 */
export default function Composer({ onSubmit, busy }: ComposerProps) {
  const [text, setText] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [fileError, setFileError] = useState<string | null>(null);
  const [expiration, setExpiration] = useState("1d");
  const [once, setOnce] = useState(true);
  const [passwordOn, setPasswordOn] = useState(false);
  const [password, setPassword] = useState("");
  const [passwordShown, setPasswordShown] = useState(false);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [menu, setMenu] = useState<"expires" | "opens" | null>(null);
  const [dragging, setDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const passwordId = useId();
  const passwordErrorId = useId();

  const disabled = busy !== null;
  const hasContent = files.length > 0 || text.trim().length > 0;
  const expirationLabel = EXPIRATIONS.find(([value]) => value === expiration)?.[1] ?? expiration;

  const addFiles = useCallback(
    (added: File[]) => {
      if (added.length === 0) return;
      const next = merge(files, added);
      if (!fitsBundleUploadLimit(next.map((file) => file.size))) {
        setFileError(`Together these files exceed the ${MAX_UPLOAD_LABEL} limit.`);
        return;
      }
      if (!fitsBundleManifestLimit(next)) {
        setFileError("Too many files for one link. Zip them first, or split them up.");
        return;
      }
      setFileError(null);
      setFiles(next);
    },
    [files],
  );

  // Dropping files anywhere on the page attaches them: the box is the only
  // target there is, so the whole page may as well be it.
  useEffect(() => {
    if (disabled) return;
    let depth = 0;
    const carriesFiles = (e: DragEvent) =>
      Array.from(e.dataTransfer?.types ?? []).includes("Files");
    function onDragEnter(e: DragEvent) {
      if (!carriesFiles(e)) return;
      depth++;
      setDragging(true);
    }
    function onDragOver(e: DragEvent) {
      if (carriesFiles(e)) e.preventDefault();
    }
    function onDragLeave(e: DragEvent) {
      if (!carriesFiles(e)) return;
      depth = Math.max(0, depth - 1);
      if (depth === 0) setDragging(false);
    }
    function onDrop(e: DragEvent) {
      if (!carriesFiles(e)) return;
      e.preventDefault();
      depth = 0;
      setDragging(false);
      addFiles(Array.from(e.dataTransfer?.files ?? []));
    }
    document.addEventListener("dragenter", onDragEnter);
    document.addEventListener("dragover", onDragOver);
    document.addEventListener("dragleave", onDragLeave);
    document.addEventListener("drop", onDrop);
    return () => {
      document.removeEventListener("dragenter", onDragEnter);
      document.removeEventListener("dragover", onDragOver);
      document.removeEventListener("dragleave", onDragLeave);
      document.removeEventListener("drop", onDrop);
    };
  }, [disabled, addFiles]);

  function submit() {
    if (disabled || !hasContent) return;
    if (passwordOn && password.length === 0) {
      setPasswordError("Add a password, or turn it off.");
      passwordRef.current?.focus();
      return;
    }
    const settings = { expiration, burnAfterRead: once, password: passwordOn ? password : "" };
    onSubmit(
      files.length > 0
        ? { kind: "files", files, ...settings }
        : { kind: "text", text, ...settings },
    );
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    submit();
  }

  function handleKeyDown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      submit();
    }
  }

  function togglePassword() {
    if (passwordOn) {
      setPassword("");
      setPasswordError(null);
    }
    setPasswordOn(!passwordOn);
  }

  return (
    <form onSubmit={handleSubmit} onKeyDown={handleKeyDown} className="space-y-3.5">
      <div
        className={`rounded-[20px] border bg-surface shadow-card transition-[border-color,box-shadow] duration-150 focus-within:border-focus focus-within:ring-4 focus-within:ring-ring ${
          dragging ? "border-focus ring-4 ring-ring" : "border-line"
        }`}
      >
        {files.length === 0 ? (
          <>
            <label htmlFor="secret-text" className="sr-only">
              Secret
            </label>
            <textarea
              id="secret-text"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="Paste a password, a key, a note…"
              rows={6}
              disabled={disabled}
              spellCheck={false}
              autoComplete="off"
              data-gramm="false"
              data-gramm_editor="false"
              data-enable-grammarly="false"
              data-1p-ignore
              className="block max-h-[60vh] min-h-46 w-full field-sizing-content resize-none rounded-t-[20px] bg-transparent px-6 pt-5.5 pb-3 font-mono text-body leading-[1.65] text-ink outline-none placeholder:text-faint"
            />
          </>
        ) : (
          <FileList
            files={files}
            disabled={disabled}
            onRemove={(index) => {
              setFileError(null);
              setFiles(files.filter((_, i) => i !== index));
            }}
          />
        )}
        {fileError && (
          <p role="alert" className="px-6 pb-3 text-sm text-danger">
            {fileError}
          </p>
        )}

        {passwordOn && (
          <div className="mx-2.5 border-t border-line pl-3.5 motion-safe:animate-drop">
            <div className="flex items-center gap-3 py-0.5">
              <label htmlFor={passwordId} className="text-sm font-medium text-muted">
                Password
              </label>
              <input
                ref={passwordRef}
                id={passwordId}
                type={passwordShown ? "text" : "password"}
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  if (e.target.value) setPasswordError(null);
                }}
                placeholder="Send it separately from the link"
                aria-invalid={passwordError ? true : undefined}
                aria-describedby={passwordError ? passwordErrorId : undefined}
                autoFocus
                autoComplete="new-password"
                spellCheck={false}
                disabled={disabled}
                data-gramm="false"
                data-gramm_editor="false"
                data-enable-grammarly="false"
                data-1p-ignore
                className="h-12 min-w-0 flex-1 bg-transparent font-mono text-body text-ink outline-none placeholder:text-faint"
              />
              <button
                type="button"
                onClick={() => setPasswordShown(!passwordShown)}
                aria-pressed={passwordShown}
                className={`h-11 shrink-0 rounded-[10px] px-3 text-[13px] font-medium text-muted transition-colors duration-150 hover:bg-hover hover:text-ink ${FOCUS}`}
              >
                {passwordShown ? "Hide" : "Show"}
              </button>
            </div>
            {passwordError && (
              <p id={passwordErrorId} className="pb-3 text-sm text-danger">
                {passwordError}
              </p>
            )}
          </div>
        )}

        {/* The chips wrap among themselves; the button keeps its place on the
            right, and on a phone takes a row of its own. */}
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 border-t border-line p-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5">
            <IconButton
              label="Attach files"
              disabled={disabled}
              onClick={() => fileInputRef.current?.click()}
            >
              <PaperclipIcon />
            </IconButton>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => {
                addFiles(Array.from(e.target.files ?? []));
                e.target.value = "";
              }}
            />

            <ChipMenu
              name={`Expires in ${formatExpiration(expiration)}`}
              label={
                <>
                  <span className="max-sm:hidden">Expires in</span>
                  <span className="text-ink">{expirationLabel}</span>
                </>
              }
              open={menu === "expires"}
              disabled={disabled}
              onToggle={() => setMenu(menu === "expires" ? null : "expires")}
              onClose={() => setMenu(null)}
            >
              <fieldset className="m-0 min-w-0 border-0 p-0">
                <legend className="px-2 pt-1.5 pb-2 text-xs font-medium text-faint">
                  Expires after
                </legend>
                <div className="grid grid-cols-3 gap-1">
                  {EXPIRATIONS.map(([value, label]) => (
                    <button
                      key={value}
                      type="button"
                      aria-pressed={value === expiration}
                      aria-label={formatExpiration(value)}
                      onClick={() => {
                        setExpiration(value);
                        setMenu(null);
                      }}
                      className={`min-h-11 px-1.5 text-center text-sm ${OPTION} ${
                        value === expiration ? `font-semibold ${SELECTED}` : "text-muted"
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </fieldset>
            </ChipMenu>

            <ChipMenu
              name={once ? "Opens once" : "Opens until it expires"}
              label={
                <>
                  <span>Opens</span>
                  <span className="text-ink">{once ? "once" : "until it expires"}</span>
                </>
              }
              open={menu === "opens"}
              disabled={disabled}
              onToggle={() => setMenu(menu === "opens" ? null : "opens")}
              onClose={() => setMenu(null)}
            >
              <fieldset className="m-0 min-w-0 border-0 p-0">
                <legend className="px-2 pt-1.5 pb-2 text-xs font-medium text-faint">
                  The link opens
                </legend>
                <div className="flex flex-col gap-1">
                  {OPENS.map((option) => (
                    <button
                      key={option.label}
                      type="button"
                      aria-pressed={option.once === once}
                      onClick={() => {
                        setOnce(option.once);
                        setMenu(null);
                      }}
                      className={`flex min-h-13 w-full items-center gap-3 px-3 py-2 ${OPTION} ${
                        option.once === once ? SELECTED : ""
                      }`}
                    >
                      <span className="flex flex-1 flex-col gap-px">
                        <span className="text-sm">{option.label}</span>
                        <span className="text-[13px] font-normal text-muted">{option.detail}</span>
                      </span>
                      {option.once === once && <CheckIcon />}
                    </button>
                  ))}
                </div>
              </fieldset>
            </ChipMenu>

            <button
              type="button"
              aria-pressed={passwordOn}
              disabled={disabled}
              onClick={togglePassword}
              className={`${CHIP} pl-2.5${passwordOn ? " bg-hover text-ink" : ""}`}
            >
              {passwordOn ? <CheckIcon /> : <PlusIcon />}
              Password
            </button>
          </div>

          <Button
            type="submit"
            disabled={disabled || !hasContent}
            title={`Or press ${SHORTCUT}`}
            className="max-sm:w-full"
          >
            {busy && <Spinner size="sm" />}
            {busy ? "Creating link…" : "Create link"}
          </Button>
        </div>

        {busy && (
          <ProgressRow
            label={
              busy.stage === "encrypting"
                ? "Encrypting…"
                : busy.label
                  ? `Uploading · ${busy.label}`
                  : "Uploading…"
            }
            fraction={busy.fraction}
            action={
              busy.onCancel && (
                <TextButton tone="danger" onClick={busy.onCancel}>
                  Cancel upload
                </TextButton>
              )
            }
          />
        )}
      </div>

      {/* Only where there is something to drag with. */}
      <p className="hidden px-1 text-[13px] text-faint pointer-fine:block">
        {dragging
          ? "Drop to attach."
          : files.length > 0
            ? "Drop more files anywhere on the page."
            : "Or drop files anywhere on the page."}
      </p>
    </form>
  );
}
