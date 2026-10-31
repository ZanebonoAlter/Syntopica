/**
 * 本地日期 key（`YYYY-MM-DD`）——「今日是否已出日报」判定的唯一来源
 * （add-radar-pipeline-status design D7）。
 *
 * 为什么不能用 `new Date().toISOString().slice(0, 10)`：
 * `toISOString()` 是 **UTC 截断**，在 UTC+8 的 00:00–08:00 会得到「昨天」，
 * 于是「今日日报已出」在每天早上 8 点前都会被误判成未出。既有
 * `DailyReportGenerateDialog.vue` 正犯此错，本 change 不复刻。
 *
 * 这里按**本地**年月日拼（`getFullYear/getMonth/getDate`），与后端
 * `time.ParseInLocation("2006-01-02", date, time.Local)` 的日期口径一致。
 */

/** 取本地日期 key（默认此刻）。纯函数：传入 Date 即得该时刻的本地日期。 */
export function todayKey(now: Date = new Date()): string {
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}
