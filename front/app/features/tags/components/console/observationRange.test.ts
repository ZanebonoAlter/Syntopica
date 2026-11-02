/**
 * 观察窗纯函数测试（矩阵阶段 G1，test-cases-matrix-stage.md OX-3/4/5/6 + 边界值清单）。
 *
 *  - OX-3 校验四类：空串/纯空白 → 缺项；2026-10-6（缺零）/2026/10/06/不存在的日历日 → 格式；
 *    start>end → 顺序；池外 → 越界；每类文案互不相同且不静默改期。
 *  - OX-4 池边界：池首/池尾含端点合法，越界一日拒绝。
 *  - OX-5 平移/缩放：结果钳制在池内、宽度 ≥1。
 *  - OX-6 键盘：逐日/逐周/到端点，不越过对端。
 *  - 档值解析：仅 7/14/30/365；8/abc/0/-1 拒绝（归一提示由状态层负责）。
 */
import { describe, expect, it } from 'vitest'
import {
  DEFAULT_SOURCE_DAYS,
  POOL_DAYS,
  SOURCE_TIER_DAYS,
  clampRangeToBounds,
  daysBetweenInclusive,
  defaultRangeForTier,
  edgeToBound,
  parseStrictDate,
  parseTierDaysValue,
  sameRange,
  shiftDate,
  shiftRange,
  stepRangeEdge,
  validateObservationRange,
  zoomRange,
} from './observationRange'

/** 样本池：锚点 2026-10-23，池 = [2025-10-24, 2026-10-23]（365 日含两端）。 */
const BOUNDS = { start: '2025-10-24', end: '2026-10-23' }

describe('observationRange — 严格日期', () => {
  it('parseStrictDate：合法日期原样返回；缺零/斜杠/非日历日/空白拒绝（OX-3）', () => {
    expect(parseStrictDate('2026-10-06')).toBe('2026-10-06')
    expect(parseStrictDate(' 2026-10-06 ')).toBe('2026-10-06')
    expect(parseStrictDate('2026-10-6')).toBeNull()
    expect(parseStrictDate('2026/10/06')).toBeNull()
    expect(parseStrictDate('2026-02-30')).toBeNull()
    expect(parseStrictDate('2026-13-01')).toBeNull()
    expect(parseStrictDate('')).toBeNull()
    expect(parseStrictDate('   ')).toBeNull()
    // 闰年真实日历日
    expect(parseStrictDate('2028-02-29')).toBe('2028-02-29')
    expect(parseStrictDate('2026-02-29')).toBeNull()
  })

  it('shiftDate/daysBetweenInclusive：UTC 数值日算术、含两端宽度', () => {
    expect(shiftDate('2026-10-23', -13)).toBe('2026-10-10')
    expect(shiftDate('2026-10-01', -1)).toBe('2026-09-30')
    expect(shiftDate('2025-03-01', -1)).toBe('2025-02-28')
    expect(daysBetweenInclusive('2026-10-10', '2026-10-23')).toBe(14)
    expect(daysBetweenInclusive('2026-10-23', '2026-10-23')).toBe(1)
  })

  it('defaultRangeForTier：档默认区间 = [anchor-days+1, anchor]（含两端）', () => {
    expect(defaultRangeForTier('2026-10-23', 14)).toEqual({ start: '2026-10-10', end: '2026-10-23' })
    expect(defaultRangeForTier('2026-10-23', 7)).toEqual({ start: '2026-10-17', end: '2026-10-23' })
    expect(defaultRangeForTier('2026-10-23', 1)).toEqual({ start: '2026-10-23', end: '2026-10-23' })
  })
})

describe('observationRange — validateObservationRange（OX-3/OX-4）', () => {
  it('缺项：空串/纯空白/非字符串 → missing 文案', () => {
    for (const [start, end] of [['', '2026-10-20'], ['2026-10-01', '   '], ['  ', ''], [undefined, '2026-10-20']] as const) {
      const result = validateObservationRange(start, end, BOUNDS)
      expect(result.ok).toBe(false)
      if (!result.ok) expect(result.issue.code).toBe('missing')
    }
  })

  it('格式：缺零/斜杠/不存在的日历日 → format 文案', () => {
    for (const [start, end] of [['2026-10-6', '2026-10-20'], ['2026/10/06', '2026-10-20'], ['2026-02-30', '2026-10-20'], ['2026-10-01', '2026-2-30']]) {
      const result = validateObservationRange(start, end, BOUNDS)
      expect(result.ok).toBe(false)
      if (!result.ok) expect(result.issue.code).toBe('format')
    }
  })

  it('顺序：start > end → order 文案', () => {
    const result = validateObservationRange('2026-10-21', '2026-10-20', BOUNDS)
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.issue.code).toBe('order')
  })

  it('池外：越界一日拒绝并给出池范围文案；池首/池尾含端点合法（OX-4）', () => {
    const tooEarly = validateObservationRange('2025-10-23', '2026-10-20', BOUNDS)
    expect(tooEarly.ok).toBe(false)
    if (!tooEarly.ok) {
      expect(tooEarly.issue.code).toBe('out-of-pool')
      expect(tooEarly.issue.message).toContain('2025-10-24 ~ 2026-10-23')
    }
    expect(validateObservationRange('2026-10-22', '2026-10-24', BOUNDS).ok).toBe(false)
    expect(validateObservationRange(BOUNDS.start, BOUNDS.end, BOUNDS).ok).toBe(true)
  })

  it('合法：单日/2日/跨月/全池 365 日均通过（变体 2/边界值）', () => {
    expect(validateObservationRange('2026-10-23', '2026-10-23', BOUNDS).ok).toBe(true)
    expect(validateObservationRange('2026-10-22', '2026-10-23', BOUNDS).ok).toBe(true)
    expect(validateObservationRange('2026-09-28', '2026-10-23', BOUNDS).ok).toBe(true)
    expect(validateObservationRange(BOUNDS.start, BOUNDS.end, BOUNDS).ok).toBe(true)
    // 观察宽度 366（越出池）拒绝
    const wide = validateObservationRange('2025-10-23', '2026-10-23', BOUNDS)
    expect(wide.ok).toBe(false)
  })

  it('恰好 7/14/30/365 日宽度与任何宽度同等校验：本模块不携带档位语义（OX-1 结构性）', () => {
    for (const width of [7, 14, 30, 365, 10, 21]) {
      const start = shiftDate('2026-10-23', -(width - 1))
      const result = validateObservationRange(start, '2026-10-23', BOUNDS)
      expect(result.ok).toBe(true)
      if (result.ok) expect(daysBetweenInclusive(result.range.start, result.range.end)).toBe(width)
    }
  })
})

describe('observationRange — 平移/缩放/键盘（OX-5/OX-6）', () => {
  it('shiftRange：池内平移保持宽度；池首/池尾方向越界时钳制（range: shift clamp）', () => {
    const range = { start: '2026-10-10', end: '2026-10-23' }
    expect(shiftRange(range, -5, BOUNDS)).toEqual({ start: '2026-10-05', end: '2026-10-18' })
    // 拉到池首之外：钳制为 [池首, 池首+13]
    expect(shiftRange(range, -400, BOUNDS)).toEqual({ start: '2025-10-24', end: '2025-11-06' })
    // 推到池尾之外：钳制为 [池尾-13, 池尾]
    expect(shiftRange(range, 400, BOUNDS)).toEqual({ start: '2026-10-10', end: '2026-10-23' })
  })

  it('zoomRange：0.5×/2× 围绕中心，宽度钳制 [1, 池宽]（range: zoom clamp）', () => {
    const range = { start: '2026-10-10', end: '2026-10-23' } // 14 日
    const narrowed = zoomRange(range, 0.5, BOUNDS)
    expect(daysBetweenInclusive(narrowed.start, narrowed.end)).toBe(7)
    const widened = zoomRange(range, 2, BOUNDS)
    expect(daysBetweenInclusive(widened.start, widened.end)).toBe(28)
    // 单日缩 0.5× 不低于 1
    expect(daysBetweenInclusive(zoomRange({ start: '2026-10-23', end: '2026-10-23' }, 0.5, BOUNDS).start, '2026-10-23')).toBe(1)
    // 超池宽钳制到整池
    const full = zoomRange(range, 100, BOUNDS)
    expect(full).toEqual(BOUNDS)
  })

  it('stepRangeEdge：起/止端逐日（Shift 逐周）、不越过对端、池内钳制（range: keyboard bounds）', () => {
    const range = { start: '2026-10-10', end: '2026-10-13' }
    expect(stepRangeEdge(range, 'start', -1, BOUNDS).start).toBe('2026-10-09')
    expect(stepRangeEdge(range, 'start', -7, BOUNDS).start).toBe('2026-10-03')
    // 起端推进不越过止端
    expect(stepRangeEdge(range, 'start', 30, BOUNDS)).toEqual({ start: '2026-10-13', end: '2026-10-13' })
    // 止端回退不越过起端
    expect(stepRangeEdge(range, 'end', -30, BOUNDS)).toEqual({ start: '2026-10-10', end: '2026-10-10' })
    // 止端受池尾钳制
    expect(stepRangeEdge({ start: '2026-10-20', end: '2026-10-23' }, 'end', 5, BOUNDS).end).toBe('2026-10-23')
    // 起端受池首钳制
    expect(stepRangeEdge({ start: '2025-10-24', end: '2025-10-30' }, 'start', -5, BOUNDS).start).toBe('2025-10-24')
  })

  it('edgeToBound：Home/End 到池首/池尾，不越过对端', () => {
    const range = { start: '2026-10-10', end: '2026-10-13' }
    expect(edgeToBound(range, 'start', 'start', BOUNDS)).toEqual({ start: '2025-10-24', end: '2026-10-13' })
    expect(edgeToBound(range, 'end', 'end', BOUNDS)).toEqual({ start: '2026-10-10', end: '2026-10-23' })
    // 起端送池尾时不得越过止端（收敛到止端）
    expect(edgeToBound(range, 'start', 'end', BOUNDS)).toEqual({ start: '2026-10-13', end: '2026-10-13' })
  })

  it('clampRangeToBounds：宽度超池取整池；池内越界平移回池', () => {
    expect(clampRangeToBounds({ start: '2025-01-01', end: '2026-12-31' }, BOUNDS)).toEqual(BOUNDS)
    expect(clampRangeToBounds({ start: '2026-10-20', end: '2026-11-20' }, BOUNDS))
      .toEqual({ start: '2026-09-22', end: '2026-10-23' }) // 宽度 32 日，尾部锚池尾回推
  })

  it('sameRange：逐字段比较，null 安全', () => {
    expect(sameRange({ start: 'a', end: 'b' }, { start: 'a', end: 'b' })).toBe(true)
    expect(sameRange({ start: 'a', end: 'b' }, { start: 'a', end: 'c' })).toBe(false)
    expect(sameRange(null, { start: 'a', end: 'b' })).toBe(false)
    expect(sameRange(null, null)).toBe(false)
  })
})

describe('observationRange — 档值与常量', () => {
  it('parseTierDaysValue：仅 7/14/30/365；8/abc/0/-1/带空白拒绝（变体 3）', () => {
    expect(parseTierDaysValue('7')).toBe(7)
    expect(parseTierDaysValue('14')).toBe(14)
    expect(parseTierDaysValue('30')).toBe(30)
    expect(parseTierDaysValue('365')).toBe(365)
    expect(parseTierDaysValue('8')).toBeNull()
    expect(parseTierDaysValue('abc')).toBeNull()
    expect(parseTierDaysValue('0')).toBeNull()
    expect(parseTierDaysValue('-1')).toBeNull()
    // 与 parseLaneParam 同口径：带空白宽容 trim 后接受；前导零按数字归一
    expect(parseTierDaysValue(' 7 ')).toBe(7)
    expect(parseTierDaysValue('07')).toBe(7)
  })

  it('常量：默认档 14、池 365、四档枚举（与 useBoardConsole 既有口径一致）', () => {
    expect(DEFAULT_SOURCE_DAYS).toBe(14)
    expect(POOL_DAYS).toBe(365)
    expect([...SOURCE_TIER_DAYS]).toEqual([7, 14, 30, 365])
  })
})
