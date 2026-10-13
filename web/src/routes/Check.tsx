import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Check as CheckIcon, Copy, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteCheck, getCheck, getSeries, listEvents, listScopes, NotFound, type CheckDetail } from "@/api";
import { Chart, type Line } from "@/components/Chart";
import { certDays, CheckState, target } from "@/components/CheckState";
import { Config } from "@/components/Config";
import { Events } from "@/components/Events";
import { History } from "@/components/History";
import { RangePicker, type RangeSeconds } from "@/components/Range";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ago, dateTime, number, percent } from "@/lib/format";
import { status } from "@/lib/palette";
import { useMe } from "@/lib/useMe";
import { useRange } from "@/lib/useRange";

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
  const lines: Line[] = useMemo(() => [{ key: "latency_ms", label: t("check.latency") }], [t]);
  const format = useMemo(() => (v: number | null, d?: number) => (v == null ? "–" : `${number(v, d ?? 0)} ms`), []);

  if (check.isPending) return <Skeleton className="h-64 w-full" />;
  if (check.error instanceof NotFound) return <p>{t("check.notFound")}</p>;
  if (check.isError) return <p className="text-destructive">{t("app.unreachable")}</p>;
  const c = check.data;
  const env = scopes.data?.find((s) => s.id === c.environment_id);
  // The share of runs that passed, over the range shown.
  const ok = series.data?.columns.ok?.filter((v): v is number => v != null) ?? [];
  const uptime = ok.length ? (100 * ok.reduce((a, b) => a + b, 0)) / ok.length : undefined;
  const days = certDays(c);

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">
          {env && (
            <Link to="/scopes/$id" params={{ id: String(env.id) }} className="hover:underline">
              {env.path}
            </Link>
          )}{" "}
          · {t(`check.kind.${c.type}`)}
        </p>
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold">{c.name}</h2>
          <CheckState state={c.state} />
          {c.state_since && <span className="text-sm text-muted-foreground">{t("check.since", { time: ago(c.state_since) })}</span>}
        </div>
        <p className="text-sm text-muted-foreground">{target(c)}</p>
        {c.state === "down" && c.error && <p className="text-sm" style={{ color: status.critical }}>{c.error}</p>}
      </div>

      {c.type === "heartbeat" ? (
        <Heartbeat check={c} />
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex gap-6 text-sm">
              <span>
                {t("check.uptime")} <span className="font-medium tabular-nums">{percent(uptime, 2)}</span>
              </span>
              {days != null && (
                <span style={days < 14 ? { color: status.critical } : undefined}>
                  {t("check.certExpires", { count: days })}
                </span>
              )}
            </div>
            <RangePicker value={seconds} onChange={setSeconds} />
          </div>
          <Card>
            <CardHeader>
              <CardTitle className="text-sm font-medium">{t("check.latency")}</CardTitle>
            </CardHeader>
            <CardContent>
              <Chart data={series.data} lines={lines} format={format} range={range} events={events.data} />
            </CardContent>
          </Card>
          <Probes check={c} />
        </>
      )}
      <Events events={events.data ?? []} />
      <Config subject={`check:${id}`} />
      <History subject={`check:${id}`} />
      {isAdmin && <Delete id={id} envID={c.environment_id} />}
    </div>
  );
}

function Probes({ check }: { check: CheckDetail }) {
  const { t } = useTranslation();
  if (check.probes.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("check.byProbe")}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("check.probe")}</TableHead>
              <TableHead>{t("check.stateTitle")}</TableHead>
              <TableHead className="text-right">{t("check.latency")}</TableHead>
              <TableHead>{t("check.when")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {check.probes.map((r) => (
              <TableRow key={r.probe}>
                <TableCell>{r.probe}</TableCell>
                <TableCell>
                  <CheckState state={r.ok ? "up" : "down"} />
                  {r.error && <span className="ml-2 text-xs text-muted-foreground">{r.error}</span>}
                  {r.status_code != null && <span className="ml-2 text-xs text-muted-foreground">HTTP {r.status_code}</span>}
                </TableCell>
                <TableCell className="text-right tabular-nums">{r.latency_ms != null ? `${number(r.latency_ms, 0)} ms` : "–"}</TableCell>
                <TableCell className="text-muted-foreground">{dateTime(r.ts)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
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
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("check.pingTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <p>{check.last_ping ? t("check.lastPing", { time: ago(check.last_ping) }) : t("check.noPing")}</p>
        {url ? (
          <>
            <p className="text-muted-foreground">{t("check.pingHint")}</p>
            {[`curl -fsS ${url}`, `backup.sh && curl -fsS ${url} || curl -fsS ${url}/fail`].map((cmd) => (
              <div key={cmd} className="flex items-center gap-2">
                <code className="min-w-0 flex-1 truncate rounded-md bg-muted px-2 py-1.5 font-mono text-xs">{cmd}</code>
                <Button variant="ghost" size="icon-sm" aria-label={t("agent.copy")} onClick={() => copy(cmd)}>
                  {copied === cmd ? <CheckIcon className="size-3.5" /> : <Copy className="size-3.5" />}
                </Button>
              </div>
            ))}
          </>
        ) : (
          <p className="text-muted-foreground">{t("check.pingAdminOnly")}</p>
        )}
      </CardContent>
    </Card>
  );
}

function Delete({ id, envID }: { id: number; envID: number }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const del = useMutation({
    mutationFn: () => deleteCheck(id),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["checks"] });
      navigate({ to: "/scopes/$id", params: { id: String(envID) } });
    },
  });
  return (
    <Button variant="outline" size="sm" disabled={del.isPending} onClick={() => confirm(t("check.deleteConfirm")) && del.mutate()}>
      <Trash2 className="size-4" />
      {t("check.delete")}
    </Button>
  );
}
