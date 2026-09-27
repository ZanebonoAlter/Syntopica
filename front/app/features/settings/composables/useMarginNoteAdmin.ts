import { ref } from 'vue'
import { useMarginNotesApi, type MarginNoteAdminRow } from '~/api/marginNotes'

/**
 * 页边注管理页数据层（daily-report-margin-notes design D5）：
 * 与 useMarginNotes 同一 API 层（front/app/api/marginNotes.ts），走跨报告列表端点
 * GET /annotations?board_id=&q=&page_size=&page=（日期倒序）。
 * 删除与日报内同源（同一 DELETE /annotations/:id），删除后本地移除行并刷新计数。
 */

export function useMarginNoteAdmin() {
  const { listAnnotations, deleteAnnotation } = useMarginNotesApi()

  const rows = ref<MarginNoteAdminRow[]>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref('')

  async function load(params?: { board_id?: number; q?: string; page_size?: number; page?: number }) {
    loading.value = true
    error.value = ''
    try {
      const response = await listAnnotations(params)
      if (!response.success || !response.data) {
        error.value = response.error || '批注列表加载失败'
        return
      }
      rows.value = response.data.annotations ?? []
      total.value = response.data.total ?? rows.value.length
    } finally {
      loading.value = false
    }
  }

  /** 与日报内删除同源（同一端点）；成功返回 true，调用方移除行 + 计数刷新（MG-6）。 */
  async function remove(annotationId: number): Promise<boolean> {
    const response = await deleteAnnotation(annotationId)
    if (!response.success) return false
    rows.value = rows.value.filter(row => row.id !== annotationId)
    total.value = Math.max(0, total.value - 1)
    return true
  }

  return { rows, total, loading, error, load, remove }
}
