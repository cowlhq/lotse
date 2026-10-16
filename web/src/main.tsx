import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { ThemeProvider } from "next-themes";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ApiError, NotFound, Unauthorized } from "./api";
import "./i18n";
import "./index.css";
import { router } from "./router";

// A request that failed for a reason that stays, such as a page that is not
// there, is not tried again: the page says so at once, not after seconds of
// retries. Lost connections and a hub restarting are tried again.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, err) => !(err instanceof NotFound || err instanceof Unauthorized || (err instanceof ApiError && err.status < 500)) && count < 3,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);
