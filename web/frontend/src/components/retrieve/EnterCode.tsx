import { useEffect, useRef, useState } from "react";
import { parseShareLink } from "../../lib/shareLink";
import Spinner from "../Spinner";
import Button from "../ui/Button";

interface EnterCodeProps {
  /** Called with the fragment of the share link the code delivered. */
  onReceived: (fragment: string) => void;
  onCancel: () => void;
}

type Status = { busy: false; message: string | null } | { busy: true; message: string };

/**
 * Receives a share link by typing the short code shown on the sending
 * device. The code is checked locally first, so a typo never uses up the
 * transfer.
 */
export default function EnterCode({ onReceived, onCancel }: EnterCodeProps) {
  const [code, setCode] = useState("");
  const [status, setStatus] = useState<Status>({ busy: false, message: null });
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => () => abortRef.current?.abort(), []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (status.busy) return;
    const controller = new AbortController();
    abortRef.current = controller;
    setStatus({ busy: true, message: "Checking the code…" });

    // Loaded on demand: the CPace code and word list stay out of the main bundle.
    const session = await import("../../lib/transferSession");
    try {
      const link = await session.receiveWithCode(code, controller.signal);
      const share = parseShareLink(link, window.location.origin);
      if (share.kind !== "share") {
        setStatus({ busy: false, message: "The received link isn't a share on this site." });
        return;
      }
      setStatus({ busy: true, message: "Opening the share…" });
      onReceived(share.fragment);
    } catch (err) {
      if (controller.signal.aborted) return;
      setStatus({ busy: false, message: session.describeReceiveError(err) });
    }
  }

  return (
    <section
      aria-label="Enter a code"
      className="space-y-3 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-500/50 dark:bg-zinc-900"
    >
      <form onSubmit={handleSubmit} className="space-y-3">
        <label htmlFor="transfer-code" className="block text-sm text-zinc-700 dark:text-zinc-100">
          Type the code shown on the other device. Three letters per word are enough.
        </label>
        <input
          id="transfer-code"
          type="text"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          placeholder="7-acid-rocket"
          autoFocus
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          disabled={status.busy}
          className="w-full rounded-lg border border-zinc-200 bg-white px-4 py-3 font-mono text-sm text-zinc-900 placeholder:text-zinc-500 transition-colors duration-150 focus:border-amber-400 focus:outline-none focus:ring-1 focus:ring-amber-400/20 dark:border-zinc-500/50 dark:bg-zinc-800 dark:text-zinc-100"
        />
        <div role="status" className="min-h-5 text-sm text-zinc-700 dark:text-zinc-200">
          {status.message && (
            <p className="flex items-center gap-2">
              {status.busy && <Spinner size="sm" />}
              {status.message}
            </p>
          )}
        </div>
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="submit" disabled={status.busy || code.trim() === ""}>
            Receive share
          </Button>
        </div>
      </form>
    </section>
  );
}
