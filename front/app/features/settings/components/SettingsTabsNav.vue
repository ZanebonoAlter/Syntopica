<script setup lang="ts">
/**
 * 复合 section 子 tab 条（restructure-settings-navigation ui-design Layout Contract）：
 * sticky 贴内容滚动区顶、横向滚动不换行、bg 遮罩防内容透出。
 * 状态由父容器承载（tab 参数入 URL），本组件纯展示。
 */
defineProps<{
  tabs: { key: string, label: string }[]
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [key: string]
}>()
</script>

<template>
  <div class="settings-tabs-nav">
    <div class="settings-tabs-nav__track" role="tablist">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        role="tab"
        class="settings-tabs-nav__tab"
        :class="{ 'settings-tabs-nav__tab--active': modelValue === tab.key }"
        :aria-selected="modelValue === tab.key"
        @click="emit('update:modelValue', tab.key)"
      >
        {{ tab.label }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.settings-tabs-nav {
  position: sticky;
  top: 0;
  z-index: 5;
  margin: -24px -28px 20px;
  padding: 0 28px;
  background: var(--color-bg-base);
  border-bottom: 1px solid var(--color-border-subtle);
}

.settings-tabs-nav__track {
  display: flex;
  gap: 4px;
  overflow-x: auto;
  white-space: nowrap;
  scrollbar-width: thin;
}

.settings-tabs-nav__tab {
  padding: 10px 14px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color 0.15s, border-color 0.15s;
  flex-shrink: 0;
}

.settings-tabs-nav__tab:hover {
  color: var(--color-text-primary);
}

.settings-tabs-nav__tab--active {
  color: var(--color-accent);
  font-weight: 600;
  border-bottom-color: var(--color-accent);
}

@media (max-width: 767.98px) {
  .settings-tabs-nav {
    margin: -16px -16px 16px;
    padding: 0 16px;
  }
}
</style>
