<script lang="ts">
/**
 * 吸顶标签列右缘渐隐的层数（**全仓唯一一处定义**，unify-lane-canvas-shell D4）。
 *
 * 编排画布的吸顶带下缘阴影与吸顶标签列右缘阴影共用这个级数：级数 + 递减步长只有
 * 这一份，两个 SFC 都从这里取，SHALL NOT 各自再写一个 6。
 */
export const STICKY_SHADOW_BANDS = 6

/**
 * 吸顶阴影第 `band` 级的透明度（第 1 级最深）。
 *
 * **与级数同属一处定义**：吸顶标签列右缘与吸顶带下缘两处阴影共用这个公式，
 * 否则两处“看起来一样”只能靠巧合（`workshop-lane-compose` 「吸顶标签列是两支画布共用的单元」）。
 */
export function stickyShadowOpacity(band: number): number {
  return 0.28 - (band - 1) * 0.045
}
</script>

<script setup lang="ts">
import { computed } from 'vue'

/**
 * 吸顶标签列单元（unify-lane-canvas-shell D4）：横向滚动时把一整列图元钉在画布最左。
 *
 * 组件负责的全部四件事（调用方只递交几何与是否溢出）：
 * 1. 整组**反向平移** `translate(scrollLeftLogical, 0)`——不溢出时**不下发** transform
 *    （不溢出时渲染必须与无吸顶实现逐像素一致）；
 * 2. 不透明底 `label-bg`（`--color-bg-base`）：盖住从标签列下方滑过的点与标题；
 * 3. 可选**提档底** `sticky-tint`（`--color-bg-elevated`）：与吸顶带同一档，两条吸顶轴
 *    看起来是一块「固定框」。**不透明**叠在 2 之上——elevated 在深色主题带 alpha，
 *    单独用它当唯一底色会透出滚动内容；
 * 4. 右缘 N 级渐隐阴影（每级 1px、透明度递减）：表达「这一列是层界，后面还有内容」。
 *
 * 槽位（默认 slot）= 列内图元（行名 / 计数 / 拖拽热区 / 只读梯子的泳道名）：
 * 它们一律按**内容坐标**书写，横向位移由外层整组一次性给足——图元自身不得再补一次
 * 平移（否则横向滚动后偏移一个滚动量，即「双重平移」）。
 *
 * `data-role` 钩子是既有视觉基准，**不得重命名**：
 * `row-label-group` / `row-label-bg` / `row-label-tint` / `row-label-shadow`。
 */
const props = withDefaults(defineProps<{
  /** 固定列宽（逻辑坐标 = 标签区宽度；两支画布各取自己的标签区宽，不强行统一）。 */
  width: number
  /** 是否真的横向溢出：为假时不下发 transform、不画底与阴影。 */
  overflowing: boolean
  /** 覆盖高度（逻辑坐标）。 */
  height: number
  /** 覆盖起点（内容坐标 y）。 */
  y: number
  /** 是否叠提档底色（默认不叠）。 */
  tint?: boolean
  /** 右缘渐隐级数（缺省取全仓唯一那份 `STICKY_SHADOW_BANDS`）。 */
  bands?: number
  /** 整组反向位移（逻辑坐标 = `scrollLeft / zoom`）。 */
  offset?: number
  /** 是否在列右缘画 1px 竖向分隔（让这列读作「钉住的标签栅」而不是一块色）。 */
  divider?: boolean
}>(), {
  tint: false,
  divider: false,
  bands: STICKY_SHADOW_BANDS,
  offset: 0,
})

const transform = computed(() => (props.overflowing ? `translate(${props.offset}, 0)` : undefined))
</script>

<template>
  <g class="lcc-canvas__row-label" data-role="row-label-group" :transform="transform">
    <!-- 不透明底：先把滑到下面的点与标题整片盖住，提档色才叠得上去 -->
    <rect
      v-if="overflowing"
      class="lcc-canvas__label-bg"
      data-role="row-label-bg"
      x="0"
      :y="y"
      :width="width"
      :height="height"
    />
    <rect
      v-if="overflowing && tint"
      class="lcc-canvas__sticky-tint"
      data-role="row-label-tint"
      x="0"
      :y="y"
      :width="width"
      :height="height"
    />
    <!-- 列内图元：按内容坐标书写，横向位移由本组统一承担 -->
    <slot />
    <!-- 右缘竖向分隔（1px）：与下方右缘渐隐阴影一起表达「这里是层界」 -->
    <line
      v-if="overflowing && divider"
      class="lcc-canvas__label-divider"
      data-role="row-label-divider"
      :x1="width"
      :x2="width"
      :y1="y"
      :y2="y + height"
    />
    <template v-if="overflowing">
      <rect
        v-for="band in bands"
        :key="`rail-shadow-${band}`"
        class="lcc-canvas__label-shadow"
        :data-role="band === 1 ? 'row-label-shadow' : undefined"
        :x="width + band - 1"
        :y="y"
        width="1"
        :height="height"
        :opacity="stickyShadowOpacity(band)"
      />
    </template>
  </g>
</template>

<style scoped>
/* 不透明底：盖住从标签列下方滑过的点与标题 */
.lcc-canvas__label-bg,
.lcc-canvas__label-shadow {
  fill: var(--color-bg-base);
}

/*
  提档底色（与吸顶带同一档）。
  必须不透明：叠在上面那层 bg-base 之上，elevated 的 alpha 不会透出滚动内容。
*/
.lcc-canvas__sticky-tint {
  fill: var(--color-bg-elevated);
}

/* 列右缘 1px 竖向分隔（层界表达；与阴影不重复，阴影是渐隐、它是硬线） */
.lcc-canvas__label-divider {
  stroke: var(--color-border-medium);
  stroke-width: 1;
}
</style>
