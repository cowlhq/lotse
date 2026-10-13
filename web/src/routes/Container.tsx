import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getContainer, getSeries, listEvents, NotFound, type Event } from "@/api";
import { Chart, type Line } from "@/components/Chart";
import { RangePicker, type RangeSeconds } from "@/components/Range";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ago, bytes, dateTime, percent, rate } from "@/lib/format";
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

  const charts = useMemo(() => {
    const list: { title: string; lines: Line[]; format: (v: number | null, digits?: number) => string; binary?: boolean }[] = [
      { title: "metric.cpuCore", lines: [{ key: "cpu", label: t("metric.cpu") }], format: (v, d) => percent(v, d ?? 1) },
      { title: "metric.memory", lines: [{ key: "mem", label: t("metric.used") }], format: (v) => bytes(v), binary: true },
      {
        title: "metric.network",
        lines: [
          { key: "net_rx", label: t("metric.received") },
          { key: "net_tx", label: t("metric.sent") },
        ],
        format: rate,
        binary: true,
      },
    ];
    return list;
  }, [t]);

  if (container.isPending) return <Skeleton className="h-64 w-full" />;
  if (container.error instanceof NotFound) return <p>{t("container.notFound")}</p>;
  if (container.isError) return <p className="text-destructive">{t("app.unreachable")}</p>;
  const c = container.data;

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">
          <Link to="/hosts/$id" params={{ id: String(c.host_id) }} className="hover:underline">
            {c.host_name}
          </Link>
        </p>
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold">{c.slug}</h2>
          <ContainerState container={c} />
        </div>
        <p className="text-sm text-muted-foreground">
          {c.image} · {t("container.restartCount", { count: c.restarts })}
          {c.state === "running" && c.started_at > 0 && <> · {t("container.upFor", { time: ago(c.started_at) })}</>}
        </p>
      </div>
      <div className="flex justify-end">
        <RangePicker value={seconds} onChange={setSeconds} />
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        {charts.map((ch) => (
          <Card key={ch.title}>
            <CardHeader>
              <CardTitle className="text-sm font-medium">{t(ch.title)}</CardTitle>
            </CardHeader>
            <CardContent>
              <Chart data={series.data} lines={ch.lines} format={ch.format} range={range} events={events.data} binary={ch.binary} />
            </CardContent>
          </Card>
        ))}
      </div>
      <Events events={events.data ?? []} />
    </div>
  );
}

function Events({ events }: { events: Event[] }) {
  const { t } = useTranslation();
  if (events.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("container.events")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-1 text-sm">
          {[...events].reverse().map((e) => (
            <li key={e.id} className="flex gap-4">
              <span className="tabular-nums text-muted-foreground">{dateTime(e.ts)}</span>
              <span>{t(`event.${e.kind}`, { defaultValue: e.kind, code: e.data.exit_code })}</span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
