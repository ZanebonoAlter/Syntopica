import { describe, expect, it } from 'vitest'
import { AI_VENDOR_PRESETS, PRESET_GROUPS, findVendorPreset, isLocalBaseUrl } from './presets'
import type { ProviderType, StructuredOutputMode, ThinkingDialect } from '~/types'

/**
 * 预设目录锚点（tasks 8.1，FB-1：pi 内置 41 家 + Ollama/llama.cpp/自定义 = 44 项五组）。
 * pi 清单来源 pi-coding-agent dist/core/model-resolver.js defaultModelPerProvider 键；
 * logo 映射 pi-web provider-icons.svg symbol id（FB-2）。
 */
const EXPECTED_GROUP_COUNTS: Record<string, number> = {
  cn: 15,
  global: 10,
  gateway: 16,
  local: 2,
  custom: 1,
}

const VALID_PROVIDER_TYPES: ProviderType[] = ['openai_compatible', 'ollama']
const VALID_DIALECTS: ThinkingDialect[] = ['qwen-chat-template', 'deepseek', 'reasoning-effort', 'none']
const VALID_SO: StructuredOutputMode[] = ['json_schema', 'json_object', 'none']

describe('AI_VENDOR_PRESETS（tasks 8.1：44 项五组预设目录）', () => {
  it('清单完备：44 项且分组计数与 pi 内置清单对账（cn15/global10/gateway16/local2/custom1）', () => {
    expect(AI_VENDOR_PRESETS).toHaveLength(44)
    expect(new Set(AI_VENDOR_PRESETS.map(p => p.id)).size).toBe(44)
    const counts = AI_VENDOR_PRESETS.reduce<Record<string, number>>((acc, p) => {
      acc[p.group] = (acc[p.group] ?? 0) + 1
      return acc
    }, {})
    expect(counts).toEqual(EXPECTED_GROUP_COUNTS)
    // pi 内置 41 家 + 本地 2 + 自定义 1：本地/自定义组不含 pi 家
    expect(AI_VENDOR_PRESETS.filter(p => p.group === 'local' || p.group === 'custom')).toHaveLength(3)
  })

  it('PRESET_GROUPS：五组顺序固定（VendorDialog 目录展示顺序）', () => {
    expect(PRESET_GROUPS.map(g => g.id)).toEqual(['cn', 'global', 'gateway', 'local', 'custom'])
  })

  it('pi 关键厂商在册：anthropic/openai/deepseek/zai-coding-cn/xiaomi/qwen-token-plan/kimi-coding 等', () => {
    for (const id of [
      'anthropic', 'openai', 'google', 'deepseek', 'xai', 'groq', 'mistral',
      'zai', 'zai-coding-cn', 'xiaomi', 'xiaomi-token-plan-cn', 'qwen-token-plan',
      'qwen-token-plan-individual', 'kimi-coding', 'moonshotai', 'moonshotai-cn',
      'minimax', 'minimax-cn', 'ant-ling', 'openrouter', 'amazon-bedrock',
      'azure-openai-responses', 'cloudflare-workers-ai', 'github-copilot', 'nvidia',
      'huggingface', 'fireworks', 'together', 'baseten', 'meta', 'cerebras',
      'opencode', 'opencode-go', 'openai-codex', 'radius', 'vercel-ai-gateway',
      'google-vertex', 'cloudflare-ai-gateway', 'ollama', 'llamacpp', 'custom',
    ]) {
      expect(findVendorPreset(id), `${id} 应在预设目录`).toBeTruthy()
    }
  })

  it('DeepSeek 预设 structured_output=json_object（spec 明确的厂商默认档）', () => {
    const deepseek = findVendorPreset('deepseek')
    expect(deepseek).toBeTruthy()
    expect(deepseek!.structuredOutput).toBe('json_object')
    expect(deepseek!.thinkingDialect).toBe('deepseek')
    expect(deepseek!.baseUrl).toBe('https://api.deepseek.com/v1')
  })

  it('小米 Token Plan 三区 base_url 为 token-plan 域名（FB-8，与 pi 内置定义对齐）；标准 xiaomi 家用 api 域名', () => {
    expect(findVendorPreset('xiaomi')!.baseUrl).toBe('https://api.xiaomimimo.com/v1')
    expect(findVendorPreset('xiaomi-token-plan-cn')!.baseUrl).toBe('https://token-plan-cn.xiaomimimo.com/v1')
    expect(findVendorPreset('xiaomi-token-plan-ams')!.baseUrl).toBe('https://token-plan-ams.xiaomimimo.com/v1')
    expect(findVendorPreset('xiaomi-token-plan-sgp')!.baseUrl).toBe('https://token-plan-sgp.xiaomimimo.com/v1')
  })

  it('枚举值全部合法（providerType/dialect/structuredOutput 与 types/ai.ts 联合一致）', () => {
    for (const p of AI_VENDOR_PRESETS) {
      expect(VALID_PROVIDER_TYPES).toContain(p.providerType)
      expect(VALID_DIALECTS).toContain(p.thinkingDialect)
      expect(VALID_SO).toContain(p.structuredOutput)
      expect(p.label.length).toBeGreaterThan(0)
    }
  })

  it('logo 为 vendor-icons.svg symbol id 或 null（首字母回退，FB-2）', () => {
    for (const p of AI_VENDOR_PRESETS) {
      if (p.logo !== null) {
        expect(p.logo).toMatch(/^[a-z][a-z0-9-]*$/i)
      }
    }
    // 无图标厂商走首字母回退（集合比较，不依赖清单顺序）：
    // pi 家中无 symbol 的 5 家 + 本地/自定义 3 项 = 8 项
    const noLogo = AI_VENDOR_PRESETS.filter(p => p.logo === null).map(p => p.id)
    expect(noLogo).toHaveLength(8)
    expect(new Set(noLogo)).toEqual(new Set(['openai-codex', 'radius', 'opencode-go', 'baseten', 'meta', 'custom', 'ollama', 'llamacpp']))
  })

  it('云端预设 needsApiKey 且非 isLocal；本地预设无需密钥且 isLocal', () => {
    const cloud = AI_VENDOR_PRESETS.filter(p => !p.isLocal && p.group !== 'custom')
    for (const p of cloud) {
      expect(p.needsApiKey, `${p.id} 云端预设应需密钥`).toBe(true)
    }
    for (const id of ['ollama', 'llamacpp']) {
      const p = findVendorPreset(id)
      expect(p?.isLocal, `${id} 应标记 isLocal`).toBe(true)
      expect(p?.needsApiKey, `${id} 本地服务无需密钥`).toBe(false)
    }
  })

  it('除自定义外 baseUrl 非空；Ollama 预设带 /v1（连接测试统一请求 {base_url}/models）', () => {
    for (const p of AI_VENDOR_PRESETS) {
      if (p.id === 'custom') {
        expect(p.baseUrl).toBe('')
      } else {
        expect(p.baseUrl.length).toBeGreaterThan(0)
      }
    }
    expect(findVendorPreset('ollama')!.baseUrl).toMatch(/\/v1$/)
  })

  it('findVendorPreset：未知/空 id 返回 undefined（vendor.preset_id 缺省安全）', () => {
    expect(findVendorPreset('nope')).toBeUndefined()
    expect(findVendorPreset(undefined)).toBeUndefined()
    expect(findVendorPreset('')).toBeUndefined()
  })
})

describe('isLocalBaseUrl（镜像后端 hostIsLocal，embedding 选项开关口径）', () => {
  it.each([
    'http://localhost:11434/v1',
    'http://LOCALHOST:8080/v1',
    'http://127.0.0.1:8080/v1',
    'http://10.11.12.111:8080/v1',
    'http://172.16.0.5:8080/v1',
    'http://172.31.255.1:8080/v1',
    'http://192.168.1.10:8080/v1',
    '10.0.0.5:8080',
  ])('本地地址 %s → true', (url) => {
    expect(isLocalBaseUrl(url)).toBe(true)
  })

  it.each([
    'https://api.deepseek.com/v1',
    'http://openrouter.ai/api/v1',
    '',
    'not a url',
    'http://11.0.0.1/v1', // 非 RFC1918 的 11/8
    'http://172.32.0.1/v1', // 172.16/12 之外
    'http://192.169.1.1/v1',
  ])('云端/非法地址 %s → false（误判方向收紧）', (url) => {
    expect(isLocalBaseUrl(url)).toBe(false)
  })
})
