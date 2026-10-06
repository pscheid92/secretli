import { type ReactNode, useState } from "react";
import { toast } from "sonner";
import { canScan } from "../../lib/qrScanner";
import { parseShareLink } from "../../lib/shareLink";
import Button from "../ui/Button";
import { ArrowRightIcon, CameraIcon, KeyboardIcon } from "../ui/icons";
import PageTitle from "../ui/PageTitle";
import { FOCUS } from "../ui/styles";
import EnterCode from "./EnterCode";
import QRScanner from "./QRScanner";

/** RetrievePage reloads when the fragment changes and opens the share. */
function openShare(fragment: string) {
  window.location.hash = fragment;
}

type Mode = "choose" | "scan" | "code";

interface LinkPromptProps {
  /** "code" opens with code entry ready, as /c does. */
  initialMode?: "choose" | "code";
}

/** One of the other ways in: a row of the list under the link field. */
function WayIn({
  icon,
  onClick,
  children,
}: {
  icon: ReactNode;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex min-h-15 w-full items-center gap-3.5 px-4.5 text-left text-body font-medium text-ink transition-colors duration-150 hover:bg-hover ${FOCUS} focus-visible:-outline-offset-2`}
    >
      <span className="flex text-muted">{icon}</span>
      <span className="flex-1">{children}</span>
      <span className="flex text-faint">
        <ArrowRightIcon />
      </span>
    </button>
  );
}

/** Landing state when the page is opened without a share fragment. */
export default function LinkPrompt({ initialMode = "choose" }: LinkPromptProps) {
  const [linkInput, setLinkInput] = useState("");
  const [mode, setMode] = useState<Mode>(initialMode);
  const scanAvailable = canScan();

  // A pasted share link opens right away; anything else stays in the field.
  function handlePaste(e: React.ClipboardEvent<HTMLInputElement>) {
    const link = parseShareLink(e.clipboardData.getData("text"), window.location.origin);
    if (link.kind === "share") {
      e.preventDefault();
      openShare(link.fragment);
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const link = parseShareLink(linkInput, window.location.origin);
    if (link.kind === "other-host") {
      toast.error(`That link is for ${link.host}. Open it there instead.`);
      return;
    }
    if (link.kind === "invalid") {
      toast.error("Please enter a valid Secretli link.");
      return;
    }
    openShare(link.fragment);
  }

  if (mode === "code") {
    return (
      <div key="code" className="space-y-8">
        <PageTitle lead="It's on the other device: a number and two words.">
          Enter the code
        </PageTitle>
        <EnterCode onReceived={openShare} onCancel={() => setMode("choose")} />
      </div>
    );
  }

  if (mode === "scan") {
    return (
      <div key="scan" className="space-y-8">
        <PageTitle lead="Hold the other screen up to the camera.">Scan the QR code</PageTitle>
        <QRScanner onScan={openShare} onCancel={() => setMode("choose")} />
      </div>
    );
  }

  return (
    <div key="choose" className="space-y-8">
      <PageTitle
        lead={
          scanAvailable
            ? "Paste the link you were sent. No link? Scan its QR code, or type a code from the other device."
            : "Paste the link you were sent, or type a code from the other device."
        }
      >
        Open a secret
      </PageTitle>
      <form onSubmit={handleSubmit} className="flex flex-col gap-2.5">
        <label htmlFor="secret-link" className="px-1 text-sm font-medium text-muted">
          Link
        </label>
        <div className="flex items-center gap-1.5 rounded-[18px] border border-line bg-surface p-1.5 shadow-card transition-[border-color,box-shadow] duration-150 focus-within:border-focus focus-within:ring-4 focus-within:ring-ring">
          <input
            id="secret-link"
            type="text"
            value={linkInput}
            onChange={(e) => setLinkInput(e.target.value)}
            onPaste={handlePaste}
            placeholder={`${window.location.origin}/s#…`}
            autoFocus={initialMode === "choose"}
            autoComplete="off"
            spellCheck={false}
            className="h-12 min-w-0 flex-1 bg-transparent px-3.5 font-mono text-body text-ellipsis text-ink outline-none placeholder:text-faint"
          />
          <Button type="submit" size="lg" className="shrink-0 px-5.5">
            Open
          </Button>
        </div>
      </form>
      <div className="overflow-hidden rounded-[18px] border border-line bg-surface">
        {scanAvailable && (
          <>
            <WayIn icon={<CameraIcon />} onClick={() => setMode("scan")}>
              Scan a QR code
            </WayIn>
            <span aria-hidden="true" className="mx-4.5 block h-px bg-line" />
          </>
        )}
        <WayIn icon={<KeyboardIcon />} onClick={() => setMode("code")}>
          Enter a code from another device
        </WayIn>
      </div>
    </div>
  );
}
