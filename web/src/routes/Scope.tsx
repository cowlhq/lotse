import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Download, Pencil, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteScope, exportURL, getScope, listChecks, listHosts, listScopes, NotFound, renameScope, updateScope, type HostSummary } from "@/api";
import { AlertHistory, useAlerts } from "@/components/Alerts";
import { Incidents } from "@/components/Incidents";
import { CheckTable, HostTable } from "@/components/Tables";
import { StatusIcon } from "@/components/shell/Dot";
import { Empty, Page } from "@/components/shell/PageHeader";
import { Facts, ObjectHeader, Panel } from "@/components/shell/Panel";
import { PageTabs } from "@/components/shell/PageTabs";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { levelUnder, isFiring } from "@/lib/status";
import { Channels } from "@/components/Channels";
import { Maintenance } from "@/components/Maintenance";
import { AddAgent } from "@/components/AddAgent";
import { AddCheck } from "@/components/AddCheck";
import { Config } from "@/components/Config";
import { History } from "@/components/History";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useMe } from "@/lib/useMe";
import { useUpdates } from "@/lib/useUpdates";

// Scope is the page of a customer, project or environment: how it stands
// and what is in it first; its alerts, notifications, maintenance, settings
// and history in tabs.
export function Scope({ id }: { id: number }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const scope = useQuery({ queryKey: ["scope", id], queryFn: () => getScope(id) });
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts });
  const checks = useQuery({ queryKey: ["checks", "all"], queryFn: () => listChecks() });
  const alerts = useAlerts().data ?? [];

  if (scope.isPending) return <Skeleton className="m-6 h-32" />;
  if (scope.error instanceof NotFound) return <p className="p-6">{t("tree.notFound")}</p>;
  if (scope.isError) return <p className="p-6 text-destructive">{t("app.unreachable")}</p>;

  const all = scopes.data ?? [];
  const byID = new Map(all.map((s) => [s.id, s]));
  const above: typeof all = [];
  for (let p = scope.data.parent_id; p; p = byID.get(p)?.parent_id) {
    const s = byID.get(p);
    if (s) above.unshift(s);
  }
  // Whether an environment lies in this scope.
  const below = (envID?: number | null) => {
    const e = envID ? byID.get(envID) : undefined;
    return !!e && (e.path === scope.data.path || e.path.startsWith(scope.data.path + "/"));
  };
  const children = all.filter((s) => s.parent_id === id);
  const myChecks = (checks.data ?? []).filter((c) => below(c.environment_id));
  // Below an environment, rows are grouped by the environment they are in,
  // named from here: Shop / Prod on ACME's page.
  const groupOf = (envID: number | null | undefined) => {
    const e = envID ? byID.get(envID) : undefined;
    const names: string[] = [];
    for (let x = e; x && x.id !== id; x = x.parent_id ? byID.get(x.parent_id) : undefined) names.unshift(x.name);
    return {
      key: e?.path ?? "",
      label: e ? (
        <Link to="/scopes/$id" params={{ id: String(e.id) }} className="hover:underline" onClick={(ev) => ev.stopPropagation()}>
          {names.join(" / ")}
        </Link>
      ) : (
        ""
      ),
    };
  };
  const mine = (hosts.data ?? []).filter((h) => below(h.host.environment_id));
  // Where a check is, below this scope: nothing for one of the environment's
  // own, the host and container for one a container's labels made, whose
  // key goes on through them (web-01/shop-app/web).
  const within = (path: string) => {
    const parts = path.slice(scope.data.path.length + 1).split("/");
    return env ? parts.slice(0, -1).join("/") : "";
  };
  const subject = `${scope.data.kind}:${id}`;
  const env = scope.data.kind === "environment";
  const problems = alerts.filter((a) => isFiring(a) && (a.subject_path === scope.data.path || a.subject_path.startsWith(scope.data.path + "/")));

  return (
    <Tabs defaultValue="overview" className="gap-0">
      <ObjectHeader
        above={
          above.length > 0 && (
            <span className="flex items-center gap-1.5">
              {above.map((x, i) => (
                <span key={x.id} className="flex items-center gap-1.5">
                  {i > 0 && <span className="text-muted-foreground/50">/</span>}
                  <Link to="/scopes/$id" params={{ id: String(x.id) }} className="hover:text-foreground hover:underline">
                    {x.name}
                  </Link>
                </span>
              ))}
            </span>
          )
        }
        icon={mine.length + myChecks.length > 0 ? <StatusIcon level={levelUnder(alerts, scope.data.path)} size={22} /> : null}
        title={scope.data.name}
        actions={
          isAdmin &&
          env && (
            <>
              <AddCheck envID={id} />
              <AddAgent envID={id} />
            </>
          )
        }
        facts={
          <Facts
            items={[
              t(`config.kinds.${scope.data.kind}`),
              t("overview.hostCount", { count: mine.length }),
              t("overview.checkCount", { count: myChecks.length }),
              !env && children.length > 0 && t(`tree.childCount.${scope.data.kind}`, { count: children.length }),
            ]}
          />
        }
      />
      <Page>
        <PageTabs
          tabs={[
            { value: "overview", label: t("tabs.overview") },
            { value: "alerts", label: t("tabs.alerts"), count: problems.length },
            { value: "notifications", label: t("tabs.notifications") },
            { value: "maintenance", label: t("tabs.maintenance") },
            { value: "settings", label: t("tabs.settings") },
            { value: "activity", label: t("tabs.activity") },
          ]}
        />
      <TabsContent value="overview" className="space-y-6">
          {problems.length > 0 && (
            <section className="overflow-hidden rounded-lg border bg-card">
              <Incidents alerts={problems} />
            </section>
          )}
          <Panel title={t("nav.hosts")} count={mine.length} flush>
            {mine.length === 0 ? (
              <Empty>{env ? t("overview.noHostsAdd") : t("overview.noHostsYet")}</Empty>
            ) : (
              <HostTable hosts={mine} alerts={alerts} where={(h) => h.host.platform || h.host.os} group={env ? undefined : (h) => groupOf(h.host.environment_id)} />
            )}
          </Panel>
          <Panel title={t("nav.checks")} count={myChecks.length} flush>
            {myChecks.length === 0 ? (
              <Empty>{env ? t("check.none") : t("overview.noChecksYet")}</Empty>
            ) : (
              <CheckTable checks={myChecks} where={(c) => within(c.path)} group={env ? undefined : (c) => groupOf(c.environment_id)} />
            )}
          </Panel>
          {isAdmin && <UpdateAgents scopeID={id} hosts={mine} />}
      </TabsContent>
      <TabsContent value="alerts">
          <AlertHistory subject={subject} always />
      </TabsContent>
      <TabsContent value="notifications">
          <Channels scopeID={id} subject={subject} name={scope.data.name} />
      </TabsContent>
      <TabsContent value="maintenance">
          <Maintenance subject={subject} />
      </TabsContent>
      <TabsContent value="settings" className="space-y-6">
          <div className="space-y-1">
            <Title id={id} name={scope.data.name} canEdit={isAdmin} />
            <p className="text-mini text-muted-foreground">
              {t(`config.kinds.${scope.data.kind}`)} · {t("tree.key")} <code>{scope.data.path}</code>
            </p>
          </div>
          <Config subject={subject} />
          {isAdmin && (
            <Button variant="outline" size="sm" render={<a href={exportURL(scope.data.path)} download={`${scope.data.slug}.lotse.yaml`} />}>
              <Download />
              {t("token.download")}
            </Button>
          )}
          {isAdmin && mine.length === 0 && children.length === 0 && myChecks.length === 0 && <Delete id={id} />}
      </TabsContent>
      <TabsContent value="activity">
          <History subject={subject} />
      </TabsContent>
      </Page>
    </Tabs>
  );
}

function Title({ id, name, canEdit }: { id: number; name: string; canEdit: boolean }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(name);
  const save = useMutation({
    mutationFn: () => renameScope(id, value),
    onSuccess: () => {
      setEditing(false);
      client.invalidateQueries({ queryKey: ["scope", id] });
      client.invalidateQueries({ queryKey: ["scopes"] });
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });
  if (!editing) {
    return (
      <h1 className="flex items-center gap-2 text-lg font-semibold tracking-tight">
        {name}
        {canEdit && (
          <Button variant="ghost" size="icon-sm" aria-label={t("tree.rename")} onClick={() => (setValue(name), setEditing(true))}>
            <Pencil className="size-3.5" />
          </Button>
        )}
      </h1>
    );
  }
  return (
    <form
      className="flex max-w-md gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <Input value={value} onChange={(e) => setValue(e.target.value)} autoFocus />
      <Button type="submit" size="sm" disabled={save.isPending}>
        {t("tree.save")}
      </Button>
      <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(false)}>
        {t("tree.cancel")}
      </Button>
    </form>
  );
}

function Delete({ id }: { id: number }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const del = useMutation({
    mutationFn: () => deleteScope(id),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["scopes"] });
      navigate({ to: "/" });
    },
  });
  return (
    <div className="space-y-2">
      <Button variant="outline" size="sm" onClick={() => confirm(t("tree.deleteConfirm")) && del.mutate()}>
        <Trash2 className="size-4" />
        {t("tree.delete")}
      </Button>
      {del.isError && (
        <Alert variant="destructive">
          <AlertDescription>{del.error.message}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}

// UpdateAgents asks every agent in the scope that is behind to update.
export function UpdateAgents({ scopeID, hosts }: { scopeID: number; hosts: HostSummary[] }) {
  const { t } = useTranslation();
  const { updateFor, latest } = useUpdates();
  const behind = hosts.filter((h) => h.host.online && updateFor(h.host.agent_version));
  const ask = useMutation({ mutationFn: () => updateScope(scopeID) });
  if (!latest || behind.length === 0) return null;
  return (
    <div className="flex items-center gap-3 text-sm">
      <Button size="sm" variant="outline" disabled={ask.isPending || ask.isSuccess} onClick={() => ask.mutate()}>
        {t("update.agents", { count: behind.length, version: latest })}
      </Button>
      {ask.isSuccess && <span className="text-muted-foreground">{t("update.asked", { count: ask.data.asked })}</span>}
      {ask.isError && <span className="text-destructive">{ask.error.message}</span>}
    </div>
  );
}
