/**
 * 体检分诊 normalizer（board-health-triage-advice seam C1 / task 3.1）。
 *
 * 覆盖：旧快照缺 triage / 新快照四类分诊 / 非法分类 / 非法 aux 引用 / 重复记录 /
 * 无证据 / keep-insufficient 不带可执行建议 / 字符串 ID 与 camelCase /
 * magnet·neighbors·desc_flags 信号搬运（前端不重算口径）。
 */
import { describe, expect, it } from 'vitest'
import type { BoardHealthLabelMetric, BoardHealthSnapshotDTO } from '~/api/semanticBoards'
import {
  normalizeBoardHealthLabelSignals,
  normalizeBoardHealthMagnetFlow,
  normalizeBoardHealthTriage,
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
      },
      { auxId: '12', category: 'keep', evidence: '方向通过率 100%，短名天花板', suggestion: '', actionable: false, labelType: 'auxiliary' },
      { auxId: '13', category: 'remove', evidence: '磁铁信号成立：失败子集 0.52 vs 全体 0.68', suggestion: '', actionable: true, labelType: 'auxiliary' },
      { auxId: '14', category: 'insufficient', evidence: '失败子集样本 2 条，低于最小样本量', suggestion: '', actionable: false, labelType: 'auxiliary' },
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
      { auxId: '11', category: 'rewrite', evidence: '第一条', suggestion: '新描述 A', actionable: true, labelType: 'auxiliary' },
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
      { auxId: '11', category: 'rewrite', evidence: '描述偏窄', suggestion: '', actionable: false, labelType: 'auxiliary' },
    ])
  })

  it('keep/insufficient 携带的建议文案被清空（只读结论不带可执行建议）', () => {
    const view = normalizeBoardHealthTriage(withTriage([
      { aux_id: 12, category: 'keep', evidence: '短名天花板', suggestion: '不该出现的建议' },
    ]))
    expect(view.items).toEqual([
      { auxId: '12', category: 'keep', evidence: '短名天花板', suggestion: '', actionable: false, labelType: 'auxiliary' },
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
            magnet: { all_mean: 0.68, failed_mean: 0.52, all_samples: 40, failed_samples: 6, suspicious: true, insufficient: false },
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
          allMean: 0.68,
          failedMean: 0.52,
          allSamples: 40,
          failedSamples: 6,
          suspicious: true,
          insufficient: false,
        },
        neighbors: [
          { auxId: '21', label: '举办地点', cosine: 0.796 },
          { auxId: '22', label: '小商品城', cosine: 0.709 },
        ],
        descFlags: ['此次', '期间'],
      },
      // 缺信号的历史快照：磁铁/最近邻为 null（信号不足），词根空数组
      {
        auxId: '12',
        label: 'OpenAI',
        description: '',
        magnet: null,
        neighbors: null,
        descFlags: [],
      },
    ])
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
