import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import KeyEventTimelineView from './KeyEventTimelineView.vue'
import KeyEventLabel from './KeyEventLabel.vue'
import { layoutKeyEventBands } from './keyEventLayout'
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

/**
 * 归带函数的可控桩：调用记录 + 结果覆盖。
 *
 * 用途有二：① 断言**卡片矩形真的作为第 3 参传给了归带函数**（下带标签因此避开卡片列）；
 * ② 覆盖两条“真实数据几乎不触发”的几何无解路径（归带溢出 / 组内引线穿组内标签）——
 * 它们在真实几何下需要 400+ 层或超宽标签，故用可控桩覆盖（M2 遗留接线）。
 */
const bandLayout = vi.hoisted(() => ({
  calls: [] as unknown[][],
  override: null as null | ((groups: readonly { items: readonly { key: string }[] }[]) => unknown),
}))

vi.mock('./keyEventLayout', async (importOriginal) => {
  const original = await importOriginal<typeof import('./keyEventLayout')>()
  return {
    ...original,
    layoutKeyEventBands: (...args: Parameters<typeof original.layoutKeyEventBands>) => {
      bandLayout.calls.push(args)
      const override = bandLayout.override
      if (!override) return original.layoutKeyEventBands(...args)
      const result = override(args[0])
      return (result ?? original.layoutKeyEventBands(...args)) as ReturnType<typeof original.layoutKeyEventBands>
    },
  }
})

vi.mock('~/features/tags/composables/useBoardKeyEvents', async (importOriginal) => {
  const { computed } = await import('vue')
  const { buildKeyEventIndex, groupUnassignedEvents } =
    await importOriginal<typeof import('~/features/tags/composables/useBoardKeyEvents')>()
  const state = () => eventsState
  return {
    buildKeyEventIndex,
    groupUnassignedEvents,
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

/**
 * 找到含指定事件键的那次归带调用（一次调用 = 一行）。
 *
 * 取**最后一次**匹配：早于容器测量的那次调用列宽可能还是 64px 下限，值不可靠。
 */
function bandCallFor(key: string): unknown[] | undefined {
  const matches = bandLayout.calls.filter(call => {
    const groups = call[0] as { items: { key: string }[] }[]
    return groups.some(group => group.items.some(item => item.key === key))
  })
  return matches.at(-1)
}

/** 组件源码（样式契约用例用；happy-dom 不解析 scoped CSS）。 */
async function viewSource(): Promise<string> {
  const { readFileSync } = await import('node:fs')
  const { resolve } = await import('node:path')
  return readFileSync(
    resolve(process.cwd(), 'app/features/tags/components/console/KeyEventTimelineView.vue'),
    'utf-8',
  )
}

/** 节点盒宽（px）：空节点 18；带「MM.DD ★N」计数盒的更宽（review R-30 的跨列断言按它算）。 */
const NODE_WIDTH = 18
/**
 * 收起节点盒宽估算：0.55rem（8.8px）等宽数字约 5.2px/字，加左右内边距与描边 8px。
 *
 * 真实值取决于字体渲染（happy-dom 无布局引擎），取值与 `.kev-node.has-count`
 * 的 `font-size: 0.55rem` / `padding: 0 3px` 同步；「11.05 ★2」≈ 50px，
 * 64px 列宽下两侧各剩 ~7px（用例里断言 ≤ 列宽）。
 */
const NODE_COUNT_CHAR_WIDTH = 5.2
const NODE_COUNT_BOX_PADDING = 8
function countBoxWidth(text: string): number {
  return Math.ceil(text.trim().length * NODE_COUNT_CHAR_WIDTH) + NODE_COUNT_BOX_PADDING
}

/**
 * 节点矩形（行体坐标系）：从渲染出的 transform 解析散开偏移 + **元素实测盒宽**。
 *
 * 盒宽按实测（`offsetWidth`）而不是固定 18px：收起行带「★N」的节点有 padding 与
 * 边框、盒宽 > 18px，固定 ±9px 推算检不出跨列侵入（review R-30）。
 */
function nodeRects(
  wrapper: ReturnType<typeof mountView>,
  topicId: string,
  date: string,
  axis: readonly string[],
): { left: number, right: number }[] {
  const col = Number.parseFloat(
    (wrapper.find('.kev-day-heads').element as HTMLElement).style.getPropertyValue('--kev-col'),
  )
  const center = axis.indexOf(date) * col + col / 2
  return wrapper.findAll(`[data-testid="key-event-node-${topicId}-${date}"]`).map((node) => {
    const element = node.element as HTMLElement
    const transform = element.style.transform
    const offset = Number(/translateX\(calc\(-50%\s*\+\s*(-?[\d.]+)px\)\)/.exec(transform)?.[1] ?? 0)
    const width = element.offsetWidth || NODE_WIDTH
    const left = center + offset - width / 2
    return { left, right: left + width }
  })
}

// happy-dom 无布局引擎：mock offsetWidth（标签实测宽与节点盒宽）与 clientWidth（容器宽）。
beforeEach(() => {
  eventsState.events = []
  eventsState.loading = false
  eventsState.error = null
  eventsState.staleNotice = false
  eventsState.retry.mockClear()
  eventsState.contentSource = { boardId: 1980, boardLabel: '中东局势' }
  captured.days = null
  bandLayout.calls = []
  bandLayout.override = null
  // 标签实测宽固定 110；节点按真实盒宽（带「MM.DD ★N」计数盒的更宽 —— review R-30）。
  Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
    configurable: true,
    get(this: HTMLElement) {
      if (this.classList?.contains('kev-node')) {
        const text = (this.textContent ?? '').trim()
        return text.includes('★') ? countBoxWidth(text) : NODE_WIDTH
      }
      return 110
    },
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
  it('行数 = 持久化话题数（active + 休眠折叠不展开）；候选话题不新增行、其事件进「其他」行', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12'),
      event('cand', '999', '2026-11-12', '候选话题的事件'),
    ]
    const wrapper = mountView()
    await settle()
    const rows = wrapper.findAll('[data-testid^="key-event-row-"]')
    expect(rows.map(r => r.attributes('data-testid'))).toEqual([
      'key-event-row-93',
      'key-event-row-95',
      'key-event-row-__other__',
    ])
    expect(wrapper.find('[data-testid="key-event-row-999"]').exists()).toBe(false)
    // 事件不丢：收进「其他」行
    expect(wrapper.text()).toContain('候选话题的事件')
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

  it('候选话题的确认事件只在「其他」行呈现，不混入持久话题行（spec「候选话题不占行」）', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12', '持久话题的事件'),
      event('cand', '999', '2026-11-12', '候选话题的事件'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.text()).toContain('持久话题的事件')
    expect(wrapper.text()).toContain('候选话题的事件')
    // 不生成候选行；事件节点只落在「其他」行
    expect(wrapper.find('[data-testid="key-event-row-999"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-node-999-2026-11-12"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-node-__other__-2026-11-12"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-node-93-2026-11-12"]').exists()).toBe(true)
  })

  it('无事件日不落节点；无事件行只有虚线轴 + 行头「无确认事件」', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView()
    await settle()
    expect(wrapper.findAll('[data-testid^="key-event-node-95-"]')).toHaveLength(0)
    const rowB = wrapper.find('[data-testid="key-event-row-95"]')
    expect(rowB.find('.kev-axis-line').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-rowhead-95"]').find('.kev-row-meta').text()).toContain('无确认事件')
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
    // 行头在独立行头列（R-33：不参与横向滚动）
    const meta = wrapper.find('[data-testid="key-event-rowhead-93"]').find('.kev-row-meta').text()
    expect(meta).toContain('2 事件')
    expect(meta).toContain('最近 11/13')
  })

  it('点行头不产生选中（行头是只读 div，不再 emit selectLane）；点节点 → selectDate', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView()
    await settle()
    const head = wrapper.find('[data-testid="key-event-rowhead-93"]')
    // 2026-11-24 用户反馈：本视图选中话题无联动下游 → 行头改成只读列。
    expect(head.element.tagName, '行头 MUST NOT 是 button（不可点、不可键盘激活）').toBe('DIV')
    expect(head.attributes('type'), '行头 MUST NOT 再带 button 的 type').toBeUndefined()
    await head.trigger('click')
    expect(wrapper.emitted('selectLane'), '点行头 MUST NOT emit selectLane（无假联动）').toBeUndefined()
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

describe('KeyEventTimelineView — 「其他」兜底行', () => {
  it('有未归属事件时渲染，位置在休眠组之后，且休眠组收起时仍可见', async () => {
    eventsState.events = [event('cand', '999', '2026-11-12', '候选事件')]
    const wrapper = mountView()
    await settle()
    const html = wrapper.html()
    const groupAt = html.indexOf('key-event-dormant-toggle')
    const otherAt = html.indexOf('key-event-row-__other__')
    expect(groupAt).toBeGreaterThan(-1)
    expect(otherAt).toBeGreaterThan(groupAt)
    expect(wrapper.find('[data-testid="key-event-row-__other__"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-rowhead-__other__"]').text()).toContain('其他')
  })

  it('无兜底事件时不渲染该行（不造空行、不占位 0 条）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView()
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-__other__"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-rowhead-__other__"]').exists()).toBe(false)
  })

  it('读数含兜底行：视图头当日条数 / 行头事件数与当日条数', async () => {
    eventsState.events = [
      event('c1', '999', '2026-11-12', '候选 A'),
      event('c2', '998', '2026-11-12', '候选 B'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-selected-count"]').text()).toBe('11/12 · 2 条')
    expect(wrapper.find('[data-testid="key-event-rowhead-__other__"]').find('.kev-row-meta').text())
      .toContain('2 事件')
    expect(wrapper.find('[data-testid="key-event-rowday-__other__"]').text()).toBe('当日 2 条')
  })

  it('点行头不产生选中也不写 selectLane（兜底行无对应矩阵行）', async () => {
    eventsState.events = [event('cand', '999', '2026-11-12', '候选事件')]
    const wrapper = mountView()
    await settle()
    const head = wrapper.find('[data-testid="key-event-rowhead-__other__"]')
    expect(head.element.tagName).toBe('DIV')
    await head.trigger('click')
    expect(wrapper.emitted('selectLane')).toBeUndefined()
    expect(wrapper.emitted('selectDate')).toBeUndefined()
  })

  it('无归属（topicId=null）事件进兜底行，当日卡片显示冻结话题名', async () => {
    eventsState.events = [event('orphan', null, '2026-11-12', '无归属事件')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-__other__"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('无归属事件')
    const cards = wrapper.find('[data-testid="key-event-cards-__other__"]')
    expect(cards.exists()).toBe(true)
    expect(cards.text()).toContain('话题-')
  })
  it('同日多条兜底事件：节点与标签一一对应（不合并不伪造）', async () => {
    eventsState.events = [
      event('c1', '999', '2026-11-12', '候选甲'),
      event('c2', '998', '2026-11-12', '候选乙'),
    ]
    const wrapper = mountView()
    await settle()
    expect(wrapper.findAll('[data-testid="key-event-node-__other__-2026-11-12"]')).toHaveLength(2)
    expect(wrapper.findAllComponents(KeyEventLabel)).toHaveLength(2)
    const rowText = wrapper.find('[data-testid="key-event-row-__other__"]').text()
    expect(rowText).toContain('候选甲')
    expect(rowText).toContain('候选乙')
  })

  it('休眠组展开时「其他」行仍排在全部休眠行之后', async () => {
    eventsState.events = [event('cand', '999', '2026-11-12', '候选事件')]
    const wrapper = mountView({ expandedDormant: true })
    await settle()
    const html = wrapper.html()
    const dormantRow = html.indexOf('key-event-row-97')
    expect(dormantRow).toBeGreaterThan(-1)
    expect(html.indexOf('key-event-row-__other__')).toBeGreaterThan(dormantRow)
  })

  it('无持久化话题但有兜底事件：说明行与「其他」行并存（不吞事件）', async () => {
    eventsState.events = [event('cand', '999', '2026-11-12', '候选事件')]
    const wrapper = mountView({ activeRows: [], dormantRows: [], emptyState: 'no-lanes' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-empty-no-lanes"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-row-__other__"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('候选事件')
  })

  it('只有无归属事件时刷新失败：旧内容与「其他」行保留（首错判据不得用话题索引）', async () => {
    eventsState.events = [event('orphan', null, '2026-11-12', '无归属旧事件')]
    eventsState.error = '网络断了'
    eventsState.staleNotice = true
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-stale-notice"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-row-__other__"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('无归属旧事件')
  })

  it('轴外日期的事件不进兜底行、不计入读数', async () => {
    eventsState.events = [event('cand', '999', '2026-10-01', '轴外事件')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-__other__"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-selected-count"]').text()).toBe('11/12 · 0 条')
  })

  it('跨版块 stale 数据不进兜底行（来源版块必须与当前版块一致）', async () => {
    eventsState.events = [event('cand', '999', '2026-11-12', '旧版块候选事件')]
    eventsState.contentSource = { boardId: 1975, boardLabel: 'AI 与机器学习' }
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-__other__"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('旧版块候选事件')
    expect(wrapper.find('[data-testid="key-event-selected-count"]').text()).toBe('11/12 · 0 条')
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
    // 画布内容宽 = 30 × 64 = 1920px > 滚动区（舞台 1440 − 行头 192 = 1248）→ 舞台内横向滚动。
    // 旧值 2112（含行头 192）：行头已移出滚动区（R-33），滚动区的 minWidth 不再包含行头。
    const inner = wrapper.find('[data-testid="key-event-grid"]').element as HTMLElement
    expect(inner.style.minWidth).toContain('1920px')
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
    // 行高按双带层数算（50 + 32 × 层数）：两处事件分处上下两带 → 合计 2 层 = 114px
    const body = wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-body').element as HTMLElement
    expect(Number.parseFloat(body.style.height)).toBe(114)
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
    // 旧值 5 → 新值 1：64px 窄列装不下 5 个节点（5×18 + 4×4 = 106 > 64），
    // 按 review R-22 该日降级为**单节点 + 当日计数**（MUST NOT 压盖 / 竖向叠放）。
    expect(nodes, '一列装不下 → 该日降级为单个节点').toHaveLength(1)
    expect(nodes[0]!.text(), '当日事件计数落在首个节点上').toContain('★5')
    const denseBody = denseRow.find('.kev-row-body').element as HTMLElement
    expect(Number.parseFloat(denseBody.style.height), '超限行行高 = 50 + 32 × 5 = 210px（未触 246 封顶）').toBe(210)

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

describe('KeyEventTimelineView — 双带布局与引线（tasks 2.1 / 2.2 / 2.4 / 2.5）', () => {
  it('4 层标签行高 = 178px（50 + 32 × 层数；双带行高公式，未触 246 封顶）', async () => {
    // 同日 4 条 = 一个组、占 4 层（上/下带合计 4 层 ≤ 上限 → 不收起）
    eventsState.events = [
      event('a', '93', '2026-11-12', '第一条'),
      event('b', '93', '2026-11-12', '第二条'),
      event('c', '93', '2026-11-12', '第三条'),
      event('d', '93', '2026-11-12', '第四条'),
    ]
    const wrapper = mountView()
    await settle()
    const body = wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-body').element as HTMLElement
    expect(Number.parseFloat(body.style.height)).toBe(178)
    expect(wrapper.findAllComponents(KeyEventLabel), '4 层 ≤ 上限 → 标签照常显示').toHaveLength(4)
  })

  it('轴带随上带层数下移：上带 1 层 → 轴线 top = 8 + 8 + 32 × 1 + 9 = 57px', async () => {
    // 两个相邻日期 + 宽标签（400px）→ 第二组压不下下带第 0 层 → 改走上带（上带 1 层）
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
      configurable: true, get: () => 400,
    })
    eventsState.events = [
      event('a', '93', '2026-11-10', '宽标题一'),
      event('b', '93', '2026-11-11', '宽标题二'),
    ]
    const wrapper = mountView()
    await settle()
    const row = wrapper.find('[data-testid="key-event-row-93"]')
    const axisLine = row.find('.kev-axis-line').element as HTMLElement
    // axisTop = 8+8+32*1 = 48; lifeline = 48+9 = 57
    expect(Number.parseFloat(axisLine.style.top), '生命线 = axisTop(8 + 8 + 32 × La) + 9').toBe(57)
    const node = row.find('[data-testid="key-event-node-93-2026-11-10"]').element as HTMLElement
    // 节点中心骑生命线（57）→ 上沿 = 生命线 − 半径 9 = 48 = axisTop
    expect(Number.parseFloat(node.style.top)).toBe(48)
  })

  it('每条标签恰有一根引线 path，起点 x = 该事件圆点 x', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-10', '上带标题'),
      event('b', '93', '2026-11-11', '下带标题'),
    ]
    const wrapper = mountView()
    await settle()
    const row = wrapper.find('[data-testid="key-event-row-93"]')
    const paths = row.findAll('path.kev-lead')
    expect(paths.length, '每条标签恰有一根引线（MUST NOT 出现无引线的标签）')
      .toBe(row.findAllComponents(KeyEventLabel).length)
    // 圆点 x = 列中心（列宽 = max(64, (1600 − 192) / 5)）
    const col = (1600 - 192) / 5
    const dotX = (index: number) => index * col + col / 2
    const startX = (d: string) => Number.parseFloat(d.replace(/^M\s*/, '').split(' ')[0] ?? '')
    const byEvent = new Map(paths.map(path => [path.attributes('data-ev'), startX(path.attributes('d') ?? '')]))
    expect(byEvent.get('a')).toBeCloseTo(dotX(0), 3)
    expect(byEvent.get('b')).toBeCloseTo(dotX(1), 3)
  })

  it('同日多条节点横向散开 24px（节点恒在生命线上，不竖向下压）', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12', '第一条'),
      event('b', '93', '2026-11-12', '第二条'),
      event('c', '93', '2026-11-12', '第三条'),
    ]
    const wrapper = mountView()
    await settle()
    const nodes = wrapper.findAll('[data-testid="key-event-node-93-2026-11-12"]')
    const transforms = nodes.map(node => (node.element as HTMLElement).style.transform)
    expect(transforms).toContain('translateX(calc(-50% + -24px))')
    expect(transforms).toContain('translateX(calc(-50% + 0px))')
    expect(transforms).toContain('translateX(calc(-50% + 24px))')
    // 散开是横向的：三个节点同高（骑在同一段生命线上）
    const tops = nodes.map(node => (node.element as HTMLElement).style.top)
    expect(new Set(tops).size, '节点 MUST NOT 竖向溢出生命线').toBe(1)
  })

  it('收起行的节点同样骑在生命线上（不竖向叠放下压）', async () => {
    // 行 93：同日 5 条 → 合计 5 层 > 4 → 收起为「圆点 + ★5」；行 95：1 条 → 正常展开
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
    const denseNodes = wrapper.findAll(`[data-testid="key-event-node-93-${longAxis[10]}"]`)
    // 旧值 5 → 新值 1：64px 窄列装不下 5 个节点 → review R-22 降级为单节点 + 计数。
    expect(denseNodes, '一列装不下 → 该日降级为单个节点').toHaveLength(1)
    const denseTops = denseNodes.map(node => (node.element as HTMLElement).style.top)
    expect(new Set(denseTops).size, '收起行的节点不得竖向叠放').toBe(1)
    const looseNode = wrapper.find(`[data-testid="key-event-node-95-${longAxis[0]}"]`).element as HTMLElement
    expect(looseNode.style.top, '收起行节点 top 与同层非收起行一致').toBe(denseTops[0])
  })

  it('节点用事件色；选中日的节点呈实心（is-selected class）', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12', '选中日的事件'),
      event('b', '93', '2026-11-13', '未选中日的事件'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const selected = wrapper.find('[data-testid="key-event-node-93-2026-11-12"]')
    expect(selected.classes(), '选中日的节点呈实心').toContain('is-selected')
    const other = wrapper.find('[data-testid="key-event-node-93-2026-11-13"]')
    expect(other.classes()).not.toContain('is-selected')
  })

  it('节点与引线走 --color-event、生命线走 --color-text-muted（MUST NOT 硬编码色值）', async () => {
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const source = readFileSync(
      resolve(process.cwd(), 'app/features/tags/components/console/KeyEventTimelineView.vue'),
      'utf-8',
    )
    const ruleOf = (selector: string) => {
      const start = source.indexOf(`${selector} {`)
      expect(start, `源码里应有 ${selector} 规则`).toBeGreaterThan(-1)
      return source.slice(start, source.indexOf('}', start))
    }
    expect(ruleOf('.kev-axis-line')).toContain('var(--color-text-muted)')
    expect(ruleOf('.kev-axis-line')).not.toContain('--color-border-medium')
    expect(ruleOf('.kev-lead')).toContain('var(--color-event)')
    expect(ruleOf('.kev-node')).toContain('var(--color-event)')
  })

  it('悬停强调不改变选中态：引线 / 节点 / 轴头日期加重，selectedDate 不动', async () => {
    eventsState.events = [event('a', '93', '2026-11-12', '悬停目标')]
    const wrapper = mountView()
    await settle()
    await wrapper.find('[data-testid="key-event-label-a"]').trigger('mouseenter')
    expect(wrapper.find('path.kev-lead.is-hovered').exists(), '该事件的引线加重').toBe(true)
    expect(wrapper.find('[data-testid="key-event-node-93-2026-11-12"]').classes()).toContain('is-hovered')
    expect(wrapper.find('[data-testid="key-event-day-2026-11-12"]').classes()).toContain('is-hovered')
    expect(wrapper.emitted('selectDate'), '悬停 MUST NOT 改选中日').toBeUndefined()
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

  it('切换视图保留选中态：props 的 selectedDate 不因渲染而重置（受控契约）；行头不再渲染 selectedLane 选中态', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedLaneId: '93', selectedDate: '2026-11-12' })
    await settle()
    // 行头仍是旧契约的一部分（父组件照传 selectedLaneId），但本视图不据此渲染任何选中视觉。
    expect(wrapper.find('[data-testid="key-event-rowhead-93"]').classes(), '行头 MUST NOT 带 is-selected（2026-11-24 用户反馈：假选中易误解）')
      .not.toContain('is-selected')
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
    // 同日两条 = 一个组、占 2 层 → 行高 = 50 + 32 × 2 = 114px（行高必须随层数增高）
    expect(Number.parseFloat(body.style.height)).toBe(114)
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
    // 选中日取轴尾附近：行头移出滚动区后（R-33）居中口径 = 用滚动区自身宽度，靠右的日期才需要滚
    const wrapper = mountView({ selectedDate: '2026-11-28' })
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
      // 选中日取轴尾附近（R-33 后居中口径不含行头，靠右的日期才需要滚回来）
      const wrapper = mountView({ selectedDate: '2026-11-28' })
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

// ── 视图头 / 行头读数 + 休眠自动展开（tasks 4.1–4.3 / 5.1–5.2）──────────────

describe('KeyEventTimelineView — 视图头读数（tasks 4.1 / 4.3）', () => {
  it('「所属日期」下拉：选项 = 轴全部日期、无「不聚焦」项；文案用「MM/DD 周X」口径', async () => {
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const select = wrapper.find('[data-testid="key-event-date-select"]')
    expect(select.exists(), '视图头应有「所属日期」下拉').toBe(true)
    expect(select.attributes('aria-label'), '下拉必须有可访问名称').toBe('所属日期')
    const options = select.findAll('option')
    expect(options, '选项数 = 轴长（无「不聚焦」项）').toHaveLength(AXIS.length)
    expect(options.map(o => o.attributes('value'))).toEqual(AXIS)
    expect(options.map(o => o.text())).toEqual([
      '11/10 周二', '11/11 周三', '11/12 周四', '11/13 周五', '11/14 周六',
    ])
    expect((select.element as HTMLSelectElement).value, '下拉值跟随共用 selectedDate').toBe('2026-11-12')
  })

  it('下拉选某日 → emit selectDate 且参数为该日期（不新建选中态）', async () => {
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const select = wrapper.find('[data-testid="key-event-date-select"]')
    await select.setValue('2026-11-14')
    expect(wrapper.emitted('selectDate')).toEqual([['2026-11-14']])
    // 视图不存第二份选中日：未收到新 props 时不会自行切换到新日期
    expect(wrapper.emitted('selectLane')).toBeUndefined()
  })

  it('视图头显示选中日在全部泳道的当日条数', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12'),
      event('b', '95', '2026-11-12'),
      event('c', '97', '2026-11-12'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-selected-count"]').text()).toBe('11/12 · 3 条')
  })

  it('选中日全泳道 0 条 → 轴上方一行说明；MUST NOT 代替视图头「MM/DD · 0 条」', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-13' })
    await settle()
    const empty = wrapper.find('[data-testid="key-event-selected-day-empty"]')
    expect(empty.exists(), '选中日无事件应有一行说明').toBe(true)
    expect(empty.text()).toBe('11/13 没有确认的关键事件')
    expect(wrapper.find('[data-testid="key-event-selected-count"]').text(), '视图头读数仍在（0 条不被说明行代揣）')
      .toBe('11/13 · 0 条')
    // 说明行在轴**上方**（舞台之外：横滚只动列，提示行不跟着横移，R-33）
    const stage = wrapper.find('[data-testid="key-event-stage"]').element as HTMLElement
    expect(stage.previousElementSibling?.getAttribute('data-testid')).toBe('key-event-selected-day-empty')
  })

  it('选中日有事件时不出说明行；读取中 / 读取失败也不出（MUST NOT 冒称 0 条）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-selected-day-empty"]').exists()).toBe(false)

    eventsState.events = []
    eventsState.loading = true
    const loading = mountView({ selectedDate: '2026-11-13' })
    await settle()
    expect(loading.find('[data-testid="key-event-selected-day-empty"]').exists(), '读取中不判 0 条').toBe(false)

    eventsState.loading = false
    eventsState.error = 'boom'
    const failed = mountView({ selectedDate: '2026-11-13' })
    await settle()
    expect(failed.find('[data-testid="key-event-selected-day-empty"]').exists(), '读取失败不判 0 条').toBe(false)
  })

  it('无持久化话题的空态下不叠「选中日无事件」说明行（各自空态不叠加）', async () => {
    const wrapper = mountView({ selectedDate: '2026-11-13', activeRows: [], dormantRows: [], emptyState: 'no-lanes' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-empty-no-lanes"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-selected-day-empty"]').exists()).toBe(false)
  })

  it('视图头居中按钮的 title 改为「所选日居中」（阅读区退役后不再指向下方）', async () => {
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const goto = wrapper.find('[data-testid="key-event-goto-selected"]')
    expect(goto.exists()).toBe(true)
    expect(goto.attributes('title')).toBe('所选日居中')
  })
})

describe('KeyEventTimelineView — 行头「当日 k 条」（tasks 4.2）', () => {
  it('选中日某泳道 2 条 → 行头含「当日 2 条」；无事件行显示「当日 0 条」', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12'),
      event('b', '93', '2026-11-12'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-rowday-93"]').text()).toBe('当日 2 条')
    expect(wrapper.find('[data-testid="key-event-rowday-95"]').text()).toBe('当日 0 条')
  })

  it('休眠行的行头同样带「当日 k 条」', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: true })
    await settle()
    expect(wrapper.find('[data-testid="key-event-rowday-97"]').text()).toBe('当日 1 条')
  })
})

describe('KeyEventTimelineView — 下方阅读区已退役（tasks 5.1）', () => {
  it('视图下方不再渲染「当日关键事件」阅读区，能力由行内卡片承接', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-day-panel"]').exists(), '旧阅读区已删除').toBe(false)
    expect(wrapper.find('[data-testid="key-event-day-card-a"]').exists(), '能力去向 = 行内卡片').toBe(true)
  })

  it('源码不再 import 已删除的旧面板（渲染层零引用）', async () => {
    const source = await viewSource()
    // 组件名拼接：本文件不直接写出旧面板文件名（文件名在仓库里已不存在）
    const removed = ['KeyEvent', 'DayPanel'].join('')
    expect(source).not.toContain(removed)
  })

  it('下拉有可见焦点样式（MUST NOT 只靠 hover 反馈）', async () => {
    const source = await viewSource()
    expect(source).toContain('.kev-select select:focus-visible')
  })
})

describe('KeyEventTimelineView — 休眠泳道选中日自动展开（tasks 5.2 / design D9）', () => {
  it('选中日命中休眠行 → 休眠行自动出现且带卡片列', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: false })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '命中日应自动展开该行').toBe(true)
    expect(wrapper.find('[data-testid="key-event-cards-97"]').exists(), '自动展开的行同样有卡片列').toBe(true)
    expect(wrapper.find('[data-testid="key-event-day-card-d"]').exists()).toBe(true)
  })

  it('换选无事件日 → 休眠行收起消失', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: false })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(true)
    await wrapper.setProps({ selectedDate: '2026-11-13' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '无事件日不自动展开').toBe(false)
  })

  it('用户显式折叠优先：点折叠按钮后即使选中日有事件也不再自动展开，直到选中日再变', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: false })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(true)
    await wrapper.find('[data-testid="key-event-dormant-toggle"]').find('button').trigger('click')
    // 旧值 1 → 新值 0（review R-27）：本地自动展开态点「收起」只清本地状态，
    // MUST NOT emit toggleDormant（emit 会把壳层共用值翻成 true → 全部休眠行弹开，
    // 且矩阵视图被同步打开）。
    expect(wrapper.emitted('toggleDormant'), '本地自动展开态点收起 MUST NOT emit toggleDormant').toBeUndefined()
    // 显式折叠：即使同一选中日、事件数据重到，也不再自动展开
    await wrapper.setProps({ expandedDormant: false })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '用户折叠后 MUST NOT 自动展开').toBe(false)
    // 选中日再变 → 自动展开规则重新生效（该日仍有事件）
    await wrapper.setProps({ selectedDate: '2026-11-13' })
    await wrapper.setProps({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '选中日再变后重新自动展开').toBe(true)
  })

  it('壳层 expandedDormant 为真 → 全部休眠行（不只当日有事件的那几条）', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const dormantRows = [row('97', '休眠线 A', true), row('98', '休眠线 B', true)]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: true, dormantRows })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-row-98"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-cards-98"]').exists(), '无当日事件的行没有卡片列').toBe(false)
  })

  it('自动展开时休眠组标题如实反映展开态（aria-expanded / 收起文案）', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: false })
    await settle()
    const toggle = wrapper.find('[data-testid="key-event-dormant-toggle"]').find('button')
    expect(toggle.attributes('aria-expanded'), '行已自动展开 → aria-expanded 不得再说 false').toBe('true')
    expect(toggle.text()).toContain('收起')
    expect(toggle.text(), '休眠组标题带「当日 M 条」').toContain('当日 1 条')
  })

  it('MUST NOT 写壳层共用 expandedDormant：自动展开不 emit toggleDormant', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: false })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(true)
    expect(wrapper.emitted('toggleDormant'), '自动展开是视图内本地状态').toBeUndefined()
  })

  it('休眠行集合晚于选中日到位也自动展开（数据到位后派生）', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: false, dormantRows: [] })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(false)
    // 行集合（A 档）晚于事件与选中日到位 → 派生量变化 → 自动展开
    await wrapper.setProps({ dormantRows: [row('97', '休眠线', true)] })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '数据到位后应自动展开').toBe(true)
  })
})

// ── 日聚焦卡片列（key-event-day-focus-cards 3.2 / 3.3）───────────────────────

describe('KeyEventTimelineView — 日聚焦卡片列（tasks 3.2 / 3.3）', () => {
  /** 单卡高约 116px（紧凑形态）；happy-dom 无布局引擎，按固定值桩掉。 */
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
      configurable: true, get: () => 116,
    })
  })

  afterEach(() => {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetHeight
  })

  it('选中日在所有有该日事件的泳道行内展开卡片列（MUST NOT 只展开当前选中的一行）', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12'),
      event('b', '95', '2026-11-12'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-cards-93"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-cards-95"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-day-card-a"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-day-card-b"]').exists()).toBe(true)
  })

  it('该日无事件的泳道不出现卡片列', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-cards-93"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="key-event-cards-95"]').exists()).toBe(false)
  })

  it('选中日不出药丸标签（该日名称由日聚焦卡片承接）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    expect(wrapper.find('[data-testid="key-event-label-a"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="key-event-day-card-a"]').text()).toContain('事件a')
  })

  it('卡片矩形作为归带函数的第 3 参传入（下带标签因此避开卡片列）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    mountView({ selectedDate: '2026-11-12' })
    await settle()
    // 该行在选中日有事件 → 卡片列存在 → 卡片矩形照传（review R-23：按行传，不是不分行的全局值）。
    const call = bandCallFor('a')
    expect(call, '归带函数未被调用').toBeDefined()
    const rect = call![2] as { left: number, right: number } | null
    expect(rect, '卡片矩形 MUST NOT 传 null').not.toBeNull()
    // 列宽 = (1600 − 192) / 5 = 281.6；选中列 = 第 3 列（index 2）
    expect(rect!.left).toBeCloseTo(2 * 281.6, 1)
    expect(rect!.right).toBeCloseTo(2 * 281.6 + 280, 1)
    expect(call![3], '选中日作为第 4 参传入：该日不出药丸').toBe('2026-11-12')
  })

  it('卡片列宽固定 280px（行体窄于卡片宽时也不缩卡片）', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 400 })
    eventsState.events = [event('a', '93', '2026-11-14')]
    const wrapper = mountView({ selectedDate: '2026-11-14' })
    await settle()
    const style = wrapper.find('[data-testid="key-event-cards-93"]').attributes('style') ?? ''
    expect(style).toContain('width: 280px')
  })

  it('靠右时卡片列向左展开、右缘不超过行体宽（MUST NOT 伸出舞台）', async () => {
    // 窄舞台 → 列宽落 64px 下限 → 行体宽 = 5 × 64 = 320（装不下 280px 卡片 → 翻转）
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 400 })
    eventsState.events = [event('a', '93', '2026-11-14')]
    const wrapper = mountView({ selectedDate: '2026-11-14' })
    await settle()
    const style = wrapper.find('[data-testid="key-event-cards-93"]').attributes('style') ?? ''
    const left = Number(/left:\s*([\d.]+)px/.exec(style)?.[1] ?? Number.NaN)
    expect(Number.isNaN(left)).toBe(false)
    expect(left + 280).toBeLessThanOrEqual(320)
  })

  it('≤ 4 张卡片全部展开：不设固定高度，该行不因 246px 封顶而截断卡片', async () => {
    eventsState.events = [
      event('a', '93', '2026-11-12'),
      event('b', '93', '2026-11-12'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const scroll = wrapper.find('[data-testid="key-event-cards-scroll-93"]')
    expect(scroll.exists()).toBe(true)
    expect(scroll.attributes('style') ?? '', '≤ 4 张时 MUST NOT 固定高度').not.toContain('height')
    expect(scroll.find('[data-testid="key-event-day-card-a"]').exists()).toBe(true)
    expect(scroll.find('[data-testid="key-event-day-card-b"]').exists()).toBe(true)
    // 行高 = 卡片列占位高（轴带下缘 34 + 8 + 堆叠高 240 + 8 = 290），不受 246 封顶
    const bodyStyle = wrapper.find('[data-testid="key-event-row-93"] .kev-row-body').attributes('style') ?? ''
    const rowHeight = Number(/height:\s*([\d.]+)px/.exec(bodyStyle)?.[1] ?? 0)
    expect(rowHeight).toBe(290)
    expect(rowHeight).toBeGreaterThan(246)
  })

  it('超过 4 张：卡片列固定高度内部滚动并标「还有 N 条」', async () => {
    eventsState.events = [
      event('a1', '93', '2026-11-12'),
      event('a2', '93', '2026-11-12'),
      event('a3', '93', '2026-11-12'),
      event('a4', '93', '2026-11-12'),
      event('a5', '93', '2026-11-12'),
      event('a6', '93', '2026-11-12'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const scroll = wrapper.find('[data-testid="key-event-cards-scroll-93"]')
    expect(scroll.classes()).toContain('is-scrollable')
    expect(scroll.classes(), '内部滚动容器 MUST 挂 .scroll-thin').toContain('scroll-thin')
    // 固定高度 = 前 4 张实测高之和 + 间距 × 3 = 4 × 116 + 24 = 488
    expect(scroll.attributes('style') ?? '').toContain('height: 488px')
    expect(wrapper.find('[data-testid="key-event-cards-more-93"]').text()).toContain('还有 2 条')
  })

  it('点卡片内文章 → openArticle 转发给壳层（复用既有阅读链路）', async () => {
    const reading = {
      readRelated: vi.fn(() => ({
        status: 'success' as const,
        meta: { articleId: '101', title: '文章甲', pubDate: '2026-11-12T08:00:00Z', read: false, article: {} },
      })),
      ensureRelatedArticles: vi.fn(async () => {}),
      retryRelatedArticle: vi.fn(async () => {}),
    } as unknown as LaneDayReadingStore
    eventsState.events = [{ ...event('a', '93', '2026-11-12'), articleIds: ['101'] }]
    const wrapper = mountView({ selectedDate: '2026-11-12', reading })
    await settle()
    await wrapper.find('[data-testid="key-event-day-card-articles-a"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="lane-chronicle-article-101"]').trigger('click')
    expect(wrapper.emitted('openArticle')).toEqual([['101']])
  })
})

// ── M2 遗留接线：归带几何无解 → 该行收起（MUST NOT 把未校验的层当可用）───────

describe('KeyEventTimelineView — 归带几何无解时该行收起', () => {
  /** 只覆盖含指定事件键的那一行（其余行走真实归带）。 */
  function overrideFor(
    key: string,
    layoutExtra: Record<string, unknown>,
    placementExtra: Record<string, unknown> = {},
  ): void {
    bandLayout.override = (groups) => {
      const keys = groups.flatMap(group => group.items.map(item => item.key))
      if (!keys.includes(key)) return null
      return {
        placements: [{ key, band: 'down', layer: 0, left: 100, width: 110, dotX: 150, ...placementExtra }],
        upLayers: 1,
        downLayers: 1,
        ...layoutExtra,
      }
    }
  }

  it('归带溢出（overflowed）→ 该行收起：节点 + ★N，不画标签与引线', async () => {
    overrideFor('a', { overflowed: true })
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-11' })
    await settle()
    const row = wrapper.find('[data-testid="key-event-row-93"]')
    expect(row.find('[data-testid="key-event-label-a"]').exists(), '溢出行 MUST NOT 画标签').toBe(false)
    expect(row.findAll('.kev-lead'), '溢出行 MUST NOT 画引线').toHaveLength(0)
    expect(row.find('[data-testid="key-event-node-93-2026-11-12"]').text()).toContain('★1')
  })

  it('组内引线穿过组内标签（crossesGroupLabel）→ 该行收起', async () => {
    overrideFor('a', {}, { crossesGroupLabel: true })
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-11' })
    await settle()
    const row = wrapper.find('[data-testid="key-event-row-93"]')
    expect(row.find('[data-testid="key-event-label-a"]').exists(), '穿标签行 MUST NOT 画标签').toBe(false)
    expect(row.findAll('.kev-lead'), '穿标签行 MUST NOT 画引线').toHaveLength(0)
    expect(row.find('[data-testid="key-event-node-93-2026-11-12"]').text()).toContain('★1')
  })
})

// ── 实现期第二轮审查整改（R-20 ~ R-25）───────────────────────────────────────

describe('KeyEventTimelineView — R-20 事件色与选中色分离', () => {
  it('选中节点的实心填充走 --color-accent（事件色只留描边 / 引线 / 卡片日期）', async () => {
    const source = await viewSource()
    const start = source.indexOf('.kev-node.is-selected {')
    expect(start, '未找到 .kev-node.is-selected 规则块').toBeGreaterThan(-1)
    const block = source.slice(start, source.indexOf('}', start))
    expect(block, '选中节点实心填充 MUST 用强调色（红 = 选中的那天）').toContain('background: var(--color-accent)')
    expect(block, '选中节点 MUST NOT 用事件色实心填充（同一颜色表达两义）').not.toContain('var(--color-event)')
  })

  it('未选中节点仍是事件色描边 + 空心（琥珀 = 事件）', async () => {
    const source = await viewSource()
    const start = source.indexOf('.kev-node {')
    expect(start, '未找到 .kev-node 规则块').toBeGreaterThan(-1)
    const block = source.slice(start, source.indexOf('}', start))
    expect(block, '节点描边走事件色').toContain('border: 1px solid var(--color-event)')
    expect(block, '节点底色走 elevated（空心）').toContain('background: var(--color-bg-elevated)')
  })
})

describe('KeyEventTimelineView — R-21 收起行的独立轴带位置', () => {
  it('收起行按 La = Lb = 0 定位：节点顶落在本行内且与生命线一致', async () => {
    // 无卡片行：同日 9 条先落下带（9 层），相邻同日 8 条被上带让开（8 层）
    // → 合计 17 层 > 4 → 收起。收起前轴带在 16 + 32×8 = 272px（节点跑出 246px 行高）。
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 })
    const axis = ['2026-11-10', '2026-11-11', '2026-11-12']
    eventsState.events = [
      ...Array.from({ length: 9 }, (_, i) => event(`a${i}`, '93', '2026-11-10', `甲${i}`)),
      ...Array.from({ length: 8 }, (_, i) => event(`b${i}`, '93', '2026-11-11', `乙${i}`)),
    ]
    const wrapper = mountView({ axis })
    await settle()

    const row = wrapper.find('[data-testid="key-event-row-93"]')
    expect(row.findAllComponents(KeyEventLabel), '层数超上限 → 该行收起（无标签）').toHaveLength(0)
    const rowHeight = Number.parseFloat((row.find('.kev-row-body').element as HTMLElement).style.height)
    const lifeline = Number.parseFloat((row.find('.kev-axis-line').element as HTMLElement).style.top)
    const nodeTop = Number.parseFloat(
      (row.find('[data-testid="key-event-node-93-2026-11-10"]').element as HTMLElement).style.top,
    )
    expect(lifeline, '收起行的轴带 MUST 贴行顶（La = Lb = 0）').toBe(25)
    expect(nodeTop, '节点顶 MUST 落在本行内 [0, 行高)').toBeGreaterThanOrEqual(0)
    expect(nodeTop, '节点顶 MUST 落在本行内 [0, 行高)').toBeLessThan(rowHeight)
    expect(nodeTop + 9, '节点 MUST 恒在生命线上').toBe(lifeline)
  })
})

describe('KeyEventTimelineView — R-22 窄列下节点互压', () => {
  it('64px 列宽 + 相邻两天各 3 条：该行已收起（每天一个节点），相邻日节点矩形不相交且各自在本列内', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 })
    eventsState.events = [
      event('a1', '93', '2026-11-12', '第一天第一条'),
      event('a2', '93', '2026-11-12', '第一天第二条'),
      event('a3', '93', '2026-11-12', '第一天第三条'),
      event('b1', '93', '2026-11-13', '第二天第一条'),
      event('b2', '93', '2026-11-13', '第二天第二条'),
      event('b3', '93', '2026-11-13', '第二天第三条'),
    ]
    const wrapper = mountView()
    await settle()

    const col = Number.parseFloat(
      (wrapper.find('.kev-day-heads').element as HTMLElement).style.getPropertyValue('--kev-col'),
    )
    expect(col, '窄舞台 → 列宽钉在 64px 下限').toBe(64)

    const first = nodeRects(wrapper, '93', '2026-11-12', AXIS)
    const second = nodeRects(wrapper, '93', '2026-11-13', AXIS)
    // 旧值 3 → 新值 1：两组各 3 条 = 6 层 > 4 层上限 → 该行收起；review R-30 规定
    // 收起行**每天只出一个节点**（承载「★N」计数盒），故不再逐事件扇开。
    expect(first, '收起行：第一天只出一个节点').toHaveLength(1)
    expect(second, '收起行：第二天只出一个节点').toHaveLength(1)
    expect(first[0]!.right - first[0]!.left, '带「MM.DD ★N」计数盒的节点按实测盒宽算（> 18px）')
      .toBe(countBoxWidth(wrapper.find('[data-testid="key-event-node-93-2026-11-12"]').text()))
    expect(first[0]!.right - first[0]!.left, '收起节点盒宽 MUST ≤ 列宽（64px 下限）、不截文字')
      .toBeLessThanOrEqual(col)

    // 相邻日节点矩形不相交（按实测盒宽：「MM.DD ★N」计数盒 50px > 18px）
    expect(
      Math.max(...first.map(r => r.right)),
      '相邻日节点 MUST NOT 互压（圆心距 < 节点宽就会压盖）',
    ).toBeLessThanOrEqual(Math.min(...second.map(r => r.left)))
    // 每个节点都在本列内（MUST NOT 溢出到邻列）
    for (const rect of first) {
      expect(rect.left).toBeGreaterThanOrEqual(2 * col)
      expect(rect.right).toBeLessThanOrEqual(3 * col)
    }
    for (const rect of second) {
      expect(rect.left).toBeGreaterThanOrEqual(3 * col)
      expect(rect.right).toBeLessThanOrEqual(4 * col)
    }
  })

  it('64px 列宽 + 相邻两天各 2 条（未收起）：同日扇开的节点互不相交，且各自在本列内', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 })
    eventsState.events = [
      event('a1', '93', '2026-11-12', '第一天第一条'),
      event('a2', '93', '2026-11-12', '第一天第二条'),
      event('b1', '93', '2026-11-13', '第二天第一条'),
      event('b2', '93', '2026-11-13', '第二天第二条'),
    ]
    const wrapper = mountView()
    await settle()

    const col = Number.parseFloat(
      (wrapper.find('.kev-day-heads').element as HTMLElement).style.getPropertyValue('--kev-col'),
    )
    expect(col, '窄舞台 → 列宽钉在 64px 下限').toBe(64)

    const first = nodeRects(wrapper, '93', '2026-11-12', AXIS)
    const second = nodeRects(wrapper, '93', '2026-11-13', AXIS)
    // 2 组各 2 条 = 4 层 = 层数上限 → 该行**未收起**（扇开路径仍然有效）
    expect(first, '未收起行：同日扇开为 2 个节点').toHaveLength(2)
    expect(second, '未收起行：同日扇开为 2 个节点').toHaveLength(2)

    // 同日节点互不相交
    for (let i = 1; i < first.length; i += 1) {
      expect(first[i]!.left, '同日节点 MUST NOT 压盖').toBeGreaterThanOrEqual(first[i - 1]!.right)
    }
    // 相邻日节点矩形不相交（最近的一对）
    expect(
      Math.max(...first.map(r => r.right)),
      '相邻日节点 MUST NOT 互压（圆心距 < 节点宽就会压盖）',
    ).toBeLessThanOrEqual(Math.min(...second.map(r => r.left)))
    // 每个节点都在本列内（MUST NOT 溢出到邻列）
    for (const rect of first) {
      expect(rect.left).toBeGreaterThanOrEqual(2 * col)
      expect(rect.right).toBeLessThanOrEqual(3 * col)
    }
    for (const rect of second) {
      expect(rect.left).toBeGreaterThanOrEqual(3 * col)
      expect(rect.right).toBeLessThanOrEqual(4 * col)
    }
  })

  it('一列装不下（64px 列宽 + 同日 4 条）→ 降级为单节点 + 当日计数', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 })
    eventsState.events = Array.from({ length: 4 }, (_, i) => event(`d${i}`, '93', '2026-11-12', `第${i}条`))
    const wrapper = mountView()
    await settle()
    const nodes = wrapper.findAll('[data-testid="key-event-node-93-2026-11-12"]')
    expect(nodes, '一列装不下 → 该日 MUST 降级为单个节点').toHaveLength(1)
    expect(nodes[0]!.text(), '降级后呈现当日计数').toContain('★4')
    // 降级不得靠竖向叠放实现：节点仍骑在生命线上
    const row = wrapper.find('[data-testid="key-event-row-93"]')
    const lifeline = Number.parseFloat((row.find('.kev-axis-line').element as HTMLElement).style.top)
    expect(Number.parseFloat((nodes[0]!.element as HTMLElement).style.top) + 9).toBe(lifeline)
  })
})

describe('KeyEventTimelineView — R-23 卡片矩形按行传入', () => {
  it('没有卡片列的行：卡片矩形传 null，下带照常可用（不被空矩形挤到上带）', async () => {
    // 宽标签（400px）横向跨过卡片矩形所在区间 → 有卡片列的行改走上带。
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', { configurable: true, get: () => 400 })
    eventsState.events = [
      event('cards', '93', '2026-11-12'),       // 选中日 → 行 93 有卡片列
      event('a', '93', '2026-11-11', '甲行标签'),
      event('b', '95', '2026-11-11', '乙行标签'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    expect(bandCallFor('cards')![2], '有卡片列的行：传卡片矩形').not.toBeNull()
    expect(bandCallFor('b')![2], '没有卡片列的行 MUST 传 null（不拿空矩形挡下带）').toBeNull()

    const row93 = wrapper.find('[data-testid="key-event-row-93"]')
    const row95 = wrapper.find('[data-testid="key-event-row-95"]')
    const lifeline93 = Number.parseFloat((row93.find('.kev-axis-line').element as HTMLElement).style.top)
    const lifeline95 = Number.parseFloat((row95.find('.kev-axis-line').element as HTMLElement).style.top)
    const top93 = Number.parseFloat((row93.find('[data-testid="key-event-label-a"]').element as HTMLElement).style.top)
    const top95 = Number.parseFloat((row95.find('[data-testid="key-event-label-b"]').element as HTMLElement).style.top)
    expect(top93, '与卡片矩形相交的组改走上带').toBeLessThan(lifeline93)
    expect(top95, '没有卡片列的行：下带照常可用').toBeGreaterThan(lifeline95)
  })
})

describe('KeyEventTimelineView — R-24 休眠行弱化不得压垮事件色', () => {
  it('休眠行容器不再有 opacity 规则；事件色相关元素不受透明度影响', async () => {
    const source = await viewSource()
    const style = source.slice(source.indexOf('<style scoped>'))
    const rules = [...style.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
      .map(match => ({ selector: match[1]!.trim(), body: match[2]! }))

    const rowOpacity = rules.filter(rule => rule.selector.includes('.kev-row') && rule.body.includes('opacity'))
    expect(rowOpacity.map(rule => rule.selector), '休眠行容器 MUST NOT 有 opacity 规则').toEqual([])

    const eventOpacity = rules.filter(rule =>
      /kev-node|kev-lead|kev-cards|kev-cell|kev-axis-line/.test(rule.selector) && rule.body.includes('opacity'),
    )
    expect(eventOpacity.map(rule => rule.selector), '事件色相关元素 MUST NOT 受透明度影响').toEqual([])

    // 弱化只落在行头文字（次要信息），走语义 token
    // 行头已移入独立行头列（R-33）→ 弱化规则挂在 `.kev-head-cell.is-dormant` 上
    const dormant = rules.filter(rule => rule.selector.includes('.kev-head-cell.is-dormant'))
    expect(dormant.length, '休眠行应有专门的弱化规则').toBeGreaterThan(0)
    expect(
      dormant.some(rule => rule.body.includes('--color-text-muted')),
      '弱化走 --color-text-muted（MUST NOT 硬编码色值）',
    ).toBe(true)
  })
})

describe('KeyEventTimelineView — R-25 行头跟随生命线', () => {
  it('行头内容块中心对齐生命线（不再整行垂直居中）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView()
    await settle()
    const row = wrapper.find('[data-testid="key-event-row-93"]')
    const head = wrapper.find('[data-testid="key-event-rowhead-93"]')
    const lifeline = Number.parseFloat((row.find('.kev-axis-line').element as HTMLElement).style.top)
    const inner = head.find('.kev-row-head-inner').element as HTMLElement
    expect(Number.parseFloat(inner.style.top), '行头内容块中心 MUST 对齐生命线').toBe(lifeline)
  })

  it('源码契约：行头内容用 translateY(-50%) 拉回自身半高，行头不再 justify-content: center', async () => {
    const source = await viewSource()
    const headStart = source.indexOf('.kev-row-head {')
    expect(headStart, '未找到 .kev-row-head 规则块').toBeGreaterThan(-1)
    const headBlock = source.slice(headStart, source.indexOf('}', headStart))
    expect(headBlock, '行头 MUST NOT 整行垂直居中（会与生命线错位）').not.toContain('justify-content')

    const innerStart = source.indexOf('.kev-row-head-inner {')
    expect(innerStart, '未找到 .kev-row-head-inner 规则块').toBeGreaterThan(-1)
    const innerBlock = source.slice(innerStart, source.indexOf('}', innerStart))
    expect(innerBlock, '行头内容块 MUST 用 translateY(-50%) 对齐生命线').toContain('translateY(-50%)')
    expect(innerBlock, '行头内容块 MUST 绝对定位（top 由生命线注入）').toContain('position: absolute')
  })
})

// ── 实现期第三轮审查整改（R-27 ~ R-30）───────────────────────────────────────

describe('KeyEventTimelineView — R-27 折叠按钮的两条路径', () => {
  it('本地自动展开态点「收起」→ 只清本地状态，MUST NOT emit toggleDormant（不污染矩阵共用状态）', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: false })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '命中日应自动展开该行').toBe(true)

    await wrapper.find('[data-testid="key-event-dormant-toggle"]').find('button').trigger('click')
    await settle()

    expect(
      wrapper.emitted('toggleDormant'),
      '本地自动展开态 MUST NOT emit（emit 会把壳层共用值翻成 true → 全部休眠行弹开 + 矩阵视图被打开）',
    ).toBeUndefined()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '点收起后自动展开的行消失').toBe(false)
  })

  it('壳层已展开时点「收起」→ emit 一次 toggleDormant（真收起全部休眠行）', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12', expandedDormant: true })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists()).toBe(true)

    await wrapper.find('[data-testid="key-event-dormant-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
  })

  it('折叠状态下点「展开查看」→ emit 一次 toggleDormant（交壳层展开全部休眠行）', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-13', expandedDormant: false })
    await settle()
    expect(wrapper.find('[data-testid="key-event-row-97"]').exists(), '无事件日不自动展开').toBe(false)

    await wrapper.find('[data-testid="key-event-dormant-toggle"]').find('button').trigger('click')
    expect(wrapper.emitted('toggleDormant')).toHaveLength(1)
  })
})

describe('KeyEventTimelineView — R-28 读数三态（未知 ≠ 0 条）', () => {
  it('读取中：视图头不冒称「0 条」、行头不渲染「当日 0 条」', async () => {
    eventsState.events = []
    eventsState.loading = true
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    const head = wrapper.find('[data-testid="key-event-selected-count"]')
    expect(head.text(), '读取中 MUST NOT 显示「0 条」').not.toContain('0 条')
    expect(head.text(), '读取中如实说明').toContain('读取中')
    expect(wrapper.find('[data-testid="key-event-rowday-93"]').exists(), '读取中不渲染行头读数').toBe(false)
    expect(wrapper.text(), '整页 MUST NOT 出现「当日 0 条」').not.toContain('当日 0 条')
  })

  it('首读失败：视图头不冒称「0 条」，改说「读取失败」', async () => {
    eventsState.events = []
    eventsState.loading = false
    eventsState.error = 'boom'
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    const head = wrapper.find('[data-testid="key-event-selected-count"]')
    expect(head.text(), '读取失败 MUST NOT 显示「0 条」').not.toContain('0 条')
    expect(head.text()).toContain('读取失败')
    expect(wrapper.text(), '整页 MUST NOT 出现「当日 0 条」').not.toContain('当日 0 条')
  })

  it('成功且确无事件：此时才显示「MM/DD · 0 条」与「当日 0 条」', async () => {
    eventsState.events = []
    eventsState.loading = false
    eventsState.error = null
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    expect(wrapper.find('[data-testid="key-event-selected-count"]').text()).toBe('11/12 · 0 条')
    expect(wrapper.find('[data-testid="key-event-rowday-93"]').text()).toBe('当日 0 条')
  })

  it('陈旧快照：旧数据有数时标注「旧快照」；旧快照不含当前日时不冒称 0 条', async () => {
    eventsState.events = [event('a', '93', '2026-11-12', '保留的旧事件')]
    eventsState.error = '网络断了'
    eventsState.staleNotice = true

    const stale = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const head = stale.find('[data-testid="key-event-selected-count"]')
    expect(head.text(), '旧快照的数字 MUST 标注来源').toContain('旧快照')
    expect(head.text()).toContain('1 条')
    expect(stale.find('[data-testid="key-event-rowday-93"]').text()).toContain('旧快照')

    // 旧快照不覆盖当前日 → 仍属未知，MUST NOT 冒称「0 条」
    const unknown = mountView({ selectedDate: '2026-11-13' })
    await settle()
    const unknownHead = unknown.find('[data-testid="key-event-selected-count"]')
    expect(unknownHead.text(), '旧快照不含当前日 MUST NOT 显示「0 条」').not.toContain('0 条')
    expect(unknownHead.text()).toContain('旧快照')
    expect(unknown.find('[data-testid="key-event-rowday-93"]').exists(), '未知态不渲染行头读数').toBe(false)
  })
})

describe('KeyEventTimelineView — R-29 整窗空态与选中日空态互斥', () => {
  it('整窗 0 条：只显示整窗说明，不叠加「选中日无事件」行', async () => {
    eventsState.events = []
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    expect(wrapper.find('[data-testid="key-event-empty-window"]').exists(), '整窗空态出现').toBe(true)
    expect(wrapper.find('[data-testid="key-event-selected-day-empty"]').exists(), 'MUST NOT 叠加当日空态行').toBe(false)
    expect(wrapper.findAll('.kev-inline'), '空态说明只有一条').toHaveLength(1)
  })

  it('窗内其它日有事件、选中日为零：只显示「选中日无事件」行', async () => {
    eventsState.events = [event('a', '93', '2026-11-11')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    expect(wrapper.find('[data-testid="key-event-empty-window"]').exists(), '整窗有事件 → 不出现整窗空态').toBe(false)
    expect(wrapper.find('[data-testid="key-event-selected-day-empty"]').text()).toBe('11/12 没有确认的关键事件')
  })
})

describe('KeyEventTimelineView — R-30 收起行每天只出一个节点', () => {
  it('收起行同日 9 条 → 只渲染 1 个节点并承载「★9」（不再逐事件扇开）', async () => {
    // 宽舞台（列宽 281.6px）：9 条本可扇开（9×18 + 8×4 = 194 ≤ 281.6），
    // 但该行 9 层 > 4 层上限 → 收起 → 每天只出一个居中节点。
    eventsState.events = Array.from({ length: 9 }, (_, i) => event(`e${i}`, '93', '2026-11-12', `第${i}条`))
    const wrapper = mountView()
    await settle()

    const row = wrapper.find('[data-testid="key-event-row-93"]')
    expect(row.findAllComponents(KeyEventLabel), '9 层 > 4 → 该行收起（无标签）').toHaveLength(0)
    const nodes = row.findAll('[data-testid="key-event-node-93-2026-11-12"]')
    expect(nodes, '收起行：每天只出一个节点').toHaveLength(1)
    expect(nodes[0]!.text(), '该节点承载当日计数').toContain('★9')
  })

  it('64px 列宽 + 相邻两天各 9 条（都收起）→ 两节点矩形不相交（按实测盒宽算）', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 })
    eventsState.events = [
      ...Array.from({ length: 9 }, (_, i) => event(`a${i}`, '93', '2026-11-12', `甲${i}`)),
      ...Array.from({ length: 9 }, (_, i) => event(`b${i}`, '93', '2026-11-13', `乙${i}`)),
    ]
    const wrapper = mountView()
    await settle()

    const col = Number.parseFloat(
      (wrapper.find('.kev-day-heads').element as HTMLElement).style.getPropertyValue('--kev-col'),
    )
    expect(col, '窄舞台 → 列宽钉在 64px 下限').toBe(64)

    const first = nodeRects(wrapper, '93', '2026-11-12', AXIS)
    const second = nodeRects(wrapper, '93', '2026-11-13', AXIS)
    expect(first, '收起行：每天一个节点').toHaveLength(1)
    expect(second, '收起行：每天一个节点').toHaveLength(1)
    // 盒宽按实测（★N 计数盒 26px），不是固定 18px —— review R-30 的越界检出口径
    expect(first[0]!.right - first[0]!.left).toBeGreaterThan(NODE_WIDTH)
    expect(
      Math.max(...first.map(r => r.right)),
      '相邻日节点矩形 MUST NOT 相交（带计数盒的节点单点居中，18px 预算成立）',
    ).toBeLessThanOrEqual(Math.min(...second.map(r => r.left)))
    // 各自在本列内（MUST NOT 溢出到邻列）
    for (const rect of first) {
      expect(rect.left).toBeGreaterThanOrEqual(2 * col)
      expect(rect.right).toBeLessThanOrEqual(3 * col)
    }
    for (const rect of second) {
      expect(rect.left).toBeGreaterThanOrEqual(3 * col)
      expect(rect.right).toBeLessThanOrEqual(4 * col)
    }
  })
})

// ── 2026-11-24 用户实机反馈（收起节点补日期前缀 / 行头去选中）──────────────────

describe('KeyEventTimelineView — 收起节点带日期前缀（2026-11-24 用户反馈）', () => {
  it('收起行节点文本 = 「11.12 ★5」：只写 ★N 看不出是哪天', async () => {
    // 宽舞台：5 条仍会因 5 层 > 4 层上限而收起（不依赖窄列退化）。
    eventsState.events = Array.from({ length: 5 }, (_, i) => event(`d${i}`, '93', '2026-11-12', `第${i}条`))
    const wrapper = mountView()
    await settle()

    const nodes = wrapper.findAll('[data-testid="key-event-node-93-2026-11-12"]')
    expect(nodes, '收起行：每天一个节点').toHaveLength(1)
    expect(nodes[0]!.text().replace(/\s+/g, ' ').trim(), '节点文本 MUST 带日期前缀（点号分隔）+ 当日计数')
      .toBe('11.12 ★5')
    expect(nodes[0]!.attributes('title'), 'title 不变（date · 事件标题）').toBe('2026-11-12 · 第0条')
  })

  it('64px 列宽下收起节点盒宽 ≤ 列宽（MUST NOT 溢出邻列、MUST NOT 截文字）', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, get: () => 300 })
    eventsState.events = [
      ...Array.from({ length: 9 }, (_, i) => event(`a${i}`, '93', '2026-11-12', `甲${i}`)),
      ...Array.from({ length: 9 }, (_, i) => event(`b${i}`, '93', '2026-11-13', `乙${i}`)),
    ]
    const wrapper = mountView()
    await settle()

    const col = Number.parseFloat(
      (wrapper.find('.kev-day-heads').element as HTMLElement).style.getPropertyValue('--kev-col'),
    )
    expect(col, '窄舞台 → 列宽钉在 64px 下限').toBe(64)

    const expectations: [string, string][] = [['2026-11-12', '11.12 ★9'], ['2026-11-13', '11.13 ★9']]
    for (const [date, label] of expectations) {
      const node = wrapper.find(`[data-testid="key-event-node-93-${date}"]`)
      const element = node.element as HTMLElement
      expect(element.offsetWidth, `${label} 盒宽 MUST ≤ 列宽（64px）`).toBeLessThanOrEqual(col)
      expect(node.text().replace(/\s+/g, ' ').trim(), '文本完整（MUST NOT 省略号截断）').toBe(label)
      expect(element.classList.contains('has-count'), '收起节点带 has-count（nowrap + 胶囊盒）').toBe(true)
      // 单节点恒居中：盒宽 ≤ 列宽 ⇒ 两侧留白对称，不进邻列
      const rects = nodeRects(wrapper, '93', date, AXIS)
      expect(rects[0]!.left).toBeGreaterThanOrEqual(AXIS.indexOf(date) * col)
      expect(rects[0]!.right).toBeLessThanOrEqual((AXIS.indexOf(date) + 1) * col)
    }
  })

  it('源码契约：has-count 节点强制 nowrap、不靠省略号 / 换行收容超宽文本', async () => {
    const source = await viewSource()
    const start = source.indexOf('.kev-node.has-count {')
    expect(start, '未找到 .kev-node.has-count 规则块').toBeGreaterThan(-1)
    const block = source.slice(start, source.indexOf('}', start))
    expect(block, '收起节点 MUST NOT 换行').toContain('white-space: nowrap')
    expect(block, '盒宽不靠省略号收容（MUST NOT 截文字）').not.toContain('text-overflow')
    expect(block, '盒宽不靠溢出隐藏收容（MUST NOT 截文字）').not.toContain('overflow')
    expect(block, '窄列下优先缩小字号').toMatch(/font-size:\s*0\.5\dr?em/)
  })
})

describe('KeyEventTimelineView — 行头只读、不再有选中态（2026-11-24 用户反馈）', () => {
  it('活跃行行头是 div：不是 button、不可点、带 is-selected 的旧视觉不再出现', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedLaneId: '93' })
    await settle()

    const head = wrapper.find('[data-testid="key-event-rowhead-93"]')
    expect(head.element.tagName, '行头 MUST 是只读 div（本视图无联动，假选中易误解）').toBe('DIV')
    expect(head.classes(), '行头 MUST NOT 带 is-selected').not.toContain('is-selected')
    await head.trigger('click')
    expect(wrapper.emitted('selectLane'), '点行头 MUST NOT emit selectLane').toBeUndefined()
  })

  it('休眠行行头同样是只读 div（无选中态、点不出 emit）', async () => {
    eventsState.events = [event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedLaneId: '97', expandedDormant: true })
    await settle()

    const head = wrapper.find('[data-testid="key-event-rowhead-97"]')
    expect(head.element.tagName, '休眠行头 MUST 也是只读 div').toBe('DIV')
    expect(head.classes()).not.toContain('is-selected')
    await head.trigger('click')
    expect(wrapper.emitted('selectLane')).toBeUndefined()
  })

  it('源码契约：行头规则去掉 is-selected / 悬停反馈，光标回到默认', async () => {
    const source = await viewSource()
    expect(source, '行头选中态样式 MUST 删除').not.toContain('.kev-row-head.is-selected')
    expect(source, '行头不再可点 → MUST NOT 保留悬停加粗').not.toContain('.kev-row-head:hover')
    expect(source, '模板里行头 MUST NOT 再绑定 selectLane').not.toContain('@click="emit(\'selectLane\'')

    const headStart = source.indexOf('.kev-row-head {')
    expect(headStart, '未找到 .kev-row-head 规则块').toBeGreaterThan(-1)
    const headBlock = source.slice(headStart, source.indexOf('}', headStart))
    expect(headBlock, '行头不可点 → cursor: default（MUST NOT pointer）').toContain('cursor: default')
    expect(headBlock, '行头不可点 → MUST NOT 保留 cursor: pointer').not.toContain('cursor: pointer')
    // 视觉与改前一致：左缘留 2px 透明边框（改前是 button 的同一数值）
    expect(headBlock, '行头左缘保留 2px 透明边框（文字左缘与改前一致）')
      .toContain('border-left: 2px solid transparent')
  })
})

// ── 实机 UI 验收缺陷回归（azure R-31 ~ R-34）──────────────────────────────────

/** 30 天轴（11/24 前的 30 天：10/24 ~ 11/22）。 */
const LONG_AXIS = Array.from({ length: 30 }, (_, i) => encodeDay('2026-10-24', i))

/**
 * 30 天窗口下 LLM 生态行（版块 1980 真实 11 条确认事件）的标题。
 * 宽度由 `offsetWidth` 桩按字符数折算（≈ 11.5px/字），与实机 211~317px 同量级。
 */
const LONG_TITLES: [string, string][] = [
  ['e1', '2026-10-24'], ['e2', '2026-10-27'], ['e3', '2026-10-30'], ['e4', '2026-11-05'],
  ['e5', '2026-11-06'], ['e6', '2026-11-10'], ['e7', '2026-11-10'], ['e8', '2026-11-11'],
  ['e9', '2026-11-12'], ['e10', '2026-11-16'], ['e11', '2026-11-18'],
]

describe('KeyEventTimelineView — 30 天窗口不把事件名收成 ★N（R-32）', () => {
  beforeEach(() => {
    // 标签实测宽按文本长度折算（实机量得 211~317px；happy-dom 无布局引擎）
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
      configurable: true,
      get(this: HTMLElement) {
        if (this.classList?.contains('kev-label')) {
          return Math.max(120, (this.textContent ?? '').length * 11.5)
        }
        return 110
      },
    })
  })

  afterEach(() => {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetWidth
  })

  it('64px 列宽 + 11 条跳 30 天的标签：仍渲染标签（层数 ≤ 4，不收成星标）', async () => {
    eventsState.events = LONG_TITLES.map(([id, date]) => event(id, '93', date, 'LLM生态事件标签标题文本'))
    const wrapper = mountView({ axis: LONG_AXIS, activeRows: [row('93', 'LLM生态相关新闻')], dormantRows: [], selectedDate: '2026-11-13' })
    await settle()

    const heads = wrapper.find('.kev-day-heads').element as HTMLElement
    expect(heads.style.getPropertyValue('--kev-col'), '30 天 → 列宽落 64px 下限').toBe('64px')

    // 选中日 11/13 在轴上 → 该日分组跳过；其余 11 条必须都是标签（不能只剩节点 + ★N）
    expect(wrapper.findAll('.kev-label')).toHaveLength(11)

    // 用组件实测到的分组参数回放归带 → 层数必须 ≤ 4（阈值 4 层不得放宽）
    const call = bandCallFor('e11')
    expect(call, '归带函数未被调用').toBeDefined()
    const replay = layoutKeyEventBands(
      call![0] as Parameters<typeof layoutKeyEventBands>[0],
      call![1] as number,
      call![2] as Parameters<typeof layoutKeyEventBands>[2],
      call![3] as string | null,
    )
    expect(replay.upLayers + replay.downLayers).toBeLessThanOrEqual(4)
  })
})

describe('KeyEventTimelineView — 行头不在横向滚动区内（R-33）', () => {
  it('行头列独立于滚动区：滚动区里没有行头，标签 left ≥ 0（不会落进行头覆盖区）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12'), event('b', '95', '2026-11-13')]
    const wrapper = mountView({ selectedDate: null })
    await settle()

    const headCol = wrapper.find('[data-testid="key-event-head-col"]')
    expect(headCol.exists(), '行头列必须存在（行头 sticky 在滚动容器内会盖住左缘 192px 内容）').toBe(true)
    expect(headCol.findAll('.kev-row-head').length, '行头都在行头列里').toBeGreaterThan(0)

    const scroller = wrapper.find('[data-testid="key-event-scroll"]')
    expect(scroller.findAll('.kev-row-head'), '滚动区内 MUST NOT 再有行头（否则会盖住标签/引线）').toHaveLength(0)
    expect(scroller.findAll('[data-testid="key-event-head-col"]')).toHaveLength(0)

    // 行头列逐行等高：行体高度 = 行头单元高度（否则上下会错位）
    const bodyHeights = wrapper.findAll('.kev-row-body').map(el => (el.element as HTMLElement).style.height)
    const headHeights = wrapper.findAll('.kev-head-cell').map(el => (el.element as HTMLElement).style.height)
    expect(headHeights).toEqual(bodyHeights.slice(0, headHeights.length))

    // 标签 left ≥ 0（相对行体）：不会被推到行体左边界之外
    for (const label of wrapper.findAll('.kev-label')) {
      const left = Number.parseFloat((label.element as HTMLElement).style.left)
      expect(left).toBeGreaterThanOrEqual(0)
    }
  })

  it('行头列与滚动区同序等高（含休眠组标题占位）：展开休眠行也不上下错位', async () => {
    eventsState.events = [event('a', '93', '2026-11-12'), event('d', '97', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-13', expandedDormant: true })
    await settle()

    // 行头列顺序：轴头占位 → 活跃行头 → 休眠组标题占位 → 休眠行头
    const headCol = wrapper.find('[data-testid="key-event-head-col"]')
    const headOrder = headCol.element.children
      ? Array.from(headCol.element.children).map(el => el.className)
      : []
    expect(headOrder[0]).toContain('kev-head-axis-spacer')
    expect(headOrder).toContain('kev-head-summary-spacer')
    expect(headOrder.at(-1)).toContain('kev-head-cell')

    // 滚动区顺序：轴头 → 活跃行体 → 休眠组标题 → 休眠行体
    const inner = wrapper.find('[data-testid="key-event-grid"]').element
    const innerOrder = Array.from(inner.children).map(el => el.className)
    expect(innerOrder[0]).toContain('kev-day-heads')
    expect(innerOrder).toContain('kev-group-summary')
    expect(innerOrder.at(-1)).toContain('kev-row')

    // 每个行头单元与其行体等高（同用 rowHeight()）
    const bodyByRow = wrapper.findAll('.kev-row-body').map(el => (el.element as HTMLElement).style.height)
    const headByCell = wrapper.findAll('.kev-head-cell').map(el => (el.element as HTMLElement).style.height)
    expect(headByCell).toEqual(bodyByRow)
  })

  it('源码契约：行头规则不再 sticky，轴头与行体同起点（行头占位 = 列宽公式里的 192px）', async () => {
    const source = await viewSource()
    const headStart = source.indexOf('.kev-row-head {')
    expect(headStart).toBeGreaterThan(-1)
    const headBlock = source.slice(headStart, source.indexOf('}', headStart))
    expect(headBlock, '行头 MUST NOT 用 sticky 待在滚动容器里（R-33）').not.toContain('sticky')
    expect(headBlock).toContain('position: relative')

    const stageStart = source.indexOf('.kev-stage {')
    expect(stageStart).toBeGreaterThan(-1)
    const headColStart = source.indexOf('.kev-head-col {')
    expect(headColStart).toBeGreaterThan(-1)
    const headColBlock = source.slice(headColStart, source.indexOf('}', headColStart))
    expect(headColBlock, '行头列宽 = 列宽公式里的行头占位').toContain('width: 192px')

    // 轴头行与行头列占位共用同一变量 → 不靠两处手抄同一个数值对齐
    expect(source, '轴头行高必须是共享变量').toContain('--kev-axis-head:')
    const spacerStart = source.indexOf('.kev-head-axis-spacer {')
    const headsStart = source.indexOf('.kev-day-heads {')
    expect(source.slice(spacerStart, source.indexOf('}', spacerStart))).toContain('height: var(--kev-axis-head)')
    expect(source.slice(headsStart, source.indexOf('}', headsStart))).toContain('height: var(--kev-axis-head)')
  })
})

describe('KeyEventTimelineView — 轴头日期可选日（R-34）', () => {
  it('点轴头日期 → emit selectDate；可键盘激活（原生 button + aria-pressed）', async () => {
    eventsState.events = [event('a', '93', '2026-11-12')]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    const heads = wrapper.findAll('.kev-day')
    expect(heads.length).toBe(AXIS.length)
    const target = wrapper.find('[data-testid="key-event-day-2026-11-13"]')
    expect(target.element.tagName, '轴头必须是原生 button（Enter / Space 直开生效）').toBe('BUTTON')
    expect(target.attributes('type')).toBe('button')
    expect(target.attributes('aria-pressed'), '选中日 aria-pressed=true').toBe('false')
    expect(wrapper.find('[data-testid="key-event-day-2026-11-12"]').attributes('aria-pressed')).toBe('true')

    await target.trigger('click')
    expect(wrapper.emitted('selectDate')).toEqual([['2026-11-13']])
  })
})

describe('KeyEventTimelineView — 卡片展开后行高同一轮重算（R-31）', () => {
  /** 卡片实测高（展开摘要/文章列表后变大）。 */
  let cardHeight = 116
  /** 逐轮实测高队列（模拟「先摘要、后文章行」两段变高）；空则用 cardHeight。 */
  let queue: number[] = []

  beforeEach(() => {
    cardHeight = 116
    queue = []
    Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
      configurable: true,
      get(this: HTMLElement) {
        if (!this.classList?.contains('kdc')) return 0
        return queue.length > 0 ? queue.shift()! : cardHeight
      },
    })
  })

  afterEach(() => {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetHeight
  })

  it('展开摘要（卡变高）→ 同一轮重测：行高 ≥ 卡片列底，后续行标签不会压上来', async () => {
    eventsState.events = [
      { ...event('a', '93', '2026-11-12', 'LLM 当日事件'), summarySnapshot: '摘要全文：这一段展开后会把卡片撑高'.repeat(6) },
      event('b', '95', '2026-11-11', '下一行的事件'),
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()

    const rowBody = () => wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-body').element as HTMLElement
    expect(rowBody().style.height, '1 张 116px 卡：行高 = 轴带下缘 + 8 + 116 + 8 = 166px').toBe('166px')

    // 展开摘要：卡实测变高（300px）→ 点击（capture）触发重测，同一轮完成行高重算
    cardHeight = 300
    await wrapper.find('[data-testid="key-event-day-card-a"]').find('.kdc-summary-btn').trigger('click')
    await settle()
    expect(rowBody().style.height, '展开后行高 MUST 跟上卡片列底（轴带下缘 34 + 8 + 300 + 8）').toBe('350px')

    // 下一行标签的 top 在自己的行体坐标系里恒 > 0（行高覆盖卡片列 → 两行不会叠）
    const nextLabel = wrapper.find('[data-testid="key-event-label-b"]').element as HTMLElement
    expect(Number.parseFloat(nextLabel.style.top)).toBeGreaterThan(0)

    // 收起摘要（卡变矮）+ transitionend 兜底重测 → 行高回缩
    cardHeight = 116
    const cards = wrapper.find('[data-testid="key-event-cards-93"]')
    await cards.trigger('transitionend')
    await settle()
    expect(rowBody().style.height).toBe('166px')
  })

  it('卡片两段变高（关联文章行晚一轮落地）也当场收口：行高取最终实测，不留滞后一帧的缝', async () => {
    eventsState.events = [
      { ...event('a', '93', '2026-11-12', 'LLM 当日事件'), summarySnapshot: '摘要全文'.repeat(20) },
    ]
    const wrapper = mountView({ selectedDate: '2026-11-12' })
    await settle()
    const rowBody = wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-body').element as HTMLElement

    // 第一轮实测 300px（摘要展开）→ 第二轮实测 420px（文章行晚一轮才渲染）
    queue = [300, 420]
    cardHeight = 420
    await wrapper.find('[data-testid="key-event-day-card-a"]').find('.kdc-summary-btn').trigger('click')
    await settle()
    await settle()
    expect(rowBody.style.height, '两轮重测：行高 MUST 跟上最终卡片列底（34 + 8 + 420 + 8）').toBe('470px')
  })

  it('卡片变高不走点击路径也要重算：卡片列 ResizeObserver 兜底', async () => {
    // 用户反馈「点击卡片展开高度没自适应」的根因是行高只靠点击 / transitionend 触发重测，
    // 任何其它原因导致的卡片变高都会漏测。这里断言观察者路径独立生效。
    const callbacks: Array<(entries: unknown[]) => void> = []
    class FakeResizeObserver {
      constructor(cb: (entries: unknown[]) => void) { callbacks.push(cb) }
      observe(): void {}
      unobserve(): void {}
      disconnect(): void {}
    }
    const original = (globalThis as { ResizeObserver?: unknown }).ResizeObserver
    ;(globalThis as { ResizeObserver?: unknown }).ResizeObserver = FakeResizeObserver
    try {
      eventsState.events = [
        { ...event('a', '93', '2026-11-12', 'LLM 当日事件'), summarySnapshot: '摘要全文'.repeat(20) },
      ]
      const wrapper = mountView({ selectedDate: '2026-11-12' })
      await settle()
      const rowBody = wrapper.find('[data-testid="key-event-row-93"]').find('.kev-row-body').element as HTMLElement
      expect(rowBody.style.height).toBe('166px')
      expect(callbacks.length, '每行一个卡片列观察者，挂载时即登记').toBeGreaterThan(0)

      // 非点击原因变高（文章行回流等）→ 观察者回调 → 行高跟上
      cardHeight = 300
      for (const cb of callbacks) cb([])
      await settle()
      await settle()
      expect(rowBody.style.height, '观察者触发后行高 MUST 等于 34 + 8 + 300 + 8').toBe('350px')
    } finally {
      ;(globalThis as { ResizeObserver?: unknown }).ResizeObserver = original
    }
  })

})
