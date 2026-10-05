import { type InputHTMLAttributes, type Ref, useState } from "react";
import { FOCUS } from "./styles";

interface PasswordInputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "className"> {
  ref?: Ref<HTMLInputElement>;
}

/**
 * A password field with a button that shows what was typed, so a typo can
 * be caught before it locks the recipient out. Shown text is monospaced:
 * l, 1 and I stay apart.
 */
export default function PasswordInput({ ref, ...props }: PasswordInputProps) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="flex items-center gap-1 rounded-[14px] border border-line bg-surface pr-1 transition-[border-color,box-shadow] duration-150 focus-within:border-focus focus-within:ring-4 focus-within:ring-ring">
      <input
        ref={ref}
        type={visible ? "text" : "password"}
        className={`h-12 min-w-0 flex-1 bg-transparent pl-4 text-body text-ink outline-none placeholder:text-faint${visible ? " font-mono" : ""}`}
        {...props}
      />
      <button
        type="button"
        onClick={() => setVisible(!visible)}
        aria-pressed={visible}
        className={`h-11 shrink-0 rounded-[10px] px-3 text-[13px] font-medium text-muted transition-colors duration-150 hover:bg-hover hover:text-ink ${FOCUS}`}
      >
        {visible ? "Hide" : "Show"}
      </button>
    </div>
  );
}
