/**
 * otherTopicSelection 测试（8C G4a）：首份成功 A 默认展开只一次/板块 + 组视图由 G1
 * buildOtherTopics 派生。选择/清场故事在 `otherTopicSelection.select.test.ts`（同包）。
 */
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import {
  DATE,
  EMPTY_WINDOW,
  MATURE_WINDOW,
  makeSelection,
  readyDay,
  section,
  windowData,
} from './otherTopicSelectionTestHarness'

describe('otherTopicSelection — 首份成功 A 的默认展开（只一次/板块）', () => {
  it('A 未到达不初始化；成功后 0 正式/休眠行 → 展开；成熟 → 收起', async () => {
    const h = makeSelection()
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(false)

    h.setSource(EMPTY_WINDOW)
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(true)

    const mature = makeSelection()
    mature.setSource(MATURE_WINDOW)
    await nextTick()
    expect(mature.selection.othersOpen.value).toBe(false)
    h.scope.stop()
    mature.scope.stop()
  })

  it('仅首份成功 A 决定默认态：之后切档/刷新（含 0↔成熟）不重置', async () => {
    const h = makeSelection()
    h.setSource(EMPTY_WINDOW)
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(true)

    h.setSource(MATURE_WINDOW)
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(true)

    h.setSource(null)
    await nextTick()
    h.setSource(MATURE_WINDOW)
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(true)
    h.scope.stop()
  })

  it('用户 toggle 后切档/裁范围不覆盖手动展开态（ready 才可 toggle）', async () => {
    const h = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: DATE,
      reportDates: [DATE],
      reportDay: readyDay(DATE, [section('603', 0, null)]),
    })
    await nextTick()
    expect(h.selection.otherTopics.value.state).toBe('ready')
    expect(h.selection.othersOpen.value).toBe(false)

    h.selection.toggleOthers()
    expect(h.selection.othersOpen.value).toBe(true)

    h.setSource(windowData([{ topicId: '10', days: [{ date: DATE, sectionIds: ['601'] }] }]))
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(true)
    h.scope.stop()
  })

  it('非 ready 时 toggle 不制造假展开态', async () => {
    const h = makeSelection({ source: MATURE_WINDOW })
    await nextTick()
    expect(h.selection.otherTopics.value.state).toBe('no-date')
    h.selection.toggleOthers()
    expect(h.selection.othersOpen.value).toBe(false)
    h.scope.stop()
  })

  it('切板块复位选择/展开；新板块首份成功 A 再初始化', async () => {
    const h = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: DATE,
      reportDates: [DATE],
      reportDay: readyDay(DATE, [section('603', 0, null)]),
    })
    await nextTick()
    h.selection.selectOther({ sectionId: '603', date: DATE })
    h.selection.toggleOthers()
    expect(h.selection.selectedOther.value).not.toBeNull()

    h.boardId.value = 2000
    await nextTick()
    expect(h.selection.selectedOther.value).toBeNull()
    expect(h.selection.othersOpen.value).toBe(false)

    h.setSource(EMPTY_WINDOW)
    await nextTick()
    expect(h.selection.othersOpen.value).toBe(true)
    h.scope.stop()
  })
})

describe('otherTopicSelection — 组视图由 G1 buildOtherTopics 派生', () => {
  it('按 sectionId 与 persistentTopicId 并集排除 A 泳道分区；行序 clusterIndex；计数 threads.length', async () => {
    const h = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: DATE,
      reportDates: [DATE],
      reportDay: readyDay(DATE, [
        section('603', 2, null, 0),
        section('601', 0, '10', 3), // sectionId 与 topicId 双证据排除
        section('604', 1, '10', 2), // 同 A topicId 但不同 section：仍排除
        section('602', 3, '77', 4), // 不在 A 名单：保留
      ]),
    })
    await nextTick()
    const view = h.selection.otherTopics.value
    expect(view.state).toBe('ready')
    expect(view.rows.map(row => row.sectionId)).toEqual(['603', '602'])
    expect(view.rows.map(row => row.threadCount)).toEqual([0, 4])
    expect(view.count).toBe(2)
    h.scope.stop()
  })

  it('所选日无日报 → no-report（不是 0 行）；索引未取到 → index-error 可重试', async () => {
    const noReport = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: DATE,
      reportDates: [DATE],
      reportDay: { date: DATE, status: 'no-report', error: null, errorSource: null, reportId: null, sections: [] },
    })
    await nextTick()
    expect(noReport.selection.otherTopics.value.state).toBe('no-report')

    const indexError = makeSelection({
      source: MATURE_WINDOW,
      selectedDate: DATE,
      reportDates: [DATE],
      reportDay: { date: DATE, status: 'error', error: '列表挂了', errorSource: 'index', reportId: null, sections: [] },
    })
    await nextTick()
    expect(indexError.selection.otherTopics.value.state).toBe('index-error')
    expect(indexError.selection.otherTopics.value.retryable).toBe(true)
    noReport.scope.stop()
    indexError.scope.stop()
  })
})
