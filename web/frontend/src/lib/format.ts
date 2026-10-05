export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** The moment a link dies, the way a person would say it: "tomorrow at 19:53". */
export function formatExpiry(iso: string, now: Date = new Date()): string {
  const date = new Date(iso);
  const time = date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  const dayOf = (d: Date) => `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  if (dayOf(date) === dayOf(now)) return `today at ${time}`;
  if (dayOf(date) === dayOf(tomorrow)) return `tomorrow at ${time}`;
  return `on ${date.toLocaleDateString(undefined, { day: "numeric", month: "short" })} at ${time}`;
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
