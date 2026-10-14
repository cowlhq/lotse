import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, Wrench } from "lucide-react";
import { useTranslation } from "react-i18next";
import { acknowledgeAlert, type AlertState } from "@/api";
import { value } from "@/components/Alerts";
import { StatusIcon } from "@/components/shell/Dot";
import { Button } from "@/components/ui/button";
import { alertTitle, subjectName } from "@/lib/alerts";
import { ago, dateTime } from "@/lib/format";
import { levelOf } from "@/lib/status";
import { useMe } from "@/lib/useMe";

// Incidents lists what needs attention, worst first, each as one line:
// what, on what, how bad, since when; and who looks after it.
export function Incidents({ alerts }: { alerts: AlertState[] }) {
  return (
    <ul>
      {alerts.map((a) => (
        <Incident key={`${a.rule}|${a.subject_kind}|${a.subject_id}|${a.instance}`} alert={a} />
      ))}
    </ul>
  );
}

function Incident({ alert: a }: { alert: AlertState }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const ack = useMutation({
    mutationFn: () => acknowledgeAlert(a.alert_id),
    onSuccess: () => client.invalidateQueries({ queryKey: ["alerts"] }),
  });
  const { name, where } = subjectName(a);
  const firing = a.state === "crit" || a.state === "warn";
  return (
    <li className="group flex h-13 items-center gap-3 border-b px-4 transition-colors duration-100 last:border-b-0 hover:bg-accent/60">
      <StatusIcon level={levelOf(a)} />
      <Link to="/alerts/$id" params={{ id: String(a.alert_id) }} className="flex min-w-0 flex-1 items-baseline gap-2">
        <span className="shrink-0 font-medium">{alertTitle(t, a)}</span>
        <span className="truncate">
          {name}
          {a.instance && <span className="font-mono text-mini"> {a.instance}</span>}
        </span>
        <span className="hidden truncate text-muted-foreground lg:inline">
          {where}
          {a.reason && <> · {a.reason}</>}
        </span>
      </Link>
      <div className="flex shrink-0 items-center gap-4 text-mini text-muted-foreground">
        {a.value != null && a.unit && (
          <span className="tabular-nums">
            <span className="text-foreground">{value(a.value, a.unit)}</span>
            {a.threshold != null && <> / {value(a.threshold, a.unit)}</>}
          </span>
        )}
        {a.in_maintenance && (
          <span title={t("alert.inMaintenance")}>
            <Wrench className="size-3.5" />
          </span>
        )}
        {a.acked_by ? (
          <span className="inline-flex items-center gap-1" title={t("alert.ackedBy", { who: a.acked_by })}>
            <Check className="size-3.5" />
            {a.acked_by.split("@")[0]}
          </span>
        ) : (
          isAdmin &&
          firing && (
            <Button size="xs" variant="outline" className="transition-opacity md:opacity-0 md:group-hover:opacity-100 md:focus-visible:opacity-100" disabled={ack.isPending} onClick={() => ack.mutate()}>
              {t("alert.acknowledge")}
            </Button>
          )
        )}
        <span className="w-14 text-right tabular-nums" title={dateTime(a.since)}>
          {ago(a.since)}
        </span>
      </div>
    </li>
  );
}
