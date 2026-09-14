<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DiscoveryPanel from './DiscoveryPanel.vue'
import CandidateLibrary from './CandidateLibrary.vue'
import InterestRecords from './InterestRecords.vue'

/**
 * 发现页工作区：三同级页签（为你推荐 / 候选源库 / 兴趣记录）。
 *
 * - 页签状态走 URL query（?tab=），与 SettingsWorkspace 的 ?section= 惯例一致；
 *   默认 recommend 不写 query，保持旧链接兼容。
 * - 键盘可达：tablist 语义 + 左右方向键 / Home / End 切换（roving tabindex）。
 * - 「兴趣记录」空态「去找订阅源」→ 切回为你推荐并聚焦常驻查询框（go-ask）。
 */
type TabKey = 'recommend' | 'library' | 'interest'

const TABS: Array<{ key: TabKey, label: string, id: string }> = [
  { key: 'recommend', label: '为你推荐', id: 'discovery-tab-recommend' },
  { key: 'library', label: '候选源库', id: 'discovery-tab-library' },
  { key: 'interest', label: '兴趣记录', id: 'discovery-tab-interest' },
]

const route = useRoute()
const router = useRouter()

const activeTab = computed<TabKey>(() => {
  const q = route.query.tab
  const hit = TABS.find(t => t.key === q)
  return hit ? hit.key : 'recommend'
})

const tabNavRef = ref<HTMLElement | null>(null)

function switchTab(key: TabKey) {
  if (key === activeTab.value) return
  // 默认页签不写 query（兼容旧 /discovery 链接），其余以 replace 避免污染历史
  const query = { ...route.query }
  if (key === 'recommend') delete query.tab
  else query.tab = key
  router.replace({ query })
  void nextTick(() => {
    tabNavRef.value?.querySelector<HTMLButtonElement>(`#${TABS.find(t => t.key === key)!.id}`)?.focus()
  })
}

function onTabKeydown(e: KeyboardEvent) {
  const order = TABS.map(t => t.key)
  let index = order.indexOf(activeTab.value)
  if (e.key === 'ArrowRight') index = (index + 1) % order.length
  else if (e.key === 'ArrowLeft') index = (index + order.length - 1) % order.length
  else if (e.key === 'Home') index = 0
  else if (e.key === 'End') index = order.length - 1
  else return
  e.preventDefault()
  switchTab(order[index]!)
}

/** 兴趣记录空态「去找订阅源」：切回为你推荐并聚焦常驻查询输入框（R1 入口常驻） */
function goAsk() {
  switchTab('recommend')
  void nextTick(() => {
    document.getElementById('discovery-query-input')?.focus()
  })
}

const activeId = computed(() => TABS.find(t => t.key === activeTab.value)!.id)
</script>

<template>
  <div class="discovery-workspace">
    <nav class="discovery-tabs" role="tablist" aria-label="发现分区" ref="tabNavRef" @keydown="onTabKeydown">
      <button
        v-for="t in TABS"
        :id="t.id"
        :key="t.key"
        type="button"
        role="tab"
        class="discovery-tabs__tab"
        :class="{ 'is-active': activeTab === t.key }"
        :aria-selected="activeTab === t.key"
        :tabindex="activeTab === t.key ? 0 : -1"
        :data-testid="`tab-${t.key}`"
        @click="switchTab(t.key)"
      >
        {{ t.label }}
      </button>
    </nav>

    <div
      role="tabpanel"
      :aria-labelledby="activeId"
      tabindex="0"
      class="discovery-workspace__panel"
    >
      <DiscoveryPanel v-if="activeTab === 'recommend'" />
      <CandidateLibrary v-else-if="activeTab === 'library'" />
      <InterestRecords v-else @go-ask="goAsk" />
    </div>
  </div>
</template>

<style scoped>
.discovery-workspace {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
}

.discovery-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--color-border-subtle);
  overflow-x: auto;
}

.discovery-tabs__tab {
  appearance: none;
  border: none;
  background: transparent;
  padding: 10px 16px;
  font-size: 14px;
  font-family: inherit;
  color: var(--color-text-secondary);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  white-space: nowrap;
  transition: color 0.15s, border-color 0.15s;
}

.discovery-tabs__tab:hover {
  color: var(--color-text-primary);
}

.discovery-tabs__tab:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: -2px;
}

.discovery-tabs__tab.is-active {
  color: var(--color-text-primary);
  font-weight: 600;
  border-bottom-color: var(--color-accent);
}

.discovery-workspace__panel {
  min-width: 0;
}

.discovery-workspace__panel:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 4px;
}
</style>
