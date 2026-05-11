package tracing

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type OtelSpan struct {
	ID                 uint   `gorm:"primaryKey;autoIncrement"`
	TraceID            string `gorm:"type:char(32);not null;index:idx_otel_spans_trace_id"`
	SpanID             string `gorm:"type:char(16);not null"`
	ParentSpanID       string `gorm:"type:char(16);default:''"`
	TraceState         string `gorm:"type:text;default:''"`
	Name               string `gorm:"type:varchar(255);not null;index:idx_otel_spans_name"`
	Kind               int    `gorm:"default:1;index:idx_otel_spans_kind"`
	StatusCode         int    `gorm:"default:0;index:idx_otel_spans_status"`
	StatusMessage      string `gorm:"type:text;default:''"`
	StartTimeUnixNano  int64  `gorm:"not null;index:idx_otel_spans_start_time"`
	EndTimeUnixNano    int64  `gorm:"not null"`
	DurationMs         int64  `gorm:"default:0"`
	ServiceName        string `gorm:"type:varchar(100);default:'rss-reader-backend'"`
	ServiceVersion     string `gorm:"type:varchar(50);default:''"`
	ResourceAttributes string `gorm:"type:text;default:'{}'"`
	ScopeName          string `gorm:"type:varchar(100);default:''"`
	ScopeVersion       string `gorm:"type:varchar(50);default:''"`
	Attributes         string `gorm:"type:text;default:'{}'"`
	Events             string `gorm:"type:text;default:'[]'"`
	Links              string `gorm:"type:text;default:'[]'"`
	CreatedAt          time.Time
}

func (OtelSpan) TableName() string {
	return "otel_spans"
}

func EnsureTracingTable(db *gorm.DB) error {
	if err := db.AutoMigrate(&OtelSpan{}); err != nil {
		return fmt.Errorf("failed to migrate otel_spans table: %w", err)
	}

	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_otel_spans_trace_id ON otel_spans(trace_id)",
		"CREATE INDEX IF NOT EXISTS idx_otel_spans_name ON otel_spans(name)",
		"CREATE INDEX IF NOT EXISTS idx_otel_spans_start_time ON otel_spans(start_time_unix_nano)",
		"CREATE INDEX IF NOT EXISTS idx_otel_spans_kind ON otel_spans(kind)",
		"CREATE INDEX IF NOT EXISTS idx_otel_spans_status ON otel_spans(status_code)",
	}
	for _, idx := range indexes {
		if err := db.Exec(idx).Error; err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

type OtelAttribute struct {
	Key   string    `json:"key"`
	Value OtelValue `json:"value"`
}

type OtelValue struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type OtelEvent struct {
	Name         string          `json:"name"`
	TimeUnixNano int64           `json:"time_unix_nano"`
	Attributes   []OtelAttribute `json:"attributes,omitempty"`
}

type OtelLink struct {
	TraceID    string          `json:"trace_id"`
	SpanID     string          `json:"span_id"`
	Attributes []OtelAttribute `json:"attributes,omitempty"`
}

func MarshalAttributes(attrs []OtelAttribute) string {
	if len(attrs) == 0 {
		return "{}"
	}
	data, _ := json.Marshal(attrs)
	return string(data)
}

func MarshalEvents(events []OtelEvent) string {
	if len(events) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(events)
	return string(data)
}

func MarshalLinks(links []OtelLink) string {
	if len(links) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(links)
	return string(data)
}

func UnmarshalAttributes(data string) []OtelAttribute {
	var attrs []OtelAttribute
	_ = json.Unmarshal([]byte(data), &attrs)
	return attrs
}

func UnmarshalEvents(data string) []OtelEvent {
	var events []OtelEvent
	_ = json.Unmarshal([]byte(data), &events)
	return events
}
