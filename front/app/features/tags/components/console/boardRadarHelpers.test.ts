/**
 * boardRadarHelpers 纯函数测试（rework-workshop-into-radar-and-desk 8A.2 白盒 + R-18 归一形态
 * + retire-story-adopt-event-metrics 3.2/4.1/4.2 当天线索口径与阈值重标定）。
 * 分档边界按真库实测新值（P80 / P20，留痕见 design.md D9 与 tasks.md 4.1）、
 * 线索环比、延迟判定与 14 日轴都在最便宜层验证；输入为 API 边界归一后的 camelCase 读模型字段。
 */
import { describe, expect, it } from 'vitest'
import { RADAR_HOT_ATTENTION, RADAR_WARM_ATTENTION, isDelayed, miniAxis, shortDate, tierLabel, trendOf } from './boardRadarHelpers'

describe('boardRadarHelpers — 雷达总览纯函数', () => {
  it('阈值常量：真库新口径版块关注度 P80/P20（一位小数）', () => {
    expect(RADAR_HOT_ATTENTION).toBe(14.5)
    expect(RADAR_WARM_ATTENTION).toBe(2.3)
    expect(RADAR_HOT_ATTENTION).toBeGreaterThan(RADAR_WARM_ATTENTION)
  })

  it('旧口径阈值一律禁用（4.2）：不得沿用 3.3 / 0.7，也不得沿用更早的 50 / 15', () => {
    const retired = [3.3, 0.7, 50, 15]
    expect(retired).not.toContain(RADAR_HOT_ATTENTION)
    expect(retired).not.toContain(RADAR_WARM_ATTENTION)
    // 量纲是「线索条数之和」，量级必须显著高于旧的 lane_heat 阈值
    expect(RADAR_HOT_ATTENTION).toBeGreaterThan(10)
    expect(RADAR_WARM_ATTENTION).toBeGreaterThan(1)
  })

  it('tierLabel：恰等于 P80 → 热点、恰等于 P20 → 关注、各低一档 → 降一档', () => {
    expect(tierLabel(RADAR_HOT_ATTENTION)).toBe('热点')
    expect(tierLabel(RADAR_HOT_ATTENTION - 0.1)).toBe('关注')
    expect(tierLabel(RADAR_WARM_ATTENTION)).toBe('关注')
    expect(tierLabel(RADAR_WARM_ATTENTION - 0.1)).toBe('常温')
    expect(tierLabel(0)).toBe('常温')
  })

  it('tierLabel 新值边界（4.1 重标后）：14.5/2.3 分档，13.5 与 2.0 落在下档', () => {
    expect(tierLabel(13.5)).toBe('关注')
    expect(tierLabel(14.4)).toBe('关注')
    expect(tierLabel(2.0)).toBe('常温')
    expect(tierLabel(2.2)).toBe('常温')
  })

  it('trendOf：近 3 日 / 前 3 日线索合计环比（前 3 日为 0 时新出现或「—」）', () => {
    expect(trendOf({ last3Clues: 5, prev3Clues: 0 })).toEqual({ text: '新出现', dir: 'up' })
    expect(trendOf({ last3Clues: 0, prev3Clues: 0 })).toEqual({ text: '—', dir: 'flat' })
    // 线索从有到无 → -100%，不伪装成「—」
    expect(trendOf({ last3Clues: 0, prev3Clues: 5 })).toEqual({ text: '-100%', dir: 'down' })
  })

  it('trendOf：环比百分比带符号与方向', () => {
    expect(trendOf({ last3Clues: 13, prev3Clues: 10 })).toEqual({ text: '+30%', dir: 'up' })
    expect(trendOf({ last3Clues: 6, prev3Clues: 8 })).toEqual({ text: '-25%', dir: 'down' })
    expect(trendOf({ last3Clues: 8, prev3Clues: 8 })).toEqual({ text: '0%', dir: 'flat' })
  })

  it('trendOf：旧后端缺字段（null）→ 「—」，不造 0 也不伪造环比', () => {
    expect(trendOf({ last3Clues: null, prev3Clues: 10 })).toEqual({ text: '—', dir: 'flat' })
    expect(trendOf({ last3Clues: 10, prev3Clues: null })).toEqual({ text: '—', dir: 'flat' })
    expect(trendOf({ last3Clues: null, prev3Clues: null })).toEqual({ text: '—', dir: 'flat' })
  })

  it('isDelayed：显式标记或锚日落后于全局日期', () => {
    expect(isDelayed({ dataDelayed: true, anchorDate: '2026-10-24' }, '2026-10-24')).toBe(true)
    expect(isDelayed({ dataDelayed: false, anchorDate: '2026-10-22' }, '2026-10-24')).toBe(true)
    expect(isDelayed({ dataDelayed: false, anchorDate: '2026-10-24' }, '2026-10-24')).toBe(false)
    expect(isDelayed({ dataDelayed: false, anchorDate: '2026-10-22' }, '')).toBe(false)
  })

  it('shortDate：YYYY-MM-DD → MM/DD；缺失给占位；兼容 datetime 后缀', () => {
    expect(shortDate('2026-10-24')).toBe('10/24')
    expect(shortDate('2026-10-05T00:00:00Z')).toBe('10/05')
    expect(shortDate(undefined)).toBe('日期未提供')
  })

  it('miniAxis：以锚日为终点的 14 日轴，含跨月', () => {
    expect(miniAxis('2026-10-14', 14)).toHaveLength(14)
    expect(miniAxis('2026-10-14', 14)[0]).toBe('2026-10-01')
    expect(miniAxis('2026-10-14', 14)[13]).toBe('2026-10-14')
    expect(miniAxis('2026-11-02', 3)).toEqual(['2026-10-31', '2026-11-01', '2026-11-02'])
  })
})
