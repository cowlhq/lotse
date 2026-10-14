import { useQuery } from "@tanstack/react-query";
import { createRootRoute, createRoute, createRouter, Navigate, Outlet, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import { getHealth, getMe, Unauthorized } from "@/api";
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
import { Setup } from "@/routes/Setup";

// Gate shows setup while the hub is not set up, the login page without a
// session, and the app otherwise.
function Gate() {
  const { t } = useTranslation();
  const path = useRouterState({ select: (s) => s.location.pathname });
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth, refetchInterval: 30_000 });
  const setUp = health.data !== undefined && !health.data.setup;
  const me = useQuery({ queryKey: ["me"], queryFn: getMe, enabled: setUp, retry: false });

  if (health.isPending) return <Skeleton className="m-6 h-32" />;
  if (health.isError) return <p className="p-6 text-destructive">{t("app.unreachable")}</p>;
  if (health.data.setup) {
    return path === "/setup" ? <Outlet /> : <p className="mx-auto max-w-lg py-16">{t("setup.noToken")}</p>;
  }
  if (path === "/setup") return <Navigate to="/" />;
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

const settings = createRoute({ getParentRoute: () => root, path: "/settings", component: Settings });

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

export const router = createRouter({ routeTree: root.addChildren([overview, hostList, checkList, host, container, scope, check, alerts, alertDetail, settings, setup, login]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
