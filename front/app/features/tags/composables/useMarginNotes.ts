import { computed, ref, watch, type Ref } from 'vue'
import { useMarginNotesApi, type AnchorMarginNoteParams, type MarginNoteAnnotation } from '~/api/marginNotes'

/**
 * 页边注数据层（report 范围，daily-report-margin-notes design D5）：
 * - 拉取：按 reportId 批量拉批注（含问答轮）
 * - 落锚：乐观插入（临时 id），失败回滚并保留引用文本供重试
 * - 提问：每批注独立的 pending/error 状态机；失败保留问题文本，重试不丢（AV-1）
 * - 删除：确认在前端组件层，这里只发请求并移除
 */

/** 乐观落锚的临时 id 前缀：负数域避免与后端数字 id 撞车。 */
const TEMP_ID_BASE = -1

export interface MarginNoteAskState {
  /** 进行中的提问文本（骨架行渲染 + 失败重试复用，不丢问题）。 */
  pendingQuestion: string | null
  /** 上一次失败的问题文本与错误（行内错误 + 重试按钮）。 */
  failedQuestion: string | null
  error: string | null
}

export function useMarginNotes(reportId: Ref<number>) {
  const { getReportAnnotations, anchorAnnotation, askAnnotation, deleteAnnotation } = useMarginNotesApi()

  const annotations = ref<MarginNoteAnnotation[]>([])
  const loading = ref(false)
  const loadError = ref('')
  /** 记录已加载的 reportId：切报告后旧数据不串场。 */
  const loadedReportId = ref<number | null>(null)

  /** 每批注的提问状态机（键 = annotation id）。 */
  const askStates = ref(new Map<number, MarginNoteAskState>())
  /** 乐观落锚进行中（同一点击只发一次）。 */
  const anchoring = ref(false)
  const anchorError = ref('')

  function askState(id: number): MarginNoteAskState {
    return askStates.value.get(id) ?? { pendingQuestion: null, failedQuestion: null, error: null }
  }

  function setAskState(id: number, next: MarginNoteAskState) {
    askStates.value.set(id, next)
    askStates.value = new Map(askStates.value)
  }

  async function load(force = false) {
    const id = reportId.value
    if (!id) return
    if (!force && loadedReportId.value === id) return
    loading.value = true
    loadError.value = ''
    try {
      const response = await getReportAnnotations(id)
      // 竞态防护：只接受当前报告的响应
      if (reportId.value !== id) return
      if (!response.success || !response.data) {
        loadError.value = response.error || '批注加载失败'
        return
      }
      annotations.value = response.data.annotations ?? []
      loadedReportId.value = id
      askStates.value = new Map()
    } finally {
      if (reportId.value === id) loading.value = false
    }
  }

  /**
   * 落锚：乐观插入临时批注（含输入聚焦锚点），POST 成功后以服务端行替换；
   * 失败移除临时行并置 anchorError（调用方可持引用文本重试）。
   */
  async function anchor(params: AnchorMarginNoteParams): Promise<MarginNoteAnnotation | null> {
    if (anchoring.value) return null
    anchoring.value = true
    anchorError.value = ''
    const temp: MarginNoteAnnotation = {
      id: TEMP_ID_BASE,
      report_id: reportId.value,
      section_id: params.section_id,
      thread_id: params.thread_id,
      quoted_text: params.quoted_text,
      anchor_offset_start: params.anchor_offset_start,
      anchor_offset_end: params.anchor_offset_end,
      created_at: new Date().toISOString(),
      qas: [],
    }
    annotations.value = [...annotations.value, temp]
    try {
      const response = await anchorAnnotation(reportId.value, params)
      if (!response.success || !response.data?.annotation) {
        anchorError.value = response.error || '落锚失败'
        annotations.value = annotations.value.filter(item => item.id !== temp.id)
        return null
      }
      annotations.value = annotations.value.map(item => (item.id === temp.id ? response.data!.annotation : item))
      return response.data.annotation
    } finally {
      anchoring.value = false
    }
  }

  /**
   * 提问/追问：pending → 成功追加 QA 轮；失败保留问题文本置 error（重试复用）。
   * 返回 true = 成功。
   */
  async function ask(annotationId: number, question: string, retry = false): Promise<boolean> {
    const current = askState(annotationId)
    if (current.pendingQuestion) return false // loading 锁定防重复提交（ID-2）
    // 重试优先复用失败轮的问题文本（不丢问题，AV-1）；空串早退在复用之后判定
    const questionText = retry && current.failedQuestion ? current.failedQuestion : question.trim()
    if (!questionText) return false
    setAskState(annotationId, { pendingQuestion: questionText, failedQuestion: null, error: null })
    const response = await askAnnotation(annotationId, questionText)
    if (!response.success || !response.data?.qa) {
      setAskState(annotationId, { pendingQuestion: null, failedQuestion: questionText, error: response.error || '回答生成失败' })
      return false
    }
    annotations.value = annotations.value.map((item) => {
      if (item.id !== annotationId) return item
      return { ...item, qas: [...item.qas, response.data!.qa] }
    })
    setAskState(annotationId, { pendingQuestion: null, failedQuestion: null, error: null })
    return true
  }

  /** 重试上一次失败的提问（问题文本从 failedQuestion 复用，不丢）。 */
  async function retryAsk(annotationId: number): Promise<boolean> {
    return ask(annotationId, '', true)
  }

  async function remove(annotationId: number): Promise<boolean> {
    const response = await deleteAnnotation(annotationId)
    if (!response.success) return false
    annotations.value = annotations.value.filter(item => item.id !== annotationId)
    askStates.value.delete(annotationId)
    askStates.value = new Map(askStates.value)
    return true
  }

  /** 引用文本定位线索：供正文高亮渲染（偏移 + 归属 id）。 */
  const annotationsByThread = computed(() => {
    const map = new Map<number, MarginNoteAnnotation[]>()
    for (const item of annotations.value) {
      if (item.thread_id == null) continue
      const list = map.get(item.thread_id) ?? []
      list.push(item)
      map.set(item.thread_id, list)
    }
    return map
  })

  watch(reportId, () => {
    // 切报告：重置状态并按新 reportId 重新拉取
    annotations.value = []
    loadedReportId.value = null
    askStates.value = new Map()
    anchorError.value = ''
    void load(true)
  }, { immediate: true })

  return {
    annotations,
    annotationsByThread,
    askStates,
    loading,
    loadError,
    loadedReportId,
    anchoring,
    anchorError,
    load,
    anchor,
    ask,
    retryAsk,
    remove,
    askState,
  }
}
