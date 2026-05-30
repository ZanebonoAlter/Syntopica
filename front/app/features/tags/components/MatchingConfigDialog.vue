<script setup lang="ts">
import { ref, watch } from 'vue'
import { Icon } from '@iconify/vue'
import KaTeXRender from '~/components/KaTeXRender.vue'
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
          <p class="mc-desc">匹配按优先级依次尝试：<strong>direct_hit</strong> → <strong>hit_rate</strong> → <strong>max_sim</strong> → <strong>weighted</strong>。命中即停，后续规则不再计算。</p>

          <!-- ===== 公共基础 ===== -->
          <div class="mc-rule-block">
            <div class="mc-rule-title">
              <span class="mc-rule-badge mc-rule-badge--common">基础</span>
              <span>公共计算层</span>
            </div>
            <p class="mc-rule-desc">所有规则共享的基础指标：命中率 (hitRate) 和最大相似度 (maxSim)。</p>
            <div class="mc-formula">
              <KaTeXRender latex="\text{hitRate} = \frac{|\{\,t_i : \max_j \cos(t_i,\, b_j) \geq \theta_{\text{sim}}\,\}|}{\max(N_{\text{tag}},\; N_{\text{eff}})}" display />
              <KaTeXRender latex="\text{maxSim} = \max_{i,j}\;\cos(t_i,\, b_j)" display />
            </div>
            <div class="mc-grid">
              <label class="mc-field">
                <span class="mc-label">相似度阈值 θ<sub>sim</sub></span>
                <span class="mc-hint">辅助标签向量与板块辅助标签向量的最低相似度，达标才算"命中"。0.72 意味着只有 ≥ 0.72 的 pair 计入 hitRate 的分子</span>
                <input v-model.number="form.semantic_board_match_sim_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">命中率分母下限 N<sub>eff</sub></span>
                <span class="mc-hint">标签辅助锚点数少于此值时，以此值做分母。默认 3，防止只有 1 个锚点时命中率虚高 100%</span>
                <input v-model.number="form.semantic_board_match_min_effective_sample" type="number" step="1" min="1" max="10" class="mc-input" />
              </label>
            </div>
          </div>

          <!-- ===== Rule 1: direct_hit ===== -->
          <div class="mc-rule-block">
            <div class="mc-rule-title">
              <span class="mc-rule-badge mc-rule-badge--r1">①</span>
              <span>direct_hit — 精确重叠</span>
            </div>
            <p class="mc-rule-desc">标签与板块的辅助锚点有精确 ID 重叠时直接挂载，score 固定 1.0，跳过向量计算。</p>
            <div class="mc-formula">
              <KaTeXRender latex="|\text{tag\_aux} \cap \text{board\_aux}| \geq N_{\text{overlap}} \;\Longrightarrow\; \text{matched},\;\text{score}=1" display />
            </div>
            <div class="mc-grid">
              <label class="mc-field">
                <span class="mc-label">最小重叠数 N<sub>overlap</sub></span>
                <span class="mc-hint">至少几个辅助锚点 ID 完全一致才算命中。默认 2，防止偶然单个重叠导致误匹配</span>
                <input v-model.number="form.semantic_board_match_direct_hit_min_overlap" type="number" step="1" min="1" max="10" class="mc-input" />
              </label>
            </div>
          </div>

          <!-- ===== Rule 2: hit_rate ===== -->
          <div class="mc-rule-block">
            <div class="mc-rule-title">
              <span class="mc-rule-badge mc-rule-badge--r2">②</span>
              <span>hit_rate — 命中率达标</span>
            </div>
            <p class="mc-rule-desc">命中率超过阈值即匹配。分数由相似度和命中率混合加权。</p>
            <div class="mc-formula">
              <KaTeXRender latex="\text{hitRate} > \theta_{\text{hitRate}} \;\Longrightarrow\; \text{score} = \alpha \cdot \text{maxSim} + (1-\alpha) \cdot \text{hitRate}" display />
            </div>
            <div class="mc-grid">
              <label class="mc-field">
                <span class="mc-label">命中率阈值 θ<sub>hitRate</sub></span>
                <span class="mc-hint">超过此比例的辅助锚点命中即触发。0.5 = 过半数命中</span>
                <input v-model.number="form.semantic_board_match_direct_hit_rate" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">相似度混合权重 α</span>
                <span class="mc-hint">分数中 maxSim 的占比。0.7 = 七成看最相似锚点、三成看整体命中率。1.0 = 纯看最高相似度</span>
                <input v-model.number="form.semantic_board_match_hit_rate_sim_blend" type="number" step="0.05" min="0" max="1" class="mc-input" />
              </label>
            </div>
          </div>

          <!-- ===== Rule 3: max_sim ===== -->
          <div class="mc-rule-block">
            <div class="mc-rule-title">
              <span class="mc-rule-badge mc-rule-badge--r3">③</span>
              <span>max_sim — 最高相似度达标</span>
            </div>
            <p class="mc-rule-desc">存在至少一个辅助锚点对相似度极高，且有一定密度保障时，直接取最高相似度作为分数。</p>
            <div class="mc-formula">
              <KaTeXRender latex="\text{maxSim} \geq \theta_{\text{maxSim}} \;\wedge\; \text{hits} \geq K \;\wedge\; \text{hitRate} \geq \rho \;\Longrightarrow\; \text{score} = \text{maxSim}" display />
            </div>
            <div class="mc-grid">
              <label class="mc-field">
                <span class="mc-label">最大相似度阈值 θ<sub>maxSim</sub></span>
                <span class="mc-hint">最相似的锚点对超过此值才考虑。0.8 = 至少有一对非常相似的锚点</span>
                <input v-model.number="form.semantic_board_match_direct_max_sim" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">最小命中数 K</span>
                <span class="mc-hint">至少几个锚点命中（≥ θ<sub>sim</sub>）。默认 2，防止单个偶然高相似度导致误匹配</span>
                <input v-model.number="form.semantic_board_match_direct_max_sim_min_hits" type="number" step="1" min="1" max="5" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">最小命中率 ρ</span>
                <span class="mc-hint">命中率也必须达到此值。0.3 = 确保有足够密度而非孤立一个高分</span>
                <input v-model.number="form.semantic_board_match_direct_max_sim_min_hit_rate" type="number" step="0.05" min="0" max="1" class="mc-input" />
              </label>
            </div>
          </div>

          <!-- ===== Rule 4: weighted ===== -->
          <div class="mc-rule-block">
            <div class="mc-rule-title">
              <span class="mc-rule-badge mc-rule-badge--r4">④</span>
              <span>weighted — 加权综合</span>
            </div>
            <p class="mc-rule-desc">前三个规则都没命中时，用相似度和命中率的加权综合分兜底。</p>
            <div class="mc-formula">
              <KaTeXRender latex="w_{\text{sim}} \cdot \text{maxSim} + w_{\text{den}} \cdot \text{hitRate} \geq \theta_w \;\Longrightarrow\; \text{score} = w_{\text{sim}} \cdot \text{maxSim} + w_{\text{den}} \cdot \text{hitRate}" display />
            </div>
            <div class="mc-grid">
              <label class="mc-field">
                <span class="mc-label">相似度权重 w<sub>sim</sub></span>
                <span class="mc-hint">加权公式中 maxSim 的占比。与密度权重搭配，建议和为 1</span>
                <input v-model.number="form.semantic_board_match_weight_sim" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">密度权重 w<sub>den</sub></span>
                <span class="mc-hint">加权公式中 hitRate 的占比。0.4 = 命中率占 40%</span>
                <input v-model.number="form.semantic_board_match_weight_density" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">综合阈值 θ<sub>w</sub></span>
                <span class="mc-hint">加权得分 ≥ 此值才判定匹配。0.6 = 综合分六成才算数</span>
                <input v-model.number="form.semantic_board_match_weighted_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
            </div>
          </div>

          <!-- ===== 后置: 方向校验 & 全局 ===== -->
          <div class="mc-rule-block">
            <div class="mc-rule-title">
              <span class="mc-rule-badge mc-rule-badge--post">后置</span>
              <span>方向校验 &amp; 全局限制</span>
            </div>
            <p class="mc-rule-desc">②③④ 规则命中后，还需通过方向校验（direct_hit 豁免）。此外限制每个标签最多挂载的板块数。</p>
            <div class="mc-formula">
              <KaTeXRender latex="\cos(\mathbf{e}_{\text{tag}},\;\mathbf{e}_{\text{board}}) < \theta_{\text{dir}} \;\Longrightarrow\; \text{direction\_mismatch} = \text{true}" display />
            </div>
            <div class="mc-grid">
              <label class="mc-field">
                <span class="mc-label">方向校准阈值 θ<sub>dir</sub></span>
                <span class="mc-hint">标签与板块 embedding 余弦相似度低于此值判定方向不符。默认 0.5，越低越宽松</span>
                <input v-model.number="form.semantic_board_match_direction_sim_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">最大归属板块数</span>
                <span class="mc-hint">一个标签最多挂几个板块。默认 3，避免一个标签到处都是</span>
                <input v-model.number="form.semantic_board_match_max_boards" type="number" min="1" max="10" class="mc-input" />
              </label>
            </div>
          </div>

          <!-- ===== 升级建议 ===== -->
          <div class="mc-rule-block">
            <div class="mc-rule-title">
              <span class="mc-rule-badge mc-rule-badge--upgrade">升级</span>
              <span>板块升级建议</span>
            </div>
            <p class="mc-rule-desc">未匹配的辅助锚点聚类后生成新板块建议。</p>
            <div class="mc-grid">
              <label class="mc-field">
                <span class="mc-label">候选引用次数阈值</span>
                <span class="mc-hint">辅助标签被引用多少次才够格进入升级候选。比如 5 = 至少出现在 5 个标签里</span>
                <input v-model.number="form.semantic_board_upgrade_ref_count_threshold" type="number" min="1" max="100" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">聚类距离阈值</span>
                <span class="mc-hint">向量余弦距离小于此值的候选归为同一簇。0.35 严格（"富途"和"老虎"分开），0.7 宽松</span>
                <input v-model.number="form.semantic_board_upgrade_cluster_distance_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">共现事件窗口（天）</span>
                <span class="mc-hint">分析候选标签关联事件时往回看多少天。30 天适合追踪近期热点</span>
                <input v-model.number="form.semantic_board_upgrade_cotag_window_days" type="number" min="1" max="365" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">共现事件 Top N</span>
                <span class="mc-hint">每个簇最多取多少个关联事件作为 LLM 上下文</span>
                <input v-model.number="form.semantic_board_upgrade_cotag_top_n" type="number" min="1" max="100" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">共现去重相似度</span>
                <span class="mc-hint">关联事件之间相似度超过此值就合并，避免重复占位</span>
                <input v-model.number="form.semantic_board_upgrade_cotag_dedupe_sim_threshold" type="number" step="0.01" min="0" max="1" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">共现事件硬上限</span>
                <span class="mc-hint">每个簇最终送 LLM 的事件数量上限，兜底防止 prompt 过长</span>
                <input v-model.number="form.semantic_board_upgrade_cotag_hard_limit" type="number" min="1" max="50" class="mc-input" />
              </label>
              <label class="mc-field">
                <span class="mc-label">聚类算法</span>
                <span class="mc-hint">average_link: 候选需与簇内真实成员 pairwise 接近（推荐，消除枢纽效应）；centroid: 质心阈值聚类（旧算法，仅回退用）</span>
                <select v-model="form.semantic_board_upgrade_cluster_method" class="mc-input">
                  <option value="average_link">average_link（推荐）</option>
                  <option value="centroid">centroid（旧算法）</option>
                </select>
              </label>
            </div>
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
  width: min(740px, 92vw);
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
  color: rgba(255, 255, 255, 0.4);
  line-height: 1.6;
  margin-bottom: 0.75rem;
}

.mc-desc strong {
  color: rgba(255, 255, 255, 0.65);
}

.mc-rule-block {
  margin-top: 0.85rem;
  padding: 0.7rem 0.85rem;
  border-radius: 0.65rem;
  border: 1px solid rgba(255, 255, 255, 0.05);
  background: rgba(255, 255, 255, 0.015);
}

.mc-rule-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.78rem;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.75);
  margin-bottom: 0.4rem;
}

.mc-rule-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 2.2rem;
  height: 1.35rem;
  padding: 0 0.4rem;
  border-radius: 4px;
  font-size: 0.6rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  flex-shrink: 0;
}

.mc-rule-badge--common {
  background: rgba(100, 160, 255, 0.12);
  color: rgba(130, 180, 255, 0.85);
}

.mc-rule-badge--r1 {
  background: rgba(80, 220, 120, 0.12);
  color: rgba(100, 230, 140, 0.85);
}

.mc-rule-badge--r2 {
  background: rgba(240, 180, 60, 0.12);
  color: rgba(250, 200, 80, 0.85);
}

.mc-rule-badge--r3 {
  background: rgba(220, 120, 60, 0.12);
  color: rgba(240, 150, 80, 0.85);
}

.mc-rule-badge--r4 {
  background: rgba(180, 100, 220, 0.12);
  color: rgba(200, 130, 240, 0.85);
}

.mc-rule-badge--post {
  background: rgba(255, 80, 80, 0.12);
  color: rgba(255, 120, 120, 0.85);
}

.mc-rule-badge--upgrade {
  background: rgba(60, 200, 200, 0.12);
  color: rgba(100, 220, 220, 0.85);
}

.mc-rule-desc {
  font-size: 0.67rem;
  color: rgba(255, 255, 255, 0.3);
  line-height: 1.5;
  margin-bottom: 0.45rem;
}

.mc-formula {
  background: rgba(0, 0, 0, 0.2);
  border-radius: 6px;
  padding: 0.35rem 0.6rem;
  margin-bottom: 0.55rem;
  overflow-x: auto;
}

.mc-formula :deep(.katex-render) {
  font-size: 0.82rem;
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

.mc-hint {
  font-size: 0.65rem;
  color: rgba(255, 255, 255, 0.3);
  line-height: 1.4;
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
