import { useTranslation } from "react-i18next";
import type { Event } from "@/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { dateTime } from "@/lib/format";

// Events lists what happened to a host or container, newest first.
export function Events({ events }: { events: Event[] }) {
  const { t } = useTranslation();
  if (events.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("events.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-1 text-sm">
          {[...events].reverse().map((e) => (
            <li key={e.id} className="grid grid-cols-[11rem_1fr] gap-4">
              <span className="tabular-nums text-muted-foreground">{dateTime(e.ts)}</span>
              <span>
                {t(`event.${e.kind}`, { defaultValue: e.kind, ...(e.data as Record<string, unknown>), code: e.data.exit_code })}
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
