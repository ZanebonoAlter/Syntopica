<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type { BoardAnalysisResultRow, BoardBriefSectors } from '~/api/boardEnrichment'

/**
 * 版块简报主视图（board-level-deep-analysis 5.4 / D3 schema）。
 *
 * 呈现 result_kind=board_brief 的 sectors：summary / observations /
 * relationships / uncertainties / research_questions(0-4) / lane_refs。
 * 设计纪律：
 *  - 无关系、0 问题都是正常态，用直白文案，不渲染成错误；
 *  - 关系/不确定项克制展示（类型 + 置信 + 依据引用），同期发生≠因果；
 *  - lane 引用是下钻入口，emit {laneId, prefill}（prefill = 具体
 *    观察/关系说明/问题，父级切聚焦分析并预填，5.8 后可编辑）；
 *  - 「深入调查」只 emit investigate payload（generated 带 question_id /
 *    custom 只带 question 文本），绝不自动触发调查 API；
 *  - 自填问题 trim + 空白禁用 + ≤2000 rune。
 */
const props = defineProps<{
  result: BoardAnalysisResultRow | null
  loading?: boolean
  /** 调查任务在跑（board_investigation job 运行中）：禁用问题按钮/自填提交并显示「正在调查」。 */
  investigationRunning?: boolean
  /** 关系发现任务在跑：禁用全部「发现关联」按钮并提示（add-evidence-backed-cross-board-relations 6.2）。 */
  relationDiscoveryRunning?: boolean
}>()

const emit = defineEmits<{
  /** lane 引用点击：laneId + 预填问题（当前观察/关系说明/研究问题文本）。 */
  (e: 'drill-lane', payload: { laneId: number; prefill: string }): void
  /** 深入调查请求：generated 候选带 question_id（文本以父简报为准）；custom 只带 question。 */
  (e: 'investigate', payload: { briefing_result_id: number; question: string; question_id?: string }): void
  /** 跨版块关系点击另一版块：切换选中版块（add-evidence-backed-cross-board-relations 5.3）。 */
  (e: 'open-board', boardId: number): void
  /** 发现关联请求：observation/question 行内动作（source 文本不出客户端，父级只传 key）。 */
  (e: 'discover-relation', payload: { briefing_result_id: number; source_kind: 'observation' | 'question'; source_key: string }): void
}>()

const brief = computed<BoardBriefSectors | null>(() => {
  const r = props.result
  if (!r || !r.sectors) return null
  const kind = r.result_kind ?? (r.sectors as { result_kind?: string }).result_kind
  if (kind !== 'board_brief') return null
  return r.sectors as BoardBriefSectors
})

const observations = computed(() => brief.value?.observations ?? [])
const relationships = computed(() => brief.value?.relationships ?? [])
const uncertainties = computed(() => brief.value?.uncertainties ?? [])
const questions = computed(() => brief.value?.research_questions ?? [])
const laneRefs = computed(() => brief.value?.lane_refs ?? [])
/** 已确认跨版块关系（旧简报缺失字段时降级为空，不渲染分区）。 */
const crossRelations = computed(() => brief.value?.cross_board_relations ?? [])

/** 关系类型 → 人话标签（enum 见 BoardRelationType）。 */
const RELATION_LABELS: Record<string, string> = {
  common_driver: '可能共同驱动',
  possible_causal: '可能因果传导',
  divergent: '方向分化',
  context_only: '仅同板块背景相关',
  unclear: '尚无法判断',
}
function relationLabel(type: string): string {
  return RELATION_LABELS[type] ?? type
}
function confidenceLabel(conf: string): string {
  if (conf === 'high') return '高'
  if (conf === 'medium') return '中'
  if (conf === 'low') return '低'
  return conf
}
function confidenceTone(conf: string): string {
  if (conf === 'high') return 'conf-high'
  if (conf === 'medium') return 'conf-medium'
  return 'conf-low'
}

/** 跨版块关系类型 → 人话（后端枚举：causal/common_driver/divergence/correlated/contextual/unclear）。 */
const CROSS_RELATION_LABELS: Record<string, string> = {
  causal: '因果传导',
  common_driver: '共同驱动',
  divergence: '方向分化',
  correlated: '同向相关',
  contextual: '背景相关',
  unclear: '尚无法判断',
}
function crossRelationLabel(type: string): string {
  return CROSS_RELATION_LABELS[type] ?? type
}
function qualityLabel(grade: string): string {
  if (grade === 'high') return '证据充分'
  if (grade === 'medium') return '证据中等'
  if (grade === 'low') return '证据有限'
  return '未分级'
}
function directionLabel(direction: string): string {
  if (direction === 'outgoing') return '传出'
  if (direction === 'incoming') return '传入'
  return direction
}

/** 超长 claim 折叠：默认 160 字，展开后全文。 */
const CLAIM_CLAMP = 160
const expandedClaims = ref<Set<number>>(new Set())
function claimClamped(claim: string): boolean {
  return claim.length > CLAIM_CLAMP
}
function claimDisplay(claim: string, relationId: number): string {
  if (!claimClamped(claim) || expandedClaims.value.has(relationId)) return claim
  return claim.slice(0, CLAIM_CLAMP) + '…'
}
function toggleClaim(relationId: number) {
  const next = new Set(expandedClaims.value)
  if (next.has(relationId)) next.delete(relationId)
  else next.add(relationId)
  expandedClaims.value = next
}

function openBoard(boardId: number) {
  emit('open-board', boardId)
}

function discoverFromObservation(obsId: string) {
  if (!props.result || props.relationDiscoveryRunning) return
  emit('discover-relation', { briefing_result_id: props.result.id, source_kind: 'observation', source_key: obsId })
}

function discoverFromQuestion(questionId: string) {
  if (!props.result || props.relationDiscoveryRunning) return
  emit('discover-relation', { briefing_result_id: props.result.id, source_kind: 'question', source_key: questionId })
}

function drillLane(laneId: number, prefill: string) {
  emit('drill-lane', { laneId, prefill })
}

function investigateGenerated(questionId: string, question: string) {
  if (!props.result) return
  emit('investigate', {
    briefing_result_id: props.result.id,
    question_id: questionId,
    question,
  })
}

// ── 自填问题（custom）：trim + 空白禁用 + ≤2000 rune ────────────────────
const CUSTOM_MAX_RUNES = 2000
const customQuestion = ref('')
const customTrimmed = computed(() => customQuestion.value.trim())
const customRunes = computed(() => Array.from(customTrimmed.value).length)
const customTooLong = computed(() => customRunes.value > CUSTOM_MAX_RUNES)
const customValid = computed(
  () => customTrimmed.value.length > 0 && !customTooLong.value,
)
function investigateCustom() {
  if (!props.result || !customValid.value) return
  emit('investigate', {
    briefing_result_id: props.result.id,
    question: customTrimmed.value,
  })
  customQuestion.value = ''
}

function fmtDate(d?: string) {
  return d ? d.slice(0, 10) : '—'
}
</script>

<template>
  <article v-if="loading" class="bb-report loading">
    <Icon icon="mdi:loading" width="18" class="spin" /> 正在装配版块简报…
  </article>

  <article v-else-if="!result || !brief" class="bb-report empty">
    <Icon icon="mdi:file-document-outline" width="22" />
    <p>该板块还没有简报。点上方「生成简报」生成第一份：先自动补齐新闻背景档案，再汇总关键观察、泳道关系与不确定项。</p>
  </article>

  <article v-else class="bb-report">
    <!-- ── 刊头：概览（1-3 句人话） ─────────────────────────────────── -->
    <header class="bb-masthead">
      <div class="bb-eyebrow">
        版块简报
        <span v-if="result.created_at" class="bb-date">{{ fmtDate(result.created_at) }}</span>
        <span v-if="brief.all_sparse" class="bb-flag">素材不足</span>
        <span v-else-if="brief.degraded" class="bb-flag bb-flag-warn">降级生成</span>
      </div>
      <p class="bb-summary serif">{{ brief.summary || '（本轮未生成概览）' }}</p>
      <p v-if="brief.all_sparse" class="bb-sparse-note">
        各活跃泳道近期素材都太稀薄，本轮没有可观察事实。先让泳道多积累新闻（或手动补齐周期汇总）再生成简报。
      </p>
      <p v-else-if="brief.degraded && brief.degraded_why" class="bb-sparse-note">
        {{ brief.degraded_why }}
      </p>
    </header>

    <!-- ── 关键观察（内部新闻记忆读数，非外部核查事实） ──────────────── -->
    <section class="bb-section">
      <h2 class="serif">关键观察<span class="bb-count">{{ observations.length }}</span></h2>
      <p v-if="!observations.length" class="bb-plain">当前没有可观察的事实变化。</p>
      <ul v-else class="bb-obs-list">
        <li v-for="obs in observations" :key="obs.id" class="bb-obs">
          <div class="bb-obs-main">
            <span class="bb-obs-id">{{ obs.id }}</span>
            <span class="bb-obs-statement">{{ obs.statement }}</span>
          </div>
          <div class="bb-obs-meta">
            <span v-if="obs.basis">{{ obs.basis }}</span>
            <span v-if="obs.as_of_date">· 截止 {{ fmtDate(obs.as_of_date) }}</span>
            <button
              type="button"
              class="bb-lane-chip"
              :data-test="`obs-lane-${obs.lane_id}`"
              title="下钻该泳道聚焦分析（预填此观察为可修改的问题）"
              @click="drillLane(obs.lane_id, obs.statement)"
            >
              <Icon icon="mdi:transit-connection-variant" width="12" />
              泳道 #{{ obs.lane_id }}
            </button>
            <button
              type="button"
              class="bb-discover-chip"
              :data-test="`obs-discover-${obs.id}`"
              :disabled="relationDiscoveryRunning"
              :title="relationDiscoveryRunning ? '关系发现在后台运行中' : '从这条观察出发，用外部证据寻找关联的目标版块'"
              @click="discoverFromObservation(obs.id)"
            >
              <Icon icon="mdi:lan-connect" width="12" />
              发现关联
            </button>
          </div>
        </li>
      </ul>
    </section>

    <!-- ── 跨泳道关系（克制：类型 + 置信 + 依据引用） ────────────────── -->
    <section class="bb-section">
      <h2 class="serif">跨泳道关系<span class="bb-count">{{ relationships.length }}</span></h2>
      <p v-if="!relationships.length" class="bb-plain">当前未发现需要合并解释的关系——各泳道的变化可以分别理解，不必硬凑统一命题。</p>
      <ul v-else class="bb-rel-list">
        <li v-for="(rel, i) in relationships" :key="i" class="bb-rel">
          <div class="bb-rel-head">
            <span class="bb-rel-type" :class="`bb-rel-${rel.type}`">{{ relationLabel(rel.type) }}</span>
            <span class="bb-conf" :class="confidenceTone(rel.confidence)">置信 · {{ confidenceLabel(rel.confidence) }}</span>
            <span class="bb-rel-lanes">
              <button
                v-for="lid in rel.lane_ids"
                :key="lid"
                type="button"
                class="bb-lane-chip"
                :data-test="`rel-lane-${lid}`"
                title="下钻该泳道聚焦分析（预填此关系说明）"
                @click="drillLane(lid, rel.explanation)"
              >
                <Icon icon="mdi:transit-connection-variant" width="12" />
                #{{ lid }}
              </button>
            </span>
          </div>
          <p class="bb-rel-explain">{{ rel.explanation }}</p>
          <p v-if="rel.evidence_refs?.length" class="bb-rel-refs">
            依据：<span v-for="(r, j) in rel.evidence_refs" :key="j" class="bb-ref-tag">{{ r }}</span>
          </p>
        </li>
      </ul>
    </section>

    <!-- ── 已确认跨版块关系（服务端机械装配，非本期事实） ──────────────── -->
    <section v-if="crossRelations.length" class="bb-section" data-test="cross-relations-section">
      <h2 class="serif"><Icon icon="mdi:lan-connect" width="14" /> 已确认跨版块关系<span class="bb-count">{{ crossRelations.length }}</span></h2>
      <p class="bb-cross-note">以下为此前人工确认、仍在有效期内的跨版块关系（非本期态势卡事实），供背景参考。</p>
      <ul class="bb-cross-list">
        <li v-for="cr in crossRelations" :key="cr.relation_id" class="bb-cross" :data-test="`cross-relation-${cr.relation_id}`">
          <div class="bb-cross-head">
            <span class="bb-cross-dir" :data-test="`cross-dir-${cr.relation_id}`">{{ directionLabel(cr.direction) }}</span>
            <span class="bb-cross-type">{{ crossRelationLabel(cr.relation_type) }}</span>
            <span class="bb-cross-quality" :class="`bb-q-${cr.quality_grade || 'none'}`">{{ qualityLabel(cr.quality_grade) }}</span>
            <span v-if="cr.confirmed_at" class="bb-cross-date">确认于 {{ cr.confirmed_at }}</span>
            <span v-if="cr.expires_at" class="bb-cross-date">有效期至 {{ cr.expires_at }}</span>
          </div>
          <p class="bb-cross-claim" :data-test="`cross-claim-${cr.relation_id}`">{{ claimDisplay(cr.claim, cr.relation_id) }}</p>
          <button
            v-if="claimClamped(cr.claim)"
            type="button"
            class="bb-cross-toggle"
            :data-test="`cross-toggle-${cr.relation_id}`"
            @click="toggleClaim(cr.relation_id)"
          >
            {{ expandedClaims.has(cr.relation_id) ? '收起' : '展开全文' }}
          </button>
          <p v-if="cr.evidence_quote" class="bb-cross-quote">原文摘录：{{ cr.evidence_quote }}</p>
          <div class="bb-cross-actions">
            <a
              v-if="cr.evidence_url"
              :href="cr.evidence_url"
              target="_blank"
              rel="noopener noreferrer"
              class="bb-cross-evidence"
              :data-test="`cross-evidence-${cr.relation_id}`"
            >
              <Icon icon="mdi:open-in-new" width="12" />
              查看外部证据
            </a>
            <button
              type="button"
              class="bb-cross-board"
              :data-test="`cross-open-board-${cr.other_board_id}`"
              title="切换到对方版块查看"
              @click="openBoard(cr.other_board_id)"
            >
              <Icon icon="mdi:external-link" width="12" />
              版块 #{{ cr.other_board_id }}
            </button>
          </div>
        </li>
      </ul>
    </section>

    <!-- ── 不确定项（还不知道什么） ─────────────────────────────────── -->
    <section class="bb-section">
      <h2 class="serif"><Icon icon="mdi:help-circle" width="14" /> 不确定项<span class="bb-count">{{ uncertainties.length }}</span></h2>
      <p v-if="!uncertainties.length" class="bb-plain">当前没有特别标注的不确定项。</p>
      <ul v-else class="bb-unc-list">
        <li v-for="(u, i) in uncertainties" :key="i" class="bb-unc">
          <div class="bb-unc-q">{{ u.question }}</div>
          <p v-if="u.why_uncertain" class="bb-unc-why">为什么不确定：{{ u.why_uncertain }}</p>
          <p v-if="u.needed_evidence" class="bb-unc-need"><Icon icon="mdi:target" width="12" /> 需要的证据：{{ u.needed_evidence }}</p>
        </li>
      </ul>
    </section>

    <!-- ── 值得调查的问题（0-4，空是正常态） + 深入调查入口 ──────────── -->
    <section class="bb-section bb-questions">
      <h2 class="serif"><Icon icon="mdi:comment-question-outline" width="14" /> 值得调查的问题<span class="bb-count">{{ questions.length }}/4</span>
        <span v-if="investigationRunning" class="bb-running-tag">
          <Icon icon="mdi:loading" width="12" class="spin" /> 正在调查…完成后可再发起
        </span>
      </h2>
      <p v-if="!questions.length" class="bb-plain">当前没有值得展开调查的问题——普通、彼此独立的变化不需要强行调查。</p>
      <ul v-else class="bb-q-list">
        <li v-for="q in questions" :key="q.id" class="bb-q">
          <div class="bb-q-main">
            <span class="bb-q-id">{{ q.id }}</span>
            <span class="bb-q-text">{{ q.question }}</span>
          </div>
          <p v-if="q.rationale" class="bb-q-rationale">为什么值得查：{{ q.rationale }}</p>
          <div class="bb-q-foot">
            <span class="bb-rel-lanes">
              <button
                v-for="lid in q.related_lane_ids"
                :key="lid"
                type="button"
                class="bb-lane-chip"
                :data-test="`q-lane-${lid}`"
                title="下钻该泳道聚焦分析（预填此问题）"
                @click="drillLane(lid, q.question)"
              >
                <Icon icon="mdi:transit-connection-variant" width="12" />
                #{{ lid }}
              </button>
            </span>
            <button
              type="button"
              class="bb-discover-chip"
              :data-test="`q-discover-${q.id}`"
              :disabled="relationDiscoveryRunning"
              :title="relationDiscoveryRunning ? '关系发现在后台运行中' : '从这个问题出发，用外部证据寻找关联的目标版块'"
              @click="discoverFromQuestion(q.id)"
            >
              <Icon icon="mdi:lan-connect" width="12" />
              发现关联
            </button>
            <button
              type="button"
              class="bb-investigate-btn"
              data-test="q-investigate"
              :disabled="investigationRunning"
              @click="investigateGenerated(q.id, q.question)"
            >
              <Icon icon="mdi:play" width="12" />
              深入调查
            </button>
          </div>
        </li>
      </ul>

      <!-- 自填问题：与候选等价的调查入口，绝不自动触发 -->
      <div class="bb-custom">
        <label class="bb-custom-label" for="bb-custom-question">或者自己提一个问题</label>
        <textarea
          id="bb-custom-question"
          v-model="customQuestion"
          class="bb-custom-input"
          rows="2"
          placeholder="例：这两条泳道的变化是不是同一个原因驱动的？"
          :aria-invalid="customTooLong"
        />
        <div class="bb-custom-foot">
          <span v-if="customTrimmed" class="bb-rune-count" :class="{ over: customTooLong }">
            {{ customRunes }}/{{ CUSTOM_MAX_RUNES }}
          </span>
          <button
            type="button"
            class="bb-investigate-btn"
            data-test="custom-investigate"
            :disabled="!customValid || investigationRunning"
            @click="investigateCustom"
          >
            <Icon icon="mdi:play" width="12" />
            深入调查
          </button>
        </div>
      </div>
    </section>

    <!-- ── 泳道引用清单（下钻入口） ──────────────────────────────────── -->
    <section v-if="laneRefs.length" class="bb-section">
      <h2 class="serif">泳道引用（{{ laneRefs.length }} 条）</h2>
      <div class="bb-lane-list">
        <button
          v-for="lr in laneRefs"
          :key="lr.lane_id"
          type="button"
          class="bb-lane-chip"
          :data-test="`lane-ref-${lr.lane_id}`"
          title="下钻该泳道聚焦分析"
          @click="drillLane(lr.lane_id, lr.note || '')"
        >
          <Icon icon="mdi:transit-connection-variant" width="13" />
          泳道 #{{ lr.lane_id }}
          <span v-if="lr.note" class="bb-lane-note">{{ lr.note }}</span>
        </button>
      </div>
    </section>

    <footer v-if="result.session_id" class="bb-footer">
      <span class="muted">session {{ result.session_id }}</span>
    </footer>
  </article>
</template>

<style scoped>
.bb-report {
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
}
.bb-report.loading,
.bb-report.empty {
  align-items: center;
  gap: 0.6rem;
  padding: 2.2rem 1rem;
  color: var(--color-text-muted);
  border: 1px dashed var(--color-border-medium);
  border-radius: 8px;
  font-size: 0.85rem;
  text-align: center;
}
.spin { animation: bb-spin 1s linear infinite; }
@keyframes bb-spin { to { transform: rotate(360deg); } }

/* 刊头：eyebrow + 概览 */
.bb-masthead { display: flex; flex-direction: column; gap: 0.4rem; border-bottom: 2px solid var(--color-text-primary); padding-bottom: 0.8rem; }
.bb-eyebrow {
  font-size: 0.72rem; letter-spacing: 0.14em; text-transform: uppercase;
  color: var(--color-accent); font-weight: 600;
  display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;
}
.bb-date { letter-spacing: 0.04em; text-transform: none; color: var(--color-text-muted); font-weight: 400; }
.bb-flag {
  font-size: 0.66rem; padding: 0.05rem 0.45rem; border-radius: 99px;
  background: var(--color-bg-sunken); color: var(--color-text-muted);
  text-transform: none; letter-spacing: 0.02em;
}
.bb-flag-warn { background: var(--color-warning-subtle); color: var(--color-warning); }
.bb-summary { font-size: 1.02rem; line-height: 1.7; margin: 0; color: var(--color-text-primary); }
.bb-sparse-note {
  margin: 0; font-size: 0.78rem; line-height: 1.6; color: var(--color-text-secondary);
  padding: 0.5rem 0.7rem; border-radius: 8px;
  background: var(--color-warning-subtle);
}

/* 区段 */
.bb-section { display: flex; flex-direction: column; gap: 0.5rem; }
.bb-section h2 {
  margin: 0; font-size: 0.98rem; display: flex; align-items: center; gap: 0.4rem;
}
.bb-count {
  font-size: 0.68rem; color: var(--color-text-muted); font-weight: 500;
  padding: 0.02rem 0.45rem; border-radius: 99px; background: var(--color-bg-sunken);
}
.bb-plain {
  margin: 0; font-size: 0.85rem; color: var(--color-text-muted); line-height: 1.6;
  padding: 0.45rem 0.7rem; background: var(--color-bg-sunken); border-radius: 8px;
}

/* 关键观察 */
.bb-obs-list, .bb-rel-list, .bb-unc-list, .bb-q-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.55rem; }
.bb-obs { display: flex; flex-direction: column; gap: 0.2rem; padding-left: 0.7rem; border-left: 2px solid var(--color-border-subtle); }
.bb-obs-main { display: flex; align-items: baseline; gap: 0.45rem; }
.bb-obs-id { font-size: 0.68rem; color: var(--color-text-muted); flex-shrink: 0; font-family: ui-monospace, Menlo, monospace; }
.bb-obs-statement { font-size: 0.92rem; line-height: 1.6; color: var(--color-text-primary); }
.bb-obs-meta { display: flex; align-items: center; gap: 0.4rem; flex-wrap: wrap; font-size: 0.75rem; color: var(--color-text-muted); }

/* 关系 */
/* ── 已确认跨版块关系 ── */
.bb-cross-note { margin: 0; font-size: 0.78rem; color: var(--color-text-subtle); }
.bb-cross-list { display: flex; flex-direction: column; gap: 0.5rem; margin: 0; padding: 0; list-style: none; }
.bb-cross { display: flex; flex-direction: column; gap: 0.3rem; padding: 0.55rem 0.7rem; border: 1px dashed var(--color-border-subtle); border-radius: 8px; }
.bb-cross-head { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem; font-size: 0.75rem; }
.bb-cross-dir { padding: 0.05rem 0.4rem; border-radius: 4px; background: var(--color-primary-subtle); color: var(--color-primary); font-weight: 600; }
.bb-cross-type { font-weight: 600; }
.bb-cross-quality { padding: 0.05rem 0.4rem; border-radius: 4px; font-size: 0.72rem; }
.bb-q-high { background: rgba(46, 160, 67, 0.14); color: var(--color-success, #2ea043); }
.bb-q-medium { background: rgba(187, 128, 9, 0.14); color: #9e6a03; }
.bb-q-low, .bb-q-none { background: var(--color-bg-subtle); color: var(--color-text-subtle); }
.bb-cross-date { color: var(--color-text-subtle); }
.bb-cross-claim { margin: 0; font-size: 0.85rem; line-height: 1.5; }
.bb-cross-toggle { align-self: flex-start; padding: 0; border: none; background: none; color: var(--color-primary); font-size: 0.75rem; cursor: pointer; }
.bb-cross-toggle:hover { text-decoration: underline; }
.bb-cross-quote { margin: 0; padding: 0.3rem 0.5rem; border-left: 2px solid var(--color-border); font-size: 0.76rem; color: var(--color-text-subtle); }
.bb-cross-actions { display: flex; align-items: center; gap: 0.8rem; }
.bb-cross-evidence { display: inline-flex; align-items: center; gap: 0.25rem; font-size: 0.78rem; color: var(--color-primary); text-decoration: none; }
.bb-cross-evidence:hover { text-decoration: underline; }
.bb-cross-board { display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.15rem 0.5rem; border: 1px solid var(--color-border); border-radius: 6px; background: var(--color-bg); font-size: 0.78rem; color: var(--color-text); cursor: pointer; }
.bb-cross-board:hover { border-color: var(--color-primary); color: var(--color-primary); }
.bb-discover-chip { display: inline-flex; align-items: center; gap: 0.25rem; padding: 0.1rem 0.45rem; border: 1px dashed var(--color-border); border-radius: 6px; background: var(--color-bg); font-size: 0.74rem; color: var(--color-text-subtle); cursor: pointer; }
.bb-discover-chip:not(:disabled):hover { border-color: var(--color-primary); color: var(--color-primary); }
.bb-discover-chip:disabled { opacity: 0.55; cursor: not-allowed; }

.bb-rel { display: flex; flex-direction: column; gap: 0.3rem; padding: 0.55rem 0.7rem; border: 1px solid var(--color-border-subtle); border-radius: 8px; }
.bb-rel-head { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; }
.bb-rel-type { font-size: 0.72rem; font-weight: 600; padding: 0.1rem 0.5rem; border-radius: 4px; background: var(--color-bg-sunken); color: var(--color-text-secondary); }
.bb-rel-possible_causal { background: var(--color-accent-subtle); color: var(--color-accent); }
.bb-rel-divergent { background: var(--color-warning-subtle); color: var(--color-warning); }
.bb-conf { font-size: 0.72rem; padding: 0.08rem 0.45rem; border-radius: 99px; border: 1px solid; }
.conf-high { color: var(--color-success); border-color: color-mix(in srgb, var(--color-success) 45%, transparent); }
.conf-medium { color: var(--color-warning); border-color: color-mix(in srgb, var(--color-warning) 45%, transparent); }
.conf-low { color: var(--color-text-muted); border-color: var(--color-border-medium); }
.bb-rel-explain { margin: 0; font-size: 0.86rem; line-height: 1.65; color: var(--color-text-secondary); }
.bb-rel-refs { margin: 0; font-size: 0.72rem; color: var(--color-text-muted); display: flex; align-items: center; gap: 0.3rem; flex-wrap: wrap; }
.bb-ref-tag { font-family: ui-monospace, Menlo, monospace; background: var(--color-bg-sunken); border-radius: 4px; padding: 0 0.3rem; }

/* 不确定项 */
.bb-unc { display: flex; flex-direction: column; gap: 0.2rem; padding-left: 0.7rem; border-left: 2px dashed var(--color-border-subtle); }
.bb-unc-q { font-size: 0.9rem; font-weight: 600; color: var(--color-text-primary); }
.bb-unc-why { margin: 0; font-size: 0.8rem; color: var(--color-text-secondary); line-height: 1.6; }
.bb-unc-need { margin: 0; font-size: 0.78rem; color: var(--color-text-muted); display: flex; align-items: baseline; gap: 0.3rem; }

/* 研究问题 */
.bb-q { display: flex; flex-direction: column; gap: 0.25rem; padding-left: 0.7rem; border-left: 2px solid var(--color-accent); }
.bb-q-main { display: flex; align-items: baseline; gap: 0.45rem; }
.bb-q-id { font-size: 0.68rem; color: var(--color-accent); flex-shrink: 0; font-family: ui-monospace, Menlo, monospace; }
.bb-q-text { font-size: 0.92rem; line-height: 1.6; color: var(--color-text-primary); }
.bb-q-rationale { margin: 0; font-size: 0.78rem; color: var(--color-text-muted); }
.bb-q-foot { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; }
.bb-running-tag {
  display: inline-flex; align-items: center; gap: 0.3rem;
  font-size: 0.72rem; color: var(--color-accent); font-weight: 500;
  padding: 0.08rem 0.5rem; border-radius: 99px;
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
}
.bb-investigate-btn {
  display: inline-flex; align-items: center; gap: 5px;
  font-family: inherit; cursor: pointer; border: none; border-radius: 8px;
  font-weight: 600; font-size: 12.5px; padding: 6px 12px;
  background: var(--color-accent); color: #fff;
  transition: background 0.15s, opacity 0.15s;
}
.bb-investigate-btn:hover:not(:disabled) { background: var(--color-accent-hover); }
.bb-investigate-btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* 自填问题 */
.bb-custom {
  display: flex; flex-direction: column; gap: 0.35rem;
  padding: 0.7rem 0.8rem; border: 1px dashed var(--color-border-medium); border-radius: 10px;
  background: var(--color-bg-elevated);
}
.bb-custom-label { font-size: 0.78rem; font-weight: 600; color: var(--color-text-secondary); }
.bb-custom-input {
  width: 100%; box-sizing: border-box; resize: vertical; font-family: inherit;
  font-size: 0.85rem; line-height: 1.6; color: var(--color-text-primary);
  background: var(--color-input-bg); border: 1px solid var(--color-input-border);
  border-radius: 8px; padding: 0.45rem 0.6rem; outline: none;
}
.bb-custom-input:focus { border-color: var(--color-input-focus); }
.bb-custom-foot { display: flex; align-items: center; justify-content: flex-end; gap: 0.6rem; }
.bb-rune-count { font-size: 0.72rem; color: var(--color-text-muted); font-family: ui-monospace, Menlo, monospace; }
.bb-rune-count.over { color: var(--color-warning); font-weight: 600; }

/* 泳道引用 */
.bb-lane-list { display: flex; flex-wrap: wrap; gap: 0.45rem; }
.bb-lane-chip {
  display: inline-flex; align-items: center; gap: 0.3rem;
  font-size: 0.76rem; padding: 0.25rem 0.55rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 40%, transparent);
  border-radius: 99px; background: color-mix(in srgb, var(--color-accent) 7%, transparent);
  color: var(--color-text-primary); cursor: pointer; font-family: inherit;
}
.bb-lane-chip:hover { background: color-mix(in srgb, var(--color-accent) 15%, transparent); }
.bb-rel-lanes { display: inline-flex; flex-wrap: wrap; gap: 0.3rem; }
.bb-lane-note { color: var(--color-text-muted); }

.bb-footer { margin-top: 0.4rem; font-size: 0.7rem; }
.muted { color: var(--color-text-muted); }
.serif { font-family: Georgia, "Songti SC", "SimSun", "Source Serif 4", serif; }

/* ── 窄屏适配（≤720px）────────────────────────────────────────────── */
@media (max-width: 720px) {
  .bb-investigate-btn { min-height: 36px; padding: 8px 14px; }
  .bb-lane-chip { min-height: 36px; }
  .bb-summary { font-size: 0.96rem; }
}
</style>
