<script setup lang="ts">
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type { MatchingConfig } from '~/api/semanticBoards'

const props = defineProps<{
  visible: boolean
  config: MatchingConfig | null
  loading: boolean
}>()

const emit = defineEmits<{
  save: [data: Partial<MatchingConfig>]
  cancel: []
}>()

const form = ref<Partial<MatchingConfig>>({})

watch([() => props.visible, () => props.config], ([v, c]) => {
  if (v && c) {
    form.value = { ...c }
  }
})

function handleSave() {
  emit('save', form.value)
}
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="mc-overlay" @click.self="emit('cancel')">
      <div class="mc-card">
        <div class="mc-header">
          <h3 class="mc-title">参数配置</h3>
          <button type="button" class="mc-close" @click="emit('cancel')">
            <Icon icon="mdi:close" width="18" />
          </button>
        </div>

        <div v-if="loading" class="mc-loading">加载中...</div>

        <div v-else class="mc-body">
          <p class="mc-desc">调整标签匹配和板块升级的行为参数。修改后对新处理的数据生效。</p>

          <div class="mc-section-title">标签 → 板块匹配</div>
          <div class="mc-grid">
            <label class="mc-field">
              <span class="mc-label">相似度阈值</span>
              <span class="mc-hint">向量最低相似度。比如 0.72 表示只有相似度 ≥ 0.72 的候选才进入后续计算</span>
              <input v-model.number="form.semantic_board_match_sim_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">直接命中率阈值</span>
              <span class="mc-hint">标签的辅助锚点命中板块组成的比例。比如 0.5 表示超过一半命中就直接挂载</span>
              <input v-model.number="form.semantic_board_match_direct_hit_rate" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">直接匹配最大相似度</span>
              <span class="mc-hint">最高向量相似度超过此值直接挂载，跳过加权计算。比如 0.8</span>
              <input v-model.number="form.semantic_board_match_direct_max_sim" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">相似度权重</span>
              <span class="mc-hint">加权公式中向量相似度的占比，与密度权重搭配使用，建议和为 1</span>
              <input v-model.number="form.semantic_board_match_weight_sim" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">密度权重</span>
              <span class="mc-hint">加权公式中命中率（密度）的占比。0.4 表示命中率占 40%</span>
              <input v-model.number="form.semantic_board_match_weight_density" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">加权综合阈值</span>
              <span class="mc-hint">加权得分 ≥ 此值才判定匹配。0.6 意味着综合分六成才算数</span>
              <input v-model.number="form.semantic_board_match_weighted_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">最大归属板块数</span>
              <span class="mc-hint">一个标签最多挂几个板块。默认 3，避免一个标签到处都是</span>
              <input v-model.number="form.semantic_board_match_max_boards" type="number" min="1" max="10" class="mc-input" />
            </label>
          </div>

          <div class="mc-section-title">板块升级建议</div>
          <div class="mc-grid">
            <label class="mc-field">
              <span class="mc-label">候选引用次数阈值</span>
              <span class="mc-hint">辅助标签被引用多少次才够格进入升级候选。比如 5 表示至少出现在 5 个标签里</span>
              <input v-model.number="form.semantic_board_upgrade_ref_count_threshold" type="number" min="1" max="100" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">聚类距离阈值</span>
              <span class="mc-hint">向量余弦距离小于此值的候选归为同一簇。0.35 比较严格，"富途证券"和"老虎证券"会分开；0.7 则会合并到一起</span>
              <input v-model.number="form.semantic_board_upgrade_cluster_distance_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">共现事件窗口（天）</span>
              <span class="mc-hint">分析候选标签的关联事件时，往回看多少天。30 天适合追踪近期热点</span>
              <input v-model.number="form.semantic_board_upgrade_cotag_window_days" type="number" min="1" max="365" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">共现事件 Top N</span>
              <span class="mc-hint">每个簇最多取多少个关联事件作为 LLM 上下文。太多 prompt 爆炸，太少缺信息</span>
              <input v-model.number="form.semantic_board_upgrade_cotag_top_n" type="number" min="1" max="100" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">共现去重相似度</span>
              <span class="mc-hint">关联事件之间相似度超过此值就合并，避免"伊朗袭击"和"伊朗导弹"重复占位</span>
              <input v-model.number="form.semantic_board_upgrade_cotag_dedupe_sim_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
            </label>
            <label class="mc-field">
              <span class="mc-label">共现事件硬上限</span>
              <span class="mc-hint">每个簇最终送 LLM 的事件数量上限，兜底防止 prompt 过长</span>
              <input v-model.number="form.semantic_board_upgrade_cotag_hard_limit" type="number" min="1" max="50" class="mc-input" />
            </label>
          </div>
        </div>

        <div class="mc-footer">
          <button type="button" class="mc-btn mc-btn--ghost" @click="emit('cancel')">取消</button>
          <button type="button" class="mc-btn mc-btn--primary" @click="handleSave">保存</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.mc-overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(8, 12, 18, 0.75);
  backdrop-filter: blur(8px);
}

.mc-card {
  width: min(680px, 92vw);
  max-height: 85vh;
  overflow-y: auto;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(17, 27, 38, 0.98);
  padding: 1.5rem;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
}

.mc-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1rem;
}

.mc-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.9);
}

.mc-desc {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.35);
  line-height: 1.5;
  margin-bottom: 0.75rem;
}

.mc-section-title {
  font-size: 0.72rem;
  font-weight: 600;
  color: rgba(240, 138, 75, 0.75);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-top: 0.5rem;
  margin-bottom: 0.6rem;
  padding-bottom: 0.35rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.mc-hint {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  line-height: 1.4;
}

.mc-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 8px;
  background: none;
  color: rgba(255, 255, 255, 0.4);
  cursor: pointer;
  transition: all 0.12s ease;
}

.mc-close:hover {
  background: rgba(255, 255, 255, 0.08);
  color: rgba(255, 255, 255, 0.7);
}

.mc-loading {
  text-align: center;
  padding: 2rem 0;
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.8rem;
}

.mc-body {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.mc-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.65rem 1.2rem;
}

@media (max-width: 500px) {
  .mc-grid {
    grid-template-columns: 1fr;
  }
}

.mc-field {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.mc-label {
  font-size: 0.72rem;
  color: rgba(255, 255, 255, 0.5);
  letter-spacing: 0.02em;
}

.mc-input {
  width: 100%;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: rgba(0, 0, 0, 0.25);
  color: rgba(255, 255, 255, 0.88);
  font-size: 0.82rem;
  padding: 0.5rem 0.8rem;
  outline: none;
  transition: border-color 0.12s ease;
  box-sizing: border-box;
}

.mc-input:focus {
  border-color: rgba(240, 138, 75, 0.45);
}

.mc-footer {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 1.25rem;
}

.mc-btn {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  background: none;
  color: rgba(255, 255, 255, 0.7);
  font-size: 0.82rem;
  padding: 0.45rem 1.1rem;
  cursor: pointer;
  transition: all 0.12s ease;
}

.mc-btn--ghost:hover {
  background: rgba(255, 255, 255, 0.06);
}

.mc-btn--primary {
  border-color: rgba(240, 138, 75, 0.4);
  color: rgba(255, 220, 200, 0.9);
  background: rgba(240, 138, 75, 0.12);
}

.mc-btn--primary:hover {
  background: rgba(240, 138, 75, 0.2);
  border-color: rgba(240, 138, 75, 0.6);
}
</style>
