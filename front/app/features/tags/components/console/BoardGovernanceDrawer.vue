<script setup lang="ts">
/**
 * 治理抽屉（rework-workshop-into-radar-and-desk 3.2）。
 *
 * 32rem 覆盖式抽屉，五个页签：构成（质量表）/ 组合 / 候选（全量）/ 来源（窗口+排序）/ 设置。
 * 内容复用既有面板分块，抽屉本身只做页签编排与局部 loading/error 呈现：
 *  - 构成/组合/候选 → `BoardHealthDetail`（mode 拆分）
 *  - 来源 → `BoardSourcePanel`（窗口/排序/全量表，只读）
 *  - 设置 → `BoardSettingsForm`（版块级字段；全局参数在设置页）
 *
 * 失败边界：构成加载失败只影响本抽屉内容，右轨与矩阵不受影响（3.3）。
 */
import { computed, nextTick, ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import AppDrawer from '~/components/ui/AppDrawer.vue'
import type {
  AuxiliaryLabelItem,
  BoardCompositeMount,
  BoardHealthCandidate,
  BoardHealthSnapshotDTO,
  SemanticBoard,
} from '~/api/semanticBoards'
import BoardHealthDetail from './BoardHealthDetail.vue'
import BoardSourcePanel from '../BoardSourcePanel.vue'
import BoardSettingsForm from './BoardSettingsForm.vue'
import { GOVERNANCE_TABS, type GovernanceTab } from './governanceTabs'

const props = defineProps<{
  open: boolean
  boardId: number
  boardLabel: string
  board: SemanticBoard | null
  tab: GovernanceTab
  labels: AuxiliaryLabelItem[]
  composites: BoardCompositeMount[]
  loading: boolean
  error: string | null
  snapshot: BoardHealthSnapshotDTO | null
  capturingId: number | null
  /** 中屏（右轨隐藏）时抽屉头部仍要能看到体检分与趋势摘要。 */
  score?: number | null
  checkedAtText?: string
  trendSummary?: string
}>()

const emit = defineEmits<{
  close: []
  'update:tab': [tab: GovernanceTab]
  remove: [auxiliaryLabelId: number]
  capture: [candidate: BoardHealthCandidate]
  refresh: []
  saved: []
}>()

const TABS = GOVERNANCE_TABS

const activeTab = computed(() => (TABS.some((item) => item.key === props.tab) ? props.tab : 'quality'))

const drawerRef = ref<{ scrollToTop: () => void } | null>(null)
// 切页签复位滚动（来源长表 → 设置表单不应停在半截）
watch(activeTab, () => { void nextTick(() => drawerRef.value?.scrollToTop()) })

function selectTab(key: GovernanceTab): void {
  emit('update:tab', key)
}
</script>

<template>
  <AppDrawer
    ref="drawerRef"
    :open="open"
    :label="`${boardLabel} 版块治理`"
    testid="board-governance-drawer"
    close-testid="board-governance-close"
    @close="emit('close')"
  >
    <template #head>
      <div class="gvd-kicker">版块治理</div>
      <h2 class="gvd-title">{{ boardLabel }}</h2>
      <div class="gvd-sub">构成 / 组合 / 候选 / 来源 / 设置</div>
      <div v-if="snapshot" class="gvd-health" data-testid="gov-health-summary">
        <span class="gvd-health-score">{{ score ?? '—' }}</span>
        <span class="gvd-health-meta">
          版块体检<template v-if="checkedAtText"> · {{ checkedAtText }}</template>
          <template v-if="trendSummary"> · {{ trendSummary }}</template>
        </span>
      </div>
    </template>

    <div class="gvd-tabs" role="tablist" aria-label="治理页签">
      <button
        v-for="item in TABS"
        :key="item.key"
        type="button"
        role="tab"
        class="gvd-tab"
        :class="{ 'is-on': activeTab === item.key }"
        :aria-selected="activeTab === item.key"
        :aria-controls="`gov-panel-${item.key}`"
        :data-testid="`gov-tab-${item.key}`"
        @click="selectTab(item.key)"
      >
        {{ item.label }}
      </button>
    </div>

    <div
      :id="`gov-panel-${activeTab}`"
      class="gvd-body"
      role="tabpanel"
      :data-testid="`gov-body-${activeTab}`"
    >
      <template v-if="activeTab === 'sources'">
        <BoardSourcePanel :board-id="boardId" />
      </template>

      <template v-else-if="activeTab === 'settings'">
        <BoardSettingsForm :board="board" @saved="emit('saved')" />
      </template>

      <template v-else>
        <div v-if="error" class="gvd-error" data-testid="gov-composition-error">
          <Icon icon="mdi:alert-circle-outline" width="14" />
          <span>{{ error }}</span>
          <button type="button" class="gvd-retry" @click="emit('refresh')">重试</button>
        </div>
        <div v-else-if="loading && labels.length === 0" class="gvd-loading" data-testid="gov-composition-loading">
          构成加载中…
        </div>
        <BoardHealthDetail
          :key="activeTab"
          :board-id="boardId"
          :board-label="boardLabel"
          :labels="labels"
          :composites="composites"
          :loading="loading"
          :snapshot="snapshot"
          :capturing-id="capturingId"
          :mode="activeTab"
          @remove="emit('remove', $event)"
          @capture="emit('capture', $event)"
          @refresh="emit('refresh')"
        />
      </template>
    </div>
  </AppDrawer>
</template>

<style scoped>
.gvd-health {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  margin-top: 0.35rem;
}

.gvd-health-score {
  font-family: var(--font-serif);
  font-size: 1.3rem;
  line-height: 1;
  color: var(--color-text-primary);
}

.gvd-health-meta {
  font-size: 0.7rem;
  color: var(--color-text-muted);
  line-height: 1.5;
}

.gvd-kicker {
  font-size: 0.66rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

.gvd-title {
  margin: 0.35rem 0 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.05rem;
  line-height: 1.4;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow-wrap: break-word;
}

.gvd-sub {
  margin-top: 0.25rem;
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.gvd-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  padding-bottom: 0.7rem;
  margin-bottom: 0.9rem;
  border-bottom: 1px solid var(--color-border-subtle);
}

.gvd-tab {
  padding: 0.3rem 0.65rem;
  border: 1px solid transparent;
  border-radius: 7px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.74rem;
  cursor: pointer;
}

.gvd-tab:hover {
  color: var(--color-text-secondary);
  background: var(--color-bg-hover);
}

.gvd-tab.is-on {
  border-color: var(--color-border-medium);
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.gvd-body {
  min-width: 0;
}

.gvd-error {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.6rem 0;
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

.gvd-retry {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.15rem 0.45rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
}

.gvd-loading {
  padding: 0.6rem 0;
  font-size: 0.74rem;
  color: var(--color-text-muted);
}

/* 来源全量表在 32rem 抽屉内压缩：保留 spec 必列（5 列），超宽时抽屉内横向滚动 */
.gvd-body :deep(.bsp-panel) {
  margin-top: 0;
  padding: 0;
  border: none;
  background: none;
  overflow-x: auto;
}

.gvd-body :deep(.bsp-table) {
  min-width: 30rem;
  font-size: 0.72rem;
}

.gvd-body :deep(.bsp-footnote) {
  min-width: 30rem;
  font-size: 0.66rem;
}
</style>
