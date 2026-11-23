import { describe, expect, it } from 'vitest'
import {
  computeEventColumnWidth,
  computeKeyEventRequestDays,
  keyEventLabelsCollapsed,
  keyEventRowHeight,
  layoutEventLabels,
  nodeFanOffset,
  placementLayerCount,
  projectKeyEventDays,
  KEY_EVENT_MAX_LABEL_LAYERS,
  KEY_EVENT_MAX_ROW_HEIGHT,
  KEY_EVENT_MIN_COLUMN_WIDTH,
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
  it('无标签行走基础行高 70px；1 层 = 90px、2 层 = 134px（每层 +44）', () => {
    expect(keyEventRowHeight(0)).toBe(70)
    expect(keyEventRowHeight(1)).toBe(46 + 44)
    expect(keyEventRowHeight(2)).toBe(46 + 88)
  })

  it('层数统计（placementLayerCount）', () => {
    expect(placementLayerCount([])).toBe(0)
    expect(placementLayerCount([{ key: 'a', left: 0, top: 46, layer: 0 }])).toBe(1)
    expect(placementLayerCount([
      { key: 'a', left: 0, top: 46, layer: 0 },
      { key: 'b', left: 0, top: 90, layer: 1 },
    ])).toBe(2)
  })

  it('行高封顶 246px：4 层 222px 不封顶，5 层起钉在 246px（design D7）', () => {
    expect(keyEventRowHeight(4)).toBe(46 + 4 * 44)
    expect(KEY_EVENT_MAX_ROW_HEIGHT).toBe(246)
    expect(keyEventRowHeight(5)).toBe(246)
    expect(keyEventRowHeight(12)).toBe(246)
  })
})

describe('keyEventLabelsCollapsed — 收起只看层数，不看天数（design D7）', () => {
  it('层数 ≤ 4 不收起（长窗也照常显示名称）', () => {
    expect(KEY_EVENT_MAX_LABEL_LAYERS).toBe(4)
    expect(keyEventLabelsCollapsed(0)).toBe(false)
    expect(keyEventLabelsCollapsed(1)).toBe(false)
    expect(keyEventLabelsCollapsed(4)).toBe(false)
  })

  it('层数超 4 才收起（退化为节点 + 当日计数）', () => {
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
