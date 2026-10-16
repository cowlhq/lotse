import { useQuery } from "@tanstack/react-query";
import { FileDrop } from "@/components/ApplyFile";
import { createRootRoute, createRoute, createRouter, Navigate, Outlet, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { getHealth, getMe, Unauthorized } from "@/api";
import { Centered } from "@/components/shell/Centered";
import { CommandMenu } from "@/components/shell/CommandMenu";
import { Sidebar, Wordmark } from "@/components/shell/Sidebar";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Menu } from "lucide-react";
import { Toaster } from "@/components/ui/sonner";
import { Skeleton } from "@/components/ui/skeleton";
import { AlertDetail, Alerts } from "@/routes/Alerts";
import { Check } from "@/routes/Check";
import { Container } from "@/routes/Container";
import { Scope } from "@/routes/Scope";
import { Settings } from "@/routes/Settings";
import { Host } from "@/routes/Host";
import { Login } from "@/routes/Login";
import { Overview } from "@/routes/Overview";
import { NoSetupToken, Setup } from "@/routes/Setup";
import { Welcome } from "@/routes/Welcome";

// Gate shows setup while the hub is not set up, the login page without a
// session, and the app otherwise.
function Gate() {
  const { t } = useTranslation();
  const path = useRouterState({ select: (s) => s.location.pathname });
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth, refetchInterval: 30_000 });
  const setUp = health.data !== undefined && !health.data.setup;
  const me = useQuery({ queryKey: ["me"], queryFn: getMe, enabled: setUp, retry: false });

  if (health.isPending) return <Skeleton className="m-6 h-32" />;
  if (health.isError) {
    return (
      <Centered title={t("app.unreachableTitle")}>
        <p className="text-muted-foreground">{t("app.unreachable")}</p>
      </Centered>
    );
  }
  if (health.data.setup) {
    return path === "/setup" ? (
      <Outlet />
    ) : (
      <Centered title={t("setup.title")}>
        <NoSetupToken />
      </Centered>
    );
  }
  if (path === "/setup") return <Navigate to="/" />;
  // A link to choose a password works whether someone is logged in or not.
  if (path === "/welcome") return <Outlet />;
  if (me.isPending) return <Skeleton className="m-6 h-32" />;
  if (me.error instanceof Unauthorized) {
    return path === "/login" ? <Outlet /> : <Navigate to="/login" search={{ next: path }} />;
  }
  if (path === "/login") return <Navigate to="/" />;
  return <Layout />;
}

function Layout() {
  const { t } = useTranslation();
  const [search, setSearch] = useState(false);
  const [menu, setMenu] = useState(false);
  return (
    <FileDrop>
    <div className="flex h-dvh bg-sidebar">
      <aside className="hidden w-64 shrink-0 md:block">
        <Sidebar onSearch={() => setSearch(true)} />
      </aside>
      <Sheet open={menu} onOpenChange={setMenu}>
        <SheetContent side="left" className="w-72 bg-sidebar p-0">
          <Sidebar onSearch={() => (setMenu(false), setSearch(true))} onNavigate={() => setMenu(false)} />
        </SheetContent>
      </Sheet>
      <div className="flex min-w-0 flex-1 flex-col md:py-2 md:pr-2">
        <div className="flex h-12 shrink-0 items-center gap-2 border-b bg-background px-3 md:hidden">
          <Button variant="ghost" size="icon-sm" aria-label={t("nav.menu")} onClick={() => setMenu(true)}>
            <Menu />
          </Button>
          <Wordmark />
        </div>
        <main className="min-h-0 min-w-0 flex-1 overflow-y-auto bg-background pb-16 md:rounded-lg md:border md:shadow-xs">
          <Outlet />
        </main>
      </div>
      <CommandMenu open={search} onOpenChange={setSearch} />
      <Toaster position="bottom-right" />
    </div>
    </FileDrop>
  );
}

const root = createRootRoute({ component: Gate });

const overview = createRoute({ getParentRoute: () => root, path: "/", component: Overview });

const host = createRoute({
  getParentRoute: () => root,
  path: "/hosts/$id",
  component: function HostRoute() {
    const { id } = host.useParams();
    return <Host id={Number(id)} />;
  },
});

const container = createRoute({
  getParentRoute: () => root,
  path: "/containers/$id",
  component: function ContainerRoute() {
    const { id } = container.useParams();
    return <Container id={Number(id)} />;
  },
});

const scope = createRoute({
  getParentRoute: () => root,
  path: "/scopes/$id",
  component: function ScopeRoute() {
    const { id } = scope.useParams();
    return <Scope id={Number(id)} />;
  },
});

const check = createRoute({
  getParentRoute: () => root,
  path: "/checks/$id",
  component: function CheckRoute() {
    const { id } = check.useParams();
    return <Check id={Number(id)} />;
  },
});

const alerts = createRoute({ getParentRoute: () => root, path: "/alerts", component: Alerts });

const alertDetail = createRoute({
  getParentRoute: () => root,
  path: "/alerts/$id",
  component: function AlertRoute() {
    const { id } = alertDetail.useParams();
    return <AlertDetail id={Number(id)} />;
  },
});

// The lists are the home page now.
const hostList = createRoute({ getParentRoute: () => root, path: "/hosts", component: () => <Navigate to="/" /> });

const checkList = createRoute({ getParentRoute: () => root, path: "/checks", component: () => <Navigate to="/" /> });

const settings = createRoute({
  getParentRoute: () => root,
  path: "/settings",
  validateSearch: (search: Record<string, unknown>): { tab?: string } => (typeof search.tab === "string" ? { tab: search.tab } : {}),
  component: function SettingsRoute() {
    const { tab } = settings.useSearch();
    return <Settings tab={tab} />;
  },
});

const setup = createRoute({
  getParentRoute: () => root,
  path: "/setup",
  validateSearch: (search: Record<string, unknown>) => ({
    token: typeof search.token === "string" ? search.token : "",
  }),
  component: function SetupRoute() {
    const { token } = setup.useSearch();
    return <Setup token={token} />;
  },
});

const login = createRoute({
  getParentRoute: () => root,
  path: "/login",
  validateSearch: (search: Record<string, unknown>) => ({
    next: typeof search.next === "string" ? search.next : "/",
  }),
  component: function LoginRoute() {
    const { next } = login.useSearch();
    return <Login next={next} />;
  },
});

const welcome = createRoute({ getParentRoute: () => root, path: "/welcome", component: Welcome });

export const router = createRouter({ routeTree: root.addChildren([overview, hostList, checkList, host, container, scope, check, alerts, alertDetail, settings, setup, login, welcome]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
