<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { TopicKeyEventMarker } from '~/api/dailyReports'
import type { KeyEventPendingAction, ThreadKeyEventView } from '~/features/tags/composables/useTopicKeyEvents'

// ── 关键事件标记动作（add-topic-key-event-markers 3.2）──────────────────────
//
// 轻量展示组件：零 API 副作用（宿主经 useTopicKeyEvents 发起读写）。
// 按宿主模式显式渲染：
// - mode="review"（日报当日）：suggested 显示 AI 建议徽标/理由 + 确认/不采纳；
//   普通来源显示「标为关键」；confirmed 显示已标记 + 取消。
// - mode="history"（日报历史 thread）：同 review 的完整挑选（历史条目可挑可选）。
// - mode="overview"（话题总览/focus）：只显示 confirmed 注记的取消维护入口；
//   未确认按原新闻、不渲染建议徽标/理由/新挑选按钮（总览不承担新选择）。
//
// 按钮与宿主 thread 展开 button 同级（宿主放置），本组件自身不嵌套 button。

const props = defineProps<{
  mode: 'review' | 'history' | 'overview'
  threadId: number
  topicId?: number | null
  view: ThreadKeyEventView
}>()

const emit = defineEmits<{
  mark: []
  dismiss: []
  unmark: []
}>()

/** 组件内不可发起任何写动作：宿主未提供 topicId 时禁用（无归属说明由宿主渲染）。 */
const actionable = computed(() => props.topicId != null)

const state = computed(() => props.view.marker?.state ?? null)
const pending = computed(() => props.view.pending)
const busy = computed(() => pending.value != null)

/** 展示语义：状态在标题左侧可辨（spec），AI 理由仅在 review/history 模式出现。 */
const showSuggestion = computed(() =>
  (props.mode === 'review' || props.mode === 'history') && state.value === 'suggested',
)

const basisEvidenceCount = computed(() => props.view.marker?.recommendation_basis?.evidence_article_ids?.length ?? 0)

/** 详情块是否需要渲染（理由/错误/无归属说明任一存在才出右半列详情块，零内容不占行）。 */
const hasDetail = computed(() =>
  (showSuggestion.value && (props.view.marker?.recommendation_reason != null || basisEvidenceCount.value > 0))
  || props.view.error != null
  || (!actionable.value && state.value == null && props.mode !== 'overview'),
)

function isPending(action: KeyEventPendingAction) {
  return pending.value === action
}
</script>

<template>
  <!--
    多根 fragment（对齐原型 thread-row 分区）：
    ① .ke-detail —— 理由/依据/错误/无归属说明，右半列（c2/r1，与标题同一视觉行）；
    ② .ke-actions —— 状态徽标 + 按钮组，全宽底行右对齐（r2，行右下角）。
    两块由宿主 .drm-thread-row grid（minmax(0,1fr) minmax(0,1fr) 等宽两列）承载，
    但摆放规则住在本组件自身 scoped 样式：多根 fragment 根节点不继承宿主 scope 属性，
    宿主的子选择器永不命中（原因详见下方 <style> 注释，与宿主注释互指）。
  -->
  <div v-if="hasDetail" class="ke-detail">
    <!-- suggested：AI 理由 + 依据数（仅日报挑选模式，右半列详情块） -->
    <div v-if="showSuggestion" class="ke-detail__suggestion">
      <p v-if="view.marker?.recommendation_reason" class="ke-detail__reason">
        {{ view.marker.recommendation_reason }}
      </p>
      <p class="ke-detail__evidence">
        依据 {{ basisEvidenceCount }} 篇文章
      </p>
    </div>

    <!-- 保存/读取失败提示：局部、保留原文，可重试（宿主幂等重发） -->
    <p v-if="view.error" class="ke-detail__error" role="alert">
      <Icon icon="mdi:alert-circle-outline" width="13" aria-hidden="true" />
      {{ view.error }}
    </p>

    <!-- 无归属说明：无有效话题归属的线索禁标并说明原因（不自动建题） -->
    <p v-if="!actionable && state == null && mode !== 'overview'" class="ke-detail__note">
      无有效话题归属，不能标记
    </p>
  </div>

  <div
    class="ke-actions"
    :class="`ke-actions--${mode}`"
    :data-thread-id="threadId"
    :data-ke-state="state ?? 'none'"
    role="group"
    aria-label="关键事件标记"
  >
    <!-- suggested：AI 建议徽标（需人工确认才生效，仅日报挑选模式） -->
    <span v-if="showSuggestion" class="ke-actions__badge" title="AI 建议（需人工确认才生效）">
      <Icon icon="mdi:star-four-points-outline" width="12" aria-hidden="true" />
      AI 建议
    </span>

    <!-- confirmed：已标记（review/history 可取消；overview 仅维护取消） -->
    <span v-else-if="state === 'confirmed'" class="ke-actions__confirmed" :title="`已确认为关键事件（来源日报 ${view.marker?.source_period_date?.slice(0, 10) ?? ''}）`">
      <Icon icon="mdi:star-check" width="13" aria-hidden="true" />
      已标记{{ mode === 'overview' ? ' · 关键节点' : '' }}
    </span>

    <!-- dismissed：已不采纳/已取消；普通来源模式可再手标（不采纳不是否定） -->
    <span v-else-if="state === 'dismissed' && mode !== 'overview'" class="ke-actions__dismissed">已不采纳</span>

    <button
      v-if="showSuggestion"
      type="button"
      class="ke-actions__btn ke-actions__btn--confirm"
      :disabled="!actionable || busy"
      :aria-busy="isPending('mark')"
      @click="emit('mark')"
    >
      <Icon :icon="isPending('mark') ? 'mdi:loading' : 'mdi:check'" width="13" :class="{ 'ke-spin': isPending('mark') }" />
      {{ isPending('mark') ? '确认中…' : '确认为关键' }}
    </button>
    <button
      v-if="showSuggestion"
      type="button"
      class="ke-actions__btn ke-actions__btn--dismiss"
      :disabled="!actionable || busy"
      :aria-busy="isPending('dismiss')"
      @click="emit('dismiss')"
    >
      <Icon :icon="isPending('dismiss') ? 'mdi:loading' : 'mdi:close'" width="13" :class="{ 'ke-spin': isPending('dismiss') }" />
      {{ isPending('dismiss') ? '处理中…' : '不采纳' }}
    </button>

    <button
      v-else-if="state === 'confirmed'"
      type="button"
      class="ke-actions__btn ke-actions__btn--unmark"
      :disabled="busy"
      :aria-busy="isPending('unmark')"
      :title="view.marker?.source_thread_id == null ? '来源已失效，快照保留；可取消该标记' : '取消这条关键标记（纠错，不删除新闻）'"
      @click="emit('unmark')"
    >
      <Icon :icon="isPending('unmark') ? 'mdi:loading' : 'mdi:star-off'" width="13" :class="{ 'ke-spin': isPending('unmark') }" />
      {{ isPending('unmark') ? '取消中…' : '取消标记' }}
    </button>

    <button
      v-else-if="state === 'dismissed' && mode !== 'overview'"
      type="button"
      class="ke-actions__btn ke-actions__btn--confirm"
      :disabled="!actionable || busy"
      :aria-busy="isPending('mark')"
      @click="emit('mark')"
    >
      <Icon :icon="isPending('mark') ? 'mdi:loading' : 'mdi:star-plus'" width="13" :class="{ 'ke-spin': isPending('mark') }" />
      {{ isPending('mark') ? '标记中…' : '仍标为关键' }}
    </button>

    <!-- 普通来源：无注记行；日报模式显示手动标记入口；总览不渲染新挑选 -->
    <button
      v-else-if="state == null && mode !== 'overview'"
      type="button"
      class="ke-actions__btn ke-actions__btn--mark"
      :disabled="!actionable || busy"
      :aria-busy="isPending('mark')"
      :title="actionable ? '把这条线索标为关键事件（无需 AI 建议）' : '该线索无有效话题归属，不能标记'"
      @click="emit('mark')"
    >
      <Icon :icon="isPending('mark') ? 'mdi:loading' : 'mdi:star-outline'" width="13" :class="{ 'ke-spin': isPending('mark') }" />
      {{ isPending('mark') ? '标记中…' : '标为关键' }}
    </button>

    <!-- overview + dismissed/none：不渲染任何挑选按钮（未确认按原新闻） -->
  </div>
</template>

<style scoped>
/*
  摆放规则必须住在自身 scoped 样式：本组件是多根 fragment，根节点只带自家
  scope 属性、不继承宿主（DailyReportTopicSection）的 scope 属性，宿主
  .drm-thread-row 下的子选择器永不命中（fix-daily-report-ke-row-layout 根因）。
  窄屏 ≤720px 媒体查询与宿主 .drm-thread-row 的断点数值同步（两处注释互指锚定）。
*/
.ke-actions {
  /* 全宽底行右对齐（行右下角），徽标与按钮横排 */
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: flex-end;
  align-items: center;
  gap: 6px;
  grid-column: 1 / -1;
  grid-row: 2;
  font-size: 0.78rem;
  line-height: 1.45;
}

.ke-detail {
  /* 右半列顶对齐（c2/r1，与标题同一视觉行） */
  display: flex;
  flex-wrap: wrap;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0 0 2px;
  font-size: 0.76rem;
  line-height: 1.5;
  grid-column: 2;
  grid-row: 1;
}

.ke-detail__suggestion {
  display: flex;
  flex-direction: column;
  gap: 3px;
  border-left: 2px solid var(--color-border, #d8cfc2);
  padding: 2px 0 2px 10px;
}

.ke-detail__reason {
  margin: 0;
  color: var(--color-text-muted, #6b6155);
  overflow-wrap: anywhere;
  white-space: pre-line;
}

.ke-detail__evidence {
  margin: 0;
  color: var(--color-text-faint, #9a8f81);
}

.ke-detail__error {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin: 0;
  color: var(--color-danger, #b3261e);
  font-size: 0.74rem;
  overflow-wrap: anywhere;
}

.ke-detail__note {
  margin: 0;
  color: var(--color-text-faint, #9a8f81);
  font-size: 0.74rem;
}

.ke-actions__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 1px 7px;
  border-radius: 999px;
  background: var(--color-accent, #d94a4a);
  color: #fff;
  font-weight: 600;
  white-space: nowrap;
  align-self: center;
  text-align: center;
}

.ke-actions__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 8px;
  border: 1px solid var(--color-border, #d8cfc2);
  background: transparent;
  color: inherit;
  font-size: 0.76rem;
  line-height: 1.4;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s ease, border-color 0.15s ease;
}

.ke-actions__btn:hover:not(:disabled) {
  background: var(--color-bg-elevated, #f5f0e6);
  border-color: var(--color-accent, #d94a4a);
}

.ke-actions__btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.ke-actions__btn--confirm {
  border-color: var(--color-accent, #d94a4a);
  color: var(--color-accent, #d94a4a);
}

.ke-actions__confirmed {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  color: var(--color-accent, #d94a4a);
  font-weight: 600;
  white-space: nowrap;
}

.ke-actions__dismissed {
  color: var(--color-text-faint, #9a8f81);
  text-align: center;
  white-space: nowrap;
}

.ke-spin {
  animation: ke-spin 0.9s linear infinite;
}

@keyframes ke-spin {
  to {
    transform: rotate(360deg);
  }
}

/*
  窄屏 ≤720px：与宿主 DailyReportTopicSection 的 .drm-thread-row 断点数值同步
  （断点必须两处同改，互指锚定）——宿主该断点回退单列，detail 显式落 r2。
  动作组用 auto 落轨避免空 r2 轨道的双倍 row-gap：detail 存在时 auto 放置
  自然落 r3，不存在时落 r2，均紧贴上一行。摆放规则必须住在这里：
  多根 fragment 根节点不继承宿主 scope 属性。
*/
@media (max-width: 720px) {
  .ke-detail {
    grid-column: 1 / -1;
    grid-row: 2;
  }

  .ke-actions {
    grid-row: auto;
  }
}
</style>
