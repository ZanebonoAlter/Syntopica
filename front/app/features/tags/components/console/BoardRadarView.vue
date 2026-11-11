<script setup lang="ts">
/**
 * 今日雷达总览（rework-workshop-into-radar-and-desk 8.1/8.3 + 8A.1 已批准报刊式总览）。
 *
 * L0：/tags 未选版块时的跨版块决策面。本组件为总览壳：数据/派生/导航在
 * useBoardRadarOverview（API 边界已归一 camelCase/字符串 ID），报头、故事行、
 * 常温区、阅读右栏、治理宿主各自拆分为子组件。分档按版块关注度（Σ 泳道当天热度）
 * 的 P80/P20 阈值（见 boardRadarHelpers）；矩阵/小矩阵/3 日窗口统一为当日线索合计，
 * 事件线量与颜色通道已退役（retire-story-adopt-event-metrics D4/D5/D10）。
 * 首次取数失败不把初始化 0 当业务结论（R-17），有旧数据时刷新失败保留旧值。
 * 右栏契约：一块「当天线索最多的泳道」清单（服务端归并/排序/截断，上限 8）+
 * partial/滞后诚实标记；旧后端缺该段时回退版块级日报标题列表。
 * 管线状态块默认折叠、异常自展开。
 */
import { onMounted, ref } from 'vue'
import { Icon } from '@iconify/vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import { useBoardRadarOverview } from '~/features/tags/composables/useBoardRadarOverview'
import BoardRadarMasthead from './BoardRadarMasthead.vue'
import BoardRadarStory from './BoardRadarStory.vue'
import BoardRadarCold from './BoardRadarCold.vue'
import BoardRadarReadingRail from './BoardRadarReadingRail.vue'
import BoardRadarStatusBlock from './BoardRadarStatusBlock.vue'
import BoardRadarGovernance, { type RadarAdvancedKind } from './BoardRadarGovernance.vue'

const {
  rows,
  date,
  loading,
  error,
  hotRows,
  coldRows,
  visibleRows,
  todayRead,
  hasLaneFeed,
  laneItems,
  partialLanes,
  degradedBoardCount,
  laggingBoardCount,
  todayClueTotal,
  firstLoadFailed,
  subKind,
  dateText,
  headline,
  miniRange,
  load,
  openBoard,
  openReport,
  openLane,
} = useBoardRadarOverview()

const showAddBoard = ref(false)
const governanceRef = ref<InstanceType<typeof BoardRadarGovernance> | null>(null)

onMounted(() => { void load() })

function handleAdvanced(kind: RadarAdvancedKind): void {
  governanceRef.value?.openAdvanced(kind)
}

/** 新建版块成功回程：先重拉总览再进新版块（与旧实现顺序一致）。 */
async function handleBoardCreated(boardId: number): Promise<void> {
  await load()
  openBoard(String(boardId))
}

/** 手动日报生成终态：刷新雷达总览，让「最新日报落点 / 今日已出」跟上新数据。 */
function handleGenerated(): void {
  void load()
}
</script>

<template>
  <AppPageShell mode="workspace" :gutter="0" as="div">
    <div class="radar" data-testid="board-radar">
      <BoardRadarMasthead
        :date-text="dateText"
        :headline="headline"
        :sub="subKind"
        :hot-count="hotRows.length"
        :rows-count="rows.length"
        :today-clue-total="todayClueTotal"
        :stats-visible="!firstLoadFailed"
        :creating-board="governanceRef?.creatingBoard ?? false"
        @advanced="handleAdvanced"
        @add-board="showAddBoard = true"
      />

      <div v-if="error" class="rd-error" role="alert">
        <Icon icon="mdi:alert-circle-outline" width="16" />
        <span>{{ error }}</span>
        <button type="button" data-testid="radar-retry" @click="load">重试</button>
      </div>

      <div class="rd-overview">
        <main class="rd-stage" aria-label="板块横向比较">
          <div v-if="loading" class="rd-skeleton" data-testid="radar-loading">
            <div v-for="i in 4" :key="i" class="rd-sk-row" />
          </div>

          <template v-else>
            <div v-if="rows.length" class="rd-labels">
              <span>序</span>
              <span>关注板块 · 最新日报</span>
              <span class="rd-mini-caption">
                <span>泳道活跃 · 每格线索数（未知格标「线索数未知」）</span>
                <span>{{ miniRange }}</span>
              </span>
              <span>关注强度</span>
              <span>3 日变化</span>
              <span>进入</span>
            </div>

            <BoardRadarStory
              v-for="(row, index) in visibleRows"
              :key="row.boardId"
              :row="row"
              :rank="index + 1"
              :global-date="date"
              @open-board="openBoard"
              @open-report="openReport"
              @open-lane="openLane"
            />

            <BoardRadarCold
              v-if="coldRows.length"
              :cold-rows="coldRows"
              :rank-offset="visibleRows.length"
              :global-date="date"
              @open-board="openBoard"
              @open-report="openReport"
              @open-lane="openLane"
            />

            <!-- R-17：业务空态只在成功取数且空集时出现，首次失败不算「没有板块」 -->
            <div v-if="!error && !rows.length" class="rd-empty" data-testid="radar-empty">
              <Icon icon="mdi:white-balance-sunny" width="26" />
              <p>今天没有需要看的版块。</p>
              <small>日报生成后，这里会按关注度自动排序。</small>
              <button type="button" class="rd-empty-add" data-testid="radar-empty-add" @click="showAddBoard = true">
                <Icon icon="mdi:plus" width="14" />
                新建第一个版块
              </button>
            </div>
          </template>
        </main>

        <!-- 右栏改「上滚下驻」两段容器（add-radar-pipeline-status D9）：
             上段 = 当天线索最多的泳道清单（独立滚动），下段 = 管线状态块（常驻底部）。
             R-17：首次取数失败（从未成功拿到数据）不渲染阅读右栏内容，避免「今天没有可排的泳道」
             等业务空结论与失败态混排；有旧数据时可保留，成功空集照旧。
             状态块始终常驻（它自己用「读取中 / 读取失败」表达未知态，不产出业务结论）。 -->
        <div class="rd-rail-col">
          <div class="rd-rail-scroll">
            <BoardRadarReadingRail
              v-if="!firstLoadFailed"
              :lane-mode="hasLaneFeed"
              :lane-items="laneItems"
              :partial="partialLanes"
              :degraded-count="degradedBoardCount"
              :lagging-count="laggingBoardCount"
              :today-read="todayRead"
              @open-lane="openLane"
              @open-report="openReport"
            />
          </div>
          <BoardRadarStatusBlock
            :rows="rows"
            :overview-loading="loading"
            :overview-error="error"
            @generated="handleGenerated"
          />
        </div>
      </div>

      <BoardRadarGovernance
        ref="governanceRef"
        v-model:add-board-visible="showAddBoard"
        @board-created="handleBoardCreated"
      />
    </div>
  </AppPageShell>
</template>

<style scoped>
.radar {
  /* R-21：总览沿 workspace 可用宽度（不再压 120rem 居中，2560 不再星 1920 阅读列），
     两侧 gutter 由下方 padding 提供；不改全局 shell。 */
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 0;
  height: 100dvh;
  min-height: 0;
  overflow: hidden;
  padding: 1.4rem clamp(1rem, 3vw, 2.4rem) 1.2rem;
  box-sizing: border-box;
  color: var(--color-text-primary);
}

/* ── 主舞台 + 阅读右栏 ────────────────────────────────────────────────── */
.rd-overview {
  display: grid;
  grid-template-columns: minmax(0, 1fr) clamp(17rem, 22vw, 29rem);
  gap: 2rem;
  flex: 1 1 auto;
  min-height: 0;
  padding-top: 0.4rem;
}

.rd-stage {
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding-right: 0.4rem;
  scrollbar-width: thin;
}

/* ── 右栏两段容器：上段独立滚动 + 下段状态块常驻底部 ─────────────────── */
.rd-rail-col {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  min-width: 0;
  min-height: 0;
}

.rd-rail-scroll {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  scrollbar-width: thin;
}

/* 表头列宽仅限定 .rd-labels（本组件自有元素）：scoped 样式会命中子组件根节点，
 * 若用 .rd-grid 会覆盖 BoardRadarStory 根的移动端列定义（R-20 回归根源）。
 * 与 BoardRadarStory 的 `.rd-grid` 行列宽同步（改断点两处一起改）。 */
.rd-labels {
  display: grid;
  grid-template-columns: 2.2rem minmax(13rem, 1.25fr) minmax(14rem, 1.1fr) 3.8rem 6.6rem minmax(7rem, max-content);
  gap: clamp(0.9rem, 1.1vw, 1.5rem);
  align-items: center;
  padding: 0.55rem 0.2rem 0.5rem;
  border-bottom: 1px solid var(--color-border-medium);
  color: var(--color-text-muted);
  font-size: 0.64rem;
}

.rd-mini-caption {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  min-width: 0;
}

.rd-labels > span:nth-child(4),
.rd-labels > span:nth-child(5) {
  text-align: center;
}

.rd-skeleton {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 0.7rem;
}

.rd-sk-row {
  height: 5.4rem;
  border-radius: 6px;
  background: var(--color-bg-hover);
  animation: rd-pulse 1.2s ease-in-out infinite alternate;
}

@keyframes rd-pulse {
  from { opacity: 0.55; }
  to { opacity: 1; }
}

/* ── 错误条 / 空态 ────────────────────────────────────────────────────── */
.rd-error {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex: 0 0 auto;
  margin-top: 0.75rem;
  padding: 0.55rem 0.7rem;
  border: 1px solid var(--color-danger);
  border-radius: 6px;
  color: var(--color-danger);
  font-size: 0.76rem;
}

.rd-error button {
  margin-left: auto;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  text-decoration: underline;
  cursor: pointer;
}

.rd-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.4rem;
  padding: 5rem 1rem;
  color: var(--color-text-muted);
  text-align: center;
}

.rd-empty p {
  margin: 0;
  color: var(--color-text-primary);
  font-family: var(--font-serif, serif);
  font-size: 1.1rem;
}

.rd-empty small {
  font-size: 0.72rem;
}

.rd-empty-add {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  margin-top: 0.55rem;
  padding: 0.42rem 0.7rem;
  border: 1px solid var(--color-border-medium);
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font: inherit;
  font-size: 0.74rem;
  cursor: pointer;
}

.rd-empty-add:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

/* ── <1440 桌面：页面滚动，隐藏小矩阵列 ───────────────────────────────── */
@media (min-width: 768px) and (max-width: 1439.98px) {
  .radar {
    height: auto;
    min-height: 100dvh;
    overflow: visible;
  }

  .rd-overview {
    display: block;
    min-height: 40rem;
  }

  .rd-stage {
    overflow: visible;
    padding-right: 0;
  }

  /* 页面整体滚动：右栏不再是「内部滚动」布局，两段都随页面走 */
  .rd-rail-col {
    display: block;
  }

  .rd-rail-scroll {
    overflow: visible;
  }

  .rd-labels {
    grid-template-columns: 1.4rem minmax(0, 1fr) 3.4rem 5.4rem minmax(7rem, max-content);
    gap: 0.7rem;
  }

  .rd-labels .rd-mini-caption {
    display: none;
  }
}

/* ── ≤767 窄屏：单列 ──────────────────────────────────────────────────── */
@media (max-width: 767.98px) {
  .radar {
    height: auto;
    min-height: 100dvh;
    overflow: visible;
    padding: 1rem 0.9rem 2rem;
  }

  .rd-overview {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    min-height: 0;
  }

  .rd-stage {
    overflow: visible;
    padding-right: 0;
  }

  .rd-rail-col {
    display: block;
  }

  .rd-rail-scroll {
    overflow: visible;
  }

  .rd-labels {
    display: none;
  }
}
</style>
