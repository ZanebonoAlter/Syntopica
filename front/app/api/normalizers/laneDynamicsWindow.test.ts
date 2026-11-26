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
    anchor_date: '2026-10-23',
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
            clue_count: 9,
            sections: [
              // 桶线索数 = thread_count − composite_count：桶1 = 3−1 = 2，桶2 = 7−0 = 7
              { section_id: 101, label: '桶1', thread_count: 3, composite_count: 1, events: ['t1'], folded_count: 1 },
              { section_id: 102, label: '桶2', thread_count: 7, composite_count: 0, events: ['t2', 't3'] },
            ],
          },
          {
            date: '2026-10-23',
            sections: [
              // 旧响应缺 composite_count → 桶线索数不可数（不得用 thread_count 冒充）
              { section_id: 103, label: '旧桶', thread_count: 3, events: ['e1', 'e2'], folded_count: 1 },
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

  it('桶线索数唯一公式：thread_count − composite_count；缺任一 → 未知（不冒充）', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    const lane = data.lanes[0]!
    const counted = lane.timeline[0]!.sections[0]!
    expect(counted.threadCount).toBe(3)
    expect(counted.compositeCount).toBe(1)
    expect(counted.clueCount).toBe(2)
    // 旧响应缺 composite_count → 桶线索数不可数（不得拿 thread_count 冒充线索数）
    const legacySection = lane.timeline[1]!.sections[0]!
    expect(legacySection.threadCount).toBe(3)
    expect(legacySection.compositeCount).toBeNull()
    expect(legacySection.clueCount).toBeNull()
    const unknown = data.lanes[1]!.timeline[0]!.sections[0]!
    expect(unknown.threadCount).toBeNull()
    expect(unknown.clueCount).toBeNull()
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
    expect(normalizeLaneDynamicsWindow(raw).lanes[0]!.timeline[0]!.clueCount).toBe(9)
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

  it('anchorDate 优先取后端 completed 锚点日；缺省回退窗内最新有分区日', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.anchorDate).toBe('2026-10-23')
    // 旧后端无 anchor_date → 回退数据最新日
    const legacy = makeRaw()
    delete legacy.anchor_date
    expect(normalizeLaneDynamicsWindow(legacy).anchorDate).toBe('2026-10-23')
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

/**
 * 「其他话题（未入泳道）」逐日读数的三态（`lane-matrix-coverage-and-others-row` 5.1）：
 * 字段缺省（旧后端）/ 空数组（有字段但窗口内无其他话题）/ 有值。
 */
describe('normalizeLaneDynamicsWindow — other_topics 三态', () => {
  it('字段缺省（旧后端）→ otherTopicsAvailable=false + 空对象（矩阵整行不渲染）', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.otherTopicsAvailable).toBe(false)
    expect(data.otherTopics).toEqual({})
  })

  it('空数组 → available=true + 空对象（与旧后端可区分）', () => {
    const raw = makeRaw()
    raw.other_topics = []
    const data = normalizeLaneDynamicsWindow(raw)
    expect(data.otherTopicsAvailable).toBe(true)
    expect(data.otherTopics).toEqual({})
  })

  it('有值 → date 为键，值带分区数与非综合稿线索数（含真实 0）', () => {
    const raw = makeRaw()
    raw.other_topics = [
      { date: '2026-10-22', sections: 3, clues: 7 },
      { date: '2026-10-23', sections: 0, clues: 0 },
    ]
    const data = normalizeLaneDynamicsWindow(raw)
    expect(data.otherTopicsAvailable).toBe(true)
    expect(data.otherTopics['2026-10-22']).toEqual({ date: '2026-10-22', sections: 3, clues: 7 })
    expect(data.otherTopics['2026-10-23']).toEqual({ date: '2026-10-23', sections: 0, clues: 0 })
  })

  it('数字不合法（非有限数 / 负数）→ 跳过该条，不拿 0 冒充', () => {
    const raw = makeRaw()
    raw.other_topics = [
      { date: '2026-10-22', sections: 3, clues: Number.NaN },
      { date: '2026-10-21', sections: -1, clues: 2 },
      { date: '', sections: 1, clues: 1 },
      { date: '2026-10-20', sections: 1, clues: 4 },
    ]
    const data = normalizeLaneDynamicsWindow(raw)
    expect(data.otherTopicsAvailable).toBe(true)
    expect(Object.keys(data.otherTopics)).toEqual(['2026-10-20'])
  })
})

describe('dayCellMetrics — 当日线索合计', () => {
  const day = (
    sections: { threadCount?: number | null, compositeCount?: number | null }[],
    clueCount?: number | null,
  ) => {
    const raw = {
      date: '2026-10-25',
      sections: sections.map((s, i) => ({
        section_id: i + 1,
        label: `桶${i + 1}`,
        // raw 是 snake_case 协议：threadCount/compositeCount 必须映射回 snake_case
        thread_count: s.threadCount,
        // composite_count 默认 0（无综合稿）；null = 缺省（旧响应 → 桶线索数不可数）
        composite_count: s.compositeCount === null ? undefined : (s.compositeCount ?? 0),
        events: [],
        folded_count: 0,
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

  it('桶峰值 = max、total = 各桶非综合稿合计、sectionCount 与未知标记（MX-1）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 3 }, { threadCount: 7 }, { threadCount: 1 }]))
    expect(metrics.peak).toBe(7)
    expect(metrics.total).toBe(11)
    expect(metrics.sectionCount).toBe(3)
    expect(metrics.hasUnknown).toBe(false)
    expect(metrics.clueCount).toBe(11)
    expect(metrics.bucketClues).toEqual([3, 7, 1])
  })

  it('综合稿不计入线索数：桶线索数 = thread_count − composite_count', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 5, compositeCount: 2 }, { threadCount: 4, compositeCount: 0 }]))
    expect(metrics.clueCount).toBe(7)
    expect(metrics.total).toBe(7)
    expect(metrics.bucketClues).toEqual([3, 4])
    expect(metrics.compositeCount).toBe(2)
    expect(metrics.clueCountSource).toBe('buckets')
  })

  it('后端 clue_count 优先于各桶合计（综合稿已排除，权威值）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 3, compositeCount: 0 }, { threadCount: 7, compositeCount: 0 }], 8))
    expect(metrics.clueCount).toBe(8)
    expect(metrics.total).toBe(8)
    expect(metrics.clueCountSource).toBe('backend')
  })

  it('clue_count 显式 0 是合法已知值（真零线索，不回落合计）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 0, compositeCount: 0 }], 0))
    expect(metrics.clueCount).toBe(0)
  })

  it('缺 clue_count：回退各桶已知非综合稿合计；全未知 → null（不造 0）', () => {
    expect(dayCellMetrics(day([{ threadCount: 3, compositeCount: 0 }, { threadCount: 2, compositeCount: 0 }])).clueCount).toBe(5)
    expect(dayCellMetrics(day([{ threadCount: null, compositeCount: null }])).clueCount).toBeNull()
    // 部分桶未知：已知部分求和，且明确标为下界（不冒充完整合计）
    const partial = dayCellMetrics(day([{ threadCount: 5, compositeCount: 0 }, { threadCount: null, compositeCount: null }]))
    expect(partial.clueCount).toBe(5)
    expect(partial.isLowerBound).toBe(true)
  })

  it('差值为负（综合稿数 > thread 总数，后端统计异常）→ 不可数，不抹成合法 0', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 1, compositeCount: 2 }]))
    expect(metrics.clueCount).toBeNull()
    expect(metrics.hasUnknown).toBe(true)
    expect(metrics.clueCountSource).toBe('none')
  })

  it('旧响应缺 composite_count → 桶线索数不可数（不拿 thread_count 冒充线索数）', () => {
    const metrics = dayCellMetrics(day([{ threadCount: 3, compositeCount: null }]))
    expect(metrics.clueCount).toBeNull()
    expect(metrics.hasUnknown).toBe(true)
    expect(metrics.compositeCount).toBeNull()
    expect(metrics.clueCountSource).toBe('none')
  })

  it('空日 / 有分区零线索：空集返回 0（与 gap 由 resolver 区分）；零是合法已知值', () => {
    expect(dayCellMetrics(null)).toEqual({
      clueCount: 0, total: 0, peak: null, sectionCount: 0,
      hasUnknown: false, compositeCount: 0, bucketClues: [],
      clueCountSource: 'backend', isLowerBound: false,
    })
    expect(dayCellMetrics({ date: 'd', sections: [], clueCount: null }).sectionCount).toBe(0)
    const zero = dayCellMetrics(day([{ threadCount: 0, compositeCount: 0 }]))
    expect(zero.peak).toBe(0)
    expect(zero.clueCount).toBe(0)
  })

  it('全未知日：peak null + hasUnknown（MX-2）', () => {
    const unknown = dayCellMetrics(day([{ threadCount: null, compositeCount: null }]))
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
