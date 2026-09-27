import { describe, expect, it, vi, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MarginNotesRail from './MarginNotesRail.vue'
import type { MarginNoteAnnotation } from '~/api/marginNotes'
import type { MarginNoteAskState } from '~/features/tags/composables/useMarginNotes'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function makeNote(over: Partial<MarginNoteAnnotation> = {}): MarginNoteAnnotation {
  return {
    id: 5,
    report_id: 10,
    section_id: 100,
    thread_id: 7,
    quoted_text: '开展逆回购操作，中标利率不变',
    anchor_offset_start: null,
    anchor_offset_end: null,
    created_at: '2026-09-22T00:00:00Z',
    qas: [
      {
        id: 9,
        annotation_id: 5,
        question: '逆回购是什么？',
        answer: '短期流动性注入操作。',
        cited_article_ids: [101],
        cited_web_sources: [],
        extracted_terms: [],
        created_at: '2026-09-22T01:00:00Z',
      },
    ],
    ...over,
  }
}


const mountedWrappers: Array<{ unmount: () => void }> = []

function mountRail(props: Record<string, unknown> = {}) {
  const on = {
    retryLoad: vi.fn(),
    jump: vi.fn(),
    remove: vi.fn(),
    ask: vi.fn(),
    retryAsk: vi.fn(),
    openCited: vi.fn(),
  }
  const wrapper = mount(MarginNotesRail, {
    props: {
      notes: [] as MarginNoteAnnotation[],
      loading: false,
      loadError: '',
      askStates: new Map<number, MarginNoteAskState>(),
      onRetryLoad: on.retryLoad,
      onJump: on.jump,
      onRemove: on.remove,
      onAsk: on.ask,
      onRetryAsk: on.retryAsk,
      onOpenCited: on.openCited,
      ...props,
    },
    attachTo: document.body,
  })
  mountedWrappers.push(wrapper)
  return { wrapper, on }
}

afterEach(() => {
  mountedWrappers.splice(0).forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
})

describe('MarginNotesRail（页边注栏状态矩阵）', () => {
  it('AV-3：加载中显示骨架行，不闪空态', () => {
    const { wrapper } = mountRail({ loading: true })
    expect(wrapper.find('[data-testid="mn-rail-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="mn-rail-empty"]').exists()).toBe(false)
  })

  it('落锚失败：行内提示不静默（且与空态/加载态并存互不干扰）', () => {
    const { wrapper } = mountRail({ anchorError: '落锚失败', loading: false })
    const alert = wrapper.find('[data-testid="mn-anchor-error"]')
    expect(alert.exists()).toBe(true)
    expect(alert.text()).toContain('落锚失败')
    // 独立 v-if：不能挤掉空态渲染（曾经插在 v-if 链中间切割了 v-else-if 归属）
    expect(wrapper.find('[data-testid="mn-rail-empty"]').exists()).toBe(true)
    // 无错误时不渲染
    const { wrapper: clean } = mountRail()
    expect(clean.find('[data-testid="mn-anchor-error"]').exists()).toBe(false)
  })

  it('specs 空态：极轻灰色提示，无卡片容器边框', () => {
    const { wrapper } = mountRail()
    const empty = wrapper.find('[data-testid="mn-rail-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('划选左侧正文文字')
    // 空态不渲染任何卡片（无卡片容器边框）
    expect(wrapper.findAll('.mn-card').length).toBe(0)
  })

  it('加载失败：整栏错误 + 重试（State Matrix 恢复路径）', async () => {
    const { wrapper, on } = mountRail({ loadError: '批注加载失败' })
    expect(wrapper.find('[data-testid="mn-rail-error"]').exists()).toBe(true)
    await wrapper.find('button').trigger('click')
    expect(on.retryLoad).toHaveBeenCalledTimes(1)
  })

  it('有批注：渲染卡片，卡内 QA 折叠语义透传（ask/remove/jump/openCited 上抛）', async () => {
    const { wrapper, on } = mountRail({ notes: [makeNote()] })
    const card = wrapper.find('[data-annotation-id="5"]')
    expect(card.exists()).toBe(true)
    await wrapper.find('[data-testid="mn-jump"]').trigger('click')
    expect(on.jump).toHaveBeenCalledWith(5)
    await wrapper.find('[data-testid="mn-delete"]').trigger('click')
    // 删除先弹确认（AppDialog Teleport 到 body），不直接 remove
    expect(on.remove).not.toHaveBeenCalled()
    const confirmText = document.body.querySelector('[data-testid="mn-confirm-text"]')
    expect(confirmText?.textContent).toContain('1')
    expect(confirmText?.textContent).toContain('连带删除')
    ;(document.body.querySelector('[data-testid="mn-confirm-delete"]') as HTMLElement).click()
    await flushPromises()
    expect(on.remove).toHaveBeenCalledWith(5)
    await wrapper.find('[data-cited-article="101"]').trigger('click')
    expect(on.openCited).toHaveBeenCalledWith(5, 101)
  })

  it('ID-3：删除确认取消 → 不发 remove、半透明态恢复', async () => {
    const { wrapper, on } = mountRail({ notes: [makeNote()] })
    await wrapper.find('[data-testid="mn-delete"]').trigger('click')
    expect(wrapper.find('.mn-card--deleting').exists()).toBe(true)
    ;(document.body.querySelector('[data-testid="mn-cancel-delete"]') as HTMLElement).click()
    await flushPromises()
    expect(on.remove).not.toHaveBeenCalled()
    expect(wrapper.find('.mn-card--deleting').exists()).toBe(false)
  })

  it('litId 变化：对应卡获得 lit 点亮态', async () => {
    const { wrapper } = mountRail({ notes: [makeNote()], litId: null })
    await wrapper.setProps({ litId: 5 })
    expect(wrapper.find('.mn-card--lit').exists()).toBe(true)
  })

  it('changedIds：对应卡呈「原文已变更」降级态（PR-3）', () => {
    const { wrapper } = mountRail({ notes: [makeNote()], changedIds: [5] })
    expect(wrapper.find('[data-testid="mn-changed"]').exists()).toBe(true)
  })
})
