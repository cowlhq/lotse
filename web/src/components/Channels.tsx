import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Send, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { createChannel, deleteChannel, listChannels, testChannel, updateChannel, type Channel, type SecretInput, type TestResult } from "@/api";
import { emptySecret, SecretField, secretInput, type SecretDraft } from "@/components/Secret";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { slugify } from "@/lib/slug";
import { useMe } from "@/lib/useMe";

type Type = Channel["type"];
const types: Type[] = ["email", "googlechat", "webhook"];

// Channels are where alerts go, defined on a scope: those of this scope,
// and those from above that can be used here. notify.channels names them.
export function Channels({ scopeID }: { scopeID: number }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const channels = useQuery({ queryKey: ["channels", scopeID], queryFn: () => listChannels(scopeID) });
  const here = (channels.data ?? []).filter((c) => c.scope_id === scopeID);
  const above = (channels.data ?? []).filter((c) => c.scope_id !== scopeID);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("channel.title")}</CardTitle>
        <CardDescription>{t("channel.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {here.length === 0 && above.length === 0 && <p className="text-muted-foreground">{t("channel.none")}</p>}
        {here.length > 0 && (
          <ul className="divide-y">
            {here.map((c) => (
              <ChannelRow key={c.id} channel={c} />
            ))}
          </ul>
        )}
        {above.length > 0 && (
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">{t("channel.above")}</p>
            <ul className="space-y-1">
              {above.map((c) => (
                <li key={c.id} className="text-muted-foreground">
                  <code className="text-foreground">{c.key}</code> · {c.name} · {t(`channel.type.${c.type}`)} ·{" "}
                  {c.scope_path || t("config.kinds.global")}
                </li>
              ))}
            </ul>
          </div>
        )}
        {isAdmin && <ChannelDialog scopeID={scopeID} />}
      </CardContent>
    </Card>
  );
}

function ChannelRow({ channel: c }: { channel: Channel }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const [result, setResult] = useState<TestResult | null>(null);
  const test = useMutation({ mutationFn: () => testChannel(c.id), onSuccess: setResult });
  const remove = useMutation({
    mutationFn: () => deleteChannel(c.id),
    onSuccess: () => (client.invalidateQueries({ queryKey: ["channels"] }), toast.success(t("channel.deleted"))),
  });
  const spec = c.spec as { to?: string[]; smtp?: { host: string } };
  return (
    <li className="space-y-1 py-2">
      <div className="flex items-center justify-between gap-4">
        <span className="min-w-0">
          <code>{c.key}</code> <span className="font-medium">{c.name}</span>
          <span className="ml-2 text-muted-foreground">
            {t(`channel.type.${c.type}`)}
            {spec.to && <> · {spec.to.join(", ")}</>}
            {spec.smtp && <> · {spec.smtp.host}</>}
          </span>
        </span>
        {isAdmin && (
          <span className="flex shrink-0 items-center gap-1">
            <Button variant="ghost" size="sm" disabled={test.isPending} onClick={() => (setResult(null), test.mutate())}>
              <Send className="size-3.5" />
              {t("channel.test")}
            </Button>
            <ChannelDialog scopeID={c.scope_id} channel={c} />
            <Button variant="ghost" size="icon-sm" aria-label={t("channel.delete")} onClick={() => confirm(t("channel.deleteConfirm", { key: c.key })) && remove.mutate()}>
              <Trash2 className="size-3.5" />
            </Button>
          </span>
        )}
      </div>
      {result && (
        <p className={result.sent ? "text-xs text-muted-foreground" : "text-xs text-destructive"}>
          {result.sent ? t("channel.testSent") : t("channel.testFailed", { error: result.error })}
        </p>
      )}
    </li>
  );
}

function ChannelDialog({ scopeID, channel }: { scopeID: number; channel?: Channel }) {
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
      <DialogContent className="sm:max-w-lg">{open && <ChannelForm scopeID={scopeID} channel={channel} onDone={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

type SMTPDraft = { host: string; port: string; username: string; from: string; tls: "starttls" | "tls" | "none" };

function ChannelForm({ scopeID, channel, onDone }: { scopeID: number; channel?: Channel; onDone: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const spec = (channel?.spec ?? {}) as { to?: string[]; smtp?: { host: string; port: number; username?: string; from: string; tls: SMTPDraft["tls"] } };
  const [type, setType] = useState<Type>(channel?.type ?? "email");
  const [name, setName] = useState(channel?.name ?? "");
  const [key, setKey] = useState(channel?.key ?? "");
  const [to, setTo] = useState((spec.to ?? []).join(", "));
  const [ownServer, setOwnServer] = useState(!!spec.smtp);
  const [smtp, setSMTP] = useState<SMTPDraft>({
    host: spec.smtp?.host ?? "",
    port: spec.smtp ? String(spec.smtp.port) : "587",
    username: spec.smtp?.username ?? "",
    from: spec.smtp?.from ?? "",
    tls: spec.smtp?.tls ?? "starttls",
  });
  const [secrets, setSecrets] = useState<Record<string, SecretDraft>>({});
  const draft = (field: string) => secrets[field] ?? emptySecret;
  const setDraft = (field: string) => (d: SecretDraft) => setSecrets({ ...secrets, [field]: d });

  const save = useMutation({
    mutationFn: () => {
      const out: Record<string, unknown> = {};
      if (type === "email") {
        out.to = to.split(/[\s,;]+/).filter(Boolean);
        if (ownServer) out.smtp = { host: smtp.host.trim(), port: Number(smtp.port) || 0, username: smtp.username.trim() || undefined, from: smtp.from.trim(), tls: smtp.tls };
      }
      const sec: Record<string, SecretInput> = {};
      for (const [field, d] of Object.entries(secrets)) {
        const x = secretInput(d);
        if (x) sec[field] = x;
      }
      if (type === "email" && !ownServer && channel?.secrets.password?.set) sec.password = {};
      const n = name.trim();
      return channel
        ? updateChannel(channel.id, { name: n, spec: out, secrets: sec })
        : createChannel({ scope_id: scopeID, key: key.trim() || slugify(n), name: n, type, spec: out, secrets: sec });
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["channels"] });
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
        <ToggleGroup value={[type]} onValueChange={(v: string[]) => v[0] && setType(v[0] as Type)} variant="segmented" size="sm">
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
      {type !== "email" && (
        <SecretField
          id="channel-url"
          label={t("channel.url")}
          hint={t(`channel.urlHint.${type}`)}
          info={channel?.secrets.url}
          draft={draft("url")}
          onChange={setDraft("url")}
          placeholder={type === "googlechat" ? "https://chat.googleapis.com/v1/spaces/…" : "https://example.com/hooks/lotse"}
        />
      )}
      {type === "webhook" && (
        <SecretField id="channel-signing" label={t("channel.signingKey")} hint={t("channel.signingHint")} info={channel?.secrets.signing_key} draft={draft("signing_key")} onChange={setDraft("signing_key")} placeholder={t("check.optional")} />
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
