<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useBoardEnrichment } from '~/features/tags/composables/useBoardEnrichment'
import { useSignalWorkbench } from '~/features/tags/composables/useSignalWorkbench'
import { useNotify } from '~/composables/useNotify'
import { useSemanticBoardsApi } from '~/api/semanticBoards'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import SignalCandidateList from './SignalCandidateList.vue'
import SignalReportView from './SignalReportView.vue'
import TopicNewsBackground from './TopicNewsBackground.vue'
import LegacyResearchHistory from './LegacyResearchHistory.vue'

/**
 * 研究 · 信号研究工作台（redesign-enrichment-research-workbench 4.1）。
 *
 * 主从（AppPageShell split）布局：左栏 = SignalCandidateList 紧凑索引（筛选
 * tab + 候选行；忽略/恢复/研究/阅读全部 emit 回本面板，FE-1 边界不变）；
 * 右栏 = 详情（未选提示 → 候选详情：问题/依据/相关话题/动作 → SignalReportView
 * 报告阅读）。已生成报告不再在列表底部重复排列，多版本归在候选详情下。
 *
 * 退役区块（旧 API 后端保留兼容，面板不再挂生成/追问/沉淀/辩论控件）：
 * 聚焦分析折叠区（单泳道下钻 + QAPanel）、新闻背景折叠区（循环A 新闻记忆，
 * 迁话题上下文）、DebateSection（FinGenius 辩论）。useBoardEnrichment 仅保留
 * bootstrap 所需最小集合（任务互斥恢复/话题上下文装载仍要）。
 *
 * 窄屏（useIsNarrowViewport，767.98px）单栏：默认索引，选中/读报告进详情，
 * 返回按钮恢复索引与选择；≥768px 宽屏布局零变化（workbenchDetailOpen 只在
 * 窄屏分支读取）。
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
  /** 打开旧研究历史（只读，5.2 LegacyResearchHistory 接入）。 */
}>()

const {
  // topic selector（话题上下文仍由 bootstrap 装载：running 任务互斥恢复要用）
  selectedTopicId, loadTopics,
  // data sources（绑定管理入口已退役；bootstrap 仍拉取保持旧 API 可读）
  loadDataSources,
  // board-level analysis（旧 brief/investigation 视图已退役；保留加载/同步：
  // 旧任务在跑时与新发现/研究共享 409 互斥，恢复轮询的提示仍真实）
  loadBoardAnalysisResults, syncBoardAnalysisStatus, syncTopicAnalysisStatus, activateBoardContext,
  // misc
  loadAllTopicTables,
} = useBoardEnrichment()

// ── 信号研究工作台（redesign-enrichment-research-workbench 4.1）───────────
// 轮询与任务态由 useSignalWorkbench 持有（发现结束即停，绝不串发研究）；
// 忽略/恢复/候选详情/版本/任务找回均在状态层完成，面板只做视图编排。
const sig = reactive(useSignalWorkbench())

// ── 相关话题 → 只读背景预览（redesign 4.3：研究中只读，管理转话题总览）──────
const backgroundPreview = ref<{ topicId: number; label: string } | null>(null)
function openTopicBackgroundPreview(topicId: number, label: string) {
  backgroundPreview.value = { topicId, label }
}

// ── 旧研究历史（只读，5.2）：xl 弹层外壳，零触发/追问/沉淀。─────
const legacyHistoryOpen = ref(false)

const { success: notifySuccess, error: notifyError } = useNotify()

// ── 窄屏单栏（767.98px）：默认索引，选中/读报告进详情；宽屏分支不读取此状态 ──
const isNarrow = useIsNarrowViewport()
const workbenchDetailOpen = ref(false)

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
async function handleSignalDiscover(payload: { granularity: 'month' | 'year'; period: string }) {
  if (payload.granularity !== sig.granularity || payload.period !== sig.period) {
    const ok = await sig.changePeriod(props.boardId, payload.granularity, payload.period)
    if (!ok) return
  }
  void sig.discover(props.boardId)
}

/** 查看候选历史研究进展（tasks 4.8 历史入口）。 */
async function handleSignalOpenProgress(candidateId: number) {
  await sig.openProgressHistory(props.boardId, candidateId)
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

// ── 索引 ↔ 详情联动 ──────────────────────────────────────────────────────

/** 选中候选：打开详情；读报告中先收起报告（返回详情栏确定性——所选即所显）；
 * 窄屏进入详情单栏。运行中仍可浏览（浏览与任务分离）。 */
function handleSignalSelect(candidateId: number) {
  if (sig.activeReportId !== null) sig.closeReport()
  void sig.openCandidate(props.boardId, candidateId)
  if (isNarrow.value) workbenchDetailOpen.value = true
}

/** 忽略/恢复（disposition PUT 由 composable 裁决，409 提示不重试）。 */
function handleSignalIgnore(candidateId: number) {
  void sig.ignoreCandidate(props.boardId, candidateId)
}
function handleSignalRestore(candidateId: number) {
  void sig.restoreCandidate(props.boardId, candidateId)
}

/** 索引筛选 tab：已忽略视图读 ignored，其余 visible（只重拉列表）。 */
function handleVisibilityChange(visibility: 'visible' | 'ignored' | 'all') {
  void sig.setCandidateVisibility(props.boardId, visibility)
}

/** 筛选 tab（R-2）：visibility + view 提交服务端。
 * R-17：本函数必须保持在 script-setup 顶层——曾因少一个右花括号嵌套进
 * handleVisibilityChange，模板 @select-view 解析为 undefined、监听器被 prod
 * 静默丢弃（lint/typecheck/单测均未拦截），已忽略 tab 因此从不触发重拉。 */
function handleSelectView(payload: { visibility: 'visible' | 'ignored' | 'all'; view: 'all' | 'pending' | 'readable' }) {
  void sig.selectCandidateView(props.boardId, payload)
}

// 报告阅读激活时窄屏同步进入详情单栏（宽屏详情栏本就常驻）
watch(() => sig.activeReportId, (id) => {
  if (id !== null && isNarrow.value) workbenchDetailOpen.value = true
})

// ── 头部：任务态与「返回任务」入口（D6 任务可找回）────────────────────────
/** 研究任务在跑且其周期 ≠ 当前浏览周期时显示：点击切回任务所属周期。 */
const returnTaskVisible = computed(() =>
  sig.researchCandidateId !== null &&
  !!sig.researchTaskPeriod &&
  (sig.researchTaskPeriod !== sig.period || sig.researchTaskGranularity !== sig.granularity),
)
function handleReturnToTask() {
  void sig.returnToResearchTask(props.boardId)
}

// ── 详情栏（未读报告时：候选详情）────────────────────────────────────────
const detailCandidate = computed(() => sig.activeCandidate)
/** 详情动作互斥：发现/任一研究任务在跑时忽略/恢复/开始研究禁用（服务端 409 兜底）。 */
const detailTaskRunning = computed(() => sig.discoveryRunning || sig.researchCandidateId !== null)
const detailIgnored = computed(() => detailCandidate.value?.ignored_at != null)
/** 开始研究可点：待研究、未忽略、无任何任务（FE-2：一条一任务，服务端互斥）。 */
const detailCanResearch = computed(() =>
  detailCandidate.value !== null &&
  detailCandidate.value.status === 'pending' &&
  detailCandidate.value.ignored_at == null &&
  !detailTaskRunning.value,
)
/** 该候选的报告版本（真实列表按 source_signal_id 归属；旧列表 API 兼容）。 */
const detailVersions = computed(() => {
  const c = detailCandidate.value
  if (!c) return []
  return sig.reports.filter((r) => r.source_signal_id === c.id)
})
/** 「阅读报告」目标：最新版本优先；列表未及（加载失败等）回退候选行快照 id。 */
const detailReadResultId = computed<number | null>(() => {
  const latest = detailVersions.value[0]
  if (latest) return latest.id
  if (detailCandidate.value?.status === 'reported') return detailCandidate.value.latest_result_id
  return null
})
const detailHasReport = computed(() => detailReadResultId.value !== null)

function startDetailResearch() {
  const c = detailCandidate.value
  if (!c || !detailCanResearch.value) return
  handleSignalResearch(c.id, { regenerate: false })
}
function toggleDetailIgnore() {
  const c = detailCandidate.value
  if (!c || detailTaskRunning.value) return
  if (c.ignored_at != null) handleSignalRestore(c.id)
  else handleSignalIgnore(c.id)
}
function openDetailReport() {
  if (detailReadResultId.value !== null) handleOpenSignalReport(detailReadResultId.value)
}
function handleDetailRetry() {
  const id = sig.activeCandidateId
  if (id !== null) void sig.openCandidate(props.boardId, id)
}

function formatDiscoveredAt(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toISOString().slice(0, 10)
}

// ── 增强开关快捷入口（fix-board-analysis-material：开关可发现性）───────────
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
  // board 级同步启动（与 loadTopics 无依赖，保持挂载即拉）；loadTopics
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
  // topic 轮询，selectTopic/watch/bootstrap 三入口兑底），若 sync 在前，刚恢复
  // 的 running 轮询会被随后的 loadAll 误杀（poll 消失、triggering 归零，后台
  // 任务在前端失联）。sync 放在 boardLoads 之后收尾：bootstrap 内 sync 之后不
  // 存在任何 stopTopicPoll 路径，恢复的轮询跨出 bootstrap 存活；无 topic（列表
  // 空/加载失败 selectedTopicId=null）则 loadAll/sync 二者都不调。
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
// 显式换 topic 才再触发，不存在 sync 后再次误停恢复轮询的路径。同 topic 刷新
// 由 bootstrap 自身完成「加载表 + 恢复轮询」。
watch(selectedTopicId, (id) => { if (id !== null) void loadAllTopicTables(id) })
</script>

<template>
  <div class="ew-panel">
    <!-- ── 研究工作台（发现 → 候选索引 → 详情/报告，无评审） ──────────────── -->
    <section class="block board-analysis-main">
      <div class="block-head board-head">
        <h2 class="serif">研究</h2>
        <span class="helper">发现信号只保存候选 · 逐条深入研究 · 报告生成即可阅读</span>
        <span v-if="sig.discoveryRunning" class="bb-job-tag" data-testid="signal-discovery-tag">
          <Icon icon="mdi:loading" width="13" class="spin" />
          信号发现中…（不生成报告）
        </span>
        <span v-else-if="sig.researchCandidateId !== null" class="bb-job-tag" data-testid="signal-research-tag">
          <Icon icon="mdi:loading" width="13" class="spin" />
          {{ sig.researchProgressLabel }}
        </span>
        <button
          v-if="returnTaskVisible"
          type="button"
          class="ew-ghost-btn"
          data-testid="signal-return-task"
          title="研究任务在另一周期进行中，点击切回任务所属周期"
          @click="handleReturnToTask"
        >
          <Icon icon="mdi:target" width="14" />
          返回任务
        </button>
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

      <!-- 主从布局：左栏索引 / 右栏详情（各栏 min-width:0，超长不顶破页面） -->
      <AppPageShell mode="split" aside-side="left" as="div" class="workbench-shell">
        <template #aside>
          <div v-show="!isNarrow || !workbenchDetailOpen" class="workbench-index">
            <!-- 候选信号索引（周期工具栏 + 筛选 tab + 候选行；动作全部 emit） -->
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
              :reports="sig.reports"
              :visibility="sig.candidateVisibility"
              @commit="handleSignalCommit"
              @discover="handleSignalDiscover"
              @research="handleSignalResearch"
              @open-report="handleOpenSignalReport"
              @open-progress="handleSignalOpenProgress"
              @select="handleSignalSelect"
              @ignore="handleSignalIgnore"
              @restore="handleSignalRestore"
              @set-visibility="handleVisibilityChange"
              @select-view="handleSelectView"
            />
            <!-- 历史研究入口（只读，5.2 LegacyResearchHistory 接入） -->
            <div class="workbench-index-foot">
              <button
                type="button"
                class="ew-ghost-btn"
                data-testid="open-legacy-history"
                title="旧研究历史（只读）：简报/调查/旧版分析及其辩论与问答"
                @click="legacyHistoryOpen = true"
              >
                <Icon icon="mdi:history" width="14" />
                历史研究（只读）
              </button>
            </div>
          </div>
        </template>

        <!-- 右栏详情：报告阅读 → 候选详情 → 未选提示 -->
        <div v-show="!isNarrow || workbenchDetailOpen" class="workbench-detail" data-testid="workbench-detail">
          <button
            v-if="isNarrow && workbenchDetailOpen"
            type="button"
            class="ew-ghost-btn"
            data-testid="workbench-back-index"
            @click="workbenchDetailOpen = false"
          >
            <Icon icon="mdi:arrow-left" width="14" />
            返回列表
          </button>

          <!-- 报告阅读（reader≤760；返回回到候选详情/索引，周期保留） -->
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
          <template v-else-if="sig.activeCandidateId !== null">
            <div v-if="sig.activeCandidateLoading" class="workbench-detail-state" data-testid="workbench-detail-loading">
              <Icon icon="mdi:loading" width="14" class="spin" />
              候选详情加载中…
            </div>
            <div v-else-if="sig.activeCandidateError" class="workbench-detail-state is-error" role="alert" data-testid="workbench-detail-error">
              <Icon icon="mdi:alert-circle-outline" width="14" />
              <span>{{ sig.activeCandidateError }}</span>
              <button type="button" class="btn btn-ghost btn-sm" data-testid="workbench-detail-retry" @click="handleDetailRetry">重试</button>
            </div>
            <article v-else-if="detailCandidate" class="workbench-candidate" data-testid="workbench-candidate-detail">
              <h3 class="serif">{{ detailCandidate.signal }}</h3>
              <p class="wc-meta">
                发现于 {{ formatDiscoveredAt(detailCandidate.discovery_created_at) }} · {{ detailCandidate.period }}
                <template v-if="detailIgnored"> · 已忽略</template>
              </p>

              <div class="wc-block">
                <span class="wc-label">要查的问题</span>
                <p class="wc-text">{{ detailCandidate.research_question }}</p>
              </div>
              <div class="wc-block">
                <span class="wc-label">为什么值得查</span>
                <p class="wc-text">{{ detailCandidate.why_it_matters }}</p>
              </div>
              <template v-if="detailCandidate.research_brief">
                <div class="wc-block">
                  <span class="wc-label">还不清楚什么</span>
                  <p class="wc-text" data-testid="detail-brief-unknown">{{ detailCandidate.research_brief.unknown }}</p>
                </div>
                <div class="wc-block">
                  <span class="wc-label">准备先查什么</span>
                  <p class="wc-text" data-testid="detail-brief-verification">{{ detailCandidate.research_brief.verification }}</p>
                </div>
              </template>

              <div v-if="detailCandidate.evidence_refs.length" class="wc-block">
                <span class="wc-label">新闻依据（只读）</span>
                <ul class="wc-chips">
                  <li
                    v-for="refId in detailCandidate.evidence_refs"
                    :key="refId"
                    class="wc-chip"
                    title="新闻切片编号（原地展示，不跳转）"
                  >
                    新闻切片 #{{ refId }}
                  </li>
                </ul>
              </div>

              <div v-if="detailCandidate.related_topics.length || detailCandidate.related_gaps.length" class="wc-block">
                <span class="wc-label">相关话题</span>
                <ul v-if="detailCandidate.related_topics.length" class="wc-chips">
                  <li
                    v-for="t in detailCandidate.related_topics"
                    :key="t.topic_id"
                    class="wc-chip wc-chip--clickable"
                    :data-topic-status="t.frozen_status"
                    role="button"
                    tabindex="0"
                    data-testid="related-topic-chip"
                    title="点击只读预览该话题的新闻背景（管理在话题总览）"
                    @click="openTopicBackgroundPreview(t.topic_id, t.label)"
                    @keydown.enter.prevent="openTopicBackgroundPreview(t.topic_id, t.label)"
                  >
                    {{ t.label }}<template v-if="t.frozen_status !== 'active'">（{{ t.frozen_status }}）</template>
                    <button
                      type="button"
                      class="wc-chip-preview"
                      :data-testid="`related-topic-preview-${t.topic_id}`"
                      title="只读预览该话题的新闻背景（管理在话题总览）"
                      @click.stop="openTopicBackgroundPreview(t.topic_id, t.label)"
                    >背景预览</button>
                  </li>
                </ul>
                <p v-for="g in detailCandidate.related_gaps" :key="g" class="wc-gap">缺口：{{ g }}</p>
              </div>

              <p v-if="sig.researchCandidateId === detailCandidate.id" class="wc-progress" role="status">
                <Icon icon="mdi:loading" width="13" class="spin" />
                {{ sig.researchProgressLabel }}
              </p>

              <div class="wc-actions">
                <AppButton
                  v-if="detailCandidate.status === 'pending' && !detailIgnored"
                  variant="primary"
                  size="sm"
                  data-testid="signal-detail-research"
                  :disabled="!detailCanResearch"
                  :title="detailTaskRunning ? '发现/研究进行中，结束后才能开始新研究' : '逐条启动，不自动研究全部线索'"
                  @click="startDetailResearch"
                >
                  <Icon v-if="sig.researchCandidateId === detailCandidate.id" icon="mdi:loading" width="12" class="spin" />
                  开始研究
                </AppButton>
                <AppButton
                  v-if="detailHasReport"
                  variant="primary"
                  size="sm"
                  data-testid="signal-detail-read"
                  @click="openDetailReport"
                >
                  阅读报告
                </AppButton>
                <AppButton
                  variant="ghost"
                  size="sm"
                  data-testid="signal-detail-ignore"
                  :disabled="detailTaskRunning"
                  :title="detailTaskRunning ? '任务进行中，结束后才能忽略/恢复' : (detailIgnored ? '恢复到默认索引，不触发研究' : '忽略后从默认索引隐藏，可在「已忽略」中恢复')"
                  @click="toggleDetailIgnore"
                >
                  {{ detailIgnored ? '恢复' : '忽略' }}
                </AppButton>
                <span v-if="detailTaskRunning" class="wc-hint">发现/研究进行中，忽略与新增研究暂不可用</span>
                <span
                  v-else-if="detailCandidate.research_contract === 'question-v1'"
                  class="wc-hint"
                  data-testid="detail-research-hint-question"
                >新流程研究会先读冻结原文，并可能调用已配置的外部搜索；未配置时不影响本地解释</span>
                <span
                  v-else
                  class="wc-hint"
                  data-testid="detail-research-hint-legacy"
                >沿用发现时的原快照研究；想使用新流程（原文核查/外部检索），请重新发现</span>
              </div>

              <div v-if="detailVersions.length" class="wc-block wc-versions">
                <span class="wc-label">报告版本（{{ detailVersions.length }}）</span>
                <div class="wc-version-row">
                  <button
                    v-for="(v, i) in detailVersions"
                    :key="v.id"
                    type="button"
                    class="ew-ghost-btn"
                    data-testid="signal-version-btn"
                    :title="`研究时点 ${formatDiscoveredAt(v.created_at)}`"
                    @click="handleOpenSignalReport(v.id)"
                  >
                    版本 {{ i + 1 }} · {{ formatDiscoveredAt(v.created_at) }}
                  </button>
                </div>
              </div>
            </article>
          </template>
          <div v-else class="workbench-detail-state" data-testid="workbench-detail-empty">
            从左侧选择一条线索，查看问题、依据与相关话题；报告生成后在这里阅读。
          </div>
        </div>
      </AppPageShell>
    </section>

    <!-- 旧研究历史（只读 xl 弹层；零触发/追问/沉淀） -->
    <LegacyResearchHistory
      :board-id="boardId"
      :open="legacyHistoryOpen"
      @update:open="(v: boolean) => legacyHistoryOpen = v"
    />

    <!-- 相关话题只读背景预览（md=560；管理入口在话题总览） -->
    <AppDialog
      :model-value="backgroundPreview !== null"
      :title="`背景预览 · ${backgroundPreview?.label ?? ''}`"
      size="md"
      data-testid="topic-background-preview-dialog"
      @update:model-value="(v: boolean) => { if (!v) backgroundPreview = null }"
    >
      <TopicNewsBackground
        v-if="backgroundPreview"
        :topic-id="backgroundPreview.topicId"
        :topic-label="backgroundPreview.label"
        :semantic-board-id="boardId"
        readonly
      />
    </AppDialog>
  </div>
</template>

<style scoped>
.ew-panel { display: flex; flex-direction: column; gap: 1rem; }

/* ── 主从工作台（redesign-enrichment-research-workbench 4.1）──────────────
   索引栏固定 280px（768~1099px 收窄到 244px）；各栏 min-width:0，超长内容
   独立换行/滚动，不产生整页横向滚动。 ─────────────────────────────────── */
.workbench-shell { --aside-max: 280px; }
@media (min-width: 768px) and (max-width: 1099px) {
  .workbench-shell { --aside-min: 244px !important; --aside-max: 244px; }
}
.workbench-index { min-width: 0; display: flex; flex-direction: column; gap: 0.5rem; }
.workbench-index-foot { display: flex; justify-content: flex-end; padding-top: 0.25rem; border-top: 1px solid var(--color-border-subtle); }
.workbench-detail { min-width: 0; display: flex; flex-direction: column; gap: 0.6rem; overflow-wrap: anywhere; }
/* 详情正文铺满主栏剩余宽度（用户反馈 2026-10-16）：去掉 760px 帽，宽屏不再大片留白 */
.workbench-candidate { width: 100%; display: flex; flex-direction: column; gap: 0.55rem; }
/* 宽屏双栏定高内滚（用户反馈 2026-10-16）：工作台占视口剩余高，左右栏各自滚动，
 * 页面不再被长列表整体拉长。190px ≈ 顶栏+tab行+研究标题区实测偏移；窄屏分支不适用（保持页面滚）。 */
@media (min-width: 768px) {
  .workbench-shell { height: calc(100dvh - 190px); min-height: 420px; }
  .workbench-shell :deep(.app-page-shell__inner) { height: 100%; }
  .workbench-shell :deep(.app-page-shell__main),
  .workbench-shell :deep(.app-page-shell__aside) { height: 100%; overflow-y: auto; }
}
.workbench-candidate h3 { margin: 0; font-size: 1.15rem; line-height: 1.5; font-weight: 600; color: var(--color-text-primary); }
.workbench-detail-state {
  display: flex; align-items: center; gap: 0.45rem; flex-wrap: wrap;
  padding: 1.2rem 0.5rem; color: var(--color-text-muted); font-size: 0.85rem;
}
.workbench-detail-state.is-error { color: var(--color-error, var(--color-accent)); }
.wc-meta { margin: 0; font-size: 0.75rem; color: var(--color-text-muted); }
.wc-block { display: flex; flex-direction: column; gap: 0.3rem; }
.wc-label { font-size: 0.68rem; font-weight: 600; letter-spacing: 0.05em; color: var(--color-text-muted); }
.wc-text { margin: 0; font-size: 0.85rem; color: var(--color-text-primary); line-height: 1.7; }
.wc-gap { margin: 0; font-size: 0.78rem; color: var(--color-warning); line-height: 1.6; }
.wc-chips { list-style: none; display: flex; gap: 0.4rem; flex-wrap: wrap; margin: 0; padding: 0; }
.wc-chip {
  font-size: 0.72rem; padding: 0.12rem 0.55rem; border-radius: 999px;
  background: var(--color-bg-sunken); border: 1px solid var(--color-border-subtle);
  color: var(--color-text-secondary);
}
.wc-chip[data-topic-status='archived'] { color: var(--color-text-muted); }
/* 相关话题 chip 整体可点（2026-10-16 用户反馈）：点击开只读背景预览，键盘可达 */
.wc-chip--clickable { cursor: pointer; }
.wc-chip--clickable:hover { border-color: var(--color-accent); color: var(--color-text-primary); }
.wc-chip--clickable:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
.wc-progress { margin: 0; display: inline-flex; align-items: center; gap: 0.3rem; font-size: 0.78rem; color: var(--color-accent); }
.wc-actions { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.25rem; }
.wc-hint { font-size: 0.72rem; color: var(--color-text-muted); }
.wc-version-row { display: flex; gap: 0.4rem; flex-wrap: wrap; }

/* topic 工具条 */
.ew-spacer { flex: 1; }
.ew-ghost-btn { display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.2rem 0.5rem; border-radius: 6px; border: 1px solid var(--color-border-medium); background: var(--color-bg-sunken); color: var(--color-text-primary); font-size: 0.68rem; cursor: pointer; font-family: inherit; }
.ew-ghost-btn:hover { background: var(--color-bg-hover); }

/* 通用 btn */
.btn { display: inline-flex; align-items: center; gap: 6px; font-family: inherit; cursor: pointer; border: none; border-radius: 8px; font-weight: 600; font-size: 13px; padding: 7px 14px; transition: background 0.15s, opacity 0.15s, transform 0.1s; }
.btn:active { transform: translateY(1px); }
.btn-primary { background: var(--color-accent); color: #fff; }
.btn-primary:hover { background: var(--color-accent-hover); }
.btn-ghost { background: transparent; color: var(--color-text-secondary); border: 1px solid var(--color-border-medium); }
.btn-ghost:hover { background: var(--color-bg-hover); color: var(--color-text-primary); }
.btn-sm { padding: 4px 10px; font-size: 12px; }
.btn:disabled { opacity: 0.4; cursor: not-allowed; }

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

/* 区块 */
.block { margin-bottom: 1.5rem; }
.block-head { display: flex; align-items: baseline; gap: 0.6rem; margin-bottom: 0.9rem; padding-bottom: 0.4rem; border-bottom: 1px solid var(--color-border-subtle); }
.block-head h2 { font-size: 1.1rem; margin: 0; font-weight: 700; color: var(--color-text-primary); }
.block-head .helper { font-size: 12px; color: var(--color-text-muted); }
@media (max-width: 767.98px) {
  .board-head { flex-wrap: wrap; align-items: flex-start; gap: 0.35rem; }
  .board-head h2 { white-space: nowrap; }
  .board-head .helper { flex-basis: 100%; }
}

/* 任务 tag（发现/研究 running） */
.bb-job-tag {
  display: inline-flex; align-items: center; gap: 0.3rem;
  font-size: 0.72rem; color: var(--color-accent);
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
  padding: 0.15rem 0.55rem; border-radius: 99px; white-space: nowrap;
}
.bb-job-tag .spin { animation: bb-tag-spin 1s linear infinite; }
@keyframes bb-tag-spin { to { transform: rotate(360deg); } }

.muted { color: var(--color-text-muted); }
.serif { font-family: Georgia, "Songti SC", "SimSun", "Source Serif 4", serif; }
.spin { animation: bb-tag-spin 1s linear infinite; }

/* ── 窄屏适配（≤767.98px，统一断点；单栏切换由模板窄屏分支负责） ───────── */
@media (max-width: 767.98px) {
  .ew-panel { gap: 0.75rem; }

  /* 刷新/返回任务/历史入口按钮：hit-target ≥36px */
  .ew-ghost-btn { min-width: 36px; min-height: 36px; justify-content: center; }
}
</style>
