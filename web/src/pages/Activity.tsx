// Engagement and stickiness
import { useState } from 'react'
import { num, rows } from '../api'
import { useCli, useFilters, useMetric } from '../data'
import { fmtDuration, fmtInt, fmtPct } from '../format'
import { Page } from '../components/Layout'
import { Card, Grid, KpiCard, QueryState, Segmented } from '../components/ui'
import { ColumnChart, TrendChart } from '../components/Chart'
import { emptyHint } from './Overview'
import { t } from '../prefs'

export function ActivityPage() {
  const f = useFilters()
  const cli = useCli()
  const act = useMetric('active_users')
  const days = useMetric('activity_days')
  const ret = useMetric('returning')
  const sess = useMetric('sessions_daily')
  const [mode, setMode] = useState<'users' | 'sticky'>('users')

  const daily = rows(act.data, 'daily')
  const l7 = rows(days.data, 'l7')
  const l28 = rows(days.data, 'l28')
  const summary = rows(ret.data, 'summary')[0]
  const retDaily = rows(ret.data, 'daily')
  const sessDaily = rows(sess.data, 'daily')
  const resurrected = retDaily.reduce((s, d) => s + num(d.resurrected), 0)
  const empty = !act.isPending && daily.every((d) => num(d.dau) === 0)

  return (
    <Page title={t('活跃与粘性', 'Engagement')} subtitle={t('用户是不是天天来', 'Do users come back every day')}>
      <Card title={mode === 'users' ? 'DAU / WAU / MAU' : t('粘性 DAU / MAU', 'Stickiness DAU / MAU')} cli={cli('active_users')}
        info={t('DAU：当天活跃；WAU：含当天的近 7 天；MAU：近 30 天。粘性 = DAU ÷ MAU', 'DAU: active that day; WAU: last 7 days incl. that day; MAU: last 30 days. Stickiness = DAU ÷ MAU')}
        actions={<Segmented value={mode} onChange={setMode} size="sm" options={[{ value: 'users', label: t('人数', 'Users') }, { value: 'sticky', label: t('粘性', 'Stickiness') }]} />}>
        <QueryState loading={act.isPending} error={act.error} empty={empty} height={280} emptyHint={emptyHint(f.env)}>
          {mode === 'users' ? (
            <TrendChart
              x={daily.map((d) => String(d.day))}
              format={fmtInt}
              series={[
                { name: 'DAU', data: daily.map((d) => num(d.dau)), color: 'var(--c1)', area: true },
                { name: 'WAU', data: daily.map((d) => num(d.wau)), color: 'var(--c5)' },
                { name: 'MAU', data: daily.map((d) => num(d.mau)), color: 'var(--c6)' },
              ]}
            />
          ) : (
            <TrendChart
              x={daily.map((d) => String(d.day))}
              format={(v) => `${v}%`}
              series={[{ name: t('粘性', 'Stickiness'), data: daily.map((d) => num(d.stickiness_pct)), color: 'var(--c1)', area: true, format: fmtPct }]}
            />
          )}
        </QueryState>
      </Card>

      <Grid>
        <Card span={6} title={t('近 7 天活跃天数（L7）', 'Active days in last 7 (L7)')} info={t(`截至 ${f.to}，近 7 天里每个活跃用户来了几天`, `As of ${f.to}, how many of the last 7 days each active user came`)} cli={cli('activity_days')}>
          <QueryState loading={days.isPending} error={days.error} empty={l7.every((r) => num(r.users) === 0)} height={220}>
            <ColumnChart
              categories={l7.map((r) => t(`${r.active_days_in_7} 天`, `${r.active_days_in_7}d`))}
              values={l7.map((r) => num(r.users))}
              format={fmtInt}
              tooltipLabel={(c) => t(`7 天里来了 ${c}`, `Came ${c} of 7 days`)}
            />
          </QueryState>
        </Card>
        <Card span={6} title={t('近 28 天活跃天数（L28）', 'Active days in last 28 (L28)')} info={t(`截至 ${f.to}，近 28 天里每个活跃用户来了几天`, `As of ${f.to}, how many of the last 28 days each active user came`)} cli={cli('activity_days')}>
          <QueryState loading={days.isPending} error={days.error} empty={l28.every((r) => num(r.users) === 0)} height={220}>
            <ColumnChart
              categories={l28.map((r) => t(`${r.active_days_in_28} 天`, `${r.active_days_in_28}d`))}
              values={l28.map((r) => num(r.users))}
              color="var(--c5)"
              format={fmtInt}
              tooltipLabel={(c) => t(`28 天里来了 ${c}`, `Came ${c} of 28 days`)}
            />
          </QueryState>
        </Card>
      </Grid>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 sm:gap-4">
        <KpiCard label={t('回流用户（期间）', 'Resurrected (period)')} loading={ret.isPending} value={fmtInt(resurrected)}
          info={t('期间内有过「连续 ≥7 天没来后又回来」的用户人次', 'Returns after ≥7 consecutive inactive days during the period')} spark={retDaily.map((d) => num(d.resurrected))} sparkColor="var(--seg-resurrected)" />
        <KpiCard label={t('流失风险', 'At risk')} loading={ret.isPending} value={fmtInt(summary?.at_risk_7_13d)} suffix={t('人', 'users')}
          info={t(`截至 ${f.to}，最后一次活跃在 7–13 天前的用户`, `As of ${f.to}, users last active 7–13 days ago`)} />
        <KpiCard label={t('已流失', 'Churned')} loading={ret.isPending} value={fmtInt(summary?.churned_14d_plus)} suffix={t('人', 'users')}
          info={t(`截至 ${f.to}，最后一次活跃在 14 天及以前的用户（累计）`, `As of ${f.to}, users last active 14+ days ago (cumulative)`)} />
      </div>

      <Card title={t('每日会话', 'Daily sessions')} info={t('柱：会话数；线：当天结束会话的前台时长中位数（右轴）', 'Bars: sessions; line: median foreground duration of sessions ended that day (right axis)')} cli={cli('sessions_daily')}>
        <QueryState loading={sess.isPending} error={sess.error} empty={empty} height={280}>
          <TrendChart
            x={sessDaily.map((d) => String(d.day))}
            format={fmtInt}
            rightFormat={(v) => fmtDuration(v)}
            series={[
              { name: t('会话数', 'Sessions'), type: 'bar', data: sessDaily.map((d) => num(d.sessions)), color: 'var(--c1)' },
              { name: t('时长中位数', 'Median duration'), type: 'line', yAxisIndex: 1, data: sessDaily.map((d) => (d.median_session_s == null ? null : num(d.median_session_s))), color: 'var(--c3)' },
            ]}
          />
        </QueryState>
      </Card>
    </Page>
  )
}
