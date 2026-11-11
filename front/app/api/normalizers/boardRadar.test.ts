import { describe, expect, it } from 'vitest'
import {
  mapRadarOverviewResponse,
  normalizeRadarOverview,
  normalizeRadarStoryFeed,
  type BoardRadarRawOverview,
  type BoardRadarRawStoryFeed,
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
        today_stories: 6,
        last3_stories: 9,
        prev3_stories: 7,
        today_heat: 5.2,
        last3_heat: 8.4,
        prev3_heat: 6.1,
        heat_days: Array.from({ length: 14 }, () => 0.5),
        heat_rule_version: 'heat-v1-48h-hl24h',
        heat_incomplete: true,
        stories_backfilled: true,
        lanes: [
          {
            topic_id: 1204,
            label: '美伊博弈',
            section_count_14d: 15,
            thread_count_14d: 40,
            counts: Array.from({ length: 14 }, () => 1),
            story_counts: Array.from({ length: 14 }, (_, i) => (i === 13 ? 3 : 0)),
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
    // add-event-story-heat 3b：事件线/热度字段（attention 已是 lane_heat 求和的 float）
    expect(board.todayStories).toBe(6)
    expect(board.storiesBackfilled).toBe(true)
    expect(board.last3Stories).toBe(9)
    expect(board.prev3Stories).toBe(7)
    expect(board.hasHeatData).toBe(true)
    expect(board.todayHeat).toBeCloseTo(5.2, 5)
    expect(board.last3Heat).toBeCloseTo(8.4, 5)
    expect(board.prev3Heat).toBeCloseTo(6.1, 5)
    expect(board.heatDays).toHaveLength(14)
    expect(board.heatRuleVersion).toBe('heat-v1-48h-hl24h')
    expect(board.heatIncomplete).toBe(true)
    const lane = board.lanes[0]!
    expect(lane.topicId).toBe('1204')
    expect(lane.label).toBe('美伊博弈')
    expect(lane.sectionCount14d).toBe(15)
    expect(lane.threadCount14d).toBe(40)
    expect(lane.counts).toHaveLength(14)
    expect(lane.storyCounts).toHaveLength(14)
    expect(lane.storyCounts[13]).toBe(3)
    // 新后端下发 story_counts → 已知性是 true（展示层按事件线措辞）
    expect(lane.storyCountsKnown).toBe(true)
  })

  it('attention 为 float（lane_heat 求和，保留一位小数）时 num() 兼容不丢精度', () => {
    const overview = normalizeRadarOverview(rawOverview({
      boards: [{ ...rawOverview().boards[0]!, attention: 12.3 }],
    }))
    expect(overview.boards[0]!.attention).toBeCloseTo(12.3, 5)
  })

  it('stories_backfilled 三态：false 原样、缺省归 null（老 payload 未知），不影响 hasHeatData', () => {
    const [degraded, legacy] = normalizeRadarOverview(rawOverview({
      boards: [
        { ...rawOverview().boards[0]!, board_id: 2, stories_backfilled: false },
        { ...rawOverview().boards[0]!, board_id: 3, stories_backfilled: undefined },
      ],
    })).boards
    expect(degraded!.storiesBackfilled).toBe(false)
    expect(legacy!.storiesBackfilled).toBeNull()
    // 回退标记不是热度信号：三态下 hasHeatData 判定不受影响
    expect(degraded!.hasHeatData).toBe(true)
    expect(legacy!.hasHeatData).toBe(true)
  })

  it('缺字段降级（旧后端）：事件线/热度按 0、story_counts 回退 counts，不抛错不拿桶数冒充', () => {
    const overview = normalizeRadarOverview(rawOverview({
      boards: [{
        ...rawOverview().boards[0]!,
        today_stories: undefined,
        last3_stories: undefined,
        prev3_stories: undefined,
        today_heat: undefined,
        last3_heat: undefined,
        prev3_heat: undefined,
        heat_days: undefined,
        heat_rule_version: undefined,
        heat_incomplete: undefined,
        stories_backfilled: undefined,
        lanes: [{
          topic_id: 1204,
          label: '美伊博弈',
          section_count_14d: 15,
          thread_count_14d: 40,
          counts: Array.from({ length: 14 }, () => 1),
          story_counts: undefined,
        }],
      }],
    }))
    const board = overview.boards[0]!
    expect(board.todayStories).toBe(0)
    expect(board.last3Stories).toBe(0)
    expect(board.prev3Stories).toBe(0)
    expect(board.todayHeat).toBe(0)
    expect(board.last3Heat).toBe(0)
    expect(board.prev3Heat).toBe(0)
    expect(board.hasHeatData).toBe(false)
    expect(board.storiesBackfilled).toBeNull()
    expect(board.heatDays).toEqual([])
    expect(board.heatRuleVersion).toBe('')
    expect(board.heatIncomplete).toBe(false)
    // story_counts 缺省 → counts 兜底（回退值是分区数；不拿桶数冒充事件线数）
    expect(board.lanes[0]!.storyCounts).toEqual(board.lanes[0]!.counts)
    expect(board.lanes[0]!.storyCounts.every(count => count === 1)).toBe(true)
    // 回退的是**分区数**：已知性 false，展示层不得写成「条事件线」（R-2）
    expect(board.lanes[0]!.storyCountsKnown).toBe(false)
  })

  it('真实全零热度不等于旧字段缺失；部分缺失安全回退', () => {
    const raw = rawOverview()
    Object.assign(raw.boards[0]!, { today_stories: 0, today_heat: 0, last3_heat: 0, prev3_heat: 0 })
    expect(normalizeRadarOverview(raw).boards[0]).toMatchObject({ hasHeatData: true, todayHeat: 0, todayStories: 0 })
    raw.boards[0]!.last3_heat = undefined
    expect(normalizeRadarOverview(raw).boards[0]!.hasHeatData).toBe(false)
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

describe('normalizeRadarStoryFeed — 事件线清单段归一（surface-story-events-in-radar）', () => {
  const rawFeed: BoardRadarRawStoryFeed = {
    anchor_date: '2026-11-06',
    heat_rule_version: 'heat-v1-48h-hl24h',
    top_stories: [{
      board_id: 1974,
      board_label: '中东地缘政治与美伊关系',
      story_id: 4211,
      story_title: '美伊博弈升级',
      participants: 4,
      heat: 5.0123,
      baseline_heat: 1.2,
      ratio: 4.1769166666,
      has_baseline: true,
      has_today_evidence: true,
      anchor_date: '2026-11-05',
      report_date: '2026-11-06',
    }],
    // matrix-clue-fallback-and-rail-merge：warming/new_stories/new_count/
    // new_total_count 已随右栏徽标化移除；夹具保留一段「旧后端仍发这些字段」
    // 的形态，断言归一结果不再暴露它们（多余字段自然忽略，不冒充读模型）。
    warming: [],
    new_stories: [],
    warming_count: 6,
    new_count: 2,
    new_total_count: 7,
    cross_day_story_count: 3,
    unbackfilled_board_ids: [12],
    unbackfilled_board_count: 1,
    no_story_board_ids: [77],
    no_story_board_count: 1,
    lagging_board_ids: [3122],
    partial: false,
    degraded_board_ids: [],
  }

  it('完整段：camelCase + 字符串 ID，倍率/热度保原始精度（展示层再保留一位小数）', () => {
    const feed = normalizeRadarStoryFeed(rawFeed)
    expect(feed.anchorDate).toBe('2026-11-06')
    expect(feed.heatRuleVersion).toBe('heat-v1-48h-hl24h')
    const top = feed.topStories[0]!
    expect(top.boardId).toBe('1974')
    expect(top.storyId).toBe('4211')
    expect(top.storyTitle).toBe('美伊博弈升级')
    expect(top.participants).toBe(4)
    expect(top.heat).toBeCloseTo(5.0123, 6)
    expect(top.baselineHeat).toBeCloseTo(1.2, 6)
    expect(top.ratio).toBeCloseTo(4.1769, 4)
    expect(top.hasBaseline).toBe(true)
    expect(top.hasTodayEvidence).toBe(true)
    expect(top.anchorDate).toBe('2026-11-05')
    expect(top.reportDate).toBe('2026-11-06')
    // 全量计数（非截断数）；单清单化后只余 warming_count / cross_day_story_count
    expect(feed.warmingCount).toBe(6)
    expect(feed.crossDayStoryCount).toBe(3)
    // 徽标化去留（D4）：读模型不再有独立升温/新起清单与去 Top 计数
    expect('warming' in feed).toBe(false)
    expect('newStories' in feed).toBe(false)
    expect('newCount' in feed).toBe(false)
    expect('newTotalCount' in feed).toBe(false)
    // 徽标判定信息随 top_stories 条目自带（has_baseline/ratio/has_today_evidence）
    expect(top.hasBaseline).toBe(true)
    expect(top.ratio).toBeGreaterThan(2)
    // 降级标记
    expect(feed.unbackfilledBoardIds).toEqual(['12'])
    expect(feed.unbackfilledBoardCount).toBe(1)
    expect(feed.noStoryBoardIds).toEqual(['77'])
    expect(feed.noStoryBoardCount).toBe(1)
    expect(feed.laggingBoardIds).toEqual(['3122'])
    expect(feed.partial).toBe(false)
    expect(feed.degradedBoardIds).toEqual([])
  })

  it('缺字段安全默认：无基线条目 ratio 0 / has_baseline false、无落点为空串（徽标判定的降级形态）', () => {
    const feed = normalizeRadarStoryFeed({
      ...rawFeed,
      top_stories: [{ ...rawFeed.top_stories[0]!, story_title: '', has_baseline: false, ratio: 0, report_date: '' }],
    })
    const fresh = feed.topStories[0]!
    expect(fresh.storyTitle).toBe('')
    expect(fresh.hasBaseline).toBe(false)
    expect(fresh.ratio).toBe(0)
    expect(fresh.reportDate).toBe('')
  })

  it('participants 缺失 → null（省略数字，不伪造 0）', () => {
    const feed = normalizeRadarStoryFeed({
      ...rawFeed,
      top_stories: [{ ...rawFeed.top_stories[0]!, participants: undefined }],
    })
    expect(feed.topStories[0]!.participants).toBeNull()
  })

  it('空/坏字段降级：缺数组落空数组、不抛错', () => {
    const feed = normalizeRadarStoryFeed({
      ...rawFeed,
      top_stories: undefined as unknown as BoardRadarRawStoryFeed['top_stories'],
      unbackfilled_board_ids: undefined as unknown as number[],
      partial: undefined as unknown as boolean,
    })
    expect(feed.topStories).toEqual([])
    expect(feed.unbackfilledBoardIds).toEqual([])
    expect(feed.partial).toBe(false)
  })
})

describe('normalizeRadarOverview — 事件线清单段的有无（旧响应回退）', () => {
  it('响应带 stories 段 → 归一后的读模型带该段', () => {
    const overview = normalizeRadarOverview({
      ...rawOverview(),
      stories: {
        anchor_date: '2026-10-24',
        heat_rule_version: 'heat-v1-48h-hl24h',
        top_stories: [],
        warming: [],
        new_stories: [],
        warming_count: 0,
        new_count: 0,
        unbackfilled_board_ids: [],
        unbackfilled_board_count: 0,
        lagging_board_ids: [],
        partial: false,
        degraded_board_ids: [],
      },
    })
    expect(overview.stories).not.toBeNull()
    expect(overview.stories!.topStories).toEqual([])
  })

  it('旧后端响应（无 stories 段）→ stories 为 null（调用方回退版块级列表，不渲染业务空态）', () => {
    const overview = normalizeRadarOverview(rawOverview())
    expect(overview.stories).toBeNull()
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

  it('cross_day_story_count：0 是有效值，缺失归一为 null（两者语义不同）', () => {
    const zero = normalizeRadarStoryFeed({ cross_day_story_count: 0 } as BoardRadarRawStoryFeed)
    expect(zero.crossDayStoryCount).toBe(0)

    const missing = normalizeRadarStoryFeed({} as BoardRadarRawStoryFeed)
    expect(missing.crossDayStoryCount).toBeNull()

    const garbage = normalizeRadarStoryFeed({ cross_day_story_count: 'x' } as unknown as BoardRadarRawStoryFeed)
    expect(garbage.crossDayStoryCount).toBeNull()
  })
})
