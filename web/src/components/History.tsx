import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { listChanges, type Change } from "@/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { dateTime } from "@/lib/format";

// History shows who changed an object, when, how, and from what to what.
export function History({ subject }: { subject: string }) {
  const { t } = useTranslation();
  const changes = useQuery({ queryKey: ["changes", subject], queryFn: () => listChanges(subject) });
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("history.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {changes.data?.length ? (
          <ul className="space-y-2 text-sm">
            {changes.data.map((c) => (
              <li key={c.id} className="grid grid-cols-[11rem_1fr] gap-4">
                <span className="tabular-nums whitespace-nowrap text-muted-foreground">{dateTime(c.at)}</span>
                <span>
                  <What change={c} />
                  <span className="text-muted-foreground"> · {t("history.by", { who: c.actor_name, via: t(`history.via.${c.via}`) })}</span>
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">{t("history.empty")}</p>
        )}
      </CardContent>
    </Card>
  );
}

const events = ["enrolled", "enrolled again", "agent revoked", "enrollment token created", "enrollment token replaced"];

function What({ change: c }: { change: Change }) {
  const { t } = useTranslation();
  const show = (v: unknown) => (v == null ? t("history.nothing") : typeof v === "string" ? v : JSON.stringify(v));
  if (c.field === "created") return <>{t("history.created")}</>;
  if (c.field.startsWith("alert ") && c.new === "acknowledged") return <>{t("history.acknowledged", { alert: c.field.slice(6) })}</>;
  if (c.field === "api token created" || c.field === "api token revoked") {
    const x = (c.new ?? c.old) as { name?: string };
    return <>{t(c.new ? "history.tokenCreated" : "history.tokenRevoked", { name: x.name })}</>;
  }
  if (c.field === "maintenance") {
    const w = (c.new ?? c.old) as { schedule?: string; starts_at?: number; ends_at?: number; reason?: string };
    const what = (w.schedule ?? `${dateTime(w.starts_at ?? 0)} – ${dateTime(w.ends_at ?? 0)}`) + (w.reason ? `, ${w.reason}` : "");
    return <>{t(c.new ? "history.maintenanceAdded" : "history.maintenanceRemoved", { what })}</>;
  }
  if (c.field === "deleted") return <>{t("history.deleted")}</>;
  // Things that happened rather than values that changed: the label, and
  // where to if there is a where.
  if (events.includes(c.field)) {
    return (
      <>
        {t(`history.fields.${c.field}`, { defaultValue: c.field })}
        {c.new != null && <> {t("history.into", { where: show(c.new) })}</>}
      </>
    );
  }
  return (
    <>
      {t("history.changed", { field: t(`history.fields.${c.field}`, { defaultValue: c.field }) })}{" "}
      <span className="text-muted-foreground line-through">{show(c.old)}</span> → {show(c.new)}
    </>
  );
}
