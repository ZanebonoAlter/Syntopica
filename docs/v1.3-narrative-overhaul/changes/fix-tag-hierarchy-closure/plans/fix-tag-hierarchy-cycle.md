# Fix Tag Hierarchy Cycles from Merge Operations

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent `MergeTags` from creating tag hierarchy cycles and from transferring abstract relations onto non-abstract topic tags.

**Architecture:** Add cycle detection and abstract-target guard into `migrateTagRelations` (the function `MergeTags` calls to transfer tag relations). Also add a one-shot SQL cleanup for existing bad data. No new files — bug fix in existing infrastructure.

**Tech Stack:** Go, GORM, PostgreSQL, `wouldCreateCycle` (exists in `abstract_tag_tree.go`)

**Root Cause:** When an abstract tag (Source="abstract") is merged into a non-abstract tag (Source="llm"), `migrateTagRelations` blindly transfers all parent/child relations to the target. Since non-abstract tags lack the abstract guards in `linkAbstractParentChild`, subsequent merges compound the damage until cycles form.

**Current State:**
- 59 non-abstract→non-abstract abstract relations in DB
- 1 confirmed cycle: 64399 → 68315 → 71421 → 64399 ("DeepSeek V4 发布" family)

---

### Task 1: Clean Up Existing Bad Relations in DB

**Files:**
- No file changes — direct DB operation

**Step 1: Delete relations where BOTH parent and child are non-abstract**

Run the SQL to remove all `topic_tag_relations` where the parent side is not abstract:

```sql
DELETE FROM topic_tag_relations r
USING topic_tags p
WHERE r.parent_id = p.id
  AND r.relation_type = 'abstract'
  AND p.status = 'active'
  AND p.kind != 'abstract'
  AND p.source != 'abstract';
```

**Step 2: Verify deletion count**

Run: count of remaining non-abstract parent relations should be 0

```sql
SELECT COUNT(*) FROM topic_tag_relations r
JOIN topic_tags p ON r.parent_id = p.id
WHERE r.relation_type = 'abstract'
  AND p.status = 'active'
  AND p.kind != 'abstract'
  AND p.source != 'abstract';
```

Expected: 0

**Step 3: Verify the DeepSeek V4 cycle is broken**

```sql
SELECT r.id, r.parent_id, p.label, r.child_id, c.label
FROM topic_tag_relations r
JOIN topic_tags p ON r.parent_id = p.id
JOIN topic_tags c ON r.child_id = c.id
WHERE r.id IN (28472, 39965, 41144);
```

Expected: Only 28472 remains (64399→68315 involves 64399 as non-abstract parent, deleted by step 1).

Wait — 28472 has parent 64399 (topic, llm, NOT abstract). It should be deleted too. After cleanup, none of the three cycle edges should exist.

---

### Task 2: Test — Cycle Prevention in migrateTagRelations

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/embedding_merge_test.go`

**Step 1: Write tests for migrateTagRelations cycle prevention**

```go
package topicanalysis

import (
	"testing"

	"my-robot-backend/internal/domain/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db := setupEmbeddingTestDB(t) // reuse existing test helper from embedding_test.go
	require.NotNil(t, db)
	return db
}

// Test: migrateTagRelations should reject creating a cycle
func TestMigrateTagRelations_RejectsCycle(t *testing.T) {
	db := setupTestDB(t)

	// Create three tags
	root := &models.TopicTag{ID: 1, Label: "Root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	mid := &models.TopicTag{ID: 2, Label: "Mid", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	leaf := &models.TopicTag{ID: 3, Label: "Leaf", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(mid).Error)
	require.NoError(t, db.Create(leaf).Error)

	// Build: root → mid → leaf (root is parent of mid, mid is parent of leaf)
	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: mid.ID, RelationType: "abstract"}).Error)
	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: mid.ID, ChildID: leaf.ID, RelationType: "abstract"}).Error)

	// Now add leaf → root which would create a cycle
	require.NoError(t, db.Create(&models.TopicTagRelation{ParentID: leaf.ID, ChildID: root.ID, RelationType: "abstract"}).Error)

	// Simulate merging leaf into mid
	// leaf has parent relations: leaf is parent of root (cycle edge)
	// When migrating leaf→mid, the relation parent_id=leaf→root should be REJECTED
	// because adding mid→root would create a cycle (root→mid→root)
	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, leaf.ID, mid.ID)
	})
	assert.NoError(t, err)

	// Verify the cycle edge was deleted, not transferred
	var count int64
	db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND child_id = ?", mid.ID, root.ID).
		Count(&count)
	assert.Equal(t, int64(0), count, "cycle edge should not be transferred")
}

// Test: migrateTagRelations should not transfer relations to non-abstract targets
func TestMigrateTagRelations_SkipsNonAbstractTarget(t *testing.T) {
	db := setupTestDB(t)

	abstractParent := &models.TopicTag{ID: 10, Label: "AbstractTag", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := &models.TopicTag{ID: 11, Label: "Child", Category: "event", Kind: "topic", Source: "llm", Status: "active"}
	nonAbstractTarget := &models.TopicTag{ID: 12, Label: "NormalTag", Category: "event", Kind: "topic", Source: "llm", Status: "active"}

	require.NoError(t, db.Create(abstractParent).Error)
	require.NoError(t, db.Create(child).Error)
	require.NoError(t, db.Create(nonAbstractTarget).Error)

	// Abstract tag is parent of child
	require.NoError(t, db.Create(&models.TopicTagRelation{
		ParentID: abstractParent.ID, ChildID: child.ID, RelationType: "abstract",
	}).Error)

	// Merge abstractParent into nonAbstractTarget
	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, abstractParent.ID, nonAbstractTarget.ID)
	})
	assert.NoError(t, err)

	// Verify the relation was NOT transferred to the non-abstract target
	var count int64
	db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ?", nonAbstractTarget.ID).
		Count(&count)
	assert.Equal(t, int64(0), count, "relations should not transfer to non-abstract target")
}

// Test: migrateTagRelations correctly migrates to abstract target
func TestMigrateTagRelations_MigratesToAbstractTarget(t *testing.T) {
	db := setupTestDB(t)

	source := &models.TopicTag{ID: 20, Label: "SourceAbstract", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := &models.TopicTag{ID: 21, Label: "ChildTag", Category: "event", Kind: "topic", Source: "llm", Status: "active"}
	target := &models.TopicTag{ID: 22, Label: "TargetAbstract", Category: "event", Kind: "event", Source: "abstract", Status: "active"}

	require.NoError(t, db.Create(source).Error)
	require.NoError(t, db.Create(child).Error)
	require.NoError(t, db.Create(target).Error)

	require.NoError(t, db.Create(&models.TopicTagRelation{
		ParentID: source.ID, ChildID: child.ID, RelationType: "abstract",
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		return migrateTagRelations(tx, source.ID, target.ID)
	})
	assert.NoError(t, err)

	var count int64
	db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND child_id = ?", target.ID, child.ID).
		Count(&count)
	assert.Equal(t, int64(1), count, "relation should transfer to abstract target")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/topicanalysis -run "TestMigrateTagRelations" -v`
Expected: FAIL — new tests fail because migrateTagRelations still transfers all relations blindly

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/embedding_merge_test.go
git commit -m "test: add failing tests for migrateTagRelations cycle and non-abstract guard"
```

---

### Task 3: Fix migrateTagRelations in embedding.go

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go:582-640`

**Step 1: Add abstract target guard and cycle detection to migrateTagRelations**

Replace the `migrateTagRelations` function:

```go
func migrateTagRelations(tx *gorm.DB, sourceTagID, targetTagID uint) error {
	var targetTag models.TopicTag
	if err := tx.First(&targetTag, targetTagID).Error; err != nil {
		return fmt.Errorf("load target tag %d: %w", targetTagID, err)
	}
	isAbstractTarget := targetTag.Kind == "abstract" || targetTag.Source == "abstract"

	// Source as parent: relations where parent_id = source
	var parentRelations []models.TopicTagRelation
	if err := tx.Where("parent_id = ?", sourceTagID).Find(&parentRelations).Error; err != nil {
		return fmt.Errorf("find source parent relations: %w", err)
	}
	for _, rel := range parentRelations {
		childID := rel.ChildID
		if childID == targetTagID {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete self-referencing parent relation %d: %w", rel.ID, err)
			}
			continue
		}
		if !isAbstractTarget {
			// Target is not abstract — don't transfer abstract parent relations onto it
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete non-transferable parent relation %d: %w", rel.ID, err)
			}
			continue
		}
		var existing int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ?", targetTagID, childID).
			Count(&existing)
		if existing > 0 {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete duplicate parent relation %d: %w", rel.ID, err)
			}
			continue
		}
		// Cycle detection for abstract→abstract migration
		wouldCycle, err := wouldCreateCycle(tx, targetTagID, childID)
		if err != nil {
			return fmt.Errorf("cycle check migrating parent relation %d→%d: %w", sourceTagID, childID, err)
		}
		if wouldCycle {
			logging.Warnf("migrateTagRelations: skipping cycle relation %d→%d (would create cycle)", targetTagID, childID)
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete cyclic parent relation %d: %w", rel.ID, err)
			}
			continue
		}
		if err := tx.Model(&rel).Update("parent_id", targetTagID).Error; err != nil {
			return fmt.Errorf("migrate parent relation %d: %w", rel.ID, err)
		}
	}

	// Source as child: relations where child_id = source
	var childRelations []models.TopicTagRelation
	if err := tx.Where("child_id = ?", sourceTagID).Find(&childRelations).Error; err != nil {
		return fmt.Errorf("find source child relations: %w", err)
	}
	for _, rel := range childRelations {
		parentID := rel.ParentID
		if parentID == targetTagID {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete self-referencing child relation %d: %w", rel.ID, err)
			}
			continue
		}
		if !isAbstractTarget {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete non-transferable child relation %d: %w", rel.ID, err)
			}
			continue
		}
		var existing int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ?", parentID, targetTagID).
			Count(&existing)
		if existing > 0 {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete duplicate child relation %d: %w", rel.ID, err)
			}
			continue
		}
		wouldCycle, err := wouldCreateCycle(tx, parentID, targetTagID)
		if err != nil {
			return fmt.Errorf("cycle check migrating child relation %d→%d: %w", parentID, sourceTagID, err)
		}
		if wouldCycle {
			logging.Warnf("migrateTagRelations: skipping cycle relation %d→%d (would create cycle)", parentID, targetTagID)
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete cyclic child relation %d: %w", rel.ID, err)
			}
			continue
		}
		if err := tx.Model(&rel).Update("child_id", targetTagID).Error; err != nil {
			return fmt.Errorf("migrate child relation %d: %w", rel.ID, err)
		}
	}

	return nil
}
```

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/domain/topicanalysis -run "TestMigrateTagRelations" -v`
Expected: PASS

**Step 3: Run broad tests to check for regressions**

Run: `go test ./internal/domain/topicanalysis/... -v`
Expected: ALL tests pass

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/embedding.go
git commit -m "fix: prevent tag hierarchy cycles in migrateTagRelations

- Add cycle detection via wouldCreateCycle before transferring relations
- Skip relation transfer when target tag is not abstract (kind!=abstract && source!=abstract)
- Non-transferable relations are deleted instead of being transferred"
```

---

### Task 4: Quick Manual Verification

**Step 1: Delete existing bad relations**

```bash
docker exec zanebono-rssreader-pgvector psql -U postgres -d rss_reader -c "DELETE FROM topic_tag_relations r USING topic_tags p WHERE r.parent_id = p.id AND r.relation_type = 'abstract' AND p.status = 'active' AND p.kind != 'abstract' AND p.source != 'abstract';"
```

**Step 2: Verify the DeepSeek V4 cycle is gone**

```bash
docker exec zanebono-rssreader-pgvector psql -U postgres -d rss_reader -c "SELECT r.id, r.parent_id, p.label, r.child_id, c.label FROM topic_tag_relations r JOIN topic_tags p ON r.parent_id = p.id JOIN topic_tags c ON r.child_id = c.id WHERE r.id IN (28472, 39965, 41144);"
```

Expected: 0 rows (all three edges involve non-abstract parent 64399 or 71421)

**Step 3: Rebuild and restart backend**

```bash
cd backend-go && go build ./... && go test ./...
```

---

### Summary

| # | Task | Files | Key Change |
|---|------|-------|------------|
| 1 | DB cleanup | SQL only | Delete 59 existing non-abstract→non-abstract relations |
| 2 | Failing tests | `embedding_merge_test.go` (new) | 3 test cases for cycle + abstract guard |
| 3 | Fix code | `embedding.go:582-640` | Add `wouldCreateCycle` + abstract target check |
| 4 | Verify | DB + `go test` | Confirm cycle broken, tests pass |
