import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import KeyEventTimelineView from './KeyEventTimelineView.vue'
import KeyEventLabel from './KeyEventLabel.vue'
import type { ObservationLaneRow } from './observationLanes'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'
import type { LaneDayReadingStore } from '~/features/tags/composables/useLaneDayReading'

// ── mock 数据源（useBoardKeyEvents 受控状态）─────────────────────────────────

const eventsState = {
  events: [] as BoardKeyEventItem[],
  loading: false,
  error: null as string | null,
  staleNotice: false,
  retry: vi.fn(),
  /** 当前展示内容的来源版块（stale 时点名用；null = 还没有内容）。 */
  contentSource: null as { boardId: number, boardLabel: string } | null,
}

/** 接线层捕获：composable 收到的 days ref（请求窗口断言用）。 */
const captured = vi.hoisted(() => ({ days: null as { value: number } | null }))

vi.mock('~/features/tags/composables/useBoardKeyEvents', async (importOriginal) => {
  const { computed } = await import('vue')
  const { buildKeyEventIndex } = await importOriginal<typeof import('~/features/tags/composables/useBoardKeyEvents')>()
  const state = () => eventsState
  return {
    buildKeyEventIndex,
    useBoardKeyEvents: (deps: { days: { value: number } }) => {
      captured.days = deps.days
      return {
        events: computed(() => state().events),
        eventIndex: computed(() => buildKeyEventIndex(state().events)),
        eventsByDate: computed(() => {
          const byDate = new Map<string, BoardKeyEventItem[]>()
          for (const e of state().events) {
            const list = byDate.get(e.sourcePeriodDate)
            if (list) list.push(e)
            else byDate.set(e.sourcePeriodDate, [e])
          }
          return byDate
        }),
        loading: computed(() => state().loading),
        error: computed(() => state().error),
        staleNotice: computed(() => state().staleNotice),
        contentSource: computed(() => state().contentSource),
        retry: state().retry,
      }
    },
  }
})

function event(id: string, topicId: string | null, date: string, title = `事件${id}`): BoardKeyEventItem {
  return {
    id, topicId, topicLabelSnapshot: `话题${topicId ?? '-'}`, sourcePeriodDate: date,
    titleSnapshot: title, summarySnapshot: '', articleIds: [],
  }
}

function row(topicId: string, label: string, dormant = false): ObservationLaneRow {
  return {
    topicId, label, watchLinked: false, sectionTotal: dormant ? 1 : 5,
    clueTotal: dormant ? 1 : 10, laneHeat: dormant ? 0 : 1, dormant, snapshot: null,
  }
}

const AXIS = ['2026-11-10', '2026-11-11', '2026-11-12', '2026-11-13', '2026-11-14']

/** 文章按需读三项的测试桩（其余成员不参与本组件，用 unknown 断言补齐）。 */
const readingStore = {
  readRelated: vi.fn(() => ({ status: 'idle' as const })),
  ensureRelatedArticles: vi.fn(async () => {}),
  retryRelatedArticle: vi.fn(async () => {}),
} as unknown as LaneDayReadingStore

function baseProps(overrides: Record<string, unknown> = {}) {
  return {
    boardId: 1980,
    boardLabel: '中东局势',
    axis: [...AXIS],
    activeRows: [row('93', '主线 A'), row('95', '主线 B')],
    dormantRows: [row('97', '休眠线', true)],
    selectedLaneId: null,
    selectedDate: null,
    today: '2026-11-14',
    sourceDays: 14,
    expandedDormant: false,
    loading: false,
    error: null,
    staleNotice: false,
    emptyState: null,
    reading: readingStore,
    ...overrides,
  }
}

function mountView(overrides: Record<string, unknown> = {}) {
  return mount(KeyEventTimelineView, { props: baseProps(overrides) })
}

async function settle() {
  await flushPromises()
  await flushPromises()
}

/** 从 start（date-only）起第 offset 天（UTC 推算，仅供测试造轴）。 */
function encodeDay(start: string, offset: number): string {
  const base = new Date(`${start}T00:00:00Z`)
  return new Date(base.getTime() + offset * 86400000).toISOString().slice(0, 10)
}

// happy-dom 无布局引擎：mock offsetWidth（标签实测宽）与 clientWidth（容器宽）。
beforeEach(() => {
  eventsState.events = []
  eventsState.loading = false
  eventsState.error = null
  eventsState.staleNotice = false
  eventsState.retry.mockClear()
  eventsState.contentSource = null
  captured.days = null
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
    configurable: true, get: () => 110,
  })
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true, get: () => 1600,
  })
})

afterEach(() => {
  delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetWidth
  delete (HTMLElement.prototype as unknown as Record<string, unknown>).clientWidth
})

describe('KeyEventTimelineView — 行集合与节点', () => {
  it('行数 = 持久化话题数（active + 休眠折叠不展开）；候选话题不占行、其事件不出现', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12'),
      event('cand', '999', '2026-11-12', '候选话题的事件'),
    ]
    const wrapper = mountView()
    await settle()
    const rows = wrapper.findAll('[data-testid^="key-event-row-"]')
    expect(rows.map(r => r.attributes('data-testid'))).toEqual(['key-event-row-93', 'key-event-row-95'])
    expect(wrapper.text()).not.toContain('候选话题的事件')
    // 休眠折叠：默认不展开（只有休眠 summary）
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(false)
  })

  it('休眠组展开后渲染休眠行（沿用休眠判定与折叠交互）', async () => {
    const wrapper = mountView({ expandedDormant: true })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(true)
    await wrapper.find('[data-testid="key-event-dormant-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
  })

  it('节点数 = 事件数：同日 3 条 → 3 节点 + 3 标签（不合并不伪造）', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12', '第一条'),
      event('b', '93', '2026-11-12', '第二条'),
      event('c', '93', '2026-11-12', '第三条'),
    ]
    const wrapper = mountView()
    await settle()
    const nodes = wrapper.findAll('[data-testid="key-event-node-93-2026-11-12"]')
    expect(nodes).toHaveLength(3)
    const labels = wrapper.findAllComponents(KeyEventLabel)
    expect(labels).toHaveLength(3)
    expect(wrapper.text()).toContain('第一条')
    expect(wrapper.text()).toContain('第三条')
  })

  it('节点恒在生命线（轴线）上：同日多条横向散开，不竖向压到别层标签', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12', '第一条'),
      event('b', '93', '2026-11-12', '第二条'),
      event('c', '93', '2026-11-12', '第三条'),
    ]
    const wrapper = mountView()
    await settle()
    const nodes = wrapper.findAll('[data-testid="key-event-node-93-2026-11-12"]')
    const tops = nodes.map(n => (n.element as HTMLElement).style.top)
    expect(new Set(tops).size).toBe(1)
    // 横向散开：三个节点的 transform 各不相同
    const transforms = nodes.map(n => (n.element as HTMLElement).style.transform)
    expect(new Set(transforms).size).toBe(3)
  })

  it('候选话题的确认事件不呈现：时间轴无行、当日阅读区也不出现（spec「候选话题不占行」）', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12', '持久话题的事件'),
      event('cand', '999', '2026-11-12', '候选话题的事件'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.text()).not.toContain('候选话题的事件')
    expect(wrapper.text()).toContain('持久话题的事件')
  })

  it('无事件日不落节点；无事件行只有虚线轴 + 行头「无确认事件」', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView()
    await settle()
    expect(wrapper.findAll('[data-testid^="key-event-node-95-"]')).toHaveLength(0)
    const rowB = wrapper.find('[data-testid="key-event-row-95"]')
    expect(rowB.find('.kev-axis-line').exists()).toBe(true)
    expect(rowB.find('.kev-row-meta').text()).toContain('无确认事件')
    // 节点只在事件日
    expect(wrapper.find('[data-testid="key-event-node-93-2026-11-12"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-node-93-2026-11-13"]').exists()).toBe(false)
  })

  it('行头带事件数与最近事件日', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-11'),
      event('b', '93', '2026-11-13'),
    ]
    const wrapper = mountView()
    await settle()
    const meta = wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-meta').text()
    expect(meta).toContain('2 事件')
    expect(meta).toContain('最近 11/13')
  })

  it('点行头 → selectLane（与矩阵同一选中态）；点节点 → selectDate', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView()
    await settle()
    await wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-head').trigger('click')
    expect(wrapper.emitted('selectLane')).toEqual([['93']])
    await wrapper.find('[data-testid="key-event-node-93-2026-11-12"]').trigger('click')
    expect(wrapper.emitted('selectDate')).toEqual([['2026-11-12']])
  })

  it('选中列淡底：selectedDate 命中列有 is-col-selected', async () => {
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const selected = wrapper.findAll('.kev-cell.is-col-selected')
    expect(selected.length).toBeGreaterThan(0)
  })
})

describe('KeyEventTimelineView — 布局与滚动', () => {
  it('列宽 = max(64, 可用宽/日数)：宽屏均分（clientWidth 1600、5 天 → (1600-192)/5 > 200）', async () => {
    const wrapper = mountView()
    await settle()
    const heads = wrapper.find('.kev-day-heads').element as HTMLElement
    const width = Number.parseFloat(heads.style.getPropertyValue('--kev-col'))
    expect(width).toBeGreaterThan(200)
  })

  it('列宽守 64 下限：窗口不足时每列 64px（横滚承接），不压缩', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true, get: () => 300,
    })
    const wrapper = mountView({ axis: Array.from({ length: 30 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`) })
    await settle()
    const heads = wrapper.find('.kev-day-heads').element as HTMLElement
    expect(heads.style.getPropertyValue('--kev-col')).toBe('64px')
  })

  it('1440 宽 + 30 天：总宽超出舞台（由容器内横滚承接，不压缩列宽）', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true, get: () => 1440,
    })
    const axis = Array.from({ length: 30 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)
    const wrapper = mountView({ axis })
    await settle()
    const heads = wrapper.find('.kev-day-heads').element as HTMLElement
    expect(heads.style.getPropertyValue('--kev-col')).toBe('64px')
    // 画布内容宽 = 行头 192 + 30 × 64 = 2112 > 容器 1440 → 舞台内横向滚动
    const inner = wrapper.find('[data-testid="key-event-grid"]').element as HTMLElement
    expect(inner.style.minWidth).toContain('2112px')
  })

  it('2560 宽 + 14 天：列宽均分填满（>120px，不固定小列宽留白）', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true, get: () => 2560,
    })
    const axis = Array.from({ length: 14 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)
    const wrapper = mountView({ axis })
    await settle()
    const heads = wrapper.find('.kev-day-heads').element as HTMLElement
    expect(Number.parseFloat(heads.style.getPropertyValue('--kev-col'))).toBeGreaterThan(120)
  })

  it('长窗（55 天）+ 层数 ≤ 4：标签照常显示事件名称（MUST NOT 按天数一刀切收起）', async () => {
    // 用户实机观察窗：2026-09-29 ~ 11-22 共 55 天（30 天按钮是取数档，观察窗可以更宽）。
    // 稀疏事件（每 14 天一条）分层只占 1 层 → 名称照常显示；旧实现 axis.length > 31 会整窗收起。
    const longAxis = Array.from({ length: 55 }, (_, i) => encodeDay('2026-09-29', i))
    eventsState.events = [
      event('s0', '93', longAxis[0]!, '五十五天窗的第一条'),
      event('s1', '93', longAxis[14]!, '五十五天窗的第四条'),
    ]
    const wrapper = mountView({ axis: longAxis, today: '2026-11-22', sourceDays: 30 })
    await settle()

    const labels = wrapper.findAllComponents(KeyEventLabel)
    expect(labels, '轴长 55 天、层数 ≤ 4 → 标签必须照常渲染（不只剩光点）').toHaveLength(2)
    expect(wrapper.text()).toContain('五十五天窗的第一条')
    expect(wrapper.text()).toContain('五十五天窗的第四条')
    expect(wrapper.find(`[data-testid="key-event-node-93-${longAxis[0]}"]`).text()).not.toContain('★')
    // 行高按 1 层算（46 + 44），不因天数封顶
    const body = wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-body').element as HTMLElement
    expect(Number.parseFloat(body.style.height)).toBe(90)
  })

  it('层数超上限才收起：该行只渲染节点 + ★计数（无标签文字），同行集合里其他行不受连坐', async () => {
    // 行 93：同一天 5 条（横向散开 + 实测宽 110 → 贪心分层 5 层 > 4 层上限）→ 该行退化；
    // 行 95：1 条且远处无邻居 → 1 层 → 名称照常显示。
    const longAxis = Array.from({ length: 55 }, (_, i) => encodeDay('2026-09-29', i))
    eventsState.events = [
      event('d0', '93', longAxis[10]!, '拥挤行第一条'),
      event('d1', '93', longAxis[10]!, '拥挤行第二条'),
      event('d2', '93', longAxis[10]!, '拥挤行第三条'),
      event('d3', '93', longAxis[10]!, '拥挤行第四条'),
      event('d4', '93', longAxis[10]!, '拥挤行第五条'),
      event('s0', '95', longAxis[0]!, '宽松行唯一的名称'),
    ]
    const wrapper = mountView({ axis: longAxis, today: '2026-11-22', sourceDays: 30 })
    await settle()

    const denseRow = wrapper.find('[data-testid="key-event-row-93"]')
    expect(denseRow.findAllComponents(KeyEventLabel), '超上限行不再渲染标签').toHaveLength(0)
    expect(denseRow.text()).not.toContain('拥挤行第一条')
    const nodes = denseRow.findAll(`[data-testid="key-event-node-93-${longAxis[10]}"]`)
    expect(nodes).toHaveLength(5)
    expect(nodes[0]!.text(), '当日事件计数落在首个节点上').toContain('★5')
    const denseBody = denseRow.find('.kev-row-body').element as HTMLElement
    expect(Number.parseFloat(denseBody.style.height), '超限行行高封顶 246px').toBe(246)

    // 逐行判定：同一次渲染里另一行照常显示名称（MUST NOT 整视图连坐收起）
    const looseRow = wrapper.find('[data-testid="key-event-row-95"]')
    expect(looseRow.findAllComponents(KeyEventLabel)).toHaveLength(1)
    expect(looseRow.text()).toContain('宽松行唯一的名称')
  })

  it('首帧乐观显示标签：未测量前不误判成「超限」，按测量结果收敛到收起', async () => {
    const longAxis = Array.from({ length: 55 }, (_, i) => encodeDay('2026-09-29', i))
    eventsState.events = Array.from({ length: 5 }, (_, i) => event(`d${i}`, '93', longAxis[10]!, `拥挤行第${i}条`))
    const wrapper = mountView({ axis: longAxis, today: '2026-11-22', sourceDays: 30 })
    // 未布局（无实测层数）时先乐观渲染标签，允许一帧后按实测结果收敛
    expect(wrapper.findAllComponents(KeyEventLabel).length).toBeGreaterThan(0)
    await settle()
    expect(wrapper.findAllComponents(KeyEventLabel)).toHaveLength(0)
    expect(wrapper.find(`[data-testid="key-event-node-93-${longAxis[10]}"]`).text()).toContain('★5')
  })

  it('左键拖拽平移：位移 >5px 才平移、未超阈值不动（不误伤点选）', async () => {
    const wrapper = mountView()
    await settle()
    const surface = wrapper.find('[data-testid="key-event-scroll"]')
    const el = surface.element as HTMLElement
    el.scrollLeft = 0
    await surface.trigger('pointerdown', { pointerId: 1, clientX: 500, clientY: 100, button: 0 })
    await surface.trigger('pointermove', { pointerId: 1, clientX: 503, clientY: 101 })
    expect(el.scrollLeft, '位移 3px < 5px 阈值：不进入平移').toBe(0)
    await surface.trigger('pointermove', { pointerId: 1, clientX: 523, clientY: 101 })
    expect(el.scrollLeft, '位移 23px：向右拖 → 内容左移（scrollLeft = start − dx）').toBe(-23)
    await surface.trigger('pointerup', { pointerId: 1 })
  })

  it('选中日滚入视口：selectedDate 变化后 scrollLeft 朝该列移动', async () => {
    const wrapper = mountView()
    await settle()
    const el = wrapper.find('[data-testid="key-event-scroll"]').element as HTMLElement
    el.scrollLeft = 0
    await wrapper.setProps({ selectedDate: '2026-11-13' })
    await settle()
    expect(el.scrollLeft).not.toBe(0)
  })
})

describe('KeyEventTimelineView — 状态分层', () => {
  it('空态：窗口内 0 条 confirmed → 行与轴照常渲染 + 说明指向日报阅读台', async () => {
    eventsState.events = []
    const wrapper = mountView()
    await settle()
    expect(wrapper.find('[data-testid="key-event-empty-window"]').text()).toContain('这段时间没有确认的关键事件')
    expect(wrapper.find('[data-testid="key-event-empty-window"]').text()).toContain('日报阅读台')
    expect(wrapper.find('[data-testid="key-event-row-93"]').exists()).toBe(true)
    expect(wrapper.findAll('.kev-day')).toHaveLength(AXIS.length)
  })

  it('事件读取失败（无旧内容）：错误条 + 重试，MUST NOT 冒称 0 条', async () => {
    eventsState.events = []
    eventsState.error = 'boom'
    const wrapper = mountView()
    await settle()
    expect(wrapper.find('[data-testid="key-event-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-error"]').text()).toContain('读取失败')
    expect(wrapper.find('[data-testid="key-event-empty-window"]').exists()).toBe(false)
    await wrapper.find('[data-testid="key-event-retry"]').trigger('click')
    expect(eventsState.retry).toHaveBeenCalled()
  })

  it('事件刷新失败但已有内容：保留内容 + stale 条', async () => {
    eventsState.events = [event('a', '93', '2026-11-12', '保留的旧事件')]
    eventsState.error = '网络断了'
    eventsState.staleNotice = true
    const wrapper = mountView()
    await settle()
    expect(wrapper.find('[data-testid="key-event-stale-notice"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('保留的旧事件')
    await wrapper.find('[data-testid="key-event-stale-retry"]').trigger('click')
    expect(eventsState.retry).toHaveBeenCalled()
  })

  it('stale 条点名来源版块：保留的旧数据属于哪个版块要说清楚（review R-7）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12', '保留的旧事件')]
    eventsState.error = '网络断了'
    eventsState.staleNotice = true
    eventsState.contentSource = { boardId: 1980, boardLabel: '中东局势' }
    const wrapper = mountView()
    await settle()
    const notice = wrapper.find('[data-testid="key-event-stale-notice"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text(), 'stale 文案 MUST NOT 只说「之前的数据」').toContain('中东局势')
  })

  it('stale 条跨版块时明说「不是当前版块的数据」（review R-7）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12', '保留的旧事件')]
    eventsState.error = '网络断了'
    eventsState.staleNotice = true
    eventsState.contentSource = { boardId: 1975, boardLabel: 'AI 与机器学习' }
    const wrapper = mountView()
    await settle()
    const notice = wrapper.find('[data-testid="key-event-stale-notice"]')
    expect(notice.text()).toContain('AI 与机器学习')
    expect(notice.text()).toContain('不是当前版块')
  })

  it('stale 条在版块名缺失时回落到版块 ID（也能判断是不是当前版块）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12', '保留的旧事件')]
    eventsState.error = '网络断了'
    eventsState.staleNotice = true
    eventsState.contentSource = { boardId: 1975, boardLabel: '' }
    const wrapper = mountView()
    await settle()
    expect(wrapper.find('[data-testid="key-event-stale-notice"]').text()).toContain('1975')
  })

  it('A 档 loading（行集合未到）：骨架占位', async () => {
    const wrapper = mountView({ loading: true, activeRows: [], dormantRows: [] })
    await settle()
    expect(wrapper.find('[data-testid="key-event-skeleton"]').exists()).toBe(true)
  })

  it('A 档首错：错误条 + 重试（转发 retry）', async () => {
    const wrapper = mountView({ error: 'lane-dynamics 挂了', activeRows: [], dormantRows: [] })
    await settle()
    expect(wrapper.find('[data-testid="key-event-lane-error"]').exists()).toBe(true)
    await wrapper.find('[data-testid="key-event-lane-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it('无持久化话题：诚实空态（不补候选行）', async () => {
    const wrapper = mountView({ activeRows: [], dormantRows: [], emptyState: 'no-lanes' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-empty-no-lanes"]').text()).toContain('暂无持久化话题')
  })

  it('切换视图保留选中态：props 的 selectedLane/selectedDate 不因渲染而重置（受控契约）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedLaneId: '93', selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-head').classes()).toContain('is-selected')
    expect(wrapper.emitted('selectLane')).toBeUndefined()
    expect(wrapper.emitted('selectDate')).toBeUndefined()
  })
})

// ── 请求窗口（review R-4：365 封顶 → 轴最左若干天静默无事件）────────────────

describe('KeyEventTimelineView — 请求窗口覆盖轴起点', () => {
  it('轴跨度 376 天（日报落后今天 11 天）→ 请求 376 天，MUST NOT 被截成 365', async () => {
    // 池左沿 = anchor−364 = 2025-10-17（anchor = 2026-10-16，落后今天 11 天）。
    // 旧实现 `Math.min(365, …)` 会把窗口截到 [今天−364, 今天]，轴最左 11 天的事件
    // 静默不返回 → 轴上凭空少节点。
    const longAxis = Array.from({ length: 376 }, (_, i) => encodeDay('2025-10-17', i))
    // 骨架态即可断言（请求窗口与渲染无关），避免渲染 376 列拖慢用例
    mountView({ axis: longAxis, today: '2026-10-27', sourceDays: 365, loading: true, activeRows: [], dormantRows: [] })
    expect(captured.days, '请求天数 ref 未接线').not.toBeNull()
    expect(captured.days!.value).toBe(376)
    expect(captured.days!.value, 'MUST NOT 封顶 365').toBeGreaterThan(365)
  })

  it('轴未到（空轴）→ 请求窗口回落取数档天数', async () => {
    mountView({ axis: [], today: '2026-11-14', sourceDays: 30 })
    expect(captured.days!.value).toBe(30)
  })
})

// ── 实机验收回归护栏（2026-11-23 ui-verification/report.md 的 A-9 / A-23 / A-12 / A-8）──
// 上一轮单测只喂了纯函数与组件自身，漏了「父组件怎么拿到实测宽度」这条接线，
// 绿灯漏掉了 4 个实机缺陷；下面 4 条全部走**接线层**断言（父组件拿到的宽 / 行体宽 /
// 舞台容器宽 / 滚入触发信号），回归时能直接抓住同型接线 bug。

describe('KeyEventTimelineView — 实机验收回归护栏', () => {
  it('P1-① 标签按实测宽度分层（测量对象必须是真实 DOM 元素，不能是组件公开实例）', async () => {
    // 只给**真实 DOM 元素**宽度；组件公开实例没有 getAttribute → 量不到宽度（= 0）。
    // 旧接线（直接对组件实例量 offsetWidth）→ 两个标签都落第 0 层、top 相同、行高不增高。
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
      configurable: true,
      get(this: HTMLElement) {
        if (typeof this.getAttribute !== 'function') return 0
        return this.getAttribute('data-testid') === 'key-event-label-b' ? 240 : 90
      },
    })
    eventsState.events = [
      event('a', '93', '2026-11-12', '短标题'),
      event('b', '93', '2026-11-12', '很长的一个标题'),
    ]
    const wrapper = mountView()
    await settle()
    const labelA = wrapper.find('[data-testid="key-event-label-a"]').element as HTMLElement
    const labelB = wrapper.find('[data-testid="key-event-label-b"]').element as HTMLElement
    expect(labelA.style.top, '两个标签必须分层（top 不同），MUST NOT 全挤第 0 层')
      .not.toBe(labelB.style.top)
    const body = wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-body').element as HTMLElement
    // 单层行高 = 46 + 44 = 90px；行高必须随层数增高（MUST NOT 恒为基础行高）
    expect(Number.parseFloat(body.style.height)).toBeGreaterThan(90)
  })

  it('P1-② 行内虚线轴有可撑开的宽度（行体宽 = 轴宽，生命线不再是 0px）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView()
    await settle()
    const row = wrapper.find('[data-testid="key-event-row-93"]')
    expect(row.find('.kev-axis-line').exists()).toBe(true)
    const body = row.find('.kev-row-body').element as HTMLElement
    const bodyWidth = Number.parseFloat(body.style.width)
    expect(bodyWidth, '行体必须有显式宽度（否则虚线轴 left:0;right:0 撑成 0px）').toBeGreaterThan(0)
    const col = Number.parseFloat(
      (wrapper.find('.kev-day-heads').element as HTMLElement).style.getPropertyValue('--kev-col'),
    )
    expect(bodyWidth).toBeCloseTo(col * AXIS.length, 1)
  })

  it('P2-① 舞台容器晚于组件出现（骨架态）也要测到宽：列宽均分填满、不钉在 64px 下限', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true, get: () => 2560,
    })
    const wide = Array.from({ length: 14 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)
    const wrapper = mountView({ loading: true, activeRows: [], dormantRows: [], axis: wide })
    await settle()
    expect(wrapper.find('[data-testid="key-event-scroll"]').exists(), '骨架态下还没有滚动容器').toBe(false)
    await wrapper.setProps({ loading: false, activeRows: [row('93', '主线 A')] })
    await settle()
    const heads = wrapper.find('.kev-day-heads').element as HTMLElement
    expect(
      Number.parseFloat(heads.style.getPropertyValue('--kev-col')),
      '容器出现后必须按实测宽均分（MUST NOT 钉在 64px 下限留白）',
    ).toBeGreaterThan(120)
  })

  it('P2-② 轴/窗口变化后选中日自动滚入视口（不只换日才滚）', async () => {
    const wrapper = mountView({ selectedDate: '2026-11-13' })
    await settle()
    const el = wrapper.find('[data-testid="key-event-scroll"]').element as HTMLElement
    el.scrollLeft = 0
    // 14 天 → 30 天：轴变了、选中日没变，仍要把选中日拉回视口
    const wide = Array.from({ length: 30 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)
    await wrapper.setProps({ axis: wide })
    await settle()
    expect(el.scrollLeft, '切 30 天后选中日应自动滚入视口').toBeGreaterThan(0)
  })

  it('P2-② 轴内容平移（长度与列宽都不变）也把选中日拉回视口', async () => {
    const axisA = Array.from({ length: 14 }, (_, i) => {
      return new Date(Date.UTC(2026, 9, 25 + i)).toISOString().slice(0, 10)
    })
    const axisB = Array.from({ length: 14 }, (_, i) => {
      return new Date(Date.UTC(2026, 9, 24 + i)).toISOString().slice(0, 10)
    })
    const wrapper = mountView({ axis: axisA, selectedDate: '2026-11-05' })
    await settle()
    const el = wrapper.find('[data-testid="key-event-scroll"]').element as HTMLElement
    const before = el.scrollLeft
    await wrapper.setProps({ axis: axisB })
    await settle()
    expect(el.scrollLeft, '轴内容平移后选中日位置变了，必须重新滚入视口').not.toBe(before)
    expect(el.scrollLeft).toBeGreaterThan(0)
  })

  it('A-8 切档导致滚动容器卸载重建：重建后的新容器仍把选中日滚入视口（滚动不丢）', async () => {
    // 实机路径（2026-11-23 复验 A-8）：切 30 天 → setSourceDays 清空数据走骨架态
    // → `.kev-scroll` 整块卸载 → 数据到位重新挂上。滚动位置随**旧容器**一起丢失，
    // 而重建前后轴签名与列宽都没变 → watch 不再二次触发 → 之前的滚入白做。
    // 上一轮单测只验了「轴变触发滚动」（容器不重建），所以没抓住这个坑。
    //
    // happy-dom 的平滑滚动是 setTimeout 异步落地的 → 这里把 scrollTo 换成「立即生效」的
    // 桩，让断言不依赖定时器时序（浏览器里 smooth 只是动画，落点相同）。
    const originalScrollTo = Element.prototype.scrollTo
    Object.defineProperty(Element.prototype, 'scrollTo', {
      configurable: true,
      writable: true,
      value: function scrollToStub(this: Element, options?: ScrollToOptions | number): void {
        const left = typeof options === 'object' && options !== null
          ? Number(options.left ?? 0)
          : Number(options ?? 0)
        this.scrollLeft = left
      },
    })
    try {
      const wide = Array.from({ length: 30 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)
      const wrapper = mountView({ selectedDate: '2026-11-13' })
      await settle()
      const before = wrapper.find('[data-testid="key-event-scroll"]').element as HTMLElement

      // 切档：行集合清空 → 骨架态 → 滚动容器整块卸载
      await wrapper.setProps({ loading: true, activeRows: [], dormantRows: [] })
      await settle()
      expect(wrapper.find('[data-testid="key-event-scroll"]').exists(), '骨架态下滚动容器已卸载').toBe(false)

      // 数据到位 + 轴换成 30 天 → 滚动容器重建
      await wrapper.setProps({ loading: false, activeRows: [row('93', '主线 A')], dormantRows: [], axis: wide })
      await settle()
      const after = wrapper.find('[data-testid="key-event-scroll"]').element as HTMLElement
      expect(after, '数据到位后滚动容器应重建').not.toBe(before)
      expect(
        after.scrollLeft,
        '重建后的新容器必须把选中日滚入视口（MUST NOT 停在 scrollLeft=0，选中日会落在舞台右沿之外）',
      ).toBeGreaterThan(0)
    } finally {
      Element.prototype.scrollTo = originalScrollTo
    }
  })
})
