import { useForm } from "react-hook-form";
import type { SecretMeta } from "../../lib/encryption";
import Spinner from "../Spinner";
import BurnWarning from "./BurnWarning";

interface PasswordPromptProps {
  clientMeta: SecretMeta;
  burnAfterRead: boolean;
  loading: boolean;
  /** Resolves to an error message to show on the field, or null on success. */
  onSubmit: (password: string) => Promise<string | null>;
}

export default function PasswordPrompt({
  clientMeta,
  burnAfterRead,
  loading,
  onSubmit,
}: PasswordPromptProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<{ password: string }>({ defaultValues: { password: "" } });

  const isBundle = clientMeta.type === "bundle";
  const submitLabel = burnAfterRead
    ? isBundle
      ? "Prepare Download & Burn"
      : "Reveal & Burn"
    : isBundle
      ? "Prepare Download"
      : "Reveal Text";

  return (
    <div className="mx-auto max-w-xl space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
      <div>
        <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
          Unlock Share
        </h1>
        <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-100">
          Enter the password to decrypt the protected content.
        </p>
      </div>
      <form
        onSubmit={handleSubmit(async (data) => {
          const message = await onSubmit(data.password);
          if (message) setError("password", { message });
        })}
        className="space-y-4"
      >
        <input
          type="password"
          {...register("password", { required: "Password is required" })}
          placeholder="Enter password..."
          autoFocus
          autoComplete="off"
          data-gramm="false"
          data-gramm_editor="false"
          data-enable-grammarly="false"
          data-1p-ignore
          className="w-full rounded-lg border border-zinc-200 dark:border-zinc-500/50 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-4 py-3 text-sm placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
        />
        {errors.password && (
          <p className="text-xs text-red-500 dark:text-red-400">{errors.password.message}</p>
        )}
        {burnAfterRead && (
          <BurnWarning>
            Submitting the password starts retrieval and permanently consumes this share.
          </BurnWarning>
        )}
        <button
          type="submit"
          disabled={loading}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
        >
          {loading && <Spinner size="sm" className="text-zinc-700" />}
          {loading ? "Decrypting..." : submitLabel}
        </button>
      </form>
    </div>
  );
}
