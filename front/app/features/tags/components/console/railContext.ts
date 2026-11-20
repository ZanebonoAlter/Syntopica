/**
 * 本板块上下文 / 单级宽工作面共享类型（rework-workshop-into-radar-and-desk 矩阵阶段 G5）。
 *
 * 右轨与单级宽工作面读的是同一份真实信号（壳层只取一次）：
 *  - 健康（useBoardHealth）、话题追踪（useTopicWatchesApi.listWatches，纯板块作用域）、
 *    话题候选数（lane-dynamics 返回候选）、A 名单 ID（仅供 watch 定位泳道）。
 * 分组后由壳层打包成 `RailContext` 透出，避免两个消费方各自复制一套 props 语义。
 */
import type { TopicWatch } from '~/api/topicWatches'
import type { RailHealthSnapshotView } from '~/api/normalizers/consoleRail'
import type { GovernanceTab } from './governanceTabs'
import type { NowBriefModel } from './nowBrief'

export interface RailHealthView {
  /** 归一后的 camelCase/字符串 ID 快照（旧管理面板仍用 raw DTO，只在壳层桥接）。 */
  snapshot: RailHealthSnapshotView | null
  loading: boolean
  error: string | null
  /** 刷新失败但保留上次成功快照（不等于无数据）。 */
  stale: boolean
  score: number | null
  tier: 'good' | 'mid' | 'low' | null
  checkedAtText: string
  trendPolyline: string
  trendSummary: string
}

export interface RailWatchView {
  items: TopicWatch[]
  loading: boolean
  error: string | null
}

/** 壳层注入右轨/工作面的一份上下文（只读派生，不含网络状态机）。 */
export interface RailContext {
  boardId: number
  /** A 名单 topicId（仅用于把关注记录的 persistentTopicId 定位到泳道）。 */
  laneIds: string[]
  /** 话题候选条数（lane-dynamics candidates；人工激活入口在编排态）。 */
  topicCandidateCount: number
  health: RailHealthView
  watches: RailWatchView
  /** 「当日日报」视图模型（跟随矩阵选中日；壳层用既有读取链组装，右轨不另发请求）。 */
  now: NowBriefModel
}

/** 单级宽工作面页签：本板块上下文 / 体检依据 + 治理五页签。 */
export type WorkfaceTab = 'context' | 'health' | GovernanceTab

export const WORKFACE_TABS: Array<{ key: WorkfaceTab, label: string }> = [
  { key: 'context', label: '本板块上下文' },
  { key: 'health', label: '体检依据' },
  { key: 'quality', label: '构成' },
  { key: 'composites', label: '组合' },
  { key: 'candidates', label: '候选' },
  { key: 'sources', label: '来源' },
  { key: 'settings', label: '设置' },
]

/** `BoardContextRail` 的受控 props（工作面里同一份上下文全量呈现）。 */
export interface RailBinding {
  boardId: number
  healthSnapshot: RailHealthSnapshotView | null
  healthLoading: boolean
  healthError: string | null
  healthStale: boolean
  healthScore: number | null
  healthTier: 'good' | 'mid' | 'low' | null
  healthCheckedAtText: string
  healthTrendPolyline: string
  healthTrendSummary: string
  watches: TopicWatch[]
  watchesLoading: boolean
  watchesError: string | null
  laneIds: string[]
  topicCandidateCount: number
  /** 「当日日报」视图模型（点评层 + 泳道层）。 */
  now: NowBriefModel
}

export function toRailBinding(context: RailContext): RailBinding {
  return {
    boardId: context.boardId,
    healthSnapshot: context.health.snapshot,
    healthLoading: context.health.loading,
    healthError: context.health.error,
    healthStale: context.health.stale,
    healthScore: context.health.score,
    healthTier: context.health.tier,
    healthCheckedAtText: context.health.checkedAtText,
    healthTrendPolyline: context.health.trendPolyline,
    healthTrendSummary: context.health.trendSummary,
    watches: context.watches.items,
    watchesLoading: context.watches.loading,
    watchesError: context.watches.error,
    laneIds: context.laneIds,
    topicCandidateCount: context.topicCandidateCount,
    now: context.now,
  }
}

/** 页签合法性收敛（URL/旧状态给出未知值时不渲染空白）。 */
export function normalizeWorkfaceTab(value: unknown): WorkfaceTab | null {
  if (typeof value !== 'string') return null
  return WORKFACE_TABS.some(tab => tab.key === value) ? value as WorkfaceTab : null
}
