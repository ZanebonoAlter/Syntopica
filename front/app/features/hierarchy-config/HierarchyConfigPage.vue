<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import type { HierarchyTemplate } from '~/api/hierarchyConfig'

const api = useHierarchyConfigApi()
const templates = ref<HierarchyTemplate[]>([])
const selectedKey = ref('')
const saving = ref(false)
const saved = ref(false)
const loading = ref(true)
const error = ref('')
const saveError = ref('')

onMounted(async () => {
  try {
    const result = await api.getConfig()
    if (result.success && result.data) {
      templates.value = result.data.templates
      const first = templates.value[0]
      if (first && !selectedKey.value) {
        selectedKey.value = templateKey(first)
      }
    } else {
      error.value = result.error || '加载配置失败'
    }
  } catch (e: any) {
    error.value = e.message || '网络错误'
  } finally {
    loading.value = false
  }
})

function templateKey(t: HierarchyTemplate): string {
  return `${t.category}${t.sub_type ? ':' + t.sub_type : ''}`
}

const selectedTemplate = computed(() => {
  return templates.value.find(t => templateKey(t) === selectedKey.value)
})

async function save() {
  saving.value = true
  saveError.value = ''
  try {
    const result = await api.updateConfig(templates.value, 'UI update')
    if (result.success) {
      saved.value = true
      setTimeout(() => saved.value = false, 3000)
    } else {
      saveError.value = result.error || '保存失败'
    }
  } catch (e: any) {
    saveError.value = e.message || '网络错误'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="p-4">
    <h2 class="text-lg font-bold mb-4">层级模板配置</h2>
    <div v-if="loading" class="text-sm text-gray-500 py-8 text-center">加载中...</div>
    <div v-else-if="error" class="text-sm text-red-500 py-4">{{ error }}</div>
    <div v-else class="flex gap-4">
      <div class="w-48 shrink-0">
        <button
          v-for="t in templates"
          :key="templateKey(t)"
          @click="selectedKey = templateKey(t)"
          :class="[
            'block w-full text-left p-2 rounded mb-1 text-sm transition-colors',
            selectedKey === templateKey(t)
              ? 'bg-blue-600 text-white'
              : 'hover:bg-gray-100 dark:hover:bg-gray-800'
          ]"
        >
          {{ t.category }}{{ t.sub_type ? ` (${t.sub_type})` : '' }}
        </button>
      </div>
      <div v-if="selectedTemplate" class="flex-1 space-y-2">
        <div
          v-for="level in selectedTemplate.levels"
          :key="level.level"
          class="border rounded-lg p-3 dark:border-gray-700"
        >
          <div class="flex items-center gap-2">
            <span class="font-bold text-sm w-8">L{{ level.level }}</span>
            <input
              v-model="level.name"
              class="border rounded px-2 py-1 flex-1 text-sm dark:border-gray-600 dark:bg-gray-800"
            />
            <label class="text-sm flex items-center gap-1">
              <input type="checkbox" v-model="level.is_leaf" />
              叶子节点
            </label>
          </div>
          <input
            v-model="level.description"
            class="border rounded px-2 py-1 w-full mt-2 text-sm dark:border-gray-600 dark:bg-gray-800"
            placeholder="层级描述"
          />
        </div>
        <button
          @click="save"
          :disabled="saving"
          class="mt-3 px-4 py-2 bg-green-600 text-white rounded text-sm hover:bg-green-700 disabled:opacity-50"
        >
          {{ saving ? '保存中...' : saved ? '已保存 ✓' : '保存' }}
        </button>
        <p v-if="saveError" class="mt-2 text-sm text-red-500">{{ saveError }}</p>
      </div>
    </div>
  </div>
</template>
