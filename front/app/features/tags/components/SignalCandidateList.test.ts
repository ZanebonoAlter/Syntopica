/**
 * SignalCandidateList — 候选信号列表（board-signal-reports 5.2，FE-1~5）。
 *
 * FE-1 点击发现只发 discovery：组件 emit discover，绝不 emit/触发 research
 * FE-2 多条候选只研究所点一条：research 事件只携被点候选 id
 * FE-3 字段可读可追溯：signal/why/question/依据/发现时间/派生状态徽标；
 *      score 是内部阈值分，不渲染为可信度
 * FE-4 无信号/发现错误：安静空态 ≠ 错误态；空数组不清旧列表（列表由父级传入）
 * FE-5 刷新候选：pending 状态由服务端派生只读渲染（重启不卡 researching）
 *
 * 断言方式：事件经 attrs onXxx spy 捕获（本仓库 vitest 环境下
 * wrapper.emitted() 不记录自定义事件——Vue 以非 dev 模式解析，既有
 * FeedDetailEditor.test.ts 同源红为证；handler 直调路径不受影响）。
 * 组件为纯展示（props in / events out），API 边界在 useSignalWorkbench。
 */
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SignalCandidateList from './SignalCandidateList.vue'
import type { SignalCandidateRow, SignalResearchProgress } from '~/api/boardSignals'

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', inheritAttrs: true, template: '<span class="icon-stub" aria-hidden="true" />' },
}))

function makeCandidate(overrides: Partial<SignalCandidateRow> = {}): SignalCandidateRow {
  return {
    id: 31,
    discovery_id: 8,
    granularity: 'month',
    period: '2026-09',
    signal: '仓库里的原油少了，真的说明大家用油更多了吗？',
    why_it_matters: '库存少了可能是用得更多，也可能只是进货或调货的时间变了，不能直接当成涨价证据。',
    research_question: '原油为什么少了？这是不是一个值得关注的涨价信号？',
    evidence_refs: ['31', '32'],
    score: 8,
    rationale: '报道密度与数据异动同向',
    discovery_created_at: '2026-09-22T08:30:00Z',
    status: 'pending',
    latest_result_id: null,
    last_research_progress: null,
    ...overrides,
  }
}

interface MountOptions {
  candidates?: SignalCandidateRow[]
  error?: string | null
  discoveryError?: string | null
  discoveryRunning?: boolean
  discoveryPhase?: string | null
  researchCandidateId?: number | null
  researchError?: string | null
  researchProgressLabel?: string
  researchProgress?: SignalResearchProgress | null
}

function makeProgress(overrides: Partial<SignalResearchProgress> = {}): SignalResearchProgress {
  return {
    rounds_done: 5, source_calls: 6, calculation_calls: 1,
    status: 'running', stop_reason: null, updated_at: '2026-09-23T12:00:00Z',
    job_id: 'rj1', candidate_id: 31, semantic_board_id: 9,
    granularity: 'month', period: '2026-09',
    ledger: {
      calls: [
        { call_id: 'c1', question: '当前库存多少？', tool: 'eia_weekly', status: 'ok', observations: [{ observation_id: 'c1:o1' }] },
      ],
      calculations: [],
      gaps: [],
    },
    error: null, created_at: '2026-09-23T11:00:00Z',
    ...overrides,
  }
}

function mountList(opts: MountOptions = {}) {
  const handlers = {
    onCommit: vi.fn(),
    onDiscover: vi.fn(),
    onResearch: vi.fn(),
    onOpenReport: vi.fn(),
    onOpenProgress: vi.fn(),
  }
  const wrapper = mount(SignalCandidateList, {
    props: {
      candidates: opts.candidates ?? [],
      loading: false,
      error: opts.error ?? null,
      granularity: 'month',
      period: '2026-09',
      discoveryRunning: opts.discoveryRunning ?? false,
      discoveryPhase: opts.discoveryPhase ?? null,
      discoveryError: opts.discoveryError ?? null,
      researchCandidateId: opts.researchCandidateId ?? null,
      researchPhase: opts.researchCandidateId != null ? 'research' : null,
      researchError: opts.researchError ?? null,
      researchProgressLabel: opts.researchProgressLabel ?? '正在研究 · 决策上限 40 轮 · 可离开页面',
      researchProgress: opts.researchProgress ?? null,
    },
    attrs: handlers,
  })
  return { wrapper, handlers }
}

describe('SignalCandidateList 工具栏与发现（FE-1）', () => {
  it('点击「发现信号」只发 discover（携当前粒度/周期），绝不发 research——发现结束即停', async () => {
    const { wrapper, handlers } = mountList()
    await wrapper.find('[data-testid="signal-discover"]').trigger('click')

    expect(handlers.onDiscover).toHaveBeenCalledTimes(1)
    expect(handlers.onDiscover).toHaveBeenCalledWith({ granularity: 'month', period: '2026-09' })
    // FE-1 核心：发现动作不存在 research / open-report 出口
    expect(handlers.onResearch).not.toHaveBeenCalled()
    expect(handlers.onOpenReport).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('发现进行中：按钮禁用 + 进度文案显示（不显示「报告生成中」类 research 文案）', () => {
    const { wrapper } = mountList({ discoveryRunning: true, discoveryPhase: 'detect' })
    const btn = wrapper.find('[data-testid="signal-discover"]')
    expect((btn.element as HTMLButtonElement).disabled).toBe(true)
    const progress = wrapper.find('[data-testid="signal-discovery-progress"]')
    expect(progress.exists()).toBe(true)
    expect(progress.text()).toContain('识别信号中')
    expect(progress.text()).toContain('只保存候选，不生成报告')
    expect(wrapper.text()).not.toContain('报告生成中')
    wrapper.unmount()
  })

  it('周期输入 change 提交 commit（只重拉列表不发现）；非法形状不提交', async () => {
    const { wrapper, handlers } = mountList()
    const input = wrapper.find('input[aria-label="月度周期"]')
    // 注：happy-dom+VTU 组合下 setValue/trigger('change') 可能触发多次 change
    // 事件（真实浏览器每次提交一次）；断言只看是否提交与最后一次载荷。
    await input.setValue('2026-08')
    await input.trigger('change')
    expect(handlers.onCommit).toHaveBeenCalled()
    expect(handlers.onCommit).toHaveBeenLastCalledWith({ granularity: 'month', period: '2026-08' })
    expect(handlers.onDiscover).not.toHaveBeenCalled()

    // 非法形状（月13）：不提交、不误触
    const callsBefore = handlers.onCommit.mock.calls.length
    const input2 = wrapper.find('input[aria-label="月度周期"]')
    await input2.setValue('2026-13')
    await input2.trigger('change')
    expect(handlers.onCommit.mock.calls.length).toBe(callsBefore)
    expect(handlers.onDiscover).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})

describe('SignalCandidateList 候选行（FE-2/FE-3）', () => {
  const rows = [
    makeCandidate({ id: 31, status: 'pending' }),
    makeCandidate({ id: 32, status: 'researching', signal: '产量两周没增加，是不想多生产，还是做不到？' }),
    makeCandidate({ id: 33, status: 'reported', latest_result_id: 501 }),
  ]

  it('每行展示 signal/why_it_matters/research_question/发现时间/依据', () => {
    const { wrapper } = mountList({ candidates: rows })
    const first = wrapper.find('[data-candidate-id="31"]')
    expect(first.find('.signal-candidate-title').text()).toContain('原油少了')
    expect(first.find('.signal-why').text()).toContain('进货或调货')
    expect(first.find('.signal-question').text()).toContain('要查的问题')
    expect(first.find('.signal-meta').text()).toContain('发现于 2026-09-22')
    const evidence = first.find('.signal-evidence')
    expect(evidence.text()).toContain('2 条切片')
    expect(evidence.text()).toContain('新闻切片 #31')
    wrapper.unmount()
  })

  it('派生状态徽标：待研究/研究中/已有报告（服务端只读）', () => {
    const { wrapper } = mountList({ candidates: rows, researchCandidateId: 32 })
    expect(wrapper.find('[data-candidate-id="31"] .signal-status').text()).toBe('待研究')
    expect(wrapper.find('[data-candidate-id="32"] .signal-status').text()).toBe('研究中')
    expect(wrapper.find('[data-candidate-id="33"] .signal-status').text()).toBe('已有报告')
    wrapper.unmount()
  })

  it('FE-2：只发被点候选的 research（其余行不跟跑）', async () => {
    const { wrapper, handlers } = mountList({ candidates: rows })
    const buttons = wrapper.findAll('[data-testid="signal-research"]')
    expect(buttons).toHaveLength(2) // researching/reported 行不出现「深入分析」
    await buttons[0]!.trigger('click')
    expect(handlers.onResearch).toHaveBeenCalledTimes(1)
    expect(handlers.onResearch).toHaveBeenCalledWith(31, { regenerate: false })
    wrapper.unmount()
  })

  it('reported 行：主按钮「阅读报告」发 open-report(latest_result_id)，次级显式「重新研究」发 regenerate=true', async () => {
    const { wrapper, handlers } = mountList({ candidates: rows })
    const row = wrapper.find('[data-candidate-id="33"]')
    await row.find('[data-testid="signal-open-report"]').trigger('click')
    expect(handlers.onOpenReport).toHaveBeenCalledTimes(1)
    expect(handlers.onOpenReport).toHaveBeenCalledWith(501)
    await row.find('[data-testid="signal-regenerate"]').trigger('click')
    expect(handlers.onResearch).toHaveBeenCalledTimes(1)
    expect(handlers.onResearch).toHaveBeenCalledWith(33, { regenerate: true })
    wrapper.unmount()
  })

  it('发现中/研究中期间：深入分析按钮禁用（服务端 409 互斥的前端表现）', () => {
    const { wrapper } = mountList({ candidates: rows, discoveryRunning: true })
    for (const btn of wrapper.findAll('[data-testid="signal-research"]')) {
      expect((btn.element as HTMLButtonElement).disabled).toBe(true)
    }
    const regen = wrapper.find('[data-testid="signal-regenerate"]')
    expect((regen.element as HTMLButtonElement).disabled).toBe(true)
    wrapper.unmount()
  })

  it('FE-3：score 不渲染为可信度（内部阈值分不出现在任何文案里）', () => {
    const { wrapper } = mountList({ candidates: [makeCandidate({ score: 8, rationale: '内部理由' })] })
    expect(wrapper.text()).not.toContain('8 分')
    expect(wrapper.text()).not.toContain('可信度')
    expect(wrapper.text()).not.toContain('score')
    wrapper.unmount()
  })
})

describe('SignalCandidateList 空态/错误态（FE-4）', () => {
  it('首次无候选：安静空态（引导发现，不弹错误）', () => {
    const { wrapper } = mountList()
    const empty = wrapper.find('[data-testid="signal-candidates-empty"]')
    expect(empty.exists()).toBe(true)
    expect(empty.text()).toContain('还没有候选信号')
    expect(wrapper.find('.signal-error').exists()).toBe(false)
    wrapper.unmount()
  })

  it('发现失败：错误态与空态区分、可重试提示；不伪装成零信号', () => {
    const { wrapper } = mountList({ discoveryError: '信号发现失败：detect 两次尝试仍不可解析' })
    const err = wrapper.find('[data-testid="signal-discovery-error"]')
    expect(err.exists()).toBe(true)
    expect(err.attributes('role')).toBe('alert')
    expect(err.text()).toContain('detect 两次尝试仍不可解析')
    expect(err.text()).toContain('重试')
    // 按钮仍可点（手动重试入口）
    expect((wrapper.find('[data-testid="signal-discover"]').element as HTMLButtonElement).disabled).toBe(false)
    wrapper.unmount()
  })

  it('FE-4：无信号不清旧列表——列表渲染由父级候选数据驱动（空数组才显示空态，旧数据传入即原样展示）', async () => {
    const { wrapper } = mountList({ candidates: [makeCandidate()] })
    expect(wrapper.find('[data-testid="signal-candidates-empty"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-candidate-id]')).toHaveLength(1)
    // 父级重拉后传入空数组（no_signal 重拉后）才出现安静空态
    await wrapper.setProps({ candidates: [] })
    expect(wrapper.find('[data-testid="signal-candidates-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('列表加载失败：错误态不伪装无数据（FE-12 的列表半边）', () => {
    const { wrapper } = mountList({ error: '候选列表加载失败' })
    expect(wrapper.find('.signal-error').text()).toContain('并非无数据')
    expect(wrapper.find('[data-testid="signal-candidates-empty"]').exists()).toBe(false)
    wrapper.unmount()
  })
})

describe('SignalCandidateList 刷新与恢复（FE-5）', () => {
  it('刷新后候选保持待研究（派生状态只读渲染，不本地推断）', async () => {
    const { wrapper } = mountList({ candidates: [makeCandidate({ id: 31, status: 'pending' })] })
    expect(wrapper.find('[data-candidate-id="31"]').attributes('data-candidate-status')).toBe('pending')
    // 父级重拉后传入同一状态（重启后 researching 必然回落 pending——后端 live job 派生）
    await wrapper.setProps({ candidates: [makeCandidate({ id: 31, status: 'pending' })] })
    expect(wrapper.find('[data-candidate-id="31"] .signal-status').text()).toBe('待研究')
    wrapper.unmount()
  })

  it('研究失败（researchError）：错误块显示，候选行保留可重试', () => {
    const { wrapper } = mountList({
      candidates: [makeCandidate({ id: 31, status: 'pending' })],
      researchError: '研究失败（compose 阶段）：三次成文校验未通过',
    })
    const err = wrapper.find('[data-testid="signal-research-error"]')
    expect(err.exists()).toBe(true)
    expect(err.text()).toContain('compose 阶段')
    // 候选仍在且可再次点击（重试仍需用户点击）
    const btn = wrapper.find('[data-testid="signal-research"]')
    expect((btn.element as HTMLButtonElement).disabled).toBe(false)
    wrapper.unmount()
  })

  it('4.7：研究失败错误态完整渲染带进展摘要的文案（断了不能白跑在错误态可见）', () => {
    // 摘要文案由 useSignalWorkbench 从候选行 last_research_progress 拼装；
    // 组件契约：错误块原样展示全文，不截断不吞括号片段。
    const enriched = '研究被取消或超时（已保留 8 轮进展（9 次取数））'
    const { wrapper } = mountList({
      candidates: [
        makeCandidate({
          id: 31,
          status: 'pending',
          last_research_progress: {
            rounds_done: 8,
            source_calls: 9,
            calculation_calls: 2,
            status: 'abandoned',
            stop_reason: 'timeout',
            updated_at: '2026-09-22T10:00:00Z',
          },
        }),
      ],
      researchError: enriched,
    })
    const err = wrapper.find('[data-testid="signal-research-error"]')
    expect(err.exists()).toBe(true)
    expect(err.text()).toContain(enriched)
    // 候选行渲染不受 last_research_progress 字段影响（仍可重试）
    expect(wrapper.find('[data-candidate-id="31"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('研究进度透传：被研究行显示真实阶段进度文案（无 review phase，FE-10 前端半边）', () => {
    const { wrapper } = mountList({
      candidates: [makeCandidate({ id: 31, status: 'researching' })],
      researchCandidateId: 31,
      researchProgressLabel: '正在研究 · 决策上限 40 轮 · 可离开页面',
    })
    expect(wrapper.text()).toContain('正在研究 · 决策上限 40 轮')
    expect(wrapper.text()).not.toContain('评审')
    expect(wrapper.text()).not.toContain('采纳')
    wrapper.unmount()
  })
})

describe('SignalCandidateList 研究进展回看（4.8）', () => {
  it('进行中：行内渲染研究过程明细（计数 + 可展开每步调用）', () => {
    const { wrapper } = mountList({
      researchCandidateId: 31,
      researchProgressLabel: '正在研究 · 第 5/40 轮 · 取数 6 · 计算 1',
      researchProgress: makeProgress(),
      candidates: [makeCandidate({ id: 31 })],
    })
    const trace = wrapper.find('[data-testid="signal-research-trace"]')
    expect(trace.exists()).toBe(true)
    expect(trace.text()).toContain('已进行 5/40 轮')
    expect(trace.text()).toContain('取数 6')
    // 明细默认折叠，summary 存在
    expect(trace.find('summary').text()).toContain('研究过程')
    wrapper.unmount()
  })

  it('进行中但无进展数据（首轮未落库）：不渲染 trace 区块', () => {
    const { wrapper } = mountList({ researchCandidateId: 31, researchProgress: null })
    expect(wrapper.find('[data-testid="signal-research-trace"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('失败态：错误区下方渲染保留的全程进展（caption 注明任务已结束）', () => {
    const { wrapper } = mountList({
      researchError: '研究超时（已保留 8 轮进展（9 次取数））',
      researchProgress: makeProgress({ status: 'abandoned', stop_reason: 'timeout', rounds_done: 8, source_calls: 9 }),
    })
    const err = wrapper.find('[data-testid="signal-research-error"]')
    expect(err.exists()).toBe(true)
    const trace = wrapper.find('[data-testid="signal-research-trace"]')
    expect(trace.exists()).toBe(true)
    expect(trace.text()).toContain('任务已结束')
    expect(trace.text()).toContain('已进行 8/40 轮')
    wrapper.unmount()
  })

  it('无失败无进行中：不渲染 trace（成功后链路由报告 appendix 接管）', () => {
    const { wrapper } = mountList()
    expect(wrapper.find('[data-testid="signal-research-trace"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
describe('SignalCandidateList 历史研究进展入口（4.8）', () => {
  const withHistory = {
    last_research_progress: {
      rounds_done: 34, source_calls: 22, calculation_calls: 3,
      status: 'running' as const, stop_reason: null, updated_at: '2026-09-23T11:51:58Z',
    },
  }

  it('待研究候选有已落库进展：显示「上次研究」入口，点击 emit open-progress', async () => {
    const { wrapper, handlers } = mountList({
      candidates: [makeCandidate({ id: 6, ...withHistory })],
    })
    const btn = wrapper.find('[data-testid="signal-history-progress"]')
    expect(btn.exists()).toBe(true)
    expect(btn.text()).toContain('34 轮')
    expect(btn.text()).toContain('取数 22')
    await btn.trigger('click')
    expect(handlers.onOpenProgress).toHaveBeenCalledWith(6)
    wrapper.unmount()
  })

  it('从未研究的候选（无进展摘要）：不显示历史入口', () => {
    const { wrapper } = mountList({ candidates: [makeCandidate({ id: 6 })] })
    expect(wrapper.find('[data-testid="signal-history-progress"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('研究进行中：不显示历史入口（行内渲染实时 trace），防覆盖实时数据', () => {
    const { wrapper } = mountList({
      candidates: [makeCandidate({ id: 31, ...withHistory })],
      researchCandidateId: 31,
      researchProgress: makeProgress(),
    })
    expect(wrapper.find('[data-testid="signal-history-progress"]').exists()).toBe(false)
    // 实时 trace 在行内
    expect(wrapper.find('[data-testid="signal-research-trace"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('失败态：行内不重复渲染 trace（失败区已渲染），也不显示历史入口', () => {
    const { wrapper } = mountList({
      candidates: [makeCandidate({ id: 31, ...withHistory })],
      researchError: '研究超时',
      researchProgress: makeProgress(),
    })
    expect(wrapper.find('[data-testid="signal-history-progress"]').exists()).toBe(false)
    // 失败区恰一个 trace
    expect(wrapper.findAll('[data-testid="signal-research-trace"]').length).toBe(1)
    wrapper.unmount()
  })
})
