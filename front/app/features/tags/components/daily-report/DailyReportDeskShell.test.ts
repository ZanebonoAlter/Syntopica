import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import DailyReportDeskShell from './DailyReportDeskShell.vue'

const routeState = { query: {} as Record<string, string> }
const router = { push: vi.fn(), replace: vi.fn() }
const getBoards = vi.fn()
const { getArticle, updateArticle, notifyError, notifyWarn } = vi.hoisted(() => ({
  getArticle: vi.fn(),
  updateArticle: vi.fn(),
  notifyError: vi.fn(),
  notifyWarn: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => router,
}))

vi.mock('~/api/semanticBoards', () => ({
  useSemanticBoardsApi: () => ({ getBoards }),
}))
vi.mock('~/api/articles', () => ({
  useArticlesApi: () => ({ getArticle, updateArticle }),
}))
vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ success: vi.fn(), error: notifyError, warn: notifyWarn }),
}))
vi.mock('../ArticlePreviewModal.vue', () => ({
  default: {
    name: 'ArticlePreviewModal',
    props: ['visible', 'selectedPreviewArticle', 'loadingPreviewArticle', 'relatedContext'],
    emits: ['close', 'favorite', 'article-update', 'navigate'],
    template: '<div data-testid="article-preview-stub" />',
  },
}))

const readerStub = {
  name: 'DailyReportReader',
  props: ['boardId', 'boardTitle', 'boards', 'initialDate', 'initialAnnotationId', 'initialClusterId', 'initialLegacyStory', 'mode'],
  emits: ['close', 'selectDate', 'openLane', 'openArticle', 'selectBoard'],
  template: '<div data-testid="reader-stub" />',
}

const previewStub = {
  name: 'ArticlePreviewModal',
  props: ['visible', 'selectedPreviewArticle', 'loadingPreviewArticle', 'relatedContext'],
  emits: ['close', 'favorite', 'article-update', 'navigate'],
  template: '<div data-testid="article-preview-stub" />',
}

function mountShell() {
  return mount(DailyReportDeskShell, {
    props: { boardId: 1974, date: '2026-10-23' },
    global: {
      stubs: { DailyReportReader: readerStub },
    },
  })
}

/** 线索点击直达预览（daily-report-lead-direct-preview）：
 * reader emit openArticle(id) → shell getArticle → 弹窗直接打开（无抽屉中转层） */
async function openPreviewWith(wrapper: ReturnType<typeof mountShell>, favorite = false) {
  getArticle.mockResolvedValue({ success: true, data: leadArticle(favorite) })
  wrapper.findComponent(readerStub).vm.$emit('openArticle', 158497)
  await flushPromises()
}

function leadArticle(favorite = false) {
  return {
    id: 158497,
    feed_id: 1,
    title: '一篇文章',
    link: 'https://example.com/a',
    pub_date: '2026-10-28T10:00:00+08:00',
    created_at: '2026-10-28T10:00:00+08:00',
    read: false,
    favorite,
  }
}

describe('DailyReportDeskShell — 阅读台路由（6.1/6.4/6.5）', () => {
  beforeEach(() => {
    routeState.query = { lane: '1204' }
    router.push.mockClear()
    router.replace.mockClear()
    getBoards.mockReset().mockResolvedValue({
      success: true,
      data: { items: [{ id: 1974, label: '中东' }, { id: 1975, label: 'AI' }], total: 2 },
    })
    document.documentElement.style.fontSize = ''
  })

  it('?annotation= 深链透传 initialAnnotationId', async () => {
    routeState.query = { annotation: '8' }
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.findComponent(readerStub).props('initialAnnotationId')).toBe(8)
  })

  it('?cluster= 深链透传 initialClusterId（今日重点聚簇 → 日报正文定位）', async () => {
    routeState.query = { cluster: '42' }
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.findComponent(readerStub).props('initialClusterId')).toBe(42)
    expect(wrapper.findComponent(readerStub).props('initialLegacyStory')).toBe(false)
  })

  it('旧 ?story= 深链：仍被解析但只标记已下线（不定位、不报错）', async () => {
    routeState.query = { story: '42' }
    const wrapper = mountShell()
    await flushPromises()
    expect(wrapper.findComponent(readerStub).props('initialClusterId')).toBeNull()
    expect(wrapper.findComponent(readerStub).props('initialLegacyStory')).toBe(true)
  })

  it('?cluster= 非法值忽略（非数字 / 0 / 负数 → null，不报错阻断阅读）', async () => {
    for (const raw of ['abc', '0', '-1', '1.5', '']) {
      routeState.query = { cluster: raw }
      const wrapper = mountShell()
      await flushPromises()
      expect(wrapper.findComponent(readerStub).props('initialClusterId')).toBeNull()
      wrapper.unmount()
    }
  })

  it('翻页/切报告清参：切日期的 URL 不再带 ?story=（不在别的报告重复定位）', async () => {
    routeState.query = { lane: '1204', story: '42' }
    const wrapper = mountShell()
    await flushPromises()
    wrapper.findComponent(readerStub).vm.$emit('selectDate', '2026-10-22')
    expect(router.replace).toHaveBeenCalledWith({ path: '/tags/1974/2026-10-22', query: { lane: '1204' } })
  })

  it('返回矩阵：恢复进入前选中泳道', async () => {
    const wrapper = mountShell()
    await flushPromises()
    wrapper.findComponent(readerStub).vm.$emit('close')
    expect(router.push).toHaveBeenCalledWith({ path: '/tags/1974', query: { lane: '1204', focus: 'report' } })
  })

  it('切日期：replace URL 且保留 lane', async () => {
    const wrapper = mountShell()
    await flushPromises()
    wrapper.findComponent(readerStub).vm.$emit('selectDate', '2026-10-22')
    expect(router.replace).toHaveBeenCalledWith({ path: '/tags/1974/2026-10-22', query: { lane: '1204' } })
  })

  it('在矩阵中查看这条线：回矩阵并选中泳道', async () => {
    const wrapper = mountShell()
    await flushPromises()
    wrapper.findComponent(readerStub).vm.$emit('openLane', 88)
    expect(router.push).toHaveBeenCalledWith({ path: '/tags/1974', query: { lane: '88' } })
  })

  it('切版块：保留日期与 lane', async () => {
    const wrapper = mountShell()
    await flushPromises()
    wrapper.findComponent(readerStub).vm.$emit('selectBoard', 1975)
    expect(router.replace).toHaveBeenCalledWith({ path: '/tags/1975/2026-10-23', query: { lane: '1204' } })
  })

})

describe('DailyReportDeskShell — 线索文章直达站内阅读预览（daily-report-lead-direct-preview）', () => {
  beforeEach(() => {
    routeState.query = { lane: '1204' }
    router.push.mockClear()
    router.replace.mockClear()
    getArticle.mockReset().mockResolvedValue({ success: true, data: leadArticle() })
    updateArticle.mockReset().mockResolvedValue({ success: true, data: {} })
    notifyError.mockReset()
    notifyWarn.mockReset()
    getBoards.mockReset().mockResolvedValue({
      success: true,
      data: { items: [{ id: 1974, label: '中东' }], total: 1 },
    })
  })

  it('点线索文章：直达预览弹窗并填充文章（无中间抽屉层）', async () => {
    const wrapper = mountShell()
    await flushPromises()

    // 直达性锚：openArticle 直接使弹窗可见即为「无抽屉中转」——旧链路必须经抽屉 emit open-article 才会开
    await openPreviewWith(wrapper)

    expect(wrapper.findComponent(previewStub).props('visible')).toBe(true)
    expect(wrapper.findComponent(previewStub).props('selectedPreviewArticle')).toMatchObject({ id: '158497', favorite: false })
    expect(wrapper.findComponent(previewStub).props('loadingPreviewArticle')).toBe(false)
  })

  it('详情加载中：弹窗先开并呈现加载态，返回后替换为文章', async () => {
    const wrapper = mountShell()
    await flushPromises()

    let resolveFetch!: (value: { success: boolean, data: unknown }) => void
    getArticle.mockReturnValue(new Promise(resolve => { resolveFetch = resolve }))
    wrapper.findComponent(readerStub).vm.$emit('openArticle', 158497)
    await flushPromises()

    expect(wrapper.findComponent(previewStub).props('visible')).toBe(true)
    expect(wrapper.findComponent(previewStub).props('loadingPreviewArticle')).toBe(true)
    expect(wrapper.findComponent(previewStub).props('selectedPreviewArticle')).toBeNull()

    resolveFetch({ success: true, data: leadArticle() })
    await flushPromises()

    expect(wrapper.findComponent(previewStub).props('loadingPreviewArticle')).toBe(false)
    expect(wrapper.findComponent(previewStub).props('selectedPreviewArticle')).toMatchObject({ id: '158497' })
  })

  it('详情加载失败：可见提示且不留空白弹窗', async () => {
    getArticle.mockResolvedValue({ success: false, error: '加载失败' })
    const wrapper = mountShell()
    await flushPromises()

    wrapper.findComponent(readerStub).vm.$emit('openArticle', 158497)
    await flushPromises()

    expect(wrapper.findComponent(previewStub).props('visible')).toBe(false)
    expect(notifyWarn).toHaveBeenCalledWith('文章加载失败，请稍后重试')
  })

  // ── review R-1：chip 点击带相关度上下文，弹窗内多一行阅读上下文 ──
  it('数字入参（普通文章行）：relatedContext 为 null，弹窗不多渲染一行', async () => {
    const wrapper = mountShell()
    await flushPromises()

    await openPreviewWith(wrapper)
    expect(wrapper.findComponent(previewStub).props('relatedContext')).toBeNull()
  })

  it('对象入参（相关线索 chip）：上下文透传给预览弹窗，关闭后清空', async () => {
    const wrapper = mountShell()
    await flushPromises()

    wrapper.findComponent(readerStub).vm.$emit('openArticle', {
      articleId: 158497,
      score: 0.89,
      level: 'strong',
      periodDate: '2026-11-02',
    })
    await flushPromises()

    expect(getArticle).toHaveBeenCalledWith(158497)
    expect(wrapper.findComponent(previewStub).props('visible')).toBe(true)
    expect(wrapper.findComponent(previewStub).props('relatedContext')).toEqual({
      score: 0.89,
      level: 'strong',
      periodDate: '2026-11-02',
    })

    wrapper.findComponent(previewStub).vm.$emit('close')
    await flushPromises()
    expect(wrapper.findComponent(previewStub).props('visible')).toBe(false)
    expect(wrapper.findComponent(previewStub).props('relatedContext')).toBeNull()
  })
})

describe('DailyReportDeskShell — 站内阅读预览的状态同步（2026-10-28 用户反馈修复，契约继承）', () => {
  beforeEach(() => {
    routeState.query = { lane: '1204' }
    router.push.mockClear()
    router.replace.mockClear()
    getArticle.mockReset().mockResolvedValue({ success: true, data: leadArticle() })
    updateArticle.mockReset().mockResolvedValue({ success: true, data: {} })
    notifyError.mockReset()
    notifyWarn.mockReset()
    getBoards.mockReset().mockResolvedValue({
      success: true,
      data: { items: [{ id: 1974, label: '中东' }], total: 1 },
    })
  })

  it('点收藏：立即翻转并落库（不再是空实现无响应）', async () => {
    const wrapper = mountShell()
    await openPreviewWith(wrapper)

    wrapper.findComponent(previewStub).vm.$emit('favorite', '158497')
    await flushPromises()

    expect(updateArticle).toHaveBeenCalledWith(158497, { favorite: true })
    expect(wrapper.findComponent(previewStub).props('selectedPreviewArticle')).toMatchObject({ id: '158497', favorite: true })
    expect(notifyError).not.toHaveBeenCalled()
  })

  it('收藏失败：回滚到点击前状态并提示', async () => {
    updateArticle.mockResolvedValue({ success: false, error: '写入失败' })
    const wrapper = mountShell()
    await openPreviewWith(wrapper)

    wrapper.findComponent(previewStub).vm.$emit('favorite', '158497')
    await flushPromises()

    expect(wrapper.findComponent(previewStub).props('selectedPreviewArticle')).toMatchObject({ favorite: false })
    expect(notifyError).toHaveBeenCalledWith('写入失败')
  })

  it('预览内文章状态更新回流到当前文章', async () => {
    const wrapper = mountShell()
    await openPreviewWith(wrapper)

    wrapper.findComponent(previewStub).vm.$emit('article-update', '158497', { read: true })
    await flushPromises()

    expect(wrapper.findComponent(previewStub).props('selectedPreviewArticle')).toMatchObject({ id: '158497', read: true })
  })

})
