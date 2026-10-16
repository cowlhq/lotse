import { Link, type LinkProps } from "@tanstack/react-router";
import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";

export type Crumb = { label: string } & Pick<LinkProps, "to" | "params">;

// PageHeader is the top of a page: where it is, its views,
// and what can be done there.
export function PageHeader({ crumbs, tabs, actions }: { crumbs: Crumb[]; tabs?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mx-auto flex w-full max-w-7xl flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 pt-6 md:px-6">
      <div className="flex min-w-0 items-center gap-4">
        <nav className="flex min-w-0 items-center gap-1">
          {crumbs.map((c, i) => {
            const last = i === crumbs.length - 1;
            return (
              <span key={i} className="flex min-w-0 items-center gap-1">
                {i > 0 && <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/50" />}
                {c.to && !last ? (
                  <Link to={c.to} params={c.params} className="truncate rounded px-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">
                    {c.label}
                  </Link>
                ) : (
                  <span className={`truncate px-1 ${last ? "font-medium" : "text-muted-foreground"}`}>{c.label}</span>
                )}
              </span>
            );
          })}
        </nav>
        {tabs}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}

// Page lays out what is not a list: a column of readable width.
export function Page({ children }: { children: ReactNode; wide?: boolean }) {
  return <div className="mx-auto w-full max-w-7xl space-y-6 px-4 py-6 md:px-6">{children}</div>;
}

// Group is a heading over rows in a list: a name, a count, and what else
// it says to the right.
export function Group({ title, count, right }: { title: ReactNode; count?: number; right?: ReactNode }) {
  return (
    <div className="flex h-11 items-center gap-2 border-b bg-group px-4">
      <span className="flex min-w-0 items-center gap-2 font-medium">{title}</span>
      {count != null && <span className="text-muted-foreground tabular-nums">{count}</span>}
      {right && <span className="ml-auto flex items-center gap-3 text-mini text-muted-foreground">{right}</span>}
    </div>
  );
}

// Empty says that a list has nothing, and what to do about it.
export function Empty({ children }: { children: ReactNode }) {
  return <div className="px-6 py-12 text-center text-muted-foreground">{children}</div>;
}
