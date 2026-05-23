package datamigrate

import (
	"time"
)

type Mode string

const (
	ModeDryRun     Mode = "dry-run"
	ModeExecute    Mode = "execute"
	ModeVerifyOnly Mode = "verify-only"
)

type TableSpec struct {
	Name                        string
	PrimaryKey                  string
	SampleColumns               []string
	AllowedMissingTargetColumns []string
	Optional                    bool
}

type TableSummary struct {
	Table         string
	SourceCount   int64
	ImportedCount int64
	Skipped       bool
	Reason        string
}

type TableCountCheck struct {
	Table       string
	SourceCount int64
	TargetCount int64
}

type SequenceState struct {
	Table     string
	Sequence  string
	MaxID     int64
	NextValue int64
}

type SampleCheck struct {
	Table       string
	PrimaryKey  any
	Column      string
	SourceValue any
	TargetValue any
}

type EmbeddingVectorCheck struct {
	Table        string
	PrimaryKey   any
	SourceVector string
	LegacyVector string
	TargetVector string
}

type VerificationReport struct {
	CountChecks     []TableCountCheck
	SequenceStates  []SequenceState
	SampleChecks    []SampleCheck
	EmbeddingChecks []EmbeddingVectorCheck
	CheckedTables   []string
	CheckedAt       time.Time
}

const verificationSampleLimit = 10

func DefaultTableSpecs() []TableSpec {
	return []TableSpec{
		{Name: "categories", PrimaryKey: "id", SampleColumns: []string{"name", "slug"}},
		{Name: "feeds", PrimaryKey: "id", SampleColumns: []string{"title", "url", "category_id"}},
		{Name: "articles", PrimaryKey: "id", SampleColumns: []string{"feed_id", "title", "link"}},

		{Name: "scheduler_tasks", PrimaryKey: "id", SampleColumns: []string{"name", "status"}},
		{Name: "ai_settings", PrimaryKey: "id", SampleColumns: []string{"key", "value"}},
		{Name: "ai_providers", PrimaryKey: "id", SampleColumns: []string{"name", "provider_type", "enabled"}},
		{Name: "ai_routes", PrimaryKey: "id", SampleColumns: []string{"name", "capability", "enabled"}},
		{Name: "ai_route_providers", PrimaryKey: "id", SampleColumns: []string{"route_id", "provider_id", "priority"}},
		{Name: "ai_call_logs", PrimaryKey: "id", SampleColumns: []string{"capability", "provider_name", "success"}},
		{Name: "reading_behaviors", PrimaryKey: "id", SampleColumns: []string{"article_id", "feed_id", "event_type"}},
		{Name: "user_preferences", PrimaryKey: "id", SampleColumns: []string{"feed_id", "category_id", "preference_score"}},
		{Name: "topic_tags", PrimaryKey: "id", SampleColumns: []string{"slug", "label", "category"}},
		{Name: "topic_tag_embeddings", PrimaryKey: "id", SampleColumns: []string{"topic_tag_id", "dimension", "model", "text_hash", "vector"}, AllowedMissingTargetColumns: []string{"vector"}},
		{Name: "topic_tag_analyses", PrimaryKey: "id", SampleColumns: []string{"topic_tag_id", "analysis_type", "window_type"}},
		{Name: "topic_analysis_cursors", PrimaryKey: "id", SampleColumns: []string{"topic_tag_id", "analysis_type", "window_type"}},

		{Name: "article_topic_tags", PrimaryKey: "id", SampleColumns: []string{"article_id", "topic_tag_id", "score"}},
		{Name: "firecrawl_jobs", PrimaryKey: "id", SampleColumns: []string{"article_id", "status", "attempt_count"}},
		{Name: "tag_jobs", PrimaryKey: "id", SampleColumns: []string{"article_id", "status", "attempt_count"}},

		{Name: "topic_analysis_jobs", Optional: true},
	}
}
