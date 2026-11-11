/**
 * 泳道动态观察窗 API 边界测试（矩阵阶段 G1；retire-story-adopt-event-metrics 3.1 换口径）。
 *
 * 覆盖：snake_case → camelCase 归一、数字 ID 字符串化、thread_count 缺省三级语义
 * （已知数 / 回退 events+folded / 全未知）、snapshot null 与 detail 两级、anchorDate
 * 派生、候选条目 anchor_date/clue_count/lane_heat、日格线索合计 dayCellMetrics
 * （clue_count 优先 + 旧响应回退）、失败透传不断言 data。
 */
import { describe, expect, it } from 'vitest'
import type { LaneDynamicsResponse } from '../laneDynamics'
import { mapLaneDynamicsWindowResponse } from '../laneDynamicsWindow'
import { dayCellMetrics, normalizeLaneDynamicsWindow } from './laneDynamicsWindow'

function makeRaw(): LaneDynamicsResponse {
  return {
    window_days: 14,
    has_reports: true,
    heat_rule_version: 'heat-v2-lane-clues-1d',
    lanes: [
      {
        topic_id: 10,
        label: '主线 A',
        watch_linked: true,
        section_count_14d: 5,
        thread_count_14d: 9,
        lane_heat: 7,
        snapshot: { summary: '滚动态势句', as_of: '2026-10-23', detail: '长版叙述' },
        timeline: [
          {
            date: '2026-10-22',
            clue_count: 8,
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
      {
        topic_id: 99,
        label: '候选',
        last_seen_date: '2026-10-21',
        recent_hint: '最近动向',
        anchor_date: '2026-10-23',
        clue_count: 4,
        lane_heat: 2,
      },
    ],
  }
}

describe('normalizeLaneDynamicsWindow', () => {
  it('snake_case → camelCase、数字 ID 字符串化、标量透传（F10 兼容不侵入）', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.windowDays).toBe(14)
    expect(data.hasReports).toBe(true)
    expect(data.heatRuleVersion).toBe('heat-v2-lane-clues-1d')
    expect(data.lanes).toHaveLength(2)
    const lane = data.lanes[0]!
    expect(lane.topicId).toBe('10')
    expect(lane.watchLinked).toBe(true)
    expect(lane.sectionCountWindow).toBe(5)
    expect(lane.threadCountWindow).toBe(9)
    expect(lane.laneHeat).toBe(7)
    expect(lane.snapshot).toEqual({ summary: '滚动态势句', asOf: '2026-10-23', detail: '长版叙述' })
    expect(lane.timeline[0]!.sections[0]!.sectionId).toBe('101')
    expect(data.candidates[0]).toEqual({
      topicId: '99',
      label: '候选',
      lastSeenDate: '2026-10-21',
      recentHint: '最近动向',
      anchorDate: '2026-10-23',
      clueCount: 4,
      laneHeat: 2,
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

  it('逐日 clue_count 透传；候选缺省字段降级 null（不造 0）', () => {
    const raw = makeRaw()
    expect(normalizeLaneDynamicsWindow(raw).lanes[0]!.timeline[0]!.clueCount).toBe(8)
    // 旧响应缺 clue_count → null（展示层按各桶合计派生）
    expect(normalizeLaneDynamicsWindow(raw).lanes[0]!.timeline[1]!.clueCount).toBeNull()

    const legacy = makeRaw()
    delete legacy.candidates![0]!.anchor_date
    delete legacy.candidates![0]!.clue_count
    delete legacy.candidates![0]!.lane_heat
    expect(normalizeLaneDynamicsWindow(legacy).candidates[0]).toMatchObject({
      anchorDate: null, clueCount: null, laneHeat: null,
    })
  })

  it('anchorDate = 窗内最新有分区日；无泳道为 null', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.anchorDate).toBe('2026-10-23')
    const empty = normalizeLaneDynamicsWindow({ window_days: 14, has_reports: true, lanes: [], candidates: [] })
    expect(empty.anchorDate).toBeNull()
  })

  it('退役字段不再进入读模型（事件线 / 来源数 / 回填证据）', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data).not.toHaveProperty('storiesBackfilled')
    expect(data).not.toHaveProperty('heatIncomplete')
    expect(data).not.toHaveProperty('heatStaleFeedIds')
    expect(data.lanes[0]).not.toHaveProperty('storyCountWindow')
    expect(data.lanes[0]!.timeline[0]).not.toHaveProperty('storyCount')
    expect(data.lanes[0]!.timeline[0]).not.toHaveProperty('hottestStoryId')
  })
})

describe('dayCellMetrics — 当日线索合计', () => {
  const day = (
    sections: { threadCount?: number | null, fallbackClueCount?: number | null }[],
    clueCount?: number | null,
  ) => {
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
    const payload = clueCount === undefined ? raw : { ...raw, clue_count: clueCount }
    return normalizeLaneDynamicsWindow({
      window_days: 14,
      has_reports: true,
      lanes: [{ topic_id: 1, label: 'L', watch_linked: false, section_count_14d: 1, snapshot: null, timeline: [payload] }],
      candidates: [],
    } as unknown as LaneDynamicsResponse).lanes[0]!.timeline[0]!
  }

  it('桶峰值 = max、total = 各桶合计、sectionCount 与未知标记（MX-1）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 3 }, { threadCount: 7 }, { threadCount: 1 }]))
    expect(metrics.peak).toBe(7)
    expect(metrics.total).toBe(11)
    expect(metrics.sectionCount).toBe(3)
    expect(metrics.hasUnknown).toBe(false)
    expect(metrics.clueCount).toBe(11)
  })

  it('后端 clue_count 优先于各桶合计（综合稿已排除，权威值）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 3 }, { threadCount: 7 }], 8))
    expect(metrics.clueCount).toBe(8)
    expect(metrics.total).toBe(10)
  })

  it('clue_count 显式 0 是合法已知值（真零线索，不回落合计）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 0 }], 0))
    expect(metrics.clueCount).toBe(0)
  })

  it('旧响应缺 clue_count：回退各桶已知合计；全未知 → null（不造 0）', () => {
    expect(dayCellMetrics(day([{ threadCount: 3 }, { threadCount: 2 }])).clueCount).toBe(5)
    expect(dayCellMetrics(day([{ threadCount: null, fallbackClueCount: null }])).clueCount).toBeNull()
    // 部分桶未知：用已知部分求和
    expect(dayCellMetrics(day([{ threadCount: 5 }, { threadCount: null, fallbackClueCount: null }])).clueCount).toBe(5)
  })

  it('空日 / 有分区零线索：空集返回 0（与 gap 由 resolver 区分）；零是合法已知值', () => {
    expect(dayCellMetrics(null)).toEqual({ clueCount: 0, total: 0, peak: null, sectionCount: 0, hasUnknown: false })
    expect(dayCellMetrics({ date: 'd', sections: [], clueCount: null }).sectionCount).toBe(0)
    const zero = dayCellMetrics(day([{ threadCount: 0, fallbackClueCount: 0 }]))
    expect(zero.peak).toBe(0)
    expect(zero.clueCount).toBe(0)
  })

  it('events+folded 回退 3；全未知 → peak null + hasUnknown（MX-2）', () => {
    const fallback = dayCellMetrics(day([{ threadCount: null, fallbackClueCount: 3 }]))
    expect(fallback.peak).toBe(3)
    expect(fallback.clueCount).toBe(3)
    const unknown = dayCellMetrics(day([{ threadCount: null, fallbackClueCount: null }]))
    expect(unknown.peak).toBeNull()
    expect(unknown.hasUnknown).toBe(true)
    expect(unknown.sectionCount).toBe(1)
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
