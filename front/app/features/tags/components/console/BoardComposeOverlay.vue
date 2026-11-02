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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import { Z_INDEX } from '~/components/ui/layout-contract'
import LaneComposeCandidates from '~/features/tags/components/compose/LaneComposeCandidates.vue'
import LaneComposeCanvas from '~/features/tags/components/compose/LaneComposeCanvas.vue'
import LaneComposeHistoryDialog from '~/features/tags/components/compose/LaneComposeHistoryDialog.vue'
import LaneComposeInspector from '~/features/tags/components/compose/LaneComposeInspector.vue'
import { useLaneCompose, type LaneComposeConfirmRequest } from '~/features/tags/composables/useLaneCompose'
import type { ManualTopicResult } from '~/api/persistentTopics'
import { useIsNarrowViewport } from '~/composables/useMediaQuery'
import { useNotify } from '~/composables/useNotify'

const props = defineProps<{
  boardId: number
  boardLabel: string
}>()

const emit = defineEmits<{
  close: []
  saved: [topic?: ManualTopicResult]
}>()

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

// ── 窗口控件 ──────────────────────────────────────────────────────────────
async function onModeChange(event: Event): Promise<void> {
  const value = (event.target as HTMLSelectElement).value
  if (value === 'custom') {
    compose.startCustomRange()
    return
  }
  await compose.setQuickDays(Number(value) as 7 | 14 | 30 | 90)
}

function onFromInput(event: Event): void {
  compose.controlFrom.value = (event.target as HTMLInputElement).value
}

function onToInput(event: Event): void {
  compose.controlTo.value = (event.target as HTMLInputElement).value
}

function onToggleAssigned(event: Event): void {
  compose.showAssigned.value = (event.target as HTMLInputElement).checked
}

// ── 退出 ──────────────────────────────────────────────────────────────────
async function onCancel(): Promise<void> {
  const ok = await compose.cancel()
  if (ok) emit('close')
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  if (confirmOpen.value || namingOpen.value) return
  // 历史名单弹窗自己处理 Esc（AppDialog 关掉它，不连带退出编排态）
  if (historyOpen.value) {
    historyOpen.value = false
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
  await compose.load()
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
    <div v-if="isNarrow" class="bco-narrow" data-testid="board-compose-narrow">
      <h2 class="bco-narrow__title">新建泳道需要宽屏或横屏</h2>
      <p class="bco-narrow__text">请在宽屏上查看完整关系并挑选事件。</p>
    </div>

    <div
      v-else
      class="bco"
      data-testid="board-compose"
      role="dialog"
      aria-modal="true"
      :aria-label="`新建泳道 · ${boardLabel}`"
    >
      <header class="bco__top">
        <button type="button" class="bco__back" data-testid="compose-cancel" @click="onCancel">← 返回</button>
        <h2 class="bco__title">新建泳道</h2>
        <div class="bco__window">
          <label class="bco__window-field">
            日期
            <select :value="compose.controlMode.value" data-testid="lane-compose-window-mode" @change="onModeChange">
              <option value="7">近7天</option>
              <option value="14">近14天</option>
              <option value="30">近30天</option>
              <option value="90">近90天</option>
              <option value="custom">自选起止</option>
            </select>
          </label>
          <span class="bco__window-label" data-testid="lane-compose-window-label">{{ compose.windowLabel.value }}</span>
          <div v-if="compose.controlMode.value === 'custom'" class="bco__window-custom">
            <input
              type="date"
              :value="compose.controlFrom.value"
              aria-label="开始日期"
              data-testid="lane-compose-window-from"
              @input="onFromInput"
            >
            <span>至</span>
            <input
              type="date"
              :value="compose.controlTo.value"
              aria-label="结束日期"
              data-testid="lane-compose-window-to"
              @input="onToInput"
            >
            <button type="button" data-testid="lane-compose-window-apply" @click="compose.applyCustomRange()">应用</button>
          </div>
          <p v-if="compose.dateError.value" class="bco__error" role="alert" data-testid="lane-compose-date-error">
            {{ compose.dateError.value }}
          </p>
        </div>
      </header>

      <div class="bco__tools">
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
        <span class="bco__tool-note">推荐不自动选入；点的大小与深浅只表示接近程度，不是置信度。</span>
      </div>

      <p v-if="compose.loading.value && !compose.sections.value.length" class="bco__state" data-testid="lane-compose-loading">
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
            @focus="id => compose.toggleFocus(id)"
            @clear-focus="compose.clearFocus()"
            @toggle-member="(id, checked) => compose.toggleMember(id, checked)"
          />

          <div class="bco__right">
            <LaneComposeInspector
              class="bco__inspector"
              :focus-section="compose.focusSection.value"
              :recommendations="compose.recommendations.value"
              :selected-sections="compose.selectedSections.value"
              :selected-ids="compose.selectedIds.value"
              :sections="compose.sections.value"
              :show-assigned="compose.showAssigned.value"
              @add="id => compose.toggleMember(id, true)"
              @remove="id => compose.toggleMember(id, false)"
              @focus="id => compose.setFocus(id)"
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

.bco__window {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
}

.bco__window-field {
  display: flex;
  align-items: center;
  gap: 6px;
}

.bco__window select,
.bco__window input {
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-base);
  color: var(--color-text-primary);
  padding: 4px 6px;
  font-size: 12px;
}

.bco__window-label {
  color: var(--color-text-muted);
  font-size: 11px;
}

.bco__window-custom {
  display: flex;
  align-items: center;
  gap: 6px;
}

.bco__window-custom button {
  border: 1px solid var(--color-border-medium);
  background: transparent;
  color: var(--color-text-primary);
  padding: 4px 10px;
  font-size: 12px;
  cursor: pointer;
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
