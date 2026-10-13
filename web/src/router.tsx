import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createRootRoute, createRoute, createRouter, Link, Navigate, Outlet, useRouterState } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { getHealth, getMe, logout, Unauthorized } from "@/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Container } from "@/routes/Container";
import { Environment } from "@/routes/Environment";
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
  return <Layout name={me.data?.name || me.data?.email || ""} version={health.data.version} />;
}

function Layout({ name, version }: { name: string; version: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const out = useMutation({ mutationFn: logout, onSuccess: () => client.invalidateQueries({ queryKey: ["me"] }) });
  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
          <Link to="/" className="text-lg font-semibold tracking-tight">
            {t("app.name")}
          </Link>
          <div className="flex items-center gap-4 text-sm">
            <span className="text-xs text-muted-foreground">{version}</span>
            <span>{name}</span>
            <Button variant="ghost" size="sm" onClick={() => out.mutate()}>
              {t("login.logout")}
            </Button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-6 py-6">
        <Outlet />
      </main>
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

const environment = createRoute({
  getParentRoute: () => root,
  path: "/environments/$id",
  component: function EnvironmentRoute() {
    const { id } = environment.useParams();
    return <Environment id={Number(id)} />;
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

export const router = createRouter({ routeTree: root.addChildren([overview, host, container, environment, setup, login]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
