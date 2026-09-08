<script setup lang="ts">
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import type { BoardAnalysisResultRow, BoardSectors } from '~/api/boardEnrichment'

/**
 * 版块级深度分析报告（board-level-deep-analysis D2/D8）。
 *
 * 论文式连续长文呈现 scope=board 的 sectors 五字段：
 *  - 候选命题（interpret 候选 × 切角，标注选中理由）
 *  - 论证骨架（层级递进机制层：intro → layers → boundary → conclusion）
 *  - 深度层 depth（系统重定位/多层机制/历史类比/范式转折/证据链）
 *  - lane_refs 泳道引用可点击 → 触发聚焦分析预填 lens（emit 给父级）
 *
 * 兼容降级：sparse 档（素材不足诚实降级）/ 旧格式无 argument/depth 均不崩，
 * 渲染占位提示。
 */
const props = defineProps<{
  result: BoardAnalysisResultRow | null
  loading?: boolean
}>()

const emit = defineEmits<{
  /** lane 引用点击：lane_id + 预填 lens（泳道名/命题方向），父级切聚焦分析并触发。 */
  (e: 'drill-lane', payload: { laneId: number; lens: string }): void
}>()

/* 本组件只承接 legacy_board_analysis / 旧无 kind 行（面板 5.5 分派），
 * sectors 此时必为旧五字段形状；联合类型在此显式收窄。 */
const sectors = computed<BoardSectors | null>(() =>
  (props.result?.sectors as BoardSectors | null) ?? null,
)
const isSparse = computed(() => sectors.value?.form === 'sparse')
const arg = computed(() => sectors.value?.argument ?? null)
const depth = computed(() => sectors.value?.depth ?? null)
const certTone = computed(() => {
  const cert = arg.value?.conclusion?.cert ?? ''
  if (cert.includes('high')) return 'cert-high'
  if (cert.includes('medium')) return 'cert-medium'
  if (cert.includes('question')) return 'cert-question'
  return 'cert-low'
})
const certLabel = computed(() => {
  const cert = arg.value?.conclusion?.cert ?? ''
  if (cert.includes('high')) return '确定性 · 高'
  if (cert.includes('medium')) return '确定性 · 中'
  if (cert.includes('question')) return '存疑待证'
  return '确定性 · 低'
})

/** lane 泳道证据：深度证据链里 source_type=lane 的条目（ref=lane_id）。 */
const laneEvidences = computed(() => {
  const chain = depth.value?.evidence_chain ?? []
  return chain.filter((e) => e.source_type === 'lane')
})

function drillLane(laneId: number, lensHint: string) {
  emit('drill-lane', { laneId, lens: lensHint })
}

function fmtDate(d?: string) {
  return d ? new Date(d).toISOString().slice(0, 10) : '—'
}
</script>

<template>
  <article v-if="loading" class="ba-report loading">
    <Icon icon="mdi:loading" width="18" class="spin" /> 正在装配版块分析报告…
  </article>

  <article v-else-if="!result || !sectors" class="ba-report empty">
    <Icon icon="mdi:file-document-outline" width="22" />
    <p>该板块还没有版块级分析报告。点上方「分析板块」触发第一次结构化深度分析。</p>
  </article>

  <!-- sparse 诚实降级：素材不足，明说而不是硬编 -->
  <article v-else-if="isSparse" class="ba-report sparse">
    <header class="ba-masthead">
      <div class="ba-eyebrow">版块级分析 · 素材不足</div>
      <h1 class="serif">{{ sectors.thesis || '（未生成命题）' }}</h1>
      <p class="ba-lede">
        当前板块各泳道的新闻记忆不够稠密，结构化深度分析会变成无米之炊。本轮诚实降级为骨架结论——先让泳道多积累几周新闻，或手动「重新汇总」补齐周期后再试。
      </p>
      <p v-if="sectors.interpret_meta?.degraded_why" class="ba-meta">{{ sectors.interpret_meta.degraded_why }}</p>
    </header>
  </article>

  <article v-else class="ba-report">
    <!-- ── 刊头：命题 + 确定性 ─────────────────────────────────────── -->
    <header class="ba-masthead">
      <div class="ba-eyebrow">
        版块级深度分析
        <span v-if="result.created_at" class="ba-date">{{ fmtDate(result.created_at) }}</span>
      </div>
      <h1 class="serif">{{ sectors.thesis }}</h1>
      <p v-if="sectors.angle" class="ba-angle">切角：{{ sectors.angle }}</p>

      <!-- 候选命题：interpret 候选 × 切角，选中者高亮 -->
      <div v-if="sectors.candidates?.length" class="ba-candidates">
        <button
          v-for="(c, i) in sectors.candidates"
          :key="i"
          type="button"
          class="ba-cand"
          :class="{ chosen: i === sectors.chosen_index }"
          :title="i === sectors.chosen_index ? (sectors.reason || '选中理由见报告') : '本轮未选中的候选命题'"
        >
          <span class="ba-cand-hook">{{ c.hook }}</span>
          <span class="ba-cand-thesis">{{ c.thesis }}</span>
          <Icon v-if="i === sectors.chosen_index" icon="mdi:check-decagram" width="13" class="ba-cand-on" />
        </button>
      </div>
    </header>

    <!-- ── 论证主体：层级递进机制层（论文式连续长文） ───────────────── -->
    <template v-if="arg">
      <p class="ba-intro serif">{{ arg.intro }}</p>

      <section v-for="(layer, i) in arg.layers" :key="i" class="ba-layer">
        <h3 class="serif">
          <span class="ba-layer-no">第 {{ i + 1 }} 层</span>
          {{ layer.layer }}
        </h3>
        <p class="ba-logic">{{ layer.deep_logic }}</p>
        <p v-if="layer.basis" class="ba-basis">
          <Icon icon="mdi:source-branch" width="12" /> {{ layer.basis }}
        </p>
      </section>

      <!-- 深度层机制拆解呼应：与 argument.layers 一一呼应的补充依据 -->
      <section v-if="depth?.mechanism_layers?.length" class="ba-depth-layers">
        <h2 class="serif">机制层拆解</h2>
        <div v-for="(m, i) in depth.mechanism_layers" :key="i" class="ba-mech">
          <div class="ba-mech-head">
            <span class="ba-mech-idx">{{ i + 1 }}</span>
            <span class="serif">{{ m.layer }}</span>
          </div>
          <p class="ba-logic">{{ m.deep_logic }}</p>
          <p v-if="m.basis" class="ba-basis"><Icon icon="mdi:link-variant" width="12" /> {{ m.basis }}</p>
        </div>
      </section>

      <!-- 历史类比 -->
      <section v-if="depth?.historical_analogy?.length" class="ba-analogy">
        <h2 class="serif">历史类比</h2>
        <div v-for="(a, i) in depth.historical_analogy" :key="i" class="ba-analogy-item">
          <div class="ba-analogy-case serif">{{ a.case }}</div>
          <p>机制类比：{{ a.mechanism }}</p>
          <p class="ba-basis"><Icon icon="mdi:compare-horizontal" width="12" /> 何处不同：{{ a.diff }}</p>
        </div>
      </section>

      <!-- 范式转折（可空：无迹象后端出 null） -->
      <section v-if="depth?.regime_shift" class="ba-regime">
        <h2 class="serif"><Icon icon="mdi:trending-up" width="15" /> 范式转折判断</h2>
        <p>{{ depth.regime_shift.judgment }}</p>
        <p v-if="depth.regime_shift.evidence" class="ba-basis">{{ depth.regime_shift.evidence }}</p>
      </section>

      <!-- 系统重定位（放论证前也可，此处作收束前的升维段） -->
      <section v-if="depth?.system_reframe" class="ba-reframe">
        <h2 class="serif">系统重定位</h2>
        <p class="serif ba-reframe-text">{{ depth.system_reframe }}</p>
      </section>

      <!-- 边界：反过度解读（argument.boundary 与 depth.boundary 呼应） -->
      <section class="ba-boundary">
        <h2 class="serif"><Icon icon="mdi:shield-alert-outline" width="15" /> 边界：目前还不能下结论的</h2>
        <p>{{ arg.boundary || depth?.boundary }}</p>
      </section>

      <!-- 收束结论 -->
      <section class="ba-conclusion">
        <div class="ba-conclusion-head">
          <h2 class="serif">结论</h2>
          <span class="ba-cert" :class="certTone">{{ certLabel }}</span>
        </div>
        <p class="serif">{{ arg.conclusion?.judgment }}</p>
      </section>

      <!-- 可核查证据链（web/page 可点击核查；lane 可点击下钻） -->
      <section v-if="depth?.evidence_chain?.length" class="ba-evidence">
        <h2 class="serif">证据链（{{ depth.evidence_chain.length }} 条）</h2>
        <ul class="ba-evidence-list">
          <li v-for="(e, i) in depth.evidence_chain" :key="i" class="ba-ev">
            <span class="ba-ev-type" :class="`ba-ev-${e.source_type}`">
              {{ e.source_type === 'news' ? '新闻' : e.source_type === 'web' ? '网页' : e.source_type === 'page' ? '原文' : e.source_type === 'lane' ? '泳道' : e.source_type }}
            </span>
            <div class="ba-ev-body">
              <template v-if="e.url">
                <a :href="e.url" target="_blank" rel="noopener noreferrer" class="ba-ev-quote">{{ e.quote || e.url }}</a>
              </template>
              <template v-else-if="e.source_type === 'lane'">
                <button type="button" class="ba-ev-drill" title="下钻该泳道聚焦分析" @click="drillLane(Number(e.ref), e.lane_note || e.quote || '')">
                  <Icon icon="mdi:transit-connection-variant" width="12" />
                  {{ e.quote || e.lane_note || `泳道 #${e.ref}` }}
                </button>
              </template>
              <template v-else>
                <span class="ba-ev-quote">{{ e.quote }}</span>
              </template>
              <div v-if="e.institution || e.date" class="ba-ev-src">
                <span v-if="e.institution">{{ e.institution }}</span>
                <span v-if="e.institution && e.date"> · </span>
                <span v-if="e.date">{{ e.date }}</span>
                <span v-if="e.kind" class="ba-ev-kind">{{ e.kind === 'quote' ? '原文摘录' : e.kind === 'series' ? '数据序列' : e.kind === 'chart' ? '图表' : e.kind }}</span>
              </div>
            </div>
          </li>
        </ul>
      </section>

      <!-- 泳道引用清单：点击下钻聚焦分析 -->
      <section v-if="sectors.lane_refs?.length" class="ba-lanes">
        <h2 class="serif">泳道引用（{{ sectors.lane_refs.length }} 条）</h2>
        <div class="ba-lane-list">
          <button
            v-for="lr in sectors.lane_refs"
            :key="lr.lane_id"
            type="button"
            class="ba-lane-chip"
            title="下钻该泳道聚焦分析"
            @click="drillLane(lr.lane_id, lr.note || '')"
          >
            <Icon icon="mdi:transit-connection-variant" width="13" />
            泳道 #{{ lr.lane_id }}
            <span v-if="lr.note" class="ba-lane-note">{{ lr.note }}</span>
          </button>
        </div>
      </section>
    </template>

    <!-- argument 缺席（旧格式）：只渲染 thesis + depth 的降级视图 -->
    <p v-else class="ba-legacy-hint">
      该报告为旧格式（无论证骨架）。重新触发一次分析可产出完整论文式论证。
    </p>

    <footer v-if="result?.session_id" class="ba-footer">
      <span class="muted">session {{ result.session_id }}</span>
    </footer>
  </article>
</template>

<style scoped>
.ba-report {
  display: flex;
  flex-direction: column;
  gap: 1.1rem;
}
.ba-report.loading,
.ba-report.empty,
.ba-report.sparse {
  align-items: center;
  gap: 0.6rem;
  padding: 2.2rem 1rem;
  color: var(--ui-muted, #888);
  border: 1px dashed var(--ui-border, #3336);
  border-radius: 8px;
  font-size: 0.85rem;
}
.spin { animation: ba-spin 1s linear infinite; }
@keyframes ba-spin { to { transform: rotate(360deg); } }

/* 刊头 */
.ba-masthead { display: flex; flex-direction: column; gap: 0.45rem; }
.ba-eyebrow {
  font-size: 0.72rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--ui-muted, #888);
  display: flex; align-items: center; gap: 0.5rem;
}
.ba-date { letter-spacing: 0.04em; text-transform: none; }
.ba-masthead h1 { font-size: 1.45rem; line-height: 1.35; margin: 0; }
.ba-angle { color: var(--ui-muted, #888); font-size: 0.85rem; margin: 0; }

/* 候选命题 */
.ba-candidates { display: flex; flex-direction: column; gap: 0.4rem; margin-top: 0.3rem; }
.ba-cand {
  display: flex; align-items: center; gap: 0.6rem;
  padding: 0.45rem 0.6rem;
  border: 1px solid var(--ui-border, #3334);
  border-radius: 6px;
  background: transparent;
  color: inherit;
  text-align: left;
  font-size: 0.8rem;
  cursor: default;
}
.ba-cand.chosen { border-color: var(--ui-primary, #3b82f6); background: color-mix(in srgb, var(--ui-primary, #3b82f6) 8%, transparent); }
.ba-cand-hook { color: var(--ui-muted, #888); flex-shrink: 0; }
.ba-cand-thesis { flex: 1; }
.ba-cand-on { color: var(--ui-primary, #3b82f6); }

/* 论文主体 */
.ba-intro { font-size: 1.02rem; line-height: 1.75; margin: 0; opacity: 0.94; }
.ba-layer { display: flex; flex-direction: column; gap: 0.35rem; padding-left: 0.9rem; border-left: 2px solid var(--ui-border, #3334); }
.ba-layer h3 { margin: 0; font-size: 1rem; display: flex; align-items: baseline; gap: 0.5rem; }
.ba-layer-no { font-size: 0.72rem; color: var(--ui-muted, #888); letter-spacing: 0.08em; }
.ba-logic { margin: 0; line-height: 1.7; font-size: 0.92rem; }
.ba-basis { margin: 0; font-size: 0.78rem; color: var(--ui-muted, #888); display: flex; align-items: flex-start; gap: 0.3rem; }

/* 深度层 */
.ba-depth-layers, .ba-analogy, .ba-regime, .ba-reframe, .ba-boundary, .ba-conclusion, .ba-evidence, .ba-lanes { display: flex; flex-direction: column; gap: 0.55rem; }
.ba-depth-layers h2, .ba-analogy h2, .ba-regime h2, .ba-reframe h2, .ba-boundary h2, .ba-conclusion h2, .ba-evidence h2, .ba-lanes h2 {
  margin: 0.4rem 0 0;
  font-size: 0.98rem;
  display: flex; align-items: center; gap: 0.4rem;
}
.ba-mech { display: flex; flex-direction: column; gap: 0.3rem; padding-left: 0.7rem; border-left: 2px dashed var(--ui-border, #3334); }
.ba-mech-head { display: flex; align-items: center; gap: 0.5rem; font-size: 0.9rem; }
.ba-mech-idx {
  width: 1.2rem; height: 1.2rem; border-radius: 50%;
  display: inline-flex; align-items: center; justify-content: center;
  font-size: 0.7rem; color: var(--ui-muted, #888);
  border: 1px solid var(--ui-border, #3334);
}

.ba-analogy-item { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.9rem; padding-left: 0.7rem; border-left: 2px dashed var(--ui-border, #3334); }
.ba-analogy-case { font-size: 0.95rem; }

.ba-reframe-text { font-size: 1.02rem; line-height: 1.7; font-style: italic; opacity: 0.95; margin: 0; }

.ba-boundary { padding: 0.7rem 0.85rem; border: 1px solid color-mix(in srgb, orange 35%, transparent); border-radius: 8px; background: color-mix(in srgb, orange 6%, transparent); }
.ba-boundary p { margin: 0; font-size: 0.88rem; line-height: 1.65; }

.ba-conclusion { padding: 0.8rem 0.9rem; border: 1px solid var(--ui-border, #3334); border-radius: 8px; }
.ba-conclusion-head { display: flex; align-items: center; justify-content: space-between; }
.ba-conclusion p { margin: 0; line-height: 1.7; }
.ba-cert { font-size: 0.72rem; padding: 0.15rem 0.5rem; border-radius: 99px; border: 1px solid; }
.cert-high { color: #22c55e; border-color: #22c55e66; }
.cert-medium { color: #eab308; border-color: #eab30866; }
.cert-low { color: #f97316; border-color: #f9731666; }
.cert-question { color: #a855f7; border-color: #a855f766; }

/* 证据链 */
.ba-evidence-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.45rem; }
.ba-ev { display: flex; gap: 0.55rem; font-size: 0.82rem; align-items: flex-start; }
.ba-ev-type {
  flex-shrink: 0; min-width: 2.6rem; text-align: center;
  font-size: 0.68rem; padding: 0.12rem 0.3rem; border-radius: 4px;
  border: 1px solid var(--ui-border, #3334); color: var(--ui-muted, #888);
}
.ba-ev-lane { border-color: color-mix(in srgb, #3b82f6 50%, transparent); color: #6ba1f8; }
.ba-ev-web, .ba-ev-page { border-color: color-mix(in srgb, #22c55e 40%, transparent); color: #4ade80; }
.ba-ev-body { display: flex; flex-direction: column; gap: 0.15rem; flex: 1; }
.ba-ev-quote { line-height: 1.55; }
a.ba-ev-quote { color: inherit; text-decoration: underline; text-underline-offset: 3px; text-decoration-color: var(--ui-border, #3336); }
.ba-ev-drill {
  background: none; border: none; padding: 0; cursor: pointer;
  color: #6ba1f8; display: inline-flex; align-items: center; gap: 0.3rem;
  text-align: left; font-size: 0.82rem;
}
.ba-ev-drill:hover { text-decoration: underline; }
.ba-ev-src { display: flex; align-items: center; gap: 0.25rem; color: var(--ui-muted, #888); font-size: 0.72rem; }
.ba-ev-kind { margin-left: 0.4rem; padding: 0.05rem 0.35rem; border-radius: 4px; background: var(--ui-border, #3333); }

/* 泳道引用 */
.ba-lane-list { display: flex; flex-wrap: wrap; gap: 0.45rem; }
.ba-lane-chip {
  display: inline-flex; align-items: center; gap: 0.35rem;
  font-size: 0.78rem; padding: 0.3rem 0.55rem;
  border: 1px solid color-mix(in srgb, #3b82f6 40%, transparent);
  border-radius: 99px; background: color-mix(in srgb, #3b82f6 7%, transparent);
  color: inherit; cursor: pointer;
}
.ba-lane-chip:hover { background: color-mix(in srgb, #3b82f6 15%, transparent); }
.ba-lane-note { color: var(--ui-muted, #888); }

.ba-legacy-hint { color: var(--ui-muted, #888); font-size: 0.82rem; }
.ba-footer { margin-top: 0.4rem; font-size: 0.7rem; }
.muted { color: var(--ui-muted, #888); }
</style>
