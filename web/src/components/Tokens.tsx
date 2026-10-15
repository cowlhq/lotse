import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Download, KeyRound, Plus } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { createToken, exportURL, listScopes, listTokens, revokeToken, type NewToken, type Token } from "@/api";
import { Panel } from "@/components/shell/Panel";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ago, dateTime } from "@/lib/format";

// Tokens are for lotse apply and export from a pipeline, and for reading
// through the API: each for everything, or for one customer, project or
// environment.
export function Tokens() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const tokens = useQuery({ queryKey: ["tokens"], queryFn: listTokens });
  const revoke = useMutation({
    mutationFn: revokeToken,
    onSuccess: () => (client.invalidateQueries({ queryKey: ["tokens"] }), toast.success(t("token.revoked"))),
  });
  const live = (tokens.data ?? []).filter((x) => !x.revoked);
  return (
    <>
      <Panel title={t("token.title")} count={live.length} right={<NewTokenDialog />} flush>
        <p className="border-b px-4 py-3 text-muted-foreground">{t("token.description")}</p>
        {live.length === 0 ? (
          <p className="px-4 py-8 text-center text-muted-foreground">{t("token.none")}</p>
        ) : (
          <ul>
            {live.map((x) => (
              <li key={x.id} className="flex items-center gap-3 border-b px-4 py-3 last:border-b-0">
                <KeyRound className="size-4 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <p className="font-medium">{x.name}</p>
                  <p className="text-mini text-muted-foreground">
                    {t(`token.kind.${x.kind}`)} · {x.scope_path || t("token.everything")} · {t("token.by", { who: x.created_by, time: dateTime(x.created_at) })}
                    {" · "}
                    {x.last_used_at ? t("token.used", { time: ago(x.last_used_at) }) : t("token.unused")}
                  </p>
                </div>
                <Button variant="ghost" size="sm" disabled={revoke.isPending} onClick={() => confirm(t("token.revokeConfirm", { name: x.name })) && revoke.mutate(x.id)}>
                  {t("token.revoke")}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Panel>
      <Panel title={t("token.exportTitle")}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-muted-foreground">{t("token.exportHint")}</p>
          <Button variant="outline" size="sm" render={<a href={exportURL("")} download="lotse.yaml" />}>
            <Download />
            {t("token.download")}
          </Button>
        </div>
      </Panel>
    </>
  );
}

function NewTokenDialog() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <Plus />
        {t("token.new")}
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl">{open && <NewTokenForm />}</DialogContent>
    </Dialog>
  );
}

function NewTokenForm() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const [name, setName] = useState("");
  const [kind, setKind] = useState<Token["kind"]>("apply");
  const [scope, setScope] = useState(1);
  const [made, setMade] = useState<NewToken | null>(null);
  const save = useMutation({
    mutationFn: () => createToken(name.trim(), kind, scope),
    onSuccess: (x) => {
      setMade(x);
      client.invalidateQueries({ queryKey: ["tokens"] });
    },
  });
  if (made) return <Made token={made} />;
  const depth = (path: string) => path.split("/").length - 1;
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <DialogHeader>
        <DialogTitle>{t("token.new")}</DialogTitle>
        <DialogDescription>{t("token.newHint")}</DialogDescription>
      </DialogHeader>
      <div className="space-y-1.5">
        <Label htmlFor="token-name">{t("token.name")}</Label>
        <Input id="token-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("token.namePlaceholder")} autoFocus />
      </div>
      <div className="space-y-1.5">
        <Label>{t("token.what")}</Label>
        <ToggleGroup value={[kind]} onValueChange={(v: string[]) => v[0] && setKind(v[0] as Token["kind"])} variant="segmented" size="sm">
          <ToggleGroupItem value="apply">{t("token.kind.apply")}</ToggleGroupItem>
          <ToggleGroupItem value="read">{t("token.kind.read")}</ToggleGroupItem>
        </ToggleGroup>
        <p className="text-mini text-muted-foreground">{t(`token.kindHint.${kind}`)}</p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="token-scope">{t("token.where")}</Label>
        <select id="token-scope" value={scope} onChange={(e) => setScope(Number(e.target.value))} className="h-9 w-full rounded-md border border-input bg-background px-2.5 shadow-xs dark:bg-white/[0.02]">
          <option value={1}>{t("token.everything")}</option>
          {[...(scopes.data ?? [])]
            .sort((a, b) => a.path.localeCompare(b.path))
            .map((x) => (
              <option key={x.id} value={x.id}>
                {" ".repeat(depth(x.path))}
                {x.name} ({x.path})
              </option>
            ))}
        </select>
      </div>
      {save.isError && (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
      <div className="flex justify-end">
        <Button type="submit" disabled={save.isPending || !name.trim()}>
          {t("token.create")}
        </Button>
      </div>
    </form>
  );
}

// Made shows a new token once, with how to use it.
function Made({ token }: { token: NewToken }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState("");
  const copy = (text: string) => {
    navigator.clipboard?.writeText(text);
    setCopied(text);
    setTimeout(() => setCopied(""), 1500);
  };
  const hub = location.origin;
  // A token goes in the clear only when told to (decision 0006).
  const plain = location.protocol === "http:" && !["localhost", "127.0.0.1"].includes(location.hostname);
  const lines = [`export LOTSE_HUB=${hub}`, `export LOTSE_TOKEN=${token.secret}`, ...(plain ? ["export LOTSE_INSECURE=true"] : []), token.token.kind === "apply" ? "lotse apply -f lotse.yaml --dry-run" : `lotse export ${token.token.scope_path}`.trim()];
  const text = lines.join("\n");
  return (
    <div className="space-y-4">
      <DialogHeader>
        <DialogTitle>{t("token.madeTitle", { name: token.token.name })}</DialogTitle>
        <DialogDescription>{t("token.madeHint")}</DialogDescription>
      </DialogHeader>
      <div className="relative">
        <pre className="overflow-auto rounded-lg border bg-muted/60 p-3 pr-12 font-mono text-mini leading-relaxed whitespace-pre-wrap break-all">{text}</pre>
        <Button variant="ghost" size="icon-sm" className="absolute top-2 right-2" aria-label={t("agent.copy")} onClick={() => copy(text)}>
          {copied === text ? <Check /> : <Copy />}
        </Button>
      </div>
    </div>
  );
}
