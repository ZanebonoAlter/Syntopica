/**
 * 版块来源构成数据源（rework-workshop-into-radar-and-desk task 3.1/3.2）。
 *
 * 治理右轨（top 3 占比条）与治理抽屉 / 旧来源面板（全量表格）共用同一套取数、窗口、
 * 排序与占比派生，避免两处口径漂移：
 *  - 取数：GET /semantic-boards/:id/source-breakdown?window=N（只读）；
 *  - 竞态：请求序号丢弃旧版块/旧窗口的迟到响应（切版块时不得覆盖）；
 *  - 派生的 `shareText/initialOf` 只做展示格式化，不改后端口径。
 *
 * 组件只渲染：`BoardSourcePanel`（阶段④退役）与 `BoardGovernanceRail` 共用本 composable。
 */
import { computed, onMounted, ref, watch, type Ref } from 'vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import type { BoardSourceBreakdown, BoardSourceBreakdownSource } from '~/types'
import { STATS_WINDOWS, type StatsWindowDays } from '~/features/settings/utils/sourceQuality'

export type BoardSourceSort = 'articles' | 'hit_rate'

export interface BoardSourcesOptions {
  /** 初始窗口，默认 7 天（与旧面板一致）。 */
  window?: StatsWindowDays
}

export function useBoardSources(boardId: Ref<number>, options: BoardSourcesOptions = {}) {
  const api = useSemanticBoardsApi()

  const windowDays = ref<StatsWindowDays>(options.window ?? 7)
  const sortMode = ref<BoardSourceSort>('articles')
  const breakdown = ref<BoardSourceBreakdown | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  let requestSeq = 0

  async function load(): Promise<void> {
    const seq = ++requestSeq
    loading.value = true
    error.value = null
    const res = await api.getBoardSourceBreakdown(boardId.value, windowDays.value)
    if (seq !== requestSeq) return // 过期请求：放弃（不覆盖新版块/新窗口的数据）
    if (res?.success && res.data) {
      breakdown.value = res.data
    } else {
      error.value = res?.error || '统计加载失败'
    }
    loading.value = false
  }

  watch(boardId, () => { void load() })
  watch(windowDays, () => { void load() })
  onMounted(() => { void load() })

  function retry(): void {
    void load()
  }

  const sortedSources = computed<BoardSourceBreakdownSource[]>(() => {
    const sources = [...(breakdown.value?.sources ?? [])]
    if (sortMode.value === 'hit_rate') {
      sources.sort((a, b) =>
        (a.feed_hit_rate - b.feed_hit_rate) || (b.articles - a.articles),
      )
    } else {
      sources.sort((a, b) => b.articles - a.articles)
    }
    return sources
  })

  /** 右轨用：按当前排序取前 N 个来源。 */
  function topSources(limit: number): BoardSourceBreakdownSource[] {
    return sortedSources.value.slice(0, Math.max(0, limit))
  }

  /** 最大来源及其占比（固定按篇数取，不随排序切换变化）。 */
  const topSource = computed(() => {
    const sources = [...(breakdown.value?.sources ?? [])].sort((a, b) => b.articles - a.articles)
    return sources[0] ?? null
  })

  const totalArticles = computed(() => breakdown.value?.total_articles ?? 0)
  const sourceCount = computed(() => breakdown.value?.source_count ?? 0)

  function shareText(share: number): string {
    const pct = Math.round(share * 100)
    return share > 0 && pct === 0 ? '<1%' : `${pct}%`
  }

  function initialOf(title: string): string {
    return title ? title.slice(0, 1) : '?'
  }

  return {
    STATS_WINDOWS,
    windowDays,
    sortMode,
    breakdown,
    loading,
    error,
    load,
    retry,
    sortedSources,
    topSources,
    topSource,
    totalArticles,
    sourceCount,
    shareText,
    initialOf,
  }
}
