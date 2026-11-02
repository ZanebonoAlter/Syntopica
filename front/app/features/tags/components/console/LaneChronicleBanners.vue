<script setup lang="ts">
/**
 * LaneChronicleBanners — 编年状态横幅（rework-workshop-into-radar-and-desk 矩阵阶段 G4）。
 *
 * 分层（全部经父级 display gate 注入，旧日横幅不进新上下文，R24）：
 *  - stale：同上下文刷新失败保留旧数据可读（带失败原因）。
 *  - error：读取失败（列表/详情）+ 重试（失败不入缓存，非 force 即真重发）。
 *  - no-report：聚合已有分区但列表未命中 period_date —— 中性「未匹配到当日日报详情」
 *    + 刷新（R25：不直称业务无日报、不自动生成、不借别日）。
 *  - degraded：≥1 桶未命中详情（报告可能已更新）+ 折叠数 + 强制刷新。
 */
import { Icon } from '@iconify/vue'
import type { LaneDayCurrentReading } from '~/features/tags/composables/useLaneDayReading'

defineProps<{
  display: LaneDayCurrentReading | null
  staleNotice: boolean
  errorText: string | null
}>()

const emit = defineEmits<{
  /** 失败重试（非 force：失败不入缓存，重试即真重发）。 */
  retry: []
  /** 强制重读（绕过索引+详情成功缓存；「报告已更新/未匹配」刷新路径）。 */
  refresh: []
}>()
</script>

<template>
  <div v-if="staleNotice && display" class="lch-banner lch-banner--stale" data-testid="lane-chronicle-stale">
    刷新失败，当前展示上次数据{{ errorText ? `（${errorText}）` : '' }}
  </div>
  <template v-if="display">
    <div v-if="display.status === 'error'" class="lch-banner lch-banner--error" data-testid="lane-chronicle-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ display.error || '当日线索加载失败' }}</span>
      <button type="button" class="lch-link" data-testid="lane-chronicle-retry" @click="emit('retry')">重试</button>
    </div>
    <div v-else-if="display.status === 'no-report'" class="lch-banner" data-testid="lane-chronicle-no-report">
      未匹配到当日日报详情，暂只能显示聚合返回的标题。
      <button type="button" class="lch-link" data-testid="lane-chronicle-retry" @click="emit('refresh')">刷新</button>
    </div>
    <div
      v-else-if="display.status === 'degraded'"
      class="lch-banner lch-banner--degraded" data-testid="lane-chronicle-degraded"
    >
      {{ display.missingSectionCount }} 个主题桶的完整线索未读取（报告可能已更新）<template v-if="display.foldedTotal > 0">，另有 {{ display.foldedTotal }} 条折叠未读取</template>。
      <button type="button" class="lch-link" data-testid="lane-chronicle-refresh" @click="emit('refresh')">刷新</button>
    </div>
  </template>
</template>

<style scoped>
.lch-banner {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex-wrap: wrap;
  font-size: 0.72rem;
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
  color: var(--color-text-secondary);
}

.lch-banner--stale {
  background: var(--color-warning-subtle, var(--color-bg-hover));
}

.lch-banner--error {
  background: color-mix(in srgb, var(--color-error, #b3543e) 10%, transparent);
}

.lch-link {
  border: 1px solid var(--color-border-medium);
  border-radius: 6px;
  padding: 0.1rem 0.4rem;
  background: none;
  color: var(--color-accent);
  font-size: 0.7rem;
  cursor: pointer;
}
</style>
