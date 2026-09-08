<script setup lang="ts">
import SettingsWorkspace from '~/features/settings/components/SettingsWorkspace.vue'
import SettingsSectionFeeds from '~/features/settings/components/SettingsSectionFeeds.vue'
import SettingsSectionAiProviders from '~/features/settings/components/SettingsSectionAiProviders.vue'
import SettingsSectionCapabilityRoutes from '~/features/settings/components/SettingsSectionCapabilityRoutes.vue'
import SettingsSectionAiHealth from '~/features/settings/components/SettingsSectionAiHealth.vue'
import SettingsSectionQueues from '~/features/settings/components/SettingsSectionQueues.vue'
import SettingsSectionPreferences from '~/features/settings/components/SettingsSectionPreferences.vue'
import SettingsSectionFirecrawl from '~/features/settings/components/SettingsSectionFirecrawl.vue'
import SettingsSectionBocha from '~/features/settings/components/SettingsSectionBocha.vue'
import SettingsSectionAnalysisMethods from '~/features/settings/components/SettingsSectionAnalysisMethods.vue'
import SettingsSectionRsshub from '~/features/settings/components/SettingsSectionRsshub.vue'
import SettingsSectionProxy from '~/features/settings/components/SettingsSectionProxy.vue'
import SettingsSectionSchedulers from '~/features/settings/components/SettingsSectionSchedulers.vue'

import type { SectionKey } from '~/features/settings/components/SettingsWorkspace.vue'

const sectionComponents: Record<SectionKey, any> = {
  'feeds': SettingsSectionFeeds,
  'ai-providers': SettingsSectionAiProviders,
  'capability-routes': SettingsSectionCapabilityRoutes,
  'ai-health': SettingsSectionAiHealth,
  'queues': SettingsSectionQueues,
  'preferences': SettingsSectionPreferences,
  'firecrawl': SettingsSectionFirecrawl,
  'bocha': SettingsSectionBocha,
  'analysis-methods': SettingsSectionAnalysisMethods,
  'rsshub': SettingsSectionRsshub,
  'proxy': SettingsSectionProxy,
  'schedulers': SettingsSectionSchedulers,
}
</script>

<template>
  <SettingsWorkspace>
    <template #default="{ section }">
      <component :is="sectionComponents[section]" :key="section" />
    </template>
  </SettingsWorkspace>
</template>
