import { toast } from "sonner";

/**
 * Copies text to the clipboard. Where the browser refuses, the element that
 * shows the text is selected instead, so it can be copied by hand.
 */
export async function copyToClipboard(text: string, shown?: HTMLElement | null): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    if (shown) {
      const range = document.createRange();
      range.selectNodeContents(shown);
      const selection = window.getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
      toast.error("Couldn't copy automatically. It's selected: copy it with Ctrl+C or ⌘C.");
    } else {
      toast.error("Couldn't copy automatically. Select the text and copy it with Ctrl+C or ⌘C.");
    }
    return false;
  }
}
