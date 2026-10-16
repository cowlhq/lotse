import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Bell, Home, LogOut, Monitor, Moon, Search, Settings, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { listScopes, logout } from "@/api";
import { useAlerts } from "@/components/Alerts";
import { Logo } from "@/components/Logo";
import { AddMenu } from "@/components/shell/AddMenu";
import { StatusIcon } from "@/components/shell/Dot";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Kbd } from "@/components/ui/kbd";
import { firing, levelUnder } from "@/lib/status";
import { useMe } from "@/lib/useMe";
import { useUpdates } from "@/lib/useUpdates";
import { newer } from "@/lib/version";

const item =
  "flex h-9 w-full items-center gap-2.5 rounded-md px-2.5 text-sidebar-foreground transition-colors duration-100 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground";
const active = "bg-sidebar-accent text-sidebar-accent-foreground";
const icon = "size-4 text-muted-foreground";

// Sidebar is the way around: the views, the tree of customers, projects and
// environments, marked where something is wrong, and the settings.
export function Sidebar({ onSearch, onNavigate }: { onSearch: () => void; onNavigate?: () => void }) {
  const { t } = useTranslation();
  const alerts = useAlerts();
  const list = alerts.data ?? [];
  const count = firing(list).length;
  const crit = list.some((a) => a.state === "crit");
  const { isAdmin } = useMe();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const { updates } = useUpdates();
  const customers = (scopes.data ?? []).filter((x) => x.kind === "customer");
  const link = (to: string, glyph: ReactNode, label: string, extra?: ReactNode) => (
    <Link to={to} className={item} activeProps={{ className: active }} activeOptions={{ exact: to === "/" }} onClick={onNavigate}>
      {glyph}
      <span className="flex-1 truncate">{label}</span>
      {extra}
    </Link>
  );
  return (
    <div className="flex h-full flex-col gap-5 px-3 py-3">
      <div className="flex h-9 items-center justify-between pl-2">
        <Wordmark />
        {isAdmin && <AddMenu compact />}
      </div>
      <nav className="space-y-px">
        <button type="button" onClick={onSearch} className={item}>
          <Search className={icon} />
          <span className="flex-1 text-left">{t("nav.search")}</span>
          <Kbd className="text-[11px]">⌘K</Kbd>
        </button>
        {link("/", <Home className={icon} />, t("nav.home"))}
        {link(
          "/alerts",
          <Bell className={icon} />,
          t("nav.alerts"),
          count > 0 && (
            <span className="rounded-full px-1.5 text-mini font-medium text-white tabular-nums" style={{ background: crit ? "var(--crit)" : "var(--warn)" }}>
              {count}
            </span>
          ),
        )}
      </nav>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <p className="mb-0.5 px-2.5 text-mini font-medium text-muted-foreground">{t("nav.customers")}</p>
        {customers.length === 0 ? (
          <p className="px-2.5 text-mini text-muted-foreground">{t("nav.noCustomers")}</p>
        ) : (
          <ul className="space-y-px">
            {customers.map((c) => {
              const level = levelUnder(list, c.path);
              return (
                <li key={c.id}>
                  <Link to="/scopes/$id" params={{ id: String(c.id) }} className={item} activeProps={{ className: active }} onClick={onNavigate}>
                    <span className="flex size-5 items-center justify-center rounded bg-muted text-[11px] font-semibold text-muted-foreground">{c.name.slice(0, 1).toUpperCase()}</span>
                    <span className="flex-1 truncate">{c.name}</span>
                    {level !== "ok" && <StatusIcon level={level} size={12} />}
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </div>
      <nav className="space-y-px">
        {link(
          "/settings",
          <Settings className={icon} />,
          t("nav.settings"),
          updates && newer(updates.latest?.version, updates.hub_version) && <span className="size-1.5 rounded-full bg-brand" title={t("nav.updateAvailable")} />,
        )}
        <UserMenu />
      </nav>
    </div>
  );
}

function UserMenu() {
  const { t } = useTranslation();
  const { user: me } = useMe();
  const { theme, setTheme } = useTheme();
  const client = useQueryClient();
  const out = useMutation({ mutationFn: logout, onSuccess: () => client.invalidateQueries({ queryKey: ["me"] }) });
  const name = me?.name || me?.email || "";
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className={item}>
        <span className="flex size-5 items-center justify-center rounded-full bg-muted-foreground/25 text-[10px] font-semibold text-foreground">{name.slice(0, 1).toUpperCase()}</span>
        <span className="flex-1 truncate text-left">{name}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="w-56">
        {/* A label belongs in a group, or the menu does not open. */}
        <DropdownMenuGroup>
          <DropdownMenuLabel className="truncate text-mini text-muted-foreground">{me?.email}</DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuRadioGroup value={theme ?? "system"} onValueChange={(v) => setTheme(v as string)}>
          <DropdownMenuRadioItem value="system">
            <Monitor />
            {t("nav.themeSystem")}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="light">
            <Sun />
            {t("nav.themeLight")}
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">
            <Moon />
            {t("nav.themeDark")}
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => out.mutate()}>
          <LogOut />
          {t("login.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// Wordmark is lotse's logo, leading home.
export function Wordmark({ className = "h-[22px]" }: { className?: string }) {
  return (
    <Link to="/" className="flex items-center text-foreground">
      <Logo className={`w-auto ${className}`} />
    </Link>
  );
}
