import { useState } from "react";
import { toast } from "sonner";

/** Landing state when the page is opened without a share fragment. */
export default function LinkPrompt() {
  const [linkInput, setLinkInput] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const url = new URL(linkInput.trim());
      const fragment = url.hash.slice(1);
      if (!fragment) {
        toast.error("That link doesn't contain a share key.");
        return;
      }
      window.location.href = `${window.location.pathname}#${fragment}`;
      window.location.reload();
    } catch {
      toast.error("Please enter a valid Secretli link.");
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
          Open a Share
        </h1>
        <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
          Paste a Secretli link to decrypt it in this browser.
        </p>
      </div>
      <form onSubmit={handleSubmit} className="space-y-4">
        <input
          id="secret-link"
          type="text"
          value={linkInput}
          onChange={(e) => setLinkInput(e.target.value)}
          placeholder="https://secretli.example/s#..."
          autoFocus
          className="w-full rounded-lg border border-zinc-200 dark:border-zinc-500/50 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-4 py-3 text-sm font-mono placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
        />
        <button
          type="submit"
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 transition-all duration-150"
        >
          Open Share
        </button>
      </form>
    </div>
  );
}
