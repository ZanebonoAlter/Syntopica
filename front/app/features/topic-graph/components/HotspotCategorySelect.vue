<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, nextTick, ref, watch } from 'vue'
import type { TopicTag } from '~/api/topicGraph'

interface Props {
  label: string
  icon: string
  topics: TopicTag[]
  placeholder?: string
  selectedSlug?: string | null
  headerClass?: string
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: '搜索主题...',
  selectedSlug: null,
  headerClass: '',
})

const emit = defineEmits<{
  select: [slug: string]
}>()

// Search state
const searchQuery = ref('')
const isDropdownOpen = ref(false)
const highlightedIndex = ref(-1)
const dropdownRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)

// Filter topics based on search query
const filteredTopics = computed(() => {
  if (!searchQuery.value.trim()) {
    return props.topics
  }
  
  const query = searchQuery.value.toLowerCase()
  return props.topics.filter(topic => 
    topic.label.toLowerCase().includes(query) ||
    topic.slug.toLowerCase().includes(query)
  )
})

// Show empty state
const showEmptyState = computed(() => {
  return searchQuery.value.trim() && filteredTopics.value.length === 0
})

// Handle keyboard navigation
function handleKeydown(event: KeyboardEvent) {
  if (!isDropdownOpen.value) {
    if (event.key === 'ArrowDown' || event.key === 'Enter') {
      isDropdownOpen.value = true
      nextTick(() => {
        highlightedIndex.value = 0
        scrollToHighlighted()
      })
    }
    return
  }

  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      highlightedIndex.value = Math.min(
        highlightedIndex.value + 1,
        filteredTopics.value.length - 1
      )
      scrollToHighlighted()
      break
    case 'ArrowUp':
      event.preventDefault()
      highlightedIndex.value = Math.max(highlightedIndex.value - 1, 0)
      scrollToHighlighted()
      break
    case 'Enter':
      event.preventDefault()
      if (highlightedIndex.value >= 0 && highlightedIndex.value < filteredTopics.value.length) {
        const topic = filteredTopics.value[highlightedIndex.value]
        if (topic) {
          selectTopic(topic)
        }
      }
      break
    case 'Escape':
      isDropdownOpen.value = false
      highlightedIndex.value = -1
      break
  }
}

function scrollToHighlighted() {
  nextTick(() => {
    if (!dropdownRef.value) return
    const highlightedElement = dropdownRef.value.querySelector(`[data-index="${highlightedIndex.value}"]`)
    if (highlightedElement) {
      highlightedElement.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    }
  })
}

function selectTopic(topic: TopicTag) {
  emit('select', topic.slug)
  searchQuery.value = ''
  isDropdownOpen.value = false
  highlightedIndex.value = -1
}

function toggleDropdown() {
  isDropdownOpen.value = !isDropdownOpen.value
  if (isDropdownOpen.value) {
    nextTick(() => {
      searchInputRef.value?.focus()
    })
  }
}

// Close dropdown when clicking outside
function handleClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.hotspot-category-select')) {
    isDropdownOpen.value = false
    highlightedIndex.value = -1
  }
}

// Watch for topics change to reset highlight
watch(() => props.topics, () => {
  highlightedIndex.value = -1
}, { deep: true })

// Add click outside listener
onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<template>
  <div class="hotspot-category-select" :class="{ 'is-open': isDropdownOpen }">
    <!-- Header -->
    <div 
      class="hotspot-category-header" 
      :class="headerClass"
      @click="toggleDropdown"
    >
      <div class="header-content">
        <Icon :icon="icon" width="16" />
        <span class="header-label">{{ label }}</span>
        <span class="header-count" v-if="topics.length > 0">{{ topics.length }}</span>
      </div>
      <Icon 
        icon="mdi:chevron-down" 
        width="18" 
        class="header-arrow"
        :class="{ 'is-rotated': isDropdownOpen }"
      />
    </div>

    <!-- Dropdown Panel -->
    <Transition name="dropdown">
      <div 
        v-show="isDropdownOpen" 
        class="hotspot-dropdown-panel"
        ref="dropdownRef"
        @keydown="handleKeydown"
      >
        <!-- Search Input -->
        <div class="search-container">
          <Icon icon="mdi:magnify" width="16" class="search-icon" />
          <input
            ref="searchInputRef"
            v-model="searchQuery"
            type="text"
            :placeholder="placeholder"
            class="search-input"
            @keydown="handleKeydown"
          />
          <button 
            v-if="searchQuery" 
            class="search-clear"
            @click="searchQuery = ''"
          >
            <Icon icon="mdi:close" width="14" />
          </button>
        </div>

        <!-- Topic List -->
        <div class="topics-list" v-if="filteredTopics.length > 0">
          <button
            v-for="(topic, index) in filteredTopics"
            :key="topic.slug"
            type="button"
            class="topic-item"
            :class="{ 
              'is-highlighted': index === highlightedIndex,
              'is-selected': selectedSlug === topic.slug 
            }"
            :data-index="index"
            @click="selectTopic(topic)"
            @mouseenter="highlightedIndex = index"
          >
            <div class="topic-icon">
              <Icon v-if="topic.icon" :icon="topic.icon" width="14" />
              <Icon v-else icon="mdi:tag-outline" width="14" />
            </div>
            <span class="topic-label">{{ topic.label }}</span>
            <span v-if="topic.score" class="topic-score">
              {{ Math.round(topic.score * 100) / 100 }}
            </span>
          </button>
        </div>

        <!-- Empty State -->
        <div v-else-if="showEmptyState" class="empty-state">
          <Icon icon="mdi:text-search" width="32" />
          <p>未找到匹配 "{{ searchQuery }}" 的主题</p>
        </div>

        <!-- No Topics Available -->
        <div v-else-if="topics.length === 0" class="empty-state">
          <Icon icon="mdi:tag-off" width="32" />
          <p>暂无主题数据</p>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.hotspot-category-select {
  position: relative;
  width: 100%;
}

.hotspot-category-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  cursor: pointer;
  transition: all 0.2s ease;
}

.hotspot-category-header:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.15);
}

.is-open .hotspot-category-header {
  background: rgba(255, 255, 255, 0.1);
  border-color: rgba(255, 255, 255, 0.2);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: rgba(255, 255, 255, 0.9);
}

.header-label {
  font-size: 0.9rem;
  font-weight: 500;
}

.header-count {
  padding: 0.15rem 0.4rem;
  border-radius: 999px;
  background: rgba(240, 138, 75, 0.2);
  color: rgba(240, 138, 75, 0.9);
  font-size: 0.75rem;
  font-weight: 600;
}

.header-arrow {
  color: rgba(255, 255, 255, 0.5);
  transition: transform 0.2s ease;
}

.header-arrow.is-rotated {
  transform: rotate(180deg);
}

/* Dropdown Panel */
.hotspot-dropdown-panel {
  position: absolute;
  top: calc(100% + 0.5rem);
  left: 0;
  right: 0;
  max-height: 400px;
  border-radius: 16px;
  background: rgba(30, 30, 35, 0.95);
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.4);
  z-index: 100;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

/* Search Container */
.search-container {
  position: relative;
  padding: 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.search-icon {
  position: absolute;
  left: 1.5rem;
  top: 50%;
  transform: translateY(-50%);
  color: rgba(255, 255, 255, 0.4);
  pointer-events: none;
}

.search-input {
  width: 100%;
  padding: 0.6rem 2.5rem;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.9);
  font-size: 0.9rem;
  outline: none;
  transition: all 0.2s ease;
}

.search-input::placeholder {
  color: rgba(255, 255, 255, 0.4);
}

.search-input:focus {
  border-color: rgba(240, 138, 75, 0.5);
  background: rgba(255, 255, 255, 0.08);
}

.search-clear {
  position: absolute;
  right: 1.5rem;
  top: 50%;
  transform: translateY(-50%);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 50%;
  border: none;
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  transition: all 0.2s ease;
}

.search-clear:hover {
  background: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.9);
}

/* Topics List */
.topics-list {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem;
}

.topics-list::-webkit-scrollbar {
  width: 6px;
}

.topics-list::-webkit-scrollbar-track {
  background: transparent;
}

.topics-list::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.15);
  border-radius: 3px;
}

.topics-list::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.25);
}

/* Topic Item */
.topic-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  padding: 0.6rem 0.75rem;
  border-radius: 10px;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.8);
  font-size: 0.85rem;
  text-align: left;
  cursor: pointer;
  transition: all 0.15s ease;
}

.topic-item:hover,
.topic-item.is-highlighted {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.95);
}

.topic-item.is-selected {
  background: rgba(240, 138, 75, 0.15);
  color: rgba(240, 138, 75, 0.95);
}

.topic-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.05);
  color: rgba(255, 255, 255, 0.5);
  flex-shrink: 0;
}

.topic-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.topic-score {
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.75rem;
  font-weight: 500;
  flex-shrink: 0;
}

/* Empty State */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem 1rem;
  color: rgba(255, 255, 255, 0.4);
  text-align: center;
}

.empty-state svg {
  margin-bottom: 0.75rem;
  opacity: 0.5;
}

.empty-state p {
  font-size: 0.85rem;
  margin: 0;
}

/* Dropdown Animation */
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px) scale(0.96);
}
</style>
