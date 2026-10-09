// Revenue
import { num, rows, type Cell } from '../api'
import { paywallLabel, productName, useCli, useFilters, useMeta, useMetric } from '../data'
import { addDays, fmtInt, fmtMoney, fmtMoneyAxis, fmtPct, moneyBasis } from '../format'
import { t } from '../prefs'
import { Page } from '../components/Layout'
import { BarList, Card, DataTable, Grid, KpiCard, KpiRow, QueryState, type Column } from '../components/ui'
import { ColumnChart, TrendChart } from '../components/Chart'

type Row = Record<string, Cell>

export function RevenuePage() {
  const f = useFilters()
  const tz = useMeta().timezone
  const cli = useCli()
  const rev = useMetric('revenue')
  const fun = useMetric('funnel')
  const summary = rows<Row>(rev.data, 'summary')[0]
  const funnel = rows<Row>(fun.data, 'by_context')
  type Totals = { shown: number; cta: number; started: number; success: number; sales: number }
  const total = funnel.reduce<Totals>(
    (a, r) => ({ shown: a.shown + num(r.shown_users), cta: a.cta + num(r.cta_users), started: a.started + num(r.started_users), success: a.success + num(r.success_users), sales: a.sales + num(r.verified_sales) }),
    { shown: 0, cta: 0, started: 0, success: 0, sales: 0 },
  )
  const conv = total.shown ? (total.success / total.shown) * 100 : null
  const label = (k: Cell) => paywallLabel(f.appMeta, String(k))
  const product = (k: Cell) => productName(f.appMeta, String(k))

  const columns: Column<Row>[] = [
    { key: 'context', title: t('入口', 'Entry point'), render: (r) => <span className="font-medium">{label(r.context)}</span> },
    { key: 'shown', title: t('曝光', 'Shown'), align: 'right', render: (r) => fmtInt(r.shown_users), sortValue: (r) => num(r.shown_users) },
    { key: 'cta', title: t('点击', 'Tapped'), align: 'right', render: (r) => fmtInt(r.cta_users), sortValue: (r) => num(r.cta_users) },
    { key: 'started', title: t('发起购买', 'Started'), align: 'right', render: (r) => fmtInt(r.started_users) },
    { key: 'success', title: t('购买成功', 'Succeeded'), align: 'right', render: (r) => fmtInt(r.success_users), sortValue: (r) => num(r.success_users) },
    { key: 'sales', title: t('服务器确认', 'Verified'), align: 'right', render: (r) => <span className="text-success">{fmtInt(r.verified_sales)}</span> },
    { key: 'trials', title: t('试用开通', 'Trials'), align: 'right', render: (r) => <span className="text-text-2">{fmtInt(r.verified_trials)}</span> },
    { key: 'cta_rate', title: t('点击率', 'Tap rate'), align: 'right', render: (r) => <span className="text-text-2">{fmtPct(r.cta_rate_pct)}</span> },
    { key: 'rate', title: t('转化率', 'Conversion'), align: 'right', render: (r) => <span className="font-medium">{fmtPct(r.success_rate_pct)}</span>, sortValue: (r) => num(r.success_rate_pct) },
  ]

  const steps = [
    { name: t('曝光', 'Shown'), v: total.shown },
    { name: t('点击', 'Tapped'), v: total.cta },
    { name: t('发起', 'Started'), v: total.started },
    { name: t('成功', 'Succeeded'), v: total.success },
  ]
  const moneyItems = (table: string, lab: (k: Cell) => string) =>
    rows<Row>(rev.data, table).map((r) => ({ key: String(r.key), label: lab(r.key), value: num(r.net), sub: t(`${fmtInt(r.sales)} 笔`, `${fmtInt(r.sales)} sales`) }))
  const days = rows<Row>(rev.data, 'days_to_purchase')
  // The API only returns days with transactions; fill the filter range day by day with 0
  const byDay = new Map(rows<Row>(rev.data, 'daily').map((r) => [String(r.key), r]))
  const axis: string[] = []
  for (let d = f.from; d <= f.to; d = addDays(d, 1)) axis.push(d)
  const hasDaily = byDay.size > 0

  return (
    <Page title={t('付费', 'Revenue')} subtitle={t('钱从哪里来（收入来自 App Store 服务器通知，真实成交）', 'Where the money comes from (App Store server notifications, real transactions)')}>
      <KpiRow>
        <KpiCard label={t('净收入', 'Net revenue')} loading={rev.isPending} value={fmtMoney(summary?.net ?? 0)} info={t(`成交减退款（${moneyBasis()}，未扣除 Apple 抽成）`, `Sales minus refunds (${moneyBasis()}, before Apple's commission)`)} />
        <KpiCard label={t('成交笔数', 'Sales')} loading={rev.isPending} value={fmtInt(summary?.sales ?? 0)}
          suffix={num(summary?.trials) > 0 ? t(`另有 ${fmtInt(summary?.trials)} 笔试用开通`, `+${fmtInt(summary?.trials)} trials`) : undefined}
          info={t('App Store 服务器确认的购买 / 续订笔数，只算价格大于 0 的交易；免费试用开通单独计数', 'Purchases / renewals verified by the App Store server with a price above 0; free trial starts are counted separately')} />
        <KpiCard label={t('退款笔数', 'Refunds')} loading={rev.isPending} value={fmtInt(summary?.refunds ?? 0)} info={t('App Store 服务器通知的退款笔数', 'Refunds reported by App Store server notifications')} />
        <KpiCard label={t('付费墙转化率', 'Paywall conversion')} loading={fun.isPending} value={fmtPct(conv)} info={t('购买成功人数 ÷ 看到付费墙的人数（去重用户，所有入口合计）', 'Users who purchased ÷ users who saw the paywall (unique users, all entry points)')} />
      </KpiRow>

      <Card title={t('每日收入', 'Daily revenue')} info={t(`柱为当天净收入（成交减退款，${moneyBasis()}），线为当天成交笔数（不含试用开通）；按 ${tz} 日期`, `Bars: net revenue per day (sales minus refunds, ${moneyBasis()}); line: sales per day (excluding trials); ${tz} dates`)} cli={cli('revenue')}>
        <QueryState loading={rev.isPending} error={rev.error} empty={!rev.isPending && !hasDaily} height={260} emptyHint={t('这段时间没有成交', 'No sales in this period')}>
          <TrendChart
            height={260}
            x={axis}
            format={fmtMoney}
            axisFormat={fmtMoneyAxis}
            rightFormat={fmtInt}
            series={[
              { name: t('净收入', 'Net revenue'), type: 'bar', color: 'var(--c1)', data: axis.map((d) => num(byDay.get(d)?.net)) },
              { name: t('成交笔数', 'Sales'), type: 'line', color: 'var(--c2)', yAxisIndex: 1, format: fmtInt, data: axis.map((d) => num(byDay.get(d)?.sales)) },
            ]}
          />
        </QueryState>
      </Card>

      <Card title={t('付费漏斗', 'Paywall funnel')} info={t('按付费墙入口拆分，均为去重用户数；服务器确认 = 通过 appAccountToken 关联到的真实成交（价格大于 0），试用开通单独一列', 'By paywall entry point, unique users; Verified = real sales (price above 0) linked via appAccountToken; trials shown separately')} cli={cli('funnel')}>
        <QueryState loading={fun.isPending} error={fun.error} empty={!fun.isPending && funnel.length === 0} height={200}
          emptyHint={t('这段时间没有付费墙曝光', 'No paywall impressions in this period')}>
          <div className="mb-5 grid grid-cols-4 gap-1.5">
            {steps.map((s, i) => (
              <div key={s.name} className="min-w-0">
                <div className="h-2 rounded-full bg-primary-soft">
                  <div className="h-2 rounded-full bg-primary" style={{ width: `${total.shown ? Math.max(3, (s.v / total.shown) * 100) : 0}%`, opacity: 1 - i * 0.15 }} />
                </div>
                <div className="mt-2 flex items-baseline justify-between gap-2">
                  <span className="truncate text-caption text-text-2">{s.name}</span>
                  <span className="num text-small font-medium text-text">{fmtInt(s.v)}</span>
                </div>
                {i > 0 && <div className="num text-caption font-normal text-text-3">{steps[i - 1].v ? fmtPct((s.v / steps[i - 1].v) * 100) : '—'}</div>}
              </div>
            ))}
          </div>
          <DataTable columns={columns} data={funnel} rowKey={(r) => String(r.context)} defaultSort={{ key: 'shown', desc: true }} />
        </QueryState>
      </Card>

      <Grid>
        <Card span={6} title={t('按商品', 'By product')} cli={cli('revenue')}>
          <QueryState loading={rev.isPending} error={rev.error} empty={moneyItems('by_product', product).length === 0} height={160} emptyHint={t('这段时间没有成交', 'No sales in this period')}>
            <BarList items={moneyItems('by_product', product)} format={fmtMoney} valueLabel={t('净收入', 'Net revenue')} secondary={t('笔数', 'Sales')} />
          </QueryState>
        </Card>
        <Card span={6} title={t('按入口', 'By entry point')} info={t('成交通过 appAccountToken 关联到发起购买时的付费墙入口', 'Sales are linked to the paywall entry point via appAccountToken')}>
          <QueryState loading={rev.isPending} error={rev.error} empty={moneyItems('by_context', label).length === 0} height={160} emptyHint={t('这段时间没有成交', 'No sales in this period')}>
            <BarList items={moneyItems('by_context', label)} format={fmtMoney} valueLabel={t('净收入', 'Net revenue')} secondary={t('笔数', 'Sales')} />
          </QueryState>
        </Card>
        <Card span={6} title={t('按 App Store 国家', 'By App Store country')}>
          <QueryState loading={rev.isPending} error={rev.error} empty={moneyItems('by_storefront', (k) => String(k)).length === 0} height={160} emptyHint={t('这段时间没有成交', 'No sales in this period')}>
            <BarList items={moneyItems('by_storefront', (k) => String(k))} format={fmtMoney} valueLabel={t('净收入', 'Net revenue')} secondary={t('笔数', 'Sales')} />
          </QueryState>
        </Card>
        <Card span={6} title={t('安装后第几天付费', 'Days from install to purchase')} info={t('从首次下载日算起，获客当天为第 1 天', 'Counted from the first download date; install day is day 1')}>
          <QueryState loading={rev.isPending} error={rev.error} empty={days.length === 0} height={200} emptyHint={t('这段时间没有成交', 'No sales in this period')}>
            <ColumnChart categories={days.map((d) => t(`第 ${d.install_day} 天`, `Day ${d.install_day}`))} values={days.map((d) => num(d.sales))} color="var(--money)" name={t('成交', 'Sales')} format={fmtInt} height={200} />
          </QueryState>
        </Card>
      </Grid>
      <p className="text-caption font-normal text-text-3">{f.env === 'production' ? '' : t('沙盒环境的成交来自 TestFlight / 沙盒账号，不是真实收入。', 'Sandbox sales come from TestFlight / sandbox accounts and are not real revenue.')}</p>
    </Page>
  )
}
