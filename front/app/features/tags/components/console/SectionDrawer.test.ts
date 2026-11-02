/**
 * SectionDrawer 测试（rework-workshop-into-radar-and-desk task 2.3）。
 *
 * 覆盖 test-cases.md D-1..D-7：
 *  - 命中格：分区标题 + 全部事件 + 后端截断提示
 *  - 当日日报线索：按 section_id 精确联查；无日报/无线索给提示
 *  - 关闭：Esc / 遮罩 / 关闭按钮，选中泳道状态不受影响（由宿主保持）
 *  - 展示锚：Teleport 到 body + overlay 样式规则（layout.md 浮层机械锚）
 *  - 只读：无生命周期写操作；未接入动作置灰 + reason
 */
import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import SectionDrawer from './SectionDrawer.vue'
import type { LaneTimelineSection } from '~/api/laneDynamics'

const getBoardDailyReports = vi.fn()
const getDailyReportDetail = vi.fn()

vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ getBoardDailyReports, getDailyReportDetail }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))

const DATE = '2026-10-22'
const TODAY = '2026-10-23'

const SECTIONS: LaneTimelineSection[] = [
  { section_id: 501, label: '霍尔木兹海峡封锁', events: ['伊朗宣布演习', '油价上行'], folded_count: 2 },
  { section_id: 502, label: '美债收益率', events: ['十年期突破 4.5%'] },
]

function mountDrawer(overrides: Record<string, unknown> = {}) {
  return mount(SectionDrawer, {
    attachTo: document.body,
    props: {
      open: true,
      boardId: 1974,
      laneLabel: '中东地缘政治',
      date: DATE,
      sections: SECTIONS,
      windowDays: 14,
      today: TODAY,
      ...overrides,
    },
  })
}

function mockReportFound() {
  getBoardDailyReports.mockResolvedValue({
    success: true,
    data: {
      reports: [
        { id: 1211, period_date: DATE, title: '日报', article_count: 8, cluster_count: 5 },
        { id: 1212, period_date: TODAY, title: '今日', article_count: 5, cluster_count: 4 },
      ],
    },
  })
  getDailyReportDetail.mockResolvedValue({
    success: true,
    data: {
      report: {
        id: 1211,
        period_date: DATE,
        sections: [
          {
            id: 501,
            cluster_label: '霍尔木兹海峡封锁',
            threads: [
              { id: 1, title: '海峡演习升级', related_article_ids: [11, 12, 13], confidence: 0.82 },
            ],
          },
        ],
      },
    },
  })
}

describe('SectionDrawer — 内容与线索', () => {
  beforeEach(() => {
    getBoardDailyReports.mockReset()
    getDailyReportDetail.mockReset()
    document.body.innerHTML = ''
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('close 时不渲染抽屉', () => {
    mountDrawer({ open: false })
    expect(document.body.querySelector('[data-testid="section-drawer"]')).toBeNull()
  })

  it('命中格：分区标题、全部事件、截断提示（D-2/D-3）', async () => {
    mockReportFound()
    mountDrawer()
    await flushPromises()
    const drawer = document.body.querySelector('[data-testid="section-drawer"]')!
    expect(drawer.textContent).toContain('霍尔木兹海峡封锁')
    expect(drawer.textContent).toContain('伊朗宣布演习')
    expect(drawer.textContent).toContain('美债收益率')
    expect(drawer.textContent).toContain('另有 2 条未载入')
    expect(drawer.textContent).toContain('中东地缘政治 · 2 个分区 / 3 条事件')
  })

  it('当日日报线索：按 section_id 精确联查 thread 与文章数（D-4）', async () => {
    mockReportFound()
    mountDrawer()
    await flushPromises()
    const clues = document.body.querySelector('[data-testid="section-drawer-clues"]')!
    expect(clues.textContent).toContain('海峡演习升级')
    expect(clues.textContent).toContain('3 篇')
    expect(clues.textContent).toContain('0.82')
  })

  it('当天没有日报：明确提示且「打开日报」置灰（D-4）', async () => {
    getBoardDailyReports.mockResolvedValue({ success: true, data: { reports: [] } })
    mountDrawer()
    await flushPromises()
    const drawer = document.body.querySelector('[data-testid="section-drawer"]')!
    expect(drawer.textContent).toContain('这一天没有日报')
    const openReport = document.body.querySelector('[data-testid="section-drawer-open-report"]') as HTMLButtonElement
    expect(openReport.disabled).toBe(true)
  })

  it('线索加载失败：局部错误 + 重试（状态矩阵 error）', async () => {
    getBoardDailyReports.mockResolvedValue({ success: false, error: '日报接口挂了' })
    mountDrawer()
    await flushPromises()
    expect(document.body.textContent).toContain('日报接口挂了')
    expect(document.body.textContent).toContain('重试')
  })

  it('两个动作：打开日报 / 看归档都 emit（D-7；已归属 section 不进编排，spec MUST NOT 移出）', async () => {
    mockReportFound()
    const wrapper = mountDrawer()
    await flushPromises()
    const openReport = document.body.querySelector('[data-testid="section-drawer-open-report"]') as HTMLButtonElement
    expect(openReport.disabled).toBe(false)
    openReport.click()
    expect(wrapper.emitted('openReport')![0]).toEqual([{ reportId: 1211, date: DATE }])

    const archive = document.body.querySelector('[data-testid="section-drawer-archive"]') as HTMLButtonElement
    expect(archive.disabled).toBe(false)
    archive.click()
    expect(wrapper.emitted('viewArchive')).toHaveLength(1)
    expect(document.body.querySelector('[data-testid="section-drawer-compose"]')).toBeNull()
    // 只读：抽屉内没有生命周期写操作
    expect(document.body.textContent).not.toContain('归档话题')
    expect(document.body.textContent).not.toContain('合并泳道')
  })
})

describe('SectionDrawer — 关闭与展示锚', () => {
  beforeEach(() => {
    getBoardDailyReports.mockResolvedValue({ success: true, data: { reports: [] } })
    document.body.innerHTML = ''
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('Esc / 遮罩 / 关闭按钮都 emit close（D-5）', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)

    document.body.querySelector('.ad-scrim')!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(wrapper.emitted('close')).toHaveLength(2)

    document.body.querySelector('[data-testid="section-drawer-close"]')!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(wrapper.emitted('close')).toHaveLength(3)
  })

  it('浮层展示锚：Teleport 到 body + overlay 样式规则存在（layout.md）', () => {
    mountDrawer()
    expect(document.body.querySelector(':scope > [data-testid="section-drawer"]')).not.toBeNull()

    // overlay 规则由共享 AppDrawer 提供（SectionDrawer 不再自建浮层容器）
    const source = readFileSync(`${import.meta.dirname}/../../../../components/ui/AppDrawer.vue`, 'utf8')
    const style = source.slice(source.indexOf('<style'))
    expect(style).toMatch(/\.ad\s*\{[^}]*position:\s*fixed/s)
    expect(style).toMatch(/\.ad\s*\{[^}]*inset:\s*0/s)
    expect(style).toMatch(/\.ad\s*\{[^}]*z-index:/s)
  })
})
