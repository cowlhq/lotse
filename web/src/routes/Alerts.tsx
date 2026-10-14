import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { acknowledgeAlert, getAlert, listAlerts, NotFound } from "@/api";
import { AlertBadge, AlertHistory, AlertList, SubjectLink, value } from "@/components/Alerts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { dateTime } from "@/lib/format";
import { useMe } from "@/lib/useMe";

// Alerts is what fires now, everywhere, and what happened.
export function Alerts() {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">{t("alert.title")}</h1>
      <AlertList always />
      <AlertHistory />
    </div>
  );
}

// AlertDetail is one transition: what it measured, against which rule as it
// applied, and where that rule's settings come from; and what was sent.
export function AlertDetail({ id }: { id: number }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const alert = useQuery({ queryKey: ["alerts", "detail", id], queryFn: () => getAlert(id) });
  const current = useQuery({ queryKey: ["alerts", "all"], queryFn: () => listAlerts() });
  const ack = useMutation({
    mutationFn: () => acknowledgeAlert(id),
    onSuccess: () => client.invalidateQueries({ queryKey: ["alerts"] }),
  });
  if (alert.isPending) return <Skeleton className="h-32 w-full" />;
  if (alert.error instanceof NotFound) return <p>{t("alert.notFound")}</p>;
  if (alert.isError) return <p className="text-destructive">{t("app.unreachable")}</p>;
  const a = alert.data;
  const now = current.data?.find((s) => s.alert_id === id);
  const firing = now && (now.state === "crit" || now.state === "warn");
  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">
          <SubjectLink kind={a.subject_kind} id={a.subject_id} path={a.subject_path} />
          {a.instance && <span className="ml-1 font-mono text-xs">{a.instance}</span>}
        </p>
        <h1 className="flex items-center gap-3 text-xl font-semibold">
          <AlertBadge state={a.to} />
          <span className="font-mono text-lg">{a.rule}</span>
        </h1>
        <p className="text-sm text-muted-foreground">{a.description}</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("alert.what")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <p>
            {t("alert.went", { from: t(`alert.state.${a.from}`), to: t(`alert.state.${a.to}`), time: dateTime(a.at) })}
          </p>
          {a.value != null && (
            <p>
              {t("alert.measured", { value: value(a.value, a.unit) })}
              {a.threshold != null && <> · {t("alert.threshold", { value: value(a.threshold, a.unit) })}</>}
            </p>
          )}
          {a.reason && <p className="text-muted-foreground">{a.reason}</p>}
          {a.in_maintenance && <p className="text-muted-foreground">{t("alert.inMaintenance")}</p>}
          {now && (
            <p className="flex items-center gap-3">
              {t("alert.now")} <AlertBadge state={now.state} />
              {now.acked_by && <span className="text-muted-foreground">{t("alert.ackedBy", { who: now.acked_by })}</span>}
              {isAdmin && firing && !now.acked_by && (
                <Button size="sm" variant="outline" disabled={ack.isPending} onClick={() => ack.mutate()}>
                  {t("alert.acknowledge")}
                </Button>
              )}
            </p>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("alert.why")}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("alert.setting")}</TableHead>
                <TableHead>{t("alert.settingValue")}</TableHead>
                <TableHead>{t("alert.settingFrom")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {a.settings.map((s) => (
                <TableRow key={s.key}>
                  <TableCell className="font-mono text-xs">{s.key}</TableCell>
                  <TableCell className="tabular-nums">{Array.isArray(s.value) ? s.value.join(", ") || "–" : String(s.value)}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {s.from ? t("config.from", { kind: t(`config.kinds.${s.from.kind}`), name: s.from.name || s.from.path }) : t("config.default")}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("alert.notifications")}</CardTitle>
        </CardHeader>
        <CardContent>
          {a.notifications.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("alert.noNotifications")}</p>
          ) : (
            <ul className="space-y-1.5 text-sm">
              {a.notifications.map((n) => (
                <li key={n.id} className="grid grid-cols-[11rem_1fr] gap-4">
                  <span className="tabular-nums whitespace-nowrap text-muted-foreground">{dateTime(n.sent_at ?? n.created_at)}</span>
                  <span>
                    <span className="font-medium">{n.channel}</span> · {t(`alert.status.${n.status}`)}
                    {n.kind === "repeat" && <> · {t("alert.repeat")}</>}
                    {n.attempts > 1 && <> · {t("alert.attempts", { count: n.attempts })}</>}
                    {n.error && <p className="text-destructive">{n.error}</p>}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
