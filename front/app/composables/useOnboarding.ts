import { driver, type DriveStep, type Driver } from 'driver.js'
import 'driver.js/dist/driver.css'
import '~/assets/css/onboarding.css'
import { computed, nextTick, ref } from 'vue'

/**
 * useOnboarding — 分步引导（driver.js）。
 *
 * 支持多个 tour preset：首页（home）与叙事工坊页（tags）。
 * 每个 tour 有独立的 localStorage 完成标记，各自首次访问自动启动。
 * 复用同一 driver.js 实例管理、prefers-reduced-motion 检测、缺失元素预检过滤。
 *
 * 当前应用 `ssr: false`（SPA），所有 `import.meta.client` 守卫均为防御性预留。
 *
 * 详见 openspec/changes/user-onboarding/design.md（D2/D3/D4/D8）。
 */

/** localStorage 完成标记前缀：每个 tour 一个独立键。 */
const STORAGE_KEY_PREFIX = 'syntopica_onboarding_'
const HOME_KEY = `${STORAGE_KEY_PREFIX}complete`
const TAGS_KEY = `${STORAGE_KEY_PREFIX}tags_complete`
const SETTINGS_KEY = `${STORAGE_KEY_PREFIX}settings_complete`
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

/** 当前激活的 tour 标识（用于 onDestroyed 回写对应标记）。 */
type TourId = 'home' | 'tags' | 'settings'
let activeTourId: TourId | null = null

/** 模块级单例：driver.js 实例 + 响应式状态，跨调用方共享。 */
let driverInstance: Driver | null = null
const tourActive = ref(false)

/**
 * Client-environment guard. Defaults to the Nuxt `import.meta.client` macro so
 * production stays SSR-safe; exposed as a mutable token purely so unit tests can
 * toggle client/SSR behavior — the macro itself is not runtime-overridable under
 * Vitest (it resolves to `undefined` there). See useOnboarding.test.ts.
 */
export const __onboardingClient = { value: import.meta.client }

/** 防御性客户端守卫：非客户端环境一律短路。 */
function isClient(): boolean {
  return __onboardingClient.value
}

/** prefers-reduced-motion 偏好（仅客户端调用）。 */
function prefersReducedMotion(): boolean {
  if (!isClient() || typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia(REDUCED_MOTION_QUERY).matches
}

/** 标记某 tour 完成（完成/跳过/点外均写入），幂等。 */
function markComplete(id: TourId): void {
  if (!isClient()) return
  const key = id === 'home' ? HOME_KEY : id === 'tags' ? TAGS_KEY : SETTINGS_KEY
  localStorage.setItem(key, 'true')
}

/**
 * 预检过滤缺失元素。
 *
 * driver.js v1 不会自动跳过 `element` 选择器查不到的步骤（见 design.md D2），
 * 这里在 `await nextTick()` 之后、传给 driver() 之前，剔除查不到的步骤。
 * 无 `element`（欢迎弹窗）的步骤始终保留。
 */
function preFilterSteps(steps: DriveStep[]): DriveStep[] {
  return steps.filter((step) => {
    if (!step.element || typeof step.element !== 'string') return true
    if (!isClient()) return true
    return document.querySelector(step.element) !== null
  })
}

/** 通用启动逻辑：预检 + 创建 driver 实例 + 绑定结束钩子。 */
async function runTour(id: TourId, steps: DriveStep[]): Promise<void> {
  if (!isClient()) return
  if (driverInstance) return // 已有实例在跑，忽略重复启动

  await nextTick()
  const filtered = preFilterSteps(steps)
  if (filtered.length === 0) {
    markComplete(id)
    return
  }

  activeTourId = id
  tourActive.value = true
  driverInstance = driver({
    steps: filtered,
    animate: !prefersReducedMotion(),
    showProgress: true,
    allowClose: true,
    overlayClickBehavior: 'close',
    progressText: '步骤 {{current}} / {{total}}',
    nextBtnText: '下一步',
    prevBtnText: '上一步',
    doneBtnText: '完成',
    onDestroyed: () => {
      // 覆盖所有结束路径：完成最后一步 / 点 Skip / 点遮罩外
      if (activeTourId) markComplete(activeTourId)
      activeTourId = null
      driverInstance = null
      tourActive.value = false
    },
  })
  driverInstance.drive()
}

/* ------------------------------------------------------------------ *
 * Tour presets
 * ------------------------------------------------------------------ */

/** 首页引导步骤（见 design.md D2）。 */
const HOME_STEPS: DriveStep[] = [
  {
    popover: {
      title: '欢迎使用 Syntopica',
      description: '一个信息聚合 + AI 标签系统。接下来用几步带你了解核心入口。',
      side: 'over',
    },
  },
  { element: '[data-onboarding="sidebar-feeds"]', popover: { title: '订阅源与分类', description: '在这里添加和管理你的 RSS 源与分类，文章会自动抓取。' } },
  { element: '[data-onboarding="nav-tags"]', popover: { title: '叙事工坊', description: '查看和管理 AI 为文章自动生成的标签与语义版块。' } },
  { element: '[data-onboarding="watched-tags"]', popover: { title: '关注标签', description: '关注感兴趣的标签，获取个性化文章推送。' } },
]

/** 叙事工坊页引导步骤（见 design.md D8）。 */
const TAGS_STEPS: DriveStep[] = [
  {
    popover: {
      title: '叙事工坊',
      description: '把 AI 自动生成的标签组织成可阅读的板块。接下来介绍核心流转。',
      side: 'over',
    },
  },
  { element: '[data-onboarding="tags-board-list"]', popover: { title: '语义板块列表', description: '这里是你创建的语义板块。选中某个板块后，右侧会展示它的内容构成、日报与文章。' } },
  { element: '[data-onboarding="tags-content-tabs"]', popover: { title: '三个视图', description: '板块内容（标签构成）、日报（按日自动聚合热点）、文章（按标签筛选浏览）。' } },
  { element: '[data-onboarding="tags-board-actions"]', popover: { title: '核心操作', description: '升级建议把零散标签聚合成板块、匹配回填补全历史文章、标签合并去重、匹配参数微调规则。' } },
  { element: '[data-onboarding="tags-add-board"]', popover: { title: '开始创建', description: '从零手动创建第一个语义板块，或用「升级建议」让系统自动建议。' } },
]

/** 设置页引导步骤（见 design.md D9）。 */
const SETTINGS_STEPS: DriveStep[] = [
  {
    popover: {
      title: '设置中心',
      description: '配置 Syntopica 的数据源、AI 能力与运行参数。接下来按配置主链路介绍。',
      side: 'over',
    },
  },
  { element: '[data-onboarding="settings-nav"]', popover: { title: '分组导航', description: '左侧导航按四组组织：内容管理、AI 配置、数据源与网络、运行状态。点击组内项切换。' } },
  { element: '[data-onboarding="settings-nav-feeds"]', popover: { title: '订阅源', description: '管理 RSS 源的刷新频率、抓取深度与标签规则，是数据的入口。' } },
  { element: '[data-onboarding="settings-nav-ai-providers"]', popover: { title: 'AI 模型', description: '配置主/备模型提供商；能力路由决定哪个能力（打标、总结、整理稿）走哪个模型。' } },
  { element: '[data-onboarding="settings-nav-runtime-status"]', popover: { title: '运行状态', description: 'AI 健康、队列与定时任务（含日报生成时刻）合并为一个分区，内部子 tab 切换。' } },
]

/* ------------------------------------------------------------------ *
 * Public API
 * ------------------------------------------------------------------ */

/** 启动首页引导教程。 */
async function startTour(): Promise<void> {
  await runTour('home', HOME_STEPS)
}

/** 启动叙事工坊页引导教程。 */
async function startTagsTour(): Promise<void> {
  await runTour('tags', TAGS_STEPS)
}

/** 启动设置页引导教程。 */
async function startSettingsTour(): Promise<void> {
  await runTour('settings', SETTINGS_STEPS)
}

/** 主动结束并标记当前 tour 完成（可用于程序化跳过）。 */
function dismissTour(): void {
  if (!isClient()) return
  if (activeTourId) markComplete(activeTourId)
  if (driverInstance) {
    driverInstance.destroy()
    driverInstance = null
  }
  activeTourId = null
  tourActive.value = false
}

/** 清除首页完成标记并刷新页面，使首次访问逻辑重新触发引导。 */
function resetOnboarding(): void {
  if (!isClient()) return
  localStorage.removeItem(HOME_KEY)
  location.reload()
}

/** 读取某 tour 是否已完成。 */
function isTourComplete(id: TourId): boolean {
  if (!isClient()) return false
  const key = id === 'home' ? HOME_KEY : id === 'tags' ? TAGS_KEY : SETTINGS_KEY
  return localStorage.getItem(key) === 'true'
}

export function useOnboarding() {
  return {
    // 首页 tour
    isFirstRun: computed(() => {
      if (!isClient()) return false
      return isTourComplete('home') === false
    }),
    isTourActive: computed(() => tourActive.value),
    startTour,
    dismissTour,
    resetOnboarding,
    // 叙事工坊 tour
    isTagsFirstRun: computed(() => {
      if (!isClient()) return false
      return isTourComplete('tags') === false
    }),
    startTagsTour,
    // 设置页 tour
    isSettingsFirstRun: computed(() => {
      if (!isClient()) return false
      return isTourComplete('settings') === false
    }),
    startSettingsTour,
  }
}
