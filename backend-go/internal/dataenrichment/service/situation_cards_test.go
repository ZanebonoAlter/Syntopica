package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 态势卡装配器（tasks 3.1 / M1）────────────────────────────────────────────

// setupSituationDB seeds a board with lanes in fixture/laneRow.go shape:
// board_persistent_topics rows + optional week lifelines + optional recent
// sections via the mock lifeline reader.
func seedLane(t *testing.T, db *gorm.DB, id uint, label, desc string, hits, consec int, lastSeen time.Time) {
	t.Helper()
	mustExec(t, db, fmt.Sprintf(
		`INSERT INTO board_persistent_topics (id, semantic_board_id, label, description, status, source, first_seen_date, last_seen_date, hit_count, consecutive_hits, created_at, updated_at)
		 VALUES (%d, 7701, '%s', '%s', 'active', 'auto', '%s', '%s', %d, %d, datetime('now'), datetime('now'))`,
		id, label, desc, lastSeen.Format("2006-01-02"), lastSeen.Format("2006-01-02"), hits, consec))
}

func seedWeekLifeline(t *testing.T, db *gorm.DB, topicID uint, period, content string, asOf time.Time) {
	t.Helper()
	mustExec(t, db, fmt.Sprintf(
		`INSERT INTO topic_lifeline_context (persistent_topic_id, granularity, period, content, as_of_date, source, created_at, updated_at)
		 VALUES (%d, 'week', '%s', '%s', '%s', 'manual', datetime('now'), datetime('now'))`,
		topicID, period, content, asOf.Format("2006-01-02")))
}

func seedMonthLifeline(t *testing.T, db *gorm.DB, topicID uint, period, content string, asOf time.Time) {
	t.Helper()
	mustExec(t, db, fmt.Sprintf(
		`INSERT INTO topic_lifeline_context (persistent_topic_id, granularity, period, content, as_of_date, source, created_at, updated_at)
		 VALUES (%d, 'month', '%s', '%s', '%s', 'manual', datetime('now'), datetime('now'))`,
		topicID, period, content, asOf.Format("2006-01-02")))
}

func seedTopicResult(t *testing.T, db *gorm.DB, topicID uint, form string) {
	t.Helper()
	// GORM Create (not raw SQL): raw-INSERTed SQLite text does not scan back
	// into json.RawMessage on read.
	res := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(topicID),
		AnalysisScope:     "topic",
		Sectors:           json.RawMessage(fmt.Sprintf(`{"form":"%s"}`, form)),
		SessionID:         fmt.Sprintf("sit-card-%d-%d", topicID, time.Now().UnixNano()),
	}
	if err := db.Create(res).Error; err != nil {
		t.Fatalf("seed topic result: %v", err)
	}
}

func mustExec(t *testing.T, db *gorm.DB, sql string) {
	t.Helper()
	if err := db.Exec(sql).Error; err != nil {
		t.Fatalf("exec %q: %v", sql[:min(60, len(sql))], err)
	}
}

func newSituationOrch(t *testing.T, lifeline service.SectionTimelineData) (*service.OrchestratorService, *repository.Repository) {
	t.Helper()
	repo := setupOrchTestDB(t)
	orch := service.NewOrchestratorService(
		newMockAirRouter(), repo, &orchMockLifelineReader{data: lifeline}, service.NewLifelineRenderer(),
		service.NewRegistry(&nilFetcher{}), &mockBoardConfigReader{cfg: service.DefaultBoardConfig()}, testCap,
	)
	return orch, repo
}

// M1.1 多泳道排序：质量分（活跃度+密度-稀疏惩罚）降序，平分按 lane_id。
func TestSituationCards_QualityOrdering(t *testing.T) {
	today := time.Now()
	lifeline := service.SectionTimelineData{Topic: service.TopicBrief{ID: 1}}
	orch, repo := newSituationOrch(t, lifeline)
	db := repo.DB()

	// hot lane: consecutive 10, seen today → activity ≈ 34
	seedLane(t, db, 101, "hot", "", 30, 10, today)
	// cold lane: consecutive 1, unseen 20 days → activity ≈ 2
	seedLane(t, db, 102, "cold", "", 30, 1, today.AddDate(0, 0, -20))
	// sparse-history lane: hot stats but 2 sparse results → −6
	seedLane(t, db, 103, "sparse", "", 30, 10, today)
	seedTopicResult(t, db, 103, "sparse")
	seedTopicResult(t, db, 103, "sparse")

	cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("want 3 cards, got %d", len(cards))
	}
	if cards[0].LaneID != 101 || cards[1].LaneID != 103 || cards[2].LaneID != 102 {
		t.Fatalf("quality order wrong: got %d,%d,%d want 101,103,102", cards[0].LaneID, cards[1].LaneID, cards[2].LaneID)
	}
}

// M1.2 稀疏降级：连续 sparse 历史 → 详情降 brief，digest 收紧。
func TestSituationCards_SparseDegradation(t *testing.T) {
	today := time.Now()
	lifeline := service.SectionTimelineData{Topic: service.TopicBrief{ID: 103}}
	orch, repo := newSituationOrch(t, lifeline)
	db := repo.DB()

	seedLane(t, db, 201, "degraded", "长描述"+strings.Repeat("字", 100), 30, 10, today)
	seedTopicResult(t, db, 201, "sparse")
	seedTopicResult(t, db, 201, "sparse")

	cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	c := cards[0]
	if c.Signals.SparseHistory != 2 {
		t.Fatalf("sparse history: want 2, got %d", c.Signals.SparseHistory)
	}
	if c.DetailLevel != "brief" {
		t.Fatalf("double-sparse lane must degrade to brief, got %s", c.DetailLevel)
	}
	// brief digest is capped at situationCardBriefRunes (48) + ellipsis.
	if len([]rune(c.FactsDigest)) > 49 {
		t.Fatalf("brief digest too long: %d runes", len([]rune(c.FactsDigest)))
	}
}

// M1.3 无 lifeline 兜底：week lifeline 缺失时降级 section 指纹 / 描述 / 空。
func TestSituationCards_FallbackChain(t *testing.T) {
	today := time.Now()

	t.Run("section fingerprint", func(t *testing.T) {
		lifeline := service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 301},
			Sections: []service.TimelineSectionNode{
				{SectionID: 1, PeriodDate: today.AddDate(0, 0, -1), ClusterLabel: "政策转向", ArticleCount: 8},
			},
		}
		orch, repo := newSituationOrch(t, lifeline)
		seedLane(t, repo.DB(), 301, "withsections", "", 10, 3, today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if cards[0].FactsSource != "section_fingerprint" {
			t.Fatalf("want section_fingerprint, got %s", cards[0].FactsSource)
		}
		if !strings.Contains(cards[0].FactsDigest, "政策转向") || !strings.Contains(cards[0].FactsDigest, "8篇") {
			t.Fatalf("fingerprint digest wrong: %q", cards[0].FactsDigest)
		}
	})

	t.Run("lifeline preferred over sections", func(t *testing.T) {
		lifeline := service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 302},
			Sections: []service.TimelineSectionNode{
				{SectionID: 1, PeriodDate: today, ClusterLabel: "sectionlabel", ArticleCount: 5},
			},
		}
		orch, repo := newSituationOrch(t, lifeline)
		db := repo.DB()
		seedLane(t, db, 302, "withlifeline", "", 10, 3, today)
		seedWeekLifeline(t, db, 302, "2026-W34", "周摘要内容", today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if cards[0].FactsSource != "lifeline_week" {
			t.Fatalf("want lifeline_week, got %s", cards[0].FactsSource)
		}
		if !strings.Contains(cards[0].FactsDigest, "2026-W34") || !strings.Contains(cards[0].FactsDigest, "周摘要内容") {
			t.Fatalf("lifeline digest wrong: %q", cards[0].FactsDigest)
		}
	})

	t.Run("description only", func(t *testing.T) {
		orch, repo := newSituationOrch(t, service.SectionTimelineData{Topic: service.TopicBrief{ID: 303}})
		seedLane(t, repo.DB(), 303, "desconly", "只有描述的泳道", 10, 3, today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if cards[0].FactsSource != "description" {
			t.Fatalf("want description, got %s", cards[0].FactsSource)
		}
		if !strings.Contains(cards[0].FactsDigest, "只有描述的泳道") {
			t.Fatalf("digest should carry description, got %q", cards[0].FactsDigest)
		}
	})

	t.Run("nothing readable → none", func(t *testing.T) {
		orch, repo := newSituationOrch(t, service.SectionTimelineData{Topic: service.TopicBrief{ID: 304}})
		seedLane(t, repo.DB(), 304, "bare", "", 10, 3, today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if cards[0].FactsSource != "none" || cards[0].FactsDigest != "" {
			t.Fatalf("want none/empty, got %s/%q", cards[0].FactsSource, cards[0].FactsDigest)
		}
	})
}

// M1.1b 生产形态（month 在、week 缺）：month 档兜底成为主要事实源。
// 生产库实测：month 全量 67 泳道在库，week 仅 2（lifeline_weekly 从未跑）。
func TestSituationCards_MonthBackstop(t *testing.T) {
	today := time.Now()

	t.Run("month without week → lifeline_month", func(t *testing.T) {
		orch, repo := newSituationOrch(t, service.SectionTimelineData{Topic: service.TopicBrief{ID: 311}})
		db := repo.DB()
		seedLane(t, db, 311, "monthbacked", "", 10, 3, today)
		seedMonthLifeline(t, db, 311, "2026-08", "8月主线：美国对伊制裁升级，霍尔木兹海峡风险上升。", today)
		seedMonthLifeline(t, db, 311, "2026-07", "7月主线：美伊谈判多轮拉锯。", today.AddDate(0, -1, 0))
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		c := cards[0]
		if c.FactsSource != "lifeline_month" {
			t.Fatalf("want lifeline_month, got %s", c.FactsSource)
		}
		if !strings.Contains(c.FactsDigest, "2026-08") || !strings.Contains(c.FactsDigest, "制裁升级") {
			t.Fatalf("month digest wrong: %q", c.FactsDigest)
		}
		// density carries the +2 lifeline-backed bonus.
		if c.Signals.DensityScore < 2.0 {
			t.Fatalf("lifeline-backed density bonus missing: %.2f", c.Signals.DensityScore)
		}
	})

	t.Run("week still preferred over month", func(t *testing.T) {
		orch, repo := newSituationOrch(t, service.SectionTimelineData{Topic: service.TopicBrief{ID: 312}})
		db := repo.DB()
		seedLane(t, db, 312, "both", "", 10, 3, today)
		seedWeekLifeline(t, db, 312, "2026-W35", "本周：制裁落地执行。", today)
		seedMonthLifeline(t, db, 312, "2026-08", "8月主线：……", today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if cards[0].FactsSource != "lifeline_week" {
			t.Fatalf("want lifeline_week, got %s", cards[0].FactsSource)
		}
	})

	t.Run("month rows with blank content fall through to fingerprint", func(t *testing.T) {
		lifeline := service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 313},
			Sections: []service.TimelineSectionNode{
				{SectionID: 9, PeriodDate: today.AddDate(0, 0, -1), ClusterLabel: "对伊制裁", ArticleCount: 6,
					ThreadTitles: []string{"美财政部宣布新制裁", "伊议员威胁封锁海峡"}},
			},
		}
		orch, repo := newSituationOrch(t, lifeline)
		db := repo.DB()
		seedLane(t, db, 313, "blankmonth", "", 10, 3, today)
		seedMonthLifeline(t, db, 313, "2026-08", "", today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if cards[0].FactsSource != "section_fingerprint" {
			t.Fatalf("want section_fingerprint, got %s", cards[0].FactsSource)
		}
	})
}

// M1.2b/M2 指纹提质：thread 标题优先，无 threads 退回 cluster_label。
func TestSituationCards_FingerprintThreadTitles(t *testing.T) {
	today := time.Now()

	t.Run("thread titles replace cluster label", func(t *testing.T) {
		lifeline := service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 321},
			Sections: []service.TimelineSectionNode{
				{SectionID: 1, PeriodDate: today.AddDate(0, 0, -1), ClusterLabel: "美伊局势", ArticleCount: 8,
					ThreadTitles: []string{"美宣布对伊史上最严制裁", "伊朗警告封锁霍尔木兹", "伊外长呼吁对话", "第四条不该进指纹"}},
			},
		}
		orch, repo := newSituationOrch(t, lifeline)
		seedLane(t, repo.DB(), 321, "threads", "", 10, 3, today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if cards[0].FactsSource != "section_fingerprint" {
			t.Fatalf("want section_fingerprint, got %s", cards[0].FactsSource)
		}
		d := cards[0].FactsDigest
		for _, want := range []string{"美宣布对伊史上最严制裁", "伊朗警告封锁霍尔木兹", "伊外长呼吁对话", "8篇"} {
			if !strings.Contains(d, want) {
				t.Fatalf("fingerprint missing %q: %s", want, d)
			}
		}
		if strings.Contains(d, "第四条不该进指纹") {
			t.Fatalf("fingerprint must cap at 3 thread titles: %s", d)
		}
	})

	t.Run("no threads falls back to cluster label", func(t *testing.T) {
		lifeline := service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 322},
			Sections: []service.TimelineSectionNode{
				{SectionID: 1, PeriodDate: today.AddDate(0, 0, -1), ClusterLabel: "纯标签", ArticleCount: 3},
			},
		}
		orch, repo := newSituationOrch(t, lifeline)
		seedLane(t, repo.DB(), 322, "nothreads", "", 10, 3, today)
		cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if !strings.Contains(cards[0].FactsDigest, "纯标签") {
			t.Fatalf("cluster-label fallback missing: %q", cards[0].FactsDigest)
		}
	})
}

// M1.4 泳道数上限：低质量泳道被截断，上限 12。
func TestSituationCards_LaneCap(t *testing.T) {
	today := time.Now()
	lifeline := service.SectionTimelineData{Topic: service.TopicBrief{ID: 1}}
	orch, repo := newSituationOrch(t, lifeline)
	db := repo.DB()
	for i := 0; i < 20; i++ {
		seedLane(t, db, uint(400+i), fmt.Sprintf("lane%d", i), "", 5, 1, today)
	}
	cards, err := orch.AssembleSituationCardsForTest(context.Background(), 7701)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(cards) != 12 {
		t.Fatalf("cap: want 12 cards, got %d", len(cards))
	}
}

// M1.5 渲染：markdown 含命中统计 + 事实行。
func TestSituationCards_RenderMarkdown(t *testing.T) {
	cards := []service.LaneSituationCard{{
		LaneID: 501, Label: "测试泳道", HitCount: 30, ConsecutiveHits: 9,
		LastSeenDate: "2026-08-26", DaysSinceSeen: 0, QualityScore: 28.5,
		DetailLevel: "full", FactsDigest: "最近事实",
	}}
	md := service.RenderSituationCardsMarkdown(cards)
	for _, want := range []string{"泳道#501", "测试泳道", "命中30天", "最近事实", "28.5"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}
