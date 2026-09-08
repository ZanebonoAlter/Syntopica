<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Icon } from '@iconify/vue'
import { useReferenceRolesApi, type ReferenceRole } from '~/api/referenceRoles'
import { useNotify } from '~/composables/useNotify'

/**
 * 参考角色（方法论画像）管理 —— board-level-deep-analysis 2.x / 4.4。
 *
 * 画像内容注入循环 B 三个 LLM 环节（命题/分析/agent 检索）的 system prompt；
 * 启停即时生效（后端每次现查 DB）；单条正文 >4000 字符（rune 计）注入时整条
 * 丢弃——录入时提示长度上限。对齐博查配置的管理交互模式。
 */
const api = useReferenceRolesApi()
const { success: notifySuccess, error: notifyError } = useNotify()

const roles = ref<ReferenceRole[]>([])
const loading = ref(false)
/** 编辑中行（null=无；isNew=新建）。 */
const editing = ref<(ReferenceRole & { isNew?: boolean }) | null>(null)
const saving = ref(false)
const togglingId = ref<number | null>(null)

/** rune 计数字符数（与后端注入上限同口径）。 */
function charLen(s: string): number {
  return Array.from(s).length
}

async function load() {
  loading.value = true
  try {
    const res = await api.listRoles()
    if (res.success && res.data) roles.value = res.data
    else notifyError(res.error || '加载参考角色失败')
  } finally {
    loading.value = false
  }
}
onMounted(load)

function openCreate() {
  editing.value = { id: 0, name: '', title: '', content: '', enabled: true, isNew: true }
}
function openEdit(r: ReferenceRole) {
  editing.value = { ...r }
}
function closeEdit() {
  editing.value = null
}

async function save() {
  const e = editing.value
  if (!e) return
  const name = e.name.trim()
  const content = e.content.trim()
  if (!name) { notifyError('短名不能为空'); return }
  if (!content) { notifyError('画像正文不能为空'); return }
  if (charLen(content) > 4000) {
    notifyError(`画像正文 ${charLen(content)} 字符超出 4000 注入上限，注入时会被整条丢弃；请精简`)
    return
  }
  saving.value = true
  try {
    const body = { name, title: e.title?.trim() || undefined, content, enabled: e.enabled }
    const res = e.isNew || e.id === 0
      ? await api.createRole(body)
      : await api.updateRole(e.id, body)
    if (res.success) {
      notifySuccess(e.isNew || e.id === 0 ? '已创建' : '已保存')
      closeEdit()
      await load()
    } else {
      notifyError(res.error || '保存失败')
    }
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(r: ReferenceRole) {
  togglingId.value = r.id
  try {
    const res = await api.updateRole(r.id, { enabled: !r.enabled })
    if (res.success && res.data) {
      const idx = roles.value.findIndex((x) => x.id === r.id)
      if (idx >= 0) roles.value[idx] = res.data
      notifySuccess(res.data.enabled ? '已启用（即时生效）' : '已停用（即时生效）')
    } else {
      notifyError(res.error || '切换失败')
    }
  } finally {
    togglingId.value = null
  }
}

async function removeRole(r: ReferenceRole) {
  if (!confirm(`删除参考角色「${r.title || r.name}」？\n（删除后分析 prompt 不再注入该画像，不可恢复）`)) return
  const res = await api.deleteRole(r.id)
  if (res.success) {
    notifySuccess('已删除')
    await load()
  } else {
    notifyError(res.error || '删除失败')
  }
}
</script>

<template>
  <div class="rr-panel">
    <div class="rr-head">
      <div>
        <h2>参考角色（方法论画像）</h2>
        <p class="rr-desc">
          启用的画像会注入数据增强的命题 / 分析 / agent 检索 prompt，给分析一套可参考的方法论人格。
          单条正文上限 4000 字符，超限注入时整条丢弃；启停即时生效。
        </p>
      </div>
      <button type="button" class="btn btn-primary" @click="openCreate">
        <Icon icon="mdi:plus" width="14" /> 新建画像
      </button>
    </div>

    <p v-if="loading && !roles.length" class="rr-empty">加载中…</p>
    <p v-else-if="!roles.length" class="rr-empty">还没有参考角色。新建一条方法论画像，让深度分析有「参照系」可依。</p>

    <ul v-else class="rr-list">
      <li v-for="r in roles" :key="r.id" class="rr-item" :class="{ off: !r.enabled }">
        <div class="rr-item-main">
          <div class="rr-item-head">
            <span class="rr-name">{{ r.name }}</span>
            <span v-if="r.title" class="rr-title">{{ r.title }}</span>
            <span class="rr-status" :class="r.enabled ? 'on' : 'off'">{{ r.enabled ? '启用中' : '已停用' }}</span>
            <span class="rr-len" :class="{ over: charLen(r.content) > 4000 }">
              {{ charLen(r.content) }}/4000 字符
            </span>
          </div>
          <p class="rr-preview">{{ r.content.slice(0, 120) }}{{ charLen(r.content) > 120 ? '…' : '' }}</p>
        </div>
        <div class="rr-actions">
          <button type="button" class="btn btn-ghost btn-sm" :disabled="togglingId === r.id" @click="toggleEnabled(r)">
            {{ r.enabled ? '停用' : '启用' }}
          </button>
          <button type="button" class="btn btn-ghost btn-sm" @click="openEdit(r)">
            <Icon icon="mdi:pencil-outline" width="13" /> 编辑
          </button>
          <button type="button" class="btn btn-ghost btn-sm rr-del" @click="removeRole(r)">
            <Icon icon="mdi:delete-outline" width="13" /> 删除
          </button>
        </div>
      </li>
    </ul>

    <!-- ── 编辑对话框 ─────────────────────────────────────────────── -->
    <div v-if="editing" class="rr-modal" @click.self="closeEdit">
      <div class="rr-dialog">
        <h3>{{ editing.isNew ? '新建参考角色' : `编辑：${editing.name}` }}</h3>
        <label class="rr-field">
          <span>短名（唯一标识，如 inside-america）</span>
          <input v-model="editing.name" :disabled="!editing.isNew && editing.id !== 0" class="rr-input" placeholder="inside-america">
        </label>
        <label class="rr-field">
          <span>标题（展示用，如「内部看美国 · 分析基因画像」）</span>
          <input v-model="editing.title" class="rr-input" placeholder="内部看美国 · 分析基因画像">
        </label>
        <label class="rr-field">
          <span>画像正文（方法论描述，注入 prompt；{{ charLen(editing.content) }}/4000 字符）</span>
          <textarea v-model="editing.content" rows="12" class="rr-input rr-textarea" placeholder="分析基因条目：每条一个方法论模式，如「概念考古：先问这个词是谁发明的、为谁服务的…」" />
        </label>
        <label class="rr-check">
          <input v-model="editing.enabled" type="checkbox">
          <span>创建后立即启用</span>
        </label>
        <div class="rr-dialog-actions">
          <button type="button" class="btn btn-ghost" @click="closeEdit">取消</button>
          <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? '保存中…' : '保存' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rr-panel { display: flex; flex-direction: column; gap: 1rem; }
.rr-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.rr-head h2 { margin: 0 0 0.3rem; font-size: 1.05rem; }
.rr-desc { margin: 0; color: var(--color-text-muted); font-size: 0.8rem; line-height: 1.6; max-width: 60ch; }
.rr-empty { color: var(--color-text-muted); font-size: 0.85rem; padding: 1.4rem 0; text-align: center; border: 1px dashed var(--color-border-subtle); border-radius: 8px; }

.rr-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.6rem; }
.rr-item {
  display: flex; align-items: center; gap: 0.8rem;
  padding: 0.7rem 0.9rem; border: 1px solid var(--color-border-subtle);
  border-radius: 10px; background: var(--color-bg-elevated);
}
.rr-item.off { opacity: 0.65; }
.rr-item-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.25rem; }
.rr-item-head { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; }
.rr-name { font-weight: 600; font-size: 0.85rem; }
.rr-title { color: var(--color-text-muted); font-size: 0.78rem; }
.rr-status { font-size: 0.7rem; padding: 0.1rem 0.45rem; border-radius: 99px; }
.rr-status.on { color: #22c55e; background: color-mix(in srgb, #22c55e 12%, transparent); }
.rr-status.off { color: var(--color-text-muted); background: var(--color-bg-hover); }
.rr-len { margin-left: auto; font-size: 0.7rem; color: var(--color-text-muted); }
.rr-len.over { color: #f97316; font-weight: 600; }
.rr-preview { margin: 0; font-size: 0.76rem; color: var(--color-text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rr-actions { display: flex; gap: 0.35rem; flex-shrink: 0; }
.rr-del:hover { color: #f87171; }

.rr-modal {
  position: fixed; inset: 0; z-index: 50;
  display: flex; align-items: center; justify-content: center;
  background: rgb(0 0 0 / 0.5); padding: 1rem;
}
.rr-dialog {
  width: min(640px, 100%); max-height: 90vh; overflow-y: auto;
  background: var(--color-bg-elevated); border: 1px solid var(--color-border-subtle);
  border-radius: 12px; padding: 1.1rem 1.2rem;
  display: flex; flex-direction: column; gap: 0.8rem;
}
.rr-dialog h3 { margin: 0; font-size: 1rem; }
.rr-field { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.78rem; color: var(--color-text-secondary); }
.rr-input {
  padding: 0.4rem 0.6rem; border: 1px solid var(--color-border-subtle); border-radius: 8px;
  background: var(--color-bg); color: var(--color-text-primary); font-size: 0.85rem;
  font-family: inherit;
}
.rr-textarea { font-family: ui-monospace, monospace; font-size: 0.78rem; line-height: 1.6; resize: vertical; }
.rr-check { display: flex; align-items: center; gap: 0.4rem; font-size: 0.8rem; }
.rr-dialog-actions { display: flex; justify-content: flex-end; gap: 0.5rem; }
</style>
