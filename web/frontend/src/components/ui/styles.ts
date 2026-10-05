/**
 * Shared styles for interactive elements, so colours meet WCAG AA contrast and
 * targets stay large enough to hit everywhere. Plain class strings work on
 * <button>, <a> and router links alike.
 */

export type ButtonVariant = "primary" | "secondary" | "quiet" | "danger-outline";
export type ButtonSize = "sm" | "md" | "lg";

/** Keyboard focus: an outline in the focus colour, 3:1 on paper and on ink. */
export const FOCUS =
  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus";

const BUTTON_BASE = `inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-xl font-medium transition-colors duration-150 ${FOCUS} disabled:cursor-not-allowed disabled:opacity-40`;

/** quiet: the toolbar kind, text until hovered. */
const BUTTON_VARIANTS: Record<ButtonVariant, string> = {
  primary: "bg-accent text-on-accent hover:bg-accent-hover",
  secondary: "border border-line bg-surface text-ink hover:bg-hover",
  quiet: "text-muted hover:bg-hover hover:text-ink",
  "danger-outline": "border border-danger text-danger hover:bg-danger-soft",
};

const BUTTON_SIZES: Record<ButtonSize, string> = {
  sm: "min-h-9 px-3 text-sm",
  md: "min-h-11 px-4 text-sm",
  lg: "min-h-12 px-5 text-body font-semibold",
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

const TEXT_BASE = `inline-flex min-h-11 items-center gap-1.5 rounded-md text-sm font-medium transition-colors duration-150 ${FOCUS}`;

const TEXT_TONES: Record<TextTone, string> = {
  accent: "text-ink decoration-accent decoration-2 underline-offset-4 hover:underline",
  muted: "text-muted hover:text-ink",
  danger: "text-danger decoration-2 underline-offset-4 hover:underline",
};

/** A text-only action or link, at least 44 px tall so it is easy to hit. */
export function textButtonClass(tone: TextTone = "accent"): string {
  return `${TEXT_BASE} ${TEXT_TONES[tone]}`;
}
