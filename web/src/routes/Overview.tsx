import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { listChecks, listHosts, listScopes, type HostSummary } from "@/api";
import { useAlerts } from "@/components/Alerts";
import { Incidents } from "@/components/Incidents";
import { StatusIcon } from "@/components/shell/Dot";
import { CheckTable, HostTable } from "@/components/Tables";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useLive } from "@/lib/live";
import { firing } from "@/lib/status";
import { GettingStarted } from "@/components/GettingStarted";
import { useMe } from "@/lib/useMe";

// Home is everything at once: whether all is well, what needs attention,
// every host with its load, every check with its last day. A customer's or
// an environment's own page, from the sidebar, shows just theirs.
export function Overview() {
  const { t } = useTranslation();
  const { isHubAdmin } = useMe();
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts, refetchInterval: 60_000 });
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const checks = useQuery({ queryKey: ["checks", "all"], queryFn: () => listChecks(), refetchInterval: 30_000 });
  const alerts = useAlerts().data ?? [];
  const [filter, setFilter] = useState("");
  const [startHidden, setStartHidden] = useState(() => localStorage.getItem("lotse.started") === "hidden");
  const client = useQueryClient();
  useLive(["hosts"], (m) => {
    if (m.type === "host") {
      client.setQueryData<HostSummary[]>(["hosts"], (list) => list?.map((h) => (h.host.id === m.host_id ? { ...h, latest: m.reading } : h)));
    } else if (m.type === "host_state") {
      client.setQueryData<HostSummary[]>(["hosts"], (list) => list?.map((h) => (h.host.id === m.host_id ? { ...h, host: { ...h.host, online: m.online! } } : h)));
    } else if (m.type !== "alerts") {
      client.invalidateQueries({ queryKey: ["hosts"] });
    }
  });

  if (hosts.isPending || scopes.isPending) {
    return (
      <div className="mx-auto max-w-7xl space-y-4 px-4 py-6 md:px-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-72 w-full" />
      </div>
    );
  }
  if (hosts.isError || scopes.isError) return <p className="p-6 text-destructive">{t("app.unreachable")}</p>;

  const byID = new Map(scopes.data.map((s) => [s.id, s]));
  const text = filter.toLowerCase();
  const where = (envID: number | null | undefined) => (envID ? byID.get(envID)?.path ?? "" : t("overview.unassigned"));
  const shownHosts = hosts.data.filter((h) => `${h.host.name} ${where(h.host.environment_id)}`.toLowerCase().includes(text));
  const shownChecks = (checks.data ?? []).filter((c) => `${c.name} ${c.path}`.toLowerCase().includes(text));
  const problems = firing(alerts);
  const crit = problems.filter((a) => a.state === "crit").length;
  // Before the first host or check, there is no status to show, and Get
  // started says what to do; empty tables beside it would only repeat it.
  const empty = hosts.data.length === 0 && (checks.data?.length ?? 0) === 0;
  // Getting started is setting up the hub, for whoever looks after all of it.
  const guided = isHubAdmin && !startHidden;

  return (
    <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 md:px-6">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="flex items-center gap-2.5 text-xl font-semibold tracking-tight">
          {!empty && <StatusIcon level={crit ? "crit" : problems.length ? "warn" : "ok"} size={20} />}
          {empty
            ? t("overview.welcome")
            : crit
              ? [t("overview.critical", { count: crit }), problems.length > crit && t("overview.warnings", { count: problems.length - crit })].filter(Boolean).join(", ")
              : problems.length
                ? t("overview.warnings", { count: problems.length })
                : t("overview.allGood")}
        </h1>
        <div className="flex-1" />
        {!empty && (
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("rows.filter")} className="w-56 pl-8" />
          </div>
        )}
      </div>

      {guided && (
        <GettingStarted
          scopes={scopes.data}
          hosts={hosts.data}
          checks={checks.data ?? []}
          onHide={() => (localStorage.setItem("lotse.started", "hidden"), setStartHidden(true))}
        />
      )}

      {problems.length > 0 && (
        <section className="overflow-hidden rounded-lg border bg-card">
          <Incidents alerts={problems} />
        </section>
      )}

      {!(empty && guided) && (
        <>
          <section className="overflow-hidden rounded-lg border bg-card">
            <header className="flex items-baseline gap-2 border-b px-4 py-3">
              <h2 className="font-semibold">{t("nav.hosts")}</h2>
              <span className="text-muted-foreground tabular-nums">{shownHosts.length}</span>
            </header>
            {shownHosts.length === 0 ? (
              <p className="px-4 py-10 text-center text-muted-foreground">{filter ? t("rows.noMatch") : t("overview.noHostsYet")}</p>
            ) : (
              <HostTable hosts={shownHosts} alerts={alerts} where={(h) => where(h.host.environment_id)} />
            )}
          </section>

          <section className="overflow-hidden rounded-lg border bg-card">
            <header className="flex items-baseline gap-2 border-b px-4 py-3">
              <h2 className="font-semibold">{t("nav.checks")}</h2>
              <span className="text-muted-foreground tabular-nums">{shownChecks.length}</span>
            </header>
            {shownChecks.length === 0 ? (
              <p className="px-4 py-10 text-center text-muted-foreground">{filter ? t("rows.noMatch") : t("overview.noChecksYet")}</p>
            ) : (
              <CheckTable checks={shownChecks} where={(c) => where(c.environment_id)} />
            )}
          </section>
        </>
      )}
    </div>
  );
}
