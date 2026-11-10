<script setup lang="ts">
/**
 * 问题索引（board-health-triage-advice seam C2 / task 3.2）。
 *
 * 左侧紧凑索引：**只列需核对项**（remove / rewrite），不回退 v1 的四组彩色卡堆叠。
 * 「其余标签」（keep / insufficient）默认折叠，各自保留语义与证据（不丢项、不伪造"全部健康"）。
 *
 * 交互约定（standard/frontend/interaction-conventions）：
 *  - 状态标记在**标题左侧**同一行（图标 + aria-label），不塞进右侧 meta；
 *  - 状态说明行是纯说明（不可交互），不伪装成动作。
 *
 * fix-board-health-magnet-signal（ui-design 三处信息级变化之②③）：
 *  - 核心主体徽标（只读标记，紧跟状态标记之后、标题文字之前）；
 *  - 闸门降级警示条（`triage_guard.degraded === true` 时出现在索引顶部）。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import {
  BOARD_HEALTH_ISSUE_STATUS_LABEL,
  boardHealthTriageGuardNotice,
  type BoardHealthIssueItem,
  type BoardHealthIssueStatus,
  type BoardHealthOtherItem,
} from './boardHealthWorkspace'
import type { BoardHealthTriageGuardView } from '~/api/normalizers/boardHealthTriage'

const props = defineProps<{
  issues: BoardHealthIssueItem[]
  others: BoardHealthOtherItem[]
  selectedAuxId: string | null
  /** 已由宿主确认真实摘除成功的标签（canonical ID）；未收到反馈前不标"已摘除"。 */
  removedAuxIds?: string[]
  /** 分诊合理性闸门结论（服务端已算好）；null = 无闸门记录。 */
  guard?: BoardHealthTriageGuardView | null
}>()

const emit = defineEmits<{
  select: [auxId: string]
}>()

const removed = computed(() => new Set(props.removedAuxIds ?? []))

/** 闸门警示文案（纯派生；无闸门记录 / 未降级 → null，不显示警示条）。 */
const guardNotice = computed(() => boardHealthTriageGuardNotice(props.guard ?? null))

function statusLabel(item: BoardHealthIssueItem): string {
  if (removed.value.has(item.auxId)) return '已摘除'
  return BOARD_HEALTH_ISSUE_STATUS_LABEL[item.status]
}

/** 状态图标（左侧位）：只有真实状态才给图标，todo 不加标记。 */
function statusIcon(item: BoardHealthIssueItem): string | null {
  if (removed.value.has(item.auxId)) return 'mdi:check-circle-outline'
  switch (item.status as BoardHealthIssueStatus) {
    case 'running':
      return 'mdi:progress-clock'
    case 'completed':
      return 'mdi:check-circle-outline'
    case 'failed':
      return 'mdi:alert-circle-outline'
    case 'conflict':
      return 'mdi:source-branch'
    case 'unavailable':
      return 'mdi:lock-outline'
    case 'unknown':
      return 'mdi:help-circle-outline'
    default:
      return null
  }
}

function categoryLabel(item: BoardHealthIssueItem): string {
  // 不可执行项（组合标签的描述修复）标「只读」：不说得像能直接改。
  if (item.category === 'rewrite') return item.notExecutableReason ? '建议重写描述 · 只读' : '建议重写描述'
  return '建议摘除'
}

function otherCategoryLabel(item: BoardHealthOtherItem): string {
  return item.category === 'keep' ? '保留' : '证据不足'
}
</script>

<template>
  <div class="bhw-index" data-testid="bhw-index">
    <!-- 闸门降级警示条：服务端判定本轮信号不可用（占比/阈值/原因均取服务端值） -->
    <p v-if="guardNotice" class="bhw-guard" role="alert" data-testid="bhw-guard-notice">
      <Icon icon="mdi:shield-alert-outline" width="14" class="bhw-guard-icon" />
      <span>{{ guardNotice }}</span>
    </p>

    <nav class="bhw-tasks" aria-label="待核对问题">
      <p v-if="issues.length === 0" class="bhw-empty" data-testid="bhw-index-empty">
        本轮没有需核对的项。
      </p>
      <button
        v-for="item in issues"
        :key="item.auxId"
        type="button"
        class="bhw-task"
        :class="{ 'is-selected': item.auxId === selectedAuxId }"
        :aria-current="item.auxId === selectedAuxId ? 'true' : 'false'"
        :data-testid="`bhw-task-${item.auxId}`"
        @click="emit('select', item.auxId)"
      >
        <span class="bhw-task-head">
          <Icon
            v-if="statusIcon(item)"
            :icon="statusIcon(item)!"
            width="13"
            class="bhw-task-flag"
            :aria-label="statusLabel(item)"
          />
          <span
            v-if="item.coreSubject"
            class="bhw-core-badge"
            :data-testid="`bhw-core-subject-${item.auxId}`"
          >核心主体</span>
          <strong class="bhw-task-name">{{ item.label }}</strong>
          <span class="bhw-task-status">{{ statusLabel(item) }}</span>
        </span>
        <span class="bhw-task-kind">{{ categoryLabel(item) }}</span>
        <span class="bhw-task-issue">{{ item.evidence }}</span>
      </button>
    </nav>

    <details class="bhw-other" data-testid="bhw-other-labels">
      <summary>其余 {{ others.length }} 个标签 · 按需查看</summary>
      <p v-if="others.length === 0" class="bhw-empty">本轮没有保留或证据不足项。</p>
      <div v-for="item in others" :key="item.auxId" class="bhw-other-item" :data-testid="`bhw-other-${item.auxId}`">
        <p class="bhw-other-head">
          <b>{{ item.label }}</b>
          <span v-if="item.coreSubject" class="bhw-core-badge" :data-testid="`bhw-core-subject-${item.auxId}`">核心主体</span>
          <span class="bhw-other-kind">{{ otherCategoryLabel(item) }}</span>
        </p>
        <p class="bhw-other-evidence">{{ item.evidence }}</p>
      </div>
    </details>
  </div>
</template>

<style scoped>
.bhw-index {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  min-width: 0;
}

.bhw-tasks {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

/* 闸门降级警示条：醒目（警示色边框 + 底色），纯说明不伪装成动作 */
.bhw-guard {
  display: flex;
  align-items: flex-start;
  gap: 0.35rem;
  margin: 0;
  padding: 0.45rem 0.6rem;
  border: 1px solid var(--color-warning);
  border-radius: 8px;
  background: var(--color-warning-subtle);
  color: var(--color-warning);
  font-size: 0.72rem;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.bhw-guard-icon {
  flex: none;
  margin-top: 0.1rem;
}

/* 核心主体徽标：沿用面板内 chip 语汇（细边框 + 沉底 + 弱化文字） */
.bhw-core-badge {
  display: inline-flex;
  align-items: center;
  flex: none;
  padding: 0 0.35rem;
  height: 16px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
  font-size: 0.62rem;
  letter-spacing: normal;
  white-space: nowrap;
}

.bhw-task {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  width: 100%;
  text-align: left;
  padding: 0.45rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-base);
  cursor: pointer;
  font: inherit;
}

.bhw-task:hover {
  background: var(--color-bg-hover);
}

.bhw-task.is-selected {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.bhw-task-head {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  min-width: 0;
}

.bhw-task-flag {
  color: var(--color-accent);
  flex: none;
}

.bhw-task-name {
  font-size: 0.8rem;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.bhw-task-status {
  margin-left: auto;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  white-space: nowrap;
}

.bhw-task-kind {
  font-size: 0.66rem;
  color: var(--color-text-secondary);
}

.bhw-task-issue {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.bhw-other {
  border-top: 1px solid var(--color-border-subtle);
  padding-top: 0.5rem;
}

.bhw-other summary {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.bhw-other-item {
  margin-top: 0.45rem;
}

.bhw-other-head {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  font-size: 0.74rem;
  color: var(--color-text-primary);
}

.bhw-other-kind {
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

.bhw-other-evidence {
  margin: 0.1rem 0 0;
  font-size: 0.7rem;
  color: var(--color-text-secondary);
  line-height: 1.5;
  overflow-wrap: anywhere;
}

.bhw-empty {
  font-size: 0.72rem;
  color: var(--color-text-muted);
}
</style>
