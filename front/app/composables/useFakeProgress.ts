import { ref } from 'vue'

/**
 * 拟真进度组合式（loading-progress-tips change）
 *
 * 游戏加载画面式反馈：进度条 ease-out 爬升至天花板（默认 90%）后停滞等待真实完成，
 * finish() 推进到 100%。附游戏风短句（随机 + 长加载轮换）。
 *
 * 契约（openspec/specs/spa-loading-ux「初始化加载屏进度反馈」）：
 * - 爬升不得超过天花板（不在真实完成前谎报 100%）
 * - 加载持续超过 4s 时短句约 4s 轮换，且不与当前重复
 * - 卸载/失败路径 MUST dispose() 清理全部计时器
 * - 短句清单与首帧模板 spa-loading-template.html 内联清单同步维护
 *   （模板零外部依赖，不能共享模块；维护约定见 docs/reference/standard/frontend/loading-experience.md）
 */

/** 游戏风加载短句（约 12 条；改动时同步 spa-loading-template.html 内联副本） */
export const LOADING_TIPS: readonly string[] = [
  '正在唤醒语义向量…',
  '从互联网的深海捕捞新鲜文章…',
  'RSS 小火车排队进站中…',
  '正在把时间线熨得平平整整…',
  '知识正在入栏，请勿投喂…',
  '拼装万物的并置宇宙…',
  '教标签们各司其职…',
  '模型干饭中，吃得很好…',
  '抚平向量空间的邻里关系…',
  '阅览室掸灰中，灰尘略多…',
  '给每个话题安放一朵云…',
  '进度条很努力了，别催它…',
]

export interface FakeProgressOptions {
  /** 爬升天花板（%），finish 前不越过 */
  ceiling?: number
  /** 爬升 tick 间隔（ms） */
  tickMs?: number
  /** ease-out 系数：每 tick 向天花板逼近的比例 */
  ease?: number
  /** 短句轮换间隔（ms） */
  tipRotateMs?: number
  /** 最小展示时长（ms）：start 后过早 finish 时延迟收尾，避免进度条闪跳 */
  minVisibleMs?: number
}

const DEFAULTS: Required<FakeProgressOptions> = {
  ceiling: 90,
  tickMs: 120,
  ease: 0.06,
  tipRotateMs: 4000,
  minVisibleMs: 400,
}

function pickTip(exclude?: string): string {
  if (LOADING_TIPS.length < 2) return LOADING_TIPS[0] ?? ''
  let next = LOADING_TIPS[Math.floor(Math.random() * LOADING_TIPS.length)]!
  // 清单 ≥2 条时必然能抽到不同的一条（防御性上限防死循环）
  for (let i = 0; i < LOADING_TIPS.length && next === exclude; i++) {
    next = LOADING_TIPS[Math.floor(Math.random() * LOADING_TIPS.length)]!
  }
  return next
}

export function useFakeProgress(options: FakeProgressOptions = {}) {
  const opts = { ...DEFAULTS, ...options }

  const progress = ref(0)
  const finished = ref(false)
  const tip = ref('')

  let climbTimer: ReturnType<typeof setInterval> | null = null
  let tipTimer: ReturnType<typeof setInterval> | null = null
  let finishTimer: ReturnType<typeof setTimeout> | null = null
  let startedAt = 0

  function stopClimb() {
    if (climbTimer !== null) {
      clearInterval(climbTimer)
      climbTimer = null
    }
  }

  function stopTipRotate() {
    if (tipTimer !== null) {
      clearInterval(tipTimer)
      tipTimer = null
    }
  }

  function finishNow() {
    finishTimer = null
    progress.value = 100
    finished.value = true
    stopClimb()
    stopTipRotate()
  }

  /** 开始拟真爬升与短句展示（重复调用等价于重新开始） */
  function start() {
    dispose()
    progress.value = 0
    finished.value = false
    tip.value = pickTip()
    startedAt = Date.now()

    climbTimer = setInterval(() => {
      // ease-out：前快后慢，渐近天花板但按构造不可越过
      const next = progress.value + (opts.ceiling - progress.value) * opts.ease
      progress.value = Math.min(opts.ceiling, Math.round(next * 10) / 10)
    }, opts.tickMs)

    tipTimer = setInterval(() => {
      tip.value = pickTip(tip.value)
    }, opts.tipRotateMs)
  }

  /** 真实加载完成：推进到 100%（start 后 minVisibleMs 内调用则延迟收尾防闪跳） */
  function finish() {
    if (finished.value) return
    stopClimb()
    const elapsed = Date.now() - startedAt
    const wait = opts.minVisibleMs - elapsed
    if (wait > 0) {
      if (finishTimer === null) finishTimer = setTimeout(finishNow, wait)
    } else {
      finishNow()
    }
  }

  /** 清理全部计时器（error 分支 / 组件卸载 MUST 调用；幂等） */
  function dispose() {
    stopClimb()
    stopTipRotate()
    if (finishTimer !== null) {
      clearTimeout(finishTimer)
      finishTimer = null
    }
  }

  return { progress, finished, tip, start, finish, dispose }
}
