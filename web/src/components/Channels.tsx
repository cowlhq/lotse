import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, Hash, Mail, MessageSquare, MessagesSquare, Pencil, Plus, Send, Smartphone, Trash2, TriangleAlert, Undo2, Webhook } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { createChannel, deleteChannel, getConfig, listChannels, setSetting, testChannel, updateChannel, type Channel, type ConfigValue, type SecretInput, type TestResult } from "@/api";
import { emptySecret, SecretField, secretInput, type SecretDraft } from "@/components/Secret";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Panel } from "@/components/shell/Panel";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { slugify } from "@/lib/slug";
import { useMe } from "@/lib/useMe";

type Type = Channel["type"];
const types: Type[] = ["email", "slack", "discord", "telegram", "ntfy", "pushover", "googlechat", "webhook"];

// hooks are the types that are nothing but a webhook's address.
const hooks: Type[] = ["googlechat", "webhook", "slack", "discord"];

const hookPlaceholder: Partial<Record<Type, string>> = {
  googlechat: "https://chat.googleapis.com/v1/spaces/…",
  slack: "https://hooks.slack.com/services/…",
  discord: "https://discord.com/api/webhooks/…",
  webhook: "https://example.com/hooks/lotse",
};

// Channels are where alerts go, defined on a scope: those of this scope,
// and those from above that can be used here. A switch on each says whether
// the alerts of this scope go to it, which is the setting notify.channels:
// the nearest that sets it wins, so a customer's alerts can go to its own
// channel and nowhere else.
export function Channels({ scopeID, subject, name }: { scopeID: number; subject: string; name?: string }) {
  const { t, i18n } = useTranslation();
  const { isAdmin } = useMe();
  const channels = useQuery({ queryKey: ["channels", scopeID], queryFn: () => listChannels(scopeID) });
  const config = useQuery({ queryKey: ["config", subject], queryFn: () => getConfig(subject) });
  const route = config.data?.find((v) => v.key === "notify.channels");
  const repeat = config.data?.find((v) => v.key === "notify.repeat");
  const on = (route?.value as string[] | undefined) ?? [];
  const all = channels.data ?? [];
  // The nearest channel of a key is the one alerts reach; one further up
  // with the same key is hidden by it.
  const usable = all
    .filter((c, i) => all.findIndex((d) => d.key === c.key) === i || c.scope_id === scopeID)
    .sort((a, b) => Number(b.scope_id === scopeID) - Number(a.scope_id === scopeID));
  const missing = on.filter((k) => !usable.some((c) => c.key === k));
  const save = useSetting(subject, "notify.channels");
  const set = (keys: string[]) => save.mutate(keys);
  const here = route?.from !== undefined && `${route.from.kind}:${route.from.id}` === subject;
  const global = subject.startsWith("global:");
  const who = name ?? t("route.everything");
  const names = on.map((k) => usable.find((c) => c.key === k)?.name ?? k);

  return (
    <Panel title={t("route.title")} count={usable.length} right={isAdmin && <ChannelDialog scopeID={scopeID} subject={subject} routed={on} who={who} />} flush>
      {route && (
        <div className={`flex items-start gap-3 border-b px-4 py-3 ${on.length === 0 ? "bg-warn/10" : ""}`}>
          {on.length === 0 ? <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warn" /> : <Send className="mt-0.5 size-4 shrink-0 text-muted-foreground" />}
          <div className="min-w-0 flex-1">
            <p className={on.length === 0 ? "font-medium" : undefined}>
              {on.length === 0 ? t("route.nowhere", { who }) : t("route.to", { who, list: new Intl.ListFormat(i18n.language, { type: "conjunction" }).format(names) })}
            </p>
            <p className="text-mini text-muted-foreground">
              {on.length === 0 ? t(usable.length ? "route.nowhereHint" : "route.nowhereAdd") : here || global ? t("route.setHere") : !route.from || route.from.kind === "global" ? t("route.inheritedGlobal") : t("route.inherited", { from: route.from.name || route.from.path })}
            </p>
          </div>
          {isAdmin && here && !global && (
            <Button variant="ghost" size="sm" onClick={() => save.mutate(null)} disabled={save.isPending}>
              <Undo2 className="size-3.5" />
              {t("route.inherit")}
            </Button>
          )}
        </div>
      )}
      {usable.length === 0 && <p className="px-4 py-8 text-center text-muted-foreground">{t("channel.none")}</p>}
      <ul className="divide-y">
        {usable.map((c) => (
          <ChannelRow
            key={c.id}
            channel={c}
            scopeID={scopeID}
            subject={subject}
            on={on.includes(c.key)}
            onChange={(v) => set(v ? [...on, c.key] : on.filter((k) => k !== c.key))}
            busy={save.isPending}
          />
        ))}
        {missing.map((k) => (
          <li key={k} className="flex items-center gap-3 px-4 py-3">
            <TriangleAlert className="size-4 shrink-0 text-warn" />
            <p className="min-w-0 flex-1">{t("route.missing", { key: k })}</p>
            {isAdmin && (
              <Button variant="ghost" size="sm" onClick={() => set(on.filter((x) => x !== k))}>
                {t("route.drop")}
              </Button>
            )}
          </li>
        ))}
      </ul>
      {repeat && <Repeat subject={subject} value={repeat} />}
      {save.isError && <p className="border-t px-4 py-2 text-xs text-destructive">{save.error.message}</p>}
    </Panel>
  );
}

// repeats are the choices for reminding of an alert nobody acknowledged.
const repeats = ["0s", "30m", "1h", "4h", "12h", "24h"];

// Repeat is notify.repeat: how often an alert that still fires is sent
// again until someone acknowledges it.
function Repeat({ subject, value: v }: { subject: string; value: ConfigValue }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const save = useSetting(subject, "notify.repeat");
  const current = String(v.value);
  const here = v.from !== undefined && `${v.from.kind}:${v.from.id}` === subject;
  return (
    <div className="flex flex-wrap items-center gap-x-6 gap-y-2 border-t px-4 py-3">
      <div className="min-w-0 flex-1 basis-56">
        <p className="font-medium">{t("route.repeat")}</p>
        <p className="text-mini text-muted-foreground">{t("route.repeatHint")}</p>
      </div>
      <span className="flex items-center gap-1">
        <ToggleGroup value={[repeats.includes(current) ? current : ""]} onValueChange={(x: string[]) => x[0] && save.mutate(x[0])} variant="segmented" size="sm" disabled={!isAdmin || save.isPending}>
          {repeats.map((r) => (
            <ToggleGroupItem key={r} value={r} className="text-xs">
              {r === "0s" ? t("route.never") : r}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        {isAdmin && here && !subject.startsWith("global:") && (
          <Button variant="ghost" size="icon-sm" aria-label={t("config.reset")} title={t("config.reset")} onClick={() => save.mutate(null)}>
            <Undo2 className="size-3.5" />
          </Button>
        )}
      </span>
    </div>
  );
}

// useSetting saves a setting, or unsets it with null.
function useSetting(subject: string, key: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (value: unknown) => setSetting(subject, key, value),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["config"] });
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });
}

const typeIcon: Record<Type, typeof Mail> = {
  email: Mail,
  slack: Hash,
  discord: MessagesSquare,
  telegram: Send,
  ntfy: BellRing,
  pushover: Smartphone,
  googlechat: MessageSquare,
  webhook: Webhook,
};

function ChannelRow({ channel: c, scopeID, subject, on, onChange, busy }: { channel: Channel; scopeID: number; subject: string; on: boolean; onChange: (on: boolean) => void; busy: boolean }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const [result, setResult] = useState<TestResult | null>(null);
  const test = useMutation({ mutationFn: () => testChannel(c.id), onSuccess: setResult });
  const remove = useMutation({
    mutationFn: () => deleteChannel(c.id),
    onSuccess: () => (client.invalidateQueries({ queryKey: ["channels"] }), toast.success(t("channel.deleted"))),
  });
  const spec = c.spec as { to?: string[]; smtp?: { host: string }; server?: string; chat_id?: string };
  const Icon = typeIcon[c.type];
  const mine = c.scope_id === scopeID;
  return (
    <li className={`space-y-1 px-4 py-3 ${on ? "" : "text-muted-foreground"}`}>
      <div className="flex items-center gap-3">
        <span className={`flex size-8 shrink-0 items-center justify-center rounded-md border ${on ? "bg-background text-foreground" : "bg-muted/60"}`}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate">
            <span className={on ? "font-medium text-foreground" : "font-medium"}>{c.name}</span>
            <span className="ml-2 font-mono text-mini text-muted-foreground/80">{c.key}</span>
          </p>
          <p className="truncate text-mini text-muted-foreground">
            {t(`channel.type.${c.type}`)}
            {spec.to && <> · {spec.to.join(", ")}</>}
            {spec.smtp && <> · {spec.smtp.host}</>}
            {spec.server && <> · {spec.server}</>}
            {!mine && <> · {t("route.from", { from: c.scope_path || t("config.kinds.global") })}</>}
          </p>
        </div>
        {isAdmin && (
          <span className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="sm" disabled={test.isPending} onClick={() => (setResult(null), test.mutate())}>
              <Send className="size-3.5" />
              {t("channel.test")}
            </Button>
            {mine && <ChannelDialog scopeID={c.scope_id} subject={subject} channel={c} />}
            {mine && (
              <Button variant="ghost" size="icon-sm" aria-label={t("channel.delete")} onClick={() => confirm(t("channel.deleteConfirm", { key: c.key })) && remove.mutate()}>
                <Trash2 className="size-3.5" />
              </Button>
            )}
          </span>
        )}
        <Switch checked={on} disabled={!isAdmin || busy} onCheckedChange={onChange} aria-label={t("route.switch", { name: c.name })} />
      </div>
      {result && (
        <p className={result.sent ? "pl-11 text-xs text-muted-foreground" : "pl-11 text-xs text-destructive"}>
          {result.sent ? t("channel.testSent") : t("channel.testFailed", { error: result.error })}
        </p>
      )}
    </li>
  );
}

function ChannelDialog({ scopeID, subject, routed, who, channel }: { scopeID: number; subject: string; routed?: string[]; who?: string; channel?: Channel }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {channel ? (
        <DialogTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("channel.edit")} />}>
          <Pencil className="size-3.5" />
        </DialogTrigger>
      ) : (
        <DialogTrigger render={<Button size="sm" variant="outline" />}>
          <Plus className="size-4" />
          {t("channel.add")}
        </DialogTrigger>
      )}
      <DialogContent className="sm:max-w-lg">{open && <ChannelForm scopeID={scopeID} subject={subject} routed={routed} who={who} channel={channel} onDone={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

type SMTPDraft = { host: string; port: string; username: string; from: string; tls: "starttls" | "tls" | "none" };

function ChannelForm({ scopeID, subject, routed, who, channel, onDone }: { scopeID: number; subject: string; routed?: string[]; who?: string; channel?: Channel; onDone: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const spec = (channel?.spec ?? {}) as { to?: string[]; smtp?: { host: string; port: number; username?: string; from: string; tls: SMTPDraft["tls"] }; server?: string; chat_id?: string };
  const [type, setType] = useState<Type>(channel?.type ?? "email");
  const [name, setName] = useState(channel?.name ?? "");
  const [key, setKey] = useState(channel?.key ?? "");
  const [to, setTo] = useState((spec.to ?? []).join(", "));
  const [ownServer, setOwnServer] = useState(!!spec.smtp);
  const [ntfyServer, setNtfyServer] = useState(spec.server ?? "");
  const [chatID, setChatID] = useState(spec.chat_id ?? "");
  const [smtp, setSMTP] = useState<SMTPDraft>({
    host: spec.smtp?.host ?? "",
    port: spec.smtp ? String(spec.smtp.port) : "587",
    username: spec.smtp?.username ?? "",
    from: spec.smtp?.from ?? "",
    tls: spec.smtp?.tls ?? "starttls",
  });
  // A new channel gets the alerts from here, besides those it got so far.
  const [route, setRoute] = useState(true);
  const [secrets, setSecrets] = useState<Record<string, SecretDraft>>({});
  const draft = (field: string) => secrets[field] ?? emptySecret;
  const setDraft = (field: string) => (d: SecretDraft) => setSecrets({ ...secrets, [field]: d });

  const save = useMutation({
    mutationFn: async () => {
      const out: Record<string, unknown> = {};
      if (type === "email") {
        out.to = to.split(/[\s,;]+/).filter(Boolean);
        if (ownServer) out.smtp = { host: smtp.host.trim(), port: Number(smtp.port) || 0, username: smtp.username.trim() || undefined, from: smtp.from.trim(), tls: smtp.tls };
      }
      if (type === "ntfy" && ntfyServer.trim()) out.server = ntfyServer.trim();
      if (type === "telegram") out.chat_id = chatID.trim();
      const sec: Record<string, SecretInput> = {};
      for (const [field, d] of Object.entries(secrets)) {
        const x = secretInput(d);
        if (x) sec[field] = x;
      }
      if (type === "email" && !ownServer && channel?.secrets.password?.set) sec.password = {};
      const n = name.trim();
      if (channel) return updateChannel(channel.id, { name: n, spec: out, secrets: sec });
      const c = await createChannel({ scope_id: scopeID, key: key.trim() || slugify(n), name: n, type, spec: out, secrets: sec });
      if (route && routed && !routed.includes(c.key)) await setSetting(subject, "notify.channels", [...routed, c.key]);
      return c;
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["channels"] });
      client.invalidateQueries({ queryKey: ["config"] });
      client.invalidateQueries({ queryKey: ["changes"] });
      toast.success(t("channel.saved"));
      onDone();
    },
  });

  const smtpField = (k: keyof SMTPDraft, label: string, placeholder: string) => (
    <div className="space-y-1.5">
      <Label htmlFor={`smtp-${k}`}>{label}</Label>
      <Input id={`smtp-${k}`} value={smtp[k]} onChange={(e) => setSMTP({ ...smtp, [k]: e.target.value })} placeholder={placeholder} autoComplete="off" />
    </div>
  );

  return (
    <form
      className="min-w-0 space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <DialogHeader>
        <DialogTitle>{channel ? t("channel.edit") : t("channel.add")}</DialogTitle>
        <DialogDescription>{t(`channel.typeHint.${type}`)}</DialogDescription>
      </DialogHeader>
      {!channel && (
        <ToggleGroup value={[type]} onValueChange={(v: string[]) => v[0] && setType(v[0] as Type)} variant="segmented" size="sm" className="grid w-full grid-cols-2 sm:grid-cols-4">
          {types.map((k) => (
            <ToggleGroupItem key={k} value={k}>
              {t(`channel.type.${k}`)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      )}
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="channel-name">{t("channel.name")}</Label>
          <Input id="channel-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("channel.namePlaceholder")} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="channel-key">{t("channel.key")}</Label>
          <Input id="channel-key" value={channel ? channel.key : key} disabled={!!channel} onChange={(e) => setKey(e.target.value)} placeholder={slugify(name) || "ops-mail"} />
        </div>
      </div>
      {type === "email" && (
        <>
          <div className="space-y-1.5">
            <Label htmlFor="channel-to">{t("channel.to")}</Label>
            <Input id="channel-to" value={to} onChange={(e) => setTo(e.target.value)} placeholder="ops@example.com, oncall@example.com" />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={ownServer} onChange={(e) => setOwnServer(e.target.checked)} />
            {t("channel.ownServer")}
          </label>
          {ownServer && (
            <div className="space-y-3 rounded-lg border p-3">
              <div className="grid grid-cols-[1fr_6rem] gap-3">
                {smtpField("host", t("mail.host"), "smtp.example.com")}
                {smtpField("port", t("mail.port"), "587")}
              </div>
              <TLSPicker value={smtp.tls} onChange={(tls) => setSMTP({ ...smtp, tls })} />
              {smtpField("from", t("mail.from"), "lotse <lotse@example.com>")}
              {smtpField("username", t("mail.username"), t("check.optional"))}
              <SecretField id="channel-password" label={t("mail.password")} info={channel?.secrets.password} draft={draft("password")} onChange={setDraft("password")} />
            </div>
          )}
        </>
      )}
      {hooks.includes(type) && (
        <SecretField
          id="channel-url"
          label={t("channel.url")}
          hint={t(`channel.urlHint.${type}`)}
          info={channel?.secrets.url}
          draft={draft("url")}
          onChange={setDraft("url")}
          placeholder={hookPlaceholder[type]}
        />
      )}
      {type === "telegram" && (
        <>
          <SecretField id="channel-bot" label={t("channel.botToken")} hint={t("channel.botTokenHint")} info={channel?.secrets.bot_token} draft={draft("bot_token")} onChange={setDraft("bot_token")} placeholder="123456:ABC-…" />
          <div className="space-y-1.5">
            <Label htmlFor="channel-chat">{t("channel.chatID")}</Label>
            <Input id="channel-chat" value={chatID} onChange={(e) => setChatID(e.target.value)} placeholder="-1001234567890" autoComplete="off" />
            <p className="text-xs text-muted-foreground">{t("channel.chatIDHint")}</p>
          </div>
        </>
      )}
      {type === "ntfy" && (
        <>
          <SecretField id="channel-topic" label={t("channel.topic")} hint={t("channel.topicHint")} info={channel?.secrets.topic} draft={draft("topic")} onChange={setDraft("topic")} placeholder="lotse-ops-7f3k2" />
          <div className="space-y-1.5">
            <Label htmlFor="channel-server">{t("channel.server")}</Label>
            <Input id="channel-server" value={ntfyServer} onChange={(e) => setNtfyServer(e.target.value)} placeholder="https://ntfy.sh" autoComplete="off" />
          </div>
          <SecretField id="channel-token" label={t("channel.accessToken")} hint={t("channel.accessTokenHint")} info={channel?.secrets.token} draft={draft("token")} onChange={setDraft("token")} placeholder={t("check.optional")} />
        </>
      )}
      {type === "pushover" && (
        <>
          <SecretField id="channel-user" label={t("channel.pushoverUser")} hint={t("channel.pushoverUserHint")} info={channel?.secrets.user} draft={draft("user")} onChange={setDraft("user")} />
          <SecretField id="channel-app" label={t("channel.pushoverToken")} hint={t("channel.pushoverTokenHint")} info={channel?.secrets.token} draft={draft("token")} onChange={setDraft("token")} />
        </>
      )}
      {type === "webhook" && (
        <SecretField id="channel-signing" label={t("channel.signingKey")} hint={t("channel.signingHint")} info={channel?.secrets.signing_key} draft={draft("signing_key")} onChange={setDraft("signing_key")} placeholder={t("check.optional")} />
      )}
      {!channel && routed && (
        <div className="flex items-start justify-between gap-4 rounded-lg border px-3 py-2.5">
          <div className="space-y-0.5">
            <Label htmlFor="channel-route">{t("route.newSwitch", { who })}</Label>
            <p className="text-xs text-muted-foreground">{t("route.newSwitchHint")}</p>
          </div>
          <Switch id="channel-route" checked={route} onCheckedChange={setRoute} />
        </div>
      )}
      {save.isError && (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
      <DialogFooter>
        <Button type="submit" disabled={save.isPending || !name.trim()}>
          {t("tree.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function TLSPicker({ value, onChange }: { value: SMTPDraft["tls"]; onChange: (v: SMTPDraft["tls"]) => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label>{t("mail.tls")}</Label>
      <ToggleGroup value={[value]} onValueChange={(v: string[]) => v[0] && onChange(v[0] as SMTPDraft["tls"])} variant="segmented" size="sm">
        {(["starttls", "tls", "none"] as const).map((m) => (
          <ToggleGroupItem key={m} value={m}>
            {t(`mail.tlsMode.${m}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <p className="text-xs text-muted-foreground">{t(`mail.tlsHint.${value}`)}</p>
    </div>
  );
}
