import { useForm } from "react-hook-form";
import type { SecretMeta } from "../../lib/encryption";
import Spinner from "../Spinner";
import Button from "../ui/Button";
import PageTitle from "../ui/PageTitle";
import PasswordInput from "../ui/PasswordInput";
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
    setFocus,
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
    <div className="space-y-6">
      <PageTitle lead="Enter the password to decrypt the protected content.">
        Unlock Share
      </PageTitle>
      <form
        onSubmit={handleSubmit(async (data) => {
          const message = await onSubmit(data.password);
          if (message) {
            setError("password", { message });
            // Selected, so retyping replaces the mistyped password.
            setFocus("password", { shouldSelect: true });
          }
        })}
        className="max-w-md space-y-4"
      >
        <PasswordInput
          large
          {...register("password", { required: "Password is required" })}
          aria-label="Password"
          placeholder="Enter password..."
          autoFocus
          autoComplete="off"
          data-gramm="false"
          data-gramm_editor="false"
          data-enable-grammarly="false"
          data-1p-ignore
        />
        {errors.password && (
          <p className="text-xs text-red-700 dark:text-red-400">{errors.password.message}</p>
        )}
        {burnAfterRead && (
          <BurnWarning>
            This share can be opened only once. A wrong password doesn't use it up, but the right
            one does: after that the link stops working.
          </BurnWarning>
        )}
        <Button type="submit" size="lg" block disabled={loading}>
          {loading && <Spinner size="sm" className="text-zinc-700" />}
          {loading ? "Decrypting..." : submitLabel}
        </Button>
      </form>
    </div>
  );
}
