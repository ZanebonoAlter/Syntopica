import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { readFileSync } from 'node:fs'
import ThreadRelatedEvents from './ThreadRelatedEvents.vue'
import { Z_INDEX } from '~/components/ui/layout-contract'
import { hasOpenOverlay, resetOverlayStack } from '~/components/ui/overlayStack'
import type { ThreadRelationItem, ThreadRelationsResponse } from '~/api/dailyReports'
import type { RequestCacheEntry } from './dailyReportMagazine'

/**
 * 线索相关引用 chip 行（replace-lane-lifeline-with-topic-events 4.1–4.3 / 7.3）。
 *
 * 形态红线：chip 行 + 悬停/聚焦预览浮层 + 点击开文章。不得退化成按天分组的
 * 事件列表/时间轴；无候选时整行不渲染（不占位、不写空态文案）。
 * 档位（7/30 天）已移到分区级工具条（review R-3）——本组件不渲染档位控件。
 */

function makeItem(over: Partial<ThreadRelationItem> = {}): ThreadRelationItem {
  return {
    thread_id: 9001,
    title: '早前铺垫过的同一条线',
    summary: '历史线索摘要：企业级 Agent 平台需要补齐托管与度量基建。',
    period_date: '2026-11-02',
    score: 0.91,
    level: 'strong',
    confirmed: false,
    source_article: { id: 555, title: '来源文章标题', feed: '机器之心', published_at: '2026-11-02T08:00:00Z' },
    ...over,
  }
}

function successEntry(items: ThreadRelationItem[], days = 7): RequestCacheEntry<ThreadRelationsResponse> {
  return {
    status: 'success',
    data: {
      report_id: 77,
      days,
      window_start: '2026-11-11',
      window_end: '2026-11-16',
      status: items.length ? 'ready' : 'empty',
      threads: items.length ? { '1': items } : {},
      total: items.length,
      skipped_no_embedding: 0,
    },
  }
}

function mountRelated(entry: RequestCacheEntry<ThreadRelationsResponse>) {
  return mount(ThreadRelatedEvents, {
    props: { threadId: 1, reportId: 77, entry },
    attachTo: document.body,
  })
}

function tip(): HTMLElement | null {
  return document.body.querySelector('.tre-tip')
}

afterEach(() => {
  document.body.innerHTML = ''
  resetOverlayStack()
})

describe('ThreadRelatedEvents — chip 行', () => {
  it('最多渲染 3 个 chip，其余折叠成「还有 N 条」，展开后补齐且按钮消失', async () => {
    const items = [1, 2, 3, 4, 5].map(i => makeItem({ thread_id: 9000 + i, title: `候选 ${i}` }))
    const wrapper = mountRelated(successEntry(items))

    expect(wrapper.findAll('button[data-thread-id]')).toHaveLength(3)
    const more = wrapper.find('[data-testid="thread-related-more-1"]')
    expect(more.text()).toBe('还有 2 条')
    // R-14：折叠器暴露展开态（展开后按钮消失是已批准形态，末次渲染仍是 false）
    expect(more.attributes('aria-expanded')).toBe('false')

    await more.trigger('click')
    expect(wrapper.findAll('button[data-thread-id]')).toHaveLength(5)
    expect(wrapper.find('[data-testid="thread-related-more-1"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('★ 只给人工确认过的候选（描边 + 星号同取 accent）', () => {
    const wrapper = mountRelated(successEntry([
      makeItem({ thread_id: 9001, title: '已确认的线', confirmed: true }),
      makeItem({ thread_id: 9002, title: '未确认的线', confirmed: false }),
    ]))

    const chips = wrapper.findAll('button[data-thread-id]')
    const confirmed = chips[0]!
    const plain = chips[1]!
    expect(confirmed.classes()).toContain('tre__chip--confirmed')
    expect(confirmed.find('.tre__mark').text()).toBe('★')
    expect(confirmed.attributes('aria-label')).toContain('已确认关键事件')
    expect(plain.classes()).not.toContain('tre__chip--confirmed')
    expect(plain.find('.tre__mark').exists()).toBe(false)
    wrapper.unmount()
  })

  it('长标题单行省略，title 保留全文（hover 也能看到完整标题）', () => {
    const longTitle = '这条历史线索的标题非常长非常长非常长非常长非常长非常长非常长非常长'
    const wrapper = mountRelated(successEntry([makeItem({ title: longTitle })]))

    const chip = wrapper.find('button[data-thread-id]')
    expect(chip.attributes('title')).toBe(longTitle)
    expect(chip.find('.tre__label').text()).toBe(longTitle)
    wrapper.unmount()
  })

  it('chip 上不出现分数（小数只进浮层，裸数字会被读成重要度）', () => {
    const wrapper = mountRelated(successEntry([makeItem({ score: 0.913 })]))
    expect(wrapper.find('button[data-thread-id]').text()).not.toContain('0.91')
    wrapper.unmount()
  })

  it('不渲染 7/30 档位控件（档位在分区级工具条，R-3）', () => {
    const wrapper = mountRelated(successEntry([makeItem()]))
    expect(wrapper.find('.tre__days').exists()).toBe(false)
    expect(wrapper.find('[data-testid^="thread-related-days"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('天')
    wrapper.unmount()
  })
})

describe('ThreadRelatedEvents — 预览浮层', () => {
  it('悬停出浮层：标题 → 摘要 → 来源文章 → 档位+分数 → 日期', async () => {
    const wrapper = mountRelated(successEntry([makeItem()]))
    await wrapper.find('button[data-thread-id]').trigger('mouseenter')
    await flushPromises()

    const tipEl = tip()
    expect(tipEl).not.toBeNull()
    expect(tipEl!.querySelector('.tre-tip__title')?.textContent).toBe('早前铺垫过的同一条线')
    expect(tipEl!.querySelector('.tre-tip__summary')?.textContent).toContain('托管与度量基建')
    expect(tipEl!.querySelector('.tre-tip__source')?.textContent).toContain('来源文章标题')
    expect(tipEl!.querySelector('.tre-tip__level')?.textContent).toBe('强关联')
    expect(tipEl!.querySelector('.tre-tip__score')?.textContent).toBe('0.91')
    expect(tipEl!.querySelector('.tre-tip__date')?.textContent).toBe('2026-11-02')
    // 定位 = 档位表推算的 z-index + 视口坐标（fixed 浮层，不被线索行裁剪）
    expect(tipEl!.style.left).toMatch(/px$/)
    expect(tipEl!.style.top).toMatch(/px$/)
    expect(tipEl!.style.zIndex).toBe(String(Z_INDEX.readingTooltip))
    expect(tipEl!.getAttribute('role')).toBe('tooltip')
    wrapper.unmount()
  })

  it('related 档用「相关」而不是「弱关联」以外的杜撰词', async () => {
    const wrapper = mountRelated(successEntry([makeItem({ level: 'related', score: 0.8 })]))
    await wrapper.find('button[data-thread-id]').trigger('mouseenter')
    await flushPromises()
    expect(tip()!.querySelector('.tre-tip__level')?.textContent).toBe('相关')
    wrapper.unmount()
  })

  it('分数向零截断，不四舍五入跨过 0.85 档位边界（R-8）', async () => {
    const wrapper = mountRelated(successEntry([makeItem({ level: 'related', score: 0.8499 })]))
    await wrapper.find('button[data-thread-id]').trigger('mouseenter')
    await flushPromises()
    // 0.8499 四舍五入会显示 0.85（strong 边界），与「相关」自相矛盾
    expect(tip()!.querySelector('.tre-tip__score')?.textContent).toBe('0.84')
    expect(tip()!.querySelector('.tre-tip__level')?.textContent).toBe('相关')
    wrapper.unmount()
  })

  it('恰好 0.85（strong 边界）如实显示 0.85', async () => {
    const wrapper = mountRelated(successEntry([makeItem({ level: 'strong', score: 0.85 })]))
    await wrapper.find('button[data-thread-id]').trigger('mouseenter')
    await flushPromises()
    expect(tip()!.querySelector('.tre-tip__score')?.textContent).toBe('0.85')
    wrapper.unmount()
  })

  it('键盘聚焦同样出浮层（不依赖 hover），并带 aria-describedby', async () => {
    const wrapper = mountRelated(successEntry([makeItem()]))
    const chip = wrapper.find('button[data-thread-id]')
    await chip.trigger('focus')
    await flushPromises()

    expect(tip()).not.toBeNull()
    expect(chip.attributes('aria-describedby')).toBe('thread-related-tip-1')
    wrapper.unmount()
  })

  it('Esc 关闭浮层（不扰动线索行，chip 仍可再次聚焦出浮层）', async () => {
    const wrapper = mountRelated(successEntry([makeItem()]))
    const chip = wrapper.find('button[data-thread-id]')
    await chip.trigger('focus')
    await flushPromises()
    expect(tip()).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(tip()).toBeNull()
    expect(chip.attributes('aria-describedby')).toBeUndefined()

    // Esc 只关浮层：再聚焦仍能重新打开
    await chip.trigger('focus')
    await flushPromises()
    expect(tip()).not.toBeNull()
    wrapper.unmount()
  })

  it('浮层打开时登记浮层栈：同一 Esc 只关浮层，阅读台不被踢出（R-4）', async () => {
    // 宿主复刻阅读台口径：document 级 Esc，浮层开着时让位（不 emit close）
    const hostClose = vi.fn()
    function hostKeydown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return
      if (hasOpenOverlay()) return
      hostClose()
    }
    document.addEventListener('keydown', hostKeydown)
    resetOverlayStack()

    try {
      const wrapper = mountRelated(successEntry([makeItem()]))
      expect(hasOpenOverlay()).toBe(false)

      await wrapper.find('button[data-thread-id]').trigger('focus')
      await flushPromises()
      expect(tip()).not.toBeNull()
      expect(hasOpenOverlay()).toBe(true)

      // bubbles: true → document（宿主，先）→ window（本组件的 keydown）
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      await nextTick()

      expect(tip()).toBeNull()
      expect(hostClose).not.toHaveBeenCalled()
      // 浮层隐藏后计数归还，再按 Esc 才轮到阅读台
      expect(hasOpenOverlay()).toBe(false)
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      expect(hostClose).toHaveBeenCalledTimes(1)
      wrapper.unmount()
    } finally {
      document.removeEventListener('keydown', hostKeydown)
    }
  })

  it('滚动/resize 让 fixed 浮层脱位 → 直接收起（R-13）', async () => {
    const wrapper = mountRelated(successEntry([makeItem()]))
    const chip = wrapper.find('button[data-thread-id]')

    await chip.trigger('mouseenter')
    await flushPromises()
    expect(tip()).not.toBeNull()

    window.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(tip()).toBeNull()

    // resize 同口径（重新聚焦后再触发一次）
    await chip.trigger('focus')
    await flushPromises()
    expect(tip()).not.toBeNull()
    window.dispatchEvent(new Event('resize'))
    await nextTick()
    expect(tip()).toBeNull()

    // 隐藏后解绑：不再关心 scroll（重复触发不抛、也不再改状态）
    window.dispatchEvent(new Event('scroll'))
    expect(tip()).toBeNull()
    wrapper.unmount()
  })

  it('摘要缺失时浮层不出空段落（其余三段照常）', async () => {
    const wrapper = mountRelated(successEntry([makeItem({ summary: '', source_article: null })]))
    await wrapper.find('button[data-thread-id]').trigger('mouseenter')
    await flushPromises()

    expect(tip()!.querySelector('.tre-tip__summary')).toBeNull()
    expect(tip()!.querySelector('.tre-tip__source')).toBeNull()
    expect(tip()!.querySelector('.tre-tip__title')).not.toBeNull()
    wrapper.unmount()
  })
})

describe('ThreadRelatedEvents — 点击开文章', () => {
  it('有来源文章时点击 emit openArticle（带相关度上下文，R-1）', async () => {
    const wrapper = mountRelated(successEntry([makeItem()]))
    await wrapper.find('button[data-thread-id]').trigger('click')

    expect(wrapper.emitted('openArticle')).toEqual([[
      { articleId: 555, score: 0.91, level: 'strong', periodDate: '2026-11-02' },
    ]])
    wrapper.unmount()
  })

  it('没有来源文章的候选不弹空弹窗（不 emit、光标默认）', async () => {
    const wrapper = mountRelated(successEntry([makeItem({ source_article: null })]))
    const chip = wrapper.find('button[data-thread-id]')
    expect(chip.classes()).toContain('tre__chip--static')

    await chip.trigger('click')
    expect(wrapper.emitted('openArticle')).toBeUndefined()
    wrapper.unmount()
  })
})

describe('ThreadRelatedEvents — 无候选/失败时不渲染', () => {
  it('该线索没有候选 → 整行不渲染，也没有「暂无相关」文案', () => {
    const wrapper = mountRelated(successEntry([]))
    expect(wrapper.find('[data-testid="thread-related-1"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
    expect(wrapper.html()).not.toContain('暂无')
    wrapper.unmount()
  })

  it('响应里只有别的线索 → 本线索同样不渲染', () => {
    const entry: RequestCacheEntry<ThreadRelationsResponse> = {
      status: 'success',
      data: { ...successEntry([makeItem()]).data!, threads: { '2': [makeItem()] } },
    }
    const wrapper = mountRelated(entry)
    expect(wrapper.find('[data-testid="thread-related-1"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('请求失败 → 不渲染（推荐失败不阻断阅读），也不弹提示', () => {
    const wrapper = mountRelated({ status: 'error', error: '相关线索加载失败' })
    expect(wrapper.find('[data-testid="thread-related-1"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
    wrapper.unmount()
  })

  it('加载中渲染两个静态骨架 chip（切档时 chip 行不闪没）', () => {
    const wrapper = mountRelated({ status: 'loading' })
    const row = wrapper.find('[data-testid="thread-related-1"]')
    expect(row.exists()).toBe(true)
    expect(row.attributes('aria-busy')).toBe('true')
    expect(row.findAll('.tre__chip--skeleton')).toHaveLength(2)
    wrapper.unmount()
  })
})

describe('ThreadRelatedEvents — 展示合理性锚', () => {
  it('浮层定位规则留在 <style>（fixed + 视口内宽度上限），层级取档位表不手写数字', () => {
    const source = readFileSync(`${import.meta.dirname}/ThreadRelatedEvents.vue`, 'utf8')
    expect(source).toContain('position: fixed')
    expect(source).toContain('width: min(24rem, calc(100vw - 2rem))')
    expect(source).toContain('Z_INDEX.readingTooltip')
    expect(source).not.toContain('Z_INDEX.readingDialog - 1')
    expect(source).toContain('prefers-reduced-motion')
  })

  it('浮层 Teleport 到 body（不被线索行 overflow 裁剪）', async () => {
    const wrapper = mountRelated(successEntry([makeItem()]))
    await wrapper.find('button[data-thread-id]').trigger('mouseenter')
    await flushPromises()

    expect(wrapper.element.contains(tip())).toBe(false)
    expect(tip()?.parentElement).toBe(document.body)
    wrapper.unmount()
  })
})
