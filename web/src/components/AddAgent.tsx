import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Plus, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getEnrollment, getHealth, newEnrollment, NotFound } from "@/api";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
      <DialogContent className="sm:max-w-2xl">{open && <Body envID={envID} />}</DialogContent>
    </Dialog>
  );
}

function Body({ envID }: { envID: number }) {
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
        <Tabs defaultValue="systemd">
          <TabsList>
            <TabsTrigger value="systemd">{t("agent.systemd")}</TabsTrigger>
            <TabsTrigger value="docker">docker run</TabsTrigger>
            <TabsTrigger value="compose">compose</TabsTrigger>
            {cmds.local && <TabsTrigger value="local">{t("agent.local")}</TabsTrigger>}
          </TabsList>
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
      <div className="flex items-center justify-between gap-4 text-xs text-muted-foreground">
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
        <pre className="max-h-80 overflow-auto rounded-lg bg-muted p-3 pr-12 font-mono text-xs leading-relaxed whitespace-pre-wrap break-all">
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
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  );
}
