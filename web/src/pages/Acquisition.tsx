// Acquisition: where users come from on the App Store (impressions -> product page -> first download)
import { CloudDownload } from 'lucide-react'
import { num, rows, type Cell } from '../api'
import { useCli, useMetric } from '../data'
import { countryName, fmtInt, fmtLongDay, fmtPct } from '../format'
import { t } from '../prefs'
import { Page } from '../components/Layout'
import { BarList, Card, DataTable, Grid, KpiCard, KpiRow, QueryState, type Column } from '../components/ui'
import { TrendChart } from '../components/Chart'

type Row = Record<string, Cell>

export const sourceLabel = (): Record<string, string> => ({
  search: t('App Store 搜索', 'App Store search'),
  browse: t('App Store 浏览', 'App Store browse'),
  app_referrer: t('App 引荐', 'App referrer'),
  web_referrer: t('网页引荐', 'Web referrer'),
  other: t('其他', 'Other'),
})

export function AcquisitionPage() {
  const cli = useCli()
  const q = useMetric('store_funnel')
  const status = rows(q.data, 'status')[0]
  const synced = status?.synced === true
  const win = rows(q.data, 'window')[0]
  const app = rows<Row>(q.data, 'apps')[0] ?? {}
  const daily = rows<Row>(q.data, 'daily')
  const sources = rows<Row>(q.data, 'sources')
  const territories = rows<Row>(q.data, 'territories')
  // No comparison when the previous period is incomplete (no data before reports were enabled)
  const prevOk = win?.prev_complete === true
  const prev = (v: Cell) => (prevOk ? v : undefined)
  const prevCvr = prevOk ? app.prev_cvr : undefined

  const subtitle = synced && status?.latest_date
    ? t(`用户从 App Store 的哪里来 · 报表截至 ${fmtLongDay(String(status.latest_date))}`,
      `Where users come from on the App Store · reports through ${fmtLongDay(String(status.latest_date))}`)
    : t('用户从 App Store 的哪里来', 'Where users come from on the App Store')

  const imp = num(app.impressions_unique)
  const pv = num(app.page_views_unique)
  const first = num(app.first_downloads)
  const steps = [
    { name: t('曝光', 'Impressions'), v: imp },
    { name: t('产品页', 'Page views'), v: pv },
    { name: t('首次下载', 'First downloads'), v: first },
  ]

  const sources_ = sourceLabel()
  const columns: Column<Row>[] = [
    { key: 'source', title: t('来源', 'Source'), render: (r) => <span className="font-medium">{sources_[String(r.source)] ?? String(r.source)}</span> },
    { key: 'imp', title: t('曝光', 'Impressions'), align: 'right', sortValue: (r) => num(r.impressions_unique), render: (r) => (num(r.impressions_unique) ? fmtInt(r.impressions_unique) : <span className="text-text-3">—</span>) },
    { key: 'pv', title: t('产品页', 'Page views'), align: 'right', sortValue: (r) => num(r.page_views_unique), render: (r) => fmtInt(r.page_views_unique) },
    { key: 'first', title: t('首次下载', 'First downloads'), align: 'right', sortValue: (r) => num(r.first_downloads), render: (r) => fmtInt(r.first_downloads) },
    { key: 'cvr', title: t('转化率', 'Conversion'), align: 'right', sortValue: (r) => num(r.cvr), render: (r) => <span className="font-medium">{fmtPct(r.cvr)}</span> },
  ]

  return (
    <Page title={t('获客', 'Acquisition')} subtitle={subtitle} hideEnv>
      {q.isPending ? (
        <div className="skeleton h-[420px] w-full" />
      ) : q.error ? (
        <QueryState loading={false} error={q.error} onRetry={() => q.refetch()}>{null}</QueryState>
      ) : !synced ? (
        <Card>
          <div className="flex flex-col items-center gap-2 py-10 text-center">
            <CloudDownload size={28} strokeWidth={1.5} className="text-text-3" />
            <div className="text-body text-text-2">{t('还没有 App Store 分析报表', 'No App Store analytics reports yet')}</div>
            <div className="max-w-md text-small text-text-3">{t('服务器会自动为每个 App 开通分析报表，Apple 1～2 天后开始出数据，每 6 小时同步一次。', 'The server requests analytics reports for each app automatically; Apple starts delivering data after 1–2 days, synced every 6 hours.')}</div>
          </div>
        </Card>
      ) : (
        <>
          <KpiRow>
            <KpiCard label={t('曝光', 'Impressions')} value={fmtInt(imp)} current={imp} previous={prev(app.prev_impressions_unique)}
              spark={daily.map((d) => num(d.impressions_unique))} sparkColor="var(--c1)"
              info={t(`${win?.from} 至 ${win?.to}，在搜索结果、榜单、Today 等位置被看到的设备数（去重）`, `${win?.from} to ${win?.to}, unique devices that saw the app in search results, charts, Today, etc.`)} />
            <KpiCard label={t('产品页浏览', 'Page views')} value={fmtInt(pv)} info={t('打开产品页的设备数（去重）', 'Unique devices that opened the product page')} />
            <KpiCard label={t('首次下载', 'First downloads')} value={fmtInt(first)} current={first} previous={prev(app.prev_first_downloads)}
              spark={daily.map((d) => num(d.first_downloads))} sparkColor="var(--c2)" info={t('首次下载（不含重新下载、更新），来自分析报表', 'First-time downloads (excludes redownloads and updates), from analytics reports')} />
            <KpiCard label={t('转化率', 'Conversion')} value={fmtPct(app.cvr)} current={app.cvr} previous={prevCvr}
              info={t('首次下载 ÷ 去重曝光，与 App Store Connect 的转化率同口径', 'First downloads ÷ unique impressions, same as App Store Connect conversion rate')} />
          </KpiRow>

          <Grid>
            <Card span={8} title={t('每日趋势', 'Daily trend')} info={t('曝光（柱，左轴）与首次下载（线，右轴）', 'Impressions (bars, left axis) and first downloads (line, right axis)')} cli={cli('store_funnel')}>
              <TrendChart
                height={280}
                x={daily.map((d) => String(d.date))}
                series={[
                  { name: t('曝光', 'Impressions'), type: 'bar', color: 'var(--c1)', data: daily.map((d) => num(d.impressions_unique)) },
                  { name: t('首次下载', 'First downloads'), type: 'line', color: 'var(--c2)', yAxisIndex: 1, data: daily.map((d) => num(d.first_downloads)) },
                ]}
                format={fmtInt}
                rightFormat={fmtInt}
              />
            </Card>
            <Card span={4} title={t('商店漏斗', 'Store funnel')} info={t('条宽按与曝光的比例；每级下方为相对上一级的转化率', 'Bar width is relative to impressions; each step shows conversion from the previous step')}>
              <div className="flex flex-col gap-4 pt-1">
                {steps.map((s, i) => (
                  <div key={s.name}>
                    <div className="mb-1.5 flex items-baseline justify-between gap-2">
                      <span className="text-small text-text-2">{s.name}</span>
                      <span className="num text-heading text-text">{fmtInt(s.v)}</span>
                    </div>
                    <div className="h-2 rounded-full bg-primary-soft">
                      <div className="h-2 rounded-full bg-primary" style={{ width: `${imp ? Math.max(2, (s.v / imp) * 100) : 0}%`, opacity: 1 - i * 0.2 }} />
                    </div>
                    {i > 0 && (
                      <div className="num mt-1 text-caption font-normal text-text-3">
                        {t('较上一步', 'From previous')} {steps[i - 1].v ? fmtPct(Math.round((s.v / steps[i - 1].v) * 1000) / 10) : '—'}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </Card>
            <Card span={6} title={t('按来源', 'By source')} info={t('引荐类来源 Apple 不统计曝光，转化率显示 —', 'Apple does not count impressions for referrer sources, so conversion shows —')}>
              <DataTable columns={columns} data={sources} rowKey={(r) => String(r.source)} defaultSort={{ key: 'imp', desc: true }} />
            </Card>
            <Card span={6} title={t('国家 / 地区', 'Countries / regions')} info={t('按曝光排序；右侧为首次下载与转化率', 'Sorted by impressions; right side shows first downloads and conversion')}>
              <BarList
                valueLabel={t('曝光', 'Impressions')}
                secondary={t('下载 · 转化', 'DL · CVR')}
                format={fmtInt}
                items={territories.map((r) => ({
                  key: String(r.territory),
                  label: countryName(String(r.territory)),
                  value: num(r.impressions_unique),
                  sub: `${fmtInt(r.first_downloads)} · ${fmtPct(r.cvr)}`,
                  color: 'var(--c1)',
                }))}
              />
            </Card>
          </Grid>
          <p className="text-caption font-normal text-text-3">
            {t('数据来自 App Store Connect 分析报表（仅正式环境），比实际晚约 2～3 天；统计窗口以报表最新一天为终点。',
              'Data from App Store Connect analytics reports (production only), about 2–3 days behind; the window ends on the latest report day.')}
          </p>
        </>
      )}
    </Page>
  )
}
