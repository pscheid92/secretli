import { useEffect, useState } from "react";

/** How long a copy button says "Copied". */
const COPIED_MS = 2000;

/** Flips to true for a moment after copying, so the button can say so. */
export function useCopied(): [boolean, () => void] {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), COPIED_MS);
    return () => clearTimeout(timer);
  }, [copied]);
  return [copied, () => setCopied(true)];
}
