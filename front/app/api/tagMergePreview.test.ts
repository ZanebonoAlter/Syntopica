import { describe, expect, it, vi } from 'vitest'

const { get, buildQueryParams } = vi.hoisted(() => ({
  get: vi.fn(),
  buildQueryParams: vi.fn(() => 'limit=200'),
}))

vi.mock('./client', () => ({
  apiClient: { get, buildQueryParams },
}))
vi.mock('~/utils/api', () => ({ getApiOrigin: () => 'http://localhost:5100' }))

import { useTagMergePreviewApi } from './tagMergePreview'

describe('tagMergePreview API — 查询串拼接', () => {
  it('loadMergeGroups 在 buildQueryParams 结果前补问号（修复 404 空响应）', async () => {
    get.mockResolvedValue({ success: true, data: { groups: [] } })
    await useTagMergePreviewApi().loadMergeGroups({ limit: 200 })
    expect(get).toHaveBeenCalledWith('/topic-tags/merge-preview?limit=200')
  })
})
