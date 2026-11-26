/**
 * 归档时间曲线的按日计数数据源（archive-time-chart-range）。
 *
 * 只读：一次请求拿整段历史（版块命中文章按自然日计数），供归档主图与全历史导航条
 * 使用。口径固定，**不随归档筛选（来源 / 构成标签 / 方向不符 / 日报状态 / 排序）变化**
 * ——筛选只作用于下方列表，曲线口径不随之重拉。
 *
 * 缓存纪律（树莓派上版块历史可能上千天，每次进归档都全表聚合不可接受）：
 *  - 模块级 `Map<boardId, {payload, at}>` 缓存，TTL 60 秒；切视图重挂载命中缓存不发请求；
 *  - stale-while-revalidate：过期时先呈现旧值再刷新，不闪空；
 *  - 刷新失败**不覆盖旧值**（与雷达「刷新失败保留旧值」纪律一致）。
 */
import { computed, ref, watch, type Ref } from 'vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import type { BoardArticleDailyCounts } from '~/api/semanticBoards'
import { toCountsMap, type ArchiveCounts } from '../components/console/archiveTimeSeries'

/** 模块级缓存 TTL（毫秒）。 */
const CACHE_TTL_MS = 60_000

interface CacheEntry {
  payload: BoardArticleDailyCounts
  at: number
}

/** 模块级缓存（boardId → 载荷 + 取回时刻），跨挂载复用。 */
const cache = new Map<number, CacheEntry>()

/** 计数读模型（稀疏 → 查表；缺日不在表里，取值时按已知 0 处理）。 */
export type ArchiveCountsState = ArchiveCounts

function emptyState(): ArchiveCountsState {
  return { earliest: null, latest: null, byDate: new Map() }
}


export function useArchiveDailyCounts(boardId: Ref<number>) {
  const api = useSemanticBoardsApi()
  const counts = ref<ArchiveCountsState>(emptyState())
  const loading = ref(false)
  const error = ref<string | null>(null)
  let requestSeq = 0

  async function load(force = false): Promise<void> {
    const id = boardId.value
    const seq = ++requestSeq
    const cached = cache.get(id)
    const fresh = !!cached && Date.now() - cached.at < CACHE_TTL_MS
    if (fresh && !force) {
      counts.value = toCountsMap(cached.payload)
      loading.value = false
      error.value = null
      return
    }
    // 先呈现缓存值（stale-while-revalidate）；无缓存才显示加载态。
    counts.value = cached ? toCountsMap(cached.payload) : emptyState()
    loading.value = !cached
    error.value = null
    try {
      const res = await api.getBoardArticleDailyCounts(id)
      if (seq !== requestSeq) return
      if (!res?.success) {
        error.value = res?.error || '时间曲线加载失败'
        return
      }
      const payload = res.data
      if (!payload) {
        error.value = '时间曲线加载失败'
        return
      }
      cache.set(id, { payload, at: Date.now() })
      counts.value = toCountsMap(payload)
    }
    catch {
      if (seq !== requestSeq) return
      // 失败不覆盖旧值：已有曲线继续显示，只提示可重试。
      error.value = '时间曲线加载失败'
    }
    finally {
      if (seq === requestSeq) loading.value = false
    }
  }

  // 只跟 boardId：筛选 / 排序 / 区间变化 MUST NOT 触发重拉（曲线口径固定）。
  watch(boardId, () => {
    counts.value = emptyState()
    error.value = null
    void load()
  }, { immediate: true })

  return {
    earliest: computed(() => counts.value.earliest),
    latest: computed(() => counts.value.latest),
    byDate: computed(() => counts.value.byDate),
    loading,
    error,
    /** 强制重拉（曲线加载失败后的「重试」入口）。 */
    reload: () => load(true),
  }
}
