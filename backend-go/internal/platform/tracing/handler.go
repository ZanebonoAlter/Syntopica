package tracing

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TraceHandler struct {
	db *gorm.DB
}

func NewTraceHandler(db *gorm.DB) *TraceHandler {
	return &TraceHandler{db: db}
}

func (h *TraceHandler) GetTraceByTraceID(c *gin.Context) {
	traceID := c.Query("trace_id")
	if traceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "trace_id is required"})
		return
	}

	spans, err := QueryByTraceID(h.db, traceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if len(spans) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "trace not found"})
		return
	}

	attributes := make([]map[string]interface{}, 0, len(spans))
	for _, sp := range spans {
		attrs := UnmarshalAttributes(sp.Attributes)
		events := UnmarshalEvents(sp.Events)
		attributes = append(attributes, map[string]interface{}{
			"span_id":              sp.SpanID,
			"parent_span_id":       sp.ParentSpanID,
			"name":                 sp.Name,
			"kind":                 sp.Kind,
			"status_code":          sp.StatusCode,
			"status_message":       sp.StatusMessage,
			"start_time_unix_nano": sp.StartTimeUnixNano,
			"end_time_unix_nano":   sp.EndTimeUnixNano,
			"duration_ms":          sp.DurationMs,
			"service_name":         sp.ServiceName,
			"scope_name":           sp.ScopeName,
			"attributes":           attrs,
			"events":               events,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"trace_id": traceID,
			"spans":    attributes,
		},
	})
}

func (h *TraceHandler) GetRecentTraces(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	traces, err := QueryRecentTraces(h.db, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    traces,
	})
}

func (h *TraceHandler) GetTraceTimeline(c *gin.Context) {
	traceID := c.Param("trace_id")
	if traceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "trace_id is required"})
		return
	}

	spans, err := QueryByTraceID(h.db, traceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if len(spans) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "trace not found"})
		return
	}

	tree := BuildSpanTree(spans)

	var traceStartTime int64
	if len(spans) > 0 {
		traceStartTime = spans[0].StartTimeUnixNano
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"trace_id":         traceID,
			"trace_start_time": traceStartTime,
			"timeline":         tree,
		},
	})
}

func (h *TraceHandler) GetTraceStats(c *gin.Context) {
	stats, err := QueryStats(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

func (h *TraceHandler) SearchTraces(c *gin.Context) {
	operation := c.Query("operation")
	status := c.Query("status")
	minDurationStr := c.Query("min_duration_ms")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	var traces interface{}
	var err error

	switch {
	case status == "error":
		traces, err = QueryErrorTraces(h.db, limit)
	case operation != "":
		traces, err = QueryTracesByOperation(h.db, operation, limit)
	case minDurationStr != "":
		minDuration, parseErr := strconv.ParseInt(minDurationStr, 10, 64)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid min_duration_ms"})
			return
		}
		traces, err = QuerySlowTraces(h.db, minDuration, limit)
	default:
		traces, err = QueryRecentTraces(h.db, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    traces,
	})
}

func (h *TraceHandler) ExportTraceOTLP(c *gin.Context) {
	traceID := c.Param("trace_id")
	if traceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "trace_id is required"})
		return
	}

	spans, err := QueryByTraceID(h.db, traceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if len(spans) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "trace not found"})
		return
	}

	otlpSpans := make([]map[string]interface{}, 0, len(spans))
	for _, sp := range spans {
		attrs := UnmarshalAttributes(sp.Attributes)
		events := UnmarshalEvents(sp.Events)

		otlpAttrs := make([]map[string]interface{}, 0, len(attrs))
		for _, a := range attrs {
			otlpAttrs = append(otlpAttrs, map[string]interface{}{
				"key":   a.Key,
				"value": a.Value,
			})
		}

		otlpEvents := make([]map[string]interface{}, 0, len(events))
		for _, e := range events {
			evtAttrs := make([]map[string]interface{}, 0, len(e.Attributes))
			for _, a := range e.Attributes {
				evtAttrs = append(evtAttrs, map[string]interface{}{
					"key":   a.Key,
					"value": a.Value,
				})
			}
			otlpEvents = append(otlpEvents, map[string]interface{}{
				"name":         e.Name,
				"timeUnixNano": strconv.FormatInt(e.TimeUnixNano, 10),
				"attributes":   evtAttrs,
			})
		}

		statusCode := "STATUS_CODE_UNSET"
		switch sp.StatusCode {
		case 1:
			statusCode = "STATUS_CODE_ERROR"
		case 2:
			statusCode = "STATUS_CODE_OK"
		}

		otlpSpans = append(otlpSpans, map[string]interface{}{
			"traceId":           sp.TraceID,
			"spanId":            sp.SpanID,
			"parentSpanId":      sp.ParentSpanID,
			"name":              sp.Name,
			"kind":              sp.Kind,
			"startTimeUnixNano": strconv.FormatInt(sp.StartTimeUnixNano, 10),
			"endTimeUnixNano":   strconv.FormatInt(sp.EndTimeUnixNano, 10),
			"attributes":        otlpAttrs,
			"events":            otlpEvents,
			"status": map[string]interface{}{
				"code":    statusCode,
				"message": sp.StatusMessage,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"resourceSpans": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{"key": "service.name", "value": map[string]interface{}{"stringValue": "rss-reader-backend"}},
					},
				},
				"scopeSpans": []map[string]interface{}{
					{
						"scope": map[string]interface{}{"name": "rss-reader-backend"},
						"spans": otlpSpans,
					},
				},
			},
		},
	})
}
