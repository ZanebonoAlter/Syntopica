<script setup lang="ts">
/**
 * 厂商图标 spritesheet（FB-2，tasks 8.2）：pi-web public/provider-icons.svg 原样拷贝
 * （30 个品牌 symbol，多源 lobe-icons）。全局挂载一次（app.vue），VendorLogo 通过
 * <use href="#{symbolId}"> 引用；无对应 symbol 的厂商（baseten/meta/radius 等 5 家）
 * 由 VendorLogo 首字母回退。?raw 取原始文本内联，无网络请求。
 */
import rawSprite from '~/assets/vendor-icons.svg?raw'
</script>

<template>
  <svg width="0" height="0" style="position:absolute" aria-hidden="true" v-html="rawSprite" />
</template>
