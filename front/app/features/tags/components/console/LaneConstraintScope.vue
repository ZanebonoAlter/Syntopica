<script setup lang="ts">
/**
 * 左栏 · 辅助标签约束（topic-lane-auxiliary-label-constraint 第 6 组）。
 *
 * 契约（ui-design.md）：
 * - 形态照抄同目录 `LaneAnchorScope`：`panel` 骨架 + 块头（标题 + 计数徽标 + 生效徽标）
 *   + 块内列表；**默认折叠**（左栏已有「本泳道成员 / 更早成员 / 已移出 / 首义范围」四块，
 *   常态不再加高）。折叠只收列表，**计数徽标仍需真实值**，所以挂载即拉一次约束集。
 * - 写操作**逐条即时落库**，没有全局保存按钮；生效时机由块头「次日日报生效」承担。
 * - 界面文字只允许三类：动作标签（按钮/表头）、状态结论（徽标/空态/错误/计数）、
 *   必要反馈（toast/错误行）。**不写派生机制解释**（如「约束只影响吸附不影响生命周期」）
 *   —— `lane-correction-surface`「产品界面不承载解说性长句」。
 * - `margin` / `max_distance` 不在界面暴露：走 `ai_settings` 全局默认。
 * - 窄栏（≤1280px 左栏 240px）纵向堆叠，不横向挤压。
 */
import { computed, ref, watch } from 'vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppInput from '~/components/ui/AppInput.vue'
import { useLaneConstraint } from '~/features/tags/composables/useLaneConstraint'

const props = defineProps<{
  /** 当前修正的泳道话题 id；null = 未选中（整块不加载）。 */
  topicId: number | null
  /**
   * 草稿模式（新建泳道时）：不加载、不落库，只维护本地行集，由父层在保存时
   * 随建话题一并提交。此时 `topicId` 忽略。
   */
  draft?: boolean
}>()

const emit = defineEmits<{
  /** 草稿模式下行集变更（父层提交建话题时带上）。 */
  'update:draftConstraints': [rows: Array<{ term: string, mode: 'required' | 'preferred' }>]
}>()

const isDraft = computed(() => props.draft === true)

const {
  phase,
  loadError,
  rows,
  count,
  isEmpty,
  expanded,
  saving,
  actionError,
  draft,
  enableDraft,
  load,
  retryLoad,
  add,
  remove,
  setMode,
  toggleExpanded,
} = useLaneConstraint()

const draftTerm = ref('')
const draftMode = ref<'required' | 'preferred'>('required')

const canSubmit = computed(() => draftTerm.value.trim().length > 0 && !saving.value)

watch(
  () => props.draft === true,
  (isDraft) => {
    draft.value = isDraft
    if (isDraft) enableDraft()
  },
  { immediate: true },
)

watch(
  () => props.topicId,
  (id) => {
    if (draft.value || id == null) return
    void load(id)
  },
  { immediate: true },
)

// 草稿模式：行集变化即向父层报告（建话题时随 payload 提交）。
watch(
  rows,
  (list) => {
    if (!draft.value) return
    emit('update:draftConstraints', list.map(row => ({ term: row.term, mode: row.mode })))
  },
  { deep: true },
)

async function submit() {
  if (!canSubmit.value) return
  const ok = await add(draftTerm.value, draftMode.value)
  if (ok) {
    draftTerm.value = ''
    draftMode.value = 'required'
  }
}
</script>

<template>
  <section class="lcs-panel" data-testid="lane-constraint-panel">
    <header class="lcs-head">
      <button
        type="button"
        class="lcs-toggle"
        :aria-expanded="expanded"
        data-testid="lane-constraint-toggle"
        @click="toggleExpanded"
      >
        <span class="lcs-caret" :class="{ 'is-open': expanded }" aria-hidden="true">▸</span>
        <h3 class="lcs-title">辅助标签</h3>
      </button>
      <span class="lcs-count" data-testid="lane-constraint-count">{{ count }}</span>
      <span class="lcs-timing">次日日报生效</span>
    </header>

    <div v-if="expanded" class="lcs-body">
      <p v-if="phase === 'loading'" class="lcs-hint" data-testid="lane-constraint-loading">读取中…</p>
      <button
        v-else-if="phase === 'error'"
        type="button"
        class="lcs-retry"
        data-testid="lane-constraint-retry"
        @click="retryLoad"
      >读取失败 · 重试</button>

      <template v-else>
        <p v-if="isEmpty" class="lcs-hint" data-testid="lane-constraint-empty">未绑定辅助标签</p>

        <ul v-else class="lcs-rows">
          <li
            v-for="row in rows"
            :key="row.term"
            class="lcs-row"
            :class="{ 'is-computing': row.computing }"
            data-testid="lane-constraint-row"
          >
            <span class="lcs-term" :title="row.term">{{ row.term }}</span>
            <button
              type="button"
              class="lcs-mode"
              :class="{ 'is-required': row.mode === 'required' }"
              :aria-pressed="row.mode === 'required'"
              :aria-label="`切换 ${row.term} 为${row.mode === 'required' ? '偏好' : '必须'}`"
              :disabled="row.computing || saving"
              data-testid="lane-constraint-mode"
              @click="setMode(row.term, row.mode === 'required' ? 'preferred' : 'required')"
            >{{ row.mode === 'required' ? '必须' : '偏好' }}</button>
            <span v-if="row.computing" class="lcs-state" data-testid="lane-constraint-computing">计算中</span>
            <button
              v-else
              type="button"
              class="lcs-remove"
              :aria-label="`移除约束 ${row.term}`"
              :disabled="saving"
              data-testid="lane-constraint-remove"
              @click="remove(row.term)"
            >移除</button>
          </li>
        </ul>

        <div class="lcs-add">
          <AppInput
            v-model="draftTerm"
            input-id="lane-constraint-term"
            placeholder="输入主体词，如：以色列"
            :maxlength="100"
            :disabled="saving"
          />
          <div class="lcs-add-actions">
            <button
              type="button"
              class="lcs-mode"
              :class="{ 'is-required': draftMode === 'required' }"
              :aria-pressed="draftMode === 'required'"
              aria-label="切换新约束的类型"
              data-testid="lane-constraint-draft-mode"
              @click="draftMode = draftMode === 'required' ? 'preferred' : 'required'"
            >{{ draftMode === 'required' ? '必须' : '偏好' }}</button>
            <AppButton
              size="sm"
              variant="secondary"
              :disabled="!canSubmit"
              :loading="saving"
              data-testid="lane-constraint-submit"
              @click="submit"
            >添加</AppButton>
          </div>
        </div>

        <p v-if="actionError" class="lcs-error" role="alert" data-testid="lane-constraint-error">
          {{ actionError }}
        </p>
      </template>

      <p v-if="loadError && phase === 'error'" class="lcs-error" role="alert">{{ loadError }}</p>
    </div>
  </section>
</template>

<style scoped>
.lcs-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  /* 同 .las-panel：不参与 .bco__left 收缩，否则块内列表被压扁。 */
  flex-shrink: 0;
  border-top: 1px solid var(--color-border-subtle);
}

.lcs-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 10px 12px 6px;
}

.lcs-toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0;
  border: none;
  background: none;
  cursor: pointer;
  color: inherit;
}

.lcs-caret {
  font-size: 9px;
  color: var(--color-text-muted);
  transition: transform 0.12s ease;
}

.lcs-caret.is-open {
  transform: rotate(90deg);
}

.lcs-title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.lcs-count {
  font-size: 11px;
  color: var(--color-text-muted);
}

.lcs-timing {
  margin-left: auto;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  border: 1px dashed var(--color-border-medium);
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.lcs-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 0;
  flex: 0 1 auto;
  padding: 0 12px 10px;
}

.lcs-hint {
  margin: 0;
  font-size: 11px;
  color: var(--color-text-muted);
}

.lcs-rows {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-height: 0;
  overflow-y: auto;
}

/* 窄栏（≤1280px 左栏 240px）纵向堆叠，不横向挤压。 */
.lcs-row,
.lcs-add-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.lcs-term {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 12px;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lcs-mode {
  flex: 0 0 auto;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  border: 1px solid var(--color-border-medium);
  background: none;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.lcs-mode.is-required {
  border-color: var(--color-accent, var(--color-border-strong));
  color: var(--color-text-primary);
}

.lcs-mode:disabled,
.lcs-remove:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.lcs-state {
  flex: 0 0 auto;
  font-size: 10px;
  color: var(--color-text-muted);
}

.lcs-remove,
.lcs-retry {
  flex: 0 0 auto;
  padding: 0;
  border: none;
  background: none;
  font-size: 10px;
  color: var(--color-text-muted);
  cursor: pointer;
  text-decoration: underline;
}

.lcs-retry {
  align-self: flex-start;
}

.lcs-add {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.lcs-error {
  margin: 0;
  font-size: 11px;
  color: var(--color-danger, #d9534f);
}
</style>
