/**
 * A share secret, optionally followed by `!` and the owner's deletion token.
 * Both are 32 bytes, which base64url-encode to 43 characters.
 */
const SHARE_FRAGMENT = /^[A-Za-z0-9_-]{43}(![A-Za-z0-9_-]{43})?$/;

/** Whether a URL fragment (without `#`) has the shape of a share link's. */
export function isShareFragment(fragment: string): boolean {
  return SHARE_FRAGMENT.test(fragment);
}

export type ShareLinkResult =
  | { readonly kind: "share"; readonly fragment: string }
  | { readonly kind: "other-host"; readonly host: string }
  | { readonly kind: "invalid" };

/**
 * Checks a pasted or scanned share link. Only links for `origin` are opened:
 * the share lives on the server that created it, and a scanned code may point
 * anywhere.
 */
export function parseShareLink(text: string, origin: string): ShareLinkResult {
  let url: URL;
  try {
    url = new URL(text.trim());
  } catch {
    return { kind: "invalid" };
  }

  if (url.protocol !== "https:" && url.protocol !== "http:") {
    return { kind: "invalid" };
  }

  const fragment = url.hash.slice(1);
  if (url.pathname !== "/s" || !isShareFragment(fragment)) {
    return { kind: "invalid" };
  }

  if (url.origin !== origin) {
    return { kind: "other-host", host: url.host };
  }

  return { kind: "share", fragment };
}
