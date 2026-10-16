import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { deleteMailServer, getMailServer, NotFound, setMailServer, testMailServer, type MailServer as MailServerData, type TestResult } from "@/api";
import { TLSPicker } from "@/components/Channels";
import { emptySecret, SecretField, secretInput, type SecretDraft } from "@/components/Secret";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Panel } from "@/components/shell/Panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useMe } from "@/lib/useMe";

// MailServer is the SMTP server email channels send through, unless one
// names its own.
export function MailServer() {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const server = useQuery({
    queryKey: ["mail-server"],
    queryFn: () => getMailServer().catch((e) => (e instanceof NotFound ? null : Promise.reject(e))),
  });
  const [editing, setEditing] = useState(false);
  if (server.isPending) return null;
  const m = server.data ?? null;
  return (
    <Panel
      title={t("mail.title")}
      right={
        isAdmin &&
        !editing && (
          <>
            {m && <TestMail />}
            <Button size="sm" variant="outline" onClick={() => setEditing(true)}>
              {m ? t("mail.edit") : t("mail.set")}
            </Button>
          </>
        )
      }
      flush
    >
      <p className="border-b px-4 py-3 text-muted-foreground">{t("mail.description")}</p>
      <div className="px-4 py-3">
        {editing ? (
          <Form server={m} onDone={() => setEditing(false)} />
        ) : m ? (
          <p>
            <span className="font-medium">
              {m.host}:{m.port}
            </span>
            <span className="text-muted-foreground">
              {" "}
              · {t(`mail.tlsMode.${m.tls}`)} · {m.from}
              {m.username && <> · {m.username}</>}
            </span>
          </p>
        ) : (
          <p className="text-muted-foreground">{t("mail.none")}</p>
        )}
      </div>
    </Panel>
  );
}

function Form({ server: m, onDone }: { server: MailServerData | null; onDone: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [f, setF] = useState({
    host: m?.host ?? "",
    port: m ? String(m.port) : "587",
    username: m?.username ?? "",
    from: m?.from ?? "",
    tls: m?.tls ?? ("starttls" as MailServerData["tls"]),
  });
  const [password, setPassword] = useState<SecretDraft>(emptySecret);
  const done = async () => {
    toast.success(t("mail.saved"));
    await client.invalidateQueries({ queryKey: ["mail-server"] });
    client.invalidateQueries({ queryKey: ["changes"] });
    onDone();
  };
  const save = useMutation({
    mutationFn: () =>
      setMailServer({ host: f.host.trim(), port: Number(f.port) || 0, username: f.username.trim(), from: f.from.trim(), tls: f.tls, password: secretInput(password) }),
    onSuccess: done,
  });
  const remove = useMutation({ mutationFn: deleteMailServer, onSuccess: done });
  const field = (k: "host" | "port" | "username" | "from", label: string, placeholder: string) => (
    <div className="space-y-1.5">
      <Label htmlFor={`mail-${k}`}>{label}</Label>
      <Input id={`mail-${k}`} value={f[k]} onChange={(e) => setF({ ...f, [k]: e.target.value })} placeholder={placeholder} autoComplete="off" />
    </div>
  );
  return (
    <form
      className="max-w-lg space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <div className="grid grid-cols-[1fr_6rem] gap-3">
        {field("host", t("mail.host"), "smtp.example.com")}
        {field("port", t("mail.port"), "587")}
      </div>
      <TLSPicker value={f.tls} onChange={(tls) => setF({ ...f, tls, port: tls === "tls" && f.port === "587" ? "465" : tls !== "tls" && f.port === "465" ? "587" : f.port })} />
      {field("from", t("mail.from"), "lotse <lotse@example.com>")}
      {field("username", t("mail.username"), t("check.optional"))}
      <SecretField id="mail-password" label={t("mail.password")} info={m?.password} draft={password} onChange={setPassword} />
      {(save.isError || remove.isError) && (
        <Alert variant="destructive">
          <AlertDescription>{(save.error ?? remove.error)?.message}</AlertDescription>
        </Alert>
      )}
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={save.isPending}>
          {t("tree.save")}
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          {t("tree.cancel")}
        </Button>
        {m && (
          <Button type="button" size="sm" variant="ghost" className="ml-auto text-destructive" onClick={() => confirm(t("mail.removeConfirm")) && remove.mutate()}>
            {t("mail.remove")}
          </Button>
        )}
      </div>
    </form>
  );
}

function TestMail() {
  const { t } = useTranslation();
  const [to, setTo] = useState("");
  const [result, setResult] = useState<TestResult | null>(null);
  const test = useMutation({ mutationFn: () => testMailServer(to.trim()), onSuccess: setResult });
  return (
    <form
      className="flex flex-wrap items-center gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        setResult(null);
        test.mutate();
      }}
    >
      <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="you@example.com" className="h-8 w-56" type="email" />
      <Button type="submit" size="sm" variant="outline" disabled={test.isPending || !to.trim()}>
        {t("mail.test")}
      </Button>
      {result && <span className={result.sent ? "text-muted-foreground" : "text-destructive"}>{result.sent ? t("mail.testSent") : result.error}</span>}
      {test.isError && <span className="text-destructive">{test.error.message}</span>}
    </form>
  );
}
