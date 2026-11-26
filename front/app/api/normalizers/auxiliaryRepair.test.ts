/**
 * 共享描述修复 normalizer（board-health-triage-advice seam C1 / task 3.1）。
 *
 * 覆盖：字符串 ID / camelCase / typed 状态与阶段 / opaque version 透传 /
 * remaining 未知（不补 0）/ not_applicable 单列（不算成功）/ 私有执行字段过滤 /
 * 描述三规则。
 */
import { describe, expect, it } from 'vitest'
import {
  canonicalAuxiliaryId,
  isAuxiliaryRepairMutexReleasedStatus,
  isAuxiliaryRepairNewConfirmAllowedStatus,
  isAuxiliaryRepairPollStopStatus,
  isAuxiliaryRepairRetryableStatus,
  normalizeAuxiliaryRepairContext,
  normalizeAuxiliaryRepairJob,
  validateRepairDescription,
  type AuxiliaryRepairJobRaw,
} from './auxiliaryRepair'

function jobRaw(overrides: Partial<AuxiliaryRepairJobRaw> = {}): AuxiliaryRepairJobRaw {
  return {
    id: 77,
    auxiliary_label_id: 1974,
    idempotency_key: 'key-1',
    expected_version: 'v-opaque-abc',
    old_description: '旧描述',
    new_description: '新描述',
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

describe('normalizeAuxiliaryRepairJob', () => {
  it('camelCase + 字符串 ID；计数逐项搬运（NA 单列，不并入 processed）', () => {
    const view = normalizeAuxiliaryRepairJob(jobRaw())
    expect(view).toEqual({
      id: '77',
      auxiliaryLabelId: '1974',
      idempotencyKey: 'key-1',
      expectedVersion: 'v-opaque-abc',
      oldDescription: '旧描述',
      newDescription: '新描述',
      stage: 'matching',
      status: 'matching',
      descriptionCommitted: true,
      total: 40,
      processed: 12,
      failed: 3,
      notApplicable: 5,
      remaining: 20,
      error: '',
      createdAt: '2026-10-29T10:00:00Z',
      updatedAt: '2026-10-29T10:01:00Z',
    })
  })

  it('私有执行元数据不在视图里（即便 wire 混入也不透出）', () => {
    const raw = jobRaw() as AuxiliaryRepairJobRaw & Record<string, unknown>
    raw.claim_owner = 'worker-1'
    raw.phase_token = 'tok-1'
    raw.committed_version = 'v9'
    raw.committed_at = '2026-10-29T10:00:30Z'
    const view = normalizeAuxiliaryRepairJob(raw)
    expect(view).not.toHaveProperty('claim_owner')
    expect(view).not.toHaveProperty('phase_token')
    expect(view).not.toHaveProperty('committed_version')
    expect(view).not.toHaveProperty('committed_at')
    expect(Object.keys(view ?? {})).not.toContain('claimOwner')
  })

  it('旧 wire 缺 remaining：保持未知（null），不补 0、不用差值反推', () => {
    const view = normalizeAuxiliaryRepairJob(jobRaw({ remaining: undefined }))
    expect(view?.remaining).toBeNull()
  })

  it('remaining 非法值：未知（null），不落 0', () => {
    expect(normalizeAuxiliaryRepairJob(jobRaw({ remaining: -1 }))?.remaining).toBeNull()
    expect(normalizeAuxiliaryRepairJob(jobRaw({ remaining: 'x' as unknown as number }))?.remaining).toBeNull()
  })

  it('缺 not_applicable 的旧任务：读作 0（后端契约），不并入 processed', () => {
    const view = normalizeAuxiliaryRepairJob(jobRaw({ not_applicable: undefined, processed: 12 }))
    expect(view?.notApplicable).toBe(0)
    expect(view?.processed).toBe(12)
  })

  it('total/processed/failed 缺失或非法：未知（null），不得捏造 0（否则会造出完成进度）', () => {
    const missing = normalizeAuxiliaryRepairJob(jobRaw({ total: undefined, processed: undefined, failed: undefined }))
    expect(missing?.total).toBeNull()
    expect(missing?.processed).toBeNull()
    expect(missing?.failed).toBeNull()

    const invalid = normalizeAuxiliaryRepairJob(jobRaw({ total: -1, processed: 'x' as unknown as number, failed: Number.NaN }))
    expect(invalid?.total).toBeNull()
    expect(invalid?.processed).toBeNull()
    expect(invalid?.failed).toBeNull()
  })

  it('状态闭集：已知值 typed；未知状态落 unknown（不猜语义）', () => {
    expect(normalizeAuxiliaryRepairJob(jobRaw({ status: 'completed' }))?.status).toBe('completed')
    expect(normalizeAuxiliaryRepairJob(jobRaw({ status: 'matching_failed' }))?.status).toBe('matching_failed')
    expect(normalizeAuxiliaryRepairJob(jobRaw({ status: 'unavailable' }))?.status).toBe('unavailable')
    expect(normalizeAuxiliaryRepairJob(jobRaw({ status: 'weird_new_state' }))?.status).toBe('unknown')
    expect(normalizeAuxiliaryRepairJob(jobRaw({ status: undefined }))?.status).toBe('unknown')
  })

  it('阶段闭集：pending 可为空串；未知阶段落 unknown', () => {
    expect(normalizeAuxiliaryRepairJob(jobRaw({ stage: '' }))?.stage).toBe('')
    expect(normalizeAuxiliaryRepairJob(jobRaw({ stage: 'embedding' }))?.stage).toBe('embedding')
    expect(normalizeAuxiliaryRepairJob(jobRaw({ stage: 'unknown_stage' }))?.stage).toBe('unknown')
  })

  it('空/无 id 的载荷：不造任务状态（返回 null）', () => {
    expect(normalizeAuxiliaryRepairJob(null)).toBeNull()
    expect(normalizeAuxiliaryRepairJob(jobRaw({ id: undefined as unknown as number }))).toBeNull()
  })
})

describe('状态词表：停止轮询 / 互斥释放 / 可重试 三集合', () => {
  it('停止轮询：终态 + 三种失败 + unknown（词表外不可解读）', () => {
    for (const status of ['completed', 'conflict', 'unavailable', 'embedding_failed', 'save_failed', 'matching_failed', 'unknown'] as const) {
      expect(isAuxiliaryRepairPollStopStatus(status)).toBe(true)
    }
    for (const status of ['pending', 'embedding', 'saving', 'matching'] as const) {
      expect(isAuxiliaryRepairPollStopStatus(status)).toBe(false)
    }
  })

  it('互斥释放（允许显式新确认）：仅 completed/conflict/unavailable——失败态仍持同标签锁', () => {
    for (const status of ['completed', 'conflict', 'unavailable'] as const) {
      expect(isAuxiliaryRepairMutexReleasedStatus(status)).toBe(true)
      expect(isAuxiliaryRepairNewConfirmAllowedStatus(status)).toBe(true)
    }
    for (const status of ['embedding_failed', 'save_failed', 'matching_failed', 'pending', 'matching', 'unknown'] as const) {
      expect(isAuxiliaryRepairMutexReleasedStatus(status)).toBe(false)
      expect(isAuxiliaryRepairNewConfirmAllowedStatus(status)).toBe(false)
    }
  })

  it('仅三种失败态可由用户明确重试', () => {
    expect(isAuxiliaryRepairRetryableStatus('embedding_failed')).toBe(true)
    expect(isAuxiliaryRepairRetryableStatus('save_failed')).toBe(true)
    expect(isAuxiliaryRepairRetryableStatus('matching_failed')).toBe(true)
    expect(isAuxiliaryRepairRetryableStatus('completed')).toBe(false)
    expect(isAuxiliaryRepairRetryableStatus('conflict')).toBe(false)
  })
})

describe('canonicalAuxiliaryId', () => {
  it('数字串去前导零（非数字保持原样），int64 不丢精度', () => {
    expect(canonicalAuxiliaryId('007')).toBe('7')
    expect(canonicalAuxiliaryId('00197')).toBe('197')
    expect(canonicalAuxiliaryId('0')).toBe('0')
    expect(canonicalAuxiliaryId(1974)).toBe('1974')
    expect(canonicalAuxiliaryId(' 1974 ')).toBe('1974')
    expect(canonicalAuxiliaryId('')).toBe('')
    expect(canonicalAuxiliaryId(null)).toBe('')
    expect(canonicalAuxiliaryId(undefined)).toBe('')
    expect(canonicalAuxiliaryId('9007199254740993')).toBe('9007199254740993')
  })

  it('任务与上下文的 ID 走同一 canonical 形态（前导零不分家）', () => {
    const view = normalizeAuxiliaryRepairJob(jobRaw({ id: 77, auxiliary_label_id: 1974 }))
    expect(view?.id).toBe('77')
    expect(view?.auxiliaryLabelId).toBe('1974')
  })
})

describe('normalizeAuxiliaryRepairContext', () => {
  it('camelCase + 字符串 ID；version 原样透传（不解析）', () => {
    const view = normalizeAuxiliaryRepairContext({
      auxiliary_label: { id: 1974, label: '上海', description: '旧描述', status: 'active', version: 'v-opaque-abc' },
      direct_boards: [
        { id: 12, label: '中国国内新闻', status: 'active' },
        { id: 13, label: '停用版块', status: 'disabled' },
      ],
      active_tag_count: 15374,
      latest_task: jobRaw(),
    })
    expect(view).toEqual({
      label: { id: '1974', label: '上海', description: '旧描述', status: 'active', version: 'v-opaque-abc' },
      directBoards: [
        { id: '12', label: '中国国内新闻', status: 'active' },
        { id: '13', label: '停用版块', status: 'disabled' },
      ],
      activeTagCount: 15374,
      latestTask: expect.objectContaining({ id: '77', status: 'matching' }),
    })
  })

  it('无任务：latestTask 为 null（不造任务状态）', () => {
    const view = normalizeAuxiliaryRepairContext({
      auxiliary_label: { id: 1, label: '标签', description: '', status: 'active', version: 'v1' },
      direct_boards: [],
      active_tag_count: 0,
      latest_task: null,
    })
    expect(view?.latestTask).toBeNull()
    expect(view?.directBoards).toEqual([])
    expect(view?.activeTagCount).toBe(0)
  })

  it('共享范围字段缺失：directBoards/activeTagCount 为 null（未知，不谎称 0 范围）', () => {
    const view = normalizeAuxiliaryRepairContext({
      auxiliary_label: { id: 1, label: '标签', description: '', status: 'active', version: 'v1' },
    } as never)
    expect(view?.directBoards).toBeNull()
    expect(view?.activeTagCount).toBeNull()
  })

  it('缺 auxiliary_label：上下文不可用（null），不造版本', () => {
    expect(normalizeAuxiliaryRepairContext({ direct_boards: [] } as never)).toBeNull()
    expect(normalizeAuxiliaryRepairContext(null)).toBeNull()
  })
})

describe('validateRepairDescription', () => {
  it('三规则闭集：空 / 超 500 码点 / 恰好重复标签名', () => {
    expect(validateRepairDescription('上海', '  ')).toEqual({ ok: false, error: '描述不能为空' })
    expect(validateRepairDescription('上海', '字'.repeat(501)).ok).toBe(false)
    expect(validateRepairDescription('上海', '上海')).toEqual({ ok: false, error: '描述不能只重复标签名' })
  })

  it('合法描述：返回 trim 后的值', () => {
    expect(validateRepairDescription('上海', '  中国的超大城市  ')).toEqual({ ok: true, value: '中国的超大城市' })
    expect(validateRepairDescription('上海', '上海知名商业综合体').ok).toBe(true)
    expect(validateRepairDescription('上海', '字'.repeat(500)).ok).toBe(true)
  })
})
