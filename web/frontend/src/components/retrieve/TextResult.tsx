import { toast } from "sonner";
import DeleteShareButton from "./DeleteShareButton";

interface TextResultProps {
  text: string;
  canDelete: boolean;
  deleting: boolean;
  onDelete: () => void;
}

export default function TextResult({ text, canDelete, deleting, onDelete }: TextResultProps) {
  async function copy() {
    await navigator.clipboard.writeText(text);
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

        <div className="overflow-hidden rounded-lg border border-zinc-200 dark:border-zinc-700">
          <div className="flex items-center justify-between border-b border-zinc-200 bg-zinc-50 px-4 py-2 dark:border-zinc-700 dark:bg-zinc-950">
            <span className="text-xs uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
              Plaintext
            </span>
            <button
              type="button"
              onClick={copy}
              className="text-xs font-semibold text-amber-600 transition-colors duration-150 hover:text-amber-500 dark:text-amber-400 dark:hover:text-amber-300"
            >
              Copy
            </button>
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
