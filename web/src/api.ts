// Calls to the hub's API. The types come from api/openapi.yaml through
// schema.gen.ts (pnpm gen).
import type { components } from "./schema.gen";

export type Health = components["schemas"]["Health"];

export async function getHealth(): Promise<Health> {
  return get("/api/health");
}

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

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}
