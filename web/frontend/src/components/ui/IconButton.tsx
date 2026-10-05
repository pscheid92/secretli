import type { ButtonHTMLAttributes, ReactNode } from "react";

interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-label"> {
  /** Names the button for screen readers and shows as its tooltip. */
  label: string;
  children: ReactNode;
}

/** A 32 px button around an icon; the icon itself is decoration. */
export default function IconButton({ label, children, className, ...props }: IconButtonProps) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={`inline-flex h-8 w-8 items-center justify-center rounded-md text-zinc-600 transition-colors duration-150 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500/60 dark:text-zinc-300 dark:hover:bg-zinc-800 dark:hover:text-white${className ? ` ${className}` : ""}`}
      {...props}
    >
      {children}
    </button>
  );
}
