import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { checkSetup, getHealth, setupAdmin, setupDatabase } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Centered } from "@/components/shell/Centered";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";

// Setup, with the one-time token from the hub's log: first the database
// (unless LOTSE_DATABASE_URL chose it), then the admin, who is logged in
// right away.
export function Setup({ token }: { token: string }) {
  const { t } = useTranslation();
  const check = useQuery({ queryKey: ["setup", token], queryFn: () => checkSetup(token), retry: false });

  let body;
  if (!token) body = <NoSetupToken />;
  else if (check.isPending) body = <p className="text-muted-foreground">{t("setup.checking")}</p>;
  else if (check.isError) body = <p>{t("setup.done")}</p>;
  else if (!check.data.valid) body = <p>{t("setup.invalid")}</p>;
  else if (check.data.step === "database") body = <DatabaseStep token={token} postgres={check.data.postgres} />;
  else body = <AdminStep token={token} />;

  return (
    <Centered wide title={t("setup.title")} subtitle={check.data?.valid ? t("setup.step", { n: check.data.step === "database" ? 1 : 2, of: 2 }) : undefined}>
      {body}
    </Centered>
  );
}

function DatabaseStep({ token, postgres }: { token: string; postgres?: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [kind, setKind] = useState<"sqlite" | "postgres">("sqlite");
  const [url, setURL] = useState(postgres ?? "");
  const save = useMutation({
    mutationFn: async () => {
      await setupDatabase(token, kind, kind === "postgres" ? url : undefined);
      // The hub opens the database now; wait until it is at the next step.
      for (let i = 0; i < 60; i++) {
        const h = await getHealth().catch(() => null);
        if (h?.setup === "admin") return;
        await new Promise((r) => setTimeout(r, 500));
      }
    },
    onSuccess: () => client.invalidateQueries(),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <div className="space-y-5">
      <div className="space-y-1">
        <h2 className="font-semibold">{t("setup.database.title")}</h2>
        <p className="text-muted-foreground">{t("setup.database.description")}</p>
      </div>
      <div>
        <form onSubmit={submit} className="space-y-5">
          <RadioGroup value={kind} onValueChange={(v) => setKind(v as "sqlite" | "postgres")}>
            <Label className="flex items-start gap-3 font-normal">
              <RadioGroupItem value="sqlite" className="mt-0.5" />
              <span className="space-y-0.5">
                <span className="block font-medium">{t("setup.database.sqlite")}</span>
                <span className="block text-muted-foreground">{t("setup.database.sqliteHint")}</span>
              </span>
            </Label>
            <Label className="flex items-start gap-3 font-normal">
              <RadioGroupItem value="postgres" className="mt-0.5" />
              <span className="space-y-0.5">
                <span className="block font-medium">{t("setup.database.postgres")}</span>
                <span className="block text-muted-foreground">
                  {postgres ? t("setup.database.postgresFound") : t("setup.database.postgresHint")}
                </span>
              </span>
            </Label>
          </RadioGroup>
          {kind === "postgres" && (
            <div className="space-y-2">
              <Label htmlFor="db-url">{t("setup.database.url")}</Label>
              <Input
                id="db-url"
                value={url}
                onChange={(e) => setURL(e.target.value)}
                placeholder="postgres://user:password@host:5432/lotse"
                autoComplete="off"
              />
            </div>
          )}
          {save.isError && (
            <Alert variant="destructive">
              <AlertDescription>{save.error.message}</AlertDescription>
            </Alert>
          )}
          <Button type="submit" disabled={save.isPending || (kind === "postgres" && !url)}>
            {save.isPending ? t("setup.database.testing") : t("setup.continue")}
          </Button>
        </form>
      </div>
    </div>
  );
}

function AdminStep({ token }: { token: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [form, setForm] = useState({ name: "", email: "", password: "", url: location.origin });
  const save = useMutation({
    mutationFn: () => setupAdmin(token, form),
    onSuccess: async () => {
      await client.invalidateQueries();
      navigate({ to: "/" });
    },
  });
  const field = (key: keyof typeof form, type = "text", autoComplete?: string) => (
    <div className="space-y-2">
      <Label htmlFor={key}>{t(`setup.admin.${key}`)}</Label>
      <Input
        id={key}
        type={type}
        value={form[key]}
        autoComplete={autoComplete}
        onChange={(e) => setForm({ ...form, [key]: e.target.value })}
        required
      />
    </div>
  );

  return (
    <div className="space-y-5">
      <div className="space-y-1">
        <h2 className="font-semibold">{t("setup.admin.title")}</h2>
        <p className="text-muted-foreground">{t("setup.admin.description")}</p>
      </div>
      <div>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
          className="space-y-4"
        >
          {field("name", "text", "name")}
          {field("email", "email", "email")}
          {field("password", "password", "new-password")}
          <p className="-mt-2 text-xs text-muted-foreground">{t("setup.admin.passwordHint")}</p>
          {field("url", "url")}
          <p className="-mt-2 text-xs text-muted-foreground">{t("setup.admin.urlHint")}</p>
          {save.isError && (
            <Alert variant="destructive">
              <AlertDescription>{save.error.message}</AlertDescription>
            </Alert>
          )}
          <Button type="submit" disabled={save.isPending}>
            {t("setup.admin.finish")}
          </Button>
        </form>
      </div>
    </div>
  );
}

// NoSetupToken says where the setup link is, for whoever opens the hub
// without it.
export function NoSetupToken() {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <p>{t("setup.noToken")}</p>
      <pre className="rounded-md border bg-muted/60 px-3 py-2 font-mono text-mini whitespace-pre-wrap [overflow-wrap:anywhere]">sudo docker compose -f /opt/lotse/compose.yml logs hub</pre>
    </div>
  );
}
