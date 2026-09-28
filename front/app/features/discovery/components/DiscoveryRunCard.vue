<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue'
import type { CandidateAvailability, DiscoveryRunItem } from '~/types/discovery'

/**
 * 查询结果卡片（独立 run 视图用）：推荐理由 / 召回来源徽标 / 可用性状态。
 * 与 DiscoveryCard 分流：run 条目是候选快照（无推荐 id），订阅走候选弹窗
 * （POST /api/feeds，而非推荐 accept）；不伪造相似度百分比（R6：无检查记录明示未验证）。
 * 5.3：订阅动作 emit 给父级打开订阅弹窗。
 * fix-discovery-subscribed-hint：后端 item.subscribed 与会话内标记取或决定头部
 * 「已订阅」徽标；订阅按钮保留——路由级命中仍可订其它参数，重复由弹窗按最终地址判定。
 */
const props = defineProps<{
  item: DiscoveryRunItem
  /** 会话内已订阅标记（父级 store.subscribedRunIds）；与 item.subscribed 取或 */
  subscribed?: boolean
  /** 订阅提交中（防连点） */
  subscribing?: boolean
}>()

const emit = defineEmits<{
  'subscribe': [item: DiscoveryRunItem]
}>()

const availabilityText: Record<CandidateAvailability, string> = {
  unknown: '未验证',
  ok: '可用',
  broken: '失效',
  requires_parameters: '需填参验证',
}

const availabilityTone: Record<CandidateAvailability, string> = {
  unknown: 'is-unknown',
  ok: 'is-ok',
  broken: 'is-broken',
  requires_parameters: 'is-unknown',
}

const displayName = props.item.name || '未知来源'

/** 已订阅（后端字段与会话内标记取或；只读提示，不阻塞再订其它参数） */
const isSubscribed = computed(() => props.subscribed === true || props.item.subscribed)

/** 已订阅徽标 title：列出已有订阅地址供对照（路由级命中可能不只一条；无地址时只说明已订阅） */
const subscribedTitle = computed(() =>
  props.item.subscribedUrls.length > 0
    ? `已订阅：\n${props.item.subscribedUrls.join('\n')}`
    : '该来源已有订阅',
)
</script>

<template>
  <article class="run-card" data-testid="run-card">
    <div class="run-card__head">
      <h4 class="run-card__name u-break-title">{{ displayName }}</h4>
      <div class="run-card__head-badges">
        <!-- 已订阅：只读提示（路由级命中不隐藏订阅入口）→ title 附已有订阅地址供对照 -->
        <span
          v-if="isSubscribed"
          class="run-card__subscribed-badge"
          :title="subscribedTitle"
          data-testid="run-subscribed-badge"
        >已订阅</span>
        <span class="run-card__availability" :class="availabilityTone[item.availability]">
          {{ availabilityText[item.availability] }}
        </span>
      </div>
    </div>
    <div v-if="item.recallOrigins.length > 0" class="run-card__origins">
      <span
        v-for="origin in item.recallOrigins"
        :key="origin"
        class="run-card__origin u-break-title"
      >{{ origin }}</span>
    </div>
    <p v-if="item.description" class="run-card__desc">{{ item.description }}</p>
    <p v-if="item.reason" class="run-card__reason">{{ item.reason }}</p>

    <!-- 订阅动作：入库/推荐 ≠ 订阅，按需确认（R6）；已订阅只提示不换按钮（仍可订其它参数） -->
    <div class="run-card__actions">
      <AppButton
        size="sm"
        :loading="subscribing"
        :disabled="subscribing"
        :data-testid="`run-subscribe-btn-${item.candidateId}`"
        @click="emit('subscribe', item)"
      >
        订阅
      </AppButton>
    </div>
  </article>
</template>

<style scoped>
.run-card {
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  padding: 14px 16px;
  background: var(--color-bg-elevated);
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.run-card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.run-card__name {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary);
  min-width: 0;
}

.run-card__head-badges {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.run-card__subscribed-badge {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 999px;
  border: 1px solid var(--color-success-subtle);
  color: var(--color-success);
  background: var(--color-success-subtle);
}

.u-break-title {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.run-card__availability {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  background: var(--color-bg-sunken);
}

.run-card__availability.is-ok {
  color: var(--color-success);
  border-color: var(--color-success-subtle);
  background: var(--color-success-subtle);
}

.run-card__availability.is-broken {
  color: var(--color-error);
}

.run-card__origins {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.run-card__origin {
  font-size: 11px;
  font-weight: 500;
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  max-width: 100%;
}

.run-card__desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
  /* 限 3 行防超长上游介绍撑爆卡片 */
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  overflow: hidden;
}

.run-card__reason {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-muted);
  overflow-wrap: anywhere;
}

.run-card__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}
</style>
