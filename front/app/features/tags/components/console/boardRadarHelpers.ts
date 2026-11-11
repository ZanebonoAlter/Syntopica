/**
 * 雷达总览纯函数（rework-workshop-into-radar-and-desk 8A.1 + R-18 归一化读模型；
 * retire-story-adopt-event-metrics 3.2 换当天线索口径）。
 *
 * 关注分档（阈值见下方常量）、3 日线索环比、数据延迟判定与 14 日小矩阵日期轴在此
 * 唯一实现；输入为 API 边界归一后的 camelCase/字符串 ID 读模型（RadarBoard/RadarLane），
 * 展示组件只消费结果，不再各自复制口径（分档与环比语义见 ui-design.md「权威数据与口径」）。
 *
 * 量纲（D4/D5）：一切展示量都是「当日线索合计」；事件线数、分区数与 `laneLevel` /
 * `laneDegradedLevel` 四档分档已整体退役。色深不在这里：改由 `observationLanes.clueDepth`
 * 按当日线索数做行内归一连续深浅（2026-11-11 看版后修订，tasks 3.5），数值 + title/aria
 * 仍是唯一信息承载。
 */
import type { RadarBoard } from '~/api/normalizers/boardRadar'

// 来源：真库 SELECT 实测新口径「版块关注度」（Σ 泳道当天热度，含候选权重，一位小数）：
// 样本 = 近 14 个报告日内全部版块×日的关注度正值（154 个样本 / 13 个版块），
// 采样日期 2026-11-11，P20 = 2.3 / P80 = 14.5（一位小数）；
// 30 日窗复算 P20 = 2.0 / P80 = 14.0（口径稳定）。
// 旧值 3.3 / 0.7（lane_heat 量纲）与更早的 50 / 15（分区量纲）均已失效，见 design.md D9。
export const RADAR_HOT_ATTENTION = 14.5
export const RADAR_WARM_ATTENTION = 2.3

export type RadarTier = '热点' | '关注' | '常温'

/** 关注强度分档标签（版块关注度 = Σ 泳道当天热度）：≥P80 热点、≥P20 关注、其余常温。 */
export function tierLabel(attention: number): RadarTier {
  if (attention >= RADAR_HOT_ATTENTION) return '热点'
  if (attention >= RADAR_WARM_ATTENTION) return '关注'
  return '常温'
}

export interface RadarTrend {
  text: string
  dir: 'up' | 'down' | 'flat'
}

/** 近 3 日 / 前 3 日线索合计环比；任一侧未知（旧后端缺字段）→ 「—」，不造 0 断言。 */
export function trendOf(row: Pick<RadarBoard, 'last3Clues' | 'prev3Clues'>): RadarTrend {
  const last3 = row.last3Clues
  const prev3 = row.prev3Clues
  if (last3 === null || prev3 === null) return { text: '—', dir: 'flat' }
  if (!prev3) {
    return last3
      ? { text: '新出现', dir: 'up' }
      : { text: '—', dir: 'flat' }
  }
  const change = Math.round((last3 / prev3 - 1) * 100)
  return {
    text: `${change > 0 ? '+' : ''}${change}%`,
    dir: change > 0 ? 'up' : change < 0 ? 'down' : 'flat',
  }
}

/** 板块锚日落后于总览全局日期（或后端标记延迟）时 UI 必须可辨。 */
export function isDelayed(
  row: Pick<RadarBoard, 'dataDelayed' | 'anchorDate'>,
  globalDate?: string,
): boolean {
  return Boolean(row.dataDelayed)
    || (Boolean(row.anchorDate) && Boolean(globalDate) && row.anchorDate !== globalDate)
}

/**
 * 滞后判定（2026-11-11 看版后修订，tasks 3.6）：锚点日 ≠ 全局锚点日即滞后。
 *
 * 「滞后」不是「延迟」（`isDelayed` 的 dataDelayed 分支）：数据是齐的，只是这个版块的
 * 「今天」更早。分档阈值按今日分布标定，旧数据不可比 → 滞后版块不挂分档徽标、右栏条目沉底
 * 灰显；但热度数值与线索数照旧如实显示，不伪造同日可比性。任一侧日期缺失 → 不判滞后（未知不伪造）。
 */
export function isLaggingAnchor(anchorDate?: string, globalDate?: string): boolean {
  const anchor = anchorDate ? anchorDate.slice(0, 10) : ''
  const global = globalDate ? globalDate.slice(0, 10) : ''
  if (!anchor || !global) return false
  return anchor !== global
}

/** `2026-10-24` → `10/24`（报表式短日期；缺失时给占位而不是空串）。 */
export function shortDate(date?: string): string {
  return date ? date.slice(0, 10).slice(5).replace('-', '/') : '日期未提供'
}

/** 以 anchor 为终点、向前 days 天的日期轴（UTC 计算避免本地时区跨日漂移）。 */
export function miniAxis(anchor: string, days: number): string[] {
  const end = new Date(`${anchor}T00:00:00Z`)
  return Array.from({ length: days }, (_, i) => {
    const value = new Date(end)
    value.setUTCDate(value.getUTCDate() - (days - 1 - i))
    return value.toISOString().slice(0, 10)
  })
}
