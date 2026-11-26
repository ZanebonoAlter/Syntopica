<script setup lang="ts">
/**
 * 趋势图共享外置轴（rework-workshop-into-radar-and-desk 矩阵阶段 G2）。
 *
 * 批准原型 matrix-graphics.js `trendFrame` 的产品化：Y 刻度（事件线数）与 X 日期刻度
 * 全部放在**绘图区 SVG 之外**，保证指针→日期映射只消费绘图区 rect（不被轴占据的像素
 * 挤歪）。ObservationRangeControls 的 brush 背景曲线与 LaneTrendChart 共用本组件，
 * 两处「外置日期/事件线数轴」视觉与口径一致。
 *
 * 纯展示：刻度值/定位百分比由调用方（laneTrendSeries 纯函数）算好传入，本组件不算业务。
 */
withDefaults(defineProps<{
  /** Y 轴单位标签（默认事件线数——曲线与矩阵格同口径：事件线数 > 0 用事件线数，否则线索合计兜底）。 */
  unitLabel?: string
  /** Y 刻度：value + 顶部定位百分比（0=轴底、100=轴顶）。空数组 = 全未知（不伪造量级）。 */
  yTicks?: { value: number, topPercent: number }[]
  /** X 日期刻度：文本 + 左侧定位百分比 + 对齐（首端 start / 中段 center / 末端 end）。 */
  xTicks?: { label: string, leftPercent: number, align: 'start' | 'center' | 'end' }[]
}>(), {
  unitLabel: '事件线数（条）',
  yTicks: () => [],
  xTicks: () => [],
})
</script>

<template>
  <div class="ta" data-testid="trend-axes">
    <div class="ta-unit">{{ unitLabel }}</div>
    <div class="ta-chart">
      <div class="ta-y" aria-hidden="true" data-testid="trend-axis-y">
        <span
          v-for="tick in yTicks"
          :key="`y-${tick.value}`"
          class="ta-y-tick"
          :style="{ top: `${tick.topPercent}%` }"
        >{{ tick.value }}</span>
      </div>
      <div class="ta-plot-column">
        <slot />
        <div class="ta-x" aria-hidden="true" data-testid="trend-axis-x">
          <span
            v-for="(tick, index) in xTicks"
            :key="`x-${index}`"
            class="ta-x-tick"
            :class="`ta-x-tick--${tick.align}`"
            :style="{ left: `${tick.leftPercent}%` }"
          >{{ tick.label }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ta {
  min-width: 0;
}

.ta-unit {
  font-size: 11px;
  color: var(--color-text-muted);
  margin: 5px 0;
}

.ta-chart {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  gap: 8px;
  padding-right: 2px;
}

.ta-plot-column {
  min-width: 0;
}

/* 轴高与两个宿主 SVG 的显示高（88px）一致，刻度才贴齐基线 */
.ta-y {
  position: relative;
  height: 88px;
  border-right: 1px solid var(--color-border-medium);
  font-size: 11px;
  font-family: var(--font-mono, monospace);
  color: var(--color-text-muted);
}

.ta-y-tick {
  position: absolute;
  right: 5px;
  transform: translateY(-50%);
  white-space: nowrap;
}

.ta-x {
  position: relative;
  height: 25px;
  margin-top: 2px;
  font-size: 11px;
  font-family: var(--font-mono, monospace);
  color: var(--color-text-muted);
}

.ta-x-tick {
  position: absolute;
  top: 2px;
  white-space: nowrap;
}

.ta-x-tick--start {
  transform: none;
}

.ta-x-tick--center {
  transform: translateX(-50%);
}

.ta-x-tick--end {
  transform: translateX(-100%);
}

@media (max-width: 767.98px) {
  .ta-chart {
    grid-template-columns: 26px minmax(0, 1fr);
    gap: 6px;
  }

  .ta-x {
    font-size: 10px;
  }
}
</style>
