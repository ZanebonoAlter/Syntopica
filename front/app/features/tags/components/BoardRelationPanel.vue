<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { BoardRelationDetail, BoardRelationEvidence, BoardRelationRow } from '~/api/boardEnrichment'

/**
 * 跨版块关系建议面板（add-evidence-backed-cross-board-relations 6.2）。
 *
 * 分区展示生命周期（proposed 待裁决 / unresolved 待重解析 / confirmed /
 * dismissed），每条关系可展开详情：目标映射、支持证据、反证、gap 与
 * run 审计。confirm / dismiss(reason 必填) / re-resolve 全部 emit 给父级
 * （useBoardRelations 接管 API），组件只管展示与输入校验。
 * 状态纪律：
 *  - 空列表 / 加载中 / 加载失败是三种不同形态，不互相冒充；
 *  - dismiss 必填理由：空/纯空白禁用提交并提示；
 *  - 超长 claim/quote 折叠 + 展开；证据外链新窗口打开。
 */
const props = defineProps<{
  relations: BoardRelationRow[]
  loading?: boolean
  error?: string | null
  detail: BoardRelationDetail | null
  detailLoading?: boolean
  /** 动作在途标记（useBoardRelations 的 confirming/dismissing/reResolving id）。 */
  confirmingId?: number | null
  dismissingId?: number | null
  reResolvingId?: number | null
  /** 正在发现关联的 source（observation/question key），用于行内提示。 */
  activeSource?: { sourceKind: string; sourceKey: string } | null
}>()

const emit = defineEmits<{
  (e: 'reload', status?: string): void
  (e: 'open-detail', relationId: number): void
  (e: 'confirm', relationId: number): void
  (e: 'dismiss', relationId: number, reason: string): void
  (e: 're-resolve', relationId: number): void
}>()

const STATUS_ORDER = ['proposed', 'unresolved', 'confirmed', 'dismissed', 'expired'] as const

const STATUS_LABELS: Record<string, string> = {
  proposed: '待裁决',
  unresolved: '未找到目标版块',
  confirmed: '已确认',
  dismissed: '已驳回',
  expired: '已过期',
}
function statusLabel(status: string): string {
  return STATUS_LABELS[status] ?? status
}

const RELATION_LABELS: Record<string, string> = {
  causal: '因果传导',
  common_driver: '共同驱动',
  divergence: '方向分化',
  correlated: '同向相关',
  contextual: '背景相关',
  unclear: '尚无法判断',
}
function relationLabel(type: string): string {
  return RELATION_LABELS[type] ?? type
}

const VERDICT_LABELS: Record<string, string> = {
  supported: '盲验支持',
  contested: '存在反证',
  insufficient: '证据不足',
  rejected: '盲验否决',
}
function verdictLabel(v: string): string {
  return VERDICT_LABELS[v] ?? v
}

/** 生命周期分组（稳定顺序；未知状态归尾部「其他」）。 */
const groups = computed(() => {
  const byStatus = new Map<string, BoardRelationRow[]>()
  for (const r of props.relations) {
    const list = byStatus.get(r.status) ?? []
    list.push(r)
    byStatus.set(r.status, list)
  }
  const known: Array<{ status: string; rows: BoardRelationRow[] }> = STATUS_ORDER.filter((s) => byStatus.has(s)).map((s) => ({ status: s, rows: byStatus.get(s)! }))
  const rest = [...byStatus.keys()].filter((s) => !(STATUS_ORDER as readonly string[]).includes(s))
  for (const s of rest) known.push({ status: s, rows: byStatus.get(s)! })
  return known
})

/** 状态过滤（local state；reload emit 携 status）。 */
const statusFilter = ref<'all' | 'proposed' | 'unresolved' | 'confirmed' | 'dismissed' | 'expired'>('all')
function applyFilter() {
  emit('reload', statusFilter.value === 'all' ? undefined : statusFilter.value)
}

/** 展开的行（详情区就地渲染；再次点击收起）。 */
const expandedId = ref<number | null>(null)
function toggleDetail(relationId: number) {
  if (expandedId.value === relationId) {
    expandedId.value = null
    return
  }
  expandedId.value = relationId
  emit('open-detail', relationId)
}

/** dismiss 理由输入（行内）：id → 文案。 */
const dismissDrafts = ref<Record<number, string>>({})
function dismissReasonOf(id: number): string {
  return dismissDrafts.value[id] ?? ''
}
function dismissSubmittable(id: number): boolean {
  return dismissReasonOf(id).trim().length > 0
}
function submitDismiss(id: number) {
  const reason = dismissReasonOf(id).trim()
  if (!reason) return
  emit('dismiss', id, reason)
}

/** 超长文本折叠（claim/quote 各自独立阈值）。 */
const CLAIM_CLAMP = 160
const QUOTE_CLAMP = 120
const expandedClaims = ref<Set<string>>(new Set())
const expandedQuotes = ref<Set<string>>(new Set())
function clamped(text: string, clamp: number): boolean {
  return text.length > clamp
}
function claimText(relationId: number, claim: string): string {
  const key = `claim-${relationId}`
  if (!clamped(claim, CLAIM_CLAMP) || expandedClaims.value.has(key)) return claim
  return claim.slice(0, CLAIM_CLAMP) + '…'
}
function toggleClaimClamp(relationId: number) {
  const key = `claim-${relationId}`
  const next = new Set(expandedClaims.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedClaims.value = next
}
function quoteText(key: string, quote: string): string {
  if (!clamped(quote, QUOTE_CLAMP) || expandedQuotes.value.has(key)) return quote
  return quote.slice(0, QUOTE_CLAMP) + '…'
}
function toggleQuoteClamp(key: string) {
  const next = new Set(expandedQuotes.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedQuotes.value = next
}
const detail = computed(() => props.detail)
function supportEvidenceOf(row: BoardRelationDetail | null): BoardRelationEvidence[] {
  return row?.evidence?.filter((e) => e.use !== 'counter') ?? []
}
function counterEvidenceOf(row: BoardRelationDetail | null): BoardRelationEvidence[] {
  const explicit = row?.counterevidence ?? []
  const inBand = row?.evidence?.filter((e) => e.use === 'counter') ?? []
  return [...explicit, ...inBand]
}
</script>

<template>
  <section class="brp" data-test="board-relation-panel">
    <header class="brp-head">
      <h3 class="serif"><Icon icon="mdi:lan-connect" width="14" /> 跨版块关系</h3>
      <div class="brp-tools">
        <select v-model="statusFilter" class="brp-filter" data-test="relation-status-filter" @change="applyFilter">
          <option value="all">全部状态</option>
          <option value="proposed">待裁决</option>
          <option value="unresolved">未找到目标</option>
          <option value="confirmed">已确认</option>
          <option value="dismissed">已驳回</option>
        </select>
        <button type="button" class="brp-reload" data-test="relation-reload" title="刷新列表" @click="emit('reload', statusFilter === 'all' ? undefined : statusFilter)">
          <Icon icon="mdi:refresh" width="13" />
        </button>
      </div>
    </header>

    <p v-if="activeSource" class="brp-running" data-test="relation-discovery-running">
      <Icon icon="mdi:progress-clock" width="13" />
      正在发现关联（{{ activeSource.sourceKind === 'question' ? '研究问题' : '观察' }} {{ activeSource.sourceKey }}）…
    </p>

    <!-- 加载态 -->
    <p v-if="loading" class="brp-hint" data-test="relation-loading">关系列表加载中…</p>
    <!-- 错误态（不冒充空态） -->
    <div v-else-if="error" class="brp-error" data-test="relation-error">
      <p>{{ error }}</p>
      <button type="button" class="brp-retry" data-test="relation-retry" @click="emit('reload')">重试</button>
    </div>
    <!-- 空态 -->
    <p v-else-if="!relations.length" class="brp-hint" data-test="relation-empty">
      暂无跨版块关系建议。可在简报的观察或研究问题上点「发现关联」，从外部证据里找目标版块。
    </p>

    <!-- 列表（按生命周期分组） -->
    <template v-else>
      <div v-for="g in groups" :key="g.status" class="brp-group" :data-test="`relation-group-${g.status}`">
        <h4 class="brp-group-title">{{ statusLabel(g.status) }}<span class="brp-count">{{ g.rows.length }}</span></h4>
        <ul class="brp-list">
          <li v-for="r in g.rows" :key="r.id" class="brp-row" :data-test="`relation-row-${r.id}`">
            <div class="brp-row-head">
              <span class="brp-type">{{ relationLabel(r.relation_type) }}</span>
              <span class="brp-verdict" :class="`brp-verdict-${r.verification_verdict}`">{{ verdictLabel(r.verification_verdict) }}</span>
              <span class="brp-target" :data-test="`relation-target-${r.id}`">
                <template v-if="r.target_board_id">
                  <Icon icon="mdi:external-link" width="11" /> 版块 #{{ r.target_board_id }}
                </template>
                <template v-else>目标未定 · {{ r.target_concept || '外部概念' }}</template>
              </span>
              <button
                type="button"
                class="brp-expand"
                :data-test="`relation-expand-${r.id}`"
                @click="toggleDetail(r.id)"
              >
                {{ expandedId === r.id ? '收起' : '详情' }}
              </button>
            </div>
            <p class="brp-claim" :data-test="`relation-claim-${r.id}`">
              {{ claimText(r.id, r.claim) }}
              <button v-if="clamped(r.claim, CLAIM_CLAMP)" type="button" class="brp-clamp-toggle" :data-test="`relation-claim-toggle-${r.id}`" @click="toggleClaimClamp(r.id)">
                {{ expandedClaims.has(`claim-${r.id}`) ? '收起' : '展开全文' }}
              </button>
            </p>
            <p v-if="r.dismiss_reason" class="brp-dismiss-reason">驳回理由：{{ r.dismiss_reason }}</p>

            <!-- 操作（按状态） -->
            <div class="brp-actions">
              <button
                v-if="r.status === 'proposed'"
                type="button"
                class="brp-btn brp-confirm"
                :disabled="confirmingId != null"
                :data-test="`relation-confirm-${r.id}`"
                @click="emit('confirm', r.id)"
              >
                {{ confirmingId === r.id ? '确认中…' : '确认' }}
              </button>
              <button
                v-if="r.status === 'proposed' || r.status === 'unresolved'"
                type="button"
                class="brp-btn"
                :disabled="dismissingId != null"
                :data-test="`relation-dismiss-${r.id}`"
                @click="dismissDrafts[r.id] = dismissDrafts[r.id] ?? ''"
              >
                驳回
              </button>
              <button
                v-if="r.status === 'unresolved'"
                type="button"
                class="brp-btn"
                :disabled="reResolvingId != null"
                :data-test="`relation-reresolve-${r.id}`"
                @click="emit('re-resolve', r.id)"
              >
                {{ reResolvingId === r.id ? '重解析中…' : '重找目标' }}
              </button>
            </div>

            <!-- 行内 dismiss 理由输入 -->
            <div v-if="dismissDrafts[r.id] !== undefined" class="brp-dismiss-box" :data-test="`relation-dismiss-box-${r.id}`">
              <input
                v-model="dismissDrafts[r.id]"
                type="text"
                class="brp-dismiss-input"
                placeholder="驳回理由（必填，进入冷却期防重现）"
                maxlength="200"
                :data-test="`relation-dismiss-input-${r.id}`"
              />
              <button
                type="button"
                class="brp-btn brp-confirm"
                :disabled="!dismissSubmittable(r.id) || dismissingId != null"
                :data-test="`relation-dismiss-submit-${r.id}`"
                @click="submitDismiss(r.id)"
              >
                {{ dismissingId === r.id ? '提交中…' : '提交驳回' }}
              </button>
              <button type="button" class="brp-btn" @click="delete dismissDrafts[r.id]">取消</button>
            </div>

            <!-- 详情（展开时） -->
            <div v-if="expandedId === r.id" class="brp-detail" :data-test="`relation-detail-${r.id}`">
              <p v-if="detailLoading" class="brp-hint">详情加载中…</p>
              <template v-else-if="detail && detail.id === r.id">
                <div v-if="detail.mechanism" class="brp-mech">
                  <span class="brp-k">传导机制</span>{{ detail.mechanism }}
                </div>
                <div class="brp-map" data-test="relation-mapping">
                  <span class="brp-k">目标映射</span>
                  <template v-if="detail.target_board_id">
                    版块 #{{ detail.target_board_id }}<template v-if="detail.target_lane_id"> · 泳道 #{{ detail.target_lane_id }}</template> · 概念「{{ detail.target_concept }}」
                  </template>
                  <template v-else>未解析（概念「{{ detail.target_concept }}」暂无匹配版块，可稍后重找目标）</template>
                </div>

                <div v-if="supportEvidenceOf(detail).length" class="brp-evi">
                  <span class="brp-k">支持证据</span>
                  <ul>
                    <li v-for="(e, i) in supportEvidenceOf(detail)" :key="`s${i}`" class="brp-evi-item">
                      <a v-if="e.url" :href="e.url" target="_blank" rel="noopener noreferrer" class="brp-evi-link">{{ e.title || e.url }}</a>
                      <span v-if="e.institution" class="brp-evi-meta">{{ e.institution }}</span>
                      <span v-if="e.date" class="brp-evi-meta">{{ e.date }}</span>
                      <p v-if="e.quote" class="brp-evi-quote">{{ quoteText(`sq-${r.id}-${i}`, e.quote) }}</p>
                    </li>
                  </ul>
                </div>
                <div v-if="counterEvidenceOf(detail).length" class="brp-evi brp-evi-counter">
                  <span class="brp-k">反证</span>
                  <ul>
                    <li v-for="(e, i) in counterEvidenceOf(detail)" :key="`c${i}`" class="brp-evi-item">
                      <a v-if="e.url" :href="e.url" target="_blank" rel="noopener noreferrer" class="brp-evi-link">{{ e.title || e.url }}</a>
                      <p v-if="e.quote" class="brp-evi-quote">{{ quoteText(`cq-${r.id}-${i}`, e.quote) }}</p>
                    </li>
                  </ul>
                </div>
                <p v-if="!supportEvidenceOf(detail).length && !counterEvidenceOf(detail).length" class="brp-hint">无证据记录。</p>

                <div v-if="detail.run" class="brp-run" data-test="relation-run">
                  <span class="brp-k">发现轨迹</span>
                  run #{{ detail.run.id }} · {{ detail.run.status }} · {{ detail.run.trigger_kind === 'auto' ? '自动' : '手动' }}触发（{{ detail.run.source_kind }}/{{ detail.run.source_key }}）<template v-if="detail.run.error"> · 错误：{{ detail.run.error }}</template>
                </div>
              </template>
              <p v-else class="brp-hint">详情暂不可用。</p>
            </div>
          </li>
        </ul>
      </div>
    </template>
  </section>
</template>

<style scoped>
.brp { display: flex; flex-direction: column; gap: 0.6rem; padding: 0.7rem 0.9rem; border: 1px solid var(--color-border-subtle); border-radius: 10px; }
.brp-head { display: flex; align-items: center; justify-content: space-between; }
.brp-head h3 { display: flex; align-items: center; gap: 0.35rem; margin: 0; font-size: 0.95rem; }
.brp-tools { display: flex; align-items: center; gap: 0.4rem; }
.brp-filter { padding: 0.15rem 0.4rem; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-bg); font-size: 0.78rem; }
.brp-reload { padding: 0.2rem 0.4rem; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-bg); cursor: pointer; }
.brp-running { display: flex; align-items: center; gap: 0.3rem; margin: 0; padding: 0.3rem 0.5rem; border-radius: 6px; background: var(--color-primary-subtle); color: var(--color-primary); font-size: 0.78rem; }
.brp-hint { margin: 0; color: var(--color-text-subtle); font-size: 0.8rem; }
.brp-error { display: flex; align-items: center; gap: 0.6rem; }
.brp-error p { margin: 0; color: var(--color-danger, #cf222e); font-size: 0.82rem; }
.brp-retry { padding: 0.15rem 0.6rem; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-bg); cursor: pointer; font-size: 0.78rem; }
.brp-group { display: flex; flex-direction: column; gap: 0.35rem; }
.brp-group-title { display: flex; align-items: center; gap: 0.4rem; margin: 0.2rem 0 0; font-size: 0.8rem; color: var(--color-text-subtle); }
.brp-count { padding: 0 0.35rem; border-radius: 8px; background: var(--color-bg-subtle); font-size: 0.72rem; }
.brp-list { display: flex; flex-direction: column; gap: 0.45rem; margin: 0; padding: 0; list-style: none; }
.brp-row { display: flex; flex-direction: column; gap: 0.3rem; padding: 0.5rem 0.65rem; border: 1px solid var(--color-border-subtle); border-radius: 8px; }
.brp-row-head { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem; font-size: 0.76rem; }
.brp-type { font-weight: 600; }
.brp-verdict { padding: 0.05rem 0.4rem; border-radius: 4px; font-size: 0.72rem; }
.brp-verdict-supported { background: rgba(46, 160, 67, 0.14); color: var(--color-success, #2ea043); }
.brp-verdict-contested { background: rgba(187, 128, 9, 0.14); color: #9e6a03; }
.brp-verdict-insufficient, .brp-verdict-rejected { background: var(--color-bg-subtle); color: var(--color-text-subtle); }
.brp-target { display: inline-flex; align-items: center; gap: 0.2rem; color: var(--color-text-subtle); }
.brp-expand { margin-left: auto; padding: 0.1rem 0.5rem; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-bg); cursor: pointer; font-size: 0.74rem; }
.brp-claim { margin: 0; font-size: 0.84rem; line-height: 1.5; }
.brp-clamp-toggle { padding: 0 0 0 0.3rem; border: none; background: none; color: var(--color-primary); font-size: 0.74rem; cursor: pointer; }
.brp-dismiss-reason { margin: 0; font-size: 0.76rem; color: var(--color-text-subtle); }
.brp-actions { display: flex; align-items: center; gap: 0.4rem; }
.brp-btn { padding: 0.2rem 0.7rem; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-bg); font-size: 0.78rem; cursor: pointer; }
.brp-btn:disabled { opacity: 0.55; cursor: not-allowed; }
.brp-confirm { border-color: var(--color-primary); color: var(--color-primary); }
.brp-confirm:not(:disabled):hover { background: var(--color-primary-subtle); }
.brp-dismiss-box { display: flex; align-items: center; gap: 0.4rem; flex-wrap: wrap; }
.brp-dismiss-input { flex: 1; min-width: 12rem; padding: 0.25rem 0.5rem; border: 1px solid var(--color-border); border-radius: 6px; font-size: 0.8rem; }
.brp-detail { display: flex; flex-direction: column; gap: 0.4rem; padding: 0.5rem 0.6rem; border-top: 1px dashed var(--color-border-subtle); font-size: 0.8rem; }
.brp-k { display: inline-block; margin-right: 0.4rem; padding: 0.02rem 0.4rem; border-radius: 4px; background: var(--color-bg-subtle); color: var(--color-text-subtle); font-size: 0.72rem; }
.brp-map, .brp-mech, .brp-run { line-height: 1.5; }
.brp-evi ul { display: flex; flex-direction: column; gap: 0.3rem; margin: 0.25rem 0 0; padding: 0 0 0 0.9rem; list-style: disc; }
.brp-evi-item { display: flex; flex-direction: column; gap: 0.15rem; }
.brp-evi-link { color: var(--color-primary); font-size: 0.8rem; text-decoration: none; }
.brp-evi-link:hover { text-decoration: underline; }
.brp-evi-meta { color: var(--color-text-subtle); font-size: 0.72rem; }
.brp-evi-quote { margin: 0; padding: 0.2rem 0.4rem; border-left: 2px solid var(--color-border); color: var(--color-text-subtle); font-size: 0.76rem; }
.brp-evi-counter .brp-k { background: rgba(187, 128, 9, 0.12); color: #9e6a03; }
</style>
