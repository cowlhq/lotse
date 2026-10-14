import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { checkUpdates, getHealth, listHosts, updateHub } from "@/api";
import { Channels } from "@/components/Channels";
import { MailServer } from "@/components/MailServer";
import { Maintenance } from "@/components/Maintenance";
import { Config } from "@/components/Config";
import { History } from "@/components/History";
import { Probes } from "@/components/Probes";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ago } from "@/lib/format";
import { useMe } from "@/lib/useMe";
import { useUpdates } from "@/lib/useUpdates";
import { newer } from "@/lib/version";
import { UpdateAgents } from "./Scope";

// Settings are the global ones: what applies everywhere unless a customer,
// project, environment or host says otherwise; and the hub's version.
export function Settings() {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts });
  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">{t("settings.title")}</h1>
      <HubVersion />
      {isAdmin && <UpdateAgents scopeID={1} hosts={hosts.data ?? []} />}
      <Probes />
      <MailServer />
      <Channels scopeID={1} />
      <Maintenance subject="global:1" />
      <Config subject="global:1" />
      <History subject="global:1" />
    </div>
  );
}

function HubVersion() {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const { updates } = useUpdates();
  const [restarting, setRestarting] = useState(false);
  const check = useMutation({
    mutationFn: checkUpdates,
    onSuccess: (u) => client.setQueryData(["updates"], u),
  });
  const update = useMutation({
    mutationFn: updateHub,
    onSuccess: async () => {
      setRestarting(true);
      // The hub restarts into the new version: wait until it answers with it.
      const from = updates?.hub_version;
      for (let i = 0; i < 120; i++) {
        await new Promise((r) => setTimeout(r, 1000));
        const h = await getHealth().catch(() => null);
        if (h && h.version !== from) break;
      }
      location.reload();
    },
  });
  if (!updates) return null;
  const latest = updates.latest;
  const behind = latest && newer(latest.version, updates.hub_version);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("update.hub")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <p>
          {t("update.running", { version: updates.hub_version })}
          {latest && <> · {t("update.latest", { version: latest.version })}</>}
          {updates.checked_at && <span className="text-muted-foreground"> · {t("update.checked", { time: ago(updates.checked_at) })}</span>}
        </p>
        {updates.check_error && <p className="text-destructive">{updates.check_error}</p>}
        {behind && latest.notes && (
          <details>
            <summary className="cursor-pointer text-muted-foreground">{t("update.notes", { version: latest.version })}</summary>
            <pre className="mt-2 max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs whitespace-pre-wrap">{latest.notes}</pre>
          </details>
        )}
        {isAdmin && (
          <div className="flex gap-2">
            <Button size="sm" variant="outline" disabled={check.isPending} onClick={() => check.mutate()}>
              {t("update.check")}
            </Button>
            {behind && updates.can_update_hub && (
              <Button size="sm" disabled={update.isPending || restarting} onClick={() => confirm(t("update.hubConfirm", { version: latest.version })) && update.mutate()}>
                {restarting ? t("update.restarting") : t("update.hubTo", { version: latest.version })}
              </Button>
            )}
          </div>
        )}
        {behind && !updates.can_update_hub && <p className="text-muted-foreground">{t("update.hubManual")}</p>}
        {update.isError && (
          <Alert variant="destructive">
            <AlertDescription>{update.error.message}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  );
}
