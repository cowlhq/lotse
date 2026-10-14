// Colors for data and state. They live in index.css as variables, so that
// light and dark each have their own; the DOM uses the variables, and
// canvases (charts) resolve them with css(). Series take the chart slots in
// order and never cycle. Status colors mean only state and always come
// with a word.

export const series = ["var(--chart-1)", "var(--chart-2)", "var(--chart-3)"] as const;

export const status = {
  good: "var(--ok)",
  warning: "var(--warn)",
  serious: "var(--warn)",
  critical: "var(--crit)",
  unknown: "var(--unknown)",
} as const;

export const chrome = {
  grid: "var(--border)",
  axis: "var(--border)",
  muted: "var(--muted-foreground)",
} as const;

// css resolves a color such as "var(--crit)" for a canvas, in the theme
// shown now.
export function css(color: string): string {
  const m = /^var\((--[\w-]+)\)$/.exec(color);
  if (!m) return color;
  return getComputedStyle(document.documentElement).getPropertyValue(m[1]).trim() || color;
}

// usageColor is the fill of a usage meter: it turns to warning and critical
// at the thresholds of the default disk rule (85 and 95 %).
export function usageColor(pct: number): string {
  if (pct >= 95) return status.critical;
  if (pct >= 85) return status.warning;
  return series[0];
}
