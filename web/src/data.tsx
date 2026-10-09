// Global data context: app metadata, filters from the URL, metric requests
import { createContext, useCallback, useContext, useMemo, type ReactNode } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { api, type AppMeta, type Meta, type MetricResult } from './api'
import { addDays, shortModuleName } from './format'
import { getLang, t } from './prefs'

const MetaContext = createContext<Meta | null>(null)

export function MetaProvider({ meta, children }: { meta: Meta; children: ReactNode }) {
  return <MetaContext.Provider value={meta}>{children}</MetaContext.Provider>
}

export function useMeta(): Meta {
  const m = useContext(MetaContext)
  if (!m) throw new Error('MetaProvider missing')
  return m
}

/** App color palette slots (meta app.color) -> chart tokens; ECharts resolves c1..c8 through usePalette */
const slotColors: Record<string, string> = {
  purple: 'var(--c1)', teal: 'var(--c2)', orange: 'var(--c3)', pink: 'var(--c4)',
  blue: 'var(--c5)', violet: 'var(--c6)', green: 'var(--c7)', gold: 'var(--c8)',
}
export const paletteSlots = Object.keys(slotColors)
export const slotColor = (slot: string | undefined): string => slotColors[slot ?? ''] ?? 'var(--c1)'

/** Product display name: the configured name, otherwise the product ID without the app's bundle ID prefix */
export function productName(app: { bundle: string; products?: Record<string, string> | null } | undefined, id: string): string {
  const named = app?.products?.[id]
  if (named) return named
  return app?.bundle && id.startsWith(`${app.bundle}.`) ? id.slice(app.bundle.length + 1) : id
}

/** Paywall context label: the configured label, built-in labels for server values, otherwise the raw value */
export function paywallLabel(app: AppMeta | undefined, context: string): string {
  const named = app?.paywall_contexts?.[context]
  if (named) return named
  if (context === 'unattributed') return t('未归因', 'Unattributed')
  if (context === 'unknown') return t('未知', 'Unknown')
  return context
}

export type Env = 'production' | 'sandbox' | 'xcode'
export const envOptions = (): { value: Env; label: string }[] => [
  { value: 'production', label: t('正式', 'Production') },
  { value: 'sandbox', label: 'TestFlight' },
  { value: 'xcode', label: t('开发', 'Dev') },
]
export const rangeOptions = () => [
  { value: '7', label: t('7 天', '7D') },
  { value: '28', label: t('28 天', '28D') },
  { value: '90', label: t('90 天', '90D') },
]

export interface Filters {
  app: string
  env: Env
  /** 7 / 28 / 90, or custom */
  range: string
  from: string
  to: string
  today: string
  appMeta: AppMeta
  moduleName: (key: string) => string
  set: (patch: Record<string, string | null>) => void
  /** Query kept when navigating between pages (app / env / range / from / to) */
  keep: string
}

const keepKeys = ['app', 'env', 'range', 'from', 'to']

export function useFilters(): Filters {
  const meta = useMeta()
  const [sp, setSp] = useSearchParams()
  const app = meta.apps.some((a) => a.key === sp.get('app')) ? sp.get('app')! : meta.apps[0]?.key ?? ''
  const envParam = sp.get('env')
  const env: Env = envParam === 'sandbox' || envParam === 'xcode' ? envParam : 'production'
  const customFrom = sp.get('from')
  const customTo = sp.get('to')
  const isCustom = !!(customFrom && customTo && /^\d{4}-\d{2}-\d{2}$/.test(customFrom) && /^\d{4}-\d{2}-\d{2}$/.test(customTo))
  const range = isCustom ? 'custom' : ['7', '28', '90'].includes(sp.get('range') ?? '') ? sp.get('range')! : '28'
  const to = isCustom ? customTo! : meta.today
  const from = isCustom ? customFrom! : addDays(meta.today, -(Number(range) - 1))
  const appMeta = meta.apps.find((a) => a.key === app) ?? meta.apps[0]

  const set = useCallback(
    (patch: Record<string, string | null>) => {
      setSp(
        (prev) => {
          const next = new URLSearchParams(prev)
          for (const [k, v] of Object.entries(patch)) {
            if (v === null || v === '') next.delete(k)
            else next.set(k, v)
          }
          return next
        },
        { replace: false },
      )
    },
    [setSp],
  )

  // Module names come from the event catalog, written in whatever language the developer chose;
  // the English UI falls back to the module key (timeline -> Timeline) when a description is in Chinese
  const modules = useMemo(
    () => new Map((appMeta?.modules ?? []).map(([k, d]) => [
      k,
      getLang() === 'en' && /[\u4e00-\u9fff]/.test(d) ? k.charAt(0).toUpperCase() + k.slice(1) : shortModuleName(d) || k,
    ])),
    [appMeta],
  )
  const keep = useMemo(() => {
    const q = new URLSearchParams()
    for (const k of keepKeys) if (sp.get(k)) q.set(k, sp.get(k)!)
    const s = q.toString()
    return s ? `?${s}` : ''
  }, [sp])

  return {
    app, env, range, from, to, today: meta.today, appMeta, set, keep,
    moduleName: (k: string) => modules.get(k) ?? k,
  }
}

/** Fetch a predefined metric; keeps the previous data while filters change to avoid flashing the whole page */
export function useMetric(name: string, extra: Record<string, string | number | undefined> = {}, opts: { from?: string; to?: string; enabled?: boolean } = {}) {
  const f = useFilters()
  const params = { app: f.app, env: f.env, from: opts.from ?? f.from, to: opts.to ?? f.to, ...extra }
  return useQuery<MetricResult>({
    queryKey: ['metric', name, params],
    queryFn: () => api.metric(name, params),
    placeholderData: keepPreviousData,
    enabled: opts.enabled ?? true,
  })
}

/** Build a ready-to-paste CLI command (card action "Copy as CLI command") */
export function useCli() {
  const f = useFilters()
  return (metric: string, extra: Record<string, string | undefined> = {}) => {
    const parts = [`insight metric ${metric}`, `--app ${f.app}`]
    if (f.env !== 'production') parts.push(`--env ${f.env}`)
    parts.push(`--from ${f.from}`, `--to ${f.to}`)
    for (const [k, v] of Object.entries(extra)) if (v) parts.push(`--${k} ${v}`)
    return parts.join(' ')
  }
}
