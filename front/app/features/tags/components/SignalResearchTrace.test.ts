/**
 * SignalResearchTrace — 研究进展回看（board-signal-reports tasks 4.8）。
 *
 * 数据契约：progress 来自服务端 board_signal_research_progress（每轮滚动
 * upsert 的真实计数 + 全量账本 ledger=calls/calculations/gaps，与报告
 * appendix 同源结构）。组件只做如实呈现：不美化失败状态、不编造计数、
 * 空账本如实说明落库节奏。
 *
 * 断言方式：@vue/test-utils mount + iconify stub（同 SignalCandidateList.test）。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SignalResearchTrace from './SignalResearchTrace.vue'
import type { SignalResearchProgress } from '~/api/boardSignals'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function makeProgress(overrides: Partial<SignalResearchProgress> = {}): SignalResearchProgress {
  return {
    rounds_done: 5,
    source_calls: 6,
    calculation_calls: 1,
    status: 'running',
    stop_reason: null,
    updated_at: '2026-09-23T12:00:00Z',
    job_id: 'rj1',
    candidate_id: 31,
    semantic_board_id: 9,
    granularity: 'month',
    period: '2026-09',
    ledger: {
      calls: [
        {
          call_id: 'c1',
          question: '当前原油库存多少？',
          tool: 'eia_weekly',
          status: 'ok',
          observations: [{ observation_id: 'c1:o1', period: '2026-09', value: 420.1, unit: 'million barrels' }],
        },
        {
          call_id: 'c2',
          question: '上周产量数据',
          tool: 'jodi_month',
          status: 'error',
          error: 'upstream timeout',
          observations: [],
        },
      ],
      calculations: [
        {
          calc_id: 'k1',
          op: 'difference',
          inputs: ['c1:o1'],
          expression: 'c1:o1-c1:o2',
          value: '1.5',
          unit: 'million barrels',
          precision: 4,
          status: 'ok',
          computed_at: '2026-09-23T12:00:00Z',
        },
      ],
      gaps: [{ call_id: 'c2', tool: 'jodi_month', reason: '目标月份数据尚未发布' }],
    },
    error: null,
    created_at: '2026-09-23T11:00:00Z',
    ...overrides,
  }
}

describe('SignalResearchTrace 研究进展回看（4.8）', () => {
  it('progress=null：不渲染（无进展可回看时不出现空壳）', () => {
    const wrapper = mount(SignalResearchTrace, { props: { progress: null } })
    expect(wrapper.find('[data-testid="signal-research-trace"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('计数行：真实轮次/取数/计算 + 更新时间；caption 注记可见', () => {
    const wrapper = mount(SignalResearchTrace, {
      props: { progress: makeProgress(), caption: '任务已结束，以下为最后保留的进展' },
    })
    const trace = wrapper.find('[data-testid="signal-research-trace"]')
    expect(trace.text()).toContain('已进行 5/40 轮')
    expect(trace.text()).toContain('取数 6')
    expect(trace.text()).toContain('计算 1')
    // 更新时间用时区无关格式断言（formatTime 按本地时区渲染 MM-DD HH:mm:ss）
    expect(trace.text()).toMatch(/更新于 \d{2}-\d{2} \d{2}:\d{2}:\d{2}/)
    expect(trace.text()).toContain('任务已结束，以下为最后保留的进展')
    wrapper.unmount()
  })

  it('明细：每步调用显示 id/工具/问题/观测数，错误调用附原因；计算与缺口如实列出', async () => {
    const wrapper = mount(SignalResearchTrace, { props: { progress: makeProgress() } })
    await wrapper.find('summary').trigger('click')
    const items = wrapper.findAll('.signal-trace-item')
    // 2 calls + 1 calc + 1 gap
    expect(items.length).toBe(4)
    const text = wrapper.text()
    expect(text).toContain('当前原油库存多少？')
    expect(text).toContain('eia_weekly')
    expect(text).toContain('1 条观测')
    expect(text).toContain('upstream timeout') // 错误原因不吞
    expect(text).toContain('difference')
    expect(text).toContain('c1:o1-c1:o2')
    expect(text).toContain('1.5 million barrels')
    expect(text).toContain('目标月份数据尚未发布')
    wrapper.unmount()
  })

  it('空账本（进展已落库但本轮尚无收账调用）：如实提示落库节奏，不伪装调用记录', async () => {
    const p = makeProgress({
      rounds_done: 1, source_calls: 0, calculation_calls: 0,
      ledger: { calls: [], calculations: [], gaps: [] },
    })
    const wrapper = mount(SignalResearchTrace, { props: { progress: p } })
    await wrapper.find('summary').trigger('click')
    expect(wrapper.text()).toContain('还没有调用记录')
    expect(wrapper.text()).toContain('每轮结束后落库一次')
    wrapper.unmount()
  })

  it('失败状态不美化：abandoned + timeout 语义由外层（计数/错误区）表达，组件不渲染成功样式误导', () => {
    const wrapper = mount(SignalResearchTrace, {
      props: { progress: makeProgress({ status: 'abandoned', stop_reason: 'timeout' }) },
    })
    // 组件不渲染任何「完成/成功」字样——如实呈现数据，状态语义归外层
    expect(wrapper.text()).not.toContain('研究完成')
    expect(wrapper.text()).not.toContain('已成功')
    wrapper.unmount()
  })
})
