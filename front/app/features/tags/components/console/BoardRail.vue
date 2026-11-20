<script setup lang="ts">
/**
 * 版块栏（console-rails-live-signals）：态势页左栏——**逐日线索条码**。
 *
 * 每行 = 名称（超长省略号截断）+ 右侧 14 格逐日线索条码；**行内不出现任何数字**、
 * **不出现口径说明文字**（治理数据退到行的 title / aria）。
 *
 * 条码口径（design D2/D3/D4，组件只渲染不自己算）：
 *  - 14 格 = 共同轴 `[全局锚点-13 … 全局锚点]`（后端按各版块自己的锚点日生成窗口，
 *    前端已按全局锚点重对齐；落在共同轴外的位置 = 未取到 = 斜纹占位，不补 0）；
 *  - 色阶 = 行内峰值归一（复用 `observationLanes` 的 `clueDepth`/`clueMix`，与矩阵日格、
 *    雷达小矩阵同一套）；行整体强度按 14 天总量分档（安静板块压暗）；
 *  - 「未取到」（斜纹）与「安静」（真实 0 的空槽）视觉上天然分开。
 *
 * 行序保持 `GET /semantic-boards` 的 display_order（MUST NOT 因活跃度重排）。
 */
import { Icon } from '@iconify/vue'
import { RAIL_AXIS_DAYS, railCellMix, railRowTitle, type RailBoardRow } from './railClueAxis'

const props = defineProps<{
  /** 左轨行视图模型（共同轴重对齐后的纯视图模型）。 */
  rows: RailBoardRow[]
  /** 共同轴日期（oldest→newest，14 天）；只用于行 title 的逐日明细。 */
  axisDates: string[]
  selectedBoardId: number
  loading: boolean
  error: string | null
  /** overview 本次没取到（版块信号未取到；版块列表本身可能正常）。 */
  signalError?: string | null
  /** 新建版块唯一入口已迁回总览（BoardRadarGovernance）；本版块栏默认不再重复。 */
  allowAddBoard?: boolean
}>()

const emit = defineEmits<{
  select: [boardId: number]
  retry: []
  'add-board': []
}>()

/** 行 title / aria：名称 + 标签挂载数 + 锚点日 + 逐日明细。 */
function rowTitle(row: RailBoardRow): string {
  return railRowTitle(row, props.axisDates)
}

/** 单格色阶混入比例（行内峰值归一；复用矩阵同一套算法）。 */
function cellMix(row: RailBoardRow, cell: number | null): string {
  return railCellMix(cell, row.peak)
}

function cellClass(row: RailBoardRow, cell: number | null, index: number): Record<string, boolean> {
  const active = row.boardId === props.selectedBoardId
  return {
    'is-missing': cell === null,
    'is-val': cellMix(row, cell) !== '',
    // 只有当前选中行的「当日格」（共同轴末日）脉动——13 行同时闪是视觉噪声。
    'is-now': active && index === RAIL_AXIS_DAYS - 1 && cell !== null,
  }
}

function cellStyle(row: RailBoardRow, cell: number | null): Record<string, string> {
  const mix = cellMix(row, cell)
  return mix === '' ? {} : { '--rail-cell-mix': mix }
}
</script>

<template>
  <aside class="rail" data-testid="board-rail">
    <div class="rail-head">
      <span class="rail-title">语义板块</span>
      <span class="rail-count">{{ rows.length }}</span>
    </div>

    <div v-if="error || signalError" class="rail-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ error ?? signalError }}</span>
      <button type="button" class="rail-retry" data-testid="board-rail-retry" @click="emit('retry')">重试</button>
    </div>

    <div v-if="loading" class="rail-skeleton">
      <div v-for="i in 4" :key="i" class="rail-skeleton-row">
        <span class="rail-skeleton-name" />
        <span class="rail-skeleton-axis">
          <i v-for="cell in RAIL_AXIS_DAYS" :key="cell" />
        </span>
      </div>
    </div>

    <div v-else-if="rows.length === 0" class="rail-empty">
      <Icon icon="mdi:folder-outline" width="20" />
      <span>暂无板块</span>
    </div>

    <div v-else class="rail-list">
      <button
        v-for="row in rows"
        :key="row.boardId"
        type="button"
        class="rail-item"
        :class="{ 'is-active': row.boardId === selectedBoardId }"
        :title="rowTitle(row)"
        :aria-label="rowTitle(row)"
        :data-testid="`board-rail-item-${row.boardId}`"
        @click="emit('select', row.boardId)"
      >
        <span class="rail-item-name">{{ row.label }}</span>
        <span class="rail-item-axis" :style="{ '--rail-intensity': String(row.intensity) }">
          <i
            v-for="(cell, index) in row.cells"
            :key="index"
            class="rail-cell"
            :class="cellClass(row, cell, index)"
            :style="[cellStyle(row, cell), { '--i': String(index) }]"
            :data-testid="`board-rail-cell-${row.boardId}-${index}`"
          />
        </span>
      </button>
    </div>

    <button
      v-if="allowAddBoard"
      type="button"
      class="rail-add"
      data-testid="board-rail-add"
      @click="emit('add-board')"
    >
      <Icon icon="mdi:plus" width="14" />
      新建版块
    </button>
  </aside>
</template>

<style scoped>
/* 满高栏体（design D9b）：高度 = 视口高减顶栏，内容超出时整栏纵向滚动；
   极淡底色让栏读作「面」而不是浮条。 */
.rail {
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow-y: auto;
  border-right: 1px solid var(--color-border-subtle);
  background: rgba(232, 227, 218, 0.022);
  padding: 0.9rem 0.7rem 1rem 0.9rem;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.rail-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 0 0.4rem 0.6rem;
}

.rail-title {
  font-size: 0.66rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.rail-count {
  font-size: 0.68rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.rail-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.15rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.68rem;
  cursor: pointer;
}

.rail-error {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.4rem 0.5rem;
  border-radius: 6px;
  background: var(--color-error-subtle);
  color: var(--color-error);
  font-size: 0.7rem;
}

.rail-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding: 0.2rem;
}

.rail-skeleton-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  height: 1.9rem;
}

.rail-skeleton-name {
  flex: 1 1 auto;
  min-width: 0;
  height: 0.7rem;
  border-radius: 4px;
  background: var(--color-bg-hover);
  animation: rail-pulse 1.4s ease-in-out infinite;
}

.rail-skeleton-axis {
  display: flex;
  flex: 0 0 auto;
  gap: 1px;
}

.rail-skeleton-axis i {
  width: 6px;
  height: 9px;
  border-radius: 1px;
  background: var(--color-bg-hover);
  animation: rail-pulse 1.4s ease-in-out infinite;
}

@keyframes rail-pulse {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

.rail-empty {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.8rem 0.5rem;
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

/* 行列表占满栏高；行高弹性 24–40px（栏高富余时均匀撑开，不足时整栏滚动）。 */
.rail-list {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.rail-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 0.35rem;
  width: 100%;
  flex: 1 1 auto;
  min-height: 24px;
  max-height: 40px;
  padding: 0 0.4rem;
  border: none;
  border-bottom: 1px solid var(--color-border-subtle);
  border-radius: 0;
  background: none;
  color: var(--color-text-secondary);
  text-align: left;
  cursor: pointer;
  transition: background 140ms ease;
}

.rail-item:last-child {
  border-bottom: none;
}

.rail-item:hover {
  background: var(--color-bg-hover);
}

.rail-item:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: -2px;
}

/* 选中行左缘强调线：由上而下生长（scaleY）。 */
.rail-item::before {
  content: '';
  position: absolute;
  left: 0;
  top: 3px;
  bottom: 3px;
  width: 2px;
  border-radius: 2px;
  background: var(--color-accent);
  transform: scaleY(0);
  transform-origin: top;
  transition: transform 160ms cubic-bezier(0.4, 0, 0.2, 1);
}

.rail-item.is-active {
  background: var(--color-accent-subtle);
}

.rail-item.is-active::before {
  transform: scaleY(1);
}

.rail-item-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.8rem;
  line-height: 1.4;
}

.rail-item.is-active .rail-item-name {
  color: var(--color-accent);
  font-weight: 600;
}

/* 条码：14 格 × 6px + 1px gap；行整体强度按 14 天总量分档（安静行压暗）。 */
.rail-item-axis {
  flex: 0 0 auto;
  display: flex;
  align-items: stretch;
  gap: 1px;
  height: 9px;
  opacity: var(--rail-intensity, 1);
}

.rail-cell {
  width: 5px;
  border-radius: 1px;
  background: color-mix(in srgb, var(--color-text-primary) 4%, var(--color-bg-base));
  transform: scaleY(0);
  transform-origin: bottom;
  animation: rail-cell-in 240ms cubic-bezier(0.4, 0, 0.2, 1) forwards;
  animation-delay: calc(var(--i, 0) * 12ms);
}

.rail-cell.is-val {
  background: color-mix(in srgb, var(--color-accent) var(--rail-cell-mix, 0%), var(--color-bg-base));
}

/* 未取到 = 斜纹占位（与「安静」的空槽天然不同）。 */
.rail-cell.is-missing {
  background: repeating-linear-gradient(
    -45deg,
    color-mix(in srgb, var(--color-text-primary) 12%, transparent) 0 2px,
    transparent 2px 4px
  );
}

@keyframes rail-cell-in {
  to { transform: scaleY(1); }
}

/* 只有选中行的当日格脉动（不位移）。 */
.rail-cell.is-now {
  animation:
    rail-cell-in 240ms cubic-bezier(0.4, 0, 0.2, 1) forwards,
    rail-now-pulse 2.2s ease-in-out 520ms infinite;
}

@keyframes rail-now-pulse {
  0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-accent) 55%, transparent); }
  50% { box-shadow: 0 0 6px 2px color-mix(in srgb, var(--color-accent) 30%, transparent); }
}

.rail-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.3rem;
  width: 100%;
  margin-top: auto;
  padding: 0.45rem;
  border: 1px dashed var(--color-border-medium);
  border-radius: 5px;
  background: transparent;
  color: var(--color-text-muted);
  font-size: 0.72rem;
  cursor: pointer;
}

.rail-add:hover,
.rail-add:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-accent);
  outline: none;
}

@media (prefers-reduced-motion: reduce) {
  .rail-cell,
  .rail-cell.is-now {
    animation: none;
    transform: scaleY(1);
  }

  .rail-skeleton-name,
  .rail-skeleton-axis i {
    animation: none;
  }

  .rail-item,
  .rail-item::before {
    transition: none;
  }
}
</style>
