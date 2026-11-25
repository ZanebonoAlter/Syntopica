import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMock, postMock, putMock, buildQueryParamsMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
  putMock: vi.fn(),
  buildQueryParamsMock: vi.fn(() => ''),
}))

vi.mock('./client', () => ({
  apiClient: {
    get: getMock,
    post: postMock,
    put: putMock,
    delete: vi.fn(),
    buildQueryParams: buildQueryParamsMock,
  },
}))

import { useArticlesApi } from './articles'

/**
 * GET /articles/:id 的判定字段归一（add-ai-generated-article-detection review R-3）。
 *
 * 契约：`ai_detection_*` 的 snake_case → camelCase 映射 MUST 发生在 API 模块
 * （reading.md 约束 1），调用方（feature composable / 组件）只消费 camelCase，
 * 不得再自带一次映射或 `as unknown as` 断言。
 */
describe('useArticlesApi.getArticle — 判定字段在 API 模块归一', () => {
  beforeEach(() => {
    getMock.mockReset()
  })

  it('详情响应里的 ai_detection_* 转成 camelCase 四字段', async () => {
    getMock.mockResolvedValue({
      success: true,
      data: {
        id: 42,
        feed_id: 7,
        title: '一篇长得像 AI 写的文章',
        link: 'https://example.com/a',
        pub_date: '2026-11-20T10:00:00+08:00',
        created_at: '2026-11-20T10:00:00+08:00',
        read: false,
        favorite: false,
        ai_detection_status: 'suspect',
        ai_detection_score: 0.91,
        ai_detection_source: 'llm',
        ai_detection_at: '2026-11-20T10:05:00+08:00',
      },
    })

    const response = await useArticlesApi().getArticle(42)

    expect(getMock).toHaveBeenCalledWith('/articles/42')
    expect(response.data).toMatchObject({
      aiDetectionStatus: 'suspect',
      aiDetectionScore: 0.91,
      aiDetectionSource: 'llm',
      aiDetectionAt: '2026-11-20T10:05:00+08:00',
    })
  })

  it('非法枚举值降级为 undefined（不展示标记），不伪造字段', async () => {
    getMock.mockResolvedValue({
      success: true,
      data: { id: 43, title: 'x', ai_detection_status: 'maybe', ai_detection_score: '0.5' },
    })

    const response = await useArticlesApi().getArticle(43)

    expect(response.data?.aiDetectionStatus).toBeUndefined()
    expect(response.data?.aiDetectionScore).toBeUndefined()
  })

  it('失败响应原样透传（不构造假 data）', async () => {
    getMock.mockResolvedValue({ success: false, error: 'Article not found' })

    const response = await useArticlesApi().getArticle(404)

    expect(response.success).toBe(false)
    expect(response.data).toBeUndefined()
  })
})

/** 显式重判入口（design D10/D14 / review R-6）：JSON 接口路径与载荷固定。 */
describe('useArticlesApi.recheckAIDetection', () => {
  beforeEach(() => {
    postMock.mockReset()
  })

  it('POST /articles/:id/ai-detection/recheck，返回入队的 job_id', async () => {
    postMock.mockResolvedValue({
      success: true,
      message: '重新判定已提交，请稍后刷新查看结果',
      data: { job_id: 12, article_id: 42, status: 'pending' },
    })

    const response = await useArticlesApi().recheckAIDetection(42)

    expect(postMock).toHaveBeenCalledWith('/articles/42/ai-detection/recheck')
    expect(response.data?.job_id).toBe(12)
  })
})
