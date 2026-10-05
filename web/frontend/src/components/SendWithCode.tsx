import { useEffect, useState } from "react";
import Spinner from "./Spinner";
import Button from "./ui/Button";

type State =
  | { stage: "starting" }
  | { stage: "waiting"; code: string; expiresAt: number }
  | { stage: "sent" }
  | { stage: "failed"; message: string };

interface SendWithCodeProps {
  url: string;
  onClose: () => void;
}

function formatRemaining(ms: number): string {
  const seconds = Math.max(0, Math.ceil(ms / 1000));
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

/**
 * Shows a short code the receiver types on its Retrieve page. The link
 * travels encrypted through the relay once the receiver proves it typed the
 * same code; the code's words never leave the two browsers.
 */
export default function SendWithCode({ url, onClose }: SendWithCodeProps) {
  const [state, setState] = useState<State>({ stage: "starting" });
  const [attempt, setAttempt] = useState(0);
  const [now, setNow] = useState(() => Date.now());

  // biome-ignore lint/correctness/useExhaustiveDependencies: attempt restarts the transfer on purpose
  useEffect(() => {
    let cancel = () => {};
    let active = true;
    setState({ stage: "starting" });

    (async () => {
      // Loaded on demand: the CPace code and word list stay out of the main bundle.
      const session = await import("../lib/transferSession");
      try {
        const transfer = await session.startSending(url);
        if (!active) {
          transfer.cancel();
          return;
        }
        cancel = transfer.cancel;
        setState({ stage: "waiting", code: transfer.code, expiresAt: transfer.expiresAt });
        await transfer.done;
        if (active) setState({ stage: "sent" });
      } catch (err) {
        if (active) setState({ stage: "failed", message: session.describeSendError(err) });
      }
    })().catch(() => {
      if (active)
        setState({ stage: "failed", message: "The transfer failed. Start again for a new code." });
    });

    const cancelOnLeave = () => cancel();
    window.addEventListener("pagehide", cancelOnLeave);
    return () => {
      active = false;
      window.removeEventListener("pagehide", cancelOnLeave);
      cancel();
    };
  }, [url, attempt]);

  useEffect(() => {
    if (state.stage !== "waiting") return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [state.stage]);

  return (
    <div className="space-y-4 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-700 dark:bg-zinc-950">
      <div role="status" className="space-y-3">
        {state.stage === "starting" && (
          <p className="flex items-center gap-2 text-sm text-zinc-600 dark:text-zinc-300">
            <Spinner size="sm" /> Getting a code…
          </p>
        )}
        {state.stage === "waiting" && (
          <>
            <p className="text-sm text-zinc-600 dark:text-zinc-300">
              On the other device, open Retrieve, choose <strong>Enter a code</strong> and type:
            </p>
            <p className="break-all text-center font-mono text-3xl font-semibold tracking-wide text-zinc-900 dark:text-zinc-50">
              {state.code}
            </p>
            <p className="flex items-center justify-center gap-2 text-xs text-zinc-500 dark:text-zinc-400">
              <Spinner size="sm" /> Waiting for the other device · expires in{" "}
              {formatRemaining(state.expiresAt - now)}
            </p>
          </>
        )}
        {state.stage === "sent" && (
          <p className="text-sm font-medium text-emerald-700 dark:text-emerald-400">
            Sent. The other device is opening the share.
          </p>
        )}
        {state.stage === "failed" && (
          <p className="text-sm text-zinc-700 dark:text-zinc-200">{state.message}</p>
        )}
      </div>

      <div className="flex items-center justify-between gap-3">
        <p className="text-[11px] text-zinc-500 dark:text-zinc-400">
          Code words from the{" "}
          <a
            href="https://www.eff.org/dice"
            target="_blank"
            rel="noreferrer"
            className="underline hover:text-amber-700 dark:hover:text-amber-400"
          >
            EFF short word list
          </a>{" "}
          (CC BY 3.0)
        </p>
        <div className="flex gap-2">
          {state.stage === "failed" && (
            <Button size="sm" onClick={() => setAttempt((n) => n + 1)}>
              New code
            </Button>
          )}
          <Button variant="secondary" size="sm" onClick={onClose}>
            {state.stage === "sent" ? "Done" : "Cancel"}
          </Button>
        </div>
      </div>
    </div>
  );
}
