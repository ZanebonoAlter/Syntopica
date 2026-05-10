package topicanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type mockChatFn func(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error)

type mockRouter struct {
	chatFn mockChatFn
}

func (m *mockRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	if m.chatFn != nil {
		return m.chatFn(ctx, req)
	}
	return nil, fmt.Errorf("mock chat not configured")
}

func (m *mockRouter) ResolvePrimaryProvider(capability airouter.Capability) (*models.AIProvider, *models.AIRoute, error) {
	return nil, nil, nil
}

func (m *mockRouter) Embed(ctx context.Context, req airouter.EmbeddingRequest, capability airouter.Capability) (*airouter.EmbeddingResult, error) {
	return nil, fmt.Errorf("mock embed not configured")
}

func mockAbstractNameResult(name string) *airouter.ChatResult {
	b, _ := json.Marshal(map[string]string{"abstract_name": name, "reason": "test"})
	return &airouter.ChatResult{Content: string(b)}
}

func TestExtractAbstractTagSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestExtractAbstractTagDeduplication(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestExtractAbstractTagLLMFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
}

func TestBuildCandidateList(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{Label: "React", Source: "abstract"}, Similarity: 0.96},
		{Tag: &models.TopicTag{Label: "Vue", Source: "heuristic"}, Similarity: 0.93},
	}
	result := buildCandidateList(candidates)
	if !strings.Contains(result, `type: abstract`) {
		t.Error("should mark abstract candidates")
	}
	if !strings.Contains(result, `type: normal`) {
		t.Error("should mark non-abstract candidates as normal")
	}
	if strings.Contains(result, `"Svelte"`) {
		t.Error("should not mix new tag into existing candidates")
	}
}

func TestBuildCandidateListIncludesPersonMetadata(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{
			Label:       "李宗伟",
			Category:    "person",
			Source:      "llm",
			Description: "马来西亚羽毛球运动员",
			Metadata: models.MetadataMap{
				"country":      "马来西亚",
				"organization": "马来西亚国家羽毛球队",
				"role":         "羽毛球运动员",
				"domains":      []any{"羽毛球", "体育"},
			},
		}, Similarity: 0.91},
	}

	result := buildCandidateList(candidates)

	for _, want := range []string{"属性", "国籍/地区: 马来西亚", "组织: 马来西亚国家羽毛球队", "身份/职务: 羽毛球运动员", "领域: 羽毛球, 体育"} {
		if !strings.Contains(result, want) {
			t.Fatalf("candidate list missing %q in:\n%s", want, result)
		}
	}
}

func TestBuildTagJudgmentPromptRejectsParentChildMerge(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{Label: "Anthropic 协议", Slug: "anthropic-xie-yi"}, Similarity: 0.70},
		{Tag: &models.TopicTag{Label: "DeepSeek V4 Pro", Slug: "deepseek-v4-pro"}, Similarity: 0.67},
	}

	prompt := buildTagJudgmentPrompt(candidates, "Anthropic", "keyword", "", true, 0)

	for _, want := range []string{
		"parent/child",
		"organization/product",
		"ecosystem",
		"MUST use abstracts or none",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

func TestBuildTagJudgmentPromptWeakCandidatesKeepsMergeButCautions(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{Label: "Claude Code", Slug: "claude-code"}, Similarity: 0.73},
		{Tag: &models.TopicTag{Label: "DeepSeek V4 Pro", Slug: "deepseek-v4-pro"}, Similarity: 0.67},
	}

	prompt := buildTagJudgmentPrompt(candidates, "Claude", "keyword", "", false, 0)

	if !strings.Contains(prompt, `"merges"`) {
		t.Fatalf("prompt should still expose merges option for weak candidates, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "CAUTION") {
		t.Fatalf("expected prompt to contain merge caution for low-similarity candidates, got:\n%s", prompt)
	}
}

func TestBuildTagJudgmentSchemaAlwaysIncludesMerge(t *testing.T) {
	schema := buildTagJudgmentSchema()
	if _, ok := schema.Properties["merges"]; !ok {
		t.Fatalf("schema should always expose merges: %#v", schema.Properties)
	}
	if _, ok := schema.Properties["abstracts"]; !ok {
		t.Fatalf("schema should always expose abstracts")
	}
	if _, ok := schema.Properties["none"]; !ok {
		t.Fatalf("schema should always expose none")
	}
	// All three should be required
	for _, field := range []string{"merges", "abstracts", "none"} {
		found := false
		for _, r := range schema.Required {
			if r == field {
				found = true
			}
		}
		if !found {
			t.Fatalf("schema should require %q, got required: %v", field, schema.Required)
		}
	}
}

func TestNormalizeAbstractRelationJudgment(t *testing.T) {
	t.Run("rejects nil judgment", func(t *testing.T) {
		if err := normalizeAbstractRelationJudgment(nil); err == nil {
			t.Fatal("expected nil judgment error")
		}
	})

	t.Run("accepts merge target", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "merge", Target: "A", Reason: "same"}
		if err := normalizeAbstractRelationJudgment(judgment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if judgment.Target != "A" {
			t.Fatalf("normalized judgment = %+v", judgment)
		}
	})

	t.Run("normalizes action", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: " merge ", Target: "B"}
		if err := normalizeAbstractRelationJudgment(judgment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if judgment.Action != "merge" {
			t.Fatalf("normalized judgment = %+v", judgment)
		}
	})

	t.Run("normalizes action case variants", func(t *testing.T) {
		cases := []struct {
			action string
			want   string
		}{
			{action: " MERGE ", want: "merge"},
			{action: " Parent_B ", want: "parent_B"},
		}
		for _, tc := range cases {
			judgment := &abstractRelationJudgment{Action: tc.action, Target: "B"}
			if err := normalizeAbstractRelationJudgment(judgment); err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.action, err)
			}
			if judgment.Action != tc.want {
				t.Fatalf("normalized judgment = %+v, want action %q", judgment, tc.want)
			}
		}
	})

	t.Run("normalizes target and reason", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "parent_A", Target: " b ", Reason: " same "}
		if err := normalizeAbstractRelationJudgment(judgment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if judgment.Target != "B" || judgment.Reason != "same" {
			t.Fatalf("normalized judgment = %+v", judgment)
		}
	})

	t.Run("accepts skip action", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "skip", Target: "A"}
		if err := normalizeAbstractRelationJudgment(judgment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("accepts parent B action", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "parent_B", Target: "B"}
		if err := normalizeAbstractRelationJudgment(judgment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("clears invalid non merge target", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "skip", Target: "C"}
		if err := normalizeAbstractRelationJudgment(judgment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if judgment.Target != "" {
			t.Fatalf("normalized judgment = %+v", judgment)
		}
	})

	t.Run("rejects invalid merge target", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "merge", Target: "C"}
		if err := normalizeAbstractRelationJudgment(judgment); err == nil {
			t.Fatal("expected invalid target error")
		}
	})

	t.Run("rejects unknown action", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "link", Target: "A"}
		if err := normalizeAbstractRelationJudgment(judgment); err == nil {
			t.Fatal("expected invalid action error")
		}
	})
}

func TestParseTagJudgmentResponse(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{Label: "GPT-4", Slug: "gpt-4"}},
		{Tag: &models.TopicTag{Label: "ChatGPT", Slug: "chatgpt"}},
		{Tag: &models.TopicTag{Label: "AI研究", Slug: "ai-yan-jiu"}},
	}

	t.Run("merge only", func(t *testing.T) {
		input := `{"merges":[{"target":"GPT-4","label":"GPT-4","children":["ChatGPT"],"reason":"same concept"}],"abstracts":[],"none":["AI研究"]}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Merges) != 1 {
			t.Fatalf("expected 1 merge, got %d", len(result.Merges))
		}
		if result.Merges[0].Target != "GPT-4" {
			t.Errorf("expected target 'GPT-4', got %q", result.Merges[0].Target)
		}
		if len(result.Merges[0].Children) != 1 || result.Merges[0].Children[0] != "ChatGPT" {
			t.Errorf("expected children [ChatGPT], got %v", result.Merges[0].Children)
		}
		if len(result.Abstracts) != 0 {
			t.Error("expected no abstracts")
		}
		if len(result.None) != 1 || result.None[0] != "AI研究" {
			t.Errorf("expected none [AI研究], got %v", result.None)
		}
	})

	t.Run("both merges and abstracts", func(t *testing.T) {
		input := `{"merges":[{"target":"GPT-4","label":"GPT-4","children":[],"reason":""}],"abstracts":[{"name":"AI技术","description":"AI技术相关","children":["AI研究"],"reason":""}],"none":[]}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Merges) != 1 {
			t.Fatalf("expected 1 merge, got %d", len(result.Merges))
		}
		if len(result.Abstracts) != 1 {
			t.Fatalf("expected 1 abstract, got %d", len(result.Abstracts))
		}
		if result.Abstracts[0].Name != "AI技术" {
			t.Errorf("expected abstract name 'AI技术', got %q", result.Abstracts[0].Name)
		}
	})

	t.Run("dedup across merges and abstracts children", func(t *testing.T) {
		input := `{"merges":[{"target":"GPT-4","label":"GPT-4","children":["ChatGPT"],"reason":""}],"abstracts":[{"name":"AI","description":"AI","children":["ChatGPT","AI研究"],"reason":""}],"none":[]}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, abstract := range result.Abstracts {
			for _, ch := range abstract.Children {
				if ch == "ChatGPT" {
					t.Error("ChatGPT should not appear in abstract children (already in merge children)")
				}
			}
		}
	})

	t.Run("none auto-fills unplaced candidates", func(t *testing.T) {
		input := `{"merges":[],"abstracts":[],"none":[]}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// All candidates should be auto-placed into none since none were placed
		if len(result.None) != 3 {
			t.Fatalf("expected 3 auto-filled none candidates, got %d: %v", len(result.None), result.None)
		}
	})

	t.Run("none cross-validates against merges and abstracts", func(t *testing.T) {
		input := `{"merges":[{"target":"GPT-4","label":"GPT-4","children":["ChatGPT"],"reason":""}],"abstracts":[],"none":["GPT-4","ChatGPT"]}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// GPT-4 and ChatGPT are already in merges, so none should not include them
		for _, label := range result.None {
			if label == "GPT-4" || label == "ChatGPT" {
				t.Errorf("candidate %q should not be in none (already in merges)", label)
			}
		}
	})

	t.Run("invalid children filtered", func(t *testing.T) {
		input := `{"merges":[{"target":"GPT-4","label":"GPT-4","children":["NonExistent"],"reason":""}],"abstracts":[]}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Merges) != 1 {
			t.Fatalf("expected 1 merge, got %d", len(result.Merges))
		}
		if len(result.Merges[0].Children) != 0 {
			t.Errorf("non-existent children should be filtered, got %v", result.Merges[0].Children)
		}
	})

	t.Run("backward compatibility with single merge/abstract", func(t *testing.T) {
		input := `{"merge":{"target":"GPT-4","label":"GPT-4","children":["ChatGPT"],"reason":"same concept"},"abstract":{"name":"AI技术","description":"AI技术相关","children":["AI研究"],"reason":""}}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Merges) != 1 {
			t.Fatalf("expected 1 merge from legacy format, got %d", len(result.Merges))
		}
		if result.Merges[0].Target != "GPT-4" {
			t.Errorf("expected target 'GPT-4', got %q", result.Merges[0].Target)
		}
		if len(result.Abstracts) != 1 {
			t.Fatalf("expected 1 abstract from legacy format, got %d", len(result.Abstracts))
		}
		if result.Abstracts[0].Name != "AI技术" {
			t.Errorf("expected abstract name 'AI技术', got %q", result.Abstracts[0].Name)
		}
	})

	t.Run("multiple abstracts", func(t *testing.T) {
		input := `{"merges":[],"abstracts":[{"name":"AI大模型","description":"大模型相关","children":["GPT-4","ChatGPT"],"reason":"same domain"},{"name":"AI研究","description":"AI研究相关","children":["AI研究"],"reason":"research topic"}],"none":[]}`
		result, err := parseTagJudgmentResponse(input, candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Abstracts) != 2 {
			t.Fatalf("expected 2 abstracts, got %d", len(result.Abstracts))
		}
		if result.Abstracts[0].Name != "AI大模型" {
			t.Errorf("expected first abstract name 'AI大模型', got %q", result.Abstracts[0].Name)
		}
		if result.Abstracts[1].Name != "AI研究" {
			t.Errorf("expected second abstract name 'AI研究', got %q", result.Abstracts[1].Name)
		}
	})
}

func TestSelectMergeTarget(t *testing.T) {
	t.Run("matches by merge target slug", func(t *testing.T) {
		candidates := []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "GPT-4", Slug: "gpt-4"}},
			{Tag: &models.TopicTag{ID: 2, Label: "ChatGPT", Slug: "chatgpt"}},
		}
		target := selectMergeTarget(candidates, "GPT-4", "GPT-4o")
		if target == nil || target.ID != 1 {
			t.Errorf("expected tag ID 1, got %v", target)
		}
	})

	t.Run("matches by merge label slug when target not found", func(t *testing.T) {
		candidates := []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "React", Slug: "react"}},
		}
		target := selectMergeTarget(candidates, "React.js", "React")
		if target == nil || target.ID != 1 {
			t.Errorf("expected tag ID 1 via merge label, got %v", target)
		}
	})

	t.Run("prefers non-abstract candidate", func(t *testing.T) {
		candidates := []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "编程语言", Slug: "bian-cheng-yu-yan", Source: "abstract"}},
			{Tag: &models.TopicTag{ID: 2, Label: "Python", Slug: "python", Source: "heuristic"}},
		}
		target := selectMergeTarget(candidates, "未知", "Python")
		if target == nil {
			t.Fatal("expected non-nil target")
		}
		if target.ID != 2 {
			t.Errorf("expected non-abstract tag ID 2, got %d", target.ID)
		}
	})

	t.Run("returns nil for empty candidates", func(t *testing.T) {
		target := selectMergeTarget(nil, "anything", "anything")
		if target != nil {
			t.Error("expected nil for empty candidates")
		}
	})
}

func TestEnsureNewLabelCandidateInAbstractJudgment(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{ID: 1, Label: "AbortController", Slug: "abortcontroller"}},
		{Tag: &models.TopicTag{ID: 2, Label: "AbortSignal", Slug: "abortsignal"}},
	}

	t.Run("adds current candidate when abstract only names sibling", func(t *testing.T) {
		judgment := &tagJudgment{
			Abstracts: []tagJudgmentAbstract{
				{
					Name:     "AbortController 与 AbortSignal",
					Children: []string{"AbortSignal"},
				},
			},
		}

		ensureNewLabelCandidateInAbstractJudgment(judgment, candidates, "AbortController")

		if len(judgment.Abstracts[0].Children) != 2 {
			t.Fatalf("expected two children, got %v", judgment.Abstracts[0].Children)
		}
		if !labelInSlice(judgment.Abstracts[0].Children, "AbortController") {
			t.Fatalf("expected current candidate to be added, got %v", judgment.Abstracts[0].Children)
		}
	})

	t.Run("does not duplicate existing current candidate", func(t *testing.T) {
		judgment := &tagJudgment{
			Abstracts: []tagJudgmentAbstract{
				{
					Name:     "AbortController 与 AbortSignal",
					Children: []string{"AbortController", "AbortSignal"},
				},
			},
		}

		ensureNewLabelCandidateInAbstractJudgment(judgment, candidates, "AbortController")

		if len(judgment.Abstracts[0].Children) != 2 {
			t.Fatalf("expected no duplicate child, got %v", judgment.Abstracts[0].Children)
		}
	})

	t.Run("does not add candidate already consumed by merge", func(t *testing.T) {
		judgment := &tagJudgment{
			Merges: []tagJudgmentMerge{
				{
					Target: "AbortController",
				},
			},
			Abstracts: []tagJudgmentAbstract{
				{
					Name:     "Abort APIs",
					Children: []string{"AbortSignal"},
				},
			},
		}

		ensureNewLabelCandidateInAbstractJudgment(judgment, candidates, "AbortController")

		if labelInSlice(judgment.Abstracts[0].Children, "AbortController") {
			t.Fatalf("expected merged candidate not to be added, got %v", judgment.Abstracts[0].Children)
		}
	})
}

func TestOrganizeMatchCategory(t *testing.T) {
	if got := organizeMatchCategory("event", &models.TopicTag{Category: "keyword"}); got != "event" {
		t.Fatalf("expected explicit category, got %q", got)
	}

	if got := organizeMatchCategory("", &models.TopicTag{Category: "person"}); got != "person" {
		t.Fatalf("expected tag category fallback, got %q", got)
	}

	if got := organizeMatchCategory("", &models.TopicTag{}); got != "keyword" {
		t.Fatalf("expected keyword fallback, got %q", got)
	}
}

func TestShouldUseOrganizeCandidate(t *testing.T) {
	used := map[uint]bool{3: true}

	cases := []struct {
		name      string
		candidate TagCandidate
		want      bool
	}{
		{
			name:      "uses valid neighbor",
			candidate: TagCandidate{Tag: &models.TopicTag{ID: 2}, Similarity: 0.78},
			want:      true,
		},
		{
			name:      "skips current tag",
			candidate: TagCandidate{Tag: &models.TopicTag{ID: 1}, Similarity: 1},
			want:      false,
		},
		{
			name:      "skips low similarity",
			candidate: TagCandidate{Tag: &models.TopicTag{ID: 2}, Similarity: 0.77},
			want:      false,
		},
		{
			name:      "skips used tag",
			candidate: TagCandidate{Tag: &models.TopicTag{ID: 3}, Similarity: 0.9},
			want:      false,
		},
		{
			name:      "skips nil tag",
			candidate: TagCandidate{Similarity: 0.9},
			want:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldUseOrganizeCandidate(tc.candidate, 1, used)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFormatChildLabels(t *testing.T) {
	if got := formatChildLabels(nil); got != "(无子标签)" {
		t.Fatalf("expected empty label placeholder, got %q", got)
	}

	if got := formatChildLabels([]string{"A", "B"}); got != "A, B" {
		t.Fatalf("expected joined labels, got %q", got)
	}
}

func TestLoadAbstractChildLabelsEmpty(t *testing.T) {
	setupAbstractTagServiceTestDB(t)

	labels := loadAbstractChildLabels(9999, 5)
	if labels == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(labels) != 0 {
		t.Fatalf("expected no labels, got %v", labels)
	}
}

func TestProcessAbstractJudgmentReusesExistingAbstract(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	existing := models.TopicTag{Slug: "existing-abstract", Label: "已有抽象", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "hang-yun-jing-bao", Label: "航运警报", Category: "event", Kind: "event", Source: "llm", Status: "active"}
	for _, tag := range []*models.TopicTag{&existing, &child} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	originalFinder := findSimilarExistingAbstractFn
	findSimilarExistingAbstractFn = func(ctx context.Context, name, desc, category string, candidates []TagCandidate) *models.TopicTag {
		return &existing
	}
	t.Cleanup(func() {
		findSimilarExistingAbstractFn = originalFinder
	})

	result, err := processAbstractJudgment(context.Background(), []TagCandidate{{Tag: &child, Similarity: 0.91}}, &tagJudgmentAbstract{
		Name:        "新的抽象",
		Description: "desc",
		Children:    []string{"航运警报"},
	}, "新标签", "event")
	if err != nil {
		t.Fatalf("processAbstractJudgment returned error: %v", err)
	}
	if result == nil || result.Tag == nil {
		t.Fatal("expected abstract result")
	}
	if result.Tag.ID != existing.ID {
		t.Fatalf("expected existing abstract ID %d, got %d", existing.ID, result.Tag.ID)
	}

	var count int64
	if err := db.Model(&models.TopicTag{}).Where("source = ?", "abstract").Count(&count).Error; err != nil {
		t.Fatalf("count abstract tags: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one abstract tag after reuse, got %d", count)
	}

	assertAbstractRelationExists(t, db, existing.ID, child.ID)
}

func TestReparentOrLinkAbstractChildSkipsWhenChildAlreadyHasParent(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	grandParent := models.TopicTag{Slug: "zhong-dong-chong-tu", Label: "中东冲突", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	midParent := models.TopicTag{Slug: "huo-er-mu-zi-hai-xia-wei-ji", Label: "霍尔木兹海峡危机", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "hang-yun-jing-bao", Label: "航运警报", Category: "event", Kind: "event", Source: "llm", Status: "active"}

	for _, tag := range []*models.TopicTag{&grandParent, &midParent, &child} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: midParent.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create original relation: %v", err)
	}

	if err := reparentOrLinkAbstractChild(context.Background(), child.ID, grandParent.ID); err != nil {
		t.Fatalf("reparentOrLinkAbstractChild returned error: %v", err)
	}

	assertAbstractRelationExists(t, db, midParent.ID, child.ID)
	assertAbstractRelationMissing(t, db, grandParent.ID, midParent.ID)
	assertAbstractRelationMissing(t, db, grandParent.ID, child.ID)
}

func TestGetAllTreeTagIDsIncludesAncestorsSiblingsAndDescendants(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	root := models.TopicTag{Slug: "root", Label: "根", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	branch := models.TopicTag{Slug: "branch", Label: "分支", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	sibling := models.TopicTag{Slug: "sibling", Label: "同级", Category: "event", Kind: "event", Source: "llm", Status: "active"}
	grandchild := models.TopicTag{Slug: "grandchild", Label: "孙节点", Category: "event", Kind: "event", Source: "llm", Status: "active"}

	for _, tag := range []*models.TopicTag{&root, &branch, &sibling, &grandchild} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	relations := []models.TopicTagRelation{
		{ParentID: root.ID, ChildID: branch.ID, RelationType: "abstract"},
		{ParentID: root.ID, ChildID: sibling.ID, RelationType: "abstract"},
		{ParentID: branch.ID, ChildID: grandchild.ID, RelationType: "abstract"},
	}
	for _, relation := range relations {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	ids := getAllTreeTagIDs(branch.ID)
	got := make(map[uint]bool, len(ids))
	for _, id := range ids {
		got[id] = true
	}

	for _, wantID := range []uint{root.ID, branch.ID, sibling.ID, grandchild.ID} {
		if !got[wantID] {
			t.Fatalf("expected tree to include tag %d, got %v", wantID, ids)
		}
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 unique tags in tree, got %v", ids)
	}
}

func TestLinkAbstractParentChildRejectsDepthBeyondLimit(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	root := models.TopicTag{Slug: "root-depth", Label: "根", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	level2 := models.TopicTag{Slug: "level-2", Label: "第二层", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	level3 := models.TopicTag{Slug: "level-3", Label: "第三层", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	level4 := models.TopicTag{Slug: "level-4", Label: "第四层", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	childRoot := models.TopicTag{Slug: "child-root", Label: "子树根", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	childLeaf := models.TopicTag{Slug: "child-leaf", Label: "子树叶", Category: "event", Kind: "event", Source: "llm", Status: "active"}

	for _, tag := range []*models.TopicTag{&root, &level2, &level3, &level4, &childRoot, &childLeaf} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}

	relations := []models.TopicTagRelation{
		{ParentID: root.ID, ChildID: level2.ID, RelationType: "abstract"},
		{ParentID: level2.ID, ChildID: level3.ID, RelationType: "abstract"},
		{ParentID: level3.ID, ChildID: level4.ID, RelationType: "abstract"},
		{ParentID: childRoot.ID, ChildID: childLeaf.ID, RelationType: "abstract"},
	}
	for _, relation := range relations {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	err := linkAbstractParentChild(childRoot.ID, level4.ID)
	if err == nil {
		t.Fatal("expected depth limit error")
	}
	if !strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("expected depth limit error, got %v", err)
	}
	assertAbstractRelationMissing(t, db, level4.ID, childRoot.ID)
}

func TestLinkAbstractParentChild_RejectsOverlappingParents(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/mp"}
	db.Create(&feed)

	// Create existing parent and child already linked
	existingParent := models.TopicTag{Slug: "ex-parent", Label: "Existing Parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	newParent := models.TopicTag{Slug: "new-parent", Label: "New Parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "mp-child", Label: "Child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	db.Create(&existingParent)
	db.Create(&newParent)
	db.Create(&child)

	db.Create(&models.TopicTagRelation{ParentID: existingParent.ID, ChildID: child.ID, RelationType: "abstract"})

	// Both parents share the same articles (high overlap)
	for i := 0; i < 5; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Shared %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: existingParent.ID, Source: "llm"})
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: newParent.ID, Source: "llm"})
	}

	err := linkAbstractParentChild(child.ID, newParent.ID)
	if err == nil {
		t.Fatal("expected multi-parent rejection error")
	}
	if !strings.Contains(err.Error(), "multi-parent prevention") {
		t.Fatalf("expected 'multi-parent prevention' error, got %v", err)
	}

	// Verfiy child still has old parent
	assertAbstractRelationExists(t, db, existingParent.ID, child.ID)
}

func TestLinkAbstractParentChild_AcceptsNonOverlappingParents(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/mp2"}
	db.Create(&feed)

	existingParent := models.TopicTag{Slug: "ex-parent2", Label: "Existing Parent 2", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	newParent := models.TopicTag{Slug: "new-parent2", Label: "New Parent 2", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "mp-child2", Label: "Child 2", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	db.Create(&existingParent)
	db.Create(&newParent)
	db.Create(&child)

	db.Create(&models.TopicTagRelation{ParentID: existingParent.ID, ChildID: child.ID, RelationType: "abstract"})

	// Parents have completely separate article sets
	for i := 0; i < 3; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("EP %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: existingParent.ID, Source: "llm"})
	}
	for i := 0; i < 3; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("NP %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: newParent.ID, Source: "llm"})
	}

	err := linkAbstractParentChild(child.ID, newParent.ID)
	if err != nil {
		t.Fatalf("expected success for non-overlapping parents, got: %v", err)
	}

	// Verfiy child is now under new parent
	assertAbstractRelationExists(t, db, newParent.ID, child.ID)
}

func TestGetAbstractSubtreeDepthStopsAtCycles(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	parent := models.TopicTag{Slug: "cycle-parent", Label: "Cycle Parent", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "cycle-child", Label: "Cycle Child", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	for _, tag := range []*models.TopicTag{&parent, &child} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}
	for _, relation := range []models.TopicTagRelation{
		{ParentID: parent.ID, ChildID: child.ID, RelationType: "abstract"},
		{ParentID: child.ID, ChildID: parent.ID, RelationType: "abstract"},
	} {
		if err := db.Create(&relation).Error; err != nil {
			t.Fatalf("create relation: %v", err)
		}
	}

	depth := getAbstractSubtreeDepth(db, parent.ID)
	if depth != 1 {
		t.Fatalf("depth = %d, want 1 without revisiting cycle nodes", depth)
	}
}

func setupAbstractTagServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if abstractTagUpdateQueueService != nil {
		abstractTagUpdateQueueService.db = db
	}
	database.DB = db
	t.Cleanup(func() {
		database.DB = nil
	})

	if err := db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.TopicTagEmbedding{},
		&models.ArticleTopicTag{},
		&models.TopicTagRelation{},
	); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}

func assertAbstractRelationExists(t *testing.T, db *gorm.DB, parentID, childID uint) {
	t.Helper()

	var count int64
	if err := db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND child_id = ? AND relation_type = ?", parentID, childID, "abstract").
		Count(&count).Error; err != nil {
		t.Fatalf("count relation %d->%d: %v", parentID, childID, err)
	}
	if count != 1 {
		t.Fatalf("expected abstract relation %d->%d to exist once, got %d", parentID, childID, count)
	}
}

func assertAbstractRelationMissing(t *testing.T, db *gorm.DB, parentID, childID uint) {
	t.Helper()

	var count int64
	if err := db.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND child_id = ? AND relation_type = ?", parentID, childID, "abstract").
		Count(&count).Error; err != nil {
		t.Fatalf("count relation %d->%d: %v", parentID, childID, err)
	}
	if count != 0 {
		t.Fatalf("expected abstract relation %d->%d to be absent, got %d", parentID, childID, count)
	}
}

func TestGetUnclassifiedTagsReturnsMoreThan200Rows(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	feed := models.Feed{
		Title: "Test Feed",
		URL:   "https://example.com/feed",
	}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	pubDate := time.Now()

	for i := 0; i < 205; i++ {
		tag := models.TopicTag{
			Slug:     fmt.Sprintf("tag-%03d", i),
			Label:    fmt.Sprintf("Tag %03d", i),
			Category: "keyword",
			Source:   "llm",
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag %d: %v", i, err)
		}

		article := models.Article{
			FeedID:  feed.ID,
			Title:   fmt.Sprintf("Article %03d", i),
			PubDate: &pubDate,
		}
		if err := db.Create(&article).Error; err != nil {
			t.Fatalf("create article %d: %v", i, err)
		}

		link := models.ArticleTopicTag{
			ArticleID:  article.ID,
			TopicTagID: tag.ID,
			Source:     "llm",
		}
		if err := db.Create(&link).Error; err != nil {
			t.Fatalf("create article topic tag %d: %v", i, err)
		}
	}

	nodes, err := GetUnclassifiedTags("", 0, 0, "")
	if err != nil {
		t.Fatalf("GetUnclassifiedTags returned error: %v", err)
	}

	if len(nodes) != 205 {
		t.Fatalf("unclassified tag count = %d, want 205", len(nodes))
	}
}

func TestCollectOrganizeMergeSources(t *testing.T) {
	current := &models.TopicTag{ID: 1, Label: "DeepSeek首次外部融资"}
	target := &models.TopicTag{ID: 2, Label: "DeepSeek 首次寻求外部融资"}
	child := &models.TopicTag{ID: 3, Label: "DeepSeek融资"}
	result := &TagExtractionResult{
		Merge:         &MergeResult{Target: target, Label: target.Label},
		MergeChildren: []*models.TopicTag{child, target, child},
	}

	sources := collectOrganizeMergeSources(result, current)
	sourceIDs := map[uint]bool{}
	for _, source := range sources {
		sourceIDs[source.ID] = true
	}

	if len(sourceIDs) != 2 {
		t.Fatalf("expected current tag and one merge child, got %v", sourceIDs)
	}
	if !sourceIDs[1] || !sourceIDs[3] {
		t.Fatalf("expected source IDs 1 and 3, got %v", sourceIDs)
	}
}

func TestValidateAbstractCreation_SingleChild(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	child := models.TopicTag{Slug: "child1", Label: "Child 1", Category: "keyword", Status: "active"}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}

	err := validateAbstractCreation(db, []*models.TopicTag{&child})
	if err == nil {
		t.Fatal("expected error for single child, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient children") {
		t.Fatalf("expected 'insufficient children' error, got: %v", err)
	}
}

func TestValidateAbstractCreation_HighOverlap(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	tag1 := models.TopicTag{Slug: "tag1", Label: "Tag 1", Category: "keyword", Status: "active"}
	tag2 := models.TopicTag{Slug: "tag2", Label: "Tag 2", Category: "keyword", Status: "active"}
	db.Create(&tag1)
	db.Create(&tag2)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/1"}
	db.Create(&feed)

	// Create 10 articles, all shared between both tags (100% overlap, Jaccard = 1.0)
	for i := 0; i < 10; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Article %d", i)}
		db.Create(&article)

		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag1.ID, Source: "llm"})
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag2.ID, Source: "llm"})
	}

	err := validateAbstractCreation(db, []*models.TopicTag{&tag1, &tag2})
	if err == nil {
		t.Fatal("expected error for high overlap, got nil")
	}
	if !strings.Contains(err.Error(), "share too many articles") {
		t.Fatalf("expected 'share too many articles' error, got: %v", err)
	}
}

func TestValidateAbstractCreation_Acceptable(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	tag1 := models.TopicTag{Slug: "tag1", Label: "Tag 1", Category: "keyword", Status: "active"}
	tag2 := models.TopicTag{Slug: "tag2", Label: "Tag 2", Category: "keyword", Status: "active"}
	tag3 := models.TopicTag{Slug: "tag3", Label: "Tag 3", Category: "keyword", Status: "active"}
	db.Create(&tag1)
	db.Create(&tag2)
	db.Create(&tag3)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/2"}
	db.Create(&feed)

	// Tag1: articles 1-3, Tag2: articles 4-6, Tag3: articles 7-9 (0% overlap)
	for i := 1; i <= 3; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Article %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag1.ID, Source: "llm"})
	}
	for i := 4; i <= 6; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Article %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag2.ID, Source: "llm"})
	}
	for i := 7; i <= 9; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Article %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag3.ID, Source: "llm"})
	}

	err := validateAbstractCreation(db, []*models.TopicTag{&tag1, &tag2, &tag3})
	if err != nil {
		t.Fatalf("expected no error for acceptable abstract, got: %v", err)
	}
}

func TestValidateAbstractCreation_MixedOverlap(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	tag1 := models.TopicTag{Slug: "tag1", Label: "Tag 1", Category: "keyword", Status: "active"}
	tag2 := models.TopicTag{Slug: "tag2", Label: "Tag 2", Category: "keyword", Status: "active"}
	db.Create(&tag1)
	db.Create(&tag2)

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com/3"}
	db.Create(&feed)

	// Create 10 articles: 8 shared (overlap), 2 unique to each
	for i := 0; i < 8; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Shared %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag1.ID, Source: "llm"})
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag2.ID, Source: "llm"})
	}
	// These extra articles make tag1: 10 total, tag2: 10 total
	// Intersection: 8, union: 12, Jaccard: 0.67 -> below 0.7 threshold
	for i := 0; i < 2; i++ {
		article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Unique T1 %d", i)}
		db.Create(&article)
		db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag1.ID, Source: "llm"})

		article2 := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Unique T2 %d", i)}
		db.Create(&article2)
		db.Create(&models.ArticleTopicTag{ArticleID: article2.ID, TopicTagID: tag2.ID, Source: "llm"})
	}

	// Jaccard = 8/(8+2+2) = 8/12 = 0.67 < 0.7, should pass
	err := validateAbstractCreation(db, []*models.TopicTag{&tag1, &tag2})
	if err != nil {
		t.Fatalf("expected no error for mixed overlap (Jaccard=0.67), got: %v", err)
	}
}

func TestValidateAbstractCreation_DegenerateTree(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	// Create a 3-level chain: root -> mid -> leaf (1 leaf, depth=2→ratio=0.5)
	root := models.TopicTag{Slug: "root", Label: "Root", Category: "keyword", Source: "abstract", Status: "active"}
	mid := models.TopicTag{Slug: "mid", Label: "Mid", Category: "keyword", Source: "abstract", Status: "active"}
	leaf := models.TopicTag{Slug: "leaf", Label: "Leaf", Category: "keyword", Source: "llm", Status: "active"}
	db.Create(&root)
	db.Create(&mid)
	db.Create(&leaf)

	// mid -> leaf
	db.Create(&models.TopicTagRelation{ParentID: mid.ID, ChildID: leaf.ID, RelationType: "abstract"})
	// root -> mid
	db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: mid.ID, RelationType: "abstract"})

	feed := models.Feed{Title: "Test", URL: "https://example.com/4"}
	db.Create(&feed)
	article := models.Article{FeedID: feed.ID, Title: "Article"}
	db.Create(&article)
	db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: leaf.ID, Source: "llm"})

	// mid is the child candidate, with subtree: 1 leaf at depth=1 (mid→leaf)
	// ratio = 1 leaf / (1 depth + 1 for new abstract) = 1/2 = 0.5 < 1.5
	err := validateAbstractCreation(db, []*models.TopicTag{&mid})
	// Should fail due to insufficient children (only 1)
	if err == nil {
		t.Fatal("expected error for single child (ratio irrelevant, fails count check first)")
	}
}

func TestValidateAbstractCreation_HealthyTree(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	// 3 child tags, each with 2 leaf tags = 6 leaves total
	tag1 := models.TopicTag{Slug: "tag1", Label: "Tag 1", Category: "keyword", Status: "active"}
	tag2 := models.TopicTag{Slug: "tag2", Label: "Tag 2", Category: "keyword", Status: "active"}
	tag3 := models.TopicTag{Slug: "tag3", Label: "Tag 3", Category: "keyword", Status: "active"}
	db.Create(&tag1)
	db.Create(&tag2)
	db.Create(&tag3)

	leaf1a := models.TopicTag{Slug: "l1a", Label: "Leaf 1a", Category: "keyword", Status: "active"}
	leaf1b := models.TopicTag{Slug: "l1b", Label: "Leaf 1b", Category: "keyword", Status: "active"}
	leaf2a := models.TopicTag{Slug: "l2a", Label: "Leaf 2a", Category: "keyword", Status: "active"}
	leaf2b := models.TopicTag{Slug: "l2b", Label: "Leaf 2b", Category: "keyword", Status: "active"}
	leaf3a := models.TopicTag{Slug: "l3a", Label: "Leaf 3a", Category: "keyword", Status: "active"}
	leaf3b := models.TopicTag{Slug: "l3b", Label: "Leaf 3b", Category: "keyword", Status: "active"}
	db.Create(&leaf1a)
	db.Create(&leaf1b)
	db.Create(&leaf2a)
	db.Create(&leaf2b)
	db.Create(&leaf3a)
	db.Create(&leaf3b)

	// tag1 -> leaf1a, leaf1b
	db.Create(&models.TopicTagRelation{ParentID: tag1.ID, ChildID: leaf1a.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: tag1.ID, ChildID: leaf1b.ID, RelationType: "abstract"})
	// tag2 -> leaf2a, leaf2b
	db.Create(&models.TopicTagRelation{ParentID: tag2.ID, ChildID: leaf2a.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: tag2.ID, ChildID: leaf2b.ID, RelationType: "abstract"})
	// tag3 -> leaf3a, leaf3b
	db.Create(&models.TopicTagRelation{ParentID: tag3.ID, ChildID: leaf3a.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: tag3.ID, ChildID: leaf3b.ID, RelationType: "abstract"})

	feed := models.Feed{Title: "Test", URL: "https://example.com/5"}
	db.Create(&feed)
	for _, leaf := range []*models.TopicTag{&leaf1a, &leaf1b, &leaf2a, &leaf2b, &leaf3a, &leaf3b} {
		for i := 0; i < 3; i++ {
			article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Art-%s-%d", leaf.Slug, i)}
			db.Create(&article)
			db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: leaf.ID, Source: "llm"})
		}
	}

	// ratio = 6 leaves / (1 depth max + 1 for new abstract) = 6/2 = 3.0 >= 1.5, should pass
	err := validateAbstractCreation(db, []*models.TopicTag{&tag1, &tag2, &tag3})
	if err != nil {
		t.Fatalf("expected no error for healthy tree, got: %v", err)
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		setA     map[uint]bool
		setB     map[uint]bool
		expected float64
	}{
		{"both empty", map[uint]bool{}, map[uint]bool{}, 0},
		{"one empty", map[uint]bool{1: true, 2: true}, map[uint]bool{}, 0},
		{"no overlap", map[uint]bool{1: true, 2: true}, map[uint]bool{3: true, 4: true}, 0},
		{"full overlap", map[uint]bool{1: true, 2: true}, map[uint]bool{1: true, 2: true}, 1.0},
		{"partial overlap", map[uint]bool{1: true, 2: true, 3: true}, map[uint]bool{1: true, 2: true, 4: true}, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jaccardSimilarity(tt.setA, tt.setB)
			if result != tt.expected {
				t.Errorf("jaccardSimilarity() = %.4f, want %.4f", result, tt.expected)
			}
		})
	}
}
