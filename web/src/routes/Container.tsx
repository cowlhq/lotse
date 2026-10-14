import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getContainer, getSeries, listEvents, NotFound, type Container as ContainerData, type Series } from "@/api";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { ago, bytes, percent, rate } from "@/lib/format";
import { appendReading, useLive } from "@/lib/live";
import { useRange } from "@/lib/useRange";
import { ContainerState } from "./Host";

export function Container({ id }: { id: number }) {
  const { t } = useTranslation();
  const [seconds, setSeconds] = useState<RangeSeconds>(3600);
  const range = useRange(seconds);
  const container = useQuery({ queryKey: ["container", id], queryFn: () => getContainer(id), refetchInterval: 15_000 });
  const series = useQuery({
    queryKey: ["series", "containers", id, range],
    queryFn: () => getSeries("containers", id, range),
    placeholderData: (prev) => prev,
  });
  const events = useQuery({
    queryKey: ["events", "container", id, range],
    queryFn: () => listEvents(`container:${id}`, range),
    placeholderData: (prev) => prev,
  });

  const alerts = useAlerts();
  const client = useQueryClient();
  const [lastLive, setLastLive] = useState(0);
  const hostID = container.data?.host_id;
  useLive(hostID ? [`host:${hostID}`] : [], (m) => {
    if (m.host_id !== hostID) return;
    if (m.type === "container_stats") {
      const r = m.containers!.find((c) => c.container_id === id)?.reading;
      if (!r) return;
      setLastLive(Date.now());
      client.setQueriesData<Series>({ queryKey: ["series", "containers", id] }, (s) => appendReading(s, r.ts, r));
      client.setQueryData<ContainerData>(["container", id], (c) => c && { ...c, latest: r });
    } else if (m.type === "containers") {
      client.invalidateQueries({ queryKey: ["container", id] });
      client.invalidateQueries({ queryKey: ["events", "container", id] });
    }
  });

  const charts = useMemo(() => {
    type Now = Record<string, number | null | undefined> | undefined;
    const list: { title: string; lines: Line[]; format: (v: number | null, digits?: number) => string; binary?: boolean; now: (r: Now) => string }[] = [
      { title: "metric.cpuCore", lines: [{ key: "cpu", label: t("metric.cpu") }], format: (v, d) => percent(v, d ?? 1), now: (r) => percent(r?.cpu, 1) },
      { title: "metric.memory", lines: [{ key: "mem", label: t("metric.used") }], format: (v) => bytes(v), binary: true, now: (r) => bytes(r?.mem) },
      {
        title: "metric.network",
        lines: [
          { key: "net_rx", label: t("metric.received") },
          { key: "net_tx", label: t("metric.sent") },
        ],
        format: rate,
        binary: true,
        now: (r) => `↓ ${rate(r?.net_rx)}  ↑ ${rate(r?.net_tx)}`,
      },
    ];
    return list;
  }, [t]);

  if (container.isPending) return <Skeleton className="m-6 h-64" />;
  if (container.error instanceof NotFound) return <p className="p-6">{t("container.notFound")}</p>;
  if (container.isError) return <p className="p-6 text-destructive">{t("app.unreachable")}</p>;
  const c = container.data;

  const subject = `container:${id}`;
  const problems = (alerts.data ?? []).filter((a) => a.state !== "ok" && a.subject_kind === "container" && a.subject_id === id);
  const bad = (c.state === "running" && c.health === "unhealthy") || (c.state === "exited" && c.exit_code !== 0) || c.state === "dead";
  const level = c.gone_at != null ? "unknown" : problems.some((a) => a.state === "crit") || bad ? "crit" : problems.length ? "warn" : c.state === "running" ? "ok" : "unknown";
  const current = c.latest as Record<string, number | null | undefined> | undefined;

  return (
    <Tabs defaultValue="overview" className="gap-0">
      <ObjectHeader
        above={
          <Link to="/hosts/$id" params={{ id: String(c.host_id) }} className="hover:text-foreground hover:underline">
            {c.host_name}
          </Link>
        }
        icon={<StatusIcon level={level} size={22} />}
        title={c.slug}
        actions={<LiveBadge last={lastLive} />}
        facts={
          <Facts
            items={[
              <ContainerState container={c} />,
              c.ignored && t("container.ignored"),
              c.state === "running" && c.started_at > 0 && t("container.upFor", { time: ago(c.started_at) }),
              t("container.restartCount", { count: c.restarts }),
              <span className="font-mono text-mini">{c.image}</span>,
            ]}
          />
        }
      />
      <Page>
        {problems.length > 0 && (
          <section className="overflow-hidden rounded-lg border bg-card">
            <Incidents alerts={problems} />
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
          <RangePicker value={seconds} onChange={setSeconds} />
        </div>
        <TabsContent value="overview">
          <div className="grid gap-4 lg:grid-cols-2">
            {charts.map((ch) => (
              <Panel key={ch.title} title={t(ch.title)} right={<span className="text-foreground tabular-nums">{c.state === "running" ? ch.now(current) : "–"}</span>}>
                <Chart data={series.data} lines={ch.lines} format={ch.format} range={range} events={events.data} binary={ch.binary} />
              </Panel>
            ))}
          </div>
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
