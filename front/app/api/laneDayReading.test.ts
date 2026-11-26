/**
 * laneDayReading façade 测试（矩阵阶段 G4）。
 * 重点：reportIndexDaysFor 停更 lookback 覆盖算法（列表端点锚今天、lane 池锚 anchor，
 * 两者不同源；死写 365 会漏池首报告）。
 */
import { describe, expect, it } from 'vitest'
import { REPORT_INDEX_MIN_DAYS, reportIndexDaysFor } from './laneDayReading'

describe('reportIndexDaysFor — 报告列表 days 覆盖算法', () => {
  it('anchor = today：365 下限即池同宽', () => {
    // anchor 2026-10-24 → 池首 = anchor-364 = 2025-10-25 → 到今天含两端 365
    expect(reportIndexDaysFor('2025-10-25', '2026-10-24')).toBe(365)
  })

  it('停更板块：today 晚于 anchor 数日 → days = 365 + 差额（池首可被覆盖）', () => {
    // anchor 2026-10-24 → 池首 2025-10-25；today 2026-10-28 → 含两端 369 天
    expect(reportIndexDaysFor('2025-10-25', '2026-10-28')).toBe(369)
    // 长期停更 60 日（today 2026-12-23）：365 + 60
    expect(reportIndexDaysFor('2025-10-25', '2026-12-23')).toBe(425)
  })

  it('跨度不足 365 时仍取 365 下限（与池同宽，不缩水）', () => {
    expect(reportIndexDaysFor('2026-10-01', '2026-10-23')).toBe(REPORT_INDEX_MIN_DAYS)
  })

  it('异常输入（requiredStart 晚于 today）回退 365，不产生负数/NaN', () => {
    expect(reportIndexDaysFor('2026-11-01', '2026-10-23')).toBe(365)
  })

  it('端点含两端：恰跨年/闰年路径按真实日历日计算', () => {
    // 2024-02-29 → 2024-03-01 = 2 天（闰日真实日历日）
    expect(reportIndexDaysFor('2024-02-29', '2024-03-01')).toBe(365)
  })
})
