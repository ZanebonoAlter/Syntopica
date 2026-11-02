/**
 * 版块体检问题处理工作面 · 恢复与边界故事（board-health-triage-advice seam C2 / task 3.3）。
 *
 * 同样挂载**真实工作面 + 真实 C1 composable**，只 mock API 层：
 * 409 冲突 / 提交结果未知 / 查询暂停恢复 / 部分失败重试同一 job / 重进恢复 /
 * 停用标签 / 切标签迟到不覆盖 / stale 快照 / 摘除行内确认（真实 DELETE 由宿主注入）。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import BoardHealthWorkspace from './BoardHealthWorkspace.vue'

const apiMocks = vi.hoisted(() => ({
  getDescriptionRepairContext: vi.fn(),
  createDescriptionRepair: vi.fn(),
  getDescriptionRepairJob: vi.fn(),
  retryDescriptionRepair: vi.fn(),
  getBoardHealthCheckup: vi.fn(),
  addComposition: vi.fn(),
}))

vi.mock('~/api/auxiliaryLabels', () => ({
  useAuxiliaryLabelsApi: () => ({
    getDescriptionRepairContext: apiMocks.getDescriptionRepairContext,
    createDescriptionRepair: apiMocks.createDescriptionRepair,
    getDescriptionRepairJob: apiMocks.getDescriptionRepairJob,
    retryDescriptionRepair: apiMocks.retryDescriptionRepair,
  }),
}))

vi.mock('~/api/semanticBoards', async (importOriginal) => {
  const original = await importOriginal<typeof import('~/api/semanticBoards')>()
  return {
    ...original,
    useSemanticBoardsApi: () => ({
      getBoardHealthCheckup: apiMocks.getBoardHealthCheckup,
      addComposition: apiMocks.addComposition,
    }),
  }
})

vi.mock('~/composables/useNotify', () => ({
  useNotify: () => ({ success: vi.fn(), error: vi.fn(), warn: vi.fn() }),
}))

vi.mock('@iconify/vue', () => ({
  Icon: { name: 'Icon', template: '<span />' },
}))

function labelMetric(auxId: number, label: string, extra: Record<string, unknown> = {}) {
  return {
    aux_id: auxId,
    label,
    label_type: 'auxiliary',
    samples: 18,
    cosine_mean: 0.42,
    cosine_median: 0.44,
    cosine_p25: 0.3,
    cosine_p75: 0.55,
    low_sim_ratio: 0.4,
    insufficient: false,
    ...extra,
  }
}

function snapshotFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    semantic_board_id: 5,
    checked_at: '2026-10-29T06:30:00Z',
    metrics: {
      board: { matched_tags: 428, reason_dist: {}, ghost_matches: 40, vector_missing: 0 },
      labels: [labelMetric(11, '上海', { description: '上海知名商业综合体' }), labelMetric(14, '中国媒体')],
      triage: [
        { aux_id: 11, category: 'rewrite', evidence: '最近邻「举办地点」0.796，描述把标签带偏', suggestion: '中国的超大城市' },
        { aux_id: 14, category: 'remove', evidence: '磁铁信号成立：失败子集 0.52 vs 全体 0.68' },
      ],
    },
    uncaptured_candidates: [],
    llm_score: 74,
    llm_comment: '部分标签描述带偏。',
    created_at: '',
    ...overrides,
  }
}

function contextFixture(overrides: Record<string, unknown> = {}) {
  return {
    auxiliary_label: { id: 11, label: '上海', description: '上海知名商业综合体', status: 'active', version: 'v-1' },
    direct_boards: [{ id: 12, label: '中国国内新闻', status: 'active' }],
    active_tag_count: 15374,
    latest_task: null,
    ...overrides,
  }
}

function jobFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: 77,
    auxiliary_label_id: 11,
    idempotency_key: 'key-1',
    expected_version: 'v-1',
    old_description: '上海知名商业综合体',
    new_description: '中国的超大城市',
    stage: 'matching',
    status: 'matching',
    description_committed: true,
    total: 40,
    processed: 12,
    failed: 3,
    not_applicable: 5,
    remaining: 20,
    error: '',
    created_at: '',
    updated_at: '',
    ...overrides,
  }
}

async function flush(ms = 0): Promise<void> {
  await vi.advanceTimersByTimeAsync(ms)
}

/** 可手动兑现的 promise（模拟在途请求）。 */
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  apiMocks.getBoardHealthCheckup.mockResolvedValue({ success: true, data: { latest: snapshotFixture(), trend: [] } })
  apiMocks.getDescriptionRepairContext.mockResolvedValue({ success: true, data: contextFixture() })
  apiMocks.createDescriptionRepair.mockResolvedValue({ success: true, data: jobFixture({ status: 'pending', stage: '' }) })
  apiMocks.getDescriptionRepairJob.mockResolvedValue({ success: true, data: jobFixture() })
  apiMocks.retryDescriptionRepair.mockResolvedValue({ success: true, data: jobFixture({ status: 'pending', stage: 'matching' }) })
})

afterEach(() => {
  vi.useRealTimers()
})

async function mountWorkspace(props: Record<string, unknown> = {}) {
  const wrapper = mount(BoardHealthWorkspace, { props: { boardId: 5, ...props } })
  await flush()
  return wrapper
}

/** 走完"选中 → 确认 → 提交"（供需要任务态的用例复用）。 */
async function submitFor(wrapper: Awaited<ReturnType<typeof mountWorkspace>>): Promise<void> {
  await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
  await flush()
  await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
  await flush()
  await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
  await flush()
}

describe('冲突与未知结果', () => {
  it('409：展示服务端原文（不统一写成版本变化），刷新后草稿仍保留', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({
      success: false,
      error: 'idempotency key was already used with a different request',
      status: 409,
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-draft"]').setValue('用户改过的文案')
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-conflict"]').text()).toContain('idempotency key was already used with a different request')
    expect(wrapper.find('[data-testid="bhw-draft"]').exists()).toBe(true)
    expect((wrapper.find('[data-testid="bhw-draft"]').element as HTMLTextAreaElement).value).toBe('用户改过的文案')

    // 重新加载上下文核对：草稿不被覆盖
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    expect((wrapper.find('[data-testid="bhw-draft"]').element as HTMLTextAreaElement).value).toBe('用户改过的文案')
  })

  it('非 409 提交失败（如 404）：独立错误行透传服务端 message，行内确认不静默关闭', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({ success: false, error: 'auxiliary label not found', status: 404 })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="bhw-confirm"]').exists()).toBe(true)

    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    // 失败有独立错误行（服务端 message 透传，不编文案）
    const failure = wrapper.find('[data-testid="bhw-submit-failure"]')
    expect(failure.exists()).toBe(true)
    expect(failure.text()).toContain('auxiliary label not found')
    // 行内确认没有静默关闭（用户能看到失败并重试/取消）
    expect(wrapper.find('[data-testid="bhw-confirm"]').exists()).toBe(true)
  })

  it('共享范围未知：禁用确认按钮并说明原因（不谎称 0 范围）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: {
        auxiliary_label: { id: 11, label: '上海', description: '上海知名商业综合体', status: 'active', version: 'v-1' },
        // 缺 direct_boards / active_tag_count → 范围未知
      },
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-scope-unknown"]').text()).toContain('共享范围未知')
    expect(wrapper.find('[data-testid="bhw-submit"]').attributes('disabled')).toBeDefined()
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
  })

  it('提交结果未知：保留原确认内容，可同键重发（不新建第二个任务）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-draft"]').setValue('中国的超大城市')
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-unknown-result"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="bhw-unknown-payload"]').text()).toContain('中国的超大城市')
    expect(wrapper.find('[data-testid="bhw-draft"]').exists()).toBe(false)
    // 明确的「重发原确认内容」入口（不是新意图）
    expect(wrapper.find('[data-testid="bhw-resend"]').text()).toContain('重发原确认内容')

    // 同键重发：仍是一次 create（后端按同键返回同一任务），不换新键
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: true, data: jobFixture({ status: 'pending', stage: '' }) })
    await wrapper.find('[data-testid="bhw-resend"]').trigger('click')
    await flush()
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(2)
    // 严格同键 + 同 body
    expect(apiMocks.createDescriptionRepair).toHaveBeenNthCalledWith(1, '11', {
      description: '中国的超大城市',
      expected_version: 'v-1',
      idempotency_key: expect.any(String),
    })
    expect(apiMocks.createDescriptionRepair).toHaveBeenNthCalledWith(2, '11', {
      description: '中国的超大城市',
      expected_version: 'v-1',
      idempotency_key: expect.any(String),
    })
    const firstCall = apiMocks.createDescriptionRepair.mock.calls[0]
    const secondCall = apiMocks.createDescriptionRepair.mock.calls[1]
    expect(firstCall?.[1]).toEqual(secondCall?.[1])
  })

  it('查询失败：保留已知任务并可恢复查询（只查同一 job，不重建）', async () => {
    apiMocks.getDescriptionRepairJob.mockResolvedValue({ success: false, error: '网络错误' })
    const wrapper = await mountWorkspace()
    await submitFor(wrapper)

    await flush(3000)
    expect(wrapper.find('[data-testid="bhw-repair-progress"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="bhw-task-error"]').text()).toContain('网络错误')
    expect(wrapper.find('[data-testid="bhw-resume-poll"]').exists()).toBe(true)

    await wrapper.find('[data-testid="bhw-resume-poll"]').trigger('click')
    await flush(3000)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledTimes(2)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenLastCalledWith('11', '77')
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)
  })
})

describe('失败恢复与重进', () => {
  it('部分失败：只重试同一 job（打 retry 端点，不新建任务）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({
      success: true,
      data: jobFixture({ status: 'matching_failed', stage: 'matching', failed: 3 }),
    })
    const wrapper = await mountWorkspace()
    await submitFor(wrapper)

    expect(wrapper.find('[data-testid="bhw-progress-message"]').text()).toContain('不能标作修复完成')
    expect(wrapper.find('[data-testid="bhw-retry"]').exists()).toBe(true)

    await wrapper.find('[data-testid="bhw-retry"]').trigger('click')
    await flush()
    expect(apiMocks.retryDescriptionRepair).toHaveBeenCalledWith('11', '77')
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)
  })

  it('重试在途：按钮进入 loading/disabled（双击不重复发请求）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({
      success: true,
      data: jobFixture({ status: 'matching_failed', stage: 'matching', failed: 3 }),
    })
    const pending = deferred<unknown>()
    apiMocks.retryDescriptionRepair.mockImplementation(() => pending.promise)
    const wrapper = await mountWorkspace()
    await submitFor(wrapper)

    await wrapper.find('[data-testid="bhw-retry"]').trigger('click')
    await flush()
    const retryButton = wrapper.find('[data-testid="bhw-retry"]')
    expect(retryButton.attributes('disabled')).toBeDefined()
    expect(retryButton.text()).toContain('重试中')

    // 双击：第二次被拒（只发一次 retry）
    await wrapper.find('[data-testid="bhw-retry"]').trigger('click')
    await flush()
    expect(apiMocks.retryDescriptionRepair).toHaveBeenCalledTimes(1)

    pending.resolve({ success: true, data: jobFixture({ status: 'pending', stage: 'matching' }) })
    await flush()
  })

  it('重进：从 context.latest_task 恢复已知任务（服务端持久任务为准）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextFixture({
        latest_task: jobFixture({ status: 'matching_failed', stage: 'matching' }),
      }),
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-repair-progress"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="bhw-draft"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bhw-retry"]').exists()).toBe(true)
  })

  it('停用标签：编辑与提交被禁用（不执行修复）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextFixture({
        auxiliary_label: { id: 11, label: '上海', description: '上海知名商业综合体', status: 'disabled', version: 'v-1' },
      }),
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-editor-disabled"]').text()).toContain('已停用')
    expect(wrapper.find('[data-testid="bhw-submit"]').attributes('disabled')).toBeDefined()
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
  })
})

describe('切标签与快照边界', () => {
  it('切项先清草稿：编辑器不闪现上一项内容', async () => {
    const slow = deferred<unknown>()
    apiMocks.getDescriptionRepairContext.mockImplementation((auxId: string) => {
      if (auxId === '15') return slow.promise
      return Promise.resolve({ success: true, data: contextFixture() })
    })
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: {
            ...snapshotFixture().metrics,
            labels: [...snapshotFixture().metrics.labels, labelMetric(15, '中国媒体')],
            triage: [
              ...snapshotFixture().metrics.triage,
              { aux_id: 15, category: 'rewrite', evidence: '描述窄化', suggestion: '从事新闻采集与传播的媒体机构' },
            ],
          },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-draft"]').setValue('用户写的第一个草稿')
    await flush()
    expect((wrapper.find('[data-testid="bhw-draft"]').element as HTMLTextAreaElement).value).toBe('用户写的第一个草稿')

    // 切到第二项（其上下文在途）→ 草稿立即清空，不闪现上一项内容
    await wrapper.find('[data-testid="bhw-task-15"]').trigger('click')
    await flush()
    const draftEl = wrapper.find('[data-testid="bhw-draft"]')
    expect(draftEl.exists()).toBe(true)
    expect((draftEl.element as HTMLTextAreaElement).value).toBe('')

    slow.resolve({ success: true, data: contextFixture() })
    await flush()
  })

  it('切标签：旧标签的迟到上下文不覆盖当前项', async () => {
    const slow = deferred<unknown>()
    apiMocks.getDescriptionRepairContext.mockImplementation((auxId: string) => {
      if (auxId === '11') return slow.promise
      return Promise.resolve({
        success: true,
        data: contextFixture({
          auxiliary_label: { id: 14, label: '中国媒体', description: '《环球时报》', status: 'active', version: 'v-2' },
        }),
      })
    })

    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-task-14"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-detail"]').text()).toContain('中国媒体')

    slow.resolve({ success: true, data: contextFixture() })
    await flush()
    expect(wrapper.find('[data-testid="bhw-detail"]').text()).toContain('中国媒体')
  })

  it('404（版块可能已删除）：不再说「刷新失败显示上次快照」，旧清单不再可操作', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValueOnce({
      success: true,
      data: { latest: snapshotFixture(), trend: [] },
    })
    const wrapper = await mountWorkspace()
    apiMocks.getBoardHealthCheckup.mockResolvedValue({ success: false, error: 'board not found', status: 404 })
    wrapper.vm.reload()
    await flush()

    // 误导性 stale 文案不再出现
    expect(wrapper.find('[data-testid="bhw-stale-hint"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bhw-state"]').text()).toContain('这块内容可能已经不存在')
    // 旧清单不再可操作（不展示问题索引/处理区）
    expect(wrapper.find('[data-testid="bhw-flow"]').exists()).toBe(false)
  })

  it('刷新失败保留上次快照（stale）：给出提示行且旧快照仍在', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValueOnce({
      success: true,
      data: { latest: snapshotFixture(), trend: [] },
    })
    const wrapper = await mountWorkspace()
    // 同版块刷新失败 → useBoardHealth 保留上次快照并标 stale（宿主可用 reload() 触发）
    apiMocks.getBoardHealthCheckup.mockResolvedValue({ success: false, error: '服务暂时不可用' })
    wrapper.vm.reload()
    await flush()

    const hint = wrapper.find('[data-testid="bhw-stale-hint"]')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toContain('显示上次快照')
    expect(hint.text()).toContain('核对实时版本')
    // 旧快照仍在（未清空成空态）
    expect(wrapper.find('[data-testid="bhw-flow"]').exists()).toBe(true)
  })
})

describe('摘除（3.4 宿主注入契约）', () => {
  it('行内确认后发出摘除事件；未收到宿主真实成功反馈前不标「已摘除」', async () => {
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-14"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-remove-panel"]').exists()).toBe(true)
    await wrapper.find('[data-testid="bhw-remove"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="bhw-remove-confirm"]').exists()).toBe(true)

    await wrapper.find('[data-testid="bhw-remove-confirm-yes"]').trigger('click')
    await flush()
    expect(wrapper.emitted('remove-composition')).toEqual([['14']])
    expect(wrapper.find('[data-testid="bhw-removed-note"]').exists()).toBe(false)
  })

  it('宿主反馈已摘除后：索引与详情显示「已摘除」', async () => {
    const wrapper = await mountWorkspace({ removedAuxIds: ['14'] })
    await wrapper.find('[data-testid="bhw-task-14"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="bhw-removed-note"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="bhw-task-14"]').text()).toContain('已摘除')
  })
})
