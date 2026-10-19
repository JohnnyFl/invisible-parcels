export type Level = "red" | "orange" | "yellow" | "grey" | "green";
export type ProblemKind = "unknown" | "misrouted" | "risky_return" | "broken_chain" | "overdue" | "stuck" | "incident";
export type Role = "admin" | "senior" | "operator";
export type Mode = "as_is" | "with_solution";

export interface Me { role: Role; branchId?: string }
export interface Location { id: string; kind: "branch" | "hub"; city: string; name: string }
export interface Problem { kind: ProblemKind; severity: Level; since: string }
export interface ProblemRow { ttn: string; level: Level; problems: Problem[]; destination: string; location: string }
export interface ShelfRow { ttn: string; registered: boolean }
export interface IncomingRow { ttn: string; from: string; tripId: string; eta: string }
export interface ScanResult { outcome: "accept" | "redirect" | "hold" | "not_found"; nextLocation?: string }
export interface Track {
  ttn: string;
  statusCode: string;
  status: string;
  eta?: string;
  isForecast: boolean;
  details?: { city: string; warehouse: string };
}
export interface Split {
  client: Track;
  operator: { branch?: string; visibleInIncoming: boolean; level: Level; problems: Problem[] };
  mode: Mode;
}
export type LegState = "done" | "in_transit" | "broken" | "incident" | "pending";
export interface TraceLeg {
  seq: number;
  from: string;
  to: string;
  tripId: string;
  plannedArrival: string;
  loaded?: string;
  departed?: string;
  arrived?: string;
  unloaded?: string;
  state: LegState;
}
export interface Trace {
  ttn: string;
  destination?: string;
  legs: TraceLeg[];
  lastConfirmed?: { location: string; at: string; kind: string };
  break?: { seq: number; hint: string };
  delivered: boolean;
}
export interface ClockState { virtualNow: string; speed: number; mode: Mode }

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function api<T>(path: string, init?: RequestInit & { json?: unknown }): Promise<T> {
  const { json, ...rest } = init ?? {};
  const res = await fetch(path, {
    credentials: "same-origin",
    ...rest,
    headers: json !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  });
  if (!res.ok) {
    // an expired session anywhere sends the user back to the login form
    if (res.status === 401 && !path.startsWith("/api/me") && !path.startsWith("/api/login") && location.pathname !== "/login" && location.pathname !== "/track") {
      location.assign("/login");
    }
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? body.message ?? res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

/** 59000000000001 -> "59 0000 0000 0001", the way operators read it out. */
export function formatTTN(ttn: string): string {
  return ttn.length === 14 ? `${ttn.slice(0, 2)} ${ttn.slice(2, 6)} ${ttn.slice(6, 10)} ${ttn.slice(10)}` : ttn;
}

const kyiv = new Intl.DateTimeFormat("uk-UA", { timeZone: "Europe/Kyiv", day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
export const formatTime = (iso: string) => kyiv.format(new Date(iso));

export function formatAgo(sinceIso: string, now: Date): string {
  const min = Math.max(0, Math.round((now.getTime() - new Date(sinceIso).getTime()) / 60000));
  if (min < 60) return `${min} хв`;
  const h = Math.floor(min / 60);
  return h < 48 ? `${h} год ${min % 60} хв` : `${Math.floor(h / 24)} дн ${h % 24} год`;
}
