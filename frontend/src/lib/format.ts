/**
 * Format a Unix timestamp (seconds) into a human-readable local date/time string.
 * Uses the browser's Intl API so the output always reflects the user's OS timezone.
 */
export function formatUnix(value: number) {
  return new Date(value * 1000).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    timeZoneName: "short",
  });
}

/**
 * Returns the browser's current timezone abbreviation (e.g. "GMT+2", "CET", "PST").
 * Used to display a note in the UI footer so users know what timezone dates refer to.
 */
export function getBrowserTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

export function formatNumber(value: number) {
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 2,
  }).format(value);
}
