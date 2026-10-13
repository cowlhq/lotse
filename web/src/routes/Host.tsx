import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getHost, getSeries, listEvents, listScopes, NotFound, setHostEnvironment, type Container, type HostDetail, type Series } from "@/api";
import { History } from "@/components/History";
import { LiveBadge } from "@/components/LiveBadge";
import { Chart, type Line } from "@/components/Chart";
import { RangePicker, type RangeSeconds } from "@/components/Range";
import { Meter, Online } from "@/components/Status";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ago, bytes, duration, number, percent, rate } from "@/lib/format";
import { status, usageColor } from "@/lib/palette";
import { appendReading, useLive } from "@/lib/live";
import { useMe } from "@/lib/useMe";
import { useRange } from "@/lib/useRange";

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

  const charts = useCharts();
  const lastLive = useLiveHost(id);

  if (host.isPending) return <Skeleton className="h-64 w-full" />;
  if (host.error instanceof NotFound) return <p>{t("host.notFound")}</p>;
  if (host.isError) return <p className="text-destructive">{t("app.unreachable")}</p>;

  return (
    <div className="space-y-6">
      <Header detail={host.data} live={lastLive} />
      <div className="flex justify-end">
        <RangePicker value={seconds} onChange={setSeconds} />
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        {charts.map((c) => (
          <Card key={c.title}>
            <CardHeader>
              <CardTitle className="text-sm font-medium">{t(c.title)}</CardTitle>
            </CardHeader>
            <CardContent>
              <Chart data={series.data} lines={c.lines} format={c.format} range={range} events={events.data} max={c.max} binary={c.binary} />
            </CardContent>
          </Card>
        ))}
      </div>
      <Filesystems detail={host.data} />
      <Containers containers={host.data.containers} />
      <History subject={`host:${id}`} />
    </div>
  );
}

function useCharts() {
  const { t } = useTranslation();
  return useMemo(() => {
    const charts: { title: string; lines: Line[]; format: (v: number | null, digits?: number) => string; max?: number; binary?: boolean }[] = [
      {
        title: "metric.cpu",
        lines: [
          { key: "cpu", label: t("metric.busy") },
          { key: "iowait", label: t("metric.iowait") },
          { key: "steal", label: t("metric.steal") },
        ],
        format: (v, d) => percent(v, d ?? 1),
        max: 100,
      },
      {
        title: "metric.memory",
        lines: [
          { key: "mem_used", label: t("metric.used") },
          { key: "swap_used", label: t("metric.swap") },
        ],
        format: (v) => bytes(v),
        binary: true,
      },
      {
        title: "metric.load",
        lines: [
          { key: "load1", label: t("metric.load1") },
          { key: "load5", label: t("metric.load5") },
          { key: "load15", label: t("metric.load15") },
        ],
        format: (v, d) => number(v, d ?? 2),
      },
      {
        title: "metric.network",
        lines: [
          { key: "net_rx", label: t("metric.received") },
          { key: "net_tx", label: t("metric.sent") },
        ],
        format: rate,
        binary: true,
      },
      {
        title: "metric.diskio",
        lines: [
          { key: "disk_read", label: t("metric.read") },
          { key: "disk_write", label: t("metric.written") },
        ],
        format: rate,
        binary: true,
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
      default:
        client.invalidateQueries({ queryKey: ["host", id] });
    }
  });
  return last;
}

function Header({ detail, live }: { detail: HostDetail; live: number }) {
  const { t } = useTranslation();
  const { host, latest } = detail;
  const facts: [string, string][] = [
    [t("host.platform"), host.platform || host.os],
    [t("host.kernel"), host.kernel],
    [t("host.cpu"), host.cpu_model ? `${host.cores} × ${host.cpu_model}` : t("host.cores", { count: host.cores })],
    [t("host.memory"), bytes(host.mem_total)],
    [t("host.uptime"), duration(latest?.uptime)],
    [t("host.agent"), host.agent_version],
    [t("host.docker"), host.docker_version || t("host.noDocker")],
    [t("host.lastSeen"), t("ago", { time: ago(host.last_seen) })],
  ];
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <h2 className="text-xl font-semibold">{host.name}</h2>
        <Online online={host.online} />
        <LiveBadge last={live} />
      </div>
      <EnvironmentPicker hostID={host.id} current={host.environment_id ?? null} />
      <dl className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm sm:grid-cols-4">
        {facts.map(([k, v]) => (
          <div key={k} className="min-w-0">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="truncate">{v}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

function Filesystems({ detail }: { detail: HostDetail }) {
  const { t } = useTranslation();
  if (detail.filesystems.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("host.filesystems")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {detail.filesystems.map((fs) => {
          const pct = fs.used != null && fs.total ? (100 * fs.used) / fs.total : null;
          return (
            <div key={fs.id} className="space-y-1 text-sm">
              <div className="flex justify-between gap-4">
                <span className="truncate">
                  {fs.mount} <span className="text-muted-foreground">{fs.fstype}</span>
                </span>
                <span className="tabular-nums text-muted-foreground">
                  {bytes(fs.used)} / {bytes(fs.total)} · {percent(pct)}
                </span>
              </div>
              <Meter pct={pct} color={usageColor(pct ?? 0)} />
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}

function Containers({ containers }: { containers: Container[] }) {
  const { t } = useTranslation();
  if (containers.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("host.containers")}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("container.name")}</TableHead>
              <TableHead>{t("container.state")}</TableHead>
              <TableHead className="text-right">{t("metric.cpu")}</TableHead>
              <TableHead className="text-right">{t("metric.memory")}</TableHead>
              <TableHead className="text-right">{t("container.restarts")}</TableHead>
              <TableHead>{t("container.image")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {containers.map((c) => (
              <TableRow key={c.id} className={c.gone_at != null ? "opacity-50" : undefined}>
                <TableCell>
                  <Link to="/containers/$id" params={{ id: String(c.id) }} className="font-medium hover:underline">
                    {c.slug}
                  </Link>
                </TableCell>
                <TableCell>
                  <ContainerState container={c} />
                </TableCell>
                {/* The last reading of a stopped container says nothing about now. */}
                <TableCell className="text-right tabular-nums">{c.state === "running" ? percent(c.latest?.cpu, 1) : "–"}</TableCell>
                <TableCell className="text-right tabular-nums">{c.state === "running" ? bytes(c.latest?.mem) : "–"}</TableCell>
                <TableCell className="text-right tabular-nums">{c.restarts}</TableCell>
                <TableCell className="max-w-64 truncate text-muted-foreground">{c.image}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

// ContainerState names the state in words; color only backs it up.
export function ContainerState({ container: c }: { container: Container }) {
  const { t } = useTranslation();
  if (c.gone_at != null) return <Badge variant="outline">{t("container.gone")}</Badge>;
  // Docker keeps the last health of a stopped container; only a running
  // one can be unhealthy.
  const running = c.state === "running";
  const bad = (running && c.health === "unhealthy") || (c.state === "exited" && c.exit_code !== 0) || c.state === "dead";
  const label = c.health && running ? `${c.state}, ${c.health}` : c.state === "exited" ? t("container.exited", { code: c.exit_code }) : c.state;
  return (
    <Badge variant="outline" style={bad ? { color: status.critical, borderColor: status.critical } : undefined}>
      {label}
    </Badge>
  );
}

// EnvironmentPicker shows the host's environment; an admin can move it.
function EnvironmentPicker({ hostID, current }: { hostID: number; current: number | null }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const move = useMutation({
    mutationFn: (env: number | null) => setHostEnvironment(hostID, env),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["host", hostID] });
      client.invalidateQueries({ queryKey: ["hosts"] });
      client.invalidateQueries({ queryKey: ["changes", `host:${hostID}`] });
    },
  });
  const envs = (scopes.data ?? []).filter((s) => s.kind === "environment");
  const here = envs.find((e) => e.id === current);
  if (!isAdmin) {
    return <p className="text-sm text-muted-foreground">{here ? here.path : t("overview.unassigned")}</p>;
  }
  return (
    <label className="flex items-center gap-2 text-sm">
      <span className="text-muted-foreground">{t("tree.environment")}</span>
      <select
        className="h-8 rounded-md border bg-background px-2 text-sm"
        value={current ?? ""}
        onChange={(e) => move.mutate(e.target.value ? Number(e.target.value) : null)}
      >
        <option value="">{t("overview.unassigned")}</option>
        {envs.map((e) => (
          <option key={e.id} value={e.id}>
            {e.path}
          </option>
        ))}
      </select>
    </label>
  );
}
