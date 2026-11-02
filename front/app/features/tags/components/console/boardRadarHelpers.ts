/**
 * 雷达总览纯函数（rework-workshop-into-radar-and-desk 8A.1 + R-18 归一化读模型）。
 * 关注分档（≥50 热点 / ≥15 关注 / 其余常温）、3 日环比文案、数据延迟判定、
 * 14 日小矩阵日期轴与行内四档分档在此唯一实现；输入为 API 边界归一后的
 * camelCase/字符串 ID 读模型（RadarBoard/RadarLane），展示组件只消费结果，
 * 不再各自复制口径（分档与环比语义见 ui-design.md「权威数据与口径」）。
 */
import type { RadarBoard } from '~/api/normalizers/boardRadar'

export const RADAR_HOT_ATTENTION = 50
export const RADAR_WARM_ATTENTION = 15

export type RadarTier = '热点' | '关注' | '常温'

/** 关注强度分档标签：≥50 热点、≥15 关注、其余常温。 */
export function tierLabel(attention: number): RadarTier {
  if (attention >= RADAR_HOT_ATTENTION) return '热点'
  if (attention >= RADAR_WARM_ATTENTION) return '关注'
  return '常温'
}

export interface RadarTrend {
  text: string
  dir: 'up' | 'down' | 'flat'
}

/** 近 3 日 / 前 3 日环比文案；前 3 日为 0 时区分「新出现」与「—」。 */
export function trendOf(row: Pick<RadarBoard, 'last3Sections' | 'prev3Sections'>): RadarTrend {
  if (!row.prev3Sections) {
    return row.last3Sections
      ? { text: '新出现', dir: 'up' }
      : { text: '—', dir: 'flat' }
  }
  const change = Math.round((row.last3Sections / row.prev3Sections - 1) * 100)
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
