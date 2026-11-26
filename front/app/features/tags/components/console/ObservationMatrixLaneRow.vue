<script setup lang="ts">
/**
 * 矩阵单条泳道行（rework-workshop-into-radar-and-desk 8C G2；8C.4S 结构规范包从
 * `ObservationMatrixGrid.vue` 按内聚行为抽出，只 props/emits + 与其他组共用的行/格样式）。
 *
 * DOM：**多根片段**（行头容器 + 逐个日格），不包整行 wrapper——父网格按列直接收这些元素；
 * 插包装层会破坏 sticky 标签列/列对齐。行头容器本身仍是父网格第 1 列的网格项（sticky 不变）。
 * 正式行与休眠行共用本组件（`dormant` 只切弱化视觉与「休眠」标），格值/可点性/title/aria
 * 全由父投影好的 `cells` 携带，本组件不解析日内容。
 *
 * 交互契约不变：点标签 emit `selectLane(topicId)`；点格 emit `selectCell({ row, cell, date })`
 * 由父统一判定可开性（禁用格的真实点击不可达，合成事件兜底提示在父层，本组件不弹浮层）。
 *
 * 行头「⋯」入口（lane-manual-correction-loop 6.3）：行头改为 **div 容器 + 名称按钮 + 操作菜单**
 * （嵌套 button 是非法 HTML）；容器仍是父网格的第 1 列网格项（sticky 标签列与列对齐不变）。
 * 菜单触发器一律 `@click.stop`，**不触发行选中 / 选中日变更**；菜单项动作以 emit 上抛
 * （`editLane` 进修正工作面、`archiveLane` 进归档弹窗），接线由上层壳层负责。
 *
 * 行头就地重命名（`lane-matrix-coverage-and-others-row` 4.2）：菜单第一项「重命名」→
 * 名称位置换成输入框（自动 focus + select）；Enter 提交（复用既有 `PATCH
 * /daily-reports/topics/:id`），Esc / 失焦取消，空名拒收。`renameLane` 缺省时**不显示**
 * 「重命名」项（旧调用/老测试零改动）。编辑态不触发行选中（输入框不 emit `selectLane`）。
 *
 * 覆盖组行（`coverage` = 窗口内未进日报的活跃泳道）：名字是**静态标签**（该泳道不在本窗
 * 名单，选不动），只留「⋯」维护入口；格与活跃行同款（全「—」）。
 *
 * 色深（2026-11-11 用户看版后修订，tasks 3.5）：格底色 = 格视图自带的 `depth`（当日线索合计在
 * 本行**可见窗**内的归一深浅，行内峰值最深、0/未知不着色）；颜色只是冗余强化通道，格值数字与
 * title/aria 仍是唯一信息承载。本组件不自己算分母（归一分母由父/纯模型注入）。
 *
 * 名称看全（2026-11-26 用户反馈「泳道名展示不全」）：标签列宽有限，名称省略号截断后**不再挂原生
 * `title`**（1s 延迟、键盘不出、长名照样截），改用 `useNameHoverCard` 的悬停/聚焦卡片：只对被截断
 * 的名字出卡，卡片内容 = 全名 + 本行指标；`aria-label` 仍带全名与指标，卡片 `aria-hidden`。
 */
import { computed, nextTick, ref } from 'vue'
import { useNotify } from '~/composables/useNotify'
import { Z_INDEX } from '~/components/ui/layout-contract'
import { useNameHoverCard } from '../../composables/useNameHoverCard'
import { clueMix, type MatrixCellView, type ObservationLaneRow } from './observationLanes'
import RowActionMenu from './RowActionMenu.vue'
import type { RowActionMenuItem } from './rowActionMenu'

const props = defineProps<{
  row: ObservationLaneRow
  /** 与父 `axis` 等长的逐日投影（`matrixCellView` 产物）。 */
  cells: MatrixCellView[]
  /** 可见观察轴（升序 date-only；仅用于取每格的日期键/选中交点）。 */
  axis: string[]
  today: string
  selectedLaneId: string | null
  selectedDate: string | null
  /** 休眠行：弱化 + 「休眠」标（分组/折叠态由父持有）。 */
  dormant: boolean
  /** 覆盖组行（窗口内未进日报）：弱化 + 「未进日报」标；名字静态（不在本窗名单）。 */
  coverage?: boolean
  /** 覆盖组行的最近一次出现日期（YYYY-MM-DD；null = 无记录），只进 title。 */
  coverageLastSeen?: string | null
  /**
   * 就地重命名（缺省 = 无该入口，菜单不显示「重命名」）。
   * 返回 true = 已写入成功（退出编辑）；false = 失败（保留编辑态，由调用方报错）。
   */
  renameLane?: (topicId: string, label: string) => Promise<boolean>
}>()

const emit = defineEmits<{
  selectLane: [topicId: string]
  /** 点格：可开性判定与提示在父层（禁用格不 emit selectDate）。 */
  selectCell: [payload: { row: ObservationLaneRow, cell: MatrixCellView, date: string }]
  /** 「编辑泳道」→ 泳道修正工作面（画布组件由上层接线，本组件只上抛）。 */
  editLane: [topicId: string]
  /** 「归档泳道」→ 归档弹窗（sm 档 + 二次确认）。 */
  archiveLane: [topicId: string]
}>()

const { warn: notifyWarn } = useNotify()

/** 行头菜单项（动作标签；不带操作口径解说）。「重命名」仅在注入改名函数时出现。 */
const laneMenuItems = computed<RowActionMenuItem[]>(() => {
  const items: RowActionMenuItem[] = []
  if (props.renameLane) items.push({ key: 'rename', label: '重命名' })
  items.push({ key: 'edit', label: '编辑泳道' })
  items.push({ key: 'archive', label: '归档泳道' })
  return items
})

/** 就地改名编辑态（改名态不进行选中、不弹层）。 */
const editing = ref(false)
const draft = ref('')
const saving = ref(false)
const inputRef = ref<HTMLInputElement | null>(null)

function startRename(): void {
  editing.value = true
  draft.value = props.row.label
  void nextTick(() => {
    inputRef.value?.focus()
    inputRef.value?.select()
  })
}

/** 取消（Esc / 失焦）：保存中不取消（避免把正在提交的输入抹掉）。 */
function cancelRename(): void {
  if (saving.value) return
  editing.value = false
  draft.value = ''
}

/** 提交（Enter）：空名拒收并保留编辑态；失败保留编辑态（不伪装成功）。 */
async function commitRename(): Promise<void> {
  if (saving.value) return
  const rename = props.renameLane
  if (!rename) return
  const name = draft.value.trim()
  if (!name) {
    notifyWarn('泳道名不能为空')
    return
  }
  saving.value = true
  try {
    const ok = await rename(props.row.topicId, name)
    if (ok) {
      editing.value = false
      draft.value = ''
    }
  } catch {
    // 写入失败一律保留编辑态（调用方负责报错），不吞成「成功」。
  } finally {
    saving.value = false
  }
}

function onMenuSelect(key: string): void {
  if (key === 'rename') startRename()
  else if (key === 'edit') emit('editLane', props.row.topicId)
  else if (key === 'archive') emit('archiveLane', props.row.topicId)
}

/** 行头用 source 全窗指标；不随可见日期裁切重算排序或统计。 */
function rowTitle(): string {
  if (props.coverage) {
    const lastSeen = props.coverageLastSeen ? ` · 最近一次 ${props.coverageLastSeen.slice(5)}` : ' · 无记录'
    return `${props.row.label}：窗口内未进日报${lastSeen}`
  }
  return `${props.row.label}：${props.row.clueTotal} 条线索 · ${props.row.sectionTotal} 个主题桶`
}

/* ── 名称看全卡片（悬停/聚焦；卡片是视觉冗余，不重复播报） ── */
const {
  visible: nameCardVisible,
  setFloating: setNameCardEl,
  floatingStyles: nameCardStyles,
  show: showNameCard,
  hide: hideNameCard,
} = useNameHoverCard({ placement: 'bottom-start' })

/** 卡片元信息行（与 `rowTitle` 同一套指标，只是拆成「全名 / 指标」两行排版）。 */
const nameCardMeta = computed(() => {
  if (props.coverage) {
    return props.coverageLastSeen
      ? `窗口内未进日报 · 最近一次 ${props.coverageLastSeen.slice(5)}`
      : '窗口内未进日报 · 无记录'
  }
  const parts = [`${props.row.clueTotal} 条线索`, `${props.row.sectionTotal} 个主题桶`]
  if (props.row.watchLinked) parts.push('已追踪')
  if (props.dormant) parts.push('休眠')
  return parts.join(' · ')
})

/** 名称文本元素（`scrollWidth > clientWidth` 即判定为被省略号截断）。 */
const nameTextRef = ref<HTMLElement | null>(null)

function peekName(event: Event): void {
  showNameCard(event.currentTarget as HTMLElement, nameTextRef.value)
}

function isToday(date: string): boolean {
  return date === props.today
}

/** 单格色深混入比例；depth 0（零线索/未知/断档）返回空串 → 不注入样式，保持底色与未知态。 */
function cellMix(cell: MatrixCellView): string {
  return clueMix(cell.depth)
}

function cellClass(cell: MatrixCellView, date: string): unknown[] {
  return [
    {
      'is-today': isToday(date),
      'is-gap': cell.kind === 'gap',
      'is-unknown': cell.kind === 'unavailable',
      'is-selected': props.row.topicId === props.selectedLaneId && date === props.selectedDate,
      'is-clue-depth': cellMix(cell) !== '',
      // 选中列淡底（console-rails-live-signals D7）：只在**有分区**的那天铺
      // （无分区的历史日不铺，避免灰带盖过「— 无分区」的诚实表达）；不改格子自身颜色。
      'is-col-selected': date === props.selectedDate && cell.kind === 'content',
    },
  ]
}

/** 色深经 CSS 变量交给主题色阶（`--color-accent` mix `--color-bg-base`），不新增第二份色板。 */
function cellStyle(cell: MatrixCellView): Record<string, string> {
  const mix = cellMix(cell)
  return mix === '' ? {} : { '--omx-clue-mix': mix }
}
</script>

<template>
  <div
    class="omx-lane"
    :class="{ 'is-selected': row.topicId === selectedLaneId, 'is-dormant': dormant }"
  >
    <input
      v-if="editing"
      ref="inputRef"
      v-model="draft"
      type="text"
      class="omx-lane-rename-input"
      :aria-label="`重命名 ${row.label}`"
      :disabled="saving"
      :data-testid="`matrix-lane-rename-${row.topicId}`"
      @click.stop
      @keydown.enter.prevent="commitRename"
      @keydown.esc.prevent="cancelRename"
      @blur="cancelRename"
    >
    <button
      v-else-if="!coverage"
      type="button"
      class="omx-lane-name-btn"
      :class="{ 'is-selected': row.topicId === selectedLaneId, 'is-dormant': dormant }"
      :aria-label="rowTitle()"
      :aria-pressed="row.topicId === selectedLaneId"
      :data-testid="`matrix-lane-${row.topicId}`"
      :data-lane-id="row.topicId"
      @mouseenter="peekName"
      @mouseleave="hideNameCard"
      @focus="peekName"
      @blur="hideNameCard"
      @click="emit('selectLane', row.topicId)"
    >
      <span ref="nameTextRef" class="omx-lane-name">{{ row.label }}</span>
      <span v-if="dormant" class="omx-lane-dormant-tag">休眠</span>
      <span v-else-if="row.watchLinked" class="omx-lane-watch">追踪</span>
    </button>
    <span
      v-else
      class="omx-lane-name omx-lane-name-static"
      :aria-label="rowTitle()"
      :data-testid="`matrix-lane-${row.topicId}`"
      :data-lane-id="row.topicId"
      @mouseenter="peekName"
      @mouseleave="hideNameCard"
    >
      <span ref="nameTextRef" class="omx-lane-name-text">{{ row.label }}</span>
    </span>
    <RowActionMenu
      :items="laneMenuItems"
      :trigger-label="`${row.label} 的更多操作`"
      :trigger-testid="`lane-menu-trigger-${row.topicId}`"
      placement="bottom-start"
      @select="onMenuSelect"
    />
  </div>
  <button
    v-for="(cell, cellIndex) in cells"
    :key="`${row.topicId}-${axis[cellIndex]}`"
    type="button"
    class="omx-cell"
    :class="[cellClass(cell, axis[cellIndex]!), { 'is-dormant': dormant }]"
    :style="cellStyle(cell)"
    :disabled="!cell.openable"
    :data-testid="`matrix-cell-${row.topicId}-${axis[cellIndex]}`"
    :data-lane-id="row.topicId"
    :data-date="axis[cellIndex]"
    :aria-label="cell.aria"
    :title="cell.title"
    @click="emit('selectCell', { row, cell, date: axis[cellIndex]! })"
  >{{ cell.text }}</button>

  <!-- 名称看全卡片：Teleport 出矩阵滚动容器（`overflow-x:auto` 会裁掉浮层）。
       卡片只承担视觉冗余，全名与指标已在名称按钮的 `aria-label` 里。 -->
  <Teleport to="body">
    <div
      v-if="nameCardVisible"
      :ref="setNameCardEl"
      class="omx-lane-card"
      :style="[nameCardStyles, { zIndex: Z_INDEX.toast }]"
      aria-hidden="true"
      data-testid="matrix-lane-name-card"
    >
      <span class="omx-lane-card-name">{{ row.label }}</span>
      <span class="omx-lane-card-meta">{{ nameCardMeta }}</span>
    </div>
  </Teleport>
</template>

<style scoped src="./observationMatrixRows.css" />
