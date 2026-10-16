import { useQuery } from "@tanstack/react-query";
import { Link, Navigate } from "@tanstack/react-router";
import { Activity, Bell, Check, FolderPlus, PartyPopper, Server } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { getConfig, listChecks, listHosts, listScopes, type Scope } from "@/api";
import { AgentBody } from "@/components/AddAgent";
import { CheckForm } from "@/components/AddCheck";
import { Channels } from "@/components/Channels";
import { MailServer } from "@/components/MailServer";
import { EnvironmentForm } from "@/components/NewEnvironment";
import { Wordmark } from "@/components/shell/Sidebar";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { useMe } from "@/lib/useMe";

// Start takes a new hub from nothing to being told, on a page of its own
// after setup: a customer and its environment, a machine, a check, and
// where alerts go. Each step can be skipped; the overview's checklist
// picks up what is left.

const steps = ["environment", "host", "check", "notify", "done"] as const;
type Step = (typeof steps)[number];
const icons: Record<Step, typeof Server> = { environment: FolderPlus, host: Server, check: Activity, notify: Bell, done: PartyPopper };

export function Start() {
  const { t } = useTranslation();
  const { isAdmin, isHubAdmin } = useMe();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts, refetchInterval: 5000 });
  const checks = useQuery({ queryKey: ["checks", "all"], queryFn: () => listChecks() });
  const config = useQuery({ queryKey: ["config", "global:1"], queryFn: () => getConfig("global:1") });
  const [step, setStep] = useState<Step>("environment");
  const [chosen, setChosen] = useState<number | null>(null);
  const envs = (scopes.data ?? []).filter((s) => s.kind === "environment");
  // The environment the guide works in: the one made here, or the first.
  const env = chosen ?? envs[0]?.id ?? null;
  const done: Record<Step, boolean> = {
    environment: envs.length > 0,
    host: (hosts.data ?? []).some((h) => h.host.environment_id === env),
    check: (checks.data ?? []).some((c) => c.environment_id === env),
    notify: ((config.data?.find((v) => v.key === "notify.channels")?.value as string[] | undefined) ?? []).length > 0,
    done: false,
  };
  const next = (hasEnv = env !== null) => {
    let i = steps.indexOf(step) + 1;
    // Without an environment there is nowhere for a host or a check.
    while (!hasEnv && (steps[i] === "host" || steps[i] === "check")) i++;
    setStep(steps[i]);
  };

  // Only who may add things is guided through adding them.
  if (!isAdmin) return <Navigate to="/" />;
  return (
    <div className="min-h-dvh bg-sidebar px-4 py-10 sm:py-14">
      <div className="mx-auto w-full max-w-2xl space-y-8">
        <header className="flex items-center justify-between">
          <Wordmark className="h-7" />
          <Link to="/" className="text-mini text-muted-foreground hover:text-foreground">
            {step === "done" ? t("start.toOverview") : t("start.later")}
          </Link>
        </header>
        <ol className="grid grid-cols-5 gap-1.5" aria-label={t("start.title")}>
          {steps.map((s, i) => {
            const Icon = icons[s];
            const here = s === step;
            const past = steps.indexOf(step) > i;
            return (
              <li key={s}>
                <button
                  type="button"
                  onClick={() => (s === "host" || s === "check" ? env !== null && setStep(s) : setStep(s))}
                  className={`flex w-full flex-col gap-2 rounded-md text-left text-mini ${here ? "text-foreground" : "text-muted-foreground hover:text-foreground"}`}
                >
                  <span className={`h-1 rounded-full ${here ? "bg-brand" : past || done[s] ? "bg-brand/40" : "bg-border"}`} />
                  <span className="flex items-center gap-1.5 max-sm:justify-center">
                    {done[s] ? <Check className="size-3.5" style={{ color: "var(--ok)" }} /> : <Icon className="size-3.5" />}
                    <span className={`max-sm:hidden ${here ? "font-medium" : ""}`}>{t(`start.steps.${s}`)}</span>
                  </span>
                </button>
              </li>
            );
          })}
        </ol>

        {/* The forms are those of the dialogs, which want one around them. */}
        <Dialog open modal={false}>
          {step === "environment" && (
            <Frame
              title={t("start.environment.heading")}
              text={t("start.environment.text")}
              footer={<Footer onSkip={next} skip={done.environment ? t("start.next") : t("start.skip")} />}
            >
              {done.environment && chosen === null ? (
                <Existing envs={envs} env={env} onPick={(id) => (setChosen(id), next(true))} />
              ) : (
                <EnvironmentForm customer="" project="" done={() => {}} onCreated={(e) => (setChosen(e.id), next(true))} bare />
              )}
            </Frame>
          )}
          {step === "host" && env !== null && (
            <Frame title={t("start.host.heading")} text={t("start.host.text")} footer={<Footer onSkip={next} skip={done.host ? t("start.next") : t("start.skip")} />}>
              <AgentBody envID={env} onArrived={() => next()} bare />
            </Frame>
          )}
          {step === "check" && env !== null && (
            <Frame title={t("start.check.heading")} text={t("start.check.text")} footer={<Footer onSkip={next} skip={done.check ? t("start.next") : t("start.skip")} />}>
              <CheckForm envID={env} onCreated={() => next()} />
            </Frame>
          )}
          {step === "notify" && (
            <Frame
              title={t("start.notify.heading")}
              text={t("start.notify.text")}
              footer={<Footer onSkip={next} skip={done.notify ? t("start.finish") : t("start.skip")} primary={done.notify} />}
              bare
            >
              <div className="space-y-6">
                {isHubAdmin && <MailServer />}
                <Channels scopeID={1} subject="global:1" />
              </div>
            </Frame>
          )}
          {step === "done" && <Done done={done} />}
        </Dialog>
      </div>
    </div>
  );
}

function Frame({ title, text, children, footer, bare = false }: { title: string; text: string; children: ReactNode; footer: ReactNode; bare?: boolean }) {
  return (
    <section className="space-y-5">
      <div className="space-y-1.5">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="text-muted-foreground">{text}</p>
      </div>
      {bare ? children : <div className="rounded-xl border bg-background p-6 shadow-sm">{children}</div>}
      {footer}
    </section>
  );
}

function Footer({ onSkip, skip, primary = false }: { onSkip: () => void; skip: string; primary?: boolean }) {
  return (
    <div className="flex justify-end">
      <Button variant={primary ? "default" : "ghost"} onClick={() => onSkip()}>
        {skip}
      </Button>
    </div>
  );
}

// Existing offers the environments there are, for a hub that has some.
function Existing({ envs, env, onPick }: { envs: Scope[]; env: number | null; onPick: (id: number) => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <p>{t("start.environment.existing")}</p>
      <ul className="divide-y rounded-lg border">
        {envs.map((e) => (
          <li key={e.id}>
            <button type="button" onClick={() => onPick(e.id)} className="flex w-full items-center gap-2 px-3 py-2.5 text-left hover:bg-accent">
              <span className="font-medium">{e.name}</span>
              <span className="min-w-0 truncate font-mono text-mini text-muted-foreground">{e.path}</span>
              {e.id === env && <Check className="ml-auto size-4" style={{ color: "var(--ok)" }} />}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Done({ done }: { done: Record<Step, boolean> }) {
  const { t } = useTranslation();
  const left = (["environment", "host", "check", "notify"] as const).filter((s) => !done[s]);
  return (
    <section className="space-y-6 rounded-xl border bg-background p-8 text-center shadow-sm">
      <PartyPopper className="mx-auto size-10 text-brand" />
      <div className="space-y-1.5">
        <h1 className="text-2xl font-semibold tracking-tight">{left.length === 0 ? t("start.done.heading") : t("start.done.headingPartly")}</h1>
        <p className="text-muted-foreground">{left.length === 0 ? t("start.done.text") : t("start.done.left", { list: left.map((s) => t(`start.steps.${s}`)).join(", ") })}</p>
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        <Button render={<Link to="/" />}>{t("start.toOverview")}</Button>
        <Button variant="outline" render={<Link to="/settings" search={{ tab: "users" }} />}>
          {t("start.done.invite")}
        </Button>
      </div>
    </section>
  );
}
