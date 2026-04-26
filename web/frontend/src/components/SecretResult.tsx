import { useState } from "react";
import { toast } from "sonner";
import QRCode from "./QRCode";

interface SecretResultProps {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  deletionToken: string;
}

export default function SecretResult({
  url,
  expiresAt,
  burnAfterRead,
  deletionToken,
}: SecretResultProps) {
  const ownerUrl = `${url}!${deletionToken}`;
  const [showQR, setShowQR] = useState(false);

  async function copyShareUrl() {
    await navigator.clipboard.writeText(url);
    toast.success("Link copied");
  }

  async function copyOwnerUrl() {
    await navigator.clipboard.writeText(ownerUrl);
    toast.success("Owner link copied");
  }

  return (
    <div className="space-y-4">
      {/* Success header */}
      <div className="flex items-center gap-2.5">
        <div className="w-2 h-2 rounded-full bg-emerald-400 flex-shrink-0" />
        <span className="text-sm text-zinc-600 dark:text-zinc-100">Secret created</span>
        <span className="text-xs text-zinc-500 dark:text-zinc-100">
          · Expires {new Date(expiresAt).toLocaleString()}
        </span>
      </div>

      {/* Share link card */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 overflow-hidden">
        <div className="px-4 py-2 bg-zinc-50 dark:bg-zinc-900/50 border-b border-zinc-200 dark:border-zinc-500/50 flex items-center justify-between">
          <span className="text-xs tracking-widest uppercase text-zinc-500 dark:text-zinc-100">
            Share link
          </span>
          <button
            type="button"
            onClick={() => setShowQR(!showQR)}
            className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
          >
            {showQR ? "Hide QR" : "Show QR"}
          </button>
        </div>
        <div className="flex items-center">
          <input
            type="text"
            readOnly
            value={url}
            className="flex-1 px-4 py-3 bg-white dark:bg-zinc-900 text-zinc-700 dark:text-zinc-100 font-mono text-xs focus:outline-none min-w-0 cursor-text"
          />
          <button
            type="button"
            onClick={copyShareUrl}
            className="px-4 py-3 text-xs font-semibold text-amber-600 dark:text-amber-400 hover:bg-amber-400/5 border-l border-zinc-200 dark:border-zinc-500/50 transition-colors duration-150 whitespace-nowrap"
          >
            Copy
          </button>
        </div>
        {showQR && (
          <div className="border-t border-zinc-200 dark:border-zinc-500/50 p-6 bg-white dark:bg-zinc-900 flex justify-center">
            <QRCode url={url} />
          </div>
        )}
      </div>

      {/* Burn after read warning */}
      {burnAfterRead && (
        <div className="flex items-start gap-3 rounded-lg border border-amber-200 dark:border-amber-900/40 bg-amber-50 dark:bg-amber-900/10 px-4 py-3">
          <svg
            className="h-4 w-4 text-amber-500 flex-shrink-0 mt-0.5"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
            aria-hidden="true"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <p className="text-xs text-amber-700 dark:text-amber-400 leading-relaxed">
            This secret is consumed when the recipient starts reveal or download. If their download
            is interrupted after that, the link may not work again.
          </p>
        </div>
      )}

      {/* Owner link card */}
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-500/50 overflow-hidden">
        <div className="px-4 py-2 bg-zinc-50 dark:bg-zinc-900/50 border-b border-zinc-200 dark:border-zinc-500/50">
          <span className="text-xs tracking-widest uppercase text-zinc-500 dark:text-zinc-100">
            Owner link
          </span>
          <span className="ml-2 text-xs text-zinc-500 dark:text-zinc-100">
            — keep private, lets you delete
          </span>
        </div>
        <div className="flex items-center">
          <input
            type="text"
            readOnly
            value={ownerUrl}
            className="flex-1 px-4 py-3 bg-white dark:bg-zinc-900 text-zinc-500 dark:text-zinc-100 font-mono text-xs focus:outline-none min-w-0 cursor-text"
          />
          <button
            type="button"
            onClick={copyOwnerUrl}
            className="px-4 py-3 text-xs font-medium text-zinc-600 hover:text-zinc-700 dark:hover:text-zinc-300 border-l border-zinc-200 dark:border-zinc-500/50 transition-colors duration-150 whitespace-nowrap"
          >
            Copy
          </button>
        </div>
      </div>
    </div>
  );
}
