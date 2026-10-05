import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import QRCode from "./QRCode";

interface QRCodeOverlayProps {
  url: string;
  onClose: () => void;
}

/**
 * Fills the window with the share link's QR code, large enough for a laptop
 * webcam at arm's length. Light in both themes, so the code reads dark on white.
 */
export default function QRCodeOverlay({ url, onClose }: QRCodeOverlayProps) {
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    closeRef.current?.focus();
  }, []);

  // Portaled to the body so it sits above the sticky header.
  return createPortal(
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Large QR code"
      // Focusable so Escape still reaches it after a click on the code or text.
      tabIndex={-1}
      className="fixed inset-0 z-[60] flex flex-col items-center justify-center gap-4 bg-white p-4 outline-none"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") onClose();
      }}
    >
      <QRCode url={url} margin={4} className="size-[min(calc(100vh-11rem),90vw)]" />
      <p className="max-w-md text-center text-sm text-zinc-700">
        Point the other device's camera at this code. Anyone who can see it can open the share.
      </p>
      <button
        ref={closeRef}
        type="button"
        onClick={onClose}
        className="rounded-lg border border-zinc-300 px-4 py-2 text-sm font-medium text-zinc-700 transition-colors duration-150 hover:border-zinc-400 hover:text-zinc-900 focus:outline-none focus:ring-2 focus:ring-amber-400/50"
      >
        Close
      </button>
    </div>,
    document.body,
  );
}
