/**
 * 泳道动态观察窗 API 边界测试（矩阵阶段 G1）。
 *
 * 覆盖：snake_case → camelCase 归一、数字 ID 字符串化、thread_count 缺省三级语义
 * （已知数 / 回退 events+folded / 全未知）、snapshot null 与 detail 两级、anchorDate
 * 派生、日格三口径 dayCellMetrics（MX-1/2 变体）、失败透传不断言 data。
 */
import { describe, expect, it } from 'vitest'
import type { LaneDynamicsResponse } from '../laneDynamics'
import { mapLaneDynamicsWindowResponse } from '../laneDynamicsWindow'
import { dayCellMetrics, normalizeLaneDynamicsWindow } from './laneDynamicsWindow'

function makeRaw(): LaneDynamicsResponse {
  return {
    window_days: 14,
    has_reports: true,
    lanes: [
      {
        topic_id: 10,
        label: '主线 A',
        watch_linked: true,
        section_count_14d: 5,
        thread_count_14d: 9,
        snapshot: { summary: '滚动态势句', as_of: '2026-10-23', detail: '长版叙述' },
        timeline: [
          {
            date: '2026-10-22',
            sections: [
              { section_id: 101, label: '桶1', thread_count: 3, events: ['t1'], folded_count: 1 },
              { section_id: 102, label: '桶2', thread_count: 7, events: ['t2', 't3'] },
            ],
          },
          {
            date: '2026-10-23',
            sections: [
              // thread_count 缺省（旧响应）→ 回退 events.length + folded_count = 3
              { section_id: 103, label: '旧桶', events: ['e1', 'e2'], folded_count: 1 },
            ],
          },
        ],
      },
      {
        topic_id: 11,
        label: '无快照线',
        watch_linked: false,
        section_count_14d: 1,
        snapshot: null,
        timeline: [
          {
            date: '2026-10-20',
            sections: [
              // thread_count 与 events/folded 全缺 → 全未知
              { section_id: 110, label: '未知桶' } as never,
            ],
          },
        ],
      },
    ],
    candidates: [
      { topic_id: 99, label: '候选', last_seen_date: '2026-10-21', recent_hint: '最近动向' },
    ],
  }
}

describe('normalizeLaneDynamicsWindow', () => {
  it('snake_case → camelCase、数字 ID 字符串化、标量透传（F10 兼容不侵入）', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.windowDays).toBe(14)
    expect(data.hasReports).toBe(true)
    expect(data.lanes).toHaveLength(2)
    const lane = data.lanes[0]!
    expect(lane.topicId).toBe('10')
    expect(lane.watchLinked).toBe(true)
    expect(lane.sectionCountWindow).toBe(5)
    expect(lane.threadCountWindow).toBe(9)
    expect(lane.snapshot).toEqual({ summary: '滚动态势句', asOf: '2026-10-23', detail: '长版叙述' })
    expect(lane.timeline[0]!.sections[0]!.sectionId).toBe('101')
    expect(data.candidates[0]).toEqual({
      topicId: '99', label: '候选', lastSeenDate: '2026-10-21', recentHint: '最近动向',
    })
  })

  it('thread_count 缺省三级：已知数 / 回退 events+folded / 全未知（MX-2 变体）', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    const lane = data.lanes[0]!
    expect(lane.timeline[0]!.sections[0]!.threadCount).toBe(3)
    const legacySection = lane.timeline[1]!.sections[0]!
    expect(legacySection.threadCount).toBeNull()
    expect(legacySection.fallbackClueCount).toBe(3)
    const unknown = data.lanes[1]!.timeline[0]!.sections[0]!
    expect(unknown.threadCount).toBeNull()
    expect(unknown.fallbackClueCount).toBeNull()
  })

  it('snapshot null = 待结算（不影响 timeline）；detail 缺省归 null（两级可区分）', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.lanes[1]!.snapshot).toBeNull()
    expect(data.lanes[1]!.timeline).toHaveLength(1)
    const raw = makeRaw()
    raw.lanes[0]!.snapshot = { summary: 's', as_of: '2026-10-23' }
    expect(normalizeLaneDynamicsWindow(raw).lanes[0]!.snapshot).toEqual({
      summary: 's', asOf: '2026-10-23', detail: null,
    })
  })

  it('事件线/来源/热度与回填证据字段映射，显式 0 不走旧口径', () => {
    const raw = makeRaw()
    Object.assign(raw, { stories_backfilled: false, heat_rule_version: 'v1', heat_incomplete: true, heat_stale_feed_ids: [42] })
    Object.assign(raw.lanes[0]!, { lane_heat: 3.5, story_count_14d: 4 })
    Object.assign(raw.lanes[0]!.timeline[0]!, { story_count: 0, max_participant_count: 6, unassigned_event_count: 2 })
    const data = normalizeLaneDynamicsWindow(raw)
    expect(data).toMatchObject({ storiesBackfilled: false, heatRuleVersion: 'v1', heatIncomplete: true, heatStaleFeedIds: [42] })
    expect(data.lanes[0]).toMatchObject({ laneHeat: 3.5, storyCountWindow: 4 })
    expect(dayCellMetrics(data.lanes[0]!.timeline[0])).toMatchObject({ storyCount: 0, maxParticipantCount: 6, unassignedEventCount: 2, sectionCount: 2 })
    expect(data.lanes[1]!.timeline[0]!.storyCount).toBeNull()
    expect(dayCellMetrics(data.lanes[1]!.timeline[0]).storyCount).toBeNull()
    expect(normalizeLaneDynamicsWindow(makeRaw())).toMatchObject({ storiesBackfilled: null, heatRuleVersion: '', heatIncomplete: false, heatStaleFeedIds: [] })
  })

  it('anchorDate = 窗内最新有分区日；无泳道为 null', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.anchorDate).toBe('2026-10-23')
    const empty = normalizeLaneDynamicsWindow({ window_days: 14, has_reports: true, lanes: [], candidates: [] })
    expect(empty.anchorDate).toBeNull()
  })

  it('最热线字段映射（surface-story-heat-in-report-top）：透传 id/title 进 dayCellMetrics；缺省/显式 null 降级 null/空串不崩', () => {
    const raw = makeRaw()
    Object.assign(raw.lanes[0]!.timeline[0]!, { hottest_story_id: 55, hottest_story_title: '美伊博弈升级' })
    const data = normalizeLaneDynamicsWindow(raw)
    const named = data.lanes[0]!.timeline[0]!
    expect(named.hottestStoryId).toBe(55)
    expect(named.hottestStoryTitle).toBe('美伊博弈升级')
    expect(dayCellMetrics(named)).toMatchObject({ hottestStoryId: 55, hottestStoryTitle: '美伊博弈升级' })
    // 旧响应缺字段 → null/空串（不点名、不占位）
    const missing = data.lanes[0]!.timeline[1]!
    expect(missing.hottestStoryId).toBeNull()
    expect(missing.hottestStoryTitle).toBe('')
    expect(dayCellMetrics(missing)).toMatchObject({ hottestStoryId: null, hottestStoryTitle: '' })
    // 新后端显式 null（无最热线）→ 同缺省降级；标题空串不抵 #ID（点名判断只看 id）
    Object.assign(raw.lanes[0]!.timeline[1]!, { hottest_story_id: null, hottest_story_title: '' })
    const explicitNull = normalizeLaneDynamicsWindow(raw).lanes[0]!.timeline[1]!
    expect(explicitNull.hottestStoryId).toBeNull()
    expect(dayCellMetrics(explicitNull).hottestStoryId).toBeNull()
  })
})

describe('dayCellMetrics', () => {
  it('桶峰值 = max 而非合计/桶数（MX-1）', () => {
    const day = {
      date: '2026-10-22',
      sections: [
        { sectionId: '1', label: 'a', threadCount: 3, fallbackClueCount: 3, foldedCount: 0, eventTitles: [] },
        { sectionId: '2', label: 'b', threadCount: 7, fallbackClueCount: 7, foldedCount: 0, eventTitles: [] },
        { sectionId: '3', label: 'c', threadCount: 1, fallbackClueCount: 1, foldedCount: 0, eventTitles: [] },
      ],
    }
    const metrics = dayCellMetrics(day)
    expect(metrics.peak).toBe(7)
    expect(metrics.total).toBe(11)
    expect(metrics.sectionCount).toBe(3)
    expect(metrics.hasUnknown).toBe(false)
  })

  it('回退与未知：events+folded 回退 3；全未知 → peak null + hasUnknown（MX-2）', () => {
    const fallback = {
      date: 'd', sections: [
        { sectionId: '1', label: 'a', threadCount: null, fallbackClueCount: 3, foldedCount: 1, eventTitles: ['e1', 'e2'] },
      ],
    }
    expect(dayCellMetrics(fallback).peak).toBe(3)
    const unknown = {
      date: 'd', sections: [
        { sectionId: '1', label: 'a', threadCount: null, fallbackClueCount: null, foldedCount: 0, eventTitles: [] },
      ],
    }
    const metrics = dayCellMetrics(unknown)
    expect(metrics.peak).toBeNull()
    expect(metrics.hasUnknown).toBe(true)
    expect(metrics.sectionCount).toBe(1)
  })

  it('空日 / 有分区零线索：0 是合法已知值；空日无最热线身份（null/空串，不点名）', () => {
    expect(dayCellMetrics(null)).toEqual({ storyCount: 0, displayCount: 0, degradedFromClue: false, maxParticipantCount: 0, hottestStoryId: null, hottestStoryTitle: '', unassignedEventCount: 0, peak: null, total: 0, sectionCount: 0, hasUnknown: false })
    expect(dayCellMetrics({ date: 'd', sections: [] }).sectionCount).toBe(0)
    const zero = {
      date: 'd', sections: [
        { sectionId: '1', label: 'a', threadCount: 0, fallbackClueCount: 0, foldedCount: 0, eventTitles: [] },
      ],
    }
    expect(dayCellMetrics(zero).peak).toBe(0)
    expect(dayCellMetrics(zero).hottestStoryId).toBeNull()
    expect(dayCellMetrics(zero).hottestStoryTitle).toBe('')
  })
})

describe('mapLaneDynamicsWindowResponse', () => {
  it('成功：window 归一 + legacy 原样同引用（真实 raw 接线）', () => {
    const raw = makeRaw()
    const res = mapLaneDynamicsWindowResponse({ success: true, data: raw })
    expect(res.success).toBe(true)
    if (!res.success) return
    expect(res.data.legacy).toBe(raw)
    expect(res.data.window.lanes[0]!.topicId).toBe('10')
  })

  it('失败：error/status 透传、无 data；不双重断言', () => {
    const res = mapLaneDynamicsWindowResponse({ success: false, error: '网络错误', status: 500 })
    expect(res.success).toBe(false)
    if (res.success) return
    expect(res.error).toBe('网络错误')
    expect(res.status).toBe(500)
    expect('data' in res && res.data).toBeFalsy()
  })
})

// ── 展示降级口径（matrix-clue-fallback-and-rail-merge D2）───────────────────
// displayCount = 矩阵格值/顶部时间视窗曲线共用：事件线数 > 0 用事件线数，
// 否则线索合计兜底；storyCount null 维持未知（不造 0）；degradedFromClue
// 只在「事件线 0 且线索 > 0」为真（诚实降级标注的数据源）。
describe('dayCellMetrics — 展示降级口径 displayCount/degradedFromClue', () => {
  const day = (sections: { threadCount?: number | null, fallbackClueCount?: number | null }[], storyCount?: number | null) => {
    const raw = {
      date: '2026-10-25',
      sections: sections.map((s, i) => ({
        section_id: i + 1,
        label: `桶${i + 1}`,
        // raw 是 snake_case 协议：threadCount 必须映射回 thread_count
        thread_count: s.threadCount,
        events: s.fallbackClueCount === undefined ? [] : null,
        // null = 该桶全未知（events 与 folded_count 都缺）；数字 = events 折叠后的回退数
        folded_count: s.fallbackClueCount === null ? null : (s.fallbackClueCount ?? 0),
      })),
    }
    const payload = storyCount === undefined
      ? raw
      : { ...raw, story_count: storyCount }
    return normalizeLaneDynamicsWindow({
      window_days: 14,
      has_reports: true,
      lanes: [{ topic_id: 1, label: 'L', watch_linked: false, section_count_14d: 1, snapshot: null, timeline: [payload] }],
      candidates: [],
    } as unknown as LaneDynamicsResponse).lanes[0]!.timeline[0]!
  }

  it('事件线数 > 0：displayCount = 事件线数（不用线索合计混加），非降级', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 132 }], 3))
    expect(metrics.displayCount).toBe(3)
    expect(metrics.degradedFromClue).toBe(false)
  })

  it('事件线数 0 且线索 > 0（历史未回填日）：displayCount = 线索合计，标降级', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 100 }, { threadCount: 32 }], 0))
    expect(metrics.storyCount).toBe(0)
    expect(metrics.displayCount).toBe(132)
    expect(metrics.degradedFromClue).toBe(true)
  })

  it('事件线数 0 且线索 0（真零事件）：displayCount = 0，不标降级（零是合法已知值）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 0 }], 0))
    expect(metrics.displayCount).toBe(0)
    expect(metrics.degradedFromClue).toBe(false)
  })

  it('story_count 缺省（旧响应）桶已知：displayCount 与现状 storyCount 回退值相同（现状兼容）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 3 }, { threadCount: 2 }]))
    expect(metrics.storyCount).toBe(5)
    expect(metrics.displayCount).toBe(5)
    expect(metrics.degradedFromClue).toBe(false)
  })

  it('story_count 缺省且全桶未知：displayCount = null（不造 0）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: null, fallbackClueCount: null }]))
    expect(metrics.storyCount).toBeNull()
    expect(metrics.displayCount).toBeNull()
    expect(metrics.degradedFromClue).toBe(false)
  })

  it('story_count 0 且部分桶未知：displayCount 用已知部分求和兜底', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 5 }, { threadCount: null, fallbackClueCount: null }], 0))
    expect(metrics.displayCount).toBe(5)
    expect(metrics.degradedFromClue).toBe(true)
  })
})
