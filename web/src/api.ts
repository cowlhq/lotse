// Calls to the hub's API. The types come from api/openapi.yaml through
// schema.gen.ts (pnpm gen).
import type { components } from "./schema.gen";

type Schemas = components["schemas"];
export type Health = Schemas["Health"];
export type Host = Schemas["Host"];
export type HostSummary = Schemas["HostSummary"];
export type HostDetail = Schemas["HostDetail"];
export type HostReading = Schemas["HostReading"];
export type Filesystem = Schemas["Filesystem"];
export type Container = Schemas["Container"];
export type Series = Schemas["Series"];
export type Event = Schemas["Event"];

export type Range = { from: number; to: number };

export const getHealth = () => get<Health>("/api/health");
export const listHosts = () => get<HostSummary[]>("/api/hosts");
export const getHost = (id: number) => get<HostDetail>(`/api/hosts/${id}`);
export const getContainer = (id: number) => get<Container>(`/api/containers/${id}`);

export const getSeries = (kind: "hosts" | "filesystems" | "containers", id: number, r: Range) =>
  get<Series>(`/api/${kind}/${id}/series?from=${r.from}&to=${r.to}&points=600`);

export const listEvents = (subject: string, r: Range) =>
  get<Event[]>(`/api/events?subject=${subject}&from=${r.from}&to=${r.to}`);

export async function checkSetupToken(token: string): Promise<{ valid: boolean }> {
  const res = await fetch("/api/setup/check", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token }),
  });
  if (res.status === 409) return Promise.reject(new SetupDone());
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export class SetupDone extends Error {}

export class NotFound extends Error {}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (res.status === 404) throw new NotFound(path);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}
