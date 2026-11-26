package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// 事件线身份显式重算（enable-story-cross-day-continuation D9 / tasks 4.1-4.2）。
//
// 红线（本文件全部实现都不得越线）：
//   - 默认不动历史：不带 --reattribute 的既有回填路径逐字不变；本文件只服务
//     显式重算开关；
//   - 只动自动归属面：story_manual=false 且 scope<>'composite' 的事件；
//     origin='manual' 的事件线与 story_manual=true 的线程一律不动；
//   - dry-run 绝不写库：只出统计与「跨天延续样本对」；
//   - 快照先行：写模式先把「事件 → 事件线」映射与相关事件线全量落盘
//     （JSONL / 0600 / fsync），再重置、再重算；
//   - 还原只还原归属：日报正文、文章、标签一律不碰；
//   - 判定逻辑、召回 SQL、红线 14/16 一律不改：本文件只重置与重算，
//     归属判定仍走既有 attributeStoriesForReportWithStats。

// reattributeSnapshotSampleLimit 是 dry-run 打印的样本对上限（其余只计数）。
const reattributeSnapshotSampleLimit = 50

// reattributeResetBatchSize 是重置 story_id 的批大小（分批 UPDATE，避免
// 超长 IN 列表）。
const reattributeResetBatchSize = 500

// ReattributeOptions selects the inclusive Asia/Shanghai calendar dates and
// the snapshot file of the explicit identity recompute.
//
// SnapshotPath is mandatory for a writing run (the CLI rejects the
// combination before touching the database). There is deliberately no
// subject blacklist: the recompute reuses the existing attribution verdicts
// as they are.
type ReattributeOptions struct {
	BackfillOptions
	SnapshotPath string
}

// ReattributeStats counts the decisions of one reattribute run.
//
// The first block mirrors BackfillStats (reports/threads/created/attached/
// merged/failed); the second block is the dry-run preview surface
// (resettable threads, involved old story lines, cross-day sample pairs).
type ReattributeStats struct {
	Reports        int
	ThreadsReset   int
	Created        int
	Attached       int
	Merged         int
	StoriesDeleted int
	Failed         int
	DryRun         bool
	// ResettableThreads 会被重置身份的事件数（写模式 = ThreadsReset）。
	ResettableThreads int
	// OldStories 涉及的旧事件线数（重置前这些线挂着被重置的事件）。
	OldStories int
	// SamplePairs 跨天延续样本对总数（不受打印上限影响）。
	SamplePairs int
	// Pairs 打印用的样本对（最多 reattributeSnapshotSampleLimit 对）。
	Pairs []CrossDaySample
}

// CrossDaySample is one cross-day continuation candidate pair: two alive
// story lines of the same board with the same fingerprint (subject_key +
// action + object) but different first_seen_date.
type CrossDaySample struct {
	StoryID           uint
	PeerStoryID       uint
	Subject           string
	Action            string
	Object            string
	PeerSubject       string
	PeerAction        string
	PeerObject        string
	FirstSeenDate     string
	PeerFirstSeenDate string
	// Distance 是两侧召回向量（embedding <=> embedding）的余弦距离；
	// HasDistance=false 表示任一侧无向量（SQLite 测试库不算距离）。
	Distance    float64
	HasDistance bool
}

// RestoreStats counts the restored rows of one snapshot restore.
type RestoreStats struct {
	// ThreadsRestored 回写了 story_id 的事件数。
	ThreadsRestored int
	// StoriesRestored 按显式 id 重建的事件线数（快照里有、库里没有）。
	StoriesRestored int
	// StoriesDeleted 删除的「本次重算新建且已空」的事件线数。
	StoriesDeleted int
	Failed         int
}

// ── 快照格式 ────────────────────────────────────────────────────────────────

// 快照记录类型。
const (
	snapshotRecordMeta    = "meta"    // 运行元信息（起始时间、涉及版块）
	snapshotRecordThread  = "thread"  // 事件 → 事件线映射
	snapshotRecordStory   = "story"   // 事件线全量快照
	snapshotRecordCreated = "created" // 本次重算新建的事件线
	snapshotRecordMerged  = "merged"  // 本次重算的合并（from → into）
)

// snapshotRecord 是快照 JSONL 的一行。
type snapshotRecord struct {
	Type string `json:"type"`
	// meta
	CreatedAt string `json:"created_at,omitempty"`
	Boards    []uint `json:"boards,omitempty"`
	// thread / created
	ThreadID *uint `json:"thread_id,omitempty"`
	StoryID  *uint `json:"story_id,omitempty"`
	// thread：挂载时的进展标记（还原时一并回写）
	IsDevelopment *bool `json:"is_development,omitempty"`
	// story
	Story *storySnapshot `json:"story,omitempty"`
	// merged
	MergedFromID *uint `json:"from_id,omitempty"`
	MergedIntoID *uint `json:"into_id,omitempty"`
}

// storySnapshot 是事件线的一行全量快照（embedding 以 float32 数组保存，
// 无向量时为 null）。
type storySnapshot struct {
	ID               uint       `json:"id"`
	BoardID          uint       `json:"board_id"`
	LaneID           *uint      `json:"lane_id"`
	Title            string     `json:"title"`
	Subject          string     `json:"subject"`
	SubjectKey       string     `json:"subject_key"`
	Action           string     `json:"action"`
	Object           string     `json:"object"`
	OccurredAt       *time.Time `json:"occurred_at"`
	Status           string     `json:"status"`
	FirstSeenDate    time.Time  `json:"first_seen_date"`
	LastSeenDate     time.Time  `json:"last_seen_date"`
	ThreadCount      int        `json:"thread_count"`
	ArticleCount     int        `json:"article_count"`
	ParticipantCount int        `json:"participant_count"`
	Embedding        []float32  `json:"embedding"`
	SubjectEmbedding []float32  `json:"subject_embedding"`
	MergedInto       *uint      `json:"merged_into"`
	Origin           string     `json:"origin"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// snapshotWriter 追加写快照文件（0600 + fsync）。
type snapshotWriter struct {
	file *os.File
	buf  *bufio.Writer
	enc  *json.Encoder
}

func newSnapshotWriter(path string) (*snapshotWriter, error) {
	//nolint:gosec // G304: 快照路径由运维通过 --snapshot 显式指定
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open snapshot file %s: %w", path, err)
	}
	// 既有文件的权限可能被放宽过 —— 快照含归属信息，统一收回 0600。
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("chmod snapshot file %s: %w", path, err)
	}
	buf := bufio.NewWriter(file)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	return &snapshotWriter{file: file, buf: buf, enc: enc}, nil
}

func (w *snapshotWriter) Write(record snapshotRecord) error {
	if err := w.enc.Encode(record); err != nil {
		return fmt.Errorf("write snapshot record: %w", err)
	}
	return nil
}

// Sync 把缓冲落盘并 fsync —— 改库之前必须先调用（快照先于写入）。
func (w *snapshotWriter) Sync() error {
	if err := w.buf.Flush(); err != nil {
		return fmt.Errorf("flush snapshot: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("fsync snapshot: %w", err)
	}
	return nil
}

func (w *snapshotWriter) Close() error {
	if err := w.Sync(); err != nil {
		_ = w.file.Close()
		return err
	}
	return w.file.Close()
}

// ── 入口 ────────────────────────────────────────────────────────────────────

// ReattributeStories recomputes the story identity of the events inside the
// selected board × date range.
//
// Writing run (DryRun=false): snapshot first (SnapshotPath mandatory), then
// reset the identity of the non-manual events, then re-attribute every report
// of the range in ascending date order, then drop the story lines that ended
// up empty and recompute the counts of the surviving ones.
//
// Dry run: read-only — counts the resettable events, the involved old story
// lines and the cross-day continuation sample pairs. Never writes.
func ReattributeStories(ctx context.Context, opts ReattributeOptions) (ReattributeStats, error) {
	stats := ReattributeStats{DryRun: opts.DryRun}
	if opts.DryRun {
		return stats, previewReattribute(ctx, opts, &stats)
	}
	if strings.TrimSpace(opts.SnapshotPath) == "" {
		return stats, errors.New("reattribute: 写模式必须指定快照文件（--snapshot），避免不可逆的重算")
	}
	boards, err := reattributeBoardIDs(ctx, opts)
	if err != nil {
		return stats, err
	}
	snap, err := newSnapshotWriter(opts.SnapshotPath)
	if err != nil {
		return stats, err
	}
	defer func() { _ = snap.Close() }()
	meta := snapshotRecord{Type: snapshotRecordMeta, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Boards: boards}
	if err := snap.Write(meta); err != nil {
		return stats, err
	}
	for _, boardID := range boards {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		err := database.DB.WithContext(ctx).Connection(func(db *gorm.DB) error {
			return withStoryBoardLock(db, boardID, func(db *gorm.DB) error {
				return reattributeBoard(ctx, db, opts, boardID, snap, &stats)
			})
		})
		if errors.Is(err, errStoryBoardLocked) {
			logging.Warnf("story-reattribute: board %d locked by another run; skipped", boardID)
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			stats.Failed++
			logging.Warnf("story-reattribute: board %d: %v", boardID, err)
			continue
		}
	}
	return stats, ctx.Err()
}

// RestoreStoryIdentity restores the「事件 → 事件线」attribution from a
// snapshot file: missing story lines are recreated with their explicit id,
// the threads get their story_id back, and the story lines the recompute
// created (and that ended up empty) are dropped. Idempotent.
//
// Only the attribution is restored — report bodies, articles and tags are
// never touched.
func RestoreStoryIdentity(ctx context.Context, path string) (RestoreStats, error) {
	stats := RestoreStats{}
	if strings.TrimSpace(path) == "" {
		return stats, errors.New("restore: 缺少快照文件路径")
	}
	records, err := readSnapshotFile(path)
	if err != nil {
		return stats, err
	}
	db := database.DB.WithContext(ctx)
	var stories []storySnapshot
	type threadRecord struct {
		threadID uint
		storyID  *uint
		isDev    *bool
	}
	var threads []threadRecord
	created := map[uint]bool{}
	var merged [][2]uint
	var boards []uint
	var metaCreatedAt time.Time
	for _, record := range records {
		switch record.Type {
		case snapshotRecordMeta:
			boards = append(boards, record.Boards...)
			if record.CreatedAt != "" {
				if t, perr := time.Parse(time.RFC3339Nano, record.CreatedAt); perr == nil {
					metaCreatedAt = t
				}
			}
		case snapshotRecordThread:
			if record.ThreadID == nil {
				continue
			}
			threads = append(threads, threadRecord{threadID: *record.ThreadID, storyID: record.StoryID, isDev: record.IsDevelopment})
		case snapshotRecordStory:
			if record.Story != nil {
				stories = append(stories, *record.Story)
			}
		case snapshotRecordCreated:
			if record.StoryID != nil {
				created[*record.StoryID] = true
			}
		case snapshotRecordMerged:
			if record.MergedFromID != nil && record.MergedIntoID != nil {
				merged = append(merged, [2]uint{*record.MergedFromID, *record.MergedIntoID})
			}
		}
	}
	sort.Slice(boards, func(i, j int) bool { return boards[i] < boards[j] })

	// 1. 按显式 id 重建缺失的事件线（快照留了全量字段，含向量）。
	snapshotStoryIDs := make(map[uint]bool, len(stories))
	for _, story := range stories {
		snapshotStoryIDs[story.ID] = true
		var count int64
		if err := db.Model(&repository.BoardStory{}).Where("id = ?", story.ID).Count(&count).Error; err != nil {
			stats.Failed++
			return stats, fmt.Errorf("restore: check story %d: %w", story.ID, err)
		}
		if count > 0 {
			continue
		}
		if err := insertSnapshotStory(db, story); err != nil {
			stats.Failed++
			logging.Warnf("restore: recreate story %d failed: %v", story.ID, err)
			continue
		}
		stats.StoriesRestored++
	}

	// 2. 还原合并拓扑：快照内的事件线回到快照里的 merged_into/status；
	//    本次重算合并掉的其它事件线解除合并（合并前必是存活线）。
	for _, story := range stories {
		if err := db.Model(&repository.BoardStory{}).
			Where("id = ?", story.ID).
			Updates(map[string]any{"merged_into": story.MergedInto, "status": story.Status}).Error; err != nil {
			stats.Failed++
			logging.Warnf("restore: restore story %d merge state failed: %v", story.ID, err)
		}
	}
	for _, pair := range merged {
		if snapshotStoryIDs[pair[0]] {
			continue // 快照内的线已在上面按快照值还原
		}
		if err := db.Model(&repository.BoardStory{}).
			Where("id = ?", pair[0]).
			Updates(map[string]any{"merged_into": nil, "status": repository.StoryStatusActive}).Error; err != nil {
			stats.Failed++
			logging.Warnf("restore: unmerge story %d failed: %v", pair[0], err)
		}
	}

	// 3. 回写事件的归属（story_id + 进展标记）。
	for _, thread := range threads {
		updates := map[string]any{"story_id": thread.storyID}
		if thread.isDev != nil {
			updates["is_development"] = *thread.isDev
		}
		res := db.Model(&repository.DailyReportThread{}).Where("id = ?", thread.threadID).Updates(updates)
		if res.Error != nil {
			stats.Failed++
			logging.Warnf("restore: thread %d story_id failed: %v", thread.threadID, res.Error)
			continue
		}
		stats.ThreadsRestored += int(res.RowsAffected)
	}

	// 4. 删除本次重算新建、且已无事件挂靠的事件线（仅限快照涉及的版块）。
	dropped, failed := restoreDropNewStories(db, created, snapshotStoryIDs, boards, metaCreatedAt)
	stats.StoriesDeleted = dropped
	stats.Failed += failed

	// 5. 重算被还原事件线的计数与热度证据（归属变了，计数必须跟着变）。
	repo := repository.NewTopicGraphRepository(db)
	for _, story := range stories {
		if err := repo.RecomputeBoardStoryCounts(story.ID); err != nil {
			stats.Failed++
			logging.Warnf("restore: recompute story %d counts failed: %v", story.ID, err)
			continue
		}
		if err := repo.RecomputeSignalsForStory(story.ID); err != nil {
			stats.Failed++
			logging.Warnf("restore: recompute story %d signals failed: %v", story.ID, err)
		}
	}
	return stats, nil
}

// ── 写模式 ──────────────────────────────────────────────────────────────────

func reattributeBoard(ctx context.Context, db *gorm.DB, opts ReattributeOptions, boardID uint, snap *snapshotWriter, stats *ReattributeStats) error {
	boardOpts := opts.BackfillOptions
	boardOpts.BoardID = &boardID
	// 1. 范围内的日报（日期升序）。
	q, err := backfillReportQuery(db, boardOpts)
	if err != nil {
		return err
	}
	var reports []repository.BoardDailyReport
	if err := q.Select("id", "period_date").Order("period_date ASC").Find(&reports).Error; err != nil {
		return fmt.Errorf("reattribute: load reports: %w", err)
	}

	// 2. 自动归属面内的线程（人工标记与综合稿不在其中）。
	threads, err := reattributeThreadScope(db, boardOpts)
	if err != nil {
		return err
	}
	var resetIDs []uint
	var snapshotThreads []reattributeThread
	for _, thread := range threads {
		if thread.StoryID == nil {
			// 原本就未归属：重算会顺手归属它 —— 快照记下 NULL，还原时写回 NULL。
			snapshotThreads = append(snapshotThreads, thread)
			continue
		}
		if thread.StoryOrigin == repository.StoryOriginManual {
			continue // 人工事件线一律不动
		}
		resetIDs = append(resetIDs, thread.ThreadID)
		snapshotThreads = append(snapshotThreads, thread)
	}
	storyIDs := make([]uint, 0, len(snapshotThreads))
	seenStory := map[uint]bool{}
	for _, thread := range snapshotThreads {
		if thread.StoryID == nil || seenStory[*thread.StoryID] {
			continue
		}
		seenStory[*thread.StoryID] = true
		storyIDs = append(storyIDs, *thread.StoryID)
	}
	sort.Slice(storyIDs, func(i, j int) bool { return storyIDs[i] < storyIDs[j] })
	stats.OldStories += len(storyIDs)

	// 3. 先写快照（线程映射 + 相关事件线全量），fsync 之后才动库。
	stories, err := loadSnapshotStories(db, storyIDs)
	if err != nil {
		return err
	}
	for _, thread := range snapshotThreads {
		record := snapshotRecord{Type: snapshotRecordThread, ThreadID: &thread.ThreadID, StoryID: thread.StoryID}
		if thread.IsDevelopment != nil {
			record.IsDevelopment = thread.IsDevelopment
		}
		if err := snap.Write(record); err != nil {
			return err
		}
	}
	for _, story := range stories {
		snapshot := storyToSnapshot(story)
		if err := snap.Write(snapshotRecord{Type: snapshotRecordStory, Story: &snapshot}); err != nil {
			return err
		}
	}
	if err := snap.Sync(); err != nil {
		return err
	}

	// 4. 重置身份（分批）。
	for start := 0; start < len(resetIDs); start += reattributeResetBatchSize {
		end := start + reattributeResetBatchSize
		if end > len(resetIDs) {
			end = len(resetIDs)
		}
		if err := db.Exec("UPDATE daily_report_threads SET story_id = NULL WHERE id IN ?", resetIDs[start:end]).Error; err != nil {
			return fmt.Errorf("reattribute: reset threads: %w", err)
		}
	}
	stats.ThreadsReset += len(resetIDs)
	stats.ResettableThreads += len(resetIDs)

	// 5. 按日期升序重算（复用既有判定与建线/挂载路径）。
	repo := repository.NewTopicGraphRepository(db)
	store := storyStore(repoStoryStore{repo: repo})
	if storyStoreForTest != nil {
		store = storyStoreForTest
	}
	recorder := &recordingStoryStore{storyStore: store}
	for _, report := range reports {
		if err := ctx.Err(); err != nil {
			return err
		}
		stats.Reports++
		part, err := attributeStoriesForReportWithStats(ctx, recorder, boardID, report.ID, report.PeriodDate, false)
		stats.Created += part.Created
		stats.Attached += part.Attached
		stats.Merged += part.Merged
		stats.Failed += part.Failed
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			stats.Failed++
			logging.Warnf("story-reattribute: report %d: %v", report.ID, err)
		}
	}
	// 5.5 追加本次新建/合并记录 —— 还原时据此精确清理。
	for _, id := range recorder.created {
		id := id
		if err := snap.Write(snapshotRecord{Type: snapshotRecordCreated, StoryID: &id}); err != nil {
			return err
		}
	}
	for _, pair := range recorder.merged {
		from, into := pair[0], pair[1]
		if err := snap.Write(snapshotRecord{Type: snapshotRecordMerged, MergedFromID: &from, MergedIntoID: &into}); err != nil {
			return err
		}
	}
	if err := snap.Sync(); err != nil {
		return err
	}

	// 6. 旧事件线：变空的删除，非空的重算计数（first/last_seen、thread_count、
	//    participant_count）。
	for _, id := range storyIDs {
		var story repository.BoardStory
		if err := db.Select("id", "origin", "merged_into").First(&story, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // 已被合并/删除
			}
			stats.Failed++
			logging.Warnf("story-reattribute: load story %d: %v", id, err)
			continue
		}
		if story.Origin == repository.StoryOriginManual {
			continue // 人工事件线不动
		}
		if story.MergedInto != nil {
			continue // 已被合并：不删（保持一跳链）
		}
		var count int64
		if err := db.Model(&repository.DailyReportThread{}).Where("story_id = ?", id).Count(&count).Error; err != nil {
			stats.Failed++
			logging.Warnf("story-reattribute: count threads of story %d: %v", id, err)
			continue
		}
		if count == 0 {
			if err := db.Where("story_id = ?", id).Delete(&repository.StorySignal{}).Error; err != nil {
				logging.Warnf("story-reattribute: delete signals of story %d: %v", id, err)
			}
			if err := db.Delete(&repository.BoardStory{}, id).Error; err != nil {
				stats.Failed++
				logging.Warnf("story-reattribute: delete empty story %d: %v", id, err)
				continue
			}
			stats.StoriesDeleted++
			continue
		}
		if err := repo.RecomputeBoardStoryCounts(id); err != nil {
			stats.Failed++
			logging.Warnf("story-reattribute: recompute story %d counts: %v", id, err)
		}
	}
	return nil
}

// recordingStoryStore 记录本次重算新建/合并的事件线（供快照与还原使用）。
type recordingStoryStore struct {
	storyStore
	created []uint
	merged  [][2]uint
}

func (s *recordingStoryStore) CreateBoardStory(story *repository.BoardStory) error {
	if err := s.storyStore.CreateBoardStory(story); err != nil {
		return err
	}
	s.created = append(s.created, story.ID)
	return nil
}

func (s *recordingStoryStore) MergeBoardStories(fromID, intoID uint) (uint, error) {
	survivor, err := s.storyStore.MergeBoardStories(fromID, intoID)
	if err != nil {
		return survivor, err
	}
	s.merged = append(s.merged, [2]uint{fromID, survivor})
	return survivor, nil
}

// ── 只读预览 ────────────────────────────────────────────────────────────────

func previewReattribute(ctx context.Context, opts ReattributeOptions, stats *ReattributeStats) error {
	boards, err := reattributeBoardIDs(ctx, opts)
	if err != nil {
		return err
	}
	for _, boardID := range boards {
		if err := ctx.Err(); err != nil {
			return err
		}
		db := database.DB.WithContext(ctx)
		boardOpts := opts.BackfillOptions
		boardOpts.BoardID = &boardID
		reportQuery, err := backfillReportQuery(db, boardOpts)
		if err != nil {
			return err
		}
		var reports int64
		if err := reportQuery.Count(&reports).Error; err != nil {
			return fmt.Errorf("reattribute: count reports: %w", err)
		}
		stats.Reports += int(reports)
		threads, err := reattributeThreadScope(db, boardOpts)
		if err != nil {
			return err
		}
		storyIDs := map[uint]bool{}
		for _, thread := range threads {
			if thread.StoryID == nil {
				continue
			}
			if thread.StoryOrigin == repository.StoryOriginManual {
				continue
			}
			stats.ResettableThreads++
			storyIDs[*thread.StoryID] = true
		}
		stats.OldStories += len(storyIDs)
		remaining := reattributeSnapshotSampleLimit - len(stats.Pairs)
		pairs, total, err := reattributeSamplePairs(db, boardID, opts.From, opts.To, remaining)
		if err != nil {
			return err
		}
		stats.Pairs = append(stats.Pairs, pairs...)
		stats.SamplePairs += total
	}
	return nil
}

// reattributeSamplePairs finds the cross-day continuation candidates of one
// board: two alive story lines with the same fingerprint (subject_key +
// action + object, all non-empty) but different first_seen_date.
//
// The date window bounds the pairs by the story lines' first_seen_date (the
// window of the recompute). The pgvector distance between the two recall
// vectors is reported when both lines carry a vector (SQLite test databases
// have no pgvector operator → the distance is skipped).
func reattributeSamplePairs(db *gorm.DB, boardID uint, from, to time.Time, limit int) ([]CrossDaySample, int, error) {
	fromStr := from.Format(time.DateOnly)
	toExclusive := to.AddDate(0, 0, 1).Format(time.DateOnly)
	base := `FROM board_stories a JOIN board_stories b ON b.id > a.id
		WHERE a.board_id = ? AND b.board_id = ?
		  AND a.status = ? AND b.status = ?
		  AND a.merged_into IS NULL AND b.merged_into IS NULL
		  AND a.subject_key <> '' AND a.subject_key = b.subject_key
		  AND a.action <> '' AND a.action = b.action
		  AND a.object <> '' AND a.object = b.object
		  AND a.first_seen_date >= ? AND a.first_seen_date < ?
		  AND b.first_seen_date >= ? AND b.first_seen_date < ?
		  AND a.first_seen_date <> b.first_seen_date`
	args := []any{
		boardID, boardID,
		repository.StoryStatusActive, repository.StoryStatusActive,
		fromStr, toExclusive, fromStr, toExclusive,
	}
	var total int64
	if err := db.Raw("SELECT COUNT(*) "+base, args...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("reattribute: count cross-day pairs: %w", err)
	}
	if limit <= 0 {
		return nil, int(total), nil
	}
	distanceExpr := "NULL AS distance"
	hasVectorExpr := "0 AS has_vec"
	if db.Name() == "postgres" {
		distanceExpr = "CASE WHEN a.embedding IS NOT NULL AND b.embedding IS NOT NULL THEN a.embedding <=> b.embedding END AS distance"
		hasVectorExpr = "CASE WHEN a.embedding IS NOT NULL AND b.embedding IS NOT NULL THEN 1 ELSE 0 END AS has_vec"
	}
	type pairRow struct {
		AID      uint      `gorm:"column:a_id"`
		BID      uint      `gorm:"column:b_id"`
		ASubject string    `gorm:"column:a_subject"`
		AAction  string    `gorm:"column:a_action"`
		AObject  string    `gorm:"column:a_object"`
		BSubject string    `gorm:"column:b_subject"`
		BAction  string    `gorm:"column:b_action"`
		BObject  string    `gorm:"column:b_object"`
		ADate    time.Time `gorm:"column:a_date"`
		BDate    time.Time `gorm:"column:b_date"`
		Distance float64   `gorm:"column:distance"`
		HasVec   int       `gorm:"column:has_vec"`
	}
	query := `SELECT a.id AS a_id, b.id AS b_id,
		a.subject AS a_subject, a.action AS a_action, a.object AS a_object,
		b.subject AS b_subject, b.action AS b_action, b.object AS b_object,
		a.first_seen_date AS a_date, b.first_seen_date AS b_date, ` + distanceExpr + `, ` + hasVectorExpr + ` ` + base +
		` ORDER BY a.id ASC, b.id ASC LIMIT ?`
	var rows []pairRow
	if err := db.Raw(query, append(append([]any{}, args...), limit)...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("reattribute: list cross-day pairs: %w", err)
	}
	pairs := make([]CrossDaySample, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, CrossDaySample{
			StoryID:           row.AID,
			PeerStoryID:       row.BID,
			Subject:           row.ASubject,
			Action:            row.AAction,
			Object:            row.AObject,
			PeerSubject:       row.BSubject,
			PeerAction:        row.BAction,
			PeerObject:        row.BObject,
			FirstSeenDate:     row.ADate.Format(time.DateOnly),
			PeerFirstSeenDate: row.BDate.Format(time.DateOnly),
			Distance:          row.Distance,
			HasDistance:       row.HasVec == 1,
		})
	}
	return pairs, int(total), nil
}

// ── 查询辅助 ────────────────────────────────────────────────────────────────

// reattributeThread is one thread of the automatic attribution surface.
type reattributeThread struct {
	ThreadID      uint
	ReportID      uint
	StoryID       *uint
	Scope         string
	StoryOrigin   string
	IsDevelopment *bool
}

// reattributeThreadScope loads the threads of the selected range that the
// automatic attribution may touch: story_manual=false and scope<>'composite'.
// Threads pointing to a story line carry that line's origin so the caller
// can keep manual lines untouched.
func reattributeThreadScope(db *gorm.DB, opts BackfillOptions) ([]reattributeThread, error) {
	from, to, err := backfillDateBounds(opts)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse(time.DateOnly, to)
	if err != nil {
		return nil, err
	}
	type threadRow struct {
		ThreadID      uint   `gorm:"column:thread_id"`
		ReportID      uint   `gorm:"column:report_id"`
		StoryID       *uint  `gorm:"column:story_id"`
		Scope         string `gorm:"column:scope"`
		StoryOrigin   string `gorm:"column:story_origin"`
		IsDevelopment bool   `gorm:"column:is_development"`
	}
	q := db.Table("daily_report_threads AS t").
		Select("t.id AS thread_id, t.report_id AS report_id, t.story_id AS story_id, t.scope AS scope, t.is_development AS is_development, s.origin AS story_origin").
		Joins("JOIN board_daily_reports AS r ON r.id = t.report_id").
		Joins("LEFT JOIN board_stories AS s ON s.id = t.story_id").
		Where("r.period_date >= ? AND r.period_date < ?", from, end.AddDate(0, 0, 1).Format(time.DateOnly)).
		Where("t.story_manual = ?", false).
		Where("COALESCE(t.scope, '') <> ?", repository.ThreadScopeComposite)
	if opts.BoardID != nil {
		q = q.Where("r.semantic_board_id = ?", *opts.BoardID)
	}
	var rows []threadRow
	if err := q.Order("t.id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("reattribute: load threads: %w", err)
	}
	threads := make([]reattributeThread, 0, len(rows))
	for _, row := range rows {
		thread := reattributeThread{
			ThreadID:    row.ThreadID,
			ReportID:    row.ReportID,
			StoryID:     row.StoryID,
			Scope:       row.Scope,
			StoryOrigin: row.StoryOrigin,
		}
		if row.IsDevelopment {
			value := row.IsDevelopment
			thread.IsDevelopment = &value
		}
		threads = append(threads, thread)
	}
	return threads, nil
}

func reattributeBoardIDs(ctx context.Context, opts ReattributeOptions) ([]uint, error) {
	if opts.BoardID != nil {
		return []uint{*opts.BoardID}, nil
	}
	q, err := backfillReportQuery(database.DB.WithContext(ctx), opts.BackfillOptions)
	if err != nil {
		return nil, err
	}
	var boards []uint
	if err := q.Distinct("semantic_board_id").Order("semantic_board_id").Pluck("semantic_board_id", &boards).Error; err != nil {
		return nil, fmt.Errorf("reattribute: load boards: %w", err)
	}
	return boards, nil
}

// ── 快照读写 ────────────────────────────────────────────────────────────────

func loadSnapshotStories(db *gorm.DB, storyIDs []uint) ([]repository.BoardStory, error) {
	if len(storyIDs) == 0 {
		return nil, nil
	}
	var stories []repository.BoardStory
	if err := db.Where("id IN ?", storyIDs).Find(&stories).Error; err != nil {
		return nil, fmt.Errorf("reattribute: load stories: %w", err)
	}
	return stories, nil
}

func storyToSnapshot(story repository.BoardStory) storySnapshot {
	return storySnapshot{
		ID:               story.ID,
		BoardID:          story.BoardID,
		LaneID:           story.LaneID,
		Title:            story.Title,
		Subject:          story.Subject,
		SubjectKey:       story.SubjectKey,
		Action:           story.Action,
		Object:           story.Object,
		OccurredAt:       story.OccurredAt,
		Status:           story.Status,
		FirstSeenDate:    story.FirstSeenDate,
		LastSeenDate:     story.LastSeenDate,
		ThreadCount:      story.ThreadCount,
		ArticleCount:     story.ArticleCount,
		ParticipantCount: story.ParticipantCount,
		Embedding:        pgVectorToFloat32(story.Embedding),
		SubjectEmbedding: pgVectorToFloat32(story.SubjectEmbedding),
		MergedInto:       story.MergedInto,
		Origin:           story.Origin,
		CreatedAt:        story.CreatedAt,
		UpdatedAt:        story.UpdatedAt,
	}
}

func readSnapshotFile(path string) ([]snapshotRecord, error) {
	file, err := os.Open(path) //nolint:gosec // G304: 运维指定的快照文件路径
	if err != nil {
		return nil, fmt.Errorf("restore: open snapshot %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var records []snapshotRecord
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record snapshotRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("restore: parse snapshot line %q: %w", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("restore: read snapshot %s: %w", path, err)
	}
	return records, nil
}

// insertSnapshotStory recreates a story line with its explicit id. The
// pgvector columns are written as vector literals (Postgres) or plain text
// (SQLite test databases).
func insertSnapshotStory(db *gorm.DB, story storySnapshot) error {
	vecCast := ""
	if db.Name() == "postgres" {
		vecCast = "::vector"
	}
	columns := "id, board_id, lane_id, title, subject, subject_key, action, object, occurred_at, status, " +
		"first_seen_date, last_seen_date, thread_count, article_count, participant_count, " +
		"embedding, subject_embedding, merged_into, origin, created_at, updated_at"
	query := "INSERT INTO board_stories (" + columns + ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?" +
		vecCast + ", ?" + vecCast + ", ?, ?, ?, ?)"
	var embedding, subjectEmbedding any
	if literal := float32ToPgVectorLiteral(story.Embedding); literal != "" {
		embedding = literal
	}
	if literal := float32ToPgVectorLiteral(story.SubjectEmbedding); literal != "" {
		subjectEmbedding = literal
	}
	var laneID, occurredAt, mergedInto any
	if story.LaneID != nil {
		laneID = *story.LaneID
	}
	if story.OccurredAt != nil {
		occurredAt = *story.OccurredAt
	}
	if story.MergedInto != nil {
		mergedInto = *story.MergedInto
	}
	return db.Exec(query,
		story.ID, story.BoardID, laneID, story.Title, story.Subject, story.SubjectKey, story.Action, story.Object,
		occurredAt, story.Status, story.FirstSeenDate, story.LastSeenDate, story.ThreadCount, story.ArticleCount,
		story.ParticipantCount, embedding, subjectEmbedding, mergedInto, story.Origin, story.CreatedAt, story.UpdatedAt,
	).Error
}

// restoreDropNewStories deletes the story lines the recompute created and
// that ended up empty. Only boards covered by the snapshot are touched.
//
// The precise set comes from the snapshot's "created" records; without them
// the fallback is the meta record's start time (stories created after the
// snapshot was taken).
func restoreDropNewStories(db *gorm.DB, created map[uint]bool, snapshotStoryIDs map[uint]bool, boards []uint, metaCreatedAt time.Time) (int, int) {
	dropped, failed := 0, 0
	if len(boards) == 0 {
		return 0, 0
	}
	if len(created) == 0 {
		if metaCreatedAt.IsZero() {
			// 没有精确记录也没有运行时间 → 不猜（保守：不动库里的任何线）。
			return 0, 0
		}
		var rows []repository.BoardStory
		if err := db.Select("id").Where("board_id IN ? AND created_at >= ?", boards, metaCreatedAt.Add(-time.Second)).Find(&rows).Error; err != nil {
			return 0, 1
		}
		for _, row := range rows {
			created[row.ID] = true
		}
	}
	ids := make([]uint, 0, len(created))
	for id := range created {
		if snapshotStoryIDs[id] {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		var story repository.BoardStory
		if err := db.Select("id", "board_id").First(&story, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			failed++
			continue
		}
		if !containsUint(boards, story.BoardID) {
			continue // 只清理快照涉及的版块
		}
		var count int64
		if err := db.Model(&repository.DailyReportThread{}).Where("story_id = ?", id).Count(&count).Error; err != nil {
			failed++
			continue
		}
		if count != 0 {
			continue // 还有事件挂着：不是空线
		}
		if err := db.Where("story_id = ?", id).Delete(&repository.StorySignal{}).Error; err != nil {
			failed++
			continue
		}
		if err := db.Delete(&repository.BoardStory{}, id).Error; err != nil {
			failed++
			continue
		}
		dropped++
	}
	return dropped, failed
}

func containsUint(list []uint, value uint) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// ── 向量快照 ────────────────────────────────────────────────────────────────

// pgVectorToFloat32 parses a pgvector literal into float32s (nil when the
// vector is missing or unparseable — null-safe).
func pgVectorToFloat32(raw string) []float32 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	vec, err := parsePgVector(raw)
	if err != nil || len(vec) == 0 {
		return nil
	}
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}

// float32ToPgVectorLiteral renders float32s back into a pgvector literal
// (shortest round-trip decimal per component).
func float32ToPgVectorLiteral(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
