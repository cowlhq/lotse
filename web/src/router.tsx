import { useQuery } from "@tanstack/react-query";
import { createRootRoute, createRoute, createRouter, Link, Outlet } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { getHealth } from "@/api";
import { Container } from "@/routes/Container";
import { Host } from "@/routes/Host";
import { Overview } from "@/routes/Overview";
import { Setup } from "@/routes/Setup";

function Layout() {
  const { t } = useTranslation();
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth, refetchInterval: 30_000 });
  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
          <Link to="/" className="text-lg font-semibold tracking-tight">
            {t("app.name")}
          </Link>
          {health.data && <span className="text-xs text-muted-foreground">{health.data.version}</span>}
        </div>
      </header>
      {health.data?.setup && (
        <div className="border-b bg-muted/60">
          <p className="mx-auto max-w-6xl px-6 py-2 text-sm">{t("home.notSetUp")}</p>
        </div>
      )}
      <main className="mx-auto max-w-6xl px-6 py-6">
        <Outlet />
      </main>
    </div>
  );
}

const root = createRootRoute({ component: Layout });

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

export const router = createRouter({ routeTree: root.addChildren([overview, host, container, setup]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
