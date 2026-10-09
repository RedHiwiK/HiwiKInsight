// App shell: sidebar, top bar, global filters, mobile drawer
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router'
import clsx from 'clsx'
import {
  Activity, Boxes, Calendar, ChartPie, LayoutDashboard, Menu, Radar, Repeat,
  Settings, Smartphone, Store, TriangleAlert, Users, Wallet, X,
} from 'lucide-react'
import { envOptions, rangeOptions, useFilters, useMeta, type Env } from '../data'
import { t } from '../prefs'
import { Badge, Segmented, Select } from './ui'

export const portfolioItem = () => ({ to: '/', label: t('经营总览', 'Portfolio'), icon: ChartPie })

export const settingsItem = () => ({ to: '/settings', label: t('设置', 'Settings'), icon: Settings })

export const navItems = () => [
  { to: '/overview', label: t('概览', 'Overview'), icon: LayoutDashboard },
  { to: '/activity', label: t('活跃与粘性', 'Engagement'), icon: Activity },
  { to: '/retention', label: t('留存', 'Retention'), icon: Repeat },
  { to: '/modules', label: t('模块', 'Modules'), icon: Boxes },
  { to: '/users', label: t('用户', 'Users'), icon: Users },
  { to: '/acquisition', label: t('获客', 'Acquisition'), icon: Store },
  { to: '/revenue', label: t('付费', 'Revenue'), icon: Wallet },
  { to: '/devices', label: t('设备与地区', 'Devices & Regions'), icon: Smartphone },
  { to: '/events', label: t('事件', 'Events'), icon: Radar },
  { to: '/errors', label: t('错误', 'Errors'), icon: TriangleAlert },
]

export function Logo({ compact }: { compact?: boolean }) {
  return (
    <div className="flex items-center gap-2.5">
      <img src={`${import.meta.env.BASE_URL}favicon.svg`} alt="" className="size-7 shrink-0 rounded-[8px]" />
      {!compact && <span className="text-[15px] leading-5 font-[650] tracking-tight text-text">HiwiKInsight</span>}
    </div>
  )
}

type NavEntry = ReturnType<typeof portfolioItem>

/** Sidebar nav item: page links and the settings entry at the bottom share one style */
function NavItem({ item: { to, label, icon: Icon }, query, compact, onNavigate }: { item: NavEntry; query: string; compact?: boolean; onNavigate?: () => void }) {
  return (
    <NavLink
      to={to + query}
      end={to === '/'}
      onClick={onNavigate}
      title={compact ? label : undefined}
      className={({ isActive }) =>
        clsx(
          'press flex h-9 items-center gap-2.5 rounded-sm text-body',
          compact ? 'justify-center px-0' : 'px-2.5',
          isActive ? 'bg-primary-soft font-semibold text-primary' : 'text-text hover:bg-fill',
        )
      }
    >
      <Icon size={18} strokeWidth={1.75} className="shrink-0" />
      {!compact && <span className="truncate">{label}</span>}
    </NavLink>
  )
}

function NavList({ onNavigate, compact }: { onNavigate?: () => void; compact?: boolean }) {
  const { keep } = useFilters()
  return (
    <nav className="flex flex-col gap-0.5">
      <NavItem item={portfolioItem()} query="" compact={compact} onNavigate={onNavigate} />
      <div className={clsx('my-2 border-t border-border', compact && 'mx-1')} />
      {!compact && <div className="px-2.5 pb-1 text-caption font-semibold text-text-3">{t('单个 App', 'Per App')}</div>}
      {navItems().map((item) => <NavItem key={item.to} item={item} query={keep} compact={compact} onNavigate={onNavigate} />)}
    </nav>
  )
}

/** Sidebar footer: settings entry (appearance, language and currency live on the settings page) */
function SidebarFooter({ compact, onNavigate }: { compact?: boolean; onNavigate?: () => void }) {
  const { keep } = useFilters()
  return (
    <div className="border-t border-border pt-3">
      <NavItem item={settingsItem()} query={keep} compact={compact} onNavigate={onNavigate} />
    </div>
  )
}

/** Whether the page has scrolled down: drives the scroll edge of the top bar */
function useScrolled(threshold = 4) {
  const [scrolled, setScrolled] = useState(false)
  useEffect(() => {
    const on = () => setScrolled(window.scrollY > threshold)
    on()
    window.addEventListener('scroll', on, { passive: true })
    return () => window.removeEventListener('scroll', on)
  }, [threshold])
  return scrolled
}

/** Animate both enter and exit: on close switch to closed, play the exit, then unmount */
function usePresence(open: boolean, exitMs: number) {
  const [mounted, setMounted] = useState(open)
  useEffect(() => {
    if (open) {
      setMounted(true)
      return
    }
    const t = setTimeout(() => setMounted(false), exitMs)
    return () => clearTimeout(t)
  }, [open, exitMs])
  return { mounted: mounted || open, state: open ? 'open' : 'closed' }
}

export function AppShell({ children }: { children: ReactNode }) {
  const [drawer, setDrawer] = useState(false)
  const loc = useLocation()
  const scrolled = useScrolled()
  const sheet = usePresence(drawer, 240)
  useEffect(() => setDrawer(false), [loc.pathname])
  useEffect(() => {
    if (!drawer) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setDrawer(false)
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [drawer])
  return (
    <div className="min-h-screen bg-bg">
      {/* Desktop sidebar */}
      <aside className="material-bar fixed inset-y-2 left-2 z-30 hidden w-[232px] flex-col rounded-lg px-3 py-4 shadow-card xl:flex">
        <div className="mb-6 px-2.5 pt-1"><Logo /></div>
        <div className="flex-1 overflow-y-auto"><NavList /></div>
        <SidebarFooter />
      </aside>
      {/* Tablet icon rail */}
      <aside className="material-bar fixed inset-y-2 left-2 z-30 hidden w-[72px] flex-col items-stretch rounded-lg px-3 py-4 shadow-card md:flex xl:hidden">
        <div className="mb-6 flex justify-center pt-1"><Logo compact /></div>
        <div className="flex-1"><NavList compact /></div>
        <SidebarFooter compact />
      </aside>
      {/* Mobile top bar: translucent material, content scrolls underneath */}
      <div data-scrolled={scrolled} className="material-chrome scroll-edge sticky top-0 z-30 flex h-14 items-center justify-between px-4 md:hidden">
        <Logo />
        <button
          type="button"
          aria-label={t('打开菜单', 'Open menu')}
          aria-expanded={drawer}
          onClick={() => setDrawer(true)}
          className="press flex size-9 items-center justify-center rounded-sm text-text-2 hover:bg-fill"
        >
          <Menu size={20} strokeWidth={1.75} />
        </button>
      </div>
      {sheet.mounted && (
        <div className="fixed inset-0 z-50 md:hidden">
          <div data-state={sheet.state} onClick={() => setDrawer(false)} className="fade-scrim absolute inset-0 bg-[var(--scrim)]" />
          <div
            role="dialog"
            aria-modal="true"
            aria-label={t('导航菜单', 'Navigation')}
            data-state={sheet.state}
            className="sheet-top material-pop absolute inset-x-2 top-2 flex max-h-[calc(100dvh-16px)] origin-top-right flex-col rounded-lg px-3 pt-2 pb-3"
          >
            <div className="mb-2 flex h-11 items-center justify-between pl-2">
              <Logo />
              <button type="button" aria-label={t('关闭菜单', 'Close menu')} onClick={() => setDrawer(false)} className="press flex size-9 items-center justify-center rounded-sm text-text-2 hover:bg-fill">
                <X size={20} strokeWidth={1.75} />
              </button>
            </div>
            <div className="scroll-thin flex-1 overflow-y-auto"><NavList onNavigate={() => setDrawer(false)} /></div>
            <div className="mt-3"><SidebarFooter onNavigate={() => setDrawer(false)} /></div>
          </div>
        </div>
      )}
      <main className="md:pl-[88px] xl:pl-[248px]">{children}</main>
    </div>
  )
}

// ---------- Page frame: title + global filters ----------

export function Page({ title, subtitle, children, hideFilters, hideEnv, actions }: {
  title: string
  subtitle?: ReactNode
  children: ReactNode
  hideFilters?: boolean
  /** Hide the environment switch when the data has no environments (e.g. App Store Connect reports are production only) */
  hideEnv?: boolean
  actions?: ReactNode
}) {
  const f = useFilters()
  const scrolled = useScrolled()
  // Once the large title (about 56px tall) scrolls away, the top bar shows a small title
  const collapsed = useScrolled(56)
  useEffect(() => {
    document.title = `${title} · HiwiKInsight`
  }, [title])
  const testBadge = f.env !== 'production' && !hideFilters && !hideEnv && !actions && <Badge tone="warning">{t('测试数据', 'Test data')}</Badge>
  return (
    <div className="flex flex-col">
      {/* Top bar: sticky material bar on desktop, small title fades in on the left after scrolling, filters on the right; on mobile it sits below the large title and does not stick */}
      <header data-scrolled={scrolled} className="order-2 z-20 md:order-none md:material-chrome md:scroll-edge md:sticky md:top-0">
        <div className="mx-auto flex max-w-[1360px] items-center justify-between gap-3 px-4 pb-4 sm:px-6 md:min-h-14 md:py-3 lg:px-8">
          <div
            aria-hidden={!collapsed}
            className={clsx('hidden min-w-0 items-center gap-2 transition-[opacity,transform] duration-[var(--spring-dur)] ease-[var(--spring)] md:flex', collapsed ? 'opacity-100' : 'translate-y-1 opacity-0')}
          >
            <span className="truncate text-[17px] leading-[22px] font-semibold tracking-[var(--track-heading)] text-text">{title}</span>
          </div>
          {actions ?? (!hideFilters && <FilterBar hideEnv={hideEnv} />)}
        </div>
      </header>
      <div className="order-1 mx-auto w-full max-w-[1360px] px-4 pt-5 pb-4 sm:px-6 md:order-none md:pt-1 md:pb-5 lg:px-8">
        <div className="flex items-center gap-2">
          <h1 className="truncate text-[30px] leading-9 font-bold tracking-[var(--track-large)] text-text sm:text-large">{title}</h1>
          {testBadge}
        </div>
        {subtitle && <p className="mt-1 text-body text-text-2">{subtitle}</p>}
      </div>
      <div className="order-3 mx-auto flex w-full max-w-[1360px] flex-col gap-4 px-4 pb-12 sm:gap-5 sm:px-6 md:order-none lg:px-8">{children}</div>
    </div>
  )
}

function FilterBar({ hideEnv }: { hideEnv?: boolean }) {
  const f = useFilters()
  const meta = useMeta()
  const [custom, setCustom] = useState(false)
  return (
    <div className="scroll-thin -mx-4 flex items-center gap-2 overflow-x-auto px-4 pb-0.5 sm:mx-0 sm:flex-wrap sm:overflow-visible sm:px-0">
      <Select
        ariaLabel={t('选择 App', 'Select app')}
        className="w-32 shrink-0"
        value={f.app}
        options={meta.apps.map((a) => ({ value: a.key, label: a.name }))}
        onChange={(v) => f.set({ app: v })}
      />
      {!hideEnv && <Segmented<Env> value={f.env} options={envOptions()} onChange={(v) => f.set({ env: v === 'production' ? null : v })} ariaLabel={t('环境', 'Environment')} />}
      <div className="relative flex shrink-0 items-center gap-1">
        <Segmented
          value={f.range}
          options={[...rangeOptions(), ...(f.range === 'custom' ? [{ value: 'custom', label: `${f.from.slice(5)} ~ ${f.to.slice(5)}` }] : [])]}
          onChange={(v) => v !== 'custom' && f.set({ range: v === '28' ? null : v, from: null, to: null })}
          ariaLabel={t('时间范围', 'Date range')}
        />
        <button
          type="button"
          aria-label={t('自定义日期', 'Custom dates')}
          onClick={() => setCustom((v) => !v)}
          className={clsx('press flex size-8 items-center justify-center rounded-full hover:bg-fill', custom || f.range === 'custom' ? 'text-primary' : 'text-text-2')}
        >
          <Calendar size={16} strokeWidth={1.75} />
        </button>
        {custom && <DateRangePopover from={f.from} to={f.to} max={f.today} onApply={(from, to) => f.set({ from, to, range: null })} onClose={() => setCustom(false)} />}
      </div>
    </div>
  )
}

/** Custom date popover: placed inside a relative container, closes on clicks outside it */
export function DateRangePopover({ from: from0, to: to0, max, onApply, onClose }: {
  from: string
  to: string
  max: string
  onApply: (from: string, to: string) => void
  onClose: () => void
}) {
  const [from, setFrom] = useState(from0)
  const [to, setTo] = useState(to0)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const onDoc = (e: MouseEvent) => !ref.current?.parentElement?.contains(e.target as Node) && onClose()
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [onClose])
  const valid = from && to && from <= to && to <= max
  const input = 'h-9 w-full rounded-sm border border-transparent bg-fill px-2.5 text-small text-text outline-none focus:border-primary'
  return (
    <div ref={ref} className="material-pop pop-in fixed inset-x-4 top-32 z-40 origin-top rounded-lg p-4 sm:absolute sm:inset-x-auto sm:top-full sm:right-0 sm:mt-2 sm:w-72 sm:origin-top-right">
      <div className="mb-3 text-heading text-text">{t('自定义日期', 'Custom dates')}</div>
      <div className="grid grid-cols-2 gap-2">
        <label className="text-caption text-text-3">{t('开始', 'Start')}<input type="date" value={from} max={max} onChange={(e) => setFrom(e.target.value)} className={clsx(input, 'mt-1')} /></label>
        <label className="text-caption text-text-3">{t('结束', 'End')}<input type="date" value={to} max={max} onChange={(e) => setTo(e.target.value)} className={clsx(input, 'mt-1')} /></label>
      </div>
      <div className="mt-4 flex justify-end gap-2">
        <button type="button" onClick={onClose} className="press h-8 rounded-full px-4 text-small font-medium text-primary hover:bg-fill">{t('取消', 'Cancel')}</button>
        <button
          type="button"
          disabled={!valid}
          onClick={() => { onApply(from, to); onClose() }}
          className="press h-8 rounded-full bg-primary px-4 text-small font-semibold text-on-primary hover:brightness-110 disabled:opacity-40"
        >
          {t('应用', 'Apply')}
        </button>
      </div>
    </div>
  )
}
