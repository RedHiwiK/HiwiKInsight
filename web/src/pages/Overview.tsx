// Overview
import { num, rows, type Cell } from '../api'
import { useCli, useFilters, useMetric } from '../data'
import { fmtDuration, fmtInt, fmtMoney, fmtNum, fmtPct, moneyBasis } from '../format'
import { Page } from '../components/Layout'
import { BarList, Card, Dot, Grid, KpiCard, KpiRow, QueryState, tierMeta } from '../components/ui'
import { DonutChart, TrendChart } from '../components/Chart'
import { t } from '../prefs'

export function Overview() {
  const f = useFilters()
  const cli = useCli()
  const ov = useMetric('overview')
  const ret = useMetric('returning')
  const mod = useMetric('module_usage')
  const dist = useMetric('distribution')

  const kpi = new Map(rows<{ key: string; current: Cell; previous: Cell }>(ov.data, 'kpis').map((r) => [r.key, r]))
  const daily = rows(ov.data, 'daily')
  const spark = (k: string) => daily.map((d) => num(d[k]))
  const k = (key: string) => kpi.get(key) ?? { current: null, previous: null }
  const loading = ov.isPending

  const retDaily = rows(ret.data, 'daily')
  const tiers = rows<{ tier: string; users: number }>(ov.data, 'tiers').filter((t) => t.tier !== 'dormant')
  const modules = rows(mod.data, 'modules')
  const versions = rows(dist.data, 'app_version')
  const empty = !loading && num(k('active_users').current) === 0

  return (
    <Page title={t('概览', 'Overview')} subtitle={t('今天来了多少人，和上一段时间比怎么样', 'How many people came, compared with the previous period')}>
      <KpiRow>
        <KpiCard label={t('日活 DAU（日均）', 'DAU (daily avg.)')} loading={loading} value={fmtNum(k('avg_dau').current)} current={k('avg_dau').current} previous={k('avg_dau').previous}
          spark={spark('dau')} info={t('当天有任意事件的去重用户数，取期间每日平均；与等长上期对比', 'Unique users with any event that day, averaged over the period; compared with the previous period of equal length')} />
        <KpiCard label={t('期间活跃用户', 'Active users')} loading={loading} value={fmtInt(k('active_users').current)} current={k('active_users').current} previous={k('active_users').previous}
          spark={spark('wau')} sparkColor="var(--c5)" info={t('期间内至少活跃过一天的去重用户数', 'Unique users active on at least one day in the period')} />
        <KpiCard label={t('真新增', 'New users')} loading={loading} value={fmtInt(k('new_users').current)} current={k('new_users').current} previous={k('new_users').previous}
          spark={spark('new_users')} sparkColor="var(--seg-new)" info={t('首次下载日 = 首见日的新用户；升级上来的老用户不算', 'Users whose first download day equals first-seen day; upgraded existing users excluded')} />
        <KpiCard label={t('粘性 DAU / MAU', 'Stickiness DAU / MAU')} loading={loading} value={fmtPct(k('stickiness_pct').current)} current={k('stickiness_pct').current} previous={k('stickiness_pct').previous}
          info={t('日均 DAU ÷ 截至期末的近 30 天活跃用户；越高说明越多人天天来', 'Avg. DAU ÷ 30-day active users as of period end; higher means more daily users')} />
        <KpiCard label={t('会话数', 'Sessions')} loading={loading} value={fmtInt(k('sessions').current)} current={k('sessions').current} previous={k('sessions').previous}
          spark={spark('sessions')} sparkColor="var(--c6)" info={t('session.started 次数；后台超过 5 分钟再回来算新会话', 'session.started count; returning after 5+ minutes in background starts a new session')} />
        <KpiCard label={t('人均日会话', 'Sessions / user-day')} loading={loading} value={fmtNum(k('sessions_per_user_day').current)} current={k('sessions_per_user_day').current} previous={k('sessions_per_user_day').previous}
          info={t('会话数 ÷ 用户活跃天数（一个人活跃一天算一次）', 'Sessions ÷ user active days (one user active one day counts once)')} />
        <KpiCard label={t('会话时长中位数', 'Median session')} loading={loading} value={fmtDuration(k('median_session_s').current)} current={k('median_session_s').current} previous={k('median_session_s').previous}
          spark={daily.map((d) => num(d.median_session_s))} sparkColor="var(--c3)" info={t('当天结束的会话前台时长的中位数', 'Median foreground duration of sessions ended that day')} />
        <KpiCard label={t('净收入', 'Net revenue')} loading={loading} value={fmtMoney(k('net').current)} current={k('net').current} previous={k('net').previous}
          spark={spark('net')} sparkColor="var(--money)" info={t(`App Store 服务器通知的成交减退款（${moneyBasis()}，未扣除 Apple 抽成）`, `Sales minus refunds from App Store server notifications (${moneyBasis()}, before Apple's commission)`)} />
      </KpiRow>

      <Grid>
        <Card span={8} title={t('每日活跃构成', 'Daily active mix')} info={t('新用户：当天首见；回访：此前来过；回流：此前连续 ≥7 天没来', 'New: first seen that day; Returning: seen before; Resurrected: back after ≥7 inactive days')} cli={cli('returning')}>
          <QueryState loading={ret.isPending} error={ret.error} empty={empty} height={280} emptyHint={emptyHint(f.env)}>
            <TrendChart
              x={retDaily.map((d) => String(d.day))}
              series={[
                { name: t('新用户', 'New'), type: 'bar', stack: 'a', color: 'var(--seg-new)', data: retDaily.map((d) => num(d.new)) },
                { name: t('回访', 'Returning'), type: 'bar', stack: 'a', color: 'var(--seg-returning)', data: retDaily.map((d) => num(d.returning) - num(d.resurrected)) },
                { name: t('回流', 'Resurrected'), type: 'bar', stack: 'a', color: 'var(--seg-resurrected)', data: retDaily.map((d) => num(d.resurrected)) },
              ]}
            />
          </QueryState>
        </Card>
        <Card span={4} title={t('用户分层', 'User tiers')} info={t('期间活跃用户按近 28 天活跃天数：重度 ≥15 天、中度 5–14、轻度 2–4、一次性 1 天', 'Active users by active days in the last 28: Heavy ≥15, Medium 5–14, Light 2–4, One-time 1')} cli={cli('users')}>
          <QueryState loading={loading} error={ov.error} empty={empty} height={280}>
            <DonutChart
              centerLabel={t('活跃用户', 'Active users')}
              items={tiers.map((r) => ({ name: tierMeta(r.tier)?.label ?? r.tier, value: r.users, color: tierMeta(r.tier)?.color ?? 'var(--text-3)' }))}
            />
            <ul className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1.5">
              {tiers.map((r) => (
                <li key={r.tier} className="flex items-center gap-2 text-small">
                  <Dot color={tierMeta(r.tier)?.color ?? 'var(--text-3)'} />
                  <span className="text-text-2">{tierMeta(r.tier)?.label ?? r.tier}</span>
                  <span className="num ml-auto font-medium text-text">{fmtInt(r.users)}</span>
                </li>
              ))}
            </ul>
          </QueryState>
        </Card>
        <Card span={6} title={t('模块覆盖率 Top 6', 'Top 6 modules by coverage')} info={t('用过该模块的用户 ÷ 期间活跃用户', 'Users of the module ÷ active users')} cli={cli('module_usage')}>
          <QueryState loading={mod.isPending} error={mod.error} empty={modules.length === 0} height={200}>
            <BarList
              max={6}
              valueLabel={t('覆盖率', 'Coverage')}
              secondary={t('人数', 'Users')}
              format={(v) => fmtPct(v)}
              items={modules.slice(0, 6).map((m) => ({ key: String(m.module), label: f.moduleName(String(m.module)), value: num(m.coverage_pct), sub: fmtInt(m.users) }))}
            />
          </QueryState>
        </Card>
        <Card span={6} title={t('App 版本分布', 'App versions')} info={t('期间活跃用户最新的 App 版本', 'Latest app version of active users')} cli={cli('distribution')}>
          <QueryState loading={dist.isPending} error={dist.error} empty={versions.length === 0} height={200}>
            <BarList
              max={6}
              valueLabel={t('人数', 'Users')}
              secondary={t('占比', 'Share')}
              format={fmtInt}
              items={versions.map((v, i) => ({ key: String(v.value), label: String(v.value), value: num(v.users), sub: fmtPct(v.share_pct), color: i === 0 ? undefined : 'var(--c5)' }))}
            />
          </QueryState>
        </Card>
      </Grid>
    </Page>
  )
}

export function emptyHint(env: string) {
  return env === 'production'
    ? t('正式环境这段时间还没有数据，可以切换到 TestFlight 查看测试数据', 'No production data in this period yet; switch to TestFlight to see test data')
    : t('这段时间没有数据，试试放大时间范围', 'No data in this period; try a wider date range')
}
