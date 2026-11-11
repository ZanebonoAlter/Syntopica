<script setup lang="ts">
import { computed } from 'vue'
import { formatMagazineDate, highlightEvidence, selectLeadStory, type HighlightEvidence } from './dailyReportMagazine'
import type { DailyReport } from '~/api/dailyReports'

const props = defineProps<{
  report: DailyReport
  boardTitle?: string
}>()

const emit = defineEmits<{
  /** 重点定位意图（D14）：仅可定位条目发出 cluster_id，滚动/高亮由正文分区承接。 */
  locate: [clusterId: number]
}>()

const lead = computed(() => selectLeadStory(props.report))
const highlights = computed(() => (props.report.highlights || []).slice(0, 3))

/** 每条重点的展示证据（篇来源/聚簇口径/数据截至/研究时点/定位状态；旧数据全缺省 → 保持现状外观）。 */
const evidence = computed<HighlightEvidence[]>(() =>
  highlights.value.map(highlight => highlightEvidence(highlight, props.report)),
)

/** 依据行文案：缺字段的项整体省略（不显示空括号/0/占位符）。 */
function metaText(index: number): string {
  const item = evidence.value[index]
  if (!item) return ''
  return [
    item.articleCount !== null ? `${item.articleCount} 篇来源` : '',
    item.clusterMode ? `聚簇口径 ${item.clusterMode}` : '',
    item.dataAsOf ? `数据截至 ${item.dataAsOf}` : '',
    item.researchAt ? `研究时点 ${item.researchAt}` : '',
    item.locate === 'missing' ? '本期正文暂无对应线索' : '',
  ].filter(Boolean).join(' · ')
}

/** 划选文案不触发定位（保留既有划选纪律：非折叠选区时点击只当文本操作）。 */
function onLocate(clusterId: number | null | undefined): void {
  if (typeof clusterId !== 'number') return
  const selection = typeof window !== 'undefined' ? window.getSelection() : null
  if (selection && !selection.isCollapsed) return
  emit('locate', clusterId)
}
</script>

<template>
  <header class="drm-masthead" data-testid="daily-report-masthead">
    <div class="drm-masthead__mast">
      <div class="drm-masthead__eyebrow">Syntopica · 每日脉络</div>
      <h1 class="drm-masthead__title">{{ boardTitle || report.title }}</h1>
      <div v-if="boardTitle" class="drm-masthead__sub">{{ report.title }}</div>
      <div class="drm-masthead__meta">
        <span>{{ formatMagazineDate(report.period_date) }}</span>
        <span class="drm-masthead__sep" aria-hidden="true">·</span>
        <span>{{ report.article_count }} 篇文章</span>
        <span class="drm-masthead__sep" aria-hidden="true">·</span>
        <span>{{ report.cluster_count }} 个话题</span>
      </div>
    </div>

    <section v-if="lead" id="report-lead" class="drm-lead" aria-labelledby="drm-lead-title">
      <div class="drm-lead__kicker">北京时间 · {{ formatMagazineDate(report.period_date) }}</div>
      <h2 id="drm-lead-title" class="drm-lead__title">{{ lead.title }}</h2>
      <!-- data-mn-anchor="lead"：头条 lead 摘要开放划选批注（纯 p 无按钮冲突） -->
      <p v-if="lead.summary" class="drm-lead__summary" data-mn-anchor="lead">{{ lead.summary }}</p>
    </section>

    <!-- 今日重点：正常态（≥1 条）与空态（null / [] / 字段缺失）互斥渲染，
         任一时刻只有一个分支存在 → id="report-highlights" 不会重复。
         retire-story-adopt-event-metrics 3.3：可定位条目标题为键盘可达文字按钮（仅可定位时才有按钮），
         依据行给「N 篇来源 / 聚簇口径 / 数据截至日 / 研究时点」快照；缺字段整体省略，旧数据外观不变。 -->
    <section v-if="highlights.length" id="report-highlights" class="drm-highlights" aria-label="今日重点">
      <article v-for="(highlight, index) in highlights" :key="`${highlight.title}-${index}`" class="drm-highlight">
        <span class="drm-highlight__index">N°{{ String(index + 1).padStart(2, '0') }}</span>
        <h3>
          <button
            v-if="evidence[index]!.locate === 'ready'"
            type="button"
            class="drm-highlight__locate"
            :data-testid="`highlight-locate-${index}`"
            :aria-label="`定位正文：${highlight.title}`"
            title="定位到本期正文中该聚簇的第一条线索"
            @click="onLocate(highlight.cluster_id)"
          >{{ highlight.title }}</button>
          <template v-else>{{ highlight.title }}</template>
        </h3>
        <p>{{ highlight.reason }}</p>
        <p
          v-if="metaText(index)"
          class="drm-highlight__meta"
          :data-testid="`highlight-meta-${index}`"
        >
          {{ metaText(index) }}
        </p>
      </article>
    </section>
    <section v-else id="report-highlights" class="drm-highlights drm-highlights--empty" aria-label="今日重点">
      <!-- 空态占位：纯静态文本（无按钮 / 无跳转 / 无 data-mn-anchor），不进划选批注、不进目录。 -->
      <p class="drm-highlights__empty" data-testid="highlights-empty">本期暂无重点</p>
    </section>
  </header>
</template>

<style scoped>
.drm-masthead {
  padding: clamp(2rem, 5vw, 4.5rem) clamp(1rem, 4vw, 4rem) 0;
  color: var(--color-text-primary);
  font-family: "Noto Serif SC", serif;
}

.drm-masthead__mast {
  text-align: center;
  padding-bottom: 1.125rem;
  border-bottom: 3px double var(--color-border-strong);
  animation: drmInkFade 0.7s cubic-bezier(0.2, 0.7, 0.3, 1) both;
}

.drm-masthead__eyebrow {
  margin-bottom: 0.5rem;
  color: var(--color-accent);
  font-size: 0.82rem;
  font-style: italic;
  letter-spacing: 0.15em;
  text-align: center;
  text-transform: uppercase;
}

.drm-masthead__title {
  margin: 0 auto;
  max-width: 82rem;
  font-size: clamp(2.5rem, 8vw, 8rem);
  font-weight: 900;
  letter-spacing: -0.02em;
  line-height: 1.02;
  overflow-wrap: anywhere;
  text-align: center;
  text-shadow: 0 0.5px 0 color-mix(in srgb, var(--color-text-primary) 6%, transparent);
  white-space: nowrap;
}

:global([data-theme="dark"]) .drm-masthead__title {
  text-shadow: none;
}

.drm-masthead__sub {
  margin-top: 0.35rem;
  color: var(--color-text-muted);
  font-style: italic;
  font-size: 0.95rem;
}

.drm-masthead__meta {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.6rem;
  margin-top: 0.875rem;
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.68rem;
  letter-spacing: 0.09em;
  text-transform: uppercase;
}

.drm-masthead__sep {
  color: var(--color-accent);
  font-weight: 700;
}

.drm-lead {
  max-width: 72ch;
  margin: clamp(2.5rem, 6vw, 5rem) auto 0;
  text-align: center;
  animation: drmInkFade 0.7s cubic-bezier(0.2, 0.7, 0.3, 1) both;
  animation-delay: 0.12s;
}

.drm-lead__kicker {
  color: var(--color-accent);
  font-size: 0.82rem;
  font-style: italic;
  letter-spacing: 0.08em;
}

.drm-lead__kicker::before,
.drm-lead__kicker::after {
  margin: 0 0.6rem;
  color: var(--color-text-muted);
  content: "—";
  opacity: 0.6;
}

.drm-lead__title {
  margin: 0.6rem auto 1rem;
  max-width: 22ch;
  font-size: clamp(1.625rem, 3.4vw, 2.5rem);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.18;
  text-wrap: balance;
}

.drm-lead__summary {
  max-width: 72ch;
  margin: 0 auto;
  color: var(--color-text-secondary);
  font-size: clamp(1rem, 1.35vw, 1.18rem);
  font-weight: 300;
  line-height: 1.78;
  text-align: justify;
  column-count: 2;
  column-gap: 3rem;
  column-rule: 1px solid var(--color-border-subtle);
}

.drm-lead__summary::first-letter {
  float: left;
  padding: 0.35rem 0.6rem 0 0;
  color: var(--color-accent);
  font-size: 3.1em;
  font-weight: 900;
  line-height: 0.82;
}

@media (max-width: 980px) {
  .drm-lead__summary {
    column-count: 1;
  }
}

.drm-highlights {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: clamp(2rem, 5vw, 4rem);
  border-top: 1px solid var(--color-border-medium);
  border-bottom: 4px double var(--color-border-strong);
  animation: drmInkFade 0.7s cubic-bezier(0.2, 0.7, 0.3, 1) both;
  animation-delay: 0.2s;
}

.drm-highlight {
  min-width: 0;
  padding: 1.5rem clamp(1rem, 2vw, 2rem);
  border-left: 1px solid var(--color-border-medium);
}

.drm-highlight:first-child {
  border-left: 0;
}

.drm-highlight__index {
  color: var(--color-accent);
  font-size: 0.68rem;
  font-style: italic;
  letter-spacing: 0.12em;
}

.drm-highlight h3 {
  margin: 0.7rem 0 0.45rem;
  font-size: 1rem;
  font-weight: 600;
  line-height: 1.55;
}

/* 重点定位文字按钮（surface-story-heat-in-report-top）：继承标题排版，只加克制可点暗示；
   不新增色值，复用主题 token；焦点轮廓走浏览器默认（不抹除）。 */
.drm-highlight__locate {
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  font-weight: inherit;
  line-height: inherit;
  text-align: inherit;
  cursor: pointer;
  text-decoration: underline dotted;
  text-decoration-color: var(--color-border-strong);
  text-underline-offset: 3px;
}

.drm-highlight__locate:hover,
.drm-highlight__locate:focus-visible {
  color: var(--color-accent);
}

/* 「进展」徽标：昨日重点再次入选（服务端派生）；克制边框胶囊，非颜色唯一载体（文字可读）。 */
.drm-highlight__progress {
  margin-left: 0.4rem;
  padding: 0.05rem 0.45rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 999px;
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.6rem;
  font-weight: 400;
  letter-spacing: 0.08em;
  vertical-align: 2px;
  white-space: nowrap;
}

/* 来源数快照 + 降级提示行：真实 0 显示 0，快照缺失整行不渲染（不伪造 undefined/占位）。 */
.drm-highlight p.drm-highlight__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
  margin: 0.45rem 0 0;
  color: var(--color-text-muted);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.66rem;
  letter-spacing: 0.04em;
}

.drm-highlight p {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: 0.78rem;
  line-height: 1.7;
}

/* 空态（highlights 为 null / [] / 字段缺失）：单列弱化文字，占据原三栏区域的位置。
   边框语言沿用 .drm-highlights（上下双线），只复用既有 CSS 变量，不引入新色值。 */
.drm-highlights--empty {
  grid-template-columns: 1fr;
}

.drm-highlights__empty {
  margin: 0;
  padding: 1.5rem clamp(1rem, 2vw, 2rem);
  color: var(--color-text-secondary);
  font-size: 0.82rem;
  font-style: italic;
  letter-spacing: 0.08em;
  text-align: center;
}

@media (max-width: 720px) {
  .drm-masthead {
    padding-top: 4.5rem;
  }

  .drm-masthead__title {
    font-size: clamp(2.5rem, 13vw, 4.2rem);
    letter-spacing: -0.05em;
    white-space: normal;
  }

  .drm-highlights {
    grid-template-columns: 1fr;
  }

  .drm-highlight,
  .drm-highlight:first-child {
    border-top: 1px solid var(--color-border-medium);
    border-left: 0;
  }

  .drm-highlight:first-child {
    border-top: 0;
  }
}

@keyframes drmInkFade {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (prefers-reduced-motion: reduce) {
  .drm-masthead__mast,
  .drm-lead,
  .drm-highlights {
    animation: none;
  }
}
</style>
