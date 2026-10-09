// User detail
import { Link, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft } from 'lucide-react'
import { api, num, rows, type Cell } from '../api'
import { useFilters, useMeta } from '../data'
import { addDays, fmtDuration, fmtInt, fmtRelativeDay } from '../format'
import { Page } from '../components/Layout'
import { Badge, Card, CopyButton, EmptyState, Grid, QueryState, tierMeta } from '../components/ui'
import { CalendarHeatmap, Timeline } from '../components/Viz'
import { Tier } from './tier'
import { t } from '../prefs'

type Row = Record<string, Cell>

export function UserDetailPage() {
  const { id = '' } = useParams()
  const f = useFilters()
  const tz = useMeta().timezone
  const q = useQuery({ queryKey: ['user', f.app, id], queryFn: () => api.user(id, f.app) })
  const summary = rows<Row>(q.data, 'summary')[0]
  const calendar = rows<Row>(q.data, 'calendar')
  const events = rows<Row>(q.data, 'events')
  const since = addDays(f.today, -27)
  const days28 = new Set(calendar.filter((c) => String(c.day) >= since).map((c) => String(c.day))).size
  const tier = Tier(days28)
  const download = summary?.acquired_day

  const info: [string, React.ReactNode][] = summary
    ? [
        [t('首见', 'First seen'), String(summary.first_seen_day)],
        [t('首次下载', 'First download'), download && download !== summary.first_seen_day ? String(download) : String(summary.first_seen_day)],
        [t('最后活跃', 'Last seen'), fmtRelativeDay(summary.last_seen_day, f.today)],
        [t('累计活跃', 'Active days'), t(`${fmtInt(summary.active_days)} 天`, `${fmtInt(summary.active_days)}d`)],
        [t('会话', 'Sessions'), fmtInt(summary.sessions)],
        [t('总时长', 'Total time'), fmtDuration(summary.duration_s)],
        [t('版本', 'Version'), `${summary.first_version ?? '—'} → ${summary.last_version ?? '—'}`],
        [t('机型', 'Device'), String(summary.device_name ?? '—')],
        [t('系统', 'OS'), `iOS ${summary.os_version ?? '—'}`],
        [t('地区', 'Region'), String(summary.region ?? '—')],
        ['App Store', String(summary.storefront ?? '—')],
        [t('语言', 'Language'), String(summary.language ?? '—')],
        [t('环境', 'Environment'), String(summary.env ?? '—')],
      ]
    : []

  return (
    <Page title={t('用户详情', 'User detail')} subtitle={t(`活跃日历与最近的事件（${tz} 时间）`, `Activity calendar and recent events (${tz} time)`)} hideFilters>
      <div className="flex flex-wrap items-center gap-2">
        <Link to={`/users${f.keep}`} className="flex h-8 items-center gap-1 rounded-sm pr-2.5 pl-1.5 text-small text-text-2 transition-ui hover:bg-surface-2 hover:text-text">
          <ArrowLeft size={16} strokeWidth={1.75} />
          {t('用户列表', 'User list')}
        </Link>
        <span className="mx-1 h-4 w-px bg-border" />
        <span className="font-mono text-small break-all text-text">{id}</span>
        <CopyButton text={id} label={t('复制 install_id', 'Copy install_id')} />
        {summary && (
          <span className="flex items-center gap-1">
            <Badge tone={tierMeta(tier)?.tone}>{tierMeta(tier)?.label}</Badge>
            {num(summary.is_paid) === 1 && <Badge tone="success">{t('已付费', 'Paid')}</Badge>}
            {num(summary.is_new) === 1 && <Badge tone="c2">{t('真新用户', 'New user')}</Badge>}
          </span>
        )}
      </div>

      <QueryState loading={q.isPending} error={q.error} empty={!q.isPending && !summary} height={320} emptyHint={t('没有找到这个用户，可能属于其他 App', 'User not found; it may belong to another app')}>
        <Grid>
          <Card span={4} title={t('基础信息', 'Profile')}>
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-6 gap-y-2.5 text-small">
              {info.map(([k, v]) => (
                <div key={k} className="contents">
                  <dt className="text-text-3">{k}</dt>
                  <dd className="num truncate text-right text-text">{v}</dd>
                </div>
              ))}
            </dl>
          </Card>
          <Card span={8} title={t('活跃日历', 'Activity calendar')} info={t('近 26 周每天的会话次数', 'Daily sessions over the last 26 weeks')}>
            <CalendarHeatmap
              today={f.today}
              days={calendar.map((c) => ({ day: String(c.day), sessions: num(c.sessions) || (num(c.events) > 0 ? 1 : 0), duration: num(c.duration_s) }))}
            />
          </Card>
        </Grid>
        <Card title={<>{t('事件时间线', 'Event timeline')} <span className="num ml-1 text-small font-normal text-text-3">{t(`最近 ${fmtInt(events.length)} 条`, `Latest ${fmtInt(events.length)}`)}</span></>}>
          {events.length ? <Timeline rows={events} moduleName={f.moduleName} /> : <EmptyState />}
        </Card>
      </QueryState>
    </Page>
  )
}
