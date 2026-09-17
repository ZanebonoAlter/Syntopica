/**
 * 集中管理所有 SSE/WS 事件类型字符串常量
 * 修改后端事件名时只需改此处
 */

export const EVENT_TYPES = {
  // 标签完成
  TAG_COMPLETED: 'tag_completed',
  TAG_FAILED: 'tag_failed',

  // 自动刷新
  AUTO_REFRESH_COMPLETE: 'auto_refresh_complete',

  // Firecrawl 抓取进度
  FIRECRAWL_PROGRESS: 'firecrawl_progress',

  // 日报生成进度
  DAILY_REPORT_PROGRESS: 'daily_report_progress',
  DAILY_REPORT_DONE: 'daily_report_done',

  // 标签整理进度
  ORGANIZE_PROGRESS: 'organize_progress',

  // 标签合并扫描进度
  SCAN_PROGRESS: 'scan_progress',

  // 层级重建
  HIERARCHY_REBUILD: 'hierarchy_rebuild',

  // 统一通知中心事件（notification-center：日报终态通知推送）
  NOTIFICATION: 'notification',
} as const

export type EventType = (typeof EVENT_TYPES)[keyof typeof EVENT_TYPES]
