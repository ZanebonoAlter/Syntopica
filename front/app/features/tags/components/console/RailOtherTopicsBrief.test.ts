/**
 * RailOtherTopicsBrief 测试（add-other-topics-to-context-rail）。
 *
 * 本组件纯受控：行/计数/状态词全部来自壳层的 `OtherTopicsView`（唯一实现点 `dayOtherTopics.ts`），
 * 组件自己不重算排除口径与计数。
 *  - 四态渲染：success（有分区）/ empty（0 分区）/ loading（读取中）/ error（读取失败）
 *  - 计数徽标只在 ready 给数字（0 是真实结论），其余态不冒 0；单位后缀「条线索」
 *    （与泳道层的「条」分家——两套量纲不得并排成同一列）
 *  - 行内容：分区标题（两行截断）/ 该分区当日线索数 / 归属状态词（用模型给的文案，不翻译不加前缀）
 *  - 点行发 `selectOther` 事件（`sectionId` + `topicId`）；失败态给受控重试入口
 */
import { readFileSync } from 'node:fs'
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RailOtherTopicsBrief from './RailOtherTopicsBrief.vue'
import {
  OTHER_TOPICS_EMPTY_TEXT,
  OTHER_TOPICS_ERROR_TEXT,
  OTHER_TOPICS_INDEX_ERROR_TEXT,
  OTHER_TOPICS_INDEX_LOADING_TEXT,
  OTHER_TOPICS_LOADING_TEXT,
  OTHER_TOPICS_NO_DATE_TEXT,
  OTHER_TOPICS_NO_REPORT_TEXT,
  OTHER_TOPICS_UNAVAILABLE_NOTE,
  type OtherTopicsView,
} from './dayOtherTopics'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

function readyView(overrides: Partial<OtherTopicsView> = {}): OtherTopicsView {
  return {
    state: 'ready',
    rows: [
      {
        sectionId: '701',
        label: '乌干达因过境旅客确诊加德满都口岸加强防疫',
        clusterIndex: 0,
        threadCount: 3,
        compositeCount: 0,
        persistentTopicId: null,
        statusKind: 'unattributed',
        statusText: '当次未归属',
      },
      {
        sectionId: '702',
        label: '第二条其他分区（当时待观察）',
        clusterIndex: 1,
        threadCount: 1,
        compositeCount: 2,
        persistentTopicId: '88',
        statusKind: 'candidate',
        statusText: '当时待观察',
      },
    ],
    clueCount: 4,
    statusText: null,
    note: null,
    retryable: false,
    ...overrides,
  }
}

function passiveView(
  state: OtherTopicsView['state'],
  statusText: string,
  extra: Partial<OtherTopicsView> = {},
): OtherTopicsView {
  return { state, rows: [], clueCount: null, statusText, note: null, retryable: false, ...extra }
}

function mountBrief(view: OtherTopicsView) {
  return mount(RailOtherTopicsBrief, { props: { view } })
}

describe('RailOtherTopicsBrief — 四态渲染', () => {
  it('success：逐行给出分区标题 / 该分区线索数 / 归属状态词', () => {
    const wrapper = mountBrief(readyView())
    const rows = wrapper.findAll('[data-testid^="rail-other-topic-item-"]')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('乌干达因过境旅客确诊加德满都口岸加强防疫')
    expect(rows[0]!.text()).toContain('3 条线索')
    expect(rows[0]!.text()).toContain('当次未归属')
    expect(rows[1]!.text()).toContain('第二条其他分区（当时待观察）')
    expect(rows[1]!.text()).toContain('1 条线索')
    expect(rows[1]!.text()).toContain('当时待观察')
  })

  it('计数徽标只在 ready 给数字（单位「条线索」）；行数不冒充线索数', () => {
    const wrapper = mountBrief(readyView())
    expect(wrapper.find('[data-testid="rail-other-topics-count"]').text()).toBe('4 条线索')
    expect(wrapper.find('[data-testid="rail-other-topics"]').text()).not.toContain('2 条线索')
  })

  it('empty（0 分区）：诚实空态文案（复用常量），徽标给真实 0', () => {
    const wrapper = mountBrief(readyView({ rows: [], clueCount: 0, statusText: OTHER_TOPICS_EMPTY_TEXT }))
    expect(wrapper.find('[data-testid="rail-other-topics-hint"]').text()).toBe(OTHER_TOPICS_EMPTY_TEXT)
    expect(wrapper.find('[data-testid="rail-other-topics-count"]').text()).toBe('0 条线索')
    expect(wrapper.findAll('[data-testid="rail-other-topic-row"]')).toHaveLength(0)
  })

  it('loading / 无日报 / 未选日：只给既有状态文案，不冒计数', () => {
    const loading = mountBrief(passiveView('loading', OTHER_TOPICS_LOADING_TEXT))
    expect(loading.find('[data-testid="rail-other-topics-hint"]').text()).toBe(OTHER_TOPICS_LOADING_TEXT)
    expect(loading.find('[data-testid="rail-other-topics-count"]').exists()).toBe(false)

    const indexLoading = mountBrief(passiveView('index-loading', OTHER_TOPICS_INDEX_LOADING_TEXT))
    expect(indexLoading.find('[data-testid="rail-other-topics-hint"]').text()).toBe(OTHER_TOPICS_INDEX_LOADING_TEXT)

    const noReport = mountBrief(passiveView('no-report', OTHER_TOPICS_NO_REPORT_TEXT))
    expect(noReport.find('[data-testid="rail-other-topics-hint"]').text()).toBe(OTHER_TOPICS_NO_REPORT_TEXT)

    const noDate = mountBrief(passiveView('no-date', OTHER_TOPICS_NO_DATE_TEXT))
    expect(noDate.find('[data-testid="rail-other-topics-hint"]').text()).toBe(OTHER_TOPICS_NO_DATE_TEXT)
  })

  it('error：既有失败文案 + 技术附注 + 重试入口（重试事件由宿主处理）', async () => {
    const wrapper = mountBrief(passiveView('error', OTHER_TOPICS_ERROR_TEXT, { note: 'HTTP 500', retryable: true }))
    expect(wrapper.find('[data-testid="rail-other-topics-error"]').text()).toContain(OTHER_TOPICS_ERROR_TEXT)
    expect(wrapper.find('[data-testid="rail-other-topics-note"]').text()).toBe('HTTP 500')
    await wrapper.find('[data-testid="rail-other-topics-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it('index-error：同样给失败文案 + 重试入口', async () => {
    const wrapper = mountBrief(passiveView('index-error', OTHER_TOPICS_INDEX_ERROR_TEXT, { retryable: true }))
    expect(wrapper.find('[data-testid="rail-other-topics-error"]').text()).toContain(OTHER_TOPICS_INDEX_ERROR_TEXT)
    await wrapper.find('[data-testid="rail-other-topics-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it('degraded 附注（部分泳道当日内容未取到）照实显示', () => {
    const wrapper = mountBrief(readyView({ note: OTHER_TOPICS_UNAVAILABLE_NOTE }))
    expect(wrapper.find('[data-testid="rail-other-topics-note"]').text()).toBe(OTHER_TOPICS_UNAVAILABLE_NOTE)
  })
})

describe('RailOtherTopicsBrief — 点行发事件', () => {
  it('点某行 → 发 selectOther（带 sectionId 与 topicId；日期由宿主补）', async () => {
    const wrapper = mountBrief(readyView())
    await wrapper.findAll('[data-testid="rail-other-topic-row"]')[0]!.trigger('click')
    expect(wrapper.emitted('selectOther')).toEqual([[{ sectionId: '701', topicId: null }]])

    await wrapper.findAll('[data-testid="rail-other-topic-row"]')[1]!.trigger('click')
    expect(wrapper.emitted('selectOther')).toEqual([
      [{ sectionId: '701', topicId: null }],
      [{ sectionId: '702', topicId: '88' }],
    ])
  })
})

describe('RailOtherTopicsBrief — 视觉口径机械锚', () => {
  it('分区标题两行截断；单位后缀是「条线索」（不是泳道层的「条」）', () => {
    const source = readFileSync(`${import.meta.dirname}/RailOtherTopicsBrief.vue`, 'utf8')
    expect(source).toMatch(/\.rob-row-label[^{}]*\{[^}]*-webkit-line-clamp:\s*2/)
    expect(source).toContain('{{ row.threadCount }} 条线索')
    expect(source).toContain('${props.view.clueCount} 条线索')
  })
})
