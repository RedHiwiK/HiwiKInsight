// Errors
import { num, rows, type Cell } from '../api'
import { useCli, useFilters, useMetric } from '../data'
import { fmtInt } from '../format'
import { Page } from '../components/Layout'
import { Badge, Card, DataTable, EmptyState, KpiCard, QueryState, type Column } from '../components/ui'
import { TrendChart } from '../components/Chart'
import { t } from '../prefs'

type Row = Record<string, Cell>

export function ErrorsPage() {
  const f = useFilters()
  const cli = useCli()
  const res = useMetric('errors')
  const summary = rows<Row>(res.data, 'summary')[0]
  const daily = rows<Row>(res.data, 'daily')
  const list = rows<Row>(res.data, 'errors')

  const columns: Column<Row>[] = [
    { key: 'id', title: t('错误', 'Error'), render: (r) => <span className="font-mono text-small text-text">{String(r.id)}</span> },
    { key: 'category', title: t('分类', 'Category'), render: (r) => (r.category ? <Badge tone="danger">{String(r.category)}</Badge> : <span className="text-text-3">—</span>) },
    { key: 'count', title: t('次数', 'Count'), align: 'right', render: (r) => fmtInt(r.count), sortValue: (r) => num(r.count) },
    { key: 'users', title: t('影响用户', 'Affected users'), align: 'right', render: (r) => <span className="font-medium">{fmtInt(r.users)}</span>, sortValue: (r) => num(r.users) },
    { key: 'versions', title: t('涉及版本', 'Versions'), render: (r) => <span className="num text-text-2">{String(r.versions ?? '—')}</span> },
    { key: 'last', title: t('最近一次', 'Last seen'), render: (r) => <span className="num text-text-2">{String(r.last_time_local ?? '').slice(0, 16)}</span>, sortValue: (r) => String(r.last_time_local) },
    {
      key: 'msg', title: t('最近 message', 'Latest message'), render: (r) => (
        <span className="block max-w-[360px] truncate text-text-2" title={String(r.last_message ?? '')}>{String(r.last_message ?? '—')}</span>
      ),
    },
  ]

  return (
    <Page title={t('错误', 'Errors')} subtitle={t('App 上报的错误，按影响人数排序', 'Errors reported by the app, sorted by affected users')}>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 sm:gap-4">
        <KpiCard label={t('错误次数', 'Errors')} loading={res.isPending} value={fmtInt(summary?.count ?? 0)} spark={daily.map((d) => num(d.count))} sparkColor="var(--danger)" />
        <KpiCard label={t('影响用户', 'Affected users')} loading={res.isPending} value={fmtInt(summary?.users ?? 0)} info={t('期间内至少上报过一次错误的去重用户', 'Distinct users who reported at least one error in this period')} />
        <KpiCard label={t('错误种类', 'Error types')} loading={res.isPending} value={fmtInt(summary?.kinds ?? 0)} info={t('按错误 id 去重', 'Distinct error ids')} />
      </div>
      <Card title={t('每日错误次数', 'Daily errors')} cli={cli('errors')}>
        <QueryState loading={res.isPending} error={res.error} height={240}>
          <TrendChart height={240} x={daily.map((d) => String(d.day))} format={fmtInt}
            series={[{ name: t('错误次数', 'Errors'), type: 'bar', data: daily.map((d) => num(d.count)), color: 'var(--danger)' }]} />
        </QueryState>
      </Card>
      <Card title={t('错误列表', 'Error list')}>
        <QueryState loading={res.isPending} error={res.error} height={200}>
          <DataTable columns={columns} data={list} rowKey={(r) => String(r.id)} defaultSort={{ key: 'users', desc: true }}
            empty={<EmptyState title={t('没有错误', 'No errors')} hint={f.env === 'production' ? t('这段时间正式环境没有上报错误', 'No errors reported in production during this period') : undefined} />} />
        </QueryState>
      </Card>
    </Page>
  )
}
