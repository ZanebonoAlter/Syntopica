/**
 * 设置工作台旧深链重定向映射的单一来源（restructure-settings-navigation D2 /
 * switch-margin-note-search-to-deepseek design D6）。
 *
 * 消费方两处：`SettingsWorkspace`（section 级 + tab 级两级重定向）与
 * `SettingsSectionDatasourcesNetwork`（子 tab 级归一）。映射只在这里写一次——
 * 旧键下线时改一处即可，避免两边字面拷贝各自漂移。
 */
import type { SectionKey } from './components/SettingsWorkspace.vue'

/** 旧 section 键 → 复合 section 键 + tab：外部书签与 onboarding 深链不死链。 */
export const LEGACY_SECTION_REDIRECT: Record<string, { section: SectionKey, tab: string }> = {
  firecrawl: { section: 'datasources-network', tab: 'firecrawl' },
  bocha: { section: 'datasources-network', tab: 'bocha' },
  // SearXNG 面板已下线（switch-margin-note-search-to-deepseek D6）：旧键落到「网页搜索」
  searxng: { section: 'datasources-network', tab: 'web-search' },
  rsshub: { section: 'datasources-network', tab: 'rsshub' },
  datasources: { section: 'datasources-network', tab: 'datasources' },
  proxy: { section: 'datasources-network', tab: 'proxy' },
  'ai-health': { section: 'runtime-status', tab: 'ai-health' },
  queues: { section: 'runtime-status', tab: 'queues' },
  schedulers: { section: 'runtime-status', tab: 'schedulers' },
}

/** 旧子 tab 键 → 新 tab 键：`searxng` 面板下线后旧深链落到「网页搜索」而非默认 Firecrawl。 */
export const LEGACY_TAB_REDIRECT: Record<string, string> = {
  searxng: 'web-search',
}
