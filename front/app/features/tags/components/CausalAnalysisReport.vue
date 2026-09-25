<script setup lang="ts">
import { computed, ref } from 'vue'
import type {
  ResultDetailRow,
  AnalyzeOutput,
  AnalyzeForm,
  AnalyzeRef,
  AnalyzeDepth,
} from '~/api/boardEnrichment'
import AnalyzeRefChip from './AnalyzeRefChip.vue'
import { renderMarkdown, renderMarkdownInline } from '~/utils/markdown'
import { useMermaidRender } from '~/composables/useMermaidRender'
// 全局 .markdown-body 样式（文章阅读器同款），让 md 渲染产物有标题/列表/粗体样式 + 双主题
import '~/components/article/ArticleContent.css'

/**
 * 因果分析报告（报刊式 editorial）—— causal-analysis-agent 阶段3b-i。
 *
 * 消费 result.sectors 新形状 AnalyzeOutput {form, lens, analysis}，按 form 多态渲染：
 *  - event_chain（事件链）：事实层（已验证 claim）+ 横向时间线依据轴 + 推演见解层
 *  - theme_vein（主题脉络）：平行线索 veins + 跨线索洞察（平行非因果，不画箭头）
 *  - single_point（单点影响）：impact（意味/波及/对标）+ 证据（影响本身即见解）
 *  - structural（结构演化）：演化叙述 + 关键阶段（长时段结构命题，无离散事件）
 *  - sparse（骨感）：信息不足 notice + 轻量 summary（不渲染见解层与深度层）
 *  - 深度层 depth（非 sparse 形态）：系统重定位 / 多层机制 / 历史类比 / 范式转折 /
 *    边界限定 / 可核查证据链。旧结果无 depth 字段 → 不渲染该区块、不报错（降级）。
 *
 * 设计要点：
 *  - 事实层 vs 见解层视觉分区（事实=中性已验证框；见解=确定性着色框），读者可辨真假
 *  - 确定性 4 级视觉：high(绿·实线) / medium(蓝) / low(琥珀·虚线) / question(紫·虚线·？)
 *  - 双类引用 AnalyzeRefChip：news📰 / tool🔧，hover tooltip 显示 ref+quote
 *  - 视角展示：lens 作副标题（不做视角选择 UI，3c 延后）
 *  - 探索过程 trace：文末折叠 tool_calls（低调）
 *
 * 旧 position/signals/causal_chain 消费已随 EvolutionReport 移除。颜色全走 CSS 变量，
 * 明暗主题自动适配（question 紫由 accent+info color-mix 派生，不硬编码 hex）。
 */
const props = defineProps<{
  result: ResultDetailRow | null
  topicLabel?: string
  loading?: boolean
}>()

// mermaid 图渲染（render-mermaid-diagrams）：宿主 = 报告根，覆盖洞察/脉络各 markdown 块
const causalRootRef = ref<HTMLElement | null>(null)
useMermaidRender(causalRootRef, () => props.result)

// ── 形态标签 ──────────────────────────────────────────────────────────────
const FORM_LABELS: Record<AnalyzeForm, string> = {
  event_chain: '事件链',
  theme_vein: '主题脉络',
  single_point: '单点影响',
  structural: '结构演化',
  sparse: '骨感简报',
}

// ── 数据派生（判别联合窄化：绑定 local const 保证 TS 窄化生效）──────────────
const output = computed<AnalyzeOutput | null>(() => {
  const s = props.result?.sectors
  // 防御旧/畸形数据：必须形如 {form, lens, analysis}
  if (s && typeof s === 'object' && 'form' in s && 'analysis' in s) {
    return s as AnalyzeOutput
  }
  return null
})

/** event_chain analysis 体（已窄化）。 */
const ecBody = computed(() => {
  const o = output.value
  return o && o.form === 'event_chain' ? o.analysis : null
})
/** theme_vein analysis 体。 */
const tvBody = computed(() => {
  const o = output.value
  return o && o.form === 'theme_vein' ? o.analysis : null
})
/** single_point analysis 体。 */
const spBody = computed(() => {
  const o = output.value
  return o && o.form === 'single_point' ? o.analysis : null
})
/** sparse analysis 体。 */
const spSparseBody = computed(() => {
  const o = output.value
  return o && o.form === 'sparse' ? o.analysis : null
})
/** structural analysis 体。 */
const stBody = computed(() => {
  const o = output.value
  return o && o.form === 'structural' ? o.analysis : null
})

/**
 * 深度层（非 sparse 形态；可选消费）。
 * 旧 topic_enrichment_result 无 depth 字段 → 返回 null → 深度层区块不渲染（降级不崩）。
 */
const depthBody = computed<AnalyzeDepth | null>(() => {
  const o = output.value
  if (!o || o.form === 'sparse') return null
  const d = o.analysis.depth
  return d && typeof d === 'object' ? d : null
})

const form = computed<AnalyzeForm | null>(() => output.value?.form ?? null)
const formLabel = computed(() => (form.value ? FORM_LABELS[form.value] ?? '因果分析' : '因果分析'))

const headline = computed(
  () => props.topicLabel?.trim() || `${formLabel.value} · 因果分析`,
)
const lensText = computed(() => output.value?.lens?.trim() || '')
const reportDate = computed(() => {
  const c = props.result?.created_at
  if (!c) return ''
  try {
    return new Date(c).toISOString().slice(0, 10)
  } catch {
    return ''
  }
})

/** byline 计数（按形态语义）。 */
const stats = computed<string[]>(() => {
  const o = output.value
  if (!o) return []
  if (o.form === 'event_chain') {
    return [
      `${o.analysis.fact_layer?.length ?? 0} 条事实`,
      `${o.analysis.timeline?.length ?? 0} 个时序节点`,
      `${o.analysis.insight_layer?.length ?? 0} 条推演`,
    ]
  }
  if (o.form === 'theme_vein') {
    return [
      `${o.analysis.veins?.length ?? 0} 条平行线索`,
      `${o.analysis.cross_insight?.length ?? 0} 条跨线索洞察`,
    ]
  }
  if (o.form === 'single_point') {
    return [`${o.analysis.evidence?.length ?? 0} 条证据`]
  }
  if (o.form === 'structural') {
    return [
      `${o.analysis.phases?.length ?? 0} 个关键阶段`,
      `${o.analysis.depth?.evidence_chain?.length ?? 0} 条可核查证据`,
    ]
  }
  return [] // sparse：无计数
})

const hasContent = computed(() => output.value !== null)

// ── 确定性 4 级视觉 ────────────────────────────────────────────────────────
interface CertMeta {
  label: string
  cls: string
  icon: string
}
const CERT_META: Record<string, CertMeta> = {
  high: { label: '高确定性', cls: 'cert-high', icon: '' },
  medium: { label: '中确定性', cls: 'cert-medium', icon: '' },
  low: { label: '低确定性', cls: 'cert-low', icon: '≈' },
  question: { label: '存疑', cls: 'cert-question', icon: '？' },
}
function certMeta(cert?: string): CertMeta {
  const key = String(cert ?? '').toLowerCase()
  return (
    CERT_META[key] ?? {
      label: cert ? String(cert) : '确定性',
      cls: 'cert-unknown',
      icon: '',
    }
  )
}

// ── 时间线关键节点：有 ref 的节点视为"关键"（有据可查）──────────────────────
function isKeyTimelineNode(node: { ref?: AnalyzeRef }): boolean {
  return !!node.ref
}

// ── 深度层证据链来源图标：news 📰 / web 🌐 / page 📄 ──────────────────────
function evIcon(sourceType?: string): string {
  if (sourceType === 'web') return '🌐'
  if (sourceType === 'page') return '📄'
  return '📰'
}

// ── 探索过程 trace（tool_calls，结构未冻结，防御解析）──────────────────────
const toolCalls = computed<unknown[]>(() => {
  const tc = props.result?.tool_calls
  return Array.isArray(tc) ? tc : []
})
function toolName(t: unknown): string {
  if (t && typeof t === 'object') {
    const o = t as Record<string, unknown>
    return String(o.name ?? o.tool ?? o.skill ?? o.action ?? '')
  }
  return String(t ?? '')
}
</script>

<template>
  <section ref="causalRootRef" class="causal-report">
    <!-- ── loading 骨架 ─────────────────────────────────────────────── -->
    <div v-if="loading" class="ca-loading">
      <div class="ca-sk-line w-60" />
      <div class="ca-sk-line w-90" />
      <div class="ca-sk-line w-75" />
      <div class="ca-sk-bar" />
      <div class="ca-sk-line w-80" />
      <div class="ca-sk-line w-65" />
      <span class="ca-loading-note">正在生成因果分析…</span>
    </div>

    <!-- ── 空态 ─────────────────────────────────────────────────────── -->
    <div v-else-if="!result" class="ca-empty">
      <div class="ca-empty-ic">🔬</div>
      <p class="ca-empty-h">暂无因果分析</p>
      <p class="ca-empty-sub">点上方「▶ 重新分析」生成第一份因果分析报告。</p>
    </div>

    <!-- ── 畸形/旧数据兜底（sectors 不是新 AnalyzeOutput 形状） ───────── -->
    <div v-else-if="!hasContent" class="ca-empty">
      <div class="ca-empty-ic">🧩</div>
      <p class="ca-empty-h">报告格式不兼容</p>
      <p class="ca-empty-sub">该结果为旧版格式，重新分析即可生成新版因果报告。</p>
    </div>

    <!-- ── 报告 ─────────────────────────────────────────────────────── -->
    <article v-else class="report">
      <!-- Masthead -->
      <header class="masthead">
        <div class="eyebrow">
          {{ formLabel }} · 因果分析<span v-if="reportDate"> · {{ reportDate }}</span>
        </div>
        <h1 class="serif masthead-title">{{ headline }}</h1>
        <div v-if="lensText" class="lens-sub">视角 · {{ lensText }}</div>
        <div v-if="stats.length" class="byline">
          <span v-for="(s, i) in stats" :key="i">
            <span v-if="i > 0" class="sep">·</span>{{ s }}
          </span>
        </div>
      </header>

      <!-- ═════════════ event_chain：事件链 ═══════════════════════════ -->
      <template v-if="form === 'event_chain' && ecBody">
        <!-- 事实层（已验证 claim） -->
        <section v-if="ecBody.fact_layer?.length" class="layer fact-layer">
          <h3 class="layer-h">
            <span class="layer-tag fact">事实层</span>
            <span class="layer-note">已验证 · 确有其事</span>
          </h3>
          <div class="fact-list">
            <div
              v-for="(f, i) in ecBody.fact_layer"
              :key="'f'+i"
              class="fact-item"
              :class="{ verified: f.verified }"
            >
              <span class="fact-mark">{{ f.verified ? '✓' : '○' }}</span>
              <span class="fact-claim" v-html="renderMarkdownInline(f.claim)" />
              <span v-if="f.evidence?.length" class="fact-refs">
                <AnalyzeRefChip
                  v-for="(r, ri) in f.evidence"
                  :key="ri"
                  :r="r"
                  compact
                />
              </span>
            </div>
          </div>
        </section>

        <!-- 时间线依据轴（横向时序，关键节点高亮，hover 显 ref） -->
        <section v-if="ecBody.timeline?.length" class="layer">
          <h3 class="layer-h">
            <span class="layer-tag">时间线依据</span>
            <span class="layer-note">关键节点（有据）高亮</span>
          </h3>
          <div class="timeline">
            <div class="tl-track" />
            <div
              v-for="(n, i) in ecBody.timeline"
              :key="'t'+i"
              class="tl-node"
              :class="{ key: isKeyTimelineNode(n) }"
            >
              <span class="tl-dot" />
              <span class="tl-date">{{ n.date }}</span>
              <span class="tl-event" v-html="renderMarkdownInline(n.event)" />
              <AnalyzeRefChip v-if="n.ref" :r="n.ref" compact class="tl-ref" />
            </div>
          </div>
        </section>

        <!-- 推演见解层 -->
        <section v-if="ecBody.insight_layer?.length" class="layer insight-layer">
          <h3 class="layer-h">
            <span class="layer-tag insight">推演见解</span>
            <span class="layer-note">分析员推断 · 按确定性分级</span>
          </h3>
          <div
            v-for="(ins, i) in ecBody.insight_layer"
            :key="'i'+i"
            class="insight-card"
            :class="certMeta(ins.cert).cls"
          >
            <div class="ic-head">
              <span class="ic-cert">{{ certMeta(ins.cert).icon }}{{ certMeta(ins.cert).label }}</span>
              <span class="ic-title" v-html="renderMarkdownInline(ins.title)" />
            </div>
            <div class="ic-logic markdown-body" v-html="renderMarkdown(ins.logic)" />
            <div v-if="ins.evidence?.length" class="ic-refs">
              <span class="ic-refs-label">证据</span>
              <AnalyzeRefChip v-for="(r, ri) in ins.evidence" :key="'e'+ri" :r="r" compact />
            </div>
            <div v-if="ins.web_verified?.length" class="ic-refs web">
              <span class="ic-refs-label">web 核验</span>
              <AnalyzeRefChip v-for="(r, ri) in ins.web_verified" :key="'w'+ri" :r="r" compact />
            </div>
          </div>
        </section>
      </template>

      <!-- ═════════════ theme_vein：主题脉络（平行，不画箭头） ═════════ -->
      <template v-else-if="form === 'theme_vein' && tvBody">
        <div class="vein-note">
          <span class="vn-ic">⫶</span>
          以下为<b>平行线索</b>，并列展开，<b>非因果链</b>——不画因果箭头。
        </div>

        <!-- 平行线索 veins -->
        <section v-if="tvBody.veins?.length" class="layer">
          <h3 class="layer-h">
            <span class="layer-tag">平行线索</span>
            <span class="layer-note">{{ tvBody.veins.length }} 条并列</span>
          </h3>
          <div class="vein-grid">
            <div
              v-for="(v, i) in tvBody.veins"
              :key="'v'+i"
              class="vein-card"
            >
              <div class="vc-name" v-html="renderMarkdownInline(v.name)" />
              <div class="vc-desc markdown-body" v-html="renderMarkdown(v.desc)" />
              <div v-if="v.evidence?.length" class="vc-refs">
                <AnalyzeRefChip v-for="(r, ri) in v.evidence" :key="ri" :r="r" compact />
              </div>
            </div>
          </div>
        </section>

        <!-- 跨线索洞察 -->
        <section v-if="tvBody.cross_insight?.length" class="layer insight-layer">
          <h3 class="layer-h">
            <span class="layer-tag insight">跨线索洞察</span>
            <span class="layer-note">多条线索交汇处的推演</span>
          </h3>
          <div
            v-for="(ins, i) in tvBody.cross_insight"
            :key="'c'+i"
            class="insight-card"
            :class="certMeta(ins.cert).cls"
          >
            <div class="ic-head">
              <span class="ic-cert">{{ certMeta(ins.cert).icon }}{{ certMeta(ins.cert).label }}</span>
              <span class="ic-title" v-html="renderMarkdownInline(ins.title)" />
            </div>
            <div class="ic-logic markdown-body" v-html="renderMarkdown(ins.logic)" />
            <div v-if="ins.evidence?.length" class="ic-refs">
              <span class="ic-refs-label">证据</span>
              <AnalyzeRefChip v-for="(r, ri) in ins.evidence" :key="'e'+ri" :r="r" compact />
            </div>
            <div v-if="ins.web_verified?.length" class="ic-refs web">
              <span class="ic-refs-label">web 核验</span>
              <AnalyzeRefChip v-for="(r, ri) in ins.web_verified" :key="'w'+ri" :r="r" compact />
            </div>
          </div>
        </section>
      </template>

      <!-- ═════════════ single_point：单点影响 ═════════════════════════ -->
      <template v-else-if="form === 'single_point' && spBody">
        <section class="layer">
          <h3 class="layer-h">
            <span class="layer-tag">影响评估</span>
            <span class="layer-note">意味 / 波及 / 对标 · 评估本身即见解</span>
          </h3>
          <div class="impact-grid">
            <div class="impact-cell">
              <div class="ic2-label">意味</div>
              <div class="ic2-text markdown-body" v-html="renderMarkdown(spBody.impact?.implication)" />
            </div>
            <div class="impact-cell">
              <div class="ic2-label">波及</div>
              <div class="ic2-text markdown-body" v-html="renderMarkdown(spBody.impact?.ripple)" />
            </div>
            <div class="impact-cell">
              <div class="ic2-label">对标</div>
              <div class="ic2-text markdown-body" v-html="renderMarkdown(spBody.impact?.benchmark)" />
            </div>
          </div>
        </section>

        <section v-if="spBody.evidence?.length" class="layer">
          <h3 class="layer-h">
            <span class="layer-tag">证据</span>
          </h3>
          <div class="sp-evidence">
            <AnalyzeRefChip v-for="(r, ri) in spBody.evidence" :key="ri" :r="r" />
          </div>
        </section>
      </template>

      <!-- ═════════════ structural：结构演化（长时段结构命题） ═══════════ -->
      <template v-else-if="form === 'structural' && stBody">
        <section v-if="stBody.evolution_narrative" class="layer">
          <h3 class="layer-h">
            <span class="layer-tag">结构演化叙述</span>
            <span class="layer-note">长时段结构命题 · 无单一离散事件驱动</span>
          </h3>
          <div class="structural-narrative serif markdown-body" v-html="renderMarkdown(stBody.evolution_narrative)" />
        </section>

        <!-- 关键阶段（复用时间线依据轴样式，有据节点高亮） -->
        <section v-if="stBody.phases?.length" class="layer">
          <h3 class="layer-h">
            <span class="layer-tag">关键阶段</span>
            <span class="layer-note">关键节点（有据）高亮</span>
          </h3>
          <div class="timeline">
            <div class="tl-track" />
            <div
              v-for="(p, i) in stBody.phases"
              :key="'p'+i"
              class="tl-node"
              :class="{ key: isKeyTimelineNode(p) }"
            >
              <span class="tl-dot" />
              <span class="tl-date">{{ p.period }}</span>
              <span class="tl-event" v-html="renderMarkdownInline(p.event)" />
              <AnalyzeRefChip v-if="p.ref" :r="p.ref" compact class="tl-ref" />
            </div>
          </div>
        </section>
      </template>

      <!-- ═════════════ sparse：骨感简报 ═══════════════════════════════ -->
      <template v-else-if="form === 'sparse' && spSparseBody">
        <div class="sparse-notice">
          <span class="sn-ic">ⓘ</span>
          <span class="sn-text" v-html="renderMarkdownInline(spSparseBody.notice || '信息不足，暂无法展开完整因果分析。')" />
        </div>
        <div v-if="spSparseBody.summary" class="sparse-summary serif markdown-body" v-html="renderMarkdown(spSparseBody.summary)" />
      </template>

      <!-- ═════════════ 深度层（非 sparse 形态；旧结果无 depth 不渲染） ═════════ -->
      <section v-if="depthBody" class="layer depth-layer">
        <h3 class="layer-h">
          <span class="layer-tag depth">深度层</span>
          <span class="layer-note">结构化剖析 · 系统 / 机制 / 历史 / 边界 / 证据</span>
        </h3>

        <!-- 系统重定位 -->
        <div v-if="depthBody.system_reframe" class="dp-reframe">
          <span class="dp-reframe-label">系统重定位</span>
          <span class="dp-reframe-text" v-html="renderMarkdownInline(depthBody.system_reframe)" />
        </div>

        <!-- 多层机制 -->
        <div v-if="depthBody.mechanism_layers?.length" class="dp-block">
          <div class="dp-sub-h">多层机制</div>
          <div
            v-for="(m, i) in depthBody.mechanism_layers"
            :key="'m'+i"
            class="dp-mech"
          >
            <div class="dp-mech-name" v-html="renderMarkdownInline(m.layer)" />
            <div class="dp-mech-logic markdown-body" v-html="renderMarkdown(m.deep_logic)" />
            <div v-if="m.basis" class="dp-mech-basis">
              <span class="dp-k">依据</span><span v-html="renderMarkdownInline(m.basis)" />
            </div>
          </div>
        </div>

        <!-- 历史类比 -->
        <div v-if="depthBody.historical_analogy?.length" class="dp-block">
          <div class="dp-sub-h">历史类比</div>
          <div
            v-for="(h, i) in depthBody.historical_analogy"
            :key="'h'+i"
            class="dp-analogy"
          >
            <div class="dp-analogy-case" v-html="renderMarkdownInline(h.case)" />
            <div class="dp-analogy-row">
              <span class="dp-k">机制类比</span><span v-html="renderMarkdownInline(h.mechanism)" />
            </div>
            <div class="dp-analogy-row">
              <span class="dp-k">何处不同</span><span v-html="renderMarkdownInline(h.diff)" />
            </div>
          </div>
        </div>

        <!-- 范式转折（regime_shift 非 null 才显示） -->
        <div v-if="depthBody.regime_shift" class="dp-block dp-regime">
          <div class="dp-sub-h">范式转折</div>
          <div class="dp-regime-judgment" v-html="renderMarkdownInline(depthBody.regime_shift.judgment)" />
          <div v-if="depthBody.regime_shift.evidence" class="dp-regime-ev">
            <span class="dp-k">依据</span><span v-html="renderMarkdownInline(depthBody.regime_shift.evidence)" />
          </div>
        </div>

        <!-- 边界限定（高亮「还不能下结论」） -->
        <div v-if="depthBody.boundary" class="dp-boundary">
          <span class="dp-b-ic">⚠</span>
          <div class="dp-b-body">
            <span class="dp-b-label">边界限定 · 目前还不能下结论</span>
            <span class="dp-b-text" v-html="renderMarkdownInline(depthBody.boundary)" />
          </div>
        </div>

        <!-- 可核查证据链（web/page 类 url 可点击） -->
        <div v-if="depthBody.evidence_chain?.length" class="dp-block">
          <div class="dp-sub-h">可核查证据链</div>
          <ul class="dp-evidence">
            <li v-for="(ev, i) in depthBody.evidence_chain" :key="'ev'+i">
              <span class="dp-ev-ic">{{ evIcon(ev.source_type) }}</span>
              <div class="dp-ev-body">
                <div class="dp-ev-head">
                  <a
                    v-if="ev.url"
                    :href="ev.url"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="dp-ev-link"
                  >{{ ev.ref || ev.url }}</a>
                  <span v-else class="dp-ev-ref">{{ ev.ref || '引用' }}</span>
                  <span v-if="ev.institution" class="dp-ev-inst">{{ ev.institution }}</span>
                  <span v-if="ev.date" class="dp-ev-date">{{ ev.date }}</span>
                </div>
                <div v-if="ev.quote" class="dp-ev-quote">&ldquo;{{ ev.quote }}&rdquo;</div>
              </div>
            </li>
          </ul>
        </div>
      </section>

      <!-- ── 探索过程 trace（折叠 · 低调） ─────────────────────────────── -->
      <details v-if="toolCalls.length" class="trace">
        <summary>探索过程 · {{ toolCalls.length }} 次工具调用</summary>
        <ol class="trace-list">
          <li v-for="(t, i) in toolCalls" :key="i">
            <span class="trace-idx">{{ i + 1 }}</span>
            <span class="trace-name">{{ toolName(t) || '工具调用' }}</span>
          </li>
        </ol>
      </details>

      <div class="editor-note">
        <span class="en-label">关于这份报告</span>
        因果分析按叙事形态（事件链 / 主题脉络 / 单点影响 / 结构演化 / 骨感）多态呈现。
        <b>事实层</b>为已验证客观陈述，<b>推演见解</b>按确定性（高/中/低/存疑）分级着色，
        读者可辨哪是真、哪是猜。非骨感形态附<b>深度层</b>：系统重定位 / 多层机制 / 边界限定
        与可核查证据链（<b class="en-news">📰 新闻</b> / <b>🌐 网页</b> / <b>📄 正文</b>，网页与正文可点击核查原文）。
        <b class="en-news">📰 新闻</b>来自订阅源报道，
        <b class="en-tool">🔧 工具查证</b>为 agent 自主调用 opencli skill 的结果；hover 引用标记可见出处。
      </div>
    </article>
  </section>
</template>

<style scoped>
/* 本地派生 subtle 变体（main.css 仅 success/warning/accent 有 subtle）+ question 紫。
   定义在 .causal-report 根，子组件（AnalyzeRefChip）经 DOM 继承拿到 info-subtle。 */
.causal-report {
  --color-info-subtle: color-mix(in srgb, var(--color-info) 12%, transparent);
  --color-error-subtle: color-mix(in srgb, var(--color-error) 14%, transparent);
  --cert-question: color-mix(in srgb, var(--color-accent) 50%, var(--color-info));
  --cert-question-subtle: color-mix(in srgb, var(--cert-question) 14%, transparent);
}

.serif {
  font-family: Georgia, "Songti SC", "SimSun", "Source Serif 4", "Noto Serif SC", serif;
}

/* ── loading / 空态 ─────────────────────────────────────────────────── */
.ca-loading {
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
  padding: 2rem 1.5rem;
}
.ca-sk-line {
  height: 12px;
  border-radius: 6px;
  background: linear-gradient(
    90deg,
    var(--color-bg-sunken) 0%,
    var(--color-bg-hover) 50%,
    var(--color-bg-sunken) 100%
  );
  background-size: 200% 100%;
  animation: ca-sk 1.4s ease-in-out infinite;
}
.ca-sk-line.w-60 { width: 60%; }
.ca-sk-line.w-65 { width: 65%; }
.ca-sk-line.w-75 { width: 75%; }
.ca-sk-line.w-80 { width: 80%; }
.ca-sk-line.w-90 { width: 90%; }
.ca-sk-bar { height: 28px; border-radius: 8px; background: var(--color-accent-subtle); }
@keyframes ca-sk {
  0%, 100% { background-position: 200% 0; }
  50% { background-position: -200% 0; }
}
.ca-loading-note {
  font-size: 12px;
  color: var(--color-text-muted);
  text-align: center;
  margin-top: 0.4rem;
}

.ca-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 0.4rem;
  padding: 3rem 1.5rem;
  background: var(--color-bg-elevated);
  border: 1px dashed var(--color-border-medium);
  border-radius: 12px;
}
.ca-empty-ic { font-size: 2.2rem; opacity: 0.5; line-height: 1; }
.ca-empty-h { font-size: 15px; font-weight: 700; margin: 0; color: var(--color-text-primary); }
.ca-empty-sub { font-size: 12.5px; color: var(--color-text-secondary); margin: 0; }

/* ── 报告容器（与 .ew-panel 叙事工坊工作台 960px 同宽，宽屏不挤中条） ── */
.report { max-width: 960px; margin: 0 auto; padding: 0.5rem 0 1rem; }

/* Masthead */
.masthead {
  border-bottom: 3px double var(--color-text-primary);
  padding-bottom: 1.1rem;
  margin-bottom: 1.4rem;
}
.masthead .eyebrow {
  font-size: 10.5px;
  letter-spacing: 0.22em;
  text-transform: uppercase;
  color: var(--color-accent);
  font-weight: 700;
  margin-bottom: 0.6rem;
}
.masthead-title {
  font-size: 2rem;
  line-height: 1.22;
  margin: 0 0 0.5rem;
  font-weight: 800;
  letter-spacing: -0.015em;
  color: var(--color-text-primary);
}
.lens-sub {
  font-size: 13.5px;
  font-style: italic;
  color: var(--color-text-secondary);
  margin-bottom: 0.5rem;
}
.byline {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  flex-wrap: wrap;
  font-size: 11.5px;
  color: var(--color-text-muted);
}
.byline .sep { color: var(--color-border-medium); margin: 0 0.15rem; }

/* ── 层（layer）通用 ───────────────────────────────────────────────── */
.layer { margin-bottom: 1.6rem; }
.layer-h {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  margin: 0 0 0.8rem;
  padding-bottom: 0.35rem;
  border-bottom: 1px solid var(--color-border-subtle);
}
.layer-tag {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  padding: 2px 9px;
  border-radius: 4px;
  background: var(--color-bg-sunken);
  color: var(--color-text-secondary);
}
.layer-tag.fact { background: var(--color-success-subtle); color: var(--color-success); }
.layer-tag.insight { background: var(--color-accent-subtle); color: var(--color-accent); }
.layer-note {
  font-size: 11px;
  color: var(--color-text-muted);
  font-weight: 500;
}

/* ── 事实层 ─────────────────────────────────────────────────────────── */
.fact-list { display: flex; flex-direction: column; gap: 0.5rem; }
.fact-item {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.6rem 0.8rem;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  font-size: 14px;
  line-height: 1.65;
  color: var(--color-text-primary);
}
.fact-item.verified {
  border-left: 3px solid var(--color-success);
  background: var(--color-success-subtle);
}
.fact-mark {
  flex: 0 0 auto;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
}
.fact-item.verified .fact-mark {
  color: var(--color-success);
  background: color-mix(in srgb, var(--color-success) 18%, transparent);
}
.fact-claim { flex: 1 1 auto; }
.fact-refs {
  flex: 0 0 auto;
  display: inline-flex;
  gap: 4px;
  flex-wrap: wrap;
  align-items: center;
}

/* ── 时间线依据轴（横向） ───────────────────────────────────────────── */
.timeline {
  position: relative;
  display: flex;
  gap: 1.2rem;
  overflow-x: auto;
  padding: 1.4rem 0.3rem 0.6rem;
}
.tl-track {
  position: absolute;
  top: calc(1.4rem + 5px);
  left: 0.3rem;
  right: 0.3rem;
  height: 2px;
  background: var(--color-border-medium);
}
.tl-node {
  position: relative;
  flex: 0 0 auto;
  min-width: 110px;
  max-width: 160px;
  padding-top: 1.6rem;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
}
.tl-dot {
  position: absolute;
  top: 1.4rem;
  left: 6px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--color-bg-elevated);
  border: 2px solid var(--color-border-strong);
  transform: translateY(-40%);
}
.tl-node.key .tl-dot {
  background: var(--color-accent);
  border-color: var(--color-accent);
  box-shadow: 0 0 0 4px var(--color-accent-subtle);
}
.tl-date {
  font-size: 11px;
  font-weight: 700;
  color: var(--color-text-muted);
  font-family: ui-monospace, Menlo, monospace;
}
.tl-node.key .tl-date { color: var(--color-accent); }
.tl-event {
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--color-text-primary);
}
.tl-ref { margin-top: 2px; }

/* ── 推演见解层 + 确定性视觉 ────────────────────────────────────────── */
.insight-card {
  border-left: 3px solid var(--color-border-medium);
  padding: 0.7rem 0.9rem;
  margin-bottom: 0.7rem;
  border-radius: 0 8px 8px 0;
  background: var(--color-bg-elevated);
}
.insight-card.cert-high {
  border-left: 3px solid var(--color-success);
  background: var(--color-success-subtle);
}
.insight-card.cert-medium {
  border-left: 3px solid var(--color-info);
}
.insight-card.cert-low {
  border-left: 3px dashed var(--color-warning);
  background: var(--color-warning-subtle);
}
.insight-card.cert-question {
  border-left: 3px dashed var(--cert-question);
  background: var(--cert-question-subtle);
}
.ic-head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.35rem;
  flex-wrap: wrap;
}
.ic-cert {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  padding: 1px 8px;
  border-radius: 999px;
  white-space: nowrap;
}
.cert-high .ic-cert { color: var(--color-success); background: color-mix(in srgb, var(--color-success) 16%, transparent); }
.cert-medium .ic-cert { color: var(--color-info); background: var(--color-info-subtle); }
.cert-low .ic-cert { color: var(--color-warning); background: color-mix(in srgb, var(--color-warning) 16%, transparent); }
.cert-question .ic-cert { color: var(--cert-question); background: var(--cert-question-subtle); }
.cert-unknown .ic-cert { color: var(--color-text-muted); background: var(--color-bg-sunken); }
.ic-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--color-text-primary);
}
.ic-logic {
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--color-text-secondary);
  margin: 0 0 0.5rem;
}
.ic-refs {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  flex-wrap: wrap;
  font-size: 11.5px;
}
.ic-refs.web { margin-top: 0.3rem; }
.ic-refs-label {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

/* ── theme_vein：平行线索（非因果） ────────────────────────────────── */
.vein-note {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.55rem 0.85rem;
  margin-bottom: 1.2rem;
  background: var(--color-info-subtle);
  border-left: 3px solid var(--color-info);
  border-radius: 0 8px 8px 0;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--color-text-secondary);
}
.vein-note .vn-ic { color: var(--color-info); font-weight: 700; }
.vein-note b { color: var(--color-text-primary); }

.vein-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 0.8rem;
}
.vein-card {
  padding: 0.8rem 0.9rem;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-top: 3px solid var(--color-info);
  border-radius: 8px;
}
.vc-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--color-text-primary);
  margin-bottom: 0.3rem;
}
.vc-desc {
  font-size: 13px;
  line-height: 1.65;
  color: var(--color-text-secondary);
  margin: 0 0 0.5rem;
}
.vc-refs { display: flex; gap: 4px; flex-wrap: wrap; }

/* ── single_point：影响评估 ─────────────────────────────────────────── */
.impact-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 0.8rem;
}
.impact-cell {
  padding: 0.8rem 0.9rem;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
}
.impact-cell .ic2-label {
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--color-accent);
  margin-bottom: 0.35rem;
}
.impact-cell .ic2-text {
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--color-text-primary);
  margin: 0;
}
.sp-evidence { display: flex; gap: 0.4rem; flex-wrap: wrap; }

/* ── sparse：骨感简报 ───────────────────────────────────────────────── */
.sparse-notice {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.8rem 1rem;
  margin-bottom: 1rem;
  background: var(--color-warning-subtle);
  border-left: 3px solid var(--color-warning);
  border-radius: 0 8px 8px 0;
}
.sparse-notice .sn-ic {
  flex: 0 0 auto;
  color: var(--color-warning);
  font-weight: 700;
}
.sparse-notice .sn-text {
  font-size: 13.5px;
  line-height: 1.65;
  color: var(--color-text-secondary);
}
.sparse-summary {
  font-size: 15px;
  line-height: 1.8;
  color: var(--color-text-primary);
  margin: 0;
}

/* ── 深度层（非 sparse 形态；颜色全走 CSS 变量，明暗主题自适应） ────────── */
.layer-tag.depth { background: var(--color-info-subtle); color: var(--color-info); }
.dp-reframe {
  display: flex;
  align-items: flex-start;
  gap: 0.6rem;
  padding: 0.7rem 0.9rem;
  margin-bottom: 0.8rem;
  background: var(--color-info-subtle);
  border-left: 3px solid var(--color-info);
  border-radius: 0 8px 8px 0;
}
.dp-reframe-label {
  flex: 0 0 auto;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  color: var(--color-info);
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--color-info) 14%, transparent);
  margin-top: 2px;
}
.dp-reframe-text {
  font-size: 14px;
  font-weight: 600;
  line-height: 1.65;
  color: var(--color-text-primary);
}
.dp-block { margin-bottom: 0.9rem; }
.dp-sub-h {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--color-text-muted);
  margin-bottom: 0.45rem;
}
.dp-mech {
  padding: 0.6rem 0.85rem;
  margin-bottom: 0.5rem;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-left: 3px solid var(--color-info);
  border-radius: 0 8px 8px 0;
}
.dp-mech-name {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--color-text-primary);
  margin-bottom: 0.25rem;
}
.dp-mech-logic {
  font-size: 13px;
  line-height: 1.65;
  color: var(--color-text-secondary);
  margin: 0;
}
.dp-mech-basis {
  font-size: 12px;
  color: var(--color-text-muted);
  margin-top: 0.35rem;
}
.dp-k {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  color: var(--color-text-muted);
  background: var(--color-bg-sunken);
  padding: 1px 7px;
  border-radius: 4px;
  margin-right: 0.4rem;
}
.dp-analogy {
  padding: 0.6rem 0.85rem;
  margin-bottom: 0.5rem;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
}
.dp-analogy-case {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--color-text-primary);
  margin-bottom: 0.3rem;
}
.dp-analogy-row {
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--color-text-secondary);
  margin-top: 0.25rem;
}
.dp-regime {
  padding: 0.6rem 0.85rem;
  background: var(--color-accent-subtle);
  border-left: 3px solid var(--color-accent);
  border-radius: 0 8px 8px 0;
}
.dp-regime .dp-sub-h { color: var(--color-accent); }
.dp-regime-judgment {
  font-size: 13.5px;
  font-weight: 700;
  line-height: 1.6;
  color: var(--color-text-primary);
}
.dp-regime-ev {
  font-size: 12px;
  color: var(--color-text-secondary);
  margin-top: 0.3rem;
}
.dp-boundary {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.7rem 0.9rem;
  margin-bottom: 0.9rem;
  background: var(--color-warning-subtle);
  border-left: 3px solid var(--color-warning);
  border-radius: 0 8px 8px 0;
}
.dp-b-ic { flex: 0 0 auto; color: var(--color-warning); font-weight: 700; }
.dp-b-label {
  display: block;
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.06em;
  color: var(--color-warning);
  margin-bottom: 0.2rem;
}
.dp-b-text {
  font-size: 13px;
  line-height: 1.65;
  color: var(--color-text-secondary);
}
.dp-evidence {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}
.dp-evidence li {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
}
.dp-ev-ic { flex: 0 0 auto; font-size: 13px; line-height: 1.5; }
.dp-ev-body { flex: 1 1 auto; min-width: 0; }
.dp-ev-head {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  flex-wrap: wrap;
  font-size: 12px;
}
.dp-ev-link {
  color: var(--color-info);
  font-weight: 600;
  text-decoration: none;
  word-break: break-all;
}
.dp-ev-link:hover { text-decoration: underline; }
.dp-ev-ref {
  color: var(--color-text-primary);
  font-weight: 600;
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
}
.dp-ev-inst { color: var(--color-text-secondary); font-weight: 600; }
.dp-ev-date {
  color: var(--color-text-muted);
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
}
.dp-ev-quote {
  font-size: 12px;
  font-style: italic;
  line-height: 1.6;
  color: var(--color-text-secondary);
  margin-top: 0.25rem;
}

/* ── structural：结构演化叙述 ─────────────────────────────────────────── */
.structural-narrative {
  font-size: 15px;
  line-height: 1.8;
  color: var(--color-text-primary);
  margin: 0;
}

/* ── 探索过程 trace（折叠 · 低调） ──────────────────────────────────── */
.trace {
  margin-top: 1.4rem;
  background: var(--color-bg-sunken);
  border-radius: 8px;
  padding: 0.5rem 0.85rem;
}
.trace > summary {
  cursor: pointer;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--color-text-muted);
  list-style: none;
}
.trace > summary::-webkit-details-marker { display: none; }
.trace > summary::before { content: '▸ '; color: var(--color-text-muted); }
.trace[open] > summary::before { content: '▾ '; }
.trace-list {
  margin: 0.5rem 0 0.2rem;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.trace-list li {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  font-size: 11.5px;
  color: var(--color-text-secondary);
}
.trace-idx {
  font-size: 10px;
  font-weight: 700;
  color: var(--color-text-muted);
  background: var(--color-bg-elevated);
  padding: 1px 6px;
  border-radius: 4px;
  min-width: 20px;
  text-align: center;
}
.trace-name {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 11px;
}

/* ── 编者按 ─────────────────────────────────────────────────────────── */
.editor-note {
  margin-top: 1.6rem;
  padding: 0.9rem 1.1rem;
  background: var(--color-bg-sunken);
  border-radius: 8px;
  font-size: 12.5px;
  line-height: 1.7;
  color: var(--color-text-muted);
}
.editor-note .en-label {
  font-weight: 700;
  color: var(--color-text-secondary);
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  display: block;
  margin-bottom: 0.3rem;
}
.editor-note b { color: var(--color-text-secondary); }
.editor-note .en-news { color: var(--color-accent); }
.editor-note .en-tool { color: var(--color-info); }

/* ── 窄屏适配（≤720px，对齐 daily-report 家族断点） ───────────────────── */
@media (max-width: 720px) {
  /* 报告主体：收窄留白 + 标题降档，避免贴边/巨大白边 */
  .report { padding: 0.25rem 0.5rem 0.75rem; }
  .masthead { padding-bottom: 0.8rem; margin-bottom: 1rem; }
  .masthead .eyebrow { letter-spacing: 0.14em; margin-bottom: 0.45rem; }
  .masthead-title { font-size: 1.5rem; }
  .lens-sub { font-size: 12.5px; }
  .layer { margin-bottom: 1.2rem; }
  .layer-h { flex-wrap: wrap; row-gap: 0.2rem; }

  /* loading/空态留白收缩（.ca-sk-line.w-* 是容器百分比，随屏宽自适应，无需改） */
  .ca-loading { padding: 1.4rem 1rem; }
  .ca-empty { padding: 2rem 1rem; }

  /* 事实层：引用 chip 换行落到 claim 下方对齐，不横向挤压正文 */
  .fact-item { flex-wrap: wrap; font-size: 13.5px; }
  .fact-refs { flex: 1 1 100%; padding-left: 26px; }

  /* 时间线：横向滚动 → 垂直堆叠（左竖轨 + 节点全宽，解除 min/max-width 挤压） */
  .timeline {
    flex-direction: column;
    gap: 0.85rem;
    overflow-x: visible;
    padding: 0.3rem 0;
  }
  .tl-track {
    top: 0.55rem;
    bottom: 0.55rem;
    left: 5px;
    right: auto;
    width: 2px;
    height: auto;
  }
  .tl-node { min-width: 0; max-width: none; padding: 0 0 0 1.3rem; }
  .tl-dot { top: 2px; left: 0; transform: none; }

  /* insight 卡片 / 脉络卡 / 影响格 / 深度层块 / 编者按：留白收缩，cert 标签+引用 chip 自然换行 */
  .insight-card { padding: 0.6rem 0.75rem; }
  .vein-card { padding: 0.7rem 0.8rem; }
  .impact-cell { padding: 0.7rem 0.8rem; }
  .dp-mech { padding: 0.5rem 0.7rem; }
  .dp-analogy { padding: 0.5rem 0.7rem; }
  .dp-reframe { padding: 0.6rem 0.7rem; flex-wrap: wrap; }
  .dp-boundary { padding: 0.6rem 0.7rem; }
  .editor-note { padding: 0.75rem 0.85rem; }
}
</style>
