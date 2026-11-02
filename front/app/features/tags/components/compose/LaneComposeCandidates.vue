<script setup lang="ts">
/**
 * 编排态左栏 · 候选泳道（rework-workshop-into-radar-and-desk / workshop-lane-compose）。
 *
 * 口径（契约 §2.1 / §2.4）：
 * - 候选链由**服务端**按原 relations 的连通片段给出，前端只渲染，不重算、不新增边；
 * - 每条候选展示：原名、真实成员、首末日期、事件数、连接跨度；
 * - 组复选框与成员复选框都是**原生 checkbox**，三态 unchecked / indeterminate / checked
 *   与成员计数、图上 ✓、保存可用性同源派生（都来自同一个 selectedIds 集合）；
 * - 点候选标题一次补齐该片段全部成员（追加去重、不截条数、不自动补相似、不吞组外人工补选）；
 *   再点全选标题 / 取消组框 / 「取消整组」撤销该组全部成员并保留组外选择。
 */
import { computed } from 'vue'
import { laneSelectionState } from '~/features/tags/composables/laneComposeLogic'
import type { ComposeCandidate, ComposeCandidateChain } from '~/api/persistentTopics'

const props = defineProps<{
  chains: ComposeCandidateChain[]
  sections: ComposeCandidate[]
  selectedIds: Set<string>
  loading?: boolean
}>()

const emit = defineEmits<{
  'toggle-chain': [key: string, checked: boolean]
  'toggle-member': [id: string, checked: boolean]
  'cancel-chain': [key: string]
}>()

const sectionById = computed(() => {
  const map = new Map<string, ComposeCandidate>()
  for (const section of props.sections) map.set(section.id, section)
  return map
})

interface LaneView {
  chain: ComposeCandidateChain
  members: ComposeCandidate[]
  state: ReturnType<typeof laneSelectionState>
}

const lanes = computed<LaneView[]>(() =>
  props.chains
    .map(chain => {
      const members = chain.sectionIds
        .map(id => sectionById.value.get(id))
        .filter((s): s is ComposeCandidate => Boolean(s))
      return { chain, members, state: laneSelectionState(props.selectedIds, chain.sectionIds) }
    })
    .filter(lane => lane.members.length > 0),
)

function onGroupChange(key: string, event: Event): void {
  const input = event.target as HTMLInputElement
  emit('toggle-chain', key, input.checked)
}

function onMemberChange(id: string, event: Event): void {
  const input = event.target as HTMLInputElement
  emit('toggle-member', id, input.checked)
}

function onTitleClick(lane: LaneView): void {
  emit('toggle-chain', lane.chain.key, !lane.state.checked)
}
</script>

<template>
  <aside class="lcc" data-testid="lane-compose-candidates" aria-label="候选泳道">
    <header class="lcc__head">
      <h3 class="lcc__title">候选泳道</h3>
      <span class="lcc__count" data-testid="lane-compose-candidate-count">{{ lanes.length }} 条</span>
    </header>
    <p class="lcc__help">连接跨越至少 3 天。勾选整组，也可逐项调整或取消。</p>

    <div class="lcc__scroll">
      <p v-if="loading" class="lcc__empty">加载中…</p>
      <p v-else-if="lanes.length === 0" class="lcc__empty" data-testid="lane-compose-candidates-empty">
        这段日期内没有连接跨越至少 3 天的候选。可以换个范围，或在右栏「按标题找」里手动选入。
      </p>

      <article
        v-for="lane in lanes"
        :key="lane.chain.key"
        class="lcc__lane"
        :class="{ 'is-selected': lane.state.checked, 'is-partial': lane.state.mixed }"
        :data-chain-key="lane.chain.key"
        :data-testid="`lane-candidate-${lane.chain.key}`"
      >
        <header class="lcc__lane-head">
          <input
            type="checkbox"
            class="lcc__checkbox"
            :checked="lane.state.checked"
            :indeterminate.prop="lane.state.mixed"
            :aria-label="`整组选择：${lane.chain.topicLabel}`"
            :data-testid="`lane-group-${lane.chain.key}`"
            @change="onGroupChange(lane.chain.key, $event)"
          >
          <button
            type="button"
            class="lcc__lane-title"
            :aria-pressed="lane.state.mixed ? 'mixed' : lane.state.checked"
            :data-testid="`lane-title-${lane.chain.key}`"
            @click="onTitleClick(lane)"
          >
            <strong>{{ lane.chain.topicLabel }}</strong>
            <span class="lcc__lane-meta">{{ lane.members.length }} 个事件 · 连接跨度 {{ lane.chain.spanDays }} 天</span>
            <span class="lcc__lane-dates">{{ lane.chain.firstDate.slice(5) }} — {{ lane.chain.lastDate.slice(5) }}</span>
          </button>
        </header>

        <ul class="lcc__members">
          <li v-for="member in lane.members" :key="member.id" class="lcc__member">
            <label class="lcc__member-label" :class="{ 'is-picked': selectedIds.has(member.id) }">
              <input
                type="checkbox"
                class="lcc__checkbox"
                :checked="selectedIds.has(member.id)"
                :aria-label="`选入事件：${member.clusterLabel}`"
                :data-testid="`lane-member-${member.id}`"
                @change="onMemberChange(member.id, $event)"
              >
              <span class="lcc__member-text">
                <small class="lcc__member-date">{{ member.periodDate.slice(0, 10) }}</small>
                <span class="lcc__member-label-text">{{ member.clusterLabel }}</span>
              </span>
            </label>
          </li>
        </ul>

        <footer class="lcc__lane-foot">
          <span class="lcc__lane-state" :data-testid="`lane-state-${lane.chain.key}`">
            {{ lane.state.count }} / {{ lane.state.total }} 已选
          </span>
          <button
            type="button"
            class="lcc__lane-cancel"
            :disabled="lane.state.count === 0"
            :data-testid="`lane-cancel-${lane.chain.key}`"
            @click="emit('cancel-chain', lane.chain.key)"
          >
            取消整组
          </button>
        </footer>
      </article>
    </div>
  </aside>
</template>

<style scoped>
.lcc {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
  border-right: 1px solid var(--color-border-medium);
  background: var(--color-bg-base);
}

.lcc__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  padding: 12px 14px 8px;
  border-bottom: 1px solid var(--color-border-medium);
}

.lcc__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}

.lcc__count,
.lcc__help {
  font-size: 11px;
  color: var(--color-text-muted);
}

.lcc__help {
  margin: 0;
  padding: 8px 14px;
  border-bottom: 1px solid var(--color-border-subtle);
}

.lcc__scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 12px 16px;
}

.lcc__empty {
  color: var(--color-text-muted);
  font-size: 12px;
  line-height: 1.9;
  padding: 16px 2px;
}

.lcc__lane {
  border-bottom: 1px solid var(--color-border-medium);
  padding-bottom: 8px;
}

.lcc__lane.is-selected {
  background: var(--color-accent-subtle);
}

.lcc__lane.is-partial {
  box-shadow: inset 3px 0 0 var(--color-text-muted);
}

.lcc__lane-head {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 12px 2px 4px;
}

.lcc__checkbox {
  width: 15px;
  height: 15px;
  margin: 3px 0 0;
  flex: none;
  accent-color: var(--color-accent);
}

.lcc__lane-title {
  flex: 1;
  min-width: 0;
  display: block;
  text-align: left;
  border: 0;
  background: transparent;
  padding: 0;
  cursor: pointer;
  color: inherit;
  font: inherit;
}

.lcc__lane-title strong {
  display: block;
  font-size: 13px;
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.lcc__lane-meta,
.lcc__lane-dates {
  display: block;
  font-size: 11px;
  color: var(--color-text-muted);
}

.lcc__lane-meta {
  margin-top: 4px;
}

.lcc__members {
  list-style: none;
  margin: 0;
  padding: 0 0 0 24px;
}

.lcc__member-label {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 0;
  cursor: pointer;
}

.lcc__member-text {
  min-width: 0;
}

.lcc__member-date {
  display: block;
  font-size: 10px;
  color: var(--color-text-muted);
}

.lcc__member-label-text {
  display: block;
  font-size: 12px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.lcc__member-label.is-picked .lcc__member-label-text {
  color: var(--color-accent);
}

.lcc__lane-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 2px 4px 24px;
  font-size: 11px;
  color: var(--color-text-muted);
}

.lcc__lane-cancel {
  border: 0;
  background: transparent;
  padding: 2px 0;
  font-size: 11px;
  color: var(--color-accent);
  cursor: pointer;
}

.lcc__lane-cancel:disabled {
  color: var(--color-text-muted);
  cursor: not-allowed;
}
</style>
