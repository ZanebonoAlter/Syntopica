<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Icon } from '@iconify/vue'
import { useCompositeLabelsApi, type CompositeLabel } from '~/api/compositeLabels'
import CompositeLabelEditDialog from './CompositeLabelEditDialog.vue'

/**
 * 组合标签治理页：列表（label / 组件序列 / ref_count / status）+ 手动创建 +
 * 禁用/启用。自包含组件（自行拉取 /api/composite-labels）。
 */
const emit = defineEmits<{
  changed: []
}>()

const api = useCompositeLabelsApi()

const items = ref<CompositeLabel[]>([])
/** 全量组合（不过滤 status），供创建对话框的相关组合提示使用（design D7）。 */
const allComposites = ref<CompositeLabel[]>([])
const loading = ref(false)
const error = ref('')
const statusFilter = ref<'' | 'active' | 'disabled'>('')
const showCreateDialog = ref(false)
const actionBusyId = ref<number | null>(null)
const notice = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    if (statusFilter.value) {
      // 全量先行（供创建对话框相关组合提示），过滤列表收尾
      const all = await api.getLabels()
      allComposites.value = all.data?.items ?? []
      const res = await api.getLabels({ status: statusFilter.value })
      items.value = res.data?.items ?? []
    }
    else {
      const res = await api.getLabels()
      items.value = res.data?.items ?? []
      allComposites.value = items.value
    }
  }
  catch {
    error.value = '加载组合标签失败，请重试'
    items.value = []
  }
  finally {
    loading.value = false
  }
}

onMounted(load)

async function toggleStatus(item: CompositeLabel) {
  if (actionBusyId.value !== null) return
  actionBusyId.value = item.id
  notice.value = ''
  error.value = ''
  try {
    if (item.status === 'active') {
      await api.disableLabel(item.id)
      notice.value = `已禁用「${item.label}」（向量已清除，组件与别名保留）`
    }
    else {
      await api.enableLabel(item.id)
      notice.value = `已启用「${item.label}」（embedding 已重算）`
    }
    emit('changed')
    await load()
  }
  catch {
    error.value = '操作失败，请重试'
  }
  finally {
    actionBusyId.value = null
  }
}

function componentChain(item: CompositeLabel): string {
  return [...item.components].sort((a, b) => a.position - b.position).map(c => c.label).join(' × ')
}

const statusTabs: { key: '' | 'active' | 'disabled'; label: string }[] = [
  { key: '', label: '全部' },
  { key: 'active', label: '启用中' },
  { key: 'disabled', label: '已禁用' },
]
</script>

<template>
  <section class="clp" data-testid="composite-label-pool">
    <header class="clp-header">
      <div class="clp-header-text">
        <h2 class="clp-title">组合标签</h2>
        <p class="clp-subtitle">指向性主题（如「美债收益率」= 美国国债 × 收益率），作为 tag→board 匹配的最强信号</p>
      </div>
      <button type="button" class="clp-create-btn" data-testid="composite-label-create" @click="showCreateDialog = true">
        <Icon icon="mdi:plus" width="13" />
        新建组合
      </button>
    </header>

    <div class="clp-toolbar">
      <div class="clp-tabs">
        <button
          v-for="t in statusTabs"
          :key="t.key"
          type="button"
          class="clp-tab"
          :class="{ 'is-active': statusFilter === t.key }"
          @click="statusFilter = t.key; load()"
        >
          {{ t.label }}
        </button>
      </div>
      <span class="clp-count">共 {{ items.length }} 个</span>
    </div>

    <p v-if="notice" class="clp-notice" data-testid="composite-label-notice">{{ notice }}</p>
    <p v-if="error" class="clp-error" data-testid="composite-label-pool-error">{{ error }}</p>

    <div v-if="loading" class="clp-loading">
      <Icon icon="mdi:loading" width="18" class="animate-spin" /> 加载组合标签...
    </div>
    <div v-else-if="items.length === 0" class="clp-empty" data-testid="composite-label-pool-empty">
      <Icon icon="mdi:label-off-outline" width="22" />
      <p>暂无组合标签</p>
      <p class="clp-empty-hint">可在升级建议面板确认 compose 建议，或点击右上角「新建组合」手动创建</p>
    </div>
    <ul v-else class="clp-list">
      <li
        v-for="item in items"
        :key="item.id"
        class="clp-item"
        :data-status="item.status"
        :data-testid="`composite-label-item-${item.id}`"
      >
        <div class="clp-item-main">
          <div class="clp-item-title-row">
            <span class="clp-item-label" :title="item.label">{{ item.label }}</span>
            <span class="clp-item-status" :class="item.status === 'active' ? 'clp-item-status--active' : 'clp-item-status--disabled'">
              {{ item.status === 'active' ? '启用' : '禁用' }}
            </span>
            <span class="clp-item-source">{{ item.source === 'upgrade_suggest' ? '建议确认' : '手动' }}</span>
          </div>
          <p class="clp-item-chain" data-testid="composite-label-chain" :title="componentChain(item)">
            <Icon icon="mdi:link-variant" width="12" />
            {{ componentChain(item) }}
          </p>
          <p v-if="item.description" class="clp-item-desc">{{ item.description }}</p>
          <p v-if="item.aliases && item.aliases.length > 0" class="clp-item-aliases" :title="item.aliases.join('、')">
            别名：{{ item.aliases.slice(0, 4).join('、') }}{{ item.aliases.length > 4 ? '…' : '' }}
          </p>
        </div>
        <div class="clp-item-side">
          <span class="clp-item-ref" title="ref_count">{{ item.ref_count }}</span>
          <button
            type="button"
            class="clp-item-toggle"
            :disabled="actionBusyId === item.id"
            :data-testid="`composite-label-toggle-${item.id}`"
            @click="toggleStatus(item)"
          >
            <Icon v-if="actionBusyId === item.id" icon="mdi:loading" width="12" class="animate-spin" />
            {{ item.status === 'active' ? '禁用' : '启用' }}
          </button>
        </div>
      </li>
    </ul>

    <CompositeLabelEditDialog
      :visible="showCreateDialog"
      :composites="allComposites"
      @confirm="load(); emit('changed')"
      @cancel="showCreateDialog = false"
    />
  </section>
</template>

<style scoped>
.clp { display: flex; flex-direction: column; gap: 0.75rem; }
.clp-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 0.75rem; }
.clp-title { font-family: serif; font-size: 1rem; font-weight: 600; color: var(--color-text-primary); }
.clp-subtitle { margin-top: 0.15rem; font-size: 0.72rem; color: var(--color-text-muted); }
.clp-create-btn { display: inline-flex; align-items: center; gap: 0.3rem; padding: 0.4rem 0.8rem; border: 1px solid var(--color-accent); border-radius: 8px; background: var(--color-accent); color: var(--color-accent-contrast, #fff); font-size: 0.76rem; cursor: pointer; }
.clp-create-btn:hover { opacity: 0.9; }
.clp-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 0.6rem; padding-bottom: 0.5rem; border-bottom: 1px solid var(--color-border-subtle); }
.clp-tabs { display: flex; gap: 0.25rem; }
.clp-tab { padding: 0.3rem 0.7rem; border: none; border-radius: 8px; background: none; color: var(--color-text-muted); font-size: 0.74rem; cursor: pointer; }
.clp-tab:hover { color: var(--color-text-secondary); background: var(--color-bg-hover); }
.clp-tab.is-active { color: var(--color-accent); background: var(--color-accent-subtle); }
.clp-count { font-size: 0.72rem; color: var(--color-text-muted); }
.clp-notice { padding: 0.45rem 0.6rem; border: 1px solid var(--color-success-border, rgba(61,138,74,0.3)); border-radius: 8px; background: var(--color-success-bg, rgba(61,138,74,0.08)); font-size: 0.74rem; color: var(--color-success, #3d8a4a); }
.clp-error { font-size: 0.74rem; color: var(--color-danger, #c0392b); }
.clp-loading, .clp-empty { display: flex; flex-direction: column; align-items: center; gap: 0.4rem; padding: 2rem 1rem; color: var(--color-text-muted); font-size: 0.78rem; }
.clp-empty-hint { font-size: 0.7rem; opacity: 0.8; }
.clp-list { display: flex; flex-direction: column; gap: 0.5rem; list-style: none; }
.clp-item { display: flex; align-items: stretch; justify-content: space-between; gap: 0.75rem; padding: 0.65rem 0.8rem; border: 1px solid var(--color-border-subtle); border-radius: 10px; background: var(--color-bg-elevated); }
.clp-item[data-status='disabled'] { opacity: 0.6; }
.clp-item-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.2rem; }
.clp-item-title-row { display: flex; align-items: center; gap: 0.45rem; }
.clp-item-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.85rem; font-weight: 600; color: var(--color-text-primary); }
.clp-item-status { flex: 0 0 auto; padding: 0.05rem 0.4rem; border-radius: 6px; font-size: 0.64rem; }
.clp-item-status--active { border: 1px solid var(--color-success-border, rgba(61,138,74,0.3)); color: var(--color-success, #3d8a4a); }
.clp-item-status--disabled { border: 1px solid var(--color-border-medium); color: var(--color-text-muted); }
.clp-item-source { flex: 0 0 auto; font-size: 0.64rem; color: var(--color-text-muted); }
.clp-item-chain { display: flex; align-items: center; gap: 0.3rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.74rem; color: var(--color-accent); }
.clp-item-desc, .clp-item-aliases { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 0.7rem; color: var(--color-text-muted); }
.clp-item-side { flex: 0 0 auto; display: flex; flex-direction: column; align-items: flex-end; justify-content: center; gap: 0.35rem; }
.clp-item-ref { padding: 0.05rem 0.45rem; border-radius: 6px; background: var(--color-bg-sunken); font-family: ui-monospace, monospace; font-size: 0.68rem; color: var(--color-text-secondary); }
.clp-item-toggle { display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.25rem 0.6rem; border: 1px solid var(--color-border-medium); border-radius: 8px; background: none; color: var(--color-text-secondary); font-size: 0.72rem; cursor: pointer; }
.clp-item-toggle:hover:not(:disabled) { border-color: var(--color-accent); color: var(--color-accent); }
.clp-item-toggle:disabled { opacity: 0.5; cursor: wait; }
</style>
