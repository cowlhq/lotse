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
                <span className="tabular-nums text-muted-foreground">{dateTime(c.at)}</span>
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

function What({ change: c }: { change: Change }) {
  const { t } = useTranslation();
  const show = (v: unknown) => (v == null ? t("history.nothing") : typeof v === "string" ? v : JSON.stringify(v));
  if (c.field === "created") return <>{t("history.created")}</>;
  if (c.field === "deleted") return <>{t("history.deleted")}</>;
  return (
    <>
      {t("history.changed", { field: t(`history.fields.${c.field}`, { defaultValue: c.field }) })}{" "}
      <span className="text-muted-foreground line-through">{show(c.old)}</span> → {show(c.new)}
    </>
  );
}
