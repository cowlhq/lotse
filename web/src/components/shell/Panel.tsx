import type { ReactNode } from "react";

// Panel is a framed part of a page, as the home page's tables are: a title,
// what it says on the right, and its content.
export function Panel({ title, count, right, children, flush = false }: { title?: ReactNode; count?: number; right?: ReactNode; children: ReactNode; flush?: boolean }) {
  return (
    <section className="overflow-hidden rounded-lg border bg-card">
      {(title || right) && (
        <header className="flex min-h-12 items-center gap-2 border-b px-4 py-2">
          {title && <h2 className="font-semibold">{title}</h2>}
          {count != null && <span className="text-muted-foreground tabular-nums">{count}</span>}
          {right && <div className="ml-auto flex items-center gap-2 text-muted-foreground">{right}</div>}
        </header>
      )}
      <div className={flush ? "" : "p-4"}>{children}</div>
    </section>
  );
}

// Facts is a line of small facts about an object, separated by dots.
export function Facts({ items }: { items: (ReactNode | false | null | undefined)[] }) {
  const shown = items.filter(Boolean);
  return (
    <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-muted-foreground sm:gap-x-2">
      {shown.map((x, i) => (
        <span key={i} className="flex items-center gap-2">
          {/* On a phone the facts wrap, and a dot would begin a line. */}
          {i > 0 && <span className="hidden text-muted-foreground/50 sm:inline">·</span>}
          {x}
        </span>
      ))}
    </p>
  );
}

// ObjectHeader is the top of a host's, container's or check's page: how it
// stands, its name, a line of facts, and what can be done with it.
export function ObjectHeader({ icon, title, facts, actions, above }: { icon: ReactNode; title: ReactNode; facts?: ReactNode; actions?: ReactNode; above?: ReactNode }) {
  return (
    <div className="mx-auto w-full max-w-7xl space-y-1.5 px-4 pt-6 md:px-6">
      {above && <div className="text-muted-foreground">{above}</div>}
      <div className="flex items-center gap-3">
        {icon}
        <h1 className="min-w-0 flex-1 truncate text-2xl font-semibold tracking-tight">{title}</h1>
        {actions && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
      </div>
      {facts}
    </div>
  );
}
