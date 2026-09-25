<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Icon } from '@iconify/vue'
import type {
  TopicEnrichmentQA,
  AnalyzeRef,
  AskQAResponse,
} from '~/api/boardEnrichment'
import AnalyzeRefChip from './AnalyzeRefChip.vue'
import { renderMarkdown } from '~/utils/markdown'
import { useMermaidRender } from '~/composables/useMermaidRender'
// 全局 .markdown-body 样式（文章阅读器同款），让 md 渲染产物有标题/列表/粗体样式 + 双主题
import '~/components/article/ArticleContent.css'

/**
 * 报告追问 chat（causal-analysis-agent 阶段3b-ii）。
 *
 * 挂在 CausalAnalysisReport 底部，让读者就这份报告继续追问、并把有价值的回答
 * 「沉淀」回报告上下文。多轮按 created_at 升序成线程。
 *
 * 数据流（props-down / events-up，对齐 DebateSection 模式）：
 *  - useBoardEnrichment 是 instance-per-call（非单例），QAPanel 不能自己再调一次
 *    （新实例 selectedTopicId=null，loadQA/askQuestion 会直接 return）。故由
 *    BoardEnrichmentPanel 持有唯一实例，把 qa 状态作 props 传入，ask/sediment/load
 *    作事件上抛，父组件转接 composable 方法。
 *
 * refs 渲染：持久化 QA 行无 refs 列（后端只回显不落库），仅 latestAnswer（最近一次
 * ask 的即时响应）带双类引用；按 answer 文本匹配挂到对应行，复用 AnalyzeRefChip。
 *
 * 确定性标注：后端 prompt 要求 agent 把确定性写进 answer 文本，故 answer 本身即带标注。
 */
const props = defineProps<{
  /** 当前报告 result id；null 时面板不加载（父组件用 v-if 守卫）。 */
  resultId: number | null
  qaList: TopicEnrichmentQA[]
  qaLoading: boolean
  qaError: string | null
  /** 最近一次 ask 的即时响应（含 refs）；历史轮无 refs。 */
  latestAnswer: AskQAResponse | null
}>()

const emit = defineEmits<{
  /** 提交追问。 */
  ask: [question: string]
  /** 沉淀某轮回答到报告。 */
  sediment: [qaId: number]
  /** 加载某 result 的追问历史（挂载/resultId 变更时触发）。 */
  load: [resultId: number]
}>()

// mermaid 图渲染（render-mermaid-diagrams）：宿主 = 面板根，扫描全部回答的 markdown 主体
const qaRootRef = ref<HTMLElement | null>(null)
useMermaidRender(qaRootRef, () => props.qaList)

// ── 挂载 / resultId 变更 → 拉历史 ────────────────────────────────────────
watch(
  () => props.resultId,
  (id) => {
    if (id !== null) emit('load', id)
  },
  { immediate: true },
)

// ── 输入 + 提交 ──────────────────────────────────────────────────────────
const draft = ref('')
const canSubmit = computed(
  () => draft.value.trim().length > 0 && !props.qaLoading && props.resultId !== null,
)

function submit() {
  if (!canSubmit.value) return
  const q = draft.value.trim()
  emit('ask', q)
  draft.value = ''
}
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    submit()
  }
}

// ── 行派生 ───────────────────────────────────────────────────────────────
/** 多轮按 created_at 升序（后端已 oldest-first，此处兜底）。 */
const ordered = computed(() =>
  [...props.qaList].sort((a, b) => (a.created_at < b.created_at ? -1 : 1)),
)

/** 某轮的 refs：仅 latestAnswer（按 answer 文本匹配）有双类引用，历史轮为空。 */
function refsFor(qa: TopicEnrichmentQA): AnalyzeRef[] {
  if (props.latestAnswer && props.latestAnswer.answer === qa.answer) {
    return props.latestAnswer.refs ?? []
  }
  return []
}

/** tool_calls 防御解析（结构未冻结，LLM 产物）。 */
function toolNames(qa: TopicEnrichmentQA): string[] {
  const tc = qa.tool_calls
  if (!Array.isArray(tc)) return []
  return tc
    .map((t) => {
      if (t && typeof t === 'object') {
        const o = t as Record<string, unknown>
        return String(o.name ?? o.tool ?? o.skill ?? o.action ?? '')
      }
      return String(t ?? '')
    })
    .filter(Boolean)
}

function sediment(qaId: number) {
  emit('sediment', qaId)
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return ''
    return d.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return ''
  }
}
</script>

<template>
  <section ref="qaRootRef" class="qa-panel">
    <header class="qa-head">
      <h3 class="serif qa-title">
        <Icon icon="mdi:comment-question-outline" width="16" />
        追问 · 深挖这份报告
      </h3>
      <span class="qa-helper">就报告内容继续提问；有价值的回答可沉淀回报告上下文</span>
    </header>

    <!-- ── 错误态 ──────────────────────────────────────────────────── -->
    <div v-if="qaError" class="qa-error">
      <Icon icon="mdi:alert-circle-outline" width="14" />
      <span>{{ qaError }}</span>
    </div>

    <!-- ── 线程 ────────────────────────────────────────────────────── -->
    <div v-if="ordered.length" class="qa-thread">
      <div v-for="qa in ordered" :key="qa.id" class="qa-turn">
        <!-- 读者问 -->
        <div class="qa-q">
          <span class="qa-role">读者追问</span>
          <p class="qa-q-text">{{ qa.question }}</p>
          <span v-if="formatTime(qa.created_at)" class="qa-time">{{ formatTime(qa.created_at) }}</span>
        </div>

        <!-- 分析员答 -->
        <div class="qa-a" :class="{ sedimented: qa.sedimented }">
          <div class="qa-a-head">
            <span class="qa-role agent">分析员作答</span>
            <span v-if="qa.sedimented" class="qa-sed-badge">
              <Icon icon="mdi:check-decagram" width="12" /> 已沉淀
            </span>
          </div>
          <div class="qa-a-text markdown-body" v-html="renderMarkdown(qa.answer)" />

          <!-- 双类引用（复用 AnalyzeRefChip；仅最新轮有） -->
          <div v-if="refsFor(qa).length" class="qa-refs">
            <span class="qa-refs-label">引用</span>
            <AnalyzeRefChip v-for="(r, i) in refsFor(qa)" :key="i" :r="r" />
          </div>

          <!-- 探索过程 tool_calls（折叠 · 低调，复用报告 trace 风格） -->
          <details v-if="toolNames(qa).length" class="qa-trace">
            <summary>探索过程 · {{ toolNames(qa).length }} 次工具调用</summary>
            <ol class="qa-trace-list">
              <li v-for="(n, i) in toolNames(qa)" :key="i">
                <span class="qa-trace-idx">{{ i + 1 }}</span>
                <span class="qa-trace-name">{{ n }}</span>
              </li>
            </ol>
          </details>

          <!-- 沉淀按钮 -->
          <div class="qa-a-foot">
            <button
              v-if="!qa.sedimented"
              type="button"
              class="qa-sed-btn"
              :disabled="qaLoading"
              @click="sediment(qa.id)"
            >
              <Icon icon="mdi:anchor" width="12" />
              沉淀到报告
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- ── 空态 ────────────────────────────────────────────────────── -->
    <div v-else-if="!qaLoading && !qaError" class="qa-empty">
      <Icon icon="mdi:lightbulb-question-outline" width="22" />
      <p>还没有追问。在下方提问，让分析员就这份报告继续深挖。</p>
    </div>

    <!-- ── 输入区 ──────────────────────────────────────────────────── -->
    <div class="qa-input-row">
      <textarea
        v-model="draft"
        class="qa-input"
        rows="2"
        placeholder="就这份报告提问…（Enter 发送，Shift+Enter 换行）"
        :disabled="qaLoading"
        @keydown="onKeydown"
      />
      <button
        type="button"
        class="qa-send"
        :disabled="!canSubmit"
        @click="submit"
      >
        <Icon v-if="qaLoading" icon="mdi:loading" width="14" class="qa-spin" />
        <Icon v-else icon="mdi:send" width="14" />
        <span>{{ qaLoading ? '追问中…' : '发送' }}</span>
      </button>
    </div>
  </section>
</template>

<style scoped>
.qa-panel {
  /* info subtle 本地派生（与 AnalyzeRefChip/CausalAnalysisReport 一致）。 */
  --color-info-subtle: color-mix(in srgb, var(--color-info) 12%, transparent);
  /* 与 .ew-panel（叙事工坊工作台 960px）同宽，宽屏不挤中条，与报告视觉一体。 */
  max-width: 960px;
  margin: 1.6rem auto 0;
  padding: 1.1rem 1.3rem;
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-subtle);
  border-top: 3px solid var(--color-info);
  border-radius: 8px;
  box-shadow: var(--shadow-print);
}
.serif {
  font-family: Georgia, "Songti SC", "SimSun", "Source Serif 4", serif;
}

/* ── 头 ─────────────────────────────────────────────────────────────── */
.qa-head {
  display: flex;
  align-items: baseline;
  gap: 0.6rem;
  flex-wrap: wrap;
  margin-bottom: 0.9rem;
  padding-bottom: 0.6rem;
  border-bottom: 1px solid var(--color-border-subtle);
}
.qa-title {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 15px;
  font-weight: 700;
  margin: 0;
  color: var(--color-text-primary);
}
.qa-title :deep(svg) { color: var(--color-info); }
.qa-helper {
  font-size: 11.5px;
  color: var(--color-text-muted);
}

/* ── 错误 ───────────────────────────────────────────────────────────── */
.qa-error {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.5rem 0.8rem;
  margin-bottom: 0.8rem;
  background: var(--color-error-subtle, color-mix(in srgb, var(--color-error) 14%, transparent));
  border-left: 3px solid var(--color-error);
  border-radius: 0 6px 6px 0;
  font-size: 12.5px;
  color: var(--color-error);
}
.qa-error :deep(svg) { flex: 0 0 auto; }

/* ── 线程 ───────────────────────────────────────────────────────────── */
.qa-thread {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  margin-bottom: 0.9rem;
}
.qa-turn {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

/* 读者问：右倾浅框（读者来信风） */
.qa-q {
  align-self: flex-end;
  max-width: 88%;
  padding: 0.5rem 0.85rem;
  background: var(--color-bg-sunken);
  border-radius: 10px 10px 2px 10px;
  position: relative;
}
.qa-q .qa-role {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}
.qa-q-text {
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--color-text-primary);
  margin: 0.15rem 0 0;
  white-space: pre-wrap;
  word-break: break-word;
}
.qa-time {
  font-size: 10.5px;
  color: var(--color-text-muted);
  font-family: ui-monospace, Menlo, monospace;
}

/* 分析员答：左倾框（编辑回信风） */
.qa-a {
  align-self: flex-start;
  max-width: 92%;
  width: 100%;
  padding: 0.65rem 0.9rem;
  background: var(--color-bg-base);
  border: 1px solid var(--color-border-subtle);
  border-left: 3px solid var(--color-info);
  border-radius: 2px 10px 10px 10px;
}
.qa-a.sedimented {
  border-left-color: var(--color-success);
  background: var(--color-success-subtle);
}
.qa-a-head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.3rem;
}
.qa-role.agent {
  color: var(--color-info);
}
.qa-sed-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 10.5px;
  font-weight: 700;
  padding: 1px 8px;
  border-radius: 999px;
  color: var(--color-success);
  background: color-mix(in srgb, var(--color-success) 16%, transparent);
}
.qa-a-text {
  /* markdown-body 宿主：全局文章档字号（1.0625rem）在此被 scoped 高优先级覆盖回 13.5px */
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--color-text-primary);
  margin: 0;
  white-space: normal;
  word-break: break-word;
}

/* 引用 */
.qa-refs {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  flex-wrap: wrap;
  margin-top: 0.5rem;
}
.qa-refs-label {
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--color-text-muted);
}

/* 探索过程 trace（折叠 · 低调，复用报告 trace 风格） */
.qa-trace {
  margin-top: 0.5rem;
  background: var(--color-bg-sunken);
  border-radius: 6px;
  padding: 0.35rem 0.75rem;
}
.qa-trace > summary {
  cursor: pointer;
  font-size: 11px;
  font-weight: 600;
  color: var(--color-text-muted);
  list-style: none;
}
.qa-trace > summary::-webkit-details-marker { display: none; }
.qa-trace > summary::before { content: '▸ '; color: var(--color-text-muted); }
.qa-trace[open] > summary::before { content: '▾ '; }
.qa-trace-list {
  margin: 0.4rem 0 0.15rem;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}
.qa-trace-list li {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 11px;
  color: var(--color-text-secondary);
}
.qa-trace-idx {
  font-size: 9.5px;
  font-weight: 700;
  color: var(--color-text-muted);
  background: var(--color-bg-elevated);
  padding: 1px 5px;
  border-radius: 4px;
  min-width: 18px;
  text-align: center;
}
.qa-trace-name {
  font-family: ui-monospace, Menlo, monospace;
  font-size: 10.5px;
}

/* 沉淀按钮 */
.qa-a-foot {
  display: flex;
  justify-content: flex-end;
  margin-top: 0.5rem;
}
.qa-sed-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-family: inherit;
  cursor: pointer;
  border: 1px solid var(--color-border-medium);
  background: var(--color-bg-elevated);
  color: var(--color-text-secondary);
  border-radius: 6px;
  font-size: 11.5px;
  font-weight: 600;
  padding: 4px 10px;
  transition: background 0.12s, color 0.12s, border-color 0.12s;
}
.qa-sed-btn:hover:not(:disabled) {
  background: var(--color-success-subtle);
  color: var(--color-success);
  border-color: var(--color-success);
}
.qa-sed-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* ── 空态 ───────────────────────────────────────────────────────────── */
.qa-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  gap: 0.4rem;
  padding: 1.4rem 1rem;
  margin-bottom: 0.9rem;
  color: var(--color-text-muted);
}
.qa-empty :deep(svg) { opacity: 0.5; }
.qa-empty p { font-size: 12.5px; margin: 0; line-height: 1.6; }

/* ── 输入区 ─────────────────────────────────────────────────────────── */
.qa-input-row {
  display: flex;
  gap: 0.5rem;
  align-items: flex-end;
  padding-top: 0.7rem;
  border-top: 1px solid var(--color-border-subtle);
}
.qa-input {
  flex: 1 1 auto;
  resize: none;
  font-family: inherit;
  font-size: 13px;
  line-height: 1.6;
  background: var(--color-input-bg);
  border: 1px solid var(--color-input-border);
  border-radius: 8px;
  color: var(--color-text-primary);
  padding: 0.5rem 0.7rem;
  outline: none;
  box-sizing: border-box;
}
.qa-input::placeholder { color: var(--color-text-muted); }
.qa-input:focus { border-color: var(--color-input-focus); }
.qa-input:disabled { opacity: 0.6; cursor: not-allowed; }
.qa-send {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-family: inherit;
  cursor: pointer;
  border: none;
  background: var(--color-accent);
  color: #fff;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  padding: 0.5rem 0.95rem;
  transition: background 0.12s, opacity 0.12s, transform 0.1s;
  white-space: nowrap;
}
.qa-send:hover:not(:disabled) { background: var(--color-accent-hover); }
.qa-send:active:not(:disabled) { transform: translateY(1px); }
.qa-send:disabled { opacity: 0.4; cursor: not-allowed; }
.qa-spin { animation: qa-spin 0.9s linear infinite; }
@keyframes qa-spin { to { transform: rotate(360deg); } }

/* ── 窄屏适配（≤720px，对齐 daily-report 家族断点） ───────────────────── */
@media (max-width: 720px) {
  .qa-panel { padding: 0.9rem 0.85rem; margin-top: 1.2rem; }

  /* 气泡放宽：问答仍左右错位可辨，但不留大水沟 */
  .qa-q { max-width: 96%; }
  .qa-a { max-width: 100%; }

  /* 输入区：字号 ≥16px 防 iOS 聚焦自动缩放；发送按钮加大 hit-target */
  .qa-input { font-size: 16px; }
  .qa-send { min-height: 40px; padding: 0.45rem 0.85rem; }

  /* 沉淀按钮触摸友好（hit-target ≥36px） */
  .qa-sed-btn { min-height: 36px; padding: 6px 12px; font-size: 12px; }

  .qa-empty { padding: 1.1rem 0.75rem; }
}
</style>
