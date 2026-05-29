<script setup lang="ts">
import { Icon } from '@iconify/vue'

const props = defineProps<{
  icon?: string
  color?: string
  size?: number
  feedId?: string
  articleLink?: string
}>()

const articlesStore = useArticlesStore()

// Get favicon from article link as fallback
const fallbackIcon = computed(() => {
  if (props.icon) return null

  // If article link is provided directly, use it
  if (props.articleLink) {
    return getFaviconFromUrl(props.articleLink)
  }

  // Otherwise, try to get from first article of the feed
  if (props.feedId) {
    const feedArticles = articlesStore.articles.filter(a => a.feedId === props.feedId)
    const firstArticle = feedArticles[0]
    if (firstArticle?.link) {
      return getFaviconFromUrl(firstArticle.link)
    }
  }

  return null
})

const isUrl = computed(() => {
  const iconToCheck = fallbackIcon.value || props.icon
  return iconToCheck && (iconToCheck.startsWith('http://') || iconToCheck.startsWith('https://'))
})

const displayIcon = computed(() => {
  return fallbackIcon.value || props.icon
})

const iconSize = computed(() => props.size || 20)

// Extract domain and build favicon URL
function getFaviconFromUrl(url: string): string | null {
  try {
    const urlObj = new URL(url)
    const protocol = urlObj.protocol
    const hostname = urlObj.hostname
    // Try to get favicon from the site's root
    return `${protocol}//${hostname}/favicon.ico`
  } catch {
    return null
  }
}
</script>

<template>
  <img
    v-if="isUrl"
    :src="displayIcon"
    :width="iconSize"
    :height="iconSize"
    class="object-contain"
    :style="{ color }"
    @error="(e) => { (e.target as HTMLImageElement).style.display = 'none' }"
  >
  <Icon
    v-else
    :icon="displayIcon || 'mdi:rss'"
    :width="iconSize"
    :height="iconSize"
    :style="{ color }"
  />
</template>
