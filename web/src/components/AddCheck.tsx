import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { createCheck, type Check } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

type Kind = Check["type"];
const kinds: Kind[] = ["http", "tls", "tcp", "dns", "domain", "heartbeat"];
const recordTypes = ["A", "AAAA", "CNAME", "MX", "NS", "TXT"];
const methods = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"];

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

// AddCheck adds a check to an environment: its kind, a name, and what to
// look at.
export function AddCheck({ envID }: { envID: number }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <Plus className="size-4" />
        {t("check.add")}
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl">{open && <CheckForm envID={envID} onDone={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

export function CheckForm({ envID, onDone }: { envID: number; onDone?: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [kind, setKind] = useState<Kind>("http");
  const [name, setName] = useState("");
  const [f, setF] = useState<Record<string, string>>({ period: "24h", grace: "1h", type: "A", method: "GET" });
  const [follow, setFollow] = useState(true);
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setF({ ...f, [k]: e.target.value });
  const hasBody = !["GET", "HEAD"].includes(f.method);
  const parsedHeaders = headers(f.headers ?? "");

  const spec = (): Record<string, unknown> => {
    const pick = (...keys: string[]) => Object.fromEntries(keys.filter((k) => f[k]?.trim()).map((k) => [k, f[k].trim()]));
    if (kind === "http")
      return {
        ...pick("url", "method", "ip", "keyword", "absent", "max_latency", "status"),
        ...(hasBody && f.body ? { body: f.body } : {}),
        ...(parsedHeaders && Object.keys(parsedHeaders).length ? { headers: parsedHeaders } : {}),
        ...(f.json_path?.trim() ? { json: { path: f.json_path.trim(), equals: f.json_equals ?? "" } } : {}),
        ...(follow ? {} : { follow_redirects: false }),
      };
    if (kind === "tls") return pick("address", "server_name");
    if (kind === "tcp") return pick("address");
    if (kind === "dns") return { ...pick("name", "type", "server"), expect: (f.expect ?? "").split(/[\s,]+/).filter(Boolean) };
    if (kind === "domain") return pick("domain");
    return pick("period", "grace");
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
    return f.address ?? "";
  })();
  const save = useMutation({
    mutationFn: () => createCheck({ environment_id: envID, name: name.trim() || suggested, type: kind, spec: spec() }),
    onSuccess: async (c) => {
      await client.invalidateQueries({ queryKey: ["checks"] });
      onDone?.();
      toast.success(t("check.created", { name: c.name }));
      navigate({ to: "/checks/$id", params: { id: String(c.id) } });
    },
  });

  const field = (k: string, label: string, placeholder: string, hint?: string) => (
    <div className="space-y-1.5">
      <Label htmlFor={k}>{label}</Label>
      <Input id={k} value={f[k] ?? ""} onChange={set(k)} placeholder={placeholder} autoComplete="off" />
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <DialogHeader>
        <DialogTitle>{t("check.add")}</DialogTitle>
        <DialogDescription>{t(`check.kindHint.${kind}`)}</DialogDescription>
      </DialogHeader>
      <ToggleGroup value={[kind]} onValueChange={(v: string[]) => v[0] && setKind(v[0] as Kind)} variant="segmented" size="sm" className="grid w-full grid-cols-3 sm:grid-cols-6">
        {kinds.map((k) => (
          <ToggleGroupItem key={k} value={k} className="px-1">
            {t(`check.kind.${k}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {kind === "http" && (
        <>
          {field("url", t("check.url"), "https://shop.example.com/health")}
          {field("keyword", t("check.keyword"), t("check.optional"), t("check.keywordHint"))}
          {field("status", t("check.status"), "200-399", t("check.statusHint"))}
          <details className="group space-y-4">
            <summary className="cursor-pointer text-sm text-muted-foreground select-none">{t("check.more")}</summary>
            <div className="mt-4 space-y-4">
              <div className="space-y-1.5">
                <Label>{t("check.method")}</Label>
                <ToggleGroup value={[f.method]} onValueChange={(v: string[]) => v[0] && setF({ ...f, method: v[0] })} variant="segmented" size="sm" className="flex-wrap">
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
              {hasBody && (
                <div className="space-y-1.5">
                  <Label htmlFor="body">{t("check.body")}</Label>
                  <Textarea id="body" spellCheck={false} value={f.body ?? ""} onChange={set("body")} placeholder={t("check.optional")} className="min-h-10 py-2 font-mono text-mini" />
                </div>
              )}
              <div className="grid grid-cols-2 gap-3">
                {field("json_path", t("check.jsonPath"), "status", t("check.jsonPathHint"))}
                {field("json_equals", t("check.jsonEquals"), "UP")}
              </div>
              {field("absent", t("check.absent"), t("check.optional"), t("check.absentHint"))}
              <div className="grid grid-cols-2 gap-3">
                {field("max_latency", t("check.maxLatency"), "800ms", t("check.maxLatencyHint"))}
                {field("ip", t("check.ip"), "203.0.113.10", t("check.ipHint"))}
              </div>
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
      {(kind === "tls" || kind === "tcp") && field("address", t("check.address"), kind === "tls" ? "mail.example.com:465" : "db.internal:5432")}
      {kind === "dns" && (
        <>
          {field("name", t("check.dnsName"), "shop.example.com")}
          <div className="space-y-1.5">
            <Label>{t("check.recordType")}</Label>
            <ToggleGroup value={[f.type]} onValueChange={(v: string[]) => v[0] && setF({ ...f, type: v[0] })} variant="segmented" size="sm">
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
      {kind === "domain" && field("domain", t("check.domain"), "example.com", t("check.domainHint"))}
      {kind === "heartbeat" && (
        <div className="grid grid-cols-2 gap-3">
          {field("period", t("check.period"), "24h", t("check.periodHint"))}
          {field("grace", t("check.grace"), "1h", t("check.graceHint"))}
        </div>
      )}
      <div className="space-y-1.5">
        <Label htmlFor="check-name">{t("check.name")}</Label>
        <Input id="check-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={suggested || t("check.namePlaceholder")} />
      </div>
      {save.isError && (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
      <DialogFooter>
        <Button type="submit" disabled={save.isPending || !(name.trim() || suggested) || (kind === "http" && !parsedHeaders)}>
          {t("check.create")}
        </Button>
      </DialogFooter>
    </form>
  );
}
