# Thread Independent Storage & Lineage — Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Give every narrative thread a persistent database identity, enable cross-day lineage chaining via `prev_thread_id`, and build frontend views for thread lineage timeline and board-level Gantt chart.

**Architecture:** New `daily_report_threads` table stores threads as independent rows with self-referencing `prev_thread_id`. Migration extracts existing JSON thread data. `matchPreviousThreads()` is updated to assign `prev_thread_id` using DB IDs. Two new API endpoints serve lineage and timeline data. Frontend adds `ThreadLineagePanel` (side panel in newspaper modal) and `BoardThreadBrowser` (Gantt chart view).

**Tech Stack:** Go/Gin/GORM (backend), PostgreSQL with recursive CTE, Vue 3 + TypeScript (frontend), Tailwind CSS.

**Change artifacts:** `openspec/changes/thread-independent-storage-and-lineage/`

---

## Task Group A: Database Migration (backend-only, independent)

### Task 1: Add migration — create `daily_report_threads` table

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`

**Step 1: Add migration entry**

Append a new migration `20260529_0001` to the `postgresMigrations()` slice, after the last existing entry (`20260528_0003`):

```go
{
    Version:     "20260529_0001",
    Description: "Create daily_report_threads table for independent thread storage.",
    Up: func(db *gorm.DB) error {
        stmts := []string{
            `CREATE TABLE IF NOT EXISTS daily_report_threads (
                id SERIAL PRIMARY KEY,
                report_id INTEGER NOT NULL REFERENCES board_daily_reports(id) ON DELETE CASCADE,
                section_id INTEGER NOT NULL REFERENCES daily_report_sections(id) ON DELETE CASCADE,
                title TEXT NOT NULL,
                summary TEXT,
                status VARCHAR(20) NOT NULL DEFAULT 'emerging',
                tag_ids JSONB DEFAULT '[]'::jsonb,
                confidence DOUBLE PRECISION DEFAULT 0,
                prev_thread_id INTEGER REFERENCES daily_report_threads(id) ON DELETE SET NULL,
                created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
            )`,
            `CREATE INDEX IF NOT EXISTS idx_daily_report_threads_report_id ON daily_report_threads(report_id)`,
            `CREATE INDEX IF NOT EXISTS idx_daily_report_threads_section_id ON daily_report_threads(section_id)`,
            `CREATE INDEX IF NOT EXISTS idx_daily_report_threads_prev_thread_id ON daily_report_threads(prev_thread_id) WHERE prev_thread_id IS NOT NULL`,
        }
        for _, s := range stmts {
            if err := db.Exec(s).Error; err != nil {
                return fmt.Errorf("create daily_report_threads: %w", err)
            }
        }
        return nil
    },
},
```

**Step 2: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add backend-go/internal/platform/database/postgres_migrations.go
git commit -m "feat(daily-report): add migration for daily_report_threads table"
```

---

### Task 2: Add migration — extract JSON threads into new table

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`

**Step 1: Add migration entry**

Append migration `20260529_0002` after the previous one. Map JSON field names: `related_tag_ids` → `tag_ids`, skip `related_article_ids` and `parent_thread_id`.

```go
{
    Version:     "20260529_0002",
    Description: "Migrate existing thread data from daily_report_sections.threads JSONB to daily_report_threads rows.",
    Up: func(db *gorm.DB) error {
        // Only proceed if the threads column still exists
        var colExists bool
        if err := db.Raw(`SELECT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'daily_report_sections' AND column_name = 'threads'
        )`).Scan(&colExists).Error; err != nil {
            return fmt.Errorf("check threads column: %w", err)
        }
        if !colExists {
            return nil // already migrated
        }

        err := db.Exec(`
            INSERT INTO daily_report_threads (report_id, section_id, title, summary, status, tag_ids, confidence, prev_thread_id, created_at)
            SELECT
                s.report_id,
                s.id,
                COALESCE(t->>'title', ''),
                t->>'summary',
                COALESCE(t->>'status', 'emerging'),
                COALESCE(t->'related_tag_ids', '[]'::jsonb),
                COALESCE((t->>'confidence')::double precision, 0),
                NULL,
                s.created_at
            FROM daily_report_sections s
            CROSS JOIN jsonb_array_elements(s.threads) AS t
            WHERE s.threads IS NOT NULL
              AND jsonb_array_length(s.threads) > 0
        `).Error
        if err != nil {
            return fmt.Errorf("migrate threads JSONB to rows: %w", err)
        }
        return nil
    },
},
```

**Step 2: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add backend-go/internal/platform/database/postgres_migrations.go
git commit -m "feat(daily-report): add migration to extract JSON threads into daily_report_threads rows"
```

---

### Task 3: Add migration — drop `threads` column from sections

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`

**Step 1: Add migration entry**

Append migration `20260529_0003`:

```go
{
    Version:     "20260529_0003",
    Description: "Drop threads JSONB column from daily_report_sections after migration.",
    Up: func(db *gorm.DB) error {
        var colExists bool
        if err := db.Raw(`SELECT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'daily_report_sections' AND column_name = 'threads'
        )`).Scan(&colExists).Error; err != nil {
            return fmt.Errorf("check threads column: %w", err)
        }
        if !colExists {
            return nil
        }
        if err := db.Exec(`ALTER TABLE daily_report_sections DROP COLUMN threads`).Error; err != nil {
            return fmt.Errorf("drop threads column: %w", err)
        }
        return nil
    },
},
```

**Step 2: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add backend-go/internal/platform/database/postgres_migrations.go
git commit -m "feat(daily-report): add migration to drop threads column from daily_report_sections"
```

---

## Task Group B: Backend Model & Repository (depends on Group A)

### Task 4: Add `DailyReportThread` GORM model and update `DailyReportSection`

**Files:**
- Modify: `backend-go/internal/domain/daily_report/models.go`

**Step 1: Add `DailyReportThread` struct and update `DailyReportSection`**

After `DailyReportSection` struct (line ~51), add:

```go
// DailyReportThread — one narrative thread, stored independently
type DailyReportThread struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	ReportID     uint   `gorm:"index;not null" json:"report_id"`
	SectionID    uint   `gorm:"index;not null" json:"section_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Status       string `gorm:"size:20;default:emerging" json:"status"`
	TagIDs       JSON   `gorm:"type:jsonb" json:"tag_ids"`
	Confidence   float64 `gorm:"default:0" json:"confidence"`
	PrevThreadID *uint  `json:"prev_thread_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (DailyReportThread) TableName() string {
	return "daily_report_threads"
}
```

In `DailyReportSection`, replace:
```go
Threads       JSON      `gorm:"type:jsonb" json:"threads"`
```
with:
```go
Threads       []DailyReportThread `gorm:"foreignKey:SectionID" json:"threads,omitempty"`
```

**Step 2: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add backend-go/internal/domain/daily_report/models.go
git commit -m "feat(daily-report): add DailyReportThread GORM model, update section association"
```

---

### Task 5: Add thread repository functions

**Files:**
- Modify: `backend-go/internal/domain/daily_report/repository.go`

**Step 1: Add repository functions**

After the existing functions, add:

```go
// SaveThreads persists a batch of DailyReportThread rows.
func SaveThreads(reportID, sectionID uint, threads []DailyReportThread) error {
	for i := range threads {
		threads[i].ReportID = reportID
		threads[i].SectionID = sectionID
	}
	return database.DB.Create(&threads).Error
}

// GetThreadsBySection returns all threads for a section, ordered by id.
func GetThreadsBySection(sectionID uint) ([]DailyReportThread, error) {
	var threads []DailyReportThread
	err := database.DB.Where("section_id = ?", sectionID).Order("id ASC").Find(&threads).Error
	return threads, err
}

// GetThreadsByReport returns all threads for a report, ordered by section_id, id.
func GetThreadsByReport(reportID uint) ([]DailyReportThread, error) {
	var threads []DailyReportThread
	err := database.DB.Where("report_id = ?", reportID).Order("section_id ASC, id ASC").Find(&threads).Error
	return threads, err
}

// GetThreadByID returns a single thread by its primary key.
func GetThreadByID(id uint) (*DailyReportThread, error) {
	var thread DailyReportThread
	err := database.DB.First(&thread, id).Error
	if err != nil {
		return nil, fmt.Errorf("thread %d not found: %w", id, err)
	}
	return &thread, nil
}

// DeleteThreadsByReport deletes all threads for a report.
func DeleteThreadsByReport(reportID uint) error {
	return database.DB.Where("report_id = ?", reportID).Delete(&DailyReportThread{}).Error
}
```

**Step 2: Update `GetReportByID` to use nested Preload**

Change:
```go
Preload("Sections", func(db *gorm.DB) *gorm.DB {
    return db.Order("cluster_index ASC")
}).
```
to:
```go
Preload("Sections.Threads", func(db *gorm.DB) *gorm.DB {
    return db.Order("id ASC")
}).
Preload("Sections", func(db *gorm.DB) *gorm.DB {
    return db.Order("cluster_index ASC")
}).
```

**Step 3: Update `SaveReport` to handle threads**

The `SaveReport` function needs to accept thread data and handle thread persistence. Change its signature to:

```go
func SaveReport(report *BoardDailyReport, sections []DailyReportSection, threadBatches [][]DailyReportThread) error {
```

In the upsert branch (when existing report is found), add before deleting old sections:
```go
// Nullify downstream prev_thread_id references before deleting old threads
if err := tx.Model(&DailyReportThread{}).
    Where("prev_thread_id IN (SELECT id FROM daily_report_threads WHERE report_id = ?)", existing.ID).
    Update("prev_thread_id", nil).Error; err != nil {
    return fmt.Errorf("nullify downstream prev_thread_id: %w", err)
}
// Delete old threads
if err := tx.Where("report_id = ?", existing.ID).Delete(&DailyReportThread{}).Error; err != nil {
    return fmt.Errorf("delete old threads: %w", err)
}
```

After inserting new sections (both create and update branches), add:
```go
// Save threads for each section
for secIdx, sec := range sections {
    if secIdx < len(threadBatches) && len(threadBatches[secIdx]) > 0 {
        if err := SaveThreads(report.ID, sec.ID, threadBatches[secIdx]); err != nil {
            return fmt.Errorf("save threads for section %d: %w", secIdx, err)
        }
    }
}
```

**Step 4: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: May fail because `generateSingleBoard` caller not updated yet — that's OK, fix in Task 7.

**Step 5: Commit**

```bash
git add backend-go/internal/domain/daily_report/repository.go
git commit -m "feat(daily-report): add thread repository functions, update SaveReport and GetReportByID"
```

---

### Task 6: Add lineage and timeline query functions

**Files:**
- Modify: `backend-go/internal/domain/daily_report/repository.go`

**Step 1: Add `GetThreadLineage`**

```go
// ThreadLineageNode represents a thread in a lineage chain with its report date.
type ThreadLineageNode struct {
	DailyReportThread
	PeriodDate   time.Time `json:"period_date"`
	ClusterLabel string    `json:"cluster_label"`
}

// GetThreadLineage fetches the full lineage chain for a thread using recursive CTE.
// It walks backward to the root, then forward to find all descendants.
func GetThreadLineage(threadID uint) ([]ThreadLineageNode, error) {
	var nodes []ThreadLineageNode
	err := database.DB.Raw(`
		WITH RECURSIVE ancestors AS (
			-- Start from the given thread, walk back to root
			SELECT t.*, bdr.period_date, ds.cluster_label
			FROM daily_report_threads t
			JOIN board_daily_reports bdr ON bdr.id = t.report_id
			JOIN daily_report_sections ds ON ds.id = t.section_id
			WHERE t.id = ?

			UNION ALL

			SELECT parent.*, bdr.period_date, ds.cluster_label
			FROM daily_report_threads parent
			JOIN ancestors a ON a.prev_thread_id = parent.id
			JOIN board_daily_reports bdr ON bdr.id = parent.report_id
			JOIN daily_report_sections ds ON ds.id = parent.section_id
		),
		root AS (
			SELECT * FROM ancestors ORDER BY period_date ASC LIMIT 1
		),
		chain AS (
			-- Walk forward from root to find all descendants
			SELECT t.*, bdr.period_date, ds.cluster_label
			FROM root r
			JOIN daily_report_threads t ON t.id = r.id
			JOIN board_daily_reports bdr ON bdr.id = t.report_id
			JOIN daily_report_sections ds ON ds.id = t.section_id

			UNION ALL

			SELECT child.*, bdr.period_date, ds.cluster_label
			FROM daily_report_threads child
			JOIN chain c ON child.prev_thread_id = c.id
			JOIN board_daily_reports bdr ON bdr.id = child.report_id
			JOIN daily_report_sections ds ON ds.id = child.section_id
		)
		SELECT * FROM chain ORDER BY period_date ASC
	`, threadID).Scan(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("get thread lineage: %w", err)
	}
	return nodes, nil
}
```

**Step 2: Add `GetBoardThreadTimeline`**

```go
// GetBoardThreadTimeline fetches all threads for a board within a date range,
// joining period_date and cluster_label for Gantt chart display.
func GetBoardThreadTimeline(boardID uint, days int) ([]ThreadLineageNode, error) {
	if days <= 0 {
		days = 30
	}
	if days > 90 {
		days = 90
	}
	var nodes []ThreadLineageNode
	err := database.DB.Raw(`
		SELECT t.*, bdr.period_date, ds.cluster_label
		FROM daily_report_threads t
		JOIN board_daily_reports bdr ON bdr.id = t.report_id
		JOIN daily_report_sections ds ON ds.id = t.section_id
		WHERE bdr.semantic_board_id = ?
		  AND bdr.period_date >= CURRENT_DATE - ? * INTERVAL '1 day'
		  AND bdr.status = 'completed'
		ORDER BY t.prev_thread_id NULLS FIRST, bdr.period_date ASC, t.id ASC
	`, boardID, days).Scan(&nodes).Error
	if err != nil {
		return nil, fmt.Errorf("get board thread timeline: %w", err)
	}
	return nodes, nil
}
```

**Step 3: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: PASS (these are new functions, no callers yet)

**Step 4: Commit**

```bash
git add backend-go/internal/domain/daily_report/repository.go
git commit -m "feat(daily-report): add GetThreadLineage and GetBoardThreadTimeline queries"
```

---

## Task Group C: Backend Generator & Handler (depends on Group B)

### Task 7: Update generator — matchPreviousThreads, getPrevThreadSummaries, findPreviousReport, GenerateDailyReport

**Files:**
- Modify: `backend-go/internal/domain/daily_report/generator.go`
- Modify: `backend-go/internal/domain/daily_report/handler.go` (caller)

**Step 1: Update `findPreviousReport` to Preload threads**

Change the Preload call from:
```go
Preload("Sections").
```
to:
```go
Preload("Sections.Threads", func(db *gorm.DB) *gorm.DB {
    return db.Order("id ASC")
}).
```

**Step 2: Update `matchPreviousThreads` signature and logic**

Change function signature to:
```go
func matchPreviousThreads(threads []Thread, prevThreads []DailyReportThread, cluster ClusterGroup) {
```

Replace the JSON-unmarshaling block (extracting prevThreadList from sections) with using the provided `prevThreads` parameter directly. Add `PrevThreadID` assignment:

```go
func matchPreviousThreads(threads []Thread, prevThreads []DailyReportThread, cluster ClusterGroup) {
	if len(threads) == 0 || len(prevThreads) == 0 {
		return
	}

	for i := range threads {
		th := &threads[i]
		bestMatchIdx := -1
		bestOverlap := 0

		for j, prevTh := range prevThreads {
			var prevTagIDs []uint
			if prevTh.TagIDs != nil {
				_ = json.Unmarshal(prevTh.TagIDs, &prevTagIDs)
			}
			overlap := countTagOverlap(th.TagIDs, prevTagIDs)
			if overlap > bestOverlap {
				bestOverlap = overlap
				bestMatchIdx = j
			}
		}

		if bestMatchIdx >= 0 && bestOverlap > 0 {
			if th.Status == "emerging" {
				th.Status = "continuing"
			}
			// Assign prev_thread_id from the matched previous thread's DB ID
			prevID := prevThreads[bestMatchIdx].ID
			th.PrevThreadID = &prevID
		}
	}
}
```

**Step 3: Update `getPrevThreadSummaries` to read from GORM association**

Change signature and implementation:
```go
func getPrevThreadSummaries(prevReport BoardDailyReport, cluster ClusterGroup) ([]string, []DailyReportThread) {
	clusterTagSet := make(map[uint]bool, len(cluster.TagIDs))
	for _, id := range cluster.TagIDs {
		clusterTagSet[id] = true
	}

	var summaries []string
	var matchedThreads []DailyReportThread
	for _, section := range prevReport.Sections {
		for _, th := range section.Threads {
			var tagIDs []uint
			if th.TagIDs != nil {
				_ = json.Unmarshal(th.TagIDs, &tagIDs)
			}
			for _, tagID := range tagIDs {
				if clusterTagSet[tagID] {
					summaries = append(summaries, fmt.Sprintf("%s: %s", th.Title, th.Summary))
					matchedThreads = append(matchedThreads, th)
					break
				}
			}
		}
	}
	return summaries, matchedThreads
}
```

**Step 4: Update `GenerateDailyReport`**

Change the caller in Step 5 (the goroutine for Call C×K):
```go
// In the goroutine for each cluster:
go func(idx int, c ClusterGroup) {
    var prevSummaries []string
    var prevClusterThreads []DailyReportThread
    if prevReport != nil {
        prevSummaries, prevClusterThreads = getPrevThreadSummaries(*prevReport, c)
    }
    data, err := GenerateClusterThreads(ctx, c, tags, prevSummaries)
    threadsCh <- threadsResult{clusterIdx: idx, data: data, prevClusterThreads: prevClusterThreads, err: err}
}(i, cluster)
```

Add `prevClusterThreads` to `threadsResult`:
```go
type threadsResult struct {
    clusterIdx        int
    data              []Thread
    prevClusterThreads []DailyReportThread
    err               error
}
```

Update the Step 6 matching call:
```go
for idx, cluster := range clusters {
    threads := threadsByCluster[idx]
    if prevReport != nil {
        // Gather all previous threads for matching
        prevClusterThreads := threadsPrevByCluster[idx]
        matchPreviousThreads(threads, prevClusterThreads, cluster)
    }
}
```

Update the section building to NOT set `Threads` JSON field and build `[][]DailyReportThread`:
```go
// Remove: threadsJSON, _ := json.Marshal(threads)
// Remove: Threads: threadsJSON,
```

Change the return signature and build the thread batches:
```go
func GenerateDailyReport(ctx context.Context, boardID uint, date time.Time) (*BoardDailyReport, []DailyReportSection, [][]DailyReportThread, error) {
```

After building sections, add thread batch building:
```go
// Build thread batches
var threadBatches [][]DailyReportThread
for i := range clusters {
    threads := threadsByCluster[i]
    var batch []DailyReportThread
    for _, th := range threads {
        tagIDsJSON, _ := json.Marshal(th.TagIDs)
        batch = append(batch, DailyReportThread{
            Title:        th.Title,
            Summary:      th.Summary,
            Status:       th.Status,
            TagIDs:       tagIDsJSON,
            Confidence:   th.Confidence,
            PrevThreadID: th.PrevThreadID,
        })
    }
    threadBatches = append(threadBatches, batch)
}

return report, sections, threadBatches, nil
```

Also need to track `prevClusterThreads` per cluster:
```go
threadsPrevByCluster := make(map[int][]DailyReportThread)
// In collection loop:
threadsPrevByCluster[tr.clusterIdx] = tr.prevClusterThreads
```

**Step 5: Update `generateSingleBoard` caller in handler.go**

```go
report, sections, threadBatches, err := GenerateDailyReport(ctx, boardID, date)
// ...
if err := SaveReport(report, sections, threadBatches); err != nil {
```

**Step 6: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: PASS

**Step 7: Run targeted tests**

Run: `cd /mnt/d/project/Syntopica/backend-go && go test ./internal/domain/daily_report/...`
Expected: PASS (existing tests may need minor adjustments for new signatures)

**Step 8: Commit**

```bash
git add backend-go/internal/domain/daily_report/generator.go backend-go/internal/domain/daily_report/handler.go
git commit -m "feat(daily-report): update generator to assign prev_thread_id and persist threads independently"
```

---

### Task 8: Add lineage and timeline API handlers

**Files:**
- Modify: `backend-go/internal/domain/daily_report/handler.go`

**Step 1: Add handler functions**

```go
// getThreadLineage handles GET /api/daily-reports/threads/:id/lineage
func getThreadLineage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid thread id"})
		return
	}

	chain, err := GetThreadLineage(uint(id))
	if err != nil || len(chain) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "thread lineage not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"chain": chain}})
}

// getBoardThreadTimeline handles GET /api/semantic-boards/:id/thread-timeline
func getBoardThreadTimeline(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}

	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil {
			days = parsed
		}
	}

	threads, err := GetBoardThreadTimeline(uint(boardID), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to fetch thread timeline"})
		return
	}

	if threads == nil {
		threads = []ThreadLineageNode{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"threads": threads}})
}
```

**Step 2: Register routes**

In `RegisterDailyReportRoutes`, add:
```go
api.GET("/daily-reports/threads/:id/lineage", getThreadLineage)
api.GET("/semantic-boards/:id/thread-timeline", getBoardThreadTimeline)
```

**Step 3: Verify build**

Run: `cd /mnt/d/project/Syntopica/backend-go && go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add backend-go/internal/domain/daily_report/handler.go
git commit -m "feat(daily-report): add thread lineage and board timeline API endpoints"
```

---

## Task Group D: Frontend API Types (depends on backend, independent of Group E/F)

### Task 9: Update frontend API types and add new API functions

**Files:**
- Modify: `front/app/api/dailyReports.ts`

**Step 1: Update `DailyReportThread` interface**

```typescript
export interface DailyReportThread {
  id: number
  report_id: number
  section_id: number
  title: string
  summary: string
  status: string
  tag_ids: number[]
  confidence: number
  prev_thread_id: number | null
  related_article_ids: number[]
  created_at: string
}
```

**Step 2: Add new interfaces and API functions**

```typescript
export interface ThreadLineageNode {
  id: number
  report_id: number
  section_id: number
  title: string
  summary: string
  status: string
  tag_ids: number[]
  confidence: number
  prev_thread_id: number | null
  period_date: string
  cluster_label: string
  created_at: string
}
```

Add API functions in `useDailyReportsApi()`:
```typescript
async function getThreadLineage(threadId: number): Promise<ApiResponse<{ chain: ThreadLineageNode[] }>> {
  return apiClient.get(`/daily-reports/threads/${threadId}/lineage`)
}

async function getBoardThreadTimeline(boardId: number, days?: number): Promise<ApiResponse<{ threads: ThreadLineageNode[] }>> {
  const query = days ? apiClient.buildQueryParams({ days }) : ''
  return apiClient.get(`/semantic-boards/${boardId}/thread-timeline${query ? `?${query}` : ''}`)
}
```

Update return to include new functions.

**Step 3: Verify lint**

Run: `cd /mnt/d/project/Syntopica/front && pnpm lint`
Expected: PASS

**Step 4: Commit**

```bash
git add front/app/api/dailyReports.ts
git commit -m "feat(daily-report): update frontend API types, add lineage/timeline endpoints"
```

---

## Task Group E: Frontend Thread Detail Panel (depends on Task 9)

### Task 10: Create `ThreadLineagePanel.vue` component

**Files:**
- Create: `front/app/features/tags/components/ThreadLineagePanel.vue`

**Step 1: Create the component**

A side panel component that:
- Accepts props: `threadId: number`, `visible: boolean`
- Emits: `close`
- Calls `getThreadLineage(threadId)` when visible
- Renders a vertical timeline with nodes (date, status badge, title, summary)
- Highlights the current thread node
- Uses dark theme consistent with the newspaper modal
- Status colors: emerging=green, continuing=blue, splitting=orange, merging=purple, ending=gray

Key design decisions:
- Fixed-width side panel (320px) that slides in from the right of the newspaper modal
- Vertical line connecting timeline nodes
- Loading skeleton while fetching
- "独立线程" label for threads with no lineage (single node, no prev/descendants)

**Step 2: Verify lint**

Run: `cd /mnt/d/project/Syntopica/front && pnpm lint`
Expected: PASS

**Step 3: Commit**

```bash
git add front/app/features/tags/components/ThreadLineagePanel.vue
git commit -m "feat(daily-report): create ThreadLineagePanel component for thread lineage timeline"
```

---

### Task 11: Update `BoardDailyReportTimeline.vue` — integrate lineage panel + thread click split

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Step 1: Add ThreadLineagePanel integration**

- Import `ThreadLineagePanel` and `useDailyReportsApi`
- Add state: `lineageThreadId`, `lineageVisible`
- Add thread click handler: clicking thread title/body area opens lineage panel
- Keep article icon click → existing article popup (add `@click.stop` on the icon)
- Adjust newspaper modal layout: when lineage panel is open, split into two columns (paper content + lineage side panel)

Specifically:
1. Add a wrapper div around `np-paper` content that uses flex layout
2. When lineage panel is visible, add `ThreadLineagePanel` as a sibling to the paper content area
3. Modify `np-thread-item` template: wrap title+summary in a div with `@click.stop="openLineage(thread)"`, keep article icon with `@click.stop="openThreadArticles($event, thread)"`

**Step 2: Verify lint**

Run: `cd /mnt/d/project/Syntopica/front && pnpm lint`
Expected: PASS

**Step 3: Verify typecheck**

Run: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"`
Expected: PASS

**Step 4: Commit**

```bash
git add front/app/features/tags/components/BoardDailyReportTimeline.vue
git commit -m "feat(daily-report): integrate ThreadLineagePanel into newspaper modal"
```

---

## Task Group F: Frontend Board Thread Browser (depends on Task 9, independent of Group E)

### Task 12: Create `BoardThreadBrowser.vue` component

**Files:**
- Create: `front/app/features/tags/components/BoardThreadBrowser.vue`

**Step 1: Create the component**

A component that:
- Accepts props: `boardId: number`
- Calls `getBoardThreadTimeline(boardId, days)` on mount and when days changes
- Builds lineage chains client-side from flat thread list using `prev_thread_id`
- Renders Gantt-chart grid:
  - Columns = dates (left to right, most recent N days)
  - Rows = lineage chains (grouped by chain root)
  - Nodes = colored circles/squares by status (same color scheme as timeline)
  - Connecting lines between nodes in same chain
- Clicking a node shows thread detail popup (title, summary, status, date)
- Days range selector (7/14/30/60 toggle buttons)
- Empty state: "暂无线程数据"

Key implementation notes:
- Use CSS grid for the Gantt layout
- Build chain groups by: (a) find all root threads (prev_thread_id=NULL), (b) for each root, follow prev_thread_id references forward to build chain
- Thread chains may have gaps (historical data without prev_thread_id) — treat each as separate single-node chain

**Step 2: Verify lint**

Run: `cd /mnt/d/project/Syntopica/front && pnpm lint`
Expected: PASS

**Step 3: Commit**

```bash
git add front/app/features/tags/components/BoardThreadBrowser.vue
git commit -m "feat(daily-report): create BoardThreadBrowser Gantt chart component"
```

---

### Task 13: Integrate BoardThreadBrowser into BoardDailyReportTimeline

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Step 1: Add toggle between report list and thread browser**

- Import `BoardThreadBrowser`
- Add state: `showThreadBrowser: boolean`
- Add a "线程总览" toggle button in the header area
- When toggled, show `BoardThreadBrowser` instead of the report card list (keep the newspaper modal separate)
- The toggle is in the list view only (not inside the modal)

**Step 2: Verify lint**

Run: `cd /mnt/d/project/Syntopica/front && pnpm lint`
Expected: PASS

**Step 3: Commit**

```bash
git add front/app/features/tags/components/BoardDailyReportTimeline.vue
git commit -m "feat(daily-report): add thread browser toggle to BoardDailyReportTimeline"
```

---

## Task Group G: Final Verification

### Task 14: Full build verification

**Step 1: Backend full build and test**

```bash
cd /mnt/d/project/Syntopica/backend-go && go build ./... && go vet ./...
```

**Step 2: Backend targeted test**

```bash
cd /mnt/d/project/Syntopica/backend-go && go test ./internal/domain/daily_report/...
```

**Step 3: Frontend lint**

```bash
cd /mnt/d/project/Syntopica/front && pnpm lint
```

**Step 4: Frontend typecheck (Windows cmd)**

```bash
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
```

**Step 5: Frontend build (Windows cmd)**

```bash
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
```

**Step 6: Mark all tasks complete in tasks.md**
