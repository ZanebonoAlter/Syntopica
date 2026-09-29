import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { ThinkingLevel } from '~/types'

import ThinkingLevelSelect from './ThinkingLevelSelect.vue'

function mountSelect(props: {
  modelValue?: ThinkingLevel | null
  modelDefault?: ThinkingLevel | null
  supportsXhighMax?: boolean
} = {}) {
  return mount(ThinkingLevelSelect, {
    props: {
      modelValue: null,
      modelDefault: null,
      supportsXhighMax: true,
      ...props,
    },
  })
}

function optionTexts(wrapper: ReturnType<typeof mountSelect>): string[] {
  return wrapper.findAll('option').map(o => o.text())
}

describe('ThinkingLevelSelect（U4）', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('继承项文案带模型默认档中文；模型未配置默认档回退全局默认 medium', () => {
    const withDefault = mountSelect({ modelDefault: 'high' })
    expect(optionTexts(withDefault)[0]).toBe('继承（模型默认 高）')

    const withoutDefault = mountSelect({ modelDefault: null })
    expect(optionTexts(withoutDefault)[0]).toBe('继承（模型默认 中）')
  })

  it('声明支持的模型全档可选：继承项 + 七档显式中文', () => {
    const wrapper = mountSelect({ supportsXhighMax: true })
    const options = wrapper.findAll('option')
    expect(options).toHaveLength(8)
    expect(optionTexts(wrapper)).toEqual(['继承（模型默认 中）', '关闭', '微', '低', '中', '高', '超高', '极'])
    expect(options.slice(1).map(o => o.element.value)).toEqual([
      'off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max',
    ])
  })

  it('未声明 xhigh/max 的模型只有到 high 的选项（钳制不出现 xhigh·max）', () => {
    const wrapper = mountSelect({ supportsXhighMax: false })
    const values = wrapper.findAll('option').map(o => o.element.value)
    expect(values).toEqual(['', 'off', 'minimal', 'low', 'medium', 'high'])
    expect(values).not.toContain('xhigh')
    expect(values).not.toContain('max')
  })

  it('选中非继承档 → emit update:modelValue + override；value 反映选中项', async () => {
    const wrapper = mountSelect({ modelValue: null })

    await wrapper.find('select').setValue('low')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual(['low'])
    expect(wrapper.emitted('override')![0]).toEqual(['low'])
    expect((wrapper.find('select').element as HTMLSelectElement).value).toBe('low')

    // 支持受控用法：父层写入后选中项跟随
    await wrapper.setProps({ modelValue: 'low' })
    expect((wrapper.find('select').element as HTMLSelectElement).value).toBe('low')
  })

  it('选回继承（空值）→ 只 emit update:modelValue(null)，不 emit override', async () => {
    const wrapper = mountSelect({ modelValue: 'high' })

    await wrapper.find('select').setValue('')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([null])
    expect(wrapper.emitted('override')).toBeUndefined()
  })
})
