<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ArticleContentView } from '~/features/articles/public'
import { popOverlay, pushOverlay } from '~/components/ui/overlayStack'
import { formatRelationScore, relationLevelLabel } from '~/utils/threadRelation'
import type { RelatedPreviewContext } from '~/utils/threadRelation'
import type { Article } from '~/types'

const props = withDefaults(defineProps<{
  visible: boolean
  selectedPreviewArticle: Article | null
  previewArticles: Article[]
  loadingPreviewArticle: boolean
  /** 搜索预览命中词高亮（add-article-search）：传入时正文文本节点包裹 <mark class="apm-hit">；
   *  缺省零行为变化（tags 域既有用法不受影响） */
  highlightTerms?: string[]
  /** R38：仅 matrix 原文阅读显式 opt-in 对话语义/初始焦点；默认 false 保持旧行为。 */
  dialogSemantics?: boolean
  /** 线索相关推荐的阅读上下文（replace-lane-lifeline-with-topic-events R-1）：只在从
   *  相关线索 chip 打开时传入，弹窗头部多一行档位/分数/候选日期；普通文章行为 null。 */
  relatedContext?: RelatedPreviewContext | null
}>(), {
  highlightTerms: () => [],
  dialogSemantics: false,
  relatedContext: null,
})
const emit = defineEmits<{
  close: []
  navigate: [article: Article]
  favorite: [articleId: string]
  'article-update': [articleId: string, updates: Partial<Article>]
}>()

const show = computed({
  get: () => props.visible,
  set: (val: boolean) => { if (!val) emit('close') }
})

/** 相关度上下文文案（档位 + 分数 + 候选日期）；无上下文时整行不渲染。 */
const relatedContextText = computed(() => {
  const context = props.relatedContext
  if (!context) return ''
  return `相关线索 · ${relationLevelLabel(context.level)} ${formatRelationScore(context.score)} · ${context.periodDate}`
})

/**
 * 层级约定（2026-10-28 用户反馈修复）：
 * 弹窗遮罩 9100（同 MarginNotesRail 同源约定）→ 全屏层必须高于它，
 * 否则弹窗内点「全屏」时全屏层（Teleport 到 body）被遮罩盖住——看起来像没反应；
 * 全屏层取 9200，仍低于全局 toast（9999），保证全屏中失败提示可见。
 */
const DIALOG_Z_INDEX = 9100
const FULLSCREEN_Z_INDEX = 9200

// ===== 命中词高亮（add-article-search）=====
// 实现方式：渲染后对 .preview-body 内文本节点包裹 mark（DOM 层，不侵入
// ArticleContentView/ArticleContentPreviewPanel 渲染链，tags 域与阅读页零改动）。
const bodyRef = ref<HTMLElement | null>(null)

/** 去重 + 去空白 + 长词优先（长词先匹配，避免「特斯拉电池」被「电池」拆碎嵌套） */
const activeTerms = computed(() => {
  const seen = new Set<string>()
  for (const raw of props.highlightTerms ?? []) {
    const term = raw.trim()
    if (term) seen.add(term)
  }
  return [...seen].sort((a, b) => b.length - a.length)
})

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function buildPattern(): RegExp | null {
  if (!activeTerms.value.length) return null
  try {
    return new RegExp(`(${activeTerms.value.map(escapeRegExp).join('|')})`, 'gi')
  } catch {
    return null
  }
}

const SKIP_TAGS = new Set(['MARK', 'SCRIPT', 'STYLE', 'IFRAME', 'SVG', 'TEXTAREA'])

function collectTextNodes(root: HTMLElement): Text[] {
  const found: Text[] = []
  const walk = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      const parent = (node as Text).parentElement
      if (parent && !parent.closest('mark') && node.textContent && node.textContent.trim()) {
        found.push(node as Text)
      }
      return
    }
    if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node as Element
      if (SKIP_TAGS.has(el.tagName)) return
      el.childNodes.forEach(walk)
    }
  }
  walk(root)
  return found
}

function applyHighlights(): void {
  const root = bodyRef.value
  const pattern = buildPattern()
  if (!root || !pattern) return
  for (const textNode of collectTextNodes(root)) {
    const text = textNode.textContent ?? ''
    pattern.lastIndex = 0
    if (!pattern.test(text)) continue
    pattern.lastIndex = 0
    const fragment = document.createDocumentFragment()
    let cursor = 0
    let match: RegExpExecArray | null
    while ((match = pattern.exec(text)) !== null) {
      if (match.index > cursor) fragment.appendChild(document.createTextNode(text.slice(cursor, match.index)))
      const mark = document.createElement('mark')
      mark.className = 'apm-hit'
      mark.textContent = match[0]
      fragment.appendChild(mark)
      cursor = match.index + match[0].length
      if (match[0].length === 0) pattern.lastIndex += 1
    }
    if (cursor < text.length) fragment.appendChild(document.createTextNode(text.slice(cursor)))
    textNode.parentNode?.replaceChild(fragment, textNode)
  }
}

/**
 * 高亮生命周期（review R-6）：注入的 mark 属渲染后 DOM 操作，弹窗内任何响应式更新
 * （父层替换 selectedPreviewArticle 引用——收藏/article-update/导航；或 ArticleContentView
 * 内部状态重渲染——视图模式/正文源切换、v-html 区被 Vue patch 重设）都会静默丢弃 mark。
 * 双保险：① watch 按引用盯 selectedPreviewArticle（同 id 引用替换也重触发）+ nextTick
 * 重施；② bodyRef 上 MutationObserver 兜底不经 props 的内部重渲染。幂等：collectTextNodes
 * 跳过 mark 内文本，重复应用无嵌套/重复；自身写入期间 disconnect（规范：清空未派发
 * 记录）防自触发循环。断开时机：弹窗关闭、无条款（tags 域零开销零行为差异）、卸载。
 */
let highlightObserver: MutationObserver | null = null
const OBSERVE_OPTIONS: MutationObserverInit = { childList: true, subtree: true, characterData: true }

function stopHighlightObserver(): void {
  highlightObserver?.disconnect()
  highlightObserver = null
}

/** 带自触发防护的应用：断开观察 → 注入 → 重挂（bodyRef 可能因重渲染换节点，重挂即重定向） */
function applyHighlightsObserved(): void {
  const root = bodyRef.value
  if (!root || !buildPattern()) return
  if (!highlightObserver) {
    applyHighlights()
    return
  }
  highlightObserver.disconnect()
  applyHighlights()
  highlightObserver.observe(root, OBSERVE_OPTIONS)
}

function startHighlightObserver(): void {
  const root = bodyRef.value
  if (!root) return
  if (!highlightObserver) {
    highlightObserver = new MutationObserver(() => applyHighlightsObserved())
  }
  highlightObserver.disconnect()
  highlightObserver.observe(root, OBSERVE_OPTIONS)
}

/** Esc 分层锚（daily-report-lead-direct-preview）：阅读台在 document 级监听 Esc 并查 overlayStack 让位
 * ——本弹窗直达打开（无抽屉宿主代为计数）时 MUST 自登浮层栈，否则 Esc 关弹窗的同一次击键会
 * 连阅读台一起退（丢阅读台上下文）。对无 Esc 监听的宿主（矩阵页/搜索页）计数无副作用。 */
let overlayPushed = false
watch(() => props.visible, (visible) => {
  if (visible && !overlayPushed) {
    pushOverlay()
    overlayPushed = true
  } else if (!visible && overlayPushed) {
    popOverlay()
    overlayPushed = false
  }
}, { immediate: true })

watch(
  () => [props.visible, props.selectedPreviewArticle, activeTerms.value.join('\u0000')] as const,
  async () => {
    if (!props.visible || !props.selectedPreviewArticle || !activeTerms.value.length) {
      stopHighlightObserver()
      return
    }
    await nextTick()
    if (!props.visible || !props.selectedPreviewArticle) return
    startHighlightObserver()
    applyHighlightsObserved()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  stopHighlightObserver()
  if (overlayPushed) {
    popOverlay()
    overlayPushed = false
  }
})
</script>

<template>
  <AppDialog
    v-model="show"
    width="90vw"
    :close-on-overlay="true"
    :close-on-escape="true"
    :z-index="DIALOG_Z_INDEX"
    :semantic-dialog="dialogSemantics"
    dialog-label="站内原文阅读"
  >
    <template #header>
      <div class="preview-header-main">
        <p class="preview-header-text">
          {{ loadingPreviewArticle ? '正在准备文章预览...' : '文章预览' }}
        </p>
        <!-- 相关线索上下文（替换旧生命线后 chip 是唯一入口）：只在 chip 点击时出现，
             普通文章行不渲染该行（零行为变化）。 -->
        <p
          v-if="relatedContextText"
          class="preview-header-related"
          data-testid="preview-related-context"
        >
          {{ relatedContextText }}
        </p>
      </div>
      <!-- 可选 header-actions（add-article-search D1）：AppDialog header 本身是 flex 行，
           margin-left:auto 使动作落在标题行右侧、关闭钮之前；不传插槽时本节点不渲染，
           header DOM 与现状完全一致（tags 域零变化红线） -->
      <div v-if="$slots['header-actions']" class="preview-header-actions">
        <slot name="header-actions" />
      </div>
    </template>
    <!-- 确定高度：85vh（AppDialog 上限）− header ≈61px − body 上下 padding 40px，留 9px 余量（宁小勿大防双滚动条）；
         AppDialog body 非 flex，flex:1 解析不出高度，下游 h-full/flex:1 链（iframe 模式）依赖此确定高度 → change fix-preview-dialog-iframe-height D1 -->
    <div ref="bodyRef" class="preview-body" :style="{ height: 'calc(85vh - 110px)' }">
      <ArticleContentView
        v-if="selectedPreviewArticle"
        :article="selectedPreviewArticle"
        :articles="previewArticles"
        :fullscreen-z-index="FULLSCREEN_Z_INDEX"
        @navigate="(a: Article) => emit('navigate', a)"
        @favorite="(id: string) => emit('favorite', id)"
        @article-update="(id: string, updates: Partial<Article>) => emit('article-update', id, updates)"
      />
    </div>
  </AppDialog>
</template>

<style scoped>
/* 标题 + 相关度上下文两行（header 本身是 flex 行：本块靠左，header-actions 靠右） */
.preview-header-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.preview-header-text {
  font-size: 0.875rem;
  color: var(--color-text-muted);
}

.preview-header-related {
  margin: 0;
  color: var(--color-text-secondary);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.7rem;
  font-variant-numeric: tabular-nums;
}

/* header-actions 容器（add-article-search）：仅插槽传入时渲染 */
.preview-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
</style>

<style>
/* 搜索命中词高亮（add-article-search）：mark 是渲染后 DOM 注入的，无 scoped 属性，
   须全局类。warning-subtle 语义 token，editorial/dark 双主题自适应（零特判）。 */
mark.apm-hit {
  background: var(--color-warning-subtle);
  color: inherit;
  border-radius: 2px;
  padding: 0 1px;
}
</style>
