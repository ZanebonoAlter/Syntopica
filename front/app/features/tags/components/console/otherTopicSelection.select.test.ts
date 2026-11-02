/**
 * otherTopicSelection 选择/清场测试（8C G4a）：选行/列头/观察节点只写状态，
 * 日期/泳道/板块按既有规则清场，详情缺原 section 不清选择（阅读槽显示「报告已更新」）。
 * 默认展开/组视图故事在 `otherTopicSelection.test.ts`（同包）。
 */
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { DATE, EMPTY_WINDOW, MATURE_WINDOW, makeSelection, readyDay, section } from './otherTopicSelectionTestHarness'

function readyHarness() {
  return makeSelection({
    source: MATURE_WINDOW,
    selectedDate: DATE,
    selectedLaneKey: '10',
    reportDates: [DATE],
    reportDay: readyDay(DATE, [section('603', 0, '77', 2), section('605', 1, null, 0)]),
  })
}

describe('otherTopicSelection — 选择/清场', () => {
  it('选行：写入当日局部身份 + 真实 reportId/topicId（不存标题），不改选中日；重复选幂等', async () => {
    const h = readyHarness()
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })
    expect(h.selection.selectedOther.value).toEqual({
      sectionId: '603', date: DATE, reportId: '900', topicId: '77',
    })
    expect(h.selection.selectedOtherSectionId.value).toBe('603')
    expect(h.selection.selectedOtherTopicId.value).toBe('77')
    expect(h.selection.selectedOtherReadingKey.value).toBe('other:603')
    expect(h.selectedDate.value).toBe(DATE)

    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })
    expect(h.selection.selectedOther.value).toEqual({
      sectionId: '603', date: DATE, reportId: '900', topicId: '77',
    })
    expect(h.selectedDate.value).toBe(DATE)
    h.scope.stop()
  })

  it('选行 guard：日期不一致 / 缺 sectionId / 报告未 ready / 详情无该 section 一律忽略', async () => {
    const h = readyHarness()
    await nextTick()
    for (const payload of [
      { sectionId: '603', date: '2026-10-23' },
      { sectionId: '', date: DATE },
      { sectionId: 'not-a-number', date: DATE },
      { sectionId: '599', date: DATE },
    ]) {
      h.selection.selectOther(payload)
      expect(h.selection.selectedOther.value).toBeNull()
    }

    h.reportDay.value = { ...readyDay(DATE, []), status: 'loading' }
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE })
    expect(h.selection.selectedOther.value).toBeNull()
    h.scope.stop()
  })

  it('选行 guard：只接受 G1 其他组视图成员（正式/休眠泳道分区详情里存在也不得进入）', async () => {
    const h = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: DATE,
      selectedLaneKey: '10',
      reportDates: [DATE],
      reportDay: readyDay(DATE, [
        section('601', 0, '10', 3),
        section('603', 1, '77', 2),
      ]),
    })
    await nextTick()
    expect(h.selection.otherTopics.value.state).toBe('ready')
    expect(h.selection.otherTopics.value.rows.map(row => row.sectionId)).toEqual(['603'])

    // 正式泳道 10 当日可读分区：详情里真实存在，但不是其他话题成员
    h.selection.selectOther({ sectionId: '601', date: DATE, topicId: '10' })
    expect(h.selection.selectedOther.value).toBeNull()

    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })
    expect(h.selection.selectedOther.value).toEqual({
      sectionId: '603', date: DATE, reportId: '900', topicId: '77',
    })
    h.scope.stop()
  })

  it('选行 guard：休眠泳道分区 / 组 ready 但 0 行时一律不进入其他选择', async () => {
    const otherDate = '2026-10-23'
    const dormant = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: otherDate,
      selectedLaneKey: '10',
      reportDates: [otherDate],
      reportDay: readyDay(otherDate, [
        section('602', 0, '11', 1),
        section('604', 1, '88', 1),
      ], '898'),
    })
    await nextTick()
    expect(dormant.selection.otherTopics.value.rows.map(row => row.sectionId)).toEqual(['604'])
    dormant.selection.selectOther({ sectionId: '602', date: otherDate, topicId: '11' })
    expect(dormant.selection.selectedOther.value).toBeNull()
    dormant.selection.selectOther({ sectionId: '604', date: otherDate, topicId: '88' })
    expect(dormant.selection.selectedOther.value?.sectionId).toBe('604')
    dormant.scope.stop()

    const none = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: DATE,
      selectedLaneKey: '10',
      reportDates: [DATE],
      reportDay: readyDay(DATE, [section('601', 0, '10', 3)]),
    })
    await nextTick()
    expect(none.selection.otherTopics.value).toMatchObject({ state: 'ready', count: 0, rows: [] })
    none.selection.selectOther({ sectionId: '601', date: DATE, topicId: '10' })
    expect(none.selection.selectedOther.value).toBeNull()
    none.scope.stop()
  })

  it('列头选日：只对可见轴内真实报日生效；换日清旧选择，同日保持', async () => {
    const h = readyHarness()
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })

    h.selection.selectDayHeader('2026-10-23') // 非报日
    expect(h.selectedDate.value).toBe(DATE)
    h.selection.selectDayHeader('2026-09-30') // 轴外
    expect(h.selectedDate.value).toBe(DATE)

    h.selection.selectDayHeader(DATE) // 同一报日
    await nextTick()
    expect(h.selection.selectedOther.value?.sectionId).toBe('603')

    h.reportDates.value = new Set([DATE, '2026-10-23'])
    h.selection.selectDayHeader('2026-10-23')
    expect(h.selectedDate.value).toBe('2026-10-23')
    await nextTick()
    expect(h.selection.selectedOther.value).toBeNull()
    h.scope.stop()
  })

  it('观察节点：原子切同一 selectedDate + 稳定目标；排队日期 watcher 不清刚写入的新选择', async () => {
    const h = readyHarness()
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })

    h.selection.selectObservedDay({
      date: '2026-10-23', sectionId: '502', reportId: '898', topicId: '77',
    })
    expect(h.selectedDate.value).toBe('2026-10-23')
    expect(h.selection.selectedOther.value).toEqual({
      sectionId: '502', date: '2026-10-23', reportId: '898', topicId: '77',
    })

    // 排队日期 watcher 跑完后新选择仍保留（日期与当前日一致）
    await nextTick()
    expect(h.selection.selectedOther.value?.sectionId).toBe('502')
    h.scope.stop()
  })

  it('观察节点 guard：轴外日期 / 非法 ID 忽略（不写半个选择）', async () => {
    const h = readyHarness()
    await nextTick()
    h.selection.selectObservedDay({ date: '2026-09-30', sectionId: '502', reportId: '898', topicId: '77' })
    expect(h.selectedDate.value).toBe(DATE)
    expect(h.selection.selectedOther.value).toBeNull()
    h.selection.selectObservedDay({ date: '2026-10-23', sectionId: 'x', reportId: '898', topicId: '77' })
    expect(h.selection.selectedOther.value).toBeNull()
    h.scope.stop()
  })

  it('外部换日清选择；同日报日刷新/详情缺原 section 都不清选择', async () => {
    const h = readyHarness()
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })

    // 同日报日刷新（详情缺原 section）：选择保留，阅读槽显示「报告已更新」
    h.reportDay.value = readyDay(DATE, [section('605', 0, null, 0)])
    await nextTick()
    expect(h.selection.selectedOtherSectionId.value).toBe('603')

    h.selectedDate.value = '2026-10-23'
    await nextTick()
    expect(h.selection.selectedOther.value).toBeNull()
    h.scope.stop()
  })

  it('切泳道同步清选择（返回恢复先 selectLane 再写选择也不会被排队 watcher 清掉）', async () => {
    const h = readyHarness()
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })

    h.selectedLaneKey.value = '11'
    expect(h.selection.selectedOther.value).toBeNull()

    // 模拟返回恢复顺序：先换泳道（sync 清场），再写恢复的 other —— 保留
    h.selection.selectedOther.value = { sectionId: '603', date: DATE, reportId: '900', topicId: '77' }
    await nextTick()
    expect(h.selection.selectedOther.value?.sectionId).toBe('603')
    h.scope.stop()
  })

  it('resetForBoard 复位选择/展开/初始化标记', async () => {
    const h = readyHarness()
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE, topicId: '77' })
    h.selection.resetForBoard()
    expect(h.selection.selectedOther.value).toBeNull()
    expect(h.selection.othersOpen.value).toBe(false)
    h.setSource(null)
    await nextTick()
    h.setSource(EMPTY_WINDOW)
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(true)
    h.scope.stop()
  })
})

describe('otherTopicSelection — 已验证记录日生命周期（R41）', () => {
  const OBSERVED = '2026-10-23'

  function observedHarness() {
    return makeSelection({
      source: MATURE_WINDOW,
      selectedDate: OBSERVED,
      selectedLaneKey: '10',
      reportDates: [OBSERVED],
      reportDay: readyDay(OBSERVED, [section('603', 0, '77', 2)]),
    })
  }

  it('黄点选择写入已验证记录日；普通选行/切泳道/换日/切板块立即失效', async () => {
    const h = observedHarness()
    await nextTick()
    expect(h.selection.observedTargetDay.value).toBeNull()

    // 黄点（观察节点）选择：该日是已验证记录日
    h.selection.selectObservedDay({ date: OBSERVED, sectionId: '502', reportId: '898', topicId: '77' })
    expect(h.selection.observedTargetDay.value).toBe(OBSERVED)

    // 普通选行：不是观察目标 → 释放保护
    h.selection.selectOther({ sectionId: '603', date: OBSERVED, topicId: '77' })
    expect(h.selection.selectedOther.value?.sectionId).toBe('603')
    expect(h.selection.observedTargetDay.value).toBeNull()

    // 再次黄点选择 → 切泳道：同步失效
    h.selection.selectObservedDay({ date: OBSERVED, sectionId: '502', reportId: '898', topicId: '77' })
    expect(h.selection.observedTargetDay.value).toBe(OBSERVED)
    h.selectedLaneKey.value = '11'
    expect(h.selection.observedTargetDay.value).toBeNull()

    // 黄点选择 → 外部换日：选择被清 → 保护同步失效
    h.selection.selectObservedDay({ date: OBSERVED, sectionId: '502', reportId: '898', topicId: '77' })
    expect(h.selection.observedTargetDay.value).toBe(OBSERVED)
    h.selectedDate.value = '2026-10-22'
    await nextTick()
    expect(h.selection.selectedOther.value).toBeNull()
    expect(h.selection.observedTargetDay.value).toBeNull()

    // 黄点选择 → 切板块：失效
    h.selection.selectObservedDay({ date: OBSERVED, sectionId: '502', reportId: '898', topicId: '77' })
    h.boardId.value = 1975
    await nextTick()
    expect(h.selection.observedTargetDay.value).toBeNull()
    h.scope.stop()
  })
})
