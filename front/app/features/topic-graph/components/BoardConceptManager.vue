<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Icon } from '@iconify/vue'
import { useBoardConceptsApi, type BoardConcept, type ConceptSuggestion } from '~/api/boardConcepts'

const api = useBoardConceptsApi()
const concepts = ref<BoardConcept[]>([])
const suggestions = ref<ConceptSuggestion[]>([])
const loading = ref(false)
const error = ref('')
const suggestionLoading = ref(false)

const showCreateForm = ref(false)
const newName = ref('')
const newDescription = ref('')

async function loadConcepts() {
  loading.value = true
  error.value = ''
  const result = await api.getBoardConcepts()
  if (result.success && result.data) {
    concepts.value = result.data
  } else {
    error.value = result.error || '加载概念列表失败'
  }
  loading.value = false
}

async function createConcept() {
  if (!newName.value.trim()) return
  error.value = ''
  const result = await api.createBoardConcept({
    name: newName.value.trim(),
    description: newDescription.value.trim(),
  })
  if (result.success) {
    newName.value = ''
    newDescription.value = ''
    showCreateForm.value = false
    await loadConcepts()
  } else {
    error.value = result.error || '创建概念失败'
  }
}

async function deactivateConcept(id: number) {
  if (!confirm('确定要停用此板块概念吗？')) return
  error.value = ''
  const result = await api.deleteBoardConcept(id)
  if (result.success) {
    await loadConcepts()
  } else {
    error.value = result.error || '停用概念失败'
  }
}

async function suggestNewConcepts() {
  suggestionLoading.value = true
  error.value = ''
  const result = await api.suggestConcepts('event')
  if (result.success && result.data) {
    suggestions.value = result.data
  } else {
    error.value = result.error || 'LLM 建议失败'
  }
  suggestionLoading.value = false
}

async function acceptSuggestion(suggestion: ConceptSuggestion) {
  error.value = ''
  const result = await api.createBoardConcept(suggestion)
  if (result.success) {
    suggestions.value = suggestions.value.filter((s) => s.name !== suggestion.name)
    await loadConcepts()
  } else {
    error.value = result.error || '接受建议失败'
  }
}

function rejectSuggestion(suggestion: ConceptSuggestion) {
  suggestions.value = suggestions.value.filter((s) => s.name !== suggestion.name)
}

onMounted(() => {
  loadConcepts()
})

defineExpose({ loadConcepts })
</script>

<template>
  <div class="flex flex-col gap-3 py-1">
    <div class="flex items-center justify-between">
      <span class="text-[0.72rem] tracking-[0.22em] uppercase text-white/40">板块概念</span>
      <div class="flex gap-2">
        <button
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-[rgba(255,255,255,0.1)] bg-[rgba(255,255,255,0.04)] text-xs text-[rgba(255,233,220,0.6)] hover:bg-[rgba(255,255,255,0.08)] hover:text-[rgba(255,233,220,0.88)] transition-colors disabled:opacity-40"
          :disabled="suggestionLoading"
          @click="suggestNewConcepts"
        >
          <Icon v-if="suggestionLoading" icon="mdi:loading" width="13" class="animate-spin" />
          <Icon v-else icon="mdi:auto-fix" width="13" />
          {{ suggestionLoading ? '分析中...' : 'LLM 建议' }}
        </button>
        <button
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-[rgba(255,255,255,0.1)] bg-[rgba(255,255,255,0.04)] text-xs text-[rgba(255,233,220,0.6)] hover:bg-[rgba(255,255,255,0.08)] hover:text-[rgba(255,233,220,0.88)] transition-colors"
          @click="showCreateForm = !showCreateForm"
        >
          <Icon icon="mdi:plus" width="13" />
          手动创建
        </button>
      </div>
    </div>

    <div
      v-if="error"
      class="rounded-lg border border-[rgba(220,60,60,0.3)] bg-[rgba(220,60,60,0.08)] px-3 py-2 text-xs text-[rgba(255,180,180,0.9)]"
    >
      {{ error }}
    </div>

    <Transition name="concept-slide">
      <div
        v-if="showCreateForm"
        class="flex flex-col gap-2 rounded-xl border border-[rgba(255,255,255,0.06)] bg-[rgba(0,0,0,0.15)] p-3"
      >
        <input
          v-model="newName"
          type="text"
          placeholder="概念名称"
          class="w-full rounded-lg border border-[rgba(255,255,255,0.08)] bg-[rgba(0,0,0,0.2)] px-3 py-1.5 text-xs text-[rgba(255,233,220,0.88)] placeholder:text-white/25 focus:border-[rgba(240,138,75,0.4)] focus:outline-none"
        />
        <input
          v-model="newDescription"
          type="text"
          placeholder="概念描述（可选）"
          class="w-full rounded-lg border border-[rgba(255,255,255,0.08)] bg-[rgba(0,0,0,0.2)] px-3 py-1.5 text-xs text-[rgba(255,233,220,0.88)] placeholder:text-white/25 focus:border-[rgba(240,138,75,0.4)] focus:outline-none"
        />
        <div class="flex gap-2">
          <button
            class="rounded-lg bg-[rgba(240,138,75,0.2)] px-3 py-1 text-xs text-[rgba(255,200,160,0.9)] hover:bg-[rgba(240,138,75,0.3)] transition-colors"
            @click="createConcept"
          >
            确认创建
          </button>
          <button
            class="rounded-lg px-3 py-1 text-xs text-white/40 hover:text-white/60 transition-colors"
            @click="showCreateForm = false"
          >
            取消
          </button>
        </div>
      </div>
    </Transition>

    <div v-if="suggestions.length > 0" class="flex flex-col gap-2">
      <span class="text-[0.7rem] tracking-[0.18em] uppercase text-[rgba(186,206,226,0.5)]">
        LLM 建议 · 待审阅
      </span>
      <div
        v-for="s in suggestions"
        :key="s.name"
        class="flex items-start justify-between gap-3 rounded-xl border border-[rgba(96,165,250,0.15)] bg-[rgba(30,58,95,0.15)] px-3 py-2.5"
      >
        <div class="min-w-0 flex-1">
          <div class="text-xs font-medium text-[rgba(255,233,220,0.85)]">{{ s.name }}</div>
          <p v-if="s.description" class="mt-1 text-[11px] leading-relaxed text-white/40">
            {{ s.description }}
          </p>
        </div>
        <div class="flex shrink-0 gap-1.5">
          <button
            class="rounded-md bg-[rgba(34,197,94,0.15)] px-2 py-1 text-[11px] text-[rgba(134,239,172,0.85)] hover:bg-[rgba(34,197,94,0.25)] transition-colors"
            @click="acceptSuggestion(s)"
          >
            接受
          </button>
          <button
            class="rounded-md bg-[rgba(239,68,68,0.12)] px-2 py-1 text-[11px] text-[rgba(252,165,165,0.8)] hover:bg-[rgba(239,68,68,0.22)] transition-colors"
            @click="rejectSuggestion(s)"
          >
            拒绝
          </button>
        </div>
      </div>
    </div>

    <div class="flex flex-col gap-1.5">
      <span class="text-[0.7rem] tracking-[0.18em] uppercase text-[rgba(186,206,226,0.5)]">
        活跃概念 · {{ concepts.length }}
      </span>

      <div
        v-if="loading"
        class="py-4 text-center text-xs text-white/30"
      >
        加载中...
      </div>
      <div
        v-else-if="concepts.length === 0"
        class="py-4 text-center text-xs text-white/25"
      >
        暂无板块概念
      </div>

      <div
        v-for="c in concepts"
        :key="c.id"
        class="flex items-start justify-between gap-3 rounded-xl border border-[rgba(255,255,255,0.04)] bg-[rgba(0,0,0,0.12)] px-3 py-2.5"
      >
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="text-xs font-medium text-[rgba(255,233,220,0.85)]">{{ c.name }}</span>
            <span
              v-if="c.is_system"
              class="rounded px-1.5 py-px text-[10px] text-white/30 bg-white/5"
            >
              系统
            </span>
          </div>
          <p v-if="c.description" class="mt-1 text-[11px] leading-relaxed text-white/40">
            {{ c.description }}
          </p>
        </div>
        <button
          v-if="!c.is_system"
          class="shrink-0 rounded-md px-2 py-1 text-[11px] text-white/35 hover:bg-[rgba(239,68,68,0.15)] hover:text-[rgba(252,165,165,0.8)] transition-colors"
          @click="deactivateConcept(c.id)"
        >
          停用
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.concept-slide-enter-active,
.concept-slide-leave-active {
  transition: all 0.2s ease;
}
.concept-slide-enter-from,
.concept-slide-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>
