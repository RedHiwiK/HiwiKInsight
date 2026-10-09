// Modules
import { Fragment, useState } from 'react'
import clsx from 'clsx'
import { ChevronRight } from 'lucide-react'
import { num, rows, type Cell } from '../api'
import { useCli, useFilters, useMetric } from '../data'
import { fmtDuration, fmtInt, fmtNum, fmtPct } from '../format'
import { Page } from '../components/Layout'
import { Card, InlineBar, QueryState } from '../components/ui'
import { BubbleChart } from '../components/Chart'
import { emptyHint } from './Overview'
import { getLang, t } from '../prefs'

type Row = Record<string, Cell>

export function ModulesPage() {
  const f = useFilters()
  const cli = useCli()
  const mod = useMetric('module_usage')
  const [open, setOpen] = useState<string | null>(null)
  const modules = rows<Row>(mod.data, 'modules')
  const screens = rows<Row>(mod.data, 'screens')
  const features = rows<Row>(mod.data, 'features')
  const maxCov = Math.max(...modules.map((m) => num(m.coverage_pct)), 1)
  const empty = !mod.isPending && modules.length === 0

  return (
    <Page title={t('模块', 'Modules')} subtitle={t('哪些功能被用得多、停留得久', 'Which features get used most and hold attention longest')}>
      <Card title={t('模块排行', 'Module ranking')} info={t('使用人数：期间打开过该模块任一页面的用户；覆盖率 = 使用人数 ÷ 期间活跃用户；停留来自 screen.left', 'Users: opened any screen of the module in the period; Coverage = users ÷ active users; time spent from screen.left')} cli={cli('module_usage')}>
        <QueryState loading={mod.isPending} error={mod.error} empty={empty} height={320} emptyHint={emptyHint(f.env)}>
          <div className="scroll-thin -mx-4 overflow-x-auto sm:-mx-6">
            <table className="w-full min-w-[720px] text-small">
              <thead>
                <tr className="border-b border-border text-caption font-medium text-text-3">
                  <th className="h-9 pl-4 text-left sm:pl-6">{t('模块', 'Module')}</th>
                  <th className="h-9 px-3 text-right">{t('使用人数', 'Users')}</th>
                  <th className="h-9 w-[28%] px-3 text-left">{t('覆盖率', 'Coverage')}</th>
                  <th className="h-9 px-3 text-right">{t('浏览次数', 'Views')}</th>
                  <th className="h-9 px-3 text-right">{t('人均次数', 'Views / user')}</th>
                  <th className="h-9 pr-4 pl-3 text-right sm:pr-6">{t('平均停留', 'Avg. time')}</th>
                </tr>
              </thead>
              <tbody>
                {modules.map((m) => {
                  const key = String(m.module)
                  const expanded = open === key
                  const children = screens.filter((s) => String(s.module_screen).startsWith(key + '/'))
                  return (
                    <Fragment key={key}>
                      <tr
                        onClick={() => setOpen(expanded ? null : key)}
                        className={clsx('cursor-pointer border-b border-border transition-ui hover:bg-surface-2 active:bg-surface-3', expanded && 'bg-surface-2')}
                      >
                        <td className="h-12 pl-4 sm:pl-6">
                          <div className="flex items-center gap-2">
                            <ChevronRight size={14} className={clsx('text-text-3 transition-transform duration-[var(--spring-dur)] ease-[var(--spring)]', expanded && 'rotate-90')} />
                            <span className="font-medium text-text">{f.moduleName(key)}</span>
                            {getLang() === 'zh' && <span className="text-caption font-normal text-text-3">{key}</span>}
                          </div>
                        </td>
                        <td className="num px-3 text-right text-text">{fmtInt(m.users)}</td>
                        <td className="px-3">
                          <div className="flex items-center gap-3">
                            <InlineBar value={num(m.coverage_pct)} max={maxCov} />
                            <span className="num w-12 shrink-0 text-right text-text">{fmtPct(m.coverage_pct)}</span>
                          </div>
                        </td>
                        <td className="num px-3 text-right text-text-2">{fmtInt(m.views)}</td>
                        <td className="num px-3 text-right text-text-2">{fmtNum(m.views_per_user)}</td>
                        <td className="num pr-4 pl-3 text-right text-text-2 sm:pr-6">{fmtDuration(m.avg_duration_s)}</td>
                      </tr>
                      {expanded && children.map((s) => (
                        <tr key={String(s.module_screen)} className="border-b border-border bg-surface-2/50">
                          <td className="h-10 pl-12 text-text-2 sm:pl-13">{String(s.module_screen).split('/')[1]}</td>
                          <td className="num px-3 text-right text-text-2">{fmtInt(s.users)}</td>
                          <td className="px-3">
                            <div className="flex items-center gap-3">
                              <InlineBar value={num(s.coverage_pct)} max={maxCov} color="var(--c5)" />
                              <span className="num w-12 shrink-0 text-right text-text-2">{fmtPct(s.coverage_pct)}</span>
                            </div>
                          </td>
                          <td className="num px-3 text-right text-text-3">{fmtInt(s.views)}</td>
                          <td className="num px-3 text-right text-text-3">{fmtNum(s.views_per_user)}</td>
                          <td className="num pr-4 pl-3 text-right text-text-3 sm:pr-6">{fmtDuration(s.avg_duration_s)}</td>
                        </tr>
                      ))}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>
          </div>
        </QueryState>
      </Card>

      <Card title={t('功能使用', 'Feature usage')} info={t('带 module 参数的功能事件（如 AI 分析、OCR 识别、生成复盘）：使用人数、次数与成功率，与上方页面浏览次数对照看', 'Feature events tagged with a module (e.g. AI analysis, OCR, review generation): users, uses and success rate, to read alongside screen views above')} cli={cli('module_usage')}>
        <QueryState loading={mod.isPending} error={mod.error} empty={!mod.isPending && features.length === 0} height={200} emptyHint={emptyHint(f.env)}>
          <div className="scroll-thin -mx-4 overflow-x-auto sm:-mx-6">
            <table className="w-full min-w-[720px] text-small">
              <thead>
                <tr className="border-b border-border text-caption font-medium text-text-3">
                  <th className="h-9 pl-4 text-left sm:pl-6">{t('模块', 'Module')}</th>
                  <th className="h-9 px-3 text-left">{t('功能', 'Feature')}</th>
                  <th className="h-9 px-3 text-right">{t('使用人数', 'Users')}</th>
                  <th className="h-9 px-3 text-right">{t('覆盖率', 'Coverage')}</th>
                  <th className="h-9 px-3 text-right">{t('使用次数', 'Uses')}</th>
                  <th className="h-9 px-3 text-right">{t('人均次数', 'Uses / user')}</th>
                  <th className="h-9 pr-4 pl-3 text-right sm:pr-6">{t('成功率', 'Success')}</th>
                </tr>
              </thead>
              <tbody>
                {features.map((r) => (
                  <tr key={`${r.module}/${r.event}/${r.feature}`} className="border-b border-border">
                    <td className="h-11 pl-4 font-medium text-text sm:pl-6">{f.moduleName(String(r.module))}</td>
                    <td className="px-3 text-text-2">
                      <span className="num">{String(r.event)}</span>
                      {r.feature ? <span className="text-text-3"> · {String(r.feature)}</span> : null}
                    </td>
                    <td className="num px-3 text-right text-text">{fmtInt(r.users)}</td>
                    <td className="num px-3 text-right text-text-2">{fmtPct(r.coverage_pct)}</td>
                    <td className="num px-3 text-right text-text">{fmtInt(r.uses)}</td>
                    <td className="num px-3 text-right text-text-2">{fmtNum(r.uses_per_user)}</td>
                    <td className="num pr-4 pl-3 text-right text-text-2 sm:pr-6">{fmtPct(r.success_pct)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </QueryState>
      </Card>

      <Card title={t('覆盖率 × 停留', 'Coverage × time spent')} info={t('右上角：用的人多、停留也久；气泡大小为浏览次数', 'Top right: widely used and long-held; bubble size is views')}>
        <QueryState loading={mod.isPending} error={mod.error} empty={empty} height={340}>
          <BubbleChart
            height={340}
            xName={t('覆盖率', 'Coverage')}
            yName={t('平均停留', 'Avg. time')}
            xFormat={(v) => `${v}%`}
            yFormat={(v) => fmtDuration(v)}
            points={modules.map((m) => ({ name: f.moduleName(String(m.module)), x: num(m.coverage_pct), y: num(m.avg_duration_s), size: num(m.views) }))}
          />
        </QueryState>
      </Card>
    </Page>
  )
}
