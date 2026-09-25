import type { MarginNoteAnnotation } from '~/api/marginNotes'

/**
 * 批注锚定与正文高亮渲染（daily-report-margin-notes design D1/D5）。
 *
 * 锚定 = quoted_text + 归属 id（section/thread，lead 批注 thread_id=null）+ 字符偏移线索；
 * 日报 period 不可变保证可重放定位，整份重建致偏移失配时按归一化文本模糊匹配兜底，
 * 全失配 → 卡片「原文已变更」降级态（PR-3）。定位容器的 data-mn-anchor 属性是唯一反查键，
 * mark → 卡反查用 data-jump 属性，不假设任何 id 拼接规律。
 */

/** 锚定容器标记属性：thread 摘要 = `t:<threadId>`，头条 lead 摘要 = `lead`。 */
export const MN_ANCHOR_ATTR = 'data-mn-anchor'
/** mark → 卡反查属性（mark 与卡片各存一份 annotation id）。 */
export const MN_JUMP_ATTR = 'data-jump'
/** 正文高亮 mark 类名。 */
export const MN_MARK_CLASS = 'mn-highlight'

export function mnAnchorKey(annotation: Pick<MarginNoteAnnotation, 'thread_id'>): string {
  return annotation.thread_id != null ? `t:${annotation.thread_id}` : 'lead'
}

/** 卡片锚定解析结果：marked=高亮已落；unmarked=找到原文但跨节点包裹失败（WB-2 只落卡不高亮）；changed=原文已变更。 */
export type AnchorResolution = 'marked' | 'unmarked' | 'changed'

export interface AnchorApplyResult {
  /** annotationId → 解析结果 */
  resolutions: Map<number, AnchorResolution>
}

interface NormalizedText {
  norm: string
  /** norm 中每个字符在原文中的下标（空白被剔除）。 */
  map: number[]
}

function normalizeWithMap(text: string): NormalizedText {
  const map: number[] = []
  let norm = ''
  for (let i = 0; i < text.length; i++) {
    const ch = text.charAt(i)
    if (/\s/.test(ch)) continue
    norm += ch
    map.push(i)
  }
  return { norm, map }
}

/** 在原文中定位 quoted：优先偏移线索精确匹配，失配回退全文精确，再回退归一化模糊。 */
export function locateQuotedText(text: string, quoted: string, offsetHint: number | null): { start: number; end: number } | null {
  if (!quoted) return null
  if (offsetHint != null && offsetHint >= 0 && offsetHint < text.length) {
    const idx = text.indexOf(quoted, offsetHint)
    if (idx >= 0) return { start: idx, end: idx + quoted.length }
  }
  const fromStart = text.indexOf(quoted)
  if (fromStart >= 0) return { start: fromStart, end: fromStart + quoted.length }

  const src = normalizeWithMap(text)
  const needle = normalizeWithMap(quoted)
  if (!needle.norm) return null
  const found = src.norm.indexOf(needle.norm)
  if (found < 0) return null
  const start = src.map[found]!
  const endChar = src.map[found + needle.norm.length - 1]!
  return { start, end: endChar + 1 }
}

/** 用 mark 包裹 range；跨节点选区 surroundContents 抛错时返回 false（降级只落卡不高亮，不中断）。 */
export function wrapRangeWithMark(range: Range, annotationId: number): boolean {
  const mark = document.createElement('mark')
  mark.className = MN_MARK_CLASS
  mark.dataset.jump = String(annotationId)
  try {
    range.surroundContents(mark)
    return true
  } catch {
    return false
  }
}

function containerText(node: Node): string {
  return node.textContent ?? ''
}

/** 在容器内定位并包裹既有批注的引用文本。容器缺失/全文失配 → changed。 */
function resolveAnnotationInContainer(container: Element, annotation: MarginNoteAnnotation): AnchorResolution {
  const existing = container.querySelectorAll(`mark.${MN_MARK_CLASS}[${MN_JUMP_ATTR}="${annotation.id}"]`)
  if (existing.length > 0) return 'marked'

  const text = containerText(container)
  const located = locateQuotedText(text, annotation.quoted_text, annotation.anchor_offset_start)
  if (!located) return 'changed'

  const owner = document.createTreeWalker(container, NodeFilter.SHOW_TEXT)
  let walked = 0
  let startNode: Text | null = null
  let startOffset = 0
  let endNode: Text | null = null
  let endOffset = 0
  while (owner.nextNode()) {
    const node = owner.currentNode as Text
    const len = node.data.length
    if (!startNode && walked + len > located.start) {
      startNode = node
      startOffset = located.start - walked
    }
    if (!endNode && walked + len >= located.end) {
      endNode = node
      endOffset = located.end - walked
      break
    }
    walked += len
  }
  if (!startNode || !endNode) return 'changed'

  const range = document.createRange()
  range.setStart(startNode, startOffset)
  range.setEnd(endNode, endOffset)
  return wrapRangeWithMark(range, annotation.id) ? 'marked' : 'unmarked'
}

/**
 * 把批注高亮应用到正文（幂等：已存在的同 id mark 跳过）。
 * 先清理 data-jump 不在现存批注集合内的残留 mark（删除批注/乐观临时 id 替换后自愈），
 * 再按 [data-mn-anchor] 容器索引逐条落 mark；批注归属容器缺失（如 thread 不在当前
 * 分区渲染）按 changed 处理，卡片仍正常显示引用文本。
 */
export function applyMarginNoteHighlights(root: HTMLElement, annotations: MarginNoteAnnotation[]): AnchorApplyResult {
  const resolutions = new Map<number, AnchorResolution>()
  const containers = new Map<string, Element>()
  // root 自身也可能是锚定容器（querySelectorAll 只匹配后代）
  if (root.matches?.(`[${MN_ANCHOR_ATTR}]`)) {
    containers.set(root.getAttribute(MN_ANCHOR_ATTR) ?? '', root)
  }
  for (const el of root.querySelectorAll(`[${MN_ANCHOR_ATTR}]`)) {
    containers.set(el.getAttribute(MN_ANCHOR_ATTR) ?? '', el)
  }

  const validIds = new Set(annotations.map(item => String(item.id)))
  for (const mark of [...root.querySelectorAll(`mark.${MN_MARK_CLASS}[${MN_JUMP_ATTR}]`)]) {
    if (!validIds.has(mark.getAttribute(MN_JUMP_ATTR) ?? '')) {
      mark.replaceWith(...mark.childNodes)
      mark.parentElement?.normalize()
    }
  }

  for (const annotation of annotations) {
    const container = containers.get(mnAnchorKey(annotation))
    resolutions.set(annotation.id, container ? resolveAnnotationInContainer(container, annotation) : 'changed')
  }
  return { resolutions }
}

/** 从当前选区构造落锚载荷：选区必须起止于同一 [data-mn-anchor] 容器内。 */
export function buildAnchorPayloadFromSelection(selection: Selection, reportRoot: HTMLElement): {
  key: string
  quotedText: string
  start: number
  end: number
  range: Range
} | null {
  if (selection.isCollapsed || selection.rangeCount === 0) return null
  const quoted = selection.toString().trim()
  if (quoted.length < 2) return null
  const range = selection.getRangeAt(0)
  const startEl = (range.startContainer.nodeType === Node.TEXT_NODE ? range.startContainer.parentElement : range.startContainer as HTMLElement)?.closest(`[${MN_ANCHOR_ATTR}]`)
  const endEl = (range.endContainer.nodeType === Node.TEXT_NODE ? range.endContainer.parentElement : range.endContainer as HTMLElement)?.closest(`[${MN_ANCHOR_ATTR}]`)
  if (!startEl || startEl !== endEl) return null
  if (!reportRoot.contains(startEl)) return null

  const container = startEl as HTMLElement
  const text = containerText(container)
  // 选区相对容器的偏移：把容器文本按前半截断取长度（对选区首尾所在 text 节点求原点）
  const start = selectionOffsetInContainer(container, range.startContainer, range.startOffset)
  const end = start + range.toString().length
  if (start < 0 || end > text.length) return null
  return {
    key: container.getAttribute(MN_ANCHOR_ATTR) ?? '',
    quotedText: range.toString(),
    start,
    end,
    range: range.cloneRange(),
  }
}

function selectionOffsetInContainer(container: Node, node: Node, offset: number): number {
  const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT)
  let walked = 0
  while (walker.nextNode()) {
    const current = walker.currentNode
    if (current === node) return walked + offset
    walked += (current.textContent ?? '').length
  }
  return -1
}

/** 解析选区归属的锚定容器 key（无有效归属返回 null）。 */
export function selectionAnchorKey(selection: Selection | null, reportRoot: HTMLElement): string | null {
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) return null
  const anchorNode = selection.anchorNode
  if (!anchorNode) return null
  const host = (anchorNode.nodeType === Node.TEXT_NODE ? anchorNode.parentElement : anchorNode as HTMLElement)?.closest(`[${MN_ANCHOR_ATTR}]`)
  if (!host || !reportRoot.contains(host)) return null
  return host.getAttribute(MN_ANCHOR_ATTR)
}
