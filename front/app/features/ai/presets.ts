import type { ProviderType, StructuredOutputMode, ThinkingDialect } from '~/types'

/**
 * 供应商预设目录（ai-provider-model-overhaul tasks 8.1，2026-09-29 FB-1 扩充）。
 * 覆盖 pi 内置全部 41 家厂商（来源 pi-coding-agent dist/core/model-resolver.js
 * defaultModelPerProvider 键，随 pi 版本手动同步）+ 本地 Ollama/llama.cpp + 自定义，
 * 共 44 项，按五组呈现（PRESET_GROUPS）。前端内置常量、不做后端托管；
 * VendorDialog 两段式弹窗第一段的分组目录/搜索与第二段的预填均取自本清单。
 * logo 为 app/assets/vendor-icons.svg 的 symbol id（FB-2，pi-web provider-icons
 * spritesheet）；无 logo 的 5 家（openai-codex/radius/opencode-go/baseten/meta）
 * 由 VendorLogo 首字母回退。DeepSeek 的 structured_output=json_object 为 spec
 * （specs/ai-vendor-model「模型配置约束」上下文）明确的厂商默认档，
 * presets.test.ts 有锚点断言。
 */
export interface AIVendorPreset {
  id: string
  label: string
  /** vendor-icons.svg symbol id；null = 首字母回退（FB-2） */
  logo: string | null
  /** 目录分组：cn 中国区 / global 国际直连 / gateway 聚合网关与云平台 / local 本地 / custom 自定义 */
  group: 'cn' | 'global' | 'gateway' | 'local' | 'custom'
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

/** 目录分组展示顺序（FB-1：44 项分组 + 搜索，ui-design IA） */
export const PRESET_GROUPS: Array<{ id: AIVendorPreset['group'], label: string }> = [
  { id: 'cn', label: '中国区' },
  { id: 'global', label: '国际直连' },
  { id: 'gateway', label: '聚合网关与云平台' },
  { id: 'local', label: '本地' },
  { id: 'custom', label: '自定义' },
]

export const AI_VENDOR_PRESETS: AIVendorPreset[] = [
  {
    id: 'deepseek',
    label: 'DeepSeek',
    logo: 'deepseek',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.deepseek.com/v1',
    thinkingDialect: 'deepseek',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '深度求索 OpenAI 兼容端点；官方仅宽松 JSON 模式（json_object 默认，spec 锚点）',
  },
  {
    id: 'zai',
    label: 'ZAI Coding（国际）',
    logo: 'zai',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.z.ai/api/paas/v4',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'zai-coding-cn',
    label: 'ZAI Coding（中国）',
    logo: 'zai',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'xiaomi',
    label: '小米 MiMo',
    logo: 'xiaomimimo',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.xiaomimimo.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '小米 MiMo 开放平台',
  },
  {
    id: 'xiaomi-token-plan-cn',
    label: '小米 Token（中国）',
    logo: 'xiaomimimo',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://token-plan-cn.xiaomimimo.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'xiaomi-token-plan-ams',
    label: '小米 Token（阿姆斯特丹）',
    logo: 'xiaomimimo',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://token-plan-ams.xiaomimimo.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'xiaomi-token-plan-sgp',
    label: '小米 Token（新加坡）',
    logo: 'xiaomimimo',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://token-plan-sgp.xiaomimimo.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'qwen-token-plan',
    label: 'Qwen Token',
    logo: 'qwen',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    thinkingDialect: 'qwen-chat-template',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'qwen-token-plan-cn',
    label: 'Qwen Token（中国）',
    logo: 'qwen',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    thinkingDialect: 'qwen-chat-template',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'qwen-token-plan-individual',
    label: 'Qwen Token（个人）',
    logo: 'qwen',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    thinkingDialect: 'qwen-chat-template',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'minimax',
    label: 'MiniMax（国际）',
    logo: 'minimax',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.minimaxi.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'minimax-cn',
    label: 'MiniMax（中国）',
    logo: 'minimax',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.minimax.chat/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'moonshotai-cn',
    label: 'Moonshot（中国）',
    logo: 'moonshot',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.moonshot.cn/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'kimi-coding',
    label: 'Kimi For Coding',
    logo: 'kimi',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.moonshot.cn/api/v3',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'ant-ling',
    label: '蚂蚁 Ling',
    logo: 'antgroup',
    group: 'cn',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.ling.ant.dev/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '中国区云端厂商',
  },
  {
    id: 'anthropic',
    label: 'Anthropic',
    logo: 'anthropic',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.anthropic.com',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'openai',
    label: 'OpenAI',
    logo: 'openai',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.openai.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'google',
    label: 'Google Gemini',
    logo: 'google',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'xai',
    label: 'xAI',
    logo: 'xai',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.x.ai/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'groq',
    label: 'Groq',
    logo: 'groq',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.groq.com/openai/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'mistral',
    label: 'Mistral',
    logo: 'mistral',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.mistral.ai/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'moonshotai',
    label: 'Moonshot AI（国际）',
    logo: 'moonshot',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.moonshot.ai/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'meta',
    label: 'Meta',
    logo: null,
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.meta.ai/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'nvidia',
    label: 'NVIDIA NIM',
    logo: 'nvidia',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://integrate.api.nvidia.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'cerebras',
    label: 'Cerebras',
    logo: 'cerebras',
    group: 'global',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.cerebras.ai/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '国际直连厂商',
  },
  {
    id: 'openrouter',
    label: 'OpenRouter',
    logo: 'openrouter',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://openrouter.ai/api/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合多家模型的路由端点',
  },
  {
    id: 'vercel-ai-gateway',
    label: 'Vercel AI Gateway',
    logo: 'vercel',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://ai-gateway.vercel.sh/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'radius',
    label: 'Radius',
    logo: null,
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.radius.pi.dev/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'opencode',
    label: 'OpenCode Zen',
    logo: 'opencode',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://opencode.ai/zen/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'opencode-go',
    label: 'OpenCode Go',
    logo: null,
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://opencode.ai/zen/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'github-copilot',
    label: 'GitHub Copilot',
    logo: 'githubcopilot',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.githubcopilot.com',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'openai-codex',
    label: 'OpenAI Codex',
    logo: null,
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.openai.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'azure-openai-responses',
    label: 'Azure OpenAI',
    logo: 'azure',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://your-resource.ai.azure.com/openai/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '需资源名+部署映射，见高级',
  },
  {
    id: 'amazon-bedrock',
    label: 'Amazon Bedrock',
    logo: 'aws',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'bedrock-runtime（区域）',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '走 AWS 云凭证，非 API Key',
  },
  {
    id: 'google-vertex',
    label: 'Vertex AI',
    logo: 'google',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://aiplatform.googleapis.com/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_schema',
    needsApiKey: true,
    note: '走 GCP 应用默认凭证',
  },
  {
    id: 'cloudflare-workers-ai',
    label: 'Cloudflare Workers AI',
    logo: 'cloudflare',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.cloudflare.com/client/v4',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'cloudflare-ai-gateway',
    label: 'Cloudflare AI Gateway',
    logo: 'cloudflare',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.cloudflare.com/client/v4',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'huggingface',
    label: 'Hugging Face',
    logo: 'huggingface',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://router.huggingface.co/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'fireworks',
    label: 'Fireworks',
    logo: 'fireworks',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.fireworks.ai/inference/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'together',
    label: 'Together AI',
    logo: 'together',
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.together.xyz/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'baseten',
    label: 'Baseten',
    logo: null,
    group: 'gateway',
    providerType: 'openai_compatible',
    baseUrl: 'https://api.baseten.co/v1',
    thinkingDialect: 'reasoning-effort',
    structuredOutput: 'json_object',
    needsApiKey: true,
    note: '聚合网关或云平台',
  },
  {
    id: 'ollama',
    label: 'Ollama',
    logo: null,
    group: 'local',
    providerType: 'ollama',
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
    logo: null,
    group: 'local',
    providerType: 'openai_compatible',
    baseUrl: 'http://10.11.12.111:8080/v1',
    thinkingDialect: 'qwen-chat-template',
    structuredOutput: 'json_schema',
    needsApiKey: false,
    isLocal: true,
    note: '可由后端托管拉起的本地推理服务',
  },
  {
    id: 'custom',
    label: '自定义',
    logo: null,
    group: 'custom',
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
