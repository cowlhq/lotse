import type { AlertState } from "@/api";

// Level is how a thing stands, worst first in rank: crit, warn, unknown,
// ok. It comes from the alerts that are not OK.
export type Level = "crit" | "warn" | "unknown" | "ok";

const rank: Record<Level, number> = { crit: 3, warn: 2, unknown: 1, ok: 0 };

export function worse(a: Level, b: Level): Level {
  return rank[a] >= rank[b] ? a : b;
}

export const levelColor: Record<Level, string> = {
  crit: "var(--crit)",
  warn: "var(--warn)",
  unknown: "var(--unknown)",
  ok: "var(--ok)",
};

// levelOf is an alert's level; one that is only about to fire counts for
// nothing yet.
export function levelOf(a: AlertState): Level {
  return a.state === "ok" ? "ok" : a.state;
}

// levelUnder is the worst level of the alerts on a path and below it, such
// as an environment's acme/shop/prod; "" for everything.
export function levelUnder(alerts: AlertState[], path: string): Level {
  let level: Level = "ok";
  for (const a of alerts) {
    if (path === "" || a.subject_path === path || a.subject_path.startsWith(path + "/")) level = worse(level, levelOf(a));
  }
  return level;
}

// levelOfSubject is the worst level of the alerts on one object, and on
// what is on it (a host's containers).
export function levelOfSubject(alerts: AlertState[], kind: string, id: number, path?: string): Level {
  let level: Level = "ok";
  for (const a of alerts) {
    if ((a.subject_kind === kind && a.subject_id === id) || (path && a.subject_path.startsWith(path + "/"))) level = worse(level, levelOf(a));
  }
  return level;
}

// isFiring says whether an alert is sent and needs attention: WARN or
// CRIT. One that is unknown, because its readings are missing, is not a
// problem of its own; what keeps the readings away, such as a silent agent,
// is.
export function isFiring(a: Pick<AlertState, "state">) {
  return a.state === "crit" || a.state === "warn";
}

// firing are the alerts that are sent.
export function firing(alerts: AlertState[]): AlertState[] {
  return alerts.filter(isFiring);
}
