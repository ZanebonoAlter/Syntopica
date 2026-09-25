<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Icon } from '@iconify/vue'
import AppButton from '~/components/ui/AppButton.vue'
import AppDialog from '~/components/ui/AppDialog.vue'
import AppInput from '~/components/ui/AppInput.vue'
import AppPageShell from '~/components/ui/AppPageShell.vue'
import { useSemanticBoardsApi, type SemanticBoard } from '~/api/semanticBoards'
import { useMarginNoteAdmin } from '../composables/useMarginNoteAdmin'
import type { MarginNoteAdminRow } from '~/api/marginNotes'

/**
 * 设置 · 页边注管理页（daily-report-margin-notes design D5 / specs 全局批注管理）：
 * 跨报告列出全部批注（日期倒序，含仅标记未提问行）；版块筛选 + 关键词搜索（命中划词/
 * 提问/术语，服务端 q 过滤）；「↗ 原日报」深链打开日报阅读层并定位锚点闪现（MG-5/MG-7）；
 * 删除与日报内同源（同一端点、同一确认语义，MG-6）。contained=1120 布局（ui-design Layout Contract）。
 */
const router = useRouter()
const { getBoards } = useSemanticBoardsApi()
const admin = useMarginNoteAdmin()

const boards = ref<SemanticBoard[]>([])
const boardFilter = ref<number | null>(null)
const keyword = ref('')
const confirmTarget = ref<MarginNoteAdminRow | null>(null)

const confirmOpen = computed({
  get: () => confirmTarget.value != null,
  set: (open: boolean) => {
    if (!open) confirmTarget.value = null
  },
})

const confirmQaCount = computed(() => confirmTarget.value?.qas.length ?? 0)

async function loadBoards() {
  const response = await getBoards()
  if (response.success && response.data) boards.value = response.data.items ?? []
}

function loadRows() {
  void admin.load({
    board_id: boardFilter.value ?? undefined,
    q: keyword.value.trim() || undefined,
    page_size: 200,
    page: 1,
  })
}

watch([boardFilter, keyword], () => loadRows())

onMounted(() => {
  void loadBoards()
  loadRows()
})

function jumpToReport(row: MarginNoteAdminRow) {
  if (!row.report_id) return
  void router.push({
    path: '/tags',
    query: {
      board: String(row.board_id ?? ''),
      report: String(row.report_id),
      annotation: String(row.id),
    },
  })
}

async function confirmDelete() {
  if (!confirmTarget.value) return
  const target = confirmTarget.value
  confirmTarget.value = null
  await admin.remove(target.id)
}

function formatDate(periodDate?: string): string {
  return periodDate ? periodDate.slice(0, 10) : ''
}
</script>

<template>
  <AppPageShell mode="contained" as="section">
    <div class="mg-section">
      <header class="mg-section__head">
        <div>
          <h2>页边注</h2>
          <p class="mg-section__sub">
            全部日报的划词批注与问答 · 共 <strong data-testid="mg-total">{{ admin.total.value }}</strong> 条 · 删除与日报内同源（连带问答）
          </p>
        </div>
        <div class="mg-section__filters" data-testid="mg-filters">
          <select v-model.number="boardFilter" class="mg-section__board" data-testid="mg-board-filter" aria-label="按版块筛选">
            <option :value="null">全部版块</option>
            <option v-for="board in boards" :key="board.id" :value="board.id">{{ board.label }}</option>
          </select>
          <AppInput
            v-model="keyword"
            type="search"
            placeholder="搜索划词 / 提问 / 术语…"
            class="mg-section__search"
            data-testid="mg-keyword"
          />
        </div>
      </header>

      <!-- 加载骨架 -->
      <div v-if="admin.loading.value" class="mg-section__loading" aria-live="polite" data-testid="mg-loading">
        <i v-for="index in 3" :key="index" />
      </div>

      <p v-else-if="admin.error.value" class="mg-section__error" role="alert" data-testid="mg-error">{{ admin.error.value }}</p>

      <!-- 空态：无匹配/无数据（MG-3） -->
      <p v-else-if="!admin.rows.value.length" class="mg-section__empty" data-testid="mg-empty">
        {{ boardFilter || keyword.trim() ? '没有匹配的批注，清空筛选后恢复' : '暂无批注' }}
      </p>

      <ul v-else class="mg-section__list" data-testid="mg-list">
        <li
          v-for="row in admin.rows.value"
          :key="row.id"
          class="mg-row"
          :class="{ 'mg-row--deleting': confirmTarget?.id === row.id }"
          :data-annotation-id="row.id"
        >
          <div class="mg-row__head">
            <span class="mg-row__meta" data-testid="mg-row-meta">
              {{ row.board_label || '未知版块' }} · {{ formatDate(row.period_date) }}<template v-if="row.issue_number"> · {{ row.issue_number }}</template>
            </span>
            <!-- 术语 chips：用后端去重后的 row.terms（与批准原型 x.terms 同口径）；
                 按 QA 轮展开会重复展示同一术语。 -->
            <span v-if="row.terms?.length" class="mg-row__terms">
              <span v-for="term in row.terms" :key="term" class="mg-row__term">{{ term }}</span>
            </span>
            <span class="mg-row__ops">
              <button type="button" class="mg-row__op" data-testid="mg-jump" title="打开原日报并定位该批注" @click="jumpToReport(row)">
                <Icon icon="mdi:arrow-top-right" width="12" /> 原日报
              </button>
              <button type="button" class="mg-row__op mg-row__op--danger" data-testid="mg-delete" title="删除批注（连带问答）" @click="confirmTarget = row">
                ✕ 删除
              </button>
            </span>
          </div>
          <p class="mg-row__quote">{{ row.quoted_text }}</p>
          <details v-if="row.qas.length" class="mg-row__qa" data-testid="mg-row-qa">
            <summary>{{ row.qas.length }} 轮问答</summary>
            <div v-for="qa in row.qas" :key="qa.id" class="mg-row__qa-item">
              <p class="mg-row__qa-q">{{ qa.question }}</p>
              <p class="mg-row__qa-a">{{ qa.answer }}</p>
              <div v-if="qa.cited_web_sources?.length" class="mg-row__qa-webs" data-testid="mg-row-webs">
                <a
                  v-for="web in qa.cited_web_sources"
                  :key="web.url"
                  class="mg-row__web-chip"
                  :href="web.url"
                  target="_blank"
                  rel="noopener"
                  :title="web.title || web.url"
                >↗ {{ web.title || web.url }}</a>
              </div>
            </div>
          </details>
          <p v-else class="mg-row__noqa" data-testid="mg-row-noqa">尚无提问（仅标记）</p>
        </li>
      </ul>

      <AppDialog v-model="confirmOpen" title="删除这条批注？" size="sm" :show-close="false">
        <p class="mg-section__confirm-text" data-testid="mg-confirm-text">
          将连带删除其下全部 <strong>{{ confirmQaCount }}</strong> 轮问答（含已生成的回答、引用与术语记录）。此操作不可恢复。
        </p>
        <div class="mg-section__confirm-actions">
          <AppButton variant="secondary" data-testid="mg-cancel-delete" @click="confirmTarget = null">取消</AppButton>
          <AppButton variant="danger" data-testid="mg-confirm-delete" @click="confirmDelete">删除</AppButton>
        </div>
      </AppDialog>
    </div>
  </AppPageShell>
</template>

<style scoped>
.mg-section__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 1rem;
  padding-bottom: 0.8rem;
  margin-bottom: 1.2rem;
  border-bottom: 3px double var(--color-border-strong);
}

.mg-section__head h2 {
  margin: 0;
  font-family: "Noto Serif SC", serif;
  font-size: 1.4rem;
}

.mg-section__sub {
  margin: 0.2rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.74rem;
}

.mg-section__filters {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.mg-section__board {
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 6px;
  background: var(--color-bg-hover);
  color: var(--color-text-primary);
  font-size: 0.78rem;
}

.mg-section__search {
  width: 14.5rem;
}

.mg-section__loading {
  display: grid;
  gap: 0.6rem;
}

.mg-section__loading i {
  height: 3.4rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-active);
  animation: mgPulse 1.3s ease-in-out infinite;
}

.mg-section__error {
  padding: 1.5rem 0;
  color: var(--color-error);
  font-size: 0.78rem;
}

.mg-section__empty {
  padding: 2.5rem 0;
  color: var(--color-text-muted);
  font-size: 0.78rem;
  text-align: center;
}

.mg-section__list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.mg-row {
  padding: 0.7rem 0.9rem;
  margin-bottom: 0.8rem;
  border: 1px solid var(--color-border-subtle);
  border-radius: 8px;
  background: var(--color-bg-hover);
}

.mg-row--deleting {
  opacity: 0.45;
}

.mg-row__head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.7rem;
}

.mg-row__meta {
  color: var(--color-text-muted);
  font-size: 0.68rem;
  letter-spacing: 0.04em;
}

.mg-row__terms {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: 0.3rem;
  min-width: 0;
}

.mg-row__term {
  padding: 0.08rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 999px;
  color: var(--color-text-secondary);
  font-size: 0.62rem;
}

.mg-row__ops {
  display: flex;
  gap: 0.35rem;
}

.mg-row__op {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.15rem 0.5rem;
  border: 1px solid var(--color-border-strong);
  border-radius: 5px;
  background: none;
  color: var(--color-text-secondary);
  font-size: 0.68rem;
  cursor: pointer;
}

.mg-row__op:hover {
  border-color: var(--color-accent);
  color: var(--color-accent);
}

.mg-row__op--danger:hover {
  background: var(--color-accent-subtle);
}

.mg-row__quote {
  margin: 0.45rem 0 0;
  color: var(--color-text-primary);
  font-family: "Noto Serif SC", serif;
  font-size: 0.82rem;
  line-height: 1.7;
}

.mg-row__quote::before {
  content: "❝ ";
  color: var(--color-accent);
}

.mg-row__qa {
  margin-top: 0.4rem;
  font-size: 0.74rem;
}

.mg-row__qa summary {
  color: var(--color-text-muted);
  font-size: 0.7rem;
  cursor: pointer;
  user-select: none;
}

.mg-row__qa summary:hover {
  color: var(--color-accent);
}

.mg-row__qa-item {
  margin-top: 0.45rem;
}

.mg-row__qa-q {
  margin: 0 0 0.15rem;
  color: var(--color-text-primary);
  font-weight: 600;
}

.mg-row__qa-q::before {
  content: "问 ";
  color: var(--color-accent);
  font-size: 0.68rem;
}

.mg-row__qa-a {
  margin: 0;
  color: var(--color-text-secondary);
  line-height: 1.7;
}

/* D7 联网来源：外链 chips，中性色系与阅读卡片同口径。 */
.mg-row__qa-webs {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
  margin-top: 0.3rem;
}

.mg-row__web-chip {
  display: inline-block;
  max-width: 100%;
  padding: 0.1rem 0.45rem;
  border: 1px solid var(--color-border);
  border-radius: 4px;
  color: var(--color-text-secondary);
  font-size: 0.64rem;
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mg-row__web-chip:hover {
  text-decoration: underline;
  background: color-mix(in srgb, var(--color-text-primary) 5%, transparent);
}

.mg-row__qa-a::before {
  content: "答 ";
  color: var(--color-text-muted);
  font-size: 0.68rem;
}

.mg-row__noqa {
  margin: 0.4rem 0 0;
  color: var(--color-text-muted);
  font-size: 0.7rem;
}

.mg-section__confirm-text {
  margin: 0 0 1rem;
  color: var(--color-text-secondary);
  font-size: 0.8rem;
  line-height: 1.7;
}

.mg-section__confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.6rem;
}

@keyframes mgPulse {
  0%, 100% { opacity: 0.45; }
  50% { opacity: 0.85; }
}
</style>
