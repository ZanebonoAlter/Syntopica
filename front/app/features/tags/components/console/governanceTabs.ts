/** 治理抽屉页签（右轨 / 抽屉 / 壳层共用的唯一枚举源）。 */
export type GovernanceTab = 'quality' | 'composites' | 'candidates' | 'sources' | 'settings'

export const GOVERNANCE_TABS: Array<{ key: GovernanceTab, label: string }> = [
  { key: 'quality', label: '构成' },
  { key: 'composites', label: '组合' },
  { key: 'candidates', label: '候选' },
  { key: 'sources', label: '来源' },
  { key: 'settings', label: '设置' },
]
