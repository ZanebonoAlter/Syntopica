import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { ref } from 'vue'
import { buildKeyEventIndex, groupUnassignedEvents, useBoardKeyEvents } from './useBoardKeyEvents'
import type { BoardKeyEventItem } from '~/api/boardKeyEvents'

const { apiMock } = vi.hoisted(() => ({
  apiMock: { getBoardKeyEvents: vi.fn() },
}))

vi.mock('~/api/boardKeyEvents', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/boardKeyEvents')>()
  return { ...original, useBoardKeyEventsApi: () => apiMock }
})

function event(id: string, topicId: string | null, date: string, title = id): BoardKeyEventItem {
  return {
    id, topicId, topicLabelSnapshot: `话题${topicId ?? '-'}`, sourcePeriodDate: date,
    titleSnapshot: title, summarySnapshot: '', articleIds: [],
  }
}

function ok(events: BoardKeyEventItem[]) {
  return { success: true as const, data: { events } }
}

function fail(message: string) {
  return { success: false as const, error: message }
}

/** 每个用例用独立 boardId：缓存按 board+days 分键。 */
let boardSeq = 3000
function nextBoard(): number {
  boardSeq += 1
  return boardSeq
}

beforeEach(() => {
  apiMock.getBoardKeyEvents.mockReset()
})

describe('buildKeyEventIndex — 事件索引派生（纯函数）', () => {
  it('同日多事件按接口序排列', () => {
    const index = buildKeyEventIndex([
      event('3', '7', '2026-11-12'),
      event('1', '7', '2026-11-12'),
      event('2', '7', '2026-11-11'),
    ])
    const day = index.get('7')!.get('2026-11-12')!
    expect(day.map(e => e.id)).toEqual(['3', '1'])
  })

  it('未知话题（topicId null）丢弃，不进索引', () => {
    const index = buildKeyEventIndex([
      event('a', null, '2026-11-12'),
      event('b', '7', '2026-11-12'),
    ])
    expect(index.size).toBe(1)
    expect(index.has('7')).toBe(true)
    expect(index.get('7')!.get('2026-11-12')!.map(e => e.id)).toEqual(['b'])
  })

  it('空列表 → 空索引', () => {
    expect(buildKeyEventIndex([]).size).toBe(0)
  })
})

describe('groupUnassignedEvents — 未归入行集合的事件（纯函数）', () => {
  it('收集 topicId=null 的事件（话题删除后的历史注记）', () => {
    const grouped = groupUnassignedEvents([
      event('a', null, '2026-11-12'),
      event('b', '7', '2026-11-12'),
    ], new Set(['7']))
    expect(grouped.get('2026-11-12')!.map(e => e.id)).toEqual(['a'])
  })

  it('收集不在行集合的话题事件（未关注 candidate / archived）', () => {
    const grouped = groupUnassignedEvents([
      event('a', '7', '2026-11-12'),
      event('b', '999', '2026-11-12'),
      event('c', '1000', '2026-11-13'),
    ], new Set(['7']))
    expect(grouped.get('2026-11-12')!.map(e => e.id)).toEqual(['b'])
    expect(grouped.get('2026-11-13')!.map(e => e.id)).toEqual(['c'])
  })

  it('已归属事件全部排除；无未归属事件 → 空 Map', () => {
    const grouped = groupUnassignedEvents([
      event('a', '7', '2026-11-12'),
      event('b', '8', '2026-11-13'),
    ], new Set(['7', '8']))
    expect(grouped.size).toBe(0)
  })

  it('同日多事件保持接口序', () => {
    const grouped = groupUnassignedEvents([
      event('3', '999', '2026-11-12'),
      event('1', '999', '2026-11-12'),
      event('2', null, '2026-11-12'),
    ], new Set())
    expect(grouped.get('2026-11-12')!.map(e => e.id)).toEqual(['3', '1', '2'])
  })
})

describe('useBoardKeyEvents — 缓存/三态/晚到隔离', () => {
  it('成功：发布事件并建索引；同 board+days 二次 load 走缓存', async () => {
    const id = nextBoard()
    apiMock.getBoardKeyEvents.mockResolvedValue(ok([event('a', '7', '2026-11-12', '事件A')]))
    const boardId = ref(id)
    const days = ref(14)
    const state = useBoardKeyEvents({ boardId, days })
    await flushPromises()
    expect(state.events.value.map(e => e.id)).toEqual(['a'])
    expect(state.eventIndex.value.get('7')!.get('2026-11-12')!.length).toBe(1)
    expect(state.loading.value).toBe(false)
    expect(state.error.value).toBeNull()

    await state.load()
    expect(apiMock.getBoardKeyEvents).toHaveBeenCalledTimes(1)
  })

  it('首次失败：error 态（不冒称 0 条事件）', async () => {
    const id = nextBoard()
    apiMock.getBoardKeyEvents.mockResolvedValue(fail('boom'))
    const state = useBoardKeyEvents({ boardId: ref(id), days: ref(14) })
    await flushPromises()
    expect(state.error.value).toBe('boom')
    expect(state.loading.value).toBe(false)
    expect(state.staleNotice.value).toBe(false)
  })

  it('刷新失败：保留旧数据 + stale 标记；重试成功后清除', async () => {
    const id = nextBoard()
    apiMock.getBoardKeyEvents.mockResolvedValueOnce(ok([event('a', '7', '2026-11-12')]))
    const state = useBoardKeyEvents({ boardId: ref(id), days: ref(14) })
    await flushPromises()
    expect(state.events.value.length).toBe(1)

    apiMock.getBoardKeyEvents.mockResolvedValueOnce(fail('网络断了'))
    await state.load({ force: true })
    await flushPromises()
    expect(state.events.value.length).toBe(1) // 失败保留旧内容
    expect(state.staleNotice.value).toBe(true)
    expect(state.error.value).toBe('网络断了')

    apiMock.getBoardKeyEvents.mockResolvedValueOnce(ok([event('a', '7', '2026-11-12'), event('b', '8', '2026-11-13')]))
    await state.retry()
    await flushPromises()
    expect(state.staleNotice.value).toBe(false)
    expect(state.events.value.length).toBe(2)
  })

  it('切板块晚到：旧板块响应不污染新板块状态', async () => {
    const first = nextBoard()
    const second = nextBoard()
    let resolveFirst: (v: unknown) => void = () => {}
    apiMock.getBoardKeyEvents.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
    apiMock.getBoardKeyEvents.mockResolvedValueOnce(ok([event('new', '7', '2026-11-20')]))

    const boardId = ref(first)
    const state = useBoardKeyEvents({ boardId, days: ref(14) })
    await vi.waitFor(() => expect(apiMock.getBoardKeyEvents).toHaveBeenCalledTimes(1))

    boardId.value = second
    await flushPromises()
    expect(state.events.value.map(e => e.id)).toEqual(['new']) // 晚到的旧板块响应被丢弃

    resolveFirst(ok([event('late', '9', '2026-11-01')]))
    await flushPromises()
    expect(state.events.value.map(e => e.id)).toEqual(['new']) // 旧响应 resolve 后仍不得写入
  })

  it('切 days 档：重新取数（不沿用旧档缓存）', async () => {
    const id = nextBoard()
    apiMock.getBoardKeyEvents.mockResolvedValue(ok([event('a', '7', '2026-11-12')]))
    const days = ref(14)
    const state = useBoardKeyEvents({ boardId: ref(id), days })
    await flushPromises()
    expect(apiMock.getBoardKeyEvents).toHaveBeenCalledWith(id, 14)

    apiMock.getBoardKeyEvents.mockResolvedValue(ok([event('b', '7', '2026-10-20')]))
    days.value = 30
    await flushPromises()
    expect(apiMock.getBoardKeyEvents).toHaveBeenCalledWith(id, 30)
    expect(state.events.value.map(e => e.id)).toEqual(['b'])
  })

  it('内容来源版块快照：发布时记下版块 id + 名（stale 时点名用，review R-7）', async () => {
    const id = nextBoard()
    apiMock.getBoardKeyEvents.mockResolvedValue(ok([event('a', '7', '2026-11-12')]))
    const boardLabel = ref('版块甲')
    const state = useBoardKeyEvents({ boardId: ref(id), days: ref(14), boardLabel })
    await flushPromises()
    expect(state.contentSource.value).toEqual({ boardId: id, boardLabel: '版块甲' })
  })

  it('切版块读失败：保留旧数据 + stale，且来源仍指向**旧版块**（MUST NOT 指错版块）', async () => {
    const first = nextBoard()
    const second = nextBoard()
    apiMock.getBoardKeyEvents.mockResolvedValueOnce(ok([event('a', '7', '2026-11-12')]))
    const boardId = ref(first)
    const boardLabel = ref('版块甲')
    const state = useBoardKeyEvents({ boardId, days: ref(14), boardLabel })
    await flushPromises()

    apiMock.getBoardKeyEvents.mockResolvedValueOnce(fail('挂了'))
    boardLabel.value = '版块乙'
    boardId.value = second
    await flushPromises()

    // 失败不因切版块清空旧数据（stale 语义），但来源快照仍指旧版块
    expect(state.events.value.map(e => e.id)).toEqual(['a'])
    expect(state.staleNotice.value).toBe(true)
    expect(state.contentSource.value).toEqual({ boardId: first, boardLabel: '版块甲' })
  })
})
