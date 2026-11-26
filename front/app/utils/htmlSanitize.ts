/**
 * 第三方 HTML 清洗与纯文本抽取（openspec change fix-reader-lede-blank-space，design D4/D5/D6）。
 *
 * 背景：部分订阅源把整篇文章的 HTML 塞进 `articles.description`（实测均值 66 KB、最大 864 KB），
 * 公众号文章的根节点带 `style="visibility: visible; opacity: 0;"` —— 内容整体不可见却照样占位，
 * 阅读页导语段因此出现整屏空白；同一段 HTML 经 `v-html` 直出还带着 `<img onerror=...>` 一类注入面。
 *
 * 两条口径（与后端 `internal/platform/htmltext` 同规则，只允许长度上限不同）：
 * - `sanitizeHtml`：保守清洗，保留可见结构与排版 —— 正文源切换（原文源）用；
 * - `toPlainText`：去标签取纯文本 —— 导语段用，第三方 HTML 不再作为标签进入 DOM。
 *
 * 隐藏分两类处理（design D6）：`sanitizeHtml` 对「视觉隐藏」剥声明留内容、对「语义不展示」移除子树；
 * `toPlainText` 一律按隐藏子树整体剔除（摘要侧不做解隐藏）。
 *
 * 清洗顺序（design D10，与后端 `htmltext.PlainText` 同口径）：纯文本侧「**先解实体 → 再解析 → 取文本**」。
 * 反过来（浏览器语义：先解析后解码）会把源里写成 `&lt;b&gt;` 的转义标记留在纯文本里，
 * 二次清洗时被当标签吞掉 → 不幂等。结构清洗侧（`sanitizeHtml`）解析**原始串**、不解实体，
 * 与后端 `htmltext.SanitizeHTML` 一致。两侧用同一组固定样本断言逐字相同的输出（见 htmlSanitize.test.ts）。
 *
 * 不引入 DOMPurify 等依赖：项目前端目前只有 `marked` 一个渲染相关依赖（design D4 取舍），
 * 浏览器原生 `DOMParser` 足以覆盖本 change 的清洗面；无 `DOMParser` 的环境（如纯 Node 上下文）
 * 走正则降级，绝不把原始 HTML 回吐给渲染层。
 */

/** 「解实体后是否还有标记」——与后端 `markupLikeRe` 同式：`<` + 字母 + 直到 `>`。 */
const MARKUP_LIKE_RE = /<[a-zA-Z][^>]*>/

/** 可执行嵌入元素：脚本容器、表单与框架 —— 渲染前整体移除（连同子树）。 */
const EXECUTABLE_EMBED_SELECTOR = 'iframe, object, embed, form, applet, base, frame, frameset'

/** 可能承载 URL 的属性：危险 scheme 只在这几个属性上有意义。 */
const URL_ATTRIBUTES = ['href', 'src', 'xlink:href', 'action', 'formaction', 'poster', 'data']

/**
 * 剥掉 URL 里的控制字符：浏览器解析 URL 时会忽略它们（`java\t script:` 仍会被当成
 * `javascript:`），判定 scheme 前必须先剥。用逐字符判定而不是正则字符类——
 * ESLint 的 no-control-regex 不允许正则里写控制字符范围。
 */
function stripUrlControlChars(value: string): string {
  let out = ''
  for (const ch of value) {
    const code = ch.codePointAt(0) ?? 0
    if (code > 0x1f && code !== 0x7f) out += ch
  }
  return out
}

/**
 * 隐藏声明的分类：
 * - `visual`：视觉隐藏 —— 内容本身有效，只是被藏起来；剥掉声明后重新可见，元素与内容都保留；
 * - `semantic`：语义不展示 —— 本就不参与展示（常藏 SEO 关键词堆砌），元素连同子树一起移除。
 */
type HiddenKind = 'visual' | 'semantic'

/** 实体还原覆盖的具名实体（HTML 解析器已还原大部分，这里只兜正则降级路径）。 */
const NAMED_ENTITIES: Record<string, string> = {
  nbsp: ' ',
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
}

interface StyleDeclaration {
  prop: string
  value: string
}

/** 拆 `style` 属性为声明列表（自己拆而不走 CSSOM：CSSOM 会重排/丢弃浏览器不认识的声明）。 */
function parseStyleDeclarations(style: string): StyleDeclaration[] {
  return style
    .split(';')
    .map(part => part.trim())
    .filter(Boolean)
    .map((part) => {
      const idx = part.indexOf(':')
      if (idx < 0) return { prop: part.toLowerCase(), value: '' }
      return { prop: part.slice(0, idx).trim().toLowerCase(), value: part.slice(idx + 1).trim() }
    })
}

/** 数值型声明是否为 0（容忍 `0` / `0.0` / `0px` / `0%` / `0 !important`）。 */
function isZeroValue(value: string): boolean {
  const normalized = value.replace(/!\s*important$/i, '').trim().replace(/%$/, '').trim()
  const matched = /^[+-]?(?:\d+\.?\d*|\.\d+)/.exec(normalized)
  if (!matched) return false
  const num = Number(matched[0])
  return Number.isFinite(num) && num === 0
}

/**
 * 单条内联声明的隐藏判定 —— 唯一的隐藏判定入口（其它判定都从这里派生）。
 *
 * - `display:none` → `semantic`（语义不展示，移除子树）；
 * - `visibility:hidden|collapse` / `opacity:0` / `font-size:0` → `visual`（视觉隐藏，剥声明留内容）；
 * - 其余 → 非隐藏。
 */
function hiddenDeclarationKind(decl: StyleDeclaration): HiddenKind | null {
  const value = decl.value.toLowerCase()
  switch (decl.prop) {
    case 'display':
      return value === 'none' ? 'semantic' : null
    case 'visibility':
      return value === 'hidden' || value === 'collapse' ? 'visual' : null
    case 'opacity':
    case 'font-size':
      return isZeroValue(value) ? 'visual' : null
    default:
      return null
  }
}

/** 元素是否「语义不展示」（`display:none` 或带 `hidden` 属性）→ 连子树一起移除。 */
function isSemanticallyHiddenElement(el: Element): boolean {
  if (el.hasAttribute('hidden')) return true
  const style = el.getAttribute('style')
  if (!style) return false
  return parseStyleDeclarations(style).some(decl => hiddenDeclarationKind(decl) === 'semantic')
}

/** 元素是否带任一隐藏声明（视觉隐藏或语义不展示）→ 纯文本侧口径：隐藏子树整体剔除。 */
function isHiddenElement(el: Element): boolean {
  if (el.hasAttribute('hidden')) return true
  const style = el.getAttribute('style')
  if (!style) return false
  return parseStyleDeclarations(style).some(decl => hiddenDeclarationKind(decl) !== null)
}

/** 移除 script/style 元素（含其内容：脚本与样式都不该进入渲染结果）。 */
function removeDangerousElements(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll('script, style'))) {
    el.remove()
  }
}

/**
 * 移除「语义不展示」的元素及其子树（`display:none` / `hidden` 属性）。
 * 这类块本就不参与展示，且常藏 SEO 关键词堆砌，跟着走。
 */
function removeSemanticallyHiddenElements(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll('*'))) {
    // 父节点已被移除时（子树随父一起走），跳过，避免重复处理
    if (!root.contains(el)) continue
    if (isSemanticallyHiddenElement(el)) el.remove()
  }
}

/**
 * 移除带任一隐藏声明的元素及其子树（视觉隐藏 + 语义不展示）—— 纯文本侧口径。
 * 公众号整篇文章被 `opacity: 0` 的根节点裹住时，摘要侧整篇剔除，不做「解隐藏」。
 */
function removeHiddenElements(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll('*'))) {
    if (!root.contains(el)) continue
    if (isHiddenElement(el)) el.remove()
  }
}

/** 移除全部 `on*` 事件属性（`<img onerror=...>` 一类注入面）。 */
function removeEventAttributes(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll('*'))) {
    for (const attr of Array.from(el.attributes)) {
      if (attr.name.toLowerCase().startsWith('on')) el.removeAttribute(attr.name)
    }
  }
}

/**
 * 移除可执行嵌入元素（iframe/object/embed/form/applet/base/frame/frameset）连同子树。
 *
 * `<iframe srcdoc="…">` 会在本站 origin 里执行 srcdoc 里的脚本；`<form>` 能把读者引去外部表单；
 * `<base>` 会改写后续相对链接的基准 —— 三者都不该由源站 HTML 决定。
 */
function removeExecutableEmbeds(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll(EXECUTABLE_EMBED_SELECTOR))) {
    // 父节点已被移除时（子树随父一起走），跳过，避免重复处理
    if (!root.contains(el)) continue
    el.remove()
  }
}

/** 移除 `srcdoc` 属性：它是 iframe 的内联文档，留在 HTML 里就等于留一段可执行载荷。 */
function removeSrcdocAttributes(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll('*'))) {
    if (el.hasAttribute('srcdoc')) el.removeAttribute('srcdoc')
  }
}

/**
 * 过滤危险 URL 方案：属性值去掉控制字符与首尾空白后取 scheme，
 * `javascript:` / `vbscript:` / `data:`（`data:image/*` 除外）一律移除该属性。
 *
 * 判定放在「剥控制字符 + 小写」之后：`java&#9;script:alert(1)` 这类变形同样要拦住。
 */
function removeDangerousUrlAttributes(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll('*'))) {
    for (const attr of Array.from(el.attributes)) {
      if (!URL_ATTRIBUTES.includes(attr.name.toLowerCase())) continue
      if (isDangerousUrl(attr.value)) el.removeAttribute(attr.name)
    }
  }
}

/** 属性值是否用了危险 URL 方案（`data:image/*` 放行——图片不执行脚本）。 */
function isDangerousUrl(value: string): boolean {
  const cleaned = stripUrlControlChars(value).trim().toLowerCase()
  if (cleaned.startsWith('data:')) return !cleaned.startsWith('data:image/')
  return cleaned.startsWith('javascript:') || cleaned.startsWith('vbscript:')
}

/**
 * 剥离存活元素 style 里的「视觉隐藏」声明（visibility:hidden|collapse / opacity:0 / font-size:0），
 * 元素与其内容保留 —— 内容本身有效，只是被藏起来了，剥掉声明后重新可见。
 * 其余声明原样保留；声明被剥光时整个 `style` 属性删掉（不留空壳、不留畸形 CSS）。
 */
function stripVisualHiddenStyleDeclarations(root: ParentNode): void {
  for (const el of Array.from(root.querySelectorAll('*'))) {
    const style = el.getAttribute('style')
    if (!style) continue
    const declarations = parseStyleDeclarations(style)
    const kept = declarations.filter(decl => hiddenDeclarationKind(decl) !== 'visual')
    if (kept.length === declarations.length) continue
    if (kept.length === 0) {
      el.removeAttribute('style')
      continue
    }
    el.setAttribute('style', kept.map(decl => `${decl.prop}: ${decl.value}`).join('; '))
  }
}

/** 解析 HTML 文档；解析失败或环境无 DOMParser 时返回 null（调用方降级）。 */
function parseHtmlDocument(html: string): Document | null {
  if (typeof DOMParser === 'undefined') return null
  try {
    const doc = new DOMParser().parseFromString(html, 'text/html')
    if (!doc?.body) return null
    // 部分实现解析失败时塞 <parsererror> 元素而不是抛异常
    if (doc.querySelector('parsererror')) return null
    return doc
  } catch {
    return null
  }
}

/** 收集文本：标签边界视作空白分隔（与后端「标签替换为空格」的口径一致）。 */
function collectVisibleText(root: Node, parts: string[]): void {
  for (const child of Array.from(root.childNodes)) {
    // 3 = TEXT_NODE，1 = ELEMENT_NODE
    if (child.nodeType === 3) {
      parts.push(child.textContent ?? '')
      continue
    }
    if (child.nodeType !== 1) continue
    const el = child as Element
    if (el.tagName === 'SCRIPT' || el.tagName === 'STYLE') continue
    parts.push(' ')
    collectVisibleText(el, parts)
    parts.push(' ')
  }
}

/** 还原 HTML 实体（具名 + 十进制/十六进制数字实体）。 */
function decodeEntities(text: string): string {
  return text.replace(/&(#x[0-9a-f]+|#\d+|[a-z][a-z0-9]*);/gi, (match: string, entity: string) => {
    const lower = entity.toLowerCase()
    if (lower.startsWith('#x')) {
      const code = Number.parseInt(lower.slice(2), 16)
      return codePointToString(code, match)
    }
    if (lower.startsWith('#')) {
      const code = Number.parseInt(lower.slice(1), 10)
      return codePointToString(code, match)
    }
    return NAMED_ENTITIES[lower] ?? match
  })
}

/**
 * 实体还原（**一次**）：先解实体 → 再解析（design D10，与后端 `htmltext.PlainText` 同口径）。
 *
 * 解法：借浏览器的实体解码表一次性解实体——把 `<` 转义掉再交给 DOMParser，
 * 取文本节点即「只解实体、不把标记当真」的解码结果：
 * - `&amp;lt;` → `&lt;`（不会被二次解码成 `<`）；
 * - `&lt;b&gt;` → `<b>`（随后被解析器当标记处理，与后端同款取舍）。
 *
 * 无 DOM 环境退到正则解码（覆盖面小于 HTML5 全集，属已知降级）。
 */
function decodeEntitiesOnce(raw: string): string {
  if (!raw.includes('&')) return raw
  if (typeof DOMParser === 'undefined') return decodeEntities(raw)
  try {
    const doc = new DOMParser().parseFromString(raw.replace(/</g, '&lt;'), 'text/html')
    return doc.body?.textContent ?? decodeEntities(raw)
  } catch {
    return decodeEntities(raw)
  }
}

/** 数字实体 → 字符；非法码点原样保留。 */
function codePointToString(code: number, fallback: string): string {
  if (!Number.isInteger(code) || code < 0 || code > 0x10ffff) return fallback
  if (code >= 0xd800 && code <= 0xdfff) return fallback
  return String.fromCodePoint(code)
}

/** 正则降级路径：去注释/script/style/标签，取文本（无法识别隐藏子树，属已知降级）。 */
function stripTagsToText(html: string): string {
  return decodeEntities(stripTagsRaw(html))
}

/** 正则降级路径的剥标签部分（不解实体——实体已在解码步骤统一处理过一次）。 */
function stripTagsRaw(html: string): string {
  return html
    .replace(/<!--[\s\S]*?-->/g, ' ')
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, ' ')
    .replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, ' ')
    .replace(/<[^>]*>/g, ' ')
}

/** HTML 上下文转义（降级路径用：绝不把原始 HTML 当文本回吐给 v-html）。 */
function escapeHtml(raw: string): string {
  return raw
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/** 折叠连续空白为单个空格并去首尾空白。 */
function collapseWhitespace(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

/** 按 rune 截断（超长以「…」收尾，总长不超过上限）。 */
function truncateRunes(text: string, maxRunes?: number): string {
  if (maxRunes === undefined) return text
  if (maxRunes <= 0) return ''
  const runes = Array.from(text)
  if (runes.length <= maxRunes) return text
  return `${runes.slice(0, maxRunes - 1).join('')}…`
}

/**
 * 第三方 HTML 保守清洗：移除 script/style、移除可执行嵌入元素（iframe/object/embed/form/
 * applet/base/frame/frameset）与 `srcdoc`、移除 `on*` 事件属性、移除危险 URL 方案
 * （`javascript:` / `vbscript:` / `data:`，`data:image/*` 除外）、
 * 移除「语义不展示」的元素连同子树（display:none / hidden 属性）、
 * 剥离「视觉隐藏」声明但保留元素与内容（visibility:hidden / opacity:0 / font-size:0），
 * 其余结构与内联排版保留。
 *
 * 用于正文源切换（原文源）——源站 HTML 未经信任，直接 v-html 直出会执行事件处理器；
 * 而公众号整篇正文被 `opacity: 0` 的根节点裹住，剥掉声明即可见（直接移除会让正文变空，design D6）。
 *
 * 结构清洗解析**原始串**（不解实体），与后端 `htmltext.SanitizeHTML` 同口径；
 * 纯文本侧才走「先解实体 → 再解析」（design D10）。
 */
export function sanitizeHtml(html: string): string {
  if (!html) return ''
  const doc = parseHtmlDocument(html)
  if (!doc) {
    // 解析失败：降级为转义后的纯文本（不抛异常、不返回原始 HTML）
    return escapeHtml(collapseWhitespace(stripTagsToText(html)))
  }
  const body = doc.body
  removeDangerousElements(body)
  removeExecutableEmbeds(body)
  removeSemanticallyHiddenElements(body)
  removeEventAttributes(body)
  removeSrcdocAttributes(body)
  removeDangerousUrlAttributes(body)
  stripVisualHiddenStyleDeclarations(body)
  return body.innerHTML
}

/**
 * 第三方 HTML → 纯文本：先解实体 → 再解析 → 去 script/style、去隐藏子树、去标签、折叠空白，
 * 可选按 rune 截断（超长以「…」收尾）。
 *
 * 用于导语段——第三方 HTML 不得作为标签进入 DOM（spec：摘要导语 MUST 以去标签纯文本渲染）。
 *
 * 顺序（design D10，与后端 `htmltext.PlainText` 同口径）：**先解实体 → 再解析**。
 * 解实体后若已无任何标记，直接按纯文本返回（不再解析，避免解析器二次解实体
 * 把 `&amp;lt;` 变成 `<`；后端同一短路，口径一致）。
 *
 * 隐藏口径：摘要侧与后端 `htmltext.PlainText` 同口径 —— 隐藏子树整体剔除，不做「解隐藏」
 * （摘要不重复正文；视觉隐藏声明只剥不删的规则只属于 `sanitizeHtml`）。
 */
export function toPlainText(html: string, maxRunes?: number): string {
  if (!html) return ''
  const decoded = decodeEntitiesOnce(html)
  // 与后端同款短路：解实体后没有标记就不再解析（也省掉一次 DOM 构造）
  if (!MARKUP_LIKE_RE.test(decoded)) {
    return truncateRunes(collapseWhitespace(decoded), maxRunes)
  }
  const doc = parseHtmlDocument(decoded)
  const raw = doc ? extractPlainText(doc) : stripTagsRaw(decoded)
  return truncateRunes(collapseWhitespace(raw), maxRunes)
}

/** DOM 路径的纯文本抽取：脚本/样式与隐藏子树（视觉隐藏 + 语义不展示）都不算正文。 */
function extractPlainText(doc: Document): string {
  const body = doc.body
  if (!body) return ''
  removeDangerousElements(body)
  removeHiddenElements(body)
  const parts: string[] = []
  collectVisibleText(body, parts)
  return parts.join('')
}
