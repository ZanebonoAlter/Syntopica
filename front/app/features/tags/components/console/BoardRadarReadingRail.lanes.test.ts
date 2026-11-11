/**
 * BoardRadarReadingRail 泳道清单测试（retire-story-adopt-event-metrics 3.2 / design D10）。
 *
 * 右栏条目单位从事件线改为「当天线索最多的泳道」：
 *  - 跨版块混排顺序由服务端给出，前端不合并、不重排（同分稳定排序也由服务端保证）；
 *  - 每条带版块名 + 泳道名 + 当天线索数 + 该版块自己的锚点日；
 *  - 上限 8 由服务端截断，前端不另设屏高逻辑；
 *  - 无「新起 / 升温 N×」徽标、无跨天延续脚注；
 *  - 空态与「本次排序不完整」标注保留；旧响应（无 top_lanes）回退版块级日报列表。
 */
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardRadarReadingRail from './BoardRadarReadingRail.vue'
import type { RadarBoard, RadarTopLaneItem } from '~/api/semanticBoards'

function lane(boardId: string, topicId: string, over: Partial<RadarTopLaneItem> = {}): RadarTopLaneItem {
  return {
    boardId,
    boardLabel: `版块${boardId}`,
    topicId,
    topicLabel: `泳道${topicId}`,
    clueCount: 10,
    anchorDate: '2026-11-10',
    ...over,
  }
}

function board(over: Partial<RadarBoard> = {}): RadarBoard {
  return {
    boardId: '1974',
    label: '中东地缘政治与美伊关系',
    tagCount: 144,
    reportDate: '2026-11-10',
    reportTitle: '伊朗警告打击海湾海底电缆',
    reportArticleCount: 14,
    reportEventCount: 13,
    todayClues: 8,
    last3Clues: 13,
    prev3Clues: 10,
    attention: 26,
    heatRuleVersion: 'heat-v2-lane-clues-1d',
    clueDays: [],
    dataDelayed: false,
    lanes: [],
    ...over,
  }
}

function mountRail(overrides: Record<string, unknown> = {}) {
  return mount(BoardRadarReadingRail, {
    props: {
      laneMode: true,
      laneItems: [lane('1974', '1204', { clueCount: 12 })],
      partial: false,
      degradedCount: 0,
      laggingCount: 0,
      todayRead: [board()],
      ...overrides,
    },
  })
}

describe('BoardRadarReadingRail — 当天线索最多的泳道清单', () => {
  it('条目 = 版块名 + 泳道名 + 当天线索数 + 该版块自己的锚点日', () => {
    const wrapper = mountRail({
      laneItems: [lane('1974', '1204', { clueCount: 12, anchorDate: '2026-11-09' })],
    })
    const item = wrapper.find('[data-testid="radar-lane-1974-1204"]')
    expect(item.exists()).toBe(true)
    expect(item.text()).toContain('版块1974')
    expect(item.text()).toContain('泳道1204')
    expect(item.text()).toContain('12 条线索')
    // 滞后版块按自己的锚点日计，不冒充全局今天
    expect(item.text()).toContain('11/09')
    expect(item.attributes('aria-label')).toContain('12 条线索')
  })

  it('跨版块混排顺序照抄服务端（前端不合并、不重排）', () => {
    const wrapper = mountRail({
      laneItems: [
        lane('1', '11', { clueCount: 9 }),
        lane('2', '22', { clueCount: 7 }),
        lane('1', '33', { clueCount: 5 }),
      ],
    })
    const ids = wrapper.findAll('.rd-read').map(node => node.attributes('data-testid'))
    expect(ids).toEqual(['radar-lane-1-11', 'radar-lane-2-22', 'radar-lane-1-33'])
  })

  it('同分条目保持服务端给定顺序（稳定排序不在前端重算）', () => {
    const wrapper = mountRail({
      laneItems: [lane('2', '20'), lane('1', '10')],
    })
    const ids = wrapper.findAll('.rd-read').map(node => node.attributes('data-testid'))
    expect(ids).toEqual(['radar-lane-2-20', 'radar-lane-1-10'])
  })

  it('清单上限 8 条（服务端截断）：前端不另设截断、无「另有 N 条」摘要', () => {
    const wrapper = mountRail({
      laneItems: Array.from({ length: 8 }, (_, i) => lane('1', String(100 + i))),
    })
    expect(wrapper.findAll('.rd-read')).toHaveLength(8)
    expect(wrapper.text()).not.toContain('另有')
  })

  it('徽标与跨天脚注整体退役：不渲染「新起」「升温 N×」与「跨天延续建设中」', () => {
    const wrapper = mountRail({
      laneItems: [lane('1974', '1204', { clueCount: 30 })],
      partial: true,
      degradedCount: 2,
    })
    const text = wrapper.text()
    expect(text).not.toContain('新起')
    expect(text).not.toContain('升温')
    expect(text).not.toContain('×')
    expect(text).not.toContain('跨天延续')
    expect(wrapper.find('[data-testid="radar-badge-legend"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-crossday-note"]').exists()).toBe(false)
  })

  it('空清单渲染空态（不是空白，也不说成「今天没有事件」）', () => {
    const wrapper = mountRail({ laneItems: [] })
    const empty = wrapper.find('[data-testid="radar-no-lane"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('今天没有可排的泳道')
    expect(wrapper.text()).not.toContain('今天没有事件')
  })

  it('部分版块延迟/降级：明示「本次泳道排序不完整」并带降级版块数', () => {
    const wrapper = mountRail({ partial: true, degradedCount: 2 })
    const note = wrapper.find('[data-testid="radar-partial"]')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('部分版块数据延迟或降级')
    expect(note.text()).toContain('2 个版块')
    expect(note.text()).toContain('排序不完整')
  })

  it('滞后版块数明示（各条目按自己的锚点日计）', () => {
    const wrapper = mountRail({ laggingCount: 3 })
    const note = wrapper.find('[data-testid="radar-lagging"]')
    expect(note.exists()).toBe(true)
    expect(note.text()).toContain('3 个版块数据滞后')
    expect(note.text()).toContain('锚点日')
  })

  it('点条目 emit openLane（进既有控制台 ?lane=）', async () => {
    const wrapper = mountRail({ laneItems: [lane('1974', '1204')] })
    await wrapper.find('[data-testid="radar-lane-1974-1204"]').trigger('click')
    expect(wrapper.emitted('openLane')![0]).toEqual([{ boardId: '1974', topicId: '1204' }])
  })

  it('旧响应（无 top_lanes 段）：回退版块级日报列表，不渲染泳道空态', () => {
    const wrapper = mountRail({ laneMode: false })
    expect(wrapper.find('[data-testid="radar-read-1974"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="radar-read-1974"]').text()).toContain('伊朗警告打击海湾海底电缆')
    expect(wrapper.find('[data-testid="radar-no-lane"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="radar-lane-1974-1204"]').exists()).toBe(false)
  })
})
