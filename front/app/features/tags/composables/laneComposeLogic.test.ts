import { describe, expect, it } from 'vitest'
import {
  boxesOverlap,
  buildRowIndex,
  layoutRows,
  rowMemberY,
  titleBudget,
  truncateByWidth,
  thresholdOf,
  unclassifiedIds,
  visibleScatter,
  LANE_ROW_SELF,
  LANE_MIN_COLUMN,
  MAX_SLOT_PER_DAY,
  MAX_TITLE_CHARS,
  TITLE_MIN_COLUMN,
  candidatePoolDayTotals,
  candidateSelection,
  centroidOf,
  centroidRecommendations,
  clampZoom,
  cosineFor,
  defaultMemberRadius,
  estimateTextWidth,
  laneSelectionState,
  laneSeriesFromDynamics,
  isDragBeyond,
  panScroll,
  placeLabels,
  placeRelatedTitles,
  pointAppearance,
  proximityOf,
  relatedNodeIds,
  relativeProximities,
  scaledPointRadius,
  truncateText,
  validateWorkbenchRange,
  zoomAnchorScroll,
  zoomStep,
  LANE_ZOOM_DEFAULT,
  LANE_ZOOM_MAX,
  LANE_ZOOM_MIN,
  type LaneSection,
} from './laneComposeLogic'

/**
 * 白盒分支表 §3.3（质心与推荐，M1–M12）。判据来自主线程 2026-10-31 定稿，
 * 不得自行放宽：均值容差 1e-9、阈值边界等号保留、同距按 id 升序。
 */

function section(id: string, embedding: number[], status?: 'candidate' | 'active' | 'archived'): LaneSection {
  return {
    id,
    embedding,
    persistentTopic: status ? { id: 't', label: '同名', status } : null,
  }
}

describe('laneComposeLogic · 质心（M1–M5、M10）', () => {
  it('M1 空已选：无质心、无推荐，不伪造距离', () => {
    const result = centroidRecommendations([section('1', [1, 0])], new Set<string>(), 0.3)
    expect(result.center).toBeNull()
    expect(result.unassigned).toEqual([])
    expect(result.active).toEqual([])
  })

  it('M2 单成员：质心就是该成员自己的向量', () => {
    const result = centroidRecommendations([section('1', [1, 0])], new Set(['1']), 0.3)
    expect(result.center).toEqual([1, 0])
  })

  it('M3 多成员：逐维均值（容差 1e-9），不是首个成员', () => {
    const pool = [section('1', [1, 0]), section('2', [0, 1])]
    const center = centroidOf(pool)!
    expect(center[0]!).toBeCloseTo(0.5, 9)
    expect(center[1]!).toBeCloseTo(0.5, 9)
    expect(Math.max(...center.map(v => Math.abs(v - 0.5)))).toBeLessThan(1e-9)
  })

  it('M4 向量长度不齐：只用与首个有效向量同长的成员参与平均', () => {
    const center = centroidOf([section('1', [1, 0, 0, 0, 0, 0, 0, 0]), section('2', [1, 1])])!
    expect(center).toHaveLength(8)
    // 只有 8 维那条有效 → 均值就是它自己；2 维那条长度不符，不参与
    expect(center[0]).toBeCloseTo(1, 9)
    expect(center.slice(1).every(v => v === 0)).toBe(true)
  })

  it('M5 含非有限值：该成员视为无效，不参与平均也不污染结果', () => {
    const center = centroidOf([section('1', [1, 0]), section('2', [Number.NaN, 1])])!
    expect(center[0]).toBeCloseTo(1, 9)
    expect(center[1]).toBeCloseTo(0, 9)
  })

  it('M10 焦点不影响质心：质心只由已选成员决定', () => {
    const pool = [section('1', [1, 0]), section('2', [0, 1])]
    const before = centroidRecommendations(pool, new Set(['1']), 0.3)
    // 「只看不动手」：焦点节点不在已选集合里 → 质心与推荐不变
    const after = centroidRecommendations(pool, new Set(['1']), 0.3)
    expect(after.center).toEqual(before.center)
    expect(after.unassigned.map(r => r.section.id)).toEqual(before.unassigned.map(r => r.section.id))
  })

  it('cosineFor：返回余弦距离（1 − cos）；长度不等 / 零范数 / 缺参一律 null', () => {
    expect(cosineFor([0, 0], [1, 0])).toBeNull()
    expect(cosineFor([1, 0], [1, 0, 0])).toBeNull()
    expect(cosineFor(null, [1, 0])).toBeNull()
    // 同向 → 距离 0；反向 → 距离 2
    expect(cosineFor([1, 0], [1, 0])).toBeCloseTo(0, 12)
    expect(cosineFor([1, 0], [-1, 0])).toBeCloseTo(2, 12)
  })
})

describe('laneComposeLogic · 推荐筛选与排序（M6–M9、M11、M12）', () => {
  it('M6 距离等于阈值：保留（≤ 阈值）', () => {
    const pool = [section('1', [1, 0]), section('2', [0.5, 0.5])]
    // 阈值取「实际算出的距离」本身 = 边界（避免浮点字面量误差）
    const boundary = cosineFor([1, 0], [0.5, 0.5])!
    const result = centroidRecommendations(pool, new Set(['1']), boundary)
    expect(result.unassigned.map(r => r.section.id)).toEqual(['2'])
  })

  it('M7 距离超过阈值：过滤掉', () => {
    const pool = [section('1', [1, 0]), section('2', [0.5, 0.5])]
    const boundary = cosineFor([1, 0], [0.5, 0.5])!
    const result = centroidRecommendations(pool, new Set(['1']), boundary - 1e-9)
    expect(result.unassigned).toEqual([])
  })

  it('M8 距离相同：按 section id 升序稳定排序', () => {
    const pool = [
      section('0', [1, 0]),
      section('3', [1, 0]),
      section('1', [1, 0]),
      section('2', [1, 0]),
    ]
    const result = centroidRecommendations(pool, new Set(['0']), 0.3)
    expect(result.unassigned.map(r => r.section.id)).toEqual(['1', '2', '3'])
  })

  it('M9 分组：candidate 进优先组，active 进次级只读组', () => {
    const pool = [
      section('0', [1, 0]),
      section('1', [1, 0], 'candidate'),
      section('2', [1, 0], 'active'),
    ]
    const result = centroidRecommendations(pool, new Set(['0']), 0.3)
    expect(result.unassigned.map(r => r.section.id)).toEqual(['1'])
    expect(result.active.map(r => r.section.id)).toEqual(['2'])
  })

  it('M11 增删重算：加入推荐项后按新成员集合重算，被加入项从待选移除', () => {
    const pool = [section('1', [1, 0]), section('2', [0.99, 0.14])]
    const first = centroidRecommendations(pool, new Set(['1']), 0.3)
    expect(first.unassigned.map(r => r.section.id)).toEqual(['2'])

    const chosen = new Set(['1', '2'])
    const second = centroidRecommendations(pool, chosen, 0.3)
    expect(second.center![0]).toBeCloseTo(0.995, 9)
    expect(second.unassigned).toEqual([])
  })

  it('M12 无向量 section：不参与距离计算也不报错', () => {
    const pool = [
      section('1', [1, 0]),
      { id: '2', embedding: [] as number[] },
      { id: '3', embedding: null as unknown as number[] },
    ]
    const result = centroidRecommendations(pool, new Set(['1']), 0.3)
    expect(result.unassigned.map(r => r.section.id)).toEqual([])
    expect(result.center).toEqual([1, 0])
  })
})

describe('laneComposeLogic · 选择状态与整组操作（S1–S6）', () => {
  it('S1 空组：unchecked（不是 indeterminate）', () => {
    const state = laneSelectionState(new Set<string>(), [])
    expect(state).toEqual({ count: 0, total: 0, checked: false, mixed: false })
  })

  it('S2 全选：checked', () => {
    const state = laneSelectionState(new Set(['a', 'b']), ['a', 'b'])
    expect(state.checked).toBe(true)
    expect(state.mixed).toBe(false)
  })

  it('S3 半选：indeterminate', () => {
    const state = laneSelectionState(new Set(['a']), ['a', 'b', 'c'])
    expect(state.count).toBe(1)
    expect(state.total).toBe(3)
    expect(state.checked).toBe(false)
    expect(state.mixed).toBe(true)
  })

  it('S4 整组补齐：与组外已有选择合并去重，不截条数', () => {
    const group = Array.from({ length: 25 }, (_, i) => `g${i}`)
    const before = new Set(['extra1', 'extra2'])
    const next = candidateSelection(before, group, true)
    expect(next.size).toBe(27)
    expect(next.has('extra1')).toBe(true)
    expect(next.has('g24')).toBe(true)
    // 幂等：再勾一次成员数不变
    expect(candidateSelection(next, group, true).size).toBe(27)
  })

  it('S6 取消组：只移除组内成员，组外人工补选保留', () => {
    const picked = candidateSelection(new Set(['extra']), ['a', 'b', 'c'], true)
    const cancelled = candidateSelection(picked, ['a', 'b', 'c'], false)
    expect([...cancelled]).toEqual(['extra'])
  })
})

describe('laneComposeLogic · 视觉映射（避让 / 截断 / 半径）', () => {
  it('标题截断到上限并补省略号，不截断时原样', () => {
    expect(truncateText('短标题', 10)).toBe('短标题')
    const long = truncateText('这是一个非常非常长的候选话题标题需要被截断', 10)
    expect(long).toHaveLength(10)
    expect(long.endsWith('…')).toBe(true)
  })

  it('文字宽度估算随长度单调递增', () => {
    expect(estimateTextWidth('中文中文', 12)).toBeGreaterThan(estimateTextWidth('中文', 12))
    expect(estimateTextWidth('中文', 12)).toBeGreaterThan(estimateTextWidth('ab', 12))
    expect(estimateTextWidth('', 12)).toBe(0)
  })

  it('标题避让：不重叠，且拥挤时宁可少画', () => {
    const cands = [
      { id: 'a', x: 100, y: 100, r: 4, w: 60, h: 12, priority: 10, text: 'AAAA' },
      { id: 'b', x: 104, y: 102, r: 4, w: 60, h: 12, priority: 5, text: 'BBBB' },
      { id: 'c', x: 400, y: 400, r: 4, w: 60, h: 12, priority: 1, text: 'CCCC' },
    ]
    const kept = placeLabels(cands).filter(x => !x.skipped)
    for (let i = 0; i < kept.length; i++) {
      for (let j = i + 1; j < kept.length; j++) {
        expect(boxesOverlap(kept[i]!.box!, kept[j]!.box!)).toBe(false)
      }
    }
    const only = placeLabels(cands, { orders: ['right'] })
    expect(only.filter(x => !x.skipped)).toHaveLength(2)
    expect(only.find(x => x.id === 'b')!.skipped).toBe(true)
  })

  it('标题避让：越界候选直接跳过（不越出画布）', () => {
    const placed = placeLabels(
      [{ id: 'a', x: 10, y: 10, r: 4, w: 60, h: 12, priority: 1, text: 'AAA' }],
      { minX: 0, maxX: 50, minY: 0, maxY: 100 },
    )
    expect(placed[0]!.skipped).toBe(true)
  })

  it('V4 半径下限：普通点 ≥3px，只读参照 ≥2.5px', () => {
    expect(pointAppearance(undefined, 0.3, false, false, 5, 30).radius).toBeGreaterThanOrEqual(3)
    expect(pointAppearance(undefined, 0.3, false, true, 5, 30).radius).toBeGreaterThanOrEqual(2.5)
  })

  it('V5 已选点半径恒严格大于推荐点半径', () => {
    for (const [columnWidth, baseRadius] of [[8, 2], [27, 5.4], [115, 7]] as const) {
      const selected = pointAppearance(0.001, 0.3, true, false, baseRadius, columnWidth)
      const recommend = pointAppearance(0.001, 0.3, false, false, baseRadius, columnWidth)
      expect(selected.radius).toBeGreaterThan(recommend.radius)
    }
  })

  it('接近程度只表达远近：越近半径越大、填色越深，但不是置信度', () => {
    const near = pointAppearance(0.05, 0.3, false, false, 5, 30)
    const far = pointAppearance(0.25, 0.3, false, false, 5, 30)
    expect(near.radius).toBeGreaterThan(far.radius)
    expect(near.strength).toBeGreaterThan(far.strength)
    expect(near.proximity!).toBeGreaterThan(far.proximity!)
    // 未推荐点与只读参照没有接近程度
    expect(pointAppearance(undefined, 0.3, false, false, 5, 30).proximity).toBeNull()
    expect(pointAppearance(0.1, 0.3, false, true, 5, 30).proximity).toBeNull()
  })
})

describe('laneComposeLogic · 相关节点（1 跳原关系）', () => {
  const relations = [
    { fromId: '1', toId: '2' },
    { fromId: '2', toId: '3' },
    { fromId: '3', toId: '4' },
  ]

  it('R1 无已选：无相关节点', () => {
    expect(relatedNodeIds(relations, new Set()).size).toBe(0)
  })

  it('R2 已选节点的直接邻居进入相关集合', () => {
    expect([...relatedNodeIds(relations, new Set(['2']))]).toEqual(['1', '3'])
  })

  it('R3 不做传递闭包：隔一层的节点不算相关', () => {
    const related = relatedNodeIds(relations, new Set(['1']))
    expect(related.has('2')).toBe(true)
    expect(related.has('3')).toBe(false)
    expect(related.has('4')).toBe(false)
  })

  it('R4 已选成员自身不在相关集合里（用户要的是「其他」相关节点）', () => {
    const related = relatedNodeIds(relations, new Set(['1', '2']))
    expect(related.has('1')).toBe(false)
    expect(related.has('2')).toBe(false)
    expect(related.has('3')).toBe(true)
  })

  it('R5 畸形边（缺端点）不产出脏 id', () => {
    const related = relatedNodeIds([{ fromId: '1', toId: '' }], new Set(['1']))
    expect(related.size).toBe(0)
  })
})

describe('laneComposeLogic · 拖拽平移', () => {
  it('P1 拖动内容：滚动位置与指针位移反向', () => {
    expect(panScroll({ x: 100, y: 200 }, { x: 60, y: 200 }, { left: 0, top: 0 })).toEqual({ left: 40, top: 0 })
    expect(panScroll({ x: 100, y: 200 }, { x: 160, y: 240 }, { left: 50, top: 30 })).toEqual({ left: -10, top: -10 })
  })

  it('P2 位移未超阈值算「点」不算「拖」', () => {
    expect(isDragBeyond({ x: 10, y: 10 }, { x: 12, y: 11 })).toBe(false)
    expect(isDragBeyond({ x: 10, y: 10 }, { x: 30, y: 10 })).toBe(true)
    expect(isDragBeyond({ x: 10, y: 10 }, { x: 10, y: 10 }, 0)).toBe(false)
  })
})

describe('laneComposeLogic · 相关标题落位（向下错行）', () => {
  const cand = (id: string, x: number, y: number) => ({ id, x, y, r: 3, w: 90, h: 9, priority: 0, text: id })

  it('T1 只放节点下方（y 在节点之下）', () => {
    const placed = placeRelatedTitles([cand('a', 100, 200)], { minX: 0, maxX: 500, minY: 0, maxY: 400 })
    expect(placed[0]!.skipped).toBeUndefined()
    expect(placed[0]!.box!.y).toBeGreaterThan(200)
  })

  it('T2 相邻列撞上时向下错行（不压字、也不丢）', () => {
    const placed = placeRelatedTitles([cand('a', 100, 200), cand('b', 133, 200)], { lines: 2, lineHeight: 16, minX: 0, maxX: 500, minY: 0, maxY: 400 })
    expect(placed.every(p => !p.skipped)).toBe(true)
    const ys = placed.map(p => p.box!.y).sort((x, y) => x - y)
    expect(ys[1]! - ys[0]!).toBeGreaterThanOrEqual(16)
    expect(boxesOverlap(placed[0]!.box!, placed[1]!.box!)).toBe(false)
  })

  it('T3 错行用尽仍撞上 → 宁缺勿叠（跳过）', () => {
    const placed = placeRelatedTitles([cand('a', 100, 200), cand('b', 133, 200), cand('c', 166, 200)], { lines: 1, lineHeight: 16, minX: 0, maxX: 500, minY: 0, maxY: 400 })
    expect(placed.filter(p => p.skipped).length).toBe(1)
  })

  it('T4 越出画布边界直接跳过', () => {
    const placed = placeRelatedTitles([cand('a', 10, 200)], { minX: 0, maxX: 500, minY: 0, maxY: 400 })
    expect(placed[0]!.skipped).toBe(true)
  })
})

describe('laneComposeLogic · 画布缩放（档位 / 步进 / 锚点 / 半径下限）', () => {
  it('Z1 档位：0.5× – 3×，默认 100%，越界夹回边界', () => {
    expect(LANE_ZOOM_MIN).toBe(0.5)
    expect(LANE_ZOOM_MAX).toBe(3)
    expect(LANE_ZOOM_DEFAULT).toBe(1)
    expect(clampZoom(0.2)).toBe(0.5)
    expect(clampZoom(9)).toBe(3)
    expect(clampZoom(1.4)).toBe(1.4)
    // 非法值回落到 100%（不产生 NaN 缩放）
    expect(clampZoom(Number.NaN)).toBe(1)
  })

  it('Z2 步进：向上滚放大、向下滚缩小，一格滚轮 = 1.1 步进', () => {
    expect(zoomStep(1, -100)).toBeCloseTo(1.1, 10)
    expect(zoomStep(1.1, 100)).toBeCloseTo(1, 10)
    // 触控板小增量 → 更细的步子（不是一格就飞出去）
    const fine = zoomStep(1, -10)
    expect(fine).toBeGreaterThan(1)
    expect(fine).toBeLessThan(1.1)
    // deltaY = 0 → 不动
    expect(zoomStep(1.5, 0)).toBe(1.5)
    // 连续放大到顶后停在 3×，缩小到底停在 0.5×
    let z = 1
    for (let i = 0; i < 40; i++) z = zoomStep(z, -100)
    expect(z).toBe(LANE_ZOOM_MAX)
    for (let i = 0; i < 40; i++) z = zoomStep(z, 100)
    expect(z).toBe(LANE_ZOOM_MIN)
  })

  it('Z3 锚点：缩放后光标下的内容尽量不动', () => {
    // 光标在视口 100 处、内容滚动 0、1× → 2×：内容坐标 100，新滚动 = 100×2 − 100 = 100
    expect(zoomAnchorScroll(100, 0, 1, 2)).toBeCloseTo(100, 10)
    // 缩回 0.5×：内容坐标 (100 + 100) / 2 = 100，新滚动 = 100×0.5 − 100 = −50 → 夹回 0
    expect(zoomAnchorScroll(100, 100, 2, 0.5)).toBe(0)
    // 不缩放时滚动位置不变
    expect(zoomAnchorScroll(100, 40, 1, 1)).toBeCloseTo(40, 10)
  })

  it('Z4 半径下限：缩小时补偿，渲染半径仍 ≥3px；100% 时不变', () => {
    // 100%：原样（不改既有视觉）
    expect(scaledPointRadius(3, 1)).toBe(3)
    expect(scaledPointRadius(6.4, 1)).toBe(6.4)
    // 0.5×：逻辑半径补到 6，渲染半径 = 6×0.5 = 3px
    expect(scaledPointRadius(3, 0.5)).toBeCloseTo(6, 10)
    expect(scaledPointRadius(3, 0.5) * 0.5).toBeCloseTo(3, 10)
    // 放大时不需要补偿（点变大不算违规）
    expect(scaledPointRadius(3, 2)).toBe(3)
    // 只读参照的下限是 2.5px
    expect(scaledPointRadius(2.5, 0.5, 2.5)).toBeCloseTo(5, 10)
  })
})

describe('laneComposeLogic · unify-lane-workbench 点形统一（B1 / E8 / E13）', () => {
  it('memberAppearanceIgnoresDistance：成员半径固定，不随距离/接近度变', () => {
    const near = pointAppearance(0.01, 0.3, true, false, 5, 30)
    const far = pointAppearance(0.29, 0.3, true, false, 5, 30)
    const noDistance = pointAppearance(null, 0.3, true, false, 5, 30)
    expect(near.radius).toBe(far.radius)
    expect(far.radius).toBe(noDistance.radius)
    // 成员实心（实心＋✓表达选中）
    expect(near.solid).toBe(true)
  })

  it('memberRadius 覆盖：修正画布的固定成员尺寸仍不读接近度', () => {
    const a = pointAppearance(0.01, 0.3, true, false, 3, 120, { memberRadius: 5.5 })
    const b = pointAppearance(0.29, 0.3, true, false, 3, 120, { memberRadius: 5.5 })
    expect(a.radius).toBe(5.5)
    expect(b.radius).toBe(5.5)
  })

  it('scatterRadius / readonlyRadius 覆盖：缺省行为不变，显式传值时只改该档半径', () => {
    // 缺省（修正画布）：散料 = min(8, baseRadius × 0.8)、只读 = max(2.5, baseRadius × 0.65)
    expect(pointAppearance(undefined, 0.3, false, false, 7.3, 52).radius).toBeCloseTo(5.84, 10)
    expect(pointAppearance(0.1, 0.3, false, true, 7.3, 52).radius).toBeCloseTo(4.745, 10)
    // 编排画布提档：散料 11px 直径、只读梯子不跟着放大
    expect(pointAppearance(undefined, 0.3, false, false, 7.3, 52, { scatterRadius: 5.5 }).radius).toBe(5.5)
    expect(pointAppearance(0.1, 0.3, false, true, 7.3, 52, { readonlyRadius: 2.75 }).radius).toBe(2.75)
    // 覆盖只认散料剖面：推荐点仍按接近度映射，不被散料尺寸吞掉
    const recommend = pointAppearance(0, 0.3, false, false, 7.3, 52, { scatterRadius: 5.5, memberRadius: 7.5 })
    expect(recommend.radius).toBeGreaterThan(5.5)
    expect(recommend.radius).toBeLessThanOrEqual(7.5)
  })

  it('候选半径随接近度单调，且仍小于成员半径；缩放下限保留', () => {
    const near = pointAppearance(0.02, 0.3, false, false, 5, 30)
    const far = pointAppearance(0.28, 0.3, false, false, 5, 30)
    expect(near.radius).toBeGreaterThan(far.radius)
    expect(far.radius).toBeGreaterThanOrEqual(3)
    const member = pointAppearance(0.02, 0.3, true, false, 5, 30)
    expect(member.radius).toBeGreaterThan(near.radius)
    // 渲染半径下限：缩小时逻辑半径补偿（E13）
    expect(scaledPointRadius(far.radius, 0.5) * 0.5).toBeGreaterThanOrEqual(3)
  })

  it('independentNodeFlags：移出/首义/焦点/相关/弱化是正交标志，不相互顶掉', () => {
    const app = pointAppearance(null, 0.3, true, false, 5, 30, { removed: true, anchor: true, focus: true, related: false, dimmed: false })
    expect(app.removed).toBe(true)
    expect(app.anchor).toBe(true)
    expect(app.focus).toBe(true)
    expect(app.related).toBe(false)
    expect(app.dimmed).toBe(false)
    // 只读更弱更小：半径下限 2.5、无接近度
    const readonlyApp = pointAppearance(0.1, 0.3, false, true, 5, 30, { dimmed: true })
    expect(readonlyApp.radius).toBeGreaterThanOrEqual(2.5)
    expect(readonlyApp.radius).toBeLessThan(pointAppearance(0.1, 0.3, false, false, 5, 30).radius)
    expect(readonlyApp.proximity).toBeNull()
    expect(readonlyApp.dimmed).toBe(true)
    expect(readonlyApp.solid).toBe(false)
  })

  it('E8：distance=0 接近度 1；阈值含等号；阈值+ε 不进推荐；null 不显示', () => {
    expect(pointAppearance(0, 0.3, false, false, 5, 30).proximity).toBe(1)
    expect(pointAppearance(0.3, 0.3, false, false, 5, 30).proximity).not.toBeNull()
    expect(pointAppearance(0.3 + 1e-6, 0.3, false, false, 5, 30).proximity).toBeNull()
    expect(pointAppearance(null, 0.3, false, false, 5, 30).proximity).toBeNull()
  })
})

describe('laneComposeLogic · 成员半径与行距（fix-lane-row-gap-zoom 1.1 / 1.2）', () => {
  it('列宽 8：半径有下限（≥3），不因窄列宽缩到看不见', () => {
    const r = defaultMemberRadius(8)
    expect(r).toBeGreaterThanOrEqual(3)
    expect(r).toBeCloseTo(3.68, 10) // cap = max(3, min(16, 8 × 0.46))
  })

  it('列宽 20 / 46 / 80：单调不降，且上限 13.6', () => {
    const r20 = defaultMemberRadius(20)
    const r46 = defaultMemberRadius(46)
    const r80 = defaultMemberRadius(80)
    expect(r20).toBeLessThanOrEqual(r46)
    expect(r46).toBeLessThanOrEqual(r80)
    expect(r46).toBeCloseTo(13.6, 10)
    expect(r80).toBeCloseTo(13.6, 10) // cap 封顶 16 → 基准半径封顶 13.6
    expect(r20).toBeCloseTo(9.2, 10)
    for (const r of [r20, r46, r80]) expect(r).toBeLessThanOrEqual(13.6)
  })

  it('与 pointAppearance 默认路径同源：成员半径恒等于 defaultMemberRadius', () => {
    for (const width of [8, 20, 46, 80]) {
      const member = pointAppearance(0.1, 0.3, true, false, 4, width)
      expect(member.radius).toBe(defaultMemberRadius(width))
    }
  })
})

describe('laneComposeLogic · relatedNodeIds 推荐并集（B2 / B3）', () => {
  const relations = [
    { fromId: 'A', toId: 'D' },
    { fromId: 'D', toId: 'E' },
    { fromId: 'A', toId: 'A2' },
  ]

  it('focusedRelatedUnionIsOneHop：一跳 ∪ 推荐，去重、排除种子、不传递到两跳', () => {
    const related = relatedNodeIds(relations, new Set(['A']), ['F', 'F', 'A', 'E'])
    expect([...related].sort()).toEqual(['A2', 'D', 'E', 'F'])
    // E 只因推荐进入（它同时是两跳节点——并集口径下允许；但无推荐时不进入：
    expect(relatedNodeIds(relations, new Set(['A'])).has('E')).toBe(false)
  })

  it('clearFocusRestoresDefault：无焦点（空种子）时只剩推荐集；空推荐不造高亮', () => {
    expect([...relatedNodeIds(relations, new Set(), ['F'])]).toEqual(['F'])
    expect(relatedNodeIds(relations, new Set(['A'])).size).toBe(2)
    expect(relatedNodeIds(relations, new Set(), []).size).toBe(0)
  })

  it('不改所选：并集只读，不碰种子集合', () => {
    const seeds = new Set(['A'])
    relatedNodeIds(relations, seeds, ['F'])
    expect([...seeds]).toEqual(['A'])
  })
})

describe('laneComposeLogic · 接近度换算（B4）', () => {
  it('距离 0 / 0.2 / null → 接近度 1 / 0.8 / null；0 保留', () => {
    expect(proximityOf(0)).toBe(1)
    expect(proximityOf(0.2)).toBeCloseTo(0.8, 12)
    expect(proximityOf(null)).toBeNull()
    expect(proximityOf(undefined)).toBeNull()
    expect(proximityOf(Number.NaN)).toBeNull()
    expect(proximityOf(1)).toBe(0)
  })
})

describe('laneComposeLogic · 工作面曲线纯函数（B7 / B8、E1 / E2 / E5 / E6 / E7 / E15）', () => {
  it('laneSeriesFromDynamics：只取目标泳道，缺泳道为 null 不造全 0（S3.2 / E2）', () => {
    const lanes = [
      { topicId: '7', timeline: [{ date: '2026-11-01', clueCount: 3 }, { date: '2026-11-02' }] },
      { topicId: '8', timeline: [{ date: '2026-11-01', clueCount: 99 }] },
    ]
    const series = laneSeriesFromDynamics(lanes, '7')!
    expect(series).toEqual([
      { date: '2026-11-01', value: 3 },
      { date: '2026-11-02', value: null },
    ])
    // 其它泳道增减不影响目标曲线（B7 编辑侧）
    const grown = laneSeriesFromDynamics([...lanes, { topicId: '9', timeline: [{ date: '2026-11-01', clueCount: 50 }] }], '7')!
    expect(grown).toEqual(series)
    expect(laneSeriesFromDynamics(lanes, '404')).toBeNull()
  })

  it('candidatePoolDayTotals：排 active、含无归属与 candidate、同日求和、ID 去重（B7 / E15）', () => {
    const sections = [
      { id: '1', periodDate: '2026-11-01T00:00:00Z', clueCount: 5 },
      { id: '2', periodDate: '2026-11-01', clueCount: 3, persistentTopicId: '9', persistentTopic: { status: 'candidate' } },
      { id: '1', periodDate: '2026-11-01', clueCount: 5 }, // 重复 ID 不双计
      { id: '3', periodDate: '2026-11-01', clueCount: 12, persistentTopicId: '7', persistentTopic: { status: 'active' } }, // active 排除
      { id: '4', periodDate: '2026-11-02', clueCount: 2, persistentTopicId: '11', persistentTopic: { status: 'archived' } }, // 归档排除
    ]
    const totals = candidatePoolDayTotals(sections)
    expect(totals.get('2026-11-01')).toBe(8)
    expect(totals.get('2026-11-02')).toBeUndefined() // 全被排除的日期不造 0 条目？——该日无应计项
  })

  it('poolSeriesPropagatesUnknown：缺 clueCount / 归属状态未知 → 该日 null；已知 0 保留（B8 / E1）', () => {
    const sections = [
      { id: '1', periodDate: '2026-11-01', clueCount: 0 }, // 已知 0
      { id: '2', periodDate: '2026-11-02', clueCount: 2 },
      { id: '3', periodDate: '2026-11-02' }, // 应计项缺计数 → 整日未知
      { id: '4', periodDate: '2026-11-03', clueCount: 2, persistentTopicId: '9' }, // 带归属但状态未知
      { id: '5', periodDate: '2026-11-04', clueCount: 1, persistentTopicId: '7', persistentTopic: { status: 'active' } },
    ]
    const totals = candidatePoolDayTotals(sections)
    expect(totals.get('2026-11-01')).toBe(0)
    expect(totals.get('2026-11-02')).toBeNull()
    expect(totals.get('2026-11-03')).toBeNull()
    expect(totals.has('2026-11-04')).toBe(false) // 全排除日：无条目（调用方在轴上映射为已知 0 池
  })

  it('E5：空池 / 只有 active → 可选总量 0，active 参照不计曲线', () => {
    expect(candidatePoolDayTotals([]).size).toBe(0)
    const onlyActive = [{ id: '1', periodDate: '2026-11-01', clueCount: 20, persistentTopicId: '7', persistentTopic: { status: 'active' } }]
    expect(candidatePoolDayTotals(onlyActive).has('2026-11-01')).toBe(false)
  })

  it('validateWorkbenchRange：闭区间上限、下界与倒置（E6 / E7 / I1）', () => {
    // 1 天与 365 天闭区间可用；366 天拒绝（三处上限一致的前端半边）
    expect(validateWorkbenchRange('2026-01-01', '2026-01-01', 365)).toBeNull()
    expect(validateWorkbenchRange('2025-12-17', '2026-12-16', 365)).toBeNull()
    expect(validateWorkbenchRange('2025-12-16', '2026-12-16', 365)).toContain('不能超过 365 天')
    // 非法：倒置 / 格式 / 非真实日历日
    expect(validateWorkbenchRange('2026-12-02', '2026-12-01', 365)).toContain('不能晚于')
    expect(validateWorkbenchRange('2026-2-1', '2026-12-01', 365)).toContain('有效')
    expect(validateWorkbenchRange('2026-02-30', '2026-12-01', 365)).toContain('有效')
    // 取数下界：latest − 364 可用、−365 拒绝（E7）
    expect(validateWorkbenchRange('2025-12-17', '2026-12-16', 365, '2026-12-16')).toBeNull()
    const late = validateWorkbenchRange('2025-12-16', '2025-12-20', 365, '2026-12-16')
    expect(late).toContain('早于支持的取数窗')
    // 跨月/闰日合法（E7）
    expect(validateWorkbenchRange('2024-02-29', '2024-03-01', 365, null)).toBeNull()
  })
})

// ── 行式泳道画布（rework-lane-compose-swimlane-canvas） ──────────────────────
describe('laneComposeLogic · 行归属（1.1）', () => {
  const chains = [
    { key: 'A:1', topicLabel: '链 A', sectionIds: ['1', '2', '3'] },
    { key: 'B:4', topicLabel: '链 B', sectionIds: ['4', '5'] },
  ]
  const windowIds = ['1', '2', '3', '4', '5', '6', '7']

  it('同链多成员同属一行；行序按服务端候选链顺序', () => {
    const index = buildRowIndex(chains, windowIds)
    expect(index.rowOf.get('1')).toBe(1)
    expect(index.rowOf.get('2')).toBe(1)
    expect(index.rowOf.get('3')).toBe(1)
    expect(index.rowOf.get('4')).toBe(2)
    expect(index.rowOf.get('5')).toBe(2)
    expect(index.chainRows.map(r => [r.row, r.label])).toEqual([[1, '链 A'], [2, '链 B']])
    expect(index.floorRow).toBe(3)
  })

  it('不在任何链 → 地板行（含挂着 candidate 话题但未成链的节点）', () => {
    const index = buildRowIndex(chains, windowIds)
    expect(index.rowOf.get('6')).toBe(index.floorRow)
    expect(index.rowOf.get('7')).toBe(index.floorRow)
  })

  it('本次泳道成员优先级最高：命中即归第 0 行，不改链的身份', () => {
    const index = buildRowIndex(chains, windowIds, new Set(['2', '6']))
    expect(index.rowOf.get('2')).toBe(LANE_ROW_SELF)
    expect(index.rowOf.get('1')).toBe(1) // 同链其它成员仍在原行
    expect(index.rowOf.get('6')).toBe(LANE_ROW_SELF)
    expect(index.chainRows[0]!.ids).toEqual(['1', '2', '3']) // 链成员表不因渲染覆盖而缩水
  })

  it('窗口外成员忽略；重复出现的成员取第一条链（服务端顺序）', () => {
    const index = buildRowIndex([
      { key: 'A:1', sectionIds: ['1', '999'] },
      { key: 'B:1', sectionIds: ['1', '2'] },
    ], ['1', '2'])
    expect(index.rowOf.get('999')).toBeUndefined()
    expect(index.rowOf.get('1')).toBe(1)
    expect(index.rowOf.get('2')).toBe(2)
  })
})

describe('laneComposeLogic · 行几何（1.2）', () => {
  const dates = ['2026-11-06', '2026-11-07', '2026-11-08']
  const rowIndex = new Map([
    ['a', 1], ['b', 1], ['c', 1], ['d', 1], ['e', 1], ['f', 1], ['g', 1],
  ])

  it('同日 1 条：行高 = 标签 32 + 1 槽 24 + 行间 26', () => {
    const [row] = layoutRows(rowIndex, dates, new Map([[1, [{ id: 'a', date: dates[0]! }]]]))
    expect(row!.slots).toBe(1)
    expect(row!.height).toBe(32 + 24 + 26)
    expect(row!.top).toBe(0)
    expect(row!.slotOf.get('a')).toBe(0)
    expect(rowMemberY(row!, 0)).toBe(32 + 12)
  })

  it('同日 3 条：三条各占一槽（不折叠成计数点），行高随槽数增长', () => {
    const rows = layoutRows(rowIndex, dates, new Map([[1, [
      { id: 'a', date: dates[0]! },
      { id: 'b', date: dates[0]! },
      { id: 'c', date: dates[0]! },
    ]]]))
    expect(rows[0]!.slots).toBe(3)
    expect(rows[0]!.height).toBe(32 + 3 * 24 + 26)
    expect([...rows[0]!.slotOf.entries()].sort((x, y) => x[1] - y[1]).map(([id]) => id)).toEqual(['a', 'b', 'c'])
    const ys = ['a', 'b', 'c'].map(id => rowMemberY(rows[0]!, rows[0]!.slotOf.get(id)!))
    expect(ys[1]! - ys[0]!).toBe(24)
    expect(ys[2]! - ys[1]!).toBe(24)
  })

  it('同日 7 条：封顶 6 槽，多的进 +N 计数（展开后不再封顶）', () => {
    const members = ['a', 'b', 'c', 'd', 'e', 'f', 'g'].map(id => ({ id, date: dates[0]! }))
    const capped = layoutRows(rowIndex, dates, new Map([[1, members]]))
    expect(capped[0]!.slots).toBe(MAX_SLOT_PER_DAY)
    expect(capped[0]!.height).toBe(32 + MAX_SLOT_PER_DAY * 24 + 26)
    expect(capped[0]!.slotOf.size).toBe(6)
    expect(capped[0]!.hiddenIds).toEqual(['g'])
    expect(capped[0]!.overflowByDate.get(dates[0]!)).toBe(1)

    const expanded = layoutRows(rowIndex, dates, new Map([[1, members]]), {
      expanded: new Set([`1:${dates[0]}`]),
    })
    expect(expanded[0]!.slots).toBe(7)
    expect(expanded[0]!.slotOf.size).toBe(7)
    expect(expanded[0]!.hiddenIds).toEqual([])
    expect(expanded[0]!.overflowByDate.size).toBe(0)
  })

  it('多行堆叠：top 逐行累计；显式行序可占位空行（本次泳道行预留 2 槽）', () => {
    const rows = layoutRows(rowIndex, dates, new Map([[1, [{ id: 'a', date: dates[1]! }]]]), {
      rowOrder: [0, 1, 2],
      minSlots: new Map([[0, 2]]),
    })
    expect(rows.map(r => r.row)).toEqual([0, 1, 2])
    expect(rows[0]!.slots).toBe(2) // 落点预留
    expect(rows[1]!.top).toBe(rows[0]!.top + rows[0]!.height)
    expect(rows[2]!.top).toBe(rows[1]!.top + rows[1]!.height)
  })

  it('窗口外日期的成员跳过（不画到别的日期列）', () => {
    const rows = layoutRows(rowIndex, dates, new Map([[1, [{ id: 'a', date: '2026-01-01' }]]]))
    expect(rows[0]!.slots).toBe(0)
    expect(rows[0]!.slotOf.size).toBe(0)
  })
})

describe('laneComposeLogic · 标题档（1.3 / 11.5）', () => {
  it('列宽 124px（下限）→ 10 字；列宽不足 124 → 整档关闭', () => {
    expect(titleBudget(124)).toBe(10)
    expect(titleBudget(125)).toBe(10)
    expect(titleBudget(123.9)).toBeNull()
    expect(titleBudget(52)).toBeNull()
    expect(titleBudget(23)).toBeNull()
  })

  it('列宽足够大也不超过 10 字（不因窗口变宽而制造长标题）', () => {
    expect(titleBudget(300)).toBe(MAX_TITLE_CHARS)
    expect(MAX_TITLE_CHARS).toBe(10)
  })

  it('时间列宽下限就是标题档下限（124）：撑宽画布横向滚动，不缩字', () => {
    expect(LANE_MIN_COLUMN).toBe(TITLE_MIN_COLUMN)
    expect(LANE_MIN_COLUMN).toBe(124)
  })
})

describe('laneComposeLogic · 散料可见性门控（4.1）', () => {
  const pool = [
    { id: 'c1', embedding: [1, 0], persistentTopic: { status: 'candidate' } },
    { id: 's1', embedding: [1, 0] },
    { id: 's2', embedding: [0.8, 0.6] },
    { id: 's3', embedding: [0, 1] },
    { id: 'a1', embedding: [1, 0], persistentTopic: { status: 'active' } },
  ]
  const candidateIds = new Set(['c1'])
  const readonlyIds = new Set(['a1'])
  const center = [1, 0]
  const MT = 0.3

  it('三档阈值：收紧 = ×0.5、标准 = matchThreshold、全部 = 1', () => {
    expect(thresholdOf('tight', MT)).toBeCloseTo(0.15, 12)
    expect(thresholdOf('standard', MT)).toBe(MT)
    expect(thresholdOf('all', MT)).toBe(1)
  })

  it('仅高相关档：散料收紧，候选链成员/已选/只读参照逐个不变', () => {
    const selected = new Set(['s1'])
    const tight = visibleScatter(pool, selected, candidateIds, readonlyIds, center, 'tight', MT)
    const standard = visibleScatter(pool, selected, candidateIds, readonlyIds, center, 'standard', MT)

    // 三类白名单旁路：数量逐个节点不变
    for (const id of ['c1', 's1', 'a1']) {
      expect(tight.visibleIds.has(id)).toBe(true)
      expect(standard.visibleIds.has(id)).toBe(true)
    }
    expect([...candidateIds].filter(id => tight.visibleIds.has(id))).toEqual([...candidateIds])
    expect([...readonlyIds].filter(id => tight.visibleIds.has(id))).toEqual([...readonlyIds])

    // 散料（未选）按档位收紧：s2 距离 ≈0.2 在标准档可见、仅在紧档被收起
    const s2Standard = visibleScatter(pool, selected, candidateIds, readonlyIds, center, 'standard', MT)
    expect(s2Standard.visibleScatterIds.has('s2')).toBe(true)
    expect(tight.visibleScatterIds.has('s2')).toBe(false)
    expect(tight.hiddenScatterIds).toContain('s2')
    expect(tight.hiddenScatterIds).toContain('s3')
    // 已选散料不当散料计数（它已在本次泳道里）
    expect(tight.hiddenScatterIds).not.toContain('s1')
  })

  it('全部档：散料全可见（不依赖质心）；无质心的标准档全部收起（不伪造达标）', () => {
    const all = visibleScatter(pool, new Set(), candidateIds, readonlyIds, null, 'all', MT)
    expect(all.visibleScatterIds.has('s1')).toBe(true)
    expect(all.visibleScatterIds.has('s3')).toBe(true)
    expect(all.hiddenScatterIds).toEqual([])

    const noCenter = visibleScatter(pool, new Set(), candidateIds, readonlyIds, null, 'standard', MT)
    expect(noCenter.visibleScatterIds.size).toBe(0)
    expect(noCenter.hiddenScatterIds.sort()).toEqual(['s1', 's2', 's3'])
    // 白名单旁路不受质心缺失影响
    expect(noCenter.visibleIds.has('c1')).toBe(true)
    expect(noCenter.visibleIds.has('a1')).toBe(true)
  })
})

describe('laneComposeLogic · 检索范围 = 未归类（5.1）', () => {
  const sections = [
    { id: '1', persistentTopic: { status: 'candidate' } },
    { id: '2', persistentTopic: { status: 'candidate' } },
    { id: '3' },
    { id: '4', persistentTopic: { status: 'candidate' } },
    { id: '5', persistentTopic: { status: 'active' } },
  ]
  const chains = [{ sectionIds: ['1', '2'] }]

  it('= 窗口内 sections − ⋃ chain.memberIds（不是 persistentTopicId == null）', () => {
    const ids = unclassifiedIds(sections, chains)
    expect(ids.has('3')).toBe(true) // 未归属任何话题
    expect(ids.has('4')).toBe(true) // 挂着 candidate 话题但未成链
    expect(ids.has('1')).toBe(false) // 候选链成员
    expect(ids.has('2')).toBe(false)
    expect(ids.has('5')).toBe(false) // 只读参照属于正式泳道，不是未归类
  })

  it('只出现在候选链成员标题里的词 → 未归类命中 0', () => {
    const ids = unclassifiedIds(sections, chains)
    expect(sections.filter(s => ids.has(s.id) && s.id === '1')).toHaveLength(0)
  })
})

describe('laneComposeLogic · 接近度映射区间覆盖（2026-11-21 用户实拍反馈）', () => {
  /** 编排画布实际传入的区间（与 LaneComposeCanvas 常量一致）。 */
  const RANGE = { min: 5.9, max: 7.4 }
  const STRENGTH = { min: 18, max: 80 }
  const flags = {
    memberRadius: 7.5,
    scatterRadius: 5.5,
    readonlyRadius: 2.75,
    recommendRange: RANGE,
    strengthRange: STRENGTH,
  }
  /** 给定接近度反推一个距离（threshold = 0.3）。 */
  const distanceAt = (proximity: number): number => 0.3 * (1 - proximity)

  it('P1 推荐半径按接近度线性插值：0 → 5.9、0.5 → 6.65、1 → 7.4（±0.05）', () => {
    const at = (proximity: number): number =>
      pointAppearance(distanceAt(proximity), 0.3, false, false, 7.3, 52, flags).radius
    expect(at(0)).toBeCloseTo(5.9, 2)
    expect(at(0.5)).toBeCloseTo(6.65, 2)
    expect(at(1)).toBeCloseTo(7.4, 2)
    // 单调：越近越大（旧公式在编排画布只差 0.43px，肉眼等于常量）
    expect(at(0.5)).toBeGreaterThan(at(0))
    expect(at(1) - at(0)).toBeGreaterThan(1)
  })

  it('P2 不变量：推荐 ≤ 已选 7.5、散料 5.5 < 推荐下限 5.9', () => {
    const never = pointAppearance(distanceAt(0), 0.3, false, false, 7.3, 52, flags)
    const nearest = pointAppearance(distanceAt(1), 0.3, false, false, 7.3, 52, flags)
    const member = pointAppearance(distanceAt(1), 0.3, true, false, 7.3, 52, flags)
    const scatter = pointAppearance(undefined, 0.3, false, false, 7.3, 52, flags)
    expect(nearest.radius).toBe(7.4)
    expect(member.radius).toBe(7.5)
    expect(nearest.radius).toBeLessThan(member.radius)
    expect(scatter.radius).toBe(5.5)
    expect(never.radius).toBeGreaterThan(scatter.radius)
    // 选中只由实心＋✓表达：成员点不读接近度
    expect(member.solid).toBe(true)
    expect(member.radius).toBe(pointAppearance(distanceAt(0), 0.3, true, false, 7.3, 52, flags).radius)
  })

  it('P3 深浅区间同步放大：接近度 0 → 18%、1 → 80%（旧档 12%–67%）', () => {
    const strengthAt = (proximity: number): number =>
      pointAppearance(distanceAt(proximity), 0.3, false, false, 7.3, 52, flags).strength
    expect(strengthAt(0)).toBeCloseTo(18, 6)
    expect(strengthAt(0.5)).toBeCloseTo(49, 6)
    expect(strengthAt(1)).toBeCloseTo(80, 6)
  })

  it('P4 缺省（修正画布不传区间）逐位不变：6.57 / 7.0、12% / 67%', () => {
    const legacy = { memberRadius: 7.5 }
    const at = (proximity: number): ReturnType<typeof pointAppearance> =>
      pointAppearance(distanceAt(proximity), 0.3, false, false, 7.3, 52, legacy)
    expect(at(0).radius).toBeCloseTo(6.57, 6)
    // 旧公式在接近度 ≈0.06 就撞上 recommendCap = 7.0：这正是「大小被压平」的实证
    expect(at(0.5).radius).toBeCloseTo(7.0, 6)
    expect(at(1).radius).toBeCloseTo(7.0, 6)
    expect(at(0).strength).toBeCloseTo(12, 6)
    expect(at(1).strength).toBeCloseTo(67, 6)
    // 未推荐点 / 只读参照不受影响
    expect(pointAppearance(undefined, 0.3, false, false, 7.3, 52, legacy).strength).toBe(0)
    expect(pointAppearance(0.1, 0.3, false, true, 7.3, 52, legacy).strength).toBe(0)
  })

  it('P5 非法区间（非有限数）按未传处理，不把 NaN 带进半径', () => {
    const bad = pointAppearance(distanceAt(1), 0.3, false, false, 7.3, 52, {
      memberRadius: 7.5,
      recommendRange: { min: Number.NaN, max: Number.POSITIVE_INFINITY },
    })
    expect(Number.isFinite(bad.radius)).toBe(true)
    expect(bad.radius).toBeCloseTo(7.0, 6)
    expect(bad.strength).toBeCloseTo(67, 6)
  })
})

describe('laneComposeLogic · 标题按宽度截断（12.x）', () => {
  it('中英混排都吃满列宽：CJK 10 字、ASCII 约 18 字', () => {
    // 列宽 124 − 2px 内边距 = 122px；11px 字号下 CJK = 11px/字 → 10 字 + 省略号
    expect(truncateByWidth('一二三四五六七八九十十一', 122, 11)).toBe('一二三四五六七八九十…')
    // ASCII = 0.55 × 11 ≈ 6.05px/字 → 约 18 字
    const latin = truncateByWidth('abcdefghijklmnopqrstuvwxyz', 122, 11)
    expect(latin.endsWith('…')).toBe(true)
    expect(latin.length).toBeGreaterThan(14)
  })

  it('放得下就原样返回，不补省略号', () => {
    expect(truncateByWidth('Gemini 4.5', 122, 11)).toBe('Gemini 4.5')
  })

  it('预算为 0 / 非法时不返回残字', () => {
    expect(truncateByWidth('任意标题', 0, 11)).toBe('')
    expect(truncateByWidth('任意标题', Number.NaN, 11)).toBe('')
  })
})

describe('laneComposeLogic · 接近度相对归一（2026-11-24 用户实拍：画布看不出远近）', () => {
  it('R1 批次内 min-max：最近 = 1、最远 = 0、中间按比例（实测那批 0.59/0.63/0.67）', () => {
    const map = relativeProximities(new Map([['a', 0.59], ['b', 0.63], ['c', 0.67]]))
    expect(map.get('a')).toBe(1)
    expect(map.get('b')).toBeCloseTo(0.5, 12)
    expect(map.get('c')).toBe(0)
    // 旧「1 − 距离 / 阈值」口径在这批数据上只有 0.04–0.14（肉眼等于常量）
    expect(1 - 0.59 / 0.67).toBeLessThan(0.15)
  })

  it('R2 单点 / 全部等距 → 0.5（不把所有人推到 0 或 1）', () => {
    expect(relativeProximities(new Map([['a', 0.6]])).get('a')).toBe(0.5)
    const flat = relativeProximities(new Map([['a', 0.6], ['b', 0.6], ['c', 0.6]]))
    for (const id of ['a', 'b', 'c']) expect(flat.get(id)).toBe(0.5)
  })

  it('R3 null / 非有限距离不参与批次，也不出现在结果里', () => {
    const map = relativeProximities(new Map([
      ['a', Number.NaN],
      ['b', 0.6],
      ['c', Number.POSITIVE_INFINITY],
      ['d', 0.7],
    ]))
    expect(map.has('a')).toBe(false)
    expect(map.has('c')).toBe(false)
    expect(map.get('b')).toBe(1)
    expect(map.get('d')).toBe(0)
  })

  it('R4 空批次 → 空表（无质心时整块画布保持 ordinary 外观）', () => {
    expect(relativeProximities(new Map()).size).toBe(0)
  })

  it('R5 归一后的 0..1 喂进区间：最近 7.4 / 80%、中间 6.65 / 49%、最远 5.9 / 18%', () => {
    const flags = {
      memberRadius: 7.5,
      scatterRadius: 5.5,
      recommendRange: { min: 5.9, max: 7.4 },
      strengthRange: { min: 18, max: 80 },
    }
    const at = (proximity: number): ReturnType<typeof pointAppearance> =>
      pointAppearance(undefined, 0.3, false, false, 7.3, 124, { ...flags, proximityOverride: proximity })
    expect(at(1).radius).toBeCloseTo(7.4, 6)
    expect(at(1).strength).toBeCloseTo(80, 6)
    expect(at(0.5).radius).toBeCloseTo(6.65, 6)
    expect(at(0.5).strength).toBeCloseTo(49, 6)
    expect(at(0).radius).toBeCloseTo(5.9, 6)
    expect(at(0).strength).toBeCloseTo(18, 6)
    // 推荐恒小于已选 7.5
    expect(at(1).radius).toBeLessThan(pointAppearance(0, 0.3, true, false, 7.3, 124, { ...flags, proximityOverride: 1 }).radius)
  })

  it('R6 不传 proximityOverride → 旧「距离 ÷ 阈值」公式逐位不变（修正画布）', () => {
    // 修正画布 LaneCorrectionCanvas 从不传该 flag：缺省行为必须保持
    expect(pointAppearance(0.15, 0.3, false, false, 4, 20).proximity).toBeCloseTo(0.5, 12)
    expect(pointAppearance(0.05, 0.3, false, false, 4, 20).proximity).toBeCloseTo(1 - 0.05 / 0.3, 12)
    // 超阈值仍不给接近度（旧口径）
    expect(pointAppearance(0.31, 0.3, false, false, 4, 20).proximity).toBeNull()
  })

  it('R7 只读参照即使给了 override 也不得获得接近度（快照不驱动大小）', () => {
    const readonly = pointAppearance(0.1, 0.3, false, true, 4, 20, { proximityOverride: 1 })
    expect(readonly.proximity).toBeNull()
    expect(readonly.strength).toBe(0)
  })

  it('R8 override 越界夹回 [0,1]，不把 NaN/超大值带进半径', () => {
    const flags = { recommendRange: { min: 5.9, max: 7.4 } }
    expect(pointAppearance(undefined, 0.3, false, false, 7.3, 124, { ...flags, proximityOverride: 9 }).proximity).toBe(1)
    expect(pointAppearance(undefined, 0.3, false, false, 7.3, 124, { ...flags, proximityOverride: -3 }).proximity).toBe(0)
    const nan = pointAppearance(undefined, 0.3, false, false, 7.3, 124, { ...flags, proximityOverride: Number.NaN })
    expect(nan.proximity).toBeNull()
    expect(Number.isFinite(nan.radius)).toBe(true)
  })
})

describe('laneComposeLogic · 节点标题落位（boxTop / 动态障碍）', () => {
  const cand = (id: string, x: number, y: number, extra: Partial<Parameters<typeof placeRelatedTitles>[0][number]> = {}) => ({
    id,
    x,
    y,
    r: 7.5,
    w: 90,
    h: 14,
    priority: 1,
    text: id,
    ...extra,
  })

  it('B1 boxTop：按显式上缘落位，不再用 y + r + 5（基线 = 上缘 + 盒高）', () => {
    const placed = placeRelatedTitles([cand('a', 100, 200, { boxTop: 200 })], { minX: 0, maxX: 500, minY: 0, maxY: 400 })
    expect(placed[0]!.box!.y).toBe(200)
    // 不传 boxTop 时仍是 y + r + 5（既有相关标题口径）
    const legacy = placeRelatedTitles([cand('b', 100, 200)], { minX: 0, maxX: 500, minY: 0, maxY: 400 })
    expect(legacy[0]!.box!.y).toBe(200 + 7.5 + 5)
  })

  it('B2 动态障碍：自身圆不算障碍，其它节点的圆算（被挤到下一行或跳过）', () => {
    const circle = (cx: number, cy: number, r: number) => ({ x: cx - r, y: cy - r, w: r * 2, h: r * 2 })
    // 自己的圆（圆心 = 候选点）不该挤掉自己的标题
    const own = placeRelatedTitles([cand('a', 100, 200, { boxTop: 200 })], {
      minX: 0,
      maxX: 500,
      minY: 0,
      maxY: 400,
      obstaclesOf: () => [circle(100, 200, 7.5)],
    })
    expect(own[0]!.skipped).toBeUndefined()
    expect(own[0]!.box!.y).toBe(200)
    // 别人压在同一位置（但不包含本候选点）→ 错行（lineHeight 16）
    const clash = placeRelatedTitles([cand('b', 100, 200, { boxTop: 200 })], {
      minX: 0,
      maxX: 500,
      minY: 0,
      maxY: 400,
      lineHeight: 16,
      obstaclesOf: () => [circle(100, 209, 5)],
    })
    expect(clash[0]!.box!.y).toBe(216)
  })

  it('B3 错行用尽仍撞上 → 整条不画（宁缺勿叠，不叠字）', () => {
    const placed = placeRelatedTitles([
      cand('a', 100, 200, { boxTop: 200 }),
      cand('b', 100, 200, { boxTop: 200 }),
      cand('c', 100, 200, { boxTop: 200 }),
    ], { lines: 1, lineHeight: 16, minX: 0, maxX: 500, minY: 0, maxY: 400 })
    expect(placed.filter(p => p.skipped)).toHaveLength(1)
    // 已落位的两条互不重叠（一条正下方、一条错一行）
    const boxes = placed.filter(p => !p.skipped).map(p => p.box!)
    expect(boxes).toHaveLength(2)
    expect(boxesOverlap(boxes[0]!, boxes[1]!)).toBe(false)
  })

  it('B4 优先级：已选 > 候选链成员 > 焦点相关 > 散料（先放的不被后放的挤走）', () => {
    const placed = placeRelatedTitles([
      cand('floor', 100, 200, { boxTop: 200, priority: 1 }),
      cand('selected', 100, 200, { boxTop: 200, priority: 4 }),
      cand('chain', 100, 200, { boxTop: 200, priority: 3 }),
    ], { lines: 1, lineHeight: 16, minX: 0, maxX: 500, minY: 0, maxY: 400 })
    const at = new Map(placed.map(p => [p.id, p]))
    expect(at.get('selected')!.box!.y).toBe(200)
    expect(at.get('chain')!.box!.y).toBe(216)
    expect(at.get('floor')!.skipped).toBe(true)
  })
})
