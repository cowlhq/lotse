import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { listHosts, type HostSummary } from "@/api";
import { useLive } from "@/lib/live";
import { Meter, Online } from "@/components/Status";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { number, percent } from "@/lib/format";
import { series, status, usageColor } from "@/lib/palette";

export function Overview() {
  const { t } = useTranslation();
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts, refetchInterval: 60_000 });
  const client = useQueryClient();
  useLive(["hosts"], (m) => {
    if (m.type === "host") {
      client.setQueryData<HostSummary[]>(["hosts"], (list) =>
        list?.map((h) => (h.host.id === m.host_id ? { ...h, latest: m.reading } : h)),
      );
    } else if (m.type === "host_state") {
      client.setQueryData<HostSummary[]>(["hosts"], (list) =>
        list?.map((h) => (h.host.id === m.host_id ? { ...h, host: { ...h.host, online: m.online! } } : h)),
      );
    } else {
      // Containers and new hosts: counted by the hub.
      client.invalidateQueries({ queryKey: ["hosts"] });
    }
  });

  if (hosts.isPending) return <Skeleton className="h-32 w-full" />;
  if (hosts.isError) return <p className="text-destructive">{t("app.unreachable")}</p>;
  if (hosts.data.length === 0) return <p className="text-muted-foreground">{t("overview.empty")}</p>;

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {hosts.data.map((h) => (
        <HostCard key={h.host.id} summary={h} />
      ))}
    </div>
  );
}

function HostCard({ summary }: { summary: HostSummary }) {
  const { t } = useTranslation();
  const { host, latest, disk, containers } = summary;
  const mem = latest?.mem_used != null && host.mem_total > 0 ? (100 * latest.mem_used) / host.mem_total : null;
  return (
    <Link to="/hosts/$id" params={{ id: String(host.id) }} className="block">
      <Card className="transition-colors hover:bg-muted/40">
        <CardHeader>
          <CardTitle className="flex items-center justify-between gap-2">
            <span className="truncate">{host.name}</span>
            <Online online={host.online} />
          </CardTitle>
          <p className="truncate text-xs text-muted-foreground">{host.platform || host.os}</p>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <Usage label={t("metric.cpu")} pct={latest?.cpu} color={series[0]} />
          <Usage label={t("metric.memory")} pct={mem} color={series[0]} />
          <Usage label={t("metric.disk")} pct={disk} color={usageColor(disk ?? 0)} />
          <div className="flex justify-between gap-4 text-muted-foreground">
            <span className="whitespace-nowrap">
              {t("metric.load")} <span className="tabular-nums text-foreground">{number(latest?.load1)}</span>
            </span>
            <span className="whitespace-nowrap">
              {t("overview.containers", { running: containers.running, total: containers.total })}
            </span>
          </div>
          {containers.unhealthy > 0 && (
            <p className="flex items-center gap-1.5 font-medium" style={{ color: status.critical }}>
              <TriangleAlert className="size-4" />
              {t("overview.unhealthy", { count: containers.unhealthy })}
            </p>
          )}
        </CardContent>
      </Card>
    </Link>
  );
}

function Usage({ label, pct, color }: { label: string; pct: number | null | undefined; color: string }) {
  return (
    <div className="space-y-1">
      <div className="flex justify-between">
        <span className="text-muted-foreground">{label}</span>
        <span className="tabular-nums">{percent(pct)}</span>
      </div>
      <Meter pct={pct} color={color} />
    </div>
  );
}
