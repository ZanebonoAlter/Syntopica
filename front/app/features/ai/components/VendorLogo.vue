<script setup lang="ts">
/**
 * 厂商 logo（FB-2）：优先引用 vendor-icons.svg symbol（presetId→symbolId 映射
 * 见 presets.ts logo 字段），无 symbol 的厂商首字母回退。品牌色来自 symbol 内
 * 固定 fill，不随主题反转。
 */
const props = withDefaults(defineProps<{
  /** spritesheet symbol id（如 'deepseek'）；空值走首字母回退 */
  symbolId?: string | null
  /** 回退取首字母的显示名 */
  name: string
  size?: number
}>(), { symbolId: null, size: 16 })

const initial = computed(() => (props.name || '?').trim().charAt(0).toUpperCase() || '?')
</script>

<template>
  <span class="vendor-logo" :style="{ width: `${size + 8}px`, height: `${size + 8}px` }">
    <svg v-if="symbolId" :width="size" :height="size" aria-hidden="true">
      <use :href="`#${symbolId}`" />
    </svg>
    <template v-else>{{ initial }}</template>
  </span>
</template>

<style scoped>
.vendor-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  border-radius: 6px;
  background: var(--color-bg-sunken, #eee);
  color: var(--color-text-secondary, #666);
  font-size: 11px;
  font-weight: 700;
  overflow: hidden;
}
</style>

<style>
/* FB-9：pi 品牌图标为白底设计（固定 fill 深色为主），暗色主题下容器改浅底保对比度。
   .vendor-logo 仅本组件使用，全局块无冲突风险。 */
html[data-theme='dark'] .vendor-logo {
  background: #f2efe9;
  color: #444;
}
</style>
