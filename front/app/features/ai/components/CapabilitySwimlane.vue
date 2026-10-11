<script setup lang="ts">
/**
 * 能力泳道行（tasks 8.3，FB-3+FB-5）：行头（badge/名称/N 卡/健康点三态）
 * + 行内卡片（自左向右 = 降级优先级，卡片间箭头即降级方向）。
 * 卡片交互：拖拽换位（同行）、点击菜单添加走 ModelPoolStrip、×移除、←→调序、
 * 思考档下拉（ThinkingLevelSelect）。降级内嵌标注（FB-5）：失败卡红边+角标+
 * 「24h 失败 N」、接管卡「接管 N」、发生迁移的箭头高亮标次数（useRouteHealth 聚合）。
 * 卡内控件不溢出（FB-7：208px 卡 + select flex 收缩）。
 */
import type { ThinkingLevel } from '~/types'
import ThinkingLevelSelect from './ThinkingLevelSelect.vue'
import VendorLogo from './VendorLogo.vue'

export interface SwimCard {
  modelId: number
  name: string
  vendorName: string
  logo: string | null
  kind: 'llm' | 'embedding'
  defaultLevel: ThinkingLevel | null
  supportsXhighMax: boolean
  thinkingLevel: ThinkingLevel | null
  fanoutOthers: number
  /** useRouteHealth.cardHealth 结果 */
  fail: number
  lastErr: string | null
  takeover: number
}

const props = defineProps<{
  capability: string
  label: string
  /** 行头「供血对象」说明小字（routeDescriptions；空/undefined 不渲染该行） */
  description?: string
  cards: SwimCard[]
  /** 行头健康（useRouteHealth.rowHealth 结果） */
  healthLevel: 'ok' | 'warn' | 'err'
  healthText: string
  /** 拖拽态（父层统一管理：正在拖的池卡类型 / 本行拖动中的卡） */
  draggingOver: boolean
  rejectFlash: boolean
  pulsing: boolean
}>()

const emit = defineEmits<{
  'level-change': [modelId: number, value: ThinkingLevel | null]
  'remove': [modelId: number]
  'move': [modelId: number, direction: -1 | 1]
  /** 池卡/行卡 drop 到本行（targetModelId/before 为落点，null=行尾） */
  'drop-in': [targetModelId: number | null, before: boolean]
  'card-drag-start': [modelId: number]
  'card-drag-end': []
  'drag-over': []
  'drag-leave': []
}>()

/** 行卡 dragstart：写 dataTransfer（Firefox 需要）并 emit 给父层 */
function onCardDragStart(e: DragEvent, card: SwimCard) {
  e.dataTransfer?.setData('text/plain', `lane:${card.modelId}`)
  emit('card-drag-start', card.modelId)
}

function onCardDrop(e: DragEvent, target: SwimCard) {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const before = e.clientX < rect.left + rect.width / 2
  emit('drop-in', target.modelId, before)
  e.preventDefault()
  e.stopPropagation()
}

function onLaneDrop(e: DragEvent) {
  emit('drop-in', null, true)
  e.preventDefault()
}

/** 降级箭头：前一张卡是失败源（fail>0）时高亮，次数取后一张卡的接管数或前卡失败数 */
function arrowDegraded(prev: SwimCard, next: SwimCard): number | null {
  if (prev.fail === 0) return null
  return next.takeover || prev.fail
}
</script>

<template>
  <div
    class="swimlane"
    :class="{ 'swimlane--reject': rejectFlash, 'swimlane--pulse': pulsing }"
    :data-route-section="capability"
  >
    <div class="swimlane__head">
      <div class="swimlane__title">
        <span class="swimlane__badge" :class="cards.length === 0 ? 'swimlane__badge--empty' : ''">
          {{ label.charAt(0) }}
        </span>
        <span class="swimlane__name" :title="label">{{ label }}</span>
        <span class="swimlane__count">{{ cards.length }}</span>
      </div>
      <p v-if="description" class="swimlane__desc" :title="description">{{ description }}</p>
      <span class="swimlane__health">
        <span class="swimlane__dot" :class="`swimlane__dot--${healthLevel}`" />
        {{ healthText }}
      </span>
    </div>

    <div
      class="swimlane__cards"
      :class="{ 'swimlane__cards--over': draggingOver }"
      @dragover.prevent="emit('drag-over')"
      @dragleave="emit('drag-leave')"
      @drop="onLaneDrop"
    >
      <div v-if="cards.length === 0" class="swimlane__empty">
        拖入模型卡片（或点击上方模型卡选此行）
      </div>

      <template v-for="(card, index) in cards" :key="card.modelId">
        <!-- 降级箭头（第一张卡前无箭头） -->
        <span
          v-if="index > 0"
          class="swimlane__arrow"
          :class="{ 'swimlane__arrow--degraded': arrowDegraded(cards[index - 1]!, card) !== null }"
          :title="arrowDegraded(cards[index - 1]!, card) !== null ? `顺位 ${index}→${index + 1} 降级（24h ${arrowDegraded(cards[index - 1]!, card)} 次）` : '降级方向'"
        >
          →<template v-if="arrowDegraded(cards[index - 1]!, card) !== null">
            <span class="swimlane__arrow-badge">降级 {{ arrowDegraded(cards[index - 1]!, card) }}</span>
          </template>
        </span>

        <div
          class="swim-card"
          :class="{ 'swim-card--fail': card.fail > 0 }"
          draggable="true"
          :title="card.lastErr ? `${card.name} · 近期错误：${card.lastErr}` : card.name"
          @dragstart="onCardDragStart($event, card)"
          @dragend="emit('card-drag-end')"
          @dragover.prevent
          @drop="onCardDrop($event, card)"
        >
          <span v-if="card.fail > 0" class="swim-card__err-dot" :title="`近期失败 ${card.fail} 次`">{{ card.fail }}</span>
          <span class="swim-card__rank" :class="index === 0 ? 'swim-card__rank--first' : ''">{{ index + 1 }}</span>

          <div class="swim-card__top">
            <span class="swim-card__name">{{ card.name }}</span>
            <button class="swim-icon-btn swim-icon-btn--danger" title="移除" @click="emit('remove', card.modelId)">×</button>
            <span class="fanout-tip"><template v-if="card.fanoutOthers > 0">{{ card.name }} 另承载 <b>{{ card.fanoutOthers }}</b> 个能力（单点依赖注意）</template><template v-else>未承载其他能力</template><template v-if="card.lastErr"><br>近期错误：{{ card.lastErr }}</template></span>
          </div>

          <span class="swim-card__vendor">
            <VendorLogo :symbol-id="card.logo" :name="card.vendorName" :size="12" />
            <span class="swim-card__vendor-name">{{ card.vendorName }}</span>
          </span>

          <div class="swim-card__foot">
            <span v-if="card.fail > 0" class="swim-card__badge swim-card__badge--fail" :title="card.lastErr ?? ''">24h 失败 {{ card.fail }}</span>
            <span v-if="card.takeover > 0" class="swim-card__badge swim-card__badge--takeover" title="上游失败后由该卡接管">接管 {{ card.takeover }}</span>
            <span v-if="card.thinkingLevel !== null && card.kind !== 'embedding'" class="swim-card__badge swim-card__badge--override">覆盖</span>
            <ThinkingLevelSelect
              v-if="card.kind !== 'embedding'"
              :model-value="card.thinkingLevel"
              :model-default="card.defaultLevel"
              :supports-xhigh-max="card.supportsXhighMax"
              class="swim-card__level"
              @update:model-value="(v: ThinkingLevel | null) => emit('level-change', card.modelId, v)"
            />
            <span v-else class="swim-card__dash" title="embedding 绑定不携带思考档">—</span>
            <button class="swim-icon-btn" :disabled="index === 0" title="前移" @click="emit('move', card.modelId, -1)">←</button>
            <button class="swim-icon-btn" :disabled="index === cards.length - 1" title="后移" @click="emit('move', card.modelId, 1)">→</button>
          </div>
        </div>
      </template>

      <div v-if="cards.length > 0" class="swimlane__slot" title="拖到此处追加行尾">＋ 拖到此处</div>
    </div>
  </div>
</template>

<style scoped>
.swimlane {
  display: flex;
  align-items: stretch;
  border-bottom: 1px solid var(--color-border-subtle);
  min-height: 64px;
}

.swimlane:last-child {
  border-bottom: none;
}

.swimlane--reject {
  box-shadow: inset 0 0 0 1.5px var(--color-error);
}

.swimlane--pulse {
  animation: swimlane-pulse 1.2s ease;
}

@keyframes swimlane-pulse {
  0% {
    background: var(--color-accent-subtle);
  }
  100% {
    background: transparent;
  }
}

.swimlane__head {
  width: 172px;
  flex: none;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 3px;
  padding: 10px 12px 10px 20px;
  border-right: 1px solid var(--color-border-subtle);
}

.swimlane__title {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}

.swimlane__badge {
  width: 22px;
  height: 22px;
  border-radius: 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  background: var(--color-accent);
  color: var(--color-text-inverted);
  flex: none;
}

.swimlane__badge--empty {
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
}

.swimlane__name {
  font-size: 12.5px;
  font-weight: 600;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.swimlane__count {
  font-size: 10.5px;
  color: var(--color-text-muted);
  flex: none;
}

.swimlane__desc {
  margin: 0;
  font-size: 10.5px;
  line-height: 1.35;
  color: var(--color-text-muted);
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.swimlane__health {
  font-size: 10.5px;
  display: flex;
  align-items: center;
  gap: 5px;
  padding-left: 29px;
  color: var(--color-text-secondary);
}

.swimlane__dot {
  width: 6px;
  height: 6px;
  border-radius: 99px;
  flex: none;
}

.swimlane__dot--ok { background: var(--color-success); }
.swimlane__dot--warn { background: var(--color-warning); }
.swimlane__dot--err { background: var(--color-error); }

.swimlane__cards {
  flex: 1;
  display: flex;
  align-items: center;
  padding: 10px 16px;
  overflow-x: auto;
  min-height: 60px;
}

.swimlane__cards--over {
  background: var(--color-accent-subtle);
  box-shadow: inset 0 0 0 1.5px var(--color-accent);
}

.swimlane__empty {
  text-align: center;
  padding: 10px 18px;
  font-size: 11px;
  color: var(--color-text-muted);
  border-radius: 8px;
  border: 1px dashed var(--color-border-medium);
}

.swimlane__arrow {
  flex: none;
  color: var(--color-text-muted);
  font-size: 13px;
  padding: 0 7px;
  position: relative;
}

.swimlane__arrow--degraded {
  color: var(--color-warning);
  font-weight: 700;
}

.swimlane__arrow-badge {
  position: absolute;
  top: -9px;
  left: 50%;
  transform: translateX(-50%);
  font-size: 9px;
  background: var(--color-warning-subtle);
  color: var(--color-warning);
  border-radius: 4px;
  padding: 0 4px;
  white-space: nowrap;
}

.swimlane__slot {
  flex: none;
  width: 92px;
  min-height: 52px;
  border: 1.5px dashed var(--color-border-medium);
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10.5px;
  color: var(--color-text-muted);
  margin-left: 8px;
}

/* ---- 卡片（FB-7：208px + 内部收缩不溢出） ---- */
.swim-card {
  position: relative;
  flex: none;
  width: 208px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 7px 8px 7px 28px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
  overflow: hidden;
  transition: border-color 0.15s, box-shadow 0.15s;
  cursor: grab;
}

.swim-card:hover {
  border-color: var(--color-border-medium);
}

.swim-card:active {
  cursor: grabbing;
}

.swim-card--fail {
  border-color: var(--color-error);
}

.swim-card__err-dot {
  position: absolute;
  top: -5px;
  right: -5px;
  width: 15px;
  height: 15px;
  border-radius: 99px;
  background: var(--color-error);
  color: var(--color-text-inverted);
  font-size: 9px;
  line-height: 15px;
  text-align: center;
  font-weight: 700;
}

.swim-card__rank {
  position: absolute;
  left: 7px;
  top: 50%;
  transform: translateY(-50%);
  width: 17px;
  height: 17px;
  border-radius: 4px;
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
  font-size: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
}

.swim-card--fail .swim-card__rank {
  background: var(--color-error);
  color: var(--color-text-inverted);
}

.swim-card__rank--first {
  background: var(--color-accent);
  color: var(--color-text-inverted);
}

.swim-card__top {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  position: relative;
}

.swim-card__name {
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
  flex: 1;
}

.fanout-tip {
  display: none;
  position: absolute;
  left: 0;
  bottom: calc(100% + 5px);
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  box-shadow: var(--shadow-strong);
  font-size: 11.5px;
  color: var(--color-text-secondary);
  padding: 4px 10px;
  white-space: nowrap;
  z-index: 10;
}

.fanout-tip b {
  color: var(--color-warning);
}

.swim-card__top:hover .fanout-tip {
  display: block;
}

.swim-card__vendor {
  font-size: 10.5px;
  color: var(--color-text-muted);
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.swim-card__vendor-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.swim-card__foot {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  flex-wrap: wrap;
}

/* select 在卡片内收缩，不撑破（FB-7） */
.swim-card__foot :deep(select) {
  min-width: 0;
  flex: 1 1 56px;
  max-width: 100%;
  font-size: 11px;
  padding: 2px 3px;
}

.swim-card__badge {
  font-size: 9.5px;
  border-radius: 4px;
  padding: 0 4px;
  flex: none;
}

.swim-card__badge--fail {
  color: var(--color-error);
  background: var(--color-error-subtle);
}

.swim-card__badge--takeover {
  color: var(--color-warning);
  background: var(--color-warning-subtle);
}

.swim-card__badge--override {
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.swim-card__dash {
  color: var(--color-text-muted);
  font-size: 11px;
}

.swim-icon-btn {
  border: none;
  background: none;
  padding: 2px 4px;
  border-radius: 4px;
  color: var(--color-text-muted);
  font-size: 12px;
  cursor: pointer;
  flex: none;
}

.swim-icon-btn:hover:not(:disabled) {
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
}

.swim-icon-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.swim-icon-btn--danger:hover {
  color: var(--color-error);
}
</style>
