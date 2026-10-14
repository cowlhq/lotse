import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Loader2, Plus, RefreshCw } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getEnrollment, getHealth, listHosts, newEnrollment, NotFound } from "@/api";
import { PageTabs } from "@/components/shell/PageTabs";
import { StatusIcon } from "@/components/shell/Dot";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { installCommands } from "@/lib/install";

// AddAgent shows the command that puts an agent on a machine for this
// environment: one line to copy (design 7, step 3).
export function AddAgent({ envID }: { envID: number }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" />}>
        <Plus className="size-4" />
        {t("agent.add")}
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">{open && <AgentBody envID={envID} onDone={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

export function AgentBody({ envID, onDone }: { envID: number; onDone?: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  // An environment gets its token the first time someone adds an agent.
  const enrollment = useQuery({
    queryKey: ["enrollment", envID],
    queryFn: () => getEnrollment(envID).catch((e) => (e instanceof NotFound ? newEnrollment(envID) : Promise.reject(e))),
  });
  const replace = useMutation({
    mutationFn: () => newEnrollment(envID),
    onSuccess: (e) => {
      client.setQueryData(["enrollment", envID], e);
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });

  const health = useQuery({ queryKey: ["health"], queryFn: getHealth });
  const dev = health.data?.version.startsWith("dev") ?? false;
  const cmds = enrollment.data && installCommands(enrollment.data.hub, enrollment.data.token, dev);
  return (
    <div className="min-w-0 space-y-4">
      <DialogHeader>
        <DialogTitle>{t("agent.add")}</DialogTitle>
        <DialogDescription>{t("agent.addHint")}</DialogDescription>
      </DialogHeader>
      {cmds && (
        <Tabs defaultValue="systemd" className="gap-0">
          <PageTabs
            tabs={[
              { value: "systemd", label: t("agent.systemd") },
              { value: "docker", label: "docker run" },
              { value: "compose", label: "compose" },
              ...(cmds.local ? [{ value: "local", label: t("agent.local") }] : []),
            ]}
          />
          <TabsContent value="systemd">
            <Command text={cmds.systemd} hint={t("agent.systemdHint")} />
          </TabsContent>
          <TabsContent value="docker">
            <Command text={cmds.docker} hint={t("agent.dockerHint")} />
          </TabsContent>
          <TabsContent value="compose">
            <Command text={cmds.compose} hint={t("agent.composeHint")} />
          </TabsContent>
          {cmds.local && (
            <TabsContent value="local">
              <Command text={cmds.local} hint={t("agent.localHint")} />
            </TabsContent>
          )}
        </Tabs>
      )}
      <Waiting envID={envID} onDone={onDone} />
      <div className="flex items-center justify-between gap-4 text-mini text-muted-foreground">
        <span>{t("agent.tokenHint")}</span>
        <Button
          variant="outline"
          size="sm"
          disabled={replace.isPending}
          onClick={() => confirm(t("agent.replaceConfirm")) && replace.mutate()}
        >
          <RefreshCw className="size-3.5" />
          {t("agent.replace")}
        </Button>
      </div>
    </div>
  );
}

function Command({ text, hint }: { text: string; hint: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  return (
    <div className="space-y-2 pt-2">
      <div className="relative">
        <pre className="max-h-80 overflow-auto rounded-lg border bg-muted/60 p-3 pr-12 font-mono text-mini leading-relaxed whitespace-pre-wrap break-all">
          {text}
        </pre>
        <Button
          variant="ghost"
          size="icon-sm"
          className="absolute top-2 right-2"
          aria-label={t("agent.copy")}
          onClick={() => {
            navigator.clipboard?.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }}
        >
          {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
        </Button>
      </div>
      <p className="text-mini text-muted-foreground">{hint}</p>
    </div>
  );
}

// Waiting watches for the machine to arrive: a host in this environment
// that was not there when the dialog opened.
function Waiting({ envID, onDone }: { envID: number; onDone?: () => void }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts, refetchInterval: 3000 });
  const [before] = useState(() => new Set((hosts.data ?? []).map((h) => h.host.id)));
  const [ready, setReady] = useState(hosts.isSuccess);
  if (hosts.isSuccess && !ready) {
    // The first answer is what was there before.
    hosts.data.forEach((h) => before.add(h.host.id));
    setReady(true);
  }
  const arrived = ready ? (hosts.data ?? []).find((h) => h.host.environment_id === envID && !before.has(h.host.id)) : undefined;
  if (arrived) {
    return (
      <div className="flex items-center gap-3 rounded-lg border px-4 py-3" style={{ borderColor: "color-mix(in oklch, var(--ok) 40%, transparent)" }}>
        <StatusIcon level="ok" size={18} />
        <p className="flex-1">{t("agent.arrived", { name: arrived.host.name })}</p>
        <Button size="sm" onClick={() => (onDone?.(), navigate({ to: "/hosts/$id", params: { id: String(arrived.host.id) } }))}>
          {t("agent.open", { name: arrived.host.name })}
        </Button>
      </div>
    );
  }
  return (
    <div className="flex items-center gap-3 rounded-lg border border-dashed px-4 py-3 text-muted-foreground">
      <Loader2 className="size-4 animate-spin" />
      {t("agent.waiting")}
    </div>
  );
}
