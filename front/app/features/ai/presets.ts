import type { ProviderType, StructuredOutputMode, ThinkingDialect } from '~/types'

/**
 * 供应商预设目录（ai-provider-model-overhaul 4.2 交付 1）。
 * 前端内置常量、随版本更新（不做后端托管）；VendorDialog 两段式弹窗第一段
 * 的目录与第二段的预填均取自本清单。DeepSeek 的 structured_output=json_object
 * 为 spec（specs/ai-vendor-model「模型配置约束」上下文）明确的厂商默认档，
 * presets.test.ts 有锚点断言。
 */
export interface AIVendorPreset {
  id: string
  label: string
  /** Iconify 图标名（mdi:* 形式） */
  icon: string
  providerType: ProviderType
  baseUrl: string
  thinkingDialect: ThinkingDialect
  structuredOutput: StructuredOutputMode
  /** 第二段露出 API Key 输入（false 且非自定义时显示「本地服务无需密钥」） */
  needsApiKey: boolean
  /** 本地服务（Ollama/llama.cpp）：可配 embedding 模型与启动命令 */
  isLocal?: boolean
  note?: string
}

export const AI_VENDOR_PRESETS: AIVendorPreset[] = [
  {
    id: 'deepseek',
    label: 'DeepSeek',
    icon: 'mdi:fish',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.deepseek.com/v1',
    thinkingDialect: 'deepseek',
    // DeepSeek 官方仅支持宽松 JSON 模式，json_object 为该厂商默认（spec 明确）
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '深度求索 OpenAI 兼容端点',
  },
  {
    id: 'xiaomi-mimo',
    label: '小米 MiMo',
    icon: 'mdi:robot-outline',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.xiaomimimo.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '小米 MiMo 开放平台',
  },
  {
    id: 'zai',
    label: 'ZAI',
    icon: 'mdi:alpha-z-box-outline',
    providerType: 'openai_compatible',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '智谱 BigModel OpenAI 兼容端点',
  },
  {
    id: 'openrouter',
    label: 'OpenRouter',
    icon: 'mdi:routes',
    providerType: 'openai_compatible',
    baseUrl: 'https://openrouter.ai/api/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '聚合多家模型的路由端点',
  },
  {
    id: 'ollama',
    label: 'Ollama',
    icon: 'mdi:cube-outline',
    providerType: 'ollama',
    // /v1 必须保留：连接测试统一请求 {base_url}/models（Ollama OpenAI 兼容层）
    baseUrl: 'http://localhost:11434/v1',
    thinkingDialect: 'none',
    structuredOutput: 'json_schema',
    needsApiKey: false,
    isLocal: true,
    note: '本地模型服务，无需密钥',
  },
  {
    id: 'llamacpp',
    label: 'llama.cpp',
    icon: 'mdi:server',
    providerType: 'openai_compatible',
    baseUrl: 'http://localhost:8080/v1',
    thinkingDialect: 'qwen-chat-template',
    structuredOutput: 'json_schema',
    needsApiKey: false,
    isLocal: true,
    note: '可由后端托管拉起的本地推理服务',
  },
  {
    id: 'custom',
    label: '自定义',
    icon: 'mdi:tune',
    providerType: 'openai_compatible',
    baseUrl: '',
    // 与后端 normalizeVendor 缺省口径一致（dialect=none / so=json_schema）
    thinkingDialect: 'none',
    structuredOutput: 'json_schema',
    needsApiKey: false,
    note: '全量表单手填任意 OpenAI 兼容服务',
  },
]

/** 预设目录第二段的展示顺序即清单顺序；按 id 查找（vendor.preset_id 关联用） */
export function findVendorPreset(presetId?: string): AIVendorPreset | undefined {
  if (!presetId) return undefined
  return AI_VENDOR_PRESETS.find(p => p.id === presetId)
}

/**
 * base_url 是否指向本机/内网（镜像后端 hostIsLocal，store_vendor.go）：
 * localhost / loopback / RFC1918 私网 IPv4 视为本地；空串、裸域名、解析失败
 * 一律视为云端——误判方向是收紧而非放行，与后端「云端 vendor 不能配 embedding」
 * 校验保持同一口径。
 */
export function isLocalBaseUrl(baseURL: string): boolean {
  const raw = baseURL.trim()
  if (!raw) return false
  let host = raw
  try {
    const u = new URL(raw)
    if (u.hostname) host = u.hostname
  } catch {
    // 无 scheme 的裸 host（如 "10.0.0.5:8080"）：剥离 scheme/路径/端口（非 IPv6 字面量）
    const idx = host.indexOf('://')
    if (idx >= 0) host = host.slice(idx + 3)
    const slash = host.indexOf('/')
    if (slash >= 0) host = host.slice(0, slash)
    if (!host.includes(']')) {
      const colon = host.lastIndexOf(':')
      if (colon >= 0) host = host.slice(0, colon)
    }
  }
  if (host.toLowerCase() === 'localhost') return true
  // IPv6 loopback（[::1] 形式 hostname 已由 URL 解出）
  if (host === '::1' || host === '[::1]') return true
  const m = host.match(/^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/)
  if (!m) return false
  const octets = m.slice(1).map(Number)
  if (octets.some(n => Number.isNaN(n) || n < 0 || n > 255)) return false
  const [a, b] = octets as [number, number]
  if (a === 127) return true // 127/8 loopback
  if (a === 10) return true // 10/8
  if (a === 172 && b >= 16 && b <= 31) return true // 172.16/12
  if (a === 192 && b === 168) return true // 192.168/16
  return false
}
