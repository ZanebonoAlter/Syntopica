<script setup lang="ts">
/**
 * 当前项处理区（board-health-triage-advice seam C2 / tasks 3.2-3.3）。
 *
 * 右侧只展示**当前一项**：问题是什么 → 原描述/污染证据 → 可编辑新描述 → 影响范围 → 主动作与下一步。
 * 四种互斥形态（按优先级）：
 *  1. 已有持久任务 → 只读执行态（无 textarea）；
 *  2. 提交结果未知（无任务）→ 只读保留原确认内容，可同键重发 / 重新加载核对；
 *  3. 建议重写描述 → 编辑器（三规则就地反馈 + 共享范围 + 行内确认）；
 *  4. 建议摘除 → 摘除说明 + 行内确认（真实 DELETE 由 3.4 宿主注入）。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { AuxiliaryRepairContextView, AuxiliaryRepairJobView } from '~/api/normalizers/auxiliaryRepair'
import type { BoardHealthLabelSignalView, BoardHealthMagnetFlowView } from '~/api/normalizers/boardHealthTriage'
import BoardHealthRepairEditor from './BoardHealthRepairEditor.vue'
import BoardHealthRepairProgress from './BoardHealthRepairProgress.vue'
import { boardHealthMagnetComparisonText, boardHealthMagnetVerdictText, type BoardHealthIssueItem } from './boardHealthWorkspace'

const props = defineProps<{
  issue: BoardHealthIssueItem | null
  /** 该标签的信号证据（磁铁/最近邻/描述预筛）；null = 本轮无信号。 */
  signal: BoardHealthLabelSignalView | null
  /** 失败挂载主要流向（服务端已算好）；null = 未知/本轮无失败挂载。 */
  magnetFlow: BoardHealthMagnetFlowView | null
  context: AuxiliaryRepairContextView | null
  contextLoading: boolean
  contextError: string | null
  task: AuxiliaryRepairJobView | null
  taskError: string | null
  submitting: boolean
  retrying: boolean
  submitError: string | null
  /** 非 409 的提交失败（独立错误行；服务端 message 透传）。 */
  submitFailure: string | null
  conflict: boolean
  submitUncertain: boolean
  frozenDescription: string | null
  pollPaused: boolean
  draft: string
  validationError: string | null
  disabledReason: string | null
  /** 已被宿主确认真实摘除成功。 */
  removed: boolean
  narrow: boolean
  hasNext: boolean
  /** 描述修复的行内确认是否展开。 */
  confirmOpen: boolean
  /** 摘除的行内确认是否展开。 */
  removeConfirmOpen: boolean
}>()

const emit = defineEmits<{
  'update:draft': [value: string]
  'request-confirm': []
  'cancel-confirm': []
  'confirm-submit': []
  'request-remove': []
  'cancel-remove': []
  'confirm-remove': []
  'retry': []
  'resume-poll': []
  'reload-context': []
  'resend': []
  'next': []
  'back': []
}>()

const issueText = computed(() => {
  if (!props.issue) return ''
  if (props.issue.category !== 'rewrite') return '该构成标签可能把跨域标签吸进本版块。'
  return props.issue.notExecutableReason
    ? '这条建议来自体检分诊，但该标签没有描述修复入口。'
    : '描述可能把匹配带偏。'
})

const issueTitle = computed(() => {
  if (!props.issue) return ''
  if (props.issue.category !== 'rewrite') return `是否从本版块摘除「${props.issue.label}」？`
  return props.issue.notExecutableReason
    ? `「${props.issue.label}」的描述建议暂不可执行`
    : `让「${props.issue.label}」回到准确的定义`
})

const issueKicker = computed(() => {
  if (!props.issue) return ''
  if (props.issue.category !== 'rewrite') return '构成核对'
  return props.issue.notExecutableReason ? '描述修复 · 只读' : '描述修复'
})

const issueLeadTail = computed(() => {
  if (!props.issue) return ''
  if (props.issue.category !== 'rewrite') return '只有证据支持时才摘除，不把低相似度直接当作删除理由。'
  return props.issue.notExecutableReason
    ? '证据仍可展开核对；要处理这一项，可考虑从本版块摘除它（既有摘除流程）。'
    : '先核对事实，再确认修改；不要因为描述带偏，就直接删掉一个本来正确的标签。'
})

const magnet = computed(() => props.signal?.magnet ?? null)
const neighbors = computed(() => props.signal?.neighbors ?? null)
const descFlags = computed(() => props.signal?.descFlags ?? [])

/**
 * 磁铁证据对比文案（纯派生）：按服务端口径标记 `caliber` 分支——
 * 新口径用「失败子集 vs 健康子集」，旧口径快照回退并标注「旧口径快照」。
 * 前端不重算任何口径。
 */
const magnetComparison = computed(() => boardHealthMagnetComparisonText(magnet.value))

/**
 * 磁铁结论（纯派生）：服务端已判，不重算；旧口径快照的结论带口径标记
 * （旧公式语义与新口径相反，不可读成新口径结论）。
 */
const magnetVerdict = computed(() => boardHealthMagnetVerdictText(magnet.value))

function fmtCos(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '未知'
  return value.toFixed(3)
}
</script>

<template>
  <div class="bhid" data-testid="bhw-detail">
    <button
      v-if="narrow"
      type="button"
      class="bhid-back"
      data-testid="bhw-back-to-index"
      @click="emit('back')"
    >
      ← 返回问题清单
    </button>

    <p v-if="!issue" class="bhid-empty" data-testid="bhw-detail-empty">
      从左侧选择一项待核对问题开始。
    </p>

    <template v-else>
      <p class="bhid-kicker">
        {{ issueKicker }}
        <span v-if="issue.coreSubject" class="bhid-core-badge" data-testid="bhw-core-subject">核心主体</span>
      </p>
      <h3 class="bhid-title">{{ issueTitle }}</h3>
      <p class="bhid-lead">
        {{ issueText }}
        {{ issueLeadTail }}
      </p>

      <!-- 已有任务：只读执行态 -->
      <BoardHealthRepairProgress
        v-if="task"
        :label="issue.label"
        :task="task"
        :context="context"
        :poll-paused="pollPaused"
        :task-error="taskError"
        :submitting="submitting"
        :retrying="retrying"
        :submit-uncertain="submitUncertain"
        :frozen-description="frozenDescription"
        :has-next="hasNext"
        :evidence="issue.evidence"
        @retry="emit('retry')"
        @resume-poll="emit('resume-poll')"
        @reload-context="emit('reload-context')"
        @next="emit('next')"
        @back="emit('back')"
      />

      <!-- 提交结果未知（尚无任务）：保留原确认内容，只读 + 同键重发 -->
      <section v-else-if="submitUncertain" class="bhid-unknown" data-testid="bhw-unknown-result">
        <h4 class="bhid-unknown-title">上一次提交结果尚未确认</h4>
        <p class="bhid-lead">
          请求可能已被服务端受理（也可能没有）。已保留<strong>同一份确认内容</strong>：
          重发只会拿到同一个任务，不会新建第二个修复；<strong>确认前请不要修改内容</strong>。
        </p>
        <p class="bhid-unknown-body" data-testid="bhw-unknown-payload">已确认文案（只读）：{{ frozenDescription || '（空）' }}</p>
        <p v-if="submitError" class="bhid-error" role="alert" data-testid="bhw-submit-error">{{ submitError }}</p>
        <div class="bhid-actions">
          <AppButton :disabled="submitting" data-testid="bhw-resend" @click="emit('resend')">
            {{ submitting ? '重发中…' : '重发原确认内容' }}
          </AppButton>
          <AppButton variant="secondary" size="sm" :disabled="submitting" data-testid="bhw-reload-context-unknown" @click="emit('reload-context')">
            重新加载上下文核对
          </AppButton>
        </div>
      </section>

      <!-- 建议重写描述：编辑器（组合标签/类型未知 → 只读说明，无修复入口） -->
      <template v-else-if="issue.category === 'rewrite'">
        <section v-if="issue.notExecutableReason" class="bhid-unsupported" data-testid="bhw-unsupported-repair">
          <h4 class="bhid-unsupported-title">此处不能执行描述修复</h4>
          <p class="bhid-lead">
            {{ issue.notExecutableReason }}。分诊证据仍可展开核对；要处理这一项，可考虑从本版块摘除它。
          </p>
        </section>
        <BoardHealthRepairEditor
          v-else
          :label="issue.label"
          :current-description="context?.label.description ?? ''"
          :draft="draft"
          :validation-error="validationError"
          :context="context"
          :context-loading="contextLoading"
          :context-error="contextError"
          :confirm-open="confirmOpen"
          :submitting="submitting"
          :disabled-reason="disabledReason"
          @update:draft="emit('update:draft', $event)"
          @request-confirm="emit('request-confirm')"
          @cancel-confirm="emit('cancel-confirm')"
          @confirm="emit('confirm-submit')"
        />
      </template>

      <!-- 建议摘除：说明 + 行内确认（真实删除由宿主注入） -->
      <section v-else class="bhid-remove" data-testid="bhw-remove-panel">
        <p class="bhid-lead">
          摘除只移除<strong>本版块的构成关系</strong>，不会删除共享标签本身，也不会即时重算历史匹配。
        </p>
        <p v-if="removed" class="bhid-removed" data-testid="bhw-removed-note">
          已从本版块摘除。历史匹配尚未重算，需要时仍须通过既有手动回填处理。
        </p>
        <div v-if="removeConfirmOpen" class="bhid-confirm" role="group" aria-label="确认摘除" data-testid="bhw-remove-confirm">
          <strong>确认摘除本版块构成？</strong>
          <p>不会删除标签本身，也不即时重算历史匹配。</p>
          <div class="bhid-actions">
            <AppButton data-testid="bhw-remove-confirm-yes" @click="emit('confirm-remove')">确认摘除</AppButton>
            <AppButton variant="secondary" size="sm" data-testid="bhw-remove-confirm-no" @click="emit('cancel-remove')">取消</AppButton>
          </div>
        </div>
        <div v-else class="bhid-actions">
          <AppButton :disabled="removed" data-testid="bhw-remove" @click="emit('request-remove')">
            核对后摘除
          </AppButton>
        </div>
      </section>

      <!-- 证据（四形态共用：remove/未知态也能展开再确认；已有任务时由执行态的折叠修改摘要承载） -->
      <details v-if="!task" class="bhid-evidence" data-testid="bhw-evidence">
        <summary>为什么建议处理 · 展开证据</summary>
        <p class="bhid-evidence-line">{{ issue.evidence }}</p>
        <p v-if="magnet" class="bhid-evidence-line">
          磁铁信号：{{ magnetComparison }} · <span data-testid="bhw-magnet-verdict">{{ magnetVerdict }}</span>
        </p>
        <p v-else class="bhid-evidence-line">磁铁信号：未知（样本不足或向量缺失，本轮未算）。</p>
        <p v-if="neighbors === null" class="bhid-evidence-line">最近邻：未知（该标签自身向量缺失）。</p>
        <p v-else-if="neighbors.length === 0" class="bhid-evidence-line">最近邻：已计算，空间内没有其它可比标签。</p>
        <p v-else class="bhid-evidence-line">
          最近邻：{{ neighbors.map((n) => `${n.label} ${fmtCos(n.cosine)}`).join('、') }}
        </p>
        <p class="bhid-evidence-line">
          描述事件性预筛：{{ descFlags.length > 0 ? `命中 ${descFlags.join('、')}` : '未命中' }}
        </p>
        <p v-if="magnetFlow" class="bhid-evidence-line">
          失败挂载流向：{{ magnetFlow.topBoard ?? '未知（服务端未给出可靠流向）' }}
          （失败挂载 {{ magnetFlow.failedTags }} 项，其中有可靠去向 {{ magnetFlow.flowTags }} 项）
        </p>
      </details>

      <!-- 提交失败（非 409）：独立错误行，服务端 message 透传，不编文案 -->
      <p v-if="conflict" class="bhid-error" role="alert" data-testid="bhw-conflict">
        <Icon icon="mdi:alert-circle-outline" width="13" />
        <span>{{ submitError || '提交被拒绝，请加载最新版重新核对。' }}</span>
      </p>
      <p v-else-if="submitFailure" class="bhid-error" role="alert" data-testid="bhw-submit-failure">
        <Icon icon="mdi:alert-circle-outline" width="13" />
        <span>{{ submitFailure }}</span>
      </p>
    </template>
  </div>
</template>

<style scoped>
.bhid {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  min-width: 0;
}

.bhid-back {
  align-self: flex-start;
  border: none;
  background: none;
  padding: 0;
  color: var(--color-accent);
  font-size: 0.72rem;
  cursor: pointer;
}

.bhid-empty {
  font-size: 0.76rem;
  color: var(--color-text-muted);
}

.bhid-kicker {
  margin: 0;
  font-size: 0.66rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

/* 核心主体徽标：沿用面板内 chip 语汇（细边框 + 沉底 + 弱化文字） */
.bhid-core-badge {
  display: inline-flex;
  align-items: center;
  margin-left: 0.3rem;
  padding: 0 0.35rem;
  height: 16px;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-sunken);
  color: var(--color-text-muted);
  font-size: 0.62rem;
  letter-spacing: normal;
  text-transform: none;
  white-space: nowrap;
}

.bhid-title {
  margin: 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1.05rem;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.bhid-lead {
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-text-secondary);
  line-height: 1.6;
}

.bhid-evidence {
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  padding: 0.45rem 0.6rem;
  background: var(--color-bg-sunken);
}

.bhid-evidence summary {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.bhid-evidence-line {
  margin: 0.3rem 0 0;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.bhid-unknown,
.bhid-unsupported,
.bhid-remove {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.6rem 0.7rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-sunken);
}

.bhid-unsupported-title {
  margin: 0;
  font-size: 0.82rem;
  color: var(--color-text-primary);
}

.bhid-unknown-title {
  margin: 0;
  font-size: 0.82rem;
  color: var(--color-text-primary);
}

.bhid-unknown-body {
  margin: 0;
  font-size: 0.74rem;
  color: var(--color-text-secondary);
  overflow-wrap: anywhere;
}

.bhid-confirm {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.55rem 0.65rem;
  border: 1px solid var(--color-accent);
  border-radius: 8px;
  background: var(--color-accent-subtle);
}

.bhid-confirm p {
  margin: 0;
  font-size: 0.74rem;
  color: var(--color-text-primary);
  line-height: 1.55;
}

.bhid-removed {
  margin: 0;
  font-size: 0.74rem;
  color: var(--color-success);
  line-height: 1.55;
}

.bhid-error {
  display: flex;
  align-items: flex-start;
  gap: 0.3rem;
  margin: 0;
  font-size: 0.72rem;
  color: var(--color-error);
  line-height: 1.5;
}

.bhid-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}
</style>
