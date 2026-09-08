<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Icon } from '@iconify/vue'
import { useTheme } from '~/composables/useTheme'
import { useOnboarding } from '~/composables/useOnboarding'
import SettingsSidebar from './SettingsSidebar.vue'

export type SectionKey =
  | 'feeds'
  | 'ai-providers'
  | 'capability-routes'
  | 'ai-health'
  | 'queues'
  | 'preferences'
  | 'firecrawl'
  | 'bocha'
  | 'analysis-methods'
  | 'rsshub'
  | 'proxy'
  | 'schedulers'

interface SectionMeta {
  key: SectionKey
  label: string
  description: string
  icon: string
}

const sections: SectionMeta[] = [
  { key: 'feeds', label: '订阅源', description: '管理 RSS 订阅源的刷新、抓取和标签配置', icon: 'mdi:rss' },
  { key: 'ai-providers', label: 'AI 模型', description: '配置主模型与备用模型提供商', icon: 'mdi:brain' },
  { key: 'capability-routes', label: '能力路由', description: '按能力分配模型优先级与降级顺序', icon: 'mdi:routes' },
  { key: 'ai-health', label: 'AI 健康', description: '各路由主模型连通性与本地模型自动拉起开关', icon: 'mdi:heart-pulse' },
  { key: 'queues', label: '队列', description: 'Embedding 与标签打标队列的监控', icon: 'mdi:format-list-bulleted' },
  { key: 'preferences', label: '兴趣画像', description: '按版块查看兴趣标签与权重，驱动订阅源推荐', icon: 'mdi:account-heart-outline' },
  { key: 'firecrawl', label: 'Firecrawl', description: 'Firecrawl 服务配置与抓取参数', icon: 'mdi:spider' },
  { key: 'bocha', label: '博查搜索', description: '数据增强联网检索的博查 web 搜索配置', icon: 'mdi:magnify' },
  { key: 'analysis-methods', label: '分析方法', description: '全局方法卡库（适用/禁用/证据/失败模式），仅按调查问题适配选择', icon: 'mdi:book-open-outline' },
  { key: 'rsshub', label: 'RSSHub', description: '订阅源发现的 RSSHub 实例地址', icon: 'mdi:radio-tower' },
  { key: 'proxy', label: '出站代理', description: 'feed 抓取等所有外部请求的全局代理', icon: 'mdi:lan-connect' },
  { key: 'schedulers', label: '定时任务', description: '定时任务状态与手动触发', icon: 'mdi:clock-outline' },
]

const router = useRouter()
const route = useRoute()
const { toggleTheme, isDark } = useTheme()
const { isSettingsFirstRun, startSettingsTour } = useOnboarding()

onMounted(() => {
  if (isSettingsFirstRun.value) {
    void startSettingsTour()
  }
})

const sidebarOpen = ref(false)

const activeSection = computed<SectionKey>(() => {
  const q = route.query.section as string
  if (q && sections.some(s => s.key === q)) return q as SectionKey
  return 'feeds'
})

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
        <slot :section="activeSection" />
      </main>
    </div>
  </div>
</template>

<style scoped>
.settings-workspace {
  display: flex;
  flex-direction: column;
  height: 100vh;
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
@media (max-width: 768px) {
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
