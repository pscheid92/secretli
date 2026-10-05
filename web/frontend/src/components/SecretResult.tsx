import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { toast } from "sonner";
import { formatRelativeTime } from "../lib/format";
import QRCode from "./QRCode";
import SendWithCode from "./SendWithCode";
import Button from "./ui/Button";
import { CopyIcon, KeyboardIcon, KeyIcon, QrCodeIcon, ShareIcon, WarningIcon } from "./ui/icons";
import TextButton from "./ui/TextButton";

interface SecretResultProps {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  passwordProtected: boolean;
  deletionToken: string;
}

/** How long the copy button says "Copied". */
const COPIED_MS = 2000;

/**
 * Copies text to the clipboard. Where the browser refuses, the text is
 * selected in its field instead, so it can be copied by hand.
 */
async function copyText(text: string, field: HTMLInputElement | null): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    field?.focus();
    field?.select();
    toast.error("Couldn't copy automatically. The link is selected: copy it with Ctrl+C or ⌘C.");
    return false;
  }
}

/** Whether the browser can hand the link to the system share sheet. */
function canShare(url: string): boolean {
  return typeof navigator.share === "function" && (navigator.canShare?.({ url }) ?? true);
}

async function shareLink(url: string) {
  try {
    await navigator.share({ url });
  } catch (err) {
    // Closing the share sheet is not a failure.
    if (err instanceof DOMException && err.name === "AbortError") return;
    toast.error("Sharing didn't work. Copy the link instead.");
  }
}

function Notice({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-amber-800 dark:border-amber-900/40 dark:bg-amber-900/10 dark:text-amber-400">
      <span className="mt-0.5">{icon}</span>
      <p className="text-xs leading-relaxed">{children}</p>
    </div>
  );
}

function StatusRow({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="py-3">
      <div className="text-xs text-zinc-500 dark:text-zinc-400">{label}</div>
      <div className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">{value}</div>
      {detail && <div className="text-xs text-zinc-500 dark:text-zinc-400">{detail}</div>}
    </div>
  );
}

export default function SecretResult({
  url,
  expiresAt,
  burnAfterRead,
  passwordProtected,
  deletionToken,
}: SecretResultProps) {
  const ownerUrl = `${url}!${deletionToken}`;
  const [copied, setCopied] = useState(false);
  const [showQR, setShowQR] = useState(false);
  const [sendingCode, setSendingCode] = useState(false);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const linkRef = useRef<HTMLInputElement>(null);
  const ownerRef = useRef<HTMLInputElement>(null);
  const qrRef = useRef<HTMLDivElement>(null);
  const linkId = useId();
  const ownerHeadingId = useId();

  // The form was replaced by this page: move focus along, so keyboard and
  // screen reader users land on the result.
  useEffect(() => {
    headingRef.current?.focus();
  }, []);

  // On shorter screens the code reaches below the fold.
  useEffect(() => {
    if (showQR) qrRef.current?.scrollIntoView({ block: "nearest" });
  }, [showQR]);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), COPIED_MS);
    return () => clearTimeout(timer);
  }, [copied]);

  async function copyShareUrl() {
    if (await copyText(url, linkRef.current)) {
      setCopied(true);
      toast.success("Link copied");
    }
  }

  async function copyOwnerUrl() {
    if (await copyText(ownerUrl, ownerRef.current)) toast.success("Owner link copied");
  }

  const expires = new Date(expiresAt);

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start">
      <section className="space-y-6 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-emerald-400" />
            <span className="text-sm font-medium text-emerald-700 dark:text-emerald-400">
              Secure link created
            </span>
          </div>
          <h1
            ref={headingRef}
            tabIndex={-1}
            className="font-display text-2xl font-semibold text-zinc-800 focus:outline-none dark:text-zinc-100"
          >
            Share is ready
          </h1>
          <p className="text-sm text-zinc-600 dark:text-zinc-300">
            Send this link to the recipient.
          </p>
        </div>

        <div className="space-y-3">
          <label
            htmlFor={linkId}
            className="block text-xs font-medium text-zinc-500 dark:text-zinc-400"
          >
            Recipient link
          </label>
          <input
            ref={linkRef}
            id={linkId}
            type="text"
            readOnly
            value={url}
            onFocus={(e) => e.currentTarget.select()}
            className="w-full min-w-0 rounded-lg border border-zinc-200 bg-zinc-50 px-4 py-3 font-mono text-xs text-zinc-700 transition-colors duration-150 focus:border-amber-400 focus:outline-none focus:ring-1 focus:ring-amber-400/20 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100"
          />
          <div className="flex flex-wrap gap-3">
            <Button size="lg" onClick={copyShareUrl} className="flex-1 sm:min-w-40 sm:flex-none">
              <CopyIcon />
              {copied ? "Copied" : "Copy link"}
            </Button>
            {canShare(url) && (
              <Button
                variant="secondary"
                size="lg"
                onClick={() => shareLink(url)}
                className="flex-1 sm:flex-none"
              >
                <ShareIcon />
                Share…
              </Button>
            )}
          </div>
        </div>

        <div className="space-y-3">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
            Other ways to hand it over
          </h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <Button variant="secondary" aria-expanded={showQR} onClick={() => setShowQR(!showQR)}>
              <QrCodeIcon />
              {showQR ? "Hide QR code" : "Show QR code"}
            </Button>
            <Button
              variant="secondary"
              aria-expanded={sendingCode}
              onClick={() => setSendingCode(!sendingCode)}
            >
              <KeyboardIcon />
              {sendingCode ? "Stop sending" : "Send with a code"}
            </Button>
          </div>

          {sendingCode && <SendWithCode url={url} onClose={() => setSendingCode(false)} />}

          {showQR && (
            <div
              ref={qrRef}
              className="rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-700 dark:bg-zinc-950"
            >
              {/* As wide as the box, for a laptop webcam at arm's length, but
                  never taller than the window. The size is reserved before the
                  code renders, so scrolling lands in the right place. */}
              <div className="mx-auto aspect-square w-full max-w-[calc(100vh-10rem)]">
                <QRCode url={url} className="h-full w-full rounded-md" />
              </div>
              <p className="mt-3 text-center text-xs text-zinc-500 dark:text-zinc-400">
                Anyone who can see this code can open the share.
              </p>
            </div>
          )}
        </div>

        {passwordProtected && (
          <Notice icon={<KeyIcon />}>
            This share needs its password. Send the password separately, for example by phone or in
            another app, never in the same message as the link.
          </Notice>
        )}
        {burnAfterRead && (
          <Notice icon={<WarningIcon />}>
            This link can be opened only once. It stops working as soon as the recipient reveals or
            downloads the content.
          </Notice>
        )}

        <p className="text-xs text-zinc-500 dark:text-zinc-400">
          Secretli doesn't keep these links, so it can't show them again. Copy them before you leave
          this page.
        </p>
      </section>

      <aside className="space-y-4 lg:sticky lg:top-24">
        <section className="rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
            Status
          </h2>
          <div className="mt-2 divide-y divide-zinc-200 dark:divide-zinc-700">
            <StatusRow
              label="Expires"
              value={expires.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}
              detail={formatRelativeTime(expiresAt)}
            />
            <StatusRow label="Can be opened" value={burnAfterRead ? "Once" : "Until it expires"} />
            <StatusRow label="Password" value={passwordProtected ? "Required" : "None"} />
          </div>
        </section>
        <section className="space-y-3 rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
          <h2
            id={ownerHeadingId}
            className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400"
          >
            Owner link
          </h2>
          <p className="text-xs leading-relaxed text-zinc-600 dark:text-zinc-300">
            Delete the share early with this link. Keep it private: it opens the share, too.
          </p>
          <div className="flex items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-50 py-1 pr-2 pl-3 dark:border-zinc-700 dark:bg-zinc-950">
            <input
              ref={ownerRef}
              type="text"
              readOnly
              value={ownerUrl}
              aria-labelledby={ownerHeadingId}
              onFocus={(e) => e.currentTarget.select()}
              className="min-w-0 flex-1 bg-transparent py-2 font-mono text-xs text-zinc-700 outline-none dark:text-zinc-100"
            />
            <TextButton tone="muted" onClick={copyOwnerUrl}>
              Copy
            </TextButton>
          </div>
        </section>
      </aside>
    </div>
  );
}
