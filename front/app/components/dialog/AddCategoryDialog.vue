<script setup lang="ts">
import { Icon } from "@iconify/vue";

const emit = defineEmits<{
  close: []
  added: []
}>()

const apiStore = useApiStore()

const name = ref('')
const icon = ref('mdi:folder')
const color = ref('#6b7280')
const description = ref('')
const loading = ref(false)
const error = ref<string | null>(null)

const colorOptions = [
  '#3b6b87', // ink blue
  '#c12f2f', // print red
  '#2d8a7a', // teal
  '#d4883c', // amber
  '#4a5d8a', // indigo
  '#3d7a4a', // forest
  '#5a5a5a', // charcoal
]

const iconOptions = [
  'mdi:folder',
  'mdi:code-tags',
  'mdi:newspaper',
  'mdi:palette',
  'mdi:post',
  'mdi:brain',
  'mdi:cube-outline',
  'mdi:rocket',
  'mdi:book',
  'mdi:school',
]

async function handleSubmit() {
  if (!name.value) return

  loading.value = true
  error.value = null

  const response = await apiStore.createCategory({
    name: name.value,
    icon: icon.value,
    color: color.value,
    description: description.value,
  })

  loading.value = false

  if (response.success) {
    emit('added')
    emit('close')
  } else {
    error.value = response.error || 'Failed to create category'
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition ease-out duration-200"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition ease-in duration-150"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        class="fixed inset-0 bg-black/40 backdrop-blur-sm flex items-center justify-center z-50 p-4"
        @click.self="emit('close')"
      >
        <Transition
          enter-active-class="transition ease-out duration-200"
          enter-from-class="opacity-0 scale-95 translate-y-2"
          enter-to-class="opacity-100 scale-100 translate-y-0"
          leave-active-class="transition ease-in duration-150"
          leave-from-class="opacity-100 scale-100 translate-y-0"
          leave-to-class="opacity-0 scale-95 translate-y-2"
        >
          <div
            class="bg-white/95 backdrop-blur-sm rounded-lg shadow-strong border border-ink-200 w-full max-w-lg overflow-hidden"
            @click.stop
          >
            <!-- Header -->
            <div class="px-6 py-5 bg-linear-to-r from-ink-50 to-paper-cream border-b border-ink-200">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3">
                  <div class="w-11 h-11 rounded-lg bg-linear-to-br from-ink-500 to-ink-700 flex items-center justify-center shadow-lg">
                    <Icon icon="mdi:folder-plus" class="text-white" width="24" height="24" />
                  </div>
                  <div>
                    <h2 class="text-xl font-bold text-gray-800">添加分类</h2>
                    <p class="text-sm text-gray-500">创建一个新的文章分类</p>
                  </div>
                </div>
                <button
                  class="p-2.5 hover:bg-white/70 rounded-xl transition-all hover:shadow-md active:scale-95"
                  @click="emit('close')"
                >
                  <Icon icon="mdi:close" width="20" height="20" class="text-gray-500" />
                </button>
              </div>
            </div>

            <div class="p-6 space-y-5">
              <!-- Name -->
              <div class="space-y-2">
                <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                  <Icon icon="mdi:label" width="16" height="16" class="text-ink-500" />
                  分类名称
                  <span class="text-red-500">*</span>
                </label>
                <input
                  v-model="name"
                  type="text"
                  placeholder="例如：技术博客"
                  class="input w-full"
                  :class="{ 'border-red-300 focus:border-red-500 focus:ring-red-500/10': !name && name !== '' }"
                >
              </div>

              <!-- Icon & Color in Grid -->
              <div class="grid grid-cols-2 gap-5">
                <!-- Icon -->
                <div class="space-y-2">
                  <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                    <Icon icon="mdi:emoticon-happy" width="16" height="16" class="text-ink-500" />
                    图标
                  </label>
                  <div class="grid grid-cols-5 gap-2">
                    <button
                      v-for="iconOption in iconOptions"
                      :key="iconOption"
                      class="p-2.5 rounded-xl border-2 transition-all hover:shadow-md"
                      :class="icon === iconOption ? 'border-ink-500 bg-ink-50 shadow-md scale-105' : 'border-gray-200 hover:border-ink-300 hover:bg-ink-50/50'"
                      @click="icon = iconOption"
                    >
                      <Icon :icon="iconOption" width="20" height="20" :class="icon === iconOption ? 'text-ink-600' : 'text-gray-600'" />
                    </button>
                  </div>
                  <!-- Preview -->
                  <div class="flex items-center gap-2 px-3 py-2.5 bg-white/60 rounded-xl border border-gray-200/60">
                    <div class="w-9 h-9 rounded-xl flex items-center justify-center shadow-sm" :style="{ backgroundColor: color }">
                      <Icon :icon="icon" width="20" height="20" class="text-white" />
                    </div>
                    <span class="text-sm text-gray-600 font-medium">预览</span>
                  </div>
                </div>

                <!-- Color -->
                <div class="space-y-2">
                  <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                    <Icon icon="mdi:palette" width="16" height="16" class="text-ink-500" />
                    颜色
                  </label>
                  <div class="grid grid-cols-4 gap-2.5">
                    <button
                      v-for="colorOption in colorOptions"
                      :key="colorOption"
                      class="aspect-square rounded-xl border-2 transition-all hover:scale-110 hover:shadow-lg active:scale-100"
                      :class="color === colorOption ? 'border-gray-700 scale-110 shadow-lg ring-4 ring-primary-100' : 'border-transparent hover:border-gray-300'"
                      :style="{ backgroundColor: colorOption }"
                      @click="color = colorOption"
                    >
                      <Icon
                        v-if="color === colorOption"
                        icon="mdi:check"
                        width="16"
                        height="16"
                        class="text-white drop-shadow-md"
                      />
                    </button>
                  </div>
                </div>
              </div>

              <!-- Description -->
              <div class="space-y-2">
                <label class="flex items-center gap-2 text-sm font-semibold text-gray-700">
                  <Icon icon="mdi:text-box" width="16" height="16" class="text-ink-500" />
                  描述
                  <span class="text-xs font-normal text-gray-400">(可选)</span>
                </label>
                <textarea
                  v-model="description"
                  placeholder="添加分类描述..."
                  class="input w-full resize-none"
                  rows="3"
                />
              </div>

              <!-- Error -->
              <Transition
                enter-active-class="transition ease-out duration-200"
                enter-from-class="opacity-0 translate-y-1"
                enter-to-class="opacity-100 translate-y-0"
                leave-active-class="transition ease-in duration-150"
                leave-from-class="opacity-100 translate-y-0"
                leave-to-class="opacity-0 translate-y-1"
              >
                <div
                  v-if="error"
                  class="flex items-start gap-3 p-4 bg-red-50/80 backdrop-blur-sm border-2 border-red-200 rounded-lg"
                >
                  <Icon icon="mdi:alert-circle" width="20" height="20" class="text-red-500 shrink-0 mt-0.5" />
                  <p class="text-sm font-medium text-red-700">{{ error }}</p>
                </div>
              </Transition>
            </div>

            <!-- Footer -->
            <div class="px-6 py-4 bg-white/40 border-t border-white/40 flex justify-end gap-3">
              <button
                class="btn-secondary"
                @click="emit('close')"
              >
                取消
              </button>
              <button
                class="btn-primary flex items-center gap-2"
                :disabled="!name || loading"
                @click="handleSubmit"
              >
                <Icon
                  :icon="loading ? 'mdi:loading' : 'mdi:plus'"
                  :class="{ 'animate-spin': loading }"
                  width="18"
                  height="18"
                />
                {{ loading ? '创建中...' : '创建分类' }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
