/**
 * 工作面纯派生（board-health-triage-advice seam C2 / task 3.2-3.3）。
 *
 * 只测纯函数：分诊拆分、任务状态映射、三阶段推导、计数格式化、进展文案。
 * 口径不在前端重算——这里只把服务端状态翻译成展示形状。
 *
 * fix-board-health-magnet-signal：磁铁对比文案（按口径标记分支：新口径 / 新口径但
 * 健康侧不可算 / 旧口径回退 + 标注）、磁铁结论文案（旧口径带口径标记）、
 * 闸门降级提示文案（占比/阈值/原因取服务端值）、闸门降级项不进待核对索引。
 */
import { describe, expect, it } from 'vitest'
import type { AuxiliaryRepairJobView } from '~/api/normalizers/auxiliaryRepair'
import type { BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import type { BoardHealthMagnetView, BoardHealthTriageGuardView } from '~/api/normalizers/boardHealthTriage'
import {
  boardHealthIssueStatus,
  boardHealthMagnetComparisonText,
  boardHealthMagnetVerdictText,
  boardHealthProgressText,
  boardHealthRepairUnsupportedReason,
  boardHealthStageViews,
  boardHealthTriageGuardNotice,
  boardHealthWorkspaceState,
  boardHealthWorkspaceStateCopy,
  formatRepairCounter,
  splitTriageItems,
  type BoardHealthIssueStatus,
} from './boardHealthWorkspace'

function job(overrides: Partial<AuxiliaryRepairJobView> = {}): AuxiliaryRepairJobView {
  return {
    id: '77',
    auxiliaryLabelId: '11',
    idempotencyKey: 'key-1',
    expectedVersion: 'v-1',
    oldDescription: '旧',
    newDescription: '新',
    stage: 'matching',
    status: 'matching',
    descriptionCommitted: true,
    total: 40,
    processed: 12,
    failed: 3,
    notApplicable: 5,
    remaining: 20,
    error: '',
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

describe('splitTriageItems', () => {
  it('需核对项（remove/rewrite）与其余标签（keep/insufficient）分开', () => {
    const { issues, others } = splitTriageItems(
      [
        { auxId: '11', category: 'rewrite', evidence: '证据', suggestion: '建议', actionable: true, labelType: 'auxiliary', coreSubject: false },
        { auxId: '12', category: 'keep', evidence: '短名天花板', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: false },
        { auxId: '13', category: 'insufficient', evidence: '样本不足', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: false },
        { auxId: '14', category: 'remove', evidence: '磁铁成立', suggestion: '', actionable: true, labelType: 'auxiliary', coreSubject: false },
      ],
      (auxId) => `标签${auxId}`,
      () => 'todo',
    )

    expect(issues.map((item) => item.auxId)).toEqual(['11', '14'])
    expect(others.map((item) => item.auxId)).toEqual(['12', '13'])
    expect(issues[0]?.label).toBe('标签11')
  })

  // 4.1 M1：组合标签的 rewrite 在服务端没有修复入口 → 不可执行 + 明确说明。
  it('组合标签的 rewrite：不可执行且给出原因（证据仍在，不隐藏）', () => {
    const { issues } = splitTriageItems(
      [{ auxId: '15', category: 'rewrite', evidence: 'desc 预筛命中此次', suggestion: '展会举办地', actionable: false, labelType: 'composite', coreSubject: false }],
      (auxId) => `标签${auxId}`,
      () => 'todo',
    )
    expect(issues[0]?.actionable).toBe(false)
    expect(issues[0]?.notExecutableReason).toBe('组合标签暂不支持描述修复')
    expect(issues[0]?.evidence).toBe('desc 预筛命中此次')
  })

  it('组合标签的 remove：仍可摘除（无不可执行原因）', () => {
    const { issues } = splitTriageItems(
      [{ auxId: '15', category: 'remove', evidence: '磁铁成立', suggestion: '', actionable: true, labelType: 'composite', coreSubject: false }],
      (auxId) => `标签${auxId}`,
      () => 'todo',
    )
    expect(issues[0]?.actionable).toBe(true)
    expect(issues[0]?.notExecutableReason).toBeNull()
  })

  it('类型未知（旧快照缺 label_type）：按不可执行处理，不猜类型', () => {
    const { issues } = splitTriageItems(
      [{ auxId: '16', category: 'rewrite', evidence: '证据', suggestion: '建议', actionable: false, labelType: '', coreSubject: false }],
      (auxId) => `标签${auxId}`,
      () => 'todo',
    )
    expect(issues[0]?.actionable).toBe(false)
    expect(issues[0]?.notExecutableReason).toBe('标签类型未知，无法执行描述修复')
  })

  // fix-board-health-magnet-signal D8：闸门降级项已被服务端落为 insufficient，
  // 既有分流自然使其退出「待核对」索引——不新增前端过滤逻辑。
  it('闸门降级项（insufficient）：不进待核对索引，落入「其余标签」（无修复入口）', () => {
    const { issues, others } = splitTriageItems(
      [
        { auxId: '11', category: 'insufficient', evidence: '可执行占比 47/47（100%）超过阈值 0.8，本轮信号不可用', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: false },
        { auxId: '12', category: 'keep', evidence: '核心主体保护', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: true },
      ],
      (auxId) => `标签${auxId}`,
      () => 'todo',
    )
    expect(issues).toEqual([])
    expect(others.map((item) => item.auxId)).toEqual(['11', '12'])
  })

  it('核心主体标记：随分诊项透传到索引项（服务端判定，前端不重判）', () => {
    const { issues, others } = splitTriageItems(
      [
        { auxId: '11', category: 'rewrite', evidence: '证据', suggestion: '建议', actionable: true, labelType: 'auxiliary', coreSubject: true },
        { auxId: '12', category: 'keep', evidence: '核心主体保护', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: true },
        { auxId: '13', category: 'remove', evidence: '磁铁成立', suggestion: '', actionable: true, labelType: 'auxiliary', coreSubject: false },
      ],
      (auxId) => `标签${auxId}`,
      () => 'todo',
    )
    expect(issues[0]?.coreSubject).toBe(true)
    expect(others[0]?.coreSubject).toBe(true)
    expect(issues[1]?.coreSubject).toBe(false)
  })
})

describe('boardHealthMagnetComparisonText', () => {
  const newMagnet: BoardHealthMagnetView = {
    goodMean: 0.7,
    goodSamples: 284,
    allMean: 0.68,
    failedMean: 0.72,
    allSamples: 40,
    failedSamples: 6,
    suspicious: true,
    insufficient: false,
    caliber: 'healthy_subset',
  }

  it('新口径：失败子集 vs 健康子集（不拿全体均值作对比）', () => {
    const text = boardHealthMagnetComparisonText(newMagnet)
    expect(text).toContain('失败子集 0.720（6 项）')
    expect(text).toContain('健康子集 0.700（284 项）')
    expect(text).not.toContain('全体')
    expect(text).not.toContain('旧口径快照')
  })

  it('新口径但健康侧不可算（goodMean=null）：明说不可算，不回退全体均值、不标旧口径', () => {
    const text = boardHealthMagnetComparisonText({ ...newMagnet, goodMean: null, goodSamples: 0 })
    expect(text).toContain('失败子集 0.720（6 项）')
    expect(text).toContain('健康子集不可算')
    expect(text).not.toContain('全体')
    expect(text).not.toContain('旧口径快照')
  })

  it('旧口径快照（caliber 为空）：回退旧口径并标注「旧口径快照」', () => {
    const text = boardHealthMagnetComparisonText({ ...newMagnet, caliber: '', goodMean: null, goodSamples: 0 })
    expect(text).toContain('失败子集 0.720（6 项）')
    expect(text).toContain('全体 0.680（40 项）')
    expect(text).toContain('旧口径快照')
    expect(text).not.toContain('健康子集')
  })

  it('无信号：显式未知（不补 0）', () => {
    expect(boardHealthMagnetComparisonText(null)).toContain('未知')
  })
})

describe('boardHealthMagnetVerdictText', () => {
  const newMagnet: BoardHealthMagnetView = {
    goodMean: 0.52,
    goodSamples: 284,
    allMean: 0.68,
    failedMean: 0.72,
    allSamples: 40,
    failedSamples: 6,
    suspicious: true,
    insufficient: false,
    caliber: 'healthy_subset',
  }

  it('新口径快照：结论不带口径标记（照搬服务端结论）', () => {
    expect(boardHealthMagnetVerdictText(newMagnet)).toBe('差值显著，疑似磁铁污染')
    expect(boardHealthMagnetVerdictText({ ...newMagnet, suspicious: false })).toBe('差值未达阈值')
    expect(boardHealthMagnetVerdictText({ ...newMagnet, insufficient: true })).toBe('样本不足，不作判断')
  })

  it('旧口径快照：结论带口径标记（旧公式语义与新口径相反，勿据此操作）', () => {
    const text = boardHealthMagnetVerdictText({ ...newMagnet, caliber: '' })
    expect(text).toContain('旧口径快照的磁铁结论')
    expect(text).toContain('口径已反转，勿据此操作')
    expect(text).toContain('差值显著，疑似磁铁污染')
  })

  it('无信号：空串（不编结论）', () => {
    expect(boardHealthMagnetVerdictText(null)).toBe('')
  })
})

describe('boardHealthTriageGuardNotice', () => {
  const guard: BoardHealthTriageGuardView = {
    actionable: 47,
    total: 47,
    ratio: 1,
    threshold: 0.8,
    degraded: true,
    reason: '可执行占比 47/47（100%）超过阈值 0.8，本轮信号不可用',
  }

  it('降级轮：警示文案含「本轮信号可能不可用、不得按此批量操作」+ 服务端占比/阈值/原因', () => {
    const text = boardHealthTriageGuardNotice(guard)
    expect(text).toContain('本轮信号可能不可用、不得按此批量操作')
    expect(text).toContain('100%')
    expect(text).toContain('80%')
    expect(text).toContain('可执行占比 47/47（100%）超过阈值 0.8，本轮信号不可用')
  })

  it('未降级 / 无闸门记录：不显示警示条（null）', () => {
    expect(boardHealthTriageGuardNotice({ ...guard, degraded: false })).toBeNull()
    expect(boardHealthTriageGuardNotice(null)).toBeNull()
  })

  it('占比/阈值未知：不臆造数字，只保留警示句与原因', () => {
    const text = boardHealthTriageGuardNotice({
      ...guard,
      ratio: null,
      threshold: null,
      reason: '服务端未给出可执行占比',
    })
    expect(text).toContain('本轮信号可能不可用、不得按此批量操作')
    expect(text).toContain('服务端未给出可执行占比')
    expect(text).not.toContain('超过阈值')
  })
})

describe('boardHealthRepairUnsupportedReason', () => {
  it('auxiliary 可执行；composite 与未知类型给出明确说明', () => {
    expect(boardHealthRepairUnsupportedReason('auxiliary')).toBeNull()
    expect(boardHealthRepairUnsupportedReason('composite')).toBe('组合标签暂不支持描述修复')
    expect(boardHealthRepairUnsupportedReason('')).toBe('标签类型未知，无法执行描述修复')
  })
})

describe('boardHealthIssueStatus', () => {
  it('任务状态 → 处置状态（unknown 保守落 unknown，不当失败/完成）', () => {
    const cases: Array<[Parameters<typeof boardHealthIssueStatus>[0], BoardHealthIssueStatus]> = [
      ['pending', 'running'],
      ['embedding', 'running'],
      ['saving', 'running'],
      ['matching', 'running'],
      ['completed', 'completed'],
      ['embedding_failed', 'failed'],
      ['save_failed', 'failed'],
      ['matching_failed', 'failed'],
      ['conflict', 'conflict'],
      ['unavailable', 'unavailable'],
      ['unknown', 'unknown'],
    ]
    for (const [status, expected] of cases) {
      expect(boardHealthIssueStatus(status)).toBe(expected)
    }
  })
})

describe('boardHealthStageViews', () => {
  it('pending：三阶段都未开始', () => {
    expect(boardHealthStageViews(job({ status: 'pending', stage: '' })).map((s) => s.state)).toEqual(['pending', 'pending', 'pending'])
  })

  it('matching：前两阶段完成、匹配阶段进行中', () => {
    expect(boardHealthStageViews(job({ status: 'matching', stage: 'matching' })).map((s) => s.state)).toEqual(['done', 'done', 'current'])
  })

  it('completed：三阶段全部完成', () => {
    expect(boardHealthStageViews(job({ status: 'completed' })).map((s) => s.state)).toEqual(['done', 'done', 'done'])
  })

  it('matching_failed：匹配阶段失败，前两阶段完成', () => {
    expect(boardHealthStageViews(job({ status: 'matching_failed' })).map((s) => s.state)).toEqual(['done', 'done', 'error'])
  })

  it('conflict/unavailable：停在保存阶段（未覆盖数据）', () => {
    expect(boardHealthStageViews(job({ status: 'conflict' })).map((s) => s.state)).toEqual(['done', 'error', 'pending'])
    expect(boardHealthStageViews(job({ status: 'unavailable' })).map((s) => s.state)).toEqual(['done', 'error', 'pending'])
  })

  it('unknown：不当完成也不当失败（三阶段都未标）', () => {
    expect(boardHealthStageViews(job({ status: 'unknown' })).map((s) => s.state)).toEqual(['pending', 'pending', 'pending'])
  })

  it('无任务：空序列', () => {
    expect(boardHealthStageViews(null)).toEqual([])
  })
})

describe('formatRepairCounter', () => {
  it('null/undefined → 未知（不补 0）', () => {
    expect(formatRepairCounter(null)).toBe('未知')
    expect(formatRepairCounter(undefined)).toBe('未知')
    expect(formatRepairCounter(0)).toBe('0')
    expect(formatRepairCounter(12)).toBe('12')
  })
})

describe('boardHealthProgressText', () => {
  it('完成：措辞含「效果待下一轮体检验证」，不说评分已提升', () => {
    expect(boardHealthProgressText(job({ status: 'completed' }))).toContain('效果待下一轮体检验证')
  })

  it('匹配中：只报真实计数（未知显示未知）', () => {
    expect(boardHealthProgressText(job({ status: 'matching', processed: 12, total: 40 }))).toContain('已处理 12/40')
    expect(boardHealthProgressText(job({ status: 'matching', processed: null, total: null }))).toContain('未知')
  })

  it('无任务：空串（不编进展）', () => {
    expect(boardHealthProgressText(null)).toBe('')
  })

  it('匹配失败且失败数未知：只说「数量未知」，不提具体数', () => {
    const text = boardHealthProgressText(job({ status: 'matching_failed', failed: null }))
    expect(text).toContain('数量未知')
    expect(text).not.toContain('未知 项')
  })
})


function snapshotFixture(overrides: Partial<BoardHealthSnapshotDTO> = {}): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 5,
    checked_at: '2026-10-29T06:30:00Z',
    metrics: {
      board: { matched_tags: 428, reason_dist: {}, ghost_matches: 40, vector_missing: 0 },
      labels: [],
      triage: [{ aux_id: 11, category: 'rewrite', evidence: '证据', suggestion: '建议' }],
    },
    uncaptured_candidates: [],
    llm_score: 74,
    llm_comment: '评语',
    created_at: '',
    ...overrides,
  }
}

describe('boardHealthWorkspaceState', () => {
  it('降级轮（score/comment 均 NULL）先于「无 triage」判定', () => {
    const state = boardHealthWorkspaceState({
      loading: false,
      error: null,
      snapshot: snapshotFixture({ llm_score: null, llm_comment: null }),
      triageAvailable: false,
      triageCount: 0,
    })
    expect(state).toBe('degraded')
    expect(boardHealthWorkspaceStateCopy(state).text).toContain('不能据此推断当前标签健康')
  })

  it('历史快照只留「有评分无 triage」', () => {
    const state = boardHealthWorkspaceState({
      loading: false,
      error: null,
      snapshot: snapshotFixture(),
      triageAvailable: false,
      triageCount: 0,
    })
    expect(state).toBe('old-snapshot')
    expect(boardHealthWorkspaceStateCopy(state).title).toBe('历史快照')
  })

  it('空清单（有评分）→ 本轮没有分诊清单；正常轮 → normal', () => {
    expect(boardHealthWorkspaceState({
      loading: false, error: null, snapshot: snapshotFixture(), triageAvailable: true, triageCount: 0,
    })).toBe('no-triage')
    expect(boardHealthWorkspaceState({
      loading: false, error: null, snapshot: snapshotFixture(), triageAvailable: true, triageCount: 2,
    })).toBe('normal')
  })

  it('404/403：判 gone（版块可能已不存在），不当 stale', () => {
    for (const status of [404, 403]) {
      const state = boardHealthWorkspaceState({
        loading: false,
        error: 'board not found',
        errorStatus: status,
        snapshot: snapshotFixture(),
        triageAvailable: true,
        triageCount: 1,
      })
      expect(state).toBe('gone')
      const copy = boardHealthWorkspaceStateCopy(state)
      expect(copy.title).toContain('可能已经不存在')
      expect(copy.text).toContain('404/403')
    }
  })

  it('读取失败：有旧快照按 stale 保留（不进错误态），无旧快照才报错', () => {
    expect(boardHealthWorkspaceState({
      loading: false, error: '服务暂时不可用', snapshot: snapshotFixture(), triageAvailable: true, triageCount: 1,
    })).toBe('normal')
    expect(boardHealthWorkspaceState({
      loading: false, error: '服务暂时不可用', snapshot: null, triageAvailable: false, triageCount: 0,
    })).toBe('error')
  })

  it('loading / 无快照', () => {
    expect(boardHealthWorkspaceState({
      loading: true, error: null, snapshot: snapshotFixture(), triageAvailable: true, triageCount: 1,
    })).toBe('loading')
    expect(boardHealthWorkspaceState({
      loading: false, error: null, snapshot: null, triageAvailable: false, triageCount: 0,
    })).toBe('no-snapshot')
  })
})
