import { type FormEvent, type KeyboardEvent, useEffect, useRef, useState } from "react";
import { parseShareLink } from "../../lib/shareLink";
import Button from "../ui/Button";

interface EnterCodeProps {
  /** Called with the fragment of the share link the code delivered. */
  onReceived: (fragment: string) => void;
  onCancel: () => void;
}

type Status =
  | { busy: false; error: string | null }
  | { busy: true; stage: "connecting" | "opening" };

type Completer = (typed: string) => string | null;

const SEPARATOR = /[\s.\-_]+/;
const ENDS_WITH_SEPARATOR = /[\s.\-_]$/;

const digits = (s: string) => s.replace(/\D/g, "").slice(0, 3);
const letters = (s: string) => s.toLowerCase().replace(/[^a-z]/g, "");

/** A whole code typed or pasted into one field, split into its three parts. */
function split(raw: string): [string, string, string] | null {
  const parts = raw.trim().split(SEPARATOR).filter(Boolean);
  if (parts.length < 2 || !/^\d/.test(parts[0])) return null;
  return [digits(parts[0]), letters(parts[1]), letters(parts[2] ?? "")];
}

const FIELD = `h-15 rounded-[14px] border border-line font-mono text-[22px] text-ink outline-none transition-[border-color,box-shadow] duration-150 focus:border-focus focus:ring-4 focus:ring-ring disabled:opacity-60 max-sm:h-14 max-sm:text-[19px]`;
const GHOST =
  "pointer-events-none absolute inset-0 flex items-center overflow-hidden whitespace-pre px-[17px] font-mono text-[22px] max-sm:px-[13px] max-sm:text-[19px]";

/**
 * Receives a share link by typing the short code shown on the sending
 * device: a number and two words, one field each. Three letters complete a
 * word; a whole code pasted into any field spreads over all three. The code
 * is checked locally first, so a typo never uses up the transfer.
 */
export default function EnterCode({ onReceived, onCancel }: EnterCodeProps) {
  const [number, setNumber] = useState("");
  const [words, setWords] = useState<[string, string]>(["", ""]);
  const [status, setStatus] = useState<Status>({ busy: false, error: null });
  const [complete, setComplete] = useState<Completer | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const numberRef = useRef<HTMLInputElement>(null);
  const wordRefs = [useRef<HTMLInputElement>(null), useRef<HTMLInputElement>(null)];

  useEffect(() => () => abortRef.current?.abort(), []);

  // The word list stays out of the main bundle; it arrives while the first
  // letters are being typed.
  useEffect(() => {
    let active = true;
    import("../../lib/transferWords").then((m) => {
      if (active) setComplete(() => m.completeWord);
    });
    return () => {
      active = false;
    };
  }, []);

  const busy = status.busy;
  const filled = number !== "" && words[0] !== "" && words[1] !== "";

  function ghost(typed: string): string {
    if (!complete || typed.length < 3) return "";
    const full = complete(typed);
    return full && full !== typed && full.startsWith(typed) ? full.slice(typed.length) : "";
  }

  function setWord(index: 0 | 1, value: string) {
    setWords((current) => (index === 0 ? [value, current[1]] : [current[0], value]));
  }

  /** Fills all three fields and puts the cursor where typing would continue. */
  function spread(parts: [string, string, string], raw: string) {
    setNumber(parts[0]);
    setWords([parts[1], parts[2]]);
    const next = parts[2] || ENDS_WITH_SEPARATOR.test(raw) ? wordRefs[1] : wordRefs[0];
    next.current?.focus();
  }

  function handleNumberChange(raw: string) {
    const parts = split(raw);
    if (parts) {
      spread(parts, raw);
      return;
    }
    setNumber(digits(raw));
    if (ENDS_WITH_SEPARATOR.test(raw)) wordRefs[0].current?.focus();
  }

  function handleWordChange(index: 0 | 1, raw: string) {
    const parts = split(raw);
    if (parts) {
      spread(parts, raw);
      return;
    }
    const typed = letters(raw);
    if (ENDS_WITH_SEPARATOR.test(raw)) {
      setWord(index, complete?.(typed) ?? typed);
      if (index === 0) wordRefs[1].current?.focus();
      return;
    }
    setWord(index, typed);
  }

  /** Backspace in an empty field steps back to the one before it. */
  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>, previous: HTMLInputElement | null) {
    if (e.key === "Backspace" && e.currentTarget.value === "" && previous) {
      e.preventDefault();
      previous.focus();
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy || !filled) return;
    const controller = new AbortController();
    abortRef.current = controller;
    setStatus({ busy: true, stage: "connecting" });

    // Loaded on demand: the CPace code stays out of the main bundle.
    const session = await import("../../lib/transferSession");
    try {
      const code = `${number}-${words[0]}-${words[1]}`;
      const link = await session.receiveWithCode(code, controller.signal);
      const share = parseShareLink(link, window.location.origin);
      if (share.kind !== "share") {
        setStatus({ busy: false, error: "That code delivered a link for another site." });
        return;
      }
      setStatus({ busy: true, stage: "opening" });
      onReceived(share.fragment);
    } catch (err) {
      if (controller.signal.aborted) return;
      setStatus({ busy: false, error: session.describeReceiveError(err) });
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-8">
      <fieldset className="m-0 min-w-0 border-0 p-0">
        <legend className="mb-2.5 px-1 text-sm font-medium text-muted">Code</legend>
        <div className="grid grid-cols-[68px_auto_minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-2 max-sm:grid-cols-[56px_auto_minmax(0,1fr)_auto_minmax(0,1fr)] max-sm:gap-1.5">
          <input
            ref={numberRef}
            aria-label="Number"
            type="text"
            inputMode="numeric"
            value={number}
            onChange={(e) => handleNumberChange(e.target.value)}
            placeholder="7"
            autoFocus
            autoComplete="off"
            disabled={busy}
            className={`${FIELD} block w-full bg-surface px-0 text-center placeholder:text-faint`}
          />
          {([0, 1] as const).map((index) => (
            <div key={index} className="contents">
              <span
                aria-hidden="true"
                className="font-mono text-[22px] text-faint max-sm:text-[19px]"
              >
                -
              </span>
              <div className="relative min-w-0 rounded-[14px] bg-surface">
                <span aria-hidden="true" className={GHOST}>
                  <span className="text-transparent">{words[index]}</span>
                  <span
                    className="text-faint"
                    data-testid={`ghost-${index === 0 ? "first" : "second"}`}
                  >
                    {ghost(words[index])}
                  </span>
                </span>
                <input
                  ref={wordRefs[index]}
                  aria-label={index === 0 ? "First word" : "Second word"}
                  type="text"
                  value={words[index]}
                  onChange={(e) => handleWordChange(index, e.target.value)}
                  onBlur={() => setWord(index, complete?.(words[index]) ?? words[index])}
                  onKeyDown={(e) =>
                    handleKeyDown(e, (index === 0 ? numberRef : wordRefs[0]).current)
                  }
                  placeholder="word"
                  autoComplete="off"
                  autoCapitalize="none"
                  spellCheck={false}
                  disabled={busy}
                  className={`${FIELD} relative block w-full bg-transparent px-4 placeholder:text-faint max-sm:px-3`}
                />
              </div>
            </div>
          ))}
        </div>
      </fieldset>

      <div className="flex flex-wrap items-center gap-x-5 gap-y-3">
        <div className="flex gap-1">
          <Button type="submit" size="lg" disabled={busy || !filled}>
            {busy ? "Receiving…" : "Receive"}
          </Button>
          <Button variant="quiet" size="lg" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
        </div>
        <div role="status" className="text-sm">
          {status.busy && (
            <p className="flex items-center gap-3 text-muted">
              <span
                aria-hidden="true"
                className="h-2 w-2 shrink-0 rounded-full bg-accent ring-4 ring-ring"
              />
              {status.stage === "connecting"
                ? "Connecting to the other device…"
                : "Code verified. Opening the secret…"}
            </p>
          )}
          {!status.busy && status.error && <p className="text-danger">{status.error}</p>}
        </div>
      </div>

      <p className="text-pretty text-sm text-faint">
        No code yet? The sender gets one with “Send with a code” under their link.
      </p>
    </form>
  );
}
