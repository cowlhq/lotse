import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { CircleAlert, CircleCheck, CircleHelp, TriangleAlert, Wrench } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { acknowledgeAlert, listAlertHistory, listAlerts, type Alert, type AlertState } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ago, dateTime } from "@/lib/format";
import { useLive } from "@/lib/live";
import { chrome, status } from "@/lib/palette";
import { useMe } from "@/lib/useMe";

type State = AlertState["state"];

const look: Record<State, { color: string; Icon: typeof CircleAlert }> = {
  crit: { color: status.critical, Icon: CircleAlert },
  warn: { color: status.warning, Icon: TriangleAlert },
  unknown: { color: chrome.muted, Icon: CircleHelp },
  ok: { color: status.good, Icon: CircleCheck },
};

// AlertBadge names a state in words, with an icon; the color only backs
// them up.
export function AlertBadge({ state }: { state: State }) {
  const { t } = useTranslation();
  const { color, Icon } = look[state];
  return (
    <span className="inline-flex items-center gap-1.5 font-medium whitespace-nowrap" style={{ color: state === "unknown" ? undefined : color }}>
      <Icon className="size-4" style={{ color }} />
      {t(`alert.state.${state}`)}
    </span>
  );
}

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

// AlertList shows what fires now. Without anything to show, it shows
// nothing, unless it is the alerts page.
export function AlertList({ subject, always = false }: { subject?: string; always?: boolean }) {
  const { t } = useTranslation();
  const alerts = useAlerts(subject);
  const list = alerts.data ?? [];
  if (!always && list.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("alert.firing")}</CardTitle>
      </CardHeader>
      <CardContent>
        {list.length === 0 ? (
          <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <CircleCheck className="size-4" style={{ color: status.good }} />
            {alerts.isPending ? "…" : t("alert.allGood")}
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("alert.stateTitle")}</TableHead>
                <TableHead>{t("alert.rule")}</TableHead>
                <TableHead>{t("alert.subject")}</TableHead>
                <TableHead className="text-right">{t("alert.value")}</TableHead>
                <TableHead>{t("alert.since")}</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {list.map((a) => (
                <Row key={`${a.rule}|${a.subject_kind}|${a.subject_id}|${a.instance}`} alert={a} />
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function Row({ alert: a }: { alert: AlertState }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const ack = useMutation({
    mutationFn: () => acknowledgeAlert(a.alert_id),
    onSuccess: () => client.invalidateQueries({ queryKey: ["alerts"] }),
  });
  const firing = a.state === "crit" || a.state === "warn";
  return (
    <TableRow>
      <TableCell>
        {a.state === "ok" && a.pending ? (
          <span className="text-muted-foreground">{t("alert.pending", { state: t(`alert.state.${a.pending}`) })}</span>
        ) : (
          <AlertBadge state={a.state} />
        )}
      </TableCell>
      <TableCell>
        <Link to="/alerts/$id" params={{ id: String(a.alert_id) }} className="font-mono text-xs hover:underline" title={a.description}>
          {a.rule}
        </Link>
        {a.reason && <p className="text-xs text-muted-foreground">{a.reason}</p>}
      </TableCell>
      <TableCell>
        <SubjectLink kind={a.subject_kind} id={a.subject_id} path={a.subject_path} />
        {a.instance && <span className="ml-1 font-mono text-xs text-muted-foreground">{a.instance}</span>}
      </TableCell>
      <TableCell className="text-right tabular-nums whitespace-nowrap">
        {value(a.value, a.unit)}
        {a.threshold != null && <span className="text-muted-foreground"> / {value(a.threshold, a.unit)}</span>}
      </TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground">
        <span title={dateTime(a.since)}>{ago(a.since)}</span>
        {a.in_maintenance && (
          <span className="ml-2 inline-flex items-center gap-1" title={t("alert.inMaintenance")}>
            <Wrench className="size-3.5" />
            {t("alert.maintenance")}
          </span>
        )}
      </TableCell>
      <TableCell className="text-right">
        {a.acked_by ? (
          <span className="text-xs text-muted-foreground" title={a.acked_at ? dateTime(a.acked_at) : undefined}>
            {t("alert.ackedBy", { who: a.acked_by })}
          </span>
        ) : (
          isAdmin &&
          firing && (
            <Button size="sm" variant="outline" disabled={ack.isPending} onClick={() => ack.mutate()}>
              {t("alert.acknowledge")}
            </Button>
          )
        )}
      </TableCell>
    </TableRow>
  );
}

// AlertHistory lists the transitions of alerts, newest first, over the last
// days: alerts are kept longer than readings.
export function AlertHistory({ subject }: { subject?: string }) {
  const { t } = useTranslation();
  const [days, setDays] = useState(7);
  const history = useQuery({
    queryKey: ["alerts", "history", subject ?? "all", days],
    queryFn: () => {
      const to = Math.floor(Date.now() / 1000);
      return listAlertHistory({ from: to - days * 86400, to }, subject);
    },
  });
  const list = history.data ?? [];
  if (subject && list.length === 0) return null;
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-4">
        <CardTitle className="text-sm font-medium">{t("alert.history")}</CardTitle>
        <ToggleGroup value={[String(days)]} onValueChange={(v: string[]) => v[0] && setDays(Number(v[0]))} variant="outline" size="sm">
          {[1, 7, 30].map((d) => (
            <ToggleGroupItem key={d} value={String(d)}>
              {t("range.days", { count: d })}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </CardHeader>
      <CardContent>
        {list.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("alert.noHistory")}</p>
        ) : (
          <ul className="space-y-1.5 text-sm">
            {list.map((a) => (
              <Transition key={a.id} alert={a} showSubject={!subject || !subject.startsWith(a.subject_kind)} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function Transition({ alert: a, showSubject }: { alert: Alert; showSubject: boolean }) {
  const { t } = useTranslation();
  return (
    <li className="grid grid-cols-[12rem_6rem_1fr] items-baseline gap-4">
      <span className="tabular-nums whitespace-nowrap text-muted-foreground">{dateTime(a.at)}</span>
      <AlertBadge state={a.to} />
      <span className="min-w-0">
        <Link to="/alerts/$id" params={{ id: String(a.id) }} className="font-mono text-xs hover:underline">
          {a.rule}
        </Link>
        {showSubject && (
          <>
            {" "}
            <SubjectLink kind={a.subject_kind} id={a.subject_id} path={a.subject_path} />
          </>
        )}
        {a.instance && <span className="ml-1 font-mono text-xs text-muted-foreground">{a.instance}</span>}
        {a.value != null && a.unit && <span className="ml-2 tabular-nums text-muted-foreground">{value(a.value, a.unit)}</span>}
        {a.reason && <span className="ml-2 text-muted-foreground">{a.reason}</span>}
        {a.in_maintenance && <span className="ml-2 text-xs text-muted-foreground">({t("alert.maintenance")})</span>}
      </span>
    </li>
  );
}
