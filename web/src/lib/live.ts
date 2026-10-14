import { useEffect, useRef } from "react";
import type { HostReading, Series } from "@/api";
import type { components } from "@/schema.gen";

// The live connection to the hub: one WebSocket for the whole tab. Pages
// say which topics they show ("hosts", "host:3"); the hub sends what
// happens to them, and while a host is watched, its agent reads every few
// seconds.

export type LiveMessage = {
  type: "host" | "host_state" | "container_stats" | "containers" | "update" | "alerts";
  host_id: number;
  reading?: HostReading;
  online?: boolean;
  containers?: { container_id: number; reading: components["schemas"]["ContainerReading"] }[];
  update?: { version: string; state: "started" | "installed" | "failed"; error?: string };
};

type Listener = { topics: string[]; onMessage: (m: LiveMessage) => void };

const listeners = new Set<Listener>();
let socket: WebSocket | null = null;
let retry = 1000;

function topics() {
  return [...new Set([...listeners].flatMap((l) => l.topics))];
}

function subscribe() {
  if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ subscribe: topics() }));
}

function connect() {
  const url = `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/api/live`;
  const ws = new WebSocket(url);
  socket = ws;
  ws.onopen = () => {
    retry = 1000;
    subscribe();
  };
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data) as LiveMessage;
    for (const l of listeners) l.onMessage(m);
  };
  ws.onclose = () => {
    if (socket !== ws) return;
    socket = null;
    // Losing the hub for a moment is normal: try again, waiting longer
    // each time, while anyone still listens.
    if (listeners.size > 0) setTimeout(connect, retry);
    retry = Math.min(retry * 2, 30_000);
  };
}

// useLive calls onMessage for what happens to topics while the component
// is shown.
export function useLive(topicList: string[], onMessage: (m: LiveMessage) => void) {
  const handler = useRef(onMessage);
  useEffect(() => {
    handler.current = onMessage;
  });
  const key = topicList.join(",");
  useEffect(() => {
    const l: Listener = { topics: key ? key.split(",") : [], onMessage: (m) => handler.current(m) };
    listeners.add(l);
    if (!socket) connect();
    else subscribe();
    return () => {
      listeners.delete(l);
      subscribe();
      if (listeners.size === 0) {
        const ws = socket;
        socket = null;
        ws?.close();
      }
    };
  }, [key]);
}

// appendReading adds a reading to a series, if it is newer than its last
// point.
export function appendReading(s: Series | undefined, ts: number, values: Record<string, number | null | undefined>) {
  if (!s || (s.time.length > 0 && ts <= s.time[s.time.length - 1])) return s;
  const columns: Series["columns"] = {};
  for (const [k, col] of Object.entries(s.columns)) columns[k] = [...col, values[k] ?? null];
  return { ...s, time: [...s.time, ts], columns };
}
