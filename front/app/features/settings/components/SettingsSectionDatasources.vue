<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { onMounted } from 'vue'
import { useDatasources } from '~/composables/useDatasources'

const {
  sources, catalogLoading, catalogError, loadCatalog,
  comtradeEnabled, comtradeApiKey, comtradeApiKeyConfigured, comtradeApiKeyHint,
  comtradeApiKeyVisible, comtradeLoading, comtradeError, comtradeSuccess,
  loadComtrade, saveComtrade, probing, probeResults, runProbe,
} = useDatasources()

onMounted(() => {
  loadCatalog()
  loadComtrade()
})

const freqLabel: Record<string, string> = {
  weekly: '周度',
  monthly: '月度',
  'monthly+annual': '月度+年度',
  annual: '年度',
}
</script>

<template>
  <div class="space-y-6">
    <div v-if="catalogLoading && sources.length === 0" class="flex items-center justify-center py-12">
      <Icon icon="mdi:loading" width="48" height="48" class="animate-spin" style="color: var(--color-link)" />
    </div>
    <template v-else>
      <div v-if="comtradeSuccess" class="p-3 rounded-lg text-sm" style="background: var(--color-success-bg, rgba(61, 138, 74, 0.1)); border: 1px solid var(--color-success-border, rgba(61, 138, 74, 0.25)); color: var(--color-success)">
        {{ comtradeSuccess }}
      </div>
      <div v-if="comtradeError || catalogError" class="p-3 rounded-lg text-sm" style="background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25)); color: var(--color-error)">
        {{ comtradeError || catalogError }}
      </div>

      <div>
        <h3 class="font-semibold" style="color: var(--color-text-primary)">研究数据源</h3>
        <p class="text-sm mt-0.5" style="color: var(--color-text-secondary)">
          官方数据取数源（原油库存/贸易/宏观），供研究分析使用；EIA / JODI / WDI 匿名可用
        </p>
      </div>

      <!-- 四源状态列表 -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div
          v-for="s in sources" :key="s.code"
          class="p-4 rounded-lg border space-y-2"
          style="border-color: var(--color-border)"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span
                class="inline-block w-2 h-2 rounded-full"
                :style="{ background: s.status === 'enabled' ? 'var(--color-success)' : 'var(--color-text-muted)' }"
              />
              <span class="text-sm font-medium" style="color: var(--color-text-primary)">{{ s.name }}</span>
            </div>
            <span class="text-xs" style="color: var(--color-text-muted)">{{ freqLabel[s.frequency] || s.frequency }}</span>
          </div>
          <p class="text-xs" style="color: var(--color-text-secondary)">{{ s.coverage }}</p>
          <p v-if="s.status === 'disabled'" class="text-xs" style="color: var(--color-error)">
            {{ s.status_reason || '未配置订阅 key' }}
          </p>
          <div class="flex items-center justify-between pt-1">
            <span class="text-xs" style="color: var(--color-text-muted)">滞后：{{ s.typical_lag }}</span>
            <button
              type="button"
              class="text-xs px-2 py-1 rounded transition-colors hover:opacity-80"
              style="border: 1px solid var(--color-border); color: var(--color-text-secondary)"
              :disabled="probing[s.code]"
              @click="runProbe(s.code)"
            >
              <span v-if="probing[s.code]" class="flex items-center gap-1">
                <Icon icon="mdi:loading" width="12" class="animate-spin" /> 测试中
              </span>
              <span v-else>测试连通</span>
            </button>
          </div>
          <p
            v-if="probeResults[s.code]"
            class="text-xs"
            :style="{ color: probeResults[s.code]!.ok ? 'var(--color-success)' : 'var(--color-error)' }"
          >
            {{ probeResults[s.code]!.text }}
          </p>
        </div>
      </div>

      <!-- Comtrade key 管理 -->
      <div class="p-4 rounded-lg border space-y-3" style="border-color: var(--color-border)">
        <div class="flex items-center justify-between">
          <div>
            <h4 class="text-sm font-medium" style="color: var(--color-text-primary)">UN Comtrade 订阅 Key</h4>
            <p class="text-xs mt-0.5" style="color: var(--color-text-secondary)">
              免费档 500 次/天；在 comtradedeveloper.un.org 订阅 Free APIs 获取；界面配置即时生效，环境变量作兜底
            </p>
          </div>
          <AppToggle v-model="comtradeEnabled" />
        </div>
        <div>
          <label class="block text-sm font-medium mb-1" style="color: var(--color-text-secondary)">API Key</label>
          <div class="relative">
            <input
              :type="comtradeApiKeyVisible ? 'text' : 'password'"
              v-model="comtradeApiKey"
              class="w-full px-3 py-2 rounded-lg text-sm pr-10 input"
              :placeholder="comtradeApiKeyConfigured ? `已配置（末 4 位 ${comtradeApiKeyHint}），留空保持不变` : 'comtrade primary key'"
            >
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 hover:opacity-80"
              style="color: var(--color-text-muted)"
              @click="comtradeApiKeyVisible = !comtradeApiKeyVisible"
            >
              <Icon :icon="comtradeApiKeyVisible ? 'mdi:eye-off' : 'mdi:eye'" width="18" />
            </button>
          </div>
          <p v-if="comtradeApiKeyConfigured" class="text-xs mt-1" style="color: var(--color-text-muted)">
            当前已配置 Key，留空保存不修改；输入新值则覆盖
          </p>
        </div>
        <div class="flex justify-end">
          <button
            type="button"
            class="px-4 py-2 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            style="background: var(--color-accent)"
            :disabled="comtradeLoading"
            @click="saveComtrade"
          >
            <span v-if="comtradeLoading" class="flex items-center gap-2">
              <Icon icon="mdi:loading" width="16" class="animate-spin" /> 保存中...
            </span>
            <span v-else>保存设置</span>
          </button>
        </div>
      </div>
    </template>
  </div>
</template>
