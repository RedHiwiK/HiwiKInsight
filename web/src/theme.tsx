// Theme: system / light / dark, plus colors resolved from CSS variables for the charts
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

export type ThemeChoice = 'system' | 'light' | 'dark'

interface ThemeState {
  choice: ThemeChoice
  setChoice: (c: ThemeChoice) => void
  /** The effective light/dark mode; charts redraw when it changes */
  resolved: 'light' | 'dark'
}

const ThemeContext = createContext<ThemeState>({ choice: 'system', setChoice: () => {}, resolved: 'light' })

function readChoice(): ThemeChoice {
  try {
    const t = localStorage.getItem('hi-theme')
    if (t === 'light' || t === 'dark') return t
  } catch {
    /* unreadable (e.g. private mode): fall back to system */
  }
  return 'system'
}

function applyTheme(choice: ThemeChoice) {
  const el = document.documentElement
  if (choice === 'system') delete el.dataset.theme
  else el.dataset.theme = choice
  try {
    if (choice === 'system') localStorage.removeItem('hi-theme')
    else localStorage.setItem('hi-theme', choice)
  } catch {
    /* ignore */
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [choice, setChoiceState] = useState<ThemeChoice>(readChoice)
  const [systemDark, setSystemDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const on = (e: MediaQueryListEvent) => setSystemDark(e.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])

  const resolved: 'light' | 'dark' = choice === 'system' ? (systemDark ? 'dark' : 'light') : choice
  // Update data-theme synchronously before the state: children then read the new theme's CSS variables
  const value = useMemo(() => ({ choice, setChoice: (c: ThemeChoice) => { applyTheme(c); setChoiceState(c) }, resolved }), [choice, resolved])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export const useTheme = () => useContext(ThemeContext)

const tokenNames = [
  'bg', 'surface', 'surface-2', 'surface-3', 'border', 'border-strong', 'text', 'text-2', 'text-3',
  'primary', 'success', 'danger', 'warning', 'c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8',
  'seg-new', 'seg-returning', 'seg-resurrected', 'money', 'refund', 'tier-heavy', 'tier-medium', 'tier-light', 'tier-once', 'tier-dormant',
  'heat-0', 'heat-1', 'heat-2', 'heat-3', 'heat-4',
] as const
export type Token = (typeof tokenNames)[number]
export type Palette = Record<Token, string> & { series: string[]; font: string }

/** Resolve tokens to rgba() (the ECharts canvas understands neither CSS variables nor color-mix) */
function resolveTokens(): Palette {
  const probe = document.createElement('span')
  probe.style.display = 'none'
  document.body.appendChild(probe)
  const ctx = document.createElement('canvas').getContext('2d', { willReadFrequently: true })!
  const out = {} as Record<Token, string>
  for (const t of tokenNames) {
    probe.style.color = `var(--${t})`
    ctx.clearRect(0, 0, 1, 1)
    ctx.fillStyle = getComputedStyle(probe).color
    ctx.fillRect(0, 0, 1, 1)
    const [r, g, b, a] = ctx.getImageData(0, 0, 1, 1).data
    out[t] = `rgba(${r}, ${g}, ${b}, ${+(a / 255).toFixed(3)})`
  }
  probe.remove()
  // canvas does not understand font-family: inherit, so pass the concrete font stack
  const font = getComputedStyle(document.body).fontFamily
  return { ...out, font, series: ['c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8'].map((k) => out[k as Token]) }
}

export function usePalette(): Palette {
  const { resolved } = useTheme()
  return useMemo(resolveTokens, [resolved])
}

/** Change the alpha of an rgba() color (for area gradients) */
export function alpha(color: string, a: number): string {
  return color.replace(/rgba\(([^,]+),([^,]+),([^,]+),[^)]+\)/, `rgba($1,$2,$3, ${a})`)
}
