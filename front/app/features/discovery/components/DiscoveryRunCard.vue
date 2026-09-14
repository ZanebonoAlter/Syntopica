<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue'
import type { CandidateAvailability, DiscoveryRunItem } from '~/types/discovery'

/**
 * 查询结果卡片（独立 run 视图用）：推荐理由 / 召回来源徽标 / 可用性状态。
 * 与 DiscoveryCard 分流：run 条目是候选快照（无推荐 id），订阅走候选弹窗
 * （POST /api/feeds，而非推荐 accept）；不伪造相似度百分比（R6：无检查记录明示未验证）。
 * 5.3：订阅动作 emit 给父级打开订阅弹窗；subscribed 由父级本地标记（会话内防重复）。
 */
const props = defineProps<{
  item: DiscoveryRunItem
  /** 会话内已订阅标记（父级 store.subscribedRunIds） */
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
</script>

<template>
  <article class="run-card" data-testid="run-card">
    <div class="run-card__head">
      <h4 class="run-card__name u-break-title">{{ displayName }}</h4>
      <span class="run-card__availability" :class="availabilityTone[item.availability]">
        {{ availabilityText[item.availability] }}
      </span>
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

    <!-- 订阅动作：入库/推荐 ≠ 订阅，按需确认（R6）；已订阅禁用防重复创建 -->
    <div class="run-card__actions">
      <AppButton
        v-if="!subscribed"
        size="sm"
        :loading="subscribing"
        :disabled="subscribing"
        :data-testid="`run-subscribe-btn-${item.candidateId}`"
        @click="emit('subscribe', item)"
      >
        订阅
      </AppButton>
      <span v-else class="run-card__subscribed" data-testid="run-subscribed-label">已订阅</span>
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

.run-card__subscribed {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-success);
}
</style>
