<script setup lang="ts">
/**
 * 描述修复编辑器（board-health-triage-advice seam C2 / task 3.3）。
 *
 * 只负责**编辑与就地反馈**：三规则校验复用 C1 的 `validateRepairDescription`（同一闭集，
 * 不额外黑名单）、共享范围只读展示、最终确认走**行内确认**（不再叠一层弹窗）。
 *
 * 组件不直接发请求：确认动作由上层（工作面根）转交 C1 composable 执行；
 * 提交前只有 GET，确认前零写入。
 */
import { computed } from 'vue'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import type { AuxiliaryRepairContextView } from '~/api/normalizers/auxiliaryRepair'

const props = defineProps<{
  /** 辅助标签名（三规则之一：不得只重复标签名）。 */
  label: string
  /** 服务端当前描述（只读展示）。 */
  currentDescription: string
  /** 当前草稿（受控值）。 */
  draft: string
  /** 三规则就地校验结果（null = 通过）。 */
  validationError: string | null
  /** 实时修复上下文（共享范围来源；null = 未知，不谎称 0 范围）。 */
  context: AuxiliaryRepairContextView | null
  contextLoading: boolean
  contextError: string | null
  /** 行内确认是否展开。 */
  confirmOpen: boolean
  submitting: boolean
  /** 不可执行原因（如标签已停用）；非空时禁用编辑与提交。 */
  disabledReason: string | null
}>()

const emit = defineEmits<{
  'update:draft': [value: string]
  'request-confirm': []
  'cancel-confirm': []
  'confirm': []
}>()

const runeCount = computed(() => [...props.draft].length)

function onInput(event: Event): void {
  const target = event.target as HTMLTextAreaElement | null
  emit('update:draft', target?.value ?? '')
}

const scopeBoards = computed(() => props.context?.directBoards ?? null)
const activeTagCount = computed(() => props.context?.activeTagCount ?? null)

/**
 * 共享范围未知 / 未加载时的**禁确认**理由。
 * 范围缺失 = 未知（服务端未提供），不谎称 0 范围；此时不允许进入最终确认。
 */
const scopeBlockReason = computed(() => {
  if (props.contextLoading) return '共享范围读取中，请稍候再确认。'
  if (props.contextError) return `共享范围读取失败（${props.contextError}），请重试后再确认。`
  if (!props.context) return '尚未加载共享范围，请重新加载后再确认。'
  if (props.context.directBoards === null || props.context.activeTagCount === null) {
    return '共享范围未知（服务端未提供），请重新加载后再确认。'
  }
  return null
})
</script>

<template>
  <section class="bhre" data-testid="bhw-repair-editor">
    <h3 class="bhre-title">让「{{ label }}」回到准确的定义</h3>
    <p class="bhre-lead">
      先核对事实，再确认修改；不要因为描述带偏，就直接删掉一个本来正确的标签。
    </p>

    <div class="bhre-current">
      <span class="bhre-current-label">当前描述</span>
      <p class="bhre-current-text">{{ currentDescription || '（该标签当前没有描述）' }}</p>
    </div>

    <label class="bhre-field">
      <span class="bhre-field-label">新描述</span>
      <textarea
        class="bhre-textarea"
        :value="draft"
        :disabled="disabledReason !== null"
        rows="4"
        data-testid="bhw-draft"
        @input="onInput"
      />
    </label>

    <p class="bhre-count" data-testid="bhw-draft-count">{{ runeCount }}/500</p>
    <p v-if="validationError" class="bhre-error" role="alert" data-testid="bhw-validation-error">
      <Icon icon="mdi:alert-circle-outline" width="13" />
      <span>{{ validationError }}</span>
    </p>
    <p v-if="disabledReason" class="bhre-error" role="alert" data-testid="bhw-editor-disabled">
      <Icon icon="mdi:lock-outline" width="13" />
      <span>{{ disabledReason }}</span>
    </p>
    <p v-if="scopeBlockReason" class="bhre-error" role="alert" data-testid="bhw-scope-unknown">
      <Icon icon="mdi:help-circle-outline" width="13" />
      <span>{{ scopeBlockReason }}</span>
    </p>

    <!-- 共享影响范围（真实来源：repair-context API；缺失即未知，不谎称 0） -->
    <div class="bhre-scope" data-testid="bhw-scope">
      <p class="bhre-scope-lead">
        <Icon icon="mdi:link-variant" width="13" />
        <span>这是<strong>共享标签</strong>：所有使用它的版块都会受影响，匹配更新会覆盖全部活跃话题标签。</span>
      </p>
      <p v-if="contextLoading" class="bhre-scope-note">共享范围读取中…</p>
      <p v-else-if="contextError" class="bhre-scope-note">共享范围读取失败：{{ contextError }}（范围未知，请重试后再确认）</p>
      <template v-else-if="scopeBoards !== null">
        <p v-if="scopeBoards.length === 0" class="bhre-scope-note">当前没有版块直接引用它。</p>
        <ul v-else class="bhre-boards" data-testid="bhw-direct-boards">
          <li v-for="board in scopeBoards" :key="board.id">
            {{ board.label }}<span v-if="board.status !== 'active'" class="bhre-board-off">（已停用）</span>
          </li>
        </ul>
      </template>
      <p v-else class="bhre-scope-note">共享范围未知（读取失败），请重试后再确认。</p>
      <p class="bhre-scope-note">
        <template v-if="activeTagCount === null">
          匹配更新覆盖全部活跃话题标签（数量未知）。
        </template>
        <template v-else>
          匹配更新覆盖全部活跃话题标签（{{ activeTagCount }} 个）；旧体检快照与既有日报不改写。
        </template>
      </p>
    </div>

    <!-- 行内确认（不新叠弹窗）：确认前零写入 -->
    <div v-if="confirmOpen" class="bhre-confirm" role="group" aria-label="确认执行" data-testid="bhw-confirm">
      <strong>确认更新这个共享标签？</strong>
      <p>使用上方最终文案。此操作不是只保存文字，还会生成向量并更新活跃话题标签的版块匹配。</p>
      <div class="bhre-actions">
        <AppButton
          :loading="submitting"
          :disabled="submitting || scopeBlockReason !== null"
          data-testid="bhw-confirm-submit"
          @click="emit('confirm')"
        >
          {{ submitting ? '提交中…' : '确认修复并更新匹配' }}
        </AppButton>
        <AppButton variant="secondary" size="sm" :disabled="submitting" data-testid="bhw-confirm-cancel" @click="emit('cancel-confirm')">
          取消
        </AppButton>
      </div>
    </div>

    <div v-else class="bhre-actions">
      <AppButton
        :disabled="validationError !== null || disabledReason !== null || submitting || scopeBlockReason !== null"
        data-testid="bhw-submit"
        @click="emit('request-confirm')"
      >
        修复描述并更新匹配
      </AppButton>
    </div>
  </section>
</template>

<style scoped>
.bhre {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.bhre-title {
  margin: 0;
  font-family: var(--font-serif, Georgia, serif);
  font-size: 1rem;
  font-weight: 600;
  color: var(--color-text-primary);
  overflow-wrap: anywhere;
}

.bhre-lead {
  margin: 0;
  font-size: 0.76rem;
  color: var(--color-text-secondary);
  line-height: 1.6;
}

.bhre-current {
  padding: 0.5rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-sunken);
}

.bhre-current-label {
  font-size: 0.66rem;
  color: var(--color-text-muted);
}

.bhre-current-text {
  margin: 0.15rem 0 0;
  font-size: 0.78rem;
  color: var(--color-text-primary);
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.bhre-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.bhre-field-label {
  font-size: 0.72rem;
  color: var(--color-text-secondary);
}

.bhre-textarea {
  width: 100%;
  resize: vertical;
  padding: 0.5rem 0.6rem;
  border-radius: 8px;
  border: 1px solid var(--color-input-border);
  background: var(--color-input-bg);
  color: var(--color-text-primary);
  font: inherit;
  font-size: 0.78rem;
  line-height: 1.6;
}

.bhre-count {
  margin: 0;
  font-size: 0.66rem;
  color: var(--color-text-muted);
  font-variant-numeric: tabular-nums;
}

.bhre-error {
  display: flex;
  align-items: flex-start;
  gap: 0.3rem;
  margin: 0;
  font-size: 0.72rem;
  color: var(--color-error);
  line-height: 1.5;
}

.bhre-scope {
  padding: 0.5rem 0.6rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-sunken);
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.bhre-scope-lead {
  display: flex;
  align-items: flex-start;
  gap: 0.3rem;
  margin: 0;
  font-size: 0.74rem;
  color: var(--color-text-primary);
  line-height: 1.55;
}

.bhre-scope-note {
  margin: 0;
  font-size: 0.7rem;
  color: var(--color-text-secondary);
  line-height: 1.55;
}

.bhre-boards {
  margin: 0;
  padding-left: 1.1rem;
  font-size: 0.72rem;
  color: var(--color-text-secondary);
  line-height: 1.6;
}

.bhre-board-off {
  color: var(--color-text-muted);
}

.bhre-confirm {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  padding: 0.6rem 0.7rem;
  border: 1px solid var(--color-accent);
  border-radius: 8px;
  background: var(--color-accent-subtle);
}

.bhre-confirm p {
  margin: 0;
  font-size: 0.74rem;
  color: var(--color-text-primary);
  line-height: 1.55;
}

.bhre-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}
</style>
