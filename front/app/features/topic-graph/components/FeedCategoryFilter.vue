<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import { useFeedsStore } from '~/stores/feeds'
import type { RssFeed } from '~/types/feed'
import type { Category } from '~/types/category'

interface Props {
  selectedCategoryId: string | null
  selectedFeedId: string | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:selectedCategoryId': [value: string | null]
  'update:selectedFeedId': [value: string | null]
}>()

const feedsStore = useFeedsStore()
const expandedCategoryId = ref<string | null>(null)

const categories = computed<Category[]>(() => feedsStore.categories)
const feeds = computed<RssFeed[]>(() => feedsStore.feeds)

function autoSelectFirstCategory() {
  if (!props.selectedCategoryId && !props.selectedFeedId && categories.value.length > 0) {
    const first = categories.value[0]
    if (first) {
      emit('update:selectedCategoryId', first.id)
      expandedCategoryId.value = first.id
    }
  }
}

onMounted(() => {
  autoSelectFirstCategory()
})

watch(categories, () => {
  autoSelectFirstCategory()
})

const feedsByCategory = computed(() => {
  const grouped = new Map<string, RssFeed[]>()
  const uncategorized: RssFeed[] = []
  for (const feed of feeds.value) {
    if (feed.category) {
      const catId = feed.category
      const list = grouped.get(catId) || []
      list.push(feed)
      grouped.set(catId, list)
    } else {
      uncategorized.push(feed)
    }
  }
  return { grouped, uncategorized }
})

function selectAll() {
  expandedCategoryId.value = null
  emit('update:selectedCategoryId', null)
  emit('update:selectedFeedId', null)
}

function toggleCategory(categoryId: string) {
  if (expandedCategoryId.value === categoryId) {
    expandedCategoryId.value = null
  } else {
    expandedCategoryId.value = categoryId
  }
  emit('update:selectedCategoryId', categoryId)
  emit('update:selectedFeedId', null)
}

function selectFeed(feedId: string) {
  emit('update:selectedFeedId', feedId)
  emit('update:selectedCategoryId', null)
}
</script>

<template>
  <div class="feed-category-filter space-y-1">
    <button
      class="flex items-center gap-2 px-3 py-1.5 rounded-full text-sm cursor-pointer transition-colors text-white/60 hover:text-white hover:bg-white/10"
      :class="{ 'bg-white/10 text-white': !props.selectedCategoryId && !props.selectedFeedId }"
      @click="selectAll"
    >
      <Icon icon="mdi:view-grid-outline" class="text-base" />
      <span>全部</span>
    </button>

    <div
      v-for="category in categories"
      :key="category.id"
    >
      <button
        class="flex items-center gap-2 px-3 py-2 rounded-xl cursor-pointer transition-colors w-full text-left"
        :class="{ 'bg-white/10': props.selectedCategoryId === category.id }"
        @click="toggleCategory(category.id)"
      >
        <Icon :icon="category.icon || 'mdi:folder-outline'" class="text-lg" :style="{ color: category.color || undefined }" />
        <span class="text-white/80 flex-1">{{ category.name }}</span>
        <span
          class="text-xs px-1.5 py-0.5 rounded-full text-white/50"
          :style="{ backgroundColor: category.color ? `${category.color}22` : 'rgba(255,255,255,0.06)' }"
        >
          {{ feedsByCategory.grouped.get(category.id)?.length || 0 }}
        </span>
        <Icon
          icon="mdi:chevron-down"
          class="text-white/30 transition-transform"
          :class="{ 'rotate-180': expandedCategoryId === category.id }"
        />
      </button>

      <div v-if="expandedCategoryId === category.id" class="space-y-0.5 mt-0.5">
        <button
          v-for="feed in feedsByCategory.grouped.get(category.id) || []"
          :key="feed.id"
          class="flex items-center gap-2 px-3 py-1.5 pl-8 rounded-lg cursor-pointer transition-colors text-sm w-full text-left"
          :class="{ 'bg-white/10': props.selectedFeedId === feed.id }"
          @click="selectFeed(feed.id)"
        >
          <Icon :icon="feed.icon || 'mdi:rss'" class="text-base text-white/40" />
          <span class="text-white/60 truncate">{{ feed.title }}</span>
        </button>
      </div>
    </div>

    <div v-if="feedsByCategory.uncategorized.length > 0">
      <button
        class="flex items-center gap-2 px-3 py-2 rounded-xl cursor-pointer transition-colors w-full text-left"
        :class="{ 'bg-white/10': props.selectedCategoryId === '__uncategorized__' }"
        @click="toggleCategory('__uncategorized__')"
      >
        <Icon icon="mdi:help-circle-outline" class="text-lg text-white/40" />
        <span class="text-white/80 flex-1">未分类</span>
        <span class="text-xs px-1.5 py-0.5 rounded-full text-white/50 bg-white/5">
          {{ feedsByCategory.uncategorized.length }}
        </span>
        <Icon
          icon="mdi:chevron-down"
          class="text-white/30 transition-transform"
          :class="{ 'rotate-180': expandedCategoryId === '__uncategorized__' }"
        />
      </button>

      <div v-if="expandedCategoryId === '__uncategorized__'" class="space-y-0.5 mt-0.5">
        <button
          v-for="feed in feedsByCategory.uncategorized"
          :key="feed.id"
          class="flex items-center gap-2 px-3 py-1.5 pl-8 rounded-lg cursor-pointer transition-colors text-sm w-full text-left"
          :class="{ 'bg-white/10': props.selectedFeedId === feed.id }"
          @click="selectFeed(feed.id)"
        >
          <Icon :icon="feed.icon || 'mdi:rss'" class="text-base text-white/40" />
          <span class="text-white/60 truncate">{{ feed.title }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
