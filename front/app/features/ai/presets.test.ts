import { describe, expect, it } from 'vitest'
import { AI_VENDOR_PRESETS, findVendorPreset, isLocalBaseUrl } from './presets'
import type { ProviderType, StructuredOutputMode, ThinkingDialect } from '~/types'

const EXPECTED_IDS = ['deepseek', 'xiaomi-mimo', 'zai', 'openrouter', 'ollama', 'llamacpp', 'custom']

const VALID_PROVIDER_TYPES: ProviderType[] = ['openai_compatible', 'ollama']
const VALID_DIALECTS: ThinkingDialect[] = ['qwen-chat-template', 'deepseek', 'reasoning-effort', 'none']
const VALID_SO: StructuredOutputMode[] = ['json_schema', 'json_object', 'none']

describe('AI_VENDOR_PRESETS（4.2 交付 1：供应商预设目录）', () => {
  it('清单完备：7 项且 id 集合固定（VendorDialog 第一段目录顺序即清单顺序）', () => {
    expect(AI_VENDOR_PRESETS).toHaveLength(7)
    expect(AI_VENDOR_PRESETS.map(p => p.id)).toEqual(EXPECTED_IDS)
    expect(new Set(AI_VENDOR_PRESETS.map(p => p.id)).size).toBe(7)
  })

  it('DeepSeek 预设 structured_output=json_object（spec 明确的厂商默认档）', () => {
    const deepseek = findVendorPreset('deepseek')
    expect(deepseek).toBeTruthy()
    expect(deepseek!.structuredOutput).toBe('json_object')
    expect(deepseek!.thinkingDialect).toBe('deepseek')
    expect(deepseek!.baseUrl).toBe('https://api.deepseek.com/v1')
  })

  it('枚举值全部合法（providerType/dialect/structuredOutput 与 types/ai.ts 联合一致）', () => {
    for (const p of AI_VENDOR_PRESETS) {
      expect(VALID_PROVIDER_TYPES).toContain(p.providerType)
      expect(VALID_DIALECTS).toContain(p.thinkingDialect)
      expect(VALID_SO).toContain(p.structuredOutput)
      expect(p.label.length).toBeGreaterThan(0)
      expect(p.icon).toMatch(/^mdi:/)
    }
  })

  it('云端预设 needsApiKey 且非 isLocal；本地预设无需密钥且 isLocal', () => {
    const cloud = AI_VENDOR_PRESETS.filter(p => !p.isLocal && p.id !== 'custom')
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
