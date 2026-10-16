import type { ReactNode } from "react";
import { Wordmark } from "@/components/shell/Sidebar";

// Centered is the frame of what comes before the app: logging in and
// setting up. The logo, a title, and one panel in the middle.
export function Centered({ title, subtitle, children, wide = false }: { title: string; subtitle?: ReactNode; children: ReactNode; wide?: boolean }) {
  return (
    <div className="flex min-h-dvh flex-col items-center bg-sidebar px-4 py-16 sm:py-24">
      <div className={`w-full space-y-8 ${wide ? "max-w-lg" : "max-w-sm"}`}>
        <div className="flex justify-center">
          <Wordmark className="h-8" />
        </div>
        <div className="space-y-1.5 text-center">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          {subtitle && <p className="text-muted-foreground">{subtitle}</p>}
        </div>
        <div className="rounded-xl border bg-background p-6 shadow-sm">{children}</div>
      </div>
    </div>
  );
}
