<script setup lang="ts">
import { computed } from 'vue'
import { formatMagazineDate, selectLeadStory } from './dailyReportMagazine'
import type { DailyReport } from '~/api/dailyReports'

const props = defineProps<{
  report: DailyReport
  boardTitle?: string
}>()

const lead = computed(() => selectLeadStory(props.report))
const highlights = computed(() => (props.report.highlights || []).slice(0, 3))
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

    <section v-if="highlights.length" id="report-highlights" class="drm-highlights" aria-label="今日重点">
      <article v-for="(highlight, index) in highlights" :key="`${highlight.title}-${index}`" class="drm-highlight">
        <span class="drm-highlight__index">N°{{ String(index + 1).padStart(2, '0') }}</span>
        <h3>{{ highlight.title }}</h3>
        <p>{{ highlight.reason }}</p>
      </article>
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

.drm-highlight p {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: 0.78rem;
  line-height: 1.7;
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
