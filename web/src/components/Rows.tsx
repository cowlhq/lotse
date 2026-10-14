import type { AlertState, Check, HostSummary } from "@/api";
import { levelOfSubject, worse, type Level } from "@/lib/status";

// How a host and a check stand, for lists and tables.

export function hostLevel(h: HostSummary, alerts: AlertState[]): Level {
  if (!h.host.online) return h.host.agent_revoked ? "unknown" : "crit";
  return worse(levelOfSubject(alerts, "host", h.host.id), h.containers.unhealthy > 0 ? "warn" : "ok");
}

export function checkLevel(c: Check): Level {
  return c.state === "down" ? "crit" : c.state === "up" ? "ok" : "unknown";
}

