import { useState } from "react";
import { toast } from "sonner";
import { canScan } from "../../lib/qrScanner";
import { parseShareLink } from "../../lib/shareLink";
import QRScanner from "./QRScanner";

function openShare(fragment: string) {
  window.location.href = `${window.location.pathname}#${fragment}`;
  window.location.reload();
}

/** Landing state when the page is opened without a share fragment. */
export default function LinkPrompt() {
  const [linkInput, setLinkInput] = useState("");
  const [scanning, setScanning] = useState(false);
  const scanAvailable = canScan();

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
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
          Open a Share
        </h1>
        <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
          {scanAvailable
            ? "Paste a Secretli link or scan its QR code to decrypt it in this browser."
            : "Paste a Secretli link to decrypt it in this browser."}
        </p>
      </div>
      <form onSubmit={handleSubmit} className="space-y-4">
        <input
          id="secret-link"
          type="text"
          value={linkInput}
          onChange={(e) => setLinkInput(e.target.value)}
          placeholder={`${window.location.origin}/s#...`}
          autoFocus
          className="w-full rounded-lg border border-zinc-200 dark:border-zinc-500/50 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-4 py-3 text-sm font-mono placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
        />
        <button
          type="submit"
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 transition-all duration-150"
        >
          Open Share
        </button>
      </form>
      {scanAvailable &&
        (scanning ? (
          <QRScanner onScan={openShare} onCancel={() => setScanning(false)} />
        ) : (
          <button
            type="button"
            onClick={() => setScanning(true)}
            className="flex w-full items-center justify-center gap-2 rounded-lg border border-zinc-200 px-4 py-3 text-sm font-medium text-zinc-700 transition-colors duration-150 hover:border-amber-400 hover:text-zinc-900 focus:outline-none focus:ring-2 focus:ring-amber-400/50 dark:border-zinc-500/50 dark:text-zinc-100 dark:hover:border-amber-400 dark:hover:text-white"
          >
            <svg
              className="h-4 w-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={1.5}
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M6.827 6.175A2.31 2.31 0 0 1 5.186 7.23c-.38.054-.757.112-1.134.175C2.999 7.58 2.25 8.507 2.25 9.574V18a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9.574c0-1.067-.75-1.994-1.802-2.169a47.865 47.865 0 0 0-1.134-.175 2.31 2.31 0 0 1-1.64-1.055l-.822-1.316a2.192 2.192 0 0 0-1.736-1.039 48.774 48.774 0 0 0-5.232 0 2.192 2.192 0 0 0-1.736 1.039l-.821 1.316Z"
              />
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M16.5 12.75a4.5 4.5 0 1 1-9 0 4.5 4.5 0 0 1 9 0Z"
              />
            </svg>
            Scan QR code
          </button>
        ))}
    </div>
  );
}
