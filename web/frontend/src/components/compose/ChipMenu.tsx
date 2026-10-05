import { type ReactNode, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { ChevronDownIcon } from "../ui/icons";
import { FOCUS } from "../ui/styles";

/** The composer's toolbar chips: text until hovered, a tint while their panel is open. */
export const CHIP = `inline-flex min-h-11 items-center gap-1.5 whitespace-nowrap rounded-xl px-3 text-sm font-medium text-muted transition-colors duration-150 hover:bg-hover disabled:cursor-not-allowed disabled:opacity-40 ${FOCUS}`;

const PANEL_WIDTH = 300;

interface ChipMenuProps {
  /** What the chip shows; parts of it may hide on phones. */
  label: ReactNode;
  /** What the chip is called, in full. */
  name: string;
  open: boolean;
  disabled?: boolean;
  onToggle: () => void;
  onClose: () => void;
  children: ReactNode;
}

/**
 * A toolbar chip with a small panel above it, for the composer's expiry and
 * opens settings. Escape and a click anywhere else close the panel.
 */
export default function ChipMenu({
  label,
  name,
  open,
  disabled,
  onToggle,
  onClose,
  children,
}: ChipMenuProps) {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const chipRef = useRef<HTMLButtonElement>(null);
  const wasOpen = useRef(false);
  const [shift, setShift] = useState(0);
  const panelId = useId();

  useEffect(() => {
    if (!open) return;
    function onMouseDown(e: MouseEvent) {
      if (!wrapperRef.current?.contains(e.target as Node)) onClose();
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== "Escape") return;
      onClose();
      chipRef.current?.focus();
    }
    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open, onClose]);

  // The panel hangs off the chip's left edge, pulled in where it would leave the screen.
  useLayoutEffect(() => {
    if (!open || !chipRef.current) return;
    const left = chipRef.current.getBoundingClientRect().left;
    setShift(Math.min(0, window.innerWidth - 16 - PANEL_WIDTH - left));
  }, [open]);

  // Picking an option unmounts the focused button; the chip takes the focus
  // back. A click elsewhere already moved it, and is left alone.
  useEffect(() => {
    if (wasOpen.current && !open && document.activeElement === document.body) {
      chipRef.current?.focus();
    }
    wasOpen.current = open;
  }, [open]);

  return (
    <div ref={wrapperRef} className="relative">
      <button
        ref={chipRef}
        type="button"
        aria-label={name}
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        disabled={disabled}
        onClick={onToggle}
        className={`${CHIP}${open ? " bg-hover" : ""}`}
      >
        {label}
        <ChevronDownIcon />
      </button>
      {open && (
        <div
          id={panelId}
          style={{ left: shift, width: `min(${PANEL_WIDTH}px, calc(100vw - 2rem))` }}
          className="absolute bottom-[calc(100%+10px)] z-20 rounded-2xl border border-line bg-surface p-2 shadow-pop"
        >
          {children}
        </div>
      )}
    </div>
  );
}
