import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { listAlertHistory, listAlerts, type Alert, type AlertState } from "@/api";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { dateTime, shortDateTime } from "@/lib/format";
import { alertTitle, subjectName } from "@/lib/alerts";
import { levelColor } from "@/lib/status";
import { StatusIcon } from "@/components/shell/Dot";
import { Panel } from "@/components/shell/Panel";
import { useLive } from "@/lib/live";

// value writes a rule's value in its unit.
export function value(v: number | undefined, unit: AlertState["unit"]): string {
  if (v == null) return "";
  const n = Number.isInteger(v) ? String(v) : v.toFixed(unit === "load" ? 2 : 1);
  switch (unit) {
    case "percent":
      return `${n} %`;
    case "days":
      return `${n} d`;
    default:
      return n;
  }
}

// SubjectLink leads to the object an alert is about.
export function SubjectLink({ kind, id, path }: { kind: AlertState["subject_kind"]; id: number; path: string }) {
  const p = { id: String(id) };
  switch (kind) {
    case "host":
      return <Link to="/hosts/$id" params={p} className="hover:underline">{path}</Link>;
    case "container":
      return <Link to="/containers/$id" params={p} className="hover:underline">{path}</Link>;
    case "check":
      return <Link to="/checks/$id" params={p} className="hover:underline">{path}</Link>;
    default:
      return <Link to="/settings" className="hover:underline">{path}</Link>;
  }
}

// useAlerts are the alerts that are not OK, of an object or a scope or of
// everything, kept fresh by the live connection.
export function useAlerts(subject?: string) {
  const client = useQueryClient();
  useLive(["alerts"], (m) => {
    if (m.type === "alerts") client.invalidateQueries({ queryKey: ["alerts"] });
  });
  return useQuery({ queryKey: ["alerts", subject ?? "all"], queryFn: () => listAlerts(subject), refetchInterval: 60_000 });
}

// AlertHistory lists the transitions of alerts, newest first, over the last
// days: alerts are kept longer than readings.
export function AlertHistory({ subject, always = false }: { subject?: string; always?: boolean }) {
  const { t } = useTranslation();
  const [days, setDays] = useState(7);
  const history = useQuery({
    queryKey: ["alerts", "history", subject ?? "all", days],
    queryFn: () => {
      const to = Math.floor(Date.now() / 1000);
      return listAlertHistory({ from: to - days * 86400, to }, subject);
    },
  });
  // Between OK and unknown nothing was wrong: readings were missing, which
  // an alert of its own says, such as a silent agent.
  const quiet = (s: Alert["from"]) => s === "ok" || s === "unknown";
  const list = (history.data ?? []).filter((a) => !(quiet(a.from) && quiet(a.to)));
  if (subject && !always && list.length === 0) return null;
  return (
    <Panel
      title={t("alert.history")}
      flush
      right={
        <ToggleGroup value={[String(days)]} onValueChange={(v: string[]) => v[0] && setDays(Number(v[0]))} variant="segmented" size="sm">
          {[1, 7, 30].map((d) => (
            <ToggleGroupItem key={d} value={String(d)} aria-label={t("range.days", { count: d })} title={t("range.days", { count: d })}>
              {t("range.short.days", { count: d })}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      }
    >
      {list.length === 0 ? (
        <p className="px-4 py-10 text-center text-muted-foreground">{t("alert.noHistory")}</p>
      ) : (
        <ul>
          {list.map((a) => (
            <Transition key={a.id} alert={a} showSubject={!subject || !subject.startsWith(a.subject_kind)} />
          ))}
        </ul>
      )}
    </Panel>
  );
}

function Transition({ alert: a, showSubject }: { alert: Alert; showSubject: boolean }) {
  const { t } = useTranslation();
  const { name } = subjectName(a);
  return (
    <li>
      <Link to="/alerts/$id" params={{ id: String(a.id) }} className="flex items-start gap-3 border-b px-4 py-3 transition-colors last:border-b-0 hover:bg-accent/60 sm:items-center">
        <span className="mt-0.5 sm:mt-0">
          <StatusIcon level={a.to === "ok" ? "ok" : a.to} />
        </span>
        <span className="hidden w-16 shrink-0 font-medium sm:inline" style={{ color: a.to === "ok" ? "var(--ok)" : a.to === "unknown" ? undefined : levelColor[a.to] }}>
          {t(`alert.state.${a.to}`)}
        </span>
        <span className="min-w-0 flex-1 sm:truncate">
          <span className="font-medium">{alertTitle(t, a)}</span>
          {showSubject && <span> {name}</span>}
          {a.instance && <code className="ml-1.5 rounded bg-muted px-1 py-px font-mono text-mini">{a.instance}</code>}
          <span className="block text-muted-foreground sm:inline">
            {/* The reason says the value in words; without one, the value. */}
            {a.reason ? <span className="sm:before:content-['_·_']">{a.reason}</span> : a.value != null && a.unit && <span className="sm:before:content-['_·_']">{value(a.value, a.unit)}</span>}
            {a.in_maintenance && <> · {t("alert.maintenance")}</>}
          </span>
        </span>
        <span className="shrink-0 text-mini whitespace-nowrap tabular-nums text-muted-foreground" title={dateTime(a.at)}>
          {shortDateTime(a.at)}
        </span>
      </Link>
    </li>
  );
}
