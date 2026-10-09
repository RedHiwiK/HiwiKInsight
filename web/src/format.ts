// Data formatting; follows the UI language and the display currency
import type { Cell } from './api'
import { baseCurrency, getLang, locale, moneyUnit, t } from './prefs'

const fmtCache = new Map<string, Intl.NumberFormat>()
function nf(key: string, opts: Intl.NumberFormatOptions): Intl.NumberFormat {
  const k = `${locale()}|${key}`
  let f = fmtCache.get(k)
  if (!f) {
    f = new Intl.NumberFormat(locale(), opts)
    fmtCache.set(k, f)
  }
  return f
}

const isNil = (v: Cell | undefined): v is null | undefined => v === null || v === undefined || v === ''

export function fmtInt(v: Cell | undefined): string {
  return isNil(v) ? '—' : nf('int', { maximumFractionDigits: 0 }).format(Number(v))
}

export function fmtNum(v: Cell | undefined): string {
  return isNil(v) ? '—' : nf('dec', { maximumFractionDigits: 1 }).format(Number(v))
}

export function fmtPct(v: Cell | undefined): string {
  return isNil(v) ? '—' : `${nf('dec', { maximumFractionDigits: 1 }).format(Number(v))}%`
}

/** Base-currency amount -> display currency; currencies without minor units (e.g. JPY) show no decimals */
function moneyFormat(whole: boolean): Intl.NumberFormat {
  const { code } = moneyUnit()
  return nf(`money-${code}-${whole}`, {
    style: 'currency',
    currency: code,
    currencyDisplay: 'narrowSymbol',
    ...(whole ? { maximumFractionDigits: 0 } : {}),
  })
}

export function fmtMoney(v: Cell | undefined): string {
  return isNil(v) ? '—' : moneyFormat(false).format(Number(v) * moneyUnit().rate)
}

/** For axes: whole amounts */
export function fmtMoneyAxis(v: number): string {
  return moneyFormat(true).format(v * moneyUnit().rate)
}

/** Currency wording used in metric definitions, e.g. "in USD" or "converted from USD to EUR at today's rate" */
export function moneyBasis(): string {
  const { code } = moneyUnit()
  const base = baseCurrency()
  if (code === base) return t(`单位 ${base}`, `in ${base}`)
  return t(`按今日汇率由 ${base} 折合 ${code}`, `converted from ${base} to ${code} at today's rate`)
}

export function fmtDuration(v: Cell | undefined): string {
  if (isNil(v)) return '—'
  const s = Math.round(Number(v))
  const en = getLang() === 'en'
  if (s < 60) return en ? `${s}s` : `${s} 秒`
  if (s < 3600) {
    const m = Math.floor(s / 60)
    const r = s % 60
    if (en) return r ? `${m}m ${r}s` : `${m}m`
    return r ? `${m} 分 ${r} 秒` : `${m} 分`
  }
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (en) return m ? `${h}h ${m}m` : `${h}h`
  return m ? `${h} 小时 ${m} 分` : `${h} 小时`
}

const weekdays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
const weekdaysEn = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const monthsEn = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

function parseDay(d: string): Date {
  const [y, m, day] = d.split('-').map(Number)
  return new Date(y, (m || 1) - 1, day || 1)
}

/** Axis label: 9/23; monthly granularity: 2026-09 -> 9月 / Sep */
export function fmtAxisDay(d: string): string {
  if (/^\d{4}-\d{2}$/.test(d)) return fmtMonth(Number(d.slice(5)))
  const t = parseDay(d)
  return `${t.getMonth() + 1}/${t.getDate()}`
}

/** Short month name: 9月 / Sep */
export function fmtMonth(m: number): string {
  return getLang() === 'en' ? monthsEn[m - 1] : `${m}月`
}

/** Tooltip title: 9月23日 周三 / Wed, Sep 23 */
export function fmtLongDay(d: string): string {
  const en = getLang() === 'en'
  if (/^\d{4}-\d{2}$/.test(d)) return en ? `${monthsEn[Number(d.slice(5)) - 1]} ${d.slice(0, 4)}` : `${d.slice(0, 4)}年${Number(d.slice(5))}月`
  const t = parseDay(d)
  if (en) return `${weekdaysEn[t.getDay()]}, ${monthsEn[t.getMonth()]} ${t.getDate()}`
  return `${t.getMonth() + 1}月${t.getDate()}日 ${weekdays[t.getDay()]}`
}

/** Relative day: Today / Yesterday / 3d ago */
export function fmtRelativeDay(d: Cell | undefined, today: string): string {
  if (isNil(d)) return '—'
  const diff = Math.round((parseDay(today).getTime() - parseDay(String(d)).getTime()) / 86400000)
  if (diff <= 0) return t('今天', 'Today')
  if (diff === 1) return t('昨天', 'Yesterday')
  if (diff < 30) return t(`${diff} 天前`, `${diff}d ago`)
  return String(d)
}

export function shortId(id: Cell | undefined): string {
  return isNil(id) ? '—' : String(id).slice(0, 8)
}

/** Module descriptions like "Diary timeline (Timeline tab...)": keep only the short name before the parenthesis */
export function shortModuleName(desc: string): string {
  return desc.split(/[（(]/)[0].trim()
}

export function addDays(d: string, n: number): string {
  const t = parseDay(d)
  t.setDate(t.getDate() + n)
  const mm = String(t.getMonth() + 1).padStart(2, '0')
  const dd = String(t.getDate()).padStart(2, '0')
  return `${t.getFullYear()}-${mm}-${dd}`
}

/** Percent change vs. the previous period; null when the previous value is 0 or missing */
export function delta(cur: Cell | undefined, prev: Cell | undefined): number | null {
  if (isNil(cur) || isNil(prev) || Number(prev) === 0) return null
  return ((Number(cur) - Number(prev)) / Math.abs(Number(prev))) * 100
}

const countryNames: Record<string, string> = {
  CN: '中国大陆', TW: '中国台湾', HK: '中国香港', MO: '中国澳门', US: '美国', JP: '日本', SG: '新加坡', MY: '马来西亚',
  KR: '韩国', GB: '英国', CA: '加拿大', AU: '澳大利亚', DE: '德国', FR: '法国', NZ: '新西兰', TH: '泰国',
}

/** App Store country / region code -> name in the current language; unknown codes are returned as is */
export function countryName(code: string): string {
  if (getLang() === 'en') {
    try {
      return new Intl.DisplayNames(['en'], { type: 'region' }).of(code) ?? code
    } catch {
      return code
    }
  }
  return countryNames[code] ?? code
}
