<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Component } from 'vue'
import SettingsTabsNav from './SettingsTabsNav.vue'
import SettingsSectionAiHealth from './SettingsSectionAiHealth.vue'
import SettingsSectionQueues from './SettingsSectionQueues.vue'
import SettingsSectionSchedulers from './SettingsSectionSchedulers.vue'

/**
 * 复合 section「运行状态」（restructure-settings-navigation）：
 * AI 健康 / 队列 / 定时任务 3 个监控面板合并，内部子 tab 切换；
 * tab 状态由 URL `tab` 参数承载，子 tab 键沿用旧 section 键供旧键重定向。
 * 定时任务面板读写端点为 `/api/ai/settings`（D6 修复，原 `/api/settings` 404）。
 */
interface SubTab {
  key: string
  label: string
  component: Component
}

const tabs: SubTab[] = [
  { key: 'ai-health', label: 'AI 健康', component: SettingsSectionAiHealth },
  { key: 'queues', label: '队列', component: SettingsSectionQueues },
  { key: 'schedulers', label: '定时任务', component: SettingsSectionSchedulers },
]

const route = useRoute()
const router = useRouter()

const activeTab = computed<string>(() => {
  const t = route.query.tab as string | undefined
  return t && tabs.some(tab => tab.key === t) ? t : tabs[0]!.key
})

// 非法/空 tab 值回默认并清参数（settings-workspace spec：非法 tab 值回退）
watch(
  () => route.query.tab,
  (t) => {
    if (route.query.section !== 'runtime-status') return
    if ((t === undefined) || (typeof t === 'string' && tabs.some(tab => tab.key === t))) return
    router.replace({ query: { ...route.query, tab: undefined } })
  },
  { immediate: true },
)

function selectTab(key: string) {
  router.replace({ query: { ...route.query, tab: key } })
}

const activeComponent = computed(() => {
  return tabs.find(tab => tab.key === activeTab.value)?.component ?? tabs[0]!.component
})
</script>

<template>
  <div class="space-y-5">
    <SettingsTabsNav :tabs="tabs" :model-value="activeTab" @update:model-value="selectTab" />
    <component :is="activeComponent" :key="activeTab" />
  </div>
</template>
