import Spinner from "../Spinner";
import PageTitle from "../ui/PageTitle";
import { textButtonClass } from "../ui/styles";

export function RetrieveLoading() {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-20">
      <Spinner size="lg" className="text-accent" />
      <p className="text-sm text-muted">Checking share...</p>
    </div>
  );
}

export function RetrieveError({ message }: { message: string }) {
  return (
    <div className="space-y-8">
      <PageTitle lead={message}>Unable to open share</PageTitle>
      <a href="/share" className={textButtonClass("muted")}>
        ← Create a new share
      </a>
    </div>
  );
}

export function ShareDeleted() {
  return (
    <div className="space-y-8">
      <PageTitle lead="The share has been permanently destroyed.">Share deleted</PageTitle>
      <a href="/" className={textButtonClass("muted")}>
        ← Create a new share
      </a>
    </div>
  );
}
