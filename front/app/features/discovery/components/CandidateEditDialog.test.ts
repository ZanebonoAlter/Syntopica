import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import CandidateEditDialog from './CandidateEditDialog.vue'
import type { DiscoveryCandidate, SaveCandidateResult } from '~/types/discovery'

/**
 * S9 落点（test-cases.md 故事 S9 / S15 组件层）：
 * - 保存不触发订阅语义：按钮文案「保存到候选库」+ 弹窗内说明「不创建订阅」；
 * - 表单校验错误就地显示（空名称 / 非法 URL 不发请求）；
 * - 重复地址显示已存在入口提示，不静默新建；
 * - 保存失败保留输入可重试。
 * store 行为（成功 toast「尚未订阅」等）在 stores/discovery.test.ts 验证。
 */

const saveCandidateMock = vi.fn()

vi.mock('~/stores/discovery', async () => {
  const { reactive } = await import('vue')
  // reactive 包装：真实 Pinia store 会解包 ref，mock 也保证标量语义
  return {
    useDiscoveryStore: () => reactive({
      candidateSaving: false,
      saveCandidate: saveCandidateMock,
    }),
  }
})

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function candidate(over: Partial<DiscoveryCandidate> = {}): DiscoveryCandidate {
  return {
    id: '7',
    kind: 'rss',
    name: '田野笔记',
    description: '城市观察',
    language: '中文',
    region: '中国',
    address: 'https://fieldnotes.example/feed.xml',
    recommendationEnabled: true,
    subscribed: false,
    availability: 'unknown',
    lastCheckedAt: null,
    ...over,
  }
}

async function mountDialog(props: { candidate?: DiscoveryCandidate | null } = {}) {
  const wrapper = mount(CandidateEditDialog, {
    props: { modelValue: true, candidate: props.candidate ?? null },
    attachTo: document.body,
  })
  await nextTick()
  return wrapper
}

function dialogEl(): HTMLElement | null {
  return document.querySelector('.app-dialog')
}

/** AppInput 的 data-testid 落在根 wrapper div，实际 input 在内层。 */
function inputEl(testid: string): HTMLInputElement {
  return dialogEl()!.querySelector(`[data-testid="${testid}"] input`) as HTMLInputElement
}

function inputValue(testid: string): string {
  return inputEl(testid).value
}

function setInputValue(testid: string, value: string) {
  const input = inputEl(testid)
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

function clickSave() {
  const btn = dialogEl()!.querySelector('[data-testid="candidate-save"]') as HTMLButtonElement
  btn.click()
}

beforeEach(() => {
  saveCandidateMock.mockReset()
  document.body.innerHTML = ''
})

describe('CandidateEditDialog — 保存语义（S9：候选库不是订阅清单）', () => {
  it('保存按钮文案是「保存到候选库」，弹窗明示不创建订阅', async () => {
    const wrapper = await mountDialog()
    expect((dialogEl()!.querySelector('[data-testid="candidate-save"]') as HTMLElement).textContent)
      .toContain('保存到候选库')
    expect(dialogEl()!.textContent).toContain('不创建订阅')
    wrapper.unmount()
  })

  it('保存成功后关闭弹窗（订阅语义不出现）', async () => {
    saveCandidateMock.mockResolvedValue({ status: 'saved', candidate: candidate() } satisfies SaveCandidateResult)
    const wrapper = await mountDialog()
    setInputValue('candidate-name-input', '新来源')
    setInputValue('candidate-url-input', 'https://a.example/feed.xml')
    clickSave()
    await flushPromises()
    expect(saveCandidateMock).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([false])
    // 文案层不出现「订阅」作为动作（仅「尚未订阅」类说明由 store toast 发出）
    wrapper.unmount()
  })
})

describe('CandidateEditDialog — 就地校验（S9：输入校验及同源重复）', () => {
  it('空名称提交：字段错误就地显示，不发请求', async () => {
    const wrapper = await mountDialog()
    setInputValue('candidate-url-input', 'https://a.example/feed.xml')
    clickSave()
    await nextTick()
    const err = dialogEl()!.querySelector('[data-testid="candidate-name-input"] .app-input-error')
    expect(err?.textContent).toContain('名称不能为空')
    expect(saveCandidateMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    ['not-a-url', '请输入完整的 http:// 或 https:// 地址'],
    ['ftp://a.example/feed.xml', '仅支持 http:// 或 https:// 地址'],
    ['https://user:pass@a.example/feed', '地址不能携带账号或凭据'],
  ])('非法地址 %s：字段错误就地显示，不发请求', async (raw, expected) => {
    const wrapper = await mountDialog()
    setInputValue('candidate-name-input', '名字')
    setInputValue('candidate-url-input', raw)
    clickSave()
    await nextTick()
    const err = dialogEl()!.querySelector('[data-testid="candidate-url-input"] .app-input-error')
    expect(err?.textContent).toContain(expected)
    expect(saveCandidateMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('编辑 RSSHub 条目：路由地址只读，不参与 URL 校验', async () => {
    const wrapper = await mountDialog({
      candidate: candidate({
        kind: 'rsshub',
        address: 'rsshub://design-journal/:section/:author?',
      }),
    })
    expect(inputValue('candidate-url-input')).toBe('rsshub://design-journal/:section/:author?')
    expect(inputEl('candidate-url-input').disabled).toBe(true)
    // 只填名称即可提交（地址只读不算校验失败）
    saveCandidateMock.mockResolvedValue({ status: 'saved', candidate: candidate() } satisfies SaveCandidateResult)
    setInputValue('candidate-name-input', '设计周刊')
    clickSave()
    await flushPromises()
    expect(saveCandidateMock).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})

describe('CandidateEditDialog — 重复地址与失败保留（S9）', () => {
  it('重复地址：显示已存在入口提示（含已有条目名与跳转按钮），不静默新建', async () => {
    saveCandidateMock.mockResolvedValue({
      status: 'duplicate',
      existing: candidate({ id: '9', name: '已有来源' }),
    } satisfies SaveCandidateResult)
    const wrapper = await mountDialog()
    setInputValue('candidate-name-input', '重复源')
    setInputValue('candidate-url-input', 'https://dup.example/feed.xml')
    clickSave()
    await flushPromises()
    const hint = dialogEl()!.querySelector('[data-testid="candidate-duplicate-hint"]')!
    expect(hint.textContent).toContain('已有这个地址')
    expect(hint.textContent).toContain('已有来源')
    expect(hint.querySelector('[data-testid="candidate-show-existing"]')).not.toBeNull()
    // 输入保留，弹窗不关闭
    expect(inputValue('candidate-name-input')).toBe('重复源')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  it('点击「查看已有条目」发出 show-existing 事件并关闭', async () => {
    saveCandidateMock.mockResolvedValue({
      status: 'duplicate',
      existing: candidate({ id: '9', name: '已有来源' }),
    } satisfies SaveCandidateResult)
    const wrapper = await mountDialog()
    setInputValue('candidate-name-input', '重复源')
    setInputValue('candidate-url-input', 'https://dup.example/feed.xml')
    clickSave()
    await flushPromises()
    ;(dialogEl()!.querySelector('[data-testid="candidate-show-existing"]') as HTMLElement).click()
    await nextTick()
    expect(wrapper.emitted('show-existing')?.at(-1)).toEqual(['9'])
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([false])
    wrapper.unmount()
  })

  it('保存失败：错误就地显示且输入保留，可重试', async () => {
    saveCandidateMock
      .mockResolvedValueOnce({ status: 'error', error: '服务暂不可用' } satisfies SaveCandidateResult)
      .mockResolvedValueOnce({ status: 'saved', candidate: candidate() } satisfies SaveCandidateResult)
    const wrapper = await mountDialog()
    setInputValue('candidate-name-input', '待保存源')
    setInputValue('candidate-url-input', 'https://a.example/feed.xml')
    clickSave()
    await flushPromises()
    const err = dialogEl()!.querySelector('[data-testid="candidate-submit-error"]')!
    expect(err.textContent).toContain('服务暂不可用')
    expect(inputValue('candidate-name-input')).toBe('待保存源')
    clickSave()
    await flushPromises()
    expect(saveCandidateMock).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
