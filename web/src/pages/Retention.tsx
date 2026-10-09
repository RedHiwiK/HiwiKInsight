// Retention
import { useSearchParams } from 'react-router'
import { num, rows, type Cell } from '../api'
import { useCli, useFilters, useMetric } from '../data'
import { addDays, fmtAxisDay, fmtPct } from '../format'
import { Page } from '../components/Layout'
import { Card, KpiCard, QueryState, Segmented } from '../components/ui'
import { TrendChart } from '../components/Chart'
import { CohortHeatmap, HeatLegend } from '../components/Viz'
import { emptyHint } from './Overview'
import { t } from '../prefs'

const dayCols = ['d1_pct', 'd3_pct', 'd7_pct', 'd14_pct', 'd30_pct']
const cols = [...dayCols, 'within_7d_pct', 'within_30d_pct']
const colLabels = () => ['D1', 'D3', 'D7', 'D14', 'D30', t('7 天内', 'Within 7d'), t('30 天内', 'Within 30d')]

export function RetentionPage() {
  const f = useFilters()
  const cli = useCli()
  const [sp, setSp] = useSearchParams()
  const cohort = sp.get('cohort') === 'week' ? 'week' : 'day'
  const ret = useMetric('retention', { cohort: cohort === 'week' ? 'week' : undefined })
  const data = rows<Record<string, Cell>>(ret.data, 'cohorts')
  const all = data.find((r) => r.cohort_day === 'all')
  const cohorts = data.filter((r) => r.cohort_day !== 'all')
  const empty = !ret.isPending && num(all?.new_users) === 0
  const pct = (v: Cell | undefined) => (v == null ? null : num(v))

  return (
    <Page title={t('留存', 'Retention')} subtitle={t('真新用户在第 N 天还回来的比例', 'Share of truly new users who come back on day N')}>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 sm:gap-4">
        <KpiCard label={t('D1 留存', 'D1 retention')} loading={ret.isPending} value={fmtPct(all?.d1_pct)} info={t('首见后第 1 天当天又来的比例（只算已到观测日的用户）', 'Share who came back exactly 1 day after first seen (observable users only)')} />
        <KpiCard label={t('D7 留存', 'D7 retention')} loading={ret.isPending} value={fmtPct(all?.d7_pct)} info={t('首见后第 7 天当天又来的比例', 'Share who came back exactly 7 days after first seen')} />
        <KpiCard label={t('D30 留存', 'D30 retention')} loading={ret.isPending} value={fmtPct(all?.d30_pct)} info={t('首见后第 30 天当天又来的比例；需要至少 30 天的数据', 'Share who came back exactly 30 days after first seen; needs at least 30 days of data')} />
      </div>

      <Card
        title={t('留存热力图', 'Retention heatmap')}
        info={t('Dn：首见后第 n 天当天活跃的比例；7 天内 / 30 天内：这段时间里回来过至少一次。空格表示还没到观测日', 'Dn: share active exactly n days after first seen; Within 7d / 30d: came back at least once in that window. Blank cells are not yet observable')}
        cli={cli('retention', { cohort: cohort === 'week' ? 'week' : undefined })}
        actions={
          <>
            <span className="hidden sm:block"><HeatLegend /></span>
            <Segmented
              size="sm"
              value={cohort}
              onChange={(v) => setSp((prev) => { const n = new URLSearchParams(prev); if (v === 'week') n.set('cohort', 'week'); else n.delete('cohort'); return n })}
              options={[{ value: 'day', label: t('按日', 'Daily') }, { value: 'week', label: t('按周', 'Weekly') }]}
            />
          </>
        }
      >
        <QueryState loading={ret.isPending} error={ret.error} empty={empty} height={240} emptyHint={emptyHint(f.env)}>
          <CohortHeatmap
            columns={colLabels()}
            cohortLabel={(c) => (cohort === 'week' ? `${fmtAxisDay(c)} – ${fmtAxisDay(addDays(c, 6))}` : fmtAxisDay(c))}
            rows={[...cohorts.reverse(), ...(all ? [all] : [])].map((r) => ({
              cohort: String(r.cohort_day),
              size: num(r.new_users),
              cells: cols.map((c) => pct(r[c])),
            }))}
          />
        </QueryState>
      </Card>

      <Card title={t('留存曲线', 'Retention curve')} info={t('全部新用户在 D1 → D30 的留存（精确第 n 天）', 'Retention of all new users from D1 → D30 (exact day n)')}>
        <QueryState loading={ret.isPending} error={ret.error} empty={empty} height={260}>
          <TrendChart
            xIsDay={false}
            height={260}
            x={colLabels().slice(0, 5)}
            format={(v) => `${v}%`}
            series={[{ name: t('留存', 'Retention'), data: dayCols.map((c) => pct(all?.[c])), color: 'var(--c1)', area: true, format: fmtPct }]}
          />
        </QueryState>
      </Card>
    </Page>
  )
}
