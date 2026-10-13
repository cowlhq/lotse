import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Pencil, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteScope, getScope, listHosts, listScopes, NotFound, renameScope } from "@/api";
import { History } from "@/components/History";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useMe } from "@/lib/useMe";
import { HostGrid } from "./Overview";

export function Environment({ id }: { id: number }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const scope = useQuery({ queryKey: ["scope", id], queryFn: () => getScope(id) });
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts });

  if (scope.isPending) return <Skeleton className="h-32 w-full" />;
  if (scope.error instanceof NotFound) return <p>{t("tree.notFound")}</p>;
  if (scope.isError) return <p className="text-destructive">{t("app.unreachable")}</p>;

  const byID = new Map((scopes.data ?? []).map((s) => [s.id, s]));
  const project = scope.data.parent_id ? byID.get(scope.data.parent_id) : undefined;
  const customer = project?.parent_id ? byID.get(project.parent_id) : undefined;
  const mine = (hosts.data ?? []).filter((h) => h.host.environment_id === id);

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">
          {customer?.name} / {project?.name}
        </p>
        <Title id={id} name={scope.data.name} canEdit={isAdmin} />
        <p className="text-xs text-muted-foreground">
          {t("tree.key")} <code>{scope.data.path}</code>
        </p>
      </div>
      <HostGrid hosts={mine} empty={t("overview.noHosts")} />
      <History subject={`environment:${id}`} />
      {isAdmin && mine.length === 0 && <Delete id={id} />}
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
