/**
 * todayKey 测试（add-radar-pipeline-status design D7 / 用例 V5）。
 *
 * 核心回归：本地日期 key **不得**走 UTC 截断（`toISOString().slice(0, 10)`
 * 在 UTC+8 的 00:00–08:00 会得到昨天，在 UTC-5 的夜里会得到明天）。
 * 这里显式切 TZ，覆盖本地 00:00 / 23:59 边界与「UTC 已跨天但本地未跨天」
 * 两个方向。
 */
import { afterEach, describe, expect, it } from 'vitest'
import { todayKey } from './todayKey'

const originalTz = process.env.TZ

function withTz(tz: string, fn: () => void): void {
  process.env.TZ = tz
  try {
    fn()
  } finally {
    if (originalTz === undefined) delete process.env.TZ
    else process.env.TZ = originalTz
  }
}

afterEach(() => {
  if (originalTz === undefined) delete process.env.TZ
  else process.env.TZ = originalTz
})

describe('todayKey — 本地日期 key', () => {
  it('本地 00:00 边界：仍算当天', () => {
    withTz('Asia/Shanghai', () => {
      expect(todayKey(new Date(2026, 9, 24, 0, 0, 0))).toBe('2026-10-24')
    })
  })

  it('本地 23:59 边界：仍算当天', () => {
    withTz('Asia/Shanghai', () => {
      expect(todayKey(new Date(2026, 9, 24, 23, 59, 59))).toBe('2026-10-24')
    })
  })

  it('UTC 已跨天但本地未跨天（UTC+8 凌晨）：按本地日期，不取 UTC 的昨天', () => {
    withTz('Asia/Shanghai', () => {
      // 2026-10-24T16:30Z = 东八区 2026-10-25 00:30（UTC 日期还是 24 号）
      const d = new Date('2026-10-24T16:30:00Z')
      expect(d.toISOString().slice(0, 10)).toBe('2026-10-24')
      expect(todayKey(d)).toBe('2026-10-25')
    })
  })

  it('UTC 已跨天但本地未跨天（UTC-5 夜间）：按本地日期，不取 UTC 的明天', () => {
    withTz('America/New_York', () => {
      // 2026-10-25T01:30Z = 纽约 2026-10-24 21:30（UTC 日期已到 25 号）
      const d = new Date('2026-10-25T01:30:00Z')
      expect(d.toISOString().slice(0, 10)).toBe('2026-10-25')
      expect(todayKey(d)).toBe('2026-10-24')
    })
  })

  it('月份/日期补零，输出恒为 YYYY-MM-DD', () => {
    withTz('Asia/Shanghai', () => {
      expect(todayKey(new Date(2026, 0, 5, 12, 0, 0))).toBe('2026-01-05')
      expect(todayKey(new Date(2026, 11, 31, 12, 0, 0))).toBe('2026-12-31')
    })
  })

  it('不传参时取此刻的本地日期', () => {
    const expected = todayKey(new Date())
    expect(todayKey()).toBe(expected)
    expect(todayKey()).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})
