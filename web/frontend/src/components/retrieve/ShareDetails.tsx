import { type FormEvent, useId, useRef, useState } from "react";
import type { SecretMetadataResponse } from "../../lib/api";
import type { SecretMeta } from "../../lib/encryption";
import { formatExpiry, formatRelativeTime, formatSize } from "../../lib/format";
import Spinner from "../Spinner";
import Button from "../ui/Button";
import { LockIcon } from "../ui/icons";
import Note from "../ui/Note";
import PageTitle from "../ui/PageTitle";
import PasswordInput from "../ui/PasswordInput";
import DeleteShareButton from "./DeleteShareButton";

/** Label for the button that starts decryption, given what the share is. */
export function revealLabel(clientMeta: SecretMeta): string {
  return clientMeta.type === "bundle" ? "Show the files" : "Reveal secret";
}

/** What a recipient is told before deciding to open it. */
function recipientLead(serverMeta: SecretMetadataResponse, isBundle: boolean): string {
  const size = isBundle && serverMeta.blob_size > 0 ? ` (${formatSize(serverMeta.blob_size)})` : "";
  const sent = `${isBundle ? `A set of files${size}, sent` : "Sent"} ${formatRelativeTime(serverMeta.created_at)}.`;
  const expires = formatExpiry(serverMeta.expires_at);
  return serverMeta.burn_after_read
    ? `${sent} It opens once, then it's gone. The link expires ${expires}.`
    : `${sent} The link can be opened again and again until it expires, ${expires}.`;
}

interface ShareDetailsProps {
  serverMeta: SecretMetadataResponse;
  clientMeta: SecretMeta;
  revealing: boolean;
  deleting: boolean;
  canDelete: boolean;
  /** Resolves to a message for the password field, or null once the secret is open. */
  onReveal: (password?: string) => Promise<string | null>;
  onDelete: () => void;
}

/** What the recipient sees before anything is decrypted. */
export default function ShareDetails({
  serverMeta,
  clientMeta,
  revealing,
  deleting,
  canDelete,
  onReveal,
  onDelete,
}: ShareDetailsProps) {
  const isBundle = clientMeta.type === "bundle";
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const passwordId = useId();
  const errorId = useId();

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (revealing) return;
    const message = await onReveal(clientMeta.password_protected ? password : undefined);
    if (message) {
      setError(message);
      // Selected, so retyping replaces the mistyped password.
      passwordRef.current?.focus();
      passwordRef.current?.select();
    }
  }

  return (
    <div className="space-y-8">
      {canDelete ? (
        <PageTitle
          lead={`This is your owner link. You can open the secret, or delete it for everyone. The link expires ${formatExpiry(serverMeta.expires_at)}.`}
        >
          Your secret
        </PageTitle>
      ) : (
        <PageTitle lead={recipientLead(serverMeta, isBundle)}>Someone sent you a secret</PageTitle>
      )}

      {canDelete && serverMeta.burn_after_read && (
        // The owner link of a one-time share: opening it here would take it
        // away from the recipient.
        <Note>
          Nobody has opened it yet. Opening it here uses it up: your recipient won't be able to. To
          remove it instead, delete it below.
        </Note>
      )}

      <form onSubmit={handleSubmit} className="flex max-w-md flex-col gap-4">
        {clientMeta.password_protected && (
          <div className="flex flex-col gap-2">
            <label htmlFor={passwordId} className="px-1 text-sm font-medium text-muted">
              Password
            </label>
            <PasswordInput
              ref={passwordRef}
              id={passwordId}
              value={password}
              onChange={(e) => {
                setPassword(e.target.value);
                setError(null);
              }}
              placeholder="From the sender"
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
              autoFocus
              autoComplete="off"
              spellCheck={false}
              data-gramm="false"
              data-gramm_editor="false"
              data-enable-grammarly="false"
              data-1p-ignore
            />
            {error && (
              <p id={errorId} className="px-1 text-sm text-danger">
                {error}
              </p>
            )}
          </div>
        )}
        <Button type="submit" size="lg" disabled={revealing} className="self-start">
          {revealing && <Spinner size="sm" />}
          {revealing
            ? clientMeta.password_protected
              ? "Checking the password…"
              : "Decrypting…"
            : revealLabel(clientMeta)}
        </Button>
      </form>

      <p className="flex items-start gap-2.5 text-[13px] text-faint">
        <span className="mt-0.5 flex">
          <LockIcon />
        </span>
        <span>Decrypted here, in your browser. The server never sees what's inside.</span>
      </p>

      {canDelete && (
        <DeleteShareButton deleting={deleting} disabled={revealing} onDelete={onDelete} />
      )}
    </div>
  );
}
