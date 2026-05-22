# Tag Hierarchy Templates — Remaining Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the tag-hierarchy-templates change: cleanup scheduler refactoring, code review fixes, frontend UI, and tests.

**Architecture:** Modify the existing multi-phase cleanup scheduler (`jobs/tag_hierarchy_cleanup.go`) to add template compliance steps after Phase 3 and rewrite Phase 6 tree review. Build Vue 3 components for hierarchy config UI. Add unit tests for new hierarchy operations.

**Tech Stack:** Go + Gin + GORM (backend), Vue 3 + Nuxt 4 + TypeScript (frontend), pgvector (embedding search)

---

### Task 1: Code Review Critical Fixes

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_config.go`
- Modify: `backend-go/cmd/backfill-tag-levels/main.go`

**Step 1: Add lock around `LoadSystemDefaults`**

In `hierarchy_config.go:31`, `LoadSystemDefaults` writes to `m.templates` without holding the write lock. Add `m.mu.Lock()` / `m.mu.Unlock()`:

```go
func (m *HierarchyTemplateManager) LoadSystemDefaults() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// existing body unchanged
}
```

But this breaks `LoadFromDB` which calls `LoadSystemDefaults` while already holding the lock. Split into internal method:

```go
func (m *HierarchyTemplateManager) LoadSystemDefaults() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loadSystemDefaultsLocked()
}

func (m *HierarchyTemplateManager) loadSystemDefaultsLocked() {
	defaults := BuildAllDefaultTemplates()
	for _, t := range defaults {
		m.templates[t.TemplateKey()] = t
	}
	logging.Infof("Loaded %d default hierarchy templates", len(defaults))
}
```

Update `LoadFromDB` to call `m.loadSystemDefaultsLocked()` instead of `m.LoadSystemDefaults()`. Update `Reload` similarly.

**Step 2: Fix double dryRun check in backfill script**

In `backfill-tag-levels/main.go:142`, remove the redundant `if !dryRun` check inside `repairInvalidRelations` — the caller already gates on `!*dryRun`:

```go
func repairInvalidRelations(invalid []invalidRelationInfo, _ bool) int {
	repaired := 0
	for _, info := range invalid {
		result := database.DB.Where("parent_id = ? AND child_id = ? AND relation_type = 'abstract'",
			info.ParentID, info.ChildID).Delete(&models.TopicTagRelation{})
		// ...
	}
	return repaired
}
```

**Step 3: Verify compilation**

Run: `cd backend-go && go build ./...`
Expected: no errors

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/hierarchy_config.go backend-go/cmd/backfill-tag-levels/main.go
git commit -m "fix: lock LoadSystemDefaults, remove double dryRun check"
```

---

### Task 2: Add Template Compliance Checks to Phase 3 (tasks 7.1)

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go` (add Phase 3d/3e after Phase 3.6)
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup.go` (new `CleanupTemplateViolations`)

**Step 1: Add `CleanupTemplateViolations` to tag_cleanup.go**

```go
type TemplateViolationResult struct {
	DepthExceeded int `json:"depth_exceeded"`
	CrossCategory int `json:"cross_category"`
	PendingAdded  int `json:"pending_added"`
}

func CleanupTemplateViolations() (*TemplateViolationResult, error) {
	result := &TemplateViolationResult{}

	var relations []models.TopicTagRelation
	if err := database.DB.Where("relation_type = 'abstract'").
		Preload("Parent").Preload("Child").Find(&relations).Error; err != nil {
		return nil, fmt.Errorf("load relations: %w", err)
	}

	for _, r := range relations {
		if r.Parent == nil || r.Child == nil {
			continue
		}

		tmpl := GetHierarchyManager().GetTemplate(r.Parent.Category, "")
		if tmpl == nil {
			continue
		}

		childLevel := GetTagLevel(r.Child)
		if childLevel > tmpl.MaxLevel {
			result.DepthExceeded++
			createPendingChange(r.ChildID, r.Child.Label, "depth_exceeded", &r.ParentID, r.Parent.Label,
				fmt.Sprintf("Depth %d exceeds max %d for template %s", childLevel, tmpl.MaxLevel, tmpl.TemplateKey()))
		}

		if r.Parent.Category != r.Child.Category {
			result.CrossCategory++
			createPendingChange(r.ChildID, r.Child.Label, "cross_category", &r.ParentID, r.Parent.Label,
				fmt.Sprintf("Parent category %s != child category %s", r.Parent.Category, r.Child.Category))
		}
	}

	result.PendingAdded = result.DepthExceeded + result.CrossCategory
	return result, nil
}

func createPendingChange(tagID uint, label, changeType string, parentID *uint, parentLabel, reason string) {
	change := models.HierarchyPendingChange{
		TagID: tagID, TagLabel: label, ChangeType: changeType,
		CurrentParentID: parentID, CurrentParentLabel: parentLabel,
		Reason: reason, Status: "pending",
	}
	database.DB.Create(&change)
}
```

**Step 2: Add Phase 3d/3e to tag_hierarchy_cleanup.go**

After `// Phase 3.6` block (around line 384), insert:

```go
// Phase 3d: Template depth compliance check
v, err := topicanalysis.CleanupTemplateViolations()
if err != nil {
    logging.Errorf("Phase 3d template violation check failed: %v", err)
} else {
    summary.TemplateDepthViolations = v.DepthExceeded
    summary.TemplateCrossCategory = v.CrossCategory
    logging.Infof("Phase 3d: found %d depth-exceeded, %d cross-category violations, %d pending added",
        v.DepthExceeded, v.CrossCategory, v.PendingAdded)
}
```

**Step 3: Add fields to summary struct**

In `TagHierarchyCleanupRunSummary`, add:
```go
TemplateDepthViolations  int `json:"template_depth_violations"`
TemplateCrossCategory    int `json:"template_cross_category"`
```

**Step 4: Verify compilation**

Run: `cd backend-go && go build ./...`
Expected: no errors

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/tag_cleanup.go backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: add template compliance checks to cleanup Phase 3"
```

---

### Task 3: Add Level-Matching Check to Phase 4 Adoption (task 7.2)

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/adopt_narrower_queue_handler.go`

**Step 1: Add level-matching guard**

Find the adoption processing function (likely `processAdoptNarrowerTask`). Before establishing a parent-child relation, check that the target tag and candidate are at the same template level:

```go
targetLevel := GetTagLevel(targetTag)
candidateLevel := GetTagLevel(candidateTag)
if targetLevel != candidateLevel {
    logging.Infof("Adopt narrower: skip tag %d (L%d) adopting %d (L%d) — cross-level",
        targetTag.ID, targetLevel, candidateTag.ID, candidateLevel)
    return false, nil
}
```

Read the actual handler to find the exact insertion point.

**Step 2: Verify compilation**

Run: `cd backend-go && go build ./...`

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/adopt_narrower_queue_handler.go
git commit -m "feat: add level-matching guard to Phase 4 adoption"
```

---

### Task 4: Rewrite Phase 6 Tree Review with Template Alignment (tasks 7.3-7.7)

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go` (add Phase 6 new functions)
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go` (replace Phase 6 section)

**Step 1: Add `Phase6_CheckLevelAlignment`**

```go
func Phase6_CheckLevelAlignment(forest []*TreeNode, tmpl *CategoryHierarchyTemplate) []string {
	var issues []string
	for _, root := range forest {
		checkNodeLevelAlignment(root, tmpl, &issues)
	}
	return issues
}

func checkNodeLevelAlignment(node *TreeNode, tmpl *CategoryHierarchyTemplate, issues *[]string) {
	depth := node.Depth
	expectedLevel := ResolveLevelFromDepth(node.Tag.Category, depth)
	
	if depth+1 > tmpl.MaxLevel {
		*issues = append(*issues, fmt.Sprintf("tag %d(%s) depth %d exceeds max %d",
			node.Tag.ID, node.Tag.Label, depth+1, tmpl.MaxLevel))
	}
	
	for _, child := range node.Children {
		checkNodeLevelAlignment(child, tmpl, issues)
	}
}
```

**Step 2: Add `Phase6_DedupL1` and `Phase6_DedupL2`**

These reuse the existing `dedupL1`/`dedupL2` from `hierarchy_dedup.go` but operate on all L1/L2 tags in the forest. Add batch variants:

```go
func Phase6_DedupL1(ctx context.Context, tmpl *CategoryHierarchyTemplate) (int, error) {
	l1Tags, _ := loadExistingL1Tags(tmpl.Category, tmpl)
	merged := 0
	for i := 0; i < len(l1Tags); i++ {
		for j := i + 1; j < len(l1Tags); j++ {
			// check embedding similarity, if >= 0.90 call LLM dedup
		}
	}
	return merged, nil
}
```

**Step 3: Add `Phase6_SampleAuditLeaves`**

```go
func Phase6_SampleAuditLeaves(ctx context.Context, tmpl *CategoryHierarchyTemplate) (int, error) {
	// Load all L3 tags for this template
	// Sample 10%, for each submit LLM check of parent-child semantic fit
	// Return count of issues found
}
```

**Step 4: Replace Phase 6 in scheduler**

Replace the existing Phase 6 tree review block with:
```go
for _, category := range []string{"event", "person", "keyword"} {
	tmpl := topicanalysis.GetHierarchyManager().GetTemplate(category, "")
	if tmpl == nil { continue }
	
	l1Deduped, _ := topicanalysis.Phase6_DedupL1(ctx, tmpl)
	l2Deduped, _ := topicanalysis.Phase6_DedupL2(ctx, tmpl)
	auditIssues, _ := topicanalysis.Phase6_SampleAuditLeaves(ctx, tmpl)
	
	logging.Infof("Phase 6 (%s): L1 dedup=%d, L2 dedup=%d, leaf audit=%d issues",
		category, l1Deduped, l2Deduped, auditIssues)
}
```

**Step 5: Verify compilation**

Run: `cd backend-go && go build ./...`

**Step 6: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: rewrite Phase 6 with template-aligned review (level check, L1/L2 dedup, leaf audit)"
```

---

### Task 5: Frontend Hierarchy Config UI (tasks 9.2-9.4, 9.6-9.7)

**Files:**
- Create: `front/app/features/hierarchy-config/HierarchyConfigPage.vue`
- Create: `front/app/features/hierarchy-config/HierarchyPendingList.vue`
- Create: `front/app/features/hierarchy-config/RebuildTrigger.vue`
- Modify: `front/app/components/dialog/GlobalSettingsDialog.vue` (add nav entry)

**Step 1: Create `HierarchyConfigPage.vue`**

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import type { HierarchyTemplate, HierarchyLevel } from '~/api/hierarchyConfig'

const api = useHierarchyConfigApi()
const templates = ref<HierarchyTemplate[]>([])
const selectedTemplate = ref<string>('')
const saving = ref(false)
const saved = ref(false)

onMounted(async () => {
  const result = await api.getConfig()
  if (result.success && result.data) {
    templates.value = result.data.templates
  }
})

function selectTemplate(key: string) {
  selectedTemplate.value = key
}

const selectedLevels = computed(() => {
  const t = templates.value.find(t => `${t.category}${t.sub_type ? ':' + t.sub_type : ''}` === selectedTemplate.value)
  return t?.levels ?? []
})

async function save() {
  saving.value = true
  const result = await api.updateConfig(templates.value, 'UI update')
  saved.value = result.success
  saving.value = false
  if (result.success) setTimeout(() => saved.value = false, 3000)
}
</script>

<template>
  <div class="p-6">
    <h2 class="text-xl font-bold mb-4">层级配置</h2>
    <div class="flex gap-4">
      <div class="w-48">
        <button v-for="t in templates" :key="templateKey(t)"
          @click="selectTemplate(templateKey(t))"
          :class="{ 'bg-primary': selectedTemplate === templateKey(t) }"
          class="block w-full text-left p-2 rounded mb-1 hover:bg-gray-100">
          {{ t.category }}{{ t.sub_type ? ` (${t.sub_type})` : '' }}
        </button>
      </div>
      <div class="flex-1">
        <div v-for="level in selectedLevels" :key="level.level" class="border rounded p-3 mb-2">
          <div class="flex items-center gap-2">
            <span class="font-bold">L{{ level.level }}</span>
            <input v-model="level.name" class="border rounded px-2 py-1 flex-1" />
            <label class="text-sm">
              <input type="checkbox" v-model="level.is_leaf" /> 叶子节点
            </label>
          </div>
          <input v-model="level.description" class="border rounded px-2 py-1 w-full mt-1 text-sm" />
        </div>
      </div>
    </div>
    <button @click="save" :disabled="saving" class="mt-4 px-4 py-2 bg-green-600 text-white rounded">
      {{ saving ? '保存中...' : saved ? '已保存' : '保存' }}
    </button>
  </div>
</template>
```

**Step 2: Create `HierarchyPendingList.vue`**

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useHierarchyConfigApi } from '~/api/hierarchyConfig'
import type { HierarchyPendingChange } from '~/api/hierarchyConfig'

const api = useHierarchyConfigApi()
const pending = ref<HierarchyPendingChange[]>([])
const statusFilter = ref('pending')

onMounted(load)

async function load() {
  const result = await api.getPending(statusFilter.value)
  if (result.success && result.data) pending.value = result.data
}

async function triggerRebuild(category?: string) {
  const result = await api.triggerRebuild(category)
  if (result.success) await load()
}
</script>

<template>
  <div class="p-6">
    <h2 class="text-xl font-bold mb-4">待处理标签</h2>
    <select v-model="statusFilter" @change="load" class="border rounded px-2 py-1 mb-4">
      <option value="pending">待处理</option>
      <option value="resolved">已处理</option>
    </select>
    <div v-for="item in pending" :key="item.id" class="border rounded p-3 mb-2">
      <span class="font-bold">{{ item.tag_label }}</span>
      <span class="text-sm text-gray-500 ml-2">[{{ item.change_type }}]</span>
      <p class="text-sm mt-1">{{ item.reason }}</p>
    </div>
    <button @click="triggerRebuild()" class="mt-4 px-4 py-2 bg-blue-600 text-white rounded">
      重新整理
    </button>
  </div>
</template>
```

**Step 3: Create `RebuildTrigger.vue`**

Incorporate into HierarchyPendingList.vue or as standalone component with category selector + dry_run toggle + progress display via WebSocket.

**Step 4: Add navigation entry**

In `GlobalSettingsDialog.vue`, find the settings nav section and add:
```vue
<button @click="activeTab = 'hierarchy'" class="...">层级配置</button>
```
And add conditional rendering for `activeTab === 'hierarchy'` showing `HierarchyConfigPage`.

**Step 5: Verify build**

Run: `cd front && pnpm exec nuxi typecheck && pnpm build`
Expected: no errors

**Step 6: Commit**

```bash
git add front/app/features/hierarchy-config/ front/app/components/dialog/GlobalSettingsDialog.vue
git commit -m "feat: add hierarchy config frontend (config page, pending list, nav entry)"
```

---

### Task 6: Unit Tests (tasks 10.1-10.5)

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/hierarchy_template_test.go`
- Create: `backend-go/internal/domain/topicanalysis/hierarchy_placement_test.go`
- Create: `backend-go/internal/domain/topicanalysis/hierarchy_config_test.go`
- Create: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go`

**Step 1: `hierarchy_template_test.go`**

Test template loading, default fallback, level depth resolution:

```go
func TestBuildAllDefaultTemplates(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()
	
	tmpl := mgr.GetTemplate("event", "")
	if tmpl == nil || tmpl.MaxLevel != 3 {
		t.Fatal("event template not loaded or wrong max level")
	}
	if tmpl.GetLeafLevel() != 3 {
		t.Fatalf("event leaf level expected 3, got %d", tmpl.GetLeafLevel())
	}
}

func TestResolveLevelFromDepth(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()
	
	if lvl := ResolveLevelFromDepth("event", 0); lvl != 1 {
		t.Fatalf("depth=0 event expected L1, got L%d", lvl)
	}
	if lvl := ResolveLevelFromDepth("event", 2); lvl != 3 {
		t.Fatalf("depth=2 event expected L3, got L%d", lvl)
	}
	if lvl := ResolveLevelFromDepth("event", 5); lvl != 3 {
		t.Fatalf("depth=5 event should cap at L3, got L%d", lvl)
	}
}

func TestPersonTemplate(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()
	
	tmpl := mgr.GetTemplate("person", "")
	if tmpl == nil || tmpl.MaxLevel != 2 {
		t.Fatal("person template not loaded or wrong max level")
	}
}

func TestKeywordSubTypeTemplates(t *testing.T) {
	mgr := GetHierarchyManager()
	mgr.LoadSystemDefaults()
	
	if tmpl := mgr.GetTemplate("keyword", "technology"); tmpl == nil {
		t.Fatal("keyword:technology template not found")
	}
	if tmpl := mgr.GetTemplate("keyword", "company_business"); tmpl == nil {
		t.Fatal("keyword:company_business template not found")
	}
}
```

**Step 2: `hierarchy_config_test.go`**

Test config save/load, version increment, impact preview:

```go
func TestHierarchyConfigImpactPreview(t *testing.T) {
	// Create some test tags with depth > max level
	// Call previewConfigImpact
	// Verify DepthExceeded count
}
```

**Step 3: Run tests**

Run: `cd backend-go && go test ./internal/domain/topicanalysis/ -v -run "Test.*Template|Test.*Config"`

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/*_test.go
git commit -m "test: add unit tests for hierarchy templates, config, and placement"
```

---

### Task 7: Final Verification

**Step 1: Full backend build + test**

```bash
cd backend-go && go build ./... && go test ./...
```

**Step 2: Frontend typecheck + build**

```bash
cd front && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build
```

**Step 3: Update tasks.md**

Mark remaining tasks complete in `openspec/changes/tag-hierarchy-templates/tasks.md`.

**Step 4: Commit final state**

```bash
git add -A
git commit -m "feat: complete tag-hierarchy-templates implementation"
```
