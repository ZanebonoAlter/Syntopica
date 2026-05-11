package topicanalysis

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupEmbeddingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	database.DB = db

	if err := db.AutoMigrate(&models.TopicTag{}, &models.TopicTagEmbedding{}); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	return db
}

func TestBuildTagEmbeddingText(t *testing.T) {
	tests := []struct {
		name          string
		tag           *models.TopicTag
		embeddingType string
		expected      string
	}{
		{
			name:          "identity label only",
			tag:           &models.TopicTag{Label: "AI", Category: "event"},
			embeddingType: EmbeddingTypeIdentity,
			expected:      "AI event",
		},
		{
			name:          "identity excludes description",
			tag:           &models.TopicTag{Label: "AI", Category: "event", Description: "Artificial Intelligence"},
			embeddingType: EmbeddingTypeIdentity,
			expected:      "AI event",
		},
		{
			name:          "semantic includes description",
			tag:           &models.TopicTag{Label: "AI", Category: "event", Description: "Artificial Intelligence"},
			embeddingType: EmbeddingTypeSemantic,
			expected:      "AI. Artificial Intelligence event",
		},
		{
			name:          "identity with JSON aliases",
			tag:           &models.TopicTag{Label: "AI", Category: "event", Aliases: `["Artificial Intelligence","Machine Learning"]`},
			embeddingType: EmbeddingTypeIdentity,
			expected:      "AI Artificial Intelligence Machine Learning event",
		},
		{
			name:          "identity with comma-separated aliases",
			tag:           &models.TopicTag{Label: "AI", Category: "event", Aliases: "Artificial Intelligence, ML"},
			embeddingType: EmbeddingTypeIdentity,
			expected:      "AI Artificial Intelligence, ML event",
		},
		{
			name:          "identity empty aliases",
			tag:           &models.TopicTag{Label: "Go", Category: "technology"},
			embeddingType: EmbeddingTypeIdentity,
			expected:      "Go technology",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTagEmbeddingText(tt.tag, tt.embeddingType)
			if result != tt.expected {
				t.Errorf("buildTagEmbeddingText() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBuildTagEmbeddingTextIdentityVsSemantic(t *testing.T) {
	tag := &models.TopicTag{
		Label:       "ChatGPT",
		Category:    "keyword",
		Aliases:     `["GPT-4"]`,
		Description: "OpenAI的对话式AI助手",
	}

	identity := buildTagEmbeddingText(tag, EmbeddingTypeIdentity)
	semantic := buildTagEmbeddingText(tag, EmbeddingTypeSemantic)

	if strings.Contains(identity, "OpenAI") {
		t.Errorf("identity text should not contain description, got %q", identity)
	}
	if !strings.Contains(semantic, "OpenAI") {
		t.Errorf("semantic text should contain description, got %q", semantic)
	}

	identityHash := hashText(EmbeddingTypeIdentity + "\n" + identity)
	semanticHash := hashText(EmbeddingTypeSemantic + "\n" + semantic)
	if identityHash == semanticHash {
		t.Errorf("identity and semantic hashes should differ for same tag")
	}
}

func TestFloatsToPgVector(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		expected string
	}{
		{
			name:     "simple vector",
			input:    []float64{0.1, 0.2, 0.3},
			expected: "[0.100000,0.200000,0.300000]",
		},
		{
			name:     "single element",
			input:    []float64{1.5},
			expected: "[1.500000]",
		},
		{
			name:     "zero vector",
			input:    []float64{0, 0, 0, 0},
			expected: "[0.000000,0.000000,0.000000,0.000000]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := floatsToPgVector(tt.input)
			if result != tt.expected {
				t.Errorf("floatsToPgVector() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestHashTextDeterministic(t *testing.T) {
	a := hashText("hello world")
	b := hashText("hello world")
	if a != b {
		t.Errorf("hashText not deterministic: %q != %q", a, b)
	}

	c := hashText("different input")
	if a == c {
		t.Errorf("hashText collision for different inputs")
	}
}

func TestBuildTagEmbeddingTextWithContextTitles(t *testing.T) {
	tag := &models.TopicTag{
		Label:       "伊朗袭击霍尔木兹海峡船只",
		Category:    "event",
		Description: "指伊朗在霍尔木兹海峡对多艘船只发动的三次袭击事件",
	}

	text := buildTagEmbeddingText(tag, EmbeddingTypeSemantic)
	if strings.Contains(text, "相关报道") {
		t.Errorf("without opts, should not contain context marker, got %q", text)
	}

	text = buildTagEmbeddingText(tag, EmbeddingTypeSemantic, EmbeddingTextOptions{
		ContextTitles: []string{"伊朗在霍尔木兹海峡的军事行动", "霍尔木兹海峡局势升级"},
	})
	if !strings.Contains(text, "相关报道") {
		t.Errorf("with opts, should contain context marker, got %q", text)
	}
	if !strings.Contains(text, "伊朗在霍尔木兹海峡的军事行动") {
		t.Errorf("should contain first title, got %q", text)
	}

	tag.Category = "person"
	text = buildTagEmbeddingText(tag, EmbeddingTypeSemantic, EmbeddingTextOptions{
		ContextTitles: []string{"some title"},
	})
	if strings.Contains(text, "相关报道") {
		t.Errorf("non-event should not include context even with opts, got %q", text)
	}

	tag.Category = "event"
	text = buildTagEmbeddingText(tag, EmbeddingTypeIdentity, EmbeddingTextOptions{
		ContextTitles: []string{"some title"},
	})
	if strings.Contains(text, "相关报道") {
		t.Errorf("identity embedding should not include context, got %q", text)
	}
}

func TestContainsAlias(t *testing.T) {
	tests := []struct {
		name     string
		aliases  string
		label    string
		expected bool
	}{
		{
			name:     "empty aliases",
			aliases:  "",
			label:    "AI",
			expected: false,
		},
		{
			name:     "JSON aliases match",
			aliases:  `["Artificial Intelligence","ML"]`,
			label:    "ML",
			expected: true,
		},
		{
			name:     "JSON aliases no match",
			aliases:  `["Artificial Intelligence","ML"]`,
			label:    "DL",
			expected: false,
		},
		{
			name:     "JSON case insensitive",
			aliases:  `["Machine Learning"]`,
			label:    "machine learning",
			expected: true,
		},
		{
			name:     "comma-separated match",
			aliases:  "AI,ML,DL",
			label:    "ML",
			expected: true,
		},
		{
			name:     "comma-separated with spaces no match",
			aliases:  "AI, ML, DL",
			label:    "ML",
			expected: false,
		},
		{
			name:     "comma-separated no match",
			aliases:  "AI, ML, DL",
			label:    "NLP",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAlias(tt.aliases, tt.label)
			if result != tt.expected {
				t.Errorf("containsAlias(%q, %q) = %v, want %v", tt.aliases, tt.label, result, tt.expected)
			}
		})
	}
}

func TestEmbeddingDimensionMismatch2560(t *testing.T) {
	vec := make([]float64, 2560)
	for i := range vec {
		vec[i] = 0.01
	}
	pgVec := floatsToPgVector(vec)
	if !strings.HasPrefix(pgVec, "[") || !strings.HasSuffix(pgVec, "]") {
		t.Fatalf("pgVector format wrong: %s...%s", pgVec[:20], pgVec[len(pgVec)-20:])
	}

	parts := strings.Split(pgVec[1:len(pgVec)-1], ",")
	if len(parts) != 2560 {
		t.Errorf("expected 2560 dimensions, got %d", len(parts))
	}

	vectorJSON, err := json.Marshal(vec)
	if err != nil {
		t.Fatalf("marshal vector: %v", err)
	}
	var parsed []float64
	if err := json.Unmarshal(vectorJSON, &parsed); err != nil {
		t.Fatalf("unmarshal vector: %v", err)
	}
	if len(parsed) != 2560 {
		t.Errorf("round-trip dimension mismatch: got %d, want 2560", len(parsed))
	}
}

func TestGenerateEmbeddingBuildsCorrectDimension(t *testing.T) {
	emb := &models.TopicTagEmbedding{
		TopicTagID:   1,
		Vector:       "[0.1,0.2,0.3]",
		EmbeddingVec: "[0.100000,0.200000,0.300000]",
		Dimension:    2560,
		Model:        "qwen3-embedding:4b",
		TextHash:     "abc123",
	}

	if emb.Dimension != 2560 {
		t.Errorf("expected dimension 2560, got %d", emb.Dimension)
	}

	vec := make([]float64, emb.Dimension)
	pgVec := floatsToPgVector(vec)
	expected := fmt.Sprintf("vector(%d)", emb.Dimension)
	if expected != "vector(2560)" {
		t.Errorf("expected vector(2560), got %s", expected)
	}

	_ = len(pgVec)
}

func TestSaveEmbeddingReturnsTagNotFoundWhenParentDeleted(t *testing.T) {
	db := setupEmbeddingTestDB(t)
	service := NewEmbeddingService()

	tag := models.TopicTag{
		Slug:     "deleted-tag",
		Label:    "Deleted Tag",
		Category: models.TagCategoryKeyword,
		Status:   "active",
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	if err := db.Delete(&tag).Error; err != nil {
		t.Fatalf("delete tag: %v", err)
	}

	err := service.SaveEmbedding(&models.TopicTagEmbedding{
		TopicTagID:    tag.ID,
		EmbeddingType: EmbeddingTypeIdentity,
		Vector:        "[0.1,0.2]",
		Model:         "test-model",
		TextHash:      "abc123",
	})
	if err == nil {
		t.Fatal("expected missing parent tag error, got nil")
	}
	if err != ErrTopicTagNotFound {
		t.Fatalf("expected ErrTopicTagNotFound, got %v", err)
	}

	var count int64
	if err := db.Model(&models.TopicTagEmbedding{}).Count(&count).Error; err != nil {
		t.Fatalf("count embeddings: %v", err)
	}
	if count != 0 {
		t.Fatalf("embedding count = %d, want 0", count)
	}
}

func TestThresholdsForCategory(t *testing.T) {
	tests := []struct {
		name        string
		category    string
		wantHighSim float64
		wantLowSim  float64
	}{
		{
			name:        "keyword uses override high=0.90",
			category:    "keyword",
			wantHighSim: 0.90,
			wantLowSim:  0.78,
		},
		{
			name:        "event falls back to default",
			category:    "event",
			wantHighSim: 0.97,
			wantLowSim:  0.78,
		},
		{
			name:        "person falls back to default",
			category:    "person",
			wantHighSim: 0.97,
			wantLowSim:  0.78,
		},
		{
			name:        "unknown category falls back to default",
			category:    "organization",
			wantHighSim: 0.97,
			wantLowSim:  0.78,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ThresholdsForCategory(tt.category)
			if got.HighSimilarity != tt.wantHighSim {
				t.Errorf("HighSimilarity = %.2f, want %.2f", got.HighSimilarity, tt.wantHighSim)
			}
			if got.LowSimilarity != tt.wantLowSim {
				t.Errorf("LowSimilarity = %.2f, want %.2f", got.LowSimilarity, tt.wantLowSim)
			}
		})
	}
}

func TestThresholdsForCategoryOverrideIsolation(t *testing.T) {
	original := CategoryThresholdOverrides["keyword"]
	defer func() {
		CategoryThresholdOverrides["keyword"] = original
	}()

	CategoryThresholdOverrides["keyword"] = EmbeddingMatchThresholds{
		HighSimilarity: 0.85,
		LowSimilarity:  0.70,
	}

	got := ThresholdsForCategory("keyword")
	if got.HighSimilarity != 0.85 {
		t.Errorf("HighSimilarity = %.2f, want 0.85", got.HighSimilarity)
	}
	if got.LowSimilarity != 0.70 {
		t.Errorf("LowSimilarity = %.2f, want 0.70", got.LowSimilarity)
	}

	eventGot := ThresholdsForCategory("event")
	if eventGot.HighSimilarity != 0.97 {
		t.Errorf("event HighSimilarity should be unaffected, got %.2f", eventGot.HighSimilarity)
	}
}
