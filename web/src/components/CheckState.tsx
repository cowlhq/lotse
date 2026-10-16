import { useTranslation } from "react-i18next";
import type { Check } from "@/api";
import { chrome, status } from "@/lib/palette";

const colors: Record<Check["state"], string> = { up: status.good, down: status.critical, unknown: chrome.muted };

// CheckState names a check's state in words; the dot only backs it up.
export function CheckState({ state }: { state: Check["state"] }) {
  const { t } = useTranslation();
  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      <span className="size-2 rounded-full" style={{ background: colors[state] }} />
      {t(`check.state.${state}`)}
    </span>
  );
}

// target is what a check looks at, for lists.
export function target(c: Check): string {
  const spec = c.spec as Record<string, string> & { expect?: string[] };
  if (c.type === "http") return spec.method ? `${spec.method} ${spec.url}` : spec.url;
  if (c.type === "heartbeat") return `${spec.period} + ${spec.grace}`;
  if (c.type === "dns") return `${spec.name} ${spec.type}${spec.expect?.length ? ` → ${spec.expect.join(", ")}` : ""}`;
  if (c.type === "domain") return spec.domain;
  return spec.address;
}

// certDays is how many days a check's certificate has left, if it has one.
export function certDays(c: { cert_not_after?: number }): number | undefined {
  if (!c.cert_not_after) return undefined;
  return Math.floor((c.cert_not_after - Date.now() / 1000) / 86400);
}
