import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Undo2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getConfig, setSetting, type ConfigValue } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Panel } from "@/components/shell/Panel";
import { Rule } from "@/components/Rules";
import { ChannelsEditor, DurationEditor, ProbesEditor, TagsEditor, WindowEditor, wide } from "@/components/SettingEditors";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { useMe } from "@/lib/useMe";

// Config shows an object's settings as they apply to it: the value, and
// where it comes from, the object itself, a scope above, or the default
// (principle 4: every object shows its effective configuration). The alert
// rules come last, as a table, one rule a row.
export function Config({ subject, part, envID }: { subject: string; part?: "general" | "rules"; envID?: number }) {
  const { t } = useTranslation();
  const values = useQuery({ queryKey: ["config", subject], queryFn: () => getConfig(subject) });
  const all = values.data ?? [];
  // A scope's notifications have a tab of their own, with its channels.
  const scope = /^(global|customer|project|environment):/.test(subject);
  const general = all.filter((v) => !v.key.startsWith("rules.") && !(scope && v.key.startsWith("notify.")));
  const rules = new Map<string, Record<string, ConfigValue>>();
  for (const v of all) {
    if (!v.key.startsWith("rules.")) continue;
    const parts = v.key.split(".");
    const name = parts.slice(1, -1).join(".");
    rules.set(name, { ...rules.get(name), [parts[parts.length - 1]]: v });
  }
  return (
    <>
      {part !== "rules" && general.length > 0 && (
        <div className="space-y-4">
          <p className="text-muted-foreground">{t("config.description")}</p>
          {groups.map((g) => {
            const list = general.filter((v) => g.match(v.key));
            return (
              list.length > 0 && (
                <Panel key={g.key} title={t(`config.groups.${g.key}`)} flush>
                  <div className="divide-y">
                    {list.map((v) => (
                      <Row key={v.key} subject={subject} value={v} envID={envID} />
                    ))}
                  </div>
                </Panel>
              )
            );
          })}
        </div>
      )}
      {part !== "general" && rules.size > 0 && (
        <div className="space-y-4">
          <p className="text-muted-foreground">{t("config.rulesDescription")}</p>
          {ruleGroups.map((g) => {
            const names = [...rules.keys()].filter((n) => n.startsWith(g + "."));
            return (
              names.length > 0 && (
                <Panel key={g} title={t(`rule.groups.${g}`)} flush>
                  <ul className="divide-y">
                    {names.map((n) => (
                      <Rule key={n} subject={subject} rule={n} values={all} className="px-4 py-3.5" />
                    ))}
                  </ul>
                </Panel>
              )
            );
          })}
        </div>
      )}
    </>
  );
}

// ruleGroups are what rules watch, in the order they are shown.
const ruleGroups = ["host", "container", "check", "probe", "channel"];

// groups order the settings by what they are about. A setting that fits
// none is not left out: the last group takes the rest.
const groups: { key: string; match: (key: string) => boolean }[] = [
  { key: "agent", match: (k) => k.startsWith("agent.") },
  { key: "host", match: (k) => k.startsWith("host.") },
  { key: "containers", match: (k) => k === "ignore" },
  { key: "checks", match: (k) => k.startsWith("check.") },
  { key: "notify", match: (k) => k.startsWith("notify.") },
  { key: "deadman", match: (k) => k.startsWith("deadman.") },
  { key: "retention", match: (k) => k.startsWith("retention.") },
  { key: "other", match: (k) => !/^(agent\.|host\.|check\.|notify\.|deadman\.|retention\.|ignore$)/.test(k) },
];

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

function Row({ subject, value: v, envID }: { subject: string; value: ConfigValue; envID?: number }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState("");
  const { here, from } = useFrom(subject, v);
  const save = useSetting(subject, v.key, () => setEditing(false));
  const disabled = !isAdmin || save.isPending;
  const label = t(`config.keys.${v.key}`, { defaultValue: v.key });
  const props = { value: v, onChange: (x: unknown) => save.mutate(x), disabled };

  let control;
  switch (v.type) {
    case "bool":
      control = <Switch checked={Boolean(v.value)} disabled={disabled} onCheckedChange={(on) => save.mutate(on)} aria-label={label} />;
      break;
    case "duration":
      control = <DurationEditor {...props} />;
      break;
    case "window":
      control = <WindowEditor key={String(v.value)} {...props} />;
      break;
    case "probes":
      control = <ProbesEditor {...props} />;
      break;
    case "channels":
      control = <ChannelsEditor {...props} envID={envID} />;
      break;
    case "services":
      control = <TagsEditor {...props} placeholder={v.key === "host.ignore_units" ? "systemd-networkd-wait-online.service" : "nginx.service"} />;
      break;
    default:
      control = editing ? (
        <form
          className="flex items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate(parse(v.type, text));
          }}
        >
          <Input value={text} onChange={(e) => setText(e.target.value)} className="h-8 w-64" autoFocus onKeyDown={(e) => e.key === "Escape" && setEditing(false)} />
          <Button type="submit" size="sm" disabled={save.isPending}>
            {t("tree.save")}
          </Button>
        </form>
      ) : (
        <span className="flex items-center gap-1">
          <span className="max-w-64 truncate font-medium">{show(v.value) || "–"}</span>
          {isAdmin && (
            <Button variant="ghost" size="icon-sm" aria-label={t("config.set")} onClick={() => (setText(show(v.value)), setEditing(true))}>
              <Pencil className="size-3.5" />
            </Button>
          )}
        </span>
      );
  }
  const origin = (
    <p className="text-xs text-muted-foreground sm:text-right">
      {from}
      {v.overrides.length > 0 && <> · {t("config.overrides", { value: show(v.overrides[0].value), name: v.overrides[0].level.name || t(`config.kinds.${v.overrides[0].level.kind}`) })}</>}
    </p>
  );
  return (
    <div className={`grid gap-x-6 gap-y-2 px-4 py-3 ${wide(v.type) ? "" : "sm:grid-cols-[1fr_auto] sm:items-center"}`}>
      <div className="flex min-w-0 items-start gap-2">
        <div className="min-w-0 flex-1">
          <p className="font-medium">
            {label}
            {/* The key, as lotse.yaml and the API name it. */}
            <span className="ml-2 hidden font-mono text-mini font-normal text-muted-foreground/70 sm:inline">{v.key}</span>
          </p>
          <p className="text-mini text-muted-foreground">{v.description}</p>
        </div>
        {wide(v.type) && (
          <span className="flex shrink-0 items-center gap-1">
            {origin}
            {isAdmin && here && <Reset onClick={() => save.mutate(null)} />}
          </span>
        )}
      </div>
      {wide(v.type) ? (
        control
      ) : (
        <div className="flex items-center gap-2 sm:justify-end">
          <div className="space-y-1">
            <div className="flex justify-end">{control}</div>
            {origin}
          </div>
          {isAdmin && here ? <Reset onClick={() => save.mutate(null)} /> : <span className="hidden w-8 sm:block" />}
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

// parse turns what was typed into the value a setting takes: a list for
// probes and channels, a number or "off" for thresholds, the text as it is
// otherwise.
function parse(type: ConfigValue["type"], text: string): unknown {
  const trimmed = text.trim();
  if (type === "probes" || type === "channels" || type === "services") {
    const names = trimmed.split(/[\s,]+/).filter(Boolean);
    return type === "probes" && names.length === 1 && names[0] === "all" ? "all" : names;
  }
  if (type === "threshold") {
    const n = Number(trimmed.replace(",", "."));
    return trimmed === "" || Number.isNaN(n) ? trimmed : n;
  }
  return trimmed;
}
