import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Pencil, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteScope, getScope, listChecks, listHosts, listScopes, NotFound, renameScope, updateScope, type HostSummary } from "@/api";
import { AddAgent } from "@/components/AddAgent";
import { AddCheck } from "@/components/AddCheck";
import { CheckList } from "@/components/CheckList";
import { Config } from "@/components/Config";
import { History } from "@/components/History";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useMe } from "@/lib/useMe";
import { useUpdates } from "@/lib/useUpdates";
import { HostGrid } from "./Overview";

// Scope is the page of a customer, project or environment: what is in it,
// its settings and its history.
export function Scope({ id }: { id: number }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const scope = useQuery({ queryKey: ["scope", id], queryFn: () => getScope(id) });
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts });
  const checks = useQuery({ queryKey: ["checks", id], queryFn: () => listChecks(id) });

  if (scope.isPending) return <Skeleton className="h-32 w-full" />;
  if (scope.error instanceof NotFound) return <p>{t("tree.notFound")}</p>;
  if (scope.isError) return <p className="text-destructive">{t("app.unreachable")}</p>;

  const all = scopes.data ?? [];
  const byID = new Map(all.map((s) => [s.id, s]));
  const above: typeof all = [];
  for (let p = scope.data.parent_id; p; p = byID.get(p)?.parent_id) {
    const s = byID.get(p);
    if (s) above.unshift(s);
  }
  const children = all.filter((s) => s.parent_id === id);
  const mine = (hosts.data ?? []).filter((h) => h.host.environment_id === id);
  const subject = `${scope.data.kind}:${id}`;
  // Whether an environment lies in this scope.
  const below = (envID?: number | null) => {
    const env = envID ? byID.get(envID) : undefined;
    return !!env && (env.path === scope.data.path || env.path.startsWith(scope.data.path + "/"));
  };

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">
          {t(`config.kinds.${scope.data.kind}`)}
          {above.map((s) => (
            <span key={s.id}>
              {" · "}
              <Link to="/scopes/$id" params={{ id: String(s.id) }} className="hover:underline">
                {s.name}
              </Link>
            </span>
          ))}
        </p>
        <Title id={id} name={scope.data.name} canEdit={isAdmin} />
        <p className="text-xs text-muted-foreground">
          {t("tree.key")} <code>{scope.data.path}</code>
        </p>
      </div>
      {scope.data.kind === "environment" ? (
        <div className="space-y-6">
          <div className="space-y-3">
            {isAdmin && <AddAgent envID={id} />}
            <HostGrid hosts={mine} empty={t("overview.noHostsAdd")} />
          </div>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-medium">{t("check.title")}</h2>
              {isAdmin && <AddCheck envID={id} />}
            </div>
            <CheckList envID={id} />
          </div>
        </div>
      ) : (
        <ul className="space-y-1 text-sm">
          {children.map((c) => (
            <li key={c.id}>
              <Link to="/scopes/$id" params={{ id: String(c.id) }} className="font-medium hover:underline">
                {c.name}
              </Link>{" "}
              <span className="text-muted-foreground">{c.path}</span>
            </li>
          ))}
        </ul>
      )}
      {isAdmin && <UpdateAgents scopeID={id} hosts={(hosts.data ?? []).filter((h) => below(h.host.environment_id))} />}
      <Config subject={subject} />
      <History subject={subject} />
      {isAdmin && mine.length === 0 && children.length === 0 && !(checks.data?.length) && <Delete id={id} />}
    </div>
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
      <h1 className="flex items-center gap-2 text-xl font-semibold">
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
