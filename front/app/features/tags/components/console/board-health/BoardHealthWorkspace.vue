<script setup lang="ts">
/**
 * 版块体检问题处理工作面（board-health-triage-advice seam C2 / tasks 3.2-3.3）。
 *
 * 形态：紧凑主从（顶部一行本轮待核对/进展；左侧约 240px 问题索引；右侧当前项处理区），
 * **不回退 v1 的四组彩色卡堆叠**。keep/insufficient 与评分统计趋势默认折叠但完整保留。
 *
 * 边界：
 *  - 分诊只来自既有体检快照（无独立分诊请求）；旧快照/空/降级/异常一律**不猜诊断**、
 *    不说"全部健康"；分诊有丢弃项时用一行提示说明数据不完整（不加信息卡）。
 *  - 描述修复走 C1 的 `useAuxiliaryDescriptionRepair`（真实四资源 API）：确认前只有 GET、
 *    零写入；结果未知保留同键同体，不另取 latest_task 冒充。
 *  - 组件**不直接调 API**、不做病型/统计公式；不新建弹窗/页面壳（本工作面由既有 1040px
 *    单级工作面承载，见 seam-c2 handoff 的 3.4 接入契约）。
 *
 * 生命周期：两个 composable 都在**本组件（根）**创建——编辑器/进度子组件只拿 props+emit，
 * 收起编辑器不会 dispose 掉轮询；根也不按 selectedAuxId 重挂子组件（避免丢未知冻结）。
 */
import { computed, ref, toRef, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import { useBoardHealth } from '../../../composables/useBoardHealth'
import { useAuxiliaryDescriptionRepair } from '../../../composables/useAuxiliaryDescriptionRepair'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import {
  normalizeBoardHealthLabelSignals,
  normalizeBoardHealthMagnetFlow,
  normalizeBoardHealthTriage,
} from '~/api/normalizers/boardHealthTriage'
import { validateRepairDescription, type AuxiliaryRepairContextView } from '~/api/normalizers/auxiliaryRepair'
import BoardHealthWorkspaceHead from './BoardHealthWorkspaceHead.vue'
import BoardHealthIssueIndex from './BoardHealthIssueIndex.vue'
import BoardHealthIssueDetail from './BoardHealthIssueDetail.vue'
import {
  boardHealthIssueStatus,
  boardHealthProgressText,
  boardHealthWorkspaceState,
  boardHealthWorkspaceStateCopy,
  splitTriageItems,
  type BoardHealthIssueStatus,
} from './boardHealthWorkspace'

const props = withDefaults(defineProps<{
  boardId: number
  boardLabel?: string
  /** 3.4 反馈：已真实摘除成功的辅助标签 ID（canonical 字符串）；未收到反馈前不标"已摘除"。 */
  removedAuxIds?: string[]
}>(), {
  boardLabel: '',
  removedAuxIds: () => [],
})

/**
 * 暴露给宿主（3.4）：同版块刷新体检快照。
 * 刷新失败时按 stale 处理（保留上次快照 + 提示行），不创建任何任务。
 */
defineExpose({ reload: () => health.load() })

const emit = defineEmits<{
  /**
   * 3.4 宿主注入既有 DELETE 链：`DELETE /semantic-boards/:boardId/composition/:auxId`
   * （auxId 为 canonical 字符串，宿主转数字调用）。未收到宿主真实成功反馈前不标"已摘除"。
   */
  'remove-composition': [auxiliaryLabelId: string]
}>()

const health = useBoardHealth(toRef(props, 'boardId'))
const repair = useAuxiliaryDescriptionRepair()
const narrow = useIsNarrowViewport()

const selectedAuxId = ref<string | null>(null)
/** 用户是否改过草稿（改过就不许被建议/上下文/刷新覆盖）。 */
const draftTouched = ref(false)
const confirmOpen = ref(false)
const removeConfirmOpen = ref(false)
/** 非 409 的提交失败（独立错误行；服务端 message 透传，不编文案）。 */
const submitFailure = ref<string | null>(null)
/** 窄屏：索引/详情切换（宽屏恒为并排，JS 分支只在窄屏生效）。 */
const detailView = ref(false)

// ── 派生（全部来自 C1 归一器；组件不重算口径）────────────────────────────
const triage = computed(() => normalizeBoardHealthTriage(health.snapshot.value))
const signals = computed(() => normalizeBoardHealthLabelSignals(health.snapshot.value))
const magnetFlow = computed(() => normalizeBoardHealthMagnetFlow(health.snapshot.value))

const labelOf = computed(() => {
  const map = new Map<string, string>()
  for (const signal of signals.value) map.set(signal.auxId, signal.label)
  return (auxId: string): string => map.get(auxId) ?? auxId
})

const statusOf = computed(() => (auxId: string): BoardHealthIssueStatus => {
  const task = repair.task.value
  if (!task) return 'todo'
  if (task.auxiliaryLabelId !== auxId) return 'todo'
  return boardHealthIssueStatus(task.status)
})

const split = computed(() => splitTriageItems(triage.value.items, labelOf.value, statusOf.value))
const issues = computed(() => split.value.issues)
const others = computed(() => split.value.others)

const selectedIssue = computed(() => issues.value.find((item) => item.auxId === selectedAuxId.value) ?? null)
const selectedSignal = computed(() => signals.value.find((signal) => signal.auxId === selectedAuxId.value) ?? null)

/** 工作面整体状态（判定与文案在纯函数里，便于单测）。 */
const workspaceState = computed(() => boardHealthWorkspaceState({
  loading: health.loading.value,
  error: health.error.value,
  errorStatus: health.errorStatus.value,
  snapshot: health.snapshot.value,
  triageAvailable: triage.value.available,
  triageCount: triage.value.items.length,
}))

/**
 * stale 提示只在**真刷新失败**时出现（网络/5xx）。
 * 404/403（版块可能已不存在）走 'gone' 形态，不得再说"刷新失败，显示上次快照"。
 */
const showStale = computed(() => health.staleNotice.value && workspaceState.value !== 'gone')

const stateCopy = computed(() => boardHealthWorkspaceStateCopy(workspaceState.value))
const stateTitle = computed(() => stateCopy.value.title)
const stateText = computed(() => stateCopy.value.text)

/**
 * 「本轮体检依据」折叠区：**只要有快照就渲染**（默认折叠）。
 * 正常轮也要能按需查看评分/统计/趋势（ui-design 与原型均如此）；异常轮同样保留原统计。
 */
const showBasis = computed(() => health.snapshot.value !== null)

const progressText = computed(() => boardHealthProgressText(repair.task.value))

const validationError = computed(() => {
  const issue = selectedIssue.value
  const context = repair.context.value
  if (!issue || issue.category !== 'rewrite') return null
  if (!context) return null
  const check = validateRepairDescription(context.label.label, repair.draft.value)
  return check.ok ? null : check.error
})

/** 不可执行原因（停用标签等）；非空时禁用编辑与提交。 */
const disabledReason = computed(() => {
  const context = repair.context.value
  if (!context) return null
  if (context.label.status !== 'active') return '该标签已停用，不能执行修复。'
  return null
})

const removed = computed(() => new Set(props.removedAuxIds ?? []))
const currentRemoved = computed(() => (selectedAuxId.value ? removed.value.has(selectedAuxId.value) : false))

const hasNext = computed(() => {
  const current = selectedAuxId.value
  if (!current) return false
  return issues.value.some((item) => item.auxId !== current && item.status !== 'completed')
})

// ── 交互 ────────────────────────────────────────────────────────────────
/** 选项序号：期间又选了别的项时，本次的迟到结果必须丢弃（不覆盖当前选择）。 */
let selectSeq = 0

/**
 * 该标签能否进入描述修复链（4.1 M1）：服务端修复链全程只认 auxiliary 标签。
 * 组合标签（composite）/类型未知没有修复入口（context 必然 404）——不拉必然
 * 失败的上下文，避免用户看到「共享范围读取失败」的死路。
 */
function repairEligible(auxId: string): boolean {
  const item = triage.value.items.find((entry) => entry.auxId === auxId)
  return item?.labelType === 'auxiliary'
}

async function selectIssue(auxId: string): Promise<void> {
  const seq = ++selectSeq
  // 重新选中同一项 = 重新核对（保留用户草稿）；选另一项 = 切换（重置草稿并预填建议）。
  const sameItem = selectedAuxId.value === auxId
  selectedAuxId.value = auxId
  confirmOpen.value = false
  removeConfirmOpen.value = false
  detailView.value = true
  if (!sameItem) {
    draftTouched.value = false
    // 先清草稿（不闪现上一项内容），再 await 实时上下文
    repair.setDraft('')
  }
  if (!repairEligible(auxId)) {
    // 只读项（组合标签等）：清掉上一项的任务/上下文（不残留别人的进度），
    // 证据照常展示，不给修复入口。
    repair.reset()
    return
  }
  await repair.loadContext(auxId)
  if (seq !== selectSeq) return
  // 预填建议：仅在用户没有改过草稿时（改过的不许被建议覆盖）。
  if (!sameItem && !draftTouched.value) {
    const suggestion = triage.value.items.find((item) => item.auxId === auxId)?.suggestion ?? ''
    repair.setDraft(suggestion)
  }
}

function onDraftInput(value: string): void {
  draftTouched.value = true
  repair.setDraft(value)
}

function requestConfirm(): void {
  submitFailure.value = null
  confirmOpen.value = true
}

function cancelConfirm(): void {
  confirmOpen.value = false
}

async function confirmSubmit(): Promise<void> {
  const result = await repair.submitRepair()
  if (result.ok) {
    confirmOpen.value = false
    submitFailure.value = null
    return
  }
  // 失败：行内确认**不静默关闭**，错误行呈现服务端 message（不编文案）
  submitFailure.value = result.error ?? '提交失败，请重试'
}

function requestRemove(): void {
  removeConfirmOpen.value = true
}

function cancelRemove(): void {
  removeConfirmOpen.value = false
}

function confirmRemove(): void {
  removeConfirmOpen.value = false
  if (selectedAuxId.value) emit('remove-composition', selectedAuxId.value)
}

async function retryTask(): Promise<void> {
  await repair.retryTask()
}

async function reloadContext(): Promise<void> {
  if (!selectedAuxId.value) return
  // 重新核对：拉最新上下文（草稿由 C1 保留，不被覆盖）。
  await repair.loadContext(selectedAuxId.value)
}

function resumePolling(): void {
  repair.resumePolling()
}

async function resend(): Promise<void> {
  // 重发**原确认内容**：严格原 key + 原 payload（用户草稿不参与、也不被清空）——
  // 不是新意图，也不换键重建。
  await repair.resendFrozen()
}

function goNext(): void {
  const current = selectedAuxId.value
  if (!current) return
  const next = issues.value.find((item) => item.auxId !== current && item.status !== 'completed')
  if (next) {
    void selectIssue(next.auxId)
    return
  }
  // 全部处理完：回到索引（不说"所有标签健康"）。
  detailView.value = false
}

function backToIndex(): void {
  detailView.value = false
}

// 切版块：清空当前选择与行内确认（冻结记录按标签保留在 C1 内）。
watch(() => props.boardId, () => {
  selectedAuxId.value = null
  submitFailure.value = null
  confirmOpen.value = false
  removeConfirmOpen.value = false
  detailView.value = false
  draftTouched.value = false
  repair.reset()
})
</script>

<template>
  <div class="bhw" :class="{ 'bhw--narrow': narrow }" data-testid="board-health-workspace">
    <BoardHealthWorkspaceHead
      :issue-count="issues.length"
      :progress-text="progressText"
      :state-title="stateTitle"
      :state-text="stateText"
      :abnormal="workspaceState !== 'normal'"
      :retryable="workspaceState === 'error'"
      :stale="showStale"
      :dropped="triage.dropped"
      :show-basis="showBasis"
      :snapshot="health.snapshot.value"
      :score="health.score.value"
      :tier="health.tier.value"
      :checked-at-text="health.checkedAtText.value"
      :trend-polyline="health.trendPolyline.value"
      :trend-summary="health.trendSummary.value"
      @retry="health.load"
    />

    <div
      v-if="workspaceState === 'normal'"
      class="bhw-flow"
      :class="{ 'bhw-flow--detail': narrow && detailView }"
      data-testid="bhw-flow"
    >
      <aside class="bhw-flow-index">
        <BoardHealthIssueIndex
          :issues="issues"
          :others="others"
          :selected-aux-id="selectedAuxId"
          :removed-aux-ids="props.removedAuxIds"
          @select="selectIssue"
        />
      </aside>
      <article class="bhw-flow-detail">
        <BoardHealthIssueDetail
          :issue="selectedIssue"
          :signal="selectedSignal"
          :magnet-flow="magnetFlow"
          :context="repair.context.value"
          :context-loading="repair.contextLoading.value"
          :context-error="repair.contextError.value"
          :task="repair.task.value"
          :task-error="repair.taskError.value"
          :submitting="repair.submitting.value"
          :retrying="repair.retrying.value"
          :submit-error="repair.submitError.value"
          :submit-failure="submitFailure"
          :conflict="repair.conflict.value"
          :submit-uncertain="repair.submitUncertain.value"
          :frozen-description="repair.frozenSubmit.value?.description ?? null"
          :poll-paused="repair.pollPaused.value"
          :draft="repair.draft.value"
          :validation-error="validationError"
          :disabled-reason="disabledReason"
          :removed="currentRemoved"
          :narrow="narrow"
          :has-next="hasNext"
          :confirm-open="confirmOpen"
          :remove-confirm-open="removeConfirmOpen"
          @update:draft="onDraftInput"
          @request-confirm="requestConfirm"
          @cancel-confirm="cancelConfirm"
          @confirm-submit="confirmSubmit"
          @request-remove="requestRemove"
          @cancel-remove="cancelRemove"
          @confirm-remove="confirmRemove"
          @retry="retryTask"
          @resume-poll="resumePolling"
          @reload-context="reloadContext"
          @resend="resend"
          @next="goNext"
          @back="backToIndex"
        />
      </article>
    </div>

    <div v-if="workspaceState === 'normal' && issues.length === 0" class="bhw-empty-all" data-testid="bhw-no-issues">
      本轮没有需要核对的问题项。
    </div>

    <!-- 窄屏粘性动作条（仅窄屏出现；宽屏零变化） -->
    <div v-if="narrow && detailView && selectedIssue" class="bhw-sticky" data-testid="bhw-sticky">
      <AppButton variant="ghost" size="sm" data-testid="bhw-sticky-back" @click="backToIndex">返回问题清单</AppButton>
    </div>
  </div>
</template>

<style scoped>
.bhw {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  min-width: 0;
}

.bhw-flow {
  display: grid;
  grid-template-columns: 238px minmax(0, 1fr);
  gap: 0.9rem;
  min-height: 0;
  align-items: start;
}

.bhw-flow-index,
.bhw-flow-detail {
  min-width: 0;
}

.bhw-empty-all {
  font-size: 0.76rem;
  color: var(--color-text-muted);
}

.bhw-sticky {
  position: sticky;
  bottom: 0;
  display: flex;
  gap: 0.4rem;
  padding-top: 0.4rem;
  padding-bottom: 0.2rem;
  background: var(--color-bg-base);
}

/* 窄屏：索引/详情互斥（断点 767.98px，与 Tailwind md 对齐） */
@media (max-width: 767.98px) {
  .bhw-flow {
    grid-template-columns: minmax(0, 1fr);
  }

  .bhw-flow:not(.bhw-flow--detail) .bhw-flow-detail {
    display: none;
  }

  .bhw-flow--detail .bhw-flow-index {
    display: none;
  }
}
</style>
