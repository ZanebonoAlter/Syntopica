<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ArticleContentView } from '~/features/articles/public'
import type { Article } from '~/types'

const props = withDefaults(defineProps<{
  visible: boolean
  selectedPreviewArticle: Article | null
  previewArticles: Article[]
  loadingPreviewArticle: boolean
  /** 搜索预览命中词高亮（add-article-search）：传入时正文文本节点包裹 <mark class="apm-hit">；
   *  缺省零行为变化（tags 域既有用法不受影响） */
  highlightTerms?: string[]
}>(), {
  highlightTerms: () => [],
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

onBeforeUnmount(stopHighlightObserver)
</script>

<template>
  <AppDialog v-model="show" width="90vw" :close-on-overlay="true" :close-on-escape="true" :z-index="9100">
    <template #header>
      <p class="preview-header-text">
        {{ loadingPreviewArticle ? '正在准备文章预览...' : '文章预览' }}
      </p>
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
        @navigate="(a: Article) => emit('navigate', a)"
        @favorite="(id: string) => emit('favorite', id)"
        @article-update="(id: string, updates: Partial<Article>) => emit('article-update', id, updates)"
      />
    </div>
  </AppDialog>
</template>

<style scoped>
.preview-header-text {
  font-size: 0.875rem;
  color: var(--color-text-muted);
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
