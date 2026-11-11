/**
 * boardRadarHelpers 纯函数测试（rework-workshop-into-radar-and-desk 8A.2 白盒 + R-18 归一形态
 * + add-event-story-heat 5.5 lane_heat 量纲）。
 * 分档边界 3.3/0.7（5.3 全量历史 P80/P20 标定）、热度环比与旧字段回退、
 * 延迟判定、14 日轴与小矩阵分档都在最便宜层验证；输入为 API 边界归一后的 camelCase 读模型字段。
 */
import { describe, expect, it } from 'vitest'
import { RADAR_HOT_ATTENTION, RADAR_WARM_ATTENTION, isDelayed, laneDegradedLevel, laneLevel, miniAxis, shortDate, tierLabel, trendOf } from './boardRadarHelpers'

describe('boardRadarHelpers — 雷达总览纯函数', () => {
  it('阈值常量：calibrate-heat 全版块 14d 正值 lane_heat P80/P20 保留一位小数', () => {
    expect(RADAR_HOT_ATTENTION).toBe(3.3)
    expect(RADAR_WARM_ATTENTION).toBe(0.7)
  })

  it('tierLabel：3.3 热点边界、0.7 关注边界（lane_heat 标定阈值）', () => {
    expect(tierLabel(3.3)).toBe('热点')
    expect(tierLabel(3.2)).toBe('关注')
    expect(tierLabel(0.7)).toBe('关注')
    expect(tierLabel(0.6)).toBe('常温')
    expect(tierLabel(0)).toBe('常温')
  })

  it('tierLabel 引用固定阈值常量：恰等于 P80/P20 进入对应档', () => {
    expect(tierLabel(RADAR_HOT_ATTENTION)).toBe('热点')
    expect(tierLabel(RADAR_WARM_ATTENTION)).toBe('关注')
  })

  it('trendOf：热度口径优先（last3Heat/prev3Heat）；前 3 日为 0 时新出现或「—」', () => {
    expect(trendOf({ last3Heat: 5, prev3Heat: 0, last3Sections: 0, prev3Sections: 0 })).toEqual({ text: '新出现', dir: 'up' })
    expect(trendOf({ last3Heat: 0, prev3Heat: 0, last3Sections: 5, prev3Sections: 0 })).toEqual({ text: '新出现', dir: 'up' })
    expect(trendOf({ last3Heat: 0, prev3Heat: 0, last3Sections: 0, prev3Sections: 0 })).toEqual({ text: '—', dir: 'flat' })
    // 热度从有到无（last3Heat=0 且无 sections 口径可回退）→ -100%，不伪装成「—」
    expect(trendOf({ last3Heat: 0, prev3Heat: 5, last3Sections: 0, prev3Sections: 0 })).toEqual({ text: '-100%', dir: 'down' })
  })

  it('trendOf：环比百分比带符号与方向（热度口径，忽略 sections）', () => {
    expect(trendOf({ last3Heat: 13, prev3Heat: 10, last3Sections: 0, prev3Sections: 0 })).toEqual({ text: '+30%', dir: 'up' })
    expect(trendOf({ last3Heat: 6, prev3Heat: 8, last3Sections: 99, prev3Sections: 99 })).toEqual({ text: '-25%', dir: 'down' })
    expect(trendOf({ last3Heat: 8, prev3Heat: 8, last3Sections: 0, prev3Sections: 0 })).toEqual({ text: '0%', dir: 'flat' })
  })

  it('真实全零热度不回退分区环比；显式缺字段标记优先于残留热度', () => {
    expect(trendOf({ hasHeatData: true, last3Heat: 0, prev3Heat: 0, last3Sections: 13, prev3Sections: 10 })).toEqual({ text: '—', dir: 'flat' })
    expect(trendOf({ hasHeatData: false, last3Heat: 8, prev3Heat: 2, last3Sections: 6, prev3Sections: 8 })).toEqual({ text: '-25%', dir: 'down' })
  })

  it('trendOf：无 heat 字段（旧响应三值全 0）回退 sections 旧口径', () => {
    expect(trendOf({ last3Heat: 0, prev3Heat: 0, last3Sections: 13, prev3Sections: 10 })).toEqual({ text: '+30%', dir: 'up' })
    expect(trendOf({ last3Heat: 0, prev3Heat: 0, last3Sections: 6, prev3Sections: 8 })).toEqual({ text: '-25%', dir: 'down' })
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

  it('laneDegradedLevel：降级格（代理值）独立四档，除零/零值给 0 不出 NaN', () => {
    expect(laneDegradedLevel(0, 4)).toBe(0)
    expect(laneDegradedLevel(2, 0)).toBe(0)
    // 基准 = 代理值自己的行内最大值：档位随兜底值单调，封顶 4 档
    expect(laneDegradedLevel(4, 4)).toBe(4)
    expect(laneDegradedLevel(2, 4)).toBe(2)
    expect(laneDegradedLevel(1, 4)).toBe(1)
    // 代理值分档只看代理值自己的基准（不受已确证事件线峰值影响）
    expect(laneDegradedLevel(4, 8)).toBe(2)
  })
})
