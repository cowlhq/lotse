import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getProbeEnrollment, listProbes, newProbeEnrollment, NotFound, revokeProbe } from "@/api";
import { Online } from "@/components/Status";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("probe.title")}</CardTitle>
        <CardDescription>{t("probe.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <ul className="divide-y">
          {probes.data?.map((p) => (
            <li key={p.id} className="flex items-center justify-between gap-4 py-2">
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
        {isAdmin && <AddProbe />}
      </CardContent>
    </Card>
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
  const e = enrollment.data;
  const insecure = e && new URL(e.hub).protocol === "http:" ? "\n  -e LOTSE_INSECURE=true \\" : "";
  const cmd =
    e && slug
      ? `docker run -d --name lotse-probe --restart unless-stopped \\
  -e LOTSE_HUB=${e.hub} \\
  -e LOTSE_TOKEN=${e.token} \\${insecure}
  -e LOTSE_PROBE_NAME=${slug} \\
  -e LOTSE_PROBE_LOCATION="${location}" \\
  -v lotse-probe:/var/lib/lotse \\
  ghcr.io/cowlhq/lotse probe`
      : "";
  return (
    <div className="min-w-0 space-y-4">
      <DialogHeader>
        <DialogTitle>{t("probe.add")}</DialogTitle>
        <DialogDescription>{t("probe.addHint")}</DialogDescription>
      </DialogHeader>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="probe-name">{t("probe.name")}</Label>
          <Input id="probe-name" value={name} onChange={(ev) => setName(ev.target.value)} placeholder="fra" />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="probe-location">{t("probe.location")}</Label>
          <Input id="probe-location" value={location} onChange={(ev) => setLocation(ev.target.value)} placeholder="Frankfurt" />
        </div>
      </div>
      {cmd && <pre className="overflow-auto rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap break-all">{cmd}</pre>}
      <p className="text-xs text-muted-foreground">{t("probe.tokenHint")}</p>
    </div>
  );
}
