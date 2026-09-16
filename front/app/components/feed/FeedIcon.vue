<script setup lang="ts">
import { ref } from 'vue'
import { Icon } from '@iconify/vue'
import { getApiOrigin } from '~/utils/api'

const props = defineProps<{
  icon?: string
  color?: string
  size?: number
  feedId?: string
  articleLink?: string
}>()

// favicon retrieval is owned by the backend icon state machine. The component's
// only job here is graceful degradation: render the icon value as-is, and when
// an image URL fails to load, fall back to the mdi:rss placeholder rather than
// leaving a blank gap (the old display:none behavior).
const imgFailed = ref(false)

const isUrl = computed(() =>
  Boolean(props.icon && (props.icon.startsWith('http://') || props.icon.startsWith('https://'))),
)

// Same-origin relative path served by the backend (e.g. /icons/feeds/42.png).
// Resolve it against the API origin (dev: http://localhost:5000, prod: same
// origin as the page).
const isLocalPath = computed(() => Boolean(props.icon?.startsWith('/')))

const imgSrc = computed(() => {
  if (!props.icon) return ''
  return isLocalPath.value ? `${getApiOrigin()}${props.icon}` : props.icon
})

// Only a real iconify name may reach <Icon>. A local path (/icons/feeds/2.ico),
// a remote image URL, a data: URL or a legacy placeholder like 'rss' is not a
// resolvable iconify name: passing it through renders an empty <svg> — a blank
// gap instead of the placeholder. Iconify name syntax is `<prefix>:<name>` with
// lowercase alphanumeric-hyphen segments (mdi:rss, simple-icons:nuxtdotjs).
const ICONIFY_NAME_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*:[a-z0-9]+(?:-[a-z0-9]+)*$/

const iconifyName = computed(() =>
  props.icon && ICONIFY_NAME_RE.test(props.icon) ? props.icon : '',
)

const placeholderIcon = computed(() => iconifyName.value || 'mdi:rss')

const iconSize = computed(() => props.size || 20)

// Reset the failure flag when the icon prop changes (e.g. feed switched).
watch(() => props.icon, () => {
  imgFailed.value = false
})
</script>

<template>
  <img
    v-if="(isUrl || isLocalPath) && !imgFailed"
    :src="imgSrc"
    :width="iconSize"
    :height="iconSize"
    class="object-contain"
    :style="{ color }"
    @error="imgFailed = true"
  >
  <Icon
    v-else
    :icon="placeholderIcon"
    :width="iconSize"
    :height="iconSize"
    :style="{ color }"
  />
</template>
