<script setup lang="ts">
import { computed, watch } from 'vue'
import {
  BACKGROUND_GRANULARITIES,
  BACKGROUND_GRANULARITY_LABELS,
  useTopicNewsBackground,
  type BackgroundGranularity,
} from '../composables/useTopicNewsBackground'
import { renderMarkdown } from '~/utils/markdown'
// 全局 .markdown-body 样式（与旧新闻背景/QAPanel 同源），LLM 文本渲染走同一管道
import '~/components/article/ArticleContent.css'
import AppButton from '~/components/ui/AppButton.vue'

/**
 * 话题新闻背景独立上下文（redesign-enrichment-research-workbench 4.3 / D7）。
 *
 * 数据键 = topicId + granularity + period，不再依赖聚焦分析选择状态；周/月/年
 * 各自独立读取，周期历史翻阅。两种宿主形态：
 *  - readonly=true（研究工作台只读预览）：只渲染内容 + 周期选择，无编辑/生成按钮；
 *  - 管理模式（readonly=false）：inline 编辑叙事（旧 PUT 语义）、补生成/重新汇总
 *    按钮先弹确认（useConfirm → AppDialog size="sm"）才调 regenerate。
 *
 * 安全：内容渲染走 renderMarkdown（与旧实现同源），不 v-html 原始模型输出之外
 * 的任何东西。不发任何研究/发现请求，研究开关状态与背景读取无关。
 */
const props = withDefaults(
  defineProps<{
    topicId: number
    topicLabel?: string
    semanticBoardId?: number
    readonly?: boolean
  }>(),
  { topicLabel: '', semanticBoardId: undefined, readonly: false },
)

const bg = useTopicNewsBackground()

const {
  granularity,
  selectedPeriod,
  periodList,
  currentRow,
  phase,
  loadError,
  editing,
  draft,
  dirty,
  saving,
  saveError,
  regenerating,
  regenerateError,
  open,
  selectGranularity,
  selectPeriod,
  retry,
  startEdit,
  cancelEdit,
  saveEdit,
  requestRegenerate,
} = bg

// topic/板块任一变化 → 重开一轮加载（epoch 守卫让晚响应全数丢弃）。
watch(
  () => [props.topicId, props.semanticBoardId] as const,
  ([id, boardId]) => {
    open(id, boardId)
  },
  { immediate: true },
)

const renderedContent = computed(() => renderMarkdown(currentRow.value?.content ?? ''))

const granularityLabel = computed(
  () => BACKGROUND_GRANULARITY_LABELS[granularity.value],
)

function onPeriodChange(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  if (v) selectPeriod(v)
}

defineExpose({
  /** 宿主接入关闭确认用（R-1）：脏草稿时先确认再关。 */
  dirty,
  /** 宿主关闭前调用：把脏稿 stash 进 drafts（「保留」语义；组件将被卸载时仍需先确认）。 */
  stashDraft: bg.stashDraft,
  /** 话题/周期键（调试与宿主判断用）。 */
  bg,
})
</script>

<template>
  <section class="topic-news-background">
    <header class="tnb-header">
      <h3 class="tnb-title">
        {{ topicLabel || `话题 #${topicId}` }} · 新闻背景
      </h3>
      <div class="tnb-controls">
        <div class="tnb-gran-tabs" role="tablist" aria-label="周期档">
          <button
            v-for="g in BACKGROUND_GRANULARITIES"
            :key="g"
            type="button"
            class="tnb-gran-btn"
            :class="{ 'is-active': granularity === g }"
            @click="selectGranularity(g as BackgroundGranularity)"
          >
            {{ BACKGROUND_GRANULARITY_LABELS[g] }}
          </button>
        </div>
        <select
          class="tnb-period-select"
          :value="selectedPeriod ?? ''"
          :aria-label="`${granularityLabel}周期`"
          :disabled="periodList.length === 0"
          @change="onPeriodChange"
        >
          <option v-if="periodList.length === 0" value="">暂无周期</option>
          <option v-for="p in periodList" :key="p" :value="p">{{ p }}</option>
        </select>
      </div>
    </header>

    <!-- loading -->
    <p v-if="phase === 'loading'" class="tnb-state">加载中…</p>

    <!-- error：可重试 -->
    <div v-else-if="phase === 'error'" class="tnb-state tnb-state--error">
      <p>{{ loadError }}</p>
      <AppButton size="sm" variant="secondary" class="tnb-retry-btn" @click="retry">
        重试
      </AppButton>
    </div>

    <!-- empty：该周期未生成（没有记忆不自动生成；readonly 无生成按钮） -->
    <div v-else-if="phase === 'empty'" class="tnb-state tnb-state--empty">
      <p>该{{ granularityLabel }}周期暂无新闻背景。</p>
      <AppButton
        v-if="!readonly"
        size="sm"
        class="tnb-regen-btn"
        :disabled="regenerating"
        @click="requestRegenerate"
      >
        {{ regenerating ? '生成中…' : '补生成' }}
      </AppButton>
    </div>

    <!-- success -->
    <div v-else class="tnb-body">
      <!-- 管理模式 inline 编辑 -->
      <template v-if="editing">
        <textarea
          v-model="draft"
          class="tnb-textarea"
          rows="10"
          aria-label="编辑新闻背景"
        />
        <p v-if="saveError" class="tnb-error-msg">{{ saveError }}</p>
        <div class="tnb-actions">
          <AppButton
            size="sm"
            variant="secondary"
            class="tnb-cancel-btn"
            :disabled="saving"
            @click="cancelEdit"
          >
            取消
          </AppButton>
          <AppButton
            size="sm"
            class="tnb-save-btn"
            :loading="saving"
            @click="saveEdit"
          >
            保存
          </AppButton>
        </div>
      </template>

      <template v-else>
        <!-- 展示态操作（仅管理模式）：编辑 + 重新汇总（先弹确认） -->
        <div v-if="!readonly" class="tnb-actions tnb-actions--top">
          <AppButton size="sm" variant="secondary" class="tnb-edit-btn" @click="startEdit">
            编辑
          </AppButton>
          <AppButton
            size="sm"
            variant="secondary"
            class="tnb-regen-btn"
            :disabled="regenerating"
            @click="requestRegenerate"
          >
            {{ regenerating ? '汇总中…' : '重新汇总' }}
          </AppButton>
        </div>
        <p v-if="regenerateError" class="tnb-error-msg">{{ regenerateError }}</p>
        <!-- 内容渲染：renderMarkdown（与旧新闻背景同源），不裸 v-html 模型输出 -->
        <div class="tnb-content markdown-body" v-html="renderedContent" />
      </template>
    </div>
  </section>
</template>

<style scoped>
.topic-news-background {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tnb-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.tnb-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.tnb-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tnb-gran-tabs {
  display: inline-flex;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  overflow: hidden;
}

.tnb-gran-btn {
  padding: 4px 10px;
  font-size: 12.5px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.tnb-gran-btn.is-active {
  background: var(--color-accent);
  color: var(--color-text-inverted);
}

.tnb-period-select {
  padding: 4px 8px;
  font-size: 12.5px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
}

.tnb-state {
  margin: 0;
  padding: 16px 0;
  font-size: 13px;
  color: var(--color-text-secondary);
}

.tnb-state--error,
.tnb-state--empty {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 8px;
}

.tnb-state--error p {
  margin: 0;
  color: var(--color-danger, #c0392b);
}

.tnb-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.tnb-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.tnb-actions--top {
  justify-content: flex-start;
}

.tnb-textarea {
  width: 100%;
  padding: 8px 10px;
  font-size: 13.5px;
  line-height: 1.6;
  font-family: inherit;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-primary);
  color: var(--color-text-primary);
  resize: vertical;
  box-sizing: border-box;
}

.tnb-error-msg {
  margin: 0;
  font-size: 12.5px;
  color: var(--color-danger, #c0392b);
}

.tnb-content {
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--color-text-primary);
  word-break: break-word;
}
</style>
