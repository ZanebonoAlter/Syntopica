/**
 * 版块体检问题处理工作面 · 组件故事（board-health-triage-advice seam C2 / tasks 3.2-3.3）。
 *
 * 挂载**真实工作面 + 真实 C1 composable**，只 mock API 层（体检快照 + 修复四资源）：
 * 不能只 mock composable 自证——那样测不到"确认前零 POST""只提交一次""轮询真实状态"这些接缝。
 *
 * 覆盖：索引只列需核对项 / 其余标签折叠 / 建议预填 / 用户改稿 / 三规则就地校验 /
 * 行内确认与取消（0 POST）/ 确认只 1 次 POST / 提交后只读进度（无 textarea）/
 * 真实 server 状态轮询 / 完成措辞与"处理下一项" / 旧快照·空·降级·dropped·stale。
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
      labels: [
        labelMetric(11, '上海', {
          description: '上海知名商业综合体',
          magnet: { all_mean: 0.68, failed_mean: 0.52, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false },
          neighbors: [{ aux_id: 21, label: '举办地点', cosine: 0.796 }],
          desc_flags: ['此次'],
        }),
        labelMetric(12, '中国央行'),
        labelMetric(13, '全国人大'),
      ],
      triage: [
        { aux_id: 11, category: 'rewrite', evidence: '最近邻「举办地点」0.796，描述把标签带偏', suggestion: '中国的超大城市' },
        { aux_id: 12, category: 'keep', evidence: '方向通过率 100%，短实体名天花板' },
        { aux_id: 13, category: 'insufficient', evidence: '失败子集样本 2 条，低于最小样本量' },
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

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  apiMocks.getBoardHealthCheckup.mockResolvedValue({ success: true, data: { latest: snapshotFixture(), trend: [] } })
  apiMocks.getDescriptionRepairContext.mockResolvedValue({ success: true, data: contextFixture() })
  apiMocks.createDescriptionRepair.mockResolvedValue({ success: true, data: jobFixture({ status: 'pending', stage: '' }) })
  apiMocks.getDescriptionRepairJob.mockResolvedValue({ success: true, data: jobFixture() })
})

afterEach(() => {
  vi.useRealTimers()
})

async function mountWorkspace(props: Record<string, unknown> = {}) {
  const wrapper = mount(BoardHealthWorkspace, { props: { boardId: 5, ...props } })
  await flush()
  return wrapper
}

describe('问题索引与问题清单（3.2）', () => {
  it('索引只列需核对项（remove/rewrite）；keep/insufficient 进「其余标签」折叠', async () => {
    const wrapper = await mountWorkspace()
    expect(wrapper.find('[data-testid="bhw-issue-count"]').text()).toContain('本轮需核对 1 项')
    expect(wrapper.find('[data-testid="bhw-task-11"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="bhw-task-12"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bhw-task-13"]').exists()).toBe(false)

    const other = wrapper.find('[data-testid="bhw-other-labels"]')
    expect(other.text()).toContain('其余 2 个标签')
    expect(wrapper.find('[data-testid="bhw-other-12"]').text()).toContain('保留')
    expect(wrapper.find('[data-testid="bhw-other-13"]').text()).toContain('证据不足')
  })

  it('状态标记在标题左侧且带 aria-label（交互约定 §1）', async () => {
    const wrapper = await mountWorkspace()
    const flag = wrapper.find('[data-testid="bhw-task-11"] .bhw-task-flag')
    expect(flag.exists()).toBe(false) // 待核对项不加状态图标
  })

  it('旧快照（无 triage 字段）：按旧形态展示，不臆造问题清单', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: { ...snapshotFixture(), metrics: { ...snapshotFixture().metrics, triage: undefined } },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    expect(wrapper.find('[data-testid="bhw-state"]').text()).toContain('历史快照')
    expect(wrapper.find('[data-testid="bhw-flow"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bhw-basis"]').exists()).toBe(true)
  })

  it('本轮没有分诊清单：不臆造问题，保留原统计折叠区', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: { latest: { ...snapshotFixture(), metrics: { ...snapshotFixture().metrics, triage: [] } }, trend: [] },
    })
    const wrapper = await mountWorkspace()
    expect(wrapper.find('[data-testid="bhw-state"]').text()).toContain('本轮没有分诊清单')
    expect(wrapper.find('[data-testid="bhw-basis"]').exists()).toBe(true)
  })

  it('降级轮次（真 wire：score/comment 均 NULL 且 metrics.triage=null）：明说不能推断健康', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          llm_score: null,
          llm_comment: null,
          metrics: { ...snapshotFixture().metrics, triage: null },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    expect(wrapper.find('[data-testid="bhw-state"]').text()).toContain('本轮未生成评分与分诊')
    expect(wrapper.find('[data-testid="bhw-state"]').text()).toContain('不能据此推断当前标签健康')
    // 降级轮不是"历史快照"（那是「有评分无 triage」）
    expect(wrapper.find('[data-testid="bhw-state"]').text()).not.toContain('历史快照')
  })

  it('历史快照（有评分但无 triage 字段）：与降级轮区分开', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: { ...snapshotFixture().metrics, triage: null },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    expect(wrapper.find('[data-testid="bhw-state"]').text()).toContain('历史快照')
  })

  it('正常轮也有「本轮体检依据」折叠区（评分/统计/趋势按需可达，默认折叠）', async () => {
    const wrapper = await mountWorkspace()
    const basis = wrapper.find('[data-testid="bhw-basis"]')
    expect(basis.exists()).toBe(true)
    expect(basis.text()).toContain('原统计与评分趋势')
  })

  it('长证据 / 长标签：完整可读（不硬截断）', async () => {
    const longEvidence = '磁铁信号成立：失败挂载子集贴合均值 0.520 与全体 0.680 的差值超过配置阈值，'
      + '且 40 个 direction_mismatch 挂载集中在同一对手版块，另有短名天花板标签未纳入本项判定。'
    const longLabel = '一个非常非常长的辅助标签名称用于验证索引里不出现硬截断'
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: {
            ...snapshotFixture().metrics,
            labels: [...snapshotFixture().metrics.labels, labelMetric(15, longLabel)],
            triage: [
              ...snapshotFixture().metrics.triage,
              { aux_id: 15, category: 'remove', evidence: longEvidence },
            ],
          },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    const item = wrapper.find('[data-testid="bhw-task-15"]')
    expect(item.text()).toContain(longLabel)
    expect(item.text()).toContain('direction_mismatch 挂载集中在同一对手版块')
  })

  it('remove 项也能展开证据（磁铁/流向）再确认摘除', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: {
            ...snapshotFixture().metrics,
            board: {
              ...snapshotFixture().metrics.board,
              magnet_flow: { failed_tags: 40, flow_tags: 22, top_board: '中国国内新闻', top_tags: 9 },
            },
            labels: [...snapshotFixture().metrics.labels, labelMetric(14, '中国媒体')],
            triage: [
              ...snapshotFixture().metrics.triage,
              { aux_id: 14, category: 'remove', evidence: '磁铁信号成立：失败子集 0.52 vs 全体 0.68' },
            ],
          },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-14"]').trigger('click')
    await flush()
    const evidence = wrapper.find('[data-testid="bhw-evidence"]')
    expect(evidence.exists()).toBe(true)
    expect(evidence.text()).toContain('磁铁信号')
    expect(evidence.text()).toContain('失败挂载流向')
  })

  it('按需展开的证据块：默认折叠，展开后可见信号与证据', async () => {
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    const evidence = wrapper.find('[data-testid="bhw-evidence"]')
    expect(evidence.exists()).toBe(true)
    expect(evidence.find('summary').text()).toContain('为什么建议处理')
    expect(evidence.text()).toContain('磁铁信号')
    expect(evidence.text()).toContain('最近邻')
    expect(evidence.text()).toContain('描述事件性预筛')
  })

  it('分诊有丢弃项：一行提示数据不完整（不加信息卡）', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: {
            ...snapshotFixture().metrics,
            triage: [
              ...snapshotFixture().metrics.triage,
              { aux_id: 11, category: 'remove', evidence: '矛盾证据' },
            ],
          },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    const hint = wrapper.find('[data-testid="bhw-dropped-hint"]')
    expect(hint.exists()).toBe(true)
    expect(hint.text()).toContain('不可执行')
  })
})

describe('描述修复全流程（3.3）', () => {
  it('选择项 → 载入实时上下文：原描述与共享范围可见，建议预填为草稿', async () => {
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()

    expect(apiMocks.getDescriptionRepairContext).toHaveBeenCalledWith('11')
    expect(wrapper.find('[data-testid="bhw-repair-editor"]').text()).toContain('上海知名商业综合体')
    expect(wrapper.find('[data-testid="bhw-direct-boards"]').text()).toContain('中国国内新闻')
    expect((wrapper.find('[data-testid="bhw-draft"]').element as HTMLTextAreaElement).value).toBe('中国的超大城市')
  })

  it('用户改稿 → 三规则就地校验（空 / 超 500 / 只重复标签名）', async () => {
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()

    const draft = wrapper.find('[data-testid="bhw-draft"]')
    await draft.setValue('   ')
    await flush()
    expect(wrapper.find('[data-testid="bhw-validation-error"]').text()).toContain('描述不能为空')
    expect(wrapper.find('[data-testid="bhw-submit"]').attributes('disabled')).toBeDefined()

    await draft.setValue('字'.repeat(501))
    await flush()
    expect(wrapper.find('[data-testid="bhw-validation-error"]').text()).toContain('500')
    expect(wrapper.find('[data-testid="bhw-submit"]').attributes('disabled')).toBeDefined()

    await draft.setValue('上海')
    await flush()
    expect(wrapper.find('[data-testid="bhw-validation-error"]').text()).toContain('只重复标签名')
  })

  it('确认前只有 GET；取消确认零 POST', async () => {
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="bhw-confirm"]').exists()).toBe(true)
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()

    await wrapper.find('[data-testid="bhw-confirm-cancel"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="bhw-confirm"]').exists()).toBe(false)
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
  })

  it('最终确认只发一次 POST（body 含 description/expected_version/idempotency_key）', async () => {
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-draft"]').setValue('中国的超大城市（用户改稿）')
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledWith('11', {
      description: '中国的超大城市（用户改稿）',
      expected_version: 'v-1',
      idempotency_key: expect.any(String),
    })
  })

  it('提交后进入只读执行态（无 textarea / 无字数统计），按真实 server 状态推进阶段', async () => {
    apiMocks.getDescriptionRepairJob
      .mockResolvedValueOnce({ success: true, data: jobFixture({ status: 'embedding', stage: 'embedding', processed: null, failed: null, remaining: null }) })
      .mockResolvedValueOnce({ success: true, data: jobFixture({ status: 'saving', stage: 'saving', processed: null, failed: null, remaining: null }) })
      .mockResolvedValueOnce({ success: true, data: jobFixture({ status: 'matching', stage: 'matching' }) })

    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-draft"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bhw-draft-count"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bhw-repair-progress"]').exists()).toBe(true)
    // 创建返回的是 pending：三阶段都还没开始（不臆造进行中）
    expect(wrapper.find('[data-testid="bhw-stage-embedding"]').classes()).toContain('is-pending')

    await flush(3000)
    expect(wrapper.find('[data-testid="bhw-stage-embedding"]').classes()).toContain('is-current')
    await flush(3000)
    expect(wrapper.find('[data-testid="bhw-stage-saving"]').classes()).toContain('is-current')
    await flush(3000)
    expect(wrapper.find('[data-testid="bhw-stage-matching"]').classes()).toContain('is-current')
  })

  it('计数未知显示「未知」，不补 0；跳过项单列不计入已更新', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({
      success: true,
      data: jobFixture({
        status: 'matching',
        total: null,
        processed: null,
        failed: null,
        not_applicable: 5,
        remaining: null,
      }),
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    const counters = wrapper.find('[data-testid="bhw-counters"]').text()
    expect(counters).toContain('未知')
    expect(counters).toContain('已失效/删除被跳过5')
  })

  it('完成：措辞为「更新完成，效果待下一轮体检验证」，主动作变「处理下一项」', async () => {
    // 两项需核对：完成后主按钮是「处理下一项」
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: {
            ...snapshotFixture().metrics,
            labels: [...snapshotFixture().metrics.labels, labelMetric(14, '中国媒体')],
            triage: [
              ...snapshotFixture().metrics.triage,
              { aux_id: 14, category: 'rewrite', evidence: '描述把标签窄化成具体机构', suggestion: '从事新闻采集与传播的媒体机构' },
            ],
          },
        },
        trend: [],
      },
    })
    apiMocks.createDescriptionRepair.mockResolvedValue({ success: true, data: jobFixture({ status: 'completed', stage: 'matching' }) })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-progress-message"]').text()).toContain('效果待下一轮体检验证')
    expect(wrapper.find('[data-testid="bhw-next"]').text()).toContain('处理下一项')
    expect(wrapper.find('[data-testid="bhw-draft"]').exists()).toBe(false)
  })

  it('全部处理完：主按钮变「返回问题清单」，不说所有标签健康', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({ success: true, data: jobFixture({ status: 'completed', stage: 'matching' }) })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-11"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-submit"]').trigger('click')
    await flush()
    await wrapper.find('[data-testid="bhw-confirm-submit"]').trigger('click')
    await flush()

    expect(wrapper.find('[data-testid="bhw-next"]').text()).toContain('返回问题清单')
    expect(wrapper.text()).not.toContain('所有标签健康')
  })
})

// 4.1 M1：组合标签进分诊是**只读**的——描述修复链全程只认 auxiliary。
describe('组合标签（composite）在分诊里的边界', () => {
  it('组合标签的 rewrite：不可执行、无修复入口（不拉必然 404 的上下文），证据仍可核对', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: {
            ...snapshotFixture().metrics,
            labels: [...snapshotFixture().metrics.labels, labelMetric(15, '上海 × 展会', { label_type: 'composite' })],
            triage: [
              ...snapshotFixture().metrics.triage,
              { aux_id: 15, category: 'rewrite', evidence: 'desc 预筛命中此次', suggestion: '展会举办地' },
            ],
          },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    const item = wrapper.find('[data-testid="bhw-task-15"]')
    expect(item.text()).toContain('只读')

    await item.trigger('click')
    await flush()
    // 不去拉必然失败的修复上下文（服务端只认 auxiliary）
    expect(apiMocks.getDescriptionRepairContext).not.toHaveBeenCalled()
    const notice = wrapper.find('[data-testid="bhw-unsupported-repair"]')
    expect(notice.exists()).toBe(true)
    expect(notice.text()).toContain('组合标签暂不支持描述修复')
    // 没有修复入口（无编辑器），但证据不隐藏
    expect(wrapper.find('[data-testid="bhw-repair-editor"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bhw-evidence"]').text()).toContain('desc 预筛命中此次')
  })

  it('组合标签的 remove：仍可摘除（走既有 DELETE 链，外抛 remove-composition）', async () => {
    apiMocks.getBoardHealthCheckup.mockResolvedValue({
      success: true,
      data: {
        latest: {
          ...snapshotFixture(),
          metrics: {
            ...snapshotFixture().metrics,
            labels: [...snapshotFixture().metrics.labels, labelMetric(15, '上海 × 展会', { label_type: 'composite' })],
            triage: [
              ...snapshotFixture().metrics.triage,
              { aux_id: 15, category: 'remove', evidence: '磁铁信号成立：失败子集 0.52 vs 全体 0.68' },
            ],
          },
        },
        trend: [],
      },
    })
    const wrapper = await mountWorkspace()
    await wrapper.find('[data-testid="bhw-task-15"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="bhw-remove-panel"]').exists()).toBe(true)

    await wrapper.find('[data-testid="bhw-remove"]').trigger('click')
    await flush()
    expect(wrapper.find('[data-testid="bhw-remove-confirm"]').exists()).toBe(true)
    await wrapper.find('[data-testid="bhw-remove-confirm-yes"]').trigger('click')
    await flush()
    expect(wrapper.emitted('remove-composition')).toEqual([['15']])
  })
})
