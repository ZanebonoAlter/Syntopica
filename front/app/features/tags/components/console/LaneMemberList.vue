<script setup lang="ts">
/**
 * 左栏 · 本泳道成员 / 更早成员 / 已移出（本次）（lane-manual-correction-loop 5.3 / 5.12；
 * unify-lane-workbench 3.2 收窄：关联推荐迁出至右栏 `LaneRecommendationList.vue`，
 * 左栏 SHALL NOT 再出现推荐分组——左已选、右待选，两侧内容不重复）。
 *
 * 三块纵向堆叠在左栏（380px）里，**块内长清单各自内部滚动**（列表超高不撑破工作面）。
 *
 * 契约：
 * - **计数徽标 = 「窗口 N · 全部 M」**（M = 本话题成员 section 总数 = 窗口内 + 窗口外，
 *   与首义范围面板同数同定义）：数据全量、展示分层——徽标不得只显示窗口数冒充全量；
 *   后端没给全量口径时回退「N 条」。
 * - **成员接近度 = 归属快照口径**（「接近度 x.xx（快照）」）；估算口径只在右栏推荐行
 *   （不同名不同形，fix-lane-metric-provenance）。
 * - **「更早成员 · N」折叠分组**（默认折叠）：窗口外的本话题成员，按月分堆、
 *   组内独立滚动；每条可点开节点卡（标题/日期/接近度/「更早成员」身份标）。
 * - **取消勾选 = 移除**（直接解除归属，不弹任何去处选择器）；与画布上「取消选中」
 *   在视觉上区分：这里用破坏性描边 + tooltip 说明，画布上是细圈。
 * - 成员行带「首义」标（该节点 ∈ 当前首义集 —— 派生自 `topic_anchor_overrides` 的选定节点）。
 * - 「已移出（本次）」只装**本轮**移走的节点，每条带「加回来」（归回本泳道）。
 */
import { computed, ref } from 'vue'
import type { LaneCorrectionNode } from '~/features/tags/composables/useLaneCorrection'

const props = defineProps<{
  /** 本泳道成员（窗口内；计数与清单同源）。 */
  members: LaneCorrectionNode[]
  /** 更早成员（窗口外的本话题成员）；null = 后端未提供全量口径。 */
  earlyMembers?: LaneCorrectionNode[] | null
  /** 全量成员数（窗口成员 + 更早成员）；null = 无全量口径（回退只显示窗口数）。 */
  totalCount?: number | null
  /** 本次移出的节点。 */
  removed: LaneCorrectionNode[]
  /** 当前首义节点 id（成员行标「首义」）。 */
  anchorIds: string[]
  /** 正在提交的节点 id（按钮 loading，防重复提交）。 */
  pendingIds: string[]
}>()

const emit = defineEmits<{
  /** 取消勾选成员 → 移除（直接解除归属）。 */
  remove: [id: string]
  /** 「加回来」→ 归回本泳道。 */
  restore: [id: string]
  /** 「定位」→ 画布聚焦该节点（只改选中态）。 */
  focus: [id: string]
}>()

/** 计数徽标：全量口径可得 → 「窗口 N · 全部 M」；不可得 → 回退「N 条」。 */
const countText = computed(() => {
  const n = props.members.length
  if (props.totalCount == null) return `${n} 条`
  return `窗口 ${n} · 全部 ${props.totalCount}`
})

/** 更早成员按月分堆（月份倒序：最近的月份在前）。 */
interface EarlyMonthBucket {
  /** 分组键（YYYY-MM）。 */
  key: string
  /** 分组标题（如「10月」）。 */
  label: string
  nodes: LaneCorrectionNode[]
}

const earlyBuckets = computed<EarlyMonthBucket[]>(() => {
  const groups = new Map<string, LaneCorrectionNode[]>()
  for (const node of props.earlyMembers ?? []) {
    const key = node.date.slice(0, 7)
    const bucket = groups.get(key)
    if (bucket) bucket.push(node)
    else groups.set(key, [node])
  }
  return [...groups.entries()]
    .sort((a, b) => (a[0] < b[0] ? 1 : -1))
    .map(([key, nodes]) => ({ key, label: `${Number(key.slice(5, 7))}月`, nodes }))
})

/** 「更早成员」分组是否展开（默认折叠：折叠态不占高）。 */
const earlyOpen = ref(false)
/** 打开节点卡的更早成员 id（同一时刻只开一张）。 */
const openCardId = ref<string | null>(null)

function toggleCard(id: string): void {
  openCardId.value = openCardId.value === id ? null : id
}

function isAnchor(id: string): boolean {
  return props.anchorIds.includes(id)
}

function isPending(id: string): boolean {
  return props.pendingIds.includes(id)
}

/**
 * 接近度展示（没有距离口径就不显示 —— 不伪造数字）。
 * 成员/更早成员 = 归属快照；估算口径在右栏推荐行（不同名不同形）。
 */
function proximityText(node: LaneCorrectionNode): string {
  if (node.proximity === null) return ''
  return `接近度 ${node.proximity.toFixed(2)}（快照）`
}
</script>

<template>
  <section class="lml-panel" data-testid="lane-members-panel">
    <header class="lml-head">
      <h3 class="lml-title">本泳道成员</h3>
      <span class="lml-count" data-testid="lane-members-count">{{ countText }}</span>
      <span class="lml-timing lml-timing--now">归属 · 立即生效</span>
    </header>
    <div class="lml-body">
      <ul v-if="members.length" class="lml-list" data-testid="lane-left-members">
        <li
          v-for="node in members"
          :key="node.id"
          class="lml-row"
          :class="{ 'is-anchor': isAnchor(node.id) }"
          :data-testid="`lane-left-member-${node.id}`"
        >
          <input
            type="checkbox"
            class="lml-check lml-check--destructive"
            checked
            :disabled="isPending(node.id)"
            :title="'取消勾选即移出本泳道'"
            :aria-label="`${node.title} · 本泳道成员`"
            :data-testid="`lane-left-check-${node.id}`"
            @change="emit('remove', node.id)"
          >
          <span class="lml-date">{{ node.date.slice(5) }}</span>
          <span class="lml-name" :title="node.title">{{ node.title }}</span>
          <span v-if="proximityText(node)" class="lml-prox" data-testid="lane-left-prox">{{ proximityText(node) }}</span>
          <span v-if="isAnchor(node.id)" class="lml-tag" data-testid="lane-left-anchor-tag">首义</span>
          <button
            type="button"
            class="lml-peek"
            :aria-label="`在画布上定位 ${node.title}`"
            :data-testid="`lane-left-peek-${node.id}`"
            @click="emit('focus', node.id)"
          >定位</button>
        </li>
      </ul>
      <p v-else class="lml-empty" data-testid="lane-left-members-empty">这条泳道还没有成员。</p>
    </div>
  </section>

  <!-- 更早成员（窗口外的本话题成员）：次级子分区（默认折叠，展开后按月分堆） -->
  <section v-if="earlyBuckets.length" class="lml-panel lml-panel--early" data-testid="lane-early-group">
    <header class="lml-head">
      <h3 class="lml-title">更早成员</h3>
      <span class="lml-count" data-testid="lane-early-count">· {{ earlyBuckets.reduce((sum, bucket) => sum + bucket.nodes.length, 0) }}</span>
      <button
        type="button"
        class="lml-fold"
        :aria-expanded="earlyOpen"
        :aria-label="earlyOpen ? '收起更早成员' : '展开更早成员'"
        data-testid="lane-early-toggle"
        @click="earlyOpen = !earlyOpen"
      >{{ earlyOpen ? '收起' : '展开' }}</button>
      <span class="lml-timing lml-timing--next">重心 · 次日重算</span>
    </header>
    <div v-if="earlyOpen" class="lml-early-body">
      <div
        v-for="bucket in earlyBuckets"
        :key="bucket.key"
        class="lml-bucket"
        :data-testid="`lane-early-month-${bucket.key}`"
      >
        <p class="lml-bucket-title">{{ bucket.label }}</p>
        <ul class="lml-list lml-list--bucket">
          <li
            v-for="node in bucket.nodes"
            :key="node.id"
            class="lml-row lml-row--early"
            :class="{ 'is-open': openCardId === node.id }"
            :data-testid="`lane-early-member-${node.id}`"
          >
            <button
              type="button"
              class="lml-early-open"
              :aria-label="`查看「${node.title}」的节点卡`"
              :data-testid="`lane-early-peek-${node.id}`"
              @click="toggleCard(node.id)"
            >
              <span class="lml-date">{{ node.date.slice(5) }}</span>
              <span class="lml-name" :title="node.title">{{ node.title }}</span>
            </button>
            <span v-if="isAnchor(node.id)" class="lml-tag" data-testid="lane-early-anchor-tag">首义</span>
            <!-- 节点卡（更早成员不在画布上：无画布定位） -->
            <div v-if="openCardId === node.id" class="lml-card" :data-testid="`lane-early-card-${node.id}`">
              <strong class="lml-card__title">{{ node.title }}</strong>
              <span class="lml-card__meta">
                {{ node.date }}<template v-if="proximityText(node)"> · {{ proximityText(node) }}</template>
              </span>
              <span class="lml-card__meta">更早成员</span>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </section>

  <section v-if="removed.length" class="lml-panel lml-panel--out" data-testid="lane-removed-group">
    <header class="lml-head">
      <h3 class="lml-title">已移出（本次）</h3>
      <span class="lml-count">{{ removed.length }} 条</span>
      <span class="lml-timing lml-timing--now">归属 · 立即生效</span>
    </header>
    <div class="lml-body">
      <ul class="lml-list" data-testid="lane-removed-list">
        <li
          v-for="node in removed"
          :key="node.id"
          class="lml-row lml-row--out"
          :data-testid="`lane-removed-${node.id}`"
        >
          <span class="lml-date">{{ node.date.slice(5) }}</span>
          <span class="lml-name" :title="node.title">{{ node.title }}</span>
          <button
            type="button"
            class="lml-back"
            :disabled="isPending(node.id)"
            :aria-label="`把「${node.title}」加回本泳道`"
            :data-testid="`lane-restore-${node.id}`"
            @click="emit('restore', node.id)"
          >↩ 加回来</button>
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.lml-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  /* 作为 .bco__left 的 flex 子项：不参与收缩。
     否则块被压到低于自身内容高度 → 行高不足、标题与按钮被硬切（1440×900 实测）。
     左栏整体 `overflow-y:auto` 承接超屏，块内长清单仍由各自的 max-height 内部滚动。 */
  flex-shrink: 0;
  border-bottom: 1px solid var(--color-border-subtle);
}

.lml-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
  padding: 10px 12px 6px;
}

.lml-title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.lml-count {
  font-size: 11px;
  color: var(--color-text-muted);
}

.lml-timing {
  margin-left: auto;
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 3px;
  border: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.lml-timing--now {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.lml-timing--next {
  border-style: dashed;
}

.lml-body {
  flex: 0 1 auto;
  min-height: 0;
  max-height: 190px;
  overflow-y: auto;
  padding: 0 12px 10px;
}

.lml-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.lml-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 0;
  border-bottom: 1px solid var(--color-border-subtle);
  font-size: 12px;
}

.lml-row:last-child {
  border-bottom: 0;
}

.lml-row--out .lml-name {
  color: var(--color-text-muted);
  text-decoration: line-through;
}

/* ---------- 更早成员：次级子分区（从属「本泳道成员」，视觉降级 + 明确分隔） ---------- */
.lml-panel--early {
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent);
  border-top: 1px dashed var(--color-border-medium);
  border-bottom: 1px solid var(--color-border-subtle);
}
.lml-panel--early .lml-title {
  font-size: 11px;
  color: var(--color-text-secondary);
}
.lml-panel--early .lml-row .lml-date,
.lml-panel--early .lml-row .lml-name {
  color: var(--color-text-secondary);
}
.lml-panel--early .lml-bucket-title {
  margin: 6px 0 2px;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--color-text-muted);
}

.lml-check {
  flex: none;
  accent-color: var(--color-accent);
}

/* 破坏性动作与「取消选中」在视觉上区分：移除用危险描边，选中取消是画布细圈。 */
.lml-check--destructive {
  accent-color: var(--color-error);
}

.lml-date {
  flex: none;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.lml-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-primary);
}

.lml-row.is-anchor .lml-name {
  color: var(--color-accent);
}

.lml-tag {
  flex: none;
  font-size: 10px;
  color: var(--color-accent);
  border: 1px dashed var(--color-accent);
  border-radius: 3px;
  padding: 0 4px;
}

.lml-prox {
  flex: none;
  font-size: 10px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.lml-peek,
.lml-back {
  flex: none;
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 11px;
  padding: 1px 6px;
  cursor: pointer;
}

.lml-peek:hover,
.lml-back:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.lml-peek:disabled,
.lml-back:disabled {
  opacity: 0.55;
  cursor: wait;
}

.lml-empty {
  margin: 4px 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

/* 「更早成员」分组：默认折叠（折叠态只有标题行，不占高）。 */
.lml-fold {
  flex: none;
  border: 1px solid var(--color-border-medium);
  border-radius: 3px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 11px;
  padding: 1px 6px;
  cursor: pointer;
}

.lml-fold:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.lml-early-body {
  max-height: 190px;
  overflow-y: auto;
  padding: 0 12px 10px;
}

.lml-bucket + .lml-bucket {
  margin-top: 6px;
}

.lml-bucket-title {
  margin: 4px 0 2px;
  font-size: 10px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

/* 每一堆内部独立滚动（不让某一堆吃掉整块高度）。 */
.lml-list--bucket {
  max-height: 110px;
  overflow-y: auto;
}

.lml-row--early {
  flex-wrap: wrap;
}

.lml-early-open {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  border: 0;
  background: transparent;
  padding: 0;
  text-align: left;
  font-size: 12px;
  cursor: pointer;
}

.lml-early-open .lml-name {
  white-space: nowrap;
}

/* 节点卡（更早成员不在画布上：无画布定位）。 */
.lml-card {
  flex: 1 0 100%;
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 4px 0 6px;
  padding: 6px 8px;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: var(--color-bg-base);
}

.lml-card__title {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.lml-card__meta {
  font-size: 10px;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}
</style>
