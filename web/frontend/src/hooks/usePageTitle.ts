import { useEffect } from "react";

/** Names the browser tab after the page that is showing. */
export function usePageTitle(title: string) {
  useEffect(() => {
    document.title = `${title} · Secretli`;
  }, [title]);
}
