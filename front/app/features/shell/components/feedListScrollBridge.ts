import type { InjectionKey } from 'vue'

/**
 * 虚拟列表滚动容器桥（mobile-viewport-stage1 任务 3.1，design.md D4）。
 *
 * 列表/阅读单栏切换时 FeedLayoutShell 需要记忆并恢复列表 scrollTop，但
 * useVirtualList 的 containerProps.ref 归 ArticleListPanelView 所有。Shell
 * （provide 方）不重复持有 DOM 引用，由 ArticleListPanelView（inject 方）
 * 注册滚动容器；两端均为窄屏分支专属——宽屏不注册不读取（宽屏零变化红线）。
 */
export interface FeedListScrollBridge {
  /** 虚拟列表滚动容器；容器挂载/变更/卸载时由注册方更新 */
  el: HTMLElement | null
}

export const FEED_LIST_SCROLL_BRIDGE: InjectionKey<FeedListScrollBridge> = Symbol(
  'feed-list-scroll-bridge'
)
