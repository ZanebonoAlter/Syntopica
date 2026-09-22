import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import LaneTrendOverview from './LaneTrendOverview.vue'
import type { LaneDynamicsLane, LaneDynamicsResponse } from '~/api/laneDynamics'
import type { ContextRow } from '~/api/boardEnrichment'
import type { RequestCacheEntry } from './dailyReportMagazine'

// Icon stub — keeps the suite offline (no iconify CDN fetch in happy-dom).
vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function makeLane(over: Partial<LaneDynamicsLane> = {}): LaneDynamicsLane {
  return {
    topic_id: 5,
    label: '测试泳道',
    watch_linked: false,
    section_count_14d: 2,
    snapshot: { summary: '短版态势句', detail: '长版全文叙述，完整成段。', as_of: '2026-06-21' },
    timeline: [
      { date: '2026-06-21', sections: [{ section_id: 1, label: 'S1', events: ['事件A1', '事件A2'] }] },
      { date: '2026-06-20', sections: [{ section_id: 2, label: 'S2', events: ['事件B1'] }] },
    ],
    ...over,
  }
}

function contextRow(over: Partial<ContextRow> = {}): ContextRow {
  return {
    id: 1,
    granularity: 'month',
    period: '2026-06',
    content: '六月归档全文。',
    as_of_date: '2026-06-30',
    source: 'llm_assisted',
    ...over,
  }
}

// 事件断言用 onX 监听器 prop（Vue 3 emit 语义），不经 VTU emitted 表：
// 本机环境的 VTU devtools 记录管线失效（多份既有测试的 emitted() 同样收不到，域外预存问题）。
function mountTrend(over: {
  lane?: LaneDynamicsLane | null
  laneEntry?: RequestCacheEntry<LaneDynamicsResponse>
  monthEntry?: RequestCacheEntry<ContextRow[]>
  yearEntry?: RequestCacheEntry<ContextRow[]>
  onEnsureContext?: (granularity: 'month' | 'year', retry?: boolean) => void
  onRetryLane?: () => void
} = {}) {
  return mount(LaneTrendOverview, {
    props: {
      topicId: 5,
      topicColor: '#b44f45',
      lane: over.lane !== undefined ? over.lane : makeLane(),
      laneEntry: over.laneEntry ?? { status: 'success' },
      monthEntry: over.monthEntry ?? { status: 'idle' },
      yearEntry: over.yearEntry ?? { status: 'idle' },
      ...(over.onEnsureContext ? { onEnsureContext: over.onEnsureContext } : {}),
      ...(over.onRetryLane ? { onRetryLane: over.onRetryLane } : {}),
    },
  })
}

describe('LaneTrendOverview — 14 天档内容与降级', () => {
  it('FD-1: 默认 14 天档选中，展示长版全文 + 汇总截止日（全文无截断）', () => {
    const wrapper = mountTrend()
    expect(wrapper.find('[data-testid="trend-tab-14d"]').classes()).toContain('active')
    expect(wrapper.find('[data-testid="trend-panel-14d"]').exists()).toBe(true)

    const detail = wrapper.find('[data-testid="trend-detail"]')
    expect(detail.exists()).toBe(true)
    // 全文渲染：textContent 与源文本完全一致（无省略号截断）
    expect(detail.text()).toBe('长版全文叙述，完整成段。')
    expect(wrapper.find('[data-testid="trend-asof"]').text()).toContain('2026-06-21')
    // 无截断类样式（line-clamp/ellipsis 不存在于该元素类上）
    expect(detail.classes()).not.toContain('lto__status')
  })

  it('FD-2: detail 缺失但 summary 有值 → 短版 + 「长版随下次日报结算生成」提示', () => {
    const wrapper = mountTrend({ lane: makeLane({ snapshot: { summary: '短版态势句', detail: null, as_of: '2026-06-21' } }) })
    expect(wrapper.find('[data-testid="trend-detail"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="trend-summary"]').text()).toBe('短版态势句')
    expect(wrapper.find('[data-testid="trend-detail-hint"]').text()).toContain('长版随下次日报结算生成')
    expect(wrapper.find('[data-testid="trend-asof"]').text()).toContain('2026-06-21')
  })

  it('FD-3: snapshot=null → 「态势待结算」占位，三档切换仍可用', async () => {
    const onEnsureContext = vi.fn()
    const wrapper = mountTrend({ lane: makeLane({ snapshot: null }), onEnsureContext })
    expect(wrapper.find('[data-testid="trend-pending"]').text()).toContain('态势待结算')
    expect(wrapper.find('[data-testid="trend-detail"]').exists()).toBe(false)

    await wrapper.find('[data-testid="trend-tab-year"]').trigger('click')
    expect(onEnsureContext).toHaveBeenCalledTimes(1)
    expect(onEnsureContext).toHaveBeenCalledWith('year')
    expect(wrapper.find('[data-testid="trend-panel-year"]').exists()).toBe(true)
  })

  it('FD-4: lane=null（泳道不在聚合 lanes 中）→ 同 FD-3 降级；逐日事件区显示无事件占位', async () => {
    const wrapper = mountTrend({ lane: null })
    expect(wrapper.find('[data-testid="trend-pending"]').exists()).toBe(true)

    await wrapper.find('[data-testid="trend-events-toggle"]').trigger('click')
    expect(wrapper.find('[data-testid="trend-events-empty"]').text()).toContain('暂无事件脉络')
  })

  it('D6: 14 天档请求失败 → 内联错误条 + 重试（重试信号带 force 语义）', async () => {
    const onRetryLane = vi.fn()
    const wrapper = mountTrend({ laneEntry: { status: 'error', error: '网络错误' }, onRetryLane })
    expect(wrapper.find('[data-testid="trend-error"]').text()).toContain('网络错误')

    await wrapper.find('[data-testid="trend-retry"]').trigger('click')
    expect(onRetryLane).toHaveBeenCalledTimes(1)
  })

  it('D6: 14 天档加载中 → 加载提示，不显示旧档内容', () => {
    const wrapper = mountTrend({ laneEntry: { status: 'loading' } })
    expect(wrapper.find('[data-testid="trend-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="trend-detail"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="trend-events-toggle"]').exists()).toBe(false)
  })
})

describe('LaneTrendOverview — 月/年档内容与降级', () => {
  it('FD-5: 切到月档，contexts 返回多条 period → 取字典序最大一条渲染全文 + 该条 as_of_date', async () => {
    const wrapper = mountTrend({
      monthEntry: {
        status: 'success',
        data: [
          contextRow({ id: 1, period: '2026-04', content: '四月归档。', as_of_date: '2026-04-30' }),
          contextRow({ id: 2, period: '2026-06', content: '六月归档全文。', as_of_date: '2026-06-30' }),
          contextRow({ id: 3, period: '2026-05', content: '五月归档。', as_of_date: '2026-05-31' }),
        ],
      },
    })
    await wrapper.find('[data-testid="trend-tab-month"]').trigger('click')

    expect(wrapper.find('[data-testid="trend-panel-month"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="trend-context-content"]').text()).toBe('六月归档全文。')
    expect(wrapper.find('[data-testid="trend-asof"]').text()).toContain('2026-06-30')
  })

  it('FD-6: 切到月档，contexts 返回空数组 → 「该周期暂无归档摘要」占位，不冒充其它粒度', async () => {
    const wrapper = mountTrend({
      monthEntry: { status: 'success', data: [] },
      yearEntry: { status: 'success', data: [contextRow({ id: 9, granularity: 'year', period: '2026', content: '年度归档不应出现' })] },
    })
    await wrapper.find('[data-testid="trend-tab-month"]').trigger('click')

    expect(wrapper.find('[data-testid="trend-empty"]').text()).toContain('该周期暂无归档摘要')
    expect(wrapper.text()).not.toContain('年度归档不应出现')
  })

  it('FD-7: 切档请求 pending → 该档加载提示，不显示旧档内容', async () => {
    const onEnsureContext = vi.fn()
    const wrapper = mountTrend({ monthEntry: { status: 'loading' }, onEnsureContext })
    await wrapper.find('[data-testid="trend-tab-month"]').trigger('click')

    expect(wrapper.find('[data-testid="trend-loading"]').text()).toContain('正在加载月度归档')
    expect(wrapper.find('[data-testid="trend-detail"]').exists()).toBe(false)
    // loading 态不发取数信号（仅 idle 才发）
    expect(onEnsureContext).not.toHaveBeenCalled()
  })

  it('FD-8: 切档请求失败 → 内联错误条 + 重试按钮；重试成功后正常渲染', async () => {
    const onEnsureContext = vi.fn()
    const wrapper = mountTrend({ monthEntry: { status: 'error', error: '归档服务超时' }, onEnsureContext })
    await wrapper.find('[data-testid="trend-tab-month"]').trigger('click')

    expect(wrapper.find('[data-testid="trend-error"]').text()).toContain('归档服务超时')
    await wrapper.find('[data-testid="trend-retry"]').trigger('click')
    expect(onEnsureContext).toHaveBeenCalledTimes(1)
    expect(onEnsureContext).toHaveBeenCalledWith('month', true)

    // 重试成功（宿主回填 success 缓存条目）→ 正常渲染全文
    await wrapper.setProps({ monthEntry: { status: 'success', data: [contextRow()] } })
    expect(wrapper.find('[data-testid="trend-context-content"]').text()).toBe('六月归档全文。')
  })

  it('FD-9: 已拉取过的档位再次切入 → 命中缓存不再发取数信号', async () => {
    const onEnsureContext = vi.fn()
    const wrapper = mountTrend({ onEnsureContext })
    await wrapper.find('[data-testid="trend-tab-month"]').trigger('click')
    expect(onEnsureContext).toHaveBeenCalledTimes(1)
    expect(onEnsureContext).toHaveBeenCalledWith('month')

    // 模拟宿主回填成功缓存
    await wrapper.setProps({ monthEntry: { status: 'success', data: [contextRow()] } })

    await wrapper.find('[data-testid="trend-tab-14d"]').trigger('click')
    await wrapper.find('[data-testid="trend-tab-month"]').trigger('click')
    expect(onEnsureContext).toHaveBeenCalledTimes(1)
  })

  it('FD-15: 月/年 content 含换行 → pre-line 纯文本渲染保留段落结构，不解析 markdown', async () => {
    const wrapper = mountTrend({
      monthEntry: {
        status: 'success',
        data: [contextRow({ content: '第一段**加粗**陈述。\n第二段带 [链接](x) 与 # 标签。' })],
      },
    })
    await wrapper.find('[data-testid="trend-tab-month"]').trigger('click')

    const content = wrapper.find('[data-testid="trend-context-content"]')
    // pre-line 渲染载体：lto__text 类（white-space: pre-line），markdown 语法保持字面
    expect(content.classes()).toContain('lto__text')
    expect(content.text()).toContain('第一段**加粗**陈述。')
    expect(content.text()).toContain('第二段带 [链接](x) 与 # 标签。')
    expect(content.find('strong').exists()).toBe(false)
    expect(content.find('a').exists()).toBe(false)
  })
})

describe('LaneTrendOverview — 逐日事件就地展开', () => {
  it('FD-10: 逐日事件默认收起，仅展开开关可见', () => {
    const wrapper = mountTrend()
    expect(wrapper.find('[data-testid="trend-events-toggle"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="trend-events-toggle"]').text()).toContain('展开逐日事件')
    expect(wrapper.find('[data-testid="trend-events"]').exists()).toBe(false)
  })

  it('FD-11: 展开后按日期倒序分组；某日 8 条 → 单日显 5 条 + 「还有 3 条」就地展开', async () => {
    const wrapper = mountTrend({
      lane: makeLane({
        timeline: [
          // 故意给升序输入，验证倒序渲染语义
          { date: '2026-06-19', sections: [{ section_id: 3, label: 'S3', events: ['早事件'] }] },
          { date: '2026-06-21', sections: [{ section_id: 1, label: 'S1', events: Array.from({ length: 8 }, (_, i) => `事件${i + 1}`) }] },
        ],
      }),
    })
    await wrapper.find('[data-testid="trend-events-toggle"]').trigger('click')

    const days = wrapper.findAll('.lto__day')
    expect(days).toHaveLength(2)
    expect(days[0]?.attributes('data-testid')).toBe('trend-day-2026-06-21')

    const day = wrapper.find('[data-testid="trend-day-2026-06-21"]')
    expect(day.findAll('.lto__event')).toHaveLength(5)
    const more = day.find('[data-testid="trend-day-more-2026-06-21"]')
    expect(more.text()).toContain('还有 3 条')

    await more.trigger('click')
    expect(wrapper.find('[data-testid="trend-day-2026-06-21"]').findAll('.lto__event')).toHaveLength(8)
  })

  it('FD-12: 后端 folded_count>0 → 「另有 N 条未载入」如实标注（不可展开）', async () => {
    const wrapper = mountTrend({
      lane: makeLane({
        timeline: [
          { date: '2026-06-21', sections: [{ section_id: 1, label: 'S1', events: ['事件一', '事件二'], folded_count: 4 }] },
        ],
      }),
    })
    await wrapper.find('[data-testid="trend-events-toggle"]').trigger('click')

    const day = wrapper.find('[data-testid="trend-day-2026-06-21"]')
    expect(day.findAll('.lto__event')).toHaveLength(2)
    expect(day.find('[data-testid="trend-day-folded-2026-06-21"]').text()).toContain('另有 4 条未载入')
    // 不可展开：没有「还有 N 条」按钮
    expect(day.find('[data-testid="trend-day-more-2026-06-21"]').exists()).toBe(false)
  })

  it('折叠计数 = 前端折叠数 + 后端截断数（8 条事件 + folded_count 2 → 收起时「还有 5 条」，展开后另注 2 条）', async () => {
    const wrapper = mountTrend({
      lane: makeLane({
        timeline: [
          { date: '2026-06-21', sections: [{ section_id: 1, label: 'S1', events: Array.from({ length: 8 }, (_, i) => `事件${i + 1}`), folded_count: 2 }] },
        ],
      }),
    })
    await wrapper.find('[data-testid="trend-events-toggle"]').trigger('click')

    const more = wrapper.find('[data-testid="trend-day-more-2026-06-21"]')
    expect(more.text()).toContain('还有 5 条')

    await more.trigger('click')
    const day = wrapper.find('[data-testid="trend-day-2026-06-21"]')
    expect(day.findAll('.lto__event')).toHaveLength(8)
    expect(day.find('[data-testid="trend-day-folded-2026-06-21"]').text()).toContain('另有 2 条未载入')
    expect(day.find('[data-testid="trend-day-more-2026-06-21"]').exists()).toBe(false)
  })
})

describe('LaneTrendOverview — 泳道展开间状态隔离', () => {
  it('FD-14: 收起泳道再展开（卸载重挂）→ 档位与展开态重置为默认（14d、事件收起）', async () => {
    const first = mountTrend()
    await first.find('[data-testid="trend-events-toggle"]').trigger('click')
    expect(first.find('[data-testid="trend-events"]').exists()).toBe(true)
    await first.find('[data-testid="trend-tab-year"]').trigger('click')
    expect(first.find('[data-testid="trend-panel-year"]').exists()).toBe(true)
    first.unmount()

    const second = mountTrend()
    expect(second.find('[data-testid="trend-tab-14d"]').classes()).toContain('active')
    expect(second.find('[data-testid="trend-panel-14d"]').exists()).toBe(true)
    expect(second.find('[data-testid="trend-events"]').exists()).toBe(false)
    second.unmount()
  })
})
