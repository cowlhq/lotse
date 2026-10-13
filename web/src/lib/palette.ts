// Colors for data, from the dataviz reference palette. Series take the
// categorical slots in this order and never cycle; the first three are safe
// together for color-blind readers. Status colors are reserved for state and
// always come with a label.

export const series = ["#2a78d6", "#eb6834", "#1baf7a"] as const;

export const status = {
  good: "#0ca30c",
  warning: "#fab219",
  serious: "#ec835a",
  critical: "#d03b3b",
} as const;

export const chrome = {
  grid: "#e1e0d9",
  axis: "#c3c2b7",
  muted: "#898781",
} as const;

// usageColor is the fill of a usage meter: it turns to warning and critical
// at the thresholds of the default disk rule (85 and 95 %).
export function usageColor(pct: number): string {
  if (pct >= 95) return status.critical;
  if (pct >= 85) return status.warning;
  return series[0];
}
