import { describe, expect, it } from 'vitest'
import {
  mapRadarOverviewResponse,
  normalizeRadarOverview,
  type BoardRadarRawOverview,
} from './boardRadar'

/**
 * 今日雷达客户端边界（rework-workshop-into-radar-and-desk 8A/R-18）：
 * 真实 raw snake_case 响应（对齐 GET /semantic-boards/overview 后端契约）
 * → camelCase/字符串 ID 读模型。接线测试不只改组件 mock：
 * normalizer 纯函数 + ApiResponse 分支两处都按真实 raw 形态验证。
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
        today_sections: 8,
        last3_sections: 13,
        prev3_sections: 10,
        attention: 26,
        data_delayed: false,
        days: Array.from({ length: 14 }, (_, i) => (i === 13 ? 8 : 1)),
        lanes: [
          {
            topic_id: 1204,
            label: '美伊博弈',
            section_count_14d: 15,
            thread_count_14d: 40,
            counts: Array.from({ length: 14 }, () => 1),
          },
        ],
      },
    ],
    ...over,
  }
}

describe('normalizeRadarOverview — raw → 客户端读模型', () => {
  it('完整 raw 响应：snake_case → camelCase，board/topic ID 字符串化', () => {
    const overview = normalizeRadarOverview(rawOverview())
    expect(overview.date).toBe('2026-10-24')
    expect(overview.boards).toHaveLength(1)
    const board = overview.boards[0]!
    expect(board.boardId).toBe('1974')
    expect(board.label).toBe('中东地缘政治与美伊关系')
    expect(board.tagCount).toBe(144)
    expect(board.reportDate).toBe('2026-10-24')
    expect(board.reportTitle).toBe('伊朗警告打击海湾海底电缆')
    expect(board.reportArticleCount).toBe(14)
    expect(board.reportEventCount).toBe(13)
    expect(board.anchorDate).toBe('2026-10-24')
    expect(board.todaySections).toBe(8)
    expect(board.last3Sections).toBe(13)
    expect(board.prev3Sections).toBe(10)
    expect(board.attention).toBe(26)
    expect(board.dataDelayed).toBe(false)
    expect(board.days).toHaveLength(14)
    const lane = board.lanes[0]!
    expect(lane.topicId).toBe('1204')
    expect(lane.label).toBe('美伊博弈')
    expect(lane.sectionCount14d).toBe(15)
    expect(lane.threadCount14d).toBe(40)
    expect(lane.counts).toHaveLength(14)
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
          today_sections: 0,
          last3_sections: 0,
          prev3_sections: 0,
          attention: 0,
          data_delayed: false,
          days: [],
          lanes: [],
        },
      ],
    })
    const board = overview.boards[0]!
    expect(board.boardId).toBe('50413')
    expect(board.anchorDate).toBeUndefined()
    expect(board.lanes).toEqual([])
    expect(board.days).toEqual([])
    expect(board.reportTitle).toBe('本周剧集与行业动向')
  })

  it('脏数据防御：非数字计数与非数组 days/lanes 落安全默认', () => {
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
        today_sections: Number.NaN,
        last3_sections: 4,
        prev3_sections: 2,
        attention: 5,
        data_delayed: true,
        days: undefined as unknown as number[],
        lanes: undefined as unknown as never[],
      }],
    })
    const board = overview.boards[0]!
    expect(board.tagCount).toBe(0)
    expect(board.todaySections).toBe(0)
    expect(board.dataDelayed).toBe(true)
    expect(board.days).toEqual([])
    expect(board.lanes).toEqual([])
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
