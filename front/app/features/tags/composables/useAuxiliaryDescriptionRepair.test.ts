/**
 * 共享描述修复 composable（board-health-triage-advice seam C1 / task 3.1，
 * C1 独立 review 修复版）。
 *
 * 覆盖用例：
 *  - 重新进入从 context.latest_task 恢复（不凭本地 map 假冒持久进度）；
 *  - 一次确认 = 一个幂等键 + 一份冻结 payload；重复提交只创建一次；
 *  - **未知结果不换键**：网络错/500/408 → 重载上下文（latest 为空或他键任务）
 *    仍保留原键原 body；只有同键同 payload 的任务才证明已受理并解冻；
 *  - 409 冲突保留草稿、供 UI 加载最新版重新核对（不解析错误文案猜状态）；
 *  - 404 / 传输错：已知任务保留、错误暴露给上层一次处理；
 *  - 查询失败保留已知任务并标"可恢复"，不伪造后台失败/完成；
 *  - 轮询按 task id、只 GET、终态/unknown 停、预算耗尽停、手动恢复只查同一 job；
 *  - 切标签的迟到响应不污染新标签；卸载/scope.stop 后不再发请求、不复活 refs；
 *  - reset 不是 dispose（随后仍可加载新标签）；失败态只走 retry 不新建。
 *
 * API 全 mock + fake timers；幂等键用**递增工厂**（key-1、key-2…）并断言调用数，
 * 避免恒定 key 掩盖换键 bug。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { effectScope, h } from 'vue'
import { mount } from '@vue/test-utils'
import { useAuxiliaryDescriptionRepair } from './useAuxiliaryDescriptionRepair'

const apiMocks = vi.hoisted(() => ({
  getDescriptionRepairContext: vi.fn(),
  createDescriptionRepair: vi.fn(),
  getDescriptionRepairJob: vi.fn(),
  retryDescriptionRepair: vi.fn(),
}))

vi.mock('~/api/auxiliaryLabels', () => ({
  useAuxiliaryLabelsApi: () => apiMocks,
}))

function contextRaw(overrides: Record<string, unknown> = {}) {
  return {
    auxiliary_label: { id: 1974, label: '上海', description: '上海知名商业综合体', status: 'active', version: 'v-opaque-1' },
    direct_boards: [{ id: 12, label: '中国国内新闻', status: 'active' }],
    active_tag_count: 15374,
    latest_task: null,
    ...overrides,
  }
}

function jobRaw(overrides: Record<string, unknown> = {}) {
  return {
    id: 77,
    auxiliary_label_id: 1974,
    idempotency_key: 'key-1',
    expected_version: 'v-opaque-1',
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
    created_at: '2026-10-29T10:00:00Z',
    updated_at: '2026-10-29T10:01:00Z',
    ...overrides,
  }
}

/** 递增幂等键工厂（暴露调用数，便于断言“没有换键”）。 */
let keyCounter = 0
const keyFactory = vi.fn(() => `key-${++keyCounter}`)

const scopes: Array<{ stop: () => void }> = []

function makeRepair(overrides: Record<string, unknown> = {}) {
  const scope = effectScope()
  scopes.push(scope)
  const repair = scope.run(() => useAuxiliaryDescriptionRepair({
    pollIntervalMs: 1000,
    maxPollAttempts: 3,
    createIdempotencyKey: keyFactory,
    ...overrides,
  }))!
  return repair
}

/** 可手动兑现的 promise（模拟"请求在途"）。 */
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
  keyCounter = 0
  keyFactory.mockClear()
  apiMocks.getDescriptionRepairContext.mockResolvedValue({ success: true, data: contextRaw() })
  apiMocks.getDescriptionRepairJob.mockResolvedValue({ success: true, data: jobRaw() })
})

afterEach(() => {
  vi.useRealTimers()
  while (scopes.length) scopes.pop()?.stop()
})

describe('装载上下文与重进恢复', () => {
  it('从 context.latest_task 恢复已知任务并进入轮询（服务端持久任务为准）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    const repair = makeRepair()
    await repair.loadContext('1974')

    expect(repair.context.value?.label.version).toBe('v-opaque-1')
    expect(repair.context.value?.directBoards).toEqual([{ id: '12', label: '中国国内新闻', status: 'active' }])
    expect(repair.task.value?.id).toBe('77')
    expect(repair.task.value?.notApplicable).toBe(5)
    expect(repair.task.value?.remaining).toBe(20)
    expect(repair.polling.value).toBe(true)
    repair.stopPolling()
  })

  it('无任务：latestTask 为 null，不造任务状态，也不启动轮询', async () => {
    const repair = makeRepair()
    await repair.loadContext('1974')
    expect(repair.task.value).toBeNull()
    expect(repair.polling.value).toBe(false)
  })

  it('终态任务：恢复后不轮询（终态停止）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'completed' }) }),
    })
    const repair = makeRepair()
    await repair.loadContext('1974')
    expect(repair.task.value?.status).toBe('completed')
    expect(repair.polling.value).toBe(false)
  })

  it('上下文加载失败：暴露错误，不造上下文', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({ success: false, error: 'auxiliary label not found', status: 404 })
    const repair = makeRepair()
    const view = await repair.loadContext('1974')
    expect(view).toBeNull()
    expect(repair.contextError.value).toBe('auxiliary label not found')
    expect(repair.context.value).toBeNull()
  })

  it('归属校验：服务端返回的标签不是请求的标签 → 不采纳任务、不串台', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    const repair = makeRepair()
    const view = await repair.loadContext('2000')
    expect(view).toBeNull()
    expect(repair.contextError.value).toBe('修复上下文与请求标签不一致')
    expect(repair.task.value).toBeNull()
  })
})

describe('提交确认（冻结 payload 与幂等键）', () => {
  it('提交体带 description/expected_version/idempotency_key；version 不透明透传', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({ success: true, data: jobRaw({ status: 'pending' }) })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('  中国的超大城市  ')

    const result = await repair.submitRepair()
    expect(result.ok).toBe(true)
    expect(keyFactory).toHaveBeenCalledTimes(1)
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledWith('1974', {
      description: '中国的超大城市',
      expected_version: 'v-opaque-1',
      idempotency_key: 'key-1',
    })
    expect(repair.task.value?.id).toBe('77')
    repair.stopPolling()
  })

  it('重复提交只创建一次（提交中再次点击被拒）', async () => {
    const pending = deferred<unknown>()
    apiMocks.createDescriptionRepair.mockImplementation(() => pending.promise)
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')

    const first = repair.submitRepair()
    const second = await repair.submitRepair()
    expect(second.ok).toBe(false)
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)

    pending.resolve({ success: true, data: jobRaw({ status: 'pending' }) })
    await first
    repair.stopPolling()
  })

  it('网络错 = 结果未知：保留冻结，重发必须同键同体', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    expect(repair.frozenSubmit.value).toBeNull()

    const first = await repair.submitRepair()
    expect(first.ok).toBe(false)
    expect(first.uncertain).toBe(true)
    expect(repair.submitUncertain.value).toBe(true)
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')

    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: true, data: jobRaw({ status: 'pending' }) })
    await repair.submitRepair()
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(2)
    // 同键同体：第二次请求与第一次逐字段相同，且没有铸新键
    expect(keyFactory).toHaveBeenCalledTimes(1)
    expect(apiMocks.createDescriptionRepair).toHaveBeenNthCalledWith(2, '1974', {
      description: '中国的超大城市',
      expected_version: 'v-opaque-1',
      idempotency_key: 'key-1',
    })
    // 确认落地后冻结解除
    expect(repair.frozenSubmit.value).toBeNull()
    repair.stopPolling()
  })

  it('改了内容不得复用旧幂等键（拒绝且不发新请求）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()

    repair.setDraft('另一个描述')
    const result = await repair.submitRepair()
    expect(result.ok).toBe(false)
    expect(result.uncertain).toBe(true)
    expect(result.error).toContain('提交结果尚未确认')
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)
    expect(keyFactory).toHaveBeenCalledTimes(1)
  })

  it('显式重发原确认内容：草稿已变也只发原 body + 原 key，且草稿不丢', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)

    // 用户草稿被改成别的内容（不应被当作新意图，也不该丢）
    repair.setDraft('用户后来写的另一个文案')
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: true, data: jobRaw({ status: 'pending' }) })
    const result = await repair.resendFrozen()

    expect(result.ok).toBe(true)
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(2)
    // 严格同键 + 同 body（不换成新草稿、不换新键）
    expect(apiMocks.createDescriptionRepair).toHaveBeenNthCalledWith(2, '1974', {
      description: '中国的超大城市',
      expected_version: 'v-opaque-1',
      idempotency_key: 'key-1',
    })
    expect(keyFactory).toHaveBeenCalledTimes(1)
    // 草稿不丢
    expect(repair.draft.value).toBe('用户后来写的另一个文案')
    repair.stopPolling()
  })

  it('没有冻结时重发：直接拒绝（不发请求）', async () => {
    const repair = makeRepair()
    await repair.loadContext('1974')
    const result = await repair.resendFrozen()
    expect(result.ok).toBe(false)
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
  })

  it('409 冲突：保留草稿、标冲突（不解析错误文案猜状态，透传服务端 message）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({
      success: false,
      error: 'auxiliary label has changed, reload and confirm again',
      status: 409,
    })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')

    const result = await repair.submitRepair()
    expect(result.ok).toBe(false)
    expect(result.conflict).toBe(true)
    expect(result.error).toBe('auxiliary label has changed, reload and confirm again')
    expect(repair.conflict.value).toBe(true)
    expect(repair.draft.value).toBe('中国的超大城市')
  })

  it('409 后重载上下文：草稿仍保留（可重新核对），冲突标记清除', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({
      success: false,
      error: 'idempotency key was already used with a different request',
      status: 409,
    })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()
    expect(repair.conflict.value).toBe(true)

    await repair.loadContext('1974')
    expect(repair.draft.value).toBe('中国的超大城市')
    expect(repair.conflict.value).toBe(false)
  })

  it('404：提交被拒，错误暴露给上层（已知任务保留）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({ success: false, error: 'auxiliary label not found', status: 404 })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')

    const result = await repair.submitRepair()
    expect(result.ok).toBe(false)
    expect(result.error).toBe('auxiliary label not found')
    expect(repair.submitError.value).toBe('auxiliary label not found')
  })

  it('408（超时类）视结果未知：保留冻结，不解冻、不换键', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({ success: false, error: 'request timeout', status: 408 })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')

    const result = await repair.submitRepair()
    expect(result.ok).toBe(false)
    expect(result.uncertain).toBe(true)
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')
    expect(keyFactory).toHaveBeenCalledTimes(1)
  })

  it('空描述/停用标签/缺版本：就地拒绝，不发请求', async () => {
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('   ')
    expect((await repair.submitRepair()).ok).toBe(false)
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()

    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ auxiliary_label: { id: 1974, label: '上海', description: '', status: 'disabled', version: 'v1' } }),
    })
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    expect((await repair.submitRepair()).ok).toBe(false)
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
  })
})

describe('未知结果不换键（重载上下文不解冻）', () => {
  it('网络错后重载上下文（latest=null）：不换键，重发仍是原键原 body', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()
    expect(keyFactory).toHaveBeenCalledTimes(1)

    // 重载上下文：服务端尚无任务（POST 未落库）→ 查询为空不是撤销授权
    apiMocks.getDescriptionRepairContext.mockResolvedValue({ success: true, data: contextRaw({ latest_task: null }) })
    await repair.loadContext('1974')
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')
    expect(keyFactory).toHaveBeenCalledTimes(1)

    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: true, data: jobRaw({ status: 'pending' }) })
    await repair.submitRepair()
    expect(keyFactory).toHaveBeenCalledTimes(1)
    expect(apiMocks.createDescriptionRepair).toHaveBeenNthCalledWith(2, '1974', {
      description: '中国的超大城市',
      expected_version: 'v-opaque-1',
      idempotency_key: 'key-1',
    })
    repair.stopPolling()
  })

  it('500 后重载上下文（latest=null）：同样不换键', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: 'internal server error', status: 500 })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    const result = await repair.submitRepair()
    expect(result.uncertain).toBe(true)

    apiMocks.getDescriptionRepairContext.mockResolvedValue({ success: true, data: contextRaw({ latest_task: null }) })
    await repair.loadContext('1974')
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')
    expect(keyFactory).toHaveBeenCalledTimes(1)
  })

  it('重载上下文返回他键任务：不采纳为本次结果、不解冻、不换键', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()

    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ id: 88, idempotency_key: 'other-key', status: 'completed' }) }),
    })
    await repair.loadContext('1974')

    expect(repair.task.value).toBeNull()
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')
    expect(keyFactory).toHaveBeenCalledTimes(1)
  })

  it('重载上下文返回同键同 payload 任务：确认已被受理 → 采纳并解冻', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')

    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({
        latest_task: jobRaw({
          idempotency_key: 'key-1',
          new_description: '中国的超大城市',
          expected_version: 'v-opaque-1',
          status: 'pending',
        }),
      }),
    })
    await repair.loadContext('1974')

    expect(repair.task.value?.id).toBe('77')
    expect(repair.frozenSubmit.value).toBeNull()
    repair.stopPolling()
  })
})

describe('明确重试（只打 retry 端点）', () => {
  it('失败态 retry：只调 retry 端点，不新建任务、不改版本', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching_failed' }) }),
    })
    apiMocks.retryDescriptionRepair.mockResolvedValue({ success: true, data: jobRaw({ status: 'pending' }) })
    const repair = makeRepair()
    await repair.loadContext('1974')

    const result = await repair.retryTask()
    expect(result.ok).toBe(true)
    expect(apiMocks.retryDescriptionRepair).toHaveBeenCalledWith('1974', '77')
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
    expect(keyFactory).not.toHaveBeenCalled()
    expect(repair.task.value?.status).toBe('pending')
    repair.stopPolling()
  })

  it('非失败态不提供 retry（不改状态、不发请求）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'completed' }) }),
    })
    const repair = makeRepair()
    await repair.loadContext('1974')
    const result = await repair.retryTask()
    expect(result.ok).toBe(false)
    expect(apiMocks.retryDescriptionRepair).not.toHaveBeenCalled()
  })

  it('retry 双击只发一次请求；retry 在途时 submit 被拒（互斥）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching_failed' }) }),
    })
    const pending = deferred<unknown>()
    apiMocks.retryDescriptionRepair.mockImplementation(() => pending.promise)
    const repair = makeRepair()
    await repair.loadContext('1974')

    const first = repair.retryTask()
    const second = await repair.retryTask()
    expect(second.ok).toBe(false)
    expect(apiMocks.retryDescriptionRepair).toHaveBeenCalledTimes(1)

    repair.setDraft('中国的超大城市')
    const submit = await repair.submitRepair()
    expect(submit.ok).toBe(false)
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()

    pending.resolve({ success: true, data: jobRaw({ status: 'pending' }) })
    await first
    repair.stopPolling()
  })

  it('失败态（后端仍持同标签互斥）：只能重试，不得铸新键新建', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching_failed' }) }),
    })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')

    const result = await repair.submitRepair()
    expect(result.ok).toBe(false)
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
    expect(keyFactory).not.toHaveBeenCalled()
  })
})

describe('有界轮询', () => {
  it('轮询只发 GET（不写任何东西）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValue({ success: true, data: jobRaw({ status: 'pending' }) })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()

    await vi.advanceTimersByTimeAsync(1000)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledWith('1974', '77')
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledTimes(1)
    repair.stopPolling()
  })

  it('终态停止轮询', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    apiMocks.getDescriptionRepairJob.mockResolvedValue({ success: true, data: jobRaw({ status: 'completed' }) })
    const repair = makeRepair()
    await repair.loadContext('1974')

    await vi.advanceTimersByTimeAsync(1000)
    expect(repair.task.value?.status).toBe('completed')
    expect(repair.polling.value).toBe(false)

    await vi.advanceTimersByTimeAsync(5000)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledTimes(1)
  })

  it('未知状态：停轮询并标可恢复（不当后台失败、不当完成）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    apiMocks.getDescriptionRepairJob.mockResolvedValue({ success: true, data: jobRaw({ status: 'brand_new_state' }) })
    const repair = makeRepair()
    await repair.loadContext('1974')

    await vi.advanceTimersByTimeAsync(1000)
    expect(repair.task.value?.status).toBe('unknown')
    expect(repair.polling.value).toBe(false)
    expect(repair.pollPaused.value).toBe(true)

    // 可恢复：只查同一 job
    repair.resumePolling()
    await vi.advanceTimersByTimeAsync(1000)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledTimes(2)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenLastCalledWith('1974', '77')
  })

  it('查询失败：保留已知任务、标查询暂停（不伪造后台失败）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    apiMocks.getDescriptionRepairJob.mockResolvedValue({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')

    await vi.advanceTimersByTimeAsync(1000)
    expect(repair.task.value?.id).toBe('77')
    expect(repair.task.value?.status).toBe('matching')
    expect(repair.pollPaused.value).toBe(true)
    expect(repair.polling.value).toBe(false)
    expect(repair.taskError.value).toBe('网络错误')
  })

  it('预算耗尽：停止轮询、保留任务、可手动恢复（只查同一 job）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    const repair = makeRepair({ maxPollAttempts: 2 })
    await repair.loadContext('1974')

    await vi.advanceTimersByTimeAsync(1000)
    await vi.advanceTimersByTimeAsync(1000)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledTimes(2)
    expect(repair.polling.value).toBe(false)
    expect(repair.pollPaused.value).toBe(true)
    expect(repair.task.value?.id).toBe('77')

    repair.resumePolling()
    expect(repair.polling.value).toBe(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledTimes(3)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenLastCalledWith('1974', '77')
    expect(apiMocks.createDescriptionRepair).not.toHaveBeenCalled()
    repair.stopPolling()
  })

  it('切标签：旧标签的迟到响应不污染新标签', async () => {
    const oldLoad = deferred<unknown>()
    apiMocks.getDescriptionRepairContext.mockImplementation((auxId: string) => {
      if (auxId === '1974') return oldLoad.promise
      return Promise.resolve({
        success: true,
        data: contextRaw({
          auxiliary_label: { id: 2000, label: '北京', description: '旧', status: 'active', version: 'v-2' },
          latest_task: null,
        }),
      })
    })

    const repair = makeRepair()
    const pending = repair.loadContext('1974')
    await repair.loadContext('2000')
    expect(repair.context.value?.label.label).toBe('北京')

    oldLoad.resolve({ success: true, data: contextRaw() })
    await pending
    // 迟到响应不得覆盖新标签的上下文
    expect(repair.context.value?.label.label).toBe('北京')
    expect(repair.context.value?.label.version).toBe('v-2')
  })

  it('切标签后旧标签的 POST 迟到：只维护旧标签冻结，不写当前标签的全局 ref', async () => {
    const pending = deferred<unknown>()
    apiMocks.createDescriptionRepair.mockImplementation(() => pending.promise)
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    const submit = repair.submitRepair()

    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({
        auxiliary_label: { id: 2000, label: '北京', description: '旧', status: 'active', version: 'v-2' },
        latest_task: null,
      }),
    })
    await repair.loadContext('2000')
    expect(repair.context.value?.label.id).toBe('2000')

    pending.resolve({ success: false, error: '网络错误' })
    const result = await submit
    expect(result.uncertain).toBe(true)

    // 当前标签的视图不被污染
    expect(repair.submitUncertain.value).toBe(false)
    expect(repair.submitError.value).toBeNull()
    expect(repair.task.value).toBeNull()

    // 旧标签的冻结记录仍在：切回该标签可恢复未知意图
    apiMocks.getDescriptionRepairContext.mockResolvedValue({ success: true, data: contextRaw({ latest_task: null }) })
    await repair.loadContext('1974')
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')
    expect(repair.submitUncertain.value).toBe(true)
  })
})

describe('生命周期（dispose / reset）', () => {
  it('scope.stop()（卸载）在途 POST 迟到：零后续 GET、当前 refs 不复活', async () => {
    const pending = deferred<unknown>()
    apiMocks.createDescriptionRepair.mockImplementation(() => pending.promise)
    const scope = effectScope()
    const repair = scope.run(() => useAuxiliaryDescriptionRepair({
      pollIntervalMs: 1000,
      maxPollAttempts: 3,
      createIdempotencyKey: keyFactory,
    }))!
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    const submit = repair.submitRepair()

    scope.stop()
    pending.resolve({ success: true, data: jobRaw({ status: 'pending' }) })
    await submit

    expect(apiMocks.getDescriptionRepairJob).not.toHaveBeenCalled()
    expect(repair.task.value).toBeNull()
    expect(repair.polling.value).toBe(false)
  })

  it('scope.stop() 在途 poll 迟到：不写状态、不再排下一发', async () => {
    const pending = deferred<unknown>()
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    apiMocks.getDescriptionRepairJob.mockImplementation(() => pending.promise)
    const scope = effectScope()
    const repair = scope.run(() => useAuxiliaryDescriptionRepair({
      pollIntervalMs: 1000,
      maxPollAttempts: 3,
      createIdempotencyKey: keyFactory,
    }))!
    await repair.loadContext('1974')
    await vi.advanceTimersByTimeAsync(1000)
    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledTimes(1)

    scope.stop()
    pending.resolve({ success: true, data: jobRaw({ status: 'completed' }) })
    await vi.advanceTimersByTimeAsync(5000)

    expect(apiMocks.getDescriptionRepairJob).toHaveBeenCalledTimes(1)
    expect(repair.task.value?.status).toBe('matching')
    expect(repair.polling.value).toBe(false)
  })

  it('scope.stop() 在途 context 迟到：不复活上下文', async () => {
    const pending = deferred<unknown>()
    apiMocks.getDescriptionRepairContext.mockImplementation(() => pending.promise)
    const scope = effectScope()
    const repair = scope.run(() => useAuxiliaryDescriptionRepair({ createIdempotencyKey: keyFactory }))!
    const load = repair.loadContext('1974')

    scope.stop()
    pending.resolve({ success: true, data: contextRaw() })
    await load

    expect(repair.context.value).toBeNull()
  })

  it('scope.stop() 在途 retry 迟到：不写状态、不重启轮询', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching_failed' }) }),
    })
    const pending = deferred<unknown>()
    apiMocks.retryDescriptionRepair.mockImplementation(() => pending.promise)
    const scope = effectScope()
    const repair = scope.run(() => useAuxiliaryDescriptionRepair({
      pollIntervalMs: 1000,
      maxPollAttempts: 3,
      createIdempotencyKey: keyFactory,
    }))!
    await repair.loadContext('1974')

    const retry = repair.retryTask()
    scope.stop()
    pending.resolve({ success: true, data: jobRaw({ status: 'pending' }) })
    await retry

    expect(apiMocks.getDescriptionRepairJob).not.toHaveBeenCalled()
    expect(repair.task.value?.status).toBe('matching_failed')
    expect(repair.polling.value).toBe(false)
  })

  it('组件卸载后不再发请求（组件作用域自动 dispose）', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    let repair: ReturnType<typeof useAuxiliaryDescriptionRepair> | null = null
    const wrapper = mount({
      setup() {
        // 直接在组件 setup 内创建 → 随组件作用域自动释放
        repair = useAuxiliaryDescriptionRepair({
          pollIntervalMs: 1000,
          maxPollAttempts: 3,
          createIdempotencyKey: keyFactory,
        })
        return () => h('div')
      },
    })
    await repair!.loadContext('1974')
    expect(repair!.polling.value).toBe(true)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(5000)
    expect(apiMocks.getDescriptionRepairJob).not.toHaveBeenCalled()
  })

  it('scope 外创建：dev 提示要求显式 dispose；显式 dispose 后不再发请求', async () => {
    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({ latest_task: jobRaw({ status: 'matching' }) }),
    })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const repair = useAuxiliaryDescriptionRepair({
      pollIntervalMs: 1000,
      maxPollAttempts: 3,
      createIdempotencyKey: keyFactory,
    })
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()

    await repair.loadContext('1974')
    expect(repair.polling.value).toBe(true)

    repair.dispose()
    await vi.advanceTimersByTimeAsync(5000)
    expect(apiMocks.getDescriptionRepairJob).not.toHaveBeenCalled()
    expect(repair.polling.value).toBe(false)
  })

  it('reset 不是 dispose：随后仍可加载新标签', async () => {
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.reset()
    expect(repair.context.value).toBeNull()
    expect(repair.task.value).toBeNull()

    apiMocks.getDescriptionRepairContext.mockResolvedValue({
      success: true,
      data: contextRaw({
        auxiliary_label: { id: 2000, label: '北京', description: '旧', status: 'active', version: 'v-2' },
        latest_task: null,
      }),
    })
    const view = await repair.loadContext('2000')
    expect(view?.label.id).toBe('2000')
    expect(repair.context.value?.label.id).toBe('2000')
  })

  it('reset 后未知意图仍在：切回旧标签可恢复该冻结', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('1974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()
    expect(repair.submitUncertain.value).toBe(true)

    repair.reset()
    expect(repair.submitUncertain.value).toBe(false)

    await repair.loadContext('1974')
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')
    expect(repair.submitUncertain.value).toBe(true)
    expect(keyFactory).toHaveBeenCalledTimes(1)
  })
})

describe('canonical ID 一致性', () => {
  it('请求路径、冻结键与任务归属用同一 canonical 形态（前导零不分家）', async () => {
    apiMocks.createDescriptionRepair.mockResolvedValueOnce({ success: false, error: '网络错误' })
    const repair = makeRepair()
    await repair.loadContext('01974')
    repair.setDraft('中国的超大城市')
    await repair.submitRepair()

    // 请求路径用 canonical 形态（'01974' → '1974'）
    expect(apiMocks.createDescriptionRepair).toHaveBeenCalledWith('1974', expect.objectContaining({ idempotency_key: 'key-1' }))
    // 冻结键也用同一形态
    expect(repair.frozenSubmit.value?.auxId).toBe('1974')
    expect(repair.frozenSubmit.value?.idempotencyKey).toBe('key-1')
  })
})
