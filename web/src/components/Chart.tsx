import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import type { Event, Series } from "@/api";
import { digitsFor } from "@/lib/format";
import { chrome, series as palette, status } from "@/lib/palette";

export type Line = {
  // column of the series
  key: string;
  label: string;
  // turns the stored value into the plotted one (bytes to percent, …)
  map?: (v: number) => number;
};

type Props = {
  data: Series | undefined;
  lines: Line[];
  // formats values on the axis and in the legend; on the axis, digits is
  // how many decimals the ticks need to differ
  format: (v: number | null, digits?: number) => string;
  range: { from: number; to: number };
  events?: Event[];
  // fixed top of the scale, such as 100 for percent
  max?: number;
  // ticks at 1, 2 and 5 times powers of 1024, for bytes
  binary?: boolean;
  height?: number;
};

// Events that mean something went wrong get the critical color; the rest
// are muted.
const badEvents = ["container.die", "container.oom", "container.health_status: unhealthy", "agent.disconnected", "host.reboot"];

// Chart draws a time series with uPlot: one y-axis, at most three lines in
// the palette's fixed order, a crosshair whose values show in the legend,
// and events as hairlines across the plot.
// Tick steps for byte scales: 1, 2 and 5 times each power of 1024.
const binaryIncrs = Array.from({ length: 7 }, (_, i) => [1, 2, 5, 10, 20, 50, 100, 200, 500].map((m) => m * 1024 ** i)).flat();

export function Chart({ data, lines, format, range, events = [], max, binary, height = 180 }: Props) {
  const { t } = useTranslation();
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = box.current;
    if (!el || !data) return;
    const columns = lines.map((l) =>
      (data.columns[l.key] ?? []).map((v) => (v == null ? null : l.map ? l.map(v) : v)),
    );
    const plotted: uPlot.AlignedData = [data.time, ...columns];
    const opts: uPlot.Options = {
      width: el.clientWidth,
      height,
      cursor: { points: { size: 8 }, drag: { x: false, y: false } },
      // The legend names the lines and shows their values under the cursor.
      legend: { show: true, live: true },
      scales: {
        x: { time: true, min: range.from, max: range.to },
        y: { range: (_u, _min, dataMax) => [0, max ?? (dataMax > 0 ? dataMax * 1.1 : 1)] },
      },
      axes: [
        { stroke: chrome.muted, grid: { stroke: chrome.grid, width: 1 }, ticks: { stroke: chrome.axis, width: 1 } },
        {
          stroke: chrome.muted,
          grid: { stroke: chrome.grid, width: 1 },
          ticks: { show: false },
          values: (_u, vals, _axis, _space, incr) => vals.map((v) => format(v, digitsFor(incr))),
          incrs: binary ? binaryIncrs : undefined,
          // as wide as the widest label, so none is cut off
          size: (u, values) => {
            if (!values) return 50;
            u.ctx.font = "12px system-ui, sans-serif";
            const widest = Math.max(...values.map((v) => u.ctx.measureText(v).width));
            return Math.ceil(widest / devicePixelRatio) + 24;
          },
        },
      ],
      series: [
        {},
        ...lines.map((l, i) => ({
          label: l.label,
          stroke: palette[i],
          width: 2,
          points: { show: false },
          value: (_u: uPlot, v: number | null) => format(v),
        })),
      ],
      hooks: {
        draw: [
          (u) => {
            const ctx = u.ctx;
            ctx.save();
            ctx.lineWidth = 1;
            for (const e of events) {
              if (e.ts < range.from || e.ts > range.to) continue;
              const x = Math.round(u.valToPos(e.ts, "x", true));
              ctx.strokeStyle = badEvents.includes(e.kind) ? status.critical : chrome.muted;
              ctx.setLineDash([3, 3]);
              ctx.beginPath();
              ctx.moveTo(x, u.bbox.top);
              ctx.lineTo(x, u.bbox.top + u.bbox.height);
              ctx.stroke();
            }
            ctx.restore();
          },
        ],
      },
    };
    const plot = new uPlot(opts, plotted, el);
    const resize = new ResizeObserver(() => plot.setSize({ width: el.clientWidth, height }));
    resize.observe(el);
    return () => {
      resize.disconnect();
      plot.destroy();
    };
  }, [data, lines, format, range.from, range.to, events, max, binary, height]);

  const empty = data !== undefined && lines.every((l) => (data.columns[l.key] ?? []).every((v) => v == null));
  return (
    <div className="relative w-full min-w-0">
      <div ref={box} />
      {empty && (
        <p className="absolute inset-x-0 top-1/3 text-center text-sm text-muted-foreground">{t("chart.empty")}</p>
      )}
    </div>
  );
}
