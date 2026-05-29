<script setup lang="ts">
import { Icon } from "@iconify/vue";

const emit = defineEmits<{
  close: []
  imported: []
}>()

const apiStore = useApiStore()

const file = ref<File | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const dragging = ref(false)
const importResult = ref<{ feeds: number; categories: number } | null>(null)

function handleFileSelect(event: Event) {
  const target = event.target as HTMLInputElement
  if (target.files && target.files[0]) {
    file.value = target.files[0]
  }
}

function handleDragOver(event: DragEvent) {
  event.preventDefault()
  dragging.value = true
}

function handleDragLeave() {
  dragging.value = false
}

function handleDrop(event: DragEvent) {
  event.preventDefault()
  dragging.value = false

  if (event.dataTransfer?.files && event.dataTransfer.files[0]) {
    file.value = event.dataTransfer.files[0]
  }
}

async function handleImport() {
  if (!file.value) return

  loading.value = true
  error.value = null
  importResult.value = null

  const response = await apiStore.importOpml(file.value)

  loading.value = false

  if (response.success) {
    importResult.value = {
      feeds: (response.data?.feeds_added as number) || 0,
      categories: (response.data?.categories_added as number) || 0
    }

    // Show success for 2 seconds before closing
    setTimeout(() => {
      emit('imported')
      emit('close')
    }, 2000)
  } else {
    error.value = response.error || 'Failed to import OPML'
  }
}
</script>

<template>
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
    @click.self="emit('close')"
  >
    <div
      class="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 overflow-hidden"
      @click.stop
    >
      <div class="px-6 py-4 border-b border-gray-200 flex items-center justify-between">
        <h2 class="text-xl font-bold text-gray-900">导入 OPML</h2>
        <button
          class="p-2 hover:bg-gray-100 rounded-lg transition-colors"
          @click="emit('close')"
        >
          <Icon icon="mdi:close" width="20" height="20" />
        </button>
      </div>

      <div class="p-6 space-y-4">
        <!-- Drop Zone -->
        <div
          class="border-2 border-dashed rounded-xl p-8 text-center transition-colors cursor-pointer"
          :class="
            dragging
              ? 'border-blue-500 bg-blue-50'
              : 'border-gray-300 hover:border-gray-400'
          "
          @dragover="handleDragOver"
          @dragleave="handleDragLeave"
          @drop="handleDrop"
          @click="($refs.fileInput as HTMLInputElement).click()"
        >
          <input
            ref="fileInput"
            type="file"
            accept=".opml,.xml"
            class="hidden"
            @change="handleFileSelect"
          >
          <Icon
            :icon="file ? 'mdi:file-check' : 'mdi:file-upload'"
            width="48"
            height="48"
            class="mx-auto mb-4"
            :class="file ? 'text-green-500' : 'text-gray-400'"
          />
          <p class="text-gray-700 font-medium mb-1">
            {{ file ? file.name : '点击或拖拽 OPML 文件到此处' }}
          </p>
          <p class="text-sm text-gray-500">
            支持 .opml 和 .xml 格式
          </p>
        </div>

        <!-- Help Text -->
        <div class="p-4 bg-blue-50 border border-blue-200 rounded-lg">
          <h3 class="font-medium text-blue-900 mb-2 flex items-center gap-2">
            <Icon icon="mdi:information" width="18" height="18" />
            关于 OPML
          </h3>
          <p class="text-sm text-blue-700">
            OPML 是一种常用的 RSS 订阅源导出格式。你可以从其他 RSS 阅读器（如 Feedly、Inoreader）导出 OPML 文件，然后在这里导入。
          </p>
        </div>

        <!-- Error -->
        <div
          v-if="error"
          class="p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-600"
        >
          {{ error }}
        </div>

        <!-- Success Message -->
        <div
          v-if="importResult"
          class="p-4 bg-green-50 border border-green-200 rounded-lg"
        >
          <div class="flex items-center gap-2 text-green-700 mb-2">
            <Icon icon="mdi:check-circle" width="20" height="20" />
            <span class="font-medium">导入成功！</span>
          </div>
          <p class="text-sm text-green-600">
            已导入 {{ importResult.feeds }} 个订阅源和 {{ importResult.categories }} 个分类。<br>
            订阅源元数据正在后台更新中，请稍后刷新查看。
          </p>
        </div>
      </div>

      <div class="px-6 py-4 bg-gray-50 border-t border-gray-200 flex justify-between items-center">
        <a
          href="https://feedly.com/i/opml"
          target="_blank"
          class="text-sm text-blue-600 hover:text-blue-700 flex items-center gap-1"
        >
          <Icon icon="mdi:help-circle" width="16" height="16" />
          如何导出 OPML？
        </a>
        <div class="flex gap-3">
          <button class="btn btn-secondary" @click="emit('close')">
            取消
          </button>
          <button
            class="btn btn-primary"
            :disabled="!file || loading"
            @click="handleImport"
          >
            <Icon
              :icon="loading ? 'mdi:loading' : 'mdi:upload'"
              :class="{ 'animate-spin': loading }"
              width="18"
              height="18"
            />
            {{ loading ? '导入中...' : '导入' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
