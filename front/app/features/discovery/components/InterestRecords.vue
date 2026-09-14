<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import { useDiscoveryStore } from '~/stores/discovery'
import type { InterestStatus } from '~/types/discovery'

/**
 * 兴趣记录页签（R3 Independent Interest Records；替换 5.1 占位）。
 *
 * - 逐条独立展示：查询原句 / 归属版块或「未匹配版块」/ 记录时间 / 参与状态徽标；
 *   不把向量相似度伪装成兴趣百分比（不显示任何百分比）。
 * - 顶部说明：近期记录用于补充推荐，影响会逐渐降低。
 * - 四态：加载 / 错误（重试，不冒称空）/ 空（去找订阅源 → 切回为你推荐并聚焦输入框）/ 列表。
 */
const store = useDiscoveryStore()
const emit = defineEmits<{ 'go-ask': [] }>()

const statusText: Record<InterestStatus, string> = {
  active: '参与中',
  faded: '已淡出',
  legacy: '历史迁移',
}

const statusTone: Record<InterestStatus, string> = {
  active: 'is-active',
  faded: 'is-faded',
  legacy: 'is-legacy',
}

type View = 'loading' | 'error' | 'empty' | 'list'

const view = computed<View>(() => {
  if (store.interestsLoading && store.interests.length === 0) return 'loading'
  if (store.interestsError) return 'error'
  if (store.interests.length === 0) return 'empty'
  return 'list'
})

onMounted(() => {
  if (!store.interestsLoaded && !store.interestsLoading) {
    void store.loadInterests()
  }
})

function goAsk() {
  emit('go-ask')
}
</script>

<template>
  <div class="interests" data-testid="interest-records">
    <p class="interests__note">
      近期记录用于补充推荐，影响会逐渐降低。这里只展示你的原句、归属与参与状态，不合成平均兴趣。
    </p>

    <!-- 加载态 -->
    <div v-if="view === 'loading'" class="interests__state" data-testid="interest-state-loading">
      <Icon icon="mdi:loading" width="36" height="36" class="interests__spin" />
      <p class="interests__state-text">正在加载兴趣记录…</p>
    </div>

    <!-- 错误态：重试，不冒称空 -->
    <div v-else-if="view === 'error'" class="interests__state" data-testid="interest-state-error">
      <Icon icon="mdi:alert-circle-outline" width="36" height="36" style="color: var(--color-error)" />
      <p class="interests__state-title">兴趣记录加载失败</p>
      <p class="interests__state-text">{{ store.interestsError }}。记录可能仍在，请重试。</p>
      <AppButton size="md" variant="secondary" data-testid="interest-retry-btn" @click="store.loadInterests()">
        重试
      </AppButton>
    </div>

    <!-- 空态：给找源入口（切回为你推荐并聚焦查询框） -->
    <div v-else-if="view === 'empty'" class="interests__state" data-testid="interest-state-empty">
      <Icon icon="mdi:thought-bubble-outline" width="36" height="36" style="color: var(--color-text-muted)" />
      <p class="interests__state-title">还没有问答兴趣</p>
      <p class="interests__state-text">在「为你推荐」顶部的查询框输入想看的内容，每次成功查询都会在这里留下独立记录。</p>
      <AppButton size="md" data-testid="interest-go-ask-btn" @click="goAsk">
        去找订阅源
      </AppButton>
    </div>

    <!-- 列表 -->
    <ul v-else class="interests__list" data-testid="interest-list" aria-live="polite">
      <li v-for="entry in store.interests" :key="entry.id" class="interests__item">
        <div class="interests__main">
          <h4 class="interests__query u-break-title">{{ entry.queryText }}</h4>
          <p class="interests__meta">
            <span class="interests__board">{{ entry.boardLabel || '未匹配版块' }}</span>
            <span class="interests__sep" aria-hidden="true">·</span>
            <span>{{ entry.createdAt }}</span>
          </p>
        </div>
        <span class="interests__badge" :class="statusTone[entry.status]">
          {{ statusText[entry.status] }}
        </span>
      </li>
    </ul>

    <p v-if="view === 'list'" class="interests__caption">
      「已淡出」表示记录因时间窗口或阅读行为成熟而降低影响，不是你拒绝过它；「历史迁移」为旧数据升级保留的记录。
    </p>
  </div>
</template>

<style scoped>
.interests {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}

.interests__note {
  margin: 0;
  padding: 10px 14px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border-subtle);
  border-radius: 10px;
  background: var(--color-bg-sunken);
}

.interests__state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 56px 24px;
  text-align: center;
}

.interests__state-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.interests__state-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--color-text-muted);
  max-width: 420px;
}

.interests__spin {
  color: var(--color-link, var(--color-accent));
  animation: interests-spin 1s linear infinite;
}

@keyframes interests-spin {
  to { transform: rotate(360deg); }
}

.interests__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.interests__item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 16px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  background: var(--color-bg-elevated);
}

.interests__main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.interests__query {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-primary);
  min-width: 0;
}

.u-break-title {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.interests__meta {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--color-text-muted);
}

.interests__board {
  color: var(--color-text-secondary);
}

.interests__sep {
  color: var(--color-text-muted);
}

.interests__badge {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 500;
  padding: 2px 10px;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  background: var(--color-bg-sunken);
}

.interests__badge.is-active {
  color: var(--color-success);
  border-color: var(--color-success-subtle);
  background: var(--color-success-subtle);
}

.interests__badge.is-faded {
  color: var(--color-text-muted);
}

.interests__badge.is-legacy {
  color: var(--color-accent);
  border-color: var(--color-accent-subtle);
  background: var(--color-accent-subtle);
}

.interests__caption {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--color-text-muted);
}

/* 窄屏（390×844 检查档）：条目单列堆叠 */
@media (max-width: 720px) {
  .interests__item {
    flex-direction: column;
  }
}
</style>
