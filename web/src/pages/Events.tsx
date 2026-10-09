// Event explorer: pick an event -> group by a param -> granularity, instead of configuring cards one by one
import { useSearchParams } from 'react-router'
import { num, rows, type Cell } from '../api'
import { useCli, useFilters, useMeta, useMetric } from '../data'
import { fmtInt, fmtPct } from '../format'
import { Page } from '../components/Layout'
import { BarList, Card, QueryState, Segmented, Select } from '../components/ui'
import { TrendChart, type Series } from '../components/Chart'
import { emptyHint } from './Overview'
import { t } from '../prefs'

type Row = Record<string, Cell>

export function EventsPage() {
  const f = useFilters()
  const tz = useMeta().timezone
  const cli = useCli()
  const [sp, setSp] = useSearchParams()
  const event = sp.get('event') ?? ''
  const groupBy = sp.get('group_by') ?? ''
  const gran = (['day', 'week', 'month'].includes(sp.get('granularity') ?? '') ? sp.get('granularity') : 'day') as 'day' | 'week' | 'month'
  const measure = sp.get('measure') === 'users' ? 'users' : 'count'
  const setParam = (patch: Record<string, string | null>) =>
    setSp((prev) => {
      const n = new URLSearchParams(prev)
      for (const [k, v] of Object.entries(patch)) v === null || v === '' ? n.delete(k) : n.set(k, v)
      return n
    })

  const res = useMetric('event_trend', { event: event || undefined, group_by: groupBy || undefined, granularity: gran === 'day' ? undefined : gran })
  const selected = rows<Row>(res.data, 'selected')[0]
  const current = String(selected?.event ?? event)
  const events = rows<Row>(res.data, 'events')
  const params = rows<Row>(res.data, 'params')
  const buckets = rows<Row>(res.data, 'buckets').map((b) => String(b.bucket))
  const series = rows<Row>(res.data, 'series')
  const breakdown = rows<Row>(res.data, 'breakdown')
  const desc = f.appMeta.events?.[current]

  const groups = [...new Set(breakdown.map((b) => String(b.value)))]
  const valueOf = (bucket: string, g: string) => {
    const r = series.find((s) => s.bucket === bucket && String(s.value) === g)
    return r ? num(r[measure]) : 0
  }
  const chartSeries: Series[] = groupBy
    ? groups.map((g) => ({ name: g === '(other)' ? t('其他', 'Other') : g === '(none)' ? t('（无）', '(none)') : g, type: 'bar', stack: 's', data: buckets.map((b) => valueOf(b, g)), color: g === '(other)' ? 'var(--text-3)' : undefined }))
    : [{ name: measure === 'count' ? t('次数', 'Count') : t('人数', 'Users'), data: buckets.map((b) => valueOf(b, '(all)')), color: 'var(--c1)', area: true }]
  const totalCount = breakdown.reduce((s, b) => s + num(b[measure]), 0)

  return (
    <Page title={t('事件', 'Events')} subtitle={t('任选一个事件，看它的趋势和参数分布', 'Pick any event to see its trend and parameter breakdown')}>
      <div className="flex flex-wrap items-center gap-2">
        <Select
          ariaLabel={t('选择事件', 'Select event')}
          searchable
          className="w-full sm:w-64"
          value={current}
          placeholder={t('选择事件', 'Select event')}
          options={events.map((e) => ({ value: String(e.name), label: String(e.name), hint: fmtInt(e.count) }))}
          onChange={(v) => setParam({ event: v, group_by: null })}
        />
        <Select
          ariaLabel={t('分组参数', 'Group by parameter')}
          className="w-[calc(50%-4px)] sm:w-44"
          value={groupBy}
          options={[{ value: '', label: t('不分组', 'No grouping') }, ...params.map((p) => ({ value: String(p.key), label: t(`按 ${p.key}`, `By ${p.key}`), hint: t(`${fmtInt(p.distinct_values)} 种`, `${fmtInt(p.distinct_values)} values`) }))]}
          onChange={(v) => setParam({ group_by: v })}
        />
        <Segmented value={gran} onChange={(v) => setParam({ granularity: v === 'day' ? null : v })} ariaLabel={t('粒度', 'Granularity')}
          options={[{ value: 'day', label: t('日', 'Day') }, { value: 'week', label: t('周', 'Week') }, { value: 'month', label: t('月', 'Month') }]} />
        <Segmented value={measure} onChange={(v) => setParam({ measure: v === 'count' ? null : v })} ariaLabel={t('指标', 'Measure')}
          options={[{ value: 'count', label: t('次数', 'Count') }, { value: 'users', label: t('人数', 'Users') }]} />
      </div>
      {desc && <p className="-mt-1 text-small text-text-2"><span className="font-mono text-text">{current}</span>{t('：', ': ')}{desc}</p>}

      <Card title={t('趋势', 'Trend')} cli={cli('event_trend', { event: current, group_by: groupBy || undefined, granularity: gran === 'day' ? undefined : gran })}
        info={gran === 'week' ? t('按周（周一开始）汇总', 'Weekly totals (weeks start Monday)') : gran === 'month' ? t('按自然月汇总', 'Calendar-month totals') : t(`按 ${tz} 自然日汇总`, `Daily totals (${tz} days)`)}>
        <QueryState loading={res.isPending} error={res.error} empty={!res.isPending && series.length === 0} height={300} emptyHint={emptyHint(f.env)}>
          <TrendChart height={300} x={buckets} series={chartSeries} format={fmtInt} legend={!!groupBy} />
        </QueryState>
      </Card>

      <Card title={groupBy ? t(`按 ${groupBy} 分组`, `Grouped by ${groupBy}`) : t('参数', 'Parameters')} info={groupBy ? t('最多显示 8 组，其余合并为「其他」；人数为去重用户', 'Up to 8 groups; the rest are merged into "Other". Users are distinct') : t('该事件带的参数，选择「分组参数」查看取值分布', 'Parameters of this event; pick one to group by and see its values')}>
        <QueryState loading={res.isPending} error={res.error} empty={!res.isPending && (groupBy ? breakdown.length === 0 : params.length === 0)} height={160}
          emptyHint={groupBy || !current ? undefined : t('这个事件没有参数', 'This event has no parameters')}>
          {groupBy ? (
            <BarList
              valueLabel={measure === 'count' ? t('次数', 'Count') : t('人数', 'Users')}
              secondary={t('占比', 'Share')}
              format={fmtInt}
              items={breakdown.map((b) => ({
                key: String(b.value),
                label: b.value === '(other)' ? t('其他', 'Other') : b.value === '(none)' ? t('（无）', '(none)') : String(b.value),
                value: num(b[measure]),
                sub: measure === 'count' ? fmtPct(totalCount ? (num(b.count) / totalCount) * 100 : 0) : undefined,
              }))}
            />
          ) : (
            <div className="flex flex-wrap gap-2">
              {params.map((p) => (
                <button key={String(p.key)} type="button" onClick={() => setParam({ group_by: String(p.key) })}
                  className="press flex h-8 items-center gap-2 rounded-full bg-fill px-3.5 text-small text-text hover:bg-fill-strong hover:text-primary">
                  <span className="font-mono">{String(p.key)}</span>
                  <span className="num text-caption font-normal text-text-3">{t(`${fmtInt(p.distinct_values)} 种取值`, `${fmtInt(p.distinct_values)} values`)}</span>
                </button>
              ))}
            </div>
          )}
        </QueryState>
      </Card>
    </Page>
  )
}
