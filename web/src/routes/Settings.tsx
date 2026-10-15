import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { checkUpdates, getHealth, listHosts, updateHub } from "@/api";
import { Page } from "@/components/shell/PageHeader";
import { ObjectHeader } from "@/components/shell/Panel";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BellRing, Cog, History, KeyRound, Radar, Send, SlidersHorizontal, Wrench } from "lucide-react";
import { Tokens } from "@/components/Tokens";
import { Channels } from "@/components/Channels";
import { MailServer } from "@/components/MailServer";
import { Maintenance } from "@/components/Maintenance";
import { Config } from "@/components/Config";
import { History as HistoryList } from "@/components/History";
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
export function Settings({ tab }: { tab?: string }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts });
  const sections = [
    { value: "general", label: t("settings.general"), icon: <SlidersHorizontal /> },
    { value: "notifications", label: t("settings.notifications"), icon: <Send /> },
    { value: "rules", label: t("settings.rules"), icon: <BellRing /> },
    { value: "probes", label: t("probe.title"), icon: <Radar /> },
    { value: "maintenance", label: t("maintenance.title"), icon: <Wrench /> },
    { value: "tokens", label: t("token.title"), icon: <KeyRound /> },
    { value: "advanced", label: t("settings.advanced"), icon: <Cog /> },
    { value: "activity", label: t("tabs.activity"), icon: <History /> },
  ];
  return (
    <>
      <ObjectHeader icon={null} title={t("settings.title")} />
      <Page>
        <Tabs defaultValue={tab ?? "general"} orientation="vertical" className="flex-col gap-6 md:flex-row">
          {/* A column beside the page; on a phone, a row that scrolls. */}
          <TabsList variant="line" className="-mx-4 h-auto w-auto shrink-0 flex-row! items-stretch justify-start gap-0.5 overflow-x-auto px-4 pb-1 [scrollbar-width:none] md:mx-0 md:w-52 md:flex-col! md:overflow-visible md:p-0">
            {sections.map((x) => (
              <TabsTrigger
                key={x.value}
                value={x.value}
                className="h-9 flex-none justify-start gap-2.5 rounded-md max-md:w-auto! border-0 px-2.5 text-ui! font-medium text-muted-foreground after:hidden hover:bg-accent/60 hover:text-foreground data-active:bg-accent! data-active:text-foreground! [&_svg]:size-4"
              >
                {x.icon}
                {x.label}
              </TabsTrigger>
            ))}
          </TabsList>
          <div className="min-w-0 flex-1">
            <TabsContent value="general" className="space-y-6">
              <HubVersion />
              {isAdmin && <UpdateAgents scopeID={1} hosts={hosts.data ?? []} />}
            </TabsContent>
            <TabsContent value="notifications" className="space-y-6">
              <MailServer />
              <Channels scopeID={1} />
            </TabsContent>
            <TabsContent value="rules">
              <Config subject="global:1" part="rules" />
            </TabsContent>
            <TabsContent value="probes">
              <Probes />
            </TabsContent>
            <TabsContent value="maintenance">
              <Maintenance subject="global:1" />
            </TabsContent>
            <TabsContent value="tokens" className="space-y-6">
              <Tokens />
            </TabsContent>
            <TabsContent value="advanced">
              <Config subject="global:1" part="general" />
            </TabsContent>
            <TabsContent value="activity">
              <HistoryList subject="global:1" />
            </TabsContent>
          </div>
        </Tabs>
      </Page>
    </>
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
