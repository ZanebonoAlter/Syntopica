/**
 * 共享描述修复四个 HTTP 资源（board-health-triage-advice seam C1 / task 3.1）。
 *
 * 断言 URL / 方法 / body 组装；wire 仍是数字 ID，字符串 ID 视图由 normalizer 产出。
 * 只 mock apiClient 的 HTTP 动词——不触网、不打真库。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getMock, postMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
}))

// 只覆盖 HTTP 动词；prototype 上的 buildQueryParams 等真实方法保留。
vi.mock('./client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./client')>()
  actual.apiClient.get = getMock
  actual.apiClient.post = postMock
  return { apiClient: actual.apiClient }
})

import { useAuxiliaryLabelsApi } from './auxiliaryLabels'

describe('共享描述修复四资源', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
  })

  it('GET context：URL 带标签 id', async () => {
    getMock.mockResolvedValue({ success: true, data: {} })
    const api = useAuxiliaryLabelsApi()
    await api.getDescriptionRepairContext(1974)
    expect(getMock).toHaveBeenCalledWith('/auxiliary-labels/1974/description-repair-context')
  })

  it('GET context：字符串 ID 也能拼 URL（前端边界已转 string）', async () => {
    getMock.mockResolvedValue({ success: true, data: {} })
    const api = useAuxiliaryLabelsApi()
    await api.getDescriptionRepairContext('1974')
    expect(getMock).toHaveBeenCalledWith('/auxiliary-labels/1974/description-repair-context')
  })

  it('POST 创建任务：body 含 description/expected_version/idempotency_key（snake_case wire）', async () => {
    postMock.mockResolvedValue({ success: true, data: {} })
    const api = useAuxiliaryLabelsApi()
    await api.createDescriptionRepair(1974, {
      description: '中国的超大城市',
      expected_version: 'v-opaque-abc',
      idempotency_key: 'key-1',
    })
    expect(postMock).toHaveBeenCalledWith('/auxiliary-labels/1974/description-repairs', {
      description: '中国的超大城市',
      expected_version: 'v-opaque-abc',
      idempotency_key: 'key-1',
    })
  })

  it('GET 任务：URL 带标签 id 与任务 id', async () => {
    getMock.mockResolvedValue({ success: true, data: {} })
    const api = useAuxiliaryLabelsApi()
    await api.getDescriptionRepairJob(1974, 77)
    expect(getMock).toHaveBeenCalledWith('/auxiliary-labels/1974/description-repairs/77')
  })

  it('POST retry：无参数（空对象），只打 retry 端点', async () => {
    postMock.mockResolvedValue({ success: true, data: {} })
    const api = useAuxiliaryLabelsApi()
    await api.retryDescriptionRepair(1974, 77)
    expect(postMock).toHaveBeenCalledWith('/auxiliary-labels/1974/description-repairs/77/retry', {})
  })
})
