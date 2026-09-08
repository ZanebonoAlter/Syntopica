<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import type {
  BoardAnalysisResultRow,
  BoardInvestigationSectors,
  BoardInvestigationQuestion,
  BoardInvestigationEvidence,
  BoardInvestigationHypothesis,
} from '~/api/boardEnrichment'

/**
 * 调查报告视图（board-level-deep-analysis 5.6 / D4 终局 schema）。
 *
 * 呈现 result_kind=board_investigation 的 sectors：question / hypotheses
 * （含 assessment）/ conclusion / evidence_chain / lane_refs / method_refs。
 * 设计纪律（「研究档案 / 证据台账」，非 AI 紫色卡片）：
 *  - 首屏有限结论：调查问题 + 当前判断（summary/confidence/scope/boundary）
 *    + 各 hypothesis assessment；支持证据 / 反证 / gap / 证据详情折叠展开，
 *    绝不首屏铺连续长文；
 *  - 不渲染 argument / depth / 重复长文（调查 schema 无此字段，塞了也忽略）；
 *  - assessment 五态、允许 H0 最可信、允许全部 insufficient（不强选赢家）；
 *  - 证据 quote 原地展开（可核查：web/page URL 可点、机构+日期+逐字摘录）；
 *  - lane 证据可下钻：emit {laneId, prefill}，prefill = 具体调查问题 +
 *    证据 lane_note/quote + 反证假设说明（长度受控），不是抽象 lens/结论；
 *    幽灵泳道（ref 不在本份 lane_refs 白名单）或非法 ref 不渲染入口不 emit。
 */
const props = defineProps<{
  result: BoardAnalysisResultRow | null
  loading?: boolean
}>()

const emit = defineEmits<{
  /** lane 证据下钻：laneId + 可编辑的预填问题（具体调查问题 + 证据信息）。 */
  (e: 'drill-lane', payload: { laneId: number; prefill: string }): void
  /** 跨版块泳道引用跳转对方版块（2.4）：本调查经动态授权引用的外板块泳道。 */
  (e: 'open-board', boardId: number): void
}>()

const inv = computed<BoardInvestigationSectors | null>(() => {
  const r = props.result
  if (!r || !r.sectors) return null
  const kind = r.result_kind ?? (r.sectors as { result_kind?: string }).result_kind
  if (kind !== 'board_investigation') return null
  return r.sectors as BoardInvestigationSectors
})

const question = computed(() => inv.value?.question ?? null)
const hypotheses = computed(() => inv.value?.hypotheses ?? [])
const conclusion = computed(() => inv.value?.conclusion ?? null)
const evidenceChain = computed(() => inv.value?.evidence_chain ?? [])
const methodRefs = computed(() => inv.value?.method_refs ?? [])

/** 本份调查的活跃泳道白名单（后端已 sanitize；前端再防幽灵引用）。 */
const activeLaneIds = computed<Set<number>>(
  () => new Set((inv.value?.lane_refs ?? []).map((lr) => lr.lane_id)),
)

/** lane id → 所属版块（跨版块引用；0/缺省 = 本版块）。2.4：旧报告无
 * board_id 时查表得 undefined → 视为本版块，读取不崩。 */
const laneOwnerBoards = computed<Map<number, number>>(() => {
  const m = new Map<number, number>()
  for (const lr of inv.value?.lane_refs ?? []) {
    if (lr.board_id && lr.board_id > 0) m.set(lr.lane_id, lr.board_id)
  }
  return m
})

function crossBoardOf(laneId: number): number | null {
  return laneOwnerBoards.value.get(laneId) ?? null
}

/** 证据 id → 条目（假设的 support/counter 引用解析）。 */
const evidenceById = computed<Map<string, BoardInvestigationEvidence>>(() => {
  const m = new Map<string, BoardInvestigationEvidence>()
  for (const ev of evidenceChain.value) m.set(ev.id, ev)
  return m
})

// ── 中文标签 ────────────────────────────────────────────────────────────
const ASSESSMENT_LABELS: Record<string, string> = {
  supported: '证据支持',
  plausible: '初步可信',
  insufficient: '证据不足',
  weakened: '被削弱',
  refuted: '被推翻',
}
function assessmentLabel(a: string): string {
  return ASSESSMENT_LABELS[a] ?? a
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
const SOURCE_LABELS: Record<string, string> = {
  news: '内部新闻记忆',
  web: '外部网页',
  page: '原文页',
  lane: '泳道事实',
}
function sourceLabel(ev: BoardInvestigationEvidence): string {
  return SOURCE_LABELS[ev.source_type] ?? ev.source_type
}
const KIND_LABELS: Record<string, string> = {
  quote: '原文摘录',
  series: '数据序列',
  chart: '图表',
}
function kindLabel(k?: string): string {
  if (!k) return ''
  return KIND_LABELS[k] ?? k
}
function questionSourceLabel(source: string): string {
  return source === 'custom' ? '自填' : '简报候选'
}

// ── 折叠态（首屏有限结论：细节渐进展开） ────────────────────────────────
const openHypotheses = ref(new Set<string>())
const openEvidence = ref(new Set<string>())

function toggleHypothesis(id: string) {
  const next = new Set(openHypotheses.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  openHypotheses.value = next
}
function hypothesisOpen(id: string): boolean {
  return openHypotheses.value.has(id)
}
function toggleEvidence(id: string) {
  const next = new Set(openEvidence.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  openEvidence.value = next
}
function evidenceOpen(id: string): boolean {
  return openEvidence.value.has(id)
}

/** 假设的 support/counter 引用 → 证据行（悬空引用原样显示 id，不崩）。 */
function resolveEvidence(ids: string[]): Array<{ id: string; source: string }> {
  return ids.map((id) => ({
    id,
    source: evidenceById.value.has(id) ? sourceLabel(evidenceById.value.get(id)!) : '',
  }))
}

// ── lane 下钻 ───────────────────────────────────────────────────────────
/** lane 证据 ref → 泳道编号（非法/非正整数 → null）。 */
function laneIdOf(ev: BoardInvestigationEvidence): number | null {
  if (ev.source_type !== 'lane') return null
  const n = Number(ev.ref)
  return Number.isInteger(n) && n > 0 ? n : null
}
function laneChipVisible(ev: BoardInvestigationEvidence): boolean {
  const laneId = laneIdOf(ev)
  return laneId !== null && activeLaneIds.value.has(laneId)
}

/** rune 截断（保多字节字符完整）。 */
function clipRunes(s: string, max: number): string {
  const runes = Array.from(s)
  return runes.length <= max ? s : `${runes.slice(0, max - 1).join('')}…`
}

const PREFILL_TOTAL_MAX = 400

/** 下钻 prefill：具体调查问题 + 反证假设说明 + 证据 lane_note/quote（组合可用信息，总长受控）。 */
function buildLanePrefill(
  q: BoardInvestigationQuestion | null,
  ev: BoardInvestigationEvidence,
  hyps: BoardInvestigationHypothesis[],
): string {
  const parts: string[] = []
  if (q?.text?.trim()) parts.push(clipRunes(q.text.trim(), 160))
  for (const hid of ev.counters ?? []) {
    const h = hyps.find((x) => x.id === hid)
    if (h) {
      parts.push(
        clipRunes(
          `该证据是假设「${h.label}」的反证（当前评估：${assessmentLabel(h.assessment)}）`,
          160,
        ),
      )
    }
  }
  const evid: string[] = []
  if (ev.lane_note?.trim()) evid.push(clipRunes(ev.lane_note.trim(), 120))
  if (ev.quote?.trim()) evid.push(clipRunes(`证据摘录：「${ev.quote.trim()}」`, 160))
  if (evid.length) parts.push(evid.join('；'))
  return clipRunes(parts.join('\n'), PREFILL_TOTAL_MAX)
}

function drillEvidenceLane(ev: BoardInvestigationEvidence) {
  const laneId = laneIdOf(ev)
  // 幽灵泳道（不在本份 lane_refs）或非法 ref：不 emit
  if (laneId === null || !activeLaneIds.value.has(laneId)) return
  // 跨版块泳道（2.4）：聚焦分析属于本版块，跳对方版块查看该泳道。
  if (crossBoardOf(laneId) !== null) {
    emit('open-board', crossBoardOf(laneId)!)
    return
  }
  emit('drill-lane', {
    laneId,
    prefill: buildLanePrefill(question.value, ev, hypotheses.value),
  })
}

function fmtDate(d?: string) {
  return d ? d.slice(0, 10) : '—'
}
</script>

<template>
  <article v-if="loading" class="bi-report loading">
    <Icon icon="mdi:loading" width="18" class="spin" /> 正在装配调查报告…
  </article>

  <article v-else-if="!result || !inv" class="bi-report empty">
    <Icon icon="mdi:file-document-outline" width="22" />
    <p>还没有调查报告。在版块简报的「值得调查的问题」里点「深入调查」，或自填一个问题发起。</p>
  </article>

  <article v-else class="bi-report">
    <!-- ── 刊头：调查问题 ─────────────────────────────────────────── -->
    <header class="bi-masthead">
      <div class="bi-eyebrow">
        调查报告
        <span v-if="result.created_at" class="bi-date">{{ fmtDate(result.created_at) }}</span>
        <span class="bi-flag">源简报 #{{ inv.parent_briefing_id }}</span>
      </div>
      <h1 class="bi-question serif">{{ question?.text || '（调查问题缺失）' }}</h1>
      <p class="bi-q-meta">
        来源：{{ questionSourceLabel(question?.source ?? '') }}<template v-if="question?.id">（候选 id {{ question.id }}）</template>
      </p>
    </header>

    <!-- ── 当前判断（首屏有限结论：conclusion 四字段） ──────────────── -->
    <section class="bi-section bi-conclusion">
      <h2 class="serif">当前判断</h2>
      <p class="bi-concl-summary serif">{{ conclusion?.summary || '（本轮未产出结论）' }}</p>
      <div class="bi-concl-meta">
        <span class="bi-conf" :class="confidenceTone(conclusion?.confidence ?? '')">
          置信 · {{ confidenceLabel(conclusion?.confidence ?? '') }}
        </span>
        <span v-if="conclusion?.scope">适用范围：{{ conclusion.scope }}</span>
        <span v-if="conclusion?.boundary">边界：{{ conclusion.boundary }}</span>
      </div>
    </section>

    <!-- ── 假设评估（首屏只看 assessment；细节折叠） ────────────────── -->
    <section class="bi-section">
      <h2 class="serif">假设评估<span class="bi-count">{{ hypotheses.length }}</span></h2>
      <p v-if="!hypotheses.length" class="bi-plain">本轮没有产出可评估的假设。</p>
      <div v-else class="bi-hyp-list">
        <div v-for="h in hypotheses" :key="h.id" class="bi-hyp">
          <button
            type="button"
            class="bi-hyp-head"
            :data-test="`hyp-toggle-${h.id}`"
            :aria-expanded="hypothesisOpen(h.id)"
            @click="toggleHypothesis(h.id)"
          >
            <Icon :icon="hypothesisOpen(h.id) ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
            <span class="bi-hyp-id">{{ h.id }}</span>
            <span class="bi-hyp-label">{{ h.label }}</span>
            <span v-if="h.is_null" class="bi-hyp-null">零假设</span>
            <span class="bi-assess" :class="`bi-assess-${h.assessment}`">{{ assessmentLabel(h.assessment) }}</span>
            <span class="bi-conf-inline">{{ confidenceLabel(h.confidence) }}</span>
          </button>

          <div v-if="hypothesisOpen(h.id)" class="bi-hyp-body">
            <p v-if="h.scope" class="bi-hyp-scope">适用范围：{{ h.scope }}</p>

            <div class="bi-hyp-part">
              <h3 class="bi-part-title bi-part-support">支持证据<span class="bi-count">{{ h.support_evidence?.length ?? 0 }}</span></h3>
              <p v-if="!h.support_evidence?.length" class="bi-plain">暂无支持证据。</p>
              <ul v-else class="bi-ref-list">
                <li v-for="r in resolveEvidence(h.support_evidence)" :key="r.id" class="bi-ref">
                  <span class="bi-ref-id">{{ r.id }}</span>
                  <span v-if="r.source" class="bi-ref-src">{{ r.source }}</span>
                </li>
              </ul>
            </div>

            <div class="bi-hyp-part">
              <h3 class="bi-part-title bi-part-counter">反证<span class="bi-count">{{ h.counter_evidence?.length ?? 0 }}</span></h3>
              <p v-if="!h.counter_evidence?.length" class="bi-plain">暂无反证。</p>
              <ul v-else class="bi-ref-list">
                <li v-for="r in resolveEvidence(h.counter_evidence)" :key="r.id" class="bi-ref">
                  <span class="bi-ref-id">{{ r.id }}</span>
                  <span v-if="r.source" class="bi-ref-src">{{ r.source }}</span>
                </li>
              </ul>
            </div>

            <div class="bi-hyp-part">
              <h3 class="bi-part-title bi-part-gap">缺口<span class="bi-count">{{ h.gaps?.length ?? 0 }}</span></h3>
              <p v-if="!h.gaps?.length" class="bi-plain">无标注缺口。</p>
              <ul v-else class="bi-gap-list">
                <li v-for="(g, i) in h.gaps" :key="i" class="bi-gap">{{ g }}</li>
              </ul>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ── 证据台账（折叠展开：quote 原地可核查） ───────────────────── -->
    <section class="bi-section">
      <h2 class="serif">证据台账<span class="bi-count">{{ evidenceChain.length }}</span></h2>
      <p v-if="!evidenceChain.length" class="bi-plain">本轮调查没有通过核验、可展示的证据。</p>
      <div v-else class="bi-ev-list">
        <div v-for="ev in evidenceChain" :key="ev.id" class="bi-ev">
          <button
            type="button"
            class="bi-ev-head"
            :data-test="`ev-toggle-${ev.id}`"
            :aria-expanded="evidenceOpen(ev.id)"
            @click="toggleEvidence(ev.id)"
          >
            <Icon :icon="evidenceOpen(ev.id) ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="14" />
            <span class="bi-ev-id">{{ ev.id }}</span>
            <span class="bi-ev-src">{{ sourceLabel(ev) }}</span>
            <span v-if="ev.kind" class="bi-ev-kind">{{ kindLabel(ev.kind) }}</span>
            <span v-if="ev.date" class="bi-ev-date">{{ fmtDate(ev.date) }}</span>
          </button>

          <div v-if="evidenceOpen(ev.id)" class="bi-ev-body">
            <p v-if="ev.institution" class="bi-ev-meta">机构：{{ ev.institution }}</p>
            <p v-if="ev.quote" class="bi-ev-quote serif">&ldquo;{{ ev.quote }}&rdquo;</p>

            <div class="bi-ev-polarity">
              <span v-if="ev.supports?.length">支持：<span v-for="id in ev.supports" :key="id" class="bi-ref-tag">{{ id }}</span></span>
              <span v-if="ev.counters?.length">反对：<span v-for="id in ev.counters" :key="id" class="bi-ref-tag">{{ id }}</span></span>
              <span v-if="!ev.supports?.length && !ev.counters?.length">未标注指向</span>
            </div>

            <a
              v-if="ev.url"
              :href="ev.url"
              target="_blank"
              rel="noopener noreferrer"
              class="bi-ev-link"
            >
              <Icon icon="mdi:open-in-new" width="12" />
              打开原文核查
            </a>

            <button
              v-if="laneChipVisible(ev)"
              type="button"
              class="bi-lane-chip"
              :class="{ 'bi-lane-cross': crossBoardOf(laneIdOf(ev)!) !== null }"
              :data-test="`ev-lane-${laneIdOf(ev)}`"
              :title="crossBoardOf(laneIdOf(ev)!) !== null ? `该泳道来自版块 #${crossBoardOf(laneIdOf(ev)!)}（本次调查经动态授权引用），点击跳转` : '下钻该泳道聚焦分析（预填此证据与反证说明为可修改的问题）'"
              @click="drillEvidenceLane(ev)"
            >
              <Icon icon="mdi:transit-connection-variant" width="12" />
              <template v-if="crossBoardOf(laneIdOf(ev)!) !== null">版块 #{{ crossBoardOf(laneIdOf(ev)!) }} · 泳道 #{{ laneIdOf(ev) }}</template>
              <template v-else>泳道 #{{ laneIdOf(ev) }} · 下钻核查</template>
            </button>
            <p v-else-if="ev.source_type === 'lane'" class="bi-ev-invalid muted">（该证据的泳道引用已失效）</p>
          </div>
        </div>
      </div>
    </section>

    <!-- ── 引用泳道（含跨版块标注，2.4） ───────────────────────────── -->
    <section v-if="inv?.lane_refs?.length" class="bi-section" data-test="inv-lane-refs">
      <h2 class="serif">引用泳道（{{ inv.lane_refs.length }}）</h2>
      <ul class="bi-lanerefs">
        <li
          v-for="lr in inv.lane_refs"
          :key="lr.lane_id"
          class="bi-laneref"
          :data-test="`inv-laneref-${lr.lane_id}`"
        >
          <template v-if="lr.board_id">
            <button
              type="button"
              class="bi-lane-chip bi-lane-cross"
              :data-test="`inv-laneref-open-${lr.lane_id}`"
              title="该泳道来自对方版块（本次调查经动态授权引用），点击跳转"
              @click="emit('open-board', lr.board_id)"
            >
              <Icon icon="mdi:external-link" width="12" />
              版块 #{{ lr.board_id }} · 泳道 #{{ lr.lane_id }}
            </button>
            <span v-if="lr.note" class="bi-laneref-note muted">{{ lr.note }}</span>
          </template>
          <template v-else>
            <span class="bi-laneref-local">泳道 #{{ lr.lane_id }}</span>
            <span v-if="lr.note" class="bi-laneref-note muted">{{ lr.note }}</span>
          </template>
        </li>
      </ul>
    </section>

    <!-- ── 方法留痕（审计可见，不冒充证据） ─────────────────────────── -->
    <section v-if="methodRefs.length" class="bi-section">
      <h2 class="serif">方法留痕（{{ methodRefs.length }}）</h2>
      <p class="bi-method-note">
        <span v-for="(m, i) in methodRefs" :key="m.id">
          <template v-if="i > 0">；</template>《{{ m.title || `方法 #${m.id}` }}》
        </span>
      </p>
    </section>

    <footer v-if="result.session_id" class="bi-footer">
      <span class="muted">session {{ result.session_id }}</span>
    </footer>
  </article>
</template>

<style scoped>
.bi-report {
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
}
.bi-report.loading,
.bi-report.empty {
  align-items: center;
  gap: 0.6rem;
  padding: 2.2rem 1rem;
  color: var(--color-text-muted);
  border: 1px dashed var(--color-border-medium);
  border-radius: 8px;
  font-size: 0.85rem;
  text-align: center;
}
.spin { animation: bi-spin 1s linear infinite; }
@keyframes bi-spin { to { transform: rotate(360deg); } }

/* 刊头 */
.bi-masthead { display: flex; flex-direction: column; gap: 0.4rem; border-bottom: 2px solid var(--color-text-primary); padding-bottom: 0.8rem; }
.bi-eyebrow {
  font-size: 0.72rem; letter-spacing: 0.14em; text-transform: uppercase;
  color: var(--color-accent); font-weight: 600;
  display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;
}
.bi-date { letter-spacing: 0.04em; text-transform: none; color: var(--color-text-muted); font-weight: 400; }
.bi-flag {
  font-size: 0.66rem; padding: 0.05rem 0.45rem; border-radius: 99px;
  background: var(--color-bg-sunken); color: var(--color-text-muted);
  text-transform: none; letter-spacing: 0.02em;
}
.bi-question { margin: 0; font-size: 1.12rem; line-height: 1.6; color: var(--color-text-primary); font-weight: 600; }
.bi-q-meta { margin: 0; font-size: 0.75rem; color: var(--color-text-muted); }

/* 区段 */
.bi-section { display: flex; flex-direction: column; gap: 0.5rem; }
.bi-section h2 { margin: 0; font-size: 0.98rem; display: flex; align-items: center; gap: 0.4rem; }
.bi-count {
  font-size: 0.68rem; color: var(--color-text-muted); font-weight: 500;
  padding: 0.02rem 0.45rem; border-radius: 99px; background: var(--color-bg-sunken);
}
.bi-plain {
  margin: 0; font-size: 0.85rem; color: var(--color-text-muted); line-height: 1.6;
  padding: 0.45rem 0.7rem; background: var(--color-bg-sunken); border-radius: 8px;
}

/* 当前判断 */
.bi-conclusion { padding-left: 0.7rem; border-left: 3px solid var(--color-accent); }
.bi-concl-summary { margin: 0; font-size: 1rem; line-height: 1.7; color: var(--color-text-primary); }
.bi-concl-meta { display: flex; align-items: baseline; gap: 0.7rem; flex-wrap: wrap; font-size: 0.78rem; color: var(--color-text-secondary); }
.bi-conf { font-size: 0.72rem; padding: 0.08rem 0.45rem; border-radius: 99px; border: 1px solid; }
.conf-high { color: var(--color-success); border-color: color-mix(in srgb, var(--color-success) 45%, transparent); }
.conf-medium { color: var(--color-warning); border-color: color-mix(in srgb, var(--color-warning) 45%, transparent); }
.conf-low { color: var(--color-text-muted); border-color: var(--color-border-medium); }

/* 假设评估 */
.bi-hyp-list { display: flex; flex-direction: column; gap: 0.45rem; }
.bi-hyp { border: 1px solid var(--color-border-subtle); border-radius: 8px; overflow: hidden; }
.bi-hyp-head {
  display: flex; align-items: center; gap: 0.45rem; flex-wrap: wrap;
  width: 100%; padding: 0.5rem 0.7rem; border: none; background: none;
  font-family: inherit; text-align: left; cursor: pointer; font-size: 0.88rem;
  color: var(--color-text-primary);
}
.bi-hyp-head:hover { background: var(--color-bg-hover); }
.bi-hyp-id { font-size: 0.68rem; color: var(--color-text-muted); font-family: ui-monospace, Menlo, monospace; flex-shrink: 0; }
.bi-hyp-label { line-height: 1.5; }
.bi-hyp-null {
  font-size: 0.66rem; padding: 0.03rem 0.4rem; border-radius: 99px; flex-shrink: 0;
  background: var(--color-bg-sunken); color: var(--color-text-muted);
}
.bi-assess { font-size: 0.7rem; font-weight: 600; padding: 0.08rem 0.5rem; border-radius: 4px; flex-shrink: 0; background: var(--color-bg-sunken); color: var(--color-text-secondary); }
.bi-assess-supported { background: var(--color-success-subtle); color: var(--color-success); }
.bi-assess-plausible { background: color-mix(in srgb, var(--color-accent) 12%, transparent); color: var(--color-accent); }
.bi-assess-insufficient { background: var(--color-bg-sunken); color: var(--color-text-muted); }
.bi-assess-weakened, .bi-assess-refuted { background: var(--color-warning-subtle); color: var(--color-warning); }
.bi-conf-inline { font-size: 0.7rem; color: var(--color-text-muted); flex-shrink: 0; }

.bi-hyp-body {
  display: flex; flex-direction: column; gap: 0.6rem;
  padding: 0.6rem 0.8rem; border-top: 1px dashed var(--color-border-subtle);
  background: var(--color-bg-sunken);
}
.bi-hyp-scope { margin: 0; font-size: 0.76rem; color: var(--color-text-muted); }
.bi-hyp-part { display: flex; flex-direction: column; gap: 0.3rem; }
.bi-part-title { margin: 0; font-size: 0.76rem; font-weight: 600; display: flex; align-items: center; gap: 0.35rem; }
.bi-part-support { color: var(--color-success); }
.bi-part-counter { color: var(--color-warning); }
.bi-part-gap { color: var(--color-text-muted); }
.bi-hyp-part .bi-plain { padding: 0.3rem 0.5rem; font-size: 0.78rem; }
.bi-ref-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.2rem; }
.bi-ref { font-size: 0.78rem; display: flex; align-items: baseline; gap: 0.4rem; }
.bi-ref-id { font-family: ui-monospace, Menlo, monospace; color: var(--color-text-primary); }
.bi-ref-src { color: var(--color-text-muted); font-size: 0.72rem; }
.bi-gap-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.2rem; }
.bi-gap { font-size: 0.78rem; color: var(--color-text-secondary); line-height: 1.55; }

/* 证据台账 */
.bi-ev-list { display: flex; flex-direction: column; gap: 0.4rem; }
.bi-ev { border: 1px solid var(--color-border-subtle); border-radius: 8px; overflow: hidden; }
.bi-ev-head {
  display: flex; align-items: center; gap: 0.45rem; flex-wrap: wrap;
  width: 100%; padding: 0.45rem 0.7rem; border: none; background: none;
  font-family: inherit; text-align: left; cursor: pointer; font-size: 0.82rem;
  color: var(--color-text-primary);
}
.bi-ev-head:hover { background: var(--color-bg-hover); }
.bi-ev-id { font-family: ui-monospace, Menlo, monospace; font-size: 0.7rem; color: var(--color-text-muted); }
.bi-ev-src { font-size: 0.72rem; padding: 0.05rem 0.45rem; border-radius: 4px; background: var(--color-bg-sunken); color: var(--color-text-secondary); }
.bi-ev-kind { font-size: 0.7rem; color: var(--color-text-muted); }
.bi-ev-date { font-size: 0.7rem; color: var(--color-text-muted); font-family: ui-monospace, Menlo, monospace; }
.bi-ev-body {
  display: flex; flex-direction: column; gap: 0.4rem;
  padding: 0.6rem 0.8rem; border-top: 1px dashed var(--color-border-subtle);
  background: var(--color-bg-sunken);
}
.bi-ev-meta { margin: 0; font-size: 0.76rem; color: var(--color-text-secondary); }
.bi-ev-quote {
  margin: 0; font-size: 0.86rem; line-height: 1.7; color: var(--color-text-primary);
  padding: 0.45rem 0.7rem; border-left: 2px solid var(--color-border-medium);
  background: var(--color-bg); border-radius: 0 6px 6px 0;
}
.bi-ev-polarity { display: flex; align-items: baseline; gap: 0.7rem; flex-wrap: wrap; font-size: 0.74rem; color: var(--color-text-muted); }
.bi-ref-tag { font-family: ui-monospace, Menlo, monospace; background: var(--color-bg); border-radius: 4px; padding: 0 0.3rem; margin-left: 0.2rem; }
.bi-ev-link {
  display: inline-flex; align-items: center; gap: 0.3rem; align-self: flex-start;
  font-size: 0.78rem; color: var(--color-accent); text-decoration: underline;
  text-underline-offset: 3px; word-break: break-all;
}
.bi-ev-link:hover { color: var(--color-accent-hover); }
.bi-lane-cross { border-style: dashed; }
.bi-lanerefs { display: flex; flex-direction: column; gap: 0.35rem; margin: 0; padding: 0; list-style: none; }
.bi-laneref { display: flex; flex-wrap: wrap; align-items: center; gap: 0.45rem; font-size: 0.8rem; }
.bi-laneref-note { font-size: 0.76rem; }
.bi-ev-invalid { margin: 0; font-size: 0.72rem; }

/* lane 下钻 chip */
.bi-lane-chip {
  display: inline-flex; align-items: center; gap: 0.3rem; align-self: flex-start;
  font-size: 0.76rem; padding: 0.25rem 0.55rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 40%, transparent);
  border-radius: 99px; background: color-mix(in srgb, var(--color-accent) 7%, transparent);
  color: var(--color-text-primary); cursor: pointer; font-family: inherit;
}
.bi-lane-chip:hover { background: color-mix(in srgb, var(--color-accent) 15%, transparent); }

/* 方法留痕 */
.bi-method-note { margin: 0; font-size: 0.75rem; color: var(--color-text-muted); line-height: 1.6; }

.bi-footer { margin-top: 0.4rem; font-size: 0.7rem; }
.muted { color: var(--color-text-muted); }
.serif { font-family: Georgia, "Songti SC", "SimSun", "Source Serif 4", serif; }

/* ── 窄屏适配（≤720px）────────────────────────────────────────────── */
@media (max-width: 720px) {
  .bi-lane-chip { min-height: 36px; }
  .bi-ev-link { min-height: 36px; align-items: center; }
  .bi-question { font-size: 1.02rem; }
  .bi-hyp-head, .bi-ev-head { padding: 0.6rem 0.6rem; }
}
</style>
