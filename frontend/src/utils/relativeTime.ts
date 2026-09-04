/**
 * Formats an RFC3339 timestamp as a short relative time ("just now",
 * "5 minutes ago", "2 hours ago", "3 days ago"), falling back to a locale
 * date for older stamps. Empty/invalid input yields "".
 */
export function formatRelativeTime(iso: string | undefined | null, now: number = Date.now()): string {
  if (!iso) return "";
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  const diffMs = now - then;
  // Future stamps come from clock skew on LastUpdateCheck; clamping to
  // "just now" beats rendering them as "never checked".
  if (diffMs < 0) return "just now";
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diffMs < minute) return "just now";
  if (diffMs < hour) {
    const n = Math.floor(diffMs / minute);
    return `${n} minute${n === 1 ? "" : "s"} ago`;
  }
  if (diffMs < day) {
    const n = Math.floor(diffMs / hour);
    return `${n} hour${n === 1 ? "" : "s"} ago`;
  }
  if (diffMs < 30 * day) {
    const n = Math.floor(diffMs / day);
    return `${n} day${n === 1 ? "" : "s"} ago`;
  }
  return new Date(then).toLocaleDateString();
}
