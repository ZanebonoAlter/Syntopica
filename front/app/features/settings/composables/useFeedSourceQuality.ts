/**
 * useFeedSourceQuality — 订阅源来源质量聚合状态（add-source-board-hit-rate §3.3）。
 *
 * 由 SettingsSectionFeeds.vue 实例化一次并向下传 props（design D5：窗口状态提升，
 * 列表工具栏与详情块共享同一状态，spec 要求「任一处切换两处同步」）。
 * 不引入新 store；统计失败只影响 pill/详情块，不阻塞列表与设置表单。
 */

import { ref } from 'vue'
import { useFeedsApi } from '~/api/feeds'
import type { FeedBoardHitStats } from '~/types'
import type { StatsWindowDays } from '../utils/sourceQuality'

export function useFeedSourceQuality() {
  const feedsApi = useFeedsApi()

  /** 统计窗口（默认 7 天）；列表工具栏与详情块共用。 */
  const windowDays = ref<StatsWindowDays>(7)
  /** 按 feed_id（字符串键，前端 RssFeed.id 为 string）索引的统计。 */
  const statsByFeed = ref<Record<string, FeedBoardHitStats>>({})
  const loading = ref(false)
  const error = ref<string | null>(null)

  /** 竞态防护：窗口快速连切时旧请求结果不得覆盖新窗口数据。 */
  let requestSeq = 0

  async function load(): Promise<void> {
    const seq = ++requestSeq
    loading.value = true
    error.value = null
    const res = await feedsApi.getBoardHitStats(windowDays.value)
    if (seq !== requestSeq) return // 过期请求：放弃（新请求已接管 loading/error）
    if (res.success && res.data) {
      const map: Record<string, FeedBoardHitStats> = {}
      for (const item of res.data.items) {
        map[String(item.feed_id)] = item
      }
      statsByFeed.value = map
    } else {
      error.value = res.error || '统计加载失败'
    }
    loading.value = false
  }

  /** 切窗口：立即按新窗口重取（骨架/「—」占位，不阻塞其它内容）。 */
  function setWindow(days: StatsWindowDays): void {
    if (days === windowDays.value) return
    windowDays.value = days
    void load()
  }

  /** 块内重试：走同一端点，不改变窗口与排序状态（State Matrix 错误恢复路径）。 */
  function retry(): void {
    void load()
  }

  return {
    windowDays,
    statsByFeed,
    loading,
    error,
    load,
    setWindow,
    retry,
  }
}
