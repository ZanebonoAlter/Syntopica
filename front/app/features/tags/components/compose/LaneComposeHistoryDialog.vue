<script setup lang="ts">
/**
 * 编排态 · 历史待启用话题弹窗（rework-workshop-into-radar-and-desk / workshop-lane-compose）。
 *
 * 纯展示组件：名单列表 + 搜索框 + 两个动作按钮，逻辑全在 `useLaneCompose`（取数、过滤、
 * 带入草稿、直接启用原话题）。snake_case → camelCase 的转换已在 composable 层完成，
 * 本组件只渲染 camelCase 字段。
 *
 * 口径（原型 `openHistory` / `renderHistory` 的产品语气改写 + 契约 §2.4）：
 * - 这是**管理资格名单**，不是本次推荐；
 * - 未达管理门槛者「直接启用原话题」置灰，但**仍可带入草稿**观察；
 * - 采纳成草稿 ≠ 可启用原话题；资格沿服务端 `can_activate`；
 * - 直接启用沿用原向量，**不在启用时重算质心**；
 * - 搜索是**纯前端按名过滤**（不调 AI、不调 embed）。
 */
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import type { LaneHistoryTopic } from '~/features/tags/composables/useLaneCompose'

defineProps<{
  modelValue: boolean
  topics: LaneHistoryTopic[]
  loading?: boolean
  error?: string | null
  query?: string
  /** 正在「直接启用」的 topicId（该行按钮 loading）。 */
  activatingId?: string | null
  /** 浮层层级（取自 Z_INDEX 档位表；宿主是工作面遮罩 → 嵌套浮层取 dialogNested）。 */
  zIndex?: number
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'update:query': [value: string]
  'adopt': [topicId: string]
  'activate': [topicId: string]
}>()

function close(): void {
  emit('update:modelValue', false)
}

function onQuery(value: string | number): void {
  emit('update:query', String(value))
}

function onAdopt(topic: LaneHistoryTopic): void {
  emit('adopt', topic.id)
}

function onActivate(topic: LaneHistoryTopic): void {
  emit('activate', topic.id)
}
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    size="lg"
    title="历史待启用话题"
    :close-on-escape="false"
    :z-index="zIndex"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="lchd">
      <p class="lchd__explainer">
        这是管理资格名单，不是本次推荐。未达管理门槛的话题「直接启用原话题」置灰，但仍可带入草稿观察；
        带入草稿不等于可启用原话题。启用资格沿服务端 can_activate；直接启用沿用原向量，不在启用时重算质心。
      </p>
      <p class="lchd__note">
        「直接启用原话题」改的是原话题的状态，与本次草稿的「人工新建」是两件事。
      </p>

      <AppInput
        input-id="lane-history-query"
        :model-value="query ?? ''"
        type="search"
        placeholder="查找话题"
        data-testid="lane-history-search"
        @update:model-value="onQuery"
      />

      <p v-if="loading" class="lchd__empty">加载中…</p>
      <p v-else-if="error" class="lchd__error" role="alert" data-testid="lane-history-error">{{ error }}</p>
      <p v-else-if="topics.length === 0" class="lchd__empty" data-testid="lane-history-empty">
        没有匹配的话题。清空关键词可重新查看。
      </p>

      <ul v-else class="lchd__list">
        <li
          v-for="topic in topics"
          :key="topic.id"
          class="lchd__row"
          :data-testid="`lane-history-${topic.id}`"
        >
          <div class="lchd__main">
            <h4 class="lchd__label">{{ topic.label }}</h4>
            <p class="lchd__meta">
              累计命中 {{ topic.hitCount }} 次 · 最近 {{ topic.lastSeenDate ? topic.lastSeenDate.slice(0, 10) : '未知' }}
              · {{ topic.sectionCount }} 条 section
            </p>
          </div>
          <div class="lchd__actions">
            <AppButton
              size="sm"
              variant="secondary"
              :data-testid="`lane-history-adopt-${topic.id}`"
              @click="onAdopt(topic)"
            >
              带入草稿
            </AppButton>
            <AppButton
              size="sm"
              :disabled="topic.activateBlockReason !== null"
              :loading="activatingId === topic.id"
              :data-testid="`lane-history-activate-${topic.id}`"
              @click="onActivate(topic)"
            >
              直接启用原话题
            </AppButton>
            <span
              v-if="topic.activateBlockReason"
              class="lchd__hint"
              :data-testid="`lane-history-hint-${topic.id}`"
            >{{ topic.activateBlockReason }}</span>
          </div>
        </li>
      </ul>
    </div>
    <template #footer>
      <button type="button" data-testid="lane-history-close" @click="close">关闭</button>
    </template>
  </AppDialog>
</template>

<style scoped>
.lchd {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.lchd__explainer,
.lchd__note {
  margin: 0;
  font-size: 12px;
  line-height: 1.9;
  color: var(--color-text-secondary);
}

.lchd__note {
  color: var(--color-text-muted);
}

.lchd__empty {
  margin: 0;
  padding: 16px 2px;
  font-size: 12px;
  color: var(--color-text-muted);
}

.lchd__error {
  margin: 0;
  padding: 16px 2px;
  font-size: 12px;
  color: var(--color-error);
}

.lchd__list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 46vh;
  overflow: auto;
}

.lchd__row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid var(--color-border-subtle);
}

.lchd__main {
  min-width: 0;
}

.lchd__label {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.lchd__meta {
  margin: 4px 0 0;
  font-size: 11px;
  color: var(--color-text-muted);
}

.lchd__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
  flex: none;
}

.lchd__hint {
  font-size: 11px;
  color: var(--color-text-muted);
  flex-basis: 100%;
  text-align: right;
}
</style>
