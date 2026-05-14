const EXPIRATION_LABELS: Record<string, string> = {
  "5m": "5 minutes",
  "10m": "10 minutes",
  "15m": "15 minutes",
  "1h": "1 hour",
  "4h": "4 hours",
  "12h": "12 hours",
  "1d": "1 day",
  "3d": "3 days",
  "7d": "7 days",
};

export function formatExpiration(value: string): string {
  return EXPIRATION_LABELS[value] ?? value;
}
