import { ref } from 'vue'
import { useSchedulerApi } from '~/api'
import type { SchedulerAIHealthRoute, SchedulerStatus, SchedulerTriggerResult } from '~/types/scheduler'
import { usePollBundle } from '~/composables/usePollBundle'

export function useSchedulerStatus() {
  // 调度器状态列表：由 usePollBundle 单例对账 /api/poll 分发（client-poll-budget），
  // useState 跨组件共享；本 composable 不再持有独立轮询 timer。
  const schedulerStatuses = useState<SchedulerStatus[]>('poll:schedulers', () => [])
  // 分析暂停总闸是后端全局开关，用 useState 跨组件共享（同 useNotify 模式，SSR 安全）
  const analysisPaused = useState<boolean>('scheduler:analysis-paused', () => false)
  const analysisPausedAt = useState<string>('scheduler:analysis-paused-at', () => '')
  // AI 模型健康态（宽松判定，后端启动探活后才有真实值）。默认 true 避免首屏轮询前误弹健康 banner；
  // 轮询拿到 false 才弹。只用于提示，不影响暂停按钮/favicon（按钮跟 analysisPaused 用户意图）。
  const aiHealthy = useState<boolean>('scheduler:ai-healthy', () => true)
  const aiHealthRoutes = useState<SchedulerAIHealthRoute[]>('scheduler:ai-health-routes', () => [])
  const schedulerTriggerFeedback = ref<Record<string, SchedulerTriggerResult | undefined>>({})
  const lastSchedulerTriggerAt = ref<number | null>(null)
  const schedulerLoading = ref(false)
  const schedulerTriggerLoading = ref(false)
  const scheduleTimeLoading = ref(false)
  const schedulerError = ref<string | null>(null)
  const schedulerSuccess = ref<string | null>(null)
  // 近期反馈信号：触发/更新后写入，usePollBundle 据此进入 ≥15s 档（≤20s 窗口）
  const lastFeedbackAt = useState<number>('poll:last-feedback', () => 0)

  /**
   * 立即对账一次（原独立轮询入口，保留函数名兼容消费点）：
   * 数据源已合并进 /api/poll，这里转调 usePollBundle 即时对账并重新排程。
   * 失败静默（保留旧值），不再写 schedulerError —— spec：单次失败 MUST NOT
   * 把状态指示转错误态。
   */
  async function loadSchedulersStatus() {
    const poll = usePollBundle()
    schedulerLoading.value = true
    try {
      await poll.reconcileNow()
    } finally {
      schedulerLoading.value = false
    }
  }

  async function triggerScheduler(name: string) {
    schedulerTriggerLoading.value = true
    schedulerError.value = null
    schedulerSuccess.value = null
    try {
      const { triggerScheduler: trigger } = useSchedulerApi()
      const response = await trigger(name)
      if (response.success) {
        schedulerTriggerFeedback.value[name] = response.data
        lastSchedulerTriggerAt.value = Date.now()
        lastFeedbackAt.value = Date.now()
        schedulerSuccess.value = response.data?.message || response.message || '任务请求已处理'
        setTimeout(() => { schedulerSuccess.value = null }, 2000)
        await loadSchedulersStatus()
      } else {
        schedulerTriggerFeedback.value[name] = response.data ?? {
          name, accepted: false, started: false, reason: 'request_rejected', message: '请求被拒绝',
        }
        lastSchedulerTriggerAt.value = Date.now()
        lastFeedbackAt.value = Date.now()
        schedulerError.value = response.error || '触发失败'
      }
    } catch {
      schedulerTriggerFeedback.value[name] = {
        name, accepted: false, started: false, reason: 'request_failed', message: '请求失败',
      }
      lastSchedulerTriggerAt.value = Date.now()
      lastFeedbackAt.value = Date.now()
      schedulerError.value = '触发失败'
    } finally {
      schedulerTriggerLoading.value = false
    }
  }

  async function updateScheduleTime(name: string, time: string) {
    scheduleTimeLoading.value = true
    schedulerError.value = null
    schedulerSuccess.value = null
    try {
      const { updateScheduleTime: update } = useSchedulerApi()
      const response = await update(name, time)
      if (response.success) {
        schedulerSuccess.value = response.message || '定时时间已更新'
        setTimeout(() => { schedulerSuccess.value = null }, 2000)
        await loadSchedulersStatus()
        return true
      }
      schedulerError.value = response.error || '更新定时时间失败'
      return false
    } catch {
      schedulerError.value = '更新定时时间失败'
      return false
    } finally {
      scheduleTimeLoading.value = false
    }
  }

  async function setAnalysisPaused(paused: boolean): Promise<{ ok: boolean; message: string }> {
    try {
      const { setAnalysisPause } = useSchedulerApi()
      const response = await setAnalysisPause(paused)
      if (response.success && response.data) {
        analysisPaused.value = response.data.paused
        analysisPausedAt.value = response.data.paused_at ?? ''
        return { ok: true, message: response.message || (paused ? '分析已暂停' : '分析已恢复') }
      }
      return { ok: false, message: response.error || '操作失败' }
    } catch {
      return { ok: false, message: '操作失败' }
    }
  }

  return {
    schedulerStatuses, schedulerTriggerFeedback, schedulerLoading,
    schedulerTriggerLoading, schedulerError, schedulerSuccess,
    analysisPaused, analysisPausedAt, setAnalysisPaused,
    aiHealthy, aiHealthRoutes,
    loadSchedulersStatus, triggerScheduler,
    scheduleTimeLoading, updateScheduleTime,
  }
}
