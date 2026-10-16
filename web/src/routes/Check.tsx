import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Check as CheckIcon, Copy, MoreHorizontal, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { deleteCheck, getCheck, getSeries, listEvents, listScopes, NotFound, type CheckDetail } from "@/api";
import { AlertBell } from "@/components/AlertBell";
import { AlertHistory, useAlerts } from "@/components/Alerts";
import { Chart, type Line } from "@/components/Chart";
import { certDays, target } from "@/components/CheckState";
import { Config } from "@/components/Config";
import { Events } from "@/components/Events";
import { History } from "@/components/History";
import { Incidents } from "@/components/Incidents";
import { Maintenance } from "@/components/Maintenance";
import { RangePicker, type RangeSeconds } from "@/components/Range";
import { StatusIcon } from "@/components/shell/Dot";
import { Page } from "@/components/shell/PageHeader";
import { PageTabs } from "@/components/shell/PageTabs";
import { Facts, ObjectHeader, Panel } from "@/components/shell/Panel";
import { UptimeBar } from "@/components/UptimeBar";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { ago, dateTime, number, percent } from "@/lib/format";
import { useMe } from "@/lib/useMe";
import { useRange } from "@/lib/useRange";
import { isFiring } from "@/lib/status";

export function Check({ id }: { id: number }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const [seconds, setSeconds] = useState<RangeSeconds>(3600);
  const range = useRange(seconds);
  const check = useQuery({ queryKey: ["check", id], queryFn: () => getCheck(id), refetchInterval: 10_000 });
  const series = useQuery({
    queryKey: ["series", "checks", id, range],
    queryFn: () => getSeries("checks", id, range),
    placeholderData: (prev) => prev,
    refetchInterval: 30_000,
  });
  const events = useQuery({
    queryKey: ["events", "check", id, range],
    queryFn: () => listEvents(`check:${id}`, range),
    placeholderData: (prev) => prev,
    refetchInterval: 30_000,
  });
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const alertsQuery = useAlerts();
  const lines: Line[] = useMemo(() => [{ key: "latency_ms", label: t("check.latency") }], [t]);
  const format = useMemo(() => (v: number | null, d?: number) => (v == null ? "–" : `${number(v, d ?? 0)} ms`), []);

  if (check.isPending) return <Skeleton className="m-6 h-64" />;
  if (check.error instanceof NotFound) return <p className="p-6">{t("check.notFound")}</p>;
  if (check.isError) return <p className="p-6 text-destructive">{t("app.unreachable")}</p>;
  const c = check.data;
  const env = scopes.data?.find((s) => s.id === c.environment_id);
  // The share of runs that passed, over the range shown.
  const ok = series.data?.columns.ok?.filter((v): v is number => v != null) ?? [];
  const uptime = ok.length ? (100 * ok.reduce((a, b) => a + b, 0)) / ok.length : undefined;
  const days = certDays(c);
  const useAlertsData = alertsQuery.data;

  const alerts = (useAlertsData ?? []).filter((x) => isFiring(x) && x.subject_kind === "check" && x.subject_id === id);
  const level = c.state === "down" ? "crit" : c.state === "up" ? "ok" : "unknown";
  const subject = `check:${id}`;

  return (
    <Tabs defaultValue="overview" className="gap-0">
      <ObjectHeader
        above={
          env && (
            <Link to="/scopes/$id" params={{ id: String(env.id) }} className="font-mono text-mini hover:text-foreground hover:underline">
              {env.path}
            </Link>
          )
        }
        icon={<StatusIcon level={level} size={22} />}
        title={c.name}
        actions={
          <>
            <AlertBell subject={subject} name={c.name} kind={c.type === "heartbeat" ? "heartbeat" : "check"} />
            {isAdmin && <CheckMenu id={id} envID={c.environment_id} />}
          </>
        }
        facts={
          <Facts
            items={[
              <span style={c.state === "down" ? { color: "var(--crit)" } : undefined}>
                {t(`check.state.${c.state}`)}
                {c.state_since && <> {t("check.since", { time: ago(c.state_since) })}</>}
              </span>,
              t(`check.kind.${c.type}`),
              <span className="font-mono text-mini">{target(c)}</span>,
            ]}
          />
        }
      />
      <Page>
        {c.state === "down" && c.error && (
          <p className="rounded-lg border px-4 py-3" style={{ borderColor: "color-mix(in oklch, var(--crit) 35%, transparent)", color: "var(--crit)" }}>
            {c.error}
          </p>
        )}
        {alerts.length > 0 && (
          <section className="overflow-hidden rounded-xl border bg-card">
            <Incidents alerts={alerts} />
          </section>
        )}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <PageTabs
            tabs={[
              { value: "overview", label: t("tabs.overview") },
              { value: "alerts", label: t("tabs.alerts") },
              { value: "settings", label: t("tabs.settings") },
              { value: "activity", label: t("tabs.activity") },
            ]}
          />
          {c.type !== "heartbeat" && <RangePicker value={seconds} onChange={setSeconds} />}
        </div>
        <TabsContent value="overview" className="space-y-4">
          {c.type === "heartbeat" ? (
            <Heartbeat check={c} />
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-3">
                <Stat label={t("check.uptime")} value={percent(uptime, uptime != null && uptime < 99.995 && uptime > 0.005 ? 2 : 0)} />
                <Stat label={t("check.latency")} value={c.latency_ms != null ? `${Math.round(c.latency_ms)} ms` : "–"} />
                {/* A domain check keeps its registration's end where others keep
                    a certificate's. */}
                <Stat
                  label={c.type === "domain" ? t("check.domainExpires") : t("check.cert")}
                  value={days != null ? t("check.days", { count: days }) : "–"}
                  tone={days != null && days < (c.type === "domain" ? 30 : 14) ? "var(--warn)" : undefined}
                />
              </div>
              <Panel title={t("table.last24h")}>
                <UptimeBar checkID={id} slices={96} />
              </Panel>
              <Panel title={t("check.latency")}>
                <Chart data={series.data} lines={lines} format={format} range={range} events={events.data} />
              </Panel>
              <Probes check={c} />
            </>
          )}
        </TabsContent>
        <TabsContent value="alerts" className="space-y-6">
          <AlertHistory subject={subject} always />
          <Maintenance subject={subject} />
        </TabsContent>
        <TabsContent value="settings" className="space-y-6">
          <Config subject={subject} envID={c.environment_id} />
        </TabsContent>
        <TabsContent value="activity" className="space-y-6">
          <Events events={events.data ?? []} />
          <History subject={subject} />
        </TabsContent>
      </Page>
    </Tabs>
  );
}

// Stat is one figure, large, with what it is.
function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="rounded-lg border bg-card px-4 py-3">
      <p className="text-mini text-muted-foreground">{label}</p>
      <p className="text-2xl font-semibold tracking-tight tabular-nums" style={tone ? { color: tone } : undefined}>
        {value}
      </p>
    </div>
  );
}

// CheckMenu holds what is seldom done to a check: deleting it.
function CheckMenu({ id, envID }: { id: number; envID: number }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const del = useMutation({
    mutationFn: () => deleteCheck(id),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["checks"] });
      toast.success(t("check.deleted"));
      navigate({ to: "/scopes/$id", params: { id: String(envID) } });
    },
  });
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("host.more")} />}>
        <MoreHorizontal />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        <DropdownMenuItem variant="destructive" onClick={() => confirm(t("check.deleteConfirm")) && del.mutate()}>
          <Trash2 />
          {t("check.delete")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function Probes({ check }: { check: CheckDetail }) {
  const { t } = useTranslation();
  if (check.probes.length === 0) return null;
  return (
    <Panel title={t("check.byProbe")} flush>
      <ul>
        {check.probes.map((r) => (
          <li key={r.probe} className="flex items-center gap-3 border-b px-4 py-3 last:border-b-0">
            <StatusIcon level={r.ok ? "ok" : "crit"} />
            <span className="font-medium">{r.probe}</span>
            <span className="min-w-0 flex-1 truncate text-muted-foreground">
              {r.status_code != null && <>HTTP {r.status_code}</>}
              {r.error && <span style={{ color: "var(--crit)" }}> {r.error}</span>}
            </span>
            <span className="tabular-nums">{r.latency_ms != null ? `${number(r.latency_ms, 0)} ms` : "–"}</span>
            <span className="w-20 text-right text-mini text-muted-foreground" title={dateTime(r.ts)}>
              {t("ago", { time: ago(r.ts) })}
            </span>
          </li>
        ))}
      </ul>
    </Panel>
  );
}

// Heartbeat shows where to ping and how, from a cron job or a script.
function Heartbeat({ check }: { check: CheckDetail }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState("");
  const url = check.ping_url;
  const copy = (text: string) => {
    navigator.clipboard?.writeText(text);
    setCopied(text);
    setTimeout(() => setCopied(""), 1500);
  };
  return (
    <Panel title={t("check.pingTitle")}>
      <div className="space-y-3">
        <p>{check.last_ping ? t("check.lastPing", { time: ago(check.last_ping) }) : t("check.noPing")}</p>
        {url ? (
          <>
            <p className="text-muted-foreground">{t("check.pingHint")}</p>
            {[`curl -fsS ${url}`, `backup.sh && curl -fsS ${url} || curl -fsS ${url}/fail`].map((cmd) => (
              <div key={cmd} className="flex items-center gap-2">
                <code className="min-w-0 flex-1 truncate rounded-md bg-muted px-2.5 py-2 font-mono text-mini">{cmd}</code>
                <Button variant="ghost" size="icon-sm" aria-label={t("agent.copy")} onClick={() => copy(cmd)}>
                  {copied === cmd ? <CheckIcon className="size-3.5" /> : <Copy className="size-3.5" />}
                </Button>
              </div>
            ))}
          </>
        ) : (
          <p className="text-muted-foreground">{t("check.pingAdminOnly")}</p>
        )}
      </div>
    </Panel>
  );
}
