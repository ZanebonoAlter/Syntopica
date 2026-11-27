/**
 * ObservationMatrix 测试（矩阵阶段 G3，test-cases-matrix-stage.md MX 组 + 交互/状态）。
 *
 *  - MX-1 桶峰值 / MX-2 回退与未知 / MX-3 零线索可开 / MX-4 断档禁用 / MX-5 无行尾列
 *  - MX-6 长窗容器内横滚结构 / MX-8 今天列 vs 数据截止
 *  - 行序与休眠由 props 携带：观察轴缩放（恰 7/30/365）不重排（纯 props 投影）
 *  - 点格只 emit selectDate（不弹日期浮层/不强滚）；无第二组档位控件
 *
 * 8C G2 报日列头见 `ObservationMatrix.dayHeaders.test.ts`、组顺序/其他行见
 * `ObservationMatrix.groups.test.ts`（8C.4S 结构规范包按内聚子套件拆分）；fixture 与
 * mount 装配在 `observationMatrixTestHarness.ts`（断言与用例名不变）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { buildObservationRows } from './observationLanes'
import { buildDayAxis } from '~/features/tags/composables/laneMatrixModel'
import {
  AXIS14,
  TIER_START,
  TODAY,
  fixtureRows,
  fixtureSource,
  mountMatrix,
  tierResolver,
  windowDay,
} from './observationMatrixTestHarness'

const notifyWarn = vi.fn()
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ warn: notifyWarn, success: vi.fn(), error: vi.fn() }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

describe('ObservationMatrix — 矩阵派生（MX 组，线索口径）', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('MX-1：格值 = 当日线索合计（同日多桶合并，非桶峰值）；aria 给「N 条线索 · M 个主题桶」', () => {
    const wrapper = mountMatrix()
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-23"]')
    expect(cell.text()).toBe('11')
    expect(cell.attributes('aria-label')).toContain('11 条线索')
    expect(cell.attributes('aria-label')).toContain('3 个主题桶')
    // 事件线量纲与点名整体退役（D4/D5）：不出现事件线/最热/未归组文案
    expect(cell.attributes('aria-label')).not.toContain('事件线')
    expect(cell.attributes('aria-label')).not.toContain('最热')
    expect(cell.attributes('aria-label')).not.toContain('未归组')
    expect(cell.attributes('disabled')).toBeUndefined()
  })

  it('MX-1c：无事件线身份时也不渲染任何点名段（该量纲已整体退役）', () => {
    const wrapper = mountMatrix()
    const cell = wrapper.find('[data-testid="matrix-cell-11-2026-10-22"]')
    expect(cell.attributes('aria-label')).not.toContain('最热')
    expect(cell.attributes('aria-label')).not.toContain('事件线')
    expect(cell.attributes('aria-label')).not.toContain('个来源')
    expect(cell.attributes('aria-label')).toContain('4 条线索')
  })

  it('MX-1b：色深按当日线索行内归一（无事件线 is-p* 分档），数值仍全在 title/aria', () => {
    const wrapper = mountMatrix()
    for (const testid of ['matrix-cell-10-2026-10-23', 'matrix-cell-11-2026-10-22']) {
      const cell = wrapper.find(`[data-testid="${testid}"]`)
      // 两行各自的峰值都是 66%（行内归一只，冷泳道不被热门泳道压白）
      expect(cell.attributes('style')).toContain('--omx-clue-mix: 66%')
      expect(cell.classes()).toContain('is-clue-depth')
      expect(cell.classes()).not.toContain('is-p1')
      expect(cell.classes()).not.toContain('is-p2')
      expect(cell.classes()).not.toContain('is-p3')
      expect(cell.attributes('aria-label')).toMatch(/条线索/)
    }
    // 第二深的 3/4 → 50%（线性插值）；0 线索日不着色（无变量、无档位类）
    expect(wrapper.find('[data-testid="matrix-cell-11-2026-10-21"]').attributes('style')).toContain('--omx-clue-mix: 50%')
    const zero = wrapper.find('[data-testid="matrix-cell-10-2026-10-21"]')
    expect(zero.text()).toBe('0')
    expect(zero.attributes('style')).toBeUndefined()
    expect(zero.classes()).not.toContain('is-clue-depth')
  })

  it('MX-2：clue_count 缺省回退各桶线索合计（11 的 10-21 日 → 1+2=3，非桶数 2）', () => {
    const wrapper = mountMatrix()
    const cell = wrapper.find('[data-testid="matrix-cell-11-2026-10-21"]')
    expect(cell.text()).toBe('3')
    expect(cell.attributes('aria-label')).toContain('3 条线索')
    expect(cell.attributes('disabled')).toBeUndefined()
  })

  it('MX-2b：线索全未知日（旧数据）→ 「?」不造 0，aria 注明线索数未知，有分区仍可开', () => {
    const source = fixtureSource()
    source.lanes[0]!.timeline.push(windowDay('2026-10-22', [
      { threadCount: null },
      { threadCount: null },
    ]))
    const wrapper = mountMatrix({
      resolveDay: tierResolver(source),
      activeRows: buildObservationRows(source, AXIS14).filter(row => row.topicId === '10'),
      dormantRows: [],
    })
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]')
    expect(cell.text()).toBe('?')
    expect(cell.attributes('aria-label')).toContain('线索数未知')
    expect(cell.attributes('disabled')).toBeUndefined()
  })

  it('tooltip 与格值同口径：综合稿只以差额出现，thread 总数不得当线索数', () => {
    const source = fixtureSource()
    source.lanes[0]!.timeline.push(windowDay('2026-10-22', [
      { threadCount: 5, compositeCount: 2 },
    ], { clueCount: 3 }))
    const wrapper = mountMatrix({
      resolveDay: tierResolver(source),
      activeRows: buildObservationRows(source, AXIS14).filter(row => row.topicId === '10'),
    })
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]')
    expect(cell.text()).toBe('3')
    const title = cell.attributes('title') ?? ''
    expect(title).toContain('3 条线索')
    expect(title).toContain('1 个主题桶')
    expect(title).toContain('另有 2 条综合稿未计入')
    expect(title).not.toContain('5 条线索')
    expect(title).not.toContain('最多一天桶')
  })

  it('MX-3：有分区 0 线索 = 已知 0，可点击选日', async () => {
    const wrapper = mountMatrix()
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-21"]')
    expect(cell.text()).toBe('0')
    expect(cell.attributes('disabled')).toBeUndefined()
    await cell.trigger('click')
    expect(wrapper.emitted('selectDate')![0]).toEqual([{ topicId: '10', date: '2026-10-21' }])
  })

  it('MX-4：无分区 = 「—」 禁用、aria 含无分区；点断档格不 emit、不弹任何浮层', async () => {
    const wrapper = mountMatrix()
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-22"]')
    expect(cell.text()).toBe('—')
    expect(cell.attributes('disabled')).toBeDefined()
    expect(cell.attributes('aria-label')).toContain('无分区')
    await cell.trigger('click')
    expect(wrapper.emitted('selectDate')).toBeUndefined()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('MX-5：无行尾合计列/读数（DOM 反向断言）', () => {
    const wrapper = mountMatrix()
    expect(wrapper.findAll('[data-testid^="matrix-total-"]')).toHaveLength(0)
    expect(wrapper.find('.omx-sumhead').exists()).toBe(false)
  })

  it('MX-6：≤31 单屏网格；>31 才启用矩阵容器内横滚分支', () => {
    const axis30 = buildDayAxis(TODAY, 30)
    const grid30 = mountMatrix({ axis: axis30 }).find('[data-testid="matrix-grid"]')
    expect(grid30.classes()).not.toContain('is-scrollable')
    expect(grid30.findAll('.omx-dayhead')).toHaveLength(30)
    expect(grid30.find('.omx-day-month').exists()).toBe(true)
    expect(grid30.find('.omx-dayhead').attributes('title')).toMatch(/^\d{4}-\d{2}-\d{2}$/)

    const axis31 = buildDayAxis(TODAY, 31)
    expect(mountMatrix({ axis: axis31 }).find('[data-testid="matrix-grid"]').classes()).not.toContain('is-scrollable')

    const axis32 = buildDayAxis(TODAY, 32)
    expect(mountMatrix({ axis: axis32 }).find('[data-testid="matrix-grid"]').classes()).toContain('is-scrollable')
  })

  it('MX-6：长窗（365 列）容器内横滚结构：滚动容器在场、365 个日期头、列数样式变量', () => {
    const axis365 = buildDayAxis(TODAY, 365)
    const wrapper = mountMatrix({ axis: axis365 })
    const scroll = wrapper.find('[data-testid="matrix-scroll"]')
    expect(scroll.exists()).toBe(true)
    expect(scroll.classes()).toContain('omx-scroll')
    expect(wrapper.findAll('.omx-dayhead')).toHaveLength(365)
    expect(wrapper.find('[data-testid="matrix-grid"]').attributes('style')).toContain('--matrix-cols: 365')
  })

  it('MX-8：今天列高亮仅一处；数据截止提示与今天分清', () => {
    const wrapper = mountMatrix()
    expect(wrapper.findAll('.omx-dayhead.is-today')).toHaveLength(1)
    expect(wrapper.find('[data-testid="matrix-cell-11-2026-10-22"]').classes()).not.toContain('is-today')
    expect(wrapper.find('[data-testid="matrix-data-asof"]').exists()).toBe(false)

    const stale = mountMatrix({ anchorDate: '2026-10-20', dataAsOf: '2026-10-20' })
    expect(stale.find('[data-testid="matrix-data-asof"]').text()).toContain('日报截至 2026-10-20')
  })
})

describe('ObservationMatrix — 名单稳定与三态', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('观察轴缩成恰 7/30/365 日：行序/休眠分组逐项不变（纯 props 投影）', () => {
    const { active, dormant } = fixtureRows()
    const signature = (wrapper: ReturnType<typeof mountMatrix>) => ({
      order: wrapper.findAll('[data-testid^="matrix-lane-"]').map(node => node.attributes('data-lane-id')),
      dormant: wrapper.find('[data-testid="dormant-toggle"]').text(),
    })
    const base = signature(mountMatrix({ activeRows: active, dormantRows: dormant }))
    for (const width of [7, 30, 365]) {
      const wrapper = mountMatrix({
        axis: buildDayAxis(TODAY, width),
        activeRows: active,
        dormantRows: dormant,
      })
      expect(signature(wrapper).order).toEqual(base.order)
      expect(signature(wrapper).dormant).toBe(base.dormant)
    }
  })

  it('B 池独有 topic 不渲染行（即使 resolver 能解析其内容）', () => {
    const { active, dormant } = fixtureRows()
    const resolver = (topicId: string, date: string) => {
      if (topicId === '99') return { kind: 'content' as const, source: 'pool' as const, day: windowDay(date, [{ threadCount: 9 }]) }
      return tierResolver()(topicId, date)
    }
    const wrapper = mountMatrix({ resolveDay: resolver, activeRows: active, dormantRows: dormant })
    expect(wrapper.find('[data-testid="matrix-lane-99"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid^="matrix-cell-99-"]')).toHaveLength(0)
  })

  it('历史未取到（池外/池失败）= 「?」 禁用，不补 0、不 emit', async () => {
    const axis30 = buildDayAxis(TODAY, 30) // 跨出 14 日档窗 → 10-09 前为池外历史
    const resolver = (topicId: string, date: string) => {
      if (date < TIER_START) return { kind: 'unavailable' as const }
      return tierResolver()(topicId, date)
    }
    const wrapper = mountMatrix({ axis: axis30, resolveDay: resolver })
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-01"]')
    expect(cell.text()).toBe('?')
    expect(cell.attributes('disabled')).toBeDefined()
    expect(cell.attributes('aria-label')).toContain('未取到')
    await cell.trigger('click')
    expect(wrapper.emitted('selectDate')).toBeUndefined()
  })

  it('历史提示条件出现：常态不写档位口径，仅跨出档窗标「历史仅补数据，泳道名单不变」', () => {
    const tier = mountMatrix()
    expect(tier.find('[data-testid="matrix-tier-note"]').exists()).toBe(false)
    expect(tier.text()).not.toContain('日档')

    const pool = mountMatrix({ axis: buildDayAxis(TODAY, 30) })
    const note = pool.find('[data-testid="matrix-tier-note"]')
    expect(note.text()).toBe('历史仅补数据，泳道名单不变')
  })

  it('图例单条三态：当日线索数（不含综合稿）· — 无分区 · ? 数据不可用', () => {
    const wrapper = mountMatrix()
    const legend = wrapper.find('[data-testid="matrix-legend"]')
    expect(legend.exists()).toBe(true)
    expect(legend.text()).toBe('当日线索数（不含综合稿） · — 无分区 · ? 数据不可用')
    expect(legend.text()).not.toContain('事件线')
    expect(legend.text()).not.toContain('兜底')
    const header = wrapper.find('[data-testid="matrix-lane-10"]')
    // 名称看全契约（2026-11-26）：不再挂原生 title（延迟 1s、键盘不出、长名照样截），
    // 全名与指标只留在 aria-label；悬停/聚焦卡是视觉冗余。
    expect(header.attributes('title')).toBeUndefined()
    expect(header.attributes('aria-label')).toContain('11 条线索')
    expect(header.attributes('aria-label')).toContain('4 个主题桶')
    expect(header.attributes('aria-label')).not.toContain('事件线')
  })

  it('重复口径说明段退役：矩阵区只剩一条图例，不再另写第二条说明行', () => {
    const wrapper = mountMatrix()
    expect(wrapper.find('[data-testid="matrix-basis-note"]').exists()).toBe(false)
    // 头部只剩 kicker 与图例两个 span（口径说明段已删）
    expect(wrapper.findAll('.omx-head > span')).toHaveLength(2)
    expect(wrapper.find('[data-testid="matrix-legend"]').text()).not.toContain('数字 =')
  })

  it('事件身份未回填 footnote 整体退役（D4）：无论旧 props 取值都不再渲染', () => {
    expect(mountMatrix({ storiesBackfilled: false }).find('[data-testid="matrix-backfill-note"]').exists()).toBe(false)
    expect(mountMatrix().find('[data-testid="matrix-backfill-note"]').exists()).toBe(false)
    expect(mountMatrix({ storiesBackfilled: true }).find('[data-testid="matrix-backfill-note"]').exists()).toBe(false)
    expect(mountMatrix({ storiesBackfilled: null }).find('[data-testid="matrix-backfill-note"]').exists()).toBe(false)
  })

  it('行序由 lane_heat 决定（11 热 7.2 在 10 热 4.5 前），不因分桶/线索更多而反转', () => {
    const wrapper = mountMatrix()
    // fixture：10 分桶/线索更多（3+7+1），但 11 的 lane_heat 更高 → 11 在前
    expect(wrapper.findAll('[data-testid^="matrix-lane-"]').map(node => node.attributes('data-lane-id')))
      .toEqual(['11', '10'])
    const rows = fixtureRows()
    expect(rows.active.map(row => [row.topicId, row.laneHeat])).toEqual([['11', 7.2], ['10', 4.5]])
  })
})

describe('ObservationMatrix — 交互与状态', () => {
  beforeEach(() => notifyWarn.mockReset())

  it('点泳道名 emit selectLane（字符串 ID）；日格为原生可聚焦 button、携带 DOM 焦点锚，点格 emit selectDate', async () => {
    const wrapper = mountMatrix()
    await wrapper.find('[data-testid="matrix-lane-11"]').trigger('click')
    expect(wrapper.emitted('selectLane')![0]).toEqual(['11'])

    // 键盘可达性：原生 button（Enter/Space 浏览器原生触发 click）+ data-lane-id/data-date 焦点锚
    const cell = wrapper.find('[data-testid="matrix-cell-10-2026-10-23"]')
    expect(cell.element.tagName).toBe('BUTTON')
    expect(cell.attributes('disabled')).toBeUndefined()
    expect(cell.attributes('data-lane-id')).toBe('10')
    expect(cell.attributes('data-date')).toBe('2026-10-23')
    await cell.trigger('click')
    expect(wrapper.emitted('selectDate')![0]).toEqual([{ topicId: '10', date: '2026-10-23' }])
  })

  it('点格不弹日期浮层、无 select/无第二组档位控件；「查看所选日 ↓」显式交父', async () => {
    const wrapper = mountMatrix()
    await wrapper.find('[data-testid="matrix-cell-10-2026-10-23"]').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('近 1 年')
    expect(wrapper.find('[data-testid="matrix-goto-selected"]').exists()).toBe(false)

    const withDate = mountMatrix({ selectedDate: '2026-10-23' })
    const goto = withDate.find('[data-testid="matrix-goto-selected"]')
    expect(goto.exists()).toBe(true)
    await goto.trigger('click')
    expect(withDate.emitted('gotoSelected')).toHaveLength(1)
  })

  it('选中日格高亮（选中泳道 × 选中日交点）', () => {
    const wrapper = mountMatrix({ selectedDate: '2026-10-23' })
    expect(wrapper.find('[data-testid="matrix-cell-10-2026-10-23"]').classes()).toContain('is-selected')
    expect(wrapper.find('[data-testid="matrix-cell-11-2026-10-23"]').classes()).not.toContain('is-selected')
  })

  it('休眠：默认折叠，点折叠条 emit toggleDormant；展开后弱化行可选、选中休眠可展开交父', async () => {
    const wrapper = mountMatrix()
    expect(wrapper.find('[data-testid="matrix-lane-12"]').exists()).toBe(false)
    await wrapper.find('[data-testid="dormant-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)

    const expanded = mountMatrix({ expandedDormant: true })
    const dormantLane = expanded.find('[data-testid="matrix-lane-12"]')
    expect(dormantLane.exists()).toBe(true)
    expect(dormantLane.classes()).toContain('is-dormant')
    await dormantLane.trigger('click')
    expect(expanded.emitted('selectLane')![0]).toEqual(['12'])
  })

  it('状态分层：loading 骨架 / 首次错误+重试 / 无日报引导生成 / 有日报无泳道', async () => {
    const loading = mountMatrix({ loading: true, activeRows: [], dormantRows: [] })
    expect(loading.find('[data-testid="matrix-skeleton"]').exists()).toBe(true)
    expect(loading.find('[data-testid="matrix-grid"]').exists()).toBe(false)

    const error = mountMatrix({ error: '网络错误', activeRows: [], dormantRows: [] })
    expect(error.find('[data-testid="matrix-error"]').text()).toContain('网络错误')
    await error.find('[data-testid="matrix-retry"]').trigger('click')
    expect(error.emitted('retry')).toHaveLength(1)

    const noReports = mountMatrix({ emptyState: 'no-reports', activeRows: [], dormantRows: [] })
    await noReports.find('[data-testid="matrix-empty-generate"]').trigger('click')
    expect(noReports.emitted('generate')).toHaveLength(1)

    const noLanes = mountMatrix({ emptyState: 'no-lanes', activeRows: [], dormantRows: [] })
    expect(noLanes.find('[data-testid="matrix-empty-no-lanes"]').exists()).toBe(true)
    // 8C G2：0 正式泳道不再用整块空态替换网格——轴/网格/组同时在场
    expect(noLanes.find('[data-testid="matrix-grid"]').exists()).toBe(true)
    expect(noLanes.findAll('.omx-dayhead')).toHaveLength(14)
    expect(noLanes.find('[data-testid="other-topics-toggle"]').exists()).toBe(true)
    expect(noLanes.find('.omx-empty').exists()).toBe(false)
  })

  it('刷新失败但保留旧数据：stale 提示条 + 旧网格仍可读', () => {
    const wrapper = mountMatrix({ error: '超时', staleNotice: true })
    expect(wrapper.find('[data-testid="matrix-stale-notice"]').text()).toContain('超时')
    expect(wrapper.find('[data-testid="matrix-grid"]').exists()).toBe(true)
  })
})

// ── lane-matrix-coverage-and-others-row + lane-coverage-entry-list：覆盖组 / 其他话题整行（纯受控透传） ─

describe('ObservationMatrix — 覆盖组与「其他话题」整行', () => {
  const coverage = [{ topicId: '20', label: '以色列内部选举', lastSeenDate: '2026-11-09' }]

  it('覆盖组默认折叠：只有折叠条（数量 + 名字），展开后才是列表项', async () => {
    const wrapper = mountMatrix({ coverageLanes: coverage })
    const summary = wrapper.find('[data-testid="coverage-group-summary"]')
    expect(summary.exists()).toBe(true)
    expect(summary.text()).toContain('窗口内未进日报')
    expect(summary.text()).toContain('1')
    expect(summary.text()).toContain('以色列内部选举')
    expect(summary.text()).toContain('展开查看')
    // 矩阵网格里不再有为它渲染的行 / 日期格
    expect(wrapper.find('[data-testid="matrix-lane-20"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid^="matrix-cell-20-"]')).toHaveLength(0)

    await wrapper.find('[data-testid="coverage-toggle"]').trigger('click')
    const items = wrapper.findAll('[data-testid="coverage-list"] > li')
    expect(items).toHaveLength(1)
    expect(items[0]!.text()).toContain('以色列内部选举')
    expect(items[0]!.text()).toContain('最近命中 11-09')
  })

  it('覆盖清单失败：底部一行提示 + 重试（既有行/格照常渲染）', async () => {
    const wrapper = mountMatrix({ coverageLanes: [], coverageError: '接口挂了' })
    expect(wrapper.find('[data-testid="coverage-error-row"]').text()).toContain('窗口外泳道清单未取到')
    expect(wrapper.find('[data-testid="matrix-lane-10"]').exists()).toBe(true)
    await wrapper.find('[data-testid="coverage-retry"]').trigger('click')
    expect(wrapper.emitted('retryCoverage')).toHaveLength(1)
  })

  it('覆盖清单为空 / 读取中 → 不渲染组（不造空组）', () => {
    expect(mountMatrix({ coverageLanes: [] }).find('[data-testid="coverage-group-summary"]').exists()).toBe(false)
    expect(mountMatrix({ coverageLanes: coverage, coverageLoading: true })
      .find('[data-testid="coverage-group-summary"]').exists()).toBe(false)
  })

  it('「其他话题（未入泳道）」整行：点有日报的格 → emit selectOthersDay(date)', async () => {
    const wrapper = mountMatrix({
      otherTopicDays: { '2026-10-23': { date: '2026-10-23', sections: 3, clues: 7 } },
      otherTopicDaysAvailable: true,
    })
    expect(wrapper.find('[data-testid="matrix-other-row"]').exists()).toBe(true)
    await wrapper.find('[data-testid="matrix-other-cell-2026-10-23"]').trigger('click')
    expect(wrapper.emitted('selectOthersDay')![0]).toEqual(['2026-10-23'])
  })

  it('旧后端缺 other_topics → 整行不渲染（矩阵其余行为不变）', () => {
    const wrapper = mountMatrix()
    expect(wrapper.find('[data-testid="matrix-other-row"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="matrix-lane-10"]').exists()).toBe(true)
  })
})

// ── 8C G2：日报日驱动的日期轴 + 当日其他话题组（S11/GRP，纯受控） ─────────────
