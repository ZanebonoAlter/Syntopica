import { ref } from 'vue'
import { useAIAdminApi } from '~/api/aiAdmin'
import type { DegradationEvent } from '~/types'

/**
 * 泳道降级标注聚合（tasks 8.3，FB-5「断掉了也能在这里体现」）。
 * 数据源 GET /ai/observability/degradations?window=24h（纯只读，ai_call_logs
 * 聚合，specs/ai-route-observability 口径），前端按 capability 聚合到行/卡两级：
 * - 行级：fallback 计数与全链失败 → 行头健康点三态（ok/warn/err）
 * - 卡级：fallback 事件 from 顺位 → 「24h 失败 N」红边（index 近似：事件记录的是
 *   当时链序，与当前编辑态不一致时仅在行头体现，不误导到具体卡）；to 顺位且
 *   事件 model 名与该位卡名一致 → 「接管 N」黄徽（名字校验防调序后错标）
 */
export interface CardHealth {
  /** 24h 内该顺位失败次数（fallback from 侧，index 近似） */
  fail: number
  /** 最近一次失败错误码（tooltip） */
  lastErr: string | null
  /** 24h 内接管次数（fallback to 侧，模型名校验通过才计入） */
  takeover: number
}

export interface RowHealthSummary {
  level: 'ok' | 'warn' | 'err'
  text: string
  /** 24h 降级次数（fallback 迁移事件数） */
  degradedCount: number
}

const EMPTY_CARD: CardHealth = { fail: 0, lastErr: null, takeover: 0 }

export function useRouteHealth() {
  const events = ref<DegradationEvent[]>([])
  const loaded = ref(false)
  const error = ref<string | null>(null)

  async function fetchHealth() {
    error.value = null
    try {
      const res = await useAIAdminApi().getDegradations('24h')
      if (res.success && res.data) {
        events.value = res.data.events.filter(e => e.type === 'fallback')
      } else if (!res.success) {
        error.value = res.error ?? '降级观测数据加载失败'
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : '降级观测数据加载失败'
    } finally {
      loaded.value = true
    }
  }

  function eventsFor(capability: string): DegradationEvent[] {
    return events.value.filter(e => e.capability === capability)
  }

  /**
   * 卡级健康（按当前行序 index 对齐，chainNames = 当前行卡名序列）。
   * 结构吻合校验（防换绑/调序后旧事件错标）：迁移事件要求命中模型名与 to 位卡名
   * 一致才计入两侧；全链失败事件（to=null，model=最后尝试的模型）要求该名与
   * from 位卡名一致。对不上则只在行头体现（rowHealth），不落到具体卡。
   */
  function cardHealth(capability: string, index: number, chainNames: string[]): CardHealth {
    let fail = 0
    let takeover = 0
    let lastErr: string | null = null
    for (const e of eventsFor(capability)) {
      if (e.to !== null) {
        // 迁移事件：结构吻合才标（to 位命中模型名一致）
        if (chainNames[e.to] !== e.model) continue
        if (e.from === index) {
          fail += 1
          lastErr = e.code
        }
        if (e.to === index) takeover += 1
      } else {
        // 全链失败：model=最后尝试的模型，须与 from 位卡名一致
        if (e.from === index && chainNames[e.from] === e.model) {
          fail += 1
          lastErr = e.code
        }
      }
    }
    return fail || takeover ? { fail, lastErr, takeover } : EMPTY_CARD
  }

  /** 行级健康：fail 事件有 to（被接管）→ warn；有 from 无 to（全链失败/无兜底）→ err */
  function rowHealth(capability: string, hasBackup: boolean): RowHealthSummary {
    const list = eventsFor(capability)
    const degradedCount = list.filter(e => e.from !== null && e.to !== null).length
    if (list.length === 0) return { level: 'ok', text: '正常', degradedCount: 0 }
    const rescued = list.some(e => e.to !== null)
    if (rescued && hasBackup) {
      return { level: 'warn', text: `24h 降级 ${degradedCount} 次·已兜底`, degradedCount }
    }
    return { level: 'err', text: hasBackup ? `24h 失败 ${list.length} 次` : `24h 失败 ${list.length} 次·无兜底`, degradedCount }
  }

  return { fetchHealth, cardHealth, rowHealth, loaded, error }
}
