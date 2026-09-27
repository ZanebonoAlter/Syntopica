/**
 * 路由切换加载反馈状态（fix-spa-nav-loading-ux D1）
 *
 * 两态状态机（idle 非显式状态，pending 即「计时中未到点」）：
 *   idle --begin()--> pending --(250ms 未 end)--> visible
 *   pending/visible --end()--> idle
 *
 * - begin() 幂等重置：已有计时器先清再起新计时（连续导航覆盖，
 *   新导航取代旧反馈）；visible 态下 begin() 立即回到 pending，不残留旧反馈
 * - end() 幂等：清计时器并回 idle（导航完成 afterEach / 失败 onError 均调用）
 * - 快导航（<250ms 完成）全程不显示，避免闪烁
 *
 * 状态载体 useState（Nuxt 惯例）：plugins/nav-loading.ts 写、
 * components/common/NavLoadingOverlay.vue 读，ssr:false 下等价全局单例。
 */

/** 显示延迟：导航超过该时长未完成才出现反馈（ms） */
export const NAV_LOADING_DELAY_MS = 250

export function useNavLoading() {
  const visible = useState<boolean>('nav-loading', () => false)

  let timer: ReturnType<typeof setTimeout> | null = null

  /** 导航开始：重置计时（连续导航覆盖旧计时；visible 先归位 pending） */
  function begin() {
    if (timer !== null) {
      clearTimeout(timer)
    }
    visible.value = false
    timer = setTimeout(() => {
      timer = null
      visible.value = true
    }, NAV_LOADING_DELAY_MS)
  }

  /** 导航结束（成功/失败/被取代）：清计时器回 idle（幂等） */
  function end() {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
    visible.value = false
  }

  return { visible, begin, end }
}
