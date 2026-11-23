<script setup lang="ts">
/**
 * 编排态右栏 · 三段 + 两个 tab（workshop-lane-compose / rework-lane-compose-swimlane-canvas）。
 *
 * 口径（契约 §2.4 / delta spec「右栏职责与画布标题」）：
 * - 右栏**只做三段**：① 当前事件的线索（只叙述标题/日期/归属/归档来源，**不再放选入按钮**）
 *   ② 可补充的事件（待选推荐，逐条明确加入）③ tab 区（已选事件 / 按标题找）；
 * - **三段是恒定骨架**：空态 / 编辑中 / 已确认 / 检索态都渲染，只换内容不换骨架；
 *   空态 ③ 默认落「按标题找」，编辑中与已确认态默认落「已选事件」；
 * - 「按标题找」只在本窗范围内、且**只在未归类节点里**按标题过滤
 *   （口径由宿主 `unclassifiedIds` 给出，不是 `persistentTopicId == null`），
 *   不调 AI、不自动勾选；
 * - 检索态 = ③ 切到按标题找 + 画布降级，**不是**另一套右栏。
 */
import { computed, ref, watch } from 'vue'
import { proximityOf, type LaneRecommendations } from '~/features/tags/composables/laneComposeLogic'
import type { ComposeCandidate } from '~/api/persistentTopics'

const props = defineProps<{
  focusSection: ComposeCandidate | null
  recommendations: LaneRecommendations<ComposeCandidate>
  selectedSections: ComposeCandidate[]
  selectedIds: Set<string>
  /** 窗口内全部 section（按标题找的取数范围）。 */
  sections: ComposeCandidate[]
  /** 检索范围 = 未归类节点 id（宿主按「不属于任何候选链」派生）。 */
  unclassifiedIds?: Set<string>
  /** 检索关键词（宿主持有，画布命中集与右栏同源）。 */
  query?: string
  showAssigned?: boolean
}>()

const emit = defineEmits<{
  'add': [id: string]
  'remove': [id: string]
  'focus': [id: string]
  'update:query': [value: string]
}>()

const activeTab = ref<'chosen' | 'find'>(props.selectedSections.length ? 'chosen' : 'find')
const keyword = computed(() => props.query ?? '')

function setQuery(value: string): void {
  emit('update:query', value)
}

// ③ 的默认落点跟着态走：空态 → 按标题找；编辑中 / 已确认 → 已选事件。
// 检索中（关键词非空）不抢用户的 tab 选择。
watch(() => props.selectedSections.length, (now, prev) => {
  if (keyword.value.trim()) return
  if (prev === 0 && now > 0) activeTab.value = 'chosen'
  else if (now === 0) activeTab.value = 'find'
})

// 退出检索（关键词由非空回到空）→ ③ 退回「已选事件」；①② 全程不变。
watch(keyword, (now, prev) => {
  if (!prev.trim() || now.trim()) return
  activeTab.value = props.selectedSections.length ? 'chosen' : 'find'
})

/** 当前事件线索（只叙述）。 */
const focusOrigin = computed(() => {
  const section = props.focusSection
  if (!section) return ''
  const status = section.persistentTopic?.status
  if (status === 'active') return `归属「${section.persistentTopic?.label ?? '已有泳道'}」· 只读参照。`
  if (status === 'archived') return `来自已归档泳道「${section.persistentTopic?.label ?? ''}」，只显示来源，不自动恢复原话题状态。`
  if (status === 'candidate') return `候选话题「${section.persistentTopic?.label ?? ''}」的成员。`
  return '当前窗口内未归属任何话题的事件。'
})

/** 可补充的事件（待选推荐，逐条明确加入）。 */
const suggestions = computed(() => props.recommendations.unassigned)

/** 按标题找：只在本窗范围内、且只在**未归类**节点里过滤，不自动勾选。 */
const findResults = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  const scope = props.unclassifiedIds
  const pool = props.sections.filter(section => (scope ? scope.has(section.id) : section.persistentTopic?.status !== 'active'))
  if (!q) return pool
  return pool.filter(section => section.clusterLabel.toLowerCase().includes(q))
})

/** 检索范围的大小（③ 的口径行：未归类 · 命中 N 条）。 */
const scopeCount = computed(() => (props.unclassifiedIds ? props.unclassifiedIds.size : findResults.value.length))
const searching = computed(() => keyword.value.trim().length > 0)

function archivedOrigin(section: ComposeCandidate): string {
  if (section.persistentTopic?.status !== 'archived') return ''
  return section.persistentTopic.label ?? ''
}

function onChosenToggle(section: ComposeCandidate, event: Event): void {
  const input = event.target as HTMLInputElement
  if (input.checked) emit('add', section.id)
  else emit('remove', section.id)
}
</script>

<template>
  <aside class="lci" data-testid="lane-compose-inspector" aria-label="本次泳道">
    <header class="lci__head">
      <h3 class="lci__title">本次泳道</h3>
      <span class="lci__count" data-testid="lane-compose-member-count">{{ selectedSections.length }} 条</span>
    </header>

    <div class="lci__scroll">
      <!-- ① 当前事件的线索（只叙述，不放选入按钮） -->
      <section class="lci__focus" data-testid="lane-compose-focus">
        <h4 class="lci__section-title">当前事件的线索</h4>
        <p v-if="!focusSection" class="lci__empty">点画布上的点看详情。</p>
        <template v-else>
          <p class="lci__focus-meta">
            事件 · {{ focusSection.periodDate.slice(0, 10) }}
          </p>
          <p class="lci__focus-title">{{ focusSection.clusterLabel }}</p>
          <p class="lci__focus-origin">{{ focusOrigin }}</p>
          <p v-if="focusSection.persistentTopic?.status === 'active' && !showAssigned" class="lci__focus-origin">
            图中已隐藏该归属，需手动开启「显示已有泳道」查看位置。
          </p>
        </template>
      </section>

      <!-- ② 可补充的事件（待选推荐，逐条明确加入） -->
      <section class="lci__suggest" data-testid="lane-compose-suggestions">
        <h4 class="lci__section-title">
          可补充的事件
          <small v-if="suggestions.length">{{ suggestions.length }}</small>
        </h4>
        <p v-if="!suggestions.length" class="lci__empty">
          {{ selectedSections.length ? '当前没有符合阈值的补充推荐，仍可在「按标题找」里手动选入。' : '无' }}
        </p>
        <ul v-else class="lci__suggest-list">
          <li v-for="item in suggestions" :key="item.section.id" class="lci__suggest-row" :data-testid="`lane-suggest-${item.section.id}`">
            <button type="button" class="lci__suggest-title" @click="emit('focus', item.section.id)">{{ item.section.clusterLabel }}</button>
            <p v-if="archivedOrigin(item.section)" class="lci__origin">来自已归档泳道：{{ archivedOrigin(item.section) }}</p>
            <div class="lci__suggest-foot">
              <small>{{ item.section.periodDate.slice(5) }} · 估算接近度 {{ proximityOf(item.distance)?.toFixed(2) }}</small>
              <button type="button" class="lci__suggest-add" :data-testid="`lane-suggest-add-${item.section.id}`" @click="emit('add', item.section.id)">＋ 加入</button>
            </div>
          </li>
        </ul>
      </section>

      <!-- ③ tab 区：已选事件 / 按标题找 -->
      <nav class="lci__tabs" role="tablist" aria-label="本次泳道内容切换">
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'chosen'"
          data-testid="lane-compose-tab-chosen"
          @click="activeTab = 'chosen'"
        >已选事件</button>
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'find'"
          data-testid="lane-compose-tab-find"
          @click="activeTab = 'find'"
        >按标题找</button>
      </nav>

      <div v-show="activeTab === 'chosen'" role="tabpanel" data-testid="lane-compose-panel-chosen">
        <p v-if="!selectedSections.length" class="lci__empty">还没选中的事件。左侧勾候选，或在「按标题找」里补。</p>
        <ul v-else class="lci__chosen-list">
          <li v-for="section in selectedSections" :key="section.id" class="lci__chosen-row" :data-testid="`lane-chosen-row-${section.id}`">
            <label>
              <input
                type="checkbox"
                checked
                :aria-label="`移出本次：${section.clusterLabel}`"
                :data-testid="`lane-chosen-check-${section.id}`"
                @change="onChosenToggle(section, $event)"
              >
              <span class="lci__chosen-title">{{ section.clusterLabel }}</span>
            </label>
            <small>{{ section.periodDate.slice(5) }}</small>
          </li>
        </ul>
      </div>

      <div v-show="activeTab === 'find'" role="tabpanel" data-testid="lane-compose-panel-find">
        <label class="lci__find-label" for="lane-compose-find-input">
          在当前范围内按标题找事件
          <input
            id="lane-compose-find-input"
            type="search"
            :value="keyword"
            placeholder="输入标题关键词"
            data-testid="lane-compose-find-input"
            @input="setQuery(($event.target as HTMLInputElement).value)"
          >
        </label>
        <p class="lci__find-scope" data-testid="lane-compose-find-scope">
          未归类 · <template v-if="searching">命中 {{ findResults.length }} 条</template><template v-else>{{ scopeCount }} 条可补</template>
          <button
            v-if="searching"
            type="button"
            class="lci__find-clear"
            data-testid="lane-compose-find-clear"
            @click="setQuery('')"
          >清空</button>
        </p>
        <p v-if="!findResults.length" class="lci__empty">
          {{ searching ? '没有匹配的事件，清空关键词可重新查看。' : '窗口内没有未归类的事件。' }}
        </p>
        <ul v-else class="lci__find-list">
          <li v-for="section in findResults" :key="section.id" class="lci__find-row" :data-testid="`lane-find-${section.id}`">
            <div class="lci__find-text">
              <span>{{ section.clusterLabel }}</span>
              <small>{{ section.periodDate.slice(0, 10) }}</small>
              <p v-if="archivedOrigin(section)" class="lci__origin">来自已归档泳道：{{ archivedOrigin(section) }}</p>
            </div>
            <button
              type="button"
              class="lci__find-add"
              :disabled="selectedIds.has(section.id)"
              :data-testid="`lane-find-add-${section.id}`"
              @click="emit('add', section.id)"
            >{{ selectedIds.has(section.id) ? '已加入' : '加入' }}</button>
          </li>
        </ul>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.lci {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  border-left: 1px solid var(--color-border-medium);
  background: var(--color-bg-base);
}

.lci__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  padding: 12px 14px 8px;
  border-bottom: 1px solid var(--color-border-medium);
}

.lci__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}

.lci__count {
  font-size: 11px;
  color: var(--color-text-muted);
}

.lci__scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 14px 16px;
}

.lci__section-title {
  margin: 14px 0 6px;
  font-size: 13px;
  font-weight: 600;
}

.lci__section-title small {
  font-weight: 400;
  color: var(--color-text-muted);
}

.lci__empty {
  color: var(--color-text-muted);
  font-size: 12px;
  line-height: 1.9;
  padding: 8px 0;
}

.lci__focus {
  border-bottom: 1px solid var(--color-border-subtle);
  padding-bottom: 8px;
}

.lci__focus-meta {
  margin: 0;
  font-size: 11px;
  color: var(--color-text-muted);
}

.lci__focus-title {
  margin: 6px 0;
  font-size: 15px;
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.lci__focus-origin {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--color-text-muted);
  line-height: 1.7;
}

.lci__suggest-list,
.lci__chosen-list,
.lci__find-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.lci__suggest-row {
  padding: 10px 0;
  border-bottom: 1px solid var(--color-border-subtle);
}

.lci__suggest-title {
  border: 0;
  background: transparent;
  padding: 0;
  text-align: left;
  font-size: 12px;
  line-height: 1.6;
  color: inherit;
  cursor: pointer;
  overflow-wrap: anywhere;
}

.lci__suggest-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 6px;
}

.lci__suggest-foot small {
  font-size: 10px;
  color: var(--color-text-muted);
}

.lci__suggest-add,
.lci__find-add {
  font-size: 11px;
  padding: 3px 9px;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-primary);
  cursor: pointer;
}

.lci__find-add:disabled {
  color: var(--color-text-muted);
  cursor: not-allowed;
}

.lci__origin {
  margin: 4px 0 0;
  font-size: 11px;
  color: var(--color-text-muted);
}

.lci__tabs {
  display: flex;
  gap: 0;
  margin: 14px 0 0;
  border-bottom: 1px solid var(--color-border-medium);
}

.lci__tabs button {
  flex: 1;
  border: 0;
  background: transparent;
  padding: 8px 4px;
  font-size: 12px;
  color: var(--color-text-muted);
  border-bottom: 2px solid transparent;
  cursor: pointer;
}

.lci__tabs button[aria-selected="true"] {
  color: var(--color-text-primary);
  border-bottom-color: var(--color-accent);
  font-weight: 600;
}

.lci__chosen-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 9px 0;
  border-bottom: 1px solid var(--color-border-subtle);
}

.lci__chosen-row label {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.lci__chosen-title {
  font-size: 12px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.lci__chosen-row small {
  color: var(--color-text-muted);
  font-size: 10px;
  flex: none;
}

.lci__find-label {
  display: block;
  font-size: 11px;
  color: var(--color-text-muted);
  margin: 10px 0 8px;
}

.lci__find-label input {
  width: 100%;
  margin-top: 5px;
  padding: 6px 8px;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-base);
  color: var(--color-text-primary);
  font-size: 12px;
}

.lci__find-scope {
  margin: 0 0 6px;
  font-size: 10px;
  color: var(--color-text-muted);
}

.lci__find-clear {
  margin-left: 6px;
  border: 0;
  background: transparent;
  color: var(--color-accent);
  font-size: 10px;
  cursor: pointer;
  padding: 0;
}

.lci__find-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 0;
  border-bottom: 1px solid var(--color-border-subtle);
}

.lci__find-text {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.lci__find-text small {
  display: block;
  font-size: 10px;
  color: var(--color-text-muted);
}
</style>
