import { describe, expect, it } from 'vitest'
import {
  boxesOverlap,
  candidateSelection,
  centroidOf,
  centroidRecommendations,
  clampZoom,
  cosineFor,
  estimateTextWidth,
  laneSelectionState,
  isDragBeyond,
  panScroll,
  placeLabels,
  placeRelatedTitles,
  pointAppearance,
  relatedNodeIds,
  scaledPointRadius,
  truncateText,
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
