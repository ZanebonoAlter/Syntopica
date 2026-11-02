import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const route = vi.hoisted(() => ({ params: { boardId: '1974', date: 'not-a-date' }, query: {} as Record<string, string> }))
const replace = vi.hoisted(() => vi.fn())
const getBoardDailyReports = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace }),
}))
vi.mock('~/api/dailyReports', () => ({
  useDailyReportsApi: () => ({ getBoardDailyReports }),
}))
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span class="icon-stub" />' },
}))
// 阅读台壳依赖 ArticlePreviewModal → ArticleContentView（Nuxt `#app` 自动导入）。
// 本用例只验日期反查分支，整体替换避免解析不存在的 `#app` 别名。
vi.mock('~/features/tags/components/daily-report/DailyReportDeskShell.vue', () => ({
  default: { name: 'DailyReportDeskShell', template: '<div data-testid="desk-shell-stub" />' },
}))

import DatePage from './[date].vue'

describe('日报日期路由', () => {
  beforeEach(() => {
    route.params = { boardId: '1974', date: 'not-a-date' }
    route.query = {}
    replace.mockReset()
    getBoardDailyReports.mockReset()
  })

  it('非法日期反查最近一期并 replace 到阅读台', async () => {
    getBoardDailyReports.mockResolvedValue({
      success: true,
      data: { reports: [{ id: 1229, period_date: '2026-10-24T00:00:00Z' }] },
    })
    mount(DatePage, { global: { stubs: { DailyReportDeskShell: true, NuxtLink: true } } })
    await flushPromises()
    expect(getBoardDailyReports).toHaveBeenCalledWith(1974, { days: 90 })
    expect(replace).toHaveBeenCalledWith({
      path: '/tags/1974/2026-10-24',
      query: { fallback: 'invalid-date' },
    })
  })

  it('版块无日报时保留错误页并可返回矩阵', async () => {
    getBoardDailyReports.mockResolvedValue({ success: true, data: { reports: [] } })
    const wrapper = mount(DatePage, { global: { stubs: { DailyReportDeskShell: true, NuxtLink: true } } })
    await flushPromises()
    expect(replace).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="desk-invalid-date"]').text()).toContain('还没有日报')
  })
})
