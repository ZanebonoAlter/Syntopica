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

  it('anchorDate = 窗内最新有分区日；无泳道为 null', () => {
    const data = normalizeLaneDynamicsWindow(makeRaw())
    expect(data.anchorDate).toBe('2026-10-23')
    const empty = normalizeLaneDynamicsWindow({ window_days: 14, has_reports: true, lanes: [], candidates: [] })
    expect(empty.anchorDate).toBeNull()
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

  it('空日 / 有分区零线索：0 是合法已知值', () => {
    expect(dayCellMetrics(null)).toEqual({ peak: null, total: 0, sectionCount: 0, hasUnknown: false })
    expect(dayCellMetrics({ date: 'd', sections: [] }).sectionCount).toBe(0)
    const zero = {
      date: 'd', sections: [
        { sectionId: '1', label: 'a', threadCount: 0, fallbackClueCount: 0, foldedCount: 0, eventTitles: [] },
      ],
    }
    expect(dayCellMetrics(zero).peak).toBe(0)
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
