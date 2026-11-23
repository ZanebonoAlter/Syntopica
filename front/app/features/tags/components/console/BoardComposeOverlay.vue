<script setup lang="ts">
/**
 * 新建泳道编排态 · 三栏工作面宿主（rework-workshop-into-radar-and-desk / workshop-lane-compose）。
 *
 * 本组件是**同一挂载点**的重写：旧的「散装事件列表 + 相似补选 + 20 条上限」编排面被
 * 三栏工作面主动替换（契约 §3「旧能力去向」）。旧 `useInlineCompose.adopt` 的相似补选语义
 * 保留在 `useInlineCompose`（线程浏览器仍在用），编排态不复用。
 *
 * 布局：左栏候选泳道（原生复选框组/成员）｜中央 SVG 画布（容器真实像素 1:1、话题带 + 散点悬停）
 * ｜右栏三段（当前事件线索 / 可补充的事件 / tab：已选事件 + 按标题找）。
 *
 * 口径来源：`lane-compose-implementation-contract.md`（唯一口径）+ delta spec
 * `specs/workshop-lane-compose/spec.md`。窄屏（<768px）只提示宽屏/横屏，不画残缺工作台。
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import { Z_INDEX } from '~/components/ui/layout-contract'
import { unclassifiedIds, type LaneThresholdTier } from '~/features/tags/composables/laneComposeLogic'
import LaneComposeCandidates from '~/features/tags/components/compose/LaneComposeCandidates.vue'
import LaneComposeCanvas from '~/features/tags/components/compose/LaneComposeCanvas.vue'
import LaneComposeHistoryDialog from '~/features/tags/components/compose/LaneComposeHistoryDialog.vue'
import LaneComposeInspector from '~/features/tags/components/compose/LaneComposeInspector.vue'
import LaneAnchorScope from '~/features/tags/components/console/LaneAnchorScope.vue'
import LaneCorrectionCanvas from '~/features/tags/components/console/LaneCorrectionCanvas.vue'
import LaneMemberList from '~/features/tags/components/console/LaneMemberList.vue'
import LaneRecommendationList from '~/features/tags/components/console/LaneRecommendationList.vue'
import ObservationRangeControls from '~/features/tags/components/console/ObservationRangeControls.vue'
import { useLaneCompose, type LaneComposeConfirmRequest } from '~/features/tags/composables/useLaneCompose'
import { useLaneCorrection } from '~/features/tags/composables/useLaneCorrection'
import type { ManualTopicResult } from '~/api/persistentTopics'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import { useNotify } from '~/composables/useNotify'

const props = defineProps<{
  boardId: number
  boardLabel: string
  /** 工作面模式：新建（默认，既有行为）/ 修正（既有泳道的就地修正）。 */
  mode?: 'create' | 'correction'
  /** 修正模式的目标泳道（持久话题）id。 */
  topicId?: number
  /** 修正模式的泳道名（标题可就地编辑）。 */
  laneLabel?: string
}>()

const emit = defineEmits<{
  close: []
  saved: [topic?: ManualTopicResult]
  /** 修正模式：泳道名就地编辑已保存。 */
  renamed: [label: string]
}>()

/** 修正模式（既有泳道）；新建模式行为不变。 */
const isCorrection = computed(() => props.mode === 'correction')

const isNarrowViewport = useIsNarrowViewport()
const isNarrow = computed(() => Boolean(isNarrowViewport.value))
const { success: notifySuccess, warn: notifyWarn, error: notifyError } = useNotify()

// ── 草稿确认弹窗（切窗口 / 取消本次） ─────────────────────────────────────
const confirmOpen = ref(false)
const confirmRequest = ref<LaneComposeConfirmRequest | null>(null)
let confirmResolver: ((value: boolean) => void) | null = null

function requestConfirm(request: LaneComposeConfirmRequest): Promise<boolean> {
  confirmRequest.value = request
  confirmOpen.value = true
  return new Promise<boolean>((resolve) => { confirmResolver = resolve })
}

function settleConfirm(ok: boolean): void {
  const resolver = confirmResolver
  confirmResolver = null
  confirmOpen.value = false
  resolver?.(ok)
}

function onConfirmDialogUpdate(open: boolean): void {
  if (!open) settleConfirm(false)
}

// ── 命名弹窗（命名在最后） ────────────────────────────────────────────────
const namingOpen = ref(false)
const nameDraft = ref('')

function openNaming(): void {
  if (!compose.memberCount.value) return
  nameDraft.value = compose.laneName.value
  compose.nameError.value = null
  namingOpen.value = true
}

function onNameInput(value: string | number): void {
  const text = String(value ?? '')
  nameDraft.value = text
  compose.setLaneName(text)
}

function closeNaming(): void {
  // 关闭命名保留草稿（成员与已写下的名字都不丢）
  namingOpen.value = false
}

function onNamingDialogUpdate(open: boolean): void {
  if (!open) closeNaming()
}

async function confirmCreate(): Promise<void> {
  const ok = await compose.save(nameDraft.value)
  if (ok) {
    namingOpen.value = false
  }
}

// ── 历史待启用话题（管理名单） ────────────────────────────────────────────
const historyOpen = ref(false)

/** 打开入口：拉管理名单（服务端可见性，前端不放宽）。 */
async function openHistory(): Promise<void> {
  historyOpen.value = true
  await compose.loadHistoryTopics()
}

/**
 * 「带入草稿」：只带该话题在当前窗口内的 owned 成员（不补相似、不截条）。
 * 名称预填/保留手写名的口径在 composable；这里只给明确反馈。
 */
function onHistoryAdopt(topicId: string): void {
  const outcome = compose.adoptHistoryTopic(topicId)
  if (!outcome) {
    notifyError('话题不存在或已从名单移除')
    return
  }
  if (outcome.ownedCount === 0) {
    notifyWarn(`当前日期范围内没有「${outcome.topicLabel}」的事件，未带入草稿。`)
    return
  }
  if (outcome.keptManualName) {
    notifySuccess(`已保留你写的名称，未覆盖；新增预选 ${outcome.addedCount} 条。`)
  } else {
    notifySuccess(`已预填名称，保留原先选择；新增预选 ${outcome.addedCount} 条。`)
  }
  historyOpen.value = false
}

/** 「直接启用原话题」：改原话题状态（与本次草稿的人工新建是两件事）。 */
async function onHistoryActivate(topicId: string): Promise<void> {
  const outcome = await compose.activateHistoryTopic(topicId)
  if (outcome.ok) {
    notifySuccess(`${outcome.message}；这一步只改原话题的状态，与本次草稿的人工新建无关。`)
  } else {
    notifyError(outcome.message)
  }
}

// ── 编排态 ────────────────────────────────────────────────────────────────
function handleSaved(topic?: ManualTopicResult): void {
  emit('saved', topic)
  emit('close')
}

const compose = useLaneCompose({
  boardId: computed(() => props.boardId),
  requestConfirm,
  onSaved: handleSaved,
})

// ── 阈值三档（画布工具条控件；与右栏「可补充的事件」共用同一状态） ────────
const THRESHOLD_TIERS: { key: LaneThresholdTier, label: string }[] = [
  { key: 'tight', label: '仅高相关' },
  { key: 'standard', label: '标准' },
  { key: 'all', label: '全部' },
]

function setThresholdTier(tier: LaneThresholdTier): void {
  compose.thresholdTier.value = tier
}

// ── 检索态（口径 = 未归类；范围与分析都在前端纯派生） ──────────────────────
/**
 * 检索范围 = **未归类**（不属于画布上任何候选链的节点，含未归属话题 +
 * 挂着 candidate 话题但未成链）；候选链成员不进检索结果（它们靠拖行标签整组采纳）。
 */
const unclassified = computed(() => unclassifiedIds(compose.sections.value, compose.candidateChains.value))
const searchQuery = ref('')
const searchActive = computed(() => searchQuery.value.trim().length > 0)
/** 命中集 = 未归类层里按标题 `includes` 命中的节点（画布与右栏同一批）。 */
const searchHits = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const hits = new Set<string>()
  if (!q) return hits
  for (const section of compose.sections.value) {
    if (!unclassified.value.has(section.id)) continue
    if (section.clusterLabel.toLowerCase().includes(q)) hits.add(section.id)
  }
  return hits
})

/** 整组采纳（拖行标签）：与左栏组勾选同一个写入口（含自动预填名口径）。 */
function onAdoptChain(key: string): void {
  compose.setChainSelected(key, true)
}

// ── 修正模式（既有泳道的就地修正） ─────────────────────────────────────────
/**
 * 修正模式的状态机（`useLaneCorrection`）：成员移除 / 加回 / 加入、首义范围、改名。
 *
 * 与新建模式的草稿**完全隔离**：修正模式不碰 `useLaneCompose` 的任何状态
 * （`selectedIds` / `laneName` / 候选链），两者互不影响（workshop-lane-compose 契约）。
 */
const laneTitle = ref(props.laneLabel ?? '')

watch(() => props.laneLabel, (value) => {
  if (!nameEditing.value) laneTitle.value = value ?? ''
})

const correction = useLaneCorrection({
  boardId: computed(() => props.boardId),
  topicId: computed(() => props.topicId ?? 0),
  laneLabel: laneTitle,
  onRenamed: (label) => emit('renamed', label),
})

/** 首义节点 id 列表（成员行的「首义」标）。 */
const anchorIdList = computed(() => correction.anchorNodes.value.map(node => node.id))
/** 成员 id 列表（首义池内的身份标注）。 */
const memberIdList = computed(() => correction.memberNodes.value.map(node => node.id))
/** 已勾选的首义范围节点（与画布折叠带同一份）。 */
const anchorSelectionList = computed(() => [...correction.anchorSelection.value])
/** 正在提交的节点（按钮 loading）。 */
const pendingIdList = computed(() => [...correction.pendingIds.value])
/** 画布空态（只有「无节点可展示」走画布；「无成员」走左栏）。 */
const canvasEmptyNote = computed(() => (
  correction.emptyKind.value === 'no-nodes' ? '该日期范围内没有可展示的事件。' : null
))

// ── 泳道名就地编辑（标题处点文字变输入框；零弹窗） ─────────────────────────
const nameEditing = ref(false)
const laneNameDraft = ref('')
const nameSaving = ref(false)
const nameWrap = ref<HTMLElement | null>(null)

function startNameEdit(): void {
  laneNameDraft.value = laneTitle.value
  nameEditing.value = true
  void nextTick(() => {
    const input = nameWrap.value?.querySelector('input')
    input?.focus()
    input?.select()
  })
}

async function commitNameEdit(): Promise<void> {
  if (nameSaving.value) return
  nameSaving.value = true
  const ok = await correction.commitRename(laneNameDraft.value)
  nameSaving.value = false
  if (ok) nameEditing.value = false
}

function cancelNameEdit(): void {
  if (nameSaving.value) return
  nameEditing.value = false
  laneNameDraft.value = laneTitle.value
}

function onNameKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter') {
    event.preventDefault()
    void commitNameEdit()
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    // 就地编辑的 Esc 只取消编辑：必须阻断冒泡，否则 window 级 Esc 监听
    // （此时 nameEditing 已置 false）会把整个修正工作面一起关掉（2026-11-15 实测）。
    event.stopPropagation()
    cancelNameEdit()
  }
}

/** 失焦取消（改写未保存就还原标题）。 */
function onNameFocusOut(): void {
  if (nameSaving.value) return
  cancelNameEdit()
}

// ── 窗口控件（ObservationRangeControls 复用：模态本地，不写 router/query） ────
/** 新建模式：档位切换（7/14/30/365；OQ-2 后 365 可用）。 */
async function onComposeSourceChange(days: number): Promise<void> {
  await compose.setQuickDays(days as 7 | 14 | 30 | 365)
}

/** 修正模式：档位切换。 */
async function onCorrectionSourceChange(days: number): Promise<void> {
  await correction.setWindow(days as 7 | 14 | 30 | 365)
}

function onToggleAssigned(event: Event): void {
  compose.showAssigned.value = (event.target as HTMLInputElement).checked
}

// ── 退出 ──────────────────────────────────────────────────────────────────
async function onCancel(): Promise<void> {
  // 修正模式的改动逐条即时落库，没有「未保存草稿」，直接退出。
  if (isCorrection.value) {
    emit('close')
    return
  }
  const ok = await compose.cancel()
  if (ok) emit('close')
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  // 就地改名中：Esc 归输入框自己处理（先取消编辑，不退出工作面）。
  if (nameEditing.value) return
  if (isCorrection.value) {
    emit('close')
    return
  }
  if (confirmOpen.value || namingOpen.value) return
  // 历史名单弹窗自己处理 Esc（AppDialog 关掉它，不连带退出编排态）
  if (historyOpen.value) {
    historyOpen.value = false
    return
  }
  // 检索态：Esc 先退出检索（不改成员），再按一次才退出编排态
  if (searchActive.value) {
    searchQuery.value = ''
    return
  }
  void onCancel()
}

let previousOverflow: string | null = null

onMounted(async () => {
  if (isNarrow.value) return
  previousOverflow = document.body.style.overflow
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', onKeydown)
  if (isCorrection.value) {
    await correction.load()
    void correction.reloadCurve() // 365 日本泳道曲线（独立 payload，失败可单独重试）
    return
  }
  await compose.load()
  void compose.loadPoolCurve() // 365 日可选池曲线（轻量：omit_embeddings=1）
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  if (previousOverflow !== null) {
    document.body.style.overflow = previousOverflow
    previousOverflow = null
  }
})
</script>

<template>
  <Teleport to="body">
    <!-- 窄屏：只提示宽屏/横屏，不画残缺工作台 -->
    <div v-if="isNarrow" class="bco-narrow" :data-testid="isCorrection ? 'lane-correction-narrow' : 'board-compose-narrow'">
      <h2 class="bco-narrow__title">{{ isCorrection ? '泳道修正需要宽屏或横屏' : '新建泳道需要宽屏或横屏' }}</h2>
      <p class="bco-narrow__text">请在宽屏或横屏上打开。</p>
    </div>

    <div
      v-else
      class="bco"
      :class="{ 'bco--correction': isCorrection }"
      data-testid="board-compose"
      role="dialog"
      aria-modal="true"
      :aria-label="`${isCorrection ? `修正泳道 · ${laneTitle}` : `新建泳道 · ${boardLabel}`}`"
    >
      <header class="bco__top">
        <button type="button" class="bco__back" data-testid="compose-cancel" @click="onCancel">← 返回</button>
        <h2 class="bco__title">
          <!-- 修正模式：泳道名就地编辑（点文字变输入框，Enter 保存 / Esc 取消 / 失焦取消，零弹窗） -->
          <template v-if="isCorrection">
            <button
              v-if="!nameEditing"
              type="button"
              class="bco__lane-name"
              data-testid="lane-correction-name"
              @click="startNameEdit"
            >{{ laneTitle }}</button>
            <div
              v-else
              ref="nameWrap"
              class="bco__name-edit"
              @keydown="onNameKeydown"
              @focusout="onNameFocusOut"
            >
              <AppInput
                input-id="lane-correction-name-input"
                :model-value="laneNameDraft"
                maxlength="120"
                data-testid="lane-correction-name-input"
                @update:model-value="laneNameDraft = String($event ?? '')"
              />
            </div>
          </template>
          <template v-else>新建泳道</template>
        </h2>
        <span v-if="isCorrection" class="bco__window-label" data-testid="lane-correction-window-label">{{ correction.windowLabel.value }}</span>
        <span v-else class="bco__window-label" data-testid="lane-compose-window-label">{{ compose.windowLabel.value }}</span>

        <!-- 修正模式：生效时机结论型徽标，常驻标题行右侧（只给结论，不解释机制） -->
        <div v-if="isCorrection" class="bco__effect-badges" data-testid="lane-correction-effect-badges">
          <span class="bco__timing bco__timing--now">立即生效</span>
          <span class="bco__timing bco__timing--next">次日日报生效</span>
          <span class="bco__timing bco__timing--plain">无需保存</span>
        </div>
      </header>

      <!-- 时间视窗（与矩阵页同一控件；模态内本地 state，不写 URL query） -->
      <div v-if="isCorrection" class="bco__range" data-testid="lane-correction-range">
        <ObservationRangeControls
          :bounds="correction.curveBounds.value"
          :range="correction.windowRange.value"
          :source-days="correction.tierDays.value"
          :series="correction.curveSeries.value"
          :loading="correction.curveLoading.value"
          :notice="correction.windowError.value"
          :tiers="[7, 14, 30, 365]"
          :max-span-days="365"
          :caption="['365 日 · 本泳道逐日线索数（历史参考）', '线索数（非文章数）· 曲线为历史参考，可编排范围见反馈行']"
          @preview="range => correction.previewWindow(range)"
          @commit="range => correction.commitWindow(range)"
          @source-change="onCorrectionSourceChange"
        />
        <p v-if="correction.curveError.value" class="bco__range-error" role="alert" data-testid="lane-correction-curve-error">
          {{ correction.curveError.value }}
          <button type="button" data-testid="lane-correction-curve-retry" @click="correction.reloadCurve()">重试历史池</button>
        </p>
        <p v-else-if="correction.curveTargetKnown.value === false" class="bco__range-error" data-testid="lane-correction-curve-unknown">
          本泳道曲线未知（目标泳道未返回；不影响成员与首义）。
        </p>
      </div>
      <div v-else class="bco__range" data-testid="lane-compose-range">
        <ObservationRangeControls
          :bounds="compose.poolBounds.value"
          :range="compose.observationRange.value"
          :source-days="compose.tierDays.value"
          :series="compose.poolCurve.value"
          :loading="compose.poolCurveLoading.value"
          :notice="compose.dateError.value"
          :tiers="[7, 14, 30, 365]"
          :max-span-days="365"
          :caption="['365 日 · 可选线索数（不含 active 泳道）', '线索数（非文章数）· 不含 active 泳道']"
          @preview="range => compose.previewObservation(range)"
          @commit="range => compose.commitObservation(range)"
          @source-change="onComposeSourceChange"
        />
        <p v-if="compose.poolCurveError.value" class="bco__range-error" role="alert" data-testid="lane-compose-curve-error">
          {{ compose.poolCurveError.value }}
          <button type="button" data-testid="lane-compose-curve-retry" @click="compose.loadPoolCurve()">重试历史池</button>
        </p>
      </div>

      <div class="bco__tools">
        <template v-if="isCorrection">
          <button
            type="button"
            class="bco__tool"
            :disabled="!correction.focusId.value"
            data-testid="lane-correction-clear-focus"
            @click="correction.clearFocus()"
          >取消聚焦</button>
        </template>
        <template v-else>
          <label class="bco__tool">
            <input
              type="checkbox"
              :checked="compose.showAssigned.value"
              data-testid="lane-compose-show-assigned"
              @change="onToggleAssigned"
            >
            显示已有泳道（只读）
          </label>
          <button
            type="button"
            class="bco__tool bco__tool--entry"
            data-testid="lane-compose-history-open"
            @click="openHistory"
          >历史待启用话题</button>
          <button
            type="button"
            class="bco__tool"
            :disabled="!compose.focusId.value"
            data-testid="lane-compose-clear-focus"
            @click="compose.clearFocus()"
          >取消聚焦</button>
          <div class="bco__tiers" role="group" aria-label="散料阈值（只影响散料显隐）">
            <button
              v-for="tier in THRESHOLD_TIERS"
              :key="tier.key"
              type="button"
              class="bco__tier"
              :class="{ 'is-active': compose.thresholdTier.value === tier.key }"
              :aria-pressed="compose.thresholdTier.value === tier.key"
              :data-testid="`lane-compose-tier-${tier.key}`"
              @click="setThresholdTier(tier.key)"
            >{{ tier.label }}</button>
          </div>
          <span class="bco__tool-note" data-testid="lane-compose-tool-note">
            已选 {{ compose.memberCount.value }} 条 · 未归类 {{ unclassified.size }} 条
          </span>
          <span v-if="searchActive" class="bco__tool-note" data-testid="lane-compose-search-state">检索态 · 候选链层降为背景</span>
        </template>
      </div>

      <!-- 修正模式：三栏工作面（左 380 已选侧 / 中央画布自适应 / 右 300 待选侧；两侧内容不重复） -->
      <AppPageShell v-if="isCorrection" mode="workspace" as="div" :gutter="0" class="bco__shell">
        <div class="bco__workbench bco__workbench--correction">
          <aside
            class="bco__left"
            data-testid="lane-correction-left"
            aria-label="本泳道成员、更早成员、已移出（本次）与首义范围"
          >
            <LaneMemberList
              :members="correction.memberNodes.value"
              :early-members="correction.earlyMembers.value"
              :total-count="correction.totalMemberCount.value"
              :removed="correction.removedNodes.value"
              :anchor-ids="anchorIdList"
              :pending-ids="pendingIdList"
              @remove="id => correction.removeMember(id)"
              @restore="id => correction.restoreMember(id)"
              @focus="id => correction.setFocus(id)"
            />
            <LaneAnchorScope
              :source-label="correction.anchorSourceLabel.value"
              :manual="correction.anchorSource.value === 'manual'"
              :pool="correction.anchorPool.value"
              :member-ids="memberIdList"
              :anchor-ids="anchorIdList"
              :selection="anchorSelectionList"
              :applying="correction.pendingIds.value.size > 0"
              :window-count="correction.memberNodes.value.length"
              :total-count="correction.totalMemberCount.value"
              @toggle="id => correction.toggleAnchorNode(id)"
              @apply="correction.requestAnchorApply()"
            />
          </aside>

          <LaneCorrectionCanvas
            class="bco__correction-canvas"
            :lane-label="laneTitle"
            :dates="correction.dates.value"
            :member-nodes="correction.memberNodes.value"
            :candidate-nodes="correction.candidateNodes.value"
            :readonly-nodes="correction.readonlyNodes.value"
            :removed-nodes="correction.removedNodes.value"
            :fold-nodes="correction.earlierAnchorNodes.value"
            :anchor-selection="anchorSelectionList"
            :focus-id="correction.focusId.value"
            :loading="correction.loading.value"
            :error="correction.loadError.value"
            :empty-note="canvasEmptyNote"
            :confirm-visible="correction.anchorConfirmOpen.value"
            :source-label="correction.anchorSourceLabel.value"
            :match-threshold="correction.matchThreshold.value"
            :relations="correction.relations.value"
            :pending-ids="pendingIdList"
            @focus="id => correction.setFocus(id)"
            @add="id => correction.addMember(id)"
            @remove="id => correction.removeMember(id)"
            @restore="id => correction.restoreMember(id)"
            @toggle-anchor="id => correction.toggleAnchorNode(id)"
            @confirm-apply="correction.applyAnchor()"
            @cancel-apply="correction.cancelAnchorApply()"
            @retry="correction.load()"
          />

          <aside class="bco__right-correction" data-testid="lane-correction-right" aria-label="关联推荐（待选侧）">
            <LaneRecommendationList
              :recommendations="correction.candidateNodes.value"
              :pending-ids="pendingIdList"
              :error="correction.loadError.value"
              @add="id => correction.addMember(id)"
              @focus="id => correction.setFocus(id)"
            />
          </aside>
        </div>
      </AppPageShell>

      <p v-else-if="compose.loading.value && !compose.sections.value.length" class="bco__state" data-testid="lane-compose-loading">
        加载中…
      </p>
      <div v-else-if="compose.error.value && !compose.sections.value.length" class="bco__state" data-testid="lane-compose-error">
        <p>{{ compose.error.value }}</p>
        <button type="button" data-testid="lane-compose-retry" @click="compose.load()">重试</button>
      </div>

      <AppPageShell v-else mode="workspace" as="div" :gutter="0" class="bco__shell">
        <div class="bco__workbench">
          <LaneComposeCandidates
            :chains="compose.candidateChains.value"
            :sections="compose.sections.value"
            :selected-ids="compose.selectedIds.value"
            :loading="compose.loading.value"
            @toggle-chain="(key, checked) => compose.setChainSelected(key, checked)"
            @toggle-member="(id, checked) => compose.toggleMember(id, checked)"
            @cancel-chain="key => compose.cancelChain(key)"
          />

          <LaneComposeCanvas
            class="bco__canvas"
            :sections="compose.sections.value"
            :relations="compose.relations.value"
            :chains="compose.candidateChains.value"
            :selected-ids="compose.selectedIds.value"
            :focus-id="compose.focusId.value"
            :match-threshold="compose.matchThreshold.value"
            :show-assigned="compose.showAssigned.value"
            :dates="compose.dates.value"
            :threshold-tier="compose.thresholdTier.value"
            :search-active="searchActive"
            :search-hits="searchHits"
            :undo-id="compose.lastRemoval.value?.id ?? null"
            @focus="id => compose.toggleFocus(id)"
            @clear-focus="compose.clearFocus()"
            @toggle-member="(id, checked) => compose.toggleMember(id, checked)"
            @adopt-chain="onAdoptChain"
            @undo-removal="compose.undoRemoval()"
          />

          <div class="bco__right">
            <LaneComposeInspector
              class="bco__inspector"
              :focus-section="compose.focusSection.value"
              :recommendations="compose.recommendations.value"
              :selected-sections="compose.selectedSections.value"
              :selected-ids="compose.selectedIds.value"
              :sections="compose.sections.value"
              :unclassified-ids="unclassified"
              :query="searchQuery"
              :show-assigned="compose.showAssigned.value"
              @add="id => compose.toggleMember(id, true)"
              @remove="id => compose.toggleMember(id, false)"
              @focus="id => compose.setFocus(id)"
              @update:query="searchQuery = $event"
            />
            <footer class="bco__actions">
              <button type="button" class="bco__cancel" data-testid="compose-cancel-draft" @click="onCancel">取消本次</button>
              <AppButton
                :disabled="!compose.memberCount.value"
                :loading="compose.saving.value"
                data-testid="compose-save"
                @click="openNaming"
              >起名并创建 →</AppButton>
              <span class="bco__actions-note">人工创建 · 初始向量取全部有效已选成员的平均值</span>
            </footer>
          </div>
        </div>
      </AppPageShell>

      <!-- 历史待启用话题（管理名单）：工作面内的明确入口 -->
      <LaneComposeHistoryDialog
        :model-value="historyOpen"
        :topics="compose.filteredHistoryTopics.value"
        :loading="compose.historyLoading.value"
        :error="compose.historyError.value"
        :query="compose.historyQuery.value"
        :activating-id="compose.activatingTopicId.value"
        :z-index="Z_INDEX.dialogNested"
        @update:model-value="historyOpen = $event"
        @update:query="compose.historyQuery.value = $event"
        @adopt="onHistoryAdopt"
        @activate="onHistoryActivate"
      />

      <!-- 草稿确认 -->
      <AppDialog
        :model-value="confirmOpen"
        size="sm"
        :title="confirmRequest?.title ?? '确认'"
        @update:model-value="onConfirmDialogUpdate"
      >
        <p class="bco__dialog-text">{{ confirmRequest?.message }}</p>
        <template #footer>
          <button type="button" data-testid="compose-confirm-cancel" @click="settleConfirm(false)">
            {{ confirmRequest?.cancelText ?? '继续编排' }}
          </button>
          <AppButton variant="danger" data-testid="compose-confirm-ok" @click="settleConfirm(true)">
            {{ confirmRequest?.confirmText ?? '确认' }}
          </AppButton>
        </template>
      </AppDialog>

      <!-- 命名（命名在最后） -->
      <AppDialog
        :model-value="namingOpen"
        size="md"
        title="给这条线起个名字"
        @update:model-value="onNamingDialogUpdate"
      >
        <div class="bco__name">
          <label class="bco__name-label" for="lane-compose-name">泳道名称</label>
          <AppInput
            input-id="lane-compose-name"
            :model-value="nameDraft"
            maxlength="120"
            placeholder="为你选中的事件命名"
            data-testid="compose-name-input"
            @update:model-value="onNameInput"
          />
          <p v-if="compose.nameError.value" class="bco__error" role="alert" data-testid="compose-name-error">
            {{ compose.nameError.value }}
          </p>
          <p class="bco__dialog-text">
            {{ compose.memberCount.value }} 条事件 · 起始向量取全部有效已选成员的平均值，不要求已有算法连线。
          </p>
        </div>
        <template #footer>
          <button type="button" data-testid="compose-name-close" @click="closeNaming">继续编排</button>
          <AppButton :loading="compose.saving.value" data-testid="compose-name-confirm" @click="confirmCreate">确认创建</AppButton>
        </template>
      </AppDialog>
    </div>
  </Teleport>
</template>

<style scoped>
.bco,
.bco-narrow {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  flex-direction: column;
  background: var(--color-bg-base);
  color: var(--color-text-primary);
  overflow: hidden;
}

.bco-narrow {
  align-items: center;
  justify-content: center;
  padding: 24px;
  text-align: center;
  gap: 8px;
}

.bco-narrow__title {
  margin: 0;
  font-size: 18px;
}

.bco-narrow__text {
  margin: 0;
  color: var(--color-text-muted);
  font-size: 13px;
}

.bco__top {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 12px 20px;
  border-bottom: 1px solid var(--color-border-medium);
  flex: none;
}

.bco__back {
  border: 0;
  background: transparent;
  padding: 4px 0;
  color: var(--color-text-secondary);
  font-size: 13px;
  cursor: pointer;
}

.bco__title {
  margin: 0;
  font-size: 17px;
  font-weight: 600;
}

.bco__window-label {
  color: var(--color-text-muted);
  font-size: 11px;
}

.bco__error {
  margin: 0;
  color: var(--color-error);
  font-size: 11px;
}

.bco__tools {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 8px 20px;
  border-bottom: 1px solid var(--color-border-subtle);
  font-size: 12px;
  color: var(--color-text-secondary);
  flex: none;
}

.bco__tool {
  display: flex;
  align-items: center;
  gap: 6px;
  border: 0;
  background: transparent;
  color: inherit;
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}

.bco__tool:disabled {
  color: var(--color-text-muted);
  cursor: not-allowed;
}

/* 明确入口（历史待启用话题）：与普通工具按钮区分 */
.bco__tool--entry {
  color: var(--color-accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}

/* 阈值三档（画布工具条内联控件；样式沿用既有档位按钮组） */
.bco__tiers {
  display: inline-flex;
  align-items: center;
  gap: 0;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  overflow: hidden;
}

.bco__tier {
  border: 0;
  border-right: 1px solid var(--color-border-medium);
  background: transparent;
  color: var(--color-text-muted);
  font-size: 11.5px;
  padding: 3px 9px;
  cursor: pointer;
}

.bco__tier:last-child {
  border-right: 0;
}

.bco__tier.is-active {
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
  color: var(--color-accent);
  font-weight: 600;
}

.bco__tool-note {
  margin-left: auto;
  color: var(--color-text-muted);
  font-size: 11px;
}

.bco__state {
  margin: 0;
  padding: 40px 20px;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 13px;
}

.bco__shell {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* 让 page shell 的内层铺满可用高度（工作面要填满视口，不被内容高度塑形） */
.bco__shell :deep(.app-page-shell__inner),
.bco__shell :deep(.app-page-shell__main) {
  height: 100%;
  min-height: 0;
}

.bco__workbench {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr) 320px;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

/* 修正模式：三栏（左 380 已选 / 中央自适应 / 右 300 待选；unify-lane-workbench D5） */
.bco__workbench--correction {
  grid-template-columns: 380px minmax(0, 1fr) 300px;
}

.bco__right-correction {
  min-height: 0;
  overflow: hidden;
}

/* 时间视窗条（顶部工具条下、工作面上；曲线高度与矩阵页同档） */
.bco__range {
  flex: none;
  padding: 4px 20px 0;
  border-bottom: 1px solid var(--color-border-subtle);
}

.bco__range-error {
  margin: 4px 0 6px;
  font-size: 12px;
  color: var(--color-error);
  display: flex;
  align-items: center;
  gap: 8px;
}

.bco__range-error button {
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-primary);
  font-size: 11px;
  padding: 1px 8px;
  cursor: pointer;
}

.bco__left {
  display: flex;
  flex-direction: column;
  min-height: 0;
  /* 左栏整体纵向可滚：三块堆叠总高超屏时仍能滚到第三块（A-10）。
     块内长清单各自内部滚动，操作按钮 sticky 于块内底部。 */
  overflow-y: auto;
  border-right: 1px solid var(--color-border-medium);
}

.bco__correction-canvas {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

/* 泳道名就地编辑：标题处点文字变输入框（零弹窗） */
.bco__lane-name {
  border: 0;
  background: transparent;
  padding: 0;
  font: inherit;
  color: inherit;
  cursor: text;
  border-bottom: 1px dashed transparent;
}

.bco__lane-name:hover {
  border-bottom-color: var(--color-border-medium);
}

.bco__name-edit {
  width: 220px;
}

/* 修正模式：生效时机结论型徽标（顶部标题行常驻；不解释机制、无保存出口） */
.bco__effect-badges {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-left: auto;
  font-size: 11px;
  color: var(--color-text-secondary);
}

.bco__timing {
  padding: 1px 6px;
  border-radius: 3px;
  border: 1px solid var(--color-border-medium);
  white-space: nowrap;
}

.bco__timing--now {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.bco__timing--next {
  border-style: dashed;
}

.bco__timing--plain {
  color: var(--color-text-secondary);
}

.bco__canvas {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.bco__right {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
}

.bco__inspector {
  flex: 1 1 auto;
  min-height: 0;
}

.bco__actions {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 14px;
  border-top: 1px solid var(--color-border-medium);
}

.bco__cancel {
  border: 0;
  background: transparent;
  padding: 4px 0;
  color: var(--color-text-secondary);
  font-size: 12px;
  cursor: pointer;
  text-align: left;
}

.bco__actions-note {
  text-align: center;
  font-size: 10px;
  color: var(--color-text-muted);
}

.bco__dialog-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.8;
  color: var(--color-text-secondary);
}

.bco__name-label {
  display: block;
  font-size: 12px;
  color: var(--color-text-secondary);
  margin-bottom: 6px;
}

@media (max-width: 1279.98px) {
  .bco__workbench {
    grid-template-columns: 240px minmax(0, 1fr) 280px;
  }
}
</style>
