<script setup lang="ts">
/**
 * 就地编排态右侧候选侧边栏（inline-compose-lane 切片②）.
 *
 * 纯展示组件：自然语言语义搜索框 + 候选话题引导区（连续命中候选）。
 * 所有逻辑由 useInlineCompose 提供（host 第③块装配），本组件只做 props→DOM/emit 接线。
 * 全 Layer 2 语义 token，跟随双主题；editorial 风格。
 *
 * 分组（spec「候选话题引导」「已中断候选单列分组」Scenario）：
 *  - 正在连续命中：brokenStreak===false（composable 已把 activatable 置顶）。
 *    每条渲染「确认启用」（disabled 当 !activatable）+「采纳」。
 *  - 已中断·近期未命中：brokenStreak===true，视觉弱化，标「近期未命中」而非「连续 0 天」，
 *    只渲染「采纳」，不渲染「确认启用」。
 *
 * 状态标记左对齐（interaction-conventions §1）：色点 + 状态文案置于标题左侧。
 * 无候选（items 空）时整个候选区隐藏（spec「无候选时侧边栏该区隐藏」）。
 *
 * 相似 section 推荐（recommendations）：按已选聚合向量匹配未勾选 pool 节点，分主组（待确认来源）
 *   与次组（现有泳道来源·弱化）；两组皆空时整区隐藏。点击清单项即 toggle 勾选。
 * 「已中断·近期未命中」组默认折叠（brokenCollapsed），标题带计数，点击展开。
 *
 * 设计依据：section-lifecycle spec「编排态候选池语义搜索」「候选话题引导」Requirement。
 */
import { computed, ref, watch } from 'vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { Recommendations, SidebarCandidateItem } from '~/features/tags/composables/useInlineCompose'

interface Props {
  /** 候选话题列表（来自 composable.sidebarItems，已 activatable 置顶）。 */
  items: SidebarCandidateItem[]
  /** 搜索文本（v-model:queryText）。 */
  queryText: string
  /** 搜索降级提示（embedQuery 失败 / 空向量）。 */
  searchError: string | null
  /** 搜索进行中（debounce / embedQuery 进行中）。 */
  searching: boolean
  /** 相似 section 推荐（按主信号；两组皆空=无信号或无匹配，整区隐藏）。 */
  recommendations: Recommendations
  /** 推荐区标题（已选/搜索词），由 composable 按 activeSignal 来源定。 */
  recommendationTitle: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:queryText': [value: string]
  'activate': [topicId: number]
  'adopt': [item: SidebarCandidateItem]
  'recommend': [sectionId: string]
}>()

/** 正在连续命中（brokenStreak===false），composable 已保证 activatable 在前。 */
const mainGroup = computed(() => props.items.filter(i => !i.brokenStreak))

/** 已中断·近期未命中（consecutive_hits===0）。 */
const brokenGroup = computed(() => props.items.filter(i => i.brokenStreak))

/** 「已中断」组默认折叠（让位主组 + section 推荐），点标题展开。 */
const brokenCollapsed = ref(true)

// 没有「正在连续命中」候选时，「已中断」就是唯一可用候选，默认展开，
// 否则真实数据下侧栏只剩一个计数按钮（S3 主链路不可达）。
watch(mainGroup, (group) => {
  if (group.length === 0) brokenCollapsed.value = false
}, { immediate: true })

/** 推荐区可见性：两组皆空（无勾选/无匹配）→ 隐藏。 */
const hasRecommendations = computed(() =>
  props.recommendations.unassigned.length > 0 || props.recommendations.active.length > 0,
)

function onQuery(value: string | number): void {
  emit('update:queryText', String(value))
}
</script>

<template>
  <aside class="compose-sidebar" aria-label="编排态候选侧边栏">
    <!-- 顶部：自然语言语义搜索 -->
    <div class="cs-search">
      <div class="cs-search__field">
        <svg
          class="cs-search__icon"
          viewBox="0 0 24 24"
          width="16"
          height="16"
          aria-hidden="true"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <circle cx="11" cy="11" r="7" />
          <line x1="21" y1="21" x2="16.65" y2="16.65" />
        </svg>
        <AppInput
          class="cs-search__input"
          :model-value="queryText"
          placeholder="自然语言搜索未分类 section"
          @update:model-value="onQuery"
        />
      </div>
      <span v-if="searching" class="cs-search__loading">搜索中…</span>
    </div>
    <p v-if="searchError" class="cs-search__error" role="status">
      {{ searchError }}
    </p>

    <!-- 相似 section 推荐（按已选聚合向量；无勾选或无匹配→整区隐藏） -->
    <section v-if="hasRecommendations" class="cs-recommend" aria-label="相似 section 推荐">
      <h4 class="cs-section__title">{{ recommendationTitle }}</h4>
      <!-- 待确认来源（主组） -->
      <div v-if="recommendations.unassigned.length > 0" class="cs-rec-group">
        <span class="cs-rec-group__sub">待确认来源</span>
        <button
          v-for="r in recommendations.unassigned"
          :key="r.id"
          type="button"
          class="cs-rec"
          @click="emit('recommend', r.id)"
        >
          <span class="cs-rec__row">
            <span class="cs-rec__label">{{ r.clusterLabel }}</span>
            <span class="cs-rec__dist">{{ r.distance.toFixed(2) }}</span>
          </span>
        </button>
      </div>
      <!-- 现有泳道来源（次组·弱化；点击即勾走移出，复用现有移出提示/保存二次确认） -->
      <div v-if="recommendations.active.length > 0" class="cs-rec-group cs-rec-group--muted">
        <span class="cs-rec-group__sub">现有泳道来源</span>
        <button
          v-for="r in recommendations.active"
          :key="r.id"
          type="button"
          class="cs-rec cs-rec--muted"
          @click="emit('recommend', r.id)"
        >
          <span class="cs-rec__row">
            <span class="cs-rec__label">{{ r.clusterLabel }}</span>
            <span class="cs-rec__dist">{{ r.distance.toFixed(2) }}</span>
          </span>
          <span class="cs-rec__origin">从「{{ r.originLabel }}」移出</span>
        </button>
      </div>
    </section>

    <!-- 候选话题区（无候选整体隐藏） -->
    <section v-if="items.length > 0" class="cs-candidates">
      <!-- 正在连续命中 -->
      <div v-if="mainGroup.length > 0" class="cs-group">
        <h4 class="cs-group__title">正在连续命中</h4>
        <article
          v-for="item in mainGroup"
          :key="item.topic.id"
          class="cs-card"
          :class="{ 'is-activatable': item.activatable }"
        >
          <div class="cs-card__head">
            <span
              v-if="item.activatable"
              class="cs-card__flag cs-card__flag--ok"
              aria-label="可启用"
            >●</span>
            <span class="cs-card__label">{{ item.topic.label }}</span>
          </div>
          <p class="cs-card__meta">
            连续 {{ item.topic.consecutive_hits }} 天 · 含 {{ item.topic.section_count }} 条 section
            <template v-if="item.topic.last_seen_date"> · 最近 {{ item.topic.last_seen_date.slice(0, 10) }}</template>
          </p>
          <div class="cs-card__actions">
            <div class="cs-card__activate">
              <AppButton
                size="sm"
                :disabled="!item.activatable"
                @click="emit('activate', item.topic.id)"
              >
                确认启用
              </AppButton>
              <span v-if="!item.activatable" class="cs-card__hint">需先满足连续多天出现条件</span>
            </div>
            <AppButton size="sm" variant="secondary" @click="emit('adopt', item)">
              采纳
            </AppButton>
          </div>
        </article>
      </div>

      <!-- 已中断·近期未命中（默认折叠，标题带计数） -->
      <div v-if="brokenGroup.length > 0" class="cs-group cs-group--broken">
        <button
          type="button"
          class="cs-group__title cs-group__title--btn"
          :aria-expanded="!brokenCollapsed"
          @click="brokenCollapsed = !brokenCollapsed"
        >
          <svg
            class="cs-group__chevron"
            :class="{ 'is-open': !brokenCollapsed }"
            viewBox="0 0 24 24"
            width="12"
            height="12"
            aria-hidden="true"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <polyline points="9 6 15 12 9 18" />
          </svg>
          已中断·近期未命中（{{ brokenGroup.length }}）
        </button>
        <template v-if="!brokenCollapsed">
          <article
            v-for="item in brokenGroup"
            :key="item.topic.id"
            class="cs-card is-broken"
          >
            <div class="cs-card__head">
              <span
                class="cs-card__flag cs-card__flag--broken"
                aria-label="近期未命中"
              >○</span>
              <span class="cs-card__label">{{ item.topic.label }}</span>
              <span class="cs-card__streak">近期未命中</span>
            </div>
            <p class="cs-card__meta">
              含 {{ item.topic.section_count }} 条 section
              <template v-if="item.topic.last_seen_date"> · 最近 {{ item.topic.last_seen_date.slice(0, 10) }}</template>
            </p>
            <div class="cs-card__actions">
              <AppButton size="sm" variant="secondary" @click="emit('adopt', item)">
                采纳
              </AppButton>
            </div>
          </article>
        </template>
      </div>
    </section>
  </aside>
</template>

<style scoped>
.compose-sidebar {
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: 100%;
  min-width: 0;
  padding: 14px;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: 14px;
  box-shadow: var(--shadow-strong);
  color: var(--color-text-primary);
  font-size: 14px;
}

/* ── 搜索区 ───────────────────────────────────────────────────────────── */
.cs-search {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.cs-search__field {
  display: flex;
  align-items: center;
  gap: 6px;
}
.cs-search__icon {
  flex: 0 0 auto;
  color: var(--color-text-muted);
}
.cs-search__input {
  flex: 1 1 auto;
  min-width: 0;
}
.cs-search__loading {
  font-size: 12px;
  color: var(--color-text-muted);
}
.cs-search__error {
  margin: 0;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--color-warning);
  background: var(--color-warning-subtle);
  border-radius: 6px;
}

/* ── 候选区 ───────────────────────────────────────────────────────────── */
.cs-candidates {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.cs-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.cs-group__title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--color-text-muted);
  text-transform: none;
}
.cs-group--broken {
  opacity: 0.7;
}
.cs-group--broken .cs-group__title {
  color: var(--color-text-muted);
}

.cs-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--color-bg-hover);
  border: 1px solid var(--color-border-subtle);
}
.cs-card.is-activatable {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
}
.cs-card.is-broken {
  background: var(--color-bg-sunken);
}

.cs-card__head {
  display: flex;
  align-items: center;
  gap: 6px;
}
.cs-card__flag {
  font-size: 12px;
  line-height: 1;
}
.cs-card__flag--ok {
  color: var(--color-success);
}
.cs-card__flag--broken {
  color: var(--color-text-muted);
}
.cs-card__label {
  font-weight: 600;
  color: var(--color-text-primary);
}
.cs-card__streak {
  margin-left: auto;
  font-size: 12px;
  color: var(--color-text-muted);
}
.cs-card__meta {
  margin: 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}
.cs-card__actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}
.cs-card__activate {
  display: flex;
  align-items: center;
  gap: 6px;
}
.cs-card__hint {
  font-size: 11px;
  color: var(--color-text-muted);
}

/* ── 已中断组折叠标题（button 复位 + chevron） ─────────────────────────── */
.cs-group__title--btn {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  margin: 0;
  padding: 0;
  background: none;
  border: none;
  font: inherit;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--color-text-muted);
  text-align: left;
  cursor: pointer;
}
.cs-group__chevron {
  flex: 0 0 auto;
  transition: transform 0.15s ease;
}
.cs-group__chevron.is-open {
  transform: rotate(90deg);
}

/* ── 相似 section 推荐区 ───────────────────────────────────────────────── */
.cs-recommend {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.cs-section__title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--color-text-muted);
}
.cs-rec-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.cs-rec-group__sub {
  font-size: 11px;
  color: var(--color-text-muted);
}
.cs-rec-group--muted {
  opacity: 0.75;
}
.cs-rec {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  padding: 6px 10px;
  background: var(--color-bg-hover);
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  text-align: left;
  cursor: pointer;
  font: inherit;
  color: var(--color-text-primary);
  transition: background 0.12s ease, border-color 0.12s ease;
}
.cs-rec:hover {
  background: var(--color-accent-subtle);
  border-color: var(--color-accent);
}
.cs-rec--muted {
  background: var(--color-bg-sunken);
}
.cs-rec__row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.cs-rec__label {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cs-rec__dist {
  flex: 0 0 auto;
  font-size: 11px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}
.cs-rec__origin {
  font-size: 11px;
  color: var(--color-text-muted);
}
</style>
