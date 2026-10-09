// Custom visualizations: retention heatmap, activity calendar, event timeline
import { useMemo, useState } from 'react'
import clsx from 'clsx'
import type { Cell } from '../api'
import { addDays, fmtDuration, fmtInt, fmtLongDay, fmtPct } from '../format'
import { Badge } from './ui'
import { t } from '../prefs'
import { fmtMonth } from '../format'

// ---------- Retention heatmap ----------

const heat = ['var(--heat-0)', 'var(--heat-1)', 'var(--heat-2)', 'var(--heat-3)', 'var(--heat-4)']

function pctLevel(v: number): number {
  if (v <= 0) return 0
  if (v < 12) return 1
  if (v < 25) return 2
  if (v < 40) return 3
  return 4
}

export interface CohortRow {
  cohort: string
  size: number
  cells: (number | null)[]
}

export function CohortHeatmap({ columns, rows, cohortLabel }: { columns: string[]; rows: CohortRow[]; cohortLabel: (c: string) => string }) {
  return (
    <div className="scroll-thin -mx-4 overflow-x-auto px-4 sm:-mx-6 sm:px-6">
      <table className="w-full min-w-[640px] border-separate" style={{ borderSpacing: 3 }}>
        <thead>
          <tr>
            <th className="h-8 pr-3 text-left text-caption font-medium text-text-3">{t('首见', 'Cohort')}</th>
            <th className="h-8 pr-3 text-right text-caption font-medium text-text-3">{t('人数', 'Users')}</th>
            {columns.map((c) => (
              <th key={c} className="h-8 text-center text-caption font-medium text-text-3">{c}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const all = r.cohort === 'all'
            return (
              <tr key={r.cohort}>
                <td className={clsx('h-9 pr-3 text-small whitespace-nowrap', all ? 'border-t border-border pt-1 font-semibold text-text' : 'text-text-2')}>
                  {all ? t('全部', 'All') : cohortLabel(r.cohort)}
                </td>
                <td className={clsx('num h-9 pr-3 text-right text-small', all ? 'border-t border-border pt-1 font-semibold text-text' : 'text-text-2')}>
                  {fmtInt(r.size)}
                </td>
                {r.cells.map((v, i) => {
                  const lvl = v == null ? -1 : pctLevel(v)
                  return (
                    <td key={i} className={clsx('p-0', all && 'border-t border-border pt-1')}>
                      <div
                        title={v == null ? t('尚未到观测日', 'Not yet observable') : `${columns[i]}: ${fmtPct(v)}`}
                        className={clsx(
                          'num flex h-9 min-w-14 items-center justify-center rounded-[6px] text-small',
                          all && 'font-semibold',
                          lvl === 4 ? 'text-on-primary' : lvl === -1 ? 'text-text-3' : 'text-text',
                        )}
                        style={{ background: lvl >= 0 ? heat[lvl] : 'transparent' }}
                      >
                        {v == null ? '·' : fmtPct(v)}
                      </div>
                    </td>
                  )
                })}
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export function HeatLegend({ low = t('低', 'Low'), high = t('高', 'High') }: { low?: string; high?: string }) {
  return (
    <div className="flex items-center gap-1.5 text-caption font-normal text-text-3">
      {low}
      {heat.map((h) => (
        <span key={h} className="size-2.5 rounded-[3px]" style={{ background: h }} />
      ))}
      {high}
    </div>
  )
}

// ---------- Activity calendar ----------

export function CalendarHeatmap({
  days, today, weeks = 26,
}: {
  days: { day: string; sessions: number; duration: number }[]
  today: string
  weeks?: number
}) {
  const byDay = useMemo(() => new Map(days.map((d) => [d.day, d])), [days])
  const [hover, setHover] = useState<string | null>(null)
  // Start from the Monday `weeks` weeks ago
  const [ty, tm, td] = today.split('-').map(Number)
  const now = new Date(ty, tm - 1, td)
  const dow = (now.getDay() + 6) % 7
  const start = addDays(today, -dow - (weeks - 1) * 7)
  const cols: string[][] = []
  for (let w = 0; w < weeks; w++) {
    const col: string[] = []
    for (let d = 0; d < 7; d++) col.push(addDays(start, w * 7 + d))
    cols.push(col)
  }
  const level = (s: number) => (s <= 0 ? 0 : s === 1 ? 1 : s === 2 ? 2 : s <= 4 ? 3 : 4)
  const months: { i: number; label: string }[] = []
  cols.forEach((c, i) => {
    const m = Number(c[0].slice(5, 7))
    const last = months[months.length - 1]
    if ((i === 0 || m !== Number(cols[i - 1][0].slice(5, 7))) && (!last || i - last.i >= 3)) months.push({ i, label: fmtMonth(m) })
  })
  const info = hover ? byDay.get(hover) : undefined
  return (
    <div>
      <div className="scroll-thin overflow-x-auto pb-1">
        <div className="inline-flex flex-col gap-1">
          <div className="relative h-4 text-caption font-normal text-text-3">
            {months.map((m) => (
              <span key={m.i} className="absolute" style={{ left: 24 + m.i * 15 }}>{m.label}</span>
            ))}
          </div>
          <div className="flex gap-[3px]">
            <div className="mr-1 flex w-5 flex-col gap-[3px] text-[10px] leading-3 text-text-3">
              {(t('一,,三,,五,,日', 'M,,W,,F,,S')).split(',').map((l, i) => <span key={i} className="h-3">{l}</span>)}
            </div>
            {cols.map((col, ci) => (
              <div key={ci} className="flex flex-col gap-[3px]">
                {col.map((d) => {
                  const v = byDay.get(d)
                  const future = d > today
                  return (
                    <span
                      key={d}
                      onMouseEnter={() => setHover(d)}
                      onMouseLeave={() => setHover(null)}
                      className={clsx('size-3 rounded-[3px]', hover === d && 'ring-2 ring-primary/40')}
                      style={{ background: future ? 'transparent' : heat[level(v?.sessions ?? 0)] }}
                    />
                  )
                })}
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <div className="min-h-5 text-small text-text-2">
          {hover ? (
            <>
              <span className="text-text">{fmtLongDay(hover)}</span>
              {info ? ` · ${t(`${info.sessions} 次会话`, `${info.sessions} sessions`)} · ${fmtDuration(info.duration)}` : ` · ${t('未活跃', 'Inactive')}`}
            </>
          ) : (
            <span className="text-text-3">{t('悬停查看每天的会话', 'Hover to see daily sessions')}</span>
          )}
        </div>
        <HeatLegend low={t('少', 'Less')} high={t('多', 'More')} />
      </div>
    </div>
  )
}

// ---------- Event timeline ----------

interface Ev {
  time: string
  name: string
  session: string
  params: Record<string, string>
  version: string
}

const sourceLabel = (): Record<string, string> => ({
  icon: t('图标', 'Icon'),
  widget_diary: t('小组件', 'Widget'),
  notification_reminder: t('提醒通知', 'Reminder'),
  notification_retention: t('召回通知', 'Win-back'),
  notification_item: t('用品通知', 'Item alert'),
  notification_other: t('通知', 'Notification'),
})

export function Timeline({ rows, moduleName }: { rows: Record<string, Cell>[]; moduleName: (k: string) => string }) {
  const groups = useMemo(() => {
    const evs: Ev[] = rows.map((r) => {
      let params: Record<string, string> = {}
      try {
        params = r.params ? JSON.parse(String(r.params)) : {}
      } catch {
        /* ignore non-JSON params */
      }
      return { time: String(r.time_local ?? ''), name: String(r.name ?? ''), session: String(r.session ?? ''), params, version: String(r.app_version ?? '') }
    })
    // The API returns newest first; group by session (adjacent rows of the same session), oldest first within a group
    const out: { session: string; events: Ev[] }[] = []
    for (const e of evs) {
      const last = out[out.length - 1]
      if (last && last.session === e.session) last.events.push(e)
      else out.push({ session: e.session, events: [e] })
    }
    return out.map((g) => ({ ...g, events: g.events.reverse() }))
  }, [rows])

  if (groups.length === 0) return null
  return (
    <ol className="flex flex-col gap-4">
      {groups.map((g, gi) => {
        const started = g.events.find((e) => e.name === 'session.started')
        const ended = g.events.find((e) => e.name === 'session.ended')
        const body = g.events.filter((e) => !e.name.startsWith('session.') && e.name !== 'screen.left')
        // Attach the duration from screen.left to the matching screen.viewed
        const dwell = new Map<Ev, string>()
        g.events.forEach((e, i) => {
          if (e.name !== 'screen.viewed') return
          const left = g.events.slice(i + 1).find((x) => x.name === 'screen.left' && x.params.screen === e.params.screen)
          if (left?.params.duration_s) dwell.set(e, fmtDuration(Number(left.params.duration_s)))
        })
        const first = g.events[0]
        return (
          <li key={gi} className="overflow-hidden rounded-[14px] bg-surface-2">
            <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
              <span className="num text-small font-medium text-text">{first.time.slice(0, 16)}</span>
              {g.session ? (
                <Badge tone="primary">{sourceLabel()[started?.params.source ?? ''] ?? started?.params.source ?? t('会话', 'Session')}</Badge>
              ) : (
                <Badge>{t('会话外', 'No session')}</Badge>
              )}
              {ended?.params.duration_s && <span className="text-caption font-normal text-text-3">{t('时长', 'Duration')} {fmtDuration(Number(ended.params.duration_s))}</span>}
              <span className="ml-auto text-caption font-normal text-text-3">v{first.version}</span>
            </div>
            <ul className="px-4 py-1.5">
              {body.length === 0 && <li className="py-1.5 text-small text-text-3">{t('仅会话开始 / 结束', 'Session start / end only')}</li>}
              {body.map((e, i) => (
                <li key={i} className="grid grid-cols-[64px_minmax(0,1fr)] items-start gap-3 py-1.5">
                  <span className="num pt-px font-mono text-caption font-normal text-text-3">{e.time.slice(11, 19)}</span>
                  <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                    {e.name === 'screen.viewed' ? (
                      <>
                        <span className="text-small text-text-2">{t('打开', 'Opened')}</span>
                        <span className="text-small font-medium text-text">{e.params.screen}</span>
                        {e.params.module && <Badge>{moduleName(e.params.module)}</Badge>}
                        {dwell.get(e) && <span className="text-caption font-normal text-text-3">{t('停留', 'Stayed')} {dwell.get(e)}</span>}
                      </>
                    ) : (
                      <>
                        <span className={clsx('text-small font-medium', e.name === 'error' ? 'text-danger' : e.name.startsWith('purchase.') ? 'text-success' : 'text-text')}>
                          {e.name}
                        </span>
                        {Object.entries(e.params).map(([k, v]) => (
                          <span key={k} className="inline-flex h-5 max-w-full items-center truncate rounded-[6px] bg-surface-2 px-1.5 text-caption font-normal text-text-2">
                            <span className="text-text-3">{k}=</span>{v}
                          </span>
                        ))}
                      </>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          </li>
        )
      })}
    </ol>
  )
}
