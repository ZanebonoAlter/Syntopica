package datasources

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// ProbeFunc runs one real fetch with the source's minimal parameter set
// (spec「probe 端点」). Returns a machine-readable summary and the upstream
// retrieved_at; errors must be *SourceError.
type ProbeFunc func(ctx context.Context) (summary map[string]any, retrievedAt time.Time, err error)

// Handler serves the catalog listing and per-source probe endpoints.
type Handler struct {
	db      *gorm.DB
	probes  map[string]ProbeFunc
	resolve KeyResolver // optional; when set, key-requiring rows get live status
}

// NewHandler builds a handler over db. Probe functions are registered later
// via RegisterProbe (sources live in their own files; wiring is additive so
// this file compiles before all sources exist).
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db, probes: make(map[string]ProbeFunc)}
}

// RegisterProbe wires a probe function for a source code.
func (h *Handler) RegisterProbe(code string, fn ProbeFunc) {
	h.probes[code] = fn
}

// SetKeyResolver enables live status: key-requiring sources re-resolve their
// key on every List call (UI-configured keys take effect without restart).
// Seeded rows keep the boot-time snapshot; the response reflects now.
func (h *Handler) SetKeyResolver(r KeyResolver) { h.resolve = r }

// List handles GET /api/datasources: the full catalog with runtime status.
func (h *Handler) List(c *gin.Context) {
	rows, err := ListCatalog(h.db)
	if err != nil {
		logging.Errorf("datasources list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "目录查询失败"})
		return
	}
	out := make([]CatalogDTO, 0, len(rows))
	for _, r := range rows {
		if h.resolve != nil && r.RequiresKey {
			if strings.TrimSpace(h.resolve(r.ConfigKeyName)) == "" {
				r.Status = StatusDisabled
				r.StatusReason = fmt.Sprintf("未配置 %s：可在设置界面「研究数据源」配置（即时生效），或用环境变量/config.yaml 兜底", r.ConfigKeyName)
			} else {
				r.Status = StatusEnabled
				r.StatusReason = ""
			}
		}
		out = append(out, r.ToDTO())
	}
	c.JSON(http.StatusOK, gin.H{"data_sources": out})
}

// Probe handles POST /api/datasources/{code}/probe: one real fetch per the
// source's minimal parameter set. Results NEVER mutate the catalog row
// (spec: probe 不改源状态). Error mapping (design decision 8):
// INVALID_ARGUMENT→400, SOURCE_UNAVAILABLE/SCHEMA_CHANGED→502 with error_code.
func (h *Handler) Probe(c *gin.Context) {
	code := c.Param("code")
	if _, ok := Definition(code); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error_code": "INVALID_ARGUMENT", "message": "未知数据源: " + code})
		return
	}
	fn, ok := h.probes[code]
	if !ok {
		c.JSON(http.StatusNotImplemented, gin.H{"error_code": "SOURCE_UNAVAILABLE", "message": "该源 probe 未接线"})
		return
	}
	start := time.Now()
	summary, retrievedAt, err := fn(c.Request.Context())
	elapsed := time.Since(start)
	if err != nil {
		se, ok := AsSourceError(err)
		if !ok {
			logging.Errorf("datasources probe %s: %v", code, err)
			c.JSON(http.StatusBadGateway, gin.H{"error_code": "SOURCE_UNAVAILABLE", "message": "取数失败", "detail": err.Error(), "elapsed_ms": elapsed.Milliseconds()})
			return
		}
		status := http.StatusBadGateway
		if se.Kind == ErrInvalidArgument {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error_code": string(se.Kind), "message": se.Message, "detail": se.Detail, "elapsed_ms": elapsed.Milliseconds()})
		return
	}
	resp := gin.H{"code": code, "elapsed_ms": elapsed.Milliseconds(), "summary": summary}
	if !retrievedAt.IsZero() {
		resp["retrieved_at"] = retrievedAt.UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, resp)
}
