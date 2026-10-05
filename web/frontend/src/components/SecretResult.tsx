import { useEffect, useId, useRef, useState } from "react";
import { toast } from "sonner";
import { formatExpiry } from "../lib/format";
import QRCode from "./QRCode";
import DeleteShareButton from "./retrieve/DeleteShareButton";
import SendWithCode from "./SendWithCode";
import Button from "./ui/Button";
import {
  CheckIcon,
  ChevronDownIcon,
  CopyIcon,
  KeyboardIcon,
  QrCodeIcon,
  ShareIcon,
} from "./ui/icons";
import Note from "./ui/Note";
import PageTitle from "./ui/PageTitle";
import { FOCUS } from "./ui/styles";

interface SecretResultProps {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  passwordProtected: boolean;
  deletionToken: string;
  deleting: boolean;
  onDelete: () => void;
}

/** How long a copy button says "Copied". */
const COPIED_MS = 2000;

/**
 * Copies text to the clipboard. Where the browser refuses, the text is
 * selected on the page instead, so it can be copied by hand.
 */
async function copyText(text: string, shown: HTMLElement | null): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    if (shown) {
      const range = document.createRange();
      range.selectNodeContents(shown);
      const selection = window.getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
    }
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

/** Flips to true for a moment after copying. */
function useCopied(): [boolean, () => void] {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), COPIED_MS);
    return () => clearTimeout(timer);
  }, [copied]);
  return [copied, () => setCopied(true)];
}

const QUIET = "border-0 px-3.25";

export default function SecretResult({
  url,
  expiresAt,
  burnAfterRead,
  passwordProtected,
  deletionToken,
  deleting,
  onDelete,
}: SecretResultProps) {
  const ownerUrl = `${url}!${deletionToken}`;
  const [copied, markCopied] = useCopied();
  const [ownerCopied, markOwnerCopied] = useCopied();
  const [panel, setPanel] = useState<"qr" | "code" | null>(null);
  const [ownerOpen, setOwnerOpen] = useState(false);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const linkRef = useRef<HTMLElement>(null);
  const ownerRef = useRef<HTMLElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const ownerHeadingId = useId();

  // The form was replaced by this page: move focus along, so keyboard and
  // screen reader users land on the result.
  useEffect(() => {
    headingRef.current?.focus();
  }, []);

  // On shorter screens an opened panel reaches below the fold.
  useEffect(() => {
    if (panel) panelRef.current?.scrollIntoView({ block: "nearest" });
  }, [panel]);

  const hashAt = url.indexOf("#");
  const linkBase = hashAt >= 0 ? url.slice(0, hashAt) : url;
  const linkFragment = hashAt >= 0 ? url.slice(hashAt) : "";
  const summary = [
    burnAfterRead ? "Opens once" : "Opens until it expires",
    `expires ${formatExpiry(expiresAt)}`,
    passwordProtected ? "password required" : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <div className="space-y-7">
      <PageTitle ref={headingRef} tabIndex={-1} lead={summary}>
        Your link is ready
      </PageTitle>

      <div className="overflow-hidden rounded-[20px] border border-line bg-surface shadow-card">
        <p className="px-6 pt-5.5 pb-4.5 font-mono text-[14.5px] leading-[1.65] break-all text-ink">
          <code ref={linkRef} data-testid="share-link">
            {linkBase}
            <span className="text-muted">{linkFragment}</span>
          </code>
        </p>
        <div className="flex flex-wrap items-center gap-0.5 border-t border-line p-2">
          <Button
            onClick={async () => {
              if (await copyText(url, linkRef.current)) markCopied();
            }}
            className="mr-1 pl-4"
          >
            {copied ? <CheckIcon /> : <CopyIcon />}
            {copied ? "Copied" : "Copy link"}
          </Button>
          <Button
            variant="quiet"
            className={QUIET}
            aria-expanded={panel === "qr"}
            onClick={() => setPanel(panel === "qr" ? null : "qr")}
          >
            <QrCodeIcon />
            QR code
          </Button>
          <Button
            variant="quiet"
            className={QUIET}
            aria-expanded={panel === "code"}
            onClick={() => setPanel(panel === "code" ? null : "code")}
          >
            <KeyboardIcon />
            Send with a code
          </Button>
          {canShare(url) && (
            <Button variant="quiet" className={QUIET} onClick={() => shareLink(url)}>
              <ShareIcon />
              Share…
            </Button>
          )}
        </div>

        {panel === "qr" && (
          <div
            ref={panelRef}
            className="flex flex-wrap items-center gap-x-7 gap-y-5 border-t border-line bg-sunken p-6"
          >
            <div className="rounded-[14px] bg-white p-3.5 ring-1 ring-line">
              <QRCode url={url} className="block h-48 w-48" />
            </div>
            <p className="min-w-55 flex-1 text-pretty text-body text-muted">
              Point the other device's camera at it. It's the same link: anyone who sees the code
              can open the secret{burnAfterRead ? ", and it still opens only once" : ""}.
            </p>
          </div>
        )}
        {panel === "code" && (
          <div ref={panelRef}>
            <SendWithCode url={url} onClose={() => setPanel(null)} />
          </div>
        )}
      </div>

      {passwordProtected && (
        <Note>Send the password another way: say it, or text it separately.</Note>
      )}

      <section aria-labelledby={ownerHeadingId} className="space-y-3.5 border-t border-line pt-1.5">
        <h2 id={ownerHeadingId} className="m-0 text-body font-medium">
          <button
            type="button"
            aria-expanded={ownerOpen}
            onClick={() => setOwnerOpen(!ownerOpen)}
            className={`inline-flex min-h-11 items-center gap-2 rounded-lg text-ink ${FOCUS}`}
          >
            Owner link
            <span
              className={`flex text-faint transition-transform duration-200 ${ownerOpen ? "rotate-180" : ""}`}
            >
              <ChevronDownIcon />
            </span>
          </button>
        </h2>
        {ownerOpen && (
          <>
            <p className="-mt-2 max-w-[36em] text-pretty text-sm text-muted">
              See whether it was opened, or delete it before anyone does. Keep this link to
              yourself: it opens the secret, too.
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <code
                ref={ownerRef}
                data-testid="owner-link"
                className="min-w-0 flex-1 basis-65 truncate rounded-xl bg-sunken px-3.5 py-3.25 font-mono text-[13.5px] text-muted ring-1 ring-line ring-inset"
              >
                {ownerUrl}
              </code>
              <Button
                variant="quiet"
                className={QUIET}
                onClick={async () => {
                  if (await copyText(ownerUrl, ownerRef.current)) markOwnerCopied();
                }}
              >
                {ownerCopied ? <CheckIcon /> : <CopyIcon />}
                {ownerCopied ? "Copied" : "Copy"}
              </Button>
            </div>
            <DeleteShareButton
              label="Delete it now"
              question="Delete it for good? The link stops working right away."
              deleting={deleting}
              onDelete={onDelete}
            />
            <p className="text-[13px] text-faint">
              Secretli can't show these links again. Copy what you need before you leave.
            </p>
          </>
        )}
      </section>
    </div>
  );
}
