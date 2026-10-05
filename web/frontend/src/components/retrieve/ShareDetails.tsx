import type { SecretMetadataResponse } from "../../lib/api";
import type { SecretMeta } from "../../lib/encryption";
import { formatRelativeTime, formatSize } from "../../lib/format";
import SecretTypeIcon from "../SecretTypeIcon";
import Spinner from "../Spinner";
import Button from "../ui/Button";
import PageTitle from "../ui/PageTitle";
import BurnWarning from "./BurnWarning";
import DeleteShareButton from "./DeleteShareButton";

function MetaRow({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className="flex items-center justify-between py-2.5 text-sm">
      <span className="text-zinc-600 dark:text-zinc-100">{label}</span>
      <span
        className={
          accent
            ? "text-amber-700 dark:text-amber-400 font-medium"
            : "text-zinc-600 dark:text-zinc-100"
        }
      >
        {value}
      </span>
    </div>
  );
}

/** Label for the button that starts decryption, given what the share is. */
export function revealLabel(clientMeta: SecretMeta, burnAfterRead: boolean): string {
  if (clientMeta.password_protected) return "Unlock Share";
  const isBundle = clientMeta.type === "bundle";
  if (burnAfterRead) return isBundle ? "Prepare Download & Burn" : "Reveal & Burn";
  return isBundle ? "Prepare Download" : "Reveal Text";
}

interface ShareDetailsProps {
  serverMeta: SecretMetadataResponse;
  clientMeta: SecretMeta;
  revealing: boolean;
  deleting: boolean;
  canDelete: boolean;
  onReveal: () => void;
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

  return (
    <div className="space-y-6">
      <PageTitle lead="Review the details before this browser decrypts the content.">
        {isBundle ? "File Share" : "Text Share"}
      </PageTitle>

      <section className="space-y-5 rounded-lg border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-900">
        <div className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 dark:divide-zinc-700 dark:border-zinc-700">
          <div className="flex items-center gap-2.5 px-4 py-3">
            <SecretTypeIcon
              type={clientMeta.type}
              className="h-4 w-4 text-zinc-500 dark:text-zinc-100"
            />
            <span className="text-sm font-medium text-zinc-700 dark:text-zinc-100">
              {isBundle ? "Files" : "Text"}
            </span>
          </div>
          <div className="px-4">
            <MetaRow label="Created" value={formatRelativeTime(serverMeta.created_at)} />
          </div>
          <div className="px-4">
            <MetaRow label="Expires" value={formatRelativeTime(serverMeta.expires_at)} />
          </div>
          {isBundle && serverMeta.blob_size > 0 && (
            <div className="px-4">
              <MetaRow label="Size" value={formatSize(serverMeta.blob_size)} />
            </div>
          )}
          {clientMeta.password_protected && (
            <div className="px-4">
              <MetaRow label="Password" value="Required" accent />
            </div>
          )}
        </div>

        {isBundle && (
          <p className="text-xs leading-relaxed text-zinc-500 dark:text-zinc-400">
            File names and sizes are hidden until the encrypted manifest is opened.
          </p>
        )}
      </section>

      <div className="space-y-4">
        {serverMeta.burn_after_read && (
          <BurnWarning>
            {canDelete
              ? // The owner link of a one-time share: revealing it here would
                // take it away from the recipient.
                "This is your owner link, and nobody has opened the share yet. Revealing it here uses it up: your recipient won't be able to open it. To remove it instead, use Delete share."
              : isBundle
                ? "These files can be downloaded only once. Once you start, the link stops working, so keep this page open until the download finishes."
                : "This share can be opened only once. Once you reveal it, the link stops working, so copy what you need."}
          </BurnWarning>
        )}
        <section className="rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
          <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
            Next step
          </h2>
          <Button size="lg" block onClick={onReveal} disabled={revealing} className="mt-4">
            {revealing && <Spinner size="sm" className="text-zinc-700" />}
            {revealing ? "Decrypting..." : revealLabel(clientMeta, serverMeta.burn_after_read)}
          </Button>
        </section>
        {canDelete && (
          <DeleteShareButton deleting={deleting} disabled={revealing} onDelete={onDelete} />
        )}
      </div>
    </div>
  );
}
