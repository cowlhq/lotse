import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Activity, Bell, Folder, Gauge, Moon, Server, Settings, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { listChecks, listHosts, listScopes } from "@/api";
import { useAlerts } from "@/components/Alerts";
import { Dot } from "@/components/shell/Dot";
import { Command, CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { levelOfSubject, levelUnder } from "@/lib/status";

// CommandMenu finds anything by typing: pages, environments, hosts, checks,
// and a few things to do. ⌘K or / opens it.
export function CommandMenu({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { resolvedTheme, setTheme } = useTheme();
  const hosts = useQuery({ queryKey: ["hosts"], queryFn: listHosts, enabled: open });
  const checks = useQuery({ queryKey: ["checks", "all"], queryFn: () => listChecks(), enabled: open });
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes, enabled: open });
  const alerts = useAlerts().data ?? [];

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      const typing = e.target instanceof HTMLElement && (e.target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(e.target.tagName));
      if ((e.key === "k" && (e.metaKey || e.ctrlKey)) || (e.key === "/" && !typing)) {
        e.preventDefault();
        onOpenChange(!open);
      }
    };
    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, [open, onOpenChange]);

  const go = (to: string, params?: Record<string, string>) => {
    onOpenChange(false);
    navigate({ to, params } as never);
  };
  const byID = new Map((scopes.data ?? []).map((s) => [s.id, s]));

  return (
    <CommandDialog open={open} onOpenChange={onOpenChange} title={t("nav.search")} description={t("nav.searchHint")} className="rounded-xl! sm:max-w-xl">
      <Command className="rounded-xl">
      <CommandInput placeholder={t("nav.searchPlaceholder")} />
      <CommandList>
        <CommandEmpty>{t("nav.nothingFound")}</CommandEmpty>
        <CommandGroup heading={t("nav.pages")}>
          <CommandItem onSelect={() => go("/")}>
            <Gauge /> {t("nav.home")}
          </CommandItem>
          <CommandItem onSelect={() => go("/alerts")}>
            <Bell /> {t("nav.alerts")}
          </CommandItem>
          <CommandItem onSelect={() => go("/settings")}>
            <Settings /> {t("nav.settings")}
          </CommandItem>
        </CommandGroup>
        {(scopes.data?.length ?? 0) > 0 && (
          <CommandGroup heading={t("nav.environments")}>
            {scopes.data!.map((s) => (
              <CommandItem key={s.id} value={`scope ${s.path} ${s.name}`} onSelect={() => go("/scopes/$id", { id: String(s.id) })}>
                <Folder />
                <span className="max-w-[60%] shrink-0 truncate">{s.name}</span>
                <span className="min-w-0 flex-1 truncate text-right text-xs text-muted-foreground">{s.path}</span>
                <Dot level={levelUnder(alerts, s.path)} />
              </CommandItem>
            ))}
          </CommandGroup>
        )}
        {(hosts.data?.length ?? 0) > 0 && (
          <CommandGroup heading={t("nav.hosts")}>
            {hosts.data!.map(({ host }) => (
              <CommandItem key={host.id} value={`host ${host.name} ${host.hostname}`} onSelect={() => go("/hosts/$id", { id: String(host.id) })}>
                <Server />
                <span className="max-w-[60%] shrink-0 truncate">{host.name}</span>
                <span className="min-w-0 flex-1 truncate text-right text-xs text-muted-foreground">{host.environment_id ? byID.get(host.environment_id)?.path : ""}</span>
                <Dot level={host.online ? levelOfSubject(alerts, "host", host.id) : "crit"} />
              </CommandItem>
            ))}
          </CommandGroup>
        )}
        {(checks.data?.length ?? 0) > 0 && (
          <CommandGroup heading={t("nav.checks")}>
            {checks.data!.map((c) => (
              <CommandItem key={c.id} value={`check ${c.path} ${c.name}`} onSelect={() => go("/checks/$id", { id: String(c.id) })}>
                <Activity />
                <span className="max-w-[60%] shrink-0 truncate">{c.name}</span>
                {/* The name is there already; the path says where. */}
                <span className="min-w-0 flex-1 truncate text-right text-xs text-muted-foreground">{c.path.slice(0, c.path.lastIndexOf("/"))}</span>
                <Dot level={c.state === "down" ? "crit" : c.state === "up" ? "ok" : "unknown"} />
              </CommandItem>
            ))}
          </CommandGroup>
        )}
        <CommandGroup heading={t("nav.actions")}>
          <CommandItem
            onSelect={() => {
              setTheme(resolvedTheme === "dark" ? "light" : "dark");
              onOpenChange(false);
            }}
          >
            {resolvedTheme === "dark" ? <Sun /> : <Moon />}
            {resolvedTheme === "dark" ? t("nav.themeLight") : t("nav.themeDark")}
          </CommandItem>
        </CommandGroup>
      </CommandList>
      </Command>
    </CommandDialog>
  );
}
