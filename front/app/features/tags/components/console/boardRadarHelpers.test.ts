/**
 * boardRadarHelpers 纯函数测试（rework-workshop-into-radar-and-desk 8A.2 白盒 + R-18 归一形态）。
 * 分档边界 50/15、3 日环比、延迟判定、14 日轴与小矩阵分档都在最便宜层验证；
 * 输入为 API 边界归一后的 camelCase 读模型字段。
 */
import { describe, expect, it } from 'vitest'
import { isDelayed, laneLevel, miniAxis, shortDate, tierLabel, trendOf } from './boardRadarHelpers'

describe('boardRadarHelpers — 雷达总览纯函数', () => {
  it('tierLabel：49/50 热点边界、14/15 关注边界', () => {
    expect(tierLabel(50)).toBe('热点')
    expect(tierLabel(49)).toBe('关注')
    expect(tierLabel(15)).toBe('关注')
    expect(tierLabel(14)).toBe('常温')
    expect(tierLabel(0)).toBe('常温')
  })

  it('trendOf：前 3 日为 0 时新出现或「—」', () => {
    expect(trendOf({ prev3Sections: 0, last3Sections: 5 })).toEqual({ text: '新出现', dir: 'up' })
    expect(trendOf({ prev3Sections: 0, last3Sections: 0 })).toEqual({ text: '—', dir: 'flat' })
  })

  it('trendOf：环比百分比带符号与方向', () => {
    expect(trendOf({ prev3Sections: 10, last3Sections: 13 })).toEqual({ text: '+30%', dir: 'up' })
    expect(trendOf({ prev3Sections: 8, last3Sections: 6 })).toEqual({ text: '-25%', dir: 'down' })
    expect(trendOf({ prev3Sections: 8, last3Sections: 8 })).toEqual({ text: '0%', dir: 'flat' })
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

  it('laneLevel：0 分区与行内最大值四档', () => {
    expect(laneLevel(0, 8)).toBe(0)
    expect(laneLevel(3, 0)).toBe(0)
    expect(laneLevel(8, 8)).toBe(4)
    expect(laneLevel(6, 8)).toBe(3)
    expect(laneLevel(1, 8)).toBe(1)
  })
})
