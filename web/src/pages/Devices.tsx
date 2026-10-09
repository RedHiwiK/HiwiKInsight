// Devices and regions
import { num, rows } from '../api'
import { useCli, useFilters, useMetric } from '../data'
import { fmtInt, fmtPct } from '../format'
import { Page } from '../components/Layout'
import { BarList, Card, Grid, QueryState } from '../components/ui'
import { emptyHint } from './Overview'
import { t } from '../prefs'

const dims = (): { key: string; title: string; prefix?: string }[] => [
  { key: 'device', title: t('机型', 'Device') },
  { key: 'os_version', title: t('iOS 版本', 'iOS version'), prefix: 'iOS ' },
  { key: 'app_version', title: t('App 版本', 'App version') },
  { key: 'region', title: t('系统地区', 'System region') },
  { key: 'storefront', title: t('App Store 国家', 'App Store country') },
  { key: 'language', title: t('语言', 'Language') },
]

const colors = ['var(--c1)', 'var(--c5)', 'var(--c2)', 'var(--c6)', 'var(--c3)', 'var(--c4)']

export function DevicesPage() {
  const f = useFilters()
  const cli = useCli()
  const dist = useMetric('distribution')
  return (
    <Page title={t('设备与地区', 'Devices & Regions')} subtitle={t('期间活跃用户的设备与地区（取每个用户最新属性）', 'Devices and regions of active users (latest attributes per user)')}>
      <Grid>
        {dims().map((d, i) => {
          const items = rows(dist.data, d.key)
          return (
            <Card key={d.key} span={6} title={d.title} cli={i === 0 ? cli('distribution') : undefined}>
              <QueryState loading={dist.isPending} error={dist.error} empty={!dist.isPending && items.length === 0} height={200} emptyHint={emptyHint(f.env)}>
                <BarList
                  valueLabel={t('人数', 'Users')}
                  secondary={t('占比', 'Share')}
                  format={fmtInt}
                  items={items.map((r) => ({
                    key: String(r.value),
                    label: r.value === '(unknown)' ? t('未知', 'Unknown') : `${d.prefix ?? ''}${r.value}`,
                    value: num(r.users),
                    sub: fmtPct(r.share_pct),
                    color: colors[i],
                  }))}
                />
              </QueryState>
            </Card>
          )
        })}
      </Grid>
    </Page>
  )
}
