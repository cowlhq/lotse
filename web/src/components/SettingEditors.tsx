import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { listChannels, listProbes, type ConfigValue } from "@/api";
import { Input } from "@/components/ui/input";

// Controls for settings by type, so that nothing is typed that can be
// picked: durations are a row of choices, a window two times, probes and
// channels the ones there are, services a list of names.

type Props = { value: ConfigValue; onChange: (v: unknown) => void; disabled: boolean };

// presets are the durations offered for each setting, within what it
// allows; a value set otherwise is shown among them.
const presets: Record<string, string[]> = {
  "agent.interval.host": ["5s", "10s", "15s", "30s", "1m"],
  "agent.interval.containers": ["15s", "30s", "1m", "5m"],
  "check.interval": ["30s", "1m", "5m", "15m", "1h"],
  "check.timeout": ["5s", "10s", "20s", "30s"],
  "notify.repeat": ["0s", "30m", "1h", "4h", "12h", "24h"],
  "retention.raw": ["1d", "2d", "7d", "14d", "30d"],
  "retention.5m": ["7d", "30d", "90d", "180d"],
  "retention.1h": ["90d", "180d", "365d", "730d"],
  "deadman.interval": ["30s", "1m", "5m", "15m"],
};

// wide says whether a type's control goes below its name, across the row.
export const wide = (type: ConfigValue["type"]) => type === "probes" || type === "channels" || type === "services";

export function DurationEditor({ value: v, onChange, disabled }: Props) {
  const { t } = useTranslation();
  const current = String(v.value);
  const options = presets[v.key] ?? [];
  const list = options.includes(current) ? options : [...options, current];
  return (
    <div className="flex flex-wrap justify-end gap-1">
      {list.map((x) => (
        <button
          key={x}
          type="button"
          disabled={disabled}
          onClick={() => x !== current && onChange(x)}
          className={`h-7 rounded-md px-2 text-mini tabular-nums transition-colors disabled:cursor-default ${x === current ? "bg-brand text-white" : "text-muted-foreground enabled:hover:bg-accent enabled:hover:text-foreground"}`}
        >
          {x === "0s" ? t("route.never") : x}
        </button>
      ))}
    </div>
  );
}

// WindowEditor is a time of day from and to, such as 04:00-06:00.
export function WindowEditor({ value: v, onChange, disabled }: Props) {
  const { t } = useTranslation();
  const [from0, to0] = String(v.value).split("-");
  const [from, setFrom] = useState(from0);
  const [to, setTo] = useState(to0);
  const commit = (a: string, b: string) => a && b && `${a}-${b}` !== v.value && onChange(`${a}-${b}`);
  return (
    <span className="flex items-center gap-2 text-mini">
      <Input type="time" value={from} disabled={disabled} onChange={(e) => setFrom(e.target.value)} onBlur={() => commit(from, to)} className="h-8 w-32" aria-label={t("maintenance.from")} />
      –
      <Input type="time" value={to} disabled={disabled} onChange={(e) => setTo(e.target.value)} onBlur={() => commit(from, to)} className="h-8 w-32" aria-label={t("maintenance.until")} />
    </span>
  );
}

function Chip({ on, children, onClick, disabled }: { on: boolean; children: React.ReactNode; onClick: () => void; disabled: boolean }) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-pressed={on}
      className={`inline-flex h-7 items-center rounded-md border px-2.5 text-mini transition-colors disabled:cursor-default ${on ? "border-brand/40 bg-brand/10 font-medium text-foreground" : "text-muted-foreground enabled:hover:bg-accent enabled:hover:text-foreground"}`}
    >
      {children}
    </button>
  );
}

// ProbesEditor is "all", or some probes by key.
export function ProbesEditor({ value: v, onChange, disabled }: Props) {
  const { t } = useTranslation();
  const probes = useQuery({ queryKey: ["probes"], queryFn: listProbes });
  const all = v.value === "all";
  const chosen = all ? [] : ((v.value as string[]) ?? []);
  const list = (probes.data ?? []).filter((p) => !p.revoked);
  const toggle = (slug: string) => {
    const next = chosen.includes(slug) ? chosen.filter((s) => s !== slug) : [...chosen, slug];
    onChange(next.length ? next : "all");
  };
  return (
    <div className="flex flex-wrap gap-1.5">
      <Chip on={all} disabled={disabled} onClick={() => !all && onChange("all")}>
        {t("setting.allProbes")}
      </Chip>
      {list.map((p) => (
        <Chip key={p.slug} on={chosen.includes(p.slug)} disabled={disabled} onClick={() => toggle(p.slug)}>
          {p.name}
        </Chip>
      ))}
    </div>
  );
}

// ChannelsEditor picks among the channels usable where the object is.
export function ChannelsEditor({ value: v, onChange, disabled, envID }: Props & { envID?: number }) {
  const { t } = useTranslation();
  const channels = useQuery({ queryKey: ["channels", envID ?? 0], queryFn: () => listChannels(envID) });
  const chosen = (v.value as string[]) ?? [];
  const keys = [...new Map((channels.data ?? []).map((c) => [c.key, c])).values()];
  const missing = chosen.filter((k) => !keys.some((c) => c.key === k));
  const toggle = (key: string) => onChange(chosen.includes(key) ? chosen.filter((k) => k !== key) : [...chosen, key]);
  return (
    <div className="flex flex-wrap gap-1.5">
      {keys.map((c) => (
        <Chip key={c.key} on={chosen.includes(c.key)} disabled={disabled} onClick={() => toggle(c.key)}>
          {c.name}
        </Chip>
      ))}
      {missing.map((k) => (
        <Chip key={k} on disabled={disabled} onClick={() => toggle(k)}>
          <span className="text-destructive" title={t("route.missing", { key: k })}>
            {k}
          </span>
        </Chip>
      ))}
      {keys.length === 0 && missing.length === 0 && <span className="text-mini text-muted-foreground">{t("channel.none")}</span>}
    </div>
  );
}

// TagsEditor is a list of names, such as systemd units: typed and added
// with Enter or a comma, removed with their cross.
export function TagsEditor({ value: v, onChange, disabled, placeholder }: Props & { placeholder?: string }) {
  const { t } = useTranslation();
  const list = (v.value as string[]) ?? [];
  const [text, setText] = useState("");
  const add = () => {
    const names = text.split(/[\s,]+/).filter((n) => n && !list.includes(n));
    setText("");
    if (names.length) onChange([...list, ...names]);
  };
  return (
    <div className="flex min-h-9 flex-wrap items-center gap-1.5 rounded-md border px-1.5 py-1 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/30">
      {list.map((n) => (
        <span key={n} className="inline-flex h-6 items-center gap-1 rounded bg-muted pr-1 pl-2 font-mono text-mini">
          {n}
          {!disabled && (
            <button type="button" aria-label={t("setting.remove", { name: n })} onClick={() => onChange(list.filter((x) => x !== n))} className="rounded p-0.5 text-muted-foreground hover:bg-background hover:text-foreground">
              <X className="size-3" />
            </button>
          )}
        </span>
      ))}
      {!disabled && (
        <input
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === ",") {
              e.preventDefault();
              add();
            } else if (e.key === "Backspace" && !text && list.length) onChange(list.slice(0, -1));
          }}
          onBlur={add}
          placeholder={list.length ? "" : placeholder}
          className="h-6 min-w-32 flex-1 bg-transparent px-1 font-mono text-mini outline-none placeholder:text-muted-foreground/55"
        />
      )}
    </div>
  );
}
