import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Ban, FolderInput, MoreHorizontal } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { getConfig, getHost, getSeries, listEvents, listScopes, NotFound, revokeAgent, setHostEnvironment, updateHost, type Container, type Host as HostData, type HostDetail, type Series, type SystemUnit } from "@/api";
import { AlertBell } from "@/components/AlertBell";
import { AlertHistory, useAlerts } from "@/components/Alerts";
import { Chart, type Line } from "@/components/Chart";
import { Config } from "@/components/Config";
import { Events } from "@/components/Events";
import { History } from "@/components/History";
import { Incidents } from "@/components/Incidents";
import { LiveBadge } from "@/components/LiveBadge";
import { Maintenance } from "@/components/Maintenance";
import { RangePicker, type RangeSeconds } from "@/components/Range";
import { StatusIcon } from "@/components/shell/Dot";
import { Page } from "@/components/shell/PageHeader";
import { PageTabs } from "@/components/shell/PageTabs";
import { Facts, ObjectHeader, Panel } from "@/components/shell/Panel";
import { Bar } from "@/components/Tables";
import { UpdateBadge } from "@/components/UpdateBadge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { ago, bytes, duration, number, percent, rate } from "@/lib/format";
import { appendReading, useLive, type LiveMessage } from "@/lib/live";
import { useMe } from "@/lib/useMe";
import { useRange } from "@/lib/useRange";
import { useUpdates } from "@/lib/useUpdates";
import { isFiring } from "@/lib/status";

export function Host({ id }: { id: number }) {
  const { t } = useTranslation();
  const [seconds, setSeconds] = useState<RangeSeconds>(3600);
  const range = useRange(seconds);
  const host = useQuery({ queryKey: ["host", id], queryFn: () => getHost(id), refetchInterval: 15_000 });
  const series = useQuery({
    queryKey: ["series", "hosts", id, range],
    queryFn: () => getSeries("hosts", id, range),
    placeholderData: (prev) => prev,
  });
  const events = useQuery({
    queryKey: ["events", "host", id, range],
    queryFn: () => listEvents(`host:${id}`, range),
    placeholderData: (prev) => prev,
  });
  const alerts = useAlerts().data ?? [];
  const charts = useCharts();
  const { last: lastLive, update } = useLiveHost(id);

  if (host.isPending) return <Skeleton className="m-6 h-64" />;
  if (host.error instanceof NotFound) return <p className="p-6">{t("host.notFound")}</p>;
  if (host.isError) return <p className="p-6 text-destructive">{t("app.unreachable")}</p>;
  const d = host.data;
  const h = d.host;
  const subject = `host:${id}`;
  const problems = alerts.filter((a) => isFiring(a) && ((a.subject_kind === "host" && a.subject_id === id) || d.containers.some((c) => a.subject_kind === "container" && a.subject_id === c.id)));
  const level = h.waiting ? "unknown" : !h.online ? (h.agent_revoked ? "unknown" : "crit") : problems.some((a) => a.state === "crit") ? "crit" : problems.length ? "warn" : "ok";
  const current = (key: string) => (d.latest as Record<string, number | null | undefined> | undefined)?.[key];
  const running = d.containers.filter((c) => c.state === "running" && c.gone_at == null).length;

  return (
    <Tabs defaultValue="overview" className="gap-0">
      <ObjectHeader
        above={<EnvironmentLink envID={h.environment_id ?? null} />}
        icon={<StatusIcon level={level} size={22} />}
        title={h.name}
        actions={
          <>
            <LiveBadge last={lastLive} />
            <AlertBell subject={subject} name={h.name} kind="host" />
            <HostMenu host={h} />
          </>
        }
        facts={
          <Facts
            items={[
              h.waiting ? t("status.waiting") : h.online ? t("status.online") : h.agent_revoked ? t("status.revoked") : t("rows.offlineSince", { time: ago(h.last_seen) }),
              h.platform || h.os,
              h.cpu_model ? `${h.cores} × ${h.cpu_model}` : t("host.cores", { count: h.cores }),
              bytes(h.mem_total),
              d.latest?.uptime != null && t("host.upFor", { time: duration(d.latest.uptime) }),
              `${t("host.agent")} ${h.agent_version}`,
              h.docker_version && `Docker ${h.docker_version}`,
              d.system?.reboot_required && (
                <span style={{ color: "var(--warn)" }} title={d.system.reboot_reason}>
                  {t("host.rebootRequired")}
                </span>
              ),
              // A clock that is off is the machine's to fix; the hub moves
              // its readings meanwhile.
              h.clock_offset != null && (
                <span style={{ color: "var(--warn)" }} title={t("host.clockHint")}>
                  {t(h.clock_offset > 0 ? "host.clockBehind" : "host.clockAhead", { time: duration(Math.abs(h.clock_offset)) })}
                </span>
              ),
            ]}
          />
        }
      />
      <Page>
        <AgentUpdate host={h} status={update} />
        {problems.length > 0 && (
          <section className="overflow-hidden rounded-lg border bg-card">
            <Incidents alerts={problems} />
          </section>
        )}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <PageTabs
            tabs={[
              { value: "overview", label: t("tabs.overview") },
              { value: "containers", label: t("host.containers"), count: d.containers.filter((c) => c.gone_at == null).length },
              { value: "alerts", label: t("tabs.alerts") },
              { value: "settings", label: t("tabs.settings") },
              { value: "activity", label: t("tabs.activity") },
            ]}
          />
          <RangePicker value={seconds} onChange={setSeconds} />
        </div>
        <TabsContent value="overview" className="space-y-4">
          {h.waiting && (
            <Panel title={t("host.waitingTitle")}>
              <p className="text-muted-foreground">{t("host.waitingHint", { name: h.name })}</p>
            </Panel>
          )}
          <div className="grid gap-4 lg:grid-cols-2">
            {charts.map((c) => (
              <Panel key={c.title} title={t(c.title)} right={<span className="text-foreground tabular-nums">{c.now(current)}</span>}>
                <Chart data={series.data} lines={c.lines} format={c.format} range={range} events={events.data} max={c.max} binary={c.binary} />
              </Panel>
            ))}
          </div>
          <Filesystems detail={d} />
          <Services hostID={id} detail={d} />
        </TabsContent>
        <TabsContent value="containers">
          <Panel title={t("host.containers")} right={t("host.running", { count: running })} flush>
            <Containers containers={d.containers} />
          </Panel>
        </TabsContent>
        <TabsContent value="alerts" className="space-y-6">
          <AlertHistory subject={subject} always />
          <Maintenance subject={subject} />
        </TabsContent>
        <TabsContent value="settings" className="space-y-6">
          <Config subject={subject} />
        </TabsContent>
        <TabsContent value="activity" className="space-y-6">
          <Events events={events.data ?? []} />
          <History subject={subject} />
        </TabsContent>
      </Page>
    </Tabs>
  );
}

// EnvironmentLink names where a host is, leading there.
function EnvironmentLink({ envID }: { envID: number | null }) {
  const { t } = useTranslation();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const env = (scopes.data ?? []).find((s) => s.id === envID);
  if (!env) return <span>{t("overview.unassigned")}</span>;
  return (
    <Link to="/scopes/$id" params={{ id: String(env.id) }} className="font-mono text-mini hover:text-foreground hover:underline">
      {env.path}
    </Link>
  );
}

// HostMenu holds what is seldom done to a host: moving it to another
// environment, and cutting its agent off.
function HostMenu({ host }: { host: HostData }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const [moving, setMoving] = useState(false);
  const done = () => {
    client.invalidateQueries({ queryKey: ["host", host.id] });
    client.invalidateQueries({ queryKey: ["hosts"] });
    client.invalidateQueries({ queryKey: ["changes"] });
  };
  const move = useMutation({ mutationFn: (env: number | null) => setHostEnvironment(host.id, env), onSuccess: () => (done(), setMoving(false), toast.success(t("host.moved"))) });
  const revoke = useMutation({ mutationFn: () => revokeAgent(host.id), onSuccess: () => (done(), toast.success(t("agent.revoked"))) });
  if (!isAdmin) return null;
  const envs = (scopes.data ?? []).filter((s) => s.kind === "environment");
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("host.more")} />}>
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuItem onClick={() => setMoving(true)}>
            <FolderInput />
            {t("host.move")}
          </DropdownMenuItem>
          {!host.agent_revoked && (
            <DropdownMenuItem variant="destructive" onClick={() => confirm(t("agent.revokeConfirm")) && revoke.mutate()}>
              <Ban />
              {t("agent.revoke")}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <Dialog open={moving} onOpenChange={setMoving}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("host.move")}</DialogTitle>
            <DialogDescription>{t("host.moveHint")}</DialogDescription>
          </DialogHeader>
          <ul className="max-h-72 overflow-y-auto rounded-md border">
            {envs.map((e) => (
              <li key={e.id}>
                <button
                  type="button"
                  disabled={e.id === host.environment_id || move.isPending}
                  onClick={() => move.mutate(e.id)}
                  className="flex w-full items-center justify-between border-b px-3 py-2.5 text-left last:border-b-0 hover:bg-accent disabled:opacity-50"
                >
                  <span className="font-medium">{e.name}</span>
                  <span className="font-mono text-mini text-muted-foreground">{e.path}</span>
                </button>
              </li>
            ))}
          </ul>
        </DialogContent>
      </Dialog>
    </>
  );
}

function useCharts() {
  const { t } = useTranslation();
  return useMemo(() => {
    type Now = (key: string) => number | null | undefined;
    const charts: { title: string; lines: Line[]; format: (v: number | null, digits?: number) => string; max?: number; binary?: boolean; now: (get: Now) => string }[] = [
      {
        title: "metric.cpu",
        lines: [
          { key: "cpu", label: t("metric.busy") },
          { key: "iowait", label: t("metric.iowait") },
          { key: "steal", label: t("metric.steal") },
        ],
        format: (v, d) => percent(v, d ?? 1),
        max: 100,
        now: (get) => percent(get("cpu"), 1),
      },
      {
        title: "metric.memory",
        lines: [
          { key: "mem_used", label: t("metric.used") },
          { key: "swap_used", label: t("metric.swap") },
        ],
        format: (v) => bytes(v),
        binary: true,
        now: (get) => bytes(get("mem_used")),
      },
      {
        title: "metric.load",
        lines: [
          { key: "load1", label: t("metric.load1") },
          { key: "load5", label: t("metric.load5") },
          { key: "load15", label: t("metric.load15") },
        ],
        format: (v, d) => number(v, d ?? 2),
        now: (get) => number(get("load1"), 2),
      },
      {
        title: "metric.network",
        lines: [
          { key: "net_rx", label: t("metric.received") },
          { key: "net_tx", label: t("metric.sent") },
        ],
        format: rate,
        binary: true,
        now: (get) => `↓ ${rate(get("net_rx"))}  ↑ ${rate(get("net_tx"))}`,
      },
      {
        title: "metric.diskio",
        lines: [
          { key: "disk_read", label: t("metric.read") },
          { key: "disk_write", label: t("metric.written") },
        ],
        format: rate,
        binary: true,
        now: (get) => `${rate(get("disk_read"))} / ${rate(get("disk_write"))}`,
      },
    ];
    return charts;
  }, [t]);
}

// useLiveHost keeps the page current from the live connection: readings
// are added to the graphs and tiles, changes of containers and state make
// the host load again. It returns when the last reading came.
function useLiveHost(id: number) {
  const client = useQueryClient();
  const [last, setLast] = useState(0);
  const [update, setUpdate] = useState<LiveMessage["update"]>();
  useLive([`host:${id}`], (m) => {
    if (m.host_id !== id) return;
    switch (m.type) {
      case "host": {
        const r = m.reading!;
        setLast(Date.now());
        client.setQueriesData<Series>({ queryKey: ["series", "hosts", id] }, (s) => appendReading(s, r.ts, r));
        client.setQueryData<HostDetail>(["host", id], (d) => d && { ...d, latest: r, host: { ...d.host, last_seen: r.ts } });
        break;
      }
      case "container_stats":
        client.setQueryData<HostDetail>(["host", id], (d) => {
          if (!d) return d;
          const byID = new Map(m.containers!.map((c) => [c.container_id, c.reading]));
          return { ...d, containers: d.containers.map((c) => (byID.has(c.id) ? { ...c, latest: byID.get(c.id) } : c)) };
        });
        break;
      case "update":
        setUpdate(m.update);
        client.invalidateQueries({ queryKey: ["events", "host", id] });
        break;
      default:
        client.invalidateQueries({ queryKey: ["host", id] });
        client.invalidateQueries({ queryKey: ["events", "host", id] });
    }
  });
  return { last, update };
}

function Filesystems({ detail }: { detail: HostDetail }) {
  const { t } = useTranslation();
  if (detail.filesystems.length === 0) return null;
  return (
    <Panel title={t("host.filesystems")} flush>
      <ul>
        {detail.filesystems.map((fs) => {
          const pct = fs.used != null && fs.total ? (100 * fs.used) / fs.total : null;
          return (
            <li key={fs.id} className="grid grid-cols-[minmax(0,1fr)_minmax(8rem,16rem)_auto] items-center gap-4 border-b px-4 py-3 last:border-b-0">
              <span className="min-w-0 truncate">
                <span className="font-mono">{fs.mount}</span> <span className="text-mini text-muted-foreground">{fs.device} · {fs.fstype}</span>
              </span>
              <Bar pct={pct} />
              <span className="text-right text-mini whitespace-nowrap tabular-nums text-muted-foreground">
                {bytes(fs.used)} / {bytes(fs.total)}
              </span>
            </li>
          );
        })}
      </ul>
    </Panel>
  );
}

// Services are the machine's systemd units as its agent sees them: those
// that failed and those that must run first, the rest on request.
function Services({ hostID, detail }: { hostID: number; detail: HostDetail }) {
  const { t } = useTranslation();
  const [all, setAll] = useState(false);
  const config = useQuery({ queryKey: ["config", `host:${hostID}`], queryFn: () => getConfig(`host:${hostID}`) });
  const sys = detail.system;
  if (!sys?.systemd) return null;
  const setting = (key: string) => new Set((config.data?.find((v) => v.key === key)?.value as string[] | undefined) ?? []);
  const must = setting("host.services");
  const ignored = setting("host.ignore_units");
  const services = sys.units.filter((u) => u.name.endsWith(".service"));
  const active = services.filter((u) => u.active === "active").length;
  const failed = sys.units.filter((u) => u.active === "failed").length;
  const bad = (u: SystemUnit) => (u.active === "failed" && !ignored.has(u.name)) || (must.has(u.name) && u.active !== "active");
  const missing = [...must].filter((name) => !sys.units.some((u) => u.name === name)).map((name): SystemUnit => ({ name, active: "missing", sub: "" }));
  const shown = all ? [...missing, ...sys.units] : [...missing, ...sys.units.filter((u) => bad(u) || must.has(u.name) || u.active === "failed")];
  shown.sort((a, b) => Number(!bad(a) && a.active !== "missing") - Number(!bad(b) && b.active !== "missing") || a.name.localeCompare(b.name));
  return (
    <Panel
      title={t("host.services")}
      right={
        <>
          <span className="tabular-nums">{failed ? t("host.servicesFailed", { active, failed }) : t("host.servicesActive", { active })}</span>
          <Button variant="ghost" size="xs" onClick={() => setAll(!all)}>
            {all ? t("host.servicesFew") : t("host.servicesAll")}
          </Button>
        </>
      }
      flush
    >
      {shown.length === 0 ? (
        <p className="px-4 py-3 text-muted-foreground">{t("host.servicesFine")}</p>
      ) : (
        <ul className={all ? "max-h-96 overflow-y-auto" : undefined}>
          {shown.map((u) => (
            <li key={u.name} className="flex items-center gap-3 border-b px-4 py-2 last:border-b-0">
              <StatusIcon level={bad(u) || u.active === "missing" ? (must.has(u.name) ? "crit" : "warn") : u.active === "active" ? "ok" : "unknown"} />
              <span className="min-w-0 flex-1 truncate font-mono text-mini">{u.name}</span>
              {must.has(u.name) && <span className="text-mini text-muted-foreground">{t("host.mustRun")}</span>}
              {ignored.has(u.name) && <span className="text-mini text-muted-foreground">{t("host.ignored")}</span>}
              <span className="w-36 text-right text-mini text-muted-foreground">
                {u.active === "missing" ? t("host.unitMissing") : u.sub ? `${u.active} (${u.sub})` : u.active}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function Containers({ containers }: { containers: Container[] }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  if (containers.length === 0) return <p className="px-4 py-10 text-center text-muted-foreground">{t("host.noContainers")}</p>;
  const shown = [...containers].sort((a, b) => Number(a.gone_at != null) - Number(b.gone_at != null) || a.slug.localeCompare(b.slug));
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left">
        <thead className="border-b text-muted-foreground">
          <tr>
            <th className="h-10 pl-4 font-medium">{t("container.name")}</th>
            <th className="h-10 px-3 font-medium">{t("container.state")}</th>
            <th className="h-10 w-[16%] px-3 font-medium">{t("metric.cpu")}</th>
            <th className="h-10 w-[12%] px-3 font-medium">{t("metric.memory")}</th>
            <th className="hidden h-10 px-3 font-medium md:table-cell">{t("container.restarts")}</th>
            <th className="hidden h-10 pr-4 font-medium lg:table-cell">{t("container.image")}</th>
          </tr>
        </thead>
        <tbody>
          {shown.map((c) => {
            const running = c.state === "running" && c.gone_at == null;
            return (
              <tr
                key={c.id}
                onClick={() => navigate({ to: "/containers/$id", params: { id: String(c.id) } })}
                className={`cursor-pointer border-b transition-colors last:border-b-0 hover:bg-accent/60 ${c.gone_at != null ? "opacity-50" : ""}`}
              >
                <td className="py-3 pl-4 font-medium">
                  {c.slug}
                  {c.ignored && <span className="ml-2 text-mini font-normal text-muted-foreground">{t("container.ignored")}</span>}
                </td>
                <td className="px-3">
                  <ContainerState container={c} />
                </td>
                {/* The last reading of a stopped container says nothing about now. */}
                <td className="px-3">{running ? <Bar pct={c.latest?.cpu} /> : <span className="text-muted-foreground">–</span>}</td>
                <td className="px-3 tabular-nums">{running ? bytes(c.latest?.mem) : <span className="text-muted-foreground">–</span>}</td>
                <td className="hidden px-3 tabular-nums md:table-cell">{c.restarts}</td>
                <td className="hidden max-w-72 truncate pr-4 font-mono text-mini text-muted-foreground lg:table-cell">{c.image}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// ContainerState names the state in words; color only backs it up.
export function ContainerState({ container: c }: { container: Container }) {
  const { t } = useTranslation();
  if (c.gone_at != null) return <span className="text-muted-foreground">{t("container.gone")}</span>;
  // Docker keeps the last health of a stopped container; only a running
  // one can be unhealthy.
  const running = c.state === "running";
  const bad = (running && c.health === "unhealthy") || (c.state === "exited" && c.exit_code !== 0) || c.state === "dead";
  const label = c.health && running ? `${c.state}, ${c.health}` : c.state === "exited" ? t("container.exited", { code: c.exit_code }) : c.state;
  return (
    <span className="inline-flex items-center gap-2 whitespace-nowrap">
      <StatusIcon level={bad ? "crit" : running ? "ok" : "unknown"} size={12} />
      <span style={bad ? { color: "var(--crit)" } : undefined}>{label}</span>
    </span>
  );
}

// AgentUpdate offers the newest release to the host's agent, and shows how
// the update goes as the agent reports it.
function AgentUpdate({ host, status }: { host: HostData; status?: LiveMessage["update"] }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const { updateFor } = useUpdates();
  const ask = useMutation({ mutationFn: () => updateHost(host.id) });
  const latest = updateFor(host.agent_version);
  if (status) {
    return (
      <p className="text-sm">
        {status.state === "failed" ? (
          <span style={{ color: "var(--crit)" }}>{t("update.failed", { version: status.version, error: status.error })}</span>
        ) : (
          t(`update.${status.state}`, { version: status.version })
        )}
      </p>
    );
  }
  if (!latest) return null;
  return (
    <div className="flex items-center gap-3">
      <UpdateBadge version={latest} />
      {isAdmin && host.online && (
        <Button size="sm" variant="outline" disabled={ask.isPending || ask.isSuccess} onClick={() => ask.mutate()}>
          {t("update.agent", { version: latest })}
        </Button>
      )}
      {ask.isError && <span className="text-sm text-destructive">{ask.error.message}</span>}
    </div>
  );
}
