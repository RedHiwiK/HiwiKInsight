// Charts: ECharts imported on demand; all colors come from tokens (usePalette), never literal values
import { useEffect, useRef } from 'react'
import * as echarts from 'echarts/core'
import { BarChart, LineChart, PieChart, ScatterChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TitleComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { EChartsCoreOption } from 'echarts/core'
import { alpha, usePalette, type Palette } from '../theme'
import { fmtLongDay, fmtAxisDay } from '../format'
import { t } from '../prefs'

echarts.use([BarChart, LineChart, PieChart, ScatterChart, GridComponent, LegendComponent, TitleComponent, TooltipComponent, CanvasRenderer])

export function EChart({ option, height, className }: { option: EChartsCoreOption; height: number | string; className?: string }) {
  const el = useRef<HTMLDivElement>(null)
  const chart = useRef<echarts.ECharts | null>(null)
  useEffect(() => {
    if (!el.current) return
    const c = echarts.init(el.current, undefined, { renderer: 'canvas' })
    chart.current = c
    const ro = new ResizeObserver(() => c.resize())
    ro.observe(el.current)
    return () => {
      ro.disconnect()
      c.dispose()
      chart.current = null
    }
  }, [])
  useEffect(() => {
    // Shared motion: updates morph smoothly from the current shape; with reduced motion they switch instantly
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const motion: EChartsCoreOption = reduced
      ? { animation: false }
      : { animationEasing: 'cubicOut', animationEasingUpdate: 'cubicOut', animationDurationUpdate: 450 }
    chart.current?.setOption({ ...motion, ...option }, { notMerge: true })
  }, [option])
  return <div ref={el} className={className} style={{ height, width: '100%' }} />
}

// ---------- Shared styles ----------

function tooltipBase(p: Palette) {
  return {
    backgroundColor: p.surface,
    borderColor: p.border,
    borderWidth: 1,
    padding: [8, 12],
    textStyle: { color: p.text, fontSize: 12, fontFamily: p.font },
    extraCssText: `border-radius: 8px; box-shadow: var(--shadow-pop);`,
  }
}

function axisBase(p: Palette) {
  return {
    axisLine: { show: false },
    axisTick: { show: false },
    axisLabel: { color: p['text-3'], fontSize: 12, fontFamily: p.font },
    splitLine: { lineStyle: { color: p.border, type: 'dashed' as const } },
  }
}

const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)

function tooltipRow(color: string, name: string, value: string) {
  return `<div style="display:flex;align-items:center;gap:8px;line-height:20px">
    <span style="width:8px;height:8px;border-radius:50%;background:${color}"></span>
    <span style="color:var(--text-2);flex:1">${esc(name)}</span>
    <span style="font-variant-numeric:tabular-nums;font-weight:600;margin-left:16px">${esc(value)}</span></div>`
}

// ---------- Trend chart ----------

/** Tick interval of 1 / 2 / 5 x 10^n; at least 1 when integer */
function niceStep(raw: number, integer: boolean): number {
  if (!(raw > 0)) return 1
  const exp = 10 ** Math.floor(Math.log10(raw))
  const f = raw / exp
  const step = (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * exp
  return integer ? Math.max(1, Math.round(step)) : step
}

/**
 * Dual-axis ticks: both axes get the same number of positive and negative steps, so the two zeros and the grid lines line up;
 * picks the step count in 2-5 that best fills both axes (ties prefer 4); integer-only axes use a step of at least 1
 */
function alignedAxes(series: Series[]) {
  const axes = ([0, 1] as const).map((i) => {
    const vals = series.filter((s) => (s.yAxisIndex ?? 0) === i).flatMap((s) => s.data.filter((v): v is number => v != null))
    return { min: Math.min(0, ...vals), max: Math.max(0, ...vals), integer: vals.every(Number.isInteger) }
  })
  let best = { fill: -1, splits: 4, steps: [1, 1] }
  for (const splits of [4, 5, 3, 2]) {
    const steps = axes.map((a) => niceStep(a.max / splits || -a.min / splits, a.integer))
    const fill = Math.min(...axes.map((a, i) => (a.max > 0 ? a.max / (splits * steps[i]) : 1)))
    if (fill > best.fill) best = { fill, splits, steps }
  }
  const below = Math.max(...axes.map((a, i) => Math.ceil(-a.min / best.steps[i])))
  return axes.map((a, i) => ({ min: -below * best.steps[i], max: best.splits * best.steps[i], interval: best.steps[i], hasNegative: a.min < 0 }))
}

export interface Series {
  name: string
  data: (number | null)[]
  type?: 'line' | 'bar'
  color?: string
  stack?: string
  area?: boolean
  /** 1 = right axis */
  yAxisIndex?: 0 | 1
  format?: (v: number) => string
}

export function TrendChart({
  x, series, height = 280, format = (v) => String(v), rightFormat, xIsDay = true, legend, showTotal, xFormat, axisFormat,
}: {
  x: string[]
  series: Series[]
  height?: number
  format?: (v: number) => string
  rightFormat?: (v: number) => string
  xIsDay?: boolean
  legend?: boolean
  /** Show a total at the end of the tooltip (for stacked charts) */
  showTotal?: boolean
  /** Custom x-axis / tooltip title format (e.g. months) */
  xFormat?: { axis: (v: string) => string; tooltip: (v: string) => string }
  /** Y-axis tick format (defaults to format; money axes can drop decimals) */
  axisFormat?: (v: number) => string
}) {
  const p = usePalette()
  const showLegend = legend ?? series.length > 1
  const hasRight = series.some((s) => s.yAxisIndex === 1)
  const aligned = hasRight ? alignedAxes(series) : null
  // After alignment a negative half may exist only to match the other axis; hide negative ticks on a side with no negative values
  const axisLabel = (i: 0 | 1, f: (v: number) => string) => (v: number) => (aligned && v < 0 && !aligned[i].hasNegative ? '' : f(v))
  const stackTops = new Map<string, number>()
  series.forEach((s, i) => s.stack && s.type === 'bar' && stackTops.set(s.stack, i))
  const option: EChartsCoreOption = {
    animationDuration: 300,
    grid: { left: 8, right: 8, top: showLegend ? 36 : 12, bottom: 4, containLabel: true },
    legend: showLegend
      ? { top: 0, right: 0, icon: 'circle', itemWidth: 8, itemHeight: 8, itemGap: 16, textStyle: { color: p['text-2'], fontSize: 12, fontFamily: p.font } }
      : undefined,
    tooltip: {
      ...tooltipBase(p),
      trigger: 'axis',
      axisPointer: { type: 'line', lineStyle: { color: p['border-strong'] } },
      formatter: (items: { axisValue: string; seriesName: string; value: number | null; color: string; seriesIndex: number }[]) => {
        const head = xFormat ? xFormat.tooltip(items[0].axisValue) : xIsDay ? fmtLongDay(items[0].axisValue) : items[0].axisValue
        const total = items.reduce((sum, it) => sum + (it.value ?? 0), 0)
        return `<div style="font-weight:600;margin-bottom:4px">${esc(head)}</div>` +
          items.map((it) => {
            const s = series[it.seriesIndex]
            const f = s.format ?? (s.yAxisIndex === 1 && rightFormat ? rightFormat : format)
            return tooltipRow(typeof it.color === 'string' ? it.color : p.primary, it.seriesName, it.value == null ? '—' : f(it.value))
          }).join('') +
          (showTotal ? `<div style="border-top:1px solid var(--border);margin-top:4px;padding-top:4px">${tooltipRow('transparent', t('合计', 'Total'), format(total))}</div>` : '')
      },
    },
    xAxis: {
      type: 'category',
      data: x,
      ...axisBase(p),
      splitLine: { show: false },
      axisLabel: { ...axisBase(p).axisLabel, formatter: xFormat ? xFormat.axis : xIsDay ? fmtAxisDay : undefined, hideOverlap: true },
    },
    yAxis: [
      { type: 'value', ...axisBase(p), splitNumber: 4, ...(aligned && { min: aligned[0].min, max: aligned[0].max, interval: aligned[0].interval }),
        axisLabel: { ...axisBase(p).axisLabel, formatter: axisLabel(0, axisFormat ?? format) } },
      ...(aligned
        ? [{ type: 'value', ...axisBase(p), splitLine: { show: false }, min: aligned[1].min, max: aligned[1].max, interval: aligned[1].interval,
            axisLabel: { ...axisBase(p).axisLabel, formatter: axisLabel(1, rightFormat ?? format) } }]
        : []),
    ],
    series: series.map((s, i) => {
      const color = s.color ? resolveVar(p, s.color) : p.series[i % p.series.length]
      if (s.type === 'bar') {
        const top = !s.stack || stackTops.get(s.stack) === i
        return {
          name: s.name, type: 'bar', data: s.data, stack: s.stack, yAxisIndex: s.yAxisIndex ?? 0,
          barMaxWidth: 24, itemStyle: { color, borderRadius: top ? [4, 4, 0, 0] : 0 },
          emphasis: { focus: 'none' },
        }
      }
      return {
        name: s.name, type: 'line', data: s.data, yAxisIndex: s.yAxisIndex ?? 0, smooth: false,
        showSymbol: false, symbolSize: 6, lineStyle: { width: 2, color }, itemStyle: { color },
        z: 3,
        areaStyle: s.area
          ? { color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [{ offset: 0, color: alpha(color, 0.16) }, { offset: 1, color: alpha(color, 0) }]) }
          : undefined,
      }
    }),
  }
  return <EChart option={option} height={height} />
}

// ---------- Sparkline ----------

export function Sparkline({ data, color }: { data: number[]; color?: string }) {
  const p = usePalette()
  const c = color ? resolveVar(p, color) : p.primary
  const option: EChartsCoreOption = {
    animation: false,
    grid: { left: 0, right: 0, top: 2, bottom: 2 },
    xAxis: { type: 'category', show: false, data: data.map((_, i) => i), boundaryGap: false },
    yAxis: { type: 'value', show: false, min: (v: { min: number }) => v.min },
    series: [{
      type: 'line', data, smooth: 0.3, showSymbol: false, lineStyle: { width: 1.5, color: c },
      areaStyle: { color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [{ offset: 0, color: alpha(c, 0.18) }, { offset: 1, color: alpha(c, 0) }]) },
    }],
  }
  return <EChart option={option} height={32} />
}

/** Replace 'var(--c2)'-style colors with the concrete color from the palette */
export function resolveVar(p: Palette, color: string): string {
  const m = color.match(/^var\(--([a-z0-9-]+)\)$/)
  if (!m) return color
  // Tokens not registered in the palette fall back to the primary color so the canvas never sees var()
  return m[1] in p ? (p as unknown as Record<string, string>)[m[1]] : p.primary
}

// ---------- Donut chart ----------

export function DonutChart({
  items, height = 220, centerLabel, format = (v) => String(v),
}: {
  items: { name: string; value: number; color: string }[]
  height?: number
  centerLabel: string
  format?: (v: number) => string
}) {
  const p = usePalette()
  const total = items.reduce((s, x) => s + x.value, 0)
  const option: EChartsCoreOption = {
    animationDuration: 300,
    tooltip: {
      ...tooltipBase(p),
      trigger: 'item',
      formatter: (it: { name: string; value: number; color: string; percent: number }) =>
        tooltipRow(it.color, it.name, `${format(it.value)} · ${it.percent.toFixed(1)}%`),
    },
    title: {
      text: format(total),
      subtext: centerLabel,
      left: 'center',
      top: 'center',
      itemGap: 2,
      textStyle: { color: p.text, fontSize: 24, fontWeight: 650, fontFamily: p.font },
      subtextStyle: { color: p['text-3'], fontSize: 12, fontFamily: p.font },
    },
    series: [{
      type: 'pie',
      radius: ['64%', '88%'],
      padAngle: 2,
      itemStyle: { borderRadius: 4 },
      label: { show: false },
      emphasis: { scale: true, scaleSize: 4, label: { show: false } },
      data: items.map((it) => ({ name: it.name, value: it.value, itemStyle: { color: resolveVar(p, it.color) } })),
    }],
  }
  return <EChart option={option} height={height} />
}

// ---------- Column chart (categories) ----------

export function ColumnChart({
  categories, values, height = 220, color, format = (v) => String(v), name = t('人数', 'Users'), tooltipLabel,
}: {
  categories: string[]
  values: number[]
  height?: number
  color?: string
  format?: (v: number) => string
  name?: string
  tooltipLabel?: (c: string) => string
}) {
  const p = usePalette()
  const c = color ? resolveVar(p, color) : p.primary
  const option: EChartsCoreOption = {
    animationDuration: 300,
    grid: { left: 8, right: 8, top: 12, bottom: 4, containLabel: true },
    tooltip: {
      ...tooltipBase(p),
      trigger: 'axis',
      axisPointer: { type: 'shadow', shadowStyle: { color: alpha(p['surface-3'], 0.5) } },
      formatter: (items: { axisValue: string; value: number }[]) =>
        `<div style="font-weight:600;margin-bottom:4px">${esc(tooltipLabel ? tooltipLabel(items[0].axisValue) : items[0].axisValue)}</div>` +
        tooltipRow(c, name, format(items[0].value)),
    },
    xAxis: { type: 'category', data: categories, ...axisBase(p), splitLine: { show: false } },
    yAxis: { type: 'value', ...axisBase(p), splitNumber: 4, axisLabel: { ...axisBase(p).axisLabel, formatter: format } },
    series: [{ type: 'bar', data: values, barMaxWidth: 32, itemStyle: { color: c, borderRadius: [4, 4, 0, 0] } }],
  }
  return <EChart option={option} height={height} />
}

// ---------- Scatter chart (modules: reach x dwell time) ----------

export function BubbleChart({
  points, height = 320, xFormat, yFormat, xName, yName,
}: {
  points: { name: string; x: number; y: number; size: number }[]
  height?: number
  xFormat: (v: number) => string
  yFormat: (v: number) => string
  xName: string
  yName: string
}) {
  const p = usePalette()
  const maxSize = Math.max(...points.map((pt) => pt.size), 1)
  const option: EChartsCoreOption = {
    animationDuration: 300,
    grid: { left: 8, right: 24, top: 28, bottom: 28, containLabel: true },
    tooltip: {
      ...tooltipBase(p),
      trigger: 'item',
      formatter: (it: { data: { name: string; value: number[] }; color: string }) =>
        `<div style="font-weight:600;margin-bottom:4px">${esc(it.data.name)}</div>` +
        tooltipRow(it.color, xName, xFormat(it.data.value[0])) + tooltipRow(it.color, yName, yFormat(it.data.value[1])) +
        tooltipRow(it.color, t('浏览次数', 'Views'), String(it.data.value[2])),
    },
    xAxis: {
      type: 'value', name: xName, nameLocation: 'middle', nameGap: 28, nameTextStyle: { color: p['text-3'], fontSize: 12, fontFamily: p.font },
      ...axisBase(p), axisLabel: { ...axisBase(p).axisLabel, formatter: xFormat },
    },
    yAxis: {
      type: 'value', name: yName, nameTextStyle: { color: p['text-3'], fontSize: 12, fontFamily: p.font, align: 'left' },
      ...axisBase(p), splitNumber: 4, axisLabel: { ...axisBase(p).axisLabel, formatter: yFormat },
    },
    series: [{
      type: 'scatter',
      data: points.map((pt, i) => ({
        name: pt.name,
        value: [pt.x, pt.y, pt.size],
        itemStyle: { color: alpha(p.series[i % p.series.length], 0.75), borderColor: p.series[i % p.series.length], borderWidth: 1 },
      })),
      symbolSize: (v: number[]) => 10 + Math.sqrt(v[2] / maxSize) * 34,
      label: { show: true, formatter: '{b}', position: 'right', color: p['text-2'], fontSize: 12, fontFamily: p.font },
      labelLayout: { hideOverlap: true },
    }],
  }
  return <EChart option={option} height={height} />
}
