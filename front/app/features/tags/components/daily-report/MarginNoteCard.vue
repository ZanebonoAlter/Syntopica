<script setup lang="ts">
import { computed, ref } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppInput from '~/components/ui/AppInput.vue'
import type { MarginNoteAnnotation } from '~/api/marginNotes'
import type { MarginNoteAskState } from '~/features/tags/composables/useMarginNotes'

/**
 * 页边注批注卡（daily-report-margin-notes design D5 / specs 划段批注锚定 + 页边注问答）：
 * 引用摘要（2 行截断 + 独立常显「⌄ 展开全文」提示行，2026-09-22 批准要求：提示不得藏进
 * line-clamp 截断盒被吃掉）+ QA 轮列表（追加、历史保留）+ 提问输入（loading 锁定、
 * 失败行内重试不丢问题）+「↗ 跳回原文」「✕ 删除」常显。mark 反查靠根节点
 * data-annotation-id 属性（data-jump 对应），不假设任何 id 拼接规律。
 */
const props = defineProps<{
  note: MarginNoteAnnotation
  /** 被点名（mark 跳卡 / 深链定位）的点亮态。 */
  lit?: boolean
  /** 原文已变更（引用文本在正文失配，PR-3）：卡片头显示降级提示，跳回原文不可用。 */
  changed?: boolean
  /** 删除确认中：卡片半透明。 */
  deleting?: boolean
  askState: MarginNoteAskState
}>()

const emit = defineEmits<{
  jump: []
  removeRequest: []
  ask: [question: string]
  retryAsk: []
  openCited: [articleId: number]
}>()

const expanded = ref(false)
const question = ref('')
const questionInput = ref<{ $el: HTMLElement } | null>(null)

const asking = computed(() => props.askState.pendingQuestion != null)
/** 确认弹窗口中的问答轮数（含进行中的一轮，明示连带删除范围）。 */
const qaCount = computed(() => props.note.qas.length + (asking.value ? 1 : 0))

/** D7：网络来源 chip 文案 —— 标题缺失时回退域名（URL 不可用时回退原文，防御非标准 URL）。 */
function webLabel(web: { title: string; url: string }): string {
  if (web.title) return web.title
  try {
    return new URL(web.url).hostname
  } catch {
    return web.url
  }
}

function submit() {
  const trimmed = question.value.trim()
  if (!trimmed) {
    questionInput.value?.$el?.querySelector('input')?.focus()
    return
  }
  if (asking.value) return
  emit('ask', trimmed)
  question.value = ''
}

defineExpose({
  /** 新落锚卡自动聚焦问题输入（specs 划选落锚 Scenario）。 */
  focusInput() {
    questionInput.value?.$el?.querySelector('input')?.focus()
  },
})
</script>

<template>
  <article
    class="mn-card"
    :class="{ 'mn-card--lit': lit, 'mn-card--deleting': deleting }"
    :data-annotation-id="note.id"
    :data-qa-count="qaCount"
  >
    <div class="mn-card__quote" :class="{ 'mn-card__quote--expanded': expanded }" @click="expanded = !expanded">
      <span class="mn-card__quote-mark" aria-hidden="true">❝</span>
      <span class="mn-card__quote-text">{{ note.quoted_text }}</span>
    </div>
    <!-- 独立常显展开提示行（批准要求：不藏在截断盒内） -->
    <button
      type="button"
      class="mn-card__toggle"
      :aria-expanded="expanded"
      data-testid="mn-quote-toggle"
      @click.stop="expanded = !expanded"
    >
      {{ expanded ? '⌃ 收起' : '⌄ 展开全文' }}
    </button>

    <header class="mn-card__ops">
      <button
        v-if="!changed"
        type="button"
        class="mn-card__jump"
        title="跳回原文"
        data-testid="mn-jump"
        @click.stop="emit('jump')"
      >
        ↗
      </button>
      <button
        type="button"
        class="mn-card__del"
        title="删除批注（连带问答）"
        data-testid="mn-delete"
        @click.stop="emit('removeRequest')"
      >
        ✕
      </button>
    </header>

    <p v-if="changed" class="mn-card__changed" data-testid="mn-changed">原文已变更（日报重建后引用文本未命中）</p>

    <div class="mn-card__body">
      <div v-for="qa in note.qas" :key="qa.id" class="mn-qa" :data-qa-id="qa.id">
        <p class="mn-qa__q">{{ qa.question }}</p>
        <p class="mn-qa__a">{{ qa.answer }}</p>
        <div v-if="qa.cited_article_ids.length" class="mn-qa__cites">
          <span class="mn-qa__cites-label">基于 {{ qa.cited_article_ids.length }} 篇当天文章</span>
          <button
            v-for="articleId in qa.cited_article_ids"
            :key="articleId"
            type="button"
            class="mn-cite-chip"
            :data-cited-article="articleId"
            @click.stop="emit('openCited', articleId)"
          >
            <Icon icon="mdi:file-document-outline" width="11" />
            文章 #{{ articleId }}
          </button>
        </div>
        <!-- D7 联网来源：与站内引用分行，外链新窗口（target=_blank rel=noopener）。 -->
        <div v-if="qa.cited_web_sources?.length" class="mn-qa__webs" data-testid="mn-web-sources">
          <span class="mn-qa__cites-label">网络来源</span>
          <a
            v-for="web in qa.cited_web_sources"
            :key="web.url"
            class="mn-web-chip"
            :href="web.url"
            target="_blank"
            rel="noopener"
            :title="web.title || web.url"
            @click.stop
          >
            <Icon icon="mdi:open-in-new" width="11" />
            {{ webLabel(web) }}
          </a>
        </div>
        <!-- D7 口径：本地引用与网络来源均空才标纯模型知识。 -->
        <p
          v-if="!qa.cited_article_ids.length && !qa.cited_web_sources?.length"
          class="mn-qa__pure"
          data-testid="mn-pure-model"
        >纯模型知识 · 无当天文章引用</p>
        <div v-if="qa.extracted_terms.length" class="mn-qa__terms">
          <span class="mn-qa__terms-label">本次涉及</span>
          <span
            v-for="term in qa.extracted_terms"
            :key="term.term"
            class="mn-term-chip"
            :class="{ 'mn-term-chip--new': term.is_new }"
          >{{ term.term }}<i v-if="term.is_new" class="mn-term-chip__badge">入库</i></span>
        </div>
      </div>

      <!-- 生成中：骨架呼吸行（pending 问题保留展示） -->
      <div v-if="askState.pendingQuestion" class="mn-qa mn-qa--pending" data-testid="mn-qa-pending">
        <p class="mn-qa__q">{{ askState.pendingQuestion }}</p>
        <div class="mn-skeleton" aria-live="polite">
          <span>正在翻阅当天文章……</span>
          <i /><i /><i />
        </div>
      </div>

      <!-- 失败：行内错误 + 重试（问题文本保留） -->
      <div v-if="askState.error" class="mn-qa__error" data-testid="mn-qa-error" role="alert">
        <span>{{ askState.error }}：「{{ askState.failedQuestion }}」</span>
        <AppButton size="sm" data-testid="mn-qa-retry" @click="emit('retryAsk')">重试</AppButton>
      </div>

      <form class="mn-card__input" data-testid="mn-ask-form" @submit.prevent="submit">
        <AppInput
          ref="questionInput"
          v-model="question"
          :placeholder="note.qas.length ? '继续追问……' : '问点什么，比如：这段什么意思？'"
          :disabled="asking"
          class="mn-card__input-field"
        />
        <AppButton type="submit" size="sm" :loading="asking" :disabled="asking" data-testid="mn-ask-submit">问</AppButton>
      </form>
    </div>
  </article>
</template>

<style scoped>
.mn-card {
  position: relative;
  margin-bottom: 0.9rem;
  overflow: hidden;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-bg-hover);
  animation: mnCardIn 0.3s ease;
}

.mn-card--lit {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px var(--color-accent-subtle);
}

.mn-card--deleting {
  opacity: 0.45;
}

.mn-card__quote {
  position: relative;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  overflow: hidden;
  padding: 0.55rem 3.4rem 0.55rem 0.9rem;
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  font-family: "Noto Serif SC", serif;
  font-size: 0.78rem;
  line-height: 1.6;
  cursor: pointer;
}

.mn-card__quote--expanded {
  display: block;
  -webkit-line-clamp: unset;
}

.mn-card__quote-text {
  overflow-wrap: anywhere;
}

.mn-card__quote-mark {
  position: absolute;
  top: 0.35rem;
  left: 0.25rem;
  color: var(--color-accent);
  font-size: 0.7rem;
  opacity: 0.6;
}

/* 展开提示行：独立于截断盒之外常显（2026-09-22 批准反馈） */
.mn-card__toggle {
  display: block;
  width: 100%;
  padding: 0.28rem 0.6rem 0.35rem;
  border: 0;
  border-bottom: 1px solid var(--color-border-subtle);
  background: var(--color-bg-elevated);
  color: var(--color-accent);
  font-size: 0.66rem;
  letter-spacing: 0.05em;
  text-align: right;
  cursor: pointer;
}

.mn-card__toggle:hover {
  color: var(--color-accent-hover);
}

.mn-card__ops {
  position: absolute;
  top: 0.3rem;
  right: 0.35rem;
  z-index: 2;
  display: flex;
  gap: 0.2rem;
}

.mn-card__jump,
.mn-card__del {
  padding: 0.2rem 0.35rem;
  border: 0;
  border-radius: 4px;
  background: var(--color-bg-hover);
  color: var(--color-text-muted);
  font-size: 0.72rem;
  line-height: 1;
  cursor: pointer;
}

.mn-card__jump:hover {
  color: var(--color-accent);
}

.mn-card__del {
  opacity: 0.45;
  transition: opacity 0.15s;
}

.mn-card:hover .mn-card__del {
  opacity: 1;
}

.mn-card__del:hover {
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.mn-card__changed {
  margin: 0;
  padding: 0.35rem 0.75rem;
  border-bottom: 1px dashed var(--color-border-medium);
  color: var(--color-text-muted);
  font-size: 0.68rem;
}

.mn-card__body {
  padding: 0.65rem 0.75rem;
}

.mn-qa {
  margin-bottom: 0.6rem;
}

.mn-qa__q {
  margin: 0 0 0.3rem;
  color: var(--color-text-primary);
  font-size: 0.78rem;
  font-weight: 600;
}

.mn-qa__q::before {
  content: "问 ";
  color: var(--color-accent);
  font-size: 0.68rem;
  letter-spacing: 0.05em;
}

.mn-qa__a {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: 0.75rem;
  line-height: 1.7;
  text-align: justify;
}

.mn-qa__a::before {
  content: "答 ";
  color: var(--color-text-muted);
  font-size: 0.68rem;
  letter-spacing: 0.05em;
}

.mn-qa__cites {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.3rem;
  margin-top: 0.45rem;
}

.mn-qa__cites-label {
  color: var(--color-text-muted);
  font-size: 0.62rem;
}

.mn-cite-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.1rem 0.45rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 25%, transparent);
  border-radius: 4px;
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-size: 0.64rem;
  cursor: pointer;
}

.mn-cite-chip:hover {
  background: color-mix(in srgb, var(--color-accent) 16%, transparent);
}

/* D7 联网来源 chips：与站内引用分行，中性色系 + 外链标识区分站内/站外。 */
.mn-qa__webs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.3rem;
  margin-top: 0.35rem;
}

.mn-web-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  max-width: 100%;
  padding: 0.1rem 0.45rem;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  color: var(--color-text-secondary);
  font-size: 0.64rem;
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mn-web-chip:hover {
  text-decoration: underline;
  background: color-mix(in srgb, var(--color-text-primary) 5%, transparent);
}

.mn-qa__pure {
  margin: 0.35rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.64rem;
  font-style: italic;
}

.mn-qa__terms {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.3rem;
  margin-top: 0.45rem;
}

.mn-qa__terms-label {
  color: var(--color-text-muted);
  font-size: 0.62rem;
}

.mn-term-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.08rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
  font-size: 0.64rem;
}

.mn-term-chip__badge {
  color: var(--color-accent);
  font-size: 0.56rem;
  font-style: normal;
}

.mn-qa--pending .mn-qa__q {
  color: var(--color-text-secondary);
}

.mn-skeleton span {
  color: var(--color-text-muted);
  font-size: 0.7rem;
}

.mn-skeleton i {
  display: block;
  height: 0.55rem;
  margin-top: 0.3rem;
  border-radius: 3px;
  background: var(--color-bg-sunken);
  animation: mnBreathe 1.2s ease-in-out infinite;
}

.mn-skeleton i:nth-child(3) {
  width: 88%;
  animation-delay: 0.15s;
}

.mn-skeleton i:nth-child(4) {
  width: 62%;
  animation-delay: 0.3s;
}

.mn-qa__error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 0.6rem;
  padding: 0.4rem 0.6rem;
  border: 1px solid color-mix(in srgb, var(--color-accent) 30%, transparent);
  border-radius: 4px;
  background: var(--color-accent-subtle);
  color: var(--color-accent);
  font-size: 0.72rem;
}

.mn-card__input {
  display: flex;
  align-items: flex-end;
  gap: 0.4rem;
  margin-top: 0.5rem;
}

.mn-card__input-field {
  flex: 1;
  min-width: 0;
}

@keyframes mnCardIn {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: none; }
}

@keyframes mnBreathe {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
}

@media (prefers-reduced-motion: reduce) {
  .mn-card,
  .mn-skeleton i {
    animation: none;
  }
}
</style>
