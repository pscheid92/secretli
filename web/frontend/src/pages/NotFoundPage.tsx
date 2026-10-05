import { Link } from "react-router";
import PageTitle from "../components/ui/PageTitle";
import { textButtonClass } from "../components/ui/styles";
import { usePageTitle } from "../hooks/usePageTitle";

export default function NotFoundPage() {
  usePageTitle("Page not found");
  return (
    <div className="space-y-8">
      <PageTitle>This page doesn't exist</PageTitle>
      <Link to="/" className={textButtonClass("muted")}>
        ← Go home
      </Link>
    </div>
  );
}
