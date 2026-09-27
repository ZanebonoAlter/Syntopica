<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { inject } from 'vue'

interface AIRouterCtx {
  saving: boolean
  backupProviders: any[]
  showNewProviderForm: boolean
  newProviderForm: any
  editingProviderId: string | null
  editProviderForm: any
  showEditProviderApiKey: boolean
  showNewProviderApiKey: boolean
  testingProviderId: number | null
  saveNewProvider: () => void
  startEditingProvider: (p: any) => void
  cancelEditingProvider: () => void
  saveEditedProvider: () => void
  deleteBackupProvider: (p: any) => void
  isProviderLinked: (id: string) => boolean
  testBackupProvider: (p: any) => void
}

const ctx = inject<AIRouterCtx>('ai-router-ctx')!
</script>

<template>
  <div class="ai-card">
    <div class="ai-card__header">
      <AppSectionHeader title="备用模型池" description="挂到能力路由做 failover，主模型挂了自动切" icon-name="mdi:server-network" />
      <AppButton variant="secondary" size="sm" @click="ctx.showNewProviderForm = !ctx.showNewProviderForm">
        <Icon :icon="ctx.showNewProviderForm ? 'mdi:chevron-up' : 'mdi:plus'" width="14" height="14" />
        {{ ctx.showNewProviderForm ? '收起' : '新增' }}
      </AppButton>
    </div>

    <div class="ai-card__body">
      <!-- New Provider Form -->
      <div v-if="ctx.showNewProviderForm" class="ai-form-dashed">
        <div class="ai-form-grid">
          <AppInput v-model="ctx.newProviderForm.name" type="text" placeholder="名称" />
          <AppInput v-model="ctx.newProviderForm.model" type="text" placeholder="模型名" />
          <select v-model="ctx.newProviderForm.provider_type" class="ai-select">
            <option value="openai_compatible">OpenAI Compatible</option>
            <option value="ollama">Ollama (本地)</option>
          </select>
          <select v-model="ctx.newProviderForm.model_kind" class="ai-select">
            <option value="llm">LLM（对话/总结）</option>
            <option value="embedding">Embedding（向量）</option>
          </select>
          <AppInput v-model="ctx.newProviderForm.base_url" type="text"
            :placeholder="ctx.newProviderForm.provider_type === 'ollama' ? 'http://localhost:11434/v1' : 'https://api.example.com/v1'" />
          <AppInput v-model="ctx.newProviderForm.start_command" type="text" placeholder="启动命令（可选）：llama-server -m D:/models/qwen.gguf --port 8081" />
          <div v-if="ctx.newProviderForm.provider_type !== 'ollama'" class="relative md:col-span-2">
            <AppInput v-model="ctx.newProviderForm.api_key" :type="ctx.showNewProviderApiKey ? 'text' : 'password'" placeholder="API Key" />
            <button class="ai-eye-btn" @click="ctx.showNewProviderApiKey = !ctx.showNewProviderApiKey">
              <Icon :icon="ctx.showNewProviderApiKey ? 'mdi:eye-off' : 'mdi:eye'" width="15" height="15" />
            </button>
          </div>
          <div v-else class="ai-notice ai-notice--warning md:col-span-2">Ollama 模式无需 API Key</div>
          <div class="ai-toggle-row">
            <AppToggle v-model="ctx.newProviderForm.enable_thinking" /> 清理推理输出
          </div>
          <p class="md:col-span-2 text-[11px]" style="color: var(--color-text-muted)">启动命令：本地进程启动命令，留空表示外部托管服务；填写后可在启动时由后端自动拉起</p>
        </div>
        <div class="flex justify-end">
          <AppButton variant="primary" size="sm" :disabled="ctx.saving" @click="ctx.saveNewProvider">添加</AppButton>
        </div>
      </div>

      <!-- Empty State -->
      <div v-if="ctx.backupProviders.length === 0" class="ai-empty">
        还没有备用模型，先加一个
      </div>

      <!-- Provider List -->
      <div v-else class="space-y-2">
        <div v-for="provider in ctx.backupProviders" :key="provider.id" class="ai-provider-item">
          <div class="flex items-center justify-between gap-3">
            <div class="flex items-center gap-3 min-w-0">
              <div class="ai-provider-icon">
                <Icon icon="mdi:cube-outline" width="14" height="14" />
              </div>
              <div class="min-w-0">
                <div class="ai-provider-name">{{ provider.name }}</div>
                <div class="ai-provider-meta">{{ provider.model }} · {{ provider.base_url }}</div>
              </div>
            </div>
            <div class="flex items-center gap-1.5 shrink-0">
              <span class="ai-badge" :class="provider.model_kind === 'embedding' ? 'ai-badge--kind-embedding' : 'ai-badge--kind-llm'">
                {{ provider.model_kind === 'embedding' ? 'Embedding' : 'LLM' }}
              </span>
              <span class="ai-badge" :class="provider.enabled ? 'ai-badge--success' : 'ai-badge--muted'">
                {{ provider.enabled ? '启用' : '停用' }}
              </span>
              <button class="ai-icon-btn" title="测试连接（用已保存配置含密钥）"
                :disabled="ctx.testingProviderId !== null"
                @click="ctx.testBackupProvider(provider)">
                <Icon :icon="ctx.testingProviderId === provider.id ? 'mdi:loading' : 'mdi:radar'" width="14" height="14"
                  :class="ctx.testingProviderId === provider.id ? 'animate-spin' : ''" />
              </button>
              <button class="ai-icon-btn" @click="ctx.startEditingProvider(provider)">
                <Icon icon="mdi:pencil-outline" width="14" height="14" />
              </button>
              <button class="ai-icon-btn ai-icon-btn--danger" @click="ctx.deleteBackupProvider(provider)">
                <Icon icon="mdi:trash-can-outline" width="14" height="14" />
              </button>
            </div>
          </div>
          <p v-if="ctx.isProviderLinked(provider.id)" class="ai-warn-text">挂在线路上，删除将自动从所有线路解绑</p>

          <!-- Edit Form -->
          <div v-if="ctx.editingProviderId === provider.id" class="ai-edit-form">
            <div class="ai-form-grid">
              <AppInput v-model="ctx.editProviderForm.name" type="text" placeholder="名称" />
              <AppInput v-model="ctx.editProviderForm.model" type="text" placeholder="模型名" />
              <select v-model="ctx.editProviderForm.provider_type" class="ai-select">
                <option value="openai_compatible">OpenAI Compatible</option>
                <option value="ollama">Ollama (本地)</option>
              </select>
              <select v-model="ctx.editProviderForm.model_kind" class="ai-select">
                <option value="llm">LLM（对话/总结）</option>
                <option value="embedding">Embedding（向量）</option>
              </select>
              <AppInput v-model="ctx.editProviderForm.base_url" type="text"
                :placeholder="ctx.editProviderForm.provider_type === 'ollama' ? 'http://localhost:11434/v1' : 'https://api.example.com/v1'" />
              <AppInput v-model="ctx.editProviderForm.start_command" type="text"
                :placeholder="provider.start_command_configured ? '启动命令已配置（留空保持不变）' : '启动命令（可选）：llama-server -m D:/models/qwen.gguf --port 8081'" />
              <div v-if="provider.start_command_configured" class="md:col-span-2">
                <button v-if="!ctx.editProviderForm.clear_start_command"
                  class="text-xs px-3 py-1.5 rounded-lg transition-colors"
                  style="color: var(--color-error); background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25))"
                  @click="ctx.editProviderForm.clear_start_command = true">
                  清除已配置的启动命令
                </button>
                <span v-else class="text-xs px-3 py-1.5 rounded-lg"
                  style="color: var(--color-warning); background: var(--color-warning-bg, rgba(196, 136, 60, 0.1)); border: 1px solid var(--color-warning-border, rgba(196, 136, 60, 0.25))">
                  保存后将清除已配置的启动命令
                </span>
              </div>
              <div v-if="ctx.editProviderForm.provider_type !== 'ollama'" class="relative md:col-span-2">
                <AppInput v-model="ctx.editProviderForm.api_key" :type="ctx.showEditProviderApiKey ? 'text' : 'password'" placeholder="留空表示沿用已保存密钥" />
                <button class="ai-eye-btn" @click="ctx.showEditProviderApiKey = !ctx.showEditProviderApiKey">
                  <Icon :icon="ctx.showEditProviderApiKey ? 'mdi:eye-off' : 'mdi:eye'" width="15" height="15" />
                </button>
              </div>
              <div v-if="ctx.editProviderForm.provider_type !== 'ollama' && provider.api_key_configured" class="md:col-span-2">
                <button v-if="!ctx.editProviderForm.clear_api_key"
                  class="text-xs px-3 py-1.5 rounded-lg transition-colors"
                  style="color: var(--color-error); background: var(--color-error-bg, rgba(196, 47, 60, 0.1)); border: 1px solid var(--color-error-border, rgba(196, 47, 60, 0.25))"
                  @click="ctx.editProviderForm.clear_api_key = true">
                  清除已保存密钥
                </button>
                <span v-else class="text-xs px-3 py-1.5 rounded-lg"
                  style="color: var(--color-warning); background: var(--color-warning-bg, rgba(196, 136, 60, 0.1)); border: 1px solid var(--color-warning-border, rgba(196, 136, 60, 0.25))">
                  保存后将清除已保存密钥
                </span>
              </div>
              <div v-else class="ai-notice ai-notice--warning md:col-span-2">Ollama 模式无需 API Key</div>
              <AppInput v-model="ctx.editProviderForm.timeout_seconds" type="number" min="30" placeholder="Timeout (秒)" />
              <div class="ai-toggle-row">
                <AppToggle v-model="ctx.editProviderForm.enabled" /> 启用
              </div>
              <div class="ai-toggle-row">
                <AppToggle v-model="ctx.editProviderForm.enable_thinking" /> 清理推理输出
              </div>
            </div>
            <div class="flex justify-end gap-2">
              <AppButton variant="ghost" size="sm" @click="ctx.cancelEditingProvider">取消</AppButton>
              <AppButton variant="primary" size="sm" :disabled="ctx.saving" @click="ctx.saveEditedProvider">保存</AppButton>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ai-card {
  border-radius: 12px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  overflow: hidden;
}

.ai-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.ai-card__body {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.ai-form-dashed {
  border-radius: 8px;
  border: 1px dashed var(--color-border-medium);
  padding: 16px;
  background: var(--color-bg-sunken);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ai-form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

@media (max-width: 768px) {
  .ai-form-grid {
    grid-template-columns: 1fr;
  }
}

.ai-select {
  width: 100%;
  padding: 7px 10px;
  font-size: 13px;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  outline: none;
  transition: border-color 0.15s;
}

.ai-select:focus {
  border-color: var(--color-input-focus);
}

.ai-eye-btn {
  position: absolute;
  right: 10px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--color-text-muted);
  transition: color 0.15s;
}

.ai-eye-btn:hover {
  color: var(--color-text-secondary);
}

.ai-notice {
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 12px;
}

.ai-notice--warning {
  background: rgba(196, 136, 60, 0.1);
  border: 1px solid rgba(196, 136, 60, 0.25);
  color: var(--color-warning);
}

.ai-toggle-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--color-text-secondary);
}

.ai-empty {
  text-align: center;
  padding: 24px 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.ai-provider-item {
  border-radius: 8px;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-sunken);
  padding: 12px 16px;
}

.ai-provider-icon {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: var(--color-bg-hover);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: var(--color-text-secondary);
}

.ai-provider-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ai-provider-meta {
  font-size: 11px;
  color: var(--color-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ai-badge {
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 9999px;
  font-weight: 500;
}

.ai-badge--success {
  background: rgba(61, 138, 74, 0.12);
  color: var(--color-success);
}

.ai-badge--muted {
  background: var(--color-bg-hover);
  color: var(--color-text-muted);
}

.ai-badge--kind-llm {
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.ai-badge--kind-embedding {
  background: rgba(45, 138, 122, 0.12);
  color: var(--color-success);
}

.ai-icon-btn {
  padding: 4px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.ai-icon-btn:hover {
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.ai-icon-btn--danger:hover {
  background: rgba(196, 47, 60, 0.1);
  color: var(--color-error);
}

.ai-icon-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.ai-warn-text {
  margin-top: 8px;
  font-size: 11px;
  color: var(--color-warning);
  padding-left: 40px;
}

.ai-edit-form {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--color-border-subtle);
  display: flex;
  flex-direction: column;
  gap: 12px;
}
</style>
