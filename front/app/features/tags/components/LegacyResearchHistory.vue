<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import { useBoardEnrichmentApi } from '~/api/boardEnrichment'
import type {
  BoardAnalysisResultRow,
  LegacyHistoryRow,
  ResultDetailRow,
  ReviewRow,
  StockDebateResult,
  TopicEnrichmentQA,
} from '~/api/boardEnrichment'
import BoardBriefReport from './BoardBriefReport.vue'
import BoardInvestigationReport from './BoardInvestigationReport.vue'
import BoardAnalysisReport from './BoardAnalysisReport.vue'
import CausalAnalysisReport from './CausalAnalysisReport.vue'
import DebateSection from './DebateSection.vue'
import { renderMarkdown } from '~/utils/markdown'

/**
 * 旧研究历史只读外壳（redesign-enrichment-research-workbench 5.2 / D7）。
 *
 * 版块维度的旧产物清单：board_brief / board_investigation /
 * legacy_board_analysis / topic_analysis 四类旧 kind（含已归档话题），
 * id 排他游标分页；选中后复用既有单结果详情接口拉正文，按 result_kind
 * 分发到既有 reader 纯展示。设计纪律（spec「旧研究历史只读访问」）：
 *  - 读取零 LLM：不调任何触发/追问/沉淀/采纳 API，无生成/追问/沉淀按钮；
 *  - reader 自带的「深入调查/发现关联/自填问题/跨版块跳转」生成入口
 *    一律隐藏（内联样式兑底 + 样式表双保险；本组件不监听对应事件，
 *    即使漏藏点击也无效果）；
 *  - QAPanel 无只读模式（内置 ask/sediment 控件），追问历史在本外壳内
 *    静态展示（与 QAPanel 同款 renderMarkdown 管线）；
 *  - 列表与详情分开加载、分开错误态、独立可重试；
 *  - 单条坏载荷只降级该条（「原始记录不可解析」+ 原始记录只读入口），
 *    列表其余行照常；
 *  - 缺标题用「{kind 中文名} #ID」回退，计数无记录不补 0（空态直说）；
 *  - lane/话题引用点击 emit open-topic（下钻钩子，后续接话题总览，
 *    本组件不做导航）。
 */
const props = defineProps<{
  boardId: number
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  /** lane/话题引用下钻钩子（后续由宿主接话题总览），本组件只上报。 */
  'open-topic': [topicId: number]
}>()

const api = useBoardEnrichmentApi()

// ── kind 中文名（未知/非法 kind 如实显示原始标识，不崩） ─────────────────
const KIND_LABELS: Record<string, string> = {
  board_brief: '简报',
  board_investigation: '调查',
  legacy_board_analysis: '旧版分析',
  topic_analysis: '话题分析',
}
function kindLabel(kind?: string | null): string {
  if (!kind) return '未知类型'
  return KIND_LABELS[kind] ?? kind
}
/** 该行能否走既有详情口：未知 kind、或 topic 行缺归属 id 的脏数据不可打开。 */
function canOpen(row: LegacyHistoryRow): boolean {
  if (!KIND_LABELS[row.result_kind]) return false
  if (row.result_kind === 'topic_analysis' && !row.persistent_topic_id) return false
  return true
}

// ── 列表（独立加载/错误态） ──────────────────────────────────────────────
const PAGE_SIZE = 20
const rows = ref<LegacyHistoryRow[]>([])
const listLoading = ref(false)
const listError = ref<string | null>(null)
const moreLoading = ref(false)
const moreError = ref<string | null>(null)
/** 是否可能还有下一页（后端无总数，按整页填满推断，不凑总数）。 */
const hasMore = ref(false)

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback
}

async function loadList() {
  listLoading.value = true
  listError.value = null
  try {
    const res = await api.listLegacyHistory(props.boardId, { limit: PAGE_SIZE })
    if (!res.success || !res.data) throw new Error(res.error || '加载历史列表失败')
    rows.value = res.data
    hasMore.value = res.data.length === PAGE_SIZE
  } catch (e) {
    listError.value = errText(e, '加载历史列表失败')
  } finally {
    listLoading.value = false
  }
}

/** 「加载更多」：按最后一条 id 作排他游标追加，不重置已有行。 */
async function loadMore() {
  const last = rows.value[rows.value.length - 1]
  if (!last || moreLoading.value) return
  moreLoading.value = true
  moreError.value = null
  try {
    const res = await api.listLegacyHistory(props.boardId, {
      limit: PAGE_SIZE,
      beforeId: last.id,
    })
    if (!res.success || !res.data) throw new Error(res.error || '加载更多失败')
    rows.value = [...rows.value, ...res.data]
    hasMore.value = res.data.length === PAGE_SIZE
  } catch (e) {
    moreError.value = errText(e, '加载更多失败')
  } finally {
    moreLoading.value = false
  }
}

// ── 详情（独立加载/错误态，按 result_kind 分发既有 reader） ────────────────
const selected = ref<LegacyHistoryRow | null>(null)
const detailLoading = ref(false)
const detailError = ref<string | null>(null)
/** board 档详情（三 kind），喂 BoardBriefReport/Investigation/AnalysisReport。 */
const boardResult = ref<BoardAnalysisResultRow | null>(null)
/** topic 档详情，喂 CausalAnalysisReport。 */
const topicResult = ref<ResultDetailRow | null>(null)
/** 单条坏载荷降级：该条显示提示 + 原始记录只读入口，不影响列表其他行。 */
const degraded = ref(false)
const degradedWhy = ref<string | null>(null)
const rawRecord = ref<string | null>(null)

// ── 子产物（只读展示；有则显示、无则不出区块） ─────────────────────────────
const qaList = ref<TopicEnrichmentQA[]>([])
const debates = ref<StockDebateResult[]>([])
const reviews = ref<ReviewRow[]>([])
const subError = ref<string | null>(null)

function resetDetail() {
  selected.value = null
  detailLoading.value = false
  detailError.value = null
  boardResult.value = null
  topicResult.value = null
  degraded.value = false
  degradedWhy.value = null
  rawRecord.value = null
  qaList.value = []
  debates.value = []
  reviews.value = []
  subError.value = null
}

function degrade(why: string, raw: unknown) {
  degraded.value = true
  degradedWhy.value = why
  try {
    rawRecord.value = JSON.stringify(raw, null, 2)
  } catch {
    rawRecord.value = String(raw)
  }
}

/** 追问/辩论/评审子产物：best-effort 并行拉取，失败合并一条可重试提示。 */
async function loadSubProducts(row: LegacyHistoryRow) {
  qaList.value = []
  debates.value = []
  reviews.value = []
  subError.value = null
  const jobs: Array<Promise<void>> = []
  if (row.result_kind === 'topic_analysis' && row.persistent_topic_id) {
    const tid = row.persistent_topic_id
    jobs.push(
      api.listQA(tid, row.id).then((res) => {
        if (res.success && res.data) qaList.value = res.data
      }),
      api.listDebates(tid, row.id).then((res) => {
        if (res.success && res.data) debates.value = res.data
      }),
      api.listReviews(tid).then((res) => {
        if (res.success && res.data) {
          reviews.value = res.data.filter((r) => r.curr_result_id === row.id)
        }
      }),
    )
  } else {
    jobs.push(
      api.listBoardQA(props.boardId, row.id).then((res) => {
        if (res.success && res.data) qaList.value = res.data
      }),
    )
  }
  const settled = await Promise.allSettled(jobs)
  if (settled.some((s) => s.status === 'rejected')) {
    subError.value = '追问/辩论/评审记录加载失败'
  }
}

async function openRow(row: LegacyHistoryRow) {
  resetDetail()
  selected.value = row
  detailLoading.value = true
  detailError.value = null
  try {
    if (!canOpen(row)) {
      degrade('未知类型记录，没有对应的兼容阅读视图', row)
      return
    }
    if (row.result_kind === 'topic_analysis' && row.persistent_topic_id) {
      const res = await api.getResult(row.persistent_topic_id, row.id)
      if (!res.success) throw new Error(res.error || '加载详情失败')
      const data = res.data ?? null
      if (!data || !data.sectors || typeof data.sectors !== 'object') {
        degrade('原始记录不可解析', data ?? res)
        return
      }
      topicResult.value = data
    } else {
      const res = await api.getBoardAnalysisResult(props.boardId, row.id)
      if (!res.success) throw new Error(res.error || '加载详情失败')
      const data = res.data ?? null
      if (!data || !data.sectors || typeof data.sectors !== 'object') {
        degrade('原始记录不可解析', data ?? res)
        return
      }
      boardResult.value = data
    }
  } catch (e) {
    detailError.value = errText(e, '加载详情失败')
    return
  } finally {
    detailLoading.value = false
  }
  await loadSubProducts(row)
}

function retryDetail() {
  if (selected.value) openRow(selected.value)
}

// 详情分发：以详情载荷自身 kind 为准（脏数据行与载荷不一致时忠于内容），
// 回退行 kind。kind 是只读分发，不改写任何载荷。
const detailKind = computed<string | null>(() => {
  if (degraded.value || detailError.value) return null
  return (
    boardResult.value?.result_kind
    ?? (topicResult.value ? 'topic_analysis' : null)
    ?? selected.value?.result_kind
    ?? null
  )
})

// ── 只读纪律（JS 兑底层）：reader 自带的生成/追问/跨版块跳转入口隐藏 ──────
// 双保险之一（另一层是 <style> 里同选择器的 display:none 规则）。用内联样式
// 而非仅样式表：vitest 默认不注入 SFC 样式表，测试锚（isVisible）与生产
// 行为都能稳定命中；组件不监听 investigate/discover-relation/open-board，
// 即使漏藏点击也无效果。
const readerHost = ref<HTMLElement | null>(null)
const WRITE_CTRL_SELECTORS = [
  '.bb-discover-chip',
  '.bb-investigate-btn',
  '.bb-custom',
  '[data-test^="cross-open-board-"]',
  '[data-test^="inv-laneref-open-"]',
  '.bi-lane-cross',
]
function hideWriteControls() {
  const root = readerHost.value
  if (!root) return
  for (const sel of WRITE_CTRL_SELECTORS) {
    for (const el of root.querySelectorAll(sel)) {
      ;(el as HTMLElement).style.display = 'none'
    }
  }
}
watch(
  [boardResult, topicResult, detailLoading],
  () => {
    nextTick().then(hideWriteControls)
  },
)

// ── 泳道引用下钻钩子：三条 reader 的 drill-lane 统一转 open-topic ─────────
// （泳道=话题；后续宿主接话题总览，本组件不导航、不预填、不触发生成）
function onDrillLane(payload: { laneId: number }) {
  emit('open-topic', payload.laneId)
}

// ── 弹层生命周期：打开即重拉列表（旧产物集合小，不值得做陈旧缓存）；
// immediate：宿主以 open=true 初始挂载也要拉
watch(
  () => [props.open, props.boardId] as const,
  ([open]) => {
    if (!open) return
    resetDetail()
    rows.value = []
    hasMore.value = false
    moreError.value = null
    loadList()
  },
  { immediate: true },
)

function fmtDate(d?: string | null): string {
  return d ? d.slice(0, 10) : '—'
}
function shortSession(sid?: string | null): string {
  return sid ? sid.slice(0, 8) : ''
}

const listEmpty = computed(
  () => !listLoading.value && !listError.value && rows.value.length === 0,
)
</script>

<template>
  <AppDialog
    :model-value="open"
    size="xl"
    title="旧版研究历史"
    @update:model-value="emit('update:open', $event)"
  >
    <div class="lh-body" data-test="legacy-history">
      <!-- ── 左：列表（独立加载/错误/空态） ─────────────────────────── -->
      <aside class="lh-list" data-test="legacy-list">
        <div v-if="listLoading" class="lh-state">
          <Icon icon="mdi:loading" width="16" class="lh-spin" /> 正在加载旧版记录…
        </div>
        <div v-else-if="listError" class="lh-state lh-error" data-test="legacy-list-error">
          <p>{{ listError }}</p>
          <button type="button" class="lh-btn" data-test="legacy-list-retry" @click="loadList">
            重试
          </button>
        </div>
        <p v-else-if="listEmpty" class="lh-state" data-test="legacy-empty">
          该版块暂无旧版研究记录。
        </p>
        <template v-else>
          <ul class="lh-rows">
            <li v-for="row in rows" :key="row.id">
              <button
                type="button"
                class="lh-row"
                :class="{ active: selected?.id === row.id }"
                :data-test="`legacy-row-${row.id}`"
                @click="openRow(row)"
              >
                <span class="lh-row-kind">
                  {{ kindLabel(row.result_kind) }} #{{ row.id }}
                  <span v-if="!canOpen(row)" class="lh-row-flag">不可解析</span>
                </span>
                <span class="lh-row-meta">
                  <span>{{ fmtDate(row.created_at) }}</span>
                  <span v-if="shortSession(row.session_id)" class="lh-row-session">{{ shortSession(row.session_id) }}</span>
                  <span
                    v-if="row.persistent_topic_id"
                    class="lh-topic-chip"
                    role="button"
                    tabindex="0"
                    :data-test="`legacy-topic-link-${row.persistent_topic_id}`"
                    title="打开该话题（下钻钩子）"
                    @click.stop="emit('open-topic', row.persistent_topic_id)"
                    @keydown.enter.stop="emit('open-topic', row.persistent_topic_id)"
                  >
                    话题 #{{ row.persistent_topic_id }}
                  </span>
                </span>
              </button>
            </li>
          </ul>
          <div v-if="moreError" class="lh-state lh-error" data-test="legacy-more-error">
            <p>{{ moreError }}</p>
            <button type="button" class="lh-btn" data-test="legacy-more-retry" @click="loadMore">重试</button>
          </div>
          <button
            v-if="hasMore && !moreError"
            type="button"
            class="lh-btn lh-more"
            data-test="legacy-load-more"
            :disabled="moreLoading"
            @click="loadMore"
          >
            <Icon v-if="moreLoading" icon="mdi:loading" width="13" class="lh-spin" />
            {{ moreLoading ? '加载中…' : '加载更多' }}
          </button>
        </template>
      </aside>

      <!-- ── 右：详情（独立加载/错误态，按 kind 分发既有 reader） ─────── -->
      <section class="lh-detail" data-test="legacy-detail">
        <div v-if="!selected" class="lh-state">从左侧选择一条旧版记录查看。</div>
        <template v-else>
          <header class="lh-detail-head">
            <h3 class="lh-detail-title">
              {{ kindLabel(selected.result_kind) }} #{{ selected.id }}
              <span class="lh-badge">旧版 · 只读</span>
            </h3>
            <p class="lh-detail-meta">
              <span>{{ fmtDate(selected.created_at) }}</span>
              <span v-if="shortSession(selected.session_id)">session {{ shortSession(selected.session_id) }}</span>
              <span
                v-if="selected.persistent_topic_id"
                class="lh-topic-chip"
                role="button"
                tabindex="0"
                @click="emit('open-topic', selected.persistent_topic_id)"
                @keydown.enter="emit('open-topic', selected.persistent_topic_id)"
              >
                话题 #{{ selected.persistent_topic_id }}
              </span>
            </p>
          </header>

          <div v-if="detailLoading" class="lh-state">
            <Icon icon="mdi:loading" width="16" class="lh-spin" /> 正在加载详情…
          </div>
          <div v-else-if="detailError" class="lh-state lh-error" data-test="legacy-detail-error">
            <p>{{ detailError }}</p>
            <button type="button" class="lh-btn" data-test="legacy-detail-retry" @click="retryDetail">
              重试
            </button>
          </div>

          <template v-else>
            <!-- 单条坏载荷降级：只影响本条，原始记录仍可只读访问 -->
            <div v-if="degraded" class="lh-degraded" data-test="legacy-degraded">
              <p>原始记录不可解析{{ degradedWhy ? `（${degradedWhy}）` : '' }}。</p>
              <details class="lh-raw" data-test="legacy-raw">
                <summary>原始记录</summary>
                <pre>{{ rawRecord }}</pre>
              </details>
            </div>

            <!-- 主视图：按 result_kind 分发既有 reader（生成入口隐藏，见 script） -->
            <div v-else ref="readerHost" class="lh-reader">
              <BoardBriefReport
                v-if="detailKind === 'board_brief'"
                :result="boardResult"
                @drill-lane="onDrillLane"
              />
              <BoardInvestigationReport
                v-else-if="detailKind === 'board_investigation'"
                :result="boardResult"
                @drill-lane="onDrillLane"
              />
              <BoardAnalysisReport
                v-else-if="detailKind === 'legacy_board_analysis'"
                :result="boardResult"
                @drill-lane="onDrillLane"
              />
              <CausalAnalysisReport
                v-else-if="detailKind === 'topic_analysis'"
                :result="topicResult"
                :topic-label="selected.persistent_topic_id ? `话题 #${selected.persistent_topic_id}` : ''"
              />
            </div>

            <!-- 子产物只读展示：有则显示，无则不出区块 -->
            <p v-if="subError" class="lh-sub-note" data-test="legacy-sub-error">{{ subError }}</p>
            <section v-if="reviews.length" class="lh-sub" data-test="legacy-reviews">
              <h4>旧评审记录（{{ reviews.length }}）</h4>
              <ul>
                <li v-for="r in reviews" :key="r.id" class="lh-review">
                  <p class="lh-review-text">{{ r.deviation_summary }}</p>
                  <p class="lh-review-meta">{{ fmtDate(r.created_at) }}</p>
                </li>
              </ul>
            </section>
            <section v-if="qaList.length" class="lh-sub" data-test="legacy-qa">
              <h4>追问记录（{{ qaList.length }}）</h4>
              <ul>
                <li v-for="qa in qaList" :key="qa.id" class="lh-qa">
                  <p class="lh-qa-q">问：{{ qa.question }}</p>
                  <!-- 与 QAPanel 同款渲染管线（renderMarkdown），只读无追问/沉淀控件 -->
                  <div class="lh-qa-a markdown-body" v-html="renderMarkdown(qa.answer)" />
                  <p class="lh-qa-meta">
                    {{ fmtDate(qa.created_at) }}
                    <span v-if="qa.sedimented" class="lh-badge">已沉淀</span>
                  </p>
                </li>
              </ul>
            </section>
            <section v-if="debates.length" class="lh-sub" data-test="legacy-debates">
              <h4>个股辩论（{{ debates.length }}）</h4>
              <!-- stage 固定 result：DebateSection 只渲染结果视图，无触发/重试按钮 -->
              <DebateSection stage="result" :debates="debates" />
            </section>
          </template>
        </template>
      </section>
    </div>
  </AppDialog>
</template>

<style scoped>
/* 弹层宽度由 AppDialog size="xl"（1040px，92vw 上限）统一约束，此处不另设宽 */
.lh-body {
  display: flex;
  gap: 20px;
  min-height: 320px;
  max-height: 62vh;
}

.lh-list {
  width: 300px;
  flex: none;
  overflow-y: auto;
  border-right: 1px solid var(--color-border-subtle);
  padding-right: 16px;
}

.lh-detail {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
}

.lh-rows {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.lh-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  text-align: left;
  padding: 8px 10px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;
}

.lh-row:hover {
  border-color: var(--color-accent);
}

.lh-row.active {
  border-color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
}

.lh-row-kind {
  font-weight: 600;
  font-size: 13px;
}

.lh-row-flag {
  margin-left: 6px;
  font-size: 11px;
  font-weight: 400;
  color: var(--color-text-muted);
}

.lh-row-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.lh-row-session {
  font-family: var(--font-mono, monospace);
  font-size: 10px;
}

.lh-topic-chip {
  display: inline-flex;
  align-items: center;
  padding: 1px 7px;
  border-radius: 999px;
  border: 1px solid var(--color-border-subtle);
  font-size: 11px;
  cursor: pointer;
  color: inherit;
  background: transparent;
}

.lh-topic-chip:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.lh-state {
  padding: 14px 4px;
  font-size: 13px;
  color: var(--color-text-muted);
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}

.lh-error p {
  margin: 0;
  color: var(--color-error);
}

.lh-btn {
  padding: 4px 12px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: transparent;
  color: inherit;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.lh-btn:hover:not(:disabled) {
  border-color: var(--color-accent);
}

.lh-btn:disabled {
  opacity: 0.6;
  cursor: default;
}

.lh-more {
  margin-top: 10px;
  width: 100%;
}

.lh-detail-head {
  border-bottom: 1px solid var(--color-border-subtle);
  padding-bottom: 10px;
  margin-bottom: 14px;
}

.lh-detail-title {
  margin: 0;
  font-size: 16px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.lh-detail-meta {
  margin: 6px 0 0;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 12px;
  color: var(--color-text-muted);
}

.lh-badge {
  display: inline-flex;
  padding: 1px 8px;
  border-radius: 999px;
  border: 1px solid var(--color-border-subtle);
  font-size: 11px;
  font-weight: 400;
  color: var(--color-text-muted);
}

.lh-degraded p {
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--color-text-muted);
}

.lh-raw summary {
  cursor: pointer;
  font-size: 12px;
  color: var(--color-accent);
}

.lh-raw pre {
  margin: 8px 0 0;
  padding: 10px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  font-size: 11px;
  overflow: auto;
  max-height: 320px;
  white-space: pre-wrap;
  word-break: break-all;
}

.lh-sub {
  margin-top: 20px;
  border-top: 1px dashed var(--color-border-subtle);
  padding-top: 12px;
}

.lh-sub h4 {
  margin: 0 0 8px;
  font-size: 13px;
}

.lh-sub ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.lh-sub-note {
  margin: 16px 0 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.lh-review-text {
  margin: 0;
  font-size: 13px;
}

.lh-review-meta,
.lh-qa-meta {
  margin: 4px 0 0;
  font-size: 11px;
  color: var(--color-text-muted);
  display: flex;
  gap: 8px;
  align-items: center;
}

.lh-qa-q {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}

.lh-qa-a {
  font-size: 13px;
}

.lh-spin {
  animation: lh-rotate 1s linear infinite;
}

@keyframes lh-rotate {
  to {
    transform: rotate(360deg);
  }
}

/* 窄屏（767.98px，standard/frontend/layout.md 统一断点）：单栏堆叠 */
@media (max-width: 767.98px) {
  .lh-body {
    flex-direction: column;
    max-height: none;
  }

  .lh-list {
    width: auto;
    max-height: 40vh;
    border-right: none;
    border-bottom: 1px solid var(--color-border-subtle);
    padding-right: 0;
    padding-bottom: 12px;
  }
}

/* ── 只读纪律：reader 自带的生成/追问/跨版块跳转入口一律隐藏（样式表层）──
 * 与 script 里 hideWriteControls 的内联样式双保险；本组件同时不监听
 * investigate / discover-relation / open-board 事件。保留 lane 下钻 chip
 * （drill-lane → open-topic 只读钩子）。 */
.lh-reader :deep(.bb-discover-chip),
.lh-reader :deep(.bb-investigate-btn),
.lh-reader :deep(.bb-custom),
.lh-reader :deep([data-test^='cross-open-board-']),
.lh-reader :deep([data-test^='inv-laneref-open-']),
.lh-reader :deep(.bi-lane-cross) {
  display: none !important;
}
</style>
