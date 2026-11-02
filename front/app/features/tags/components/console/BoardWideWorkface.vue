<script setup lang="ts">
/**
 * 单级宽工作面（rework-workshop-into-radar-and-desk 矩阵阶段 G5，获批「依据与管理分离」）。
 *
 * 用途：右轨/窄屏只放结论，完整依据与需要动手的管理都在这一层 1040px（AppDialog size=xl）
 * 工作面里按需查看——**单级、不套 32rem 抽屉套抽屉**：
 *  - 本板块上下文：右轨同源四块（体检/构成候选/追踪/来源）全量呈现（窄屏进入时同样有内容）；
 *  - 体检依据：**问题处理工作面**（board-health-triage-advice seam C2/3.4）——
 *    紧凑问题索引 + 单项处理区 + 折叠的「本轮体检依据」（评分/评语/挂载统计/趋势）；
 *  - 构成 / 组合 / 候选 / 来源 / 设置：复用既有真实面板与处理函数（BoardHealthDetail /
 *    BoardSourcePanel / BoardSettingsForm），不新增功能、不重写口径。
 *
 * 管理类动作（关注管理 / 话题管理）不在本工作面里再叠一层：由壳层关闭本面后再开既有面板
 * （emit openWatchManage，壳层负责顺序），保持单级交互。
 *
 * 只读边界：本组件不发网络请求（来源/构成面板自带既有读链），所有写动作仍由宿主既有
 * 面板与确认流程执行（收编/移除/保存/删除都由壳层注入的处理函数完成）。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import type {
  AuxiliaryLabelItem,
  BoardCompositeMount,
  BoardHealthCandidate,
  BoardHealthSnapshotDTO,
  SemanticBoard,
} from '~/api/semanticBoards'
import BoardHealthDetail from './BoardHealthDetail.vue'
import BoardSettingsForm from './BoardSettingsForm.vue'
import BoardSourcePanel from '../BoardSourcePanel.vue'
import BoardContextRail from './BoardContextRail.vue'
import BoardHealthWorkspace from './board-health/BoardHealthWorkspace.vue'
import { toRailBinding, WORKFACE_TABS, type RailContext, type WorkfaceTab } from './railContext'

const props = defineProps<{
  open: boolean
  boardId: number
  boardLabel: string
  board: SemanticBoard | null
  tab: WorkfaceTab
  /** 右轨同源上下文（体检/追踪/候选数/名单 ID）；context 页签与体检页签共用。 */
  rail: RailContext
  /** 构成面板数据（壳层 useBoardComposition 持有）。 */
  labels: AuxiliaryLabelItem[]
  composites: BoardCompositeMount[]
  compositionLoading: boolean
  compositionError: string | null
  snapshot: BoardHealthSnapshotDTO | null
  capturingId: number | null
  /**
   * 已由宿主**真实摘除成功**的辅助标签 ID（canonical 字符串）。
   * 未回传前工作面不标「已摘除」（失败不谎称已摘除）。
   */
  removedAuxIds?: string[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  'update:tab': [tab: WorkfaceTab]
  remove: [auxiliaryLabelId: number]
  /** 工作面发起的摘除请求（canonical 字符串 ID；宿主转数字后走既有 DELETE 链）。 */
  'remove-composition': [auxiliaryLabelId: string]
  capture: [candidate: BoardHealthCandidate]
  refresh: []
  saved: []
  deleted: []
  retryHealth: []
  retryWatches: []
  locateLane: [topicId: string]
  openWatchManage: []
  openTopics: []
}>()

const TABS = WORKFACE_TABS
const railBinding = computed(() => toRailBinding(props.rail))

const visible = computed({
  get: () => props.open,
  set: (value: boolean) => emit('update:open', value),
})

/**
 * 「已摘除」徽标集合 = **会话摘除记录 ∩ 当前构成中已不存在的标签**。
 *
 * 会话记录只增不减（同一标签被重新挂回后仍在记录里），但徽标按**当前构成**
 * 取差集：重新挂回后徽标自动撤销、「核对后摘除」按钮恢复可用（否则用户看着
 * 一个已经被撤销的「已摘除」，按钮还灰着）。
 */
const effectiveRemovedAuxIds = computed(() => {
  const mounted = new Set<string>()
  for (const item of props.labels) mounted.add(String(item.id))
  for (const item of props.composites) mounted.add(String(item.id))
  return (props.removedAuxIds ?? []).filter((id) => !mounted.has(id))
})

/** context 页里的入口 → 切到对应治理页签（不叠第二层工作面）。 */
function openSources(): void {
  emit('update:tab', 'sources')
}

function openCandidates(): void {
  emit('update:tab', 'candidates')
}

function openHealth(): void {
  emit('update:tab', 'health')
}
</script>

<template>
  <AppDialog v-model="visible" size="xl" data-testid="board-workface">
    <template #header>
      <div class="bwf-head">
        <span class="bwf-kicker">本板块上下文 / 治理</span>
        <h2 class="bwf-title">{{ boardLabel }}</h2>
        <span class="bwf-sub">结论常驻右轨 · 依据与管理在此按需查看；只对本板块生效</span>
      </div>
    </template>

    <div class="bwf-tabs" role="tablist" aria-label="本板块工作面页签">
      <button
        v-for="item in TABS"
        :key="item.key"
        type="button"
        role="tab"
        class="bwf-tab"
        :class="{ 'is-on': tab === item.key }"
        :aria-selected="tab === item.key"
        :data-testid="`workface-tab-${item.key}`"
        @click="emit('update:tab', item.key)"
      >{{ item.label }}</button>
    </div>

    <div class="bwf-body" :data-testid="`workface-body-${tab}`">
      <BoardContextRail
        v-if="tab === 'context'"
        v-bind="railBinding"
        variant="full"
        @retry-health="emit('retryHealth')"
        @open-health="openHealth"
        @open-candidates="openCandidates"
        @open-sources="openSources"
        @open-watch-manage="emit('openWatchManage')"
        @retry-watches="emit('retryWatches')"
        @locate-lane="emit('locateLane', $event)"
        @open-topics="emit('openTopics')"
      />

      <BoardHealthWorkspace
        v-else-if="tab === 'health'"
        :board-id="boardId"
        :board-label="boardLabel"
        :removed-aux-ids="effectiveRemovedAuxIds"
        @remove-composition="emit('remove-composition', $event)"
      />

      <BoardSourcePanel v-else-if="tab === 'sources'" :board-id="boardId" />

      <BoardSettingsForm
        v-else-if="tab === 'settings'"
        :board="board"
        @saved="emit('saved')"
        @deleted="emit('deleted')"
      />

      <template v-else>
        <div v-if="compositionError" class="bwf-error" data-testid="workface-composition-error">
          <Icon icon="mdi:alert-circle-outline" width="14" />
          <span>{{ compositionError }}</span>
          <button type="button" class="bwf-link" @click="emit('refresh')">重试</button>
        </div>
        <div v-else-if="compositionLoading && labels.length === 0" class="bwf-state">
          构成加载中…
        </div>
        <BoardHealthDetail
          :key="tab"
          :board-id="boardId"
          :board-label="boardLabel"
          :labels="labels"
          :composites="composites"
          :loading="compositionLoading"
          :snapshot="snapshot"
          :capturing-id="capturingId"
          :mode="tab"
          @remove="emit('remove', $event)"
          @capture="emit('capture', $event)"
          @refresh="emit('refresh')"
        />
      </template>
    </div>
  </AppDialog>
</template>

<style scoped>
.bwf-head {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}

.bwf-kicker {
  font-size: 0.66rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.bwf-title {
  margin: 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.12rem;
  line-height: 1.4;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.bwf-sub {
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.bwf-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  padding-bottom: 0.7rem;
  margin-bottom: 0.9rem;
  border-bottom: 1px solid var(--color-border-subtle);
}

.bwf-tab {
  padding: 0.25rem 0.6rem;
  border: 1px solid transparent;
  border-radius: 999px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.74rem;
  cursor: pointer;
}

.bwf-tab:hover {
  color: var(--color-text-primary);
}

.bwf-tab.is-on {
  border-color: var(--color-accent);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.bwf-body {
  min-height: 8rem;
}

.bwf-state {
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.bwf-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.74rem;
  color: var(--color-error);
}

.bwf-link {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
}
</style>
