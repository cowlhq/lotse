import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Braces, CalendarClock, CircleCheck, CircleX, Globe, HeartPulse, Loader2, Network, Plug, Plus, ShieldCheck, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { createCheck, tryCheck, type Check, type CheckTryResult, type SecretInput } from "@/api";
import { emptySecret, SecretField, secretInput } from "@/components/Secret";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { certDays } from "@/components/CheckState";

// A check is added by what it is for, not by its protocol: a website and
// an API are both http checks, but ask different things first. Each asks
// for the little it needs, has the rest under More options, and can be
// tried before it is added.

type Kind = Check["type"];
type Intent = "website" | "api" | "port" | "certificate" | "heartbeat" | "domain" | "dns";

const intents: { key: Intent; kind: Kind; icon: LucideIcon }[] = [
  { key: "website", kind: "http", icon: Globe },
  { key: "api", kind: "http", icon: Braces },
  { key: "port", kind: "tcp", icon: Plug },
  { key: "certificate", kind: "tls", icon: ShieldCheck },
  { key: "heartbeat", kind: "heartbeat", icon: HeartPulse },
  { key: "domain", kind: "domain", icon: CalendarClock },
  { key: "dns", kind: "dns", icon: Network },
];

const recordTypes = ["A", "AAAA", "CNAME", "MX", "NS", "TXT"];
const methods = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"];

// Ports people check most, by what listens there.
const ports: Partial<Record<Intent, { name: string; port: string }[]>> = {
  port: [
    { name: "PostgreSQL", port: "5432" },
    { name: "MySQL", port: "3306" },
    { name: "Redis", port: "6379" },
    { name: "SSH", port: "22" },
    { name: "SMTP", port: "25" },
  ],
  certificate: [
    { name: "SMTPS", port: "465" },
    { name: "IMAPS", port: "993" },
    { name: "POP3S", port: "995" },
    { name: "LDAPS", port: "636" },
    { name: "HTTPS", port: "443" },
  ],
};

const periods = ["1h", "6h", "24h", "168h"];

// headers reads "Name: value" lines; null when a line has no colon.
function headers(text: string): Record<string, string> | null {
  const out: Record<string, string> = {};
  for (const line of text.split("\n").filter((l) => l.trim())) {
    const i = line.indexOf(":");
    if (i < 1) return null;
    out[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return out;
}

// AddCheck adds a check to an environment.
export function AddCheck({ envID }: { envID: number }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <Plus className="size-4" />
        {t("check.add")}
      </DialogTrigger>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">{open && <CheckForm envID={envID} onDone={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

// CheckForm first asks what to watch, then for that.
export function CheckForm({ envID, onDone }: { envID: number; onDone?: () => void }) {
  const { t } = useTranslation();
  const [intent, setIntent] = useState<Intent | null>(null);
  if (intent) return <IntentForm key={intent} envID={envID} intent={intent} onBack={() => setIntent(null)} onDone={onDone} />;
  return (
    <div className="space-y-4">
      <DialogHeader>
        <DialogTitle>{t("check.add")}</DialogTitle>
        <DialogDescription>{t("check.what")}</DialogDescription>
      </DialogHeader>
      <div className="grid gap-2 sm:grid-cols-2">
        {intents.map(({ key, icon: Icon }) => (
          <button
            key={key}
            type="button"
            onClick={() => setIntent(key)}
            className="flex items-start gap-3 rounded-lg border px-3 py-3 text-left transition-colors hover:border-brand/40 hover:bg-accent/60 focus-visible:ring-3 focus-visible:ring-ring/30 focus-visible:outline-none"
          >
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-foreground">
              <Icon className="size-4" />
            </span>
            <span className="min-w-0">
              <span className="block font-medium">{t(`check.intent.${key}.title`)}</span>
              <span className="block text-mini text-muted-foreground">{t(`check.intent.${key}.hint`)}</span>
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

function IntentForm({ envID, intent, onBack, onDone }: { envID: number; intent: Intent; onBack: () => void; onDone?: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const kind = intents.find((i) => i.key === intent)!.kind;
  const [name, setName] = useState("");
  const [f, setF] = useState<Record<string, string>>({
    period: "24h",
    grace: "1h",
    type: "A",
    method: "GET",
    port: intent === "certificate" ? "465" : "",
  });
  const [follow, setFollow] = useState(true);
  const [headerSecret, setHeaderSecret] = useState(emptySecret);
  const [password, setPassword] = useState(emptySecret);
  const [result, setResult] = useState<CheckTryResult | null>(null);
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => (setF({ ...f, [k]: e.target.value }), setResult(null));
  const put = (k: string, v: string) => (setF({ ...f, [k]: v }), setResult(null));
  const hasBody = !["GET", "HEAD"].includes(f.method);
  const parsedHeaders = headers(f.headers ?? "");
  const address = f.host?.trim() && f.port?.trim() ? `${f.host.trim()}:${f.port.trim()}` : "";

  const spec = (): Record<string, unknown> => {
    const pick = (...keys: string[]) => Object.fromEntries(keys.filter((k) => f[k]?.trim()).map((k) => [k, f[k].trim()]));
    switch (kind) {
      case "http":
        return {
          ...pick("url", "method", "ip", "keyword", "absent", "max_latency", "status"),
          ...(hasBody && f.body ? { body: f.body } : {}),
          ...(parsedHeaders && Object.keys(parsedHeaders).length ? { headers: parsedHeaders } : {}),
          ...(f.json_path?.trim() ? { json: { path: f.json_path.trim(), equals: f.json_equals ?? "" } } : {}),
          ...(follow ? {} : { follow_redirects: false }),
          ...(f.username?.trim() ? { basic_auth: { username: f.username.trim() } } : {}),
        };
      case "tls":
        return { address, ...pick("server_name") };
      case "tcp":
        return { address };
      case "dns":
        return { ...pick("name", "type", "server"), expect: (f.expect ?? "").split(/[\s,]+/).filter(Boolean) };
      case "domain":
        return pick("domain");
      default:
        return pick("period", "grace");
    }
  };
  // A secret header's value and basic_auth's password, which the hub keeps
  // apart from the spec.
  const secrets = () => {
    if (kind !== "http") return undefined;
    const out: Record<string, SecretInput> = {};
    const header = secretInput(headerSecret);
    if (header) out[`headers.${f.secret_header?.trim() || "Authorization"}`] = header;
    const pass = secretInput(password);
    if (pass && f.username?.trim()) out["basic_auth.password"] = pass;
    return Object.keys(out).length ? out : undefined;
  };
  // A name from what is checked, until one is typed.
  const suggested = (() => {
    try {
      if (kind === "http" && f.url) return new URL(f.url).host;
    } catch {
      /* not a URL yet */
    }
    if (kind === "dns") return f.name ? `${f.name} ${f.type}` : "";
    if (kind === "domain") return f.domain ?? "";
    if (kind === "heartbeat") return t("check.heartbeatName", { period: periods.includes(f.period) ? t(`check.periods.${f.period}`) : f.period });
    return address;
  })();
  const ready = (() => {
    if (kind === "http") return !!f.url?.trim() && !!parsedHeaders;
    if (kind === "tls" || kind === "tcp") return !!address;
    if (kind === "dns") return !!f.name?.trim();
    if (kind === "domain") return !!f.domain?.trim();
    return !!f.period;
  })();

  const test = useMutation({ mutationFn: () => tryCheck({ environment_id: envID, type: kind, spec: spec(), secrets: secrets() }), onSuccess: setResult });
  const save = useMutation({
    mutationFn: () => createCheck({ environment_id: envID, name: name.trim() || suggested, type: kind, spec: spec(), secrets: secrets() }),
    onSuccess: async (c) => {
      await client.invalidateQueries({ queryKey: ["checks"] });
      onDone?.();
      toast.success(t("check.created", { name: c.name }));
      navigate({ to: "/checks/$id", params: { id: String(c.id) } });
    },
  });

  const field = (k: string, label: string, placeholder: string, hint?: string, autoFocus?: boolean) => (
    <div className="space-y-1.5">
      <Label htmlFor={k}>{label}</Label>
      <Input id={k} value={f[k] ?? ""} onChange={set(k)} placeholder={placeholder} autoComplete="off" autoFocus={autoFocus} />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
  const chips = (k: string, options: { label: string; value: string }[]) => (
    <div className="flex flex-wrap gap-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => put(k, o.value)}
          className={`h-7 rounded-md border px-2 text-mini transition-colors ${f[k] === o.value ? "border-brand/40 bg-brand/10 font-medium" : "text-muted-foreground hover:bg-accent hover:text-foreground"}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
  const Icon = intents.find((i) => i.key === intent)!.icon;

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <DialogHeader>
        <button type="button" onClick={onBack} className="-ml-1 inline-flex w-fit items-center gap-1 rounded px-1 text-mini text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-3.5" />
          {t("check.otherKind")}
        </button>
        <DialogTitle className="flex items-center gap-2">
          <Icon className="size-4 text-muted-foreground" />
          {t(`check.intent.${intent}.title`)}
        </DialogTitle>
        <DialogDescription>{t(`check.intent.${intent}.description`)}</DialogDescription>
      </DialogHeader>

      {kind === "http" && (
        <>
          {field("url", t("check.url"), intent === "api" ? "https://api.example.com/health" : "https://shop.example.com", undefined, true)}
          {intent === "website" && field("keyword", t("check.keyword"), t("check.optional"), t("check.keywordHint"))}
          {intent === "api" && (
            <>
              <div className="grid grid-cols-2 gap-3">
                {field("json_path", t("check.jsonPath"), "status", t("check.jsonPathHint"))}
                {field("json_equals", t("check.jsonEquals"), "UP")}
              </div>
              <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-3">
                {field("secret_header", t("check.secretHeader"), "Authorization")}
                <SecretField id="header-secret" label={t("check.secretHeaderValue")} draft={headerSecret} onChange={(d) => (setHeaderSecret(d), setResult(null))} placeholder={t("check.optional")} />
              </div>
            </>
          )}
          <details className="group">
            <summary className="cursor-pointer text-sm text-muted-foreground select-none">{t("check.more")}</summary>
            <div className="mt-4 space-y-4">
              <div className="space-y-1.5">
                <Label>{t("check.method")}</Label>
                <ToggleGroup value={[f.method]} onValueChange={(v: string[]) => v[0] && put("method", v[0])} variant="segmented" size="sm" className="flex-wrap">
                  {methods.map((m) => (
                    <ToggleGroupItem key={m} value={m} className="font-mono text-mini">
                      {m}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="headers">{t("check.headers")}</Label>
                <Textarea id="headers" spellCheck={false} value={f.headers ?? ""} onChange={set("headers")} placeholder="X-Api-Version: 2" className="min-h-10 py-2 font-mono text-mini" aria-invalid={!parsedHeaders} />
                <p className={parsedHeaders ? "text-xs text-muted-foreground" : "text-xs text-destructive"}>{parsedHeaders ? t("check.headersHint") : t("check.headersInvalid")}</p>
              </div>
              {intent === "website" && (
                <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-3">
                  {field("secret_header", t("check.secretHeader"), "Authorization")}
                  <SecretField id="header-secret" label={t("check.secretHeaderValue")} draft={headerSecret} onChange={setHeaderSecret} placeholder={t("check.optional")} />
                </div>
              )}
              <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-3">
                {field("username", t("check.username"), t("check.optional"), t("check.usernameHint"))}
                {f.username?.trim() ? <SecretField id="basic-password" label={t("check.password")} draft={password} onChange={setPassword} /> : <div />}
              </div>
              {hasBody && (
                <div className="space-y-1.5">
                  <Label htmlFor="body">{t("check.body")}</Label>
                  <Textarea id="body" spellCheck={false} value={f.body ?? ""} onChange={set("body")} placeholder={t("check.optional")} className="min-h-10 py-2 font-mono text-mini" />
                </div>
              )}
              {intent === "website" && (
                <div className="grid grid-cols-2 gap-3">
                  {field("json_path", t("check.jsonPath"), "status", t("check.jsonPathHint"))}
                  {field("json_equals", t("check.jsonEquals"), "UP")}
                </div>
              )}
              {intent === "api" && field("keyword", t("check.keyword"), t("check.optional"), t("check.keywordHint"))}
              {field("absent", t("check.absent"), t("check.optional"), t("check.absentHint"))}
              <div className="grid grid-cols-2 gap-3">
                {field("status", t("check.status"), "200-399", t("check.statusHint"))}
                {field("max_latency", t("check.maxLatency"), "800ms", t("check.maxLatencyHint"))}
              </div>
              {field("ip", t("check.ip"), "203.0.113.10", t("check.ipHint"))}
              <div className="flex items-start justify-between gap-4">
                <div className="space-y-0.5">
                  <Label htmlFor="follow">{t("check.followRedirects")}</Label>
                  <p className="text-xs text-muted-foreground">{t("check.followRedirectsHint")}</p>
                </div>
                <Switch id="follow" checked={follow} onCheckedChange={setFollow} />
              </div>
            </div>
          </details>
        </>
      )}

      {(kind === "tls" || kind === "tcp") && (
        <>
          <div className="grid grid-cols-[minmax(0,1fr)_7rem] gap-3">
            {field("host", t("check.host"), intent === "certificate" ? "mail.example.com" : "db.internal", undefined, true)}
            {field("port", t("check.port"), intent === "certificate" ? "465" : "5432")}
          </div>
          {chips(
            "port",
            (ports[intent] ?? []).map((p) => ({ label: `${p.name} ${p.port}`, value: p.port })),
          )}
          {kind === "tls" && (
            <details>
              <summary className="cursor-pointer text-sm text-muted-foreground select-none">{t("check.more")}</summary>
              <div className="mt-4">{field("server_name", t("check.serverName"), t("check.optional"), t("check.serverNameHint"))}</div>
            </details>
          )}
        </>
      )}

      {kind === "dns" && (
        <>
          {field("name", t("check.dnsName"), "shop.example.com", undefined, true)}
          <div className="space-y-1.5">
            <Label>{t("check.recordType")}</Label>
            <ToggleGroup value={[f.type]} onValueChange={(v: string[]) => v[0] && put("type", v[0])} variant="segmented" size="sm">
              {recordTypes.map((x) => (
                <ToggleGroupItem key={x} value={x}>
                  {x}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>
          {field("expect", t("check.expect"), t("check.optional"), t("check.expectHint"))}
          {field("server", t("check.resolver"), t("check.optional"), t("check.resolverHint"))}
        </>
      )}

      {kind === "domain" && field("domain", t("check.domain"), "example.com", t("check.domainHint"), true)}

      {kind === "heartbeat" && (
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>{t("check.period")}</Label>
            {chips(
              "period",
              periods.map((p) => ({ label: t(`check.periods.${p}`), value: p })),
            )}
            <p className="text-xs text-muted-foreground">{t("check.periodHint")}</p>
          </div>
          <div className="space-y-1.5">
            <Label>{t("check.grace")}</Label>
            {chips(
              "grace",
              ["5m", "15m", "1h", "6h"].map((g) => ({ label: g, value: g })),
            )}
            <p className="text-xs text-muted-foreground">{t("check.graceHint")}</p>
          </div>
        </div>
      )}

      <div className="space-y-1.5">
        <Label htmlFor="check-name">{t("check.name")}</Label>
        <Input id="check-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={suggested || t(`check.intent.${intent}.namePlaceholder`)} />
      </div>
      {result && <TryResult result={result} />}
      {(save.isError || test.isError) && (
        <Alert variant="destructive">
          <AlertDescription>{(save.error ?? test.error)?.message}</AlertDescription>
        </Alert>
      )}
      <DialogFooter className="gap-2 sm:justify-between">
        {kind !== "heartbeat" ? (
          <Button type="button" variant="outline" disabled={!ready || test.isPending} onClick={() => (setResult(null), test.mutate())}>
            {test.isPending && <Loader2 className="size-4 animate-spin" />}
            {t("check.try")}
          </Button>
        ) : (
          <span />
        )}
        <Button type="submit" disabled={save.isPending || !ready || !(name.trim() || suggested)}>
          {t("check.create")}
        </Button>
      </DialogFooter>
    </form>
  );
}

// TryResult says how a test run went, in a line.
function TryResult({ result: r }: { result: CheckTryResult }) {
  const { t } = useTranslation();
  const days = certDays({ cert_not_after: r.cert_not_after });
  const facts = [
    r.status_code && t("check.tryStatus", { code: r.status_code }),
    r.latency_ms > 0 && (r.latency_ms < 1 ? t("check.tryFast") : t("check.tryLatency", { ms: Math.round(r.latency_ms) })),
    days !== undefined && t("check.tryCert", { count: days }),
  ].filter(Boolean);
  return (
    <div className={`flex items-start gap-2.5 rounded-lg border px-3 py-2.5 ${r.ok ? "border-[color:var(--ok)]/30 bg-[color:var(--ok)]/8" : "border-destructive/30 bg-destructive/8"}`}>
      {r.ok ? <CircleCheck className="mt-0.5 size-4 shrink-0" style={{ color: "var(--ok)" }} /> : <CircleX className="mt-0.5 size-4 shrink-0 text-destructive" />}
      <div className="min-w-0">
        <p className="font-medium">{r.ok ? t("check.tryUp") : (r.error ?? t("check.tryDown"))}</p>
        {facts.length > 0 && <p className="text-mini text-muted-foreground">{facts.join(" · ")}</p>}
        {!r.ok && <p className="text-mini text-muted-foreground">{t("check.tryHint")}</p>}
      </div>
    </div>
  );
}
