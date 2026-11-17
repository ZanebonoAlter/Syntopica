<script setup lang="ts">
/**
 * 左栏 · 首义范围（lane-manual-correction-loop 5.4 / 5.13）。
 *
 * 契约：
 * - **当前首义来源**：自动（创建话题时写入）/ 人工（选定的 N 个节点）；自动首义**无节点集**
 *   → 画布不出现「更早」折叠带（不虚构节点列表）。
 * - **选择池 = 窗口内本泳道节点 + 全部更早成员**（都是本话题成员，都可选）：
 *   未属本话题的候选不入池（候选/关联推荐仍是窗口内语义）。
 * - **「更早成员」分组**装池内**窗口外**的节点（日期如实，按月分堆由成员面板承担）；
 *   原「更早 · N（首义集）」不再单独分组——属当前首义集的节点用**首义标记**区分。
 * - 底部按钮「用选中的 N 个节点定首义」**sticky 于块内底部**（块内长清单独立滚动时
 *   按钮不被裁到视口外 —— A-10）。
 */
import { computed, ref } from 'vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

const props = defineProps<{
  /** 当前首义来源结论（自动 / 人工 + 节点数）。 */
  sourceLabel: string
  /** 当前首义来源是否为人工（有节点集）。 */
  manual: boolean
  /** 选择池：窗口内本泳道节点 + 全部更早成员。 */
  pool: LaneCorrectionNode[]
  /** 本泳道成员 id（池内节点的身份标注用）。 */
  memberIds: string[]
  /** 当前首义集的节点 id（池内节点带首义标记）。 */
  anchorIds?: string[]
  /** 已勾选的节点 id（与画布折叠带同一份）。 */
  selection: string[]
  /** 提交中（按钮 loading，防重复提交）。 */
  applying: boolean
  /**
   * 窗口内成员数（本话题成员口径）；缺省 = 按选择池条数回退（旧调用）。
   */
  windowCount?: number | null
  /**
   * 本话题成员总数（= 窗口内 + 窗口外，与左栏「全部 M」同数同定义）；
   * 缺省 = 按选择池条数回退（旧调用）。
   */
  totalCount?: number | null
}>()

const emit = defineEmits<{
  /** 勾选 / 取消勾选一个节点。 */
  toggle: [id: string]
  /** 「用选中的 N 个节点定首义」。 */
  apply: []
}>()

/** 窗外成员（「更早成员」分组；自动首义恒为空）。 */
const earlier = computed<LaneCorrectionNode[]>(() => props.pool.filter(node => node.outsideWindow))

/** 窗口内的可选节点。 */
const inWindow = computed<LaneCorrectionNode[]>(() => props.pool.filter(node => !node.outsideWindow))

/**
 * 「更早成员」分组是否展开（**默认折叠**：97 条堆在那儿占满左栏，
 * 用户要的是「看得见、选得到」，不是一打开就糊一屏）。
 */
const earlierOpen = ref(false)

/** 「窗口内」分组是否展开（默认展开：这是本工作面的主选择区）。 */
const windowOpen = ref(true)

/**
 * 选择池计数（与成员面板徽标同口径：窗口内 N · 全部 M）。
 *
 * 「全部 M」= 本话题成员 section 总数（与左栏同一把尺子，fix-lane-metric-provenance
 * ——不得一个用加法、一个用去重并集）；「窗口内 N」同理用窗口内成员数。
 * 两者缺省（未接线/旧调用）时回退选择池条数。
 */
const poolCountText = computed(() => {
  const windowCount = props.windowCount ?? inWindow.value.length
  const totalCount = props.totalCount ?? props.pool.length
  return `窗口内 ${windowCount} · 全部 ${totalCount}`
})

function isChecked(id: string): boolean {
  return props.selection.includes(id)
}

function isMember(id: string): boolean {
  return props.memberIds.includes(id)
}

/** 属当前首义集（带首义标记）。 */
function isAnchor(id: string): boolean {
  return (props.anchorIds ?? []).includes(id)
}

/** 池内节点的身份标注（成员 / 更早成员 / 未选入）。 */
function nodeTag(node: LaneCorrectionNode): string {
  if (node.outsideWindow) return '更早成员'
  return isMember(node.id) ? '成员' : '未选入'
}
</script>

<template>
  <section class="las-panel" data-testid="lane-anchor-panel">
    <header class="las-head">
      <h3 class="las-title">首义范围</h3>
      <span class="las-count">{{ poolCountText }}</span>
      <span class="las-timing">次日日报生效</span>
    </header>
    <div class="las-body">
      <p class="las-source" :class="{ 'is-manual': manual }" data-testid="lane-anchor-source">
        当前首义来源：<b>{{ sourceLabel }}</b>
      </p>

      <div v-if="earlier.length" class="las-group las-group--early" data-testid="lane-anchor-earlier-group">
        <button
          type="button"
          class="las-group-head"
          :aria-expanded="earlierOpen"
          :aria-label="earlierOpen ? '收起更早成员' : '展开更早成员'"
          data-testid="lane-anchor-earlier-toggle"
          @click="earlierOpen = !earlierOpen"
        >
          <b>更早成员 · {{ earlier.length }}</b>
          <span class="las-group-toggle">{{ earlierOpen ? '收起' : '展开' }}</span>
        </button>
        <ul v-if="earlierOpen" class="las-list" data-testid="lane-anchor-earlier-list">
          <li v-for="node in earlier" :key="node.id" class="las-row">
            <input
              type="checkbox"
              :checked="isChecked(node.id)"
              :aria-label="`首义范围：${node.title}`"
              :data-testid="`lane-anchor-earlier-check-${node.id}`"
              @change="emit('toggle', node.id)"
            >
            <span class="las-name" :title="node.title">{{ node.title }}</span>
            <span v-if="isAnchor(node.id)" class="las-tag" data-testid="lane-anchor-anchor-tag">首义</span>
            <span class="las-meta">{{ node.date.slice(5) }} · {{ nodeTag(node) }}</span>
          </li>
        </ul>
      </div>

      <div v-if="inWindow.length" class="las-group las-group--window" data-testid="lane-anchor-window-group">
        <button
          type="button"
          class="las-group-head"
          :aria-expanded="windowOpen"
          :aria-label="windowOpen ? '收起窗口内成员' : '展开窗口内成员'"
          data-testid="lane-anchor-window-toggle"
          @click="windowOpen = !windowOpen"
        >
          <b>窗口内 · {{ inWindow.length }}</b>
          <span class="las-group-toggle">{{ windowOpen ? '收起' : '展开' }}</span>
        </button>
        <ul v-if="windowOpen" class="las-list" data-testid="lane-anchor-list">
          <li v-for="node in inWindow" :key="node.id" class="las-row">
            <input
              type="checkbox"
              :checked="isChecked(node.id)"
              :aria-label="`首义范围：${node.title}`"
              :data-testid="`lane-anchor-check-${node.id}`"
              @change="emit('toggle', node.id)"
            >
            <span class="las-name" :title="node.title">{{ node.title }}</span>
            <span v-if="isAnchor(node.id)" class="las-tag" data-testid="lane-anchor-anchor-tag">首义</span>
            <span class="las-meta">{{ node.date.slice(5) }} · {{ nodeTag(node) }}</span>
          </li>
        </ul>
      </div>
      <p v-else class="las-empty" data-testid="lane-anchor-empty">暂无可选节点。</p>

      <div class="las-sticky">
        <button
          type="button"
          class="las-apply"
          :disabled="!selection.length || applying"
          data-testid="lane-anchor-apply"
          @click="emit('apply')"
        >用选中的 {{ selection.length }} 个节点定首义</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.las-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.las-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  padding: 10px 12px 6px;
}

.las-title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.las-count {
  font-size: 11px;
  color: var(--color-text-muted);
}

.las-timing {
  margin-left: auto;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  border: 1px dashed var(--color-border-medium);
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.las-body {
  display: flex;
  flex-direction: column;
  min-height: 0;
  flex: 0 1 auto;
  padding: 0 12px 10px;
}

.las-source {
  margin: 0 0 6px;
  font-size: 11px;
  color: var(--color-text-secondary);
  border-left: 2px solid var(--color-border-medium);
  padding-left: 6px;
}

.las-source.is-manual {
  border-left-color: var(--color-warning);
}

.las-source b {
  color: var(--color-text-primary);
  font-weight: 600;
}

.las-list {
  margin: 0;
  padding: 0;
  list-style: none;
  flex: 0 1 auto;
  min-height: 0;
  max-height: 150px;
  overflow-y: auto;
}

.las-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 0;
  border-bottom: 1px solid var(--color-border-subtle);
  font-size: 12px;
}

.las-row:last-child {
  border-bottom: 0;
}

.las-earlier .las-row {
  border-bottom-style: dashed;
}

/* ---------- 分组标题（窗口内 / 更早成员：两个独立分组，样式对齐成员面板） ---------- */
.las-group {
  margin-bottom: 4px;
}

.las-group-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  width: 100%;
  padding: 3px 0 4px;
  border: 0;
  background: transparent;
  cursor: pointer;
  text-align: left;
  color: var(--color-text-primary);
}

.las-group-head b {
  font-size: 11px;
  font-weight: 600;
}

.las-group-toggle {
  margin-left: auto;
  font-size: 10px;
  color: var(--color-text-muted);
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  padding: 0 5px;
  white-space: nowrap;
}

/* 更早成员：次级子分区（降级标题 + 虚线分隔，与成员面板同一视觉语言） */
.las-group--early {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed var(--color-border-medium);
}

.las-group--early .las-group-head b {
  font-size: 11px;
  color: var(--color-text-secondary);
}

.las-group--early .las-name {
  color: var(--color-text-secondary);
}

.las-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
}

.las-meta {
  flex: none;
  font-size: 10px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

/* 首义标记（属当前首义集）：与成员面板同款虚线小标。 */
.las-tag {
  flex: none;
  font-size: 10px;
  color: var(--color-accent);
  border: 1px dashed var(--color-accent);
  border-radius: 3px;
  padding: 0 4px;
}

.las-empty {
  margin: 4px 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

/* 块内操作按钮始终可见（sticky 于块内底部）：块内清单再长也不把它裁到视口外（A-10）。 */
.las-sticky {
  position: sticky;
  bottom: 0;
  flex: none;
  padding: 7px 0 6px;
  background: var(--color-bg-base);
  border-top: 1px solid var(--color-border-subtle);
}

.las-apply {
  width: 100%;
  border: 1px solid var(--color-accent);
  border-radius: 4px;
  background: var(--color-accent-subtle);
  color: var(--color-text-primary);
  font-size: 12px;
  padding: 5px 8px;
  cursor: pointer;
}

.las-apply:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
