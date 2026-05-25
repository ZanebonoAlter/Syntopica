<script setup lang="ts">
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useSemanticBoardsApi, type BoardNarrative } from '~/api/semanticBoards'

const props = defineProps<{ boardId: number }>()

const { getBoardNarratives } = useSemanticBoardsApi()

const narratives = ref<BoardNarrative[]>([])
const days = ref(7)
const loading = ref(false)
const expandedId = ref<number | null>(null)

const statusStyle: Record<string, string> = {
  emerging: 'bg-green-100 text-green-700',
  continuing: 'bg-blue-100 text-blue-700',
  splitting: 'bg-orange-100 text-orange-700',
  merging: 'bg-purple-100 text-purple-700',
  ending: 'bg-gray-100 text-gray-600',
}

const statusLabel: Record<string, string> = {
  emerging: '新兴',
  continuing: '持续',
  splitting: '分裂',
  merging: '合并',
  ending: '结束',
}

async function loadNarratives() {
  loading.value = true
  try {
    const res = await getBoardNarratives(props.boardId, { days: days.value })
    narratives.value = res.data || []
  } finally {
    loading.value = false
  }
}

function toggleExpand(narrative: BoardNarrative) {
  if (expandedId.value === narrative.id) {
    expandedId.value = null
    return
  }
  expandedId.value = narrative.id
}

function loadMore() {
  days.value += 7
  loadNarratives()
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

watch(() => props.boardId, () => {
  days.value = 7
  expandedId.value = null
  loadNarratives()
}, { immediate: true })
</script>

<template>
  <div class="bnt-panel">
    <div class="bnt-header">
      <Icon icon="mdi:timeline-text-outline" width="15" class="text-white/50" />
      <span class="bnt-title">板块叙事</span>
      <span v-if="narratives.length" class="bnt-count">{{ narratives.length }}</span>
    </div>

    <div v-if="loading" class="bnt-loading">
      <div v-for="i in 2" :key="i" class="bnt-skeleton" />
    </div>

    <div v-else-if="narratives.length === 0" class="bnt-empty">
      暂无叙事
    </div>

    <div v-else class="bnt-list">
      <div
        v-for="n in narratives"
        :key="n.id"
        class="bnt-card"
        :class="{ 'bnt-card--expanded': expandedId === n.id }"
        @click="toggleExpand(n)"
      >
        <div class="bnt-card-top">
          <span class="bnt-status" :class="statusStyle[n.status] || 'bg-gray-100 text-gray-600'">
            {{ statusLabel[n.status] || n.status }}
          </span>
          <span class="bnt-date">{{ formatDate(n.period_date) }}</span>
          <span class="bnt-article-count">{{ n.article_count }} 篇</span>
        </div>
        <div class="bnt-card-title">{{ n.title }}</div>
        <div class="bnt-card-summary">{{ n.summary }}</div>
        <div v-if="n.related_tags.length > 0" class="bnt-tags">
          <span v-for="tag in n.related_tags" :key="tag.id" class="bnt-tag-chip">{{ tag.label }}</span>
        </div>

        <!-- Expanded detail -->
        <div v-if="expandedId === n.id" class="bnt-expanded">
          <div class="bnt-expanded-row">
            <Icon icon="mdi:file-document-outline" width="12" class="text-white/30" />
            <span class="bnt-expanded-label">关联文章</span>
            <span class="bnt-expanded-value">{{ n.article_count }} 篇</span>
          </div>
          <div class="bnt-expanded-row">
            <Icon icon="mdi:tag-multiple-outline" width="12" class="text-white/30" />
            <span class="bnt-expanded-label">关联标签</span>
            <span class="bnt-expanded-value">{{ n.related_tags.map(t => t.label).join('、') || '无' }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="narratives.length > 0" class="bnt-more">
      <button type="button" class="bnt-more-btn" @click="loadMore">
        加载更早
      </button>
    </div>
  </div>
</template>

<style scoped>
.bnt-panel {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  margin-top: 1rem;
  padding: 1rem;
  border-radius: 12px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.025);
}

.bnt-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.bnt-title {
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.7);
}

.bnt-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  padding: 0.05rem 0.4rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.bnt-loading {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.bnt-skeleton {
  height: 72px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.03);
  animation: bntPulse 1.5s ease-in-out infinite;
}

@keyframes bntPulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 0.8; }
}

.bnt-empty {
  text-align: center;
  color: rgba(255, 255, 255, 0.25);
  font-size: 0.75rem;
  padding: 1rem 0;
}

.bnt-list {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.bnt-card {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  padding: 0.65rem 0.75rem;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.02);
  cursor: pointer;
  transition: all 0.12s ease;
}

.bnt-card:hover {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.1);
}

.bnt-card--expanded {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.12);
}

.bnt-card-top {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.bnt-status {
  font-size: 0.6rem;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  font-weight: 500;
  line-height: 1.4;
}

.bnt-date {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.35);
}

.bnt-article-count {
  font-size: 0.6rem;
  color: rgba(255, 255, 255, 0.25);
  margin-left: auto;
}

.bnt-card-title {
  font-size: 0.78rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.8);
  line-height: 1.4;
}

.bnt-card-summary {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.45);
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.bnt-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  margin-top: 0.15rem;
}

.bnt-tag-chip {
  font-size: 0.62rem;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 255, 255, 0.5);
}

.bnt-expanded {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.bnt-expanded-row {
  display: flex;
  align-items: center;
  gap: 0.3rem;
}

.bnt-expanded-label {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.35);
}

.bnt-expanded-value {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.55);
}

.bnt-more {
  text-align: center;
}

.bnt-more-btn {
  font-size: 0.68rem;
  padding: 0.2rem 0;
  border: none;
  background: none;
  color: rgba(255, 255, 255, 0.3);
  cursor: pointer;
  transition: color 0.12s ease;
}

.bnt-more-btn:hover {
  color: rgba(255, 255, 255, 0.55);
}
</style>
