<script setup lang="ts">
/**
 * 模型池横条（tasks 8.3，FB-3+FB-5+FB-7）：按 vendor 分组的可拖模型卡。
 * 双通道添加：拖拽到能力行（HTML5 DnD，dragstart 携带 model id）或点击卡片
 * 弹能力选择菜单（类型不匹配置灰、已在行中标出、点外/ESC 关闭）。
 * 卡片数据与「该卡可加入哪些行」的判定均由父层（AIRouterCapabilityRoutes）下发。
 */
import { nextTick, onBeforeUnmount, ref } from 'vue'
import type { ModelKind } from '~/types'
import VendorLogo from './VendorLogo.vue'

export interface PoolModel {
  id: number
  name: string
  kind: ModelKind
}

export interface PoolGroup {
  vendorName: string
  logo: string | null
  models: PoolModel[]
}

/** 能力菜单项（父层按模型类型与当前行内容算好 disabled/inChain/count） */
export interface PickerCapability {
  capability: string
  label: string
  firstChar: string
  disabled: boolean
  disabledReason?: string
  inChain: boolean
  count: number
}

const props = defineProps<{
  groups: PoolGroup[]
  /** 点击卡片时的候选能力清单（顺序=泳道顺序） */
  capabilitiesFor: (model: PoolModel) => PickerCapability[]
}>()

const emit = defineEmits<{
  'drag-start': [modelId: number]
  'add': [capability: string, modelId: number]
}>()

// ---- 点击菜单（FB-7） ----
const menu = ref<{ x: number, y: number, model: PoolModel, caps: PickerCapability[] } | null>(null)

function openMenu(e: MouseEvent, model: PoolModel) {
  const caps = props.capabilitiesFor(model)
  menu.value = { x: e.clientX, y: e.clientY, model, caps }
  void nextTick(() => clampMenu())
}

function clampMenu() {
  const m = menu.value
  const el = document.querySelector('.cap-picker') as HTMLElement | null
  if (!m || !el) return
  const mw = el.offsetWidth
  const mh = el.offsetHeight
  let x = m.x + 10
  let y = m.y + 10
  if (x + mw > window.innerWidth - 10) x = window.innerWidth - mw - 10
  if (y + mh > window.innerHeight - 10) y = Math.max(10, window.innerHeight - mh - 10)
  m.x = x
  m.y = y
}

function pick(cap: PickerCapability) {
  const m = menu.value
  if (!m || cap.disabled || cap.inChain) return
  emit('add', cap.capability, m.model.id)
  menu.value = null
}

function onDocClick(e: MouseEvent) {
  if (menu.value && !(e.target as HTMLElement).closest?.('.cap-picker')) menu.value = null
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') menu.value = null
}

document.addEventListener('click', onDocClick, true)
document.addEventListener('keydown', onKeydown)
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick, true)
  document.removeEventListener('keydown', onKeydown)
})

const totalUsable = () => props.groups.reduce((n, g) => n + g.models.length, 0)
</script>

<template>
  <div class="pool-strip">
    <div class="pool-strip__head">
      <span class="pool-strip__title">模型池</span>
      <span class="pool-strip__note">卡片健康与降级标注来自降级观测（24h 窗口）</span>
    </div>
    <p class="pool-strip__hint">拖卡片到能力行，或<span class="pool-strip__accent">点击卡片</span>选择能力行；行内左右位置 = 降级优先级</p>
    <template v-if="totalUsable() > 0">
      <div v-for="g in groups" :key="g.vendorName" class="pool-strip__group">
        <span class="pool-strip__group-label">
          <VendorLogo :symbol-id="g.logo" :name="g.vendorName" :size="13" />
          {{ g.vendorName }}
        </span>
        <button
          v-for="m in g.models" :key="m.id" type="button" class="pool-card"
          draggable="true"
          :title="`${m.name}（点击选择能力行，或拖拽）`"
          @dragstart="emit('drag-start', m.id)"
          @click="openMenu($event, m)"
        >
          <span class="pool-card__name">{{ m.name }}</span>
          <span class="pool-card__kind">{{ m.kind }}</span>
          <span class="pool-card__plus">＋</span>
        </button>
      </div>
    </template>
    <p v-else class="pool-strip__hint">先在「供应商与模型」里启用模型</p>

    <!-- 点击菜单（fixed 定位，pick 后/点外/ESC 关闭） -->
    <Teleport to="body">
      <div v-if="menu" class="cap-picker" :style="{ left: `${menu.x}px`, top: `${menu.y}px` }">
        <p class="cap-picker__title">「{{ menu.model.name }}」加入哪个能力行？</p>
        <button
          v-for="c in menu.caps" :key="c.capability" type="button" class="cap-picker__item"
          :class="{ 'cap-picker__item--disabled': c.disabled, 'cap-picker__item--inchain': c.inChain }"
          @click="pick(c)"
        >
          <span class="cap-picker__badge">{{ c.firstChar }}</span>
          <span class="cap-picker__label">{{ c.label }}</span>
          <span class="cap-picker__count">
            {{ c.disabled ? (c.disabledReason ?? '不可用') : (c.inChain ? '已在行中' : `${c.count} 卡`) }}
          </span>
        </button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.pool-strip {
  border-bottom: 1px solid var(--color-border-subtle);
  padding: 12px 20px 10px;
}

.pool-strip__head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}

.pool-strip__title {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-secondary);
}

.pool-strip__note {
  margin-left: auto;
  font-size: 10.5px;
  color: var(--color-text-muted);
}

.pool-strip__hint {
  font-size: 11px;
  color: var(--color-text-muted);
  margin: 0 0 8px;
}

.pool-strip__accent {
  color: var(--color-accent);
}

.pool-strip__group {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
  margin-bottom: 6px;
}

.pool-strip__group-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
  color: var(--color-text-muted);
  margin-right: 4px;
}

.pool-card {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 3px 9px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
  font-size: 12px;
  color: var(--color-text-primary);
  cursor: grab;
  transition: border-color 0.15s;
}

.pool-card:hover {
  border-color: var(--color-border-medium);
}

.pool-card:active {
  cursor: grabbing;
}

.pool-card__name {
  max-width: 150px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 500;
}

.pool-card__kind {
  font-size: 10px;
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  border-radius: 4px;
  padding: 0 5px;
  flex: none;
}

.pool-card__plus {
  color: var(--color-text-muted);
  font-size: 10px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 4px;
  padding: 0 4px;
  flex: none;
}
</style>

<style>
/* 菜单 Teleport 到 body，样式不能 scoped */
.cap-picker {
  position: fixed;
  z-index: 60;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-medium);
  border-radius: 10px;
  box-shadow: var(--shadow-strong);
  padding: 6px;
  min-width: 210px;
  max-height: 46vh;
  overflow-y: auto;
}

.cap-picker__title {
  font-size: 11px;
  color: var(--color-text-muted);
  padding: 3px 8px 5px;
  margin: 0;
}

.cap-picker__item {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  border: none;
  background: none;
  padding: 5px 8px;
  border-radius: 6px;
  font-size: 12.5px;
  cursor: pointer;
  text-align: left;
  color: var(--color-text-primary);
}

.cap-picker__item:hover {
  background: var(--color-bg-hover);
}

.cap-picker__item--disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.cap-picker__item--inchain {
  color: var(--color-accent);
}

.cap-picker__badge {
  width: 18px;
  height: 18px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 9px;
  font-weight: 700;
  background: var(--color-accent);
  color: var(--color-text-inverted);
  flex: none;
}

.cap-picker__label {
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cap-picker__count {
  margin-left: auto;
  font-size: 10.5px;
  color: var(--color-text-muted);
  flex: none;
}
</style>
