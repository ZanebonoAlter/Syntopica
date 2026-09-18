<script setup lang="ts">
import { Icon } from '@iconify/vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import { DiscoveryWorkspace } from '~/features/discovery/public'
</script>

<template>
  <div class="discovery-page">
    <div class="discovery-topbar">
      <div class="discovery-topbar-inner">
        <div class="flex items-center gap-3">
          <NuxtLink to="/" class="discovery-back-btn" title="返回首页">
            <Icon icon="mdi:arrow-left" width="16" />
          </NuxtLink>
          <Icon icon="mdi:compass-outline" width="18" class="text-white/50" />
          <h1 class="discovery-page-title">发现订阅源</h1>
        </div>
        <ThemeToggle />
      </div>
    </div>

    <!-- 布局契约（ui-design Layout Contract）：contained 最大 1120px 居中，不自写 max-width -->
    <div class="discovery-main">
      <AppPageShell mode="contained" as="div">
        <p class="discovery-lede">先收藏来源到候选库，再按需订阅——入库不等于订阅。</p>
        <DiscoveryWorkspace />
      </AppPageShell>
    </div>
  </div>
</template>

<style scoped>
.discovery-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  height: 100dvh;
  background: var(--color-bg-base);
  color: var(--color-text-primary);
}

.discovery-topbar {
  flex-shrink: 0;
  background: var(--color-bg-elevated);
  border-bottom: 1px solid var(--color-border-subtle);
}

.discovery-topbar-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 20px;
}

.discovery-back-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  color: var(--color-text-secondary);
  transition: background 0.15s, color 0.15s;
}

.discovery-back-btn:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.discovery-page-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.discovery-main {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 24px 0 48px;
}

.discovery-lede {
  margin: 0 0 20px;
  font-size: 13px;
  color: var(--color-text-muted);
}

@media (max-width: 768px) {
  .discovery-main {
    padding: 16px 0 32px;
  }
}
</style>
