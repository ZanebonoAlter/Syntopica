<script setup lang="ts">
import { Icon } from '@iconify/vue'
import NotificationItem from './NotificationItem.vue'
import AppDialog from './AppDialog.vue'
import { useNotifications } from '~/composables/useNotifications'
import { useSchedulerStatus } from '~/composables/useSchedulerStatus'
import { useHealthReprobe } from '~/composables/useHealthReprobe'
import type { AppNotification } from '~/api/notifications'

/**
 * 通知下拉面板（notification-center）
 * ui-design：锚定 popover min(380px,92vw)、max-height 480px 内滚、
 * 状态矩阵 loading/empty/error/success、滚动加载更多、错误态+重试、
 * 全部标已读、清空走 AppDialog sm confirm、单条 hover 标已读。
 */

const emit = defineEmits<{ close: [] }>()

const {
  view,
  unreadCount,
  markRead,
  markAllRead,
  clearAll,
  hasMore,
  pageSize,
  fetchList,
  browsingSessionActive,
} = useNotifications()

/**
 * 置顶系统状态条（客户端虚拟条目）：首例 AI 健康未就绪提示（ai-health-to-notifications）。
 * 由前端运行时状态驱动显隐，不落库/不触发 WS/不计未读数/不参与淘汰与已读生命周期；
 * 可见性契约沿用 analysis-pause-control spec——意图运行（analysisPaused=false）
 * 但健康门未通过（aiHealthy=false），用户主动暂停不提示。约束（flow/scheduler.md
 * 业务约束第 7 条）：只做提示，不禁用/不改写暂停/启动按钮。
 */
const { analysisPaused, aiHealthy, loadSchedulersStatus } = useSchedulerStatus()
const { reprobing, reprobeHealth } = useHealthReprobe()

const aiUnready = computed(() => !analysisPaused.value && !aiHealthy.value)

async function onReprobe() {
  await reprobeHealth()
  // 置顶条可见性由 scheduler status 的 ai_healthy 驱动，重探后刷新以关闭。
  await loadSchedulersStatus()
}

const confirmVisible = ref(false)
const markingAll = ref(false)
const loadingMore = ref(false)
const listEl = ref<HTMLElement | null>(null)

async function loadMore() {
  if (loadingMore.value || !hasMore()) return
  loadingMore.value = true
  try {
    await fetchList(view.value.list.length)
  } finally {
    loadingMore.value = false
  }
}

function onListScroll() {
  const el = listEl.value
  if (!el) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 24) {
    void loadMore()
  }
}

async function onMarkAllRead() {
  markingAll.value = true
  try {
    await markAllRead()
  } finally {
    markingAll.value = false
  }
}

async function onClearConfirm() {
  confirmVisible.value = false
  await clearAll()
}

/** 点击通知：按 link_type 跳转 + 自动标已读 + 关闭面板（spec「失败通知携带可跳转目标」） */
function onItemClick(item: AppNotification) {
  if (!item.is_read) void markRead(item.id)
  if (item.link_type === 'daily-report') {
    // 日报读者在 /tags 页（useDailyReportReader 挂 TagsPage）；link_id 为生成日期，
    // 页面内默认展示最新日期，暂不带 query（无锚定到具体日期的路由参数）
    void navigateTo('/tags')
  }
  // 其他/空 link_type：无跳转目标，保持仅标已读 + 关闭
  emit('close')
}

/** 相对时间（今昨日常见窗口；更旧显示日期） */
function relativeTime(created: string): string {
  const ts = new Date(created).getTime()
  if (Number.isNaN(ts)) return ''
  const diff = Date.now() - ts
  const min = Math.floor(diff / 60_000)
  if (min < 1) return '刚刚'
  if (min < 60) return `${min} 分钟前`
  const hour = Math.floor(min / 60)
  if (hour < 24) return `${hour} 小时前`
  const day = Math.floor(hour / 24)
  if (day === 1) return '昨天'
  return new Date(created).toLocaleDateString('zh-CN')
}
</script>

<template>
  <Teleport to="body">
    <div class="notif-panel" data-testid="notification-panel" role="dialog" aria-label="通知">
      <div class="notif-panel__head">
        <h3 class="notif-panel__title">
          通知
          <span v-if="browsingSessionActive" class="notif-panel__hint" data-testid="panel-browsing-hint">已全部浏览</span>
        </h3>
        <button class="notif-panel__mark-all" data-testid="mark-all-read" :disabled="markingAll" @click="onMarkAllRead">
          全部已读
        </button>
      </div>

      <!-- 置顶系统状态条（客户端虚拟条目，不入 view.list 分页序列）：
           列表 loading/empty/error 各态下均可见，不受清空/全部已读影响 -->
      <div v-if="aiUnready" class="notif-panel__status" data-testid="panel-system-status" role="alert">
        <Icon icon="mdi:alert" width="16" height="16" class="shrink-0" />
        <span class="notif-panel__status-text">AI 模型未就绪（LLM/Embedding 未连通），分析暂停运行</span>
        <div class="notif-panel__status-actions">
          <button
            type="button"
            class="notif-panel__status-action"
            data-testid="panel-system-reprobe"
            :disabled="reprobing"
            @click="onReprobe"
          >
            {{ reprobing ? '检测中…' : '重新检测' }}
          </button>
          <NuxtLink
            to="/settings?section=ai-health"
            class="notif-panel__status-action"
            data-testid="panel-system-config"
            @click="emit('close')"
          >
            去配置
          </NuxtLink>
        </div>
      </div>

      <div ref="listEl" class="notif-panel__list" @scroll.passive="onListScroll">
        <!-- loading 态：骨架行 -->
        <template v-if="view.loading && view.list.length === 0">
          <div v-for="i in 3" :key="i" class="notif-skeleton" data-testid="panel-skeleton" />
        </template>

        <!-- error 态：面板内错误 + 重试 -->
        <div v-else-if="view.error" class="notif-empty" data-testid="panel-error">
          <Icon icon="mdi:alert-circle-outline" width="32" height="32" />
          <p>{{ view.error }}</p>
          <button class="notif-panel__mark-all" @click="fetchList(0)">重试</button>
        </div>

        <!-- empty 态 -->
        <div v-else-if="view.list.length === 0" class="notif-empty" data-testid="panel-empty">
          <Icon icon="mdi:bell-off-outline" width="32" height="32" />
          <p>暂无通知</p>
          <p class="notif-empty__sub">任务完成/失败时会出现通知</p>
        </div>

        <!-- success 态 -->
        <template v-else>
          <NotificationItem
            v-for="item in view.list"
            :key="item.id"
            :item="item"
            :highlight-unread="browsingSessionActive && !item.is_read"
            @click="onItemClick(item)"
            @mark-read="markRead(item.id)"
          />
          <div v-if="loadingMore" class="notif-panel__loading-more" data-testid="panel-loading-more">加载中…</div>
        </template>
      </div>

      <div class="notif-panel__foot">
        <button data-testid="panel-view-all" @click="$router.push('/settings?section=queues')">设置</button>
        <button class="notif-panel__danger" data-testid="panel-clear" @click="confirmVisible = true">清空</button>
      </div>
    </div>

    <!-- 清空确认（危险操作，AppDialog sm 档） -->
    <AppDialog v-model="confirmVisible" size="sm" title="清空全部通知">
      <p style="color: var(--color-text-secondary); font-size: 14px">确认清空全部通知？此操作不可恢复。</p>
      <template #footer>
        <AppButton variant="ghost" @click="confirmVisible = false">取消</AppButton>
        <AppButton variant="danger" data-testid="panel-clear-confirm" @click="onClearConfirm">清空</AppButton>
      </template>
    </AppDialog>
  </Teleport>
</template>

<style scoped>
.notif-panel {
  position: fixed;
  top: 3.75rem;
  right: 0.75rem;
  width: min(380px, 92vw);
  max-height: 480px;
  display: flex;
  flex-direction: column;
  background: var(--color-bg-base);
  border: 1px solid var(--color-border-medium);
  border-radius: 10px;
  box-shadow: var(--shadow-strong);
  z-index: 60;
  overflow: hidden;
}

.notif-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--color-border-subtle);
}

.notif-panel__title {
  font-size: 0.9375rem;
  font-weight: 700;
  color: var(--color-text-primary);
  margin: 0;
}

.notif-panel__hint {
  font-size: 0.75rem;
  font-weight: 400;
  color: var(--color-text-muted);
  margin-left: 0.375rem;
}

.notif-panel__mark-all {
  background: none;
  border: 1px solid var(--color-border-medium);
  border-radius: 0.375rem;
  padding: 0.25rem 0.625rem;
  font-size: 0.75rem;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.notif-panel__mark-all:hover:not(:disabled) {
  border-color: var(--color-border-strong);
  background: var(--color-bg-hover);
}

.notif-panel__mark-all:disabled {
  opacity: 0.5;
  cursor: default;
}

/* 置顶系统状态条：warning 色令牌沿用原顶部 banner 视觉；窄屏 flex-wrap 下
   文案可换行、按钮组不挤爆 */
.notif-panel__status {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin: 0.5rem 1rem 0;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 0.8125rem;
  background: var(--color-warning-bg, rgba(196, 136, 60, 0.12));
  border: 1px solid var(--color-warning-border, rgba(196, 136, 60, 0.35));
  color: var(--color-warning);
}

.notif-panel__status-text {
  flex: 1;
  min-width: 0;
}

.notif-panel__status-actions {
  display: flex;
  gap: 10px;
  flex-shrink: 0;
}

.notif-panel__status-action {
  flex-shrink: 0;
  font-weight: 600;
  color: var(--color-link);
  text-decoration: underline;
  white-space: nowrap;
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  font-size: 0.8125rem;
}

.notif-panel__status-action:disabled {
  opacity: 0.6;
  cursor: default;
}

.notif-panel__status-action:hover:not(:disabled) {
  opacity: 0.8;
}

.notif-panel__list {
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

.notif-skeleton {
  height: 64px;
  margin: 0.5rem 1rem;
  border-radius: 6px;
  background: linear-gradient(90deg, var(--color-bg-sunken) 25%, var(--color-bg-hover) 50%, var(--color-bg-sunken) 75%);
  background-size: 200% 100%;
  animation: notif-shimmer 1.4s infinite;
}

@keyframes notif-shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}

.notif-empty {
  padding: 2.5rem 1rem;
  text-align: center;
  color: var(--color-text-muted);
  font-size: 0.8125rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
}

.notif-empty__sub {
  font-size: 0.6875rem;
  opacity: 0.8;
}

.notif-panel__loading-more {
  text-align: center;
  padding: 0.5rem;
  font-size: 0.75rem;
  color: var(--color-text-muted);
}

.notif-panel__foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 1rem;
  border-top: 1px solid var(--color-border-subtle);
}

.notif-panel__foot button {
  background: none;
  border: none;
  font-size: 0.75rem;
  color: var(--color-text-muted);
  cursor: pointer;
  padding: 0.25rem 0.5rem;
  border-radius: 0.25rem;
}

.notif-panel__foot button:hover {
  color: var(--color-text-primary);
  background: var(--color-bg-hover);
}

.notif-panel__danger:hover {
  color: var(--color-error) !important;
}
</style>
