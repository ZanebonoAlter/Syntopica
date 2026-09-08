<script setup lang="ts">
import { computed, useSlots } from 'vue'
import { SHELL_MAX_WIDTH, type ShellMode } from './layout-contract'

interface Props {
  /** 布局模式：reader=760 居中 / contained=1120 居中 / workspace=全宽 / split=主从栏（见 standard/frontend/layout.md） */
  mode?: ShellMode
  /** 左右留白（px） */
  gutter?: number
  /** 主内容语义标签（默认 main；嵌套在既有 main 内时传 div/section） */
  as?: string
  /** split 侧栏最小宽度 */
  asideMin?: string
  /** split 侧栏位置 */
  asideSide?: 'left' | 'right'
}

const props = withDefaults(defineProps<Props>(), {
  mode: 'contained',
  gutter: 24,
  as: 'main',
  asideMin: '280px',
  asideSide: 'right',
})

const slots = useSlots()

const styleVars = computed(() => {
  const max = SHELL_MAX_WIDTH[props.mode]
  return {
    '--shell-max': max === null ? 'none' : `${max}px`,
    '--shell-gutter': `${props.gutter}px`,
    '--aside-min': props.asideMin,
  }
})
</script>

<template>
  <div class="app-page-shell" :data-shell-mode="mode" :style="styleVars">
    <div
      class="app-page-shell__inner"
      :class="[`is-${mode}`, asideSide === 'left' ? 'aside-left' : 'aside-right']"
    >
      <component :is="as" class="app-page-shell__main">
        <slot />
      </component>
      <aside v-if="mode === 'split' && slots.aside" class="app-page-shell__aside">
        <slot name="aside" />
      </aside>
    </div>
  </div>
</template>

<style scoped>
.app-page-shell {
  width: 100%;
  box-sizing: border-box;
  /* 小屏：内容即 100% − gutter×2；上限只在视口更宽时生效 */
  padding-inline: var(--shell-gutter, 24px);
}

.app-page-shell__inner {
  max-width: var(--shell-max, none);
  margin-inline: auto;
}

.app-page-shell__inner.is-split {
  display: flex;
  gap: var(--shell-gap, 24px);
  align-items: stretch;
}

.app-page-shell__inner.aside-left {
  flex-direction: row-reverse;
}

/* 主栏可收缩（flex 子项默认 min-width:auto 会顶破页面） */
.app-page-shell__main {
  flex: 1 1 auto;
  min-width: 0;
}

/* 侧栏：显式 min 宽 + 自身滚动溢出策略，超长内容不顶破页面 */
.app-page-shell__aside {
  flex: 0 0 auto;
  min-width: var(--aside-min, 280px);
  max-width: var(--aside-max, 460px);
  overflow: auto;
}
</style>
