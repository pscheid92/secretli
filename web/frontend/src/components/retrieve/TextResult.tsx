import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useLeaveWarning } from "../../hooks/useLeaveWarning";
import TextButton from "../ui/TextButton";
import BurnWarning from "./BurnWarning";
import DeleteShareButton from "./DeleteShareButton";

interface TextResultProps {
  text: string;
  burnAfterRead: boolean;
  canDelete: boolean;
  deleting: boolean;
  onDelete: () => void;
}

export default function TextResult({
  text,
  burnAfterRead,
  canDelete,
  deleting,
  onDelete,
}: TextResultProps) {
  const [copied, setCopied] = useState(false);
  // A one-time text is gone from the server: until it was copied, this page
  // has the only copy.
  useLeaveWarning(burnAfterRead && !copied);

  // Copying by hand counts too.
  useEffect(() => {
    if (!burnAfterRead) return;
    const markCopied = () => setCopied(true);
    document.addEventListener("copy", markCopied);
    return () => document.removeEventListener("copy", markCopied);
  }, [burnAfterRead]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      toast.error("Couldn't copy automatically. Select the text and copy it with Ctrl+C or ⌘C.");
      return;
    }
    setCopied(true);
    toast.success("Copied to clipboard");
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start">
      <section className="space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
        <div>
          <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
            Decrypted Text
          </h1>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
            Decrypted in this browser. Copy the content below.
          </p>
        </div>

        {burnAfterRead && (
          <BurnWarning>
            This one-time share is already deleted from the server, so this page has the only copy.
            Copy what you need before you leave.
          </BurnWarning>
        )}

        <div className="overflow-hidden rounded-lg border border-zinc-200 dark:border-zinc-700">
          <div className="flex items-center justify-between border-b border-zinc-200 bg-zinc-50 px-4 py-2 dark:border-zinc-700 dark:bg-zinc-950">
            <span className="text-xs uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
              Plaintext
            </span>
            <TextButton onClick={copy}>Copy</TextButton>
          </div>
          <pre className="min-h-40 whitespace-pre-wrap break-words bg-white px-4 py-4 text-sm leading-relaxed text-zinc-800 dark:bg-zinc-950 dark:text-zinc-100">
            {text}
          </pre>
        </div>
      </section>

      {canDelete && (
        <aside className="lg:sticky lg:top-24">
          <DeleteShareButton deleting={deleting} onDelete={onDelete} />
        </aside>
      )}
    </div>
  );
}
