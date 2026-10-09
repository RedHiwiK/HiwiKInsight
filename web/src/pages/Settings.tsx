// Settings: appearance, language, display currency (stored in this browser only), and sign-out
import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Monitor, Moon, Sun } from 'lucide-react'
import { api } from '../api'
import { Page } from '../components/Layout'
import { Card, Segmented, Select } from '../components/ui'
import { useTheme, type ThemeChoice } from '../theme'
import { availableCurrencies, baseCurrency, currencyName, moneyUnit, rateOf, t, usePrefs, type Currency, type Lang } from '../prefs'

/** A row in a grouped list: name and hint on the left, control on the right */
function Row({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 border-b border-border py-3 first:pt-0 last:border-b-0 last:pb-0">
      <div className="min-w-0">
        <div className="text-body text-text">{label}</div>
        {hint && <div className="mt-0.5 text-small text-text-3">{hint}</div>}
      </div>
      {children}
    </div>
  )
}

const pill = 'press inline-flex h-8 items-center rounded-full bg-fill px-4 text-small font-medium hover:bg-fill-strong disabled:opacity-40'

function withIcon(icon: ReactNode, label: string) {
  return <span className="inline-flex items-center gap-1.5">{icon}{label}</span>
}

export function SettingsPage() {
  const { choice, setChoice } = useTheme()
  const prefs = usePrefs()
  const themeOpts: { value: ThemeChoice; label: ReactNode }[] = [
    { value: 'system', label: withIcon(<Monitor size={14} strokeWidth={1.75} />, t('跟随系统', 'System')) },
    { value: 'light', label: withIcon(<Sun size={14} strokeWidth={1.75} />, t('浅色', 'Light')) },
    { value: 'dark', label: withIcon(<Moon size={14} strokeWidth={1.75} />, t('深色', 'Dark')) },
  ]
  const langOpts: { value: Lang; label: string }[] = [
    { value: 'zh', label: '中文' },
    { value: 'en', label: 'English' },
  ]
  const me = useQuery({ queryKey: ['me'], queryFn: api.me, staleTime: 5 * 60_000 })
  const [signingOut, setSigningOut] = useState(false)
  const signOut = async () => {
    setSigningOut(true)
    try {
      await api.logout()
    } finally {
      window.location.reload()
    }
  }
  const base = baseCurrency()
  const shown = moneyUnit().code
  const available = availableCurrencies()
  const currencyOpts = available.map((c) => ({ value: c, label: currencyName(c), hint: c }))
  const rate = rateOf(shown)
  const currencyHint =
    shown === base || !rate
      ? t(`金额统一按 ${base} 记录`, `Amounts are recorded in ${base}`)
      : t(`1 ${base} ≈ ${rate.toFixed(4)} ${shown}，按今日汇率折算，历史金额仅供参考`, `1 ${base} ≈ ${rate.toFixed(4)} ${shown} at today's rate; historical amounts are approximate`)

  return (
    <Page title={t('设置', 'Settings')} subtitle={t('外观、语言与币种只保存在当前浏览器', 'Appearance, language and currency are saved in this browser only')} hideFilters>
      <div className="flex max-w-2xl flex-col gap-4 sm:gap-5">
        <Card title={t('外观', 'Appearance')}>
          <Row label={t('主题', 'Theme')}>
            <Segmented value={choice} options={themeOpts} onChange={setChoice} ariaLabel={t('主题', 'Theme')} />
          </Row>
        </Card>
        <Card title={t('语言与货币', 'Language & currency')}>
          <Row label={t('界面语言', 'Language')} hint={t('模块与事件说明按事件字典原文显示', 'Module and event descriptions are shown as written in your event catalogs')}>
            <Segmented value={prefs.lang} options={langOpts} onChange={prefs.setLang} ariaLabel={t('界面语言', 'Language')} />
          </Row>
          <Row
            label={t('展示币种', 'Display currency')}
            hint={available.length > 1 ? currencyHint : t(`汇率暂时不可用，只能显示 ${base}`, `Exchange rates are unavailable; showing ${base} only`)}
          >
            <Select<Currency>
              className="w-40"
              ariaLabel={t('展示币种', 'Display currency')}
              value={shown}
              options={currencyOpts}
              onChange={(c) => prefs.setCurrency(c === base ? '' : c)}
              searchable={currencyOpts.length > 10}
            />
          </Row>
        </Card>
        <Card title={t('报告', 'Reports')}>
          <Row label={t('报告预览', 'Report preview')} hint={t('在新标签页中打开邮件报告的 HTML 预览', 'Open the HTML preview of the email report in a new tab')}>
            <div className="flex gap-2">
              <a href="/v1/admin/report/preview?kind=daily" target="_blank" rel="noreferrer" className={`${pill} text-primary`}>{t('日报', 'Daily')}</a>
              <a href="/v1/admin/report/preview?kind=weekly" target="_blank" rel="noreferrer" className={`${pill} text-primary`}>{t('周报', 'Weekly')}</a>
            </div>
          </Row>
        </Card>
        {me.data?.auth && (
          <Card title={t('账户', 'Account')}>
            <Row label={me.data.username} hint={t('已登录', 'Signed in')}>
              <button type="button" disabled={signingOut} onClick={signOut} className={`${pill} text-danger`}>
                {t('退出登录', 'Sign out')}
              </button>
            </Row>
          </Card>
        )}
      </div>
    </Page>
  )
}
