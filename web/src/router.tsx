import { createRootRoute, createRoute, createRouter, Outlet } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Home } from "./routes/Home";
import { Setup } from "./routes/Setup";

function Layout() {
  const { t } = useTranslation();
  return (
    <div className="mx-auto max-w-3xl p-8">
      <h1 className="mb-6 text-2xl font-semibold tracking-tight">{t("app.name")}</h1>
      <Outlet />
    </div>
  );
}

const root = createRootRoute({ component: Layout });

const home = createRoute({ getParentRoute: () => root, path: "/", component: Home });

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

export const router = createRouter({ routeTree: root.addChildren([home, setup]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
