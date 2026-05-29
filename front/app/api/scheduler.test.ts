import { beforeEach, describe, expect, it, vi } from 'vitest'

const { postMock, getMock } = vi.hoisted(() => ({
  postMock: vi.fn(),
  getMock: vi.fn(),
}))

vi.mock('./client', () => ({
  apiClient: {
    get: getMock,
    post: postMock,
  },
}))

import { useSchedulerApi } from './scheduler'

describe('useSchedulerApi', () => {
  beforeEach(() => {
    postMock.mockReset()
    getMock.mockReset()
    vi.restoreAllMocks()
  })

  it('uses apiClient.post to trigger schedulers', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const expected = {
      success: true,
      data: {
        name: 'auto-refresh',
        accepted: true,
        started: true,
      },
      message: 'ok',
    }

    postMock.mockResolvedValue(expected)

    const api = useSchedulerApi()
    const response = await api.triggerScheduler('auto-refresh')

    expect(postMock).toHaveBeenCalledWith('/schedulers/auto-refresh/trigger', {})
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(response).toEqual(expected)
  })
})
