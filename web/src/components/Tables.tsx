import { useNavigate } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, GripVertical } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { AlertState, Check, HostSummary } from "@/api";
import { AlertBell } from "@/components/AlertBell";
import { certDays, target } from "@/components/CheckState";
import { checkLevel, hostLevel } from "@/components/Rows";
import { StatusIcon } from "@/components/shell/Dot";
import { UptimeBar } from "@/components/UptimeBar";
import { ago, rate } from "@/lib/format";

// Group puts rows under headings, such as an environment's name.
export type Grouping<T> = (x: T) => { key: string; label: ReactNode };

function GroupRow({ label, span, drop, hint }: { label: ReactNode; span: number; drop?: DropProps; hint?: ReactNode }) {
  return (
    <tr className={`border-b transition-colors ${drop?.over ? "bg-brand/10" : "bg-group"}`} {...drop?.handlers}>
      <td colSpan={span} className={`px-4 py-2 font-medium ${drop?.over ? "shadow-[inset_0_0_0_2px_var(--brand)]" : ""}`}>
        {label}
        {hint && <span className="ml-2 text-mini font-normal text-muted-foreground">{hint}</span>}
      </td>
    </tr>
  );
}

// MoveTo lets hosts be dragged from one group to another: the groups there
// are, such as every environment of a customer, empty ones too, and what a
// drop does.
export type MoveTo = { groups: { key: string; label: ReactNode; envID: number }[]; onMove: (h: HostSummary, envID: number) => void };

type DropProps = { over: boolean; handlers: Pick<React.HTMLAttributes<HTMLElement>, "onDragOver" | "onDragLeave" | "onDrop"> };

// The two tables of the home page, as Beszel has one: every host with its
// load as bars, and every check with its last day. A row leads to the
// host or check; the bell at its end sets its alerts.

type Sort<K extends string> = { key: K; desc: boolean };

function Head<K extends string>({ k, sort, setSort, children, className = "" }: { k: K; sort: Sort<K>; setSort: (s: Sort<K>) => void; children: ReactNode; className?: string }) {
  const on = sort.key === k;
  return (
    <th className={`h-10 px-3 font-medium whitespace-nowrap text-muted-foreground first:pl-4 last:pr-4 ${className}`}>
      <button type="button" className={`inline-flex items-center gap-1 hover:text-foreground ${on ? "text-foreground" : ""}`} onClick={() => setSort({ key: k, desc: on ? !sort.desc : k !== "name" })}>
        {children}
        {on && (sort.desc ? <ArrowDown className="size-3.5" /> : <ArrowUp className="size-3.5" />)}
      </button>
    </th>
  );
}

const rank = { crit: 0, warn: 1, unknown: 2, ok: 3 };

// Bar is a share as Beszel shows it: the number, and a bar that turns amber
// and red as it fills.
export function Bar({ pct }: { pct: number | null | undefined }) {
  if (pct == null) return <span className="text-muted-foreground">–</span>;
  const color = pct >= 90 ? "var(--crit)" : pct >= 75 ? "var(--warn)" : "var(--ok)";
  return (
    <div className="flex items-center gap-2.5">
      <span className="w-9 text-right tabular-nums">{pct < 10 ? pct.toFixed(1) : Math.round(pct)}%</span>
      <span className="relative h-1.5 w-full min-w-12 overflow-hidden rounded-full bg-[color-mix(in_oklch,var(--foreground)_10%,transparent)]">
        <span className="absolute inset-y-0 left-0 rounded-full" style={{ width: `${Math.min(100, Math.max(1.5, pct))}%`, background: color }} />
      </span>
    </div>
  );
}

type HostKey = "name" | "cpu" | "mem" | "disk" | "net" | "containers";

export function HostTable({ hosts, alerts, where, group, move }: { hosts: HostSummary[]; alerts: AlertState[]; where: (h: HostSummary) => string; group?: Grouping<HostSummary>; move?: MoveTo }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [sort, setSort] = useState<Sort<HostKey>>({ key: "name", desc: false });
  const [dragging, setDragging] = useState<HostSummary | null>(null);
  const [over, setOver] = useState<string | null>(null);
  // While a host is dragged, every group takes it but its own.
  const drop = (key: string): DropProps | undefined => {
    if (!move || !dragging || !group || group(dragging).key === key) return undefined;
    return {
      over: over === key,
      handlers: {
        onDragOver: (e) => (e.preventDefault(), (e.dataTransfer.dropEffect = "move"), setOver(key)),
        onDragLeave: () => setOver((o) => (o === key ? null : o)),
        onDrop: (e) => {
          e.preventDefault();
          const target = move.groups.find((g) => g.key === key);
          if (target) move.onMove(dragging, target.envID);
          setDragging(null);
          setOver(null);
        },
      },
    };
  };
  const mem = (h: HostSummary) => (h.latest?.mem_used != null && h.host.mem_total > 0 ? (100 * h.latest.mem_used) / h.host.mem_total : null);
  const net = (h: HostSummary) => (h.latest ? (h.latest.net_rx ?? 0) + (h.latest.net_tx ?? 0) : null);
  const value: Record<HostKey, (h: HostSummary) => number | string | null> = {
    name: (h) => h.host.name,
    cpu: (h) => h.latest?.cpu ?? null,
    mem,
    disk: (h) => h.disk ?? null,
    net,
    containers: (h) => h.containers.running,
  };
  const sorted = [...hosts].sort((a, b) => {
    if (group) {
      const g = group(a).key.localeCompare(group(b).key);
      if (g !== 0) return g;
    }
    // What needs attention stays on top, whatever the order.
    const r = rank[hostLevel(a, alerts)] - rank[hostLevel(b, alerts)];
    if (r !== 0) return r;
    const x = value[sort.key](a) ?? -1;
    const y = value[sort.key](b) ?? -1;
    const c = typeof x === "string" ? x.localeCompare(String(y)) : (x as number) - (y as number);
    return sort.desc ? -c : c;
  });
  const p = { sort, setSort };
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left">
        <thead className="border-b">
          <tr>
            <Head k="name" {...p}>{t("table.host")}</Head>
            <Head k="cpu" {...p} className="w-[13%]">{t("metric.cpu")}</Head>
            <Head k="mem" {...p} className="hidden w-[13%] sm:table-cell">{t("metric.memory")}</Head>
            <Head k="disk" {...p} className="hidden w-[13%] sm:table-cell">{t("metric.disk")}</Head>
            <Head k="net" {...p} className="hidden w-[11%] lg:table-cell">{t("table.network")}</Head>
            <Head k="containers" {...p} className="hidden w-[9%] md:table-cell">{t("table.containers")}</Head>
            <th className="w-12" />
          </tr>
        </thead>
        <tbody>
          {sorted.map((h, i) => {
            const go = () => navigate({ to: "/hosts/$id", params: { id: String(h.host.id) } });
            const g = group?.(h);
            const heading = g && (i === 0 || group!(sorted[i - 1]).key !== g.key) ? <GroupRow key={`g-${g.key}`} label={g.label} span={7} drop={drop(g.key)} hint={drop(g.key) && t("move.here")} /> : null;
            const target = g ? drop(g.key) : undefined;
            return [
              heading,
              <tr
                key={h.host.id}
                onClick={go}
                draggable={!!move}
                onDragStart={(e) => {
                  e.dataTransfer.effectAllowed = "move";
                  e.dataTransfer.setData("text/plain", h.host.name);
                  setDragging(h);
                }}
                onDragEnd={() => (setDragging(null), setOver(null))}
                {...target?.handlers}
                className={`group/row cursor-pointer border-b transition-colors last:border-b-0 hover:bg-accent/60 ${dragging?.host.id === h.host.id ? "opacity-40" : ""} ${target?.over ? "bg-brand/5" : ""}`}
              >
                <td className="relative py-3 pr-3 pl-4">
                  {move && <GripVertical className="absolute top-1/2 left-0.5 size-3.5 -translate-y-1/2 text-muted-foreground opacity-0 group-hover/row:opacity-60" aria-hidden />}
                  <div className="flex items-center gap-2.5">
                    <StatusIcon level={hostLevel(h, alerts)} />
                    <div className="min-w-0">
                      <p className="truncate font-medium">{h.host.name}</p>
                      <p className="truncate text-mini text-muted-foreground">
                        {h.host.waiting ? t("status.waiting") : h.host.online ? where(h) : h.host.agent_revoked ? t("status.revoked") : t("rows.offlineSince", { time: ago(h.host.last_seen) })}
                      </p>
                    </div>
                  </div>
                </td>
                <td className="px-3">{h.host.online ? <Bar pct={h.latest?.cpu} /> : null}</td>
                <td className="hidden px-3 sm:table-cell">{h.host.online ? <Bar pct={mem(h)} /> : null}</td>
                <td className="hidden px-3 sm:table-cell">{h.host.online ? <Bar pct={h.disk} /> : null}</td>
                <td className="hidden px-3 tabular-nums whitespace-nowrap lg:table-cell">{h.host.online ? rate(net(h)) : null}</td>
                <td className="hidden px-3 tabular-nums md:table-cell">
                  {h.containers.total > 0 && (
                    <>
                      {h.containers.running}
                      <span className="text-muted-foreground">/{h.containers.total}</span>
                    </>
                  )}
                </td>
                <td className="pr-3 text-right" onClick={(e) => e.stopPropagation()}>
                  <AlertBell subject={`host:${h.host.id}`} name={h.host.name} kind="host" />
                </td>
              </tr>,
            ];
          })}
          {/* Groups with no host yet appear while one is dragged. */}
          {dragging &&
            move?.groups
              .filter((g) => !sorted.some((h) => group?.(h).key === g.key))
              .map((g) => [
                <GroupRow key={`e-${g.key}`} label={g.label} span={7} drop={drop(g.key)} />,
                <tr key={`p-${g.key}`} className="border-b last:border-b-0" {...drop(g.key)?.handlers}>
                  <td colSpan={7} className="px-4 py-3 text-mini text-muted-foreground">
                    {t("move.empty")}
                  </td>
                </tr>,
              ])}
        </tbody>
      </table>
    </div>
  );
}

type CheckKey = "name" | "latency" | "cert";

export function CheckTable({ checks, where, group }: { checks: Check[]; where: (c: Check) => string; group?: Grouping<Check> }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [sort, setSort] = useState<Sort<CheckKey>>({ key: "name", desc: false });
  const value: Record<CheckKey, (c: Check) => number | string | null> = {
    name: (c) => c.name,
    latency: (c) => c.latency_ms ?? null,
    cert: (c) => certDays(c) ?? null,
  };
  const sorted = [...checks].sort((a, b) => {
    if (group) {
      const g = group(a).key.localeCompare(group(b).key);
      if (g !== 0) return g;
    }
    const r = rank[checkLevel(a)] - rank[checkLevel(b)];
    if (r !== 0) return r;
    const x = value[sort.key](a) ?? -1;
    const y = value[sort.key](b) ?? -1;
    const c = typeof x === "string" ? x.localeCompare(String(y)) : (x as number) - (y as number);
    return sort.desc ? -c : c;
  });
  const p = { sort, setSort };
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left">
        <thead className="border-b">
          <tr>
            <Head k="name" {...p}>{t("table.check")}</Head>
            <th className="hidden h-10 px-3 font-medium text-muted-foreground md:table-cell">{t("table.last24h")}</th>
            <Head k="latency" {...p} className="w-[10%]">{t("table.response")}</Head>
            <Head k="cert" {...p} className="hidden w-[10%] sm:table-cell">{t("table.certificate")}</Head>
            <th className="w-12" />
          </tr>
        </thead>
        <tbody>
          {sorted.map((c, i) => {
            const days = certDays(c);
            const g = group?.(c);
            const heading = g && (i === 0 || group!(sorted[i - 1]).key !== g.key) ? <GroupRow key={`g-${g.key}`} label={g.label} span={5} /> : null;
            return [
              heading,
              <tr key={c.id} onClick={() => navigate({ to: "/checks/$id", params: { id: String(c.id) } })} className="cursor-pointer border-b transition-colors last:border-b-0 hover:bg-accent/60">
                <td className="py-3 pr-3 pl-4">
                  <div className="flex items-center gap-2.5">
                    <StatusIcon level={checkLevel(c)} />
                    <div className="min-w-0">
                      <p className="truncate font-medium">{c.name}</p>
                      <p className="truncate text-mini text-muted-foreground">
                        {[where(c), t(`check.kind.${c.type}`), target(c)].filter(Boolean).join(" · ")}
                      </p>
                    </div>
                  </div>
                </td>
                <td className="hidden w-[38%] px-3 md:table-cell">
                  {c.type !== "heartbeat" ? <UptimeBar checkID={c.id} /> : <span className="text-mini text-muted-foreground">{c.last_ping ? t("rows.lastPing", { time: ago(c.last_ping) }) : t("rows.noPing")}</span>}
                </td>
                <td className="px-3 tabular-nums whitespace-nowrap">{c.latency_ms != null ? `${Math.round(c.latency_ms)} ms` : <span className="text-muted-foreground">–</span>}</td>
                <td className="hidden px-3 tabular-nums whitespace-nowrap sm:table-cell" style={days != null && days < 14 ? { color: "var(--warn)" } : undefined}>
                  {days != null ? t("check.days", { count: days }) : <span className="text-muted-foreground">–</span>}
                </td>
                <td className="pr-3 text-right" onClick={(e) => e.stopPropagation()}>
                  <AlertBell subject={`check:${c.id}`} name={c.name} kind={c.type === "heartbeat" ? "heartbeat" : "check"} />
                </td>
              </tr>,
            ];
          })}
        </tbody>
      </table>
    </div>
  );
}
