<script setup lang="ts">
import { Icon } from '@iconify/vue'

interface Feed {
  id: string
  name: string
  icon?: string
  color?: string
  categoryId?: string
}

interface Category {
  id: string
  name: string
  icon?: string
  color?: string
  feeds?: Feed[]
}

const props = defineProps<{
  visible: boolean
  categories: Category[]
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  confirm: [selectedCategoryIds: string[]]
}>()

const selectedCategoryIds = ref<string[]>([])
const selectAll = ref(false)

watch(() => props.visible, (newVal) => {
  if (newVal) {
    selectedCategoryIds.value = props.categories.map(c => c.id)
    selectAll.value = true
  }
})

function toggleSelectAll() {
  if (selectAll.value) {
    selectedCategoryIds.value = props.categories.map(c => c.id)
  } else {
    selectedCategoryIds.value = []
  }
}

function toggleCategory(categoryId: string) {
  const index = selectedCategoryIds.value.indexOf(categoryId)
  if (index === -1) {
    selectedCategoryIds.value.push(categoryId)
  } else {
    selectedCategoryIds.value.splice(index, 1)
  }
  selectAll.value = selectedCategoryIds.value.length === props.categories.length
}

function isSelected(categoryId: string): boolean {
  return selectedCategoryIds.value.includes(categoryId)
}

function handleConfirm() {
  emit('confirm', selectedCategoryIds.value)
  emit('update:visible', false)
}

function handleCancel() {
  emit('update:visible', false)
}

function getFeedCount(categoryId: string): number {
  const category = props.categories.find(c => c.id === categoryId)
  return category?.feeds?.length || 0
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-all duration-300 ease-out"
      enter-from-class="opacity-0 scale-95"
      enter-to-class="opacity-100 scale-100"
      leave-active-class="transition-all duration-200 ease-in"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-if="visible"
        class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
        @click.self="handleCancel"
      >
        <div class="bg-white rounded-2xl shadow-2xl w-full max-w-md overflow-hidden">
          <div class="px-6 py-4 border-b border-gray-100 bg-gradient-to-r from-ink-50 to-white">
            <div class="flex items-center justify-between">
              <h3 class="text-lg font-semibold text-ink-black flex items-center gap-2">
                <Icon icon="mdi:rss-box" width="20" height="20" class="text-ink-600" />
                选择订阅源
              </h3>
              <button
                class="p-1.5 hover:bg-gray-100 rounded-lg transition-colors"
                @click="handleCancel"
              >
                <Icon icon="mdi:close" width="18" height="18" class="text-ink-light" />
              </button>
            </div>
            <p class="text-xs text-ink-medium mt-1">
              选择分类，将为该分类下所有启用的订阅源生成AI总结
            </p>
          </div>

          <div class="p-4 max-h-80 overflow-y-auto">
            <div
              class="flex items-center gap-3 p-3 rounded-xl mb-3 cursor-pointer transition-all"
              :class="selectAll ? 'bg-ink-50 border border-ink-200' : 'bg-gray-50 border border-transparent hover:bg-gray-100'"
              @click="selectAll = !selectAll; toggleSelectAll()"
            >
              <div
                class="w-5 h-5 rounded-md flex items-center justify-center transition-all"
                :class="selectAll ? 'bg-ink-600' : 'bg-white border-2 border-gray-300'"
              >
                <Icon
                  v-if="selectAll"
                  icon="mdi:check"
                  width="14"
                  height="14"
                  class="text-white"
                />
              </div>
              <span class="font-medium text-sm" :class="selectAll ? 'text-ink-700' : 'text-ink-dark'">
                全部分类
              </span>
              <span class="ml-auto text-xs text-ink-light">
                {{ categories.length }} 个分类
              </span>
            </div>

            <div class="space-y-2">
              <div
                v-for="category in categories"
                :key="category.id"
                class="flex items-center gap-3 p-3 rounded-xl cursor-pointer transition-all"
                :class="isSelected(category.id) ? 'bg-ink-50 border border-ink-200' : 'bg-gray-50 border border-transparent hover:bg-gray-100'"
                @click="toggleCategory(category.id)"
              >
                <div
                  class="w-5 h-5 rounded-md flex items-center justify-center transition-all"
                  :class="isSelected(category.id) ? 'bg-ink-600' : 'bg-white border-2 border-gray-300'"
                >
                  <Icon
                    v-if="isSelected(category.id)"
                    icon="mdi:check"
                    width="14"
                    height="14"
                    class="text-white"
                  />
                </div>
                <Icon
                  :icon="category.icon || 'mdi:folder'"
                  width="18"
                  height="18"
                  :style="{ color: category.color || '#3b6b87' }"
                />
                <span
                  class="font-medium text-sm flex-1"
                  :class="isSelected(category.id) ? 'text-ink-700' : 'text-ink-dark'"
                >
                  {{ category.name }}
                </span>
                <span class="text-xs text-ink-light">
                  {{ getFeedCount(category.id) }} 源
                </span>
              </div>
            </div>
          </div>

          <div class="px-6 py-4 border-t border-gray-100 bg-gray-50 flex items-center justify-between">
            <span class="text-xs text-ink-medium">
              已选择 {{ selectedCategoryIds.length }} 个分类
            </span>
            <div class="flex items-center gap-2">
              <button
                class="px-4 py-2 text-sm font-medium text-ink-dark bg-white border border-gray-200 rounded-lg hover:bg-gray-50 transition-colors"
                @click="handleCancel"
              >
                取消
              </button>
              <button
                class="px-4 py-2 text-sm font-medium text-white bg-ink-600 rounded-lg hover:bg-ink-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
                :disabled="selectedCategoryIds.length === 0 || loading"
                @click="handleConfirm"
              >
                <Icon
                  v-if="loading"
                  icon="mdi:loading"
                  width="16"
                  height="16"
                  class="animate-spin"
                />
                <span>{{ loading ? '提交中...' : '确认生成' }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.overflow-y-auto::-webkit-scrollbar {
  width: 6px;
}

.overflow-y-auto::-webkit-scrollbar-track {
  background: transparent;
}

.overflow-y-auto::-webkit-scrollbar-thumb {
  background: rgba(59, 107, 135, 0.2);
  border-radius: 3px;
}

.overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: rgba(59, 107, 135, 0.4);
}
</style>
