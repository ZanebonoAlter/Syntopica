# Daily Report Magazine Layout Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 将日报 tab 从平铺卡片展开改为双态杂志布局（概要列表 + 全屏杂志页），同时优化 TagsPage 布局宽度、默认 tab、匹配详情 sticky 定位、文章时间筛选快捷 chip。

**Architecture:** 纯前端变更，无后端 API 改动。复用现有 `GET /semantic-boards/:id/daily-reports` (list) 和 `GET /daily-reports/:id` (detail) 端点。BoardDailyReportTimeline.vue 从展开/收起模式重构为双态模式（list / magazine）。TagsPage.vue 做布局、默认值、筛选 UI 调整。

**Tech Stack:** Vue 3 Composition API, TypeScript, Scoped CSS, @iconify/vue

---

## Task Dependency Graph

```
Task 1 (TagsPage 基础调整) ← 独立
Task 2 (时间筛选快捷 chip) ← 依赖 Task 1 (同一文件 TagsPage.vue)
Task 3 (概要列表 + 状态管理) ← 独立 (BoardDailyReportTimeline.vue)
Task 4 (全屏杂志页) ← 依赖 Task 3 (同一组件)
Task 5 (过渡动画) ← 依赖 Task 3 + Task 4
Task 6 (验证) ← 依赖所有
```

**并行策略：** Task 1+2 (TagsPage.vue) 与 Task 3 (BoardDailyReportTimeline.vue 基础) 可以并行，因为它们修改不同文件。Task 4+5 必须在 Task 3 之后顺序执行。

---

## Task 1: TagsPage 基础调整（布局宽度 + 默认 tab + sticky 面板）

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Step 1: 修改布局宽度**

在 TagsPage.vue 的 `<style scoped>` 中，找到 `.tags-topbar-inner` 和 `.tags-main` 的 `max-width: 1440px`，改为 `min(1800px, 95vw)`：

```css
/* 原来 */
max-width: 1440px;

/* 改为 */
max-width: min(1800px, 95vw);
```

共两处：`.tags-topbar-inner` (约第 673 行) 和 `.tags-main` (约第 693 行)。

**Step 2: 修改默认 tab 为 'articles'**

在 `handleSelectBoard` 函数中，将 `contentTab.value = 'composition'` 改为 `contentTab.value = 'articles'`。

同时修改初始声明：`const contentTab = ref<'composition' | 'daily-reports' | 'articles'>('articles')` → 但注意初始状态没有选中 board，所以 contentTab 初始值不影响显示。只改 `handleSelectBoard` 中的赋值即可。

**Step 3: 给 MatchDetailPanel 添加 sticky 定位**

在 `<style scoped>` 中找到 `.tags-match-detail-panel` 样式块，添加：

```css
.tags-match-detail-panel {
  width: 320px;
  flex-shrink: 0;
  position: sticky;
  top: 1rem;
  align-self: flex-start;
  max-height: calc(100vh - 6rem);
  overflow-y: auto;
}
```

**Step 4: 验证**

```bash
cd front && pnpm lint
```

**Step 5: Commit**

```bash
git add front/app/features/tags/components/TagsPage.vue
git commit -m "feat(tags): widen layout to 1800px, default to articles tab, sticky match detail"
```

---

## Task 2: 文章时间筛选快捷 chip

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Context:** Task 1 已修改过 TagsPage.vue，本任务在同一文件上继续。注意在 Task 1 完成的代码基础上操作。

**Step 1: 新增 quickRange ref 和日期初始化**

在 `<script setup>` 中添加：

```typescript
const quickRange = ref<'today' | '3d' | '7d' | '30d' | null>('today')

function getDateStr(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function applyQuickRange(range: 'today' | '3d' | '7d' | '30d') {
  quickRange.value = range
  const now = new Date()
  endDate.value = getDateStr(now)
  const start = new Date()
  if (range === 'today') {
    // start = today
  } else if (range === '3d') {
    start.setDate(start.getDate() - 2)
  } else if (range === '7d') {
    start.setDate(start.getDate() - 6)
  } else if (range === '30d') {
    start.setDate(start.getDate() - 29)
  }
  startDate.value = getDateStr(start)
  handleFilterChange()
}
```

**Step 2: 修改 handleSelectBoard — 重置 quickRange 并初始化日期**

在 `handleSelectBoard` 函数中，在现有 `startDate.value = ''` / `endDate.value = ''` 后面，改为设置默认日期并选中 today：

```typescript
// 替换原来的 startDate.value = '' 和 endDate.value = ''
quickRange.value = 'today'
const now = new Date()
startDate.value = getDateStr(now)
endDate.value = getDateStr(now)
```

**Step 3: 修改 handleFilterChange — 自定义日期时清空 quickRange**

在 `handleFilterChange` 中开头添加：当 startDate 或 endDate 不是由 quickRange 计算的值时，清空 quickRange。最简单的方法是在 date input 的 `@change` 处理中清空：

将模板中两个 `<input type="date">` 的 `@change="handleFilterChange()"` 改为各自独立 handler，或更简单地：添加一个 `handleDateInputChange` 函数：

```typescript
function handleDateInputChange() {
  quickRange.value = null
  handleFilterChange()
}
```

然后将两个 date input 的 `@change` 改为 `@change="handleDateInputChange()"`。

**Step 4: 添加快捷 chip 行到模板**

在模板的 `.tags-filter-chips` div 内、`<select>` 之前（或在 date input 之后），添加快捷 chip：

```html
<!-- 快捷时间 chip，放在 date input 之前 -->
<div class="tags-quick-range">
  <button
    v-for="opt in [
      { key: 'today', label: '今天' },
      { key: '3d', label: '3天' },
      { key: '7d', label: '7天' },
      { key: '30d', label: '30天' },
    ]"
    :key="opt.key"
    type="button"
    class="tags-filter-chip"
    :class="{ 'tags-filter-chip--active': quickRange === opt.key }"
    @click="applyQuickRange(opt.key as 'today' | '3d' | '7d' | '30d')"
  >
    {{ opt.label }}
  </button>
</div>
```

建议放在 date input 的前面（一行内 flex），这样视觉上先看到快捷选项，后面是自定义 date fallback。

**Step 5: 添加 .tags-quick-range 样式**

```css
.tags-quick-range {
  display: flex;
  gap: 0.25rem;
  margin-right: 0.5rem;
  padding-right: 0.5rem;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
}
```

快捷 chip 复用 `.tags-filter-chip` 和 `.tags-filter-chip--active` 样式，无需新增。

**Step 6: 验证**

```bash
cd front && pnpm lint
```

**Step 7: Commit**

```bash
git add front/app/features/tags/components/TagsPage.vue
git commit -m "feat(tags): add quick range chips for article date filtering"
```

---

## Task 3: 日报概要列表（收起态）+ 状态管理重构

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Context:** 这是核心重构。将现有展开/收起模式改为双态（list/magazine）模式。本任务只做概要列表（收起态）和状态管理，杂志页模板留空壳。

**Step 1: 重构 script — 新增 viewMode / selectedIndex 状态**

在 `<script setup>` 中：

1. 将 `expandedId` ref 改为 `viewMode` 和 `selectedIndex`：
```typescript
const viewMode = ref<'list' | 'magazine'>('list')
const selectedIndex = ref<number>(-1) // index into reports array
```

2. 保留 `detailCache`、`detailLoading`、`reports`、`days`、`loading` 不变。

3. 删除 `toggleExpand` 函数。

4. 新增 `openMagazine` 函数：
```typescript
async function openMagazine(index: number) {
  const report = reports.value[index]
  if (!report) return
  selectedIndex.value = index
  viewMode.value = 'magazine'

  if (detailCache.value.has(report.id)) return

  detailLoading.value = report.id
  try {
    const res = await getDailyReportDetail(report.id)
    if (res.success && res.data) {
      detailCache.value.set(report.id, res.data.report)
      detailCache.value = new Map(detailCache.value)
    }
  } finally {
    detailLoading.value = null
  }
}

function closeMagazine() {
  viewMode.value = 'list'
  // selectedIndex 保持，用于恢复滚动位置
}
```

5. 新增计算属性：
```typescript
const selectedReport = computed(() => {
  if (selectedIndex.value < 0 || selectedIndex.value >= reports.value.length) return null
  return reports.value[selectedIndex.value]
})

const selectedDetail = computed<DailyReport | null>(() => {
  if (!selectedReport.value) return null
  return detailCache.value.get(selectedReport.value.id) ?? null
})

const totalPages = computed(() => reports.value.length)
const currentPage = computed(() => selectedIndex.value + 1)
```

6. 修改 `watch(() => props.boardId)` — 重置新状态：
```typescript
watch(() => props.boardId, () => {
  days.value = 7
  viewMode.value = 'list'
  selectedIndex.value = -1
  detailCache.value = new Map()
  loadReports()
}, { immediate: true })
```

7. 新增 `formatDateForSummary` 函数（概要卡片日期）：
```typescript
const weekDays = ['日', '一', '二', '三', '四', '五', '六']

function formatDateForSummary(dateStr: string): string {
  const d = new Date(dateStr)
  const month = d.getMonth() + 1
  const day = d.getDate()
  const weekDay = weekDays[d.getDay()]
  return `${month}.${day}  星期${weekDay}`
}

function truncateText(text: string, maxLen: number): string {
  if (!text) return ''
  return text.length > maxLen ? text.slice(0, maxLen) + '...' : text
}
```

**Step 2: 重写模板 — 收起态概要列表**

替换整个 `<template>` 内容。结构如下：

```html
<template>
  <div class="drt-panel">
    <!-- LIST MODE -->
    <template v-if="viewMode === 'list'">
      <!-- Header (保持不变) -->
      <div class="drt-header">
        <Icon icon="mdi:file-document-outline" width="15" class="text-white/50" />
        <span class="drt-title">板块日报</span>
        <span v-if="reports.length" class="drt-count">{{ reports.length }}</span>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="drt-loading">
        <div v-for="i in 2" :key="i" class="drt-skeleton" />
      </div>

      <!-- Empty -->
      <div v-else-if="reports.length === 0" class="drt-empty">
        <Icon icon="mdi:file-document-outline" width="28" class="text-white/15" />
        <p>暂无日报</p>
      </div>

      <!-- Summary list -->
      <div v-else class="drt-list">
        <div
          v-for="(r, idx) in reports"
          :key="r.id"
          class="drt-summary-card"
          :style="{ animationDelay: `${idx * 50}ms` }"
          @click="openMagazine(idx)"
        >
          <div class="drt-summary-top">
            <span class="drt-summary-date">{{ formatDateForSummary(r.period_date) }}</span>
            <span class="drt-summary-status" :class="reportStatusStyle[r.status] || 'bg-gray-800/40 text-gray-400'">
              {{ reportStatusLabel[r.status] || r.status }}
            </span>
          </div>
          <div class="drt-summary-text">{{ truncateText(r.summary || r.title, 30) }}</div>
          <div class="drt-summary-meta">{{ r.article_count }} 篇 · {{ r.cluster_count }} 聚类</div>
        </div>
      </div>

      <!-- Load more -->
      <div v-if="reports.length > 0" class="drt-more">
        <button type="button" class="drt-more-btn" @click="loadMore">加载更早</button>
      </div>
    </template>

    <!-- MAGAZINE MODE (shell only — filled in Task 4) -->
    <template v-if="viewMode === 'magazine'">
      <!-- Placeholder: magazine content will be added in Task 4 -->
      <div class="drt-magazine-placeholder">
        <p>Magazine view for {{ selectedReport?.period_date }}</p>
      </div>
    </template>
  </div>
</template>
```

**Step 3: 概要卡片样式**

替换 `<style scoped>` 中旧的 `.drt-card*` 样式，改为概要卡片样式：

```css
/* 概要卡片 — 去除边框，用留白和字号层级 */
.drt-summary-card {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  padding: 0.55rem 0.6rem;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.12s ease;
  animation: drtFadeIn 200ms ease-out both;
}

@keyframes drtFadeIn {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.drt-summary-card:hover {
  background: rgba(255, 255, 255, 0.03);
}

.drt-summary-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.drt-summary-date {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.3);
}

.drt-summary-text {
  font-size: 0.8rem;
  color: rgba(255, 255, 255, 0.6);
  line-height: 1.4;
}

.drt-summary-meta {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.2);
}
```

保留 `.drt-panel`、`.drt-header`、`.drt-title`、`.drt-count`、`.drt-loading`、`.drt-skeleton`、`.drt-empty`、`.drt-list`、`.drt-more`、`.drt-more-btn`、`@keyframes drtPulse` 等通用样式。

删除所有旧的 `.drt-card*`、`.drt-expanded`、`.drt-section*`、`.drt-highlight*`、`.drt-cluster*`、`.drt-thread*`、`.drt-detail-loading`、`.drt-skeleton-sm` 样式（这些会在 Task 4 用新样式替换）。

**Step 4: 验证**

```bash
cd front && pnpm lint
```

**Step 5: Commit**

```bash
git add front/app/features/tags/components/BoardDailyReportTimeline.vue
git commit -m "refactor(daily-report): replace expand/collapse with list/magazine dual-mode state"
```

---

## Task 4: 全屏杂志页

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Context:** 在 Task 3 的基础上填充杂志页模板和样式。

**Step 1: 新增翻页函数**

```typescript
function goToPage(delta: number) {
  const newIndex = selectedIndex.value + delta
  if (newIndex < 0 || newIndex.value >= reports.value.length) return
  openMagazine(newIndex)
}
```

注意：`openMagazine` 已有缓存逻辑，翻页时会自动加载未缓存的日报详情。

**Step 2: 实现杂志页模板**

替换 Task 3 中的 magazine placeholder：

```html
<!-- MAGAZINE MODE -->
<div v-if="viewMode === 'magazine' && selectedReport" class="drt-magazine">
  <!-- Fixed top bar -->
  <div class="drt-magazine-topbar">
    <button type="button" class="drt-magazine-back" @click="closeMagazine">
      <Icon icon="mdi:arrow-left" width="14" />
      返回
    </button>
    <div class="drt-magazine-nav">
      <span class="drt-magazine-page">{{ currentPage }} / {{ totalPages }}</span>
      <button
        type="button"
        class="drt-magazine-arrow"
        :disabled="selectedIndex <= 0"
        @click="goToPage(-1)"
      >
        <Icon icon="mdi:chevron-left" width="16" />
      </button>
      <button
        type="button"
        class="drt-magazine-arrow"
        :disabled="selectedIndex >= reports.length - 1"
        @click="goToPage(1)"
      >
        <Icon icon="mdi:chevron-right" width="16" />
      </button>
    </div>
  </div>

  <!-- Content area -->
  <div class="drt-magazine-content">
    <!-- Loading -->
    <div v-if="detailLoading === selectedReport.id" class="drt-loading">
      <div v-for="i in 3" :key="i" class="drt-skeleton" />
    </div>

    <template v-else-if="selectedDetail">
      <!-- Decorative date -->
      <div class="drt-mag-date">{{ formatDateForSummary(selectedDetail.period_date) }}</div>

      <!-- Highlights -->
      <div v-if="selectedDetail.highlights?.length" class="drt-mag-section">
        <div v-for="(h, hi) in selectedDetail.highlights" :key="hi" class="drt-mag-highlight">
          <div class="drt-mag-highlight-title">{{ h.title }}</div>
          <div class="drt-mag-highlight-reason">{{ h.reason }}</div>
        </div>
      </div>

      <!-- Separator: solid line -->
      <div v-if="selectedDetail.dynamics" class="drt-mag-separator--solid" />

      <!-- Dynamics -->
      <div v-if="selectedDetail.dynamics" class="drt-mag-section">
        <div class="drt-mag-section-label">板块动态</div>
        <p class="drt-mag-dynamics">{{ selectedDetail.dynamics }}</p>
      </div>

      <!-- Separator: solid line -->
      <div v-if="selectedDetail.sections?.length" class="drt-mag-separator--solid" />

      <!-- Sections / Threads -->
      <div v-if="selectedDetail.sections?.length" class="drt-mag-section">
        <div class="drt-mag-section-label">叙事线索</div>
        <div
          v-for="(section, si) in selectedDetail.sections"
          :key="section.id"
        >
          <!-- Dashed separator between clusters -->
          <div v-if="si > 0" class="drt-mag-separator--dashed" />

          <div class="drt-mag-cluster">
            <div class="drt-mag-cluster-header">
              <span class="drt-mag-cluster-label">{{ section.cluster_label }}</span>
              <span class="drt-mag-cluster-count">{{ section.article_count }} 篇</span>
            </div>

            <div v-if="section.threads?.length" class="drt-mag-threads">
              <div
                v-for="(thread, ti) in section.threads"
                :key="ti"
              >
                <!-- Dashed separator between threads -->
                <div v-if="ti > 0" class="drt-mag-separator--dashed-subtle" />

                <div class="drt-mag-thread">
                  <span class="drt-mag-thread-status" :class="threadStatusColor[thread.status] || 'bg-gray-800/40 text-gray-400'">
                    {{ threadStatusLabel[thread.status] || thread.status }}
                  </span>
                  <div class="drt-mag-thread-body">
                    <div class="drt-mag-thread-title">{{ thread.title }}</div>
                    <div class="drt-mag-thread-summary">{{ thread.summary }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</div>
```

**Step 3: 杂志页样式**

在 `<style scoped>` 中添加：

```css
/* === Magazine Mode === */

.drt-magazine {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 8rem); /* subtract topbar + padding */
  overflow: hidden;
}

.drt-magazine-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0;
  margin-bottom: 0.75rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.drt-magazine-back {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.3rem 0.6rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 6px;
  background: none;
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.drt-magazine-back:hover {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.75);
}

.drt-magazine-nav {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.drt-magazine-page {
  font-size: 0.7rem;
  color: rgba(255, 255, 255, 0.35);
  margin-right: 0.3rem;
}

.drt-magazine-arrow {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 6px;
  background: none;
  color: rgba(255, 255, 255, 0.45);
  cursor: pointer;
  transition: all 0.12s ease;
}

.drt-magazine-arrow:hover:not(:disabled) {
  border-color: rgba(255, 255, 255, 0.2);
  color: rgba(255, 255, 255, 0.7);
}

.drt-magazine-arrow:disabled {
  opacity: 0.25;
  cursor: not-allowed;
}

.drt-magazine-content {
  flex: 1;
  overflow-y: auto;
  padding-right: 0.5rem;
}

/* Decorative date */
.drt-mag-date {
  font-size: 2.2rem;
  font-weight: 300;
  color: rgba(255, 255, 255, 0.12);
  margin-bottom: 1.5rem;
  letter-spacing: 0.05em;
}

/* Sections */
.drt-mag-section {
  margin-bottom: 1rem;
}

.drt-mag-section-label {
  font-size: 0.82rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.55);
  margin-bottom: 0.5rem;
}

/* Highlights */
.drt-mag-highlight {
  margin-bottom: 0.75rem;
}

.drt-mag-highlight-title {
  font-size: 1.3rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.88);
  line-height: 1.4;
  margin-bottom: 0.2rem;
}

.drt-mag-highlight-reason {
  font-size: 0.82rem;
  color: rgba(255, 255, 255, 0.45);
  line-height: 1.6;
}

/* Dynamics */
.drt-mag-dynamics {
  font-size: 0.82rem;
  color: rgba(255, 255, 255, 0.45);
  line-height: 1.7;
}

/* Separators */
.drt-mag-separator--solid {
  height: 1px;
  background: rgba(255, 255, 255, 0.08);
  margin: 1rem 0;
}

.drt-mag-separator--dashed {
  height: 0;
  border-top: 1px dashed rgba(255, 255, 255, 0.05);
  margin: 0.75rem 0;
}

.drt-mag-separator--dashed-subtle {
  height: 0;
  border-top: 1px dashed rgba(255, 255, 255, 0.03);
  margin: 0.4rem 0;
}

/* Cluster */
.drt-mag-cluster {
  margin-bottom: 0.5rem;
}

.drt-mag-cluster-header {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-bottom: 0.4rem;
}

.drt-mag-cluster-label {
  font-size: 0.92rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.78);
}

.drt-mag-cluster-count {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.25);
}

/* Threads */
.drt-mag-threads {
  display: flex;
  flex-direction: column;
}

.drt-mag-thread {
  display: flex;
  align-items: flex-start;
  gap: 0.4rem;
  padding: 0.3rem 0;
}

.drt-mag-thread-status {
  flex-shrink: 0;
  font-size: 0.58rem;
  padding: 0.08rem 0.35rem;
  border-radius: 3px;
  font-weight: 500;
  line-height: 1.4;
  margin-top: 0.05rem;
}

.drt-mag-thread-body {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  min-width: 0;
}

.drt-mag-thread-title {
  font-size: 0.82rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.75);
  line-height: 1.4;
}

.drt-mag-thread-summary {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.38);
  line-height: 1.5;
}
```

**Step 4: 验证**

```bash
cd front && pnpm lint
```

**Step 5: Commit**

```bash
git add front/app/features/tags/components/BoardDailyReportTimeline.vue
git commit -m "feat(daily-report): implement magazine layout with full-screen view and typography hierarchy"
```

---

## Task 5: 状态切换与过渡动画

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Context:** 在 Task 4 基础上添加过渡动画。

**Step 1: 用 Vue Transition 包裹 list 和 magazine 视图**

将模板中的 `<template v-if="viewMode === 'list'">` 改为 `<Transition name="drt-switch">` 包裹的 `v-if`：

```html
<Transition name="drt-switch" mode="out-in">
  <!-- LIST MODE -->
  <div v-if="viewMode === 'list'" key="list">
    <!-- ... 概要列表内容不变 ... -->
  </div>

  <!-- MAGAZINE MODE -->
  <div v-else-if="viewMode === 'magazine' && selectedReport" key="magazine">
    <!-- ... 杂志页内容不变 ... -->
  </div>
</Transition>
```

**Step 2: 添加过渡动画样式**

```css
/* View switch transition: fade + translateY */
.drt-switch-enter-active,
.drt-switch-leave-active {
  transition: opacity 300ms ease-out, transform 300ms ease-out;
}

.drt-switch-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

.drt-switch-leave-to {
  opacity: 0;
}
```

**Step 3: 验证**

```bash
cd front && pnpm lint
```

**Step 4: Commit**

```bash
git add front/app/features/tags/components/BoardDailyReportTimeline.vue
git commit -m "feat(daily-report): add fade+translateY transition between list and magazine views"
```

---

## Task 6: 最终验证

**Step 1: Lint 检查**

```bash
cd front && pnpm lint
```

**Step 2: TypeCheck（必须通过 Windows cmd）**

```bash
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
```

**Step 3: Build（必须通过 Windows cmd）**

```bash
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
```

**Step 4: 单元测试**

```bash
cd front && pnpm test:unit
```

**Step 5: 更新 tasks.md 标记完成**

回到 openspec change 的 tasks.md，将所有任务标记为 `[x]`。

**Step 6: Commit**

```bash
git add openspec/changes/daily-report-magazine-layout/tasks.md
git commit -m "chore: mark daily-report-magazine-layout tasks complete"
```
