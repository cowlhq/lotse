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
export type Check = Schemas["Check"];
export type CheckDetail = Schemas["CheckDetail"];
export type CheckInput = Schemas["CheckInput"];
export type CheckResult = Schemas["CheckResult"];
export type Probe = Schemas["Probe"];
export type SetupStep = Schemas["SetupStep"];
export type AlertState = Schemas["AlertState"];
export type Alert = Schemas["Alert"];
export type AlertDetail = Schemas["AlertDetail"];
export type Channel = Schemas["Channel"];
export type ChannelInput = Schemas["ChannelInput"];
export type SecretInput = Schemas["SecretInput"];
export type SecretInfo = Schemas["SecretInfo"];
export type MailServer = Schemas["MailServer"];
export type MailServerInput = Schemas["MailServerInput"];
export type Maintenance = Schemas["Maintenance"];
export type MaintenanceInput = Schemas["MaintenanceInput"];
export type TestResult = Schemas["TestResult"];

export type Range = { from: number; to: number };

export const getHealth = () => get<Health>("/api/health");
export const listHosts = () => get<HostSummary[]>("/api/hosts");
export const getHost = (id: number) => get<HostDetail>(`/api/hosts/${id}`);
export const getContainer = (id: number) => get<Container>(`/api/containers/${id}`);

export const getSeries = (kind: "hosts" | "filesystems" | "containers" | "checks", id: number, r: Range) =>
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
export const listChecks = (envID?: number) => get<Check[]>(envID ? `/api/checks?environment=${envID}` : "/api/checks");
export const getCheck = (id: number) => get<CheckDetail>(`/api/checks/${id}`);
export const createCheck = (input: CheckInput) => post<Check>("/api/checks", input);
export const updateCheck = (id: number, name: string, spec: Record<string, unknown>) =>
  send<Check>("PATCH", `/api/checks/${id}`, { name, spec });
export const deleteCheck = (id: number) => send<void>("DELETE", `/api/checks/${id}`);
export const listCheckResults = (id: number, r: Range) => get<CheckResult[]>(`/api/checks/${id}/results?from=${r.from}&to=${r.to}`);
export const listProbes = () => get<Probe[]>("/api/probes");
export const revokeProbe = (id: number) => send<void>("DELETE", `/api/probes/${id}`);
export const getProbeEnrollment = () => get<Enrollment>("/api/probes/enrollment");
export const newProbeEnrollment = () => post<Enrollment>("/api/probes/enrollment");
export const listChanges = (subject: string) => get<Change[]>(`/api/changes?subject=${subject}`);
export const listAlerts = (subject?: string) => get<AlertState[]>(subject ? `/api/alerts?subject=${subject}` : "/api/alerts");
export const listAlertHistory = (r: Range, subject?: string) =>
  get<Alert[]>(`/api/alerts/history?from=${r.from}&to=${r.to}${subject ? `&subject=${subject}` : ""}`);
export const getAlert = (id: number) => get<AlertDetail>(`/api/alerts/${id}`);
export const acknowledgeAlert = (id: number) => post<void>(`/api/alerts/${id}/acknowledge`);
export const listChannels = (scopeID?: number) => get<Channel[]>(scopeID ? `/api/channels?scope=${scopeID}` : "/api/channels");
export const createChannel = (input: ChannelInput) => post<Channel>("/api/channels", input);
export const updateChannel = (id: number, input: { name: string; spec: Record<string, unknown>; secrets?: Record<string, SecretInput> }) =>
  send<Channel>("PATCH", `/api/channels/${id}`, input);
export const deleteChannel = (id: number) => send<void>("DELETE", `/api/channels/${id}`);
export const testChannel = (id: number) => post<TestResult>(`/api/channels/${id}/test`);
export const getMailServer = () => get<MailServer>("/api/mail-server");
export const setMailServer = (input: MailServerInput) => send<MailServer>("PUT", "/api/mail-server", input);
export const deleteMailServer = () => send<void>("DELETE", "/api/mail-server");
export const testMailServer = (to: string) => post<TestResult>("/api/mail-server/test", { to });
export const listMaintenance = (subject: string) => get<Maintenance[]>(`/api/maintenance?subject=${subject}`);
export const createMaintenance = (input: MaintenanceInput) => post<Maintenance>("/api/maintenance", input);
export const deleteMaintenance = (id: number) => send<void>("DELETE", `/api/maintenance/${id}`);

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
