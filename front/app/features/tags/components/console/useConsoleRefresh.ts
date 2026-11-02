/**
 * 控制台显式刷新/生成完成刷新（矩阵阶段 G5，R28；8C G4a 刷新后回调）。
 *
 * 顺序固定：先独立 report 元信息（0 active 也更新头部/源轴锚），再 A 档；
 * 串行防重，避免按钮连点/完成事件并发刷新。元信息失败保留上次成功值由
 * useConsoleShellData 的 stale 状态承担。
 *
 * 8C G4a：可选 `after` 在同一防重窗口内、元信息/A 之后执行（壳层用于强刷报日索引、
 * 当前阅读与观察依赖）。`after` 抛错只影响附属刷新，不改变主刷新结果（矩阵仍可读）。
 */
import { computed, ref, type Ref } from 'vue'

export interface ConsoleRefreshDeps {
  loadReportMeta: () => Promise<void>
  loadSource: () => Promise<void>
  reportMetaLoading: Ref<boolean>
  /** 附属刷新（索弔/当前阅读/观察）；在同一 refreshing 窗口内排除并发重复。 */
  after?: () => Promise<void>
}

export function useConsoleRefresh(deps: ConsoleRefreshDeps) {
  const refreshing = ref(false)
  const busy = computed(() => refreshing.value || deps.reportMetaLoading.value)

  async function refresh(): Promise<void> {
    if (refreshing.value) return
    refreshing.value = true
    try {
      await deps.loadReportMeta()
      await deps.loadSource()
      if (deps.after) {
        try {
          await deps.after()
        } catch {
          // 附属刷新失败不改主刷新语义（索引陈旧由 indexError 自述，不阻塞矩阵/编年）。
        }
      }
    } finally {
      refreshing.value = false
    }
  }

  return { refreshing, busy, refresh }
}
