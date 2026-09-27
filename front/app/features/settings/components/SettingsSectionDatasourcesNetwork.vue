<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Component } from 'vue'
import SettingsTabsNav from './SettingsTabsNav.vue'
import FirecrawlConfigPanel from '~/components/dialog/FirecrawlConfigPanel.vue'
import BochaConfigPanel from '~/components/dialog/BochaConfigPanel.vue'
import SearxngConfigPanel from '~/components/dialog/SearxngConfigPanel.vue'
import SettingsSectionRsshub from './SettingsSectionRsshub.vue'
import SettingsSectionDatasources from './SettingsSectionDatasources.vue'
import SettingsSectionProxy from './SettingsSectionProxy.vue'

/**
 * 复合 section「数据源与网络」（restructure-settings-navigation）：
 * 6 个外部服务配置面板合并，内部子 tab 切换；tab 状态由 URL `tab` 参数承载
 * （可刷新/可分享；不跨 section 记忆）。子 tab 键沿用旧 section 键，
 * 供旧键深链重定向（?section=firecrawl → section=datasources-network&tab=firecrawl）。
 */
interface SubTab {
  key: string
  label: string
  component: Component
}

const tabs: SubTab[] = [
  { key: 'firecrawl', label: 'Firecrawl', component: FirecrawlConfigPanel },
  { key: 'bocha', label: '博查', component: BochaConfigPanel },
  { key: 'searxng', label: 'SearXNG', component: SearxngConfigPanel },
  { key: 'rsshub', label: 'RSSHub', component: SettingsSectionRsshub },
  { key: 'datasources', label: '研究数据源', component: SettingsSectionDatasources },
  { key: 'proxy', label: '出站代理', component: SettingsSectionProxy },
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
    if (route.query.section !== 'datasources-network') return
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
