// UI language and display currency, stored in localStorage. Switching remounts the whole tree,
// so t() and the formatters can read the module-level current values without subscribing
import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'

export type Lang = 'zh' | 'en'
/** Display currency: an ISO code; amounts are in the base currency and converted at the rates from meta */
export type Currency = string

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* not writable (e.g. private mode); still applies for this session */
  }
}

function initialLang(): Lang {
  const saved = read('hi-lang')
  if (saved === 'zh' || saved === 'en') return saved
  return navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en'
}

function initialCurrency(): Currency {
  const saved = read('hi-currency')
  return saved && /^[A-Z]{3}$/.test(saved) ? saved : ''
}

let lang: Lang = initialLang()
/** Selected display currency; empty means the base currency */
let currency: Currency = initialCurrency()
/** Base currency of all amounts, from the meta API */
let base: Currency = 'USD'
/** 1 base currency = N units, from the meta API */
let rates: Record<string, number> = {}

export const getLang = () => lang
export const locale = () => (lang === 'en' ? 'en-US' : 'zh-CN')

/** Bilingual text: t('概览', 'Overview') */
export function t(zh: string, en: string): string {
  return lang === 'en' ? en : zh
}

export function setMoneyBasis(baseCurrency: string | undefined, r: Record<string, number> | undefined) {
  base = baseCurrency || 'USD'
  rates = r ?? {}
}

export const baseCurrency = () => base

/** Currencies currently available: the base currency plus every code with a rate */
export function availableCurrencies(): Currency[] {
  return [base, ...Object.keys(rates).filter((c) => c !== base && rates[c] > 0).sort()]
}

/** How many units of the currency 1 base currency buys; undefined without a rate */
export function rateOf(c: Currency): number | undefined {
  return c === base ? 1 : rates[c] > 0 ? rates[c] : undefined
}

/** Effective currency and conversion factor; falls back to the base currency when the selected one has no rate */
export function moneyUnit(): { code: Currency; rate: number } {
  if (currency && currency !== base && rates[currency] > 0) return { code: currency, rate: rates[currency] }
  return { code: base, rate: 1 }
}

/** Localized currency name, e.g. US Dollar / 美元; falls back to the code */
export function currencyName(c: Currency): string {
  try {
    return new Intl.DisplayNames([locale()], { type: 'currency' }).of(c) ?? c
  } catch {
    return c
  }
}

interface Prefs {
  lang: Lang
  currency: Currency
  setLang: (l: Lang) => void
  setCurrency: (c: Currency) => void
}

const PrefsContext = createContext<Prefs | null>(null)

export function PrefsProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState({ lang, currency })
  document.documentElement.lang = state.lang === 'en' ? 'en' : 'zh-CN'
  const value = useMemo<Prefs>(
    () => ({
      ...state,
      setLang: (l) => {
        lang = l
        write('hi-lang', l)
        setState((s) => ({ ...s, lang: l }))
      },
      setCurrency: (c) => {
        currency = c
        write('hi-currency', c)
        setState((s) => ({ ...s, currency: c }))
      },
    }),
    [state],
  )
  return (
    <PrefsContext.Provider value={value}>
      <div key={`${state.lang}-${state.currency}`} className="contents">{children}</div>
    </PrefsContext.Provider>
  )
}

export function usePrefs(): Prefs {
  const p = useContext(PrefsContext)
  if (!p) throw new Error('PrefsProvider missing')
  return p
}
