import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { setSetting, type ConfigValue } from "@/api";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { useMe } from "@/lib/useMe";

// A rule, as Beszel shows one: a switch, and where it has thresholds, a
// slider; how long it must be bad before it fires, and good before it
// clears. The bell of a host or check shows its rules this way, and so do
// the settings of a scope, for everything in it.

// The range a slider covers, by unit.
const ranges: Record<string, { min: number; max: number; step: number; suffix: string }> = {
  percent: { min: 0, max: 100, step: 1, suffix: " %" },
  load: { min: 0, max: 10, step: 0.25, suffix: "" },
  days: { min: 1, max: 60, step: 1, suffix: " d" },
  count: { min: 1, max: 20, step: 1, suffix: "" },
};

const durations = ["0s", "1m", "2m", "5m", "10m", "15m", "30m", "1h"];

const fields = ["enabled", "warn", "crit", "level", "for", "recover"];

export function Rule({ subject, rule, values, className = "px-5 py-4" }: { subject: string; rule: string; values: ConfigValue[]; className?: string }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const get = (field: string) => values.find((v) => v.key === `rules.${rule}.${field}`);
  const enabled = get("enabled");
  const warn = get("warn");
  const crit = get("crit");
  const level = get("level");
  const save = useMutation({
    mutationFn: ({ field, value }: { field: string; value: unknown }) => setSetting(subject, `rules.${rule}.${field}`, value),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["config"] });
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });
  if (!enabled) return null;
  const set = (field: string, value: unknown) => save.mutate({ field, value });
  const here = (v?: ConfigValue) => !!v?.from && `${v.from.kind}:${v.from.id}` === subject;
  const on = enabled.value === true;
  const unit = /\((\w+)\)/.exec(crit?.description ?? warn?.description ?? "")?.[1] ?? "";
  const range = ranges[unit];
  const num = (v?: ConfigValue) => (typeof v?.value === "number" ? v.value : null);
  const w = num(warn);
  const c = num(crit);
  const thumbs = [w, c].filter((x): x is number => x != null).sort((a, b) => a - b);
  // Days a certificate has left: lower is worse, so CRIT is the lower thumb.
  const below = unit === "days";
  const anyHere = fields.some((f) => here(get(f)));
  // Where what applies comes from, if not the defaults.
  const inherited = fields.map((f) => get(f)?.from).find((from) => from && `${from.kind}:${from.id}` !== subject);
  const disabled = !isAdmin || save.isPending;
  return (
    <li className={`space-y-3 ${className}`}>
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <p className={on ? "font-medium" : "font-medium text-muted-foreground"}>{t(`alert.titles.${rule}`)}</p>
          <p className="text-mini text-muted-foreground">{t(`rules.${rule}`)}</p>
        </div>
        <Switch checked={on} disabled={disabled} onCheckedChange={(v) => set("enabled", v)} aria-label={t(`alert.titles.${rule}`)} />
      </div>
      {on && (
        <div className="space-y-3">
          {range && thumbs.length > 0 && (
            <div className="space-y-2">
              <div className="flex justify-between text-mini">
                <span className="text-muted-foreground">{t("bell.thresholds")}</span>
                <span className="tabular-nums">
                  {w != null && <span style={{ color: "var(--warn)" }}>{t("bell.warnAt", { value: `${w}${range.suffix}` })}</span>}
                  {w != null && c != null && <span className="text-muted-foreground"> · </span>}
                  {c != null && <span style={{ color: "var(--crit)" }}>{t("bell.critAt", { value: `${c}${range.suffix}` })}</span>}
                </span>
              </div>
              <Slider
                // A new key when the values change elsewhere, as by a reset.
                key={thumbs.join(",")}
                min={range.min}
                max={range.max}
                step={range.step}
                defaultValue={thumbs}
                disabled={disabled}
                onValueCommitted={(v) => {
                  const list = Array.isArray(v) ? v : [v];
                  if (w != null && c != null) {
                    set(below ? "crit" : "warn", list[0]);
                    set(below ? "warn" : "crit", list[1]);
                  } else if (c != null) {
                    set("crit", list[0]);
                  } else {
                    set("warn", list[0]);
                  }
                }}
              />
            </div>
          )}
          {level && (
            <Choice label={t("rule.level")} value={String(level.value)} options={["warn", "crit"]} name={(x) => t(`config.levels.${x}`)} onChange={(x) => set("level", x)} disabled={disabled} />
          )}
          {get("for") && <Choice label={t("bell.for")} value={String(get("for")!.value)} options={durations} name={(d) => (d === "0s" ? t("bell.now") : d)} onChange={(d) => set("for", d)} disabled={disabled} />}
          {get("recover") && (
            <Choice label={t("rule.recover")} value={String(get("recover")!.value)} options={durations} name={(d) => (d === "0s" ? t("bell.now") : d)} onChange={(d) => set("recover", d)} disabled={disabled} />
          )}
        </div>
      )}
      {anyHere ? (
        <p className="text-mini text-muted-foreground">
          {t("bell.setHere")}
          {isAdmin && (
            <>
              {" · "}
              <button type="button" className="underline-offset-2 hover:text-foreground hover:underline" onClick={() => fields.forEach((f) => here(get(f)) && set(f, null))}>
                {t("bell.reset")}
              </button>
            </>
          )}
        </p>
      ) : (
        inherited && <p className="text-mini text-muted-foreground">{t("bell.from", { name: inherited.name || t(`config.kinds.${inherited.kind}`) })}</p>
      )}
    </li>
  );
}

// Choice is a row of buttons for one value, such as a duration; a value
// set elsewhere that is none of them is shown among them.
function Choice({ label, value, options, name, onChange, disabled }: { label: string; value: string; options: string[]; name: (x: string) => string; onChange: (x: string) => void; disabled: boolean }) {
  const list = options.includes(value) ? options : [...options, value];
  return (
    <div className="flex items-center justify-between gap-3 text-mini">
      <span className="text-muted-foreground">{label}</span>
      <div className="flex flex-wrap justify-end gap-1">
        {list.map((x) => (
          <button
            key={x}
            type="button"
            disabled={disabled}
            onClick={() => x !== value && onChange(x)}
            className={`h-7 rounded-md px-2 tabular-nums transition-colors disabled:cursor-default ${x === value ? "bg-brand text-white" : "text-muted-foreground enabled:hover:bg-accent enabled:hover:text-foreground"}`}
          >
            {name(x)}
          </button>
        ))}
      </div>
    </div>
  );
}
