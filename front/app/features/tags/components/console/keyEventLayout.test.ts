import { describe, expect, it } from 'vitest'
import {
  computeEventColumnWidth,
  computeKeyEventRequestDays,
  keyEventDayCardRect,
  keyEventLabelsCollapsed,
  keyEventLeaderPath,
  keyEventRowHeight,
  keyEventRowHeightWithCards,
  layoutEventLabels,
  layoutKeyEventBands,
  nodeFanOffset,
  placementLayerCount,
  projectKeyEventDays,
  KEY_EVENT_AXIS_BAND_HEIGHT,
  KEY_EVENT_BAND_LAYER_STEP,
  KEY_EVENT_BAND_PADDING,
  KEY_EVENT_CARD_WIDTH,
  KEY_EVENT_LABEL_HEIGHT,
  KEY_EVENT_LEADER_ELBOW,
  KEY_EVENT_MAX_LABEL_LAYERS,
  KEY_EVENT_MAX_LAYER_SEARCH,
  KEY_EVENT_MAX_ROW_HEIGHT,
  KEY_EVENT_MIN_COLUMN_WIDTH,
  KEY_EVENT_NODE_CENTER_Y,
  KEY_EVENT_NODE_FAN_STEP,
  KEY_EVENT_NODE_TOP,
} from './keyEventLayout'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'

function event(id: string, date: string): BoardKeyEventItem {
  return {
    id, topicId: '7', topicLabelSnapshot: 'T', sourcePeriodDate: date,
    titleSnapshot: `事件${id}`, summarySnapshot: '', articleIds: [],
  }
}

describe('computeEventColumnWidth — 列宽 = max(64, 可用宽/日数)', () => {
  it('超宽屏均分填满：2560 可用宽 × 14 天 ≈ 183px/列', () => {
    expect(computeEventColumnWidth(2560, 14)).toBeCloseTo(183, 0)
  })

  it('可用宽不足时守 64 下限（宁可撑宽横滚，不压缩列宽）', () => {
    expect(computeEventColumnWidth(500, 30)).toBe(KEY_EVENT_MIN_COLUMN_WIDTH)
    expect(computeEventWidthDegenerate()).toBe(KEY_EVENT_MIN_COLUMN_WIDTH)
  })

  it('日数非法/为 0 按单日兜底', () => {
    expect(computeEventColumnWidth(2560, 0)).toBe(2560)
    expect(computeEventColumnWidth(2560, Number.NaN)).toBe(2560)
  })
})

function computeEventWidthDegenerate(): number {
  return computeEventColumnWidth(-100, 14)
}

describe('layoutEventLabels — 贪心分层（不重叠、不截断）', () => {
  it('放得下就复用该层：两个远距标签同层、top 相同', () => {
    const out = layoutEventLabels([
      { key: 'a', x: 100, width: 50 },
      { key: 'b', x: 700, width: 50 },
    ], 1200)
    expect(out[0]!.layer).toBe(0)
    expect(out[1]!.layer).toBe(0)
    expect(out[0]!.top).toBe(out[1]!.top)
  })

  it('放不下就新开一层：近距重叠标签分层、top 相差 44px（层距）；分层即避让（跨层允许水平重叠）', () => {
    const out = layoutEventLabels([
      { key: 'a', x: 100, width: 300 },
      { key: 'b', x: 120, width: 300 },
    ], 1200)
    expect(out[0]!.layer).toBe(0)
    expect(out[1]!.layer).toBe(1)
    expect(out[1]!.top - out[0]!.top).toBe(44)
    // 同一水平区间但不同层：垂直分离保证不重叠（标签不截断的前提）
    expect(out[0]!.top).not.toBe(out[1]!.top)
  })

  it('同层相邻标签水平间隙 ≥ 4px（层内不贴死）', () => {
    const out = layoutEventLabels([
      { key: 'a', x: 100, width: 50 },
      { key: 'b', x: 160, width: 50 },
    ], 1200)
    expect(out[1]!.layer).toBe(0)
    expect(out[1]!.left - (out[0]!.left + 50)).toBeGreaterThanOrEqual(4)
  })

  it('靠近右边界左移贴边：left = max(0, 容器宽 - 标签宽)，不溢出容器', () => {
    const out = layoutEventLabels([
      { key: 'a', x: 1150, width: 300 },
    ], 1200)
    expect(out[0]!.left).toBe(1200 - 300)
    expect(out[0]!.left + 300).toBeLessThanOrEqual(1200)
  })

  it('按节点 x 升序放置（日期升序）', () => {
    const out = layoutEventLabels([
      { key: 'late', x: 800, width: 50 },
      { key: 'early', x: 100, width: 50 },
    ], 1200)
    expect(out[0]!.key).toBe('early')
    expect(out[1]!.key).toBe('late')
  })
})

describe('keyEventRowHeight / placementLayerCount', () => {
  it('无标签行走基础行高 70px；合计 1 层 = 82px、2 层 = 114px（层距 32px，design D14）', () => {
    expect(keyEventRowHeight(0)).toBe(70)
    expect(keyEventRowHeight(1)).toBe(50 + 32)
    expect(keyEventRowHeight(2)).toBe(50 + 64)
  })

  it('层数统计（placementLayerCount）', () => {
    expect(placementLayerCount([])).toBe(0)
    expect(placementLayerCount([{ key: 'a', left: 0, top: 46, layer: 0 }])).toBe(1)
    expect(placementLayerCount([
      { key: 'a', left: 0, top: 46, layer: 0 },
      { key: 'b', left: 0, top: 90, layer: 1 },
    ])).toBe(2)
  })

  it('行高封顶 246px：4 层 178px 不封顶，7 层起钉在 246px（design D14）', () => {
    expect(keyEventRowHeight(4)).toBe(178)
    expect(KEY_EVENT_MAX_ROW_HEIGHT).toBe(246)
    expect(keyEventRowHeight(5)).toBe(210)
    expect(keyEventRowHeight(6)).toBe(242) // 50 + 32×6 = 242 < 246
    expect(keyEventRowHeight(7)).toBe(246) // 50 + 32×7 = 274 → 封顶 246
    expect(keyEventRowHeight(8)).toBe(246)
    expect(keyEventRowHeight(12)).toBe(246)
  })
})

describe('keyEventRowHeightWithCards — 卡片行高不受 246 封顶（design D6）', () => {
  it('最终行高取标签行高与卡片列占位高度的较大者', () => {
    expect(keyEventRowHeightWithCards(178, 240)).toBe(240)
    expect(keyEventRowHeightWithCards(178, 400)).toBe(400)
    expect(keyEventRowHeightWithCards(178, 120)).toBe(178)
  })

  it('卡片堆叠高过 246px 也不截断（≤ 4 张卡片全部展开，行高必须放得下）', () => {
    expect(keyEventRowHeightWithCards(178, 460)).toBeGreaterThan(KEY_EVENT_MAX_ROW_HEIGHT)
  })
})

describe('keyEventLabelsCollapsed — 收起只看合计层数，不看天数（design D7/D14）', () => {
  it('上带 + 下带合计层数 ≤ 4 不收起（长窗也照常显示名称）', () => {
    expect(KEY_EVENT_MAX_LABEL_LAYERS).toBe(4)
    expect(keyEventLabelsCollapsed(0)).toBe(false)
    expect(keyEventLabelsCollapsed(1)).toBe(false)
    expect(keyEventLabelsCollapsed(4)).toBe(false)
  })

  it('合计层数超 4 才收起（退化为节点 + 当日计数）', () => {
    expect(keyEventLabelsCollapsed(5)).toBe(true)
    expect(keyEventLabelsCollapsed(9)).toBe(true)
  })
})

describe('nodeFanOffset — 同日多节点横向散开（节点恒在轴线上）', () => {
  it('单节点不偏移', () => {
    expect(nodeFanOffset(0, 1)).toBe(0)
  })

  it('多条居中对称散开（不竖向下压）', () => {
    expect(nodeFanOffset(0, 3)).toBe(-KEY_EVENT_NODE_FAN_STEP)
    expect(nodeFanOffset(1, 3)).toBe(0)
    expect(nodeFanOffset(2, 3)).toBe(KEY_EVENT_NODE_FAN_STEP)
  })

  it('节点上沿常量：骑在轴线上（不随层数下移）', () => {
    expect(KEY_EVENT_NODE_TOP).toBe(9)
  })

  it('同日圆点散开步长 24px（18px 圆点 + 12px 步长会互相压盖，design D13）', () => {
    expect(KEY_EVENT_NODE_FAN_STEP).toBe(24)
    expect(nodeFanOffset(0, 3)).toBe(-24)
    expect(nodeFanOffset(2, 3)).toBe(24)
  })
})

// ── 双带布局（key-event-day-focus-cards design D3/D6）───────────────────────

describe('layoutKeyEventBands — 双侧代价分配（design D3）', () => {
  it('上下带都空时代价并列 → 取下带', () => {
    const out = layoutKeyEventBands([
      { date: '2026-11-10', items: [{ key: 'a', dotX: 100, width: 200 }] },
    ], 1200, null)
    expect(out.placements[0]!.band).toBe('down')
    expect(out.placements[0]!.layer).toBe(0)
    expect(out.upLayers).toBe(0)
    expect(out.downLayers).toBe(1)
  })

  it('覆盖代价按「标签条数」计（不按层聚合）：同一层里两条标签盖住本组圆点 → 代价更高，本组改选另一带', () => {
    // 覆盖项 F1 / F2 落在**同一层**（上带 0 层），两条标签各盖住本组的一个圆点：
    //   F1 标签 [40,100] 盖住圆点 x=100；F2 标签 [104,170] 盖住圆点 x=124
    // 按标签计 → 覆盖 2 条 → 上带代价 100×2 + 10×(1+2) + 3×1 = 233
    // 按层计   → 覆盖 1 层 → 上带代价 100×1 + 10×(1+2) + 3×1 = 133
    // 下带代价（11 个已占层把起始层推到 11）= 10×(11+2) + 3×11 = 163
    // → 按标签计（233 > 163）本组去下带；按层计（133 < 163）本组会去上带
    const out = layoutKeyEventBands([
      fillerGroup('2026-11-10'),
      { date: '2026-11-11', items: [{ key: 'f1', dotX: 28, width: 60 }] },
      { date: '2026-11-12', items: [{ key: 'f2', dotX: 92, width: 66 }] },
      targetGroup(),
    ], 1200, null)
    expect(out.placements.find(p => p.key === 't1')!.band).toBe('down')
  })

  it('反向用例：去掉覆盖标签后该组改选另一带（证明覆盖项确有区分力）', () => {
    const out = layoutKeyEventBands([
      fillerGroup('2026-11-10'),
      targetGroup(),
    ], 1200, null)
    expect(out.placements.find(p => p.key === 't1')!.band).toBe('up')
  })

  it('与卡片矩形相交的组改走上带（卡片是下带的固定障碍区）', () => {
    const out = layoutKeyEventBands([
      { date: '2026-11-10', items: [{ key: 'a', dotX: 200, width: 200 }] },
    ], 1200, { left: 100, right: 380 })
    expect(out.placements[0]!.band).toBe('up')
    expect(out.upLayers).toBe(1)
    expect(out.downLayers).toBe(0)
  })

  it('不相交的组仍按下带落位（卡片只挡横向范围相交的那一组）', () => {
    const out = layoutKeyEventBands([
      { date: '2026-11-10', items: [{ key: 'a', dotX: 900, width: 200 }] },
    ], 1200, { left: 100, right: 380 })
    expect(out.placements[0]!.band).toBe('down')
  })

  it('组内近端统一对齐「该日最右圆点 + 12px」，组内占连续层', () => {
    const out = layoutKeyEventBands([
      {
        date: '2026-11-10',
        items: [
          { key: 'a', dotX: 100, width: 200 },
          { key: 'b', dotX: 124, width: 200 },
        ],
      },
    ], 1200, null)
    const [a, b] = out.placements
    expect(a!.left).toBe(124 + KEY_EVENT_LEADER_ELBOW)
    expect(b!.left).toBe(a!.left)
    expect(a!.layer).toBe(0)
    expect(b!.layer).toBe(1)
    expect(out.downLayers).toBe(2)
  })

  it('同日单条组放不下时贴右缘（圆点落进自身范围 → 引线退化为竖线段，R-32）：不新开层', () => {
    const out = layoutKeyEventBands([
      { date: '2026-11-10', items: [{ key: 'a', dotX: 1150, width: 300 }] },
    ], 1200, null)
    // 旧值 1150 − 12 − 300 = 838（左侧镜像）：真实 30 天数据下镜像会把标签推进中部拥挤区、
    // 每多一条就多占一层（上 2 + 下 3 = 5 层 → 整行收成 ★N）。单条组没有「组内其它标签」
    // 可被穿越，贴右缘（= 与已批准原型同口径）仍是紧邻圆点，且不新开层。
    expect(out.placements[0]!.left).toBe(1200 - 300)
    expect(out.placements[0]!.crossesGroupLabel).toBeUndefined()
    expect(out.upLayers + out.downLayers).toBe(1)
  })

  it('同日多条组仍改摆到圆点左侧（组内引线 MUST NOT 穿过组内标签，review R-15 不变）', () => {
    const out = layoutKeyEventBands([
      {
        date: '2026-11-10',
        items: [
          { key: 'a', dotX: 1150, width: 300 },
          { key: 'b', dotX: 1174, width: 300 },
        ],
      },
    ], 1200, null)
    expect(out.placements[0]!.left).toBe(1150 - KEY_EVENT_LEADER_ELBOW - 300)
    expect(out.placements[1]!.left).toBe(out.placements[0]!.left)
  })

  it('选中日的分组不出药丸（由卡片承接）', () => {
    const out = layoutKeyEventBands([
      { date: '2026-11-10', items: [{ key: 'a', dotX: 100, width: 200 }] },
      { date: '2026-11-11', items: [{ key: 'b', dotX: 200, width: 200 }] },
    ], 1200, null, '2026-11-10')
    expect(out.placements.map(p => p.key)).toEqual(['b'])
  })
})

// ── 组内引线几何（review R-15）与分层溢出（review R-16）────────────────────

/** 节点半径（圆点外缘到圆心）。 */
const NODE_R = KEY_EVENT_NODE_CENTER_Y - KEY_EVENT_NODE_TOP

/** 圆点外缘 y（轴带占 [0, 18]，生命线居中）。 */
function dotEdgeY(band: 'up' | 'down'): number {
  return band === 'down'
    ? KEY_EVENT_AXIS_BAND_HEIGHT / 2 + NODE_R
    : KEY_EVENT_AXIS_BAND_HEIGHT / 2 - NODE_R
}

/** 一条标签的矩形（行体坐标系：上带在轴上方、下带在轴下方）。 */
function labelRectOf(p: { left: number; width: number; layer: number; band: 'up' | 'down' }) {
  const top = p.band === 'down'
    ? KEY_EVENT_AXIS_BAND_HEIGHT + KEY_EVENT_BAND_PADDING + KEY_EVENT_BAND_LAYER_STEP * p.layer
    : -KEY_EVENT_BAND_PADDING - KEY_EVENT_BAND_LAYER_STEP * p.layer - KEY_EVENT_LABEL_HEIGHT
  return { left: p.left, right: p.left + p.width, top, bottom: top + KEY_EVENT_LABEL_HEIGHT }
}

/** 占 11 层的填充组（下带 0..10 层，标签统一在 [392, 492]）。 */
function fillerGroup(date: string) {
  return {
    date,
    items: Array.from({ length: 11 }, (_, i) => ({ key: `p${i}`, dotX: 140 + i * 24, width: 100 })),
  }
}

/** 目标组：同日两个圆点 x=100 / 124，标签宽 100。 */
function targetGroup() {
  return {
    date: '2026-11-13',
    items: [
      { key: 't1', dotX: 100, width: 100 },
      { key: 't2', dotX: 124, width: 100 },
    ],
  }
}

describe('layoutKeyEventBands — 组内引线 MUST NOT 穿过组内标签（review R-15）', () => {
  const sameDay = {
    date: '2026-11-22',
    items: [
      { key: 'a', dotX: 1150, width: 300 },
      { key: 'b', dotX: 1174, width: 300 },
    ],
  }

  it('整组放不下时改摆到圆点左侧（组右缘 = 最左圆点 − 12px）', () => {
    const out = layoutKeyEventBands([sameDay], 1200, null)
    const [a, b] = out.placements
    expect(a!.left).toBe(1150 - KEY_EVENT_LEADER_ELBOW - 300)
    expect(b!.left).toBe(a!.left)
    expect(a!.left + 300, '组右缘落在最左圆点左侧').toBeLessThan(1150)
  })

  it('组内没有任何一条引线的竖段与组内其它标签矩形相交', () => {
    const out = layoutKeyEventBands([sameDay], 1200, null)
    const rects = out.placements.map(labelRectOf)
    out.placements.forEach((p, i) => {
      const own = rects[i]!
      const path = keyEventLeaderPath({
        dotX: p.dotX,
        dotY: dotEdgeY(p.band),
        labelLeft: p.left,
        labelTop: own.top,
        labelWidth: p.width,
        band: p.band,
      })
      for (const other of rects) {
        const overlapX = path.vertical.x1 >= other.left && path.vertical.x1 <= other.right
        const overlapY = Math.min(path.vertical.y2, other.bottom) - Math.max(path.vertical.y1, other.top)
        expect(overlapX && overlapY > 0, '引线竖段穿过组内标签').toBe(false)
      }
    })
  })

  it('几何无解时（组宽 > 最左圆点 − 12px 且右边放不下）只能贴右缘，并标记引线穿组内标签', () => {
    const out = layoutKeyEventBands([
      {
        date: '2026-11-22',
        items: [
          { key: 'a', dotX: 100, width: 300 },
          { key: 'b', dotX: 900, width: 300 },
        ],
      },
    ], 1200, null)
    expect(out.placements[0]!.left).toBe(1200 - 300)
    expect(out.placements[1]!.crossesGroupLabel).toBe(true)
    expect(out.placements[0]!.crossesGroupLabel).toBeUndefined()
  })
})

describe('layoutKeyEventBands — 长窗层数 MUST NOT 把事件名收成星标（azure R-32）', () => {
  /** 30 天窗口（列宽 64px → 行体 1920px）；宽度取实机 1440/1920 量到的真实标签宽。 */
  const LONG_WINDOW = [
    { date: '2026-10-24', items: [{ key: 'e1', dotX: 32, width: 257 }] },
    { date: '2026-10-27', items: [{ key: 'e2', dotX: 224, width: 288 }] },
    { date: '2026-10-30', items: [{ key: 'e3', dotX: 416, width: 270 }] },
    { date: '2026-11-05', items: [{ key: 'e4', dotX: 800, width: 308 }] },
    { date: '2026-11-06', items: [{ key: 'e5', dotX: 864, width: 229 }] },
    {
      date: '2026-11-10',
      items: [
        { key: 'e6', dotX: 1108, width: 249 },
        { key: 'e7', dotX: 1132, width: 265 },
      ],
    },
    { date: '2026-11-11', items: [{ key: 'e8', dotX: 1184, width: 211 }] },
    { date: '2026-11-12', items: [{ key: 'e9', dotX: 1248, width: 317 }] },
    { date: '2026-11-16', items: [{ key: 'e10', dotX: 1504, width: 277 }] },
    { date: '2026-11-18', items: [{ key: 'e11', dotX: 1632, width: 304 }] },
  ]

  it('11 条跳 30 天的标签在 64px 列宽下仍 ≤ 4 层（选中日 11/13 跳过一组）', () => {
    const out = layoutKeyEventBands(LONG_WINDOW, 64 * 30, null, '2026-11-13')
    expect(out.placements).toHaveLength(11)
    expect(out.overflowed).toBeFalsy()
    expect(out.placements.some(p => p.crossesGroupLabel === true)).toBe(false)
    // 旧值 5（上 2 + 下 3）：单条组走左侧镜像 → 整行超 4 层被收成 ★N（azure R-32）
    expect(out.upLayers + out.downLayers).toBe(4)
    expect(out.upLayers).toBe(2)
    expect(out.downLayers).toBe(2)
  })

  it('14 天窗口 / 1440 视口（列宽 67px → 行体 938px）同样不超 4 层', () => {
    // 14 天轴 11/09…11/22：11/10 是第 1 列…11/18 是第 9 列
    const axisIndex: Record<string, number> = {
      '2026-11-10': 1, '2026-11-11': 2, '2026-11-12': 3, '2026-11-16': 7, '2026-11-18': 9,
    }
    const shortWindow = LONG_WINDOW
      .filter(group => group.date >= '2026-11-10')
      .map(group => ({
        ...group,
        items: group.items.map((item, index) => ({
          ...item,
          dotX: axisIndex[group.date]! * 67
            + 32
            + (group.items.length > 1 ? (index - (group.items.length - 1) / 2) * 24 : 0),
        })),
      }))
    const out = layoutKeyEventBands(shortWindow, 938, null, '2026-11-22')
    expect(out.overflowed).toBeFalsy()
    // 旧值 5（上 3 + 下 2）：11/18 走左侧镜像 → 整行收成 ★N（azure R-32）
    expect(out.upLayers + out.downLayers).toBe(4)
  })
})

describe('layoutKeyEventBands — 分层搜索到上限不返回未校验的层（review R-16）', () => {
  it('两带都找不到可用连续层时该组不落位，并显式标记该行溢出', () => {
    // 卡片挡住下带 → 只能去上带；每组与已有层横向冲突 → 逐层上溯，
    // 上带占满（0..400 层）后搜索到上限仍冲突 → 该组 MUST NOT 落位
    const total = KEY_EVENT_MAX_LAYER_SEARCH + 2
    const groups = Array.from({ length: total }, (_, i) => ({
      date: `2026-11-${String(i).padStart(3, '0')}`,
      items: [{ key: `k${i}`, dotX: 300, width: 100 }],
    }))
    const out = layoutKeyEventBands(groups, 1200, { left: 250, right: 530 })
    expect(out.overflowed).toBe(true)
    expect(out.unplacedKeys).toEqual([`k${total - 1}`])
    expect(out.placements.length).toBe(KEY_EVENT_MAX_LAYER_SEARCH + 1)
    expect(out.placements.some(p => p.key === `k${total - 1}`), '溢出的组 MUST NOT 放回冲突层').toBe(false)
    expect(Math.max(...out.placements.map(p => p.layer))).toBe(KEY_EVENT_MAX_LAYER_SEARCH)
  })
})

describe('keyEventLeaderPath — 引线折点（design D4）', () => {
  it('圆点 x 在标签横向范围外：竖段到标签垂直中线 + 横段接入近端（12px 接线）', () => {
    const out = keyEventLeaderPath({
      dotX: 100, dotY: 57, labelLeft: 112, labelTop: 70, labelWidth: 200, band: 'down',
    })
    expect(out.vertical.x1).toBe(100)
    expect(out.vertical.y1).toBe(57)
    expect(out.vertical.y2).toBe(70 + KEY_EVENT_LABEL_HEIGHT / 2)
    expect(out.horizontal).not.toBeNull()
    expect(out.horizontal!.x1).toBe(100)
    expect(out.horizontal!.x2).toBe(112)
    expect(out.horizontal!.y1).toBe(out.horizontal!.y2)
  })

  it('圆点 x 落在标签横向范围内：只画竖段（无横段）', () => {
    const out = keyEventLeaderPath({
      dotX: 150, dotY: 57, labelLeft: 112, labelTop: 70, labelWidth: 200, band: 'down',
    })
    expect(out.horizontal).toBeNull()
    expect(out.vertical.y2).toBe(70)
    expect(out.d.match(/L/g)).toHaveLength(1)
  })

  it('上带：竖段止于标签下缘（标签遮线不遮字）', () => {
    const out = keyEventLeaderPath({
      dotX: 150, dotY: 48, labelLeft: 112, labelTop: 20, labelWidth: 200, band: 'up',
    })
    expect(out.horizontal).toBeNull()
    expect(out.vertical.y2).toBe(20 + KEY_EVENT_LABEL_HEIGHT)
  })
})

describe('keyEventDayCardRect — 卡片矩形与靠右翻转（design D6）', () => {
  it('默认：左缘 = 选中列左缘，宽 280px', () => {
    const rect = keyEventDayCardRect(2, 100, 1000)
    expect(rect).toEqual({ left: 200, right: 200 + KEY_EVENT_CARD_WIDTH })
  })

  it('靠右放不下时右缘对齐选中列右缘向左展开（MUST NOT 伸出舞台）', () => {
    const rect = keyEventDayCardRect(9, 100, 1000)
    expect(rect).toEqual({ left: 720, right: 1000 })
  })

  it('无选中列时没有卡片矩形', () => {
    expect(keyEventDayCardRect(-1, 100, 1000)).toBeNull()
  })
})

describe('keyEventDayCardRect — 行体窄于卡片宽的钳制（review R-18）', () => {
  it('行体窄于 280px：卡片矩形钳制在 [0, bodyWidth] 内（溢出由舞台内横滚承接）', () => {
    const rect = keyEventDayCardRect(3, 64, 256)
    expect(rect).not.toBeNull()
    expect(rect!.left).toBe(0)
    expect(rect!.right).toBe(256)
    expect(rect!.clamped).toBe(true)
  })

  it('不变式：0 <= left 且 right <= max(bodyWidth, 280)', () => {
    const cases: [number, number, number][] = [
      [0, 64, 256], [3, 64, 256], [2, 100, 1000], [9, 100, 1000],
      [20, 100, 1000], [0, 64, 280], [5, 100, 300], [0, 64, 1200],
    ]
    for (const [index, columnWidth, bodyWidth] of cases) {
      const rect = keyEventDayCardRect(index, columnWidth, bodyWidth)!
      expect(rect.left, `left >= 0（列 ${index} / 行体 ${bodyWidth}）`).toBeGreaterThanOrEqual(0)
      expect(rect.right, `right <= max(bodyWidth, 280)（列 ${index} / 行体 ${bodyWidth}）`)
        .toBeLessThanOrEqual(Math.max(bodyWidth, KEY_EVENT_CARD_WIDTH))
      expect(rect.right).toBeGreaterThanOrEqual(rect.left)
    }
  })
})

describe('projectKeyEventDays — 轴内逐日投影', () => {
  it('窗外日期过滤：axis 内无事件的日期给空数组（不落节点、不伪造 0）', () => {
    const byDate = new Map([
      ['2026-11-12', [event('a', '2026-11-12')]],
      ['2026-10-01', [event('out', '2026-10-01')]],
    ])
    const groups = projectKeyEventDays(byDate, ['2026-11-11', '2026-11-12'])
    expect(groups).toEqual([
      { date: '2026-11-11', events: [] },
      { date: '2026-11-12', events: [expect.objectContaining({ id: 'a' })] },
    ])
  })
})

// ── 请求窗口（review R-4：365 封顶会静默少事件）─────────────────────────────

describe('computeKeyEventRequestDays — 请求窗口覆盖轴起点（MUST NOT 封顶 365）', () => {
  it('轴跨度 ≤ 365：取 max(档天数, 轴跨度)', () => {
    // 轴 2026-11-01 ~ 2026-11-14，today = 2026-11-14 → 跨度 14 天
    const axis = Array.from({ length: 14 }, (_, i) => `2026-11-${String(i + 1).padStart(2, '0')}`)
    expect(computeKeyEventRequestDays(axis, '2026-11-14', 14)).toBe(14)
  })

  it('档天数更大时取档天数（轴比档窄）', () => {
    const axis = ['2026-11-13', '2026-11-14']
    expect(computeKeyEventRequestDays(axis, '2026-11-14', 30)).toBe(30)
  })

  it('轴跨度 > 365（版块日报落后今天）时**不得**被截成 365：必须覆盖轴起点', () => {
    // 池左沿 = anchor−364 = 2025-10-17（anchor = 2026-10-16，落后今天 11 天）
    const axisStart = '2025-10-17'
    const today = '2026-10-27'
    const span = 376 // 365 + 11（today − anchor）
    expect(computeKeyEventRequestDays([axisStart], today, 365)).toBe(span)
    expect(computeKeyEventRequestDays([axisStart], today, 365)).toBeGreaterThan(365)
  })

  it('空轴：回落档天数（还没轴时不发请求窗口）', () => {
    expect(computeKeyEventRequestDays([], '2026-11-14', 14)).toBe(14)
  })

  it('窗口有界：轴左沿被池边界钳住 → 跨度 ≤ 池跨度 + 日报滞后，不会无限大', () => {
    // 池左沿 = anchor−364（anchor = 2026-10-16，落后今天 11 天 → 池左沿 2025-10-17）
    const today = '2026-10-27'
    const poolStart = '2025-10-17'
    const days = computeKeyEventRequestDays([poolStart], today, 365)
    expect(days).toBe(365 + 11)
    expect(days, '跨度有界（池跨度 + 日报滞后），不会无限大').toBeLessThan(365 + 365)
  })
})
