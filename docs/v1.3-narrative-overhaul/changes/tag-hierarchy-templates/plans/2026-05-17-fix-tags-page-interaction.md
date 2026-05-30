# 修复 /tags 页面交互 BUG 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复验证报告中 4 个 CRITICAL 问题：层级树 Node 点击导航错误、未归属标签展示、行内重命名不可用、添加板块重复点击异常。

**Architecture:** 问题集中在 `TagHierarchyRow.vue`（点击事件冲突）、`TagHierarchy.vue`（select-tag 事件在 /tags 页面无意义）、`TagsPage.vue`（缺少事件拦截）。Bug 4 根因需用浏览器复现确认，可能与 Teleport 关闭后焦点或 pointer-events 有关。Bug 3（行内重命名）代码已存在，因 Bug 1 的 click/dblclick 冲突而无法触发 — 修好 Bug 1 即自动修复。

**Tech Stack:** Vue 3 Composition API, Nuxt 4, TypeScript

---

## 根因分析

| Bug | 根因 | 文件 |
|-----|------|------|
| Bug 1: 展开/折叠导航 | `TagHierarchyRow.vue` label 的 `@click` 触发 `select` → `handleSelectNode` emit `select-tag`，在 TagsPage 中未拦截；toggle 按钮本身逻辑正确但可能被事件冒泡影响 | `TagHierarchyRow.vue:134-142`, `TagHierarchy.vue:316-318`, `TagsPage.vue:245` |
| Bug 2: 缺少"未归属"折叠区域 | 当前"未分类"按钮替换整个树视图，而非在树底部追加折叠区域 | `TagHierarchy.vue:382-389` |
| Bug 3: 行内重命名失效 | 代码已实现但因 Bug 1 的 `@click` 先于 `@dblclick` 触发，导致 select 事件干扰 | 随 Bug 1 修复自动解决 |
| Bug 4: 添加板块重复点击导航 | 初步排查：`<Teleport to="body">` 关闭后可能残留 pointer-events；或 dialog-overlay 的 `@click.self` 残留 | `AddSectorDialog.vue:21-22` |

---

### Task 1: 修复 TagHierarchy click/dblclick 冲突 (Bug 1 + Bug 3)

**Files:**
- Modify: `front/app/features/topic-graph/components/TagHierarchyRow.vue:134-142`
- Modify: `front/app/features/topic-graph/components/TagHierarchy.vue:316-318`
- Modify: `front/app/features/tags/components/TagsPage.vue:245`

**Step 1: 给 TagHierarchy 新增 `selectable` prop，控制 click 是否触发 select**

在 `TagHierarchy.vue` 添加 prop:

```typescript
// TagHierarchy.vue — 在 defineProps 中添加
const props = defineProps<{
  feedId?: string | null
  categoryId?: string | null
  anchorDate?: string
  selectable?: boolean  // 默认 true，/tags 页面传 false
}>()
```

修改 `handleSelectNode` (line 316-318):

```typescript
function handleSelectNode(node: TagHierarchyNode) {
  if (props.selectable === false) return
  emit('select-tag', node.slug, normalizeHierarchyCategory(node.category))
}
```

**Step 2: 在 TagsPage 中传入 `selectable=false`**

修改 `TagsPage.vue:245`:

```html
<!-- 修改前 -->
<TagHierarchy />

<!-- 修改后 -->
<TagHierarchy :selectable="false" />
```

**Step 3: 修复 click/dblclick 时序冲突 — 使用 click timer 延迟**

在 `TagHierarchyRow.vue` 替换 label 按钮的事件处理 (lines 134-142):

```typescript
// TagHierarchyRow.vue — 在 <script setup> 中添加
const clickTimer = ref<ReturnType<typeof setTimeout> | null>(null)

function handleLabelClick(node: TagHierarchyNode) {
  if (clickTimer.value) return  // 双击第二次 click 忽略
  clickTimer.value = setTimeout(() => {
    clickTimer.value = null
    emit('select', node)
  }, 250)
}

function handleLabelDblClick(node: TagHierarchyNode) {
  if (clickTimer.value) {
    clearTimeout(clickTimer.value)
    clickTimer.value = null
  }
  editingValue.value = node.label
  emit('start-edit', node)
}
```

替换模板:

```html
<!-- 替换 TagHierarchyRow.vue lines 134-142 -->
<button
  v-else
  type="button"
  class="th-label"
  @click="handleLabelClick(node)"
  @dblclick="handleLabelDblClick(node)"
>
  {{ node.label }}
</button>
```

**Step 4: 验证**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

**Step 5: Commit**

```bash
git add front/app/features/topic-graph/components/TagHierarchyRow.vue front/app/features/topic-graph/components/TagHierarchy.vue front/app/features/tags/components/TagsPage.vue
git commit -m "fix: prevent TagHierarchy click/select from interfering with /tags page"
```

---

### Task 2: 添加"未归属"折叠区域 (Bug 2)

**Files:**
- Modify: `front/app/features/topic-graph/components/TagHierarchy.vue:369-541` (template 区域)

**Step 1: 添加未归属标签加载和展示逻辑**

在 `TagHierarchy.vue` 的 `<script setup>` 中，`showUnclassified` 已存在 (line 28)。修改 `loadHierarchy` 以便在非 unclassified 模式下也加载未归属标签数量:

```typescript
// TagHierarchy.vue — 新增 state
const unplacedTags = ref<TagHierarchyNode[]>([])
const showUnplaced = ref(false)

// 修改 loadHierarchy，在 response 成功后:
// 现有: nodes.value = response.data.nodes
// 新增:
if (response.data?.unplaced) {
  unplacedTags.value = response.data.unplaced
} else {
  unplacedTags.value = []
}
```

> **注意**: 如果后端 API `/topic-tags/hierarchy` 不返回 `unplaced` 字段，则需要单独调用。检查 API 返回格式后再决定。先检查 `abstractTags.ts:fetchHierarchy` 的返回类型。

**Step 2: 在模板树底部添加折叠区域**

在 `TagHierarchy.vue` 的树渲染区域 (`<!-- Tree -->` div 之后, line ~541) 插入:

```html
<!-- Unplaced tags section -->
<div v-if="unplacedTags.length > 0" class="th-unplaced-section">
  <button
    type="button"
    class="th-unplaced-toggle"
    @click="showUnplaced = !showUnplaced"
  >
    <Icon :icon="showUnplaced ? 'mdi:chevron-down' : 'mdi:chevron-right'" width="16" />
    <span class="th-unplaced-label">未归属</span>
    <span class="th-unplaced-count">{{ unplacedTags.length }} 个标签</span>
  </button>
  <div v-if="showUnplaced" class="th-unplaced-list">
    <TagHierarchyRow
      v-for="tag in unplacedTags"
      :key="tag.id"
      :node="tag"
      :depth="1"
      :editing-id="editingNodeId"
      :saving="saving"
      :watched-tag-ids="watchedTagIds"
      @start-edit="startEdit"
      @cancel-edit="cancelEdit"
      @confirm-edit="void confirmEdit()"
      @detach="requestDetach"
      @reassign="requestReassign"
      @select="handleSelectNode"
      @update:editing-value="handleUpdateEditingValue"
      @toggle-watch="toggleWatch"
    />
  </div>
</div>
```

**Step 3: 添加样式**

```css
.th-unplaced-section {
  margin-top: 1rem;
  border-top: 1px dashed rgba(255, 255, 255, 0.1);
  padding-top: 0.5rem;
}
.th-unplaced-toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
  padding: 0.5rem 0.75rem;
  border-radius: 10px;
  color: rgba(255, 255, 255, 0.5);
  font-size: 0.8rem;
  transition: background 0.2s;
}
.th-unplaced-toggle:hover { background: rgba(255, 255, 255, 0.04); }
.th-unplaced-label { font-weight: 500; color: rgba(255, 255, 255, 0.7); }
.th-unplaced-count { font-size: 0.75rem; }
.th-unplaced-list { padding-left: 0.5rem; }
```

**Step 4: 验证后端 API 是否返回未归属标签**

Run: `curl -s http://localhost:5000/api/topic-tags/hierarchy?category=event | python -m json.tool | head -20`

检查返回 JSON 是否有 `unplaced` 或类似字段。如果后端不返回，需额外任务修改后端。

**Step 5: Commit**

```bash
git add front/app/features/topic-graph/components/TagHierarchy.vue
git commit -m "feat: add collapsible unplaced tags section at bottom of hierarchy tree"
```

---

### Task 3: 修复添加板块重复点击导航 (Bug 4)

**Files:**
- Modify: `front/app/features/tags/components/AddSectorDialog.vue:20-22`
- Modify: `front/app/features/tags/components/TagsPage.vue:301-306`

**Step 1: 用浏览器复现确认根因**

在 agent-browser 中:
1. 打开 http://localhost:3000/tags
2. 点击 "添加板块" → 弹出对话框
3. 点击 "取消" 关闭
4. 再次点击 "添加板块"
5. Snapshot 观察页面变化 — 确认是页面导航还是 dialog 渲染问题

**Step 2: 根据 Step 1 结果修复**

**假设 A: Dialog overlay 关闭后 pointer-events 残留**

修改 `AddSectorDialog.vue` — dialog 关闭后确保清理:

```html
<!-- 确保使用 v-if 而非 v-show，且 overlay 的 @click.self 不会泄漏 -->
<Teleport to="body">
  <div
    v-if="true"
    class="dialog-overlay"
    @click.self="emit('cancel')"
    @keydown.escape="emit('cancel')"
  >
```

同时在 `TagsPage.vue` 中，确认 `showAddDialog = false` 在 cancel 后被正确执行:

```html
<!-- 已有: -->
<AddSectorDialog
  v-if="showAddDialog"
  @confirm="handleAddSector"
  @cancel="showAddDialog = false"
/>
```

这段逻辑看起来正确。如果问题是 `handleAddSector` 成功后 `showAddDialog.value = false` 但组件未卸载，可添加 `nextTick`:

```typescript
import { nextTick } from 'vue'

async function handleAddSector(data: { label: string; description: string }) {
  const res = await api.createSector({ ... })
  if (res.success) {
    showAddDialog.value = false
    await nextTick()
    await loadSectors()
  } else {
    sectorsError.value = res.error || '添加板块失败'
  }
}
```

**假设 B: 按钮被 focus 后 Enter 键触发了导航**

添加 `@click.stop` 防止事件冒泡:

```html
<!-- SectorList.vue:92-100 -->
<button type="button" class="sector-action-btn sector-action-btn--primary"
  @click.stop="emit('add')">
```

**Step 3: 验证**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck`
Expected: PASS

**Step 4: Commit**

```bash
git add front/app/features/tags/components/AddSectorDialog.vue front/app/features/tags/components/TagsPage.vue front/app/features/tags/components/SectorList.vue
git commit -m "fix: prevent add-sector dialog navigation issue on repeated clicks"
```

---

### Task 4: 端到端验证

**Files:** 无代码修改

**Step 1: 启动 agent-browser 完整验证**

1. 导航到 http://localhost:3000/tags
2. 验证树中 Node 展开/折叠正常（不导航）
3. 双击 Node 名称 → 验证进入编辑模式
4. 按 Enter 保存 → 验证调用 API
5. 查看树底部是否有 "未归属" 折叠区域
6. 点击 "添加板块" → 取消 → 再次点击 → 验证不再导航
7. 切换 category (事件/人物/关键词) → 验证树刷新正常
8. 打开 http://localhost:3000/topic-graph → 验证只有图谱+叙事 tab

**Step 2: 运行 lint + typecheck + build**

Run: `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build`
Expected: ALL PASS

**Step 3: Commit (如有遗漏修复)**

```bash
git add -A
git commit -m "fix: final adjustments for /tags page interaction bugs"
```

---

## 依赖关系

```
Task 1 (click/dblclick 修复)
  └── Task 2 (未归属区域，独立)
  └── Task 3 (添加板块，独立，需先浏览器复现)
  └── Task 4 (验证，依赖 1-3 完成)
```

Task 1-3 可并行执行，Task 4 最后做。

## 风险

- **Bug 4 根因未完全确认**: Task 3 的修复方案取决于浏览器复现结果，可能需要调整
- **后端 API 未归属标签**: Task 2 依赖后端返回未归属标签数据，可能需要额外后端改动
- **click timer 延迟 250ms**: Task 1 引入 250ms 单击延迟，对 UX 有轻微影响但避免了 dblclick 冲突
