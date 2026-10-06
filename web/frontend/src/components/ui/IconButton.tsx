import type { ButtonHTMLAttributes, ReactNode } from "react";
import { FOCUS } from "./styles";

interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-label"> {
  /** Names the button for screen readers and shows as its tooltip. */
  label: string;
  children: ReactNode;
}

/** A 44 px button around an icon; the icon itself is decoration. */
export default function IconButton({ label, children, className, ...props }: IconButtonProps) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={`inline-flex h-11 w-11 items-center justify-center rounded-xl text-muted transition-colors duration-150 hover:bg-hover hover:text-ink disabled:cursor-not-allowed disabled:text-faint disabled:hover:bg-transparent ${FOCUS}${className ? ` ${className}` : ""}`}
      {...props}
    >
      {children}
    </button>
  );
}
