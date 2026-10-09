// User list
import { useNavigate, useSearchParams } from 'react-router'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { num, rows, type Cell } from '../api'
import { useCli, useFilters, useMetric } from '../data'
import { fmtDuration, fmtInt, fmtRelativeDay, shortId } from '../format'
import { Page } from '../components/Layout'
import { Badge, Card, DataTable, QueryState, Segmented, Toggle, tierMeta, type Column } from '../components/ui'
import { emptyHint } from './Overview'
import { t } from '../prefs'

type Row = Record<string, Cell>
const pageSize = 50
const sortOptions = () => [
  { value: 'last_seen', label: t('最后活跃', 'Last seen') },
  { value: 'active_days', label: t('活跃天数', 'Active days') },
  { value: 'sessions', label: t('会话数', 'Sessions') },
  { value: 'first_seen', label: t('首见', 'First seen') },
]

export function UsersPage() {
  const f = useFilters()
  const cli = useCli()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const tier = sp.get('tier') ?? 'all'
  const paid = sp.get('paid') === '1'
  const fresh = sp.get('new') === '1'
  const sort = sp.get('sort') ?? 'last_seen'
  const page = Math.max(0, Number(sp.get('page') ?? 0) || 0)
  const setParam = (patch: Record<string, string | null>) =>
    setSp((prev) => {
      const n = new URLSearchParams(prev)
      for (const [k, v] of Object.entries(patch)) v === null ? n.delete(k) : n.set(k, v)
      if (!('page' in patch)) n.delete('page')
      return n
    })

  const res = useMetric('users', {
    tier: tier === 'all' ? undefined : tier, paid: paid ? '1' : undefined, new: fresh ? '1' : undefined,
    sort, offset: page * pageSize, limit: pageSize,
  })
  const summary = rows(res.data, 'summary')[0]
  const tiers = new Map(rows<{ tier: string; users: number }>(res.data, 'tiers').map((x) => [x.tier, x.users]))
  const list = rows<Row>(res.data, 'users')
  const total = num(summary?.total)
  const allUsers = num(summary?.all_users)

  const tierOpts = [
    { value: 'all', label: <>{t('全部', 'All')} <span className="num text-text-3">{fmtInt(allUsers)}</span></> },
    ...['heavy', 'medium', 'light', 'once', 'dormant'].map((k) => ({
      value: k,
      label: <>{tierMeta(k)?.label} <span className="num text-text-3">{fmtInt(tiers.get(k) ?? 0)}</span></>,
    })),
  ].filter((o) => o.value !== 'dormant' || (tiers.get('dormant') ?? 0) > 0)

  const columns: Column<Row>[] = [
    { key: 'id', title: t('用户', 'User'), render: (r) => <span className="font-mono text-small text-primary" title={String(r.install_id)}>{shortId(r.install_id)}</span> },
    {
      key: 'tier', title: t('分层', 'Tier'), render: (r) => (
        <span className="flex items-center gap-1">
          <Badge tone={tierMeta(String(r.tier))?.tone}>{tierMeta(String(r.tier))?.label}</Badge>
          {num(r.is_paid) === 1 && <Badge tone="success">{t('付费', 'Paid')}</Badge>}
          {num(r.is_new) === 1 && <Badge tone="c2">{t('新', 'New')}</Badge>}
        </span>
      ),
    },
    { key: 'first', title: t('首见', 'First seen'), render: (r) => <span className="num text-text-2">{String(r.first_seen_day)}</span> },
    { key: 'last', title: t('最后活跃', 'Last seen'), render: (r) => <span className="text-text-2">{fmtRelativeDay(r.last_seen_day, f.today)}</span> },
    { key: 'days28', title: t('近 28 天', 'Last 28d'), align: 'right', render: (r) => t(`${fmtInt(r.days_28)} 天`, `${fmtInt(r.days_28)}d`) },
    { key: 'days', title: t('累计活跃', 'Active days'), align: 'right', render: (r) => t(`${fmtInt(r.active_days)} 天`, `${fmtInt(r.active_days)}d`) },
    { key: 'sessions', title: t('会话', 'Sessions'), align: 'right', render: (r) => fmtInt(r.sessions) },
    { key: 'duration', title: t('总时长', 'Total time'), align: 'right', render: (r) => <span className="text-text-2">{fmtDuration(r.duration_s)}</span> },
    { key: 'device', title: t('机型', 'Device'), render: (r) => <span className="text-text-2">{String(r.device_name ?? '—')}</span> },
    { key: 'version', title: t('版本', 'Version'), render: (r) => <span className="num text-text-2">{String(r.last_version ?? '—')}</span> },
    { key: 'region', title: t('地区', 'Region'), render: (r) => <span className="text-text-2">{String(r.region ?? '—')}</span> },
  ]

  return (
    <Page title={t('用户', 'Users')} subtitle={t('每一个匿名用户的使用情况（期间内活跃过的用户）', 'Usage of each anonymous user active in this period')}>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
        <div className="scroll-thin -mx-4 max-w-[calc(100%+2rem)] overflow-x-auto px-4 sm:mx-0 sm:max-w-full sm:px-0">
          <Segmented value={tier} options={tierOpts} onChange={(v) => setParam({ tier: v === 'all' ? null : v })} ariaLabel={t('用户分层', 'User tier')} />
        </div>
        <Toggle checked={paid} onChange={(v) => setParam({ paid: v ? '1' : null })} label={t('只看付费', 'Paid only')} />
        <Toggle checked={fresh} onChange={(v) => setParam({ new: v ? '1' : null })} label={t('只看真新用户', 'New users only')} />
      </div>

      <Card
        title={<>{t('用户列表', 'User list')} <span className="num ml-1 text-small font-normal text-text-3">{t(`${fmtInt(total)} 人`, `${fmtInt(total)} users`)}</span></>}
        info={t('分层按截至期末的近 28 天活跃天数：重度 ≥15、中度 5–14、轻度 2–4、一次性 1、沉睡 0（期间内来过但近 28 天没来）', 'Tiers by active days in the 28 days up to period end: heavy ≥15, medium 5–14, light 2–4, one-time 1, dormant 0 (active in period but not in the last 28 days)')}
        cli={cli('users', { tier: tier === 'all' ? undefined : tier })}
        actions={<Segmented size="sm" value={sort} options={sortOptions()} onChange={(v) => setParam({ sort: v === 'last_seen' ? null : v })} ariaLabel={t('排序', 'Sort')} />}
      >
        <QueryState loading={res.isPending} error={res.error} empty={!res.isPending && list.length === 0} height={320} emptyHint={emptyHint(f.env)}>
          <DataTable columns={columns} data={list} rowKey={(r) => String(r.install_id)} onRowClick={(r) => navigate(`/users/${r.install_id}${f.keep}`)} />
          {total > pageSize && (
            <div className="mt-4 flex items-center justify-between text-small text-text-2">
              <span className="num">
                {fmtInt(page * pageSize + 1)}–{fmtInt(Math.min((page + 1) * pageSize, total))} / {fmtInt(total)}
              </span>
              <div className="flex gap-1">
                <PageBtn disabled={page === 0} onClick={() => setParam({ page: page - 1 ? String(page - 1) : null })} label={t('上一页', 'Previous page')}><ChevronLeft size={16} /></PageBtn>
                <PageBtn disabled={(page + 1) * pageSize >= total} onClick={() => setParam({ page: String(page + 1) })} label={t('下一页', 'Next page')}><ChevronRight size={16} /></PageBtn>
              </div>
            </div>
          )}
        </QueryState>
      </Card>
    </Page>
  )
}

function PageBtn({ disabled, onClick, label, children }: { disabled: boolean; onClick: () => void; label: string; children: React.ReactNode }) {
  return (
    <button type="button" aria-label={label} disabled={disabled} onClick={onClick}
      className="press flex size-8 items-center justify-center rounded-full bg-fill text-text-2 hover:bg-fill-strong disabled:opacity-40">
      {children}
    </button>
  )
}
