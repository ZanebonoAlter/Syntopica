/**
 * BoardConsoleShell 集成用例（8C G4b）：真实 Shell → Body → Stage → Matrix/Narrow → OtherReading 链。
 *
 * 只 mock API/复杂弹层（见 harness），主链是真实组件：报日列头可选性、当日其他话题组默认态/行、
 * 单一阅读槽（其他阅读隐藏旧演进并抑制泳道高亮）、受控重试链（force 索引 → force 当日报告）、
 * 生成完成后的 lane 编年重建、内部归档同实例返回、窄屏同源报日条与观察带节点。
 * 不覆盖真实像素/触摸/服务端生成时序（归 8C.7）。fixture 见 `boardConsoleShellOtherFixtures.ts`。
 *
 * 末段「8E review R-41」是本轮回归：报日索引缺该日时，**已验证的观察记录日**（黄点）仍原子
 * 保留日期与阅读目标，不被选中日普通回落搬走（桌面列头 / 窄屏日期条 / 连续第二次点击）。
 */
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { flushPromises } from '@vue/test-utils'
import {
  installShellTestHooks,
  mediaControl,
  getLaneDynamics,
  getBoardDailyReports,
  getDailyReportDetail,
  getBoardSectionTimeline,
  emptyResponse,
  TODAY,
} from './boardConsoleShellTestHarness'
import {
  EARLIER,
  detail,
  laneSection,
  mature,
  mockDetails,
  mountReady,
  otherSectionA,
  otherSectionB,
  reportIndex,
  section,
  thread,
} from './boardConsoleShellOtherFixtures'
import ObservationMatrix from './ObservationMatrix.vue'

installShellTestHooks()

describe('BoardConsoleShell 集成 — 报日轴与当日其他话题组（真实组件链）', () => {
  it('0 正式泳道：真实报日列头可点、其他组默认展开、真实分区可读，网格不被旧空态挡', async () => {
    getLaneDynamics.mockResolvedValue({ success: true, data: emptyResponse() })
    getBoardDailyReports.mockResolvedValue(reportIndex())
    mockDetails()

    const wrapper = await mountReady()

    // 报日索引接线：只有真实报日是 button，非报日仍是不可交互文本
    expect(wrapper.find(`[data-testid="matrix-dayhead-${TODAY}"]`).element.tagName).toBe('BUTTON')
    expect(wrapper.find(`[data-testid="matrix-dayhead-${EARLIER}"]`).element.tagName).toBe('BUTTON')
    expect(wrapper.find('[data-testid="matrix-dayhead-2026-10-22"]').element.tagName).toBe('DIV')

    // 0 正式泳道：组默认展开（不是大空态替换网格），真实行可见
    expect(wrapper.find('[data-testid="matrix-grid"]').exists()).toBe(true)
    const toggle = wrapper.find('[data-testid="other-topics-toggle"] button')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="other-topic-row-1200"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="other-topic-row-1200"]').text()).toContain('别的分区甲')
    expect(wrapper.find('[data-testid="other-topics-count"]').text()).toContain('3')

    // 0 泳道且未选其他：阅读槽给方向提示（诚实空态，不是无路可走）
    expect(wrapper.find('[data-testid="console-reading-empty"]').text()).toContain('当日其他话题')

    expect(wrapper.find('[data-testid="lane-evolution"]').exists()).toBe(false)

    // 列头选日 = 同一 selectedDate（与日格/组同源），不改 A 档/范围
    await wrapper.find(`[data-testid="matrix-dayhead-${EARLIER}"]`).trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(EARLIER)
  })

  it('成熟板块：其他组默认收起（计数可见）→ 展开点行 → 单一阅读槽显逐字内容，旧演进隐藏且泳道高亮抑制', async () => {
    mature()
    const wrapper = await mountReady()

    const toggle = wrapper.find('[data-testid="other-topics-toggle"] button')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="other-topics-count"]').text()).toContain('2')
    expect(wrapper.find('[data-testid="other-topic-row-1200"]').exists()).toBe(false)
    const detailCallsAfterMount = getDailyReportDetail.mock.calls.length

    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    expect(wrapper.find('[data-testid="other-topic-row-1200"]').exists()).toBe(true)
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()

    const reading = wrapper.find('[data-testid="other-topic-reading"]')
    expect(reading.exists()).toBe(true)
    expect(reading.text()).toContain('别的分区甲')
    expect(reading.text()).toContain('其他线索一')
    // 单一阅读槽：旧 lane 编年与演进都不在（不叠两个阅读/演进面）
    expect(wrapper.find('[data-testid="lane-chronicle"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lane-evolution"]').exists()).toBe(false)
    // 泳道高亮抑制（底层 lane 选中保留供切回）
    expect(wrapper.find('[data-testid="matrix-lane-10"]').classes()).not.toContain('is-selected')

    // 点行不额外请求：报告详情与壳层共享同一份缓存
    expect(getDailyReportDetail.mock.calls.length).toBe(detailCallsAfterMount)

    // 单条就地展开逐字摘要（复用既有 LaneChronicleThread）
    await wrapper.find('[data-testid="lane-chronicle-thread-toggle-11"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="other-topic-reading"]').text()).toContain('其他摘要一')
  })

  it('点同一底层已选泳道 / 普通格都显式清其他选择（不靠 laneKey 变化 watcher）', async () => {
    mature()
    const wrapper = await mountReady()

    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(true)

    // 点已选中的同一泳道名：仍必须清掉其他阅读槽
    await wrapper.find('[data-testid="matrix-lane-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lane-chronicle"]').exists()).toBe(true)

    // 再选其他行后点普通日格：同理清场并就地读该泳道
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()
    await wrapper.find(`[data-testid="matrix-cell-11-${TODAY}"]`).trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="lane-evolution"]').text()).toContain('主线 B')
  })

  it('日期表头黄点：跨日原子切选中日与首分区，保留阅读目标；取消选择清点', async () => {
    mature()
    getBoardSectionTimeline.mockResolvedValue({
      success: true,
      data: {
        sections: [
          { id: 2101, report_id: 60, period_date: TODAY, cluster_label: '观察乙', thread_count: 2, persistent_topic_id: 77 },
          { id: 2100, report_id: 59, period_date: EARLIER, cluster_label: '观察甲', thread_count: 1, persistent_topic_id: 77 },
          { id: 2102, report_id: 59, period_date: EARLIER, cluster_label: '同日另一区原题', thread_count: 3, persistent_topic_id: 77 },
          { id: 999, report_id: 59, period_date: EARLIER, cluster_label: '异ID不能串入', thread_count: 3, persistent_topic_id: 88 },
        ],
        relations: [],
      },
    })
    getDailyReportDetail.mockImplementation((id: number) => Promise.resolve(
      id === 59
        ? detail(59, EARLIER, [
            section({
              id: 2100, clusterIndex: 0, label: '观察甲', persistentTopicId: 77, statusAtReport: 'active',
              threads: [thread(2100, 31, '前日线索', '前日摘要')],
            }),
          ])
        : detail(60, TODAY, [laneSection(), otherSectionA(), otherSectionB()]),
    ))
    const wrapper = await mountReady()
    await flushPromises()

    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await flushPromises()
    // 缺 ID 不画点、不读观察；切至可靠 ID 才加载单份板块缓存。
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="date-association-marker"]')).toHaveLength(0)
    expect(getBoardSectionTimeline).not.toHaveBeenCalled()
    await wrapper.find('[data-testid="other-topic-row-1201"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="date-association-marker"]')).toHaveLength(2)
    const initialRequests = getBoardSectionTimeline.mock.calls.length
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="date-association-marker"]')).toHaveLength(0)
    await wrapper.find('[data-testid="other-topic-row-1201"]').trigger('click')
    await flushPromises()
    expect(getBoardSectionTimeline.mock.calls.length).toBe(initialRequests)

    expect(wrapper.find('[data-testid="other-observe-nodes"]').exists()).toBe(false)
    const header = wrapper.find(`[data-testid="matrix-dayhead-${EARLIER}"]`)
    expect(header.findAll('[data-testid="date-association-marker"]')).toHaveLength(1)
    expect(header.attributes('title')).toContain('观察甲；同日另一区原题')
    expect(header.attributes('title')).not.toContain('异ID不能串入')
    const requests = getBoardSectionTimeline.mock.calls.length
    await header.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(EARLIER)
    const reading = wrapper.find('[data-testid="other-topic-reading"]')
    expect(reading.text()).toContain('观察甲')
    expect(reading.text()).toContain('前日线索')
    expect(getBoardSectionTimeline.mock.calls.length).toBe(requests)
    // 同 ID 投影仍在；明确选普通泳道清其他选择及所有点。
    expect(wrapper.findAll('[data-testid="date-association-marker"]').length).toBeGreaterThan(0)
    await wrapper.find('[data-testid="matrix-lane-10"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="date-association-marker"]').length).toBe(0)
  })
})

describe('BoardConsoleShell 集成 — 8E review R-41：缺索引观察日黄点点击不被回落搬走', () => {
  const OTHER_REPORT_DAY = '2026-10-16'
  const SECOND_OBSERVED = '2026-10-20'

  /**
   * 报日索引含 OTHER_REPORT_DAY 与 TODAY、**缺**观察记录日（EARLIER / SECOND_OBSERVED）：
   * 模拟索引陈旧/不全，而观察带（另一条只读链）仍有该日真实记录 → 黄点可点。
   */
  function indexWithoutObservedDays(): void {
    getBoardDailyReports.mockResolvedValue({
      success: true,
      data: {
        reports: [
          { id: 58, semantic_board_id: 1974, period_date: OTHER_REPORT_DAY, title: '更早日报', created_at: '' },
          { id: 60, semantic_board_id: 1974, period_date: TODAY, title: '已有日报', created_at: '' },
        ],
      },
    })
  }

  /** OTHER_REPORT_DAY 的详情与其他话题行（1201 带稳定 ID 77 → 有黄点）。 */
  function detailsWithOtherRow(): void {
    getDailyReportDetail.mockImplementation((id: number) => Promise.resolve(
      id === 58
        ? detail(58, OTHER_REPORT_DAY, [laneSection(), otherSectionA(), otherSectionB()])
        : detail(60, TODAY, [laneSection(), otherSectionA(), otherSectionB()]),
    ))
  }

  /** 观察带：给定日期各有同 ID（77）真实记录 → 对应日期画黄点。 */
  function observedTimeline(dates: string[]): void {
    getBoardSectionTimeline.mockResolvedValue({
      success: true,
      data: {
        sections: dates.map((date, index) => ({
          id: 2100 + index, report_id: 59 + index, period_date: date,
          cluster_label: `观察${date}`, thread_count: 1, persistent_topic_id: 77,
        })),
        relations: [],
      },
    })
  }

  /** 从非默认回落日出发：先切到另一个真实报日（否则回落值与当前日相同、看不出搬走）。 */
  async function switchToOtherReportDay(wrapper: Awaited<ReturnType<typeof mountReady>>): Promise<void> {
    await wrapper.find(`[data-testid="matrix-dayhead-${OTHER_REPORT_DAY}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(OTHER_REPORT_DAY)
  }

  it('桌面：黄点点击原子保留日期与目标；索引缺该日也不回落到别的报日（诚实 no-report 态）', async () => {
    mature()
    indexWithoutObservedDays()
    detailsWithOtherRow()
    observedTimeline([EARLIER])
    const wrapper = await mountReady()
    await flushPromises()

    await switchToOtherReportDay(wrapper)
    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="other-topic-row-1201"]').trigger('click')
    await flushPromises()

    const head = wrapper.find(`[data-testid="matrix-dayhead-${EARLIER}"]`)
    expect(head.find('[data-testid="date-association-marker"]').exists()).toBe(true)

    await head.trigger('click')
    await flushPromises()
    await flushPromises()

    // 选中日不被普通回落搬走（索引没有该日、该泳道当日也无分区）
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(EARLIER)
    const reading = wrapper.find('[data-testid="other-topic-reading"]')
    expect(reading.exists()).toBe(true)
    expect(reading.find('time').attributes('datetime')).toBe(EARLIER)
    // 目标实际无日报：诚实 no-report 态，不借旧正文、不显示别日内容
    expect(wrapper.find('[data-testid="other-reading-no-report"]').exists()).toBe(true)
  })

  it('窄屏：同源日期条黄点点击同样原子保留日期与目标', async () => {
    mature()
    indexWithoutObservedDays()
    detailsWithOtherRow()
    observedTimeline([EARLIER])
    mediaControl.set(true)
    const wrapper = await mountReady()
    await flushPromises()

    await wrapper.find(`[data-testid="narrow-dayhead-${OTHER_REPORT_DAY}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    await wrapper.find('[data-testid="narrow-other-toggle"]').trigger('click')
    await wrapper.find('[data-testid="narrow-other-row-1201"]').trigger('click')
    await flushPromises()

    const head = wrapper.find(`[data-testid="narrow-dayhead-${EARLIER}"]`)
    expect(head.find('[data-testid="date-association-marker"]').exists()).toBe(true)

    await head.trigger('click')
    await flushPromises()
    await flushPromises()

    const reading = wrapper.find('[data-testid="other-topic-reading"]')
    expect(reading.exists()).toBe(true)
    expect(reading.find('time').attributes('datetime')).toBe(EARLIER)
    expect(wrapper.find('[data-testid="other-reading-no-report"]').exists()).toBe(true)
  })

  it('普通报日仍按旧语义清旧其他选择（保护只覆盖当前观察目标）', async () => {
    mature()
    indexWithoutObservedDays()
    detailsWithOtherRow()
    observedTimeline([EARLIER])
    const wrapper = await mountReady()
    await flushPromises()

    await switchToOtherReportDay(wrapper)
    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="other-topic-row-1201"]').trigger('click')
    await flushPromises()
    await wrapper.find(`[data-testid="matrix-dayhead-${EARLIER}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(EARLIER)

    // 点另一个真实报日：日期过去、旧其他选择被清（普通报日语义不变），保护同步失效
    await wrapper.find(`[data-testid="matrix-dayhead-${TODAY}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(TODAY)
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(false)
  })

  it('连续第二次黄点点击：日期与目标原子换到第二个观察日（不被排队 watcher 竞态清空）', async () => {
    mature()
    indexWithoutObservedDays()
    detailsWithOtherRow()
    observedTimeline([EARLIER, SECOND_OBSERVED])
    const wrapper = await mountReady()
    await flushPromises()

    await switchToOtherReportDay(wrapper)
    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="other-topic-row-1201"]').trigger('click')
    await flushPromises()

    await wrapper.find(`[data-testid="matrix-dayhead-${EARLIER}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(EARLIER)
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(true)

    // 第二次点另一个观察日：目标与日期一起换，不回到第一次也不被清空
    await wrapper.find(`[data-testid="matrix-dayhead-${SECOND_OBSERVED}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(SECOND_OBSERVED)
    const reading = wrapper.find('[data-testid="other-topic-reading"]')
    expect(reading.exists()).toBe(true)
    expect(reading.find('time').attributes('datetime')).toBe(SECOND_OBSERVED)
  })
})

describe('BoardConsoleShell 集成 — 受控重试与生成后刷新', () => {
  it('报日链失败：Stage 最小重试入口走 force 索引 → force 当日报告（不只刷 A）', async () => {
    mature()
    getDailyReportDetail.mockResolvedValueOnce({ success: false, error: '详情挂了' })
    const wrapper = await mountReady()

    const notice = wrapper.find('[data-testid="matrix-report-chain-notice"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('当日报告读取失败')

    const indexCallsBefore = getBoardDailyReports.mock.calls.length
    getDailyReportDetail.mockClear()
    await wrapper.find('[data-testid="matrix-report-chain-retry"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(wrapper.find('[data-testid="matrix-report-chain-notice"]').exists()).toBe(false)
    expect(getBoardDailyReports.mock.calls.length).toBe(indexCallsBefore + 1)
    expect(getDailyReportDetail).toHaveBeenCalledWith(60)
    // 顺序：force 索引先于 force 当日详情
    expect(getBoardDailyReports.mock.invocationCallOrder.at(-1)!)
      .toBeLessThan(getDailyReportDetail.mock.invocationCallOrder.at(-1)!)
  })

  it('G3 阅读槽重试：force 索引 → force 详情，缺分区给中性「报告已更新」，重试后真实内容回来', async () => {
    mature()
    const wrapper = await mountReady()

    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()

    // 报告重建后缺该分区：不借别日/别区，给中性提示 + 刷新
    mockDetails({ withOtherA: false })
    wrapper.findComponent({ name: 'DailyReportGenerateDialog' }).vm.$emit('completed')
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('[data-testid="other-reading-missing-section"]').exists()).toBe(true)

    getBoardDailyReports.mockClear()
    getDailyReportDetail.mockClear()
    mockDetails()
    await wrapper.find('[data-testid="other-reading-refresh"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(wrapper.find('[data-testid="other-topic-reading"]').text()).toContain('别的分区甲')
    expect(getBoardDailyReports.mock.calls.length).toBeGreaterThan(0)
    expect(getDailyReportDetail).toHaveBeenCalledWith(60)
    expect(getBoardDailyReports.mock.invocationCallOrder.at(-1)!)
      .toBeLessThan(getDailyReportDetail.mock.invocationCallOrder.at(-1)!)
  })

  it('生成完成：lane 编年基于刷新后共享缓存重建，无其他选择时观察零 GET', async () => {
    mature()
    const wrapper = await mountReady()

    await wrapper.find(`[data-testid="matrix-cell-10-${TODAY}"]`).trigger('click')
    await flushPromises()
    await flushPromises()
    expect(wrapper.find('[data-testid="lane-chronicle"]').text()).toContain('完整线索标题')

    mockDetails({ laneTitle: '刷新后线索标题' })
    getBoardSectionTimeline.mockClear()
    wrapper.findComponent({ name: 'DailyReportGenerateDialog' }).vm.$emit('completed')
    await flushPromises()
    await flushPromises()

    expect(wrapper.find('[data-testid="lane-chronicle"]').text()).toContain('刷新后线索标题')
    expect(getBoardSectionTimeline).not.toHaveBeenCalled()
  })

  it('内部归档 → 矩阵同实例返回：其他阅读槽/报日/滚动位置都恢复', async () => {
    mature()
    const wrapper = await mountReady()

    await wrapper.find('[data-testid="other-topics-toggle"] button').trigger('click')
    await wrapper.find('[data-testid="other-topic-row-1200"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(true)

    const stage = wrapper.find('.console-stage').element as HTMLElement
    Object.defineProperty(stage, 'clientHeight', { value: 100, configurable: true })
    Object.defineProperty(stage, 'scrollHeight', { value: 500, configurable: true })
    stage.scrollTop = 120

    await wrapper.find('[data-testid="console-view-archive"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="board-archive"]').exists()).toBe(true)

    await wrapper.find('[data-testid="console-view-matrix"]').trigger('click')
    await flushPromises()
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="other-topic-reading"]').text()).toContain('别的分区甲')
    expect(wrapper.findComponent(ObservationMatrix).props('selectedDate')).toBe(TODAY)
    expect(stage.scrollTop).toBe(120)
  })

  it('窄屏同源：报日条可点 + 其他组行 → 下方同一阅读槽', async () => {
    mature()
    mediaControl.set(true)
    const wrapper = await mountReady()

    expect(wrapper.find('[data-testid="board-matrix"]').exists()).toBe(false)
    expect(wrapper.find(`[data-testid="narrow-dayhead-${EARLIER}"]`).element.tagName).toBe('BUTTON')

    await wrapper.find(`[data-testid="narrow-dayhead-${EARLIER}"]`).trigger('click')
    await flushPromises()
    // 切到前一报日后，窄屏其他组按该日报真实分区列出（2100 是那天唯一未进泳道的分区）
    await wrapper.find('[data-testid="narrow-other-toggle"]').trigger('click')
    await wrapper.find('[data-testid="narrow-other-row-2100"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="other-topic-reading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="other-topic-reading"]').text()).toContain('观察甲')
  })

  it('窄屏复用同源日期条标点，focus 可见真实标题，点击仍原子保留目标', async () => {
    mature()
    mediaControl.set(true)
    getBoardSectionTimeline.mockResolvedValue({ success: true, data: { sections: [
      { id: 2100, report_id: 59, period_date: EARLIER, cluster_label: '观察甲', thread_count: 1, persistent_topic_id: 77 },
    ], relations: [] } })
    const wrapper = await mountReady()
    await wrapper.find('[data-testid="narrow-other-toggle"]').trigger('click')
    await wrapper.find('[data-testid="narrow-other-row-1201"]').trigger('click')
    await flushPromises()
    const head = wrapper.find(`[data-testid="narrow-dayhead-${EARLIER}"]`)
    expect(head.find('[data-testid="date-association-marker"]').exists()).toBe(true)
    expect(head.attributes('aria-label')).toContain('观察甲')
    await head.trigger('focus')
    expect(document.body.querySelector('[role="tooltip"]')?.textContent).toContain('观察甲')
    await head.trigger('blur')
    await head.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="other-topic-reading"]').text()).toContain('前日线索')
    expect(wrapper.find('[data-testid="other-observe-nodes"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
