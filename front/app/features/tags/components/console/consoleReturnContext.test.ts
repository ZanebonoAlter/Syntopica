/**
 * consoleReturnContext 测试（矩阵阶段 G5：研究工作台往返保泳道/日期/范围/滚动）。
 *
 * 判据：一次性读取、URL 显式值优先、非法项不补造、无 sessionStorage 静默降级、
 * 不携带 report/annotation 一次性深链。
 */
import { describe, expect, it } from 'vitest'
import {
  hasExplicitViewQuery,
  OTHER_READING_KEY_PREFIX,
  otherReadingKey,
  sanitizeReturnContext,
  saveReturnContext,
  takeReturnContext,
  type ConsoleReturnContext,
} from './consoleReturnContext'

function memoryStorage() {
  const map = new Map<string, string>()
  return {
    getItem: (key: string) => map.get(key) ?? null,
    setItem: (key: string, value: string) => { map.set(key, value) },
    removeItem: (key: string) => { map.delete(key) },
    size: () => map.size,
  }
}

const CONTEXT: ConsoleReturnContext = {
  laneId: '1204',
  date: '2026-10-24',
  days: 14,
  start: '2026-10-11',
  end: '2026-10-24',
  scrollTop: 846,
}

/** 反序列化（scrollTop 四舍五入）后的旧六字段形状。 */
const CONTEXT_WITH_SCROLL: ConsoleReturnContext = { ...CONTEXT, scrollTop: 120 }

describe('consoleReturnContext — 存取', () => {
  it('按板块保存并一次性取出（读后即删，不重复重放）', () => {
    const storage = memoryStorage()
    saveReturnContext(1974, CONTEXT, storage)
    expect(takeReturnContext(1974, storage)).toEqual(CONTEXT)
    expect(takeReturnContext(1974, storage)).toBeNull()
    expect(storage.size()).toBe(0)
  })

  it('板块之间互不串用', () => {
    const storage = memoryStorage()
    saveReturnContext(1974, CONTEXT, storage)
    expect(takeReturnContext(1980, storage)).toBeNull()
    expect(takeReturnContext(1974, storage)).toEqual(CONTEXT)
  })

  it('存储不可用静默降级：不抛错、取出为 null', () => {
    const broken = {
      getItem: () => { throw new Error('denied') },
      setItem: () => { throw new Error('denied') },
      removeItem: () => { throw new Error('denied') },
    }
    expect(() => saveReturnContext(1974, CONTEXT, broken)).not.toThrow()
    expect(takeReturnContext(1974, broken)).toBeNull()
    expect(takeReturnContext(1974, null)).toBeNull()
  })

  it('损坏 JSON 取出 null 并清理', () => {
    const storage = memoryStorage()
    storage.setItem('syntopica:board-console:return:1974', '{not-json')
    expect(takeReturnContext(1974, storage)).toBeNull()
    expect(storage.size()).toBe(0)
  })
})

describe('consoleReturnContext — 显式 URL 优先', () => {
  it('任一视图参数在场即视为显式（不恢复缓存）', () => {
    expect(hasExplicitViewQuery({})).toBe(false)
    expect(hasExplicitViewQuery(undefined)).toBe(false)
    expect(hasExplicitViewQuery({ lane: '1204' })).toBe(true)
    expect(hasExplicitViewQuery({ days: '30' })).toBe(true)
    expect(hasExplicitViewQuery({ start: '2026-10-01' })).toBe(true)
    expect(hasExplicitViewQuery({ end: '2026-10-01' })).toBe(true)
    expect(hasExplicitViewQuery({ lane: '' })).toBe(false)
    expect(hasExplicitViewQuery({ focus: 'report', board: '1974' })).toBe(false)
    // 数组形态（vue-router 重复参数）取首值
    expect(hasExplicitViewQuery({ lane: ['1204'] })).toBe(true)
  })
})

describe('consoleReturnContext — 反序列化收敛', () => {
  it('保留合法项，丢弃非法项（不补造日期/档位）', () => {
    expect(sanitizeReturnContext({
      laneId: '1204',
      date: '2026-10-24',
      days: 30,
      start: '2026-10-01',
      end: '2026-10-24',
      scrollTop: 120.6,
      report: '1239',
      annotation: 'abc',
    })).toEqual({
      laneId: '1204',
      date: '2026-10-24',
      days: 30,
      start: '2026-10-01',
      end: '2026-10-24',
      scrollTop: 121,
    })
  })

  it('非法档位/格式/倒序范围置空；全空返回 null', () => {
    expect(sanitizeReturnContext({ laneId: 'x', date: '2026-10-6', days: 8, start: '2026-10-24', end: '2026-10-01' }))
      .toBeNull()
    const partial = sanitizeReturnContext({ days: '7', start: '2026-10-01' })
    expect(partial).toEqual({ laneId: null, date: null, days: 7, start: null, end: null, scrollTop: 0 })
    expect(sanitizeReturnContext(null)).toBeNull()
    expect(sanitizeReturnContext('x')).toBeNull()
  })

  it('不携带一次性深链键（report/annotation 不进返回上下文）', () => {
    const restored = sanitizeReturnContext({ laneId: '1204', report: '1239', annotation: 'x' })
    expect(restored).toEqual({ laneId: '1204', date: null, days: null, start: null, end: null, scrollTop: 0 })
    expect(Object.keys(restored ?? {})).toEqual(['laneId', 'date', 'days', 'start', 'end', 'scrollTop'])
  })
})

describe('consoleReturnContext — 其他分区阅读槽（8C G4a）', () => {
  it('otherReadingKey 与 G3 阅读终态 laneKey 同命名空间', () => {
    expect(OTHER_READING_KEY_PREFIX).toBe('other:')
    expect(otherReadingKey('502')).toBe('other:502')
    expect(otherReadingKey('502')).not.toBe('502')
  })

  it('old 六字段 payload 仍可读且不新增键（向后兼容）', () => {
    const legacy = sanitizeReturnContext({
      laneId: '1204', date: '2026-10-24', days: 14,
      start: '2026-10-11', end: '2026-10-24', scrollTop: 120,
    })
    expect(legacy).toEqual(CONTEXT_WITH_SCROLL)
    expect(Object.keys(legacy ?? {})).toEqual(['laneId', 'date', 'days', 'start', 'end', 'scrollTop'])
  })

  it('合法 other 透传，reportId/topicId 非法只置 null', () => {
    const restored = sanitizeReturnContext({
      laneId: '10', date: '2026-10-24', scrollTop: 0,
      other: { sectionId: '502', date: '2026-10-24', reportId: '900', topicId: '77' },
      othersOpen: true,
    })
    expect(restored?.other).toEqual({ sectionId: '502', date: '2026-10-24', reportId: '900', topicId: '77' })
    expect(restored?.othersOpen).toBe(true)

    const degraded = sanitizeReturnContext({
      date: '2026-10-24',
      other: { sectionId: '502', date: '2026-10-24', reportId: 'x', topicId: 77 },
    })
    expect(degraded?.other).toEqual({ sectionId: '502', date: '2026-10-24', reportId: null, topicId: null })
    expect(degraded?.othersOpen).toBe(false)
  })

  it('非法 other 静默降级（缺 sectionId/日期、日期与报告日不一致、非对象）', () => {
    const base = { laneId: '10', date: '2026-10-24', scrollTop: 0 }
    for (const other of [
      null,
      'x',
      { date: '2026-10-24' },
      { sectionId: 'abc', date: '2026-10-24' },
      { sectionId: '502', date: '2026-10-20' },
      { sectionId: '502', date: '2026-10-6' },
    ]) {
      expect(sanitizeReturnContext({ ...base, other })).toEqual({
        laneId: '10', date: '2026-10-24', days: null, start: null, end: null, scrollTop: 0,
      })
    }
    // 只有 other 而无任何旧字段：非法 other 不足以单独构成恢复上下文
    expect(sanitizeReturnContext({ other: { sectionId: 'x', date: '2026-10-24' } })).toBeNull()
  })

  it('other 选择按板块保存并一次性取出（含展开态，不污染 laneId）', () => {
    const storage = memoryStorage()
    const context: ConsoleReturnContext = {
      ...CONTEXT,
      other: { sectionId: '502', date: '2026-10-24', reportId: '900', topicId: '77' },
      othersOpen: true,
    }
    saveReturnContext(1974, context, storage)
    expect(takeReturnContext(1974, storage)).toEqual(context)
    expect(takeReturnContext(1974, storage)).toBeNull()
    expect(context.laneId).toBe('1204')
  })
})
