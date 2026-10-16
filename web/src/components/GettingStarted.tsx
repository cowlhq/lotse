import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { getConfig, type Check as CheckData, type HostSummary, type Scope } from "@/api";
import { AddMenu } from "@/components/shell/AddMenu";
import { Button } from "@/components/ui/button";

// GettingStarted takes a new hub from nothing to being told: an
// environment, a host, a check, and somewhere for alerts to go. Each step
// ticks itself off when it is done; the list can be put away.
export function GettingStarted({ scopes, hosts, checks, onHide }: { scopes: Scope[]; hosts: HostSummary[]; checks: CheckData[]; onHide: () => void }) {
  const { t } = useTranslation();
  // Alerts reach someone only once a channel is switched on, not when one
  // exists.
  const config = useQuery({ queryKey: ["config", "global:1"], queryFn: () => getConfig("global:1") });
  const routed = (config.data?.find((v) => v.key === "notify.channels")?.value as string[] | undefined) ?? [];
  const steps = [
    { key: "environment", done: scopes.some((s) => s.kind === "environment") },
    { key: "host", done: hosts.length > 0 },
    { key: "check", done: checks.length > 0 },
    { key: "notify", done: routed.length > 0 },
  ];
  const left = steps.filter((s) => !s.done).length;
  if (left === 0 || config.isPending) return null;
  const next = steps.find((s) => !s.done)!.key;
  return (
    <section className="overflow-hidden rounded-lg border bg-card">
      <header className="flex items-center gap-3 border-b px-4 py-3">
        <h2 className="font-semibold">{t("start.title")}</h2>
        <span className="text-muted-foreground">{t("start.progress", { done: steps.length - left, of: steps.length })}</span>
        <Link to="/start" className="text-mini text-brand hover:underline">
          {t("start.guide")}
        </Link>
        <Button
          variant="ghost"
          size="icon-sm"
          className="ml-auto"
          aria-label={t("start.hide")}
          title={t("start.hide")}
          onClick={onHide}
        >
          <X />
        </Button>
      </header>
      <ol>
        {steps.map((s, i) => (
          <li key={s.key} className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b px-4 py-3 last:border-b-0">
            <span
              className={`flex size-6 shrink-0 items-center justify-center rounded-full text-mini font-semibold ${s.done ? "text-white" : s.key === next ? "bg-brand text-white" : "bg-muted text-muted-foreground"}`}
              style={s.done ? { background: "var(--ok)" } : undefined}
            >
              {s.done ? <Check className="size-3.5" /> : i + 1}
            </span>
            <div className="min-w-0 flex-1 basis-48">
              <p className={s.done ? "text-muted-foreground line-through decoration-muted-foreground/40" : "font-medium"}>{t(`start.${s.key}.title`)}</p>
              {!s.done && <p className="text-mini text-muted-foreground">{t(`start.${s.key}.hint`)}</p>}
            </div>
            {/* On a phone the action goes under its text, in line with it. */}
            {!s.done && s.key === next && (s.key === "notify" ? (
              <Button size="sm" className="ml-10 sm:ml-0" render={<Link to="/settings" search={{ tab: "notifications" }} />}>
                {t("start.notify.action")}
              </Button>
            ) : (
              <AddMenu direct={s.key as "environment" | "host" | "check"} />
            ))}
          </li>
        ))}
      </ol>
    </section>
  );
}
