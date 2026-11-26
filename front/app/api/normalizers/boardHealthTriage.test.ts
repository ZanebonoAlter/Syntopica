/**
 * 体检分诊 normalizer（board-health-triage-advice seam C1 / task 3.1）。
 *
 * 覆盖：旧快照缺 triage / 新快照四类分诊 / 非法分类 / 非法 aux 引用 / 重复记录 /
 * 无证据 / keep-insufficient 不带可执行建议 / 字符串 ID 与 camelCase /
 * magnet·neighbors·desc_flags 信号搬运（前端不重算口径）。
 *
 * fix-board-health-magnet-signal：健康子集口径（good_mean/good_samples）、
 * 核心主体标记（core_subject）、分诊合理性闸门（triage_guard）的新增字段搬运，
 * 以及旧快照缺这些字段时的容错（null / false / 0，不报错、不臆造）。
 */
import { describe, expect, it } from 'vitest'
import type { BoardHealthLabelMetric, BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import {
  normalizeBoardHealthLabelSignals,
  normalizeBoardHealthMagnetFlow,
  normalizeBoardHealthTriage,
  normalizeBoardHealthTriageGuard,
} from './boardHealthTriage'

function labelMetric(overrides: Partial<BoardHealthLabelMetric> = {}): BoardHealthLabelMetric {
  return {
    aux_id: 11,
    label: '上海',
    label_type: 'auxiliary',
    samples: 18,
    cosine_mean: 0.4,
    cosine_median: null,
    cosine_p25: null,
    cosine_p75: null,
    low_sim_ratio: null,
    insufficient: false,
    ...overrides,
  }
}

function snapshot(overrides: Partial<BoardHealthSnapshotDTO> = {}): BoardHealthSnapshotDTO {
  return {
    id: 1,
    semantic_board_id: 1974,
    checked_at: '2026-10-28T23:00:00+08:00',
    metrics: {
      board: { matched_tags: 128, reason_dist: {}, ghost_matches: 9, vector_missing: 2 },
      labels: [
        labelMetric(),
        labelMetric({ aux_id: 12, label: 'OpenAI', samples: 30, cosine_mean: 0.57 }),
        labelMetric({ aux_id: 13, label: '中国媒体', samples: 9, cosine_mean: 0.62 }),
        labelMetric({ aux_id: 14, label: '某某', samples: 2, cosine_mean: null, insufficient: true }),
      ],
    },
    uncaptured_candidates: [],
    llm_score: 62,
    llm_comment: '评语。',
    created_at: '',
    ...overrides,
  }
}

function withTriage(triage: unknown) {
  const base = snapshot()
  return {
    ...base,
    metrics: { ...base.metrics, triage } as BoardHealthSnapshotDTO['metrics'],
  }
}

describe('normalizeBoardHealthTriage', () => {
  it('旧快照缺 triage：available=false 且不产出任何条目（按旧形态展示，不反推诊断）', () => {
    const view = normalizeBoardHealthTriage(snapshot())
    expect(view).toEqual({ available: false, items: [], dropped: 0 })
  })

  it('无快照：available=false，不造诊断', () => {
    expect(normalizeBoardHealthTriage(null)).toEqual({ available: false, items: [], dropped: 0 })
  })

  it('降级轮次的空 triage 数组：available=true 且条目为空（不是旧形态）', () => {
    const view = normalizeBoardHealthTriage(withTriage([]))
    expect(view.available).toBe(true)
    expect(view.items).toEqual([])
    expect(view.dropped).toBe(0)
  })

  it('四类分诊：camelCase + 字符串 auxId；remove/rewrite 可执行，keep/insufficient 只读', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 11, category: 'rewrite', evidence: '最近邻「举办地点」0.796，描述偏窄', suggestion: '中国的超大城市' },
      { aux_id: 12, category: 'keep', evidence: '方向通过率 100%，短名天花板' },
      { aux_id: 13, category: 'remove', evidence: '磁铁信号成立：失败子集 0.52 vs 全体 0.68' },
      { aux_id: 14, category: 'insufficient', evidence: '失败子集样本 2 条，低于最小样本量' },
    ]))

    expect(view.available).toBe(true)
    expect(view.dropped).toBe(0)
    expect(view.items).toEqual([
      {
        auxId: '11',
        category: 'rewrite',
        evidence: '最近邻「举办地点」0.796，描述偏窄',
        suggestion: '中国的超大城市',
        actionable: true,
        labelType: 'auxiliary',
        coreSubject: false,
      },
      { auxId: '12', category: 'keep', evidence: '方向通过率 100%，短名天花板', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: false },
      { auxId: '13', category: 'remove', evidence: '磁铁信号成立：失败子集 0.52 vs 全体 0.68', suggestion: '', actionable: true, labelType: 'auxiliary', coreSubject: false },
      { auxId: '14', category: 'insufficient', evidence: '失败子集样本 2 条，低于最小样本量', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: false },
    ])
  })

  it('非法分类：丢弃该条且不作为可执行建议透出（其余合法条目保留）', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 11, category: 'explode', evidence: '未知分类' },
      { aux_id: 13, category: 'remove', evidence: '磁铁信号成立' },
    ]))
    expect(view.items.map((i) => i.auxId)).toEqual(['13'])
    expect(view.dropped).toBe(1)
  })

  it('非法 aux 引用（不在本轮构成标签里）：丢弃，不透出为可执行建议', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 9999, category: 'remove', evidence: '凭空引用' },
    ]))
    expect(view.items).toEqual([])
    expect(view.dropped).toBe(1)
  })

  it('一致重复：同标签留一条（其余计 dropped）', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 11, category: 'rewrite', evidence: '第一条', suggestion: '新描述 A' },
      { aux_id: 11, category: 'rewrite', evidence: '第一条', suggestion: '新描述 A' },
    ]))
    expect(view.items).toEqual([
      { auxId: '11', category: 'rewrite', evidence: '第一条', suggestion: '新描述 A', actionable: true, labelType: 'auxiliary', coreSubject: false },
    ])
    expect(view.dropped).toBe(1)
  })

  it('矛盾分类（同 aux 不同 category）：整组不可执行，全部丢弃并计 dropped', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 11, category: 'rewrite', evidence: '描述偏窄', suggestion: '新描述 A' },
      { aux_id: 11, category: 'remove', evidence: '磁铁信号成立' },
    ]))
    expect(view.items).toEqual([])
    expect(view.dropped).toBe(2)
  })

  it('证据/建议矛盾（同 aux 同 category 但证据不同）：整组不可执行', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 13, category: 'remove', evidence: '证据一' },
      { aux_id: 13, category: 'remove', evidence: '证据二' },
    ]))
    expect(view.items).toEqual([])
    expect(view.dropped).toBe(2)
  })
  it('无证据：不作为建议透出（不猜 rewrite/remove）', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 13, category: 'remove', evidence: '   ' },
    ]))
    expect(view.items).toEqual([])
    expect(view.dropped).toBe(1)
  })

  it('rewrite 缺建议文案：条目保留但不可执行（无文案无从执行）', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 11, category: 'rewrite', evidence: '描述偏窄' },
    ]))
    expect(view.items).toEqual([
      { auxId: '11', category: 'rewrite', evidence: '描述偏窄', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: false },
    ])
  })

  it('keep/insufficient 携带的建议文案被清空（只读结论不带可执行建议）', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 12, category: 'keep', evidence: '短名天花板', suggestion: '不该出现的建议' },
    ]))
    expect(view.items).toEqual([
      { auxId: '12', category: 'keep', evidence: '短名天花板', suggestion: '', actionable: false, labelType: 'auxiliary', coreSubject: false },
    ])
  })

  // 4.1 M1：组合标签也进分诊清单，但描述修复链只认 auxiliary → rewrite 不可执行。
  it('组合标签的 rewrite：不可执行（labelType 透出，证据仍在，不当它健康）', () => {
    const base = snapshot()
    const withComposite: BoardHealthSnapshotDTO = {
      ...base,
      metrics: {
        ...base.metrics,
        labels: [...base.metrics.labels, labelMetric({ aux_id: 15, label: '上海 × 展会', label_type: 'composite' })],
        triage: [
          { aux_id: 15, category: 'rewrite', evidence: 'desc 预筛命中此次', suggestion: '展会举办地' },
        ],
      },
    }
    const view = normalizeBoardHealthTriage(withComposite)
    expect(view.items).toEqual([
      {
        auxId: '15',
        category: 'rewrite',
        evidence: 'desc 预筛命中此次',
        suggestion: '展会举办地',
        actionable: false,
        labelType: 'composite',
        coreSubject: false,
      },
    ])
  })

  it('组合标签的 remove：仍可执行（走既有 DELETE 摘除链）', () => {
    const base = snapshot()
    const withComposite: BoardHealthSnapshotDTO = {
      ...base,
      metrics: {
        ...base.metrics,
        labels: [...base.metrics.labels, labelMetric({ aux_id: 15, label: '上海 × 展会', label_type: 'composite' })],
        triage: [
          { aux_id: 15, category: 'remove', evidence: '磁铁信号成立' },
        ],
      },
    }
    const view = normalizeBoardHealthTriage(withComposite)
    expect(view.items[0]?.actionable).toBe(true)
    expect(view.items[0]?.labelType).toBe('composite')
  })

  it('旧快照缺 label_type：rewrite 不可执行（类型未知不猜，不给死路入口）', () => {
    const base = snapshot()
    const withoutType: BoardHealthSnapshotDTO = {
      ...base,
      metrics: {
        ...base.metrics,
        labels: base.metrics.labels.map((label) => ({ ...label, label_type: undefined })),
        triage: [
          { aux_id: 11, category: 'rewrite', evidence: '描述偏窄', suggestion: '新描述' },
        ],
      },
    }
    const view = normalizeBoardHealthTriage(withoutType)
    expect(view.items[0]?.actionable).toBe(false)
    expect(view.items[0]?.labelType).toBe('')
  })
})

describe('normalizeBoardHealthLabelSignals', () => {
  it('磁铁/最近邻/词根：camelCase + 字符串 auxId，服务端数值原样搬运', () => {
    const withSignals: BoardHealthSnapshotDTO = {
      ...snapshot(),
      metrics: {
        ...snapshot().metrics,
        labels: [
          labelMetric({
            description: '上海知名商业综合体',
            magnet: { good_mean: 0.7, all_mean: 0.68, failed_mean: 0.52, good_samples: 284, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false, caliber: 'healthy_subset' },
            neighbors: [
              { aux_id: 21, label: '举办地点', cosine: 0.796 },
              { aux_id: 22, label: '小商品城', cosine: 0.709 },
            ],
            desc_flags: ['此次', '期间'],
          }),
          labelMetric({ aux_id: 12, label: 'OpenAI', magnet: null, neighbors: null, desc_flags: [] }),
        ],
      },
    }

    const views = normalizeBoardHealthLabelSignals(withSignals)
    expect(views).toEqual([
      {
        auxId: '11',
        label: '上海',
        description: '上海知名商业综合体',
        magnet: {
          goodMean: 0.7,
          goodSamples: 284,
          allMean: 0.68,
          failedMean: 0.52,
          allSamples: 40,
          failedSamples: 6,
          suspicious: true,
          insufficient: false,
          caliber: 'healthy_subset',
        },
        neighbors: [
          { auxId: '21', label: '举办地点', cosine: 0.796 },
          { auxId: '22', label: '小商品城', cosine: 0.709 },
        ],
        descFlags: ['此次', '期间'],
        coreSubject: false,
      },
      // 缺信号的历史快照：磁铁/最近邻为 null（信号不足），词根空数组
      {
        auxId: '12',
        label: 'OpenAI',
        description: '',
        magnet: null,
        neighbors: null,
        descFlags: [],
        coreSubject: false,
      },
    ])
  })

  // fix-board-health-magnet-signal：旧快照没有健康侧口径 → 不臆造，回退信号。
  it('旧快照缺 good_mean/good_samples：goodMean=null、goodSamples=0（不报错、不臆造）', () => {
    const withOldMagnet: BoardHealthSnapshotDTO = {
      ...snapshot(),
      metrics: {
        ...snapshot().metrics,
        labels: [
          labelMetric({
            magnet: { all_mean: 0.68, failed_mean: 0.52, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false },
          }),
        ],
      },
    }
    const views = normalizeBoardHealthLabelSignals(withOldMagnet)
    expect(views[0]?.magnet?.goodMean).toBeNull()
    expect(views[0]?.magnet?.goodSamples).toBe(0)
    // 旧字段仍在（历史兼容）
    expect(views[0]?.magnet?.allMean).toBe(0.68)
    expect(views[0]?.magnet?.failedMean).toBe(0.52)
    // 旧快照无口径标记 → 空串（旧口径），不臆造新口径
    expect(views[0]?.magnet?.caliber).toBe('')
  })

  // fix-board-health-magnet-signal D12 / R-4：口径标记只搬运，不判定。
  it('口径标记归一化：healthy_subset 原样；缺失 / 空串 / 非字符串 → 空串（不臆造新口径）', () => {
    function magnetOf(magnet: unknown): string {
      const withMagnet: BoardHealthSnapshotDTO = {
        ...snapshot(),
        metrics: {
          ...snapshot().metrics,
          labels: [labelMetric({ magnet: magnet as never })],
        },
      }
      return normalizeBoardHealthLabelSignals(withMagnet)[0]?.magnet?.caliber ?? ''
    }

    expect(magnetOf({ good_mean: 0.7, all_mean: 0.68, failed_mean: 0.52, good_samples: 284, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false, caliber: 'healthy_subset' })).toBe('healthy_subset')
    // 旧口径快照：字段存在但为空串
    expect(magnetOf({ all_mean: 0.68, failed_mean: 0.52, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false, caliber: '' })).toBe('')
    // 字段缺失（该字段写入之前的历史快照）
    expect(magnetOf({ all_mean: 0.68, failed_mean: 0.52, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false })).toBe('')
    // 非字符串 → 空串（不臆造 healthy_subset）
    expect(magnetOf({ all_mean: 0.68, failed_mean: 0.52, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false, caliber: 42 })).toBe('')
    expect(magnetOf({ all_mean: 0.68, failed_mean: 0.52, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false, caliber: { value: 'healthy_subset' } })).toBe('')
  })

  it('核心主体标记：core_subject 透出；缺字段按 false（不臆造）', () => {
    const withCore: BoardHealthSnapshotDTO = {
      ...snapshot(),
      metrics: {
        ...snapshot().metrics,
        labels: [
          labelMetric({ aux_id: 11, label: '伊朗', core_subject: true }),
          labelMetric({ aux_id: 12, label: '卡塔尔' }),
        ],
        triage: [
          { aux_id: 11, category: 'keep', evidence: '核心主体保护：该标签出现在板块标题/描述/别名中' },
          { aux_id: 12, category: 'keep', evidence: '方向通过率 100%' },
        ],
      },
    }
    const views = normalizeBoardHealthLabelSignals(withCore)
    expect(views[0]?.coreSubject).toBe(true)
    expect(views[1]?.coreSubject).toBe(false)

    const triage = normalizeBoardHealthTriage(withCore)
    expect(triage.items[0]?.coreSubject).toBe(true)
    expect(triage.items[1]?.coreSubject).toBe(false)
  })

  it('无快照：空数组，不补造信号', () => {
    expect(normalizeBoardHealthLabelSignals(null)).toEqual([])
  })
})

describe('normalizeBoardHealthMagnetFlow', () => {
  it('服务端已算好的流向：只搬运（camelCase），不重算 top1/可靠命中', () => {
    const base = snapshot()
    const withFlow: BoardHealthSnapshotDTO = {
      ...base,
      metrics: {
        ...base.metrics,
        board: {
          ...base.metrics.board,
          magnet_flow: { failed_tags: 40, flow_tags: 22, top_board: '中国国内新闻', top_tags: 9 },
        },
      },
    }
    expect(normalizeBoardHealthMagnetFlow(withFlow)).toEqual({
      failedTags: 40,
      flowTags: 22,
      topBoard: '中国国内新闻',
      topTags: 9,
    })
  })

  it('服务端未能给出可靠流向：topBoard 保持 null（未知，不臆造目标）', () => {
    const base = snapshot()
    const withFlow: BoardHealthSnapshotDTO = {
      ...base,
      metrics: {
        ...base.metrics,
        board: {
          ...base.metrics.board,
          magnet_flow: { failed_tags: 40, flow_tags: 0, top_board: null, top_tags: 0 },
        },
      },
    }
    expect(normalizeBoardHealthMagnetFlow(withFlow)?.topBoard).toBeNull()
  })

  it('缺该字段 / 无快照：null（未知，不补 0）', () => {
    expect(normalizeBoardHealthMagnetFlow(snapshot())).toBeNull()
    expect(normalizeBoardHealthMagnetFlow(null)).toBeNull()
  })
})

describe('normalizeBoardHealthTriageGuard', () => {
  function withGuard(guard: unknown): BoardHealthSnapshotDTO {
    const base = snapshot()
    return {
      ...base,
      metrics: { ...base.metrics, triage_guard: guard } as BoardHealthSnapshotDTO['metrics'],
    }
  }

  it('服务端已算好的闸门结论：只搬运（占比/阈值/降级原因原样透出）', () => {
    const view = normalizeBoardHealthTriageGuard(withGuard({
      actionable: 47,
      total: 47,
      ratio: 1,
      threshold: 0.8,
      degraded: true,
      reason: '可执行占比 47/47（100%）超过阈值 0.8，本轮信号不可用',
    }))
    expect(view).toEqual({
      actionable: 47,
      total: 47,
      ratio: 1,
      threshold: 0.8,
      degraded: true,
      reason: '可执行占比 47/47（100%）超过阈值 0.8，本轮信号不可用',
    })
  })

  it('未降级轮：degraded=false 也照常搬运（占比/阈值可见）', () => {
    const view = normalizeBoardHealthTriageGuard(withGuard({
      actionable: 34,
      total: 47,
      ratio: 0.7234,
      threshold: 0.8,
      degraded: false,
      reason: '',
    }))
    expect(view?.degraded).toBe(false)
    expect(view?.ratio).toBe(0.7234)
    expect(view?.reason).toBe('')
  })

  it('非法数值：ratio/threshold 回落为 null（不补 0 冒充占比），degraded 只认显式 true', () => {
    const view = normalizeBoardHealthTriageGuard(withGuard({
      actionable: '47',
      total: 47,
      ratio: 'not-a-number',
      threshold: Number.NaN,
      degraded: 'true',
      reason: 42,
    }))
    expect(view?.ratio).toBeNull()
    expect(view?.threshold).toBeNull()
    expect(view?.degraded).toBe(false)
    expect(view?.actionable).toBe(0)
    expect(view?.reason).toBe('')
  })

  it('缺该字段 / 无快照：null（无闸门记录，不臆造结论）', () => {
    expect(normalizeBoardHealthTriageGuard(snapshot())).toBeNull()
    expect(normalizeBoardHealthTriageGuard(null)).toBeNull()
  })
})
