<script setup lang="ts">
/**
 * 版块控制台顶栏（rework-workshop-into-radar-and-desk 矩阵阶段 G5，获批 scope-cleanup）。
 *
 * 只做壳层入口编排，不取数、不持有业务选择：
 *  - 面包屑（主页 / 叙事工坊 / 当前版块）；
 *  - 视图 seg：矩阵 / 追踪（active）｜文章归档｜研究工作台（真实 /tags/:boardId/research）；
 *  - 唯一「最新日报」主按钮（日期来自版块独立 report 元信息，不是 active 泳道最大日）；
 *  - 板块管理（设置 / 构成治理 / 话题管理 / 研究工作台）与新建泳道；
 *  - 中等宽度「本板块上下文」入口（右轨隐藏时的替代路径，打开同一单级工作面）。
 *
 * 全局治理六项/新建版块唯一入口在总览（BoardRadarGovernance），本顶栏不再重复。
 */
import { Icon } from '@iconify/vue'
import ThemeToggle from '~/components/ui/ThemeToggle.vue'
import type { ConsoleView } from '~/features/tags/composables/laneMatrixModel'

const props = withDefaults(defineProps<{
  boardLabel: string
  activeView: ConsoleView
  /** 最新日报日期（YYYY-MM-DD；null = 该版块确无日报或元信息未取到）。 */
  latestReportDate: string | null
  latestReportTitle?: string
  /** 最新日报元信息两个来源均失败（不代表该版块没有历史日报）。 */
  reportMetaUnavailable?: boolean
  /** 刷新失败但保留同板块最后一次成功元信息。 */
  reportMetaStale?: boolean
  refreshing?: boolean
  isNarrow?: boolean
}>(), {
  latestReportTitle: '',
  reportMetaUnavailable: false,
  reportMetaStale: false,
  refreshing: false,
  isNarrow: false,
})

const emit = defineEmits<{
  setView: [view: ConsoleView]
  openReport: []
  refresh: []
  openResearch: []
  manage: [kind: 'settings' | 'composition' | 'topics']
  newLane: []
  openContext: []
}>()

function shortDate(date: string | null): string {
  return date ? date.slice(5).replace('-', '/') : ''
}

function reportLabel(): string {
  if (!props.latestReportDate) return props.reportMetaUnavailable ? '打开日报 · 元信息未取到' : '打开日报 · 暂无'
  return `打开日报 · ${shortDate(props.latestReportDate)}`
}

function reportTitle(): string {
  if (!props.latestReportDate) {
    return props.reportMetaUnavailable
      ? '日报元信息未取到，不代表该版块没有历史日报'
      : '该版块暂无日报'
  }
  const title = props.latestReportTitle ? `《${props.latestReportTitle}》` : '日报'
  return `打开 ${props.latestReportDate} ${title}`
}

function closeManage(event: MouseEvent): void {
  const details = (event.currentTarget as HTMLElement | null)?.closest('details') as HTMLDetailsElement | null
  if (details) details.open = false
}

function pickManage(kind: 'settings' | 'composition' | 'topics', event: MouseEvent): void {
  closeManage(event)
  emit('manage', kind)
}
</script>

<template>
  <header class="console-top" data-testid="console-header">
    <nav class="console-crumb" aria-label="面包屑">
      <NuxtLink to="/" class="console-home" aria-label="返回主页" data-testid="console-home">
        <Icon icon="mdi:arrow-left" width="16" />
        <span class="console-home-label">主页</span>
      </NuxtLink>
      <Icon icon="mdi:chevron-right" width="14" class="console-crumb-sep" />
      <NuxtLink to="/tags" class="console-crumb-ws" data-testid="console-breadcrumb">叙事工坊</NuxtLink>
      <Icon icon="mdi:chevron-right" width="14" class="console-crumb-sep" />
      <span class="console-crumb-cur" :title="boardLabel">{{ boardLabel }}</span>
    </nav>

    <div class="console-actions">
      <div class="console-seg" role="group" aria-label="视图切换">
        <button
          type="button"
          :class="{ 'is-on': activeView === 'matrix' }"
          :aria-pressed="activeView === 'matrix'"
          data-testid="console-view-matrix"
          @click="emit('setView', 'matrix')"
        >
          矩阵 / 追踪
        </button>
        <button
          type="button"
          :class="{ 'is-on': activeView === 'archive' }"
          :aria-pressed="activeView === 'archive'"
          data-testid="console-view-archive"
          @click="emit('setView', 'archive')"
        >
          文章归档
        </button>
        <button
          type="button"
          class="console-seg-nav"
          data-testid="console-view-research"
          title="打开真实研究工作台（不改研究页）"
          @click="emit('openResearch')"
        >
          研究工作台
        </button>
      </div>

      <button
        type="button"
        class="console-report"
        data-testid="console-today-report"
        :disabled="!latestReportDate"
        :title="reportTitle()"
        @click="emit('openReport')"
      >
        <Icon icon="mdi:newspaper-variant-outline" width="14" />
        {{ reportLabel() }}
      </button>

      <button
        type="button"
        class="console-refresh"
        data-testid="console-refresh"
        :disabled="refreshing"
        :title="reportMetaStale || reportMetaUnavailable ? '重试最新日报元信息与泳道数据（失败时保留上次成功元信息）' : '刷新最新日报元信息与泳道数据'"
        @click="emit('refresh')"
      >
        <Icon icon="mdi:refresh" width="14" />
        {{ reportMetaStale ? '元信息陈旧 · 重试' : reportMetaUnavailable ? '重试元信息' : '刷新' }}
      </button>

      <details class="console-manage" data-testid="console-board-manage">
        <summary>板块管理</summary>
        <div class="console-manage-menu">
          <button type="button" data-testid="console-manage-settings" @click="pickManage('settings', $event)">板块设置 / 删除</button>
          <button type="button" data-testid="console-manage-composition" @click="pickManage('composition', $event)">构成与治理</button>
          <button type="button" data-testid="console-manage-topics" @click="pickManage('topics', $event)">话题管理（候选人工激活）</button>
          <button type="button" data-testid="console-manage-research" @click="closeManage($event); emit('openResearch')">研究工作台</button>
        </div>
      </details>

      <button
        type="button"
        class="console-context-toggle"
        data-testid="console-gov-toggle"
        aria-label="打开本板块上下文"
        @click="emit('openContext')"
      >
        <Icon icon="mdi:radar" width="15" />
        上下文
      </button>

      <button
        type="button"
        class="console-new-lane"
        data-testid="console-new-lane"
        :title="isNarrow ? '新建泳道需要宽屏或横屏' : '新建泳道'"
        @click="emit('newLane')"
      >
        <Icon icon="mdi:plus" width="14" />
        新建泳道
      </button>

      <ThemeToggle />
    </div>
  </header>
</template>

<style scoped>
.console-top {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.6rem 1.1rem;
  border-bottom: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  min-width: 0;
}

.console-crumb {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  min-width: 0;
}

.console-home {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  gap: 0.2rem;
  min-width: 1.5rem;
  height: 1.5rem;
  padding: 0 0.3rem;
  border-radius: 3px;
  color: var(--color-text-muted);
  font-size: 0.7rem;
  text-decoration: none;
}

.console-home:hover,
.console-home:focus-visible {
  background: var(--color-bg-hover);
  color: var(--color-accent);
  outline: none;
}

.console-crumb-ws {
  font-size: 0.8rem;
  color: var(--color-text-muted);
  text-decoration: none;
  white-space: nowrap;
}

.console-crumb-ws:hover {
  color: var(--color-accent);
}

.console-crumb-sep {
  color: var(--color-text-muted);
  flex: 0 0 auto;
}

.console-crumb-cur {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.02rem;
  color: var(--color-text-primary);
}

.console-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  justify-content: flex-end;
  min-width: 0;
}

.console-seg {
  display: flex;
  padding: 0.14rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  background: color-mix(in srgb, var(--color-text-primary) 2%, transparent);
}

.console-seg button {
  padding: 0.28rem 0.7rem;
  border: none;
  border-radius: 5px;
  background: none;
  color: var(--color-text-muted);
  font-size: 0.76rem;
  cursor: pointer;
  white-space: nowrap;
}

.console-seg button:hover:not(:disabled) {
  color: var(--color-text-primary);
}

.console-seg button.is-on {
  background: var(--color-accent-subtle);
  color: var(--color-accent);
}

.console-seg .console-seg-nav {
  border-left: 1px solid var(--color-border-subtle);
  border-radius: 0 5px 5px 0;
}

.console-report {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.32rem 0.7rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 42%, transparent);
  border-radius: 7px;
  background: var(--color-accent);
  color: #fff;
  font-size: 0.76rem;
  cursor: pointer;
  white-space: nowrap;
}

.console-report:hover:not(:disabled) {
  filter: brightness(1.06);
}

.console-report:disabled {
  border-color: var(--color-border-medium);
  background: var(--color-bg-hover);
  color: var(--color-text-muted);
  cursor: not-allowed;
}

.console-refresh {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.3rem 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.7rem;
  cursor: pointer;
  white-space: nowrap;
}

.console-refresh:hover:not(:disabled) {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.console-refresh:disabled {
  opacity: 0.55;
  cursor: wait;
}

.console-manage {
  position: relative;
  flex: 0 0 auto;
}

.console-manage summary {
  display: inline-flex;
  align-items: center;
  min-height: 2.1rem;
  padding: 0 0.55rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  cursor: pointer;
  list-style: none;
  white-space: nowrap;
}

.console-manage summary::-webkit-details-marker {
  display: none;
}

.console-manage[open] summary,
.console-manage summary:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.console-manage-menu {
  position: absolute;
  top: calc(100% + 0.3rem);
  right: 0;
  z-index: 40;
  display: flex;
  flex-direction: column;
  min-width: 12rem;
  padding: 0.3rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  background: var(--color-dialog-bg);
  box-shadow: var(--shadow-strong);
}

.console-manage-menu button {
  padding: 0.4rem 0.55rem;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: 0.72rem;
  text-align: left;
  cursor: pointer;
}

.console-manage-menu button:hover {
  background: var(--color-bg-hover);
  color: var(--color-accent);
}

.console-new-lane {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.32rem 0.7rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 32%, transparent);
  border-radius: 7px;
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-size: 0.76rem;
  cursor: pointer;
  white-space: nowrap;
}

.console-new-lane:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.console-context-toggle {
  display: none;
  align-items: center;
  gap: 0.3rem;
  padding: 0.32rem 0.7rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 7px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.76rem;
  cursor: pointer;
}

.console-context-toggle:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-hover);
}

@media (max-width: 1199.98px) and (min-width: 768px) {
  .console-context-toggle {
    display: inline-flex;
  }
}

@media (max-width: 767.98px) {
  .console-top {
    flex-wrap: wrap;
    gap: 0.5rem;
    padding: 0.6rem 0.8rem;
  }

  .console-actions {
    width: 100%;
    margin-left: 0;
    justify-content: flex-start;
  }

  .console-home-label {
    display: none;
  }

  .console-report {
    flex: 1 1 auto;
    justify-content: center;
  }
}
</style>
