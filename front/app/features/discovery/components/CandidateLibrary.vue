<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppToggle from '~/components/ui/AppToggle.vue'
import { useDiscoveryStore } from '~/stores/discovery'
import type { CandidateAvailability, DiscoveryCandidate } from '~/types/discovery'
import CandidateEditDialog from './CandidateEditDialog.vue'
import CandidateExportDialog from './CandidateExportDialog.vue'
import CandidateImportDialog from './CandidateImportDialog.vue'
import CandidateSubscribeDialog from './CandidateSubscribeDialog.vue'

/**
 * 候选源库页签（improve-discovery-recommendations 5.1）。
 *
 * - 状态区分（C7 Catalog States）：加载 / 空库（新增与导入双入口）/ 筛选无结果（清筛选）/
 *   请求失败（重试，不冒称空库）/ 成功列表。已订阅与参与推荐分列展示，互不混同。
 * - 搜索/筛选在 store 侧发请求（服务端过滤）；输入防抖 300ms。
 * - 超长标题与 URL：min-width:0 + 换行类名锚点（u-break-title/u-break-url），窄屏单列堆叠。
 * - 导入/导出（5.3）：工具栏入口接真实文件流弹窗；订阅动作按需确认（入库 ≠ 订阅）。
 */
const store = useDiscoveryStore()

/** 搜索输入本地副本（防抖同步到 store 筛选） */
const searchText = ref('')
let searchTimer: ReturnType<typeof setTimeout> | undefined

const kindOptions = [
  { value: 'all', label: '全部来源' },
  { value: 'rss', label: '原生 / 手动 RSS' },
  { value: 'rsshub', label: 'RSSHub 目录' },
] as const

const participationOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'enabled', label: '参与推荐' },
  { value: 'disabled', label: '暂停参与' },
] as const

const availabilityText: Record<CandidateAvailability, string> = {
  unknown: '未验证',
  ok: '可用',
  broken: '失效',
  requires_parameters: '需填参验证',
}

const availabilityTone: Record<CandidateAvailability, string> = {
  unknown: 'is-unknown',
  ok: 'is-ok',
  broken: 'is-broken',
  requires_parameters: 'is-unknown',
}

/** 状态视图：loading / error / empty-library / empty-filtered / list */
type LibraryView = 'loading' | 'error' | 'empty-library' | 'empty-filtered' | 'list'

const view = computed<LibraryView>(() => {
  if (store.candidatesLoading && store.candidates.length === 0) return 'loading'
  if (store.candidatesError) return 'error'
  if (store.candidates.length === 0) {
    return store.candidateFiltersActive ? 'empty-filtered' : 'empty-library'
  }
  return 'list'
})

onMounted(() => {
  if (!store.candidatesLoaded && !store.candidatesLoading) {
    void store.loadCandidates()
  }
})

function onSearchInput(value: string | number) {
  searchText.value = String(value)
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    store.setCandidateFilters({ query: searchText.value })
  }, 300)
}

function onKindChange(e: Event) {
  store.setCandidateFilters({ kind: (e.target as HTMLSelectElement).value as 'all' | 'rss' | 'rsshub' })
}

function onParticipationChange(e: Event) {
  store.setCandidateFilters({
    participation: (e.target as HTMLSelectElement).value as 'all' | 'enabled' | 'disabled',
  })
}

function clearFilters() {
  searchText.value = ''
  store.clearCandidateFilters()
}

function retry() {
  void store.loadCandidates()
}

/** 「已存在」跳转：清筛选让目标可见，再滚动高亮 */
const highlightId = ref<string | null>(null)

function showExisting(id: string) {
  searchText.value = ''
  store.clearCandidateFilters()
  highlightId.value = id
  void nextTick(() => {
    const el = document.querySelector(`[data-candidate-id="${id}"]`)
    el?.scrollIntoView({ block: 'center' })
  })
}

function kindBadge(c: DiscoveryCandidate): string {
  return c.kind === 'rsshub' ? 'RSSHub 目录' : '原生 RSS'
}

/**
 * 上游已下架（C2：route.status === 'gone'）：条目保留、订阅不取消，但不可再发起订阅。
 * 仅 gone 拦截订阅；broken 只是端点可用性提示，不在此列。
 */
function isUpstreamGone(c: DiscoveryCandidate): boolean {
  return c.route?.status === 'gone'
}

function subscribedText(c: DiscoveryCandidate): string {
  return c.subscribed ? '已订阅' : '未订阅'
}

/** 编辑弹窗 */
const dialogVisible = ref(false)
const editingCandidate = ref<DiscoveryCandidate | null>(null)

function openCreate() {
  editingCandidate.value = null
  dialogVisible.value = true
}

function openEdit(c: DiscoveryCandidate) {
  editingCandidate.value = c
  dialogVisible.value = true
}

/* —————— 导入 / 导出 / 订阅（5.3） —————— */

const exportVisible = ref(false)
const importVisible = ref(false)

/** 订阅弹窗目标（候选库条目；null = 关闭） */
const subscribeTarget = ref<DiscoveryCandidate | null>(null)
const subscribeVisible = computed({
  get: () => subscribeTarget.value !== null,
  set: (v: boolean) => {
    if (!v) subscribeTarget.value = null
  },
})

function openSubscribe(c: DiscoveryCandidate) {
  if (c.subscribed) return // 已订阅不给重复入口（防重复创建）
  subscribeTarget.value = c
}
</script>

<template>
  <div class="library" data-testid="candidate-library">
    <!-- 工具栏：搜索 + 筛选 + 新增/导入/导出（5.3 接线） -->
    <div class="library__toolbar">
      <div class="library__search">
        <AppInput
          :model-value="searchText"
          type="text"
          placeholder="搜索名称、地址或说明"
          data-testid="library-search-input"
          @update:model-value="onSearchInput"
        />
      </div>
      <select
        class="library__select"
        aria-label="来源类型"
        data-testid="library-kind-select"
        :value="store.candidateFilters.kind"
        @change="onKindChange"
      >
        <option v-for="opt in kindOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
      </select>
      <select
        class="library__select"
        aria-label="推荐参与状态"
        data-testid="library-participation-select"
        :value="store.candidateFilters.participation"
        @change="onParticipationChange"
      >
        <option v-for="opt in participationOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
      </select>
      <AppButton size="md" data-testid="library-create-btn" @click="openCreate">
        <Icon icon="mdi:plus" width="14" height="14" />
        手动新增
      </AppButton>
      <AppButton size="md" variant="secondary" data-testid="library-import-btn" @click="importVisible = true">
        <Icon icon="mdi:upload-outline" width="14" height="14" />
        导入
      </AppButton>
      <AppButton size="md" variant="secondary" data-testid="library-export-btn" @click="exportVisible = true">
        <Icon icon="mdi:download-outline" width="14" height="14" />
        导出
      </AppButton>
      <!-- RSSHub 上游目录手动同步：常驻入口（原先只在目录为空时出现，
           导致「刷新提示先同步目录」时无处可点）；与新增/导入/导出并列。 -->
      <AppButton
        size="md"
        variant="secondary"
        :loading="store.syncingCatalog"
        :disabled="store.syncingCatalog"
        data-testid="library-sync-catalog-btn"
        @click="store.syncCatalog()"
      >
        <Icon icon="mdi:cloud-download-outline" width="14" height="14" />
        同步目录
      </AppButton>
    </div>

    <!-- 加载态：首载无旧数据 -->
    <div v-if="view === 'loading'" class="library__state" data-testid="library-state-loading">
      <Icon icon="mdi:loading" width="36" height="36" class="library__spin" />
      <p class="library__state-title">正在加载候选源库…</p>
    </div>

    <!-- 错误态：请求失败 ≠ 空库，给重试 -->
    <div v-else-if="view === 'error'" class="library__state" data-testid="library-state-error">
      <Icon icon="mdi:alert-circle-outline" width="36" height="36" style="color: var(--color-error)" />
      <p class="library__state-title">候选源库加载失败</p>
      <p class="library__state-text">{{ store.candidatesError }}。这不是空库——目录数据可能仍在，请重试。</p>
      <AppButton size="md" variant="secondary" data-testid="library-retry-btn" @click="retry">重试</AppButton>
    </div>

    <!-- 空库：从未建库，给新增与导入双入口（C7 空库与筛选无结果分流） -->
    <div v-else-if="view === 'empty-library'" class="library__state" data-testid="library-state-empty">
      <Icon icon="mdi:book-open-outline" width="36" height="36" style="color: var(--color-text-muted)" />
      <p class="library__state-title">候选源库还是空的</p>
      <p class="library__state-text">先收藏来源到候选库，需要时再按需订阅；入库不会自动订阅。</p>
      <div class="library__state-actions">
        <AppButton size="md" data-testid="library-empty-create-btn" @click="openCreate">手动新增</AppButton>
        <AppButton size="md" variant="secondary" data-testid="library-empty-import-btn" @click="importVisible = true">
          导入
        </AppButton>
      </div>
    </div>

    <!-- 筛选无结果：清筛选入口 -->
    <div v-else-if="view === 'empty-filtered'" class="library__state" data-testid="library-state-empty-filtered">
      <Icon icon="mdi:magnify" width="36" height="36" style="color: var(--color-text-muted)" />
      <p class="library__state-title">没有符合条件的候选</p>
      <p class="library__state-text">换个关键词或来源类型试试。</p>
      <AppButton size="md" variant="secondary" data-testid="library-clear-filters-btn" @click="clearFilters">
        清除筛选
      </AppButton>
    </div>

    <!-- 成功列表 -->
    <ul v-else class="library__list" data-testid="library-list" aria-live="polite">
      <li
        v-for="c in store.candidates"
        :key="c.id"
        class="library__item"
        :class="{ 'is-highlight': c.id === highlightId }"
        :data-candidate-id="c.id"
      >
        <div class="library__info">
          <div class="library__title-row">
            <h3 class="library__name u-break-title">{{ c.name }}</h3>
            <span class="library__badge" :class="c.kind === 'rsshub' ? 'is-rsshub' : 'is-rss'">{{ kindBadge(c) }}</span>
            <span v-if="isUpstreamGone(c)" class="library__badge is-gone" data-testid="library-gone-badge">
              上游已下架
            </span>
          </div>
          <p class="library__url u-break-url">{{ c.address }}</p>
          <p v-if="c.description" class="library__desc">{{ c.description }}</p>
          <p v-if="c.language || c.region" class="library__meta">{{ [c.language, c.region].filter(Boolean).join(' / ') }}</p>
          <p class="library__availability">
            <span class="library__dot" :class="availabilityTone[c.availability]" />
            {{ availabilityText[c.availability] }}
            <span v-if="c.lastCheckedAt" class="library__meta">· 上次检查 {{ c.lastCheckedAt }}</span>
            <span v-else class="library__meta">· 无检查记录</span>
          </p>
        </div>

        <div class="library__actions">
          <!-- 已订阅与参与推荐分列：两个字段独立展示，不互相替代 -->
          <div class="library__status-cell">
            <span class="library__status-label">订阅状态</span>
            <span
              class="library__status-value"
              :class="c.subscribed ? 'is-subscribed' : 'is-unsubscribed'"
              data-testid="library-subscribed"
            >{{ subscribedText(c) }}</span>
          </div>
          <div class="library__status-cell">
            <span class="library__status-label">参与推荐</span>
            <AppToggle
              :model-value="c.recommendationEnabled"
              :disabled="store.candidateTogglingIds.includes(c.id)"
              :aria-label="`${c.name} 参与推荐`"
              data-testid="library-toggle"
              @update:model-value="store.setCandidateEnabled(c.id, $event)"
            />
          </div>
          <AppButton size="sm" variant="secondary" data-testid="library-edit-btn" @click="openEdit(c)">
            编辑
          </AppButton>
          <!-- 订阅动作：入库 ≠ 订阅，按需另行确认（R6）；已订阅不重复入口；上游已下架不可订阅（C2） -->
          <AppButton
            v-if="!c.subscribed"
            size="sm"
            :disabled="isUpstreamGone(c)"
            :title="isUpstreamGone(c) ? '上游已下架，无法订阅；已有订阅不受影响' : undefined"
            :data-testid="`library-subscribe-btn-${c.id}`"
            @click="openSubscribe(c)"
          >
            订阅
          </AppButton>
        </div>
      </li>
    </ul>

    <p v-if="view === 'list' && store.candidatesPages > 1" class="library__caption">
      停用候选只影响推荐资格，不取消已有订阅；参与推荐也不是自动订阅。
    </p>

    <!-- 分页条（列表末尾；单页不显示）：上一页/页码/下一页，筛选后页码重置 -->
    <nav
      v-if="view === 'list' && store.candidatesPages > 1"
      class="library__pager"
      aria-label="候选源库分页"
      data-testid="library-pager"
    >
      <AppButton
        size="sm"
        variant="secondary"
        :disabled="store.candidatesPage <= 1 || store.candidatesLoading"
        data-testid="library-pager-prev"
        @click="store.goToCandidatesPage(store.candidatesPage - 1)"
      >
        上一页
      </AppButton>
      <span class="library__pager-info" data-testid="library-pager-info" aria-live="polite">
        第 {{ store.candidatesPage }} / {{ store.candidatesPages }} 页 · 共 {{ store.candidatesTotal }} 条
      </span>
      <AppButton
        size="sm"
        variant="secondary"
        :disabled="store.candidatesPage >= store.candidatesPages || store.candidatesLoading"
        data-testid="library-pager-next"
        @click="store.goToCandidatesPage(store.candidatesPage + 1)"
      >
        下一页
      </AppButton>
    </nav>

    <CandidateEditDialog
      v-model="dialogVisible"
      :candidate="editingCandidate"
      @show-existing="showExisting"
    />

    <CandidateExportDialog v-model="exportVisible" />

    <CandidateImportDialog v-model="importVisible" />

    <CandidateSubscribeDialog
      v-model="subscribeVisible"
      :candidate-id="subscribeTarget?.id ?? null"
      :preset-name="subscribeTarget?.name"
    />
  </div>
</template>

<style scoped>
.library {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.library__toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.library__search {
  flex: 1 1 240px;
  min-width: 0;
}

.library__select {
  padding: 8px 10px;
  font-size: 13px;
  font-family: inherit;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  cursor: pointer;
}

.library__select:focus {
  outline: none;
  border-color: var(--color-input-focus);
}

.library__state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 56px 24px;
  text-align: center;
  min-width: 0;
}

.library__state-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text-primary);
}

.library__state-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--color-text-muted);
  max-width: 420px;
}

.library__spin {
  color: var(--color-link, var(--color-accent));
  animation: library-spin 1s linear infinite;
}

@keyframes library-spin {
  to { transform: rotate(360deg); }
}

.library__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.library__item {
  display: flex;
  gap: 16px;
  justify-content: space-between;
  padding: 14px 16px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 12px;
  background: var(--color-bg-elevated);
  transition: border-color 0.2s;
}

.library__item.is-highlight {
  border-color: var(--color-accent);
}

/* 超长标题/URL 可断行：flex 子项 min-width:0（布局契约） */
.library__info {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.library__title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  min-width: 0;
}

.library__name {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text-primary);
  min-width: 0;
  overflow-wrap: anywhere;
}

.u-break-title {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.u-break-url {
  overflow-wrap: anywhere;
  word-break: break-all;
}

.library__url {
  margin: 0;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  color: var(--color-text-muted);
  min-width: 0;
}

.library__desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--color-text-secondary);
  min-width: 0;
  overflow-wrap: anywhere;
  /* 清洗后仍可能较长（RSSHub 上游介绍）：限 3 行防撑爆卡片，完整内容走编辑查看 */
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  overflow: hidden;
}

.library__meta {
  margin: 0;
  font-size: 12px;
  color: var(--color-text-muted);
}

.library__badge {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 500;
  padding: 1px 8px;
  border-radius: 999px;
  border: 1px solid var(--color-border-medium);
  color: var(--color-text-secondary);
  background: var(--color-bg-sunken);
}

.library__badge.is-rsshub {
  color: var(--color-accent);
  border-color: var(--color-accent-subtle);
  background: var(--color-accent-subtle);
}

/* 上游已下架：警示色，双主题 token（light/dark 均有 --color-warning[-subtle]） */
.library__badge.is-gone {
  color: var(--color-warning);
  border-color: var(--color-warning-subtle);
  background: var(--color-warning-subtle);
}

.library__availability {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin: 2px 0 0;
  font-size: 12px;
  color: var(--color-text-secondary);
}

.library__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-text-muted);
  flex-shrink: 0;
}

.library__dot.is-ok {
  background: var(--color-success);
}

.library__dot.is-broken {
  background: var(--color-error);
}

.library__actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 18px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.library__status-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}

.library__status-label {
  font-size: 11px;
  color: var(--color-text-muted);
}

.library__status-value {
  font-size: 13px;
  font-weight: 600;
}

.library__status-value.is-subscribed {
  color: var(--color-success);
}

.library__status-value.is-unsubscribed {
  color: var(--color-text-muted);
}

.library__state-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: center;
}

.library__caption {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--color-text-muted);
}

.library__pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 14px;
  flex-wrap: wrap;
}

.library__pager-info {
  font-size: 12px;
  color: var(--color-text-secondary);
  font-variant-numeric: tabular-nums;
}

/* 窄屏（390×844 检查档）：条目单列堆叠，动作区回到顶部对齐 */
@media (max-width: 720px) {
  .library__item {
    flex-direction: column;
  }

  .library__actions {
    justify-content: flex-start;
  }
}
</style>
