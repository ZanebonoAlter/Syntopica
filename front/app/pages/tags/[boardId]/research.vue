<script setup lang="ts">
/** 版块研究工作台（原「研究」tab 的全屏落点）；数据增强开关仍在治理抽屉。 */
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import BoardEnrichmentPanel from '~/features/tags/components/BoardEnrichmentPanel.vue'
import { useSemanticBoardsApi } from '~/api/semanticBoards'

const route = useRoute()
const router = useRouter()
const boardsApi = useSemanticBoardsApi()
const boardTitle = ref('')
const enrichmentEnabled = ref(false)

const boardId = computed(() => {
  const raw = Array.isArray(route.params.boardId) ? route.params.boardId[0] : route.params.boardId
  const parsed = Number(raw)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null
})

onMounted(async () => {
  if (!boardId.value) return
  const res = await boardsApi.getBoards()
  if (!res.success || !res.data) return
  const board = res.data.items.find(item => item.id === boardId.value)
  boardTitle.value = board?.label ?? ''
  enrichmentEnabled.value = board?.enrichment_enabled ?? false
})

function back(): void {
  void router.push(`/tags/${boardId.value ?? ''}`)
}
</script>

<template>
  <div class="research-page" data-testid="board-research">
    <div class="research-bar">
      <button type="button" class="research-back" @click="back">
        <Icon icon="mdi:arrow-left" width="16" />
        返回版块{{ boardTitle ? ` · ${boardTitle}` : '' }}
      </button>
      <NuxtLink to="/" class="research-home" data-testid="research-home">
        <Icon icon="mdi:home-outline" width="16" />
        主页
      </NuxtLink>
    </div>
    <BoardEnrichmentPanel
      v-if="boardId"
      :board-id="boardId"
      :enrichment-enabled="enrichmentEnabled"
    />
    <div v-else class="research-invalid">版块不存在。</div>
  </div>
</template>

<style scoped>
.research-page {
  min-height: 100vh;
  background: var(--color-bg-base);
}

.research-bar {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.45rem 1rem;
  border-bottom: 1px solid var(--color-border-subtle);
  background: color-mix(in srgb, var(--color-bg-base) 92%, transparent);
  backdrop-filter: blur(8px);
}

.research-back {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  border: 0;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.75rem;
  cursor: pointer;
}

.research-back:hover {
  color: var(--color-accent);
}

.research-home {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  text-decoration: none;
}

.research-home:hover {
  color: var(--color-accent);
}

.research-invalid {
  padding: 3rem;
  color: var(--color-text-muted);
}
</style>
