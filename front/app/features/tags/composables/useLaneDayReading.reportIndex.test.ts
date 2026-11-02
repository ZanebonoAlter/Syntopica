/**
 * useLaneDayReading 报日索引子套件（8C G1，8C.4S 结构规范包从 useLaneDayReading.test.ts 拆出）。
 *
 *  - 停更 lookback：today 晚于 anchor 时列表 days 随差额扩大（死写 365 会漏池首报告）；
 *    覆盖不足扩大重取并整体替换，已有更大覆盖不重复取
 *  - RDC-1 索引与编年共享一份（列表端点仅 1 次；壳层与编年首读并发时在途合乘）
 *  - RDC-6 索引强制刷新与失败保留（新报日立即可见；失败不冒空集 + indexError；重试成功清错）
 *  - 父复核 R1（2026-10-29）：索引强刷的旧/新响应请求序（旧响应不回写、不永停 loading）
 *  - 父复核 R3（2026-10-29）：生成后 anchor 前移使索引需求覆盖收缩（369→365），force 仍取回新鲜列表
 *
 * fixture/harness 见 `laneDayReadingTestHarness.ts`；lane 读取主路仍在 `useLaneDayReading.test.ts`。
 */
import { describe, expect, it } from 'vitest'
import type { LaneDayReportIndexItem } from '~/api/normalizers/laneDayReading'
import {
  BOARD,
  TODAY,
  deferred,
  harness,
  indexItems,
  makeDay,
  okList,
  reportDetail,
} from './laneDayReadingTestHarness'

describe('停更 lookback — 列表 days 覆盖池首', () => {
  it('today 晚于 anchor：days = 365+差额，池最早日不误报无日报', async () => {
    const h = harness() // anchor 2026-10-24，today 2026-10-28 → 池首 2025-10-25 → 369
    h.list.mockResolvedValue(okList(indexItems([['2025-10-25', '900']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('900', '2025-10-25', [{ sectionId: '501', threads: [{ threadId: 't1', title: '一年前的线索' }] }]) })
    await h.store.ensureDayReading({ laneKey: '10', date: '2025-10-25' }, makeDay('2025-10-25'))
    expect(h.list).toHaveBeenCalledWith(BOARD, 369)
    expect(h.store.current.value?.status).toBe('ready')
    expect(h.store.current.value?.sections[0]?.threads[0]?.title).toBe('一年前的线索')
  })

  it('索引覆盖不足时扩大重取并整体替换；已有更大覆盖不重复取', async () => {
    const h = harness() // anchor 2026-10-24 / today 2026-10-28 → 369
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockImplementation((reportId: string) => Promise.resolve({ success: true, data: reportDetail(reportId, 'x', [{ sectionId: '501' }]) }))
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.list).toHaveBeenLastCalledWith(BOARD, 369)
    expect(h.store.indexDays()).toBe(369)
    // anchor 提前到 10-20 → 池首 2025-10-21 → days 373 > 369：扩大重取并替换
    h.setAnchor('2026-10-20')
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-20' }, makeDay('2026-10-20'))
    expect(h.list).toHaveBeenCalledTimes(2)
    expect(h.list).toHaveBeenLastCalledWith(BOARD, 373)
    expect(h.store.indexDays()).toBe(373)
    // anchor 后移（days 需求变小）：已有更大覆盖，不重复取同范围
    h.setAnchor('2026-10-26')
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.list).toHaveBeenCalledTimes(2)
    expect(h.store.indexDays()).toBe(373)
  })
})

describe('RDC-1 — 报日索引与编年共享一份（列表端点仅 1 次）', () => {
  it('ensureReportIndex 后编年首读：不再重复取列表；reportDates 为真实报日', async () => {
    const h = harness()
    h.list.mockResolvedValue(okList(indexItems([['2026-10-22', '1211'], ['2026-10-20', '1210']])))
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501' }]) })
    await h.store.ensureReportIndex()
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.list).toHaveBeenLastCalledWith(BOARD, 369)
    expect([...h.store.reportDates.value].sort()).toEqual(['2026-10-20', '2026-10-22'])
    await h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.store.indexError.value).toBeNull()
  })

  it('壳层启动与编年首读并发：索引在途合乘，仍只 1 次 GET', async () => {
    const h = harness()
    const gate = deferred<{ success: boolean, data?: LaneDayReportIndexItem[] }>()
    h.list.mockReturnValueOnce(gate.promise)
    h.detail.mockResolvedValue({ success: true, data: reportDetail('1211', '2026-10-22', [{ sectionId: '501' }]) })
    const boot = h.store.ensureReportIndex()
    const chronicle = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-22' }, makeDay('2026-10-22'))
    gate.resolve(okList(indexItems([['2026-10-22', '1211']])))
    await Promise.all([boot, chronicle])
    expect(h.list).toHaveBeenCalledTimes(1)
    expect(h.store.current.value?.status).toBe('ready')
    expect(h.store.reportDates.value.has('2026-10-22')).toBe(true)
  })
})

describe('RDC-6 — 索引强制刷新与失败保留', () => {
  it('ensureReportIndex(force)：重新拉列表一次，生成后的新报日立即可见', async () => {
    const h = harness()
    h.list.mockResolvedValueOnce(okList(indexItems([['2026-10-22', '1211']])))
    await h.store.ensureReportIndex()
    expect([...h.store.reportDates.value]).toEqual(['2026-10-22'])
    h.list.mockResolvedValueOnce(okList(indexItems([['2026-10-27', '1300'], ['2026-10-22', '1211']])))
    await h.store.ensureReportIndex({ force: true })
    expect(h.list).toHaveBeenCalledTimes(2)
    expect([...h.store.reportDates.value].sort()).toEqual(['2026-10-22', '2026-10-27'])
  })

  it('刷新失败：保留旧索引不冒空集 + indexError；重试成功后清错', async () => {
    const h = harness()
    h.list.mockResolvedValueOnce(okList(indexItems([['2026-10-22', '1211']])))
    await h.store.ensureReportIndex()
    h.list.mockResolvedValueOnce({ success: false, error: '列表挂了' })
    await h.store.ensureReportIndex({ force: true })
    expect(h.store.indexError.value).toBe('列表挂了')
    expect(h.store.reportDates.value.has('2026-10-22')).toBe(true)
    expect(h.store.reportDates.value.size).toBe(1)
    h.list.mockResolvedValueOnce(okList(indexItems([['2026-10-22', '1211'], ['2026-10-23', '1212']])))
    await h.store.ensureReportIndex({ force: true })
    expect(h.store.indexError.value).toBeNull()
    expect(h.store.reportDates.value.size).toBe(2)
  })

  it('索引从未成功过：reportDates 为空集 + indexError（不把暂未知当无日报）', async () => {
    const h = harness()
    h.list.mockResolvedValue({ success: false, error: '列表挂了' })
    await h.store.ensureReportIndex()
    expect(h.store.reportDates.value.size).toBe(0)
    expect(h.store.indexError.value).toBe('列表挂了')
  })
})

describe('R1 — 索引强刷的新旧响应', () => {
  type ListRes = { success: boolean, data?: LaneDayReportIndexItem[], error?: string }

  it('force 新请求先成功、旧普通请求晚到：报日/索引不回退', async () => {
    const h = harness()
    const oldGate = deferred<ListRes>()
    const newGate = deferred<ListRes>()
    h.list.mockReturnValueOnce(oldGate.promise).mockReturnValueOnce(newGate.promise)
    const oldRead = h.store.ensureReportIndex()
    const refresh = h.store.ensureReportIndex({ force: true })
    // force 确实启动了新请求，而不是合乘旧在途
    expect(h.list).toHaveBeenCalledTimes(2)
    newGate.resolve(okList(indexItems([['2026-10-27', '1300'], ['2026-10-22', '1211']])))
    await refresh
    expect([...h.store.reportDates.value].sort()).toEqual(['2026-10-22', '2026-10-27'])
    oldGate.resolve(okList(indexItems([['2026-10-22', '1211']])))
    await oldRead
    // 旧列表不得把生成后新增的报日缩回去
    expect([...h.store.reportDates.value].sort()).toEqual(['2026-10-22', '2026-10-27'])
    expect(h.store.indexDays()).toBe(369)
    expect(h.store.indexError.value).toBeNull()
    expect(h.store.indexLoading.value).toBe(false)
  })

  it('force 刷新成功后旧请求失败：不写旧 indexError、loading 归位、索引保持新值', async () => {
    const h = harness()
    const oldGate = deferred<ListRes>()
    const newGate = deferred<ListRes>()
    h.list.mockReturnValueOnce(oldGate.promise).mockReturnValueOnce(newGate.promise)
    const oldRead = h.store.ensureReportIndex()
    const refresh = h.store.ensureReportIndex({ force: true })
    newGate.resolve(okList(indexItems([['2026-10-27', '1300'], ['2026-10-22', '1211']])))
    await refresh
    oldGate.resolve({ success: false, error: '旧请求挂了' })
    await oldRead
    expect(h.store.indexError.value).toBeNull()
    expect([...h.store.reportDates.value].sort()).toEqual(['2026-10-22', '2026-10-27'])
    expect(h.store.indexLoading.value).toBe(false)
  })

  it('旧/新消费者都终结：lane 用最新索引不永 loading，报告强刷后读到新报日', async () => {
    const h = harness()
    const oldGate = deferred<ListRes>()
    const newGate = deferred<ListRes>()
    h.list.mockReturnValueOnce(oldGate.promise).mockReturnValueOnce(newGate.promise)
    h.detail.mockResolvedValue({
      success: true,
      data: reportDetail('1300', '2026-10-27', [{ sectionId: '501', threads: [{ threadId: 't1', title: '新报日线索' }] }]),
    })
    // lane 消费者先发索引（旧列表尚不含 10-27）
    const laneRead = h.store.ensureDayReading({ laneKey: '10', date: '2026-10-27' }, makeDay('2026-10-27'))
    // 生成完成强刷新列表
    const refresh = h.store.ensureReportIndex({ force: true })
    newGate.resolve(okList(indexItems([['2026-10-27', '1300'], ['2026-10-22', '1211']])))
    await refresh
    expect(h.store.reportDates.value.has('2026-10-27')).toBe(true)
    await h.store.ensureReportDay('2026-10-27')
    expect(h.store.reportDay.value?.status).toBe('ready')
    expect(h.store.reportDay.value?.reportId).toBe('1300')
    // 旧列表晚到：lane 读取必须终结且按最新索引命中（不得冒旧列表的 no-report/永 loading）
    oldGate.resolve(okList(indexItems([['2026-10-22', '1211']])))
    await laneRead
    expect(h.store.loading.value).toBe(false)
    expect(h.store.current.value?.status).toBe('ready')
    expect(h.store.current.value?.reportId).toBe('1300')
    expect(h.list).toHaveBeenCalledTimes(2)
    // 索引仍是最新覆盖
    expect([...h.store.reportDates.value].sort()).toEqual(['2026-10-22', '2026-10-27'])
  })
})

// ── 父复核 R2：切板块/卸载后相关文章晚到（缓存与调用端各一道保护，2026-10-29） ───────

describe('R3 — 索引覆盖收缩时的强刷', () => {
  type ListRes = { success: boolean, data?: LaneDayReportIndexItem[], error?: string }

  it('anchor 推进后 force：有效 days 不缩到小于当前成功覆盖，新报日立即可见', async () => {
    const h = harness() // anchor 2026-10-24 / today 2026-10-28 → 池首 2025-10-25 → 369
    h.list.mockResolvedValueOnce(okList(indexItems([['2026-10-22', '1211']])))
    await h.store.ensureReportIndex()
    expect(h.list).toHaveBeenLastCalledWith(BOARD, 369)
    expect(h.store.indexDays()).toBe(369)
    // 生成完成：板块 anchor 推进到今天，reportIndexDays 需求从 369 降到 365
    h.setAnchor(TODAY)
    h.list.mockResolvedValueOnce(okList(indexItems([['2026-10-28', '1400'], ['2026-10-22', '1211']])))
    await h.store.ensureReportIndex({ force: true })
    // 新报日必须立即出现（旧实现：365 请求回来被 369 覆盖比较挡掉 → 返回旧数据、10-28 不可见）
    expect([...h.store.reportDates.value].sort()).toEqual(['2026-10-22', '2026-10-28'])
    // 覆盖不得收缩：仍按已成功的 369 取完整新鲜列表
    expect(h.list).toHaveBeenLastCalledWith(BOARD, 369)
    expect(h.store.indexDays()).toBe(369)
    expect(h.store.indexError.value).toBeNull()
    expect(h.store.indexLoading.value).toBe(false)
  })

  it('无成功索引但已有更大在途请求：force 不缩其覆盖，新鲜列表仍可写入', async () => {
    const h = harness() // anchor 2026-10-24 → 369
    const wideGate = deferred<ListRes>()
    h.list.mockReturnValueOnce(wideGate.promise)
    const wideRead = h.store.ensureReportIndex() // 369 在途（尚未成功）
    h.setAnchor(TODAY) // 需求降到 365
    h.list.mockResolvedValueOnce(okList(indexItems([['2026-10-28', '1400'], ['2025-10-25', '900']])))
    await h.store.ensureReportIndex({ force: true })
    // 新鲜列表完整写入（含池首 2025-10-25）
    expect([...h.store.reportDates.value].sort()).toEqual(['2025-10-25', '2026-10-28'])
    // 有效覆盖取在途的较大者：池首不因本次 force 丢出索引范围
    expect(h.list).toHaveBeenLastCalledWith(BOARD, 369)
    expect(h.store.indexDays()).toBe(369)
    // 旧在途晚到不回写，消费者终态结束
    wideGate.resolve(okList(indexItems([['2026-10-22', '1211']])))
    await wideRead
    expect([...h.store.reportDates.value].sort()).toEqual(['2025-10-25', '2026-10-28'])
    expect(h.store.indexLoading.value).toBe(false)
  })
})
