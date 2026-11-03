// Command verify-lane-bucketing is a one-shot diagnostic for the "泳道内主题
// 分桶"（intra-lane theme bucketing）experiment: it takes ONE existing daily
// report section (a lane's whole day) and asks the digest_polish model to split
// its lines into 2-6 theme buckets, then prints the resulting buckets with tag
// coverage so we can eyeball whether the split is worth productizing.
//
// It is READ-ONLY: it never writes to the database and never calls the daily
// report pipeline. Run from backend-go/:
//
//	go run ./cmd/verify-lane-bucketing -section 7149
//	go run ./cmd/verify-lane-bucketing -section 7149 -source tags
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

type line struct {
	ID       uint
	Title    string
	Summary  string
	TagIDs   []uint
	TagLabel string // resolved labels, for eyeballing only
}

func main() {
	sectionID := flag.Uint("section", 7149, "daily_report_sections.id to bucket")
	source := flag.String("source", "threads", "input lines: threads | tags")
	temperature := flag.Float64("temp", 0.2, "LLM temperature")
	targetK := flag.Int("k", 0, "force an exact bucket count (0 = let the model decide)")
	fixedBuckets := flag.String("fixed", "", "comma-separated bucket names: only assign lines, never rename/merge/split")
	flag.Parse()

	if err := config.LoadConfig("./configs"); err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if config.AppConfig != nil {
		logging.Init(config.AppConfig.Log.Level, logging.FileConfig{Enabled: false})
	}
	if err := database.InitDB(config.AppConfig); err != nil {
		fmt.Fprintf(os.Stderr, "init db: %v\n", err)
		os.Exit(1)
	}
	db := database.DB
	repository.InitRepository(db)

	meta, err := loadSectionMeta(db, *sectionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load section: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("section #%d  board=%d  date=%s  lane=%q\n", meta.SectionID, meta.BoardID, meta.PeriodDate, meta.TopicLabel)
	fmt.Printf("  day title : %s\n", meta.ClusterLabel)
	fmt.Printf("  lane tier : %s   match=%s\n\n", meta.LaneTier, meta.Confidence)

	var lines []line
	switch *source {
	case "tags":
		lines, err = loadTagLines(db, meta.TagIDs)
	default:
		lines, err = loadThreadLines(db, *sectionID)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "load lines: %v\n", err)
		os.Exit(1)
	}
	if len(lines) < 2 {
		fmt.Fprintf(os.Stderr, "need >=2 lines, got %d\n", len(lines))
		os.Exit(1)
	}
	fmt.Printf("input: %d lines (source=%s)\n", len(lines), *source)
	for _, l := range lines {
		fmt.Printf("  [%d] %s  (tags:%d)\n", l.ID, truncate(l.Title, 48), len(l.TagIDs))
	}
	fmt.Println()

	system, user := buildBucketPrompt(meta.TopicLabel, lines, *targetK, *fixedBuckets)
	started := time.Now()
	content, usage, err := chat(context.Background(), system, user, *temperature)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(started)
	fmt.Printf("LLM done in %s", elapsed.Round(time.Millisecond))
	if usage != nil {
		fmt.Printf("  tokens: prompt=%d completion=%d total=%d", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}
	fmt.Printf("\n\n")

	buckets, err := parseBuckets(content, lines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse buckets: %v\nraw:\n%s\n", err, content)
		os.Exit(1)
	}

	titleByID := make(map[uint]line, len(lines))
	for _, l := range lines {
		titleByID[l.ID] = l
	}
	seen := make(map[uint]int, len(lines))
	fmt.Printf("=== %d buckets ===\n", len(buckets))
	for i, b := range buckets {
		var tagCount int
		fmt.Printf("\n[%d] %s\n    当日标题: %s\n", i+1, b.BucketTitle, b.SectionTitle)
		for _, id := range b.LineIDs {
			seen[id]++
			l := titleByID[id]
			tagCount += len(l.TagIDs)
			mark := " "
			if seen[id] > 1 {
				mark = "!"
			}
			fmt.Printf("   %s [%d] %s\n", mark, id, truncate(l.Title, 56))
		}
		fmt.Printf("     → %d lines / %d tags\n", len(b.LineIDs), tagCount)
	}

	var missing, dup int
	for _, l := range lines {
		switch seen[l.ID] {
		case 0:
			missing++
			fmt.Printf("\nMISSING line [%d] %s\n", l.ID, l.Title)
		case 1:
		default:
			dup++
		}
	}
	fmt.Printf("\ncoverage: %d/%d lines, missing=%d duplicated=%d\n", len(lines)-missing, len(lines), missing, dup)
}

type sectionMeta struct {
	SectionID    uint
	BoardID      uint
	PeriodDate   string
	ClusterLabel string
	LaneTier     string
	Confidence   string
	TopicLabel   string
	TagIDs       []uint
}

func loadSectionMeta(db *gorm.DB, sectionID uint) (sectionMeta, error) {
	var row struct {
		SectionID    uint
		BoardID      uint
		PeriodDate   time.Time
		ClusterLabel string
		LaneTier     string
		Confidence   string
		TopicLabel   string
		TagIDs       string
	}
	err := db.Raw(`
		SELECT s.id AS section_id, r.semantic_board_id AS board_id, r.period_date AS period_date,
		       COALESCE(s.cluster_label,'') AS cluster_label, COALESCE(s.lane_tier,'') AS lane_tier,
		       COALESCE(s.topic_match_confidence,'') AS confidence,
		       COALESCE(t.label,'') AS topic_label,
		       COALESCE(s.cluster_tag_ids::text,'[]') AS tag_ids
		FROM daily_report_sections s
		JOIN board_daily_reports r ON r.id = s.report_id
		LEFT JOIN board_persistent_topics t ON t.id = s.persistent_topic_id
		WHERE s.id = ?`, sectionID).Scan(&row).Error
	if err != nil {
		return sectionMeta{}, err
	}
	if row.SectionID == 0 {
		return sectionMeta{}, fmt.Errorf("section %d not found", sectionID)
	}
	var ids []uint
	_ = json.Unmarshal([]byte(row.TagIDs), &ids)
	return sectionMeta{
		SectionID:    row.SectionID,
		BoardID:      row.BoardID,
		PeriodDate:   row.PeriodDate.Format("2006-01-02"),
		ClusterLabel: row.ClusterLabel,
		LaneTier:     row.LaneTier,
		Confidence:   row.Confidence,
		TopicLabel:   row.TopicLabel,
		TagIDs:       ids,
	}, nil
}

// loadThreadLines reads the section's existing threads as bucket input. Threads
// are already fact-anchored summaries, so they cost fewer tokens than raw tags
// and reflect what the reader actually sees.
func loadThreadLines(db *gorm.DB, sectionID uint) ([]line, error) {
	var rows []struct {
		ID      uint
		Title   string
		Summary string
		TagIDs  string
	}
	err := db.Raw(`
		SELECT id, COALESCE(title,'') AS title, COALESCE(summary,'') AS summary,
		       COALESCE(tag_ids::text,'[]') AS tag_ids
		FROM daily_report_threads WHERE section_id = ? ORDER BY id`, sectionID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]line, 0, len(rows))
	for _, r := range rows {
		var ids []uint
		_ = json.Unmarshal([]byte(r.TagIDs), &ids)
		out = append(out, line{ID: r.ID, Title: r.Title, Summary: r.Summary, TagIDs: ids})
	}
	return out, nil
}

// loadTagLines reads the section's tags as one-line-per-tag input (the
// alternative signal, for comparing against the thread-based split).
func loadTagLines(db *gorm.DB, tagIDs []uint) ([]line, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	var rows []struct {
		ID          uint
		Label       string
		Description string
	}
	err := db.Raw(`
		SELECT id, COALESCE(label,'') AS label, COALESCE(description,'') AS description
		FROM topic_tags WHERE id IN ?`, tagIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]line, 0, len(rows))
	for _, r := range rows {
		out = append(out, line{ID: r.ID, Title: r.Label, Summary: r.Description, TagIDs: []uint{r.ID}})
	}
	return out, nil
}

const bucketSystemPrompt = `你是一名资深科技新闻编辑。下面给出某个"领域级话题"在某一天抓到的全部线索（每条线索 = 当天的一个事件，带标题与摘要）。

这些线索目前平铺在一起，读者读完只觉得"很杂"。你的任务：按【主题方向】把它们分成若干组，让读者一眼看出"这一天这个领域里，大家在往哪几个方向使劲"。

分组规则：
1. 分组依据是"方向 / 关注点"（例如：用 AI 做游戏或产品；做工具与插件；编排与工程化 harness；Skill 与规范生态；推理与上下文基础设施；安全与风控；商业化与市场），不是"同一新闻事件的多篇报道"，也不是"同一家公司"。
2. 每组至少 2 条线索；如果全天线索确实同属一个方向，允许只输出 1 组。
3. 最多 6 组。宁少勿碎：两条线索能合进同一方向就合。
4. 每条线索必须恰好出现在一组里，不得遗漏、不得重复。
5. 组名不超过 12 字，必须是"方向"而不是某条具体事件，也不得复述上面给出的领域话题名。
6. 每组再给一个"当日章节标题"（不超过 30 字），用组内线索的事实概括当天这组里发生了什么；禁止编造未出现在线索中的事实、数字与因果。
7. 实在无法归入任何方向的线索，放进"其他动态"组，同样遵守规则 2 和 4。

只返回合法 JSON，不要 Markdown 代码块或解释文字：
{"groups":[{"bucket_title":"...","section_title":"...","line_ids":[整数]}]}`

func buildBucketPrompt(laneLabel string, lines []line, targetK int, fixed string) (system, user string) {
	system = bucketSystemPrompt

	var sb strings.Builder
	if fixed != "" {
		system = bucketAssignSystemPrompt
		fmt.Fprintf(&sb, "固定分组（只能把线索归入下列分组，不得新增、改名、合并或拆分）：%s\n\n", fixed)
	} else if targetK > 0 {
		fmt.Fprintf(&sb, "本次要求：恰好分成 %d 组（覆盖全部线索，且每组至少 2 条）。\n\n", targetK)
	}
	fmt.Fprintf(&sb, "领域话题（泳道名，仅用于界定语境，不得作为组名复述）：%s\n\n", laneLabel)
	sb.WriteString("## 当天线索\n\n")
	for _, l := range lines {
		fmt.Fprintf(&sb, "### [line:%d] %s\n", l.ID, l.Title)
		if s := strings.TrimSpace(l.Summary); s != "" {
			fmt.Fprintf(&sb, "%s\n", truncate(s, 220))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("请按主题方向分组，只输出 JSON。\n")
	return system, sb.String()
}

type bucket struct {
	BucketTitle  string `json:"bucket_title"`
	SectionTitle string `json:"section_title"`
	LineIDs      []uint `json:"line_ids"`
}

// bucketAssignSystemPrompt is the phase-2 prompt of the two-phase design: the
// bucket set is fixed upstream (deterministic count), the model only assigns.
// Assignment is a per-line classification, which should be far more stable than
// free-form grouping.
const bucketAssignSystemPrompt = `你是一名资深科技新闻编辑。下面给出某个"领域级话题"在某一天抓到的全部线索，以及一组已确定的分组名。

你的任务：把每条线索归入其中一个分组。

规则：
1. 只能使用给定的分组名，不得新增、改名、合并、拆分或删除分组。
2. 每条线索必须恰好归入一个分组，不得遗漏、不得重复。
3. 归属依据是线索的"方向 / 关注点"，不是"同一事件"或"同一家公司"。
4. 若某条线索确实与所有分组都不搭，也请选择最接近的一组（必要时用"其他动态"，仅当该分组存在时）。

只返回合法 JSON，不要 Markdown 代码块或解释文字：
{"groups":[{"bucket_title":"给定分组名","line_ids":[整数]}]}`

func parseBuckets(content string, lines []line) ([]bucket, error) {
	cleaned := strings.TrimSpace(content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	var raw struct {
		Groups []bucket `json:"groups"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &raw); err != nil {
		return nil, err
	}
	return raw.Groups, nil
}

func bucketSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"groups": {
				Type: "array",
				Items: &airouter.SchemaProperty{
					Type: "object",
					Properties: map[string]airouter.SchemaProperty{
						"bucket_title":  {Type: "string", Description: "方向名，不超过12字"},
						"section_title": {Type: "string", Description: "当日章节标题，不超过30字，基于组内事实"},
						"line_ids":      {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
					},
					Required: []string{"bucket_title", "section_title", "line_ids"},
				},
			},
		},
		Required: []string{"groups"},
	}
}

func chat(ctx context.Context, system, user string, temperature float64) (string, *airouter.TokenUsage, error) {
	maxTokens := 2000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Operation:  "daily_report.bucket_threads",
		Capability: airouter.CapabilityDigestPolish,
		Messages: []airouter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema:  bucketSchema(),
		Metadata:    map[string]any{"operation": "daily_report_bucket_threads_experiment"},
	})
	if err != nil {
		return "", nil, err
	}
	return result.Content, result.Usage, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
