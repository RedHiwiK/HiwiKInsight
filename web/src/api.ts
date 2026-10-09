// API client: sign-in and read-only queries. Requests are same-origin, so the session cookie is sent automatically

import { t } from './prefs'

export type Cell = string | number | boolean | null
export interface Table {
  name: string
  columns: string[]
  rows: Cell[][]
}
export interface MetricResult {
  metric: string
  definition: string
  app: string
  env: string
  from: string
  to: string
  tables: Table[]
}
export interface AppMeta {
  key: string
  name: string
  bundle: string
  /** Palette slot: purple, teal, orange, pink, blue, violet, green, gold */
  color: string
  /** Product ID -> display name */
  products: Record<string, string> | null
  /** Paywall context -> display label */
  paywall_contexts: Record<string, string> | null
  modules: [string, string][] | null
  events: Record<string, string> | null
}
export interface Meta {
  apps: AppMeta[]
  today: string
  /** IANA time zone used for days and local times, e.g. America/Los_Angeles */
  timezone: string
  /** Base currency (ISO code) of every amount */
  currency: string
  /** 1 base currency = N units of each code */
  fx?: Record<string, number>
}

/** Error from the API, with the HTTP status and the parsed JSON body */
export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly body: Record<string, unknown>) {
    super(message)
  }
}

export const isUnauthorized = (e: unknown) => e instanceof ApiError && e.status === 401

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (res.status === 204) return undefined as T
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(body.error || t(`请求失败（${res.status}）`, `Request failed (${res.status})`), res.status, body)
  return body as T
}

const post = (body?: unknown): RequestInit => ({
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: body === undefined ? undefined : JSON.stringify(body),
})

export const api = {
  login: (username: string, password: string) => request<{ username: string }>('/v1/auth/login', post({ username, password })),
  logout: () => request<void>('/v1/auth/logout', post()),
  me: () => request<{ username: string; auth: boolean }>('/v1/auth/me'),
  meta: () => request<Meta>('/v1/query/meta'),
  metric: (name: string, params: Record<string, string | number | undefined>) => {
    const q = new URLSearchParams()
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '') q.set(k, String(v))
    return request<MetricResult>(`/v1/query/metric/${name}?${q}`)
  },
  user: (id: string, app: string) => request<MetricResult>(`/v1/query/user/${encodeURIComponent(id)}?app=${app}&limit=300`),
}

/** Pick a table from a result and turn its rows into objects */
export function rows<T = Record<string, Cell>>(r: MetricResult | undefined, name: string): T[] {
  const t = r?.tables.find((x) => x.name === name)
  if (!t) return []
  return t.rows.map((row) => Object.fromEntries(t.columns.map((c, i) => [c, row[i]])) as T)
}

export function num(v: Cell | undefined): number {
  return typeof v === 'number' ? v : v == null ? 0 : Number(v) || 0
}
