<script setup lang="ts">
import { ref } from 'vue'
import EmbeddingQueuePanel from '~/features/ai/components/EmbeddingQueuePanel.vue'
import TagQueuePanel from '~/features/settings/components/TagQueuePanel.vue'

// 支持 ?queue=tag|embedding 预选（tag-queue-progress-chip 点击跳转直达标签队列）
const route = useRoute()
const initialQueue = route.query.queue === 'tag' ? 'tag' : 'embedding'
const activeQueue = ref<'embedding' | 'tag'>(initialQueue)
</script>

<template>
  <div class="space-y-4">
    <!-- Sub-tab switch -->
    <div class="flex rounded-lg p-0.5" style="background: var(--color-bg-sunken)">
      <button
        type="button"
        class="px-4 py-2 text-sm rounded-md transition-colors"
        :style="activeQueue === 'embedding' ? 'background: var(--color-bg-elevated); box-shadow: var(--shadow-subtle); color: var(--color-text-primary); font-weight: 500' : 'color: var(--color-text-muted)'"
        @click="activeQueue = 'embedding'"
      >
        Embedding 队列
      </button>
      <button
        type="button"
        class="px-4 py-2 text-sm rounded-md transition-colors"
        :style="activeQueue === 'tag' ? 'background: var(--color-bg-elevated); box-shadow: var(--shadow-subtle); color: var(--color-text-primary); font-weight: 500' : 'color: var(--color-text-muted)'"
        @click="activeQueue = 'tag'"
      >
        标签队列
      </button>
    </div>

    <EmbeddingQueuePanel v-if="activeQueue === 'embedding'" />
    <TagQueuePanel v-else />
  </div>
</template>
