<script setup lang="ts">
import { Icon } from "@iconify/vue";
import type { Category } from '~/types'

interface Props {
  category: Category
}

const props = defineProps<Props>()

const emit = defineEmits<{
  click: [category: Category]
}>()
</script>

<template>
  <div
    class="card group cursor-pointer overflow-hidden"
    @click="emit('click', category)"
  >
    <div class="p-6">
      <div class="flex items-start justify-between">
        <div class="flex items-center gap-3">
          <div
            class="w-12 h-12 rounded-xl flex items-center justify-center"
            :style="{ backgroundColor: category.color + '20' }"
          >
            <Icon
              :icon="category.icon"
              :style="{ color: category.color }"
              width="24"
              height="24"
            />
          </div>
          <div>
            <h3 class="font-semibold text-lg text-gray-900 group-hover:text-blue-600 transition-colors">
              {{ category.name }}
            </h3>
            <p class="text-sm text-gray-500 mt-0.5">{{ category.description }}</p>
          </div>
        </div>
        <div class="flex flex-col items-end">
          <span class="text-2xl font-bold" :style="{ color: category.color }">
            {{ category.feedCount }}
          </span>
          <span class="text-xs text-gray-400">订阅源</span>
        </div>
      </div>
    </div>
    <div
      class="h-1 transition-all duration-300"
      :style="{ backgroundColor: category.color, width: category.feedCount > 0 ? '100%' : '0%' }"
    />
  </div>
</template>
