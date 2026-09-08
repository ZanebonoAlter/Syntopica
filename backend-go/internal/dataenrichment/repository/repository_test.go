package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/database"
)

// Test-only source types: the source_type enum is extensible (spec "板块数据源
// 绑定"); built-in financial types were removed, so tests register neutral
// dummies to exercise CRUD mechanics without coupling to any built-in type.
func init() {
	repository.RegisterSourceType("test_source")
	repository.RegisterSourceType("test_source_2")
}

func setupRepoTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database.DB = db

	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	repo := repository.NewRepository(db)
	repository.SetRepo(repo)
}

func TestBoardDataSource_CreateAndGetByBoardAndType(t *testing.T) {
	setupRepoTestDB(t)

	ctx := context.Background()

	ds := &repository.BoardDataSource{
		SemanticBoardID: 1,
		SourceType:      "test_source",
		Config:          map[string]any{"keywords": []string{"半导体"}},
		Enabled:         true,
	}
	if err := repository.Repo.CreateBoardDataSource(ctx, ds); err != nil {
		t.Fatalf("create: %v", err)
	}
	if ds.ID == 0 {
		t.Fatal("expected ID to be set after create")
	}

	got, err := repository.Repo.GetBoardDataSourceByBoardAndType(ctx, 1, "test_source")
	if err != nil {
		t.Fatalf("get by board and type: %v", err)
	}
	if got.SemanticBoardID != 1 {
		t.Fatalf("board id = %d, want 1", got.SemanticBoardID)
	}
}

func TestBoardDataSource_Upsert(t *testing.T) {
	setupRepoTestDB(t)

	ctx := context.Background()

	ds := &repository.BoardDataSource{
		SemanticBoardID: 1,
		SourceType:      "test_source",
		Config:          map[string]any{"keywords": []string{"半导体"}},
		Enabled:         true,
	}
	if err := repository.Repo.UpsertBoardDataSource(ctx, ds); err != nil {
		t.Fatalf("upsert #1: %v", err)
	}

	ds.Enabled = false
	if err := repository.Repo.UpsertBoardDataSource(ctx, ds); err != nil {
		t.Fatalf("upsert #2: %v", err)
	}

	got, err := repository.Repo.GetBoardDataSourceByBoardAndType(ctx, 1, "test_source")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false after upsert update")
	}
}

// TestBoardDataSource_UpsertTOCTOU verifies that rapid sequential upserts
// with the same unique key are atomic (no duplicate-key error).
func TestBoardDataSource_UpsertTOCTOU(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	// First upsert.
	ds := &repository.BoardDataSource{
		SemanticBoardID: 1,
		SourceType:      "test_source",
		Enabled:         true,
	}
	if err := repository.Repo.UpsertBoardDataSource(ctx, ds); err != nil {
		t.Fatalf("upsert #1: %v", err)
	}

	// Second upsert with same key — must not fail.
	ds2 := &repository.BoardDataSource{
		SemanticBoardID: 1,
		SourceType:      "test_source",
		Enabled:         false,
	}
	if err := repository.Repo.UpsertBoardDataSource(ctx, ds2); err != nil {
		t.Fatalf("upsert #2 (TOCTOU): %v", err)
	}

	// Verify only one row exists.
	list, err := repository.Repo.ListBoardDataSourcesByBoardID(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row after upserts, got %d", len(list))
	}
	if list[0].Enabled {
		t.Fatal("expected enabled=false after second upsert")
	}
}

func TestBoardDataSource_ListByBoardID(t *testing.T) {
	setupRepoTestDB(t)

	ctx := context.Background()

	ds1 := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source", Enabled: true}
	ds2 := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source_2", Enabled: true}
	ds3 := &repository.BoardDataSource{SemanticBoardID: 2, SourceType: "test_source", Enabled: true}

	for _, ds := range []*repository.BoardDataSource{ds1, ds2, ds3} {
		if err := repository.Repo.CreateBoardDataSource(ctx, ds); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	list, err := repository.Repo.ListBoardDataSourcesByBoardID(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
}

func TestBoardDataSource_UniqueConstraint(t *testing.T) {
	setupRepoTestDB(t)

	ctx := context.Background()

	ds := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source"}
	if err := repository.Repo.CreateBoardDataSource(ctx, ds); err != nil {
		t.Fatalf("create #1: %v", err)
	}

	ds2 := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source"}
	if err := repository.Repo.CreateBoardDataSource(ctx, ds2); err == nil {
		t.Fatal("expected UNIQUE constraint error for duplicate board+source_type")
	}
}

func TestBoardDataSource_Delete(t *testing.T) {
	setupRepoTestDB(t)

	ctx := context.Background()

	ds := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source"}
	if err := repository.Repo.CreateBoardDataSource(ctx, ds); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repository.Repo.DeleteBoardDataSource(ctx, ds.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := repository.Repo.GetBoardDataSourceByBoardAndType(ctx, 1, "test_source")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// TestBoardDataSource_RejectsRemovedFinancialSourceType verifies the spec
// scenario "金融 source_type 已移除": the removed built-in financial source types
// (etf_quote / exchange_rate / gdelt_event) are rejected by the CHECK-style
// application validation, since they are no longer registered.
func TestBoardDataSource_RejectsRemovedFinancialSourceType(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	for _, st := range []string{"etf_quote", "exchange_rate", "gdelt_event"} {
		ds := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: st, Enabled: true}
		if err := repository.Repo.CreateBoardDataSource(ctx, ds); err == nil {
			t.Errorf("create source_type=%s should be rejected (financial types removed), got nil", st)
		}
	}
}

// ── TopicLifelineContext ────────────────────────────────────────────────────

func TestTopicLifelineContext_UpsertAndGet(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	lc := &repository.TopicLifelineContext{
		PersistentTopicID: 1,
		Granularity:       "week",
		Period:            "2026-W27",
		Content:           "本周原油价格波动较大",
		Source:            "llm_assisted",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc); err != nil {
		t.Fatalf("upsert #1: %v", err)
	}

	// Update
	lc.Content = "updated content"
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc); err != nil {
		t.Fatalf("upsert #2: %v", err)
	}

	got, err := repository.Repo.GetTopicLifelineContext(ctx, 1, "week", "2026-W27")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "updated content" {
		t.Fatalf("content = %q, want 'updated content'", got.Content)
	}
}

// TestTopicLifelineContext_UpsertTOCTOU verifies that rapid sequential upserts
// with the same unique key are atomic (no duplicate-key error).
func TestTopicLifelineContext_UpsertTOCTOU(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	lc1 := &repository.TopicLifelineContext{
		PersistentTopicID: 1,
		Granularity:       "week",
		Period:            "2026-W27",
		Content:           "first upsert",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc1); err != nil {
		t.Fatalf("upsert #1: %v", err)
	}

	lc2 := &repository.TopicLifelineContext{
		PersistentTopicID: 1,
		Granularity:       "week",
		Period:            "2026-W27",
		Content:           "second upsert",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc2); err != nil {
		t.Fatalf("upsert #2 (TOCTOU): %v", err)
	}

	// Verify only one row exists.
	list, err := repository.Repo.ListTopicLifelineContextsByTopic(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row after upserts, got %d", len(list))
	}
	if list[0].Content != "second upsert" {
		t.Fatalf("content = %q, want 'second upsert'", list[0].Content)
	}
}

func TestTopicLifelineContext_ListByTopic(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	for _, g := range []string{"week", "month", "year", "all"} {
		lc := &repository.TopicLifelineContext{
			PersistentTopicID: 1, Granularity: g, Period: periodForGran(g), Content: g + " summary",
		}
		if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc); err != nil {
			t.Fatalf("upsert %s: %v", g, err)
		}
	}

	list, err := repository.Repo.ListTopicLifelineContextsByTopic(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("list count = %d, want 4", len(list))
	}
}

func TestTopicLifelineContext_ListStale(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	// Insert one recent context with period
	fresh := &repository.TopicLifelineContext{
		PersistentTopicID: 1, Granularity: "week", Period: "2026-W27", Content: "fresh",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, fresh); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// ListByGranularity should return the row.
	list, err := repository.Repo.ListTopicLifelineContextsByGranularity(ctx, 1, "week")
	if err != nil {
		t.Fatalf("list by granularity: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
	if list[0].Period != "2026-W27" {
		t.Fatalf("period = %q, want 2026-W27", list[0].Period)
	}

	// Test DeleteOlderThan: delete rows older than 2026-W28 (should keep 2026-W27 since it's before W28).
	err = repository.Repo.DeleteTopicLifelineContextsOlderThan(ctx, "week", "2026-W28")
	if err != nil {
		t.Fatalf("delete older than: %v", err)
	}

	list, err = repository.Repo.ListTopicLifelineContextsByGranularity(ctx, 1, "week")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 rows after delete (2026-W27 < 2026-W28), got %d", len(list))
	}
}

// periodForGran returns a dummy period string for testing.
func periodForGran(g string) string {
	switch g {
	case "week":
		return "2026-W27"
	case "month":
		return "2026-07"
	case "year":
		return "2026"
	default:
		return "all"
	}
}

// ── TopicEnrichmentResult ───────────────────────────────────────────────────

func TestTopicEnrichmentResult_CreateAndList(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	r1 := &repository.TopicEnrichmentResult{
		PersistentTopicID:   repository.TopicIDPtr(1),
		EvolutionAssessment: "first assessment",
		SessionID:           "session-1",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, r1); err != nil {
		t.Fatalf("create #1: %v", err)
	}

	r2 := &repository.TopicEnrichmentResult{
		PersistentTopicID:   repository.TopicIDPtr(1),
		EvolutionAssessment: "second assessment",
		SessionID:           "session-2",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, r2); err != nil {
		t.Fatalf("create #2: %v", err)
	}

	list, err := repository.Repo.ListTopicEnrichmentResultsByTopic(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
	// Should be in descending order by id
	if list[0].ID < list[1].ID {
		t.Fatal("expected descending order by id")
	}
}

func TestTopicEnrichmentResult_GetLatest(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	r1 := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), EvolutionAssessment: "first"}
	r2 := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), EvolutionAssessment: "latest"}
	_ = repository.Repo.CreateTopicEnrichmentResult(ctx, r1)
	_ = repository.Repo.CreateTopicEnrichmentResult(ctx, r2)

	got, err := repository.Repo.GetLatestTopicEnrichmentResult(ctx, 1)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if got.EvolutionAssessment != "latest" {
		t.Fatalf("assessment = %q, want 'latest'", got.EvolutionAssessment)
	}
}

func TestTopicEnrichmentResult_GetPrevLatest(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	r1 := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), EvolutionAssessment: "prev"}
	r2 := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), EvolutionAssessment: "curr"}
	_ = repository.Repo.CreateTopicEnrichmentResult(ctx, r1)
	_ = repository.Repo.CreateTopicEnrichmentResult(ctx, r2)

	prev, err := repository.Repo.GetPrevLatestTopicEnrichmentResult(ctx, 1, r2.ID)
	if err != nil {
		t.Fatalf("get prev: %v", err)
	}
	if prev.EvolutionAssessment != "prev" {
		t.Fatalf("assessment = %q, want 'prev'", prev.EvolutionAssessment)
	}
}

func TestTopicEnrichmentResult_GetByID(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	r := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), EvolutionAssessment: "test"}
	_ = repository.Repo.CreateTopicEnrichmentResult(ctx, r)

	got, err := repository.Repo.GetTopicEnrichmentResultByID(ctx, r.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.EvolutionAssessment != "test" {
		t.Fatalf("assessment = %q, want 'test'", got.EvolutionAssessment)
	}
}

// ── TopicEnrichmentReview ───────────────────────────────────────────────────

func TestTopicEnrichmentReview_CreateAndList(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	rv := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1),
		CurrResultID:      10,
		DeviationSummary:  "核心判断反转",
		Source:            "llm_assisted",
	}
	if err := repository.Repo.CreateTopicEnrichmentReview(ctx, rv); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rv.ID == 0 {
		t.Fatal("expected ID after create")
	}

	list, err := repository.Repo.ListTopicEnrichmentReviewsByTopic(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list count = %d, want 1", len(list))
	}
}

func TestTopicEnrichmentReview_ApplyAndListApplied(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	// Create two reviews, one applied, one not
	rv1 := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 1, DeviationSummary: "applied", Applied: true,
	}
	rv2 := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 2, DeviationSummary: "not applied", Applied: false,
	}
	_ = repository.Repo.CreateTopicEnrichmentReview(ctx, rv1)
	_ = repository.Repo.CreateTopicEnrichmentReview(ctx, rv2)

	applied, err := repository.Repo.ListAppliedTopicEnrichmentReviews(ctx, 1)
	if err != nil {
		t.Fatalf("list applied: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied count = %d, want 1", len(applied))
	}
	if !applied[0].Applied {
		t.Fatal("expected applied=true")
	}
}

func TestTopicEnrichmentReview_UpdateFields(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	rv := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 1, DeviationSummary: "original summary",
	}
	_ = repository.Repo.CreateTopicEnrichmentReview(ctx, rv)

	// Update deviation summary
	if err := repository.Repo.UpdateTopicEnrichmentReviewDeviation(ctx, rv.ID, "updated summary"); err != nil {
		t.Fatalf("update deviation: %v", err)
	}

	got, err := repository.Repo.GetTopicEnrichmentReviewByID(ctx, rv.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DeviationSummary != "updated summary" {
		t.Fatalf("deviation = %q, want 'updated summary'", got.DeviationSummary)
	}

	// Apply
	if err := repository.Repo.ApplyTopicEnrichmentReview(ctx, rv.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got, _ = repository.Repo.GetTopicEnrichmentReviewByID(ctx, rv.ID)
	if !got.Applied {
		t.Fatal("expected applied=true after ApplyTopicEnrichmentReview")
	}
}

// ── TopicEnrichmentQA ─────────────────────────────────────────────────────

func TestTopicEnrichmentQA_TableName(t *testing.T) {
	if got := (repository.TopicEnrichmentQA{}).TableName(); got != "topic_enrichment_qa" {
		t.Fatalf("TableName = %q, want %q", got, "topic_enrichment_qa")
	}
}

func TestTopicEnrichmentQA_CreateAndListByResultID(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	qa := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: 42,
		Question:                "这个板块的核心驱动力是什么？",
		Answer:                  "主要受政策预期与库存周期共振驱动。",
		ToolCalls:               json.RawMessage(`[{"name":"search","args":{"q":"半导体"}}]`),
		Source:                  "qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qa); err != nil {
		t.Fatalf("create: %v", err)
	}
	if qa.ID == 0 {
		t.Fatal("expected ID after create")
	}

	list, err := repository.Repo.ListTopicEnrichmentQAByResultID(ctx, 42)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list count = %d, want 1", len(list))
	}
	if list[0].Question != qa.Question {
		t.Fatalf("question = %q, want %q", list[0].Question, qa.Question)
	}
	if list[0].Answer != qa.Answer {
		t.Fatalf("answer = %q, want %q", list[0].Answer, qa.Answer)
	}
	if list[0].TopicEnrichmentResultID != 42 {
		t.Fatalf("result id = %d, want 42", list[0].TopicEnrichmentResultID)
	}

	// A different result_id must not leak in.
	other, err := repository.Repo.ListTopicEnrichmentQAByResultID(ctx, 999)
	if err != nil {
		t.Fatalf("list other result: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("other result count = %d, want 0", len(other))
	}
}

// TestTopicEnrichmentQA_MultiRoundOrdering verifies that multiple QA rounds
// on the same result_id are returned in chronological (created_at ASC) order,
// regardless of insertion order. Append-only: report rows are never rewritten.
func TestTopicEnrichmentQA_MultiRoundOrdering(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	older := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 18, 10, 5, 0, 0, time.UTC)

	qa1 := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: 7,
		Question:                "第一轮追问",
		CreatedAt:               older,
	}
	qa2 := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: 7,
		Question:                "第二轮追问",
		CreatedAt:               newer,
	}
	// Insert out of order to prove List sorts by created_at ASC, not insert order.
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qa2); err != nil {
		t.Fatalf("create qa2: %v", err)
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qa1); err != nil {
		t.Fatalf("create qa1: %v", err)
	}

	list, err := repository.Repo.ListTopicEnrichmentQAByResultID(ctx, 7)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
	if list[0].Question != "第一轮追问" || list[1].Question != "第二轮追问" {
		t.Fatalf("order = [%q, %q], want [第一轮追问, 第二轮追问] (created_at ASC)",
			list[0].Question, list[1].Question)
	}
}

// TestMarkQASedimented proves sediment flips Sedimented=true on the qa row and
// does NOT touch the immutable result table (业务约束#2).
func TestMarkQASedimented(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	// Seed a result + a qa row under it.
	result := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(1),
		SessionID:         "data_enrichment_1_seed",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("create result: %v", err)
	}
	qa := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: result.ID,
		Question:                "油价还会涨吗",
		Answer:                  "短期承压",
		Source:                  "qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qa); err != nil {
		t.Fatalf("create qa: %v", err)
	}
	if qa.Sedimented {
		t.Fatal("new qa row should default to sedimented=false")
	}

	// Snapshot the result before sediment to prove immutability.
	resultBefore, _ := repository.Repo.GetTopicEnrichmentResultByID(ctx, result.ID)

	if err := repository.Repo.MarkQASedimented(ctx, qa.ID); err != nil {
		t.Fatalf("MarkQASedimented: %v", err)
	}

	// The qa row is now flagged.
	after, err := repository.Repo.GetTopicEnrichmentQAByID(ctx, qa.ID)
	if err != nil {
		t.Fatalf("get qa after: %v", err)
	}
	if !after.Sedimented {
		t.Fatal("Sedimented should be true after MarkQASedimented")
	}

	// The result table is byte-for-byte unchanged.
	resultAfter, _ := repository.Repo.GetTopicEnrichmentResultByID(ctx, result.ID)
	beforeJSON, _ := json.Marshal(resultBefore)
	afterJSON, _ := json.Marshal(resultAfter)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("result table must be immutable across sediment:\nbefore=%s\nafter =%s", beforeJSON, afterJSON)
	}
}

// ── board-scope queries + reference roles (board-level-deep-analysis) ──────

func TestBoardEnrichmentResults_ScopeFiltering(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	// One topic-scope row and two board-scope rows for board 5.
	topicRow := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), AnalysisScope: "topic", SessionID: "s-topic"}
	boardRow1 := &repository.TopicEnrichmentResult{SemanticBoardID: repository.TopicIDPtr(5), AnalysisScope: "board", SessionID: "s-board-1"}
	boardRow2 := &repository.TopicEnrichmentResult{SemanticBoardID: repository.TopicIDPtr(5), AnalysisScope: "board", SessionID: "s-board-2"}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, topicRow); err != nil {
		t.Fatalf("create topic row: %v", err)
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, boardRow1); err != nil {
		t.Fatalf("create board row 1: %v", err)
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, boardRow2); err != nil {
		t.Fatalf("create board row 2: %v", err)
	}

	// Board list excludes the topic-scope row (M6.3 side).
	list, err := repository.Repo.ListBoardEnrichmentResults(ctx, 5)
	if err != nil {
		t.Fatalf("list board results: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("board list length: want 2, got %d", len(list))
	}
	// Newest first.
	if list[0].SessionID != "s-board-2" || list[1].SessionID != "s-board-1" {
		t.Fatalf("board list order wrong: %s, %s", list[0].SessionID, list[1].SessionID)
	}

	// Topic list excludes the board rows (regression).
	topicList, err := repository.Repo.ListTopicEnrichmentResultsByTopic(ctx, 1)
	if err != nil {
		t.Fatalf("list topic results: %v", err)
	}
	if len(topicList) != 1 {
		t.Fatalf("topic list length: want 1, got %d", len(topicList))
	}

	// Latest + prev-latest for review judge.
	latest, err := repository.Repo.GetLatestBoardEnrichmentResult(ctx, 5)
	if err != nil {
		t.Fatalf("get latest board result: %v", err)
	}
	if latest.SessionID != "s-board-2" {
		t.Fatalf("latest board result: want s-board-2, got %s", latest.SessionID)
	}
	prev, err := repository.Repo.GetPrevLatestBoardEnrichmentResult(ctx, 5, latest.ID)
	if err != nil {
		t.Fatalf("get prev board result: %v", err)
	}
	if prev.SessionID != "s-board-1" {
		t.Fatalf("prev board result: want s-board-1, got %s", prev.SessionID)
	}

	// Other board sees nothing.
	if _, err := repository.Repo.GetLatestBoardEnrichmentResult(ctx, 99); err == nil {
		t.Fatalf("expected error for empty board, got nil")
	}
}

func TestReferenceRole_CRUD(t *testing.T) {
	setupRepoTestDB(t)
	ctx := context.Background()

	// Create.
	r1 := &repository.ReferenceRole{Name: "inside-america", Title: "内部看美国·方法论画像", Content: "辩论流水线：钢人先行…", Enabled: true}
	if err := repository.Repo.CreateReferenceRole(ctx, r1); err != nil {
		t.Fatalf("create role: %v", err)
	}
	r2 := &repository.ReferenceRole{Name: "plain", Title: "朴素分析", Content: "直接了当", Enabled: false}
	if err := repository.Repo.CreateReferenceRole(ctx, r2); err != nil {
		t.Fatalf("create role 2: %v", err)
	}

	// Enabled-only listing (injection source, M7 ordering).
	enabled, err := repository.Repo.ListEnabledReferenceRoles(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 1 || enabled[0].Name != "inside-america" {
		t.Fatalf("enabled roles: want 1 inside-america, got %+v", enabled)
	}

	// Update (disable the first).
	r1.Enabled = false
	if err := repository.Repo.UpdateReferenceRole(ctx, r1); err != nil {
		t.Fatalf("update role: %v", err)
	}
	enabled, err = repository.Repo.ListEnabledReferenceRoles(ctx)
	if err != nil {
		t.Fatalf("list enabled after update: %v", err)
	}
	if len(enabled) != 0 {
		t.Fatalf("enabled after disable: want 0, got %d", len(enabled))
	}

	// Full listing still returns both, updated_at DESC.
	all, err := repository.Repo.ListReferenceRoles(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all roles: want 2, got %d", len(all))
	}

	// Get by ID + delete.
	got, err := repository.Repo.GetReferenceRoleByID(ctx, r2.ID)
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if got.Name != "plain" {
		t.Fatalf("get role name: want plain, got %s", got.Name)
	}
	if err := repository.Repo.DeleteReferenceRole(ctx, r2.ID); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	if _, err := repository.Repo.GetReferenceRoleByID(ctx, r2.ID); err == nil {
		t.Fatalf("expected error after delete, got nil")
	}
}
