<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useBoardEnrichment } from '~/features/tags/composables/useBoardEnrichment'
import { useSignalWorkbench } from '~/features/tags/composables/useSignalWorkbench'
import type { ContextGranularity, AnalyzeOutput, AnalyzeRef } from '~/api/boardEnrichment'
import { useNotify } from '~/composables/useNotify'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import { renderMarkdown } from '~/utils/markdown'
import { useMermaidRender } from '~/composables/useMermaidRender'
// 全局 .markdown-body 样式（文章阅读器同款），供新闻背景叙事 md 渲染产物使用
import '~/components/article/ArticleContent.css'
import CausalAnalysisReport from './CausalAnalysisReport.vue'
import QAPanel from './QAPanel.vue'
import DebateSection from './DebateSection.vue'
import SignalCandidateList from './SignalCandidateList.vue'
import SignalReportList from './SignalReportList.vue'
import SignalReportView from './SignalReportView.vue'

/**
 * 数据增强 · 信号解读工作台（board-signal-reports 5.2 重排）。
 *
 * 主视图 = 候选信号列表 + 已生成报告列表（手动「发现信号」只保存候选；
 * 逐条点「深入分析」才启动研究；报告生成即可阅读，无评审/采纳链）。
 * 报告阅读走 SignalReportView（AppPageShell reader≤760）。旧简报/调查/
 * 旧版分析视图与数据源绑定管理入口从工作台退役（后端 API 保留兼容）；
 * 聚焦分析折叠区（单泳道下钻 + 泳道选择）与新闻背景（循环A 新闻记忆）
 * 折叠区保留。
 *
 * DebateSection（FinGenius 个股辩论）作为「金融可选模块 · 独立于因果主线」
 * 默认折叠保留。
 */
const props = defineProps<{
  boardId: number
  /** 板块是否开启数据增强（循环 B 总开关，来自 semantic_labels.enrichment_enabled）。 */
  enrichmentEnabled?: boolean
}>()

const emit = defineEmits<{
  /** 开关状态被面板内一键开启修改，父组件需刷新 boards。 */
  (e: 'enrichment-toggled'): void
  /** 跨版块关系跳转对方版块（5.3）：父组件切换选中版块。 */
  (e: 'open-board', boardId: number): void
}>()

const {
  // topic selector
  topics, topicsLoading, selectedTopicId, loadTopics,
  // table 1
  contexts, contextsLoading, regenerating, saveContext, regenerateContext,
  // table 2
  results, triggering, triggerEnrichment,
  latestResultId, latestResultDetail, latestResultDetailLoading,
  // table 3
  reviews,
  // data sources（绑定管理入口已从工作台退役；bootstrap 仍拉取保持旧 API 可读）
  loadDataSources,
  // stock debates
  debates, debateTriggering, debateError, debateStage, loadDebates, triggerDebate,
  // qa (causal-analysis-agent 阶段3：报告追问 + 沉淀)
  qaList, qaLoading, qaError, latestAnswer, loadQA, askQuestion, sedimentAnswer,
  // board-level analysis（旧 brief/investigation 视图已退役；保留加载/同步：
  // 旧任务在跑时与新发现/研究共享 409 互斥，恢复轮询的提示仍真实）
  loadBoardAnalysisResults, syncBoardAnalysisStatus, syncTopicAnalysisStatus, activateBoardContext,
  // workbench UI
  selectedGran, selectedPeriodIdx, periodList, currentContext,
  setGran, shiftPeriod, selectPeriod,
  // misc
  loadAllTopicTables,
} = useBoardEnrichment()

// mermaid 图渲染（render-mermaid-diagrams）：宿主 = 工作台根，覆盖叙事/报告 markdown 块
const ewRootRef = ref<HTMLElement | null>(null)
useMermaidRender(ewRootRef, () => currentContext.value)

// ── 信号解读工作台（board-signal-reports 5.2/5.4）─────────────────────
// 主视图 = 候选信号列表 + 已生成报告列表：手动「发现信号」只保存候选；
// 逐条点「深入分析」才启动研究；报告生成即可阅读，无评审/采纳链。
// 轮询与任务态由 useSignalWorkbench 持有（发现结束即停，绝不串发研究）。
const sig = reactive(useSignalWorkbench())

/** 信号周期默认值（客户端时钟近似当期；未来周期由服务端按业务时区拒绝）。 */
function defaultSignalPeriod(gran: 'month' | 'year'): string {
  const now = new Date()
  return gran === 'year'
    ? String(now.getFullYear())
    : `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

/** 周期提交（工具栏 change）：只重拉候选/报告列表，不触发任何 LLM。 */
function handleSignalCommit(payload: { granularity: 'month' | 'year'; period: string }) {
  void sig.changePeriod(props.boardId, payload.granularity, payload.period)
}

/** 「发现信号」：先把视图切到所点周期（只重拉列表），再发 discovery——
 * FE-1：发现链路绝不串发 research 请求，发现结束即停等人工。 */
/** 查看候选历史研究进展（tasks 4.8 历史入口）。 */
async function handleSignalOpenProgress(candidateId: number) {
  await sig.openProgressHistory(props.boardId, candidateId)
}
async function handleSignalDiscover(payload: { granularity: 'month' | 'year'; period: string }) {
  if (payload.granularity !== sig.granularity || payload.period !== sig.period) {
    const ok = await sig.changePeriod(props.boardId, payload.granularity, payload.period)
    if (!ok) return
  }
  void sig.discover(props.boardId)
}

/** 深入分析/重新研究；regenerate=true 仅来自显式入口（先确认预算提示）。 */
function handleSignalResearch(candidateId: number, options: { regenerate: boolean }) {
  if (options.regenerate && !confirm('重新研究该信号？\n（新任务会重新取数与计算，可能消耗预算；成功后追加新版本报告，不覆盖原报告）')) return
  void sig.research(props.boardId, candidateId, { regenerate: options.regenerate })
}

function handleOpenSignalReport(resultId: number) {
  void sig.openReport(props.boardId, resultId)
}

function handleReportRegenerate(candidateId: number) {
  handleSignalResearch(candidateId, { regenerate: true })
}

const { success: notifySuccess, error: notifyError } = useNotify()

// ── 新闻背景折叠区（fix-board-analysis-material：去单 tab 导航）─────────
/** 新闻背景（循环A 新闻记忆）折叠态，默认收起。 */
const newsOpen = ref(false)

// ── 聚焦分析折叠区（单泳道分析收拢，board-level-deep-analysis 4.2）───────
/** 折叠区展开态（默认收起——主视图是信号工作台，单泳道是下钻入口）。 */
const focusOpen = ref(false)
/** 预填的 lens（空 = 常规单泳道分析）。 */
const prefillLens = ref('')

// ── 增强开关快捷入口（fix-board-analysis-material：新板块默认关，报错英文且
// 开关藏在编辑弹窗深处，用户找不到；工作台面板直接暴露状态 + 一键开启）──
const sbApi = useSemanticBoardsApi()
const enrichToggling = ref(false)
async function enableEnrichment() {
  if (enrichToggling.value) return
  enrichToggling.value = true
  try {
    const res = await sbApi.updateBoard(props.boardId, { enrichment_enabled: true })
    if (res.success) {
      notifySuccess('已开启数据增强，可触发循环 B 分析')
      emit('enrichment-toggled')
    } else {
      notifyError(res.error || '开启失败')
    }
  } finally {
    enrichToggling.value = false
  }
}

// ── lifecycle: board switch ──────────────────────────────────────────────
/** 已完成信号工作台初始化的板块（切板块才重置信号视图，刷新不误杀在跑轮询）。 */
let signalBoardLoaded = -1
async function bootstrap(boardId: number) {
  // 统一 board view context 守卫（5.4/5.5 review）：bootstrap 最前面激活新板块
  // 上下文——内部停旧 board 轮询（gen++ 隔离在途 poll）+ viewEpoch++（隔离旧板块
  // 在途/迟到的 trigger/sync/loader 响应）：旧板块慢响应一律静默丢弃（不 start
  // poll、不 toast、不写列表/选中/任务态），不串台新板块；随后才加载/同步新板块。
  // 信号工作台：切板块时隔离旧视图（停轮询+epoch++，迟到响应全数丢弃）；
  // 首次进入默认当前月（客户端近似，未来周期由服务端按业务时区拒）。同板块
  // 刷新（头部刷新按钮）不重置任务态，只重拉列表，不误杀在跑轮询。
  if (signalBoardLoaded !== boardId) {
    sig.setBoard(boardId)
    if (!sig.period) sig.period = defaultSignalPeriod(sig.granularity)
    signalBoardLoaded = boardId
  }
  activateBoardContext(boardId)
  // board 级三件套同步启动（与 loadTopics 无依赖，保持挂载即拉）；loadTopics
  // 先行 await 确定 selectedTopicId（5.5 最终 review：topic 档重进恢复）——
  // activate 已 epoch++，旧板块在途的 topic sync 迟到响应全数失配丢弃，不会
  // 写入新板块。
  const boardLoads = Promise.all([
    loadDataSources(boardId),
    loadBoardAnalysisResults(boardId),
    syncBoardAnalysisStatus(boardId),
    sig.loadPeriod(boardId),
  ])
  await loadTopics(boardId)
  const topicId = selectedTopicId.value
  // Critical（5.5 review 修复）：topic 维度调用顺序必须是 loadAllTopicTables
  // 在前、syncTopicAnalysisStatus 最后——loadAll 入口同步 stopTopicPoll（停旧
  // topic/debate 轮询，selectTopic/watch/bootstrap 三入口兑底），若 sync 在前，
  // 刚恢复的 running 轮询会被随后的 loadAll 误杀（poll 消失、triggering 归零，
  // 后台任务在前端失联）。sync 放在 boardLoads 之后收尾：bootstrap 内 sync
  // 之后不存在任何 stopTopicPoll 路径，恢复的轮询跨出 bootstrap 存活；无
  // topic（列表空/加载失败 selectedTopicId=null）则 loadAll/sync 二者都不调。
  if (topicId !== null) {
    await loadAllTopicTables(topicId)
  }
  await boardLoads
  if (topicId !== null) {
    await syncTopicAnalysisStatus(topicId)
  }
}
void bootstrap(props.boardId)
watch(() => props.boardId, (id) => { void bootstrap(id) })
// watch 兑底：loadTopics 换了 selectedTopicId（首挂载 null→id / 原选中失效自动
// 回退）时装载新 topic 表。与 bootstrap 的并发时序是安全的（Critical review）：
// pre-flush watch 在 ref 写入当拍的微任务里执行，必然先于 bootstrap 越过
// `await loadTopics` 的续体——其 loadAll 入口的同步 stopTopicPoll 永远发生在
// bootstrap 末尾 syncTopicAnalysisStatus 之前；sync 之后本 watch 只会因用户
// 显式换 topic（selectTopic/lane 下钻，本就该停旧轮询装载新 topic）才再触发，
// 不存在 sync 后再次误停恢复轮询的路径。同 topic 刷新（selectedTopicId 不变，
// watch 不触发）由 bootstrap 自身完成「加载表 + 恢复轮询」。
watch(selectedTopicId, (id) => { if (id !== null) void loadAllTopicTables(id) })

const selectedTopic = computed(() => topics.value.find((t) => t.id === selectedTopicId.value) ?? null)
const hasTopic = computed(() => selectedTopicId.value !== null)

// ── ① 周期筛选器 helpers ─────────────────────────────────────────────────
type PickerGran = 'week' | 'month' | 'year'
const GRANS: { id: PickerGran; label: string }[] = [
  { id: 'week', label: '按周' },
  { id: 'month', label: '按月' },
  { id: 'year', label: '按年' },
]

function formatPeriod(period: string | undefined, gran: string): string {
  if (!period) return '—'
  if (gran === 'week') {
    const m = /^(\d{4})-W(\d{1,2})$/.exec(period)
    if (m) return `${m[1] ?? ''} 第 ${parseInt(m[2] ?? '', 10)} 周`
    return period
  }
  if (gran === 'month') return period
  if (gran === 'year') return period
  return period
}
const periodLabel = computed(() => formatPeriod(currentContext.value?.period, selectedGran.value))
const granShort = computed(() => ({ week: '周', month: '月', year: '年' }[selectedGran.value] ?? '周期'))
const isLatest = computed(() => selectedPeriodIdx.value === 0)

// ── ① narrative 内联编辑 ─────────────────────────────────────────────────
const editingNarrative = ref(false)
const narrativeDraft = ref('')
function startEditNarrative() {
  narrativeDraft.value = currentContext.value?.content ?? ''
  editingNarrative.value = true
}
async function saveNarrative() {
  if (selectedTopicId.value === null || !currentContext.value) return
  const ok = await saveContext(selectedTopicId.value, selectedGran.value as ContextGranularity, currentContext.value.period, narrativeDraft.value)
  if (ok) editingNarrative.value = false
}
function cancelNarrative() { editingNarrative.value = false }

async function handleRegenerate() {
  if (selectedTopicId.value === null || regenerating.value) return
  // period=undefined 时后端默认用当前周期（RefreshGranularity）→ 空态也能生成
  const period = currentContext.value?.period
  const label = period ? periodLabel.value : `本${granShort.value}`
  if (!confirm(`重新汇总「${label}」？\n（约 10-30 秒，调用 LLM 重读新闻）`)) return
  await regenerateContext(selectedTopicId.value, selectedGran.value as ContextGranularity, period)
}

// ── ① 补生成周期（7.3.1：手动选未生成 period 触发首次生成）─────────────
const genDialogOpen = ref(false)
const genGran = ref<PickerGran>('week')
const genPeriod = ref<string>('')
/** 对话框所选 period 是否已存在（决定按钮文案「重生成」/「生成」+ 覆盖提示）。 */
const genPeriodExists = computed(() =>
  !!genPeriod.value && contexts.value.some((c) => c.granularity === genGran.value && c.period === genPeriod.value),
)
function openGenDialog() {
  genGran.value = selectedGran.value
  genPeriod.value = ''
  genDialogOpen.value = true
}
async function confirmGenerate() {
  if (selectedTopicId.value === null || regenerating.value) return
  const p = genPeriod.value.trim()
  if (!p) return
  if (genGran.value === 'year' && !/^\d{4}$/.test(p)) {
    notifyError('年份请填 4 位数字，如 2026')
    return
  }
  const ok = await regenerateContext(selectedTopicId.value, genGran.value as ContextGranularity, p)
  if (ok) {
    selectPeriod(genGran.value, p) // 选中刚生成的周期（contexts 已含新行）
    genDialogOpen.value = false
  }
}

// ①narrative 被因果报告引用次数：扫描 result.sectors（AnalyzeOutput）里所有
// AnalyzeRef（散落在事实层/时间线/见解层/线索/单点各处），按 context_id / period
// 命中当前 context 计数。
function collectAnalyzeRefs(out: AnalyzeOutput | null): AnalyzeRef[] {
  if (!out) return []
  const refs: AnalyzeRef[] = []
  const pushAll = (rs: AnalyzeRef[] | undefined) => { if (Array.isArray(rs)) refs.push(...rs) }
  const pushOne = (r: AnalyzeRef | undefined) => { if (r) refs.push(r) }
  switch (out.form) {
    case 'event_chain':
      for (const f of out.analysis.fact_layer) pushAll(f.evidence)
      for (const t of out.analysis.timeline) pushOne(t.ref)
      for (const ins of out.analysis.insight_layer) { pushAll(ins.evidence); pushAll(ins.web_verified) }
      break
    case 'theme_vein':
      for (const v of out.analysis.veins) pushAll(v.evidence)
      for (const ins of out.analysis.cross_insight) { pushAll(ins.evidence); pushAll(ins.web_verified) }
      break
    case 'single_point':
      pushAll(out.analysis.evidence)
      break
    case 'structural':
      for (const p of out.analysis.phases) pushOne(p.ref)
      break
    case 'sparse':
      break
  }
  // 深度层 evidence_chain 的 news 引用也计入（additive；旧结果无 depth 自动跳过）
  const depth = 'depth' in out.analysis ? out.analysis.depth : undefined
  if (depth && Array.isArray(depth.evidence_chain)) {
    for (const ev of depth.evidence_chain) {
      if (ev && ev.source_type === 'news' && ev.ref) {
        pushOne({ ...ev, source_type: 'news', ref: ev.ref })
      }
    }
  }
  return refs
}
const narrativeCiteCount = computed(() => {
  const ctx = currentContext.value
  if (!ctx) return 0
  const refs = collectAnalyzeRefs(latestResultDetail.value?.sectors ?? null)
  let n = 0
  for (const r of refs) {
    if (r.context_id != null && String(r.context_id) === String(ctx.id)) n++
    else if (r.period && r.period === ctx.period) n++
  }
  return n
})


// ── 触发增强 / session_id 复制 ───────────────────────────────────────────
async function handleTrigger() {
  if (selectedTopicId.value === null || triggering.value) return
  const lens = prefillLens.value.trim()
  if (!confirm(`手动触发${lens ? '（预填视角）' : ''}聚焦分析？\n（约 10-30 秒，需板块已开启增强开关）`)) return
  const ok = await triggerEnrichment(selectedTopicId.value, lens || undefined)
  if (ok) prefillLens.value = '' // 消费掉，避免下次常规分析误带视角
}
async function copySession() {
  const sid = latestResultDetail.value?.session_id
  if (!sid) return
  try { await navigator.clipboard.writeText(sid); notifySuccess('session_id 已复制') }
  catch { notifyError('复制失败') }
}

// ── ④ debate 触发（挂在最新 result） ─────────────────────────────────────
async function handleDebateTrigger() {
  if (selectedTopicId.value === null || latestResultId.value === null) return
  await triggerDebate(latestResultId.value)
}
async function handleDebateRetry() {
  if (latestResultId.value === null) return
  await loadDebates(latestResultId.value)
}
</script>

<template>
  <div ref="ewRootRef" class="ew-panel">
    <!-- ── 信号解读工作台（board-signal-reports：发现 → 候选 → 报告，无评审） ── -->
    <section class="block board-analysis-main">
      <div class="block-head board-head">
        <h2 class="serif">信号解读</h2>
        <span class="helper">发现信号只保存候选 · 逐条深入研究 · 报告生成即可阅读</span>
        <span v-if="sig.discoveryRunning" class="bb-job-tag" data-testid="signal-discovery-tag">
          <Icon icon="mdi:loading" width="13" class="spin" />
          信号发现中…（不生成报告）
        </span>
        <span v-else-if="sig.researchCandidateId !== null" class="bb-job-tag" data-testid="signal-research-tag">
          <Icon icon="mdi:loading" width="13" class="spin" />
          {{ sig.researchProgressLabel }}
        </span>
        <span class="ew-spacer" />
        <button type="button" class="ew-ghost-btn" title="刷新（重拉候选/报告/泳道）" @click="bootstrap(boardId)">
          <Icon icon="mdi:refresh" width="14" />
        </button>
      </div>
      <div v-if="!enrichmentEnabled" class="ew-enrichment-off">
        <Icon icon="mdi:shield-alert-outline" width="15" />
        <span>该板块未开启数据增强（发现信号需要先开启，防止误触烧 LLM）</span>
        <button type="button" class="btn btn-primary btn-sm" :disabled="enrichToggling" @click="enableEnrichment">
          {{ enrichToggling ? '开启中…' : '一键开启' }}
        </button>
      </div>

      <!-- 报告阅读视图（reader≤760；激活时替换候选/报告列表，返回保留周期） -->
      <SignalReportView
        v-if="sig.activeReportId !== null"
        :report="sig.activeReport"
        :loading="sig.activeReportLoading"
        :error="sig.activeReportError"
        :regenerating="sig.researchCandidateId !== null"
        :regenerate-phase="sig.researchPhase"
        @back="sig.closeReport()"
        @regenerate="handleReportRegenerate"
      />
      <template v-else>
        <!-- 候选信号列表（周期工具栏 + 发现信号 + 候选行） -->
        <SignalCandidateList
          :candidates="sig.candidates"
          :loading="sig.candidatesLoading"
          :error="sig.candidatesError"
          :granularity="sig.granularity"
          :period="sig.period"
          :discovery-running="sig.discoveryRunning"
          :discovery-phase="sig.discoveryPhase"
          :discovery-error="sig.discoveryError"
          :research-candidate-id="sig.researchCandidateId"
          :research-phase="sig.researchPhase"
          :research-error="sig.researchError"
          :research-progress-label="sig.researchProgressLabel"
          :research-progress="sig.researchProgress"
          @commit="handleSignalCommit"
          @discover="handleSignalDiscover"
          @research="handleSignalResearch"
          @open-report="handleOpenSignalReport"
          @open-progress="handleSignalOpenProgress"
        />

        <!-- 已生成报告列表（真实计数；历史标事后回顾；无评审操作） -->
        <SignalReportList
          :reports="sig.reports"
          :loading="sig.reportsLoading"
          :error="sig.reportsError"
          @open="handleOpenSignalReport"
        />
      </template>
    </section>

    <!-- ── 聚焦分析折叠区（单泳道下钻入口，默认收起） ────────────────────── -->
    <section id="focus-analysis" class="block focus-block" :class="{ open: focusOpen }">
      <button type="button" class="focus-toggle" @click="focusOpen = !focusOpen">
        <Icon :icon="focusOpen ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="16" />
        <span>聚焦分析（单泳道深挖）</span>
        <span v-if="selectedTopic" class="focus-cur">当前：{{ selectedTopic.label }}</span>
        <span v-if="prefillLens" class="focus-lens-tag" title="来自版块报告下钻的预填视角">
          <Icon icon="mdi:target" width="12" /> {{ prefillLens }}
        </span>
      </button>
      <p class="focus-hint muted">选一条泳道单独深挖；版块报告里的泳道引用点击后会跳到这里并预填视角。</p>

      <div v-if="focusOpen" class="focus-body">
        <template v-if="hasTopic">
          <div class="focus-toolbar">
            <select v-model.number="selectedTopicId" class="ew-select" :disabled="topics.length === 0">
              <option :value="null" disabled>选择泳道…</option>
              <option v-for="t in topics" :key="t.id" :value="t.id">{{ t.label }}（{{ t.status }}）</option>
            </select>
            <textarea
              v-model="prefillLens"
              class="ew-input focus-lens-input"
              rows="2"
              placeholder="预填视角（可选）：版块报告下钻会填入具体问题/证据说明，可修改后再发起聚焦分析"
            />
            <button type="button" class="btn btn-primary" :disabled="triggering" @click="handleTrigger">
              <Icon icon="mdi:play" width="13" />
              {{ triggering ? '分析中…可离开' : '▶ 聚焦分析' }}
            </button>
          </div>

          <CausalAnalysisReport
            :result="latestResultDetail"
            :topic-label="selectedTopic?.label ?? undefined"
            :loading="latestResultDetailLoading"
          />
          <div v-if="latestResultDetail" class="outlook-meta">
            <span class="link-btn" @click="copySession">
              <Icon icon="mdi:content-copy" width="12" /> session_id：{{ latestResultDetail.session_id ?? '—' }}
            </span>
          </div>

          <!-- 报告追问 chat（挂在最新 result 上） ──────────────────────── -->
          <QAPanel
            v-if="latestResultId !== null"
            :result-id="latestResultId"
            :qa-list="qaList"
            :qa-loading="qaLoading"
            :qa-error="qaError"
            :latest-answer="latestAnswer"
            @ask="askQuestion"
            @sediment="sedimentAnswer"
            @load="loadQA"
          />
        </template>
        <p v-else class="ew-empty-hint">该板块暂无持久话题，无法聚焦分析。</p>
      </div>
    </section>

    <!-- ── 新闻背景（循环A 新闻记忆 · 折叠 section，分析不回写） ────────── -->
    <section id="sec1" class="block focus-block news-block" :class="{ open: newsOpen }">
      <button type="button" class="focus-toggle" @click="newsOpen = !newsOpen">
        <Icon :icon="newsOpen ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="16" />
        <span>新闻背景（新闻记忆）</span>
        <span v-if="selectedTopic" class="focus-cur">当前：{{ selectedTopic.label }}</span>
      </button>
      <p class="focus-hint muted">新闻记忆只随新闻变，分析不回写；周期筛选翻历史，叙事可 inline 编辑。</p>

      <div v-if="newsOpen" class="focus-body">
        <template v-if="hasTopic">
          <!-- 周期筛选器 -->
          <div class="period-picker">
          <div class="gran-select">
            <button
              v-for="g in GRANS"
              :key="g.id"
              type="button"
              :class="{ on: selectedGran === g.id }"
              @click="setGran(g.id)"
            >{{ g.label }}</button>
          </div>
          <div class="period-nav">
            <button type="button" title="上一周期" :disabled="selectedPeriodIdx >= periodList.length - 1" @click="shiftPeriod(1)">‹</button>
            <span class="cur">{{ periodList.length ? periodLabel : '无历史周期' }}</span>
            <button type="button" title="下一周期" :disabled="selectedPeriodIdx <= 0" @click="shiftPeriod(-1)">›</button>
          </div>
          <span v-if="periodList.length" class="fresh-tag" :class="isLatest ? 'fresh-latest' : 'fresh-stale'">
            {{ isLatest ? '最新周期' : '历史周期' }}
          </span>
          <span class="muted ew-period-count">共 {{ periodList.length }} 个历史周期可翻</span>
          <button type="button" class="btn btn-ghost btn-sm ew-gen-trigger" @click="openGenDialog">
            <Icon icon="mdi:calendar-plus" width="13" />
            补生成周期
          </button>
        </div>

        <!-- 叙事 -->
        <div class="narrative">
          <div id="seg-narrative" class="seg">
            <template v-if="contextsLoading">
              <div class="seg-h-row"><div class="seg-h serif">叙事脉络</div></div>
              <div class="seg-b muted">加载中…</div>
            </template>
            <template v-else-if="!currentContext">
              <div class="seg-h-row"><div class="seg-h serif">该周期尚未生成汇总</div></div>
              <div class="seg-b muted">没有这一周期的新闻汇总。点「重新汇总」让 AI 重读新闻生成。</div>
              <div class="seg-actions">
                <button type="button" class="btn btn-primary btn-sm" :disabled="regenerating !== null" @click="handleRegenerate">
                  <Icon icon="mdi:refresh" width="12" />
                  {{ regenerating !== null ? '汇总中…' : '↻ 生成本' + granShort + '汇总' }}
                </button>
              </div>
            </template>
            <template v-else-if="!editingNarrative">
              <div class="seg-h-row">
                <div class="seg-h serif">叙事脉络</div>
                <span class="seg-cited" :class="{ empty: narrativeCiteCount === 0 }">
                  {{ narrativeCiteCount > 0 ? `被因果报告引用 ${narrativeCiteCount} 处` : '未被报告引用' }}
                </span>
              </div>
              <div class="seg-b markdown-body" v-html="renderMarkdown(currentContext.content)" />
              <span class="seg-edit" title="编辑" @click="startEditNarrative">✎</span>
            </template>
            <template v-else>
              <div class="seg-h-row"><div class="seg-h serif">编辑叙事</div></div>
              <textarea v-model="narrativeDraft" class="seg-textarea" />
              <div class="seg-actions">
                <button type="button" class="btn btn-primary btn-sm" @click="saveNarrative">保存</button>
                <button type="button" class="btn btn-ghost btn-sm" @click="cancelNarrative">取消</button>
              </div>
            </template>
          </div>

          <div v-if="currentContext" class="narrative-foot">
            <span>来源：{{ currentContext.source === 'manual' ? '手动' : (currentContext.source === 'llm_assisted' ? 'AI 汇总' : currentContext.source) }}</span>
            <span>·</span>
            <span>截止 {{ currentContext.as_of_date ? new Date(currentContext.as_of_date).toISOString().slice(0, 10) : '—' }}</span>
            <span style="margin-left:auto">
              <button type="button" class="btn btn-ghost btn-sm" :disabled="regenerating !== null" @click="handleRegenerate">
                <Icon icon="mdi:refresh" width="12" />
                {{ regenerating !== null ? '汇总中…' : '↻ 重新汇总' }}
              </button>
            </span>
          </div>
        </div>
        </template>
        <p v-else class="ew-empty-hint">先在上方「聚焦分析」选择泳道，再展开新闻背景查看其新闻记忆。</p>
      </div>
    </section>

    <!-- ── 金融可选模块：FinGenius 个股辩论（默认折叠 · 独立于演进主线） ── -->
      <section class="block">
        <details class="debate-fold">
          <summary>
            <span class="df-tag">金融可选模块</span>
            <span class="df-title">FinGenius 个股辩论</span>
            <span class="df-note-inline">独立于演进主线 · 仅金融话题</span>
          </summary>
          <div class="df-body">
            <p class="df-hint">该模块接 FinGenius 对个股做 6 角色辩论，属金融可选能力，<b>不参与演进主线定位</b>。仅金融话题有意义。</p>
            <DebateSection
              :stage="debateStage"
              :debates="debates"
              :sectors="null"
              :triggering="debateTriggering"
              :error-msg="debateError"
              @trigger="handleDebateTrigger"
              @retry="handleDebateRetry"
            />
          </div>
        </details>
      </section>


    <!-- ── Dialog: 补生成周期（7.3.1） ──────────────────────────────── -->
    <AppDialog :model-value="genDialogOpen" title="补生成周期" width="440px" @update:model-value="(v) => { if (!v) genDialogOpen = false }">
      <div class="ew-dialog-form">
        <p class="ew-gen-hint">选择一个周期生成新闻汇总。<b>支持从未生成过的历史周期</b>（约 10-30 秒，调用 AI 重读该周期新闻）。</p>
        <div class="ew-field">
          <span class="ew-field-label">粒度</span>
          <div class="gran-select gran-select--dialog">
            <button v-for="g in GRANS" :key="g.id" type="button" :class="{ on: genGran === g.id }" @click="genGran = g.id; genPeriod = ''">{{ g.label }}</button>
          </div>
        </div>
        <label class="ew-field">
          <span class="ew-field-label">周期</span>
          <input
            v-if="genGran !== 'year'"
            :type="genGran"
            v-model="genPeriod"
            class="ew-period-input ew-period-input--dialog"
          />
          <input
            v-else
            type="text"
            inputmode="numeric"
            pattern="\d{4}"
            maxlength="4"
            placeholder="如 2026"
            v-model="genPeriod"
            class="ew-period-input ew-period-input--dialog ew-period-input--year"
          />
          <span v-if="genPeriod && genPeriodExists" class="ew-gen-exists">该周期已存在，将覆盖重算</span>
        </label>
      </div>
      <template #footer>
        <AppButton variant="ghost" size="sm" @click="genDialogOpen = false">取消</AppButton>
        <AppButton variant="primary" size="sm" :disabled="!genPeriod.trim() || regenerating !== null" @click="confirmGenerate">
          <Icon icon="mdi:calendar-plus" width="13" />
          {{ regenerating !== null ? '生成中…' : (genPeriodExists ? '重生成' : '生成') }}
        </AppButton>
      </template>
    </AppDialog>
  </div>
</template>

<style scoped>
.ew-panel { display: flex; flex-direction: column; gap: 1rem; max-width: 960px; margin: 0 auto; }

/* topic 工具条 */
.ew-field-label { font-size: 0.68rem; color: var(--color-text-secondary); }
.ew-select { padding: 0.25rem 0.5rem; font-size: 0.78rem; border: 1px solid var(--color-input-border); border-radius: 6px; background: var(--color-input-bg); color: var(--color-text-primary); outline: none; max-width: 320px; }
.ew-select:focus { border-color: var(--color-input-focus); }
.ew-spacer { flex: 1; }
.ew-muted { color: var(--color-text-muted); }
.ew-ghost-btn { display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.2rem 0.5rem; border-radius: 6px; border: 1px solid var(--color-border-medium); background: var(--color-bg-sunken); color: var(--color-text-primary); font-size: 0.68rem; cursor: pointer; font-family: inherit; }
.ew-ghost-btn:hover { background: var(--color-bg-hover); }
.ew-empty-hint { font-size: 0.78rem; color: var(--color-text-muted); padding: 0.5rem 0.75rem; }

/* 紧凑刊头 */
.masthead { border-bottom: 2px solid var(--color-text-primary); padding-bottom: 1rem; margin-bottom: 0.5rem; }
.masthead .eyebrow { font-size: 11px; letter-spacing: 0.18em; text-transform: uppercase; color: var(--color-accent); font-weight: 600; margin-bottom: 0.4rem; }
.masthead-title { font-size: 1.6rem; line-height: 1.2; margin: 0 0 0.6rem; font-weight: 700; letter-spacing: -0.01em; color: var(--color-text-primary); }
.lede { display: flex; align-items: center; gap: 0.6rem; flex-wrap: wrap; font-size: 13px; color: var(--color-text-secondary); }
.lede b { color: var(--color-text-primary); }
.status-pill { display: inline-flex; align-items: center; gap: 5px; padding: 2px 9px; border-radius: 999px; font-size: 11px; font-weight: 600; }
.status-pill.evolving { background: var(--color-warning-subtle); color: var(--color-warning); }
.status-pill.static { background: var(--color-bg-sunken); color: var(--color-text-muted); }
.status-pill .dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; animation: ew-pulse 1.6s infinite; }
@keyframes ew-pulse { 0%,100%{opacity:1;} 50%{opacity:.35;} }
.trigger-wrap { margin-left: auto; }

/* ── 版块简报主视图 + 聚焦分析折叠区（board-level-deep-analysis 4.2 / 5.5）─ */
.board-head { display: flex; align-items: center; gap: 0.6rem; flex-wrap: wrap; }
.board-history { max-width: 240px; font-size: 0.78rem; }
.bb-job-tag {
  display: inline-flex; align-items: center; gap: 0.3rem;
  font-size: 0.72rem; color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
  padding: 0.15rem 0.55rem; border-radius: 99px; white-space: nowrap;
}
.bb-job-tag .spin { animation: bb-tag-spin 1s linear infinite; }
@keyframes bb-tag-spin { to { transform: rotate(360deg); } }
.bb-legacy-banner {
  display: inline-flex; align-items: center; gap: 0.35rem;
  font-size: 0.75rem; color: var(--color-text-muted);
  padding: 0.25rem 0.6rem; border-radius: 6px; margin-bottom: 0.6rem;
  background: var(--color-bg-sunken); border: 1px solid var(--color-border-subtle);
}
.focus-block { border: 1px solid var(--color-border-subtle); border-radius: 12px; background: var(--color-bg-elevated); overflow: hidden; }
.focus-toggle {
  display: flex; align-items: center; gap: 0.5rem; width: 100%;
  padding: 0.7rem 0.9rem; border: none; background: none;
  color: var(--color-text-primary); font-size: 0.85rem; font-weight: 600;
  cursor: pointer; text-align: left; font-family: inherit;
}
.focus-toggle:hover { background: var(--color-bg-hover); }
.focus-cur { color: var(--color-text-muted); font-weight: 400; font-size: 0.78rem; }
.focus-lens-tag {
  display: inline-flex; align-items: center; gap: 0.25rem;
  font-size: 0.72rem; font-weight: 500; padding: 0.1rem 0.5rem;
  border-radius: 99px; color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 12%, transparent);
}
.focus-hint { margin: 0 0.9rem; font-size: 0.75rem; }
.focus-body { padding: 0.4rem 0.9rem 0.9rem; display: flex; flex-direction: column; gap: 0.9rem; border-top: 1px solid var(--color-border-subtle); }
.focus-toolbar { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; }
.focus-lens-input {
  flex: 1; min-width: 200px; padding: 0.35rem 0.6rem;
  border: 1px solid var(--color-border-subtle); border-radius: 8px;
  background: var(--color-bg); color: var(--color-text-primary); font-size: 0.8rem;
  resize: vertical; font-family: inherit; line-height: 1.55;
}


/* 通用 btn */
.btn { display: inline-flex; align-items: center; gap: 6px; font-family: inherit; cursor: pointer; border: none; border-radius: 8px; font-weight: 600; font-size: 13px; padding: 7px 14px; transition: background 0.15s, opacity 0.15s, transform 0.1s; }
.btn:active { transform: translateY(1px); }
.btn-primary { background: var(--color-accent); color: #fff; }
.btn-primary:hover { background: var(--color-accent-hover); }
.btn-ghost { background: transparent; color: var(--color-text-secondary); border: 1px solid var(--color-border-medium); }
.btn-ghost:hover { background: var(--color-bg-hover); color: var(--color-text-primary); }
.btn-sm { padding: 4px 10px; font-size: 12px; }

/* 增强开关关闭时的提示条（fix-board-analysis-material：开关可发现性） */
.ew-enrichment-off {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 10px;
  border: 1px dashed var(--color-warning, #d97706);
  border-radius: 8px;
  background: color-mix(in srgb, var(--color-warning, #d97706) 8%, transparent);
  font-size: 13px;
  color: var(--color-text-secondary, #666);
  flex-wrap: wrap;
}
.btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* 区块 */
.block { margin-bottom: 1.5rem; }
.block-head { display: flex; align-items: baseline; gap: 0.6rem; margin-bottom: 0.9rem; padding-bottom: 0.4rem; border-bottom: 1px solid var(--color-border-subtle); }
.block-head h2 { font-size: 1.1rem; margin: 0; font-weight: 700; color: var(--color-text-primary); }
.block-head .helper { font-size: 12px; color: var(--color-text-muted); }

/* ① 周期筛选器 */
.period-picker { display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap; background: var(--color-bg-elevated); border: 1px solid var(--color-border-subtle); border-radius: 12px; padding: 0.6rem 0.9rem; margin-bottom: 1.1rem; box-shadow: var(--shadow-subtle); }
.gran-select { display: inline-flex; border: 1px solid var(--color-border-medium); border-radius: 8px; overflow: hidden; }
.gran-select button { border: none; background: var(--color-bg-base); color: var(--color-text-secondary); font-size: 12.5px; padding: 5px 14px; cursor: pointer; font-family: inherit; font-weight: 600; }
.gran-select button.on { background: var(--color-accent); color: #fff; }
.period-nav { display: inline-flex; align-items: center; gap: 0.5rem; }
.period-nav .cur { font-size: 14px; font-weight: 600; min-width: 120px; text-align: center; color: var(--color-text-primary); }
.period-nav button { width: 26px; height: 26px; border: 1px solid var(--color-border-medium); background: var(--color-bg-base); color: var(--color-text-secondary); border-radius: 6px; cursor: pointer; font-family: inherit; }
.period-nav button:hover:not(:disabled) { border-color: var(--color-accent); color: var(--color-accent); }
.period-nav button:disabled { opacity: 0.3; cursor: not-allowed; }
.fresh-tag { font-size: 11px; padding: 2px 8px; border-radius: 999px; }
.fresh-latest { background: var(--color-success-subtle); color: var(--color-success); }
.fresh-stale { background: var(--color-warning-subtle); color: var(--color-warning); }
.ew-period-count { font-size: 11.5px; }
/* ① 补生成周期入口（7.3.1） */
.ew-gen-trigger { margin-left: auto; }
.ew-period-input { padding: 5px 8px; font-size: 13px; border: 1px solid var(--color-input-border); border-radius: 8px; background: var(--color-input-bg); color: var(--color-text-primary); outline: none; font-family: inherit; width: 100%; box-sizing: border-box; }
.ew-period-input:focus { border-color: var(--color-input-focus); }
.ew-period-input--year { max-width: 160px; text-align: center; }
.ew-period-input--dialog { margin-top: 2px; }
.ew-gen-hint { font-size: 12px; color: var(--color-text-muted); margin: 0 0 0.4rem; line-height: 1.6; }
.ew-gen-hint b { color: var(--color-text-secondary); }
.ew-gen-exists { font-size: 11px; color: var(--color-warning); margin-top: 4px; display: block; }
.gran-select--dialog { display: inline-flex; }

/* 叙事 */
.narrative { background: var(--color-bg-elevated); border: 1px solid var(--color-border-subtle); border-left: 3px solid var(--color-accent); border-radius: 0 8px 8px 0; padding: 1.1rem 1.3rem; box-shadow: var(--shadow-print); }
.seg { position: relative; padding: 0.35rem 0.5rem 0.35rem 0; margin: 0 -0.5rem; border-radius: 6px; transition: background 0.12s; }
.seg:hover { background: var(--color-bg-hover); }
.seg .seg-h { font-weight: 600; font-size: 13.5px; margin-bottom: 0.2rem; color: var(--color-text-primary); }
.seg .seg-b { font-size: 13px; color: var(--color-text-secondary); line-height: 1.65; white-space: pre-wrap; }
/* markdown-body 宿主：HTML 已含块结构，关掉 pre-wrap 避免标签换行被渲染成空行 */
.seg .seg-b.markdown-body { white-space: normal; font-size: 13px; line-height: 1.65; }
.seg .seg-b.muted { color: var(--color-text-muted); }
.seg .seg-edit { position: absolute; right: 6px; top: 6px; opacity: 0; transition: opacity 0.12s; cursor: pointer; padding: 3px 6px; border-radius: 5px; color: var(--color-text-muted); }
.seg:hover .seg-edit { opacity: 1; }
.seg .seg-edit:hover { background: var(--color-accent-subtle); color: var(--color-accent); }
.seg-textarea { width: 100%; min-height: 90px; resize: vertical; font-family: inherit; font-size: 13px; line-height: 1.6; background: var(--color-input-bg); border: 1px solid var(--color-input-border); border-radius: 6px; color: var(--color-text-primary); padding: 0.5rem 0.6rem; outline: none; box-sizing: border-box; }
.seg-textarea:focus { border-color: var(--color-input-focus); }
.seg-actions { display: flex; gap: 0.4rem; margin-top: 0.4rem; }
.seg-h-row { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.2rem; }
.seg-cited { font-size: 10px; color: var(--color-accent); background: var(--color-accent-subtle); padding: 1px 7px; border-radius: 999px; font-weight: 600; white-space: nowrap; }
.seg-cited.empty { color: var(--color-text-muted); background: var(--color-bg-sunken); }
.narrative-foot { display: flex; align-items: center; gap: 0.6rem; margin-top: 0.9rem; padding-top: 0.7rem; border-top: 1px solid var(--color-dialog-divider); font-size: 11.5px; color: var(--color-text-muted); }

/* 演进报告 meta */
.outlook-meta { margin-top: 0.8rem; font-size: 11.5px; }
.link-btn { display: inline-flex; align-items: center; gap: 3px; padding: 2px 7px; border: none; background: none; color: var(--color-text-muted); font-size: 11.5px; cursor: pointer; border-radius: 5px; font-family: inherit; }
.link-btn:hover { color: var(--color-accent); background: var(--color-accent-subtle); }

/* 金融可选模块 · DebateSection 折叠 */
.debate-fold { background: var(--color-bg-elevated); border: 1px solid var(--color-border-subtle); border-radius: 12px; padding: 0.7rem 0.95rem; }
.debate-fold > summary { cursor: pointer; font-size: 13px; font-weight: 600; color: var(--color-text-secondary); list-style: none; display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; }
.debate-fold > summary::-webkit-details-marker { display: none; }
.debate-fold > summary::before { content: '▸ '; color: var(--color-text-muted); }
.debate-fold[open] > summary::before { content: '▾ '; }
.df-tag { font-size: 10px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--color-warning); background: var(--color-warning-subtle); padding: 2px 8px; border-radius: 4px; }
.df-title { color: var(--color-text-primary); }
.df-note-inline { font-size: 11px; color: var(--color-text-muted); font-weight: 400; }
.df-body { padding: 0.7rem 0 0; }
.df-hint { font-size: 12px; color: var(--color-text-muted); margin: 0 0 0.8rem; padding: 0.5rem 0.7rem; background: var(--color-bg-sunken); border-radius: 6px; line-height: 1.6; }
.df-hint b { color: var(--color-text-secondary); }

/* 数据源折叠高级 */
details.adv { background: var(--color-bg-elevated); border: 1px solid var(--color-border-subtle); border-radius: 10px; padding: 0.6rem 0.9rem; }
details.adv > summary { cursor: pointer; font-size: 13px; font-weight: 600; color: var(--color-text-secondary); list-style: none; }
details.adv > summary::-webkit-details-marker { display: none; }
details.adv > summary::before { content: "▸ "; color: var(--color-text-muted); }
details.adv[open] > summary::before { content: "▾ "; }
.adv-body { padding: 0.6rem 0 0; font-size: 12.5px; color: var(--color-text-secondary); }
.adv-row { display: flex; gap: 0.5rem; flex-wrap: wrap; align-items: center; margin-bottom: 0.5rem; }
.src-chip { display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 8px; background: var(--color-bg-base); border: 1px solid var(--color-border-medium); font-size: 12px; cursor: pointer; }
.src-chip .ok { color: var(--color-success); }
.kv { font-size: 12px; color: var(--color-text-secondary); }
.kv b { color: var(--color-text-primary); }

/* dialogs */
.ew-dialog-form { display: flex; flex-direction: column; gap: 0.75rem; }
.ew-field { display: flex; flex-direction: column; gap: 0.3rem; }
.ew-field--row { flex-direction: row; align-items: center; gap: 0.5rem; }
.ew-textarea { width: 100%; box-sizing: border-box; border: 1px solid var(--color-input-border); border-radius: 8px; background: var(--color-input-bg); color: var(--color-text-primary); font-size: 0.78rem; padding: 0.5rem 0.7rem; outline: none; resize: vertical; font-family: inherit; line-height: 1.5; }
.ew-textarea:focus { border-color: var(--color-input-focus); }
.ew-textarea--mono { font-family: ui-monospace, "Cascadia Code", Menlo, monospace; font-size: 0.72rem; }

.muted { color: var(--color-text-muted); }
.serif { font-family: Georgia, "Songti SC", "SimSun", "Source Serif 4", serif; }

/* ── 窄屏适配（≤720px，对齐 daily-report 家族断点） ───────────────────── */
@media (max-width: 720px) {
  .ew-panel { gap: 0.75rem; }

  /* topic 工具条：换行 + 话题下拉撑满一行，刷新按钮放大到触摸友好 */
  /* 粒度切换 / 翻页按钮：hit-target ≥36px */
  .gran-select button { min-height: 36px; }
  .ew-select { max-width: 100%; flex: 1 1 auto; min-width: 0; }
  .ew-ghost-btn { min-width: 36px; min-height: 36px; justify-content: center; }

  /* 刊头标题降档 */
  .masthead-title { font-size: 1.3rem; }

  /* 触发按钮不再被 margin-left:auto 挤到行尾，自然跟排 */
  .trigger-wrap { margin-left: 0; }

  /* 粒度切换 / 翻页按钮：hit-target ≥36px */

  .gran-select button { min-height: 36px; }
  .period-nav .cur { min-width: 0; }
  .period-nav button { width: 36px; height: 36px; }

  /* 叙事卡片留白收缩 */
  .narrative { padding: 0.85rem 0.9rem; }
}
</style>
