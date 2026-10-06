export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * A moment the way a person would say it: "today at 19:53", "tomorrow at
 * 9:00", "yesterday at 8:10", a weekday ("on Tuesday at 14:02") within the
 * past week, and the date beyond that.
 */
export function formatMoment(iso: string, now: Date = new Date()): string {
  const date = new Date(iso);
  const time = date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  const days = calendarDaysBetween(now, date);
  if (days === 0) return `today at ${time}`;
  if (days === 1) return `tomorrow at ${time}`;
  if (days === -1) return `yesterday at ${time}`;
  if (days < 0 && days > -7) {
    return `on ${date.toLocaleDateString(undefined, { weekday: "long" })} at ${time}`;
  }
  const day = date.toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
    year: date.getFullYear() === now.getFullYear() ? undefined : "numeric",
  });
  return `on ${day} at ${time}`;
}

/** The moment a link dies, the way a person would say it: "tomorrow at 19:53". */
export const formatExpiry = formatMoment;

/** Whole calendar days from a to b, in local time. */
function calendarDaysBetween(a: Date, b: Date): number {
  const dayA = new Date(a.getFullYear(), a.getMonth(), a.getDate());
  const dayB = new Date(b.getFullYear(), b.getMonth(), b.getDate());
  return Math.round((dayB.getTime() - dayA.getTime()) / 86_400_000);
}

export function formatRelativeTime(iso: string): string {
  const now = Date.now();
  const target = new Date(iso).getTime();
  const diffMs = target - now;
  const absDiff = Math.abs(diffMs);
  const future = diffMs > 0;

  const seconds = Math.floor(absDiff / 1000);
  if (seconds < 60) return future ? "in a few seconds" : "just now";

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    const label = minutes === 1 ? "1 minute" : `${minutes} minutes`;
    return future ? `in ${label}` : `${label} ago`;
  }

  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    const label = hours === 1 ? "1 hour" : `${hours} hours`;
    return future ? `in ${label}` : `${label} ago`;
  }

  const days = Math.floor(hours / 24);
  const label = days === 1 ? "1 day" : `${days} days`;
  return future ? `in ${label}` : `${label} ago`;
}
