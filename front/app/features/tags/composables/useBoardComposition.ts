/**
 * 版块构成数据源（rework-workshop-into-radar-and-desk task 3.2）。
 *
 * 治理抽屉的「构成 / 组合 / 候选」页签共用一份构成数据：
 *  - 取数：GET /semantic-boards/:id/composition（构成标签 + 组合挂载）；
 *  - 移除：DELETE /semantic-boards/:boardId/composition/:auxId → 本地重取；
 *  - 竞态：请求序号丢弃旧版块的迟到响应。
 *
 * 旧 `TagsPage` 宿主已随 8.4 退役删除；新壳右轨经 `useBoardCRUD.loadComposition` 取数，
 * 治理抽屉用本 composable（含移除/竞态口径）。
 */
import { ref, watch, type Ref } from 'vue'
import { useSemanticBoardsApi, type AuxiliaryLabelItem, type BoardCompositeMount } from '~/api/semanticBoards'
import { useNotify } from '~/composables/useNotify'

export interface BoardCompositionOptions {
  /** 移除成功后的额外刷新（如体检评分会随构成变化）。 */
  onRemoved?: () => void
}

export function useBoardComposition(boardId: Ref<number>, options: BoardCompositionOptions = {}) {
  const api = useSemanticBoardsApi()
  const { success: notifySuccess, error: notifyError } = useNotify()

  const labels = ref<AuxiliaryLabelItem[]>([])
  const composites = ref<BoardCompositeMount[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const removingId = ref<number | null>(null)

  let requestSeq = 0

  async function load(): Promise<void> {
    const seq = ++requestSeq
    loading.value = true
    error.value = null
    const res = await api.getComposition(boardId.value)
    if (seq !== requestSeq) return
    if (res.success && res.data) {
      labels.value = res.data.items
      composites.value = res.data.composites ?? []
    } else {
      labels.value = []
      composites.value = []
      error.value = res.error || '构成加载失败'
    }
    loading.value = false
  }

  watch(boardId, () => {
    labels.value = []
    composites.value = []
    void load()
  }, { immediate: true })

  /**
   * 移除一个构成标签（既有 DELETE 链）。
   * 返回**是否成功**：调用方据此回填「已摘除」反馈（失败不谎称已摘除）。
   * 通知只在这里发一次（成功/失败各一次），调用方不再叠加 toast。
   */
  async function removeLabel(auxiliaryLabelId: number): Promise<boolean> {
    removingId.value = auxiliaryLabelId
    const res = await api.removeFromComposition(boardId.value, auxiliaryLabelId)
    removingId.value = null
    if (res.success) {
      notifySuccess('已从版块构成移除')
      await load()
      options.onRemoved?.()
      return true
    }
    notifyError(res.error || '移除构成标签失败')
    return false
  }

  return {
    labels,
    composites,
    loading,
    error,
    removingId,
    load,
    removeLabel,
  }
}
