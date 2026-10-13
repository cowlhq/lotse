import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Undo2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getConfig, setSetting, type ConfigValue } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useMe } from "@/lib/useMe";

// Config shows an object's settings as they apply to it: the value, and
// where it comes from, the object itself, a scope above, or the default
// (principle 4: every object shows its effective configuration).
export function Config({ subject }: { subject: string }) {
  const { t } = useTranslation();
  const values = useQuery({ queryKey: ["config", subject], queryFn: () => getConfig(subject) });
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("config.title")}</CardTitle>
        <CardDescription>{t("config.description")}</CardDescription>
      </CardHeader>
      <CardContent className="divide-y">
        {values.data?.map((v) => <Row key={v.key} subject={subject} value={v} />)}
      </CardContent>
    </Card>
  );
}

function Row({ subject, value: v }: { subject: string; value: ConfigValue }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState("");
  const here = v.from !== undefined && `${v.from.kind}:${v.from.id}` === subject;
  const save = useMutation({
    mutationFn: (value: unknown) => setSetting(subject, v.key, value),
    onSuccess: () => {
      setEditing(false);
      client.invalidateQueries({ queryKey: ["config"] });
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });

  let from: string;
  if (!v.from) from = t("config.default");
  else if (here) from = t("config.here");
  else from = t("config.from", { kind: t(`config.kinds.${v.from.kind}`), name: v.from.name || v.from.path });

  return (
    <div className="grid gap-1 py-3 text-sm sm:grid-cols-[1fr_auto] sm:items-center">
      <div className="min-w-0">
        <p className="font-mono text-xs">{v.key}</p>
        <p className="text-muted-foreground">{v.description}</p>
      </div>
      {editing ? (
        <form
          className="flex items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate(text);
          }}
        >
          <Input value={text} onChange={(e) => setText(e.target.value)} className="h-8 w-28" autoFocus />
          <Button type="submit" size="sm" disabled={save.isPending}>
            {t("tree.save")}
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(false)}>
            {t("tree.cancel")}
          </Button>
        </form>
      ) : (
        <div className="flex items-center gap-3">
          <div className="text-right">
            <p className="font-medium tabular-nums">{String(v.value)}</p>
            <p className="text-xs text-muted-foreground">
              {from}
              {v.overrides.length > 0 && <> · {t("config.overrides", { value: String(v.overrides[0].value), name: v.overrides[0].level.name || t(`config.kinds.${v.overrides[0].level.kind}`) })}</>}
            </p>
          </div>
          {isAdmin && (
            <>
              <Button variant="ghost" size="icon-sm" aria-label={t("config.set")} onClick={() => (setText(String(v.value)), setEditing(true))}>
                <Pencil className="size-3.5" />
              </Button>
              {here && (
                <Button variant="ghost" size="icon-sm" aria-label={t("config.reset")} title={t("config.reset")} onClick={() => save.mutate(null)}>
                  <Undo2 className="size-3.5" />
                </Button>
              )}
            </>
          )}
        </div>
      )}
      {save.isError && (
        <Alert variant="destructive" className="sm:col-span-2">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}
