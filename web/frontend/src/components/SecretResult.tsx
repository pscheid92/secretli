import { useState } from "react";
import { toast } from "sonner";
import QRCode from "./QRCode";

interface SecretResultProps {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  deletionToken: string;
}

interface LinkFieldProps {
  label: string;
  value: string;
  onCopy: () => void;
  privateLink?: boolean;
}

function LinkField({ label, value, onCopy, privateLink }: LinkFieldProps) {
  return (
    <div
      className={`rounded-lg border bg-white dark:bg-zinc-900 ${
        privateLink
          ? "border-zinc-200 dark:border-zinc-700"
          : "border-zinc-200 dark:border-zinc-500/50"
      }`}
    >
      <div className="flex items-center justify-between gap-3 border-b border-inherit px-4 py-2">
        <span
          className={`text-xs font-medium ${
            privateLink ? "text-zinc-500 dark:text-zinc-400" : "text-zinc-500 dark:text-zinc-400"
          }`}
        >
          {label}
        </span>
        <button
          type="button"
          onClick={onCopy}
          className={`text-xs font-semibold transition-colors duration-150 ${
            privateLink
              ? "text-zinc-600 hover:text-zinc-900 dark:text-zinc-300 dark:hover:text-white"
              : "text-amber-600 hover:text-amber-500 dark:text-amber-400 dark:hover:text-amber-300"
          }`}
        >
          Copy
        </button>
      </div>
      <input
        type="text"
        readOnly
        value={value}
        className="w-full min-w-0 bg-transparent px-4 py-3 font-mono text-xs text-zinc-700 outline-none dark:text-zinc-100"
      />
    </div>
  );
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
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start">
      <section className="space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-emerald-400" />
            <span className="text-sm font-medium text-emerald-700 dark:text-emerald-400">
              Secure link created
            </span>
          </div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Share is ready
          </h1>
          <p className="text-sm text-zinc-600 dark:text-zinc-300">
            Send the recipient link. Keep the owner link private.
          </p>
        </div>

        <LinkField label="Recipient link" value={url} onCopy={copyShareUrl} />

        <div className="flex justify-end">
          <button
            type="button"
            onClick={() => setShowQR(!showQR)}
            className="text-xs font-medium text-zinc-500 transition-colors duration-150 hover:text-amber-500 dark:text-zinc-400 dark:hover:text-amber-400"
          >
            {showQR ? "Hide QR code" : "Show QR code"}
          </button>
        </div>

        {showQR && (
          <div className="flex justify-center rounded-lg border border-zinc-200 bg-white p-6 dark:border-zinc-700 dark:bg-zinc-950">
            <QRCode url={url} />
          </div>
        )}

        {burnAfterRead && (
          <div className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-900/40 dark:bg-amber-900/10">
            <svg
              className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500"
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
            <p className="text-xs leading-relaxed text-amber-700 dark:text-amber-400">
              This share is consumed when the recipient starts reveal or download. If their download
              is interrupted after that, the link may not work again.
            </p>
          </div>
        )}
      </section>

      <aside className="space-y-4 lg:sticky lg:top-24">
        <section className="rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
            Status
          </h2>
          <div className="mt-2 divide-y divide-zinc-200 dark:divide-zinc-700">
            <div className="py-3">
              <div className="text-xs text-zinc-500 dark:text-zinc-400">Expires</div>
              <div className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                {new Date(expiresAt).toLocaleString()}
              </div>
            </div>
            <div className="py-3">
              <div className="text-xs text-zinc-500 dark:text-zinc-400">Read behavior</div>
              <div className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">
                {burnAfterRead ? "Burn after reading" : "Reusable until expiration"}
              </div>
            </div>
          </div>
        </section>
        <section className="space-y-3 rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
            Owner controls
          </h2>
          <LinkField
            label="Private owner link"
            value={ownerUrl}
            onCopy={copyOwnerUrl}
            privateLink
          />
        </section>
      </aside>
    </div>
  );
}
