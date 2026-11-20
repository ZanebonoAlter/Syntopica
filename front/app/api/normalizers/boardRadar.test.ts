import { describe, expect, it } from 'vitest'
import {
  mapRadarOverviewResponse,
  normalizeRadarOverview,
  normalizeRadarTopLanes,
  type BoardRadarRawOverview,
  type BoardRadarRawTopLanes,
} from './boardRadar'

/**
 * 今日雷达客户端边界（rework-workshop-into-radar-and-desk 8A/R-18；
 * retire-story-adopt-event-metrics 3.2 换当天线索口径）：
 * 真实 raw snake_case 响应（对齐 GET /semantic-boards/overview 后端契约）
 * → camelCase/字符串 ID 读模型。接线测试不只改组件 mock：
 * normalizer 纯函数 + ApiResponse 分支两处都按真实 raw 形态验证。
 * 旧后端（缺 clue_counts / today_clues / top_lanes）一律归一为 null（未知，不造 0）。
 */

function rawOverview(over: Partial<BoardRadarRawOverview> = {}): BoardRadarRawOverview {
  return {
    date: '2026-10-24',
    boards: [
      {
        board_id: 1974,
        label: '中东地缘政治与美伊关系',
        tag_count: 144,
        report_date: '2026-10-24',
        report_title: '伊朗警告打击海湾海底电缆',
        report_article_count: 14,
        report_event_count: 13,
        anchor_date: '2026-10-24',
        today_clues: 8,
        last3_clues: 13,
        prev3_clues: 10,
        attention: 26,
        heat_rule_version: 'heat-v2-lane-clues-1d',
        data_delayed: false,
        clue_days: Array.from({ length: 14 }, (_, i) => (i === 13 ? 8 : 1)),
        lanes: [
          {
            topic_id: 1204,
            label: '美伊博弈',
            clue_counts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 3 : 0)),
            thread_count_14d: 40,
          },
        ],
      },
    ],
    top_lanes: {
      anchor_date: '2026-10-24',
      lanes: [{
        board_id: 1974,
        board_label: '中东地缘政治与美伊关系',
        topic_id: 1204,
        topic_label: '美伊博弈',
        clue_count: 3,
        anchor_date: '2026-10-24',
      }],
      partial: false,
      degraded_board_ids: [],
      lagging_board_ids: [],
    },
    ...over,
  }
}

describe('normalizeRadarOverview — 版块行当天线索口径', () => {
  it('snake_case → camelCase + 字符串 ID：版块行、泳道、清单段逐字段', () => {
    const overview = normalizeRadarOverview(rawOverview())
    const board = overview.boards[0]!
    expect(board.boardId).toBe('1974')
    expect(board.label).toBe('中东地缘政治与美伊关系')
    expect(board.todayClues).toBe(8)
    expect(board.last3Clues).toBe(13)
    expect(board.prev3Clues).toBe(10)
    expect(board.attention).toBe(26)
    expect(board.heatRuleVersion).toBe('heat-v2-lane-clues-1d')
    expect(board.clueDays).toHaveLength(14)
    expect(board.clueDays![13]).toBe(8)
    expect(board.lanes[0]!.topicId).toBe('1204')
    expect(board.lanes[0]!.clueCounts).toHaveLength(14)
    expect(board.lanes[0]!.clueCounts![13]).toBe(3)
    expect(board.lanes[0]!.threadCount14d).toBe(40)

    expect(overview.topLanes).not.toBeNull()
    expect(overview.topLanes!.lanes[0]).toMatchObject({
      boardId: '1974',
      boardLabel: '中东地缘政治与美伊关系',
      topicId: '1204',
      topicLabel: '美伊博弈',
      clueCount: 3,
      anchorDate: '2026-10-24',
    })
  })

  it('退役字段（事件线/分区/旧热度）不下发到读模型：键集合收窄', () => {
    const board = normalizeRadarOverview(rawOverview()).boards[0]!
    for (const retired of [
      'todaySections', 'last3Sections', 'prev3Sections', 'days',
      'todayStories', 'last3Stories', 'prev3Stories',
      'todayHeat', 'last3Heat', 'prev3Heat', 'heatDays',
      'storiesBackfilled', 'heatIncomplete',
    ]) {
      expect(board).not.toHaveProperty(retired)
    }
    expect(board.lanes[0]).not.toHaveProperty('storyCounts')
    expect(board.lanes[0]).not.toHaveProperty('counts')
    expect(board.lanes[0]).not.toHaveProperty('sectionCount14d')
  })

  it('旧后端缺 clue_counts / today_clues / clue_days → null（未知，不造 0）', () => {
    const raw = rawOverview()
    delete raw.boards[0]!.today_clues
    delete raw.boards[0]!.last3_clues
    delete raw.boards[0]!.clue_days
    delete raw.boards[0]!.lanes[0]!.clue_counts
    const board = normalizeRadarOverview(raw).boards[0]!
    expect(board.todayClues).toBeNull()
    expect(board.last3Clues).toBeNull()
    expect(board.clueDays).toBeNull()
    expect(board.lanes[0]!.clueCounts).toBeNull()
  })

  it('真实全零线索不等于旧字段缺失：0 原样保留', () => {
    const raw = rawOverview()
    Object.assign(raw.boards[0]!, { today_clues: 0, last3_clues: 0, prev3_clues: 0 })
    raw.boards[0]!.lanes[0]!.clue_counts = Array.from({ length: 14 }, () => 0)
    const board = normalizeRadarOverview(raw).boards[0]!
    expect(board.todayClues).toBe(0)
    expect(board.last3Clues).toBe(0)
    expect(board.lanes[0]!.clueCounts!.every(count => count === 0)).toBe(true)
  })

  it('零值/缺省字段：无日报无泳道版块（影视/卫生形态）归一不抛错', () => {
    const overview = normalizeRadarOverview({
      date: '2026-10-24',
      boards: [
        {
          board_id: 50413,
          label: '影视作品',
          tag_count: 0,
          report_date: '2026-10-24',
          report_title: '本周剧集与行业动向',
          report_article_count: 3,
          report_event_count: 2,
          today_clues: 0,
          last3_clues: 0,
          prev3_clues: 0,
          attention: 0,
          data_delayed: false,
          clue_days: [],
          lanes: [],
        },
      ],
    })
    const board = overview.boards[0]!
    expect(board.boardId).toBe('50413')
    expect(board.anchorDate).toBeUndefined()
    expect(board.lanes).toEqual([])
    expect(board.clueDays).toEqual([])
    expect(board.reportTitle).toBe('本周剧集与行业动向')
  })

  it('脏数据防御：非数字计数与非数组 clue_days/lanes 落安全默认', () => {
    const overview = normalizeRadarOverview({
      date: '2026-10-24',
      boards: [{
        board_id: 1,
        label: 'x',
        tag_count: 'many' as unknown as number,
        report_date: '',
        report_title: '',
        report_article_count: undefined as unknown as number,
        report_event_count: 0,
        today_clues: Number.NaN,
        last3_clues: 4,
        prev3_clues: 2,
        attention: 5,
        data_delayed: true,
        clue_days: undefined as unknown as number[],
        lanes: undefined as unknown as never[],
      }],
    })
    const board = overview.boards[0]!
    expect(board.tagCount).toBe(0)
    expect(board.todayClues).toBeNull()
    expect(board.last3Clues).toBe(4)
    expect(board.dataDelayed).toBe(true)
    expect(board.clueDays).toBeNull()
    expect(board.lanes).toEqual([])
  })
})

describe('normalizeRadarOverview — 「没取到 ≠ 0」（fix-radar-overview-false-zero）', () => {
  it('未取到的行：data_unavailable=true + 计数类字段归一为 null（未知，不造 0）', () => {
    const raw = rawOverview()
    Object.assign(raw.boards[0]!, {
      data_unavailable: true,
      attention: null,
      today_clues: null,
      last3_clues: null,
      prev3_clues: null,
      report_article_count: null,
      report_event_count: null,
      report_date: '',
      report_title: '',
      anchor_date: '',
      clue_days: [],
      lanes: [],
    })
    const board = normalizeRadarOverview(raw).boards[0]!
    expect(board.dataUnavailable).toBe(true)
    expect(board.attention).toBeNull()
    expect(board.todayClues).toBeNull()
    expect(board.last3Clues).toBeNull()
    expect(board.prev3Clues).toBeNull()
    expect(board.reportArticleCount).toBeNull()
    expect(board.reportEventCount).toBeNull()
  })

  it('真实零值行：计数为 0 且 data_unavailable=false（0 不等于没取到）', () => {
    const raw = rawOverview()
    Object.assign(raw.boards[0]!, {
      attention: 0,
      today_clues: 0,
      last3_clues: 0,
      prev3_clues: 0,
      report_article_count: 0,
      report_event_count: 0,
    })
    const board = normalizeRadarOverview(raw).boards[0]!
    expect(board.dataUnavailable).toBe(false)
    expect(board.attention).toBe(0)
    expect(board.todayClues).toBe(0)
    expect(board.reportArticleCount).toBe(0)
    expect(board.reportEventCount).toBe(0)
  })

  it('缺 data_unavailable 字段（旧后端）：按取到处理，缺失的计数字段归一为 null', () => {
    const raw = rawOverview()
    delete raw.boards[0]!.data_unavailable
    delete raw.boards[0]!.attention
    delete raw.boards[0]!.report_article_count
    const board = normalizeRadarOverview(raw).boards[0]!
    expect(board.dataUnavailable).toBe(false)
    expect(board.attention).toBeNull()
    expect(board.reportArticleCount).toBeNull()
    expect(board.reportEventCount).toBe(13)
  })
})

describe('normalizeRadarTopLanes — 当天线索最多的泳道清单段', () => {
  const rawFeed: BoardRadarRawTopLanes = {
    anchor_date: '2026-10-24',
    lanes: [
      {
        board_id: 1974,
        board_label: '中东地缘政治与美伊关系',
        topic_id: 1204,
        topic_label: '美伊博弈',
        clue_count: 12,
        anchor_date: '2026-10-24',
      },
      {
        board_id: 3122,
        board_label: '能源价格',
        topic_id: 88,
        topic_label: '原油出口',
        clue_count: 5,
        anchor_date: '2026-10-23',
      },
    ],
    partial: true,
    degraded_board_ids: [9, 10],
    lagging_board_ids: [3122],
  }

  it('条目与标记逐字段归一（字符串 ID、锚点日、partial/降级/滞后）', () => {
    const feed = normalizeRadarTopLanes(rawFeed)
    expect(feed.anchorDate).toBe('2026-10-24')
    expect(feed.lanes).toHaveLength(2)
    expect(feed.lanes[0]).toMatchObject({ boardId: '1974', topicId: '1204', clueCount: 12 })
    // 滞后版块按自己的锚点日计，不冒充全局日
    expect(feed.lanes[1]).toMatchObject({ boardId: '3122', anchorDate: '2026-10-23' })
    expect(feed.partial).toBe(true)
    expect(feed.degradedBoardIds).toEqual(['9', '10'])
    expect(feed.laggingBoardIds).toEqual(['3122'])
  })

  it('空/坏字段降级：缺数组落空数组、不抛错', () => {
    const feed = normalizeRadarTopLanes({
      ...rawFeed,
      lanes: undefined as unknown as BoardRadarRawTopLanes['lanes'],
      degraded_board_ids: undefined as unknown as number[],
      partial: undefined as unknown as boolean,
    })
    expect(feed.lanes).toEqual([])
    expect(feed.degradedBoardIds).toEqual([])
    expect(feed.partial).toBe(false)
  })
})

describe('normalizeRadarOverview — 泳道清单段的有无（旧响应回退）', () => {
  it('响应带 top_lanes 段 → 归一后的读模型带该段', () => {
    const overview = normalizeRadarOverview(rawOverview())
    expect(overview.topLanes).not.toBeNull()
    expect(overview.topLanes!.lanes).toHaveLength(1)
  })

  it('旧后端响应（无 top_lanes 段）→ topLanes 为 null（调用方回退版块级列表，不渲染业务空态）', () => {
    const raw = rawOverview()
    delete raw.top_lanes
    expect(normalizeRadarOverview(raw).topLanes).toBeNull()
  })
})

describe('mapRadarOverviewResponse — ApiResponse 分支接线', () => {
  it('成功分支：data 归一化，success/分页/消息透传', () => {
    const res = mapRadarOverviewResponse({ success: true, data: rawOverview(), pagination: undefined, message: 'ok' })
    expect(res.success).toBe(true)
    if (!res.success) throw new Error('判别窄化失败')
    expect(res.data.boards[0]!.boardId).toBe('1974')
    expect(res.data.boards[0]!.lanes[0]!.topicId).toBe('1204')
    expect(res.message).toBe('ok')
  })

  it('失败分支：raw 透传不转换（调用方按 error 处理）', () => {
    const failed = { success: false, error: '接口挂了' }
    expect(mapRadarOverviewResponse(failed)).toEqual(failed)
  })

  it('失败分支：status/错误体 raw data 原样透传，不归一不丢字段', () => {
    const failed = {
      success: false as const,
      error: '冲突',
      status: 409,
      message: 'already_running',
      data: { date: '', boards: [] },
    }
    const res = mapRadarOverviewResponse(failed)
    expect(res).toEqual(failed)
    if (!res.success) {
      expect(res.status).toBe(409)
      expect(res.data).toEqual({ date: '', boards: [] })
    }
  })

  it('异常 success 无 data：收敛为失败透传，字段不丢（消费行为与旧实现一致）', () => {
    const res = mapRadarOverviewResponse({ success: true })
    expect(res.success).toBe(false)
    if (!res.success) {
      expect(res.error).toBeUndefined()
    }
  })
})
