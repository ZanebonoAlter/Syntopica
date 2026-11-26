/**
 * otherTopicObservation（8C G3 同归属跨日观察）API 边界测试。
 *
 * 覆盖：snake_case → camelCase、数字 ID 字符串化、period_date 归一、thread_count
 * 缺失/非法 = 未知（不冒 0）、nested `persistent_topic` 当前属性**不映射**为历史、
 * 只读 GET 的 days 口径（0 = 端点全部历史；正数会被后端静默钳到 90 天，不能覆盖 365 池）。
 * G3 收口（M-API）：产品读取签名不再开放窗口 days——调用方无法传 7 天窗口却拿到
 * `coverage:'all'` 的少覆盖数据；返回统一 `ApiResponse<T>` 形状（alias，不另造包装）。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  SECTION_TIMELINE_ALL_DAYS,
  normalizeSectionTimeline,
  useOtherTopicObservationApi,
  type SectionTimelinePayloadNode,
} from './otherTopicObservation'

const api = vi.hoisted(() => ({ getBoardSectionTimeline: vi.fn() }))
vi.mock('~/api/dailyReports', () => ({ useDailyReportsApi: () => api }))

function node(overrides: Partial<SectionTimelinePayloadNode> = {}): SectionTimelinePayloadNode {
  return {
    id: 501,
    report_id: 1211,
    period_date: '2026-10-22T00:00:00Z',
    cluster_label: '海峡局势',
    thread_count: 4,
    persistent_topic_id: 1204,
    ...overrides,
  }
}

beforeEach(() => {
  api.getBoardSectionTimeline.mockReset()
})
afterEach(() => {
  vi.clearAllMocks()
})

describe('normalizeSectionTimeline', () => {
  it('数字 ID 字符串化 / 日期归一 / camelCase；保留历史分区标题与计数', () => {
    const data = normalizeSectionTimeline({ sections: [node()], relations: [] })
    expect(data.coverage).toBe('all')
    expect(data.records).toEqual([
      {
        sectionId: '501',
        reportId: '1211',
        date: '2026-10-22',
        clusterLabel: '海峡局势',
        threadCount: 4,
        persistentTopicId: '1204',
      },
    ])
    expect(data.records[0]).not.toHaveProperty('period_date')
    expect(data.records[0]).not.toHaveProperty('section_id')
  })

  it('threadCount 缺失/非数字 = null（未知，不补 0）；0 是合法已知值', () => {
    const data = normalizeSectionTimeline({
      sections: [
        node({ id: 1, thread_count: undefined }),
        node({ id: 2, thread_count: null }),
        node({ id: 3, thread_count: 0 }),
        node({ id: 4, thread_count: Number.NaN }),
      ],
    })
    expect(data.records.map(record => record.threadCount)).toEqual([null, null, 0, null])
  })

  it('nested persistent_topic 是当前话题属性：不进入历史记录（label/status/hit 不冒历史）', () => {
    const data = normalizeSectionTimeline({
      sections: [node({
        persistent_topic_id: 1204,
        // 端点的 nested brief 是**当前** label/status/hit_count，不是生成时快照
        persistent_topic: { label: '现在的名字', status: 'archived', hit_count: 9, can_activate: true },
      })],
    })
    const record = data.records[0]!
    expect(record.persistentTopicId).toBe('1204')
    expect(record.clusterLabel).toBe('海峡局势')
    expect(record).not.toHaveProperty('persistentTopic')
    expect(record).not.toHaveProperty('label')
    expect(record).not.toHaveProperty('status')
    expect(JSON.stringify(record)).not.toContain('现在的名字')
    expect(JSON.stringify(record)).not.toContain('archived')
  })

  it('缺稳定分区 ID 或日期无法归一的记录被排除（不能定位记录日）', () => {
    const data = normalizeSectionTimeline({
      sections: [
        node({ id: 1 }),
        node({ id: null }),
        node({ id: 3, period_date: null }),
        node({ id: 4, period_date: '' }),
      ],
    })
    expect(data.records.map(record => record.sectionId)).toEqual(['1'])
  })

  it('空/缺 sections 返回空记录集，日期前缀拒绝非法类型', () => {
    expect(normalizeSectionTimeline(null).records).toEqual([])
    expect(normalizeSectionTimeline({}).records).toEqual([])
    expect(normalizeSectionTimeline({ sections: [node({ period_date: '2026-10-22T00:00:00Z' })] }).records[0]!.date)
      .toBe('2026-10-22')
  })
})

describe('useOtherTopicObservationApi.loadBoardSectionTimeline', () => {
  it('产品读取只请求 days=0（端点全部历史）；coverage 只描述这一口径', async () => {
    api.getBoardSectionTimeline.mockResolvedValue({ success: true, data: { sections: [node()], relations: [] } })
    const result = await useOtherTopicObservationApi().loadBoardSectionTimeline(1974)
    expect(SECTION_TIMELINE_ALL_DAYS).toBe(0)
    expect(api.getBoardSectionTimeline).toHaveBeenCalledWith(1974, 0)
    expect(result.success).toBe(true)
    expect(result.data?.records).toHaveLength(1)
    expect(result.data?.coverage).toBe('all')
  })

  it('不开放窗口 days：即使调用方硬塞正数也仍请求 0，不会少覆盖却标 coverage all', async () => {
    api.getBoardSectionTimeline.mockResolvedValue({ success: true, data: { sections: [], relations: [] } })
    // 运行时回归守卫：签名只有 boardId，多塞的 days 参数必须被忽略（不把窗口口径冒成全部历史）
    const load = useOtherTopicObservationApi().loadBoardSectionTimeline as unknown as (
      boardId: number,
      days?: number,
    ) => Promise<{ success: boolean, data?: { records: unknown[], coverage: string } }>
    await load(1974, 7)
    expect(api.getBoardSectionTimeline).toHaveBeenCalledWith(1974, 0)
  })

  it('返回统一 ApiResponse 形状；失败错误体逐字段透传，不伪造空记录集为成功', async () => {
    api.getBoardSectionTimeline.mockResolvedValue({ success: false, error: 'boom', message: '读取失败', status: 500 })
    const result = await useOtherTopicObservationApi().loadBoardSectionTimeline(1974)
    expect(result).toEqual({ success: false, error: 'boom', message: '读取失败', status: 500 })
  })
})
