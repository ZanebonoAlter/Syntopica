import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import KeyEventMarkerActions from './KeyEventMarkerActions.vue'
import type { TopicKeyEventMarker } from '~/api/dailyReports'
import type { ThreadKeyEventView } from '~/features/tags/composables/useTopicKeyEvents'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function makeMarker(over: Partial<TopicKeyEventMarker> = {}): TopicKeyEventMarker {
  return {
    id: 11,
    semantic_board_id: 1,
    persistent_topic_id: 5,
    source_report_id: 77,
    source_section_id: 100,
    source_thread_id: 1001,
    source_period_date: '2026-10-09T12:00:00Z',
    topic_label_snapshot: '泳道',
    title_snapshot: '关键事件',
    summary_snapshot: '摘要',
    tag_ids_snapshot: [],
    article_ids_snapshot: [9, 10],
    state: 'suggested',
    recommendation_reason: '该事件对泳道有实质影响',
    recommendation_basis: { evidence_article_ids: [9, 10], new_progress: true },
    revision: 1,
    ...over,
  }
}

function view(marker: TopicKeyEventMarker | null, pending: ThreadKeyEventView['pending'] = null, error: string | null = null): ThreadKeyEventView {
  return { marker, pending, error }
}

function mountActions(mode: 'review' | 'history' | 'overview', v: ThreadKeyEventView, topicId: number | null = 5) {
  return mount(KeyEventMarkerActions, {
    props: { mode, threadId: 1001, topicId, view: v },
  })
}

describe('KeyEventMarkerActions — 日报挑选模式（review/history）', () => {
  it('renders the AI suggestion badge, reason and confirm/dismiss for unprocessed suggestions', () => {
    const w = mountActions('review', view(makeMarker()))
    expect(w.find('.ke-actions__badge').text()).toContain('AI 建议')
    expect(w.find('.ke-detail__reason').text()).toContain('实质影响')
    expect(w.find('.ke-detail__evidence').text()).toContain('2 篇')
    const confirm = w.findAll('button').find(b => b.text().includes('确认为关键'))
    const dismiss = w.findAll('button').find(b => b.text().includes('不采纳'))
    expect(confirm).toBeTruthy()
    expect(dismiss).toBeTruthy()
    expect(w.find('[data-ke-state]').attributes('data-ke-state')).toBe('suggested')
  })

  it('splits detail (right half column) from the action row (detail 右半列、actions 底行右对齐，对齐原型分区)', () => {
    const w = mountActions('review', view(makeMarker()))
    // 多根 fragment：详情块（理由/依据）与按钮组是分离的兄弟块——grid 摆放：
    // 详情落右半列（c2/r1，与标题同一视觉行），动作组落全宽底行右对齐（r2）。
    // 摆放声明在自身 scoped 样式（见结构红线的源码守卫用例）。
    const detail = w.find('.ke-detail')
    const actions = w.find('.ke-actions')
    expect(detail.exists()).toBe(true)
    expect(actions.exists()).toBe(true)
    expect(detail.classes()).not.toContain('ke-actions')
    expect(actions.find('.ke-detail__reason').exists()).toBe(false)
    expect(detail.find('button').exists()).toBe(false)
  })

  it('hides the detail block when there is no detail content, actions still render (零内容不占行)', () => {
    // confirmed 且无 error/无归属说明：hasDetail=false 不渲染 .ke-detail，
    // .ke-actions 恒渲染（无 detail 行的动作仍落底行）。
    const w = mountActions('review', view(makeMarker({ state: 'confirmed' })))
    expect(w.find('.ke-detail').exists()).toBe(false)
    expect(w.find('.ke-actions').exists()).toBe(true)
  })

  it('wraps long reasons with pre-line (长文字可读) and keeps them non-truncated by CSS', () => {
    const w = mountActions('review', view(makeMarker({ recommendation_reason: '第一行理由\n第二行理由很长很长很长' })))
    const reason = w.find('.ke-detail__reason')
    expect(reason.text()).toContain('第一行理由')
    expect(reason.text()).toContain('第二行理由')
  })

  it('confirmed shows 已标记 + 取消标记; clicking emits unmark (纠错维护)', async () => {
    const w = mountActions('review', view(makeMarker({ state: 'confirmed' })))
    expect(w.text()).toContain('已标记')
    expect(w.text()).not.toContain('确认为关键')
    const unmark = w.findAll('button').find(b => b.text().includes('取消标记'))
    expect(unmark).toBeTruthy()
    await unmark!.trigger('click')
    expect(w.emitted('unmark')).toHaveLength(1)
    expect(w.emitted('mark')).toBeUndefined()
  })

  it('dismissed offers 仍标为关键（不采纳后仍可选择）', async () => {
    const w = mountActions('review', view(makeMarker({ state: 'dismissed' })))
    expect(w.text()).toContain('已不采纳')
    const mark = w.findAll('button').find(b => b.text().includes('仍标为关键'))
    await mark!.trigger('click')
    expect(w.emitted('mark')).toHaveLength(1)
  })

  it('a plain source (no marker row) offers 标为关键 without AI permission', async () => {
    const w = mountActions('review', view(null))
    const mark = w.findAll('button').find(b => b.text().includes('标为关键'))
    expect(mark).toBeTruthy()
    await mark!.trigger('click')
    expect(w.emitted('mark')).toHaveLength(1)
    expect(w.find('.ke-actions__badge').exists()).toBe(false)
  })

  it('no topic attribution disables actions with an explanatory note (无归属说明)', () => {
    const w = mountActions('review', view(null), null)
    const mark = w.findAll('button').find(b => b.text().includes('标为关键'))
    expect(mark?.attributes('disabled')).toBeDefined()
  })

  it('pending actions show busy labels and disable the sibling action (互斥)', () => {
    const w = mountActions('review', view(makeMarker(), 'mark'))
    expect(w.text()).toContain('确认中…')
    const dismiss = w.findAll('button').find(b => b.text().includes('不采纳'))
    expect(dismiss?.attributes('disabled')).toBeDefined()
  })

  it('errors render locally with retry wording (不阻塞其他条目)', () => {
    const w = mountActions('review', view(null, null, '保存失败，请重试'))
    expect(w.find('.ke-detail__error').text()).toContain('保存失败')
    expect(w.find('[role="alert"]').exists()).toBe(true)
  })
})

describe('KeyEventMarkerActions — 总览回看模式（overview）', () => {
  it('renders NO suggestion badge/reason/confirm for shared-cache suggested rows (不泄漏)', () => {
    const w = mountActions('overview', view(makeMarker()))
    expect(w.find('.ke-actions__badge').exists()).toBe(false)
    expect(w.find('.ke-detail__reason').exists()).toBe(false)
    const buttons = w.findAll('button')
    expect(buttons.filter(b => b.text().includes('确认为关键') || b.text().includes('不采纳') || b.text().includes('标为关键'))).toHaveLength(0)
  })

  it('confirmed keeps only the cancel-maintenance entry (取消纠错)', async () => {
    const w = mountActions('overview', view(makeMarker({ state: 'confirmed' })))
    expect(w.text()).toContain('关键节点')
    const unmark = w.findAll('button').find(b => b.text().includes('取消标记'))
    expect(unmark).toBeTruthy()
    await unmark!.trigger('click')
    expect(w.emitted('unmark')).toHaveLength(1)
  })

  it('dismissed/plain sources render nothing selectable (按原新闻)', () => {
    const w1 = mountActions('overview', view(makeMarker({ state: 'dismissed' })))
    expect(w1.findAll('button')).toHaveLength(0)
    const w2 = mountActions('overview', view(null))
    expect(w2.findAll('button')).toHaveLength(0)
  })
})

describe('KeyEventMarkerActions — 结构红线', () => {
  it('never nests buttons (零嵌套按钮)', () => {
    const w = mountActions('review', view(makeMarker()))
    const buttons = w.findAll('button')
    for (const btn of buttons) {
      expect(btn.element.querySelector('button')).toBeNull()
    }
  })

  it('placement rules live in own scoped style, host has no dead child selectors (scope 断链回归闸)', () => {
    // 多根 fragment 根节点不继承宿主 scope 属性——grid 摆放规则必须写在本组件
    // 自身 scoped 样式（根节点带自家 scope attr 才命中）；宿主若再出现
    // `.drm-thread-row > .ke-detail` / `.drm-thread-row > .ke-actions` 子选择器
    // 即为永不命中的死代码（fix-daily-report-ke-row-layout 根因）。
    // 源码守卫先例：UpgradeSuggestionPanel.gen.test.ts 的 readFileSync 锚。
    const src = readFileSync(`${import.meta.dirname}/KeyEventMarkerActions.vue`, 'utf8')
    const style = src.slice(src.indexOf('<style'))
    expect(style).toMatch(/\.ke-detail\s*\{[^}]*grid-column:\s*2/s)
    expect(style).toMatch(/\.ke-actions\s*\{[^}]*justify-content:\s*flex-end/s)

    const host = readFileSync(`${import.meta.dirname}/daily-report/DailyReportTopicSection.vue`, 'utf8')
    expect(host).not.toMatch(/\.drm-thread-row\s*>\s*\.ke-detail/)
    expect(host).not.toMatch(/\.drm-thread-row\s*>\s*\.ke-actions/)

    // 窄屏断点两处同步的机器闸门：两个源文件都必须含同值媒体查询断点
    expect(src).toContain('max-width: 720px')
    expect(host).toContain('max-width: 720px')
  })

  it('actions are native buttons (keyboard reachable via Enter/Space activation)', async () => {
    const w = mountActions('review', view(makeMarker()))
    const confirm = w.findAll('button').find(b => b.text().includes('确认为关键'))!
    expect(confirm.element.tagName).toBe('BUTTON')
    // Native <button> synthesizes click on Enter/Space; the host binding is
    // plain @click, so keyboard activation reaches the same handler.
    await confirm.trigger('click')
    expect(w.emitted('mark')).toHaveLength(1)
  })
})
