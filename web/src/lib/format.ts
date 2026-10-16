// How values are shown. Sizes use binary units (KiB, MiB, …), as free and
// df do.

const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];

export function bytes(n: number | null | undefined, digits = 1): string {
  if (n == null) return "–";
  let i = 0;
  let v = n;
  while (Math.abs(v) >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  // 1000.0 KiB says no more than 1000 KiB
  const text = i === 0 || Number.isInteger(v) ? v.toFixed(0) : v.toFixed(digits);
  return `${text} ${units[i]}`;
}

// digitsFor is how many decimals tell ticks incr apart: as many as the
// step has, none for 5, one for 2.5 or 0.1, two for 0.05, …
export function digitsFor(incr: number): number {
  for (let d = 0; d < 4; d++) {
    const scaled = incr * 10 ** d;
    if (Math.abs(scaled - Math.round(scaled)) < 1e-6 * scaled) return d;
  }
  return 4;
}

export function rate(n: number | null | undefined): string {
  return n == null ? "–" : `${bytes(n)}/s`;
}

export function percent(n: number | null | undefined, digits = 0): string {
  return n == null ? "–" : `${n.toFixed(digits)} %`;
}

export function number(n: number | null | undefined, digits = 2): string {
  return n == null ? "–" : n.toFixed(digits);
}

// duration says how long seconds are, in the two largest units.
export function duration(seconds: number | null | undefined): string {
  if (seconds == null) return "–";
  const parts: [number, string][] = [
    [Math.floor(seconds / 86400), "d"],
    [Math.floor((seconds % 86400) / 3600), "h"],
    [Math.floor((seconds % 3600) / 60), "m"],
    [Math.floor(seconds % 60), "s"],
  ];
  const first = parts.findIndex(([v]) => v > 0);
  if (first < 0) return "0s";
  return parts
    .slice(first, first + 2)
    .filter(([v]) => v > 0)
    .map(([v, u]) => `${v}${u}`)
    .join(" ");
}

export function ago(ts: number | null | undefined, now = Date.now() / 1000): string {
  if (!ts) return "–";
  return duration(Math.max(0, now - ts));
}

// Dates and times in the reader's own format.
const fullDateTime = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });
const timeOnly = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const dayAndTime = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

export function dateTime(ts: number): string {
  return fullDateTime.format(ts * 1000);
}

// shortDateTime is the time alone for today, else the day and time, for
// lists where the year goes without saying.
export function shortDateTime(ts: number, now = new Date()): string {
  const d = new Date(ts * 1000);
  return d.toDateString() === now.toDateString() ? timeOnly.format(d) : dayAndTime.format(d);
}
