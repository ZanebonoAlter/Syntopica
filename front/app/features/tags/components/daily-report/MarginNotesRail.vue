<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import MarginNoteCard from './MarginNoteCard.vue'
import type { MarginNoteAnnotation } from '~/api/marginNotes'
import type { MarginNoteAskState } from '~/features/tags/composables/useMarginNotes'

/**
 * 页边注栏（daily-report-margin-notes design D5）：
 * 桌面 = drm-layout 第三列 sticky 栏内滚动；窄屏 = 右抽屉内复用同一份内容。
 * 空态为极轻灰色提示（无卡片容器边框）；删除确认 AppDialog sm=420，明示连带删除
 * 全部问答且不可恢复；跳转定位靠卡片 data-annotation-id 反查（不假设 id 拼接规律）。
 */
const props = defineProps<{
  notes: MarginNoteAnnotation[]
  loading: boolean
  loadError: string
  /** 落锚失败行内提示（划选确认后 POST 失败；不静默丢弃，见 2026-09-24 复盘）。 */
  anchorError?: string
  /** 引用文本失配的批注（原文已变更降级态，PR-3）。 */
  changedIds?: number[]
  /** 被点亮（跳卡呼应）的批注 id；变化时定位滚动到对应卡。 */
  litId?: number | null
  askStates: Map<number, MarginNoteAskState>
}>()

const emit = defineEmits<{
  retryLoad: []
  jump: [annotationId: number]
  remove: [annotationId: number]
  ask: [annotationId: number, question: string]
  retryAsk: [annotationId: number]
  openCited: [annotationId: number, articleId: number]
}>()

const railEl = ref<HTMLElement | null>(null)
const confirmTarget = ref<MarginNoteAnnotation | null>(null)

const confirmOpen = computed({
  get: () => confirmTarget.value != null,
  set: (open: boolean) => {
    if (!open) confirmTarget.value = null
  },
})

const confirmQaCount = computed(() => confirmTarget.value?.qas.length ?? 0)

function askStateOf(id: number): MarginNoteAskState {
  return props.askStates.get(id) ?? { pendingQuestion: null, failedQuestion: null, error: null }
}

function confirmDelete() {
  if (!confirmTarget.value) return
  emit('remove', confirmTarget.value.id)
  confirmTarget.value = null
}

/** 定位某卡（mark 跳卡 / 深链定位共用）：滚动进视口，可选聚焦问题输入。 */
async function focusCard(annotationId: number, options?: { focusInput?: boolean }) {
  await nextTick()
  const el = railEl.value?.querySelector<HTMLElement>(`[data-annotation-id="${annotationId}"]`)
  if (!el) return false
  if (typeof el.scrollIntoView === 'function') el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  if (options?.focusInput) el.querySelector<HTMLInputElement>('input')?.focus()
  return true
}

watch(() => props.litId, (id) => {
  if (id != null) void focusCard(id)
})

defineExpose({ focusCard })
</script>

<template>
  <div ref="railEl" class="mn-rail">
    <div class="mn-rail__head">
      <span>页边注</span>
      <em>MARGIN NOTES · {{ notes.length }}</em>
    </div>

    <!-- 落锚失败：不静默（气泡点击后无卡无声 = 用户报障「点了没反应」）；重试即重新划选。
         独立 v-if，必须放在 loading 链之前——插在链中间会切断 v-else-if 归属，
         导致加载中/加载失败时也渲染空态（AV-3）。 -->
    <p v-if="anchorError" class="mn-rail__anchor-error" role="alert" data-testid="mn-anchor-error">
      落锚失败：{{ anchorError }}（请重新划选文字）
    </p>

    <!-- 加载中：骨架行，不闪空态（AV-3） -->
    <div v-if="loading" class="mn-rail__loading" data-testid="mn-rail-loading" aria-live="polite">
      <i v-for="index in 3" :key="index" />
    </div>

    <div v-else-if="loadError" class="mn-rail__error" role="alert" data-testid="mn-rail-error">
      <span>{{ loadError }}</span>
      <AppButton size="sm" variant="secondary" @click="emit('retryLoad')">重试</AppButton>
    </div>

    <!-- 空态：极轻灰色提示，无卡片容器边框（specs 空态提示） -->
    <p v-else-if="!notes.length" class="mn-rail__empty" data-testid="mn-rail-empty">
      <span class="mn-rail__empty-mark" aria-hidden="true">❧</span>
      暂无批注<br>划选左侧正文文字<br>点「问一问」即可添加
    </p>

    <template v-else>
      <MarginNoteCard
        v-for="note in notes"
        :key="note.id"
        :note="note"
        :lit="litId === note.id"
        :changed="changedIds?.includes(note.id) ?? false"
        :deleting="confirmTarget?.id === note.id"
        :ask-state="askStateOf(note.id)"
        @jump="emit('jump', note.id)"
        @remove-request="confirmTarget = note"
        @ask="question => emit('ask', note.id, question)"
        @retry-ask="emit('retryAsk', note.id)"
        @open-cited="articleId => emit('openCited', note.id, articleId)"
      />
    </template>

    <!-- 删除确认：AppDialog 默认 z=1000 会被阅读层 overlay（z=9000）盖住——弹窗可见但不可点，
         用既有 9100 档（与 ArticlePreviewModal 同源约定）保证覆盖在阅读层之上 -->
    <AppDialog v-model="confirmOpen" title="删除这条批注？" size="sm" :show-close="false" :z-index="9100">
      <p class="mn-rail__confirm-text" data-testid="mn-confirm-text">
        将连带删除其下全部 <strong>{{ confirmQaCount }}</strong> 轮问答（含已生成的回答、引用与术语记录），正文高亮同步移除。此操作不可恢复。
      </p>
      <div class="mn-rail__confirm-actions">
        <AppButton variant="secondary" data-testid="mn-cancel-delete" @click="confirmTarget = null">取消</AppButton>
        <AppButton variant="danger" data-testid="mn-confirm-delete" @click="confirmDelete">删除</AppButton>
      </div>
    </AppDialog>
  </div>
</template>

<style scoped>
.mn-rail {
  font-family: system-ui, -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
}

.mn-rail__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding-bottom: 0.5rem;
  margin-bottom: 0.8rem;
  border-bottom: 1px solid var(--color-border-strong);
  color: var(--color-text-muted);
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.22em;
}

.mn-rail__head em {
  color: var(--color-accent);
  font-size: 0.66rem;
  font-style: normal;
}

.mn-rail__loading {
  display: grid;
  gap: 0.6rem;
}

.mn-rail__loading i {
  height: 4.2rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 6px;
  background: var(--color-bg-active);
  animation: mnRailPulse 1.3s ease-in-out infinite;
}

.mn-rail__error {
  display: grid;
  gap: 0.5rem;
  padding: 0.9rem 0;
  color: var(--color-error);
  font-size: 0.72rem;
}

.mn-rail__anchor-error {
  margin: 0 0 0.75rem;
  padding: 0.5rem 0.6rem;
  border: 1px solid color-mix(in srgb, var(--color-error) 45%, transparent);
  border-radius: 5px;
  color: var(--color-error);
  font-size: 0.7rem;
  line-height: 1.6;
}

.mn-rail__empty {
  padding: 1.4rem 0.6rem;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  line-height: 2;
  text-align: center;
}

.mn-rail__empty-mark {
  display: block;
  font-size: 0.9rem;
  opacity: 0.7;
}

.mn-rail__confirm-text {
  margin: 0 0 1rem;
  color: var(--color-text-secondary);
  font-size: 0.8rem;
  line-height: 1.7;
}

.mn-rail__confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.6rem;
}

@keyframes mnRailPulse {
  0%, 100% { opacity: 0.45; }
  50% { opacity: 0.85; }
}
</style>
