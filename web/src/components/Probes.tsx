import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getProbeEnrollment, listProbes, newProbeEnrollment, NotFound, revokeProbe } from "@/api";
import { Command } from "@/components/Command";
import { StatusIcon } from "@/components/shell/Dot";
import { PageTabs } from "@/components/shell/PageTabs";
import { Online } from "@/components/Status";
import { Panel } from "@/components/shell/Panel";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { probeCommands } from "@/lib/install";
import { slugify } from "@/lib/slug";
import { useMe } from "@/lib/useMe";

// Probes lists the places checks run from: the hub itself and probes
// elsewhere, so that a down host does not take its checker with it.
export function Probes() {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const probes = useQuery({ queryKey: ["probes"], queryFn: listProbes, refetchInterval: 15_000 });
  const revoke = useMutation({ mutationFn: revokeProbe, onSuccess: () => client.invalidateQueries({ queryKey: ["probes"] }) });
  return (
    <Panel title={t("probe.title")} count={probes.data?.filter((p) => !p.revoked).length} right={isAdmin && <AddProbe />} flush>
      <p className="border-b px-4 py-3 text-muted-foreground">{t("probe.description")}</p>
      <ul className="divide-y">
        {probes.data?.map((p) => (
          <li key={p.id} className="flex items-center justify-between gap-4 px-4 py-3">
            <span>
              <span className="font-medium">{p.builtin ? t("probe.builtin") : p.name}</span>
              {p.location && <span className="ml-2 text-muted-foreground">{p.location}</span>}
              <span className="ml-2 text-xs text-muted-foreground">{p.version}</span>
            </span>
            <span className="flex items-center gap-3">
              {p.revoked ? <span className="text-muted-foreground">{t("probe.revoked")}</span> : <Online online={p.online} />}
              {isAdmin && !p.builtin && !p.revoked && (
                <Button variant="ghost" size="sm" onClick={() => confirm(t("probe.revokeConfirm")) && revoke.mutate(p.id)}>
                  {t("probe.revoke")}
                </Button>
              )}
            </span>
          </li>
        ))}
      </ul>
    </Panel>
  );
}

function AddProbe() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <Plus className="size-4" />
        {t("probe.add")}
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">{open && <AddProbeBody />}</DialogContent>
    </Dialog>
  );
}

function AddProbeBody() {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [location, setLocation] = useState("");
  const enrollment = useQuery({
    queryKey: ["probe-enrollment"],
    queryFn: () => getProbeEnrollment().catch((e) => (e instanceof NotFound ? newProbeEnrollment() : Promise.reject(e))),
  });
  const slug = slugify(name);
  const cmds = enrollment.data && slug ? probeCommands(enrollment.data.hub, enrollment.data.token, slug, location.trim()) : undefined;
  return (
    <div className="min-w-0 space-y-4">
      <DialogHeader>
        <DialogTitle>{t("probe.add")}</DialogTitle>
        <DialogDescription>{t("probe.addHint")}</DialogDescription>
      </DialogHeader>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="probe-name">{t("probe.name")}</Label>
          <Input id="probe-name" value={name} onChange={(ev) => setName(ev.target.value)} placeholder="fra" autoFocus />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="probe-location">{t("probe.location")}</Label>
          <Input id="probe-location" value={location} onChange={(ev) => setLocation(ev.target.value)} placeholder="Frankfurt" />
        </div>
      </div>
      {cmds ? (
        <>
          <Tabs defaultValue="systemd" className="gap-0">
            <PageTabs
              tabs={[
                { value: "systemd", label: t("agent.systemd") },
                { value: "docker", label: "docker run" },
              ]}
            />
            <TabsContent value="systemd">
              <Command text={cmds.systemd} hint={t("probe.systemdHint")} />
            </TabsContent>
            <TabsContent value="docker">
              <Command text={cmds.docker} hint={t("probe.dockerHint")} />
            </TabsContent>
          </Tabs>
          <Waiting slug={slug} />
        </>
      ) : (
        <p className="text-mini text-muted-foreground">{t("probe.nameFirst")}</p>
      )}
      <p className="text-mini text-muted-foreground">{t("probe.tokenHint")}</p>
    </div>
  );
}

// Waiting watches for the probe to connect under its name.
function Waiting({ slug }: { slug: string }) {
  const { t } = useTranslation();
  const probes = useQuery({ queryKey: ["probes"], queryFn: listProbes, refetchInterval: 3000 });
  const arrived = probes.data?.find((p) => p.slug === slug && p.online && !p.revoked);
  if (arrived) {
    return (
      <div className="flex items-center gap-3 rounded-lg border px-4 py-3" style={{ borderColor: "color-mix(in oklch, var(--ok) 40%, transparent)" }}>
        <StatusIcon level="ok" size={18} />
        <p className="flex-1">{t("probe.arrived", { name: arrived.name })}</p>
      </div>
    );
  }
  return (
    <div className="flex items-center gap-3 rounded-lg border border-dashed px-4 py-3 text-muted-foreground">
      <Loader2 className="size-4 animate-spin" />
      {t("probe.waiting")}
    </div>
  );
}
