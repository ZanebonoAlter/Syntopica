package service_test

import (
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/service"
)

// mockLifelineReader implements service.LifelineReader.
type mockLifelineReader struct {
	data    service.SectionTimelineData
	archive []service.LifelineArchiveRow
	archErr error
}

func (m *mockLifelineReader) GetTopicLifeline(topicID uint) (service.SectionTimelineData, error) {
	return m.data, nil
}

func (m *mockLifelineReader) GetTopicLifelineArchive(topicID uint) ([]service.LifelineArchiveRow, error) {
	return m.archive, m.archErr
}

func TestRenderLifelineForAgent_ContainsTopicBody(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{
				ID:              1,
				Label:           "中东地缘紧张与能源连锁反应",
				Description:     "产油国设施遭袭 → 油价飙升 → 下游成本承压",
				Status:          "active",
				Source:          "auto",
				FirstSeenDate:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				LastSeenDate:    time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
				HitCount:        4,
				ConsecutiveHits: 4,
			},
			Sections: []service.TimelineSectionNode{
				{
					SectionID: 101, PeriodDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
					ClusterLabel: "产油国石油设施遭袭", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 8, ThreadCount: 2,
					ThreadTitles: []string{"布伦特原油涨幅超6%", "市场恐慌蔓延"},
				},
				{
					SectionID: 102, PeriodDate: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
					ClusterLabel: "油价飙升传导", Status: "continuing",
					TopicMatchConfidence: "anchor_hit", ArticleCount: 12, ThreadCount: 3,
					ThreadTitles: []string{"油气开采股价走高", "化工行业承压"},
				},
			},
		},
	}

	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Topic body checks
	if !strings.Contains(output, "中东地缘紧张与能源连锁反应") {
		t.Fatal("output should contain topic label")
	}
	if !strings.Contains(output, "产油国设施遭袭") {
		t.Fatal("output should contain topic description")
	}
}

func TestRenderLifelineForAgent_ContainsDayByDaySections(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{
				ID: 1, Label: "test", Status: "active", Source: "auto",
				FirstSeenDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				LastSeenDate:  time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
			},
			Sections: []service.TimelineSectionNode{
				{SectionID: 1, PeriodDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), ClusterLabel: "day1", Status: "emerging", TopicMatchConfidence: "auto_new", ArticleCount: 3, ThreadCount: 1},
				{SectionID: 2, PeriodDate: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), ClusterLabel: "day2", Status: "continuing", TopicMatchConfidence: "anchor_hit", ArticleCount: 5, ThreadCount: 2, ThreadTitles: []string{"t1"}},
			},
		},
	}

	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(output, "07-01") || !strings.Contains(output, "07-02") {
		t.Fatal("output should contain day-by-day dates")
	}
}

func TestRenderLifelineForAgent_StatusChineseMapping(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	tests := []struct {
		status   string
		wantText string
	}{
		{"emerging", "涌现"},
		{"continuing", "延续"},
		{"split", "分叉"},
		{"merge", "合并"},
		{"ending", "结束"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			reader := &mockLifelineReader{
				data: service.SectionTimelineData{
					Topic: service.TopicBrief{
						ID: 1, Label: "test", Status: "active", Source: "auto",
						FirstSeenDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
						LastSeenDate:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
					},
					Sections: []service.TimelineSectionNode{
						{SectionID: 1, PeriodDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), ClusterLabel: "test", Status: tt.status, TopicMatchConfidence: "auto_new", ArticleCount: 1, ThreadCount: 1},
					},
				},
			}
			output, _ := renderer.RenderLifelineForAgent(reader, 1, 14)
			if !strings.Contains(output, tt.wantText) {
				t.Fatalf("output for status %q should contain %q, got:\n%s", tt.status, tt.wantText, output)
			}
		})
	}
}

func TestRenderLifelineForAgent_ContainsRelationsSection(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{
				ID: 1, Label: "test-topic", Status: "active", Source: "auto",
				FirstSeenDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				LastSeenDate:  time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
			},
			Sections: []service.TimelineSectionNode{
				{SectionID: 101, PeriodDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), ClusterLabel: "板块A", Status: "emerging", TopicMatchConfidence: "auto_new", ArticleCount: 3, ThreadCount: 1},
				{SectionID: 102, PeriodDate: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), ClusterLabel: "板块B", Status: "continuing", TopicMatchConfidence: "anchor_hit", ArticleCount: 5, ThreadCount: 2},
				{SectionID: 103, PeriodDate: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), ClusterLabel: "板块C", Status: "split", TopicMatchConfidence: "anchor_hit", ArticleCount: 4, ThreadCount: 1},
			},
			Relations: []service.SectionRelation{
				{FromID: 101, ToID: 102, Distance: 0.82, RelationType: "identity"},
				{FromID: 102, ToID: 103, Distance: 0.45, RelationType: "similarity"},
			},
		},
	}

	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Verify the relations section header exists.
	if !strings.Contains(output, "section 关联关系") {
		t.Fatalf("output should contain relations header, got:\n%s", output)
	}

	// Verify from section date + cluster_label.
	if !strings.Contains(output, "07-01") || !strings.Contains(output, "[板块A]") {
		t.Fatal("output should contain from section date and label")
	}

	// Verify to section date + cluster_label.
	if !strings.Contains(output, "07-02") || !strings.Contains(output, "[板块B]") {
		t.Fatal("output should contain to section date and label")
	}

	// Verify relation_type is present.
	if !strings.Contains(output, "identity") {
		t.Fatal("output should contain relation_type 'identity'")
	}
	if !strings.Contains(output, "similarity") {
		t.Fatal("output should contain relation_type 'similarity'")
	}

	// Verify distance value.
	if !strings.Contains(output, "0.82") {
		t.Fatal("output should contain distance 0.82")
	}
	if !strings.Contains(output, "0.45") {
		t.Fatal("output should contain distance 0.45")
	}
}

func TestRenderLifelineForAgent_NoRelationsSkipsSection(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{
				ID: 1, Label: "test", Status: "active", Source: "auto",
				FirstSeenDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				LastSeenDate:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			},
			Sections: []service.TimelineSectionNode{
				{SectionID: 1, PeriodDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), ClusterLabel: "day1", Status: "emerging", TopicMatchConfidence: "auto_new", ArticleCount: 3, ThreadCount: 1},
			},
			Relations: []service.SectionRelation{},
		},
	}

	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if strings.Contains(output, "section 关联关系") {
		t.Fatal("empty relations should NOT render relations section")
	}
}

func TestRenderLifelineForAgent_EmptySections(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{
				ID: 1, Label: "test", Status: "active", Source: "auto",
			},
			Sections: []service.TimelineSectionNode{},
		},
	}

	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(output, "话题本体") {
		t.Fatal("even empty sections should render topic body")
	}
}

// ── 历史背景记忆档案段（fix-board-analysis-material tasks 2.2 / M3）──────────

func TestRenderLifelineForAgent_ArchiveRendered(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data: service.SectionTimelineData{Topic: service.TopicBrief{ID: 1, Label: "lane"}},
		archive: []service.LifelineArchiveRow{
			{Granularity: "month", Period: "2026-08", AsOfDate: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), Content: "8月主线：制裁升级，海峡风险上升。"},
			{Granularity: "month", Period: "2026-07", AsOfDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC), Content: "7月主线：谈判拉锯。"},
			{Granularity: "year", Period: "2026", AsOfDate: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC), Content: "年度背景：美伊长期对抗结构。"},
		},
	}
	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"## 历史背景记忆（月/年档案）",
		"### [month 2026-08] (as_of 2026-08-26)",
		"制裁升级",
		"### [month 2026-07]",
		"### [year 2026]",
		"年度背景",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("archive render missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "[档案截断]") {
		t.Fatal("small archive must not be truncated")
	}
}

func TestRenderLifelineForAgent_ArchiveTruncationBudget(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data: service.SectionTimelineData{Topic: service.TopicBrief{ID: 1, Label: "lane"}},
		archive: []service.LifelineArchiveRow{
			{Granularity: "month", Period: "2026-08", AsOfDate: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC),
				Content: strings.Repeat("八月事实段落。", 1500)}, // ≈9000 runes > 4000 budget
		},
	}
	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(output, "[档案截断]") {
		t.Fatal("oversized archive must carry truncation marker")
	}
}

func TestRenderLifelineForAgent_ArchiveMissingStated(t *testing.T) {
	renderer := service.NewLifelineRenderer()
	reader := &mockLifelineReader{
		data:    service.SectionTimelineData{Topic: service.TopicBrief{ID: 1, Label: "lane"}},
		archive: nil,
	}
	output, err := renderer.RenderLifelineForAgent(reader, 1, 14)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(output, "## 历史背景记忆（月/年档案）") || !strings.Contains(output, "（无背景记忆归档）") {
		t.Fatalf("missing archive must be stated honestly, got:\n%s", output)
	}
}
