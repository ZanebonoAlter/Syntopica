<script setup lang="ts">
import { Icon } from '@iconify/vue'

// 路由切换加载反馈（fix-spa-nav-loading-ux D1）：状态由 plugins/nav-loading.ts 写入，
// 导航超 250ms 未完成时显示，完成/失败/被新导航取代即消失（<250ms 快导航不出现）。
// 视觉与 app.vue 初始化加载态同构：mdi:loading 转圈 + accent 主题令牌。
const { visible } = useNavLoading()
</script>

<template>
  <div v-if="visible" class="nav-loading-overlay" role="status" aria-live="polite">
    <Icon
      icon="mdi:loading"
      width="48"
      height="48"
      class="animate-spin motion-reduce:animate-none"
      style="color: var(--color-accent)"
    />
  </div>
</template>

<style scoped>
.nav-loading-overlay {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  /* 不拦截点击：加载期间用户仍可反悔点别处（非整页遮罩，不挡侧边栏） */
  pointer-events: none;
  /* 低于 dialog 系与 bottombar(40)，高于内容 */
  z-index: 30;
}
</style>
