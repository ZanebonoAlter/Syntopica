/**
 * 雷达总览纯函数（rework-workshop-into-radar-and-desk 8A.1 + R-18 归一化读模型）。
 * 关注分档（阈值见下方常量）、3 日热度环比（无 heat 回退旧 sections 口径）、
 * 数据延迟判定、14 日小矩阵日期轴与行内四档分档在此唯一实现；输入为 API 边界归一后的
 * camelCase/字符串 ID 读模型（RadarBoard/RadarLane），展示组件只消费结果，
 * 不再各自复制口径（分档与环比语义见 ui-design.md「权威数据与口径」）。
 */
import type { RadarBoard } from '~/api/normalizers/boardRadar'

// 来源：calibrate-heat --days 14 --board 0；样本：全版块 14d 正值 lane_heat（734 个）。
// P80/P20 四舍五入至一位小数；标定日期：2026-11-06。
export const RADAR_HOT_ATTENTION = 3.3
export const RADAR_WARM_ATTENTION = 0.7

export type RadarTier = '热点' | '关注' | '常温'

/** 关注强度分档标签（lane_heat 量纲）：≥P80 热点、≥P20 关注、其余常温。 */
export function tierLabel(attention: number): RadarTier {
  if (attention >= RADAR_HOT_ATTENTION) return '热点'
  if (attention >= RADAR_WARM_ATTENTION) return '关注'
  return '常温'
}

export interface RadarTrend {
  text: string
  dir: 'up' | 'down' | 'flat'
}

/** API 边界明确区分缺字段与真实 0；未经过 normalizer 的兼容读模型仍按热度信号判定。 */
export function hasHeatSignal(row: Partial<Pick<RadarBoard, 'hasHeatData' | 'todayHeat' | 'last3Heat' | 'prev3Heat'>>): boolean {
  return row.hasHeatData ?? ((row.todayHeat ?? 0) > 0 || (row.last3Heat ?? 0) > 0 || (row.prev3Heat ?? 0) > 0)
}

/** 近 3 日 / 前 3 日环比文案（热度口径优先，无 heat 回退旧 sections）；前 3 日为 0 时区分「新出现」与「—」。 */
export function trendOf(
  row: Pick<RadarBoard, 'last3Heat' | 'prev3Heat' | 'last3Sections' | 'prev3Sections'> & Partial<Pick<RadarBoard, 'hasHeatData' | 'todayHeat'>>,
): RadarTrend {
  const heat = hasHeatSignal(row)
  const last3 = heat ? row.last3Heat : row.last3Sections
  const prev3 = heat ? row.prev3Heat : row.prev3Sections
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

/** 小矩阵行内四档 0–4：0=无分区，其余按该泳道行内最大值分档。 */
export function laneLevel(count: number, max: number): number {
  if (!count || max <= 0) return 0
  return Math.max(1, Math.ceil((count / max) * 4))
}

/**
 * 小矩阵**降级格（代理值）**档位 1–4：基准 = 该泳道**代理值自己**的行内最大值
 * （分区数），不与已确证事件线数共用分母——两种量纲不挤在一条色阶上：
 * 代理值既不稀释已确证格的对比度，也不冒充事件线峰值。
 * 档数仍为 4（与 `.b1..b4` 同构，「按 4 着档」契约不变），但视觉色强封顶在
 * `.b3` 之下（最深实心档只留给已确证的事件线峰值——身份缺失不得画成最热的一天）。
 */
export function laneDegradedLevel(count: number, max: number): number {
  if (!count || max <= 0) return 0
  return Math.max(1, Math.min(4, Math.ceil((count / max) * 4)))
}
