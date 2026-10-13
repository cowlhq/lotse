import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { listChecks } from "@/api";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ago, number } from "@/lib/format";
import { status } from "@/lib/palette";
import { certDays, CheckState, target } from "./CheckState";

// CheckList shows an environment's checks with their state.
export function CheckList({ envID }: { envID: number }) {
  const { t } = useTranslation();
  const checks = useQuery({ queryKey: ["checks", envID], queryFn: () => listChecks(envID), refetchInterval: 15_000 });
  if (!checks.data?.length) return <p className="text-sm text-muted-foreground">{t("check.none")}</p>;
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("check.name")}</TableHead>
          <TableHead>{t("check.stateTitle")}</TableHead>
          <TableHead>{t("check.target")}</TableHead>
          <TableHead className="text-right">{t("check.latency")}</TableHead>
          <TableHead className="text-right">{t("check.cert")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {checks.data.map((c) => {
          const days = certDays(c);
          return (
            <TableRow key={c.id}>
              <TableCell>
                <Link to="/checks/$id" params={{ id: String(c.id) }} className="font-medium hover:underline">
                  {c.name}
                </Link>
                <span className="ml-2 text-xs text-muted-foreground">{t(`check.kind.${c.type}`)}</span>
              </TableCell>
              <TableCell>
                <CheckState state={c.state} />
                {c.state === "down" && c.error && <span className="ml-2 text-xs text-muted-foreground">{c.error}</span>}
              </TableCell>
              <TableCell className="max-w-72 truncate text-muted-foreground">{target(c)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {c.type === "heartbeat" ? (c.last_ping ? t("ago", { time: ago(c.last_ping) }) : "–") : c.latency_ms != null ? `${number(c.latency_ms, 0)} ms` : "–"}
              </TableCell>
              <TableCell className="text-right tabular-nums" style={days != null && days < 14 ? { color: status.critical } : undefined}>
                {days != null ? t("check.days", { count: days }) : "–"}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
