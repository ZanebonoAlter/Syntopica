/**
 * 左轨逐日线索条码读模型单测（console-rails-live-signals tasks 1.2）。
 *
 * 锁住 design D2 的共同轴重对齐（假比较防线）与 D3/D4 的峰值/强度口径：
 *  - offset 0 / 3 / ≥14 三种情形（停更尾段斜纹、不补 0）；
 *  - join 顺序 = 版块列表序（不被 overview 关注度序带偏）；
 *  - 未取到（data_unavailable / 缺 clue_days）→ 整行未取到，不显示全 0；
 *  - 行内峰值归一 + 跨行强度分档（0.42–1）。
 */
import { describe, expect, it } from 'vitest'
import type { RadarBoard } from '~/api/normalizers/boardRadar'
import type { SemanticBoard } from '~/api/semanticBoards'
import {
  RAIL_AXIS_DAYS,
  buildRailBoardRows,
  railAxisDates,
  railRowTitle,
} from './railClueAxis'

const GLOBAL_DATE = '2026-11-18'

function board(id: number, label: string, tagCount = 100): SemanticBoard {
  return {
    id,
    label,
    slug: `board-${id}`,
    aliases: [],
    ref_count: 0,
    tag_count: tagCount,
    description: '',
    display_order: id,
    source: 'manual',
    status: 'active',
    protected: false,
    enrichment_enabled: false,
    window_days: 14,
    context_layers: [],
    created_at: '',
    updated_at: '',
  }
}

function overviewRow(overrides: Partial<RadarBoard> = {}): RadarBoard {
  return {
    boardId: '1',
    label: '版块一',
    tagCount: 100,
    reportDate: GLOBAL_DATE,
    reportTitle: '',
    reportArticleCount: 0,
    reportEventCount: 0,
    anchorDate: GLOBAL_DATE,
    todayClues: 0,
    last3Clues: 0,
    prev3Clues: 0,
    attention: 0,
    heatRuleVersion: 'v2',
    clueDays: Array.from({ length: RAIL_AXIS_DAYS }, () => 0),
    dataDelayed: false,
    dataUnavailable: false,
    lanes: [],
    ...overrides,
  }
}

describe('railClueAxis — 共同轴重对齐（design D2）', () => {
  it('offset 0：14 格全部落在共同轴上，取值与后端逐日轴同序', () => {
    const days = Array.from({ length: RAIL_AXIS_DAYS }, (_, index) => index)
    const rows = buildRailBoardRows([board(1, '版块一')], [overviewRow({ clueDays: days })], GLOBAL_DATE)
    expect(rows).toHaveLength(1)
    expect(rows[0]!.cells).toEqual(days)
    expect(rows[0]!.cells.every(cell => cell !== null)).toBe(true)
  })

  it('offset 3：前 11 格有真实值，末 3 格未取到（斜纹），不补 0', () => {
    const days = Array.from({ length: RAIL_AXIS_DAYS }, (_, index) => index + 1)
    const rows = buildRailBoardRows(
      [board(1, '版块一')],
      [overviewRow({ clueDays: days, anchorDate: '2026-11-15' })],
      GLOBAL_DATE,
    )
    const cells = rows[0]!.cells
    expect(cells.slice(0, 11)).toEqual([4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14])
    expect(cells.slice(11)).toEqual([null, null, null])
  })

  it('offset ≥14：整行未取到（该版块窗口整体落在共同轴之外）', () => {
    const rows = buildRailBoardRows(
      [board(1, '版块一')],
      [overviewRow({ clueDays: Array(RAIL_AXIS_DAYS).fill(3), anchorDate: '2026-10-20' })],
      GLOBAL_DATE,
    )
    expect(rows[0]!.cells.every(cell => cell === null)).toBe(true)
    expect(rows[0]!.peak).toBe(0)
  })

  it('共同轴日期固定 14 天（oldest→newest，末日 = 全局锚点日）', () => {
    const dates = railAxisDates(GLOBAL_DATE)
    expect(dates).toHaveLength(RAIL_AXIS_DAYS)
    expect(dates[0]).toBe('2026-11-05')
    expect(dates[RAIL_AXIS_DAYS - 1]).toBe(GLOBAL_DATE)
    expect(railAxisDates('')).toEqual([])
  })
})

describe('railClueAxis — 未取到不等于安静', () => {
  it('data_unavailable 的行整行未取到（不显示全 0）', () => {
    const rows = buildRailBoardRows(
      [board(1, '版块一')],
      [overviewRow({ clueDays: [], dataUnavailable: true })],
      GLOBAL_DATE,
    )
    expect(rows[0]!.cells.every(cell => cell === null)).toBe(true)
  })

  it('后端没下发 clue_days（null）→ 整行未取到', () => {
    const rows = buildRailBoardRows(
      [board(1, '版块一')],
      [overviewRow({ clueDays: null })],
      GLOBAL_DATE,
    )
    expect(rows[0]!.cells.every(cell => cell === null)).toBe(true)
  })

  it('overview 里没有该版块的行 → 整行未取到（不拿别的版块的数据顶上）', () => {
    const rows = buildRailBoardRows(
      [board(1, '版块一'), board(2, '版块二')],
      [overviewRow({ boardId: '2', clueDays: Array.from({ length: 14 }, () => 1) })],
      GLOBAL_DATE,
    )
    expect(rows[0]!.cells.every(cell => cell === null)).toBe(true)
    expect(rows[1]!.cells.filter(cell => cell !== null)).toHaveLength(14)
  })

  // review R-7：共同轴对齐依赖「下标 ↔ 日期」严格对应；长度不符一律不猜（归一化层若 filter 掉
  // 非法项会整体前移一位，静默错日期还显示成有值——比整行未取到危险得多）。
  it('clue_days 长度不等于 14 → 整行未取到（不按下标猜日期）', () => {
    const rows = buildRailBoardRows(
      [board(1, '版块一')],
      [overviewRow({ clueDays: [1, 2, 3] })],
      GLOBAL_DATE,
    )
    expect(rows[0]!.cells.every(cell => cell === null)).toBe(true)
  })

  // review R-6：无锚点就无法断言 clue_days[i] 属于哪天，不得兜底 offset=0 把值贴到共同轴。
  it('缺 anchor_date → 整行未取到（宁可空，不可错位）', () => {
    const rows = buildRailBoardRows(
      [board(1, '版块一')],
      [overviewRow({ clueDays: Array.from({ length: 14 }, () => 2), anchorDate: undefined })],
      GLOBAL_DATE,
    )
    expect(rows[0]!.cells.every(cell => cell === null)).toBe(true)
    // title 文案分层：无锚点 ≠ 本次未取到（review R-5）。
    const title = railRowTitle(rows[0]!, railAxisDates(GLOBAL_DATE))
    expect(title).toContain('该版块无日报锚点')
    expect(title).not.toContain('本次未取到')
  })
})

describe('railClueAxis — 行序 / 峰值 / 强度', () => {
  it('行序按版块列表（display_order），不被 overview 行序带偏', () => {
    const rows = buildRailBoardRows(
      [board(1, '版块一'), board(2, '版块二')],
      [
        overviewRow({ boardId: '2', clueDays: [9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9] }),
        overviewRow({ boardId: '1', clueDays: [1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1] }),
      ],
      GLOBAL_DATE,
    )
    expect(rows.map(row => row.boardId)).toEqual([1, 2])
    expect(rows[0]!.peak).toBe(1)
    expect(rows[1]!.peak).toBe(9)
  })

  it('行整体强度按 14 天总量分档（0.42–1）：安静行压暗，最活跃行不压暗', () => {
    const rows = buildRailBoardRows(
      [board(1, '安静版块'), board(2, '活跃版块')],
      [
        overviewRow({ boardId: '1', clueDays: [1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0] }),
        overviewRow({ boardId: '2', clueDays: Array(RAIL_AXIS_DAYS).fill(5) }),
      ],
      GLOBAL_DATE,
    )
    expect(rows[1]!.intensity).toBeCloseTo(1, 5)
    expect(rows[0]!.intensity).toBeCloseTo(0.42 + 0.58 * (1 / 70), 5)
    expect(rows[0]!.intensity).toBeGreaterThanOrEqual(0.42)
    expect(rows[0]!.intensity).toBeLessThanOrEqual(1)
  })

  it('全部未取到时所有行都取强度下限（不出现 NaN）', () => {
    const rows = buildRailBoardRows([board(1, '版块一')], [], GLOBAL_DATE)
    expect(rows[0]!.intensity).toBeCloseTo(0.42, 5)
    expect(Number.isFinite(rows[0]!.intensity)).toBe(true)
  })
})

describe('railClueAxis — title（治理数据退到 title）', () => {
  it('title 带标签挂载数、锚点日与逐日明细；未取到写「未取到」而不是 0', () => {
    const rows = buildRailBoardRows(
      [board(1, '中东地缘政治与美伊关系', 409)],
      [overviewRow({ clueDays: [1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], anchorDate: '2026-11-16' })],
      GLOBAL_DATE,
    )
    const title = railRowTitle(rows[0]!, railAxisDates(GLOBAL_DATE))
    expect(title).toContain('中东地缘政治与美伊关系')
    expect(title).toContain('标签挂载 409')
    expect(title).toContain('数据截至 2026-11-16')
    expect(title).toContain('未取到')
  })
})
