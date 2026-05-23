package tracing

import (
	"context"
	_ "encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/logging"
)

type SQLiteSpanExporter struct {
	db     *gorm.DB
	cfg    Config
	mu     sync.Mutex
	stopCh chan struct{}
}

func NewSQLiteSpanExporter(db *gorm.DB, cfg Config) (*SQLiteSpanExporter, error) {
	if err := EnsureTracingTable(db); err != nil {
		return nil, fmt.Errorf("failed to ensure tracing table: %w", err)
	}

	exporter := &SQLiteSpanExporter{
		db:     db,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}

	go exporter.cleanupLoop()

	return exporter, nil
}

func (e *SQLiteSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}

	records := make([]OtelSpan, 0, len(spans))
	for _, sp := range spans {
		record := e.convertSpan(sp)
		records = append(records, record)
	}

	return e.batchInsert(records)
}

func (e *SQLiteSpanExporter) Shutdown(ctx context.Context) error {
	close(e.stopCh)
	return nil
}

func (e *SQLiteSpanExporter) convertSpan(sp sdktrace.ReadOnlySpan) OtelSpan {
	sc := sp.SpanContext()
	psc := sp.Parent()
	res := sp.Resource()
	scope := sp.InstrumentationScope()

	startTime := sp.StartTime()
	endTime := sp.EndTime()
	durationMs := endTime.Sub(startTime).Milliseconds()

	attrs := convertSDKAttributes(sp.Attributes())
	events := convertSDKEvents(sp.Events())
	links := convertSDKLinks(sp.Links())
	resAttrs := convertSDKAttributes(res.Attributes())

	statusCode := 0
	statusMessage := ""
	sdkStatus := sp.Status()
	switch sdkStatus.Code {
	case codes.Error:
		statusCode = 1
		statusMessage = sdkStatus.Description
	case codes.Ok:
		statusCode = 2
	}

	return OtelSpan{
		TraceID:            sc.TraceID().String(),
		SpanID:             sc.SpanID().String(),
		ParentSpanID:       psc.SpanID().String(),
		TraceState:         sc.TraceState().String(),
		Name:               sp.Name(),
		Kind:               int(sp.SpanKind()),
		StatusCode:         statusCode,
		StatusMessage:      statusMessage,
		StartTimeUnixNano:  startTime.UnixNano(),
		EndTimeUnixNano:    endTime.UnixNano(),
		DurationMs:         durationMs,
		ServiceName:        getServiceName(res),
		ServiceVersion:     getServiceVersion(res),
		ResourceAttributes: MarshalAttributes(resAttrs),
		ScopeName:          scope.Name,
		ScopeVersion:       scope.Version,
		Attributes:         MarshalAttributes(attrs),
		Events:             MarshalEvents(events),
		Links:              MarshalLinks(links),
		CreatedAt:          time.Now(),
	}
}

func (e *SQLiteSpanExporter) batchInsert(records []OtelSpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.db.Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			if err := tx.Create(&record).Error; err != nil {
				logging.Infof("[tracing] failed to insert span: %v", err)
			}
		}
		return nil
	})
}

func (e *SQLiteSpanExporter) cleanupLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.cleanExpiredSpans()
		}
	}
}

func (e *SQLiteSpanExporter) cleanExpiredSpans() {
	cutoff := time.Now().AddDate(0, 0, -e.cfg.RetentionDays)
	result := e.db.Where("created_at < ?", cutoff).Delete(&OtelSpan{})
	if result.Error != nil {
		logging.Infof("[tracing] failed to clean expired spans: %v", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		logging.Infof("[tracing] cleaned up %d expired spans (retention: %d days)", result.RowsAffected, e.cfg.RetentionDays)
	}
}

func convertSDKAttributes(attrs []attribute.KeyValue) []OtelAttribute {
	result := make([]OtelAttribute, 0, len(attrs))
	for _, attr := range attrs {
		otelAttr := OtelAttribute{
			Key: string(attr.Key),
		}
		switch attr.Value.Type() {
		case attribute.STRING:
			otelAttr.Value = OtelValue{Type: "STRING", Value: attr.Value.AsString()}
		case attribute.INT64:
			otelAttr.Value = OtelValue{Type: "INT64", Value: attr.Value.AsInt64()}
		case attribute.FLOAT64:
			otelAttr.Value = OtelValue{Type: "FLOAT64", Value: attr.Value.AsFloat64()}
		case attribute.BOOL:
			otelAttr.Value = OtelValue{Type: "BOOL", Value: attr.Value.AsBool()}
		default:
			otelAttr.Value = OtelValue{Type: "STRING", Value: attr.Value.AsString()}
		}
		result = append(result, otelAttr)
	}
	return result
}

func convertSDKEvents(events []sdktrace.Event) []OtelEvent {
	result := make([]OtelEvent, 0, len(events))
	for _, evt := range events {
		result = append(result, OtelEvent{
			Name:         evt.Name,
			TimeUnixNano: evt.Time.UnixNano(),
			Attributes:   convertSDKAttributes(evt.Attributes),
		})
	}
	return result
}

func convertSDKLinks(links []sdktrace.Link) []OtelLink {
	result := make([]OtelLink, 0, len(links))
	for _, link := range links {
		result = append(result, OtelLink{
			TraceID:    link.SpanContext.TraceID().String(),
			SpanID:     link.SpanContext.SpanID().String(),
			Attributes: convertSDKAttributes(link.Attributes),
		})
	}
	return result
}

func getServiceName(res *resource.Resource) string {
	for _, attr := range res.Attributes() {
		if attr.Key == "service.name" {
			return attr.Value.AsString()
		}
	}
	return ServiceName
}

func getServiceVersion(res *resource.Resource) string {
	for _, attr := range res.Attributes() {
		if attr.Key == "service.version" {
			return attr.Value.AsString()
		}
	}
	return ""
}
