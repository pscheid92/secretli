/**
 * Shared styles for interactive elements, so colours meet WCAG AA contrast and
 * targets stay large enough to hit everywhere. Plain class strings work on
 * <button>, <a> and router links alike.
 */

export type ButtonVariant = "primary" | "secondary" | "danger" | "danger-outline";
export type ButtonSize = "sm" | "md" | "lg";

const FOCUS = "focus-visible:outline-none focus-visible:ring-2";

const BUTTON_BASE = `inline-flex items-center justify-center gap-2 rounded-lg font-medium transition-colors duration-150 ${FOCUS} disabled:cursor-not-allowed disabled:opacity-40`;

const BUTTON_VARIANTS: Record<ButtonVariant, string> = {
  primary: "bg-amber-400 text-zinc-950 hover:bg-amber-300 focus-visible:ring-amber-500/60",
  secondary:
    "border border-zinc-200 bg-white text-zinc-700 hover:border-zinc-400 hover:text-zinc-900 focus-visible:ring-amber-500/60 dark:border-zinc-500/50 dark:bg-zinc-900 dark:text-zinc-100 dark:hover:border-zinc-400 dark:hover:text-white",
  danger: "bg-red-600 text-white hover:bg-red-500 focus-visible:ring-red-500/50",
  "danger-outline":
    "border border-red-200 text-red-700 hover:bg-red-50 focus-visible:ring-red-500/40 dark:border-red-900/40 dark:text-red-400 dark:hover:bg-red-900/10",
};

const BUTTON_SIZES: Record<ButtonSize, string> = {
  sm: "min-h-8 px-3 py-1.5 text-xs",
  md: "min-h-10 px-4 py-2 text-sm",
  lg: "min-h-12 px-4 py-3 text-sm font-semibold",
};

export function buttonClass({
  variant = "primary",
  size = "md",
  block = false,
}: {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Fill the container's width. */
  block?: boolean;
} = {}): string {
  return `${BUTTON_BASE} ${BUTTON_VARIANTS[variant]} ${BUTTON_SIZES[size]}${block ? " w-full" : ""}`;
}

/** accent: the action next to content, like Copy. muted: secondary actions and links. */
export type TextTone = "accent" | "muted" | "danger";

const TEXT_BASE = `inline-flex min-h-6 items-center gap-1 rounded text-xs transition-colors duration-150 ${FOCUS} focus-visible:ring-amber-500/60`;

const TEXT_TONES: Record<TextTone, string> = {
  accent:
    "font-semibold text-amber-700 hover:text-amber-800 dark:text-amber-400 dark:hover:text-amber-300",
  muted:
    "font-medium text-zinc-600 hover:text-amber-700 dark:text-zinc-300 dark:hover:text-amber-400",
  danger:
    "font-semibold text-zinc-600 hover:text-red-700 dark:text-zinc-300 dark:hover:text-red-400",
};

/** A text-only action or link, at least 24 px tall so it is easy to hit. */
export function textButtonClass(tone: TextTone = "accent"): string {
  return `${TEXT_BASE} ${TEXT_TONES[tone]}`;
}
