import { describe, expect, it, vi } from 'vitest'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import DailyReportTopicSection from './DailyReportTopicSection.vue'
import type { DailyReportSection, DailyReportThread, ThreadRelationsResponse } from '~/api/dailyReports'
import type { QualityZone } from './dailyReportMagazine'

/**
 * 线索相关引用 chip 行的布局回归（replace-lane-lifeline-with-topic-events 7.4）。
 *
 * 崩坏根因（explore-findings「话题泳道窄容器崩坏根因」）：旧折线把详情面板用
 * `gridColumn: N / span 2` 塞进 7 列无 min-width 网格，正文列收缩到 200–300px 时塌。
 * 所以本特性硬约束：chip 行是**整宽一行**，位于 `.drm-thread-row`（两列 grid）之后，
 * 全仓不得再有 `gridColumn: … / span 2` 式的定位残留。
 */

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

const relations = vi.hoisted(() => ({
  getThreadRelations: vi.fn(),
}))

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ getThreadRelations: relations.getThreadRelations }),
}))

function makeThread(id: number): DailyReportThread {
  return {
    id,
    report_id: 88,
    section_id: 100,
    title: `线索${id}`,
    summary: '摘要',
    tag_ids: [],
    confidence: 0.9,
    related_article_ids: [],
    created_at: '2026-11-17T12:00:00Z',
  }
}

function makeSection(threads: DailyReportThread[]): DailyReportSection {
  return {
    id: 100,
    cluster_index: 0,
    cluster_label: '板块100',
    cluster_tag_ids: [],
    threads,
    article_count: 1,
    best_tier: 0,
    avg_score: 0.9,
    persistent_topic_id: 5,
    persistent_topic: {
      id: 5,
      label: '测试话题',
      status: 'active',
      color: '#b44f45',
      consecutive_hits: 3,
      can_activate: false,
    },
  }
}

function activeZone(sections: DailyReportSection[]): QualityZone {
  return { key: 'active', label: '关心的话题', eyebrow: 'Following', sections }
}

function relationsResponse(reportId: number, threadId: number): ThreadRelationsResponse {
  return {
    report_id: reportId,
    days: 7,
    window_start: '2026-11-11',
    window_end: '2026-11-16',
    status: 'ready',
    threads: {
      [String(threadId)]: [{
        thread_id: 9001,
        title: '早前铺垫过的同一条线',
        summary: '历史摘要',
        period_date: '2026-11-12',
        score: 0.9,
        level: 'strong',
        confirmed: false,
        source_article: { id: 555, title: '来源文章', feed: 'feed', published_at: null },
      }],
    },
    total: 1,
    skipped_no_embedding: 0,
  }
}

function mountSection(reportId = 88) {
  return mount(DailyReportTopicSection, {
    props: {
      zone: activeZone([makeSection([makeThread(1)])]),
      reportId,
      reportDate: '2026-11-17T12:00:00Z',
      articleEntries: new Map(),
      laneDynamicsEntry: { status: 'idle' as const },
      contextEntries: new Map(),
    },
    global: {
      stubs: {
        SectionTierBadge: { name: 'SectionTierBadge', template: '<span class="tier-stub" />' },
        SectionAnchorBadge: { name: 'SectionAnchorBadge', template: '<span class="anchor-stub" />' },
        SectionQualityExplore: { name: 'SectionQualityExplore', template: '<span class="explore-stub" />' },
        KeyEventMarkerActions: { name: 'KeyEventMarkerActions', template: '<span class="ke-stub" />' },
      },
    },
  })
}

/** 递归收集 front/app 下的源码文件（排除测试文件本身）。 */
function collectSources(dir: string, acc: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry.startsWith('.')) continue
    const full = join(dir, entry)
    const stat = statSync(full)
    if (stat.isDirectory()) collectSources(full, acc)
    else if (/\.(vue|ts)$/.test(entry) && !entry.endsWith('.test.ts')) acc.push(full)
  }
  return acc
}

describe('DailyReportEvents 布局回归 — chip 行整宽一行', () => {
  it('chip 行是 .drm-thread-row 的兄弟后继，不在其两列 grid 内', async () => {
    relations.getThreadRelations.mockResolvedValue({ success: true, data: relationsResponse(88, 1) })
    const wrapper = mountSection()
    await flushPromises()
    await nextTick()

    const row = wrapper.find('.drm-thread-row')
    const chipRow = wrapper.find('[data-testid="thread-related-1"]')
    expect(row.exists()).toBe(true)
    expect(chipRow.exists()).toBe(true)

    // 兄弟关系 = 不在两列 grid 里（塞进 grid 会打乱 KeyEventMarkerActions 的自动摆放）
    expect(row.element.nextElementSibling).toBe(chipRow.element)
    expect(row.element.contains(chipRow.element)).toBe(false)
    // chip 行在 .drm-thread 容器内、文章列表之前
    expect(wrapper.find('.drm-thread').element.contains(chipRow.element)).toBe(true)
    wrapper.unmount()
  })

  it('无相关线索时线索行结构与引入本特性前一致（无 chip 行、无占位）', async () => {
    relations.getThreadRelations.mockResolvedValue({
      success: true,
      data: { ...relationsResponse(89, 1), status: 'empty', threads: {}, total: 0 },
    })
    const wrapper = mountSection(89)
    await flushPromises()
    await nextTick()

    const thread = wrapper.find('.drm-thread')
    expect(wrapper.find('[data-testid="thread-related-1"]').exists()).toBe(false)
    // 线索容器只剩 row +（折叠状态下的）提示行，且没有 min-height 占位块
    expect(thread.element.querySelector('.drm-thread-row')).not.toBeNull()
    expect(thread.element.querySelector('[data-testid^="thread-related"]')).toBeNull()
    wrapper.unmount()
  })

  it('.drm-thread-row 仍是两列 grid，chip 行不参与其列定义', () => {
    const source = readFileSync(`${import.meta.dirname}/DailyReportTopicSection.vue`, 'utf8')
    expect(source).toContain('.drm-thread-row {')
    expect(source).toContain('grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);')
    // chip 行挂在 thread 容器上（不在 row 内部）
    expect(source).toContain('<ThreadRelatedEvents')
  })

  it('全仓无 `gridColumn: N / span 2` 式跨列定位残留（旧折线崩坏载体）', () => {
    const appRoot = join(import.meta.dirname, '../../../..')
    const sources = collectSources(appRoot)
    expect(sources.length).toBeGreaterThan(50)

    const offenders: string[] = []
    for (const file of sources) {
      const source = readFileSync(file, 'utf8')
      if (/gridColumn\s*[:=]/.test(source) || /span\s+2['"`]?\s*[,}]/.test(source)) offenders.push(file)
    }
    expect(offenders, `仍有跨列定位残留：\n${offenders.join('\n')}`).toEqual([])
  })

  it('日报侧不再渲染折线/节点图（组件已删除，无 svg 连线）', async () => {
    relations.getThreadRelations.mockResolvedValue({ success: true, data: relationsResponse(90, 1) })
    const wrapper = mountSection(90)
    await flushPromises()
    await nextTick()

    expect(wrapper.find('[data-testid="daily-report-lifeline"]').exists()).toBe(false)
    expect(wrapper.find('.drm-lifeline').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(false)
    wrapper.unmount()
  })
})
