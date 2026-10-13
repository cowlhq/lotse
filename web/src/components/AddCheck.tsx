import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { createCheck, type Check } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

type Kind = Check["type"];
const kinds: Kind[] = ["http", "tls", "tcp", "heartbeat"];

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
      <DialogContent>{open && <Form envID={envID} />}</DialogContent>
    </Dialog>
  );
}

function Form({ envID }: { envID: number }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [kind, setKind] = useState<Kind>("http");
  const [name, setName] = useState("");
  const [f, setF] = useState<Record<string, string>>({ period: "24h", grace: "1h" });
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value });

  const spec = (): Record<string, string> => {
    const pick = (...keys: string[]) => Object.fromEntries(keys.filter((k) => f[k]).map((k) => [k, f[k].trim()]));
    if (kind === "http") return pick("url", "keyword", "status");
    if (kind === "tls") return pick("address", "server_name");
    if (kind === "tcp") return pick("address");
    return pick("period", "grace");
  };
  // A name from what is checked, until one is typed.
  const suggested = (() => {
    try {
      if (kind === "http" && f.url) return new URL(f.url).host;
    } catch {
      /* not a URL yet */
    }
    return f.address ?? "";
  })();
  const save = useMutation({
    mutationFn: () => createCheck({ environment_id: envID, name: name.trim() || suggested, type: kind, spec: spec() }),
    onSuccess: async (c) => {
      await client.invalidateQueries({ queryKey: ["checks"] });
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
      <ToggleGroup value={[kind]} onValueChange={(v: string[]) => v[0] && setKind(v[0] as Kind)} variant="outline" size="sm">
        {kinds.map((k) => (
          <ToggleGroupItem key={k} value={k}>
            {t(`check.kind.${k}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {kind === "http" && (
        <>
          {field("url", t("check.url"), "https://shop.example.com/health")}
          {field("keyword", t("check.keyword"), t("check.optional"), t("check.keywordHint"))}
          {field("status", t("check.status"), "200-399", t("check.statusHint"))}
        </>
      )}
      {(kind === "tls" || kind === "tcp") && field("address", t("check.address"), kind === "tls" ? "mail.example.com:465" : "db.internal:5432")}
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
        <Button type="submit" disabled={save.isPending || !(name.trim() || suggested)}>
          {t("check.create")}
        </Button>
      </DialogFooter>
    </form>
  );
}
