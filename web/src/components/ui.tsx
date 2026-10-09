// Base components: card, KPI, segmented control, select, badge, table, states
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import clsx from 'clsx'
import { ArrowDown, ArrowUp, ArrowUpDown, Check, ChevronDown, Copy, Inbox, Info, TriangleAlert } from 'lucide-react'
import type { Cell } from '../api'
import { delta, fmtPct } from '../format'
import { Sparkline } from './Chart'
import { t } from '../prefs'

// ---------- Card ----------

export function Card({
  title, info, actions, cli, children, className, bodyClassName, span,
}: {
  title?: ReactNode
  info?: string
  actions?: ReactNode
  cli?: string
  children: ReactNode
  className?: string
  bodyClassName?: string
  /** Number of the 12 desktop columns to span */
  span?: 3 | 4 | 5 | 6 | 7 | 8 | 12
}) {
  return (
    <section
      className={clsx(
        'min-w-0 rounded-card bg-surface p-4 shadow-card sm:p-6',
        span && spanClass[span],
        className,
      )}
    >
      {(title || actions || cli) && (
        <header className="mb-4 flex min-h-7 flex-wrap items-center justify-between gap-x-3 gap-y-2">
          <div className="flex min-w-0 items-center gap-1.5">
            {title && <h2 className="truncate text-heading text-text">{title}</h2>}
            {info && <InfoTip text={info} />}
          </div>
          <div className="flex items-center gap-1.5">
            {actions}
            {cli && <CopyButton text={cli} label={t('复制为 CLI 命令', 'Copy as CLI command')} />}
          </div>
        </header>
      )}
      <div className={bodyClassName}>{children}</div>
    </section>
  )
}

const spanClass: Record<number, string> = {
  3: 'col-span-12 sm:col-span-6 xl:col-span-3',
  4: 'col-span-12 md:col-span-4',
  5: 'col-span-12 lg:col-span-5',
  6: 'col-span-12 lg:col-span-6',
  7: 'col-span-12 lg:col-span-7',
  8: 'col-span-12 lg:col-span-8',
  12: 'col-span-12',
}

export function Grid({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={clsx('grid grid-cols-12 gap-4', className)}>{children}</div>
}

// ---------- Definition tooltip ----------

export function InfoTip({ text }: { text: string }) {
  const [open, setOpen] = useState(false)
  return (
    <span className="relative inline-flex" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <button
        type="button"
        aria-label={t('查看口径', 'Show definition')}
        onClick={() => setOpen((v) => !v)}
        onBlur={() => setOpen(false)}
        className="inline-flex size-5 items-center justify-center rounded-full text-text-3 transition-ui hover:text-text-2"
      >
        <Info size={14} strokeWidth={1.75} />
      </button>
      {open && (
        <span
          role="tooltip"
          className="absolute top-full left-1/2 z-30 mt-1.5 -translate-x-1/2"
        >
          <span className="material-pop pop-in block w-max max-w-[min(320px,80vw)] origin-top rounded-[12px] px-3 py-2 text-small font-normal text-text-2">
            {text}
          </span>
        </span>
      )}
    </span>
  )
}

// ---------- Copy button ----------

export function CopyButton({ text, label }: { text: string; label: string }) {
  const [done, setDone] = useState(false)
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={() => {
        navigator.clipboard?.writeText(text).then(() => {
          setDone(true)
          setTimeout(() => setDone(false), 1400)
        })
      }}
      className="press inline-flex size-7 items-center justify-center rounded-full text-text-3 hover:bg-fill hover:text-text-2"
    >
      {done ? <Check size={15} strokeWidth={1.75} className="text-success" /> : <Copy size={15} strokeWidth={1.75} />}
    </button>
  )
}

// ---------- KPI ----------

export function KpiCard({
  label, value, current, previous, info, spark, sparkColor, inverse, loading, suffix,
}: {
  label: string
  value: string
  current?: Cell
  previous?: Cell
  info?: string
  spark?: number[]
  sparkColor?: string
  /** Lower is better (churn, refunds, ...): delta colors are inverted */
  inverse?: boolean
  loading?: boolean
  suffix?: string
}) {
  const d = current !== undefined ? delta(current, previous) : undefined
  return (
    <div className="flex min-w-0 flex-col rounded-card bg-surface p-4 shadow-card sm:p-5">
      <div className="flex items-center gap-1 text-small font-medium text-text-2">
        <span className="truncate">{label}</span>
        {info && <InfoTip text={info} />}
      </div>
      {loading ? (
        <div className="skeleton mt-2 h-9 w-24" />
      ) : (
        <div className="num mt-1.5 truncate font-rounded text-[28px] leading-9 font-bold tracking-[var(--track-display)] text-text sm:text-display">
          {value}
          {suffix && <span className="ml-1 text-small font-normal text-text-3">{suffix}</span>}
        </div>
      )}
      <div className="mt-1.5 flex min-h-5 items-center gap-1.5">
        {d !== undefined && <DeltaBadge value={d} inverse={inverse} />}
        {d !== undefined && <span className="text-caption font-normal text-text-3">{t('较上期', 'vs. prev.')}</span>}
      </div>
      {spark && spark.length > 1 && (
        <div className="mt-3 -mb-1 h-8">
          <Sparkline data={spark} color={sparkColor} />
        </div>
      )}
    </div>
  )
}

export function DeltaBadge({ value, inverse }: { value: number | null; inverse?: boolean }) {
  if (value === null || !isFinite(value)) {
    return <span className="num inline-flex h-5 items-center rounded-[6px] bg-surface-2 px-1.5 text-caption text-text-3">—</span>
  }
  const flat = Math.abs(value) < 0.05
  const good = inverse ? value < 0 : value > 0
  return (
    <span
      className={clsx(
        'num inline-flex h-5 items-center gap-0.5 rounded-[6px] px-1.5 text-caption',
        flat ? 'bg-surface-2 text-text-3' : good ? 'bg-success/12 text-success' : 'bg-danger/12 text-danger',
      )}
    >
      {!flat && (value > 0 ? <ArrowUp size={12} strokeWidth={2.25} /> : <ArrowDown size={12} strokeWidth={2.25} />)}
      {fmtPct(Math.abs(value))}
    </span>
  )
}

export function KpiRow({ children }: { children: ReactNode }) {
  return <div className="grid grid-cols-2 gap-3 sm:gap-4 md:grid-cols-3 xl:grid-cols-4">{children}</div>
}

// ---------- Segmented control ----------

export function Segmented<T extends string>({
  value, options, onChange, size = 'md', ariaLabel,
}: {
  value: T
  options: { value: T; label: ReactNode }[]
  onChange: (v: T) => void
  size?: 'sm' | 'md'
  ariaLabel?: string
}) {
  // The selected background is a separate thumb that springs from its current position, so rapid taps never jump
  const ref = useRef<HTMLDivElement>(null)
  const [thumb, setThumb] = useState<{ x: number; w: number } | null>(null)
  const [animate, setAnimate] = useState(false)
  const index = options.findIndex((o) => o.value === value)
  useLayoutEffect(() => {
    const measure = () => {
      const btn = ref.current?.querySelectorAll<HTMLButtonElement>('[role="radio"]')[index]
      setThumb(btn ? { x: btn.offsetLeft, w: btn.offsetWidth } : null)
    }
    measure()
    const ro = new ResizeObserver(measure)
    if (ref.current) ro.observe(ref.current)
    return () => ro.disconnect()
  }, [index, options.length])
  // No animation for the first placement, only for later changes
  useEffect(() => {
    if (thumb && !animate) requestAnimationFrame(() => setAnimate(true))
  }, [thumb, animate])
  return (
    <div ref={ref} role="radiogroup" aria-label={ariaLabel} className="relative inline-flex shrink-0 rounded-sm bg-fill p-0.5">
      {thumb && (
        <span
          aria-hidden
          className="seg-thumb absolute top-0.5 bottom-0.5 left-0 rounded-[8px] bg-[var(--seg-active)] shadow-card"
          style={{
            width: thumb.w,
            transform: `translateX(${thumb.x}px)`,
            transition: animate ? 'transform var(--spring-dur) var(--spring), width var(--spring-dur) var(--spring)' : 'none',
          }}
        />
      )}
      {options.map((o) => {
        const active = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(o.value)}
            className={clsx(
              'press relative rounded-[8px] whitespace-nowrap',
              size === 'sm' ? 'h-6 px-2 text-caption' : 'h-7 px-2.5 text-small',
              active ? 'font-medium text-text' : 'text-text-2 hover:text-text',
              // Static background until the thumb has been measured
              active && !thumb && 'bg-[var(--seg-active)] shadow-card',
            )}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}

// ---------- Select ----------

export function Select<T extends string>({
  value, options, onChange, ariaLabel, className, searchable, placeholder,
}: {
  value: T
  options: { value: T; label: string; hint?: string }[]
  onChange: (v: T) => void
  ariaLabel: string
  className?: string
  searchable?: boolean
  placeholder?: string
}) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])
  const current = options.find((o) => o.value === value)
  const shown = q ? options.filter((o) => (o.label + (o.hint ?? '')).toLowerCase().includes(q.toLowerCase())) : options
  return (
    <div ref={ref} className={clsx('relative', className)}>
      <button
        type="button"
        aria-label={ariaLabel}
        aria-expanded={open}
        onClick={() => { setOpen((v) => !v); setQ('') }}
        className="press flex h-8 w-full items-center justify-between gap-2 rounded-full bg-fill pr-2.5 pl-3.5 text-small font-medium text-text hover:bg-fill-strong"
      >
        <span className="truncate">{current?.label ?? placeholder ?? t('请选择', 'Select')}</span>
        <ChevronDown size={15} strokeWidth={1.75} className={clsx('shrink-0 text-text-3 transition-transform duration-[var(--spring-dur)] ease-[var(--spring)]', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="material-pop pop-in absolute top-full left-0 z-40 mt-1.5 w-max min-w-full max-w-[min(360px,90vw)] origin-top-left rounded-[14px] p-1.5">
          {searchable && (
            <input
              autoFocus
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder={t('搜索', 'Search')}
              className="mb-1 h-8 w-full rounded-[6px] bg-surface-2 px-2.5 text-small text-text outline-none placeholder:text-text-3"
            />
          )}
          <div className="scroll-thin max-h-72 overflow-y-auto">
            {shown.map((o) => (
              <button
                key={o.value}
                type="button"
                onClick={() => { onChange(o.value); setOpen(false) }}
                className={clsx(
                  'group/opt flex h-8 w-full items-center gap-3 rounded-[8px] px-2.5 text-left text-small transition-ui hover:bg-primary hover:text-on-primary active:bg-primary active:text-on-primary',
                  o.value === value ? 'text-text' : 'text-text-2',
                )}
              >
                <span className="flex-1 truncate">{o.label}</span>
                {o.hint && <span className="num text-caption font-normal text-text-3 group-hover/opt:text-on-primary/70">{o.hint}</span>}
                <Check size={14} strokeWidth={2} className={clsx('shrink-0 text-primary group-hover/opt:text-on-primary', o.value !== value && 'invisible')} />
              </button>
            ))}
            {shown.length === 0 && <div className="px-2.5 py-2 text-small text-text-3">{t('无匹配项', 'No matches')}</div>}
          </div>
        </div>
      )}
    </div>
  )
}

// ---------- Badge ----------

export type Tone = 'primary' | 'success' | 'danger' | 'warning' | 'neutral' | 'c1' | 'c2' | 'c3' | 'c5'

const toneClass: Record<Tone, string> = {
  primary: 'bg-primary-soft text-primary',
  success: 'bg-success/12 text-success',
  danger: 'bg-danger/12 text-danger',
  warning: 'bg-warning/15 text-warning',
  neutral: 'bg-surface-2 text-text-2',
  c1: 'bg-[color-mix(in_srgb,var(--c1)_14%,transparent)] text-[var(--c1)]',
  c2: 'bg-[color-mix(in_srgb,var(--c2)_14%,transparent)] text-[var(--c2)]',
  c3: 'bg-[color-mix(in_srgb,var(--c3)_14%,transparent)] text-[var(--c3)]',
  c5: 'bg-[color-mix(in_srgb,var(--c5)_14%,transparent)] text-[var(--c5)]',
}

export function Badge({ tone = 'neutral', children, title }: { tone?: Tone; children: ReactNode; title?: string }) {
  return (
    <span title={title} className={clsx('inline-flex h-5 shrink-0 items-center rounded-[6px] px-1.5 text-caption whitespace-nowrap', toneClass[tone])}>
      {children}
    </span>
  )
}

const tiers: Record<string, { zh: string; en: string; tone: Tone; color: string }> = {
  heavy: { zh: '重度', en: 'Heavy', tone: 'c1', color: 'var(--tier-heavy)' },
  medium: { zh: '中度', en: 'Medium', tone: 'c5', color: 'var(--tier-medium)' },
  light: { zh: '轻度', en: 'Light', tone: 'c2', color: 'var(--tier-light)' },
  once: { zh: '一次性', en: 'One-time', tone: 'neutral', color: 'var(--tier-once)' },
  dormant: { zh: '沉睡', en: 'Dormant', tone: 'neutral', color: 'var(--tier-dormant)' },
}

/** User tier label (in the UI language), badge tone and chart color; undefined for unknown tiers */
export function tierMeta(key: string): { label: string; tone: Tone; color: string } | undefined {
  const m = tiers[key]
  return m && { label: t(m.zh, m.en), tone: m.tone, color: m.color }
}

export function Dot({ color }: { color: string }) {
  return <span className="inline-block size-2 shrink-0 rounded-full" style={{ background: color }} />
}

// ---------- States ----------

export function Skeleton({ className }: { className?: string }) {
  return <div className={clsx('skeleton', className)} />
}

export function EmptyState({ title = t('暂无数据', 'No data'), hint, className }: { title?: string; hint?: ReactNode; className?: string }) {
  return (
    <div className={clsx('flex flex-col items-center justify-center gap-1.5 py-10 text-center', className)}>
      <Inbox size={28} strokeWidth={1.5} className="mb-1 text-text-3" />
      <div className="text-body text-text-2">{title}</div>
      {hint && <div className="max-w-sm text-small text-text-3">{hint}</div>}
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-10 text-center">
      <TriangleAlert size={28} strokeWidth={1.5} className="text-danger" />
      <div className="text-small text-text-2">{error instanceof Error ? error.message : t('加载失败', 'Failed to load')}</div>
      {onRetry && (
        <button type="button" onClick={onRetry} className="press mt-1 h-8 rounded-full bg-fill px-4 text-small font-medium text-primary hover:bg-fill-strong">
          {t('重试', 'Retry')}
        </button>
      )}
    </div>
  )
}

/** Shared loading / error / empty handling */
export function QueryState({
  loading, error, empty, emptyHint, onRetry, height = 240, children,
}: {
  loading: boolean
  error: unknown
  empty?: boolean
  emptyHint?: ReactNode
  onRetry?: () => void
  height?: number
  children: ReactNode
}) {
  if (loading) return <div className="skeleton w-full" style={{ height }} />
  if (error) return <ErrorState error={error} onRetry={onRetry} />
  if (empty) return <div style={{ minHeight: height }} className="flex items-center justify-center"><EmptyState hint={emptyHint} /></div>
  return <>{children}</>
}

// ---------- Table ----------

export interface Column<T> {
  key: string
  title: ReactNode
  render: (row: T) => ReactNode
  align?: 'left' | 'right'
  sortValue?: (row: T) => number | string
  width?: string
  className?: string
}

export function DataTable<T>({
  columns, data, rowKey, onRowClick, defaultSort, empty, maxRows,
}: {
  columns: Column<T>[]
  data: T[]
  rowKey: (row: T, i: number) => string
  onRowClick?: (row: T) => void
  defaultSort?: { key: string; desc: boolean }
  empty?: ReactNode
  maxRows?: number
}) {
  const [sort, setSort] = useState(defaultSort)
  const col = columns.find((c) => c.key === sort?.key)
  let sorted = data
  if (col?.sortValue && sort) {
    const sv = col.sortValue
    sorted = [...data].sort((a, b) => {
      const x = sv(a)
      const y = sv(b)
      const r = typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y))
      return sort.desc ? -r : r
    })
  }
  if (maxRows) sorted = sorted.slice(0, maxRows)
  if (data.length === 0) return <>{empty ?? <EmptyState />}</>
  return (
    <div className="scroll-thin -mx-4 overflow-x-auto sm:-mx-6">
      <table className="w-full min-w-max border-collapse text-small">
        <thead>
          <tr className="border-b border-border">
            {columns.map((c, i) => (
              <th
                key={c.key}
                style={{ width: c.width }}
                className={clsx(
                  'h-9 px-3 text-caption font-medium whitespace-nowrap text-text-3',
                  c.align === 'right' ? 'text-right' : 'text-left',
                  i === 0 && 'sticky left-0 z-10 bg-surface pl-4 sm:pl-6',
                  i === columns.length - 1 && 'pr-4 sm:pr-6',
                )}
              >
                {c.sortValue ? (
                  <button
                    type="button"
                    onClick={() => setSort((s) => ({ key: c.key, desc: s?.key === c.key ? !s.desc : true }))}
                    className={clsx('inline-flex items-center gap-1 transition-ui hover:text-text-2', sort?.key === c.key && 'text-text-2')}
                  >
                    {c.title}
                    {sort?.key === c.key ? (
                      sort.desc ? <ArrowDown size={12} /> : <ArrowUp size={12} />
                    ) : (
                      <ArrowUpDown size={12} className="opacity-50" />
                    )}
                  </button>
                ) : (
                  c.title
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sorted.map((row, ri) => (
            <tr
              key={rowKey(row, ri)}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              className={clsx('group border-b border-border last:border-b-0 transition-ui hover:bg-surface-2', onRowClick && 'cursor-pointer active:bg-surface-3')}
            >
              {columns.map((c, i) => (
                <td
                  key={c.key}
                  className={clsx(
                    'h-11 px-3 whitespace-nowrap text-text',
                    c.align === 'right' && 'num text-right',
                    i === 0 && 'sticky left-0 z-10 bg-surface pl-4 transition-ui group-hover:bg-surface-2 sm:pl-6',
                    i === 0 && onRowClick && 'group-active:bg-surface-3',
                    i === columns.length - 1 && 'pr-4 sm:pr-6',
                    c.className,
                  )}
                >
                  {c.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** Inline bar */
export function InlineBar({ value, max, color = 'var(--primary)' }: { value: number; max: number; color?: string }) {
  const w = max > 0 ? Math.max(2, (value / max) * 100) : 0
  return (
    <div className="h-1.5 w-full min-w-16 overflow-hidden rounded-[3px] bg-primary-soft">
      <div className="h-full rounded-[3px] transition-[width] duration-[var(--spring-dur)] ease-[var(--spring)]" style={{ width: `${w}%`, background: color }} />
    </div>
  )
}

/** Ranked list: name + bar + value, for distributions and top N (easier to read than a horizontal bar chart) */
export function BarList({
  items, format, max: maxRows = 10, valueLabel, secondary,
}: {
  items: { key: string; label: ReactNode; value: number; sub?: ReactNode; color?: string }[]
  format: (v: number) => string
  max?: number
  valueLabel?: string
  secondary?: string
}) {
  if (items.length === 0) return <EmptyState />
  let shown = items
  if (items.length > maxRows) {
    const rest = items.slice(maxRows - 1)
    shown = [...items.slice(0, maxRows - 1), { key: '__other', label: t(`其他（${rest.length} 项）`, `Other (${rest.length})`), value: rest.reduce((s, x) => s + x.value, 0) }]
  }
  const top = Math.max(...shown.map((x) => x.value), 1)
  return (
    <div>
      {(valueLabel || secondary) && (
        <div className="mb-1 flex justify-end gap-4 text-caption text-text-3">
          {secondary && <span className="w-14 text-right">{secondary}</span>}
          {valueLabel && <span className="w-20 text-right">{valueLabel}</span>}
        </div>
      )}
      <ul className="flex flex-col">
        {shown.map((it) => (
          <li key={it.key} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 py-2">
            <div className="min-w-0">
              <div className="mb-1.5 truncate text-small text-text">{it.label}</div>
              <InlineBar value={it.value} max={top} color={it.color} />
            </div>
            <div className="flex items-baseline gap-4 self-end">
              {secondary !== undefined && <span className="num w-14 text-right text-caption font-normal text-text-3">{it.sub ?? ''}</span>}
              <span className="num w-20 text-right text-small font-medium text-text">{format(it.value)}</span>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}

export function PageHeaderActions({ children }: { children: ReactNode }) {
  return <div className="mb-4 flex flex-wrap items-center gap-2">{children}</div>
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label className="inline-flex cursor-pointer items-center gap-2 text-small text-text-2 select-none">
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
        className={clsx('press relative h-5 w-9 rounded-full', checked ? 'bg-primary' : 'bg-surface-3')}
      >
        <span
          className={clsx(
            'switch-thumb absolute top-0.5 left-0.5 size-4 rounded-full bg-on-primary shadow-card transition-transform duration-[var(--spring-bounce-dur)] ease-[var(--spring-bounce)]',
            checked && 'translate-x-4',
          )}
        />
      </button>
      {label}
    </label>
  )
}
