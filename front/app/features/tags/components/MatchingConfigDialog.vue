<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import KaTeXRender from '~/components/KaTeXRender.vue'
import type { MatchingConfig } from '~/api/semanticBoards'

const props = defineProps<{
  visible: boolean
  config: MatchingConfig | null
  loading: boolean
  /** 保存中（防重复提交）。 */
  saving?: boolean
  /** 保存失败信息（父层 API 结果回传；有值时保留输入并提示重试）。 */
  saveError?: string | null
}>()

const emit = defineEmits<{
  save: [data: Partial<MatchingConfig>]
  cancel: []
}>()

const show = computed({
  get: () => props.visible,
  set: (val: boolean) => { if (!val) emit('cancel') }
})

const form = ref<Partial<MatchingConfig>>({})

/** 候选泳道权重的文档默认值（缺项/旧后端未下发时回退）。 */
const LANE_HEAT_CANDIDATE_WEIGHT_DEFAULT = 0.5

watch([() => props.visible, () => props.config], ([v, c]) => {
  if (v && c) {
    form.value = {
      ...c,
      // 缺项回退默认（不把 undefined 留在输入框里，也不伪造 0）。
      lane_heat_candidate_weight: c.lane_heat_candidate_weight ?? LANE_HEAT_CANDIDATE_WEIGHT_DEFAULT,
    }
  }
})

/**
 * 数值字段归一：空串 / null / NaN / Infinity 一律视为非法（不是 0）。
 * 相关事件阈值是「相似度」而非身份置信度，范围固定 [0,1]（design D6）。
 */
function toFiniteNumber(value: unknown): number | null {
  if (value === '' || value === null || value === undefined) return null
  const parsed = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

const relatedThresholdError = computed(() => {
  const parsed = toFiniteNumber(form.value.related_event_similarity_threshold)
  if (parsed === null) return '请填写 0 到 1 之间的数值'
  if (parsed < 0 || parsed > 1) return '取值范围 0 ~ 1'
  return ''
})

/** 候选泳道权重：默认 0.5、域 [0,1] 有限数；与相关事件阈值各自独立校验（互不影响）。 */
const laneHeatWeightError = computed(() => {
  const parsed = toFiniteNumber(form.value.lane_heat_candidate_weight)
  if (parsed === null) return '请填写 0 到 1 之间的数值'
  if (parsed < 0 || parsed > 1) return '取值范围 0 ~ 1'
  return ''
})

const hasFormError = computed(() => Boolean(relatedThresholdError.value) || Boolean(laneHeatWeightError.value))

function handleSave() {
  // 非法输入整体拒绝：不发 PUT，旧值不被覆盖（后端同样按整体拒绝处理）。
  if (hasFormError.value) return
  emit('save', {
    ...form.value,
    related_event_similarity_threshold: toFiniteNumber(form.value.related_event_similarity_threshold) ?? undefined,
    lane_heat_candidate_weight: toFiniteNumber(form.value.lane_heat_candidate_weight) ?? undefined,
  })
}
</script>

<template>
  <AppDialog v-model="show" title="参数配置" size="lg">
    <div v-if="loading" class="loading-text">加载中...</div>

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
            <AppInput v-model="form.semantic_board_match_sim_threshold" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">命中率分母下限 N<sub>eff</sub></span>
            <span class="mc-hint">标签辅助锚点数少于此值时，以此值做分母。默认 3，防止只有 1 个锚点时命中率虚高 100%</span>
            <AppInput v-model="form.semantic_board_match_min_effective_sample" type="number" step="1" min="1" max="10" />
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
            <AppInput v-model="form.semantic_board_match_direct_hit_min_overlap" type="number" step="1" min="1" max="10" />
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
            <AppInput v-model="form.semantic_board_match_direct_hit_rate" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">相似度混合权重 α</span>
            <span class="mc-hint">分数中 maxSim 的占比。0.7 = 七成看最相似锚点、三成看整体命中率。1.0 = 纯看最高相似度</span>
            <AppInput v-model="form.semantic_board_match_hit_rate_sim_blend" type="number" step="0.05" min="0" max="1" />
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
            <AppInput v-model="form.semantic_board_match_direct_max_sim" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">最小命中数 K</span>
            <span class="mc-hint">至少几个锚点命中（≥ θ<sub>sim</sub>）。默认 2，防止单个偶然高相似度导致误匹配</span>
            <AppInput v-model="form.semantic_board_match_direct_max_sim_min_hits" type="number" step="1" min="1" max="5" />
          </label>
          <label class="mc-field">
            <span class="mc-label">最小命中率 ρ</span>
            <span class="mc-hint">命中率也必须达到此值。0.3 = 确保有足够密度而非孤立一个高分</span>
            <AppInput v-model="form.semantic_board_match_direct_max_sim_min_hit_rate" type="number" step="0.05" min="0" max="1" />
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
            <AppInput v-model="form.semantic_board_match_weight_sim" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">密度权重 w<sub>den</sub></span>
            <span class="mc-hint">加权公式中 hitRate 的占比。0.4 = 命中率占 40%</span>
            <AppInput v-model="form.semantic_board_match_weight_density" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">综合阈值 θ<sub>w</sub></span>
            <span class="mc-hint">加权得分 ≥ 此值才判定匹配。0.6 = 综合分六成才算数</span>
            <AppInput v-model="form.semantic_board_match_weighted_threshold" type="number" step="0.01" min="0" max="1" />
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
            <AppInput v-model="form.semantic_board_match_direction_sim_threshold" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">最大归属板块数</span>
            <span class="mc-hint">一个标签最多挂几个板块。默认 3，避免一个标签到处都是</span>
            <AppInput v-model="form.semantic_board_match_max_boards" type="number" min="1" max="10" />
          </label>
        </div>
      </div>

      <!-- ===== 相关事件（独立于入板规则：按事件语义推荐，不合并事件） ===== -->
      <div class="mc-rule-block" data-testid="mc-related-events">
        <div class="mc-rule-title">
          <span class="mc-rule-badge mc-rule-badge--related">相关</span>
          <span>相关事件</span>
        </div>
        <p class="mc-rule-desc">按事件名称、描述与辅助标签的语义相似度，推荐同版块「值得一起看」的其他事件。只是相关推荐，不合并事件、不判定同一事实，也不参与入板命中顺序。</p>
        <div class="mc-grid">
          <label class="mc-field">
            <span class="mc-label">相关事件相似度阈值</span>
            <span class="mc-hint">两条事件描述的余弦相似度达到此值才列为相关。默认 0.86；调高更严格、调低更宽松，改动后无需重算向量，下次展开即生效</span>
            <AppInput
              v-model="form.related_event_similarity_threshold"
              type="number"
              step="0.01"
              min="0"
              max="1"
              :error="relatedThresholdError"
              data-testid="mc-related-threshold"
            />
          </label>
        </div>
      </div>

      <!-- ===== 当天线索热度（lane-heat-metric D8：只进热度，不进生命周期） ===== -->
      <div class="mc-rule-block" data-testid="mc-lane-heat">
        <div class="mc-rule-title">
          <span class="mc-rule-badge mc-rule-badge--related">热度</span>
          <span>当天线索热度</span>
        </div>
        <p class="mc-rule-desc">版块关注度 = 各泳道当天线索数按话题状态加权求和；候选泳道按此权重计入。只影响热度与雷达分档，不参与入板匹配，也不影响话题生命周期（candidate 转正门槛不变）。</p>
        <div class="mc-grid">
          <label class="mc-field">
            <span class="mc-label">候选泳道权重</span>
            <span class="mc-hint">默认 0.5；0 = 候选不计入版块关注度，1 = 与 active 泳道同权。取值范围 0 ~ 1</span>
            <AppInput
              v-model="form.lane_heat_candidate_weight"
              type="number"
              step="0.05"
              min="0"
              max="1"
              :error="laneHeatWeightError"
              data-testid="mc-lane-heat-weight"
            />
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
            <AppInput v-model="form.semantic_board_upgrade_ref_count_threshold" type="number" min="1" max="100" />
          </label>
          <label class="mc-field">
            <span class="mc-label">聚类距离阈值</span>
            <span class="mc-hint">向量余弦距离小于此值的候选归为同一簇。0.35 严格（"富途"和"老虎"分开），0.7 宽松</span>
            <AppInput v-model="form.semantic_board_upgrade_cluster_distance_threshold" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">共现事件窗口（天）</span>
            <span class="mc-hint">分析候选标签关联事件时往回看多少天。30 天适合追踪近期热点</span>
            <AppInput v-model="form.semantic_board_upgrade_cotag_window_days" type="number" min="1" max="365" />
          </label>
          <label class="mc-field">
            <span class="mc-label">共现事件 Top N</span>
            <span class="mc-hint">每个簇最多取多少个关联事件作为 LLM 上下文</span>
            <AppInput v-model="form.semantic_board_upgrade_cotag_top_n" type="number" min="1" max="100" />
          </label>
          <label class="mc-field">
            <span class="mc-label">共现去重相似度</span>
            <span class="mc-hint">关联事件之间相似度超过此值就合并，避免重复占位</span>
            <AppInput v-model="form.semantic_board_upgrade_cotag_dedupe_sim_threshold" type="number" step="0.01" min="0" max="1" />
          </label>
          <label class="mc-field">
            <span class="mc-label">共现事件硬上限</span>
            <span class="mc-hint">每个簇最终送 LLM 的事件数量上限，兜底防止 prompt 过长</span>
            <AppInput v-model="form.semantic_board_upgrade_cotag_hard_limit" type="number" min="1" max="50" />
          </label>
          <label class="mc-field">
            <span class="mc-label">聚类算法</span>
            <span class="mc-hint">average_link: 候选需与簇内真实成员 pairwise 接近（推荐，消除枢纽效应）；centroid: 质心阈值聚类（旧算法，仅回退用）</span>
            <select v-model="form.semantic_board_upgrade_cluster_method" class="native-select">
              <option value="average_link">average_link（推荐）</option>
              <option value="centroid">centroid（旧算法）</option>
            </select>
          </label>
        </div>
      </div>
    </div>

    <template #footer>
      <span v-if="saveError" class="mc-save-error" data-testid="mc-save-error">{{ saveError }}（输入已保留，可重试）</span>
      <AppButton variant="ghost" size="sm" @click="emit('cancel')">取消</AppButton>
      <AppButton
        variant="primary"
        size="sm"
        :loading="saving"
        :disabled="hasFormError || saving"
        @click="handleSave"
      >保存</AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.loading-text {
  text-align: center;
  padding: 2rem 0;
  color: var(--color-text-muted);
  font-size: 0.8rem;
}

.mc-body {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.mc-desc {
  font-size: 0.72rem;
  color: var(--color-text-muted);
  line-height: 1.6;
  margin-bottom: 0.75rem;
}

.mc-desc strong {
  color: var(--color-text-secondary);
}

.mc-save-error {
  margin-right: auto;
  font-size: 0.72rem;
  color: var(--color-danger, #dc2626);
}

.mc-rule-block {
  margin-top: 0.85rem;
  padding: 0.7rem 0.85rem;
  border-radius: 0.65rem;
  border: 1px solid var(--color-border-subtle);
  background: var(--color-bg-hover);
}

.mc-rule-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--color-text-primary);
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

.mc-rule-badge--related {
  background: rgba(120, 200, 120, 0.12);
  color: rgba(140, 220, 140, 0.85);
}

.mc-rule-badge--upgrade {
  background: rgba(60, 200, 200, 0.12);
  color: rgba(100, 220, 220, 0.85);
}

.mc-rule-desc {
  font-size: 0.67rem;
  color: var(--color-text-muted);
  line-height: 1.5;
  margin-bottom: 0.45rem;
}

.mc-formula {
  background: var(--color-bg-sunken);
  border-radius: 6px;
  padding: 0.35rem 0.6rem;
  margin-bottom: 0.55rem;
  overflow-x: auto;
}

.mc-formula :deep(.katex-render) {
  font-size: 0.82rem;
  color: var(--color-text-primary);
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
  color: var(--color-text-secondary);
  letter-spacing: 0.02em;
}

.mc-hint {
  font-size: 0.65rem;
  color: var(--color-text-muted);
  line-height: 1.4;
}

.native-select {
  width: 100%;
  padding: 0.5rem 0.8rem;
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font-size: 0.82rem;
  outline: none;
  box-sizing: border-box;
}

.native-select:focus {
  border-color: var(--color-input-focus);
}
</style>
