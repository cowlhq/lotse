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
export type User = Schemas["User"];
export type Scope = Schemas["Scope"];
export type ScopeName = Schemas["ScopeName"];
export type Change = Schemas["Change"];
export type ConfigValue = Schemas["ConfigValue"];
export type Enrollment = Schemas["Enrollment"];
export type Updates = Schemas["Updates"];
export type SetupStep = Schemas["SetupStep"];

export type Range = { from: number; to: number };

export const getHealth = () => get<Health>("/api/health");
export const listHosts = () => get<HostSummary[]>("/api/hosts");
export const getHost = (id: number) => get<HostDetail>(`/api/hosts/${id}`);
export const getContainer = (id: number) => get<Container>(`/api/containers/${id}`);

export const getSeries = (kind: "hosts" | "filesystems" | "containers", id: number, r: Range) =>
  get<Series>(`/api/${kind}/${id}/series?from=${r.from}&to=${r.to}&points=600`);

export const listEvents = (subject: string, r: Range) =>
  get<Event[]>(`/api/events?subject=${subject}&from=${r.from}&to=${r.to}`);

export const listScopes = () => get<Scope[]>("/api/scopes");
export const getScope = (id: number) => get<Scope>(`/api/scopes/${id}`);
export const renameScope = (id: number, name: string) => send<Scope>("PATCH", `/api/scopes/${id}`, { name });
export const deleteScope = (id: number) => send<void>("DELETE", `/api/scopes/${id}`);
export const createEnvironment = (levels: { customer: ScopeName; project: ScopeName; environment: ScopeName }) =>
  post<Scope>("/api/environments", levels);
export const setHostEnvironment = (hostID: number, environmentID: number | null) =>
  send<void>("PUT", `/api/hosts/${hostID}/environment`, { environment_id: environmentID });
export const getConfig = (subject: string) => get<ConfigValue[]>(`/api/config?subject=${subject}`);
export const setSetting = (subject: string, key: string, value: unknown) =>
  send<void>("PUT", "/api/settings", { subject, key, value });
export const getEnrollment = (envID: number) => get<Enrollment>(`/api/scopes/${envID}/enrollment`);
export const newEnrollment = (envID: number) => post<Enrollment>(`/api/scopes/${envID}/enrollment`);
export const revokeAgent = (hostID: number) => send<void>("DELETE", `/api/hosts/${hostID}/agent`);
export const getUpdates = () => get<Updates>("/api/updates");
export const checkUpdates = () => post<Updates>("/api/updates");
export const updateHost = (hostID: number) => post<void>(`/api/hosts/${hostID}/update`);
export const updateScope = (scopeID: number) => post<{ asked: number }>(`/api/scopes/${scopeID}/update`);
export const updateHub = () => post<void>("/api/hub/update");
export const listChanges = (subject: string) => get<Change[]>(`/api/changes?subject=${subject}`);

export type SetupCheck = { valid: boolean; step?: SetupStep; postgres?: string };

export const checkSetup = (token: string) => post<SetupCheck>("/api/setup/check", { token });
export const setupDatabase = (token: string, kind: "sqlite" | "postgres", url?: string) =>
  post<void>("/api/setup/database", { token, kind, url });
export const setupAdmin = (token: string, admin: { name: string; email: string; password: string; url: string }) =>
  post<User>("/api/setup/admin", { token, ...admin });

export const getMe = () => get<User>("/api/auth/me");
export const login = (email: string, password: string) => post<User>("/api/auth/login", { email, password });
export const logout = () => post<void>("/api/auth/logout");

export class NotFound extends Error {}
export class Unauthorized extends Error {}

// ApiError carries the hub's own words for what went wrong.
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function get<T>(path: string): Promise<T> {
  return answer<T>(await fetch(path), path);
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return send<T>("POST", path, body);
}

async function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  return answer<T>(res, path);
}

async function answer<T>(res: Response, path: string): Promise<T> {
  if (res.status === 401 && path !== "/api/auth/login") throw new Unauthorized(path);
  if (res.status === 404) throw new NotFound(path);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? `${res.status} ${res.statusText}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}
