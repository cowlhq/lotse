import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import type { Event, Series } from "@/api";
import { digitsFor } from "@/lib/format";
import { useTheme } from "next-themes";
import { chrome, css, series as palette, status } from "@/lib/palette";

export type Line = {
  // column of the series
  key: string;
  label: string;
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

// Tick steps for byte scales: 1, 2 and 5 times each power of 1024.
const binaryIncrs = Array.from({ length: 7 }, (_, i) => [1, 2, 5, 10, 20, 50, 100, 200, 500].map((m) => m * 1024 ** i)).flat();

function plotted(data: Series | undefined, lines: Line[]): uPlot.AlignedData {
  if (!data) return [[], ...lines.map(() => [])];
  return [data.time, ...lines.map((l) => data.columns[l.key] ?? [])];
}

// xRange runs to now, or to the last reading if live readings ran past it.
// It begins with the first reading when there is nothing in the range's
// first tenth, so that a week of a host added yesterday is not an empty
// chart with a line at its end.
function xRange(data: Series | undefined, range: { from: number; to: number }) {
  const first = data?.time[0];
  const last = data?.time[data.time.length - 1] ?? 0;
  const max = Math.max(range.to, last);
  const min = first != null && first > range.from + (max - range.from) / 10 ? first : range.from;
  return { min, max };
}

// lastValue is the newest reading of a line, which the legend shows while
// the cursor is not on the plot.
function lastValue(values: uPlot.AlignedData[number]) {
  for (let i = values.length - 1; i >= 0; i--) {
    const v = values[i];
    if (v != null) return v as number;
  }
  return null;
}

// Times on the axis, in the reader's own format: hours and minutes within a
// day, the weekday with them over a few days, and dates beyond.
const day = 86400;
const hourMinute = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const weekdayTime = new Intl.DateTimeFormat(undefined, { weekday: "short", hour: "2-digit", minute: "2-digit" });
const date = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short" });

function timeTicks(u: uPlot, vals: number[], _axis: number, _space: number, incr: number) {
  const { min = 0, max = 0 } = u.scales.x;
  const f = incr >= day ? date : max - min > day ? weekdayTime : hourMinute;
  return vals.map((v) => f.format(v * 1000));
}

// labelWidth is how wide an axis label is, in CSS pixels. It is measured on
// a canvas of its own: setting the plot's font would leave uPlot drawing
// with it, unscaled, which on a screen of twice the density is half the
// size.
const measure = document.createElement("canvas").getContext("2d")!;
function labelWidth(text: string): number {
  measure.font = "12px 'Fira Sans', system-ui, sans-serif";
  return measure.measureText(text).width;
}

// Events are drawn up to these ranges; beyond, there are too many to read.
// Those that mean trouble stay longer than the rest.
const eventsUpTo = 6 * 3600;
const badEventsUpTo = 2 * day;

// Chart draws a time series with uPlot: one y-axis, at most three lines in
// the palette's fixed order, a crosshair whose values show in the legend,
// and events as hairlines across the plot. The plot is made once per set
// of lines; new readings only replace its data.
export function Chart({ data, lines, format, range, events = [], max, binary, height = 180 }: Props) {
  const { t } = useTranslation();
  // The plot is drawn again in the colors of a new theme.
  const { resolvedTheme } = useTheme();
  const box = useRef<HTMLDivElement>(null);
  const plot = useRef<uPlot | null>(null);
  const eventsRef = useRef(events);
  const latest = useRef({ data, range });
  useEffect(() => {
    eventsRef.current = events;
    latest.current = { data, range };
  });

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const opts: uPlot.Options = {
      width: el.clientWidth,
      height,
      cursor: { points: { size: 8 }, drag: { x: false, y: false } },
      // The legend names the lines and shows their values under the cursor.
      legend: { show: true, live: true },
      scales: {
        x: { time: true, range: (_u, min, max) => [min, max] },
        y: { range: (_u, _min, dataMax) => [0, max ?? (dataMax > 0 ? dataMax * 1.1 : 1)] },
      },
      axes: [
        {
          font: "12px 'Fira Sans', system-ui, sans-serif",
          stroke: css(chrome.muted),
          grid: { stroke: css(chrome.grid), width: 1 },
          ticks: { stroke: css(chrome.axis), width: 1 },
          values: timeTicks,
          // Over more than a day, a tick may need its weekday too, which
          // takes more room than a time alone.
          space: (_u, _axis, min, max) => (max - min > day ? 110 : 64),
          size: 32,
        },
        {
          font: "12px 'Fira Sans', system-ui, sans-serif",
          stroke: css(chrome.muted),
          grid: { stroke: css(chrome.grid), width: 1 },
          ticks: { show: false },
          values: (_u, vals, _axis, _space, incr) => vals.map((v) => format(v, digitsFor(incr))),
          incrs: binary ? binaryIncrs : undefined,
          // as wide as the widest label, so none is cut off
          size: (_u, values) => (values ? Math.ceil(Math.max(...values.map(labelWidth))) + 24 : 50),
        },
      ],
      series: [
        {},
        ...lines.map((l, i) => ({
          label: l.label,
          stroke: css(palette[i]),
          width: 2,
          // Dots while there are only a few readings, which lines alone
          // would not show.
          points: { show: (u: uPlot) => u.data[0].length <= 30, size: 5 },
          value: (u: uPlot, v: number | null, sidx: number, idx: number | null) => format(idx == null ? lastValue(u.data[sidx]) : v),
        })),
      ],
      hooks: {
        draw: [
          (u) => {
            const ctx = u.ctx;
            ctx.save();
            ctx.lineWidth = 1;
            ctx.setLineDash([3, 3]);
            const { min = 0, max = 0 } = u.scales.x;
            const span = max - min;
            for (const e of eventsRef.current) {
              if (e.ts < min || e.ts > max) continue;
              if (span > (badEvents.includes(e.kind) ? badEventsUpTo : eventsUpTo)) continue;
              const x = Math.round(u.valToPos(e.ts, "x", true));
              ctx.strokeStyle = css(badEvents.includes(e.kind) ? status.critical : chrome.muted);
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
    const p = new uPlot(opts, plotted(latest.current.data, lines), el);
    p.setScale("x", xRange(latest.current.data, latest.current.range));
    plot.current = p;
    const resize = new ResizeObserver(() => p.setSize({ width: el.clientWidth, height }));
    resize.observe(el);
    return () => {
      resize.disconnect();
      p.destroy();
      plot.current = null;
    };
  }, [lines, format, max, binary, height, resolvedTheme]);

  useEffect(() => {
    const p = plot.current;
    if (!p) return;
    p.setData(plotted(data, lines));
    p.setScale("x", xRange(data, range));
    // The legend shows the newest values again, unless the cursor is on
    // the plot.
    if (p.cursor.idx == null) p.setLegend({ idx: null as unknown as number });
  }, [data, lines, range, events]);

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
