import { ref } from 'vue'
import { useDailyReportsApi, type ThreadRelationsResponse } from '~/api/dailyReports'
import type { RequestCacheEntry } from '~/features/tags/components/daily-report/dailyReportMagazine'

// ── 线索相关推荐共享缓存（replace-lane-lifeline-with-topic-events D7）─────────
//
// 模块级单例：整个阅读台共用一份推荐缓存，键是 `${reportId}:${days}`——端点一次
// 返回**整份日报**所有线索的推荐（不为每条线索各发一次请求），所以缓存粒度是
// 「报告 × 窗口」而不是「线索」。
//
// 语义：
// - `ensure` 幂等：成功过的键直接命中缓存（切回 7 天档不重取）；同键并发只发一次；
//   错误态的键再 `ensure` 会自动重发，需要绕过已成功的缓存时传 `force`；
// - 失败不阻断阅读：错误态留在缓存里给调用方决定（chip 行整行不渲染），再次
//   `ensure`（重开卡片 / 切档回来）会重新请求；
// - 迟到响应只写自己的键：切档/切报告不会把旧窗口的结果串到新键上。

function cacheKey(reportId: number, days: number): string {
  return `${reportId}:${days}`
}

const entries = ref(new Map<string, RequestCacheEntry<ThreadRelationsResponse>>())
/** 在途请求（同键并发去重；结算后删除，不参与 UI 状态）。 */
const pending = new Map<string, Promise<void>>()

const IDLE: RequestCacheEntry<ThreadRelationsResponse> = { status: 'idle' }

export function useThreadRelatedEvents() {
  const { getThreadRelations } = useDailyReportsApi()

  /** 只读缓存条目：未请求过 → idle（调用方据此决定是否发取数信号）。 */
  function entry(reportId: number, days: number): RequestCacheEntry<ThreadRelationsResponse> {
    return entries.value.get(cacheKey(reportId, days)) ?? IDLE
  }

  function publish(key: string, next: RequestCacheEntry<ThreadRelationsResponse>) {
    entries.value.set(key, next)
    entries.value = new Map(entries.value)
  }

  /**
   * 拉取某报告某窗口的推荐（幂等）。
   *
   * @param force 强制重发（绕过已成功的缓存；错误态的键本就会重发，无需 force）。
   */
  async function ensure(reportId: number, days: number, force = false): Promise<void> {
    const key = cacheKey(reportId, days)
    const current = entries.value.get(key)
    if (!force && current?.status === 'success') return
    const inflight = pending.get(key)
    if (inflight) return inflight

    publish(key, { status: 'loading', data: current?.data })
    const request = Promise.resolve(getThreadRelations(reportId, { days }))
      .then((response) => {
        // 迟到/键守卫：请求期间该键被清空（切版块 resetLocal）时丢弃结果。
        if (!entries.value.has(key)) return
        if (response?.success && response.data) {
          publish(key, { status: 'success', data: response.data })
          return
        }
        publish(key, { status: 'error', error: response?.error || '相关线索加载失败' })
      })
      .catch(() => {
        if (!entries.value.has(key)) return
        publish(key, { status: 'error', error: '相关线索加载失败' })
      })
      .finally(() => { pending.delete(key) })

    pending.set(key, request)
    return request
  }

  /** 版块切换/隔离场景：清空本地缓存（后续 ensure 重新拉取，保证新鲜度）。
   *  消费者：DailyReportTopicSection 在报告 id 变化时调用。 */
  function resetLocal() {
    entries.value = new Map()
    pending.clear()
  }

  return { entries, entry, ensure, resetLocal }
}
