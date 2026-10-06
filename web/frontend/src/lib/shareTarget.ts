/**
 * Text shared into the installed app arrives as the query of /share (the
 * manifest's share target). This reads it and takes it out of the address
 * bar at once, so it stays out of the history and out of any later request.
 * The service worker has already kept it off the wire on the way in.
 */
export function takeSharedText(location: Location = window.location): string {
  if (!location.search) return "";
  const params = new URLSearchParams(location.search);
  const title = params.get("title")?.trim() ?? "";
  const text = params.get("text")?.trim() ?? "";
  const url = params.get("url")?.trim() ?? "";
  // Apps share a page as text plus url, or as url alone; a note comes as
  // text. The odd app sends nothing but a title, which is then all there is.
  const shared = text && url && !text.includes(url) ? `${text}\n${url}` : text || url || title;
  if (params.has("text") || params.has("url") || params.has("title")) {
    window.history.replaceState(null, "", location.pathname);
  }
  return shared;
}
