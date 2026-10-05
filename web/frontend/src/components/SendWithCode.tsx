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
 * Shows a short code the receiver types at /c on its own device. The link
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
    <div className="space-y-4.5 border-t border-line bg-sunken px-6 pt-6 pb-5">
      <div role="status" className="space-y-4.5">
        {state.stage === "starting" && (
          <p className="flex items-center gap-3 text-body text-muted">
            <Spinner size="sm" className="text-accent" /> Getting a code…
          </p>
        )}
        {state.stage === "waiting" && (
          <>
            <p className="text-pretty text-body text-muted">
              On the other device, go to{" "}
              <span className="break-all font-mono text-sm text-ink">{window.location.host}/c</span>{" "}
              and type:
            </p>
            <p className="break-all font-mono text-[clamp(1.75rem,3.4vw,2.375rem)] font-medium leading-[1.1] tracking-[-0.02em] text-ink">
              {state.code}
            </p>
            <p className="flex items-center gap-3 text-sm text-muted">
              <span
                aria-hidden="true"
                className="h-2 w-2 shrink-0 rounded-full bg-accent ring-4 ring-ring"
              />
              Waiting for the other device · {formatRemaining(state.expiresAt - now)} left
            </p>
          </>
        )}
        {state.stage === "sent" && (
          <p className="text-body font-medium text-ink">
            Sent. The other device is opening the secret.
          </p>
        )}
        {state.stage === "failed" && <p className="text-body text-ink">{state.message}</p>}
      </div>

      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <p className="text-xs text-faint">
          Code words from the{" "}
          <a
            href="https://www.eff.org/dice"
            target="_blank"
            rel="noreferrer"
            className="underline underline-offset-2 transition-colors duration-150 hover:text-ink"
          >
            EFF short word list
          </a>{" "}
          (CC BY 3.0)
        </p>
        <div className="-mr-3 flex gap-1">
          {state.stage === "failed" && (
            <Button size="sm" onClick={() => setAttempt((n) => n + 1)}>
              New code
            </Button>
          )}
          <Button variant="quiet" size="sm" onClick={onClose}>
            {state.stage === "sent" ? "Done" : "Stop"}
          </Button>
        </div>
      </div>
    </div>
  );
}
