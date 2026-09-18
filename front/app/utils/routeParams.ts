/**
 * RSSHub 路由参数解析（纯函数，可单测）。
 *
 * 与后端 D3 规则对齐：
 * - path 中 `:param` 为参数段；`:param?`（含 `:param{.+}?`）为可选，其余必填；
 * - 目录自带的 parameters 是原始 JSON（对象或数组），值为中文说明。
 */

/** RSSHub 官方文档基址默认值（feed-param-options D4；服务端配置不可达时兜底）。 */
export const DEFAULT_RSSHUB_DOC_BASE = 'https://docs.rsshub.app'

/** RSSHub 实例基址默认值（后端 DefaultRSSHubBaseURL 同值；settings 拉取失败时兜底）。 */
export const DEFAULT_RSSHUB_BASE_URL = 'https://rsshub.app'

export interface RouteParamSpec {
  name: string
  required: boolean
  /** 目录自带的中文参数说明；无说明为空串 */
  description: string
  /** 可选值：优先字典（manual/scraped 人工维护），无字典时用目录自带 options（RSSHub 官方数据）；都无缺省渲染输入框。绝不 LLM 生成 */
  options?: Array<{ value: string, label: string }>
  /** 官方文档链接；未提供 docUrl 入参时缺省 */
  docUrl?: string
}

/**
 * 官方文档链接：{doc_base}/routes/{namespace}#{slug}（design D4）。
 * slug 首版规则：path 去前导 /，去掉参数段（:param / 含 {} / 含 ? 的段），剩余段以 - 连接。
 * TODO: RSSHub 文档锚点精确规则待实测校准（design Open Questions 1），后续可能需按实际锚点调整。
 */
export function buildRouteDocUrl(docBase: string, namespace: string, path: string): string {
  const base = (docBase || DEFAULT_RSSHUB_DOC_BASE).replace(/\/+$/, '')
  const slug = path
    .split('/')
    .filter(seg => seg && !seg.startsWith(':') && !seg.includes('{') && !seg.includes('?'))
    .join('-')
  return `${base}/routes/${namespace}#${slug}`
}

/** 解析 path 中的参数段：`:uid` → 必填；`:category{.+}?` → 可选。 */
export function parsePathParams(path: string): Array<{ name: string, required: boolean }> {
  const out: Array<{ name: string, required: boolean }> = []
  for (const seg of path.split('/')) {
    if (!seg.startsWith(':')) continue
    let raw = seg.slice(1)
    let required = true
    if (raw.endsWith('?')) {
      required = false
      raw = raw.slice(0, -1)
    }
    // 去正则约束 {..}
    const brace = raw.indexOf('{')
    if (brace >= 0) raw = raw.slice(0, brace)
    if (raw) out.push({ name: raw, required })
  }
  return out
}

/**
 * 解析目录自带的 parameters 原始 JSON → { 参数名: 中文说明 }。
 * 容错两种形态：对象（{ name: "说明" | { description: "说明" } }）与字符串数组。
 * 非法 JSON / 非预期结构一律返回 {}（缺说明不报错）。
 */
export function parseParameterDescriptions(raw: string): Record<string, string> {
  if (!raw) return {}
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return {}
  }
  const out: Record<string, string> = {}
  if (Array.isArray(parsed)) {
    for (const item of parsed) {
      if (typeof item === 'string') {
        out[item] = ''
      } else if (item && typeof item === 'object') {
        const rec = item as Record<string, unknown>
        if (typeof rec.name === 'string' && rec.name) {
          out[rec.name] = typeof rec.description === 'string' ? rec.description : ''
        }
      }
    }
    return out
  }
  if (parsed && typeof parsed === 'object') {
    for (const [key, val] of Object.entries(parsed as Record<string, unknown>)) {
      if (typeof val === 'string') {
        out[key] = val
      } else if (val && typeof val === 'object') {
        const rec = val as Record<string, unknown>
        out[key] = typeof rec.description === 'string' ? rec.description : ''
      } else {
        out[key] = ''
      }
    }
  }
  return out
}

/**
 * 解析目录自带 parameters 里的 options 数组（部分 RSSHub 路由目录已含枚举值，如 ifanr/jrj）。
 * 形态：对象 `{ name: { options: [{value,label}], description } }` 或数组 `[{name, options}]`。
 * 与字典互补：字典优先，目录 options 兜底（两者都是真实数据，绝不 LLM 生成）。
 * 非法 / 无 options 字段 / 空数组 → 该参数不出现在结果。
 */
export function parseParameterOptions(raw: string): Record<string, Array<{ value: string, label: string }>> {
  if (!raw) return {}
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return {}
  }
  const out: Record<string, Array<{ value: string, label: string }>> = {}
  const collect = (name: string, val: unknown) => {
    if (!val || typeof val !== 'object') return
    const rec = val as Record<string, unknown>
    const opts = rec.options
    if (!Array.isArray(opts)) return
    const list: Array<{ value: string, label: string }> = []
    for (const o of opts) {
      if (!o || typeof o !== 'object') continue
      const r = o as Record<string, unknown>
      const value = typeof r.value === 'string' ? r.value : ''
      if (!value) continue
      const label = typeof r.label === 'string' ? r.label : value
      list.push({ value, label })
    }
    if (list.length > 0) out[name] = list
  }
  if (Array.isArray(parsed)) {
    for (const item of parsed) {
      if (item && typeof item === 'object') {
        const rec = item as Record<string, unknown>
        if (typeof rec.name === 'string' && rec.name) collect(rec.name, rec)
      }
    }
    return out
  }
  if (parsed && typeof parsed === 'object') {
    for (const [key, val] of Object.entries(parsed as Record<string, unknown>)) {
      collect(key, val)
    }
  }
  return out
}

/**
 * 去掉 URL 模板中的可选参数段与正则约束残留（后端 stripOptionalParams 同规则）：
 * `:param?` 整段（连同前导 /）丢弃；`{regex}` 片段丢弃。
 */
export function stripOptionalParams(url: string): string {
  let u = url
  while (u.includes('{')) {
    const i = u.indexOf('{')
    const j = u.indexOf('}')
    if (j < i) break
    u = u.slice(0, i) + u.slice(j + 1)
  }
  const out: string[] = []
  for (const seg of u.split('/')) {
    if (seg.startsWith(':') && seg.endsWith('?')) continue
    out.push(seg)
  }
  return out.join('/')
}

/**
 * 由路由模板 + 参数构建最终 RSSHub 实例订阅地址（后端 recommendation_service.buildFeedURL
 * 同规则）：用户填了任一参数值 → 一律走模板替换（usable_directly 的 example 只是缺省形态，
 * 不能压过用户显式填参，否则填参被无视、URL 恒为 example）；未填参时 usable_directly 优先
 * example，否则把 `:param` 填入实际值（未提供的可选参数段丢弃），仍有未填必填参数时返回
 * 空串（调用方就地报错，不发请求）。
 */
export function buildRSSHubFeedUrl(opts: {
  baseUrl: string
  namespace: string
  path: string
  parameters: Record<string, string>
  example?: string
  usableDirectly?: boolean
}): string {
  const base = (opts.baseUrl || DEFAULT_RSSHUB_BASE_URL).replace(/\/+$/, '')
  const hasParams = Object.values(opts.parameters).some(v => v.trim() !== '')
  if (!hasParams && opts.usableDirectly) {
    return opts.example ? base + opts.example : `${base}/${opts.namespace}${opts.path}`
  }
  // 模板填参：先剥 {regex} 约束（暴露参数名），再替换 `:name?`（可选标记跟随参数名一起
  // 替换，防值尾残留 `?`），后替换裸 `:name`；未提供的可选段 strip，剩 `:` 即未填必填。
  let u = `/${opts.namespace}${opts.path}`.replace(/\{[^}]*\}/g, '')
  for (const [name, val] of Object.entries(opts.parameters)) {
    if (!val) continue
    // split/join 替换避免 replace 的 $ 特殊语义；值做 path 编码与后端 url.PathEscape 对齐
    const enc = encodeURIComponent(val)
    u = u.split(`:${name}?`).join(enc).split(`:${name}`).join(enc)
  }
  u = stripOptionalParams(u)
  return u.includes(':') ? '' : base + u
}

/**
 * 合并 path 参数段与目录说明，产出填参表单的字段列表（必填在前）。
 * options 来源优先级：字典（manual/scraped）> 目录自带 options；都无则缺省。
 * 向后兼容：无字典且目录无 options 时输出退化为 { name, required, description }。
 */
export function buildRouteParamSpecs(
  path: string,
  rawParameters: string,
  paramOptions?: Record<string, Array<{ value: string, label: string, source?: string }>>,
  docUrl?: string,
): RouteParamSpec[] {
  const descriptions = parseParameterDescriptions(rawParameters)
  const catalogOptions = parseParameterOptions(rawParameters)
  return parsePathParams(path)
    .map((p) => {
      const spec: RouteParamSpec = { ...p, description: descriptions[p.name] ?? '' }
      // 字典优先（manual/scraped 人工维护），无字典用目录自带 options（RSSHub 官方数据）；都无则缺省渲染输入框
      const dictOpts = paramOptions?.[p.name]
      let opts: Array<{ value: string, label: string }> | undefined
      if (dictOpts && dictOpts.length > 0) {
        opts = dictOpts.map(o => ({ value: o.value, label: o.label }))
      } else {
        const catOpts = catalogOptions[p.name]
        if (catOpts && catOpts.length > 0) opts = catOpts
      }
      if (opts) spec.options = opts
      if (docUrl) spec.docUrl = docUrl
      return spec
    })
    .sort((a, b) => Number(b.required) - Number(a.required))
}
