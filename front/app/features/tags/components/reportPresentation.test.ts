import { describe, expect, it } from 'vitest'
import {
  buildChartLayout,
  buildEvidenceTableRows,
  buildSourceFreshness,
  formatReadingValue,
  periodTimestamp,
  type SourceFreshnessSummary,
} from './reportPresentation'
import type { SignalAppendixCall, SignalCalculation, SignalReportPayload } from '~/api/boardSignals'
import type { SignalChartPoint } from './signalReport'

// 3.2 呈现纯逻辑（design D5）：长小数/极小非零/null/日期间距参数表；
// 只消费入库 payload，不改源单位与账本。

describe('formatReadingValue（阅读短格式与原值分层）', () => {
  it.each([
    // 长小数：常规值短格式可读
    ['100.19999694824219', '100.2'],
    ['419.00000001', '419'],
    // 真 0 仍为 0
    ['0', '0'],
    ['0.00', '0'],
    // 极小非零保留量级，绝不显示成 0
    ['0.0000012', '0.0000012'],
    ['-0.005', '-0.005'],
    // null/空串/纯空白 → 显式缺失，不转 0
    ['', '缺失'],
    ['   ', '缺失'],
    [null, '缺失'],
    [undefined, '缺失'],
    // 非法数值保留原样（不 NaN/Infinity、不猜单位）
    ['n/a', 'n/a'],
    ['<0.01', '<0.01'],
    // 大数安全原样（不科学计数化）
    ['123456789012345', '123456789012345'],
    // 常规两位内已精确 → 原样
    ['4.25', '4.25'],
    ['3600', '3600'],
  ])('formatReadingValue(%p) → %p', (raw, want) => {
    expect(formatReadingValue(raw as string | null | undefined)).toBe(want)
  })
})

// ── 分源时效：schema3 直读 data_freshness；schema2 从 appendix 派生并标 derived ──

function payloadWith(overrides: {
  schemaVersion?: number
  freshness?: SignalReportPayload['data_freshness']
  calls?: SignalAppendixCall[]
}): SignalReportPayload {
  return {
    schema_version: overrides.schemaVersion ?? 2,
    signal_snapshot: { cutoff: '2026-09-22T08:00:00Z' } as SignalReportPayload['signal_snapshot'],
    report: { title: 't', sections: [], charts: [] },
    appendix: { calls: overrides.calls ?? [], calculations: [], gaps: [] },
    generation_meta: { cutoff: '2026-09-22T08:00:00Z' } as SignalReportPayload['generation_meta'],
    data_freshness: overrides.freshness,
  }
}

describe('buildSourceFreshness（分源时效，cutoff 不混 created_at）', () => {
  it('schema3 直读 data_freshness，工具名升序稳定', () => {
    const summary = buildSourceFreshness(
      payloadWith({
        schemaVersion: 3,
        freshness: {
          sources: [
            { tool: 'yahoo_finance', latest: '2026-10-01' },
            { tool: 'eia_wpsr', latest: '2026-07' },
          ],
          cutoff: '2026-09-22T08:00:00Z',
          generated_at: '2026-10-02T03:00:00Z',
          has_observations: true,
        },
      }),
    )
    expect(summary.derived).toBe(false)
    expect(summary.sources.map((s) => s.tool)).toEqual(['eia_wpsr', 'yahoo_finance'])
    expect(summary.sources.map((s) => s.latest)).toEqual(['2026-07', '2026-10-01'])
    expect(summary.cutoff).toBe('2026-09-22')
    expect(summary.generatedAt).toBe('2026-10-02')
    expect(summary.hasObservations).toBe(true)
  })

  it('schema2 从 appendix 派生各源最大期并标 derived（日/月频度各自保留）', () => {
    const call = (tool: string, periods: (string | undefined)[]): SignalAppendixCall => ({
      call_id: `c_${tool}`,
      question: 'q',
      tool,
      status: 'ok',
      observations: periods.map((p) => ({
        observation_id: `o_${p}`,
        period: p,
        value: p ? 1 : null,
      })),
    })
    const summary = buildSourceFreshness(
      payloadWith({
        schemaVersion: 2,
        calls: [
          call('eia_wpsr', ['2026-05', '2026-07']),
          call('yahoo_finance', ['2026-10-01', '2026-09-28']),
          call('dead_source', [undefined]),
        ],
      }),
    )
    expect(summary.derived).toBe(true)
    expect(summary.sources).toEqual([
      { tool: 'eia_wpsr', latest: '2026-07' },
      { tool: 'yahoo_finance', latest: '2026-10-01' },
    ])
    expect(summary.generatedAt).toBeNull()
  })

  it('零观测如实空列表，不用 cutoff 冒充数据期', () => {
    const summary = buildSourceFreshness(payloadWith({ schemaVersion: 3, freshness: { sources: [], cutoff: '2026-09-22T08:00:00Z', generated_at: '2026-10-02T03:00:00Z', has_observations: false } }))
    expect(summary.sources).toEqual([])
    expect(summary.hasObservations).toBe(false)
    const derived = buildSourceFreshness(payloadWith({ schemaVersion: 2, calls: [] }))
    expect(derived.sources).toEqual([])
    expect(derived.hasObservations).toBe(false)
  })
})

// ── 证据表行：refs 解析为阅读值；缺失/悬空如实标注 ──────────────────────────

const testCalls: SignalAppendixCall[] = [
  {
    call_id: 'c1',
    question: '库存走势?',
    tool: 'eia_wpsr',
    status: 'ok',
    observations: [
      { observation_id: 'c1:o1', period: '2026-08-01', value: 100.19999694824219, unit: 'MMbbl', label: '库存' },
      { observation_id: 'c1:o2', period: '2026-08-02', value: null, unit: 'MMbbl', label: '库存' },
    ],
  },
]
const testCalcs: SignalCalculation[] = [
  {
    calc_id: 'k1',
    op: 'difference',
    inputs: ['c1:o1'],
    expression: 'c1:o1',
    value: '-1.00',
    unit: 'MMbbl',
    precision: 2,
    status: 'ok',
    computed_at: '2026-09-22T08:00:00Z',
  },
]

describe('buildEvidenceTableRows（schema3 证据表渲染）', () => {
  it('data/calc 引用解析为阅读值 + 原值锚点；null 显示缺失', () => {
    const rows = buildEvidenceTableRows(
      {
        type: 'evidence_table',
        title: '关键观测',
        rows: [
          { label: '库存两期', refs: ['c1:o1', 'c1:o2'] },
          { label: '代码计算', refs: ['k1'] },
        ],
      },
      testCalls,
      testCalcs,
    )
    expect(rows).not.toBeNull()
    expect(rows?.length).toBe(2)
    expect(rows?.[0]?.cells[0]).toMatchObject({ display: '100.2', unit: 'MMbbl', clickable: true })
    expect(rows?.[0]?.cells[1]?.display).toBe('缺失')
    expect(rows?.[0]?.hasMissing).toBe(true)
    expect(rows?.[1]?.cells[0]).toMatchObject({ display: '-1', unit: 'MMbbl', clickable: true })
  })

  it('非 evidence_table 块返回 null；悬空引用显示缺失不伪造', () => {
    expect(buildEvidenceTableRows({ type: 'paragraph', text: 'x' }, testCalls, testCalcs)).toBeNull()
    const rows = buildEvidenceTableRows(
      { type: 'evidence_table', title: 't', rows: [{ label: '幽灵', refs: ['c9:o9'] }] },
      testCalls,
      testCalcs,
    )
    expect(rows?.[0]?.cells[0]?.display).toBe('缺失')
    expect(rows?.[0]?.cells[0]?.clickable).toBe(false)
  })
})

// ── 图表真实日期布局：不等距三日 + 显式 null + 连续性降离散 ─────────────────

function point(period: string, value: number | null): SignalChartPoint {
  return { ref: period, period, value, display: value === null ? '缺失' : String(value), unit: 'u', origin: 'data', seriesLabel: 's' }
}

describe('buildChartLayout（真实日期距离与 null 断点）', () => {
  it('10/7、10/8、10/14 的 x 间距为 1:6，不按数组位置假装等时距', () => {
    const layout = buildChartLayout([point('2026-10-07', 1), point('2026-10-08', 2), point('2026-10-14', 3)], 'line')
    expect(layout.mode).toBe('line')
    expect(layout.x[1]! - layout.x[0]!).toBeCloseTo(1 / 7, 6)
    expect(layout.x[2]! - layout.x[0]!).toBeCloseTo(1, 6)
  })

  it('显式 null 保持断点（y=null 不补零），不造中间数据', () => {
    const layout = buildChartLayout(
      [point('2026-10-07', 1), point('2026-10-08', null), point('2026-10-09', 3)],
      'line',
    )
    expect(layout.y[1]).toBeNull()
    expect(layout.observationCount).toBe(2)
    expect(layout.mode).toBe('line')
  })

  it('连续性不明（最大间隔 >10× 最小间隔）降离散点并标观测数', () => {
    const layout = buildChartLayout(
      [point('2026-01-01', 1), point('2026-01-02', 2), point('2026-06-01', 3)],
      'line',
    )
    expect(layout.mode).toBe('discrete')
    expect(layout.observationCount).toBe(3)
  })

  it('comparison 柱从 0 起（y 域含 0）；期间不可解析退化为序列位置', () => {
    const bars = buildChartLayout([point('a', 4), point('b', 2)], 'comparison')
    expect(bars.yMin).toBe(0)
    expect(bars.yMax).toBe(4)
    expect(bars.y[0]).toBe(1)
    expect(bars.y[1]).toBe(0.5)
    const fallback = buildChartLayout([point('x期', 1), point('y期', 2)], 'line')
    expect(fallback.x).toEqual([0, 1])
  })

  it('periodTimestamp 接受日/月/年三档频度', () => {
    expect(periodTimestamp('2026-10-01')).toBe(Date.UTC(2026, 9, 1))
    expect(periodTimestamp('2026-10')).toBe(Date.UTC(2026, 9, 15))
    expect(periodTimestamp('2026')).toBe(Date.UTC(2026, 6, 1))
    expect(periodTimestamp('2026年10月')).toBeNull()
  })
})

// SourceFreshnessSummary 类型消费锚（保持导出形状稳定）。
describe('SourceFreshnessSummary shape anchor', () => {
  it('derived/cutoff/generatedAt 字段在 summary 上可读', () => {
    const summary: SourceFreshnessSummary = buildSourceFreshness(payloadWith({ schemaVersion: 2, calls: [] }))
    expect(summary).toHaveProperty('derived')
    expect(summary).toHaveProperty('cutoff')
    expect(summary).toHaveProperty('generatedAt')
  })
})
