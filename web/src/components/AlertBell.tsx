import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, BellRing } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getConfig, setSetting, type ConfigValue } from "@/api";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { useMe } from "@/lib/useMe";

// AlertBell sets the alerts of one host or check, as Beszel does: a switch
// for each rule, and where it has thresholds, a slider. What is set here
// applies to this object only; what is not set comes from above.

type Kind = "host" | "check" | "heartbeat";

const rulesOf: Record<Kind, string[]> = {
  host: ["host.silent", "host.cpu", "host.memory", "host.disk", "host.swap", "host.load", "host.iowait", "container.unhealthy", "container.restarting"],
  check: ["check.down", "check.cert"],
  heartbeat: ["check.heartbeat"],
};

// The range a slider covers, by unit.
const ranges: Record<string, { min: number; max: number; step: number; suffix: string }> = {
  percent: { min: 0, max: 100, step: 1, suffix: " %" },
  load: { min: 0, max: 10, step: 0.25, suffix: "" },
  days: { min: 1, max: 60, step: 1, suffix: " d" },
  count: { min: 1, max: 20, step: 1, suffix: "" },
};

const durations = ["0s", "1m", "2m", "5m", "10m", "15m", "30m", "1h"];

export function AlertBell({ subject, name, kind }: { subject: string; name: string; kind: Kind }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const [open, setOpen] = useState(false);
  const config = useQuery({ queryKey: ["config", subject], queryFn: () => getConfig(subject), enabled: open });
  // A bell that rings says something is set here.
  const own = (config.data ?? []).some((v) => v.key.startsWith("rules.") && v.from && `${v.from.kind}:${v.from.id}` === subject);
  if (!isAdmin) return null;
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("bell.title", { name })} />}>
        {own ? <BellRing className="text-brand" /> : <Bell />}
      </DialogTrigger>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("bell.title", { name })}</DialogTitle>
          <DialogDescription>{t("bell.description")}</DialogDescription>
        </DialogHeader>
        {config.data && (
          <ul className="-mx-5 divide-y border-y">
            {rulesOf[kind].map((rule) => (
              <Rule key={rule} subject={subject} rule={rule} values={config.data} />
            ))}
          </ul>
        )}
      </DialogContent>
    </Dialog>
  );
}

function Rule({ subject, rule, values }: { subject: string; rule: string; values: ConfigValue[] }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const get = (field: string) => values.find((v) => v.key === `rules.${rule}.${field}`);
  const enabled = get("enabled");
  const warn = get("warn");
  const crit = get("crit");
  const forValue = get("for");
  const save = useMutation({
    mutationFn: ({ field, value }: { field: string; value: unknown }) => setSetting(subject, `rules.${rule}.${field}`, value),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["config"] });
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });
  if (!enabled) return null;
  const here = (v?: ConfigValue) => !!v?.from && `${v.from.kind}:${v.from.id}` === subject;
  const on = enabled.value === true;
  const unit = /\((\w+)\)/.exec(crit?.description ?? "")?.[1] ?? "";
  const range = ranges[unit];
  const num = (v?: ConfigValue) => (typeof v?.value === "number" ? v.value : null);
  const w = num(warn);
  const c = num(crit);
  const thumbs = [w, c].filter((x): x is number => x != null).sort((a, b) => a - b);
  // Days a certificate has left: lower is worse, so CRIT is the lower thumb.
  const below = unit === "days";
  const anyHere = ["enabled", "warn", "crit", "for"].some((f) => here(get(f)));
  // Where what applies comes from, if not the defaults.
  const inherited = ["enabled", "warn", "crit", "for"].map((f) => get(f)?.from).find((from) => from && `${from.kind}:${from.id}` !== subject);
  return (
    <li className="space-y-3 px-5 py-4">
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <p className="font-medium">{t(`alert.titles.${rule}`)}</p>
          <p className="text-mini text-muted-foreground">{t(`rules.${rule}`)}</p>
        </div>
        <Switch checked={on} onCheckedChange={(v) => save.mutate({ field: "enabled", value: v })} />
      </div>
      {on && (
        <div className="space-y-3">
          {range && thumbs.length > 0 && (
            <div className="space-y-2">
              <div className="flex justify-between text-mini">
                <span className="text-muted-foreground">{t("bell.thresholds")}</span>
                <span className="tabular-nums">
                  {w != null && (
                    <span style={{ color: "var(--warn)" }}>
                      {t("bell.warnAt", { value: `${w}${range.suffix}` })}
                    </span>
                  )}
                  {w != null && c != null && <span className="text-muted-foreground"> · </span>}
                  {c != null && (
                    <span style={{ color: "var(--crit)" }}>
                      {t("bell.critAt", { value: `${c}${range.suffix}` })}
                    </span>
                  )}
                </span>
              </div>
              <Slider
                min={range.min}
                max={range.max}
                step={range.step}
                defaultValue={thumbs}
                onValueCommitted={(v) => {
                  const list = Array.isArray(v) ? v : [v];
                  if (w != null && c != null) {
                    save.mutate({ field: below ? "crit" : "warn", value: list[0] });
                    save.mutate({ field: below ? "warn" : "crit", value: list[1] });
                  } else if (c != null) {
                    save.mutate({ field: "crit", value: list[0] });
                  } else {
                    save.mutate({ field: "warn", value: list[0] });
                  }
                }}
              />
            </div>
          )}
          {forValue && (
            <div className="flex items-center justify-between gap-3 text-mini">
              <span className="text-muted-foreground">{t("bell.for")}</span>
              <div className="flex flex-wrap justify-end gap-1">
                {durations.map((d) => (
                  <button
                    key={d}
                    type="button"
                    onClick={() => save.mutate({ field: "for", value: d })}
                    className={`h-7 rounded-md px-2 tabular-nums transition-colors ${forValue.value === d ? "bg-brand text-white" : "text-muted-foreground hover:bg-accent hover:text-foreground"}`}
                  >
                    {d === "0s" ? t("bell.now") : d}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
      {anyHere ? (
        <p className="text-mini text-muted-foreground">
          {t("bell.setHere")} ·{" "}
          <button
            type="button"
            className="underline-offset-2 hover:text-foreground hover:underline"
            onClick={() => ["enabled", "warn", "crit", "for"].forEach((f) => here(get(f)) && save.mutate({ field: f, value: null }))}
          >
            {t("bell.reset")}
          </button>
        </p>
      ) : (
        inherited && <p className="text-mini text-muted-foreground">{t("bell.from", { name: inherited.name || t(`config.kinds.${inherited.kind}`) })}</p>
      )}
    </li>
  );
}
