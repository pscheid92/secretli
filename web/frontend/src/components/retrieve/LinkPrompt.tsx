import { useState } from "react";
import { toast } from "sonner";
import { canScan } from "../../lib/qrScanner";
import { parseShareLink } from "../../lib/shareLink";
import Button from "../ui/Button";
import { CameraIcon, KeyboardIcon } from "../ui/icons";
import PageTitle from "../ui/PageTitle";
import EnterCode from "./EnterCode";
import QRScanner from "./QRScanner";

/** RetrievePage reloads when the fragment changes and opens the share. */
function openShare(fragment: string) {
  window.location.hash = fragment;
}

interface LinkPromptProps {
  /** "code" opens with code entry ready, as /c does. */
  initialMode?: "choose" | "code";
}

/** Landing state when the page is opened without a share fragment. */
export default function LinkPrompt({ initialMode = "choose" }: LinkPromptProps) {
  const [linkInput, setLinkInput] = useState("");
  const [mode, setMode] = useState<"choose" | "scan" | "code">(initialMode);
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

  return (
    <div className="space-y-6">
      <PageTitle
        lead={
          scanAvailable
            ? "Paste a Secretli link, scan its QR code or enter a code to decrypt it in this browser."
            : "Paste a Secretli link or enter a code to decrypt it in this browser."
        }
      >
        Open a Share
      </PageTitle>
      <form onSubmit={handleSubmit} className="space-y-4">
        <input
          id="secret-link"
          type="text"
          aria-label="Share link"
          value={linkInput}
          onChange={(e) => setLinkInput(e.target.value)}
          onPaste={handlePaste}
          placeholder={`${window.location.origin}/s#...`}
          autoFocus={initialMode === "choose"}
          className="w-full rounded-lg border border-zinc-200 dark:border-zinc-500/50 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-4 py-3 text-sm font-mono placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
        />
        <Button type="submit" size="lg" block>
          Open Share
        </Button>
      </form>
      {mode === "scan" && <QRScanner onScan={openShare} onCancel={() => setMode("choose")} />}
      {mode === "code" && <EnterCode onReceived={openShare} onCancel={() => setMode("choose")} />}
      {mode === "choose" && (
        <div className={`grid gap-3 ${scanAvailable ? "sm:grid-cols-2" : ""}`}>
          {scanAvailable && (
            <Button variant="secondary" size="lg" block onClick={() => setMode("scan")}>
              <CameraIcon />
              Scan QR code
            </Button>
          )}
          <Button variant="secondary" size="lg" block onClick={() => setMode("code")}>
            <KeyboardIcon />
            Enter a code
          </Button>
        </div>
      )}
    </div>
  );
}
