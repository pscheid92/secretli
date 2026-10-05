import type { ButtonHTMLAttributes } from "react";
import { type ButtonSize, type ButtonVariant, buttonClass } from "./styles";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Fill the container's width. */
  block?: boolean;
}

export default function Button({
  variant,
  size,
  block,
  className,
  type = "button",
  ...props
}: ButtonProps) {
  const classes = buttonClass({ variant, size, block });
  return (
    <button type={type} className={className ? `${classes} ${className}` : classes} {...props} />
  );
}
