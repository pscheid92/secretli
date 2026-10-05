import type { ButtonHTMLAttributes } from "react";
import { type TextTone, textButtonClass } from "./styles";

interface TextButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  tone?: TextTone;
}

/** A text-only action, like Copy next to a link. */
export default function TextButton({ tone, className, ...props }: TextButtonProps) {
  const classes = textButtonClass(tone);
  return (
    <button type="button" className={className ? `${classes} ${className}` : classes} {...props} />
  );
}
