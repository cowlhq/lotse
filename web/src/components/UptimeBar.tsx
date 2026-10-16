import { useQuery } from "@tanstack/react-query";
import { type CSSProperties, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { getSeries } from "@/api";
import { dateTime } from "@/lib/format";

// UptimeBar shows a check's last day as slices, each the share of runs that
// passed: green when all did, amber when some failed, red when most did,
// grey when nothing ran. The share over the day stands beside it.
export function UptimeBar({ checkID, slices = 48, hours = 24 }: { checkID: number; slices?: number; hours?: number }) {
  const { t } = useTranslation();
  const [barsRef, place] = useEvenSlices(slices);
  const [to] = useState(() => Math.floor(Date.now() / 1000));
  const series = useQuery({
    queryKey: ["uptime", checkID, slices, hours],
    queryFn: () => getSeries("checks", checkID, { from: to - hours * 3600, to }, slices),
    refetchInterval: 5 * 60_000,
  });
  const step = (hours * 3600) / slices;
  const shares = new Array<number | null>(slices).fill(null);
  const time = series.data?.time ?? [];
  const ok = series.data?.columns.ok ?? [];
  time.forEach((ts, i) => {
    const slot = Math.floor((ts - (to - hours * 3600)) / step);
    if (slot >= 0 && slot < slices && ok[i] != null) shares[slot] = ok[i];
  });
  const ran = shares.filter((v): v is number => v != null);
  const total = ran.length ? (100 * ran.reduce((a, b) => a + b, 0)) / ran.length : null;
  return (
    <div className="flex items-center gap-3">
      <div ref={barsRef} className="relative h-5 min-w-0 flex-1">
        <div className="absolute inset-0 flex items-stretch gap-px" aria-label={t("check.uptime")}>
          {shares.map((v, i) => {
            const color = v == null ? "var(--muted)" : v >= 0.999 ? "var(--ok)" : v >= 0.5 ? "var(--warn)" : "var(--crit)";
            const at = to - hours * 3600 + i * step;
            return (
              <span
                key={i}
                className="rounded-[1px] transition-opacity hover:opacity-50"
                style={{ background: color, ...place(i) }}
                title={`${dateTime(at)}: ${v == null ? t("uptime.noData") : `${(v * 100).toFixed(v >= 0.999 ? 0 : 1)} %`}`}
              />
            );
          })}
        </div>
      </div>
      <span className="w-16 text-right text-mini whitespace-nowrap tabular-nums text-muted-foreground">{total == null ? "–" : `${total >= 99.995 || total <= 0.005 ? total.toFixed(0) : total.toFixed(2)} %`}</span>
    </div>
  );
}

// useEvenSlices gives every slice the same whole number of device pixels.
// Slices sharing the width evenly would be some fraction wide, and the browser
// rounds each edge on its own, so neighbours came out a pixel apart and the
// gaps blurred. Each slice is placed on its own, counted from the box's right
// edge in device pixels; what is left over stays empty on the left. The
// slices sit absolutely in the measured box, so their width cannot feed back
// into the table column that box is measured in. Until it is measured, or
// when it is too narrow for a pixel each, they share the width as before.
function useEvenSlices(slices: number) {
  const ref = useRef<HTMLDivElement>(null);
  const [fit, setFit] = useState<{ dpr: number; left: number; right: number; bar: number; gap: number } | null>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => {
      const dpr = window.devicePixelRatio || 1;
      const box = el.getBoundingClientRect();
      const right = Math.floor(box.right * dpr);
      const gap = Math.max(1, Math.round(dpr));
      const bar = Math.floor((right - Math.ceil(box.left * dpr) - gap * (slices - 1)) / slices);
      setFit(bar >= 1 ? { dpr, left: box.left, right, bar, gap } : null);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [slices]);
  const place = (i: number): CSSProperties => {
    if (!fit) return { flex: "1 1 0" };
    const end = fit.right - (slices - 1 - i) * (fit.bar + fit.gap);
    return { position: "absolute", top: 0, bottom: 0, left: (end - fit.bar) / fit.dpr - fit.left, width: fit.bar / fit.dpr };
  };
  return [ref, place] as const;
}
