import { describe, expect, it } from 'vitest'
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

  it('图例按新契约：候选大小=接近度（成员固定尺寸）+ 相关标志；无解说长句（A-9）', () => {
    const w = mountCanvas({ focusId: 'm1' })
    const legend = w.find('.lcc-legend')
    expect(legend.text().replace(/\s+/g, '')).toBe('成员候选大小=接近度虚线=首义⊘已移出相关')
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
})
