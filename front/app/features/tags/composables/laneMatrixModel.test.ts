/**
 * 矩阵纯模型层用例（laneMatrixModel.ts）。
 *
 * 本 change 新建（fix-lane-metric-provenance）：旧路径的当日线索合计回退曾用
 * 「含综合稿的 thread_count / events+folded」冒充线索数——矩阵格值口径必须统一为
 * **非综合稿线索数**（桶线索数 = thread_count − composite_count，API 归一层唯一公式）。
 *
 * 判据（机械断言）：
 * - 后端 clue_count 优先；
 * - 缺 clue_count 时回退各桶**非综合稿**线索数（综合稿不计入）；
 * - 桶线索数不可数（旧响应缺 composite_count）时该桶不贡献合计（不冒充）。
 */
import { describe, expect, it } from 'vitest'
import type { LaneDynamicsLane, LaneTimelineSection } from '~/api/laneDynamics'
import {
  DEFAULT_WINDOW_DAYS,
  buildDayAxis,
  buildLaneRow,
  buildLaneRows,
  normalizeWindowDays,
  parseLaneParam,
  splitDormantRows,
} from './laneMatrixModel'

function section(id: number, threadCount: number | undefined, compositeCount: number | undefined): LaneTimelineSection {
  return {
    section_id: id,
    label: `桶${id}`,
    thread_count: threadCount,
    composite_count: compositeCount,
    events: [],
    folded_count: 0,
  }
}

function lane(timeline: LaneDynamicsLane['timeline'], overrides: Partial<LaneDynamicsLane> = {}): LaneDynamicsLane {
  return {
    topic_id: 10,
    label: '主线 A',
    watch_linked: false,
    section_count_14d: timeline.reduce((sum, day) => sum + day.sections.length, 0),
    snapshot: null,
    timeline,
    ...overrides,
  } as LaneDynamicsLane
}

describe('laneMatrixModel — 当日线索合计回退（fix-lane-metric-provenance）', () => {
  it('后端 clue_count 优先（各桶 thread 合计不覆盖）', () => {
    const row = buildLaneRow(
      lane([{ date: '2026-10-23', clue_count: 3, sections: [section(1, 5, 2)] }]),
      ['2026-10-23'],
    )
    expect(row.clueCounts['2026-10-23']).toBe(3)
    expect(row.clueTotal).toBe(3)
  })

  it('缺 clue_count：回退各桶**非综合稿**线索数（综合稿不计入）', () => {
    const row = buildLaneRow(
      lane([{ date: '2026-10-23', sections: [section(1, 5, 2), section(2, 4, 0)] }]),
      ['2026-10-23'],
    )
    // 5−2 + 4−0 = 7（不是含综合稿的 9）
    expect(row.clueCounts['2026-10-23']).toBe(7)
    expect(row.clueTotal).toBe(7)
  })

  it('旧响应缺 composite_count：桶线索数不可数 → 不拿 thread_count 冒充', () => {
    const row = buildLaneRow(
      lane([{ date: '2026-10-23', sections: [section(1, 3, undefined)] }]),
      ['2026-10-23'],
    )
    expect(row.clueCounts['2026-10-23']).toBe(0)
  })

  it('断档判定仍用分区数；休眠分组与行序口径不因回退变化', () => {
    const rows = buildLaneRows([
      lane([{ date: '2026-10-23', sections: [section(1, 2, 0), section(2, 1, 0), section(3, 1, 0)] }], { topic_id: 10, lane_heat: 3 }),
      lane([{ date: '2026-10-23', sections: [section(4, 1, 0)] }], { topic_id: 11, lane_heat: 1 }),
    ], ['2026-10-23'])
    expect(rows.map(row => row.topicId)).toEqual([10, 11])
    expect(rows[0]!.counts['2026-10-23']).toBe(3)
    expect(rows[0]!.clueCounts['2026-10-23']).toBe(4)
    const { active, dormant } = splitDormantRows(rows)
    // 休眠阈值 = 分区数 ≤2（与线索数无关）
    expect(active.map(row => row.topicId)).toEqual([10])
    expect(dormant.map(row => row.topicId)).toEqual([11])
  })
})

describe('laneMatrixModel — 轴与参数（回归网）', () => {
  it('buildDayAxis：末日回溯 N 天（含首尾，升序）；非法输入 → 空数组', () => {
    const axis = buildDayAxis('2026-10-23', 3)
    expect(axis).toEqual(['2026-10-21', '2026-10-22', '2026-10-23'])
    expect(buildDayAxis('不是日期', 3)).toEqual([])
  })

  it('normalizeWindowDays：只接受选项内值，其余回落默认 14', () => {
    expect(normalizeWindowDays(7)).toBe(7)
    expect(normalizeWindowDays(365)).toBe(365)
    expect(normalizeWindowDays(13)).toBe(DEFAULT_WINDOW_DAYS)
    expect(normalizeWindowDays('30')).toBe(30)
    expect(normalizeWindowDays(undefined)).toBe(DEFAULT_WINDOW_DAYS)
  })

  it('parseLaneParam：正整数字符串 → id；其余 → null', () => {
    expect(parseLaneParam('12')).toBe(12)
    expect(parseLaneParam('0')).toBeNull()
    expect(parseLaneParam('-1')).toBeNull()
    expect(parseLaneParam('abc')).toBeNull()
  })
})
