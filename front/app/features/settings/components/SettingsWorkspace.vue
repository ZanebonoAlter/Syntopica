<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Icon } from '@iconify/vue'
import type { Component } from 'vue'
import { useTheme } from '~/composables/useTheme'
import { useOnboarding } from '~/composables/useOnboarding'
import { LEGACY_SECTION_REDIRECT, LEGACY_TAB_REDIRECT } from '~/features/settings/legacyRedirects'
import SettingsSidebar from './SettingsSidebar.vue'
import SettingsSectionFeeds from './SettingsSectionFeeds.vue'
import SettingsSectionPreferences from './SettingsSectionPreferences.vue'
import SettingsSectionMarginNotes from './SettingsSectionMarginNotes.vue'
import AIProviderManagement from '~/features/ai/components/AIProviderManagement.vue'
import SettingsSectionCapabilityRoutes from './SettingsSectionCapabilityRoutes.vue'
import SettingsSectionDatasourcesNetwork from './SettingsSectionDatasourcesNetwork.vue'
import SettingsSectionRuntimeStatus from './SettingsSectionRuntimeStatus.vue'

export type SectionKey =
  | 'feeds'
  | 'preferences'
  | 'margin-notes'
  | 'ai-providers'
  | 'capability-routes'
  | 'datasources-network'
  | 'runtime-status'

export interface SectionMeta {
  key: SectionKey
  /** 分组标题（内容管理 / AI 配置 / 数据源与网络 / 运行状态），组标题不可点击 */
  group: string
  label: string
  description: string
  icon: string
  component: Component
}

/** sections 单一来源（D3）：导航元数据与渲染组件收敛于此，pages/settings.vue 不再维护映射 */
const sections: SectionMeta[] = [
  { key: 'feeds', group: '内容管理', label: '订阅源', description: '管理 RSS 订阅源的刷新、抓取和标签配置', icon: 'mdi:rss', component: SettingsSectionFeeds },
  { key: 'preferences', group: '内容管理', label: '兴趣画像', description: '按版块查看兴趣标签与权重，驱动订阅源推荐', icon: 'mdi:account-heart-outline', component: SettingsSectionPreferences },
  { key: 'margin-notes', group: '内容管理', label: '页边注', description: '跨报告的日报划词批注与问答：筛选、跳原日报定位、删除', icon: 'mdi:notebook-outline', component: SettingsSectionMarginNotes },
  { key: 'ai-providers', group: 'AI 配置', label: 'AI 模型', description: '管理供应商、模型清单与能力路由', icon: 'mdi:brain', component: AIProviderManagement },
  { key: 'capability-routes', group: 'AI 配置', label: '能力路由', description: '按能力分配模型优先级与降级顺序', icon: 'mdi:routes', component: SettingsSectionCapabilityRoutes },
  { key: 'datasources-network', group: '数据源与网络', label: '数据源与网络', description: 'Firecrawl、博查、网页搜索、RSSHub、研究数据源与出站代理配置', icon: 'mdi:lan-connect', component: SettingsSectionDatasourcesNetwork },
  { key: 'runtime-status', group: '运行状态', label: '运行状态', description: 'AI 健康、队列与定时任务的监控与手动触发', icon: 'mdi:monitor-dashboard', component: SettingsSectionRuntimeStatus },
]

// 旧 section 键深链重定向（D2）与旧子 tab 键重定向（switch-margin-note-search-to-deepseek D6）
// 的映射单一来源在 ~/features/settings/legacyRedirects.ts，本文件只消费、不再本地维护字面拷贝。
const router = useRouter()
const route = useRoute()
const { toggleTheme, isDark } = useTheme()
const { startSettingsTour } = useOnboarding()

onMounted(() => {
  // 首访自动启动已移除（默认关闭）；手动入口见下方「重看引导」
})

const sidebarOpen = ref(false)

const activeSection = computed<SectionKey>(() => {
  const q = route.query.section as string
  if (q && sections.some(s => s.key === q)) return q as SectionKey
  return 'feeds'
})

// 旧键命中即 router.replace 到新键携带 tab=<旧键/新键>（幂等：replace 后键已合法不再触发）
watch(
  () => [route.query.section as string | undefined, route.query.tab as string | undefined] as const,
  ([section, tab]) => {
    if (!section) return
    const redirect = LEGACY_SECTION_REDIRECT[section]
    if (redirect) {
      router.replace({ query: { ...route.query, section: redirect.section, tab: redirect.tab } })
      return
    }
    if (section !== 'datasources-network' || !tab) return
    const tabRedirect = LEGACY_TAB_REDIRECT[tab]
    if (tabRedirect) {
      router.replace({ query: { ...route.query, tab: tabRedirect } })
    }
  },
  { immediate: true },
)

const currentMeta = computed(() => {
  const found = sections.find(s => s.key === activeSection.value)
  return found ?? sections[0]!
})

function navigateSection(key: SectionKey) {
  router.replace({ query: { section: key } })
  sidebarOpen.value = false
}

function goHome() {
  router.push('/')
}
</script>

<template>
  <div class="settings-workspace">
    <!-- Header -->
    <header class="settings-header">
      <div class="settings-header__left">
        <button class="settings-header__back" title="返回首页" @click="goHome">
          <Icon icon="mdi:arrow-left" width="20" height="20" />
        </button>
        <div class="settings-header__title-group">
          <h1 class="settings-header__title">{{ currentMeta.label }}</h1>
          <p class="settings-header__desc">{{ currentMeta.description }}</p>
        </div>
      </div>
      <div class="settings-header__right">
        <button
          class="settings-header__mobile-nav"
          title="切换导航"
          @click="sidebarOpen = !sidebarOpen"
        >
          <Icon icon="mdi:menu" width="20" height="20" />
        </button>
        <button
          class="settings-header__theme"
          title="设置引导"
          @click="startSettingsTour"
        >
          <Icon icon="mdi:compass-outline" width="20" height="20" />
        </button>
        <button
          class="settings-header__theme"
          :title="isDark ? '切换为浅色模式' : '切换为深色模式'"
          @click="toggleTheme"
        >
          <Icon :icon="isDark ? 'mdi:white-balance-sunny' : 'mdi:weather-night'" width="20" height="20" />
        </button>
      </div>
    </header>

    <!-- Body -->
    <div class="settings-body">
      <!-- Sidebar -->
      <SettingsSidebar
        :sections="sections"
        :active-section="activeSection"
        :mobile-open="sidebarOpen"
        @select="navigateSection"
        @close="sidebarOpen = false"
      />

      <!-- Content -->
      <main class="settings-content">
        <component :is="currentMeta.component" :key="activeSection" />
      </main>
    </div>
  </div>
</template>

<style scoped>
.settings-workspace {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
  background: var(--color-bg-base);
  color: var(--color-text-primary);
}

/* Header */
.settings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  border-bottom: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  flex-shrink: 0;
}

.settings-header__left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.settings-header__back {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  border-radius: 8px;
  transition: background 0.15s, color 0.15s;
  flex-shrink: 0;
}

.settings-header__back:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.settings-header__title-group {
  min-width: 0;
}

.settings-header__title {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
  margin: 0;
  line-height: 1.3;
}

.settings-header__desc {
  font-size: 12px;
  color: var(--color-text-muted);
  margin: 0;
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.settings-header__right {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.settings-header__mobile-nav {
  display: none;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  border-radius: 8px;
  transition: background 0.15s;
}

.settings-header__mobile-nav:hover {
  background: var(--color-bg-hover);
}

.settings-header__theme {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  border-radius: 8px;
  transition: background 0.15s, color 0.15s;
}

.settings-header__theme:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

/* Body */
.settings-body {
  display: flex;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* Content */
.settings-content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 24px 28px;
}

/* Narrow viewport */
@media (max-width: 767.98px) {
  .settings-header__mobile-nav {
    display: flex;
  }

  .settings-header__desc {
    display: none;
  }

  .settings-body {
    flex-direction: column;
  }

  .settings-content {
    padding: 16px;
  }
}
</style>
