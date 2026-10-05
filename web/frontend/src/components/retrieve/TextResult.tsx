import { useEffect, useRef, useState } from "react";
import { useCopied } from "../../hooks/useCopied";
import { useLeaveWarning } from "../../hooks/useLeaveWarning";
import { copyToClipboard } from "../../lib/clipboard";
import Button from "../ui/Button";
import { ArrowRightIcon, CheckIcon, CopyIcon } from "../ui/icons";
import Note from "../ui/Note";
import PageTitle from "../ui/PageTitle";
import { textButtonClass } from "../ui/styles";
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
  const [copied, markCopied] = useCopied();
  const [everCopied, setEverCopied] = useState(false);
  const preRef = useRef<HTMLPreElement>(null);
  // A one-time text is gone from the server: until it was copied, this page
  // has the only copy.
  useLeaveWarning(burnAfterRead && !everCopied);

  // Copying by hand counts too.
  useEffect(() => {
    if (!burnAfterRead) return;
    const markCopied = () => setEverCopied(true);
    document.addEventListener("copy", markCopied);
    return () => document.removeEventListener("copy", markCopied);
  }, [burnAfterRead]);

  async function copy() {
    if (await copyToClipboard(text, preRef.current)) {
      markCopied();
      setEverCopied(true);
    }
  }

  return (
    <div className="space-y-7">
      <PageTitle
        lead={
          burnAfterRead
            ? "This was its only opening. It's been deleted from the server."
            : "It stays available until the link expires."
        }
      >
        Here's your secret
      </PageTitle>

      <div className="overflow-hidden rounded-[20px] border border-line bg-surface shadow-card">
        <pre
          ref={preRef}
          className="m-0 min-h-24 overflow-x-auto whitespace-pre-wrap break-words px-6 py-5.5 font-mono text-body leading-[1.7] text-ink"
        >
          {text}
        </pre>
        <div className="flex flex-wrap items-center gap-0.5 border-t border-line p-2">
          <Button onClick={copy} className="pl-4">
            {copied ? <CheckIcon /> : <CopyIcon />}
            {copied ? "Copied" : "Copy secret"}
          </Button>
        </div>
      </div>

      {burnAfterRead && <Note>Copy it now. When you leave this page, it's gone for good.</Note>}

      {canDelete && <DeleteShareButton deleting={deleting} onDelete={onDelete} />}

      <a href="/share" className={textButtonClass("muted")}>
        Share a secret of your own
        <ArrowRightIcon />
      </a>
    </div>
  );
}
