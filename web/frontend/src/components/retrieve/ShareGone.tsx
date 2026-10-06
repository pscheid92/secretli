import type { SecretGone } from "../../lib/api";
import { formatMoment } from "../../lib/format";
import PageTitle from "../ui/PageTitle";
import { textButtonClass } from "../ui/styles";

interface ShareGoneProps {
  gone: SecretGone;
  /** Whether the link that was opened is the owner link. */
  owner: boolean;
}

const ASK_AGAIN = "Ask the sender for a new link if you still need it.";

/** The heading and the sentence under it, for the owner or for a recipient. */
export function goneCopy(gone: SecretGone, owner: boolean): { title: string; lead: string } {
  const when = formatMoment(gone.ended_at);
  const firstOpened = gone.first_opened_at ? formatMoment(gone.first_opened_at) : null;

  switch (gone.outcome) {
    case "opened":
      if (owner) {
        return gone.opened_by_owner
          ? {
              title: "Your secret is gone",
              lead: `You opened it yourself ${when}. It was a one-time secret, so nobody else can open it now.`,
            }
          : {
              title: "Your secret was opened",
              lead: `Opened ${when}. It was a one-time secret, so nothing is left on the server.`,
            };
      }
      return gone.opened_by_owner
        ? {
            title: "This secret is gone",
            lead: `The sender opened it ${when}, and a one-time secret opens only once. ${ASK_AGAIN}`,
          }
        : {
            title: "This secret was already opened",
            lead: `Someone opened it ${when}, and a one-time secret opens only once. If that wasn't you, tell the sender: the link may have reached someone else.`,
          };
    case "expired":
      if (owner) {
        return firstOpened
          ? {
              title: "Your secret expired",
              lead: `It expired ${when}. It was first opened ${firstOpened}.`,
            }
          : {
              title: "Your secret expired unopened",
              lead: `Nobody opened it before it expired ${when}. Nothing is left on the server.`,
            };
      }
      return {
        title: "This secret expired",
        lead: `It expired ${when}. Nothing is left on the server, so ask the sender for a new link if you still need it.`,
      };
    case "deleted":
      if (owner) {
        const before = firstOpened ? ` It had been opened before, first ${firstOpened}.` : "";
        return {
          title: "Secret deleted",
          lead: `You deleted it ${when}. The link doesn't open anything any more.${before}`,
        };
      }
      return {
        title: "This secret was deleted",
        lead: `The sender deleted it ${when}. ${ASK_AGAIN}`,
      };
  }
}

/** What became of a secret that is gone, told to whoever holds a link to it. */
export default function ShareGone({ gone, owner }: ShareGoneProps) {
  const { title, lead } = goneCopy(gone, owner);
  return (
    <div className="space-y-8">
      <PageTitle lead={lead}>{title}</PageTitle>
      <a href={owner ? "/" : "/share"} className={textButtonClass("muted")}>
        ← {owner ? "Share another secret" : "Share a secret of your own"}
      </a>
    </div>
  );
}
