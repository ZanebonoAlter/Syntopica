import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import LaneCorrectionCanvas from './LaneCorrectionCanvas.vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

/**
 * 泳道修正画布用例（test-cases 故事 A / B / D 的画布落点 + A-4 / A-9 / A-12）。
 *
 * 判据（机械断言，不看实现）：
 * - 点节点只改选中态（**不改成员数**：画布不抛任何归属事件）；
 * - 节点卡跟随选中节点；缩放后卡片位置不塌扁；
 * - 「更早」折叠带：虚线分隔 +「更早 · N」+ 首义虚线圆点纵向堆叠（非比例），上限 6 +「+M」；
 * - 折叠带勾选与左栏「更早」分组同一份选择集（emit toggleAnchor）；
 * - 确认条在画布内（不叠弹窗）；
 * - 状态矩阵：loading 骨架 / error 行内错误 + 重试（**首次加载失败不呈现业务空态**）/ empty 空态；
 * - 图例只 4 个符号，工具条 / 图例 / 节点卡无解说性长句（A-9）。
 */

function node(
  id: string,
  date: string,
  title: string,
  extra: Partial<LaneCorrectionNode> = {},
): LaneCorrectionNode {
  return {
    id,
    date,
    title,
    topicId: '',
    topicLabel: '',
    proximity: null,
    // 接近度来源：默认归属快照（关联推荐行为 estimate）。
    proximityKind: 'snapshot',
    outsideWindow: false,
    ...extra,
  }
}

const MEMBERS = [
  node('m1', '2026-11-10', '本泳道成员一', { topicId: '7', topicLabel: 'AI 前沿', proximity: 0.91 }),
  node('m2', '2026-11-12', '本泳道成员二', { topicId: '7', topicLabel: 'AI 前沿', proximity: 0.84 }),
]

const CANDIDATES = [
  // 候选接近度 = 估算口径（useLaneCorrection.recommendations 覆盖为 estimate）。
  node('c1', '2026-11-11', '候选节点一', { proximity: 0.62, proximityKind: 'estimate' }),
]

const READONLY = [
  node('r1', '2026-11-11', '别的泳道节点', { topicId: '9', topicLabel: '新能源' }),
]

const FOLD = [
  node('f1', '2026-10-21', '更早首义节点一', { topicId: '7', topicLabel: 'AI 前沿', outsideWindow: true }),
  node('f2', '2026-10-28', '更早首义节点二', { topicId: '7', topicLabel: 'AI 前沿', outsideWindow: true }),
]

function mountCanvas(overrides: Record<string, unknown> = {}) {
  return mount(LaneCorrectionCanvas, {
    props: {
      laneLabel: 'AI 前沿',
      dates: ['2026-11-10', '2026-11-11', '2026-11-12'],
      memberNodes: MEMBERS,
      candidateNodes: CANDIDATES,
      readonlyNodes: READONLY,
      removedNodes: [],
      foldNodes: FOLD,
      anchorSelection: [],
      focusId: null,
      loading: false,
      error: null,
      emptyNote: null,
      confirmVisible: false,
      sourceLabel: '自动（创建话题时写入）',
      ...overrides,
    },
  })
}

// ── 拖拽手势 harness（happy-dom 不实现 PointerEvent 坐标系，按浏览器语义补齐）──

interface Pt { x: number, y: number }

/** 指针事件：happy-dom 的 Event 不带 clientX/clientY/pointerId。 */
function pointer(type: 'pointerdown' | 'pointermove' | 'pointerup' | 'pointercancel', at: Pt): PointerEvent {
  const event = new Event(type, { bubbles: true, cancelable: true })
  Object.defineProperties(event, {
    clientX: { value: at.x, configurable: true },
    clientY: { value: at.y, configurable: true },
    pointerId: { value: 1, configurable: true },
  })
  return event as unknown as PointerEvent
}

function surfaceOf(w: ReturnType<typeof mountCanvas>) {
  return w.find('[data-testid="lane-correction-canvas"]')
}

/**
 * 分区带的纵向中心（读画布真实几何，不写死常量）。
 *
 * 落点只判纵向分区带（X 轴是日期事实、不参与判定），故 y 直接取
 * `lane-dropzone-*` 矩形的实际 y / height。
 */
function zoneY(w: ReturnType<typeof mountCanvas>, zone: 'member' | 'candidate' | 'readonly'): number {
  const rect = w.find(`[data-testid="lane-dropzone-${zone}"]`)
  return Number(rect.attributes('y')) + Number(rect.attributes('height')) / 2
}

/** 起手：必须命中节点 `[data-node]` 才接管拖拽手势。 */
async function pressAt(w: ReturnType<typeof mountCanvas>, selector: string, at: Pt = { x: 400, y: 70 }): Promise<void> {
  w.find(selector).element.dispatchEvent(pointer('pointerdown', at))
  await nextTick()
}

async function movePointer(w: ReturnType<typeof mountCanvas>, at: Pt): Promise<void> {
  surfaceOf(w).element.dispatchEvent(pointer('pointermove', at))
  await nextTick()
}

async function releasePointer(w: ReturnType<typeof mountCanvas>, at: Pt): Promise<void> {
  surfaceOf(w).element.dispatchEvent(pointer('pointerup', at))
  await nextTick()
}

/** 完整拖拽：按下节点 → 越过 4px 阈值 → 松手。 */
async function dragNode(w: ReturnType<typeof mountCanvas>, id: string, to: Pt): Promise<void> {
  await pressAt(w, `[data-testid="lane-node-${id}"]`)
  await movePointer(w, to)
  await releasePointer(w, to)
}

/** 空白处轻点：起手与松手都落在画布空白（直接派发到滚动容器，不命中任何节点 / 卡片）。 */
async function pressBlank(w: ReturnType<typeof mountCanvas>, at: Pt): Promise<void> {
  surfaceOf(w).element.dispatchEvent(pointer('pointerdown', at))
  await nextTick()
  await releasePointer(w, at)
}

/** 写事件一条都不该有（拖拽不可用 / 取消的负向断言）。 */
function expectNoWrites(w: ReturnType<typeof mountCanvas>): void {
  expect(w.emitted('add')).toBeUndefined()
  expect(w.emitted('remove')).toBeUndefined()
  expect(w.emitted('restore')).toBeUndefined()
}

describe('LaneCorrectionCanvas · 查看与选入分离', () => {
  it('点节点只抛 focus（不抛任何归属事件 → 成员数不变；S1.3 负向保留）', async () => {
    const w = mountCanvas()
    await w.find('[data-testid="lane-node-m1"]').trigger('click')
    expect(w.emitted('focus')).toEqual([['m1']])
    expect(w.emitted('remove')).toBeUndefined()
    expect(w.emitted('add')).toBeUndefined()
    expect(w.emitted('restore')).toBeUndefined()
  })

  it('节点卡跟随选中节点：标题 / 日期 / 快照接近度（来源可辨）/ 身份结论', () => {
    const w = mountCanvas({ focusId: 'm1' })
    const card = w.find('[data-testid="lane-node-card"]')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('本泳道成员一')
    expect(card.text()).toContain('2026-11-10')
    expect(card.text()).toContain('接近度 0.91（快照）')
    expect(card.text()).toContain('本泳道成员')
  })

  it('候选节点卡接近度为估算口径（与成员快照不同名不同形）', () => {
    const w = mountCanvas({ focusId: 'c1' })
    const card = w.find('[data-testid="lane-node-card"]')
    expect(card.text()).toContain('估算接近度 0.62')
    expect(card.text()).not.toContain('快照')
  })

  it('缩放后节点卡不塌扁（位置仍是像素坐标，读数随缩放变化）', async () => {
    const w = mountCanvas({ focusId: 'm1' })
    const before = w.find('[data-testid="lane-node-card"]').attributes('style') ?? ''
    await w.find('[data-testid="lane-zoom-in"]').trigger('click')
    expect(w.find('[data-testid="lane-zoom-readout"]').text()).toBe('110%')
    const after = w.find('[data-testid="lane-node-card"]').attributes('style') ?? ''
    expect(after).toContain('left:')
    expect(after).toContain('top:')
    expect(before).not.toBe('')
  })

  // ── 点空白取消聚焦（align-lane-correction-workface 5.8，实机反馈）──
  it('聚焦在场时轻点空白画布 → 抛 clear-focus、零写事件；未聚焦时不抛', async () => {
    const focused = mountCanvas({ focusId: 'm1' })
    await pressBlank(focused, { x: 400, y: 300 })
    expect(focused.emitted('clear-focus')).toEqual([[]])
    expectNoWrites(focused)

    const idle = mountCanvas({ focusId: null })
    await pressBlank(idle, { x: 400, y: 300 })
    expect(idle.emitted('clear-focus')).toBeUndefined()
  })

  it('拖动平移后松手不抛 clear-focus（滚动不是选择）', async () => {
    const w = mountCanvas({ focusId: 'm1' })
    surfaceOf(w).element.dispatchEvent(pointer('pointerdown', { x: 400, y: 300 }))
    await nextTick()
    await movePointer(w, { x: 440, y: 330 })
    await releasePointer(w, { x: 440, y: 330 })
    expect(w.emitted('clear-focus')).toBeUndefined()
  })

  it('落在节点上的轻点不抛 clear-focus：可拖与不可拖节点都交给节点自己的 click 聚焦', async () => {
    const w = mountCanvas()
    // 候选 c1（可拖）：按下起拖拽手势、未过阈值松手 → 不抛
    await pressAt(w, '[data-testid="lane-node-c1"]')
    await releasePointer(w, { x: 400, y: 70 })
    // 只读参照 r1（不可拖）：按下退化为平移、未过阈值松手 → 不抛
    await pressAt(w, '[data-testid="lane-node-r1"]')
    await releasePointer(w, { x: 400, y: 70 })
    expect(w.emitted('clear-focus')).toBeUndefined()
    // 聚焦仍由节点的 click 负责（再点同一节点切换的语义不被破坏）
    await w.find('[data-testid="lane-node-r1"]').trigger('click')
    expect(w.emitted('focus')).toEqual([['r1']])
  })

  it('点节点卡本体不取消聚焦（卡体接住点击，不算点空白）', async () => {
    const w = mountCanvas({ focusId: 'm1' })
    w.find('[data-testid="lane-node-card"]').element.dispatchEvent(pointer('pointerdown', { x: 420, y: 90 }))
    await nextTick()
    await releasePointer(w, { x: 420, y: 90 })
    expect(w.emitted('clear-focus')).toBeUndefined()
  })
})

describe('LaneCorrectionCanvas · 实机反馈补丁（节点卡标题换行 / 话题带横条移除）', () => {
  it('节点卡标题完整展示：全文在卡内，样式允许换行且无省略号截断', () => {
    const longTitle = 'DeepSeek 开发工具调用框架、并行调用与 Codex 调试实践：从单轮补全到多智能体编排的工程笔记'
    const w = mountCanvas({
      focusId: 'm1',
      memberNodes: [node('m1', '2026-11-10', longTitle, { topicId: '7', proximity: 0.91 })],
    })
    expect(w.find('[data-testid="lane-node-card"] .lcc-card__title').text()).toBe(longTitle)

    // 布局引擎在 happy-dom 里不算行：换行与否只能锁样式规则（白盒断言，防回退成单行省略）
    const source = readFileSync(`${import.meta.dirname}/LaneCorrectionCanvas.vue`, 'utf8')
    const rule = source.slice(source.indexOf('.lcc-card__title {'))
    const body = rule.slice(0, rule.indexOf('}'))
    expect(body).toMatch(/white-space:\s*normal/)
    expect(body).toMatch(/overflow-wrap:\s*anywhere/)
    expect(body).not.toMatch(/nowrap|text-overflow/)
  })

  it('话题带不再画强调色横条与刻度线；标签与中性基线仍在', () => {
    const w = mountCanvas()
    expect(w.find('.lcc-band-bar').exists()).toBe(false)
    expect(w.find('.lcc-band-tick').exists()).toBe(false)
    expect(w.find('.lcc-band-title').text()).toBe('本泳道话题带')
    expect(w.find('.lcc-track').exists()).toBe(true)
  })
})

describe('LaneCorrectionCanvas · 更早折叠带（A-12）', () => {
  it('画布左缘出现折叠带：虚线分隔 + 「更早 · N」+ 圆点堆叠', () => {
    const w = mountCanvas()
    const band = w.find('[data-testid="lane-fold-band"]')
    expect(band.exists()).toBe(true)
    expect(band.text()).toContain('更早 · 2')
    expect(w.find('[data-testid="lane-fold-node-f1"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-fold-node-f2"]').exists()).toBe(true)
    expect(band.find('.lcc-fold-sep').exists()).toBe(true)
  })

  it('带内节点可勾选（与左栏「更早」分组同一份选择集）', async () => {
    const w = mountCanvas()
    await w.find('[data-testid="lane-fold-check-f1"]').trigger('click')
    expect(w.emitted('toggleAnchor')).toEqual([['f1']])
  })

  it('上限 6 个 + 「+M」溢出标记（全量在左栏「更早」分组）', () => {
    const many = Array.from({ length: 8 }, (_, index) => node(`f${index}`, '2026-10-21', `更早节点 ${index}`, { outsideWindow: true }))
    const w = mountCanvas({ foldNodes: many })
    expect(w.find('[data-testid="lane-fold-node-f0"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-fold-node-f5"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-fold-node-f6"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-fold-band"]').text()).toContain('+2')
  })

  it('没有窗外首义节点时不出现折叠带', () => {
    const w = mountCanvas({ foldNodes: [] })
    expect(w.find('[data-testid="lane-fold-band"]').exists()).toBe(false)
  })
})

describe('LaneCorrectionCanvas · 状态矩阵', () => {
  it('loading：画布骨架，不闪空态', () => {
    const w = mountCanvas({ loading: true })
    expect(w.find('[data-testid="lane-correction-canvas-svg"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-correction-error"]').exists()).toBe(false)
    expect(w.find('[data-testid="lane-correction-empty"]').exists()).toBe(false)
  })

  it('error：行内错误 + 重试，且不呈现业务空态', async () => {
    const w = mountCanvas({ error: '泳道成员读取失败（网关超时）' })
    expect(w.find('[data-testid="lane-correction-error"]').exists()).toBe(true)
    expect(w.find('[data-testid="lane-correction-empty"]').exists()).toBe(false)
    await w.find('[data-testid="lane-correction-retry"]').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
  })

  it('empty：画布无节点可展示时给空态说明', () => {
    const w = mountCanvas({ emptyNote: '该日期范围内没有可展示的事件。' })
    expect(w.find('[data-testid="lane-correction-empty"]').text()).toContain('没有可展示的事件')
  })
})

describe('LaneCorrectionCanvas · 节点标题可读性（D-1）', () => {
  it('标题一个不少，且带描边光晕（paint-order: stroke，被标记圆压住仍可读）', () => {
    const w = mountCanvas()
    const titles = w.findAll('.lcc-label')
    // 标题一个不少：4 个节点各一条（用例标题均 ≤ 10 字，单行）
    expect(titles).toHaveLength(4)
    for (const title of titles) {
      expect(title.attributes('paint-order')).toBe('stroke')
      expect(title.attributes('stroke-width')).toBe('3')
      expect(title.attributes('stroke-linejoin')).toBe('round')
    }
  })
})

describe('LaneCorrectionCanvas · 确认条与文字纪律', () => {
  it('确认条在画布内（不叠弹窗），带当前首义来源', async () => {
    const w = mountCanvas({ confirmVisible: true })
    const bar = w.find('[data-testid="lane-anchor-confirm"]')
    expect(bar.exists()).toBe(true)
    expect(bar.text()).toContain('自动（创建话题时写入）')
    expect(w.find('.app-dialog').exists()).toBe(false)
    await w.find('[data-testid="lane-anchor-confirm-ok"]').trigger('click')
    expect(w.emitted('confirmApply')).toHaveLength(1)
    await w.find('[data-testid="lane-anchor-confirm-cancel"]').trigger('click')
    expect(w.emitted('cancelApply')).toHaveLength(1)
  })

  it('图例按新契约：候选大小=接近度（成员固定尺寸）+ 已生效首义/待提交勾选分立 + 相关标志；无解说长句（align-lane-correction-workface A-9）', () => {
    const w = mountCanvas({ focusId: 'm1' })
    const legend = w.find('.lcc-legend')
    expect(legend.text().replace(/\s+/g, '')).toBe(
      '成员候选大小=接近度已生效首义待提交勾选⊘已移出相关',
    )
    // 尺寸图例：候选以「小圆—大圆」表达分档，不再用单一固定圆冒充
    expect(legend.findAll('.lcc-lg--scale-sm').length).toBe(1)
    expect(legend.findAll('.lcc-lg--scale-lg').length).toBe(1)
    // 两种首义标记互斥可辨
    expect(legend.findAll('.lcc-lg--anchor-effective').length).toBe(1)
    expect(legend.findAll('.lcc-lg--anchor-pending').length).toBe(1)
    const banned = ['不是置信度', '点节点', '勾选 =', '重心 =', 'α']
    for (const text of banned) {
      expect(w.text()).not.toContain(text)
    }
  })
})

describe('LaneCorrectionCanvas · 节点卡写操作（unify-lane-workbench 4.1 / D8）', () => {
  it('成员卡上有「⊘移出本泳道」；点击 emit remove（不弹去处选择器/二次确认）', async () => {
    const w = mountCanvas({ focusId: 'm1' })
    const btn = w.find('[data-testid="lane-card-remove"]')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('移出本泳道')
    await btn.trigger('click')
    expect(w.emitted('remove')).toEqual([['m1']])
    expect(w.find('.app-dialog').exists()).toBe(false)
    expect(w.find('select').exists()).toBe(false)
  })

  it('候选卡上有「＋加入本泳道」；点击 emit add', async () => {
    const w = mountCanvas({ focusId: 'c1' })
    await w.find('[data-testid="lane-card-add"]').trigger('click')
    expect(w.emitted('add')).toEqual([['c1']])
  })

  it('已移出节点的卡上是「↩加回本泳道」；点击 emit restore', async () => {
    const removed = [node('m9', '2026-11-11', '本次移出节点', { topicId: '7', proximity: 0.5 })]
    const w = mountCanvas({ focusId: 'm9', removedNodes: removed })
    const btn = w.find('[data-testid="lane-card-restore"]')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('加回')
    await btn.trigger('click')
    expect(w.emitted('restore')).toEqual([['m9']])
  })

  it('只读参照与折叠带节点无写入口（active 不改挂、更早节点卡只读）', () => {
    const readonlyCard = mountCanvas({ focusId: 'r1' }).find('[data-testid="lane-node-card"]')
    expect(readonlyCard.find('[data-testid="lane-card-add"]').exists()).toBe(false)
    expect(readonlyCard.find('[data-testid="lane-card-remove"]').exists()).toBe(false)
    expect(readonlyCard.text()).toContain('只读')
    const foldCard = mountCanvas({ focusId: 'f1' }).find('[data-testid="lane-node-card"]')
    expect(foldCard.find('[data-testid="lane-card-add"]').exists()).toBe(false)
    expect(foldCard.find('[data-testid="lane-card-remove"]').exists()).toBe(false)
  })

  it('pending 同步禁用卡片按钮（防重复提交）', () => {
    const w = mountCanvas({ focusId: 'm1', pendingIds: ['m1'] })
    expect((w.find('[data-testid="lane-card-remove"]').element as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('LaneCorrectionCanvas · 相关节点自动高亮（unify-lane-workbench 5.1 / D6）', () => {
  const RELATIONS = [
    { fromId: 'c1', toId: 'r1' },
    { fromId: 'r1', toId: 'm2' },
  ]

  it('焦点的一跳相关高亮、非相关弱化；推荐命中也相关（B2）', () => {
    const w = mountCanvas({ focusId: 'c1', relations: RELATIONS })
    // c1—r1 一跳：r1 相关；r1—m2 两跳：不传递
    expect(w.find('[data-testid="lane-node-r1"]').classes()).toContain('is-related')
    expect(w.find('[data-testid="lane-node-m2"]').classes()).toContain('is-dimmed')
    expect(w.find('[data-testid="lane-node-m1"]').classes()).toContain('is-dimmed')
  })

  it('一跳中的 active 只读节点被强调但仍只读（卡无写入口）', () => {
    const w = mountCanvas({ focusId: 'c1', relations: RELATIONS })
    expect(w.find('[data-testid="lane-node-r1"]').classes()).toContain('is-readonly')
    const card = mountCanvas({ focusId: 'c1', relations: RELATIONS }).find('[data-testid="lane-node-card"]')
    expect(card.find('[data-testid="lane-card-add"]').exists()).toBe(true) // 候选自己可加
  })

  it('无焦点不弱化；取消聚焦恢复默认（不残留高亮态）', () => {
    const w = mountCanvas({ relations: RELATIONS })
    for (const id of ['m1', 'm2', 'c1', 'r1']) {
      expect(w.find(`[data-testid="lane-node-${id}"]`).classes()).not.toContain('is-dimmed')
      expect(w.find(`[data-testid="lane-node-${id}"]`).classes()).not.toContain('is-related')
    }
  })

  it('空质心仍可一跳：无候选时焦点相关只剩原关系（B3）', () => {
    const w = mountCanvas({ focusId: 'm1', relations: [{ fromId: 'm1', toId: 'r1' }], candidateNodes: [] })
    expect(w.find('[data-testid="lane-node-r1"]').classes()).toContain('is-related')
  })
})

describe('LaneCorrectionCanvas · 节点表达统一（unify-lane-workbench 4.3 / D7）', () => {
  it('成员半径固定（不随快照接近度变）；候选按接近度、恒小于成员', () => {
    const near = node('m-near', '2026-11-10', '近成员', { topicId: '7', proximity: 0.99 })
    const far = node('m-far', '2026-11-11', '远成员', { topicId: '7', proximity: 0.05 })
    const cand = node('c-near', '2026-11-12', '近候选', { proximity: 0.99, proximityKind: 'estimate' })
    const w = mountCanvas({ memberNodes: [near, far], candidateNodes: [cand], foldNodes: [], readonlyNodes: [] })
    const r = (id: string): string => w.find(`[data-testid="lane-node-${id}"]`).find('.lcc-point').attributes('r') ?? ''
    expect(r('m-near')).toBe(r('m-far')) // 成员固定尺寸
    expect(Number(r('c-near'))).toBeLessThan(Number(r('m-near'))) // 候选恒小于成员
    expect(Number(r('c-near'))).toBeGreaterThanOrEqual(3) // 像素下限
  })

  it('只读参照更弱更小（半径低于成员）', () => {
    const w = mountCanvas()
    const rReadonly = Number(w.find('[data-testid="lane-node-r1"]').find('.lcc-point').attributes('r'))
    const rMember = Number(w.find('[data-testid="lane-node-m1"]').find('.lcc-point').attributes('r'))
    expect(rReadonly).toBeLessThan(rMember)
  })
})

describe('LaneCorrectionCanvas · 成员区行距按节点占高（fix-lane-row-gap-zoom 4.1–4.3）', () => {
  type Canvas = ReturnType<typeof mountCanvas>

  /** 标签几何常量：须与组件 LABEL_TOP_GAP(5) + LABEL_MAX_LINES(2)×LABEL_LINE_H(12) 同步。 */
  const LABEL_BLOCK_H = 5 + 2 * 12

  /** 读节点真实 cy / r 属性（不读实现内部变量、不用快照）。 */
  function pointOf(w: Canvas, id: string): { cy: number, r: number } {
    const circle = w.find(`[data-testid="lane-node-${id}"]`).find('.lcc-point')
    return { cy: Number(circle.attributes('cy')), r: Number(circle.attributes('r')) }
  }

  function gapOf(w: Canvas, top: string, bottom: string): number {
    return Math.abs(pointOf(w, bottom).cy - pointOf(w, top).cy)
  }

  function svgHeight(w: Canvas): number {
    return Number(w.find('[data-testid="lane-correction-canvas-svg"]').attributes('height'))
  }

  function svgWidth(w: Canvas): number {
    return Number(w.find('[data-testid="lane-correction-canvas-svg"]').attributes('width'))
  }

  const SAME_DAY = [
    node('d1', '2026-11-10', '同日成员一', { topicId: '7', topicLabel: 'AI 前沿', proximity: 0.9 }),
    node('d2', '2026-11-10', '同日成员二', { topicId: '7', topicLabel: 'AI 前沿', proximity: 0.8 }),
  ]

  it('4.1 同一天两个成员：圆心距 ≥ 单个节点总占高（圆 + 两行标签）+ 4px（读真实 cy / r 属性）', () => {
    const w = mountCanvas({ dates: ['2026-11-10'], memberNodes: SAME_DAY, candidateNodes: [], readonlyNodes: [], foldNodes: [] })
    const top = pointOf(w, 'd1')
    const bottom = pointOf(w, 'd2')
    expect(bottom.cy).toBeGreaterThan(top.cy)
    // 节点总占高 = 圆直径 + 标签起笔间距 + 两行标签 = 2r + LABEL_BLOCK_H
    // 行距必须 ≥ 它 + 4，否则上一节点的标签会压在下一节点的圆上（原缺陷）
    expect(bottom.cy - top.cy).toBeGreaterThanOrEqual(2 * top.r + LABEL_BLOCK_H + 4)
  })

  it('4.2 行距由节点占高决定、与天数无关：7 天与 30 天档行距相同且都够放标签', () => {
    const dates = (count: number): string[] => Array.from({ length: count }, (_, index) => `2026-11-${String(index + 1).padStart(2, '0')}`)
    const mount = (count: number): Canvas => mountCanvas({
      dates: dates(count),
      memberNodes: SAME_DAY,
      candidateNodes: [],
      readonlyNodes: [],
      foldNodes: [],
    })
    const gap7 = gapOf(mount(7), 'd1', 'd2')
    const gap30 = gapOf(mount(30), 'd1', 'd2')
    // 本画布成员半径恒为 MEMBER_BASE_RADIUS（5.5，被 memberRadius 覆盖），
    // 不随列宽变 → 行距按「圆 + 两行标签」算，也应恒定，与天数无关。
    expect(gap7).toBe(gap30)
    // 且必须够放一个节点（圆 11 + 起笔 5 + 两行标签 24 + 间隙 4 = 44）
    expect(gap7).toBeGreaterThanOrEqual(44)
  })

  it('4.3 候选区不压成员，且成员多时画布变高（走滚动不压缩行距）', () => {
    const candidate = [node('c1', '2026-11-10', '候选一', { proximity: 0.5, proximityKind: 'estimate' })]
    const memberIds = ['m1', 'm2', 'm3', 'm4']
    const one = mountCanvas({
      dates: ['2026-11-10'],
      memberNodes: [node('m1', '2026-11-10', '成员一')],
      candidateNodes: candidate,
      readonlyNodes: [],
      foldNodes: [],
    })
    const four = mountCanvas({
      dates: ['2026-11-10'],
      memberNodes: memberIds.map(id => node(id, '2026-11-10', `成员${id}`)),
      candidateNodes: candidate,
      readonlyNodes: [],
      foldNodes: [],
    })
    const lowestMember = Math.max(...memberIds.map(id => pointOf(four, id).cy))
    expect(pointOf(four, 'c1').cy).toBeGreaterThan(lowestMember)
    expect(svgHeight(four)).toBeGreaterThan(svgHeight(one))
    // 行距不因成员变多而压缩（阈值必须覆盖标签块，否则旧的 16px 行距也能蒙混过关）
    expect(gapOf(four, 'm3', 'm4')).toBeGreaterThanOrEqual(2 * pointOf(four, 'm4').r + LABEL_BLOCK_H + 4)
  })

  it('4.5 候选区 / 只读参照区同样不重叠，且候选与已移出首节点不重合', () => {
    const w = mountCanvas({
      dates: ['2026-11-10'],
      memberNodes: [],
      candidateNodes: [
        node('c1', '2026-11-10', '候选一', { proximity: 0.6, proximityKind: 'estimate' }),
        node('c2', '2026-11-10', '候选二', { proximity: 0.5, proximityKind: 'estimate' }),
      ],
      removedNodes: [node('rm1', '2026-11-10', '已移出一')],
      readonlyNodes: [
        node('r1', '2026-11-10', '只读一', { topicId: '9', topicLabel: '新能源' }),
        node('r2', '2026-11-10', '只读二', { topicId: '9', topicLabel: '新能源' }),
      ],
      foldNodes: [],
    })
    // 候选区：同日两候选圆心距 ≥ 各自占高 + 4
    const c1 = pointOf(w, 'c1')
    const c2 = pointOf(w, 'c2')
    expect(Math.abs(c2.cy - c1.cy)).toBeGreaterThanOrEqual(2 * Math.max(c1.r, c2.r) + LABEL_BLOCK_H + 4)
    // 已移出与候选共用堆叠池 → 不得与任一候选同 y
    const rm = pointOf(w, 'rm1')
    expect([c1.cy, c2.cy]).not.toContain(rm.cy)
    // 只读参照区：同日两节点圆心距同样满足
    const r1 = pointOf(w, 'r1')
    const r2 = pointOf(w, 'r2')
    expect(Math.abs(r2.cy - r1.cy)).toBeGreaterThanOrEqual(2 * Math.max(r1.r, r2.r) + LABEL_BLOCK_H + 4)
  })

  it('4.4 缩放只等比拉伸：cy 差与 r 不变、SVG 尺寸 ×1.1（比值不变，放大不负责解拥挤）', async () => {
    const w = mountCanvas({ dates: ['2026-11-10'], memberNodes: SAME_DAY, candidateNodes: [], readonlyNodes: [], foldNodes: [] })
    const before = { gap: gapOf(w, 'd1', 'd2'), r: pointOf(w, 'd1').r, width: svgWidth(w), height: svgHeight(w) }
    await w.find('[data-testid="lane-zoom-in"]').trigger('click')
    const after = { gap: gapOf(w, 'd1', 'd2'), r: pointOf(w, 'd1').r, width: svgWidth(w), height: svgHeight(w) }
    expect(after.gap).toBe(before.gap)
    expect(after.r).toBe(before.r)
    expect(after.width).toBeCloseTo(before.width * 1.1, 6)
    expect(after.height).toBeCloseTo(before.height * 1.1, 6)
    expect(before.gap / (2 * before.r)).toBe(after.gap / (2 * after.r))
  })

  const dayList = (n: number) => Array.from({ length: n }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)

  it('4.8 同一天两个成员：两条标签都在场（标签框不得碰到同列下一个圆）', () => {
    const w = mountCanvas({
      dates: ['2026-11-10'],
      memberNodes: [
        node('m1', '2026-11-10', '第一个成员的长标题'),
        node('m2', '2026-11-10', '第二个成员的长标题'),
      ],
      candidateNodes: [],
      readonlyNodes: [],
      foldNodes: [],
    })
    // 两个成员各渲染自己的标签；若行距不足，上方标签会被 circleHit 判成压住
    // 下方圆而整条跳过（review A-R-1 的回归锚）
    const texts = w.findAll('text.lcc-label').map(el => el.text() ?? '')
    expect(texts.length).toBeGreaterThanOrEqual(2)
    expect(texts.some(v => v.includes('第一个'))).toBe(true)
    expect(texts.some(v => v.includes('第二个'))).toBe(true)
  })

  it('4.6 列宽有下限：天数多时撑宽画布横向滚动，而不是继续压缩列宽', () => {
    const seven = mountCanvas({
      dates: dayList(7),
      memberNodes: dayList(7).map((d, i) => node(`m${i}`, d, '一个比较长的成员标题')),
      candidateNodes: [],
      readonlyNodes: [],
      foldNodes: [],
    })
    const thirty = mountCanvas({
      dates: dayList(30),
      memberNodes: dayList(30).map((d, i) => node(`m${i}`, d, '一个比较长的成员标题')),
      candidateNodes: [],
      readonlyNodes: [],
      foldNodes: [],
    })
    // 7 天：均分（≈103px）够用 → 画布宽 = 视口宽 900，不出现横向滚动
    expect(svgWidth(seven)).toBeCloseTo(900, 0)
    // 30 天：均分只有 ≈24px → 抬到下限 64 → 画布撑宽（≈2098），由容器横向滚动承接
    const w30 = svgWidth(thirty)
    expect(w30).toBeGreaterThan(900)
    const col30 = (w30 - 150 - 28) / 30
    expect(col30).toBeGreaterThanOrEqual(64)
    expect(col30).toBeLessThan(70)
  })

  it('4.7 标签每行字数按列宽收窄 → 标签宽度不跨列（根治横向压邻节点）', () => {
    const dates = dayList(30)
    const w = mountCanvas({
      dates,
      memberNodes: dates.map((d, i) => node(`m${i}`, d, '一个比较长的成员标题')),
      candidateNodes: [],
      readonlyNodes: [],
      foldNodes: [],
    })
    const lines = w.findAll('text.lcc-label').map(el => (el.text() ?? '').replace('…', ''))
    expect(lines.length).toBeGreaterThan(0)
    // 列宽 64 / 字号 11 → 每行 ≤ floor((64-6)/11) = 5 字 → 标签框 ≈61px < 64px（不跨列）
    for (const line of lines) {
      expect(line.length).toBeLessThanOrEqual(5)
    }
  })
})

// ── 拖拽改归属（align-lane-correction-workface 第 4 节 ADDED） ────────────────

describe('LaneCorrectionCanvas · 拖拽改归属（align-lane-correction-workface 4.1）', () => {
  it('4.1-a 候选拖入成员区 → emit add（松手即落库，无草稿态）', async () => {
    const w = mountCanvas()
    await dragNode(w, 'c1', { x: 400, y: zoneY(w, 'member') })
    expect(w.emitted('add')).toEqual([['c1']])
    expect(w.emitted('remove')).toBeUndefined()
    expect(w.emitted('restore')).toBeUndefined()
    // 即时落库：没有「应用 / 取消」草稿态，也不叠弹窗
    expect(w.find('.app-dialog').exists()).toBe(false)
    expect(w.find('[data-testid="lane-anchor-confirm"]').exists()).toBe(false)
    w.unmount()
  })

  it('4.1-b 成员拖出成员区 → emit remove（不再归属任何泳道）', async () => {
    const w = mountCanvas()
    await dragNode(w, 'm1', { x: 400, y: zoneY(w, 'candidate') })
    expect(w.emitted('remove')).toEqual([['m1']])
    expect(w.emitted('add')).toBeUndefined()
    expect(w.emitted('restore')).toBeUndefined()
    w.unmount()
  })

  it('4.1-c 已移出节点拖回成员区 → emit restore（与左栏「加回来」同一路径）', async () => {
    const removed = [node('rm1', '2026-11-11', '本次移出节点', { topicId: '7' })]
    const w = mountCanvas({ removedNodes: removed })
    await dragNode(w, 'rm1', { x: 400, y: zoneY(w, 'member') })
    expect(w.emitted('restore')).toEqual([['rm1']])
    expect(w.emitted('add')).toBeUndefined()
    expect(w.emitted('remove')).toBeUndefined()
    w.unmount()
  })

  it('4.1-d 位移未过 4px 阈值：不启动拖拽（不出现 drag ghost）、不 emit 写事件', async () => {
    const w = mountCanvas()
    await pressAt(w, '[data-testid="lane-node-c1"]', { x: 400, y: 70 })
    await movePointer(w, { x: 403, y: 72 })
    expect(w.find('.lcc-drag-ghost').exists()).toBe(false)
    expect(w.find('[data-testid="lane-dropzone-member"]').classes()).not.toContain('is-drop-target')
    await releasePointer(w, { x: 403, y: 72 })
    expectNoWrites(w)
    w.unmount()
  })

  it('4.2 只读参照与「更早」折叠带节点不可拖：手势根本不启动，不 emit 任何写事件', async () => {
    const w = mountCanvas()
    expect(w.find('[data-testid="lane-node-r1"]').classes()).toContain('is-locked')
    expect(w.find('[data-testid="lane-node-r1"]').classes()).not.toContain('is-draggable')
    expect(w.find('[data-testid="lane-fold-node-f1"]').exists()).toBe(true)

    await pressAt(w, '[data-testid="lane-node-r1"]', { x: 400, y: 70 })
    await movePointer(w, { x: 400, y: zoneY(w, 'member') })
    // 不启动拖拽：没有 drag ghost，也没有分区高亮（不是「拖了但落点无效」）
    expect(w.find('.lcc-drag-ghost').exists()).toBe(false)
    expect(w.find('[data-testid="lane-dropzone-member"]').classes()).not.toContain('is-drop-target')
    await releasePointer(w, { x: 400, y: zoneY(w, 'member') })

    await pressAt(w, '[data-testid="lane-fold-node-f1"]', { x: 184, y: 70 })
    await movePointer(w, { x: 400, y: zoneY(w, 'member') })
    expect(w.find('.lcc-drag-ghost').exists()).toBe(false)
    await releasePointer(w, { x: 400, y: zoneY(w, 'member') })

    expectNoWrites(w)
    w.unmount()
  })

  it('4.3 落库期间（pendingIds）不可再次拖拽：不重复 emit', async () => {
    const w = mountCanvas({ pendingIds: ['c1'] })
    expect(w.find('[data-testid="lane-node-c1"]').classes()).toContain('is-pending')
    await pressAt(w, '[data-testid="lane-node-c1"]')
    await movePointer(w, { x: 400, y: zoneY(w, 'member') })
    expect(w.find('.lcc-drag-ghost').exists()).toBe(false)
    await releasePointer(w, { x: 400, y: zoneY(w, 'member') })
    expectNoWrites(w)
    w.unmount()
  })

  it('4.5 落库失败：节点回原分区 + 内联标错，不弹窗、不给去处选择器', async () => {
    const w = mountCanvas()
    const before = Number(w.find('[data-testid="lane-node-c1"] .lcc-point').attributes('cy'))
    await dragNode(w, 'c1', { x: 400, y: zoneY(w, 'member') })
    expect(w.emitted('add')).toEqual([['c1']])
    // 写请求期间：节点进 pendingIds（半透明 + 不可再拖）
    await w.setProps({ pendingIds: ['c1'] })
    expect(w.find('[data-testid="lane-node-c1"]').classes()).toContain('is-pending')
    // 写失败：props 未变 → 分区回滚；pending 结束 → 就地标错
    await w.setProps({ pendingIds: [] })
    const node = w.find('[data-testid="lane-node-c1"]')
    expect(node.classes()).toContain('is-error')
    expect(Number(node.find('.lcc-point').attributes('cy'))).toBe(before) // 仍在候选区
    expect(node.find('title').text()).toContain('落库失败')
    expect(w.find('[data-testid="lane-node-error-live"]').text()).toContain('落库失败')
    // 内联标错＝就地红描边 + 文案，不弹窗、不中断后续拖拽
    expect(w.find('.app-dialog').exists()).toBe(false)
    expect(w.find('select').exists()).toBe(false)
    // 失败后仍可继续拖其它节点（不整块禁用）
    await dragNode(w, 'm1', { x: 400, y: zoneY(w, 'candidate') })
    expect(w.emitted('remove')).toEqual([['m1']])
    w.unmount()
  })

  it('4.6 拖拽取消：Esc 与 pointercancel 都不落库（成员集合不变）', async () => {
    const w = mountCanvas()

    await pressAt(w, '[data-testid="lane-node-c1"]')
    await movePointer(w, { x: 400, y: zoneY(w, 'member') })
    expect(w.find('.lcc-drag-ghost').exists()).toBe(true) // 确认已越过阈值、拖拽真的起来了
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(w.find('.lcc-drag-ghost').exists()).toBe(false)
    await releasePointer(w, { x: 400, y: zoneY(w, 'member') })

    await pressAt(w, '[data-testid="lane-node-m1"]')
    await movePointer(w, { x: 400, y: zoneY(w, 'candidate') })
    surfaceOf(w).element.dispatchEvent(pointer('pointercancel', { x: 400, y: zoneY(w, 'candidate') }))
    await nextTick()
    await releasePointer(w, { x: 400, y: zoneY(w, 'candidate') })

    expectNoWrites(w)
    w.unmount()
  })

  it('4.x 拖拽期间的分区视觉：可落点高亮、只读参照区弱化为不可落点', async () => {
    const w = mountCanvas()
    await pressAt(w, '[data-testid="lane-node-c1"]')
    await movePointer(w, { x: 400, y: zoneY(w, 'member') })
    expect(w.find('[data-testid="lane-dropzone-member"]').classes()).toContain('is-drop-target')
    // 只读参照区永不可落点 → 与落点高亮明确不同（弱化），不是空白
    expect(w.find('[data-testid="lane-dropzone-readonly"]').classes()).toContain('is-drop-other')
    expect(w.find('[data-testid="lane-dropzone-candidate"]').classes()).not.toContain('is-drop-target')
    await releasePointer(w, { x: 400, y: zoneY(w, 'member') })
    w.unmount()
  })
})

// ── 候选分档的视觉编码（align-lane-correction-workface 5.1） ─────────────────

describe('LaneCorrectionCanvas · 候选分档只归一候选桶（align-lane-correction-workface 5.1）', () => {
  it('5.1 候选半径 / 填色按批内相对位置分档；成员尺寸与填色不读接近度', () => {
    const w = mountCanvas({
      memberNodes: [
        node('m-hi', '2026-11-10', '高接近成员', { topicId: '7', proximity: 0.95 }),
        node('m-lo', '2026-11-10', '低接近成员', { topicId: '7', proximity: 0.05 }),
      ],
      candidateNodes: [
        node('c-near', '2026-11-11', '批内最近候选', { proximity: 0.95, proximityKind: 'estimate' }),
        node('c-far', '2026-11-12', '批内最远候选', { proximity: 0.35, proximityKind: 'estimate' }),
      ],
      readonlyNodes: [],
      foldNodes: [],
    })
    const radiusOf = (id: string): number =>
      Number(w.find(`[data-testid="lane-node-${id}"]`).find('.lcc-point').attributes('r'))
    const fillOf = (id: string): string =>
      w.find(`[data-testid="lane-node-${id}"]`).attributes('style') ?? ''

    // 批内归一：最近者落在区间上限、最远者落在下限，肉眼可辨（不是窄区间）
    expect(radiusOf('c-near')).toBeCloseTo(7.4, 6)
    expect(radiusOf('c-far')).toBeCloseTo(5.9, 6)
    expect(fillOf('c-near')).toContain('--proximity-fill: 80%')
    expect(fillOf('c-far')).toContain('--proximity-fill: 18%')

    // 成员不进归一：接近度差 0.9 仍同尺寸、同填色（尺寸不编码成员远近）
    expect(radiusOf('m-hi')).toBe(radiusOf('m-lo'))
    expect(fillOf('m-hi')).toBe(fillOf('m-lo'))
    expect(fillOf('m-hi')).toContain('--proximity-fill: 0%')
  })

  it('5.1 候选批内相对位置变化不改成员尺寸（成员与候选不同形）', () => {
    const member = node('m1', '2026-11-10', '成员', { topicId: '7', proximity: 0.99 })
    const near = node('c1', '2026-11-11', '近候选', { proximity: 0.9, proximityKind: 'estimate' })
    const far = node('c2', '2026-11-12', '远候选', { proximity: 0.1, proximityKind: 'estimate' })

    const oneBatch = mountCanvas({ memberNodes: [member], candidateNodes: [near], readonlyNodes: [], foldNodes: [] })
    const twoBatch = mountCanvas({ memberNodes: [member], candidateNodes: [near, far], readonlyNodes: [], foldNodes: [] })
    const r = (w: ReturnType<typeof mountCanvas>, id: string): number =>
      Number(w.find(`[data-testid="lane-node-${id}"]`).find('.lcc-point').attributes('r'))

    expect(r(twoBatch, 'm1')).toBe(r(oneBatch, 'm1'))
    // 同一候选在更大批里的相对位置变了 → 半径收缩（归一确实只在候选桶里算）
    expect(r(twoBatch, 'c1')).toBeGreaterThan(r(twoBatch, 'c2'))
    oneBatch.unmount()
    twoBatch.unmount()
  })
})

// ── 5.5：画布高度吃满容器（align-lane-correction-workface 5.5 / R-3） ───────

describe('LaneCorrectionCanvas · 画布高度贴合容器（5.5）', () => {
  type Canvas = ReturnType<typeof mountCanvas>

  /**
   * 打桩容器可视高（画布贴合的实测口径是 `clientHeight`），再触发 window resize
   * 让组件重新 measure——与编排画布同名用例同一套做法。
   */
  async function setCanvasHeight(w: Canvas, height: number): Promise<void> {
    const el = surfaceOf(w).element as HTMLElement
    Object.defineProperty(el, 'clientHeight', { value: height, configurable: true })
    window.dispatchEvent(new Event('resize'))
    await nextTick()
  }

  function svgHeight(w: Canvas): number {
    return Number(w.find('[data-testid="lane-correction-canvas-svg"]').attributes('height'))
  }

  function memberCy(w: Canvas, id: string): number {
    return Number(w.find(`[data-testid="lane-node-${id}"]`).find('.lcc-point').attributes('cy'))
  }

  /** 成员区下缘（唯一 DOM 投影 = 成员 dropzone 的 y + height）。 */
  function memberZoneBottom(w: Canvas): number {
    const rect = w.find('[data-testid="lane-dropzone-member"]')
    return Number(rect.attributes('y')) + Number(rect.attributes('height'))
  }

  it('5.5-a 容器高 > 内容高 → svg 高度贴合容器（不退化成内容高 / 360 地板）', async () => {
    const w = mountCanvas()
    // 容器未测量时的高度 = 内容需求（本用例约 456，既高于 360 地板又远低于 900）
    const contentHeight = svgHeight(w)
    expect(contentHeight).toBeLessThan(900)

    await setCanvasHeight(w, 900)
    expect(svgHeight(w)).toBe(900)
    // 容器仍高于内容 → 继续贴合容器（不是某个固定值）
    await setCanvasHeight(w, 520)
    expect(svgHeight(w)).toBe(520)
    // viewBox 高度与像素高度同一口径（zoom = 1）
    expect(w.find('[data-testid="lane-correction-canvas-svg"]').attributes('viewBox')!.split(' ')[3]).toBe('520')
    w.unmount()
  })

  it('5.5-b 富余高度不改变成员区基线（成员点 cy 与成员区下缘与矮容器时一致）', async () => {
    const w = mountCanvas()
    await setCanvasHeight(w, 300)
    const short = { cy: memberCy(w, 'm1'), bottom: memberZoneBottom(w) }

    await setCanvasHeight(w, 900)
    // 富余 600px 全部落在画布下方（不把成员区往下推、不改行距）
    expect(svgHeight(w)).toBe(900)
    expect(memberCy(w, 'm1')).toBe(short.cy)
    expect(memberCy(w, 'm2')).toBe(memberCy(w, 'm1')) // 不同日成员同一基线
    expect(memberZoneBottom(w)).toBe(short.bottom)
    w.unmount()
  })
})

// ── 平移死区（align-lane-correction-workface R-6） ─────────────────────

describe('LaneCorrectionCanvas · 不可拖节点上也能起平移（R-6）', () => {
  it('R-6 在只读参照 / 「更早」折叠带节点上按下：不启动节点拖拽，但退化为画布平移', async () => {
    const w = mountCanvas()
    expect(w.find('[data-testid="lane-node-r1"]').classes()).toContain('is-locked')

    // 只读参照节点上按下 → 越过阈值后是平移（不是拖拽）
    await pressAt(w, '[data-testid="lane-node-r1"]', { x: 400, y: 70 })
    await movePointer(w, { x: 452, y: 122 })
    expect(surfaceOf(w).classes()).toContain('is-panning')
    expect(w.find('.lcc-drag-ghost').exists()).toBe(false)
    await releasePointer(w, { x: 452, y: 122 })
    expect(surfaceOf(w).classes()).not.toContain('is-panning')

    // 「更早」折叠带节点上按下 → 同样起平移
    await pressAt(w, '[data-testid="lane-fold-node-f1"]', { x: 184, y: 70 })
    await movePointer(w, { x: 236, y: 122 })
    expect(surfaceOf(w).classes()).toContain('is-panning')
    expect(w.find('.lcc-drag-ghost').exists()).toBe(false)
    await releasePointer(w, { x: 236, y: 122 })

    // 不可拖 ≠ 可写：任何归属写事件都不应发生
    expectNoWrites(w)
    w.unmount()
  })

  it('R-6 未移动鼠标时点击照常只聚焦（不破坏 onNodeClick 语义）', async () => {
    const w = mountCanvas()
    // 按下 + 原位松手：没有越过阈值 → 不算平移（lastPanAt 不置位，点击不被吞）
    await pressAt(w, '[data-testid="lane-node-r1"]', { x: 400, y: 70 })
    await releasePointer(w, { x: 400, y: 70 })
    expect(surfaceOf(w).classes()).not.toContain('is-panning')

    await w.find('[data-testid="lane-node-r1"]').trigger('click')
    expect(w.emitted('focus')).toEqual([['r1']])
    expectNoWrites(w)
    w.unmount()
  })
})
