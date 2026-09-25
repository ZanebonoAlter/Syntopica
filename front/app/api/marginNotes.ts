import { apiClient } from './client'
import type { ApiResponse } from '~/types'

/**
 * 日报页边注 API（daily-report-margin-notes，design D3 四+一端点）：
 * - GET  /daily-reports/:reportId/annotations   按报告拉批注（含问答轮与术语，一次拉齐）
 * - POST /daily-reports/:reportId/annotations   落锚
 * - POST /annotations/:id/questions             提问/追问（同步返回回答+引用+术语）
 * - DELETE /annotations/:id                     删除（连带问答，服务端直删）
 * - GET  /annotations                           跨报告列表（管理页，board/q/分页，日期倒序）
 *
 * id 与 snake_case 沿用 daily-report 域 API 约定（同 dailyReports.ts）：
 * section_id / thread_id / cited_article_ids 与 DailyReportSection / DailyReportThread
 * 的数字 id 同源，供 ensureArticles 链路直接消费。
 */

/** 单个抽取术语（后端归一化后透出；is_new = 本轮首次入库，供「入库」角标）。 */
export interface MarginNoteTerm {
  term: string
  is_new?: boolean
}

/** 后端 QA 载荷：terms 字段（3.3 契约）容 extract_terms 旧名；条目可 string 或对象。 */
interface QaTermsPayload {
  terms?: Array<string | MarginNoteTerm> | null
  extracted_terms?: Array<string | MarginNoteTerm> | null
}

function normalizeTerms(payload: QaTermsPayload): MarginNoteTerm[] {
  const raw = payload.terms ?? payload.extracted_terms ?? []
  return raw
    .map(item => (typeof item === 'string' ? { term: item } : item))
    .filter(item => typeof item?.term === 'string' && item.term.trim().length > 0)
}

function normalizeQa(payload: MarginNoteQa & QaTermsPayload): MarginNoteQa {
  return {
    ...payload,
    cited_article_ids: Array.isArray(payload.cited_article_ids) ? payload.cited_article_ids : [],
    cited_web_sources: normalizeWebSources(payload.cited_web_sources),
    extracted_terms: normalizeTerms(payload),
  }
}

/** D7 网络来源归一化：非数组/缺字段防御（旧轮次无该字段 → []）。 */
function normalizeWebSources(raw: unknown): MarginNoteWebSource[] {
  if (!Array.isArray(raw)) return []
  const out: MarginNoteWebSource[] = []
  for (const item of raw) {
    if (typeof item !== 'object' || item === null) continue
    const rec = item as Record<string, unknown>
    if (typeof rec.url !== 'string' || rec.url.length === 0) continue
    out.push({ title: typeof rec.title === 'string' ? rec.title : '', url: rec.url })
  }
  return out
}

/** 一轮问答。cited_article_ids / cited_web_sources 为数组契约：空写 [] 不写 null
 *  （与 thread 引用数组同契约）；cited_web_sources 为 D7 联网白名单来源（[{title,url}]）。 */
export interface MarginNoteWebSource {
  title: string
  url: string
}

export interface MarginNoteQa {
  id: number
  annotation_id: number
  question: string
  answer: string
  cited_article_ids: number[]
  cited_web_sources: MarginNoteWebSource[]
  extracted_terms: MarginNoteTerm[]
  /** 审计冗余（provider/model），离线核对用；前端仅透传展示。 */
  provider?: string
  model?: string
  created_at: string
}

/** 一条批注锚点（可独立于问答存在）。thread_id 为 null = 头条 lead 摘要批注（PR-4）。 */
export interface MarginNoteAnnotation {
  id: number
  report_id: number
  section_id: number
  thread_id: number | null
  quoted_text: string
  anchor_offset_start: number | null
  anchor_offset_end: number | null
  created_at: string
  qas: MarginNoteQa[]
}

/** 落锚请求体（偏移为线索值，失配时由前端模糊匹配兜底）。 */
export interface AnchorMarginNoteParams {
  section_id: number
  thread_id: number | null
  quoted_text: string
  anchor_offset_start: number | null
  anchor_offset_end: number | null
}

/** 提问响应：回答 + 引用 + 术语一次返回（D4 单次 LLM 调用）。 */
export interface MarginNoteQaResult {
  qa: MarginNoteQa
}

/** 跨报告列表行：批注 + 所属报告装饰字段（管理页展示版块/日期/期号）。 */
export interface MarginNoteAdminRow extends MarginNoteAnnotation {
  board_id?: number
  board_label?: string
  period_date?: string
  /** 期号文案（如「第 128 期」），后端按报告序派生；P1 可为空。 */
  issue_number?: string
  /** 该批注跨问答轮去重后的术语（管理页 chips；与批准原型 x.terms 同口径）。 */
  terms?: string[]
}

export interface MarginNoteListResult {
  annotations: MarginNoteAnnotation[]
}

export interface MarginNoteAdminListResult {
  annotations: MarginNoteAdminRow[]
  total: number
}

/** 引用文章 ID 反查豁免 archived 过滤与 thread 引用同口径（后端保证）；前端经 ensureArticles 打开。 */
export function useMarginNotesApi() {
  /** 按报告拉批注（含问答轮，一次拉齐）。 */
  async function getReportAnnotations(reportId: number): Promise<ApiResponse<MarginNoteListResult>> {
    const response = await apiClient.get<{ annotations?: Array<MarginNoteAnnotation & { qas: Array<MarginNoteQa & QaTermsPayload> }> }>(`/daily-reports/${reportId}/annotations`)
    if (!response.success || !response.data) return response as ApiResponse<MarginNoteListResult>
    return {
      ...response,
      data: {
        annotations: (response.data.annotations ?? []).map(annotation => ({
          ...annotation,
          qas: (annotation.qas ?? []).map(normalizeQa),
        })),
      },
    }
  }

  /** 落锚：quoted_text 非空由调用方保证（前端 ≥2 字符，后端 400 兜底）。
   *  POST 返回的是刚建的行，qa 关联未装载（GORM 零值序列化为 null）——与 GET 同口径
   *  归一化：qas 缺失/null 一律折成 []，否则卡片渲染读 note.qas.length 直接抛错。 */
  async function anchorAnnotation(reportId: number, params: AnchorMarginNoteParams): Promise<ApiResponse<{ annotation: MarginNoteAnnotation }>> {
    const response = await apiClient.post<{ annotation: MarginNoteAnnotation & { qas?: Array<MarginNoteQa & QaTermsPayload> | null } }>(`/daily-reports/${reportId}/annotations`, params)
    if (!response.success || !response.data?.annotation) return response as ApiResponse<{ annotation: MarginNoteAnnotation }>
    const annotation = response.data.annotation
    return {
      ...response,
      data: {
        annotation: {
          ...annotation,
          thread_id: annotation.thread_id ?? null,
          qas: (annotation.qas ?? []).map(normalizeQa),
        },
      },
    }
  }

  /** 提问/追问同端点；失败行内重试不丢问题文本（状态机在 useMarginNotes）。
   *  契约防护：响应缺 qa 载荷（如后端形状漂移）时归一为失败响应，走行内错误 +
   *  重试路径；否则 normalizeQa(undefined) 抛错，卡片会永远卡在 pending。 */
  async function askAnnotation(annotationId: number, question: string): Promise<ApiResponse<MarginNoteQaResult>> {
    const response = await apiClient.post<{ qa: MarginNoteQa & QaTermsPayload }>(`/annotations/${annotationId}/questions`, { question })
    if (!response.success) return response as ApiResponse<MarginNoteQaResult>
    const rawQa = response.data?.qa
    if (!rawQa) {
      return { ...response, success: false, data: undefined, error: response.error || '回答返回格式异常' } as ApiResponse<MarginNoteQaResult>
    }
    return { ...response, data: { qa: normalizeQa(rawQa) } }
  }

  /** 删除批注（连带全部问答）。确认在前端（AppDialog sm），服务端直接删。 */
  async function deleteAnnotation(annotationId: number): Promise<ApiResponse<null>> {
    return apiClient.delete(`/annotations/${annotationId}`)
  }

  /** 跨报告列表（管理页）：board_id / q（命中划词/提问/术语）/ 分页，按日期倒序。
   *  与按报告拉取同口径归一化 qas（extracted_terms 库里是纯字符串数组，卡片/管理页
   *  按 {term, is_new} 渲染——不折叠会渲染出 9 个空 chip）。 */
  async function listAnnotations(params?: { board_id?: number; q?: string; page_size?: number; page?: number }): Promise<ApiResponse<MarginNoteAdminListResult>> {
    const query = params ? apiClient.buildQueryParams(params) : ''
    const response = await apiClient.get<{ annotations?: Array<MarginNoteAdminRow & { qas: Array<MarginNoteQa & QaTermsPayload> }>, total?: number }>(`/annotations${query ? `?${query}` : ''}`)
    if (!response.success || !response.data) return response as ApiResponse<MarginNoteAdminListResult>
    const annotations = (response.data.annotations ?? []).map(row => ({
      ...row,
      qas: (row.qas ?? []).map(normalizeQa),
    }))
    return {
      ...response,
      data: {
        annotations,
        total: response.data.total ?? annotations.length,
      },
    }
  }

  return {
    getReportAnnotations,
    anchorAnnotation,
    askAnnotation,
    deleteAnnotation,
    listAnnotations,
  }
}
