<script setup lang="ts">
/**
 * 「能力路由」section 挂载壳（ai-provider-model-overhaul 4.3）。
 * 取数/加载/错误/保存状态已全部内聚到 AIRouterCapabilityRoutes
 * （useAIRoutes/useAIModels 自取），本壳只做 section 装配。
 */
import AIRouterCapabilityRoutes from '~/features/ai/components/AIRouterCapabilityRoutes.vue'
</script>

<template>
  <AIRouterCapabilityRoutes />
</template>
