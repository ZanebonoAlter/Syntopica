<script setup lang="ts">
/**
 * 描述修复执行态（board-health-triage-advice seam C2 / task 3.3）。
 *
 * 只读执行面：三真实阶段（服务端 status/stage 决定）+ 真实计数（total / processed /
 * failed / not_applicable / remaining，**未知显示"未知"，不补 0**）+ 恢复动作。
 * 不摆可编辑 textarea（active / 完成 / 失败态都没有）；修改摘要默认折叠（old/new 取任务审计字段）。
 *
 * 完成措辞严格为「更新完成，效果待下一轮体检验证」；不自行判定完成（看服务端 status）。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { AuxiliaryRepairContextView, AuxiliaryRepairJobView } from '~/api/normalizers/auxiliaryRepair'
import { boardHealthStageViews, formatRepairCounter } from './boardHealthWorkspace'

const props = defineProps<{
  label: string
  task: AuxiliaryRepairJobView
  /** 实时修复上下文（共享范围只读展示；null = 未知）。 */
  context: AuxiliaryRepairContextView | null
  /** 查询暂停（网络错 / 预算耗尽 / 状态不可解读）→ 可恢复查询。 */
  pollPaused: boolean
  /** 任务查询错误。 */
  taskError: string | null
  submitting: boolean
  retrying: boolean
  /** 提交结果未知（幂等键与冻结 payload 保留，等同键重发）。 */
  submitUncertain: boolean
  /** 冻结的确认内容（只读展示）。 */
  frozenDescription: string | null
  /** 本项的分诊证据（只读，随修改摘要一同折叠）。 */
  evidence?: string
  /** 是否还有下一项可处理。 */
  hasNext: boolean
}>()

const emit = defineEmits<{
  retry: []
  'resume-poll': []
  'reload-context': []
  'resend': []
  next: []
  back: []
}>()

const stages = computed(() => boardHealthStageViews(props.task))
const scopeBoards = computed(() => props.context?.directBoards ?? null)
const activeTagCount = computed(() => props.context?.activeTagCount ?? null)

const message = computed(() => {
  switch (props.task.status) {
    case 'completed':
      return '描述、向量与匹配更新完成。效果待下一轮体检验证；当前评分仍来自修复前快照。'
    case 'embedding_failed':
      return '新向量生成失败，旧描述与旧向量均未改变。可以重试。'
    case 'save_failed':
      return '保存描述与向量失败，本次没有提交（旧描述与旧向量未改变）。可以重试。'
    case 'matching_failed':
      return props.task.failed === null || props.task.failed === undefined
        ? '描述与向量已保存，但有匹配更新失败项（数量未知），不能标作修复完成。'
        : `描述与向量已保存，但有 ${formatRepairCounter(props.task.failed)} 项匹配更新失败，不能标作修复完成。`
    case 'conflict':
      return '原描述已被其他操作修改，本次没有覆盖数据。请加载最新版本重新核对；你的草稿会保留。'
    case 'unavailable':
      return '该标签已被禁用，本次没有修改描述或向量。'
    case 'unknown':
      return '任务状态未知（服务端返回了无法解读的状态），可稍后恢复查询；既不当成失败，也不当成完成。'
    default:
      return ''
  }
})

const isFailed = computed(() =>
  props.task.status === 'embedding_failed' || props.task.status === 'save_failed' || props.task.status === 'matching_failed',
)
</script>

<template>
  <section class="bhrp" data-testid="bhw-repair-progress">
    <h3 class="bhrp-title">「{{ label }}」的修复进度</h3>
    <p class="bhrp-lead">按确认的文案执行，不改变标签身份。效果待下一轮体检验证。</p>

    <ol class="bhrp-stages" aria-live="polite" data-testid="bhw-stages">
      <li
        v-for="stage in stages"
        :key="stage.key"
        class="bhrp-stage"
        :class="`is-${stage.state}`"
        :data-testid="`bhw-stage-${stage.key}`"
      >
        <Icon
          :icon="stage.state === 'done' ? 'mdi:check-circle-outline' : stage.state === 'error' ? 'mdi:alert-circle-outline' : 'mdi:circle-outline'"
          width="13"
        />
        <span>{{ stage.label }}</span>
      </li>
    </ol>

    <ul class="bhrp-counters" data-testid="bhw-counters">
      <li><span>范围</span><b>{{ formatRepairCounter(task.total) }}</b></li>
      <li><span>已更新</span><b>{{ formatRepairCounter(task.processed) }}</b></li>
      <li><span>失败</span><b>{{ formatRepairCounter(task.failed) }}</b></li>
      <li><span>已失效/删除被跳过</span><b>{{ formatRepairCounter(task.notApplicable) }}</b></li>
      <li><span>待处理</span><b>{{ formatRepairCounter(task.remaining) }}</b></li>
    </ul>
    <p class="bhrp-counters-note">跳过项不是成功，也不计入已更新；计数未知时显示"未知"，不补 0。</p>

    <details class="bhrp-summary" data-testid="bhw-change-summary">
      <summary>查看本次确认的修改与共享范围</summary>
      <p>修改前：{{ task.oldDescription || '（原描述为空）' }}</p>
      <p>已确认文案（只读）：{{ task.newDescription || '（空）' }}</p>
      <p v-if="evidence">证据：{{ evidence }}</p>
      <p v-if="submitUncertain && frozenDescription !== null">
        上次提交结果未知，已保留同一幂等键与同一份确认文案（只读）：{{ frozenDescription }}
      </p>
      <p v-if="scopeBoards !== null">
        这是共享标签，直接使用：
        <template v-if="scopeBoards.length === 0">当前没有版块直接引用它；</template>
        <template v-else>{{ scopeBoards.map((board) => board.label).join('、') }}；</template>
      </p>
      <p>
        <template v-if="activeTagCount === null">匹配更新覆盖全部活跃话题标签（数量未知）。</template>
        <template v-else>匹配更新覆盖全部活跃话题标签（{{ activeTagCount }} 个）。</template>
        旧体检快照与既有日报不改写。
      </p>
    </details>

    <p v-if="message" class="bhrp-message" :class="{ 'is-error': isFailed || task.status === 'conflict' || task.status === 'unavailable' || task.status === 'unknown' }" role="status" data-testid="bhw-progress-message">
      {{ message }}
    </p>
    <p v-if="taskError" class="bhrp-error" role="alert" data-testid="bhw-task-error">{{ taskError }}</p>

    <div class="bhrp-actions">
      <template v-if="task.status === 'completed'">
        <AppButton data-testid="bhw-next" @click="emit('next')">
          {{ hasNext ? '处理下一项 →' : '返回问题清单' }}
        </AppButton>
      </template>
      <template v-else-if="isFailed">
        <AppButton :loading="retrying" :disabled="retrying || submitting" data-testid="bhw-retry" @click="emit('retry')">
          {{ retrying ? '重试中…' : '重试修复' }}
        </AppButton>
      </template>
      <template v-else-if="task.status === 'conflict'">
        <AppButton data-testid="bhw-reload-context" @click="emit('reload-context')">加载最新描述再核对</AppButton>
      </template>
      <template v-else-if="task.status === 'unknown'">
        <AppButton :disabled="submitting" data-testid="bhw-resume-poll" @click="emit('resume-poll')">恢复查询</AppButton>
      </template>
      <template v-else-if="pollPaused">
        <AppButton data-testid="bhw-resume-poll" @click="emit('resume-poll')">恢复查询</AppButton>
      </template>
      <template v-else>
        <AppButton variant="secondary" size="sm" :disabled="true" data-testid="bhw-running">
          {{ submitting ? '提交中…' : '执行中…' }}
        </AppButton>
      </template>
      <AppButton variant="ghost" size="sm" data-testid="bhw-back" @click="emit('back')">返回问题清单</AppButton>
    </div>
    <p v-if="isFailed" class="bhrp-note">不会重新生成向量，也不会再次覆盖已保存的描述。</p>
  </section>
</template>

<style scoped>
.bhrp {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.bhrp-title {
  margin: 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1rem;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.bhrp-lead {
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-text-secondary);
  line-height: 1.6;
}

.bhrp-stages {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.bhrp-stage {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.76rem;
  color: var(--color-text-muted);
  padding: 0.3rem 0.5rem;
  border-radius: 7px;
  background: var(--color-bg-sunken);
}

.bhrp-stage.is-done {
  color: var(--color-success);
}

.bhrp-stage.is-current {
  color: var(--color-accent);
  background: var(--color-accent-subtle);
}

.bhrp-stage.is-error {
  color: var(--color-error);
}

.bhrp-counters {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

.bhrp-counters li {
  display: inline-flex;
  align-items: baseline;
  gap: 0.25rem;
  padding: 0.2rem 0.45rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 999px;
  font-size: 0.68rem;
  color: var(--color-text-muted);
}

.bhrp-counters b {
  color: var(--color-text-primary);
  font-variant-numeric: tabular-nums;
}

.bhrp-counters-note {
  margin: 0;
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

.bhrp-summary {
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  padding: 0.45rem 0.6rem;
  background: var(--color-bg-sunken);
}

.bhrp-summary summary {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.bhrp-summary p {
  margin: 0.3rem 0 0;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.bhrp-message {
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-text-primary);
  line-height: 1.6;
}

.bhrp-message.is-error {
  color: var(--color-error);
}

.bhrp-error {
  margin: 0;
  font-size: 0.72rem;
  color: var(--color-error);
}

.bhrp-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.bhrp-note {
  margin: 0;
  font-size: 0.68rem;
  color: var(--color-text-muted);
}
</style>
