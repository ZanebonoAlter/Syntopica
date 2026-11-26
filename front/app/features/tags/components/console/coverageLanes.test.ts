/**
 * 覆盖组派生测试（`lane-matrix-coverage-and-others-row` 3.1 / design §4.1）。
 *
 * 口径：活跃话题 − A 档名单行；`last_seen_date` 缺失如实写 null；
 * 排序 = 最近一次日期降序 → 泳道名升序 → 话题 ID 升序。
 */
import { describe, expect, it } from 'vitest'
import type { BoardTopicListItem } from '~/api/dailyReports'
import { buildCoverageLanes } from './coverageLanes'
import type { ObservationLaneRow } from './observationLanes'

function topic(overrides: Partial<BoardTopicListItem> & { id: number, label: string }): BoardTopicListItem {
  return {
    semantic_board_id: 1974,
    description: '',
    status: 'active',
    first_seen_date: '2026-10-01',
    last_seen_date: '2026-11-09',
    hit_count: 1,
    consecutive_hits: 1,
    section_count: 0,
    color: '#fff',
    can_activate: true,
    ...overrides,
  }
}

function row(topicId: string): ObservationLaneRow {
  return {
    topicId,
    label: `泳道${topicId}`,
    watchLinked: false,
    sectionTotal: 1,
    clueTotal: 1,
    dormant: false,
    snapshot: null,
  }
}

describe('buildCoverageLanes', () => {
  it('只留 active 话题：candidate / archived 不进覆盖组', () => {
    const lanes = buildCoverageLanes([], [
      topic({ id: 1, label: '活跃', status: 'active' }),
      topic({ id: 2, label: '候选', status: 'candidate' }),
      topic({ id: 3, label: '已归档', status: 'archived' }),
    ])
    expect(lanes.map(lane => lane.topicId)).toEqual(['1'])
  })

  it('已在 A 档名单里的泳道不重复出现（覆盖组与矩阵行零重复）', () => {
    const lanes = buildCoverageLanes([row('10')], [
      topic({ id: 10, label: '窗口内有分区' }),
      topic({ id: 11, label: '窗口外' }),
    ])
    expect(lanes.map(lane => lane.topicId)).toEqual(['11'])
  })

  it('排序：最近一次日期降序 → 泳道名升序 → 话题 ID 升序（无记录沉底）', () => {
    const lanes = buildCoverageLanes([], [
      topic({ id: 1, label: 'B', last_seen_date: '2026-11-09' }),
      topic({ id: 2, label: 'A', last_seen_date: '2026-11-09' }),
      topic({ id: 3, label: '最近', last_seen_date: '2026-11-20' }),
      topic({ id: 4, label: '无记录', last_seen_date: '' }),
      topic({ id: 5, label: '更早', last_seen_date: '2026-10-27' }),
    ])
    expect(lanes.map(lane => [lane.topicId, lane.label])).toEqual([
      ['3', '最近'],
      ['2', 'A'],
      ['1', 'B'],
      ['5', '更早'],
      ['4', '无记录'],
    ])
  })

  it('最近一次日期缺失（空串）→ null（展示层写「无记录」，不拿今天冒充）', () => {
    const lanes = buildCoverageLanes([], [
      topic({ id: 1, label: '空串', last_seen_date: '' }),
      topic({ id: 2, label: '有值', last_seen_date: '2026-11-09' }),
    ])
    expect(lanes.find(lane => lane.topicId === '1')!.lastSeenDate).toBeNull()
    expect(lanes.find(lane => lane.topicId === '2')!.lastSeenDate).toBe('2026-11-09')
  })

  it('Go 时间字段（RFC3339）归一为 date-only，不带时分秒进矩阵', () => {
    const lanes = buildCoverageLanes([], [
      topic({ id: 1, label: 'RFC3339', last_seen_date: '2026-11-09T00:00:00Z' }),
      topic({ id: 2, label: '非日期', last_seen_date: '未知' }),
    ])
    expect(lanes.find(lane => lane.topicId === '1')!.lastSeenDate).toBe('2026-11-09')
    expect(lanes.find(lane => lane.topicId === '2')!.lastSeenDate).toBeNull()
  })

  it('名单为空 / 话题清单为空 → 空覆盖组（不造行）', () => {
    expect(buildCoverageLanes([], [])).toEqual([])
    expect(buildCoverageLanes([row('10')], [])).toEqual([])
  })
})
