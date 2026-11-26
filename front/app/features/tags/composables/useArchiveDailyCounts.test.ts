import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { ref } from 'vue'
import { useArchiveDailyCounts } from './useArchiveDailyCounts'

const { apiMock } = vi.hoisted(() => ({
  apiMock: { getBoardArticleDailyCounts: vi.fn() },
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return { ...original, useSemanticBoardsApi: () => apiMock }
})

function payload(earliest: string, latest: string, days: Array<{ date: string, count: number }>) {
  return { success: true, data: { earliest_date: earliest, latest_date: latest, days } }
}

/** 每个用例用独立 boardId：缓存是模块级的，跨用例共用会命中 TTL。 */
let boardSeq = 2000
function nextBoard(): number {
  boardSeq += 1
  return boardSeq
}

beforeEach(() => {
  apiMock.getBoardArticleDailyCounts.mockReset()
})

describe('useArchiveDailyCounts — 缓存与请求纪律', () => {
  it('同版块 TTL 内二次挂载不再请求（模块级缓存）', async () => {
    const id = nextBoard()
    apiMock.getBoardArticleDailyCounts.mockResolvedValue(payload('2026-11-01', '2026-11-18', [
      { date: '2026-11-18', count: 7 },
    ]))

    const first = useArchiveDailyCounts(ref(id))
    await flushPromises()
    expect(first.earliest.value).toBe('2026-11-01')
    expect(first.byDate.value.get('2026-11-18')).toBe(7)

    const second = useArchiveDailyCounts(ref(id))
    await flushPromises()
    expect(second.earliest.value).toBe('2026-11-01')
    expect(apiMock.getBoardArticleDailyCounts).toHaveBeenCalledTimes(1)
  })

  it('reload(true) 绕过 TTL 强制重拉', async () => {
    const id = nextBoard()
    apiMock.getBoardArticleDailyCounts.mockResolvedValue(payload('2026-11-01', '2026-11-18', []))
    const state = useArchiveDailyCounts(ref(id))
    await flushPromises()
    expect(apiMock.getBoardArticleDailyCounts).toHaveBeenCalledTimes(1)

    apiMock.getBoardArticleDailyCounts.mockResolvedValue(payload('2026-11-02', '2026-11-19', []))
    await state.reload()
    await flushPromises()
    expect(apiMock.getBoardArticleDailyCounts).toHaveBeenCalledTimes(2)
    expect(state.earliest.value).toBe('2026-11-02')
  })

  it('切版块后旧请求迟到不得覆盖新版块数据（requestSeq 守卫）', async () => {
    const oldId = nextBoard()
    const newId = nextBoard()
    let resolveOld: ((value: unknown) => void) | null = null
    apiMock.getBoardArticleDailyCounts.mockImplementation((id: number) => {
      if (id === oldId) return new Promise((resolve) => { resolveOld = resolve })
      return Promise.resolve(payload('2026-11-10', '2026-11-19', [{ date: '2026-11-19', count: 3 }]))
    })

    const boardId = ref(oldId)
    const state = useArchiveDailyCounts(boardId)
    await flushPromises()

    boardId.value = newId
    await flushPromises()
    expect(state.earliest.value).toBe('2026-11-10')

    resolveOld!(payload('2000-01-01', '2000-01-02', [{ date: '2000-01-01', count: 99 }]))
    await flushPromises()
    expect(state.earliest.value).toBe('2026-11-10')
    expect(state.byDate.value.get('2000-01-01')).toBeUndefined()
  })

  it('刷新失败不覆盖旧值，只暴露 error', async () => {
    const id = nextBoard()
    apiMock.getBoardArticleDailyCounts.mockResolvedValueOnce(payload('2026-11-01', '2026-11-18', [
      { date: '2026-11-18', count: 7 },
    ]))
    const state = useArchiveDailyCounts(ref(id))
    await flushPromises()
    expect(state.earliest.value).toBe('2026-11-01')

    apiMock.getBoardArticleDailyCounts.mockRejectedValueOnce(new Error('boom'))
    await state.reload()
    await flushPromises()
    expect(state.earliest.value).toBe('2026-11-01')
    expect(state.byDate.value.get('2026-11-18')).toBe(7)
    expect(state.error.value).toBeTruthy()
  })
})
