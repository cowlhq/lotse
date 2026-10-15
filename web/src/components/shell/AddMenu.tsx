import { useQuery } from "@tanstack/react-query";
import { Activity, FolderPlus, Plus, Server } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { listScopes, type Scope } from "@/api";
import { AgentBody } from "@/components/AddAgent";
import { CheckForm } from "@/components/AddCheck";
import { EnvironmentForm } from "@/components/NewEnvironment";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Label } from "@/components/ui/label";

export type What = "host" | "check" | "environment";

// AddMenu is the one place to add something: a host, a check, or an
// environment for them. Hosts and checks ask first where they belong.
export function AddMenu({ compact = false, direct }: { compact?: boolean; direct?: What }) {
  const { t } = useTranslation();
  const [what, setWhat] = useState<What | null>(null);
  const dialog = (
    <Dialog open={what !== null} onOpenChange={(open) => !open && setWhat(null)}>
      <DialogContent className={what === "host" ? "sm:max-w-2xl" : undefined}>{what && <Body what={what} done={() => setWhat(null)} />}</DialogContent>
    </Dialog>
  );
  if (direct) {
    return (
      <>
        <Button size="sm" onClick={() => setWhat(direct)}>
          <Plus />
          {t(`add.${direct}`)}
        </Button>
        {dialog}
      </>
    );
  }
  return (
    <>
      <DropdownMenu>
        {compact ? (
          <DropdownMenuTrigger render={<Button size="icon-sm" aria-label={t("add.title")} />}>
            <Plus />
          </DropdownMenuTrigger>
        ) : (
          <DropdownMenuTrigger render={<Button size="sm" />}>
            <Plus />
            {t("add.title")}
          </DropdownMenuTrigger>
        )}
        <DropdownMenuContent align={compact ? "start" : "end"} className="w-52">
          <DropdownMenuItem onClick={() => setWhat("host")}>
            <Server />
            {t("add.host")}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setWhat("check")}>
            <Activity />
            {t("add.check")}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setWhat("environment")}>
            <FolderPlus />
            {t("add.environment")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog}
    </>
  );
}

function Body({ what, done }: { what: What; done: () => void }) {
  const { t } = useTranslation();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const envs = (scopes.data ?? []).filter((s) => s.kind === "environment");
  const byID = new Map((scopes.data ?? []).map((s) => [s.id, s]));
  const owners = (e: Scope) => {
    const project = e.parent_id != null ? byID.get(e.parent_id) : undefined;
    const customer = project?.parent_id != null ? byID.get(project.parent_id) : undefined;
    return [customer?.name, project?.name].filter(Boolean).join(" · ");
  };
  const [env, setEnv] = useState<number | null>(null);
  if (what === "environment") return <EnvironmentForm customer="" project="" done={done} />;
  const chosen = env ?? (envs.length === 1 ? envs[0].id : null);
  if (scopes.isPending) return null;
  if (envs.length === 0) {
    return (
      <div className="space-y-4">
        <DialogHeader>
          <DialogTitle>{t(what === "host" ? "add.host" : "add.check")}</DialogTitle>
          <DialogDescription>{t("add.environmentFirst")}</DialogDescription>
        </DialogHeader>
        <EnvironmentForm customer="" project="" done={() => {}} />
      </div>
    );
  }
  if (chosen === null) {
    return (
      <div className="space-y-4">
        <DialogHeader>
          <DialogTitle>{t(what === "host" ? "add.host" : "add.check")}</DialogTitle>
          <DialogDescription>{t("add.where")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label>{t("add.environmentLabel")}</Label>
          <ul className="max-h-72 overflow-y-auto rounded-md border">
            {envs.map((e) => (
              <li key={e.id}>
                <button type="button" onClick={() => setEnv(e.id)} className="flex w-full items-baseline gap-2 border-b px-3 py-2.5 text-left last:border-b-0 hover:bg-accent">
                  {/* Environments are often named alike, such as production:
                      the customer and project tell them apart. */}
                  <span className="font-medium">{e.name}</span>
                  <span className="min-w-0 truncate text-muted-foreground">{owners(e)}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>
    );
  }
  return what === "host" ? <AgentBody envID={chosen} onDone={done} /> : <CheckForm envID={chosen} onDone={done} />;
}
