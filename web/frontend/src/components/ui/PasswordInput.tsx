import { type InputHTMLAttributes, type Ref, useState } from "react";
import { EyeIcon, EyeSlashIcon } from "./icons";

interface PasswordInputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "className"> {
  ref?: Ref<HTMLInputElement>;
  /** The roomier field of the unlock page. */
  large?: boolean;
}

const FIELD =
  "w-full border border-zinc-200 text-sm text-zinc-900 placeholder:text-zinc-500 transition-colors duration-150 focus:border-amber-400 focus:outline-none focus:ring-1 focus:ring-amber-400/20 dark:text-zinc-100 dark:placeholder:text-zinc-500 dark:focus:border-amber-400";
const COMPACT = "rounded-md bg-zinc-50 py-2.5 pr-11 pl-3 dark:border-zinc-700 dark:bg-zinc-950";
const LARGE = "rounded-lg bg-white py-3 pr-12 pl-4 dark:border-zinc-500/50 dark:bg-zinc-800";

/**
 * A password field with a button that shows what was typed, so a typo can
 * be caught before it locks the recipient out. Shown text is monospaced:
 * l, 1 and I stay apart.
 */
export default function PasswordInput({ ref, large, ...props }: PasswordInputProps) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <input
        ref={ref}
        type={visible ? "text" : "password"}
        className={`${FIELD} ${large ? LARGE : COMPACT}${visible ? " font-mono" : ""}`}
        {...props}
      />
      <button
        type="button"
        onClick={() => setVisible(!visible)}
        aria-pressed={visible}
        aria-label="Show password"
        title={visible ? "Hide password" : "Show password"}
        className={`absolute inset-y-0 right-0 flex items-center justify-center rounded-r-lg text-zinc-500 transition-colors duration-150 hover:text-zinc-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500/60 dark:text-zinc-400 dark:hover:text-zinc-100 ${large ? "w-12" : "w-11"}`}
      >
        {visible ? <EyeSlashIcon /> : <EyeIcon />}
      </button>
    </div>
  );
}
