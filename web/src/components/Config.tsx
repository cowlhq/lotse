import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Undo2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getConfig, setSetting, type ConfigValue } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useMe } from "@/lib/useMe";

// Config shows an object's settings as they apply to it: the value, and
// where it comes from, the object itself, a scope above, or the default
// (principle 4: every object shows its effective configuration). The alert
// rules come last, as a table, one rule a row.
export function Config({ subject }: { subject: string }) {
  const { t } = useTranslation();
  const values = useQuery({ queryKey: ["config", subject], queryFn: () => getConfig(subject) });
  const all = values.data ?? [];
  const general = all.filter((v) => !v.key.startsWith("rules."));
  const rules = new Map<string, Record<string, ConfigValue>>();
  for (const v of all) {
    if (!v.key.startsWith("rules.")) continue;
    const parts = v.key.split(".");
    const name = parts.slice(1, -1).join(".");
    rules.set(name, { ...rules.get(name), [parts[parts.length - 1]]: v });
  }
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium">{t("config.title")}</CardTitle>
          <CardDescription>{t("config.description")}</CardDescription>
        </CardHeader>
        <CardContent className="divide-y">
          {general.map((v) => (
            <Row key={v.key} subject={subject} value={v} />
          ))}
        </CardContent>
      </Card>
      {rules.size > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">{t("config.rules")}</CardTitle>
            <CardDescription>{t("config.rulesDescription")}</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("config.rule")}</TableHead>
                  <TableHead>{t("config.enabled")}</TableHead>
                  <TableHead>{t("alert.state.warn")}</TableHead>
                  <TableHead>{t("alert.state.crit")}</TableHead>
                  <TableHead>{t("config.for")}</TableHead>
                  <TableHead>{t("config.recover")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[...rules].map(([name, f]) => (
                  <TableRow key={name} className={f.enabled?.value ? undefined : "text-muted-foreground"}>
                    <TableCell>
                      <p className="font-mono text-xs">{name}</p>
                      <p className="text-xs text-muted-foreground">{t(`rules.${name}`)}</p>
                    </TableCell>
                    <TableCell>{f.enabled && <Cell subject={subject} value={f.enabled} />}</TableCell>
                    {f.level ? (
                      <TableCell colSpan={2}>
                        <Cell subject={subject} value={f.level} />
                      </TableCell>
                    ) : (
                      <>
                        <TableCell>{f.warn && <Cell subject={subject} value={f.warn} unit={unitOf(f.warn)} />}</TableCell>
                        <TableCell>{f.crit && <Cell subject={subject} value={f.crit} unit={unitOf(f.crit)} />}</TableCell>
                      </>
                    )}
                    <TableCell>{f.for && <Cell subject={subject} value={f.for} />}</TableCell>
                    <TableCell>{f.recover && <Cell subject={subject} value={f.recover} />}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </>
  );
}

// unitOf reads a threshold's unit from its description: "… (percent), or off."
function unitOf(v: ConfigValue): string {
  return /\((\w+)\)/.exec(v.description)?.[1] ?? "";
}

// useSetting saves a setting, or unsets it with null.
function useSetting(subject: string, key: string, onSaved?: () => void) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (value: unknown) => setSetting(subject, key, value),
    onSuccess: () => {
      onSaved?.();
      client.invalidateQueries({ queryKey: ["config"] });
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });
}

function useFrom(subject: string, v: ConfigValue) {
  const { t } = useTranslation();
  const here = v.from !== undefined && `${v.from.kind}:${v.from.id}` === subject;
  let from: string;
  if (!v.from) from = t("config.default");
  else if (here) from = t("config.here");
  else from = t("config.from", { kind: t(`config.kinds.${v.from.kind}`), name: v.from.name || v.from.path });
  return { here, from };
}

// show writes a value as it is typed.
function show(v: unknown): string {
  return Array.isArray(v) ? v.join(", ") : String(v);
}

function Row({ subject, value: v }: { subject: string; value: ConfigValue }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState("");
  const { here, from } = useFrom(subject, v);
  const save = useSetting(subject, v.key, () => setEditing(false));

  return (
    <div className="grid gap-1 py-3 text-sm sm:grid-cols-[1fr_auto] sm:items-center">
      <div className="min-w-0">
        <p className="font-mono text-xs">{v.key}</p>
        <p className="text-muted-foreground">{v.description}</p>
      </div>
      {v.type === "bool" ? (
        <div className="flex items-center gap-3">
          <div className="text-right">
            <p className="font-medium">{v.value ? t("config.on") : t("config.off")}</p>
            <p className="text-xs text-muted-foreground">{from}</p>
          </div>
          {isAdmin && (
            <>
              <Button variant="outline" size="sm" disabled={save.isPending} onClick={() => save.mutate(!v.value)}>
                {v.value ? t("config.turnOff") : t("config.turnOn")}
              </Button>
              {here && <Reset onClick={() => save.mutate(null)} />}
            </>
          )}
        </div>
      ) : editing ? (
        <form
          className="flex items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate(parse(v.type, text));
          }}
        >
          <Input value={text} onChange={(e) => setText(e.target.value)} className={v.type === "url" || v.type === "channels" ? "h-8 w-64" : "h-8 w-28"} autoFocus />
          <Button type="submit" size="sm" disabled={save.isPending}>
            {t("tree.save")}
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(false)}>
            {t("tree.cancel")}
          </Button>
        </form>
      ) : (
        <div className="flex items-center gap-3">
          <div className="min-w-0 text-right">
            <p className="truncate font-medium tabular-nums">{show(v.value) || "–"}</p>
            <p className="text-xs text-muted-foreground">
              {from}
              {v.overrides.length > 0 && <> · {t("config.overrides", { value: show(v.overrides[0].value), name: v.overrides[0].level.name || t(`config.kinds.${v.overrides[0].level.kind}`) })}</>}
            </p>
          </div>
          {isAdmin && (
            <>
              <Button variant="ghost" size="icon-sm" aria-label={t("config.set")} onClick={() => (setText(show(v.value)), setEditing(true))}>
                <Pencil className="size-3.5" />
              </Button>
              {here && <Reset onClick={() => save.mutate(null)} />}
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

function Reset({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <Button variant="ghost" size="icon-sm" aria-label={t("config.reset")} title={t("config.reset")} onClick={onClick}>
      <Undo2 className="size-3.5" />
    </Button>
  );
}

// Cell is one setting of a rule: its value, set here in bold, and where it
// comes from on hover; a click to change it.
function Cell({ subject, value: v, unit }: { subject: string; value: ConfigValue; unit?: string }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState("");
  const { here, from } = useFrom(subject, v);
  const save = useSetting(subject, v.key, () => setEditing(false));
  const label =
    v.type === "bool" ? (v.value ? t("config.on") : t("config.off")) : v.type === "level" ? t(`alert.state.${v.value}`) : v.value === "off" ? t("config.off") : `${show(v.value)}${unit === "percent" ? " %" : unit === "days" ? " d" : ""}`;
  const error = save.isError && <p className="text-xs text-destructive">{save.error.message}</p>;

  if (editing) {
    return (
      <form
        className="flex items-center gap-1"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate(parse(v.type, text));
        }}
      >
        <Input value={text} onChange={(e) => setText(e.target.value)} className="h-7 w-20" autoFocus onKeyDown={(e) => e.key === "Escape" && setEditing(false)} />
        <Button type="submit" size="sm" variant="outline" disabled={save.isPending}>
          {t("tree.save")}
        </Button>
        {error}
      </form>
    );
  }
  const change = () => {
    if (v.type === "bool") save.mutate(!v.value);
    else if (v.type === "level") save.mutate(v.value === "crit" ? "warn" : "crit");
    else {
      setText(show(v.value));
      setEditing(true);
    }
  };
  return (
    <span className="inline-flex items-center gap-1 whitespace-nowrap">
      {isAdmin ? (
        <button type="button" className={`tabular-nums underline-offset-4 hover:underline ${here ? "font-semibold" : ""}`} title={from} onClick={change} disabled={save.isPending}>
          {label}
        </button>
      ) : (
        <span className={`tabular-nums ${here ? "font-semibold" : ""}`} title={from}>
          {label}
        </span>
      )}
      {isAdmin && here && <Reset onClick={() => save.mutate(null)} />}
      {error}
    </span>
  );
}

// parse turns what was typed into the value a setting takes: a list for
// probes and channels, a number or "off" for thresholds, the text as it is
// otherwise.
function parse(type: ConfigValue["type"], text: string): unknown {
  const trimmed = text.trim();
  if (type === "probes" || type === "channels") {
    const names = trimmed.split(/[\s,]+/).filter(Boolean);
    return type === "probes" && names.length === 1 && names[0] === "all" ? "all" : names;
  }
  if (type === "threshold") {
    const n = Number(trimmed.replace(",", "."));
    return trimmed === "" || Number.isNaN(n) ? trimmed : n;
  }
  return trimmed;
}
