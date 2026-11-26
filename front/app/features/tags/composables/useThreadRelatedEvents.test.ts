import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ThreadRelationsResponse } from '~/api/dailyReports'

/**
 * 线索相关推荐的取数缓存（replace-lane-lifeline-with-topic-events 3.2/3.3）。
 *
 * 判定口径：
 * - 缓存键是「报告 × 窗口」——端点一次返回整份日报所有线索的推荐，所以重复展开
 *   话题卡片不该再发请求；
 * - 同键并发只发一次（切档/多泳道同时展开时的结构性保证）；
 * - 失败可重试（推荐失败不阻断阅读，但下一次显式动作要能救回来）；
 * - 7/30 两档互不污染（切回已成功的档位命中缓存）。
 */

const api = vi.hoisted(() => ({ getThreadRelations: vi.fn() }))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ getThreadRelations: api.getThreadRelations }),
}))

import { useThreadRelatedEvents } from './useThreadRelatedEvents'

function response(reportId: number, days: number, threadIds: number[] = []): ThreadRelationsResponse {
  return {
    report_id: reportId,
    days,
    window_start: '2026-11-11',
    window_end: '2026-11-16',
    status: threadIds.length ? 'ready' : 'empty',
    threads: Object.fromEntries(threadIds.map(id => [String(id), []])),
    total: 0,
    skipped_no_embedding: 0,
  }
}

describe('useThreadRelatedEvents — 缓存与档位隔离', () => {
  beforeEach(() => {
    api.getThreadRelations.mockReset()
    useThreadRelatedEvents().resetLocal()
  })

  it('未请求过时条目为 idle，成功后按 (reportId, days) 命中缓存不再重取', async () => {
    api.getThreadRelations.mockResolvedValue({ success: true, data: response(70, 7, [1]) })
    const events = useThreadRelatedEvents()

    expect(events.entry(70, 7).status).toBe('idle')
    await events.ensure(70, 7)
    expect(events.entry(70, 7).status).toBe('success')
    expect(events.entry(70, 7).data?.threads).toHaveProperty('1')

    // 重复展开话题卡片：命中缓存，不发第二次请求
    await events.ensure(70, 7)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(1)
    expect(api.getThreadRelations).toHaveBeenCalledWith(70, { days: 7 })
  })

  it('同键并发只发一次请求（切档/多泳道同时展开）', async () => {
    let settle: ((value: unknown) => void) | undefined
    api.getThreadRelations.mockImplementation(() => new Promise((resolve) => { settle = resolve }))
    const events = useThreadRelatedEvents()

    const first = events.ensure(70, 7)
    const second = events.ensure(70, 7)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(1)
    expect(events.entry(70, 7).status).toBe('loading')

    settle?.({ success: true, data: response(70, 7) })
    await Promise.all([first, second])
    expect(events.entry(70, 7).status).toBe('success')
    expect(api.getThreadRelations).toHaveBeenCalledTimes(1)
  })

  it('失败留在错误态，再次 ensure 自动重发并恢复成功', async () => {
    api.getThreadRelations
      .mockResolvedValueOnce({ success: false, error: 'boom' })
      .mockResolvedValueOnce({ success: true, data: response(71, 7, [2]) })
    const events = useThreadRelatedEvents()

    await events.ensure(71, 7)
    expect(events.entry(71, 7).status).toBe('error')
    expect(events.entry(71, 7).error).toBe('boom')

    // 错误态不会被当成已缓存：重开卡片/切档回来就重试（生产重试入口）
    await events.ensure(71, 7)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(2)
    expect(events.entry(71, 7).status).toBe('success')
  })

  it('force 绕过已成功的缓存重发（需要强制新鲜度时用）', async () => {
    api.getThreadRelations.mockResolvedValue({ success: true, data: response(78, 7) })
    const events = useThreadRelatedEvents()

    await events.ensure(78, 7)
    await events.ensure(78, 7)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(1)

    await events.ensure(78, 7, true)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(2)
  })

  it('请求抛异常也落成错误态（不生吞成 loading）', async () => {
    api.getThreadRelations.mockRejectedValue(new Error('network down'))
    const events = useThreadRelatedEvents()

    await events.ensure(72, 7)
    expect(events.entry(72, 7).status).toBe('error')
  })

  it('7 天与 30 天档互不污染，切回已成功档位命中缓存', async () => {
    api.getThreadRelations.mockImplementation(async (reportId: number, params: { days: number }) => ({
      success: true,
      data: response(reportId, params.days, params.days === 7 ? [1] : [1, 2]),
    }))
    const events = useThreadRelatedEvents()

    await events.ensure(73, 7)
    await events.ensure(73, 30)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(2)
    expect(Object.keys(events.entry(73, 7).data?.threads ?? {})).toEqual(['1'])
    expect(Object.keys(events.entry(73, 30).data?.threads ?? {})).toEqual(['1', '2'])

    await events.ensure(73, 7)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(2)
  })

  it('不同报告的同一窗口各存一份', async () => {
    api.getThreadRelations.mockImplementation(async (reportId: number) => ({
      success: true,
      data: response(reportId, 7),
    }))
    const events = useThreadRelatedEvents()

    await events.ensure(74, 7)
    await events.ensure(75, 7)
    expect(api.getThreadRelations).toHaveBeenCalledTimes(2)
    expect(events.entry(74, 7).data?.report_id).toBe(74)
    expect(events.entry(75, 7).data?.report_id).toBe(75)
  })
})
