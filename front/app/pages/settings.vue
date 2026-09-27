<script setup lang="ts">
// 设置工作区（restructure-settings-navigation 后）：
// 导航元数据与 section 组件映射收敛在 SettingsWorkspace 的 sections 单一来源（D3），
// 本页仅作路由挂载点。
import SettingsWorkspace from '~/features/settings/components/SettingsWorkspace.vue'
</script>

<template>
  <SettingsWorkspace />
</template>
