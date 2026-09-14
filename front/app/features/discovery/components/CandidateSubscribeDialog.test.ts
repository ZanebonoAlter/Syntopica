import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import CandidateSubscribeDialog from './CandidateSubscribeDialog.vue'
import type { CandidateDetail } from '~/types/discovery'

/**
 * 5.3 候选订阅弹窗（R6 双路径）：
 * - 原生 RSS：展示规范化地址确认即建（verify=false），成功「已订阅」终态防重复创建；
 * - 失败保留输入可重试；409 already 视为已订阅不重复创建；
 * - RSSHub：复用 buildRouteParamSpecs（目录 options / 输入兜底）+ 官方文档链接
 *   （doc_base 拉取失败兜底默认常量）；提交先验证再建（verify=true）；
 * - 原生 RSS 不显示 RSSHub 文档链接（不伪造）。
 */

const detailMock = vi.fn()
const statusMock = vi.fn()
const subscribeMock = vi.fn()
const markSubscribedMock = vi.fn()

vi.mock('~/api/discovery', () => ({
  useDiscoveryApi: () => ({
    getCandidateDetail: detailMock,
  }),
}))

vi.mock('~/api/rsshub', () => ({
  useRsshubApi: () => ({
    getStatus: statusMock,
  }),
}))

vi.mock('~/stores/api', () => ({
  useApiStore: () => ({
    categories: [{ id: '3', name: '科技' }],
  }),
}))

vi.mock('~/stores/discovery', async () => {
  const { reactive } = await import('vue')
  return {
    useDiscoveryStore: () => reactive({
      subscribingIds: [] as string[],
      subscribeFeed: subscribeMock,
      markSubscribed: markSubscribedMock,
    }),
  }
})

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function rssDetail(over: Partial<CandidateDetail> = {}): CandidateDetail {
  return {
    id: '5',
    kind: 'rss',
    name: '田野笔记',
    feedUrl: 'https://field.example/feed.xml',
    subscribed: false,
    route: null,
    ...over,
  }
}

function rsshubDetail(over: Partial<CandidateDetail> = {}): CandidateDetail {
  return {
    id: '6',
    kind: 'rsshub',
    name: '测试期刊路由',
    feedUrl: '',
    subscribed: false,
    route: {
      namespace: 'test',
      path: '/journal/:section',
      name: 'test journal',
      example: '',
      // 目录自带 options（真实上游数据）：字典优先规则在 buildRouteParamSpecs，此处无字典走目录 options
      parameters: JSON.stringify({
        section: {
          description: '栏目',
          options: [
            { value: 'tech', label: '科技' },
            { value: 'life', label: '生活' },
          ],
        },
      }),
      usableDirectly: false,
      requiresParameters: true,
    },
    ...over,
  }
}

function okStatus() {
  return {
    success: true,
    data: {
      rsshub_base_url: 'https://rsshub.example',
      configured: true,
      default: 'https://rsshub.app',
      rsshub_doc_base: '', // 拉到但为空：兜底默认常量
      rsshub_doc_base_default: 'https://docs.rsshub.app',
    },
  }
}

function dialogEl(): HTMLElement | null {
  return document.querySelector('.app-dialog')
}

function clickBtn(testid: string) {
  ;(dialogEl()!.querySelector(`[data-testid="${testid}"]`) as HTMLElement).click()
}

function setSelectValue(testid: string, value: string) {
  const el = dialogEl()!.querySelector(`[data-testid="${testid}"]`) as HTMLSelectElement
  el.value = value
  el.dispatchEvent(new Event('change', { bubbles: true }))
}

beforeEach(() => {
  detailMock.mockReset()
  statusMock.mockReset()
  subscribeMock.mockReset()
  markSubscribedMock.mockReset()
  document.body.innerHTML = ''
})

async function mountDialog(props: { candidateId?: string | null, docBase?: string } = {}) {
  // 生产生命周期：先 false 挂载再置 true，触发 watch 拉详情与 RSSHub 配置
  const wrapper = mount(CandidateSubscribeDialog, {
    props: {
      modelValue: false,
      candidateId: props.candidateId ?? '5',
      presetName: undefined,
      ...(props.docBase !== undefined ? { docBase: props.docBase } : {}),
    },
    attachTo: document.body,
  })
  await nextTick()
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
  return wrapper
}

describe('CandidateSubscribeDialog — 原生 RSS（R6：确认规范化地址）', () => {
  it('展示规范化后地址；不显示 RSSHub 官方文档链接（不伪造）', async () => {
    detailMock.mockResolvedValue({ success: true, data: rssDetail() })
    statusMock.mockResolvedValue(okStatus())
    const wrapper = await mountDialog()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-rss-confirm"]')).not.toBeNull()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-final-url"]')!.textContent)
      .toBe('https://field.example/feed.xml')
    // 原生 RSS 无 RSSHub 文档链接
    expect(dialogEl()!.querySelector('[data-testid="subscribe-doc-link"]')).toBeNull()
    wrapper.unmount()
  })

  it('确认订阅：verify=false 直接建源；成功后「已订阅」终态 + 提交按钮消失防重复', async () => {
    detailMock.mockResolvedValue({ success: true, data: rssDetail() })
    statusMock.mockResolvedValue(okStatus())
    subscribeMock.mockResolvedValue({ status: 'subscribed', title: '田野笔记' })
    const wrapper = await mountDialog()
    clickBtn('subscribe-submit')
    await flushPromises()

    expect(subscribeMock).toHaveBeenCalledWith(expect.objectContaining({
      candidateId: '5',
      url: 'https://field.example/feed.xml',
      verify: false, // 原生确认即建，不走 fetch 验证
    }))
    expect(markSubscribedMock).toHaveBeenCalledWith('5')
    expect(wrapper.emitted('subscribed')?.at(-1)).toEqual(['5'])
    const done = dialogEl()!.querySelector('[data-testid="subscribe-done"]')!
    expect(done.textContent).toContain('已订阅')
    // 终态无提交按钮（防重复创建）
    expect(dialogEl()!.querySelector('[data-testid="subscribe-submit"]')).toBeNull()
    expect(subscribeMock).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('失败保留可重试：错误就地展示，再点提交第二次成功', async () => {
    detailMock.mockResolvedValue({ success: true, data: rssDetail() })
    statusMock.mockResolvedValue(okStatus())
    subscribeMock
      .mockResolvedValueOnce({ status: 'error', error: '网络中断' })
      .mockResolvedValueOnce({ status: 'subscribed', title: '田野笔记' })
    const wrapper = await mountDialog()
    clickBtn('subscribe-submit')
    await flushPromises()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-submit-error"]')!.textContent)
      .toContain('网络中断')
    expect(dialogEl()!.querySelector('[data-testid="subscribe-done"]')).toBeNull()

    clickBtn('subscribe-submit')
    await flushPromises()
    expect(subscribeMock).toHaveBeenCalledTimes(2)
    expect(dialogEl()!.querySelector('[data-testid="subscribe-done"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('409 already：视为已订阅终态，不重复创建', async () => {
    detailMock.mockResolvedValue({ success: true, data: rssDetail() })
    statusMock.mockResolvedValue(okStatus())
    subscribeMock.mockResolvedValue({ status: 'already' })
    const wrapper = await mountDialog()
    clickBtn('subscribe-submit')
    await flushPromises()
    const done = dialogEl()!.querySelector('[data-testid="subscribe-done"]')!
    expect(done.textContent).toContain('已在订阅列表')
    expect(dialogEl()!.querySelector('[data-testid="subscribe-submit"]')).toBeNull()
    wrapper.unmount()
  })
})

describe('CandidateSubscribeDialog — RSSHub 需参数（R6：验证后建）', () => {
  it('目录 options 渲染为下拉；官方文档链接兜底默认常量；未填必填时禁用提交', async () => {
    detailMock.mockResolvedValue({ success: true, data: rsshubDetail() })
    statusMock.mockResolvedValue(okStatus())
    const wrapper = await mountDialog()

    // 官方文档链接始终出现：settings 值空 → 兜底默认常量
    const doc = dialogEl()!.querySelector('[data-testid="subscribe-doc-link"]') as HTMLAnchorElement
    expect(doc).not.toBeNull()
    expect(doc.getAttribute('href')).toContain('https://docs.rsshub.app')
    expect(doc.getAttribute('href')).toContain('routes/test') // namespace 入 URL，path 首段作 anchor

    // 目录自带 options → select（字典优先规则在 utils；无字典时目录 options 兜底）
    const select = dialogEl()!.querySelector('[data-testid="subscribe-param-section"]') as HTMLSelectElement
    expect(select).not.toBeNull()
    expect(select.tagName).toBe('SELECT')
    expect([...select.options].map(o => o.value)).toEqual(['', 'tech', 'life'])

    // 必填未填：最终地址不出，提交禁用
    expect((dialogEl()!.querySelector('[data-testid="subscribe-submit"]') as HTMLButtonElement).disabled)
      .toBe(true)
    wrapper.unmount()
  })

  it('填参后地址即时更新；提交 verify=true（先验证可解析再建源）', async () => {
    detailMock.mockResolvedValue({ success: true, data: rsshubDetail() })
    statusMock.mockResolvedValue(okStatus())
    subscribeMock.mockResolvedValue({ status: 'subscribed', title: '测试期刊路由' })
    const wrapper = await mountDialog({ candidateId: '6' })

    setSelectValue('subscribe-param-section', 'tech')
    await nextTick()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-final-url"]')!.textContent)
      .toBe('https://rsshub.example/test/journal/tech')

    clickBtn('subscribe-submit')
    await flushPromises()
    expect(subscribeMock).toHaveBeenCalledWith(expect.objectContaining({
      candidateId: '6',
      url: 'https://rsshub.example/test/journal/tech',
      verify: true, // RSSHub 需参数路由：先 POST /feeds/fetch 验证再建
    }))
    expect(dialogEl()!.querySelector('[data-testid="subscribe-done"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('无 options 的参数退化为输入框（不阻塞订阅）', async () => {
    const detail = rsshubDetail()
    detail.route!.parameters = JSON.stringify({ section: { description: '栏目代码' } })
    detailMock.mockResolvedValue({ success: true, data: detail })
    statusMock.mockResolvedValue(okStatus())
    const wrapper = await mountDialog()

    const field = dialogEl()!.querySelector('[data-testid="subscribe-param-section"]')!
    expect(field.querySelector('select')).toBeNull()
    const input = field.querySelector('input') as HTMLInputElement
    expect(input).not.toBeNull()
    input.value = 'design'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-final-url"]')!.textContent)
      .toBe('https://rsshub.example/test/journal/design')
    wrapper.unmount()
  })
})

describe('CandidateSubscribeDialog — 详情加载', () => {
  it('详情失败：错误态 + 重试入口，不发订阅请求', async () => {
    detailMock.mockResolvedValue({ success: false, error: '服务暂不可用' })
    statusMock.mockResolvedValue(okStatus())
    const wrapper = await mountDialog()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-load-error"]')!.textContent)
      .toContain('服务暂不可用')
    expect(dialogEl()!.querySelector('[data-testid="subscribe-load-retry"]')).not.toBeNull()
    expect(subscribeMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('详情已是订阅状态：直接进入「已订阅」终态（不给重复订阅入口）', async () => {
    detailMock.mockResolvedValue({ success: true, data: rssDetail({ subscribed: true }) })
    statusMock.mockResolvedValue(okStatus())
    const wrapper = await mountDialog()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-done"]')).not.toBeNull()
    expect(dialogEl()!.querySelector('[data-testid="subscribe-submit"]')).toBeNull()
    expect(subscribeMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
