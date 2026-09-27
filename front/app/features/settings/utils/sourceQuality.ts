/**
 * 来源质量（add-source-board-hit-rate）前端纯逻辑：
 * pill 分档、百分比文案、分类内排序比较器。
 *
 * 阈值常量的唯一权威是 delta spec
 * openspec/specs/feed-settings-ui/spec.md
 * （「订阅源列表窗口选择与排序筛选」/「订阅源列表展示来源质量指标」）：
 * - 低命中线 30%（「只看低命中」= 率 < 30% 且 ≥5 篇且打标开启）
 * - 最小样本 5 篇（< 5 篇不给百分比，避免小样本误判）
 * 后端只给事实数字，「低命中」判定完全在前端（design D5）。
 */

import type { FeedBoardHitStats } from '~/types'

/** 低命中阈值：入板块率 < 30% 记为低（30% 本身取中档）。 */
export const LOW_HIT_RATE_THRESHOLD = 0.3
/** 高命中线：入板块率 ≥ 60% 记为高。 */
export const HIGH_HIT_RATE_THRESHOLD = 0.6
/** 最小样本量：窗口内文章数 < 5 不展示百分比。 */
export const MIN_SAMPLE_ARTICLES = 5

/** 统计窗口允许值（后端白名单 {7,30,90}，缺省 7）。 */
export const STATS_WINDOWS = [7, 30, 90] as const
export type StatsWindowDays = (typeof STATS_WINDOWS)[number]

/** 列表排序选项（在既有分类分组内排序，不打乱分类结构）。 */
export const FEED_SORT_MODES = ['default', 'hit_rate_asc', 'articles_desc', 'noise_desc'] as const
export type FeedSortMode = (typeof FEED_SORT_MODES)[number]

/**
 * pill 形态机（State Matrix）：
 * - pending   统计加载中 / 无数据 / 聚合失败 → 「—」固定宽占位
 * - off       tagging_enabled=false → 灰底「打标关闭」（不显示 0%）
 * - empty     窗口内 0 篇 → 「无新文」
 * - low-sample 窗口内 < 5 篇 → 「样本少」（不猜百分比）
 * - high/mid/low 三档配色百分比
 */
export type SourceQualityPillState =
  | 'pending'
  | 'off'
  | 'empty'
  | 'low-sample'
  | 'high'
  | 'mid'
  | 'low'

export type SourceQualityPillInput = {
  articles?: number
  hit_rate?: number
  tagging_enabled?: boolean
}

export function resolvePillState(
  input: SourceQualityPillInput | undefined,
  loading = false,
): SourceQualityPillState {
  if (loading || !input || input.articles === undefined) return 'pending'
  if (input.tagging_enabled === false) return 'off'
  if (input.articles === 0) return 'empty'
  if (input.articles < MIN_SAMPLE_ARTICLES) return 'low-sample'
  const rate = input.hit_rate ?? 0
  if (rate >= HIGH_HIT_RATE_THRESHOLD) return 'high'
  if (rate >= LOW_HIT_RATE_THRESHOLD) return 'mid'
  return 'low'
}

export function pillStateLabel(state: SourceQualityPillState, hitRate?: number): string {
  switch (state) {
    case 'pending':
      return '—'
    case 'off':
      return '打标关闭'
    case 'empty':
      return '无新文'
    case 'low-sample':
      return '样本少'
    case 'high':
    case 'mid':
    case 'low':
      return formatPercent(hitRate ?? 0)
  }
}

/** 列表 pill 百分比：四舍五入取整（26.29% → 26%，83.58% → 84%）。 */
export function formatPercent(rate: number): string {
  return `${Math.round(rate * 100)}%`
}

/** 详情块率文案：一位小数（437/1662 → 26.3%）。 */
export function formatRateOneDecimal(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

/** 杂音量 = 窗口内文章数 − 命中数（spec 排序口径）。 */
export function noiseVolume(stats: FeedBoardHitStats | undefined): number {
  if (!stats) return 0
  return stats.articles - stats.in_board
}

function titleOf(feed: { title: string }): string {
  return feed.title ?? ''
}

/**
 * 分类内排序比较器（不打乱分类结构；稳定起见同值按标题）。
 * - default：分类内标题（zh locale）
 * - hit_rate_asc：率升序；无样本源（0 篇）排在样本充足项之后（spec 硬性要求）
 * - articles_desc：窗口篇数降序
 * - noise_desc：杂音量（articles − in_board）降序
 */
export function compareFeedsBySortMode(
  a: { title: string },
  b: { title: string },
  statsA: FeedBoardHitStats | undefined,
  statsB: FeedBoardHitStats | undefined,
  mode: FeedSortMode,
): number {
  const byTitle = titleOf(a).localeCompare(titleOf(b), 'zh-Hans-CN')
  switch (mode) {
    case 'default':
      return byTitle
    case 'hit_rate_asc': {
      const aEmpty = (statsA?.articles ?? 0) === 0
      const bEmpty = (statsB?.articles ?? 0) === 0
      if (aEmpty !== bEmpty) return aEmpty ? 1 : -1
      const d = (statsA?.hit_rate ?? 0) - (statsB?.hit_rate ?? 0)
      return d !== 0 ? d : byTitle
    }
    case 'articles_desc': {
      const d = (statsB?.articles ?? 0) - (statsA?.articles ?? 0)
      return d !== 0 ? d : byTitle
    }
    case 'noise_desc': {
      const d = noiseVolume(statsB) - noiseVolume(statsA)
      return d !== 0 ? d : byTitle
    }
  }
}

/**
 * 「只看低命中」筛选（spec：窗口内 ≥5 篇 且 率 <30% 且打标开启）。
 * tagging_enabled=false 的源是配置结果而非内容质量，不参与该筛选；
 * 无统计数据的源在统计未到时保守排除（不误判为杂音）。
 */
export function isLowHitNoise(stats: FeedBoardHitStats | undefined): boolean {
  if (!stats) return false
  if (!stats.tagging_enabled) return false
  if (stats.articles < MIN_SAMPLE_ARTICLES) return false
  return stats.hit_rate < LOW_HIT_RATE_THRESHOLD
}
