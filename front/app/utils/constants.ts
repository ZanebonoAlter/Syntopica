/**
 * 应用常量定义
 * 集中管理应用中的魔法数字、硬编码值等常量
 */

/**
 * 分页相关常量
 */
export const DEFAULT_PAGE_SIZE = 10
export const MAX_PAGE_SIZE = 10000
export const SIDEBAR_ARTICLE_LIMIT = 50

/**
 * 刷新相关常量
 */
export const REFRESH_POLLING_INTERVAL = 2000 // 轮询间隔（毫秒）
export const MAX_POLLING_TIME = 60000 // 最大轮询时间（毫秒）
export const AUTO_REFRESH_MINUTES = 60 // 默认自动刷新间隔（分钟）

/**
 * 侧边栏相关常量
 */
export const SIDEBAR_DEFAULT_WIDTH = 256 // 默认宽度（像素）
export const SIDEBAR_MIN_WIDTH = 200 // 最小宽度（像素）
export const SIDEBAR_MAX_WIDTH = 500 // 最大宽度（像素）
export const SIDEBAR_COLLAPSED_WIDTH = 48 // 折叠后的宽度（像素）

/**
 * AI 相关常量
 */
export const AI_GENERATION_TIMEOUT = 120000 // AI 生成超时时间（毫秒）
export const AI_SUMMARY_MAX_LENGTH = 150 // 摘要最大长度

/**
 * 时间范围选项（AI 摘要）
 */
export const TIME_RANGE_OPTIONS = [
  { label: '最近 1 小时', value: 60 },
  { label: '最近 3 小时', value: 180 },
  { label: '最近 6 小时', value: 360 },
  { label: '最近 12 小时', value: 720 },
  { label: '最近 24 小时', value: 1440 },
] as const

/**
 * 颜色选项（用于分类、订阅源等）- 杂志风格配色
 */
export const COLOR_OPTIONS = [
  '#3b6b87', // Ink Blue - 墨水蓝
  '#c12f2f', // Print Red - 印刷红
  '#2d8a7a', // Teal - 青绿
  '#d4883c', // Amber - 琥珀
  '#4a5d8a', // Indigo - 靛蓝
  '#3d7a4a', // Forest - 森林绿
  '#8a5a4a', // Sepia - 褐色
  '#5a5a5a', // Charcoal - 炭灰
] as const

/**
 * 图标选项（用于分类等）
 */
export const ICON_OPTIONS = [
  'mdi:folder',
  'mdi:code-tags',
  'mdi:newspaper',
  'mdi:palette',
  'mdi:post',
  'mdi:brain',
  'mdi:cube-outline',
  'mdi:rocket',
  'mdi:book',
  'mdi:school',
] as const

/**
 * 刷新状态类型
 */
export type RefreshStatus = 'idle' | 'refreshing' | 'success' | 'error'

/**
 * 视图模式类型
 */
export type ViewMode = 'preview' | 'iframe'

/**
 * 消息类型（用于 Toast 提示）
 */
export type MessageType = 'success' | 'error' | 'info'
