<script setup lang="ts">
/**
 * 时间视窗手柄命中层（矩阵阶段 G2，R37/R40）。
 *
 * 纯展示/事件：24 CSS px 命中区；<24px 间距合并，重叠处按真实指针解析最近可见端；
 * 单日同一 x 时按上下半区解析起/止。坐标以父绘图区 rect 为准，不另造映射。
 */
import { computed } from 'vue'
import { buildHitRegions, type HitRegion, type HitRegionKind } from './rangeBrush'

const props = defineProps<{
  plotWidth: number
  startFraction: number
  endFraction: number
}>()

const emit = defineEmits<{
  grab: [payload: { kind: HitRegionKind, event: PointerEvent }]
}>()

const regions = computed<HitRegion[]>(() => buildHitRegions(
  props.startFraction,
  props.endFraction,
  props.plotWidth,
))

function onHitPointerDown(event: PointerEvent, region: HitRegion): void {
  emit('grab', { kind: region.kind, event })
}
</script>

<template>
  <div
    v-for="(region, index) in regions"
    :key="`hit-${region.kind}-${index}`"
    class="orc-hit"
    :data-testid="`brush-hit-${region.kind}`"
    aria-hidden="true"
    :style="{ ...region.style, top: '0', height: '100%' }"
    @pointerdown.stop="onHitPointerDown($event, region)"
  />
</template>

<style scoped>
.orc-hit {
  position: absolute;
  z-index: 2;
  background: transparent;
  cursor: ew-resize;
  touch-action: none;
}
</style>
