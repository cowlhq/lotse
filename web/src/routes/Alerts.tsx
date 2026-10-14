import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { acknowledgeAlert, getAlert, listAlerts, NotFound } from "@/api";
import { AlertHistory, SubjectLink, useAlerts, value } from "@/components/Alerts";
import { Incidents } from "@/components/Incidents";
import { StatusIcon } from "@/components/shell/Dot";
import { Page } from "@/components/shell/PageHeader";
import { Facts, ObjectHeader, Panel } from "@/components/shell/Panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { alertTitle } from "@/lib/alerts";
import { ago, dateTime } from "@/lib/format";
import { useMe } from "@/lib/useMe";

// settingLabel says what a rule's setting is, in words.
function settingLabel(t: (k: string) => string, key: string): string {
  if (key === "notify.channels") return t("why.channels");
  if (key === "notify.repeat") return t("why.repeat");
  return t(`why.${key.slice(key.lastIndexOf(".") + 1)}`);
}

// Alerts is what fires now, everywhere, and what happened before.
export function Alerts() {
  const { t } = useTranslation();
  const alerts = useAlerts();
  const now = (alerts.data ?? []).filter((a) => a.state !== "ok" || a.pending);
  return (
    <>
      <ObjectHeader icon={<StatusIcon level={now.some((a) => a.state === "crit") ? "crit" : now.some((a) => a.state === "warn") ? "warn" : "ok"} size={22} />} title={t("alert.title")} />
      <Page>
        <Panel title={t("alert.firing")} count={now.length} flush>
          {now.length === 0 ? <p className="px-4 py-10 text-center text-muted-foreground">{t("alert.allGood")}</p> : <Incidents alerts={now} />}
        </Panel>
        <AlertHistory always />
      </Page>
    </>
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
  if (alert.isPending) return <Skeleton className="m-6 h-32" />;
  if (alert.error instanceof NotFound) return <p className="p-6">{t("alert.notFound")}</p>;
  if (alert.isError) return <p className="p-6 text-destructive">{t("app.unreachable")}</p>;
  const a = alert.data;
  const now = current.data?.find((s) => s.alert_id === id);
  const firing = now && (now.state === "crit" || now.state === "warn");
  return (
    <>
      <ObjectHeader
        above={
          <>
            <SubjectLink kind={a.subject_kind} id={a.subject_id} path={a.subject_path} />
            {a.instance && <span className="ml-1 font-mono text-mini">{a.instance}</span>}
          </>
        }
        icon={<StatusIcon level={a.to === "ok" ? "ok" : a.to} size={22} />}
        title={alertTitle(t, a)}
        actions={
          isAdmin &&
          firing &&
          (now.acked_by ? (
            <span className="text-muted-foreground">{t("alert.ackedBy", { who: now.acked_by })}</span>
          ) : (
            <Button variant="outline" disabled={ack.isPending} onClick={() => ack.mutate()}>
              {t("alert.acknowledge")}
            </Button>
          ))
        }
        facts={
          <Facts
            items={[
              t("alert.went", { from: t(`alert.state.${a.from}`), to: t(`alert.state.${a.to}`), time: dateTime(a.at) }),
              a.value != null && a.unit && (
                <span>
                  {t("alert.measured", { value: value(a.value, a.unit) })}
                  {a.threshold != null && <>, {t("alert.threshold", { value: value(a.threshold, a.unit) })}</>}
                </span>
              ),
              a.reason,
              a.in_maintenance && t("alert.maintenance"),
            ]}
          />
        }
      />
      <Page>
        {now && (
          <p className="flex items-center gap-2 text-muted-foreground">
            {t("alert.now")}: <StatusIcon level={now.state === "ok" ? "ok" : now.state} size={14} />
            <span className="text-foreground">{t(`alert.state.${now.state}`)}</span>
            {t("alert.sinceAgo", { time: ago(now.since) })}
          </p>
        )}
        <Panel title={t("alert.why")} flush>
          <p className="border-b px-4 py-3 text-muted-foreground">{a.description}</p>
          <ul>
            {a.settings.map((s) => (
              <li key={s.key} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-4 border-b px-4 py-2.5 last:border-b-0 sm:grid-cols-[minmax(0,1fr)_10rem_14rem]">
                <span>{settingLabel(t, s.key)}</span>
                <span className="tabular-nums">{Array.isArray(s.value) ? s.value.join(", ") || "–" : String(s.value)}</span>
                <span className="hidden text-muted-foreground sm:block">
                  {s.from ? t("config.from", { kind: t(`config.kinds.${s.from.kind}`), name: s.from.name || s.from.path }) : t("config.default")}
                </span>
              </li>
            ))}
          </ul>
        </Panel>
        <Panel title={t("alert.notifications")} flush>
          {a.notifications.length === 0 ? (
            <p className="px-4 py-6 text-muted-foreground">{t("alert.noNotifications")}</p>
          ) : (
            <ul>
              {a.notifications.map((n) => (
                <li key={n.id} className="flex items-center gap-3 border-b px-4 py-3 last:border-b-0">
                  <StatusIcon level={n.status === "sent" ? "ok" : n.status === "failed" ? "crit" : "unknown"} />
                  <span className="font-medium">{n.channel}</span>
                  <span className="text-muted-foreground">
                    {t(`alert.status.${n.status}`)}
                    {n.kind === "repeat" && <> · {t("alert.repeat")}</>}
                    {n.attempts > 1 && <> · {t("alert.attempts", { count: n.attempts })}</>}
                  </span>
                  {n.error && <span className="min-w-0 flex-1 truncate" style={{ color: "var(--crit)" }}>{n.error}</span>}
                  <span className="ml-auto text-mini whitespace-nowrap tabular-nums text-muted-foreground">{dateTime(n.sent_at ?? n.created_at)}</span>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </Page>
    </>
  );
}
