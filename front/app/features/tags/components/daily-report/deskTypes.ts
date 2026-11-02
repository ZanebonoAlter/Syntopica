/** 阅读台右栏演进节点（14 天）。 */
export interface DeskDayNode {
  date: string
  label: string
  hasReport: boolean
  isToday: boolean
}

/** 分区索引：按质量区聚合的话题组。 */
export interface DeskZoneGroup {
  key: string
  label: string
  topics: import('./dailyReportMagazine').TopicGroup[]
}
