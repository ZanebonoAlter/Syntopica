<script setup lang="ts">
/**
 * 版块态势页路由 `/tags/:boardId`（rework-workshop-into-radar-and-desk 1.1）。
 *
 * 旧 `/tags`（标签池 / 五 tab）保留为兜底，阶段④切雷达后由新壳接管首屏。
 * 非法 boardId（非正整数）不进入控制台，给出返回入口。
 */
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Icon } from '@iconify/vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import BoardConsoleShell from '~/features/tags/components/console/BoardConsoleShell.vue'

const route = useRoute()

const boardId = computed(() => {
  const raw = route.params.boardId
  const value = Array.isArray(raw) ? raw[0] : raw
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
})
</script>

<template>
  <!-- workspace 模式 + 固定视口例外子树（layout.md 已登记；gutter=0 由控制台自带边距） -->
  <AppPageShell v-if="boardId !== null" mode="workspace" :gutter="0" as="div">
    <BoardConsoleShell :board-id="boardId" />
  </AppPageShell>
  <div v-else class="invalid-board">
    <Icon icon="mdi:alert-circle-outline" width="28" />
    <p>版块地址无效。</p>
    <NuxtLink to="/tags" class="invalid-board-link">返回叙事工坊</NuxtLink>
  </div>
</template>

<style scoped>
.invalid-board {
  height: 100vh;
  height: 100dvh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.6rem;
  color: var(--color-text-muted);
  font-size: 0.9rem;
}

.invalid-board-link {
  color: var(--color-accent);
  font-size: 0.82rem;
}
</style>
