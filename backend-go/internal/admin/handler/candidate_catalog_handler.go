package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/admin/service"
	"syntopica-backend/internal/platform/safefetch"
)

// ── 候选源库 handler（improve-discovery-recommendations，design D9）──
//
// POST/GET /api/discovery/candidates、GET/PATCH /api/discovery/candidates/:id。
// 错误响应统一可识别 code（validation/conflict/not_found），conflict 附带
// existing_id 供前端展示「已存在入口」；其余沿用 {"success": bool, "data"|"error"} 惯例。

// newCandidateCatalogService 构造候选库 service。
func newCandidateCatalogService() *service.CandidateCatalogService {
	return service.NewCandidateCatalogService(repository.Repo.DB())
}

// newCandidateCheckService 构造候选可用性检查 service（3.3 / design D7）。
func newCandidateCheckService() *service.CandidateCheckService {
	return service.NewCandidateCheckService(repository.Repo.DB())
}

// writeCandidateError 按类型化错误映射 HTTP 状态与 JSON 形状。
func writeCandidateError(c *gin.Context, err error) {
	var ce *service.CandidateError
	if errors.As(err, &ce) {
		switch ce.Code {
		case service.CandidateErrorCodeValidation:
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": ce.Message, "code": ce.Code})
		case service.CandidateErrorCodeNotFound:
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": ce.Message, "code": ce.Code})
		case service.CandidateErrorCodeForbidden:
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": ce.Message, "code": ce.Code})
		case service.CandidateErrorCodeConflict:
			body := gin.H{"success": false, "error": ce.Message, "code": ce.Code}
			if ce.ExistingID > 0 {
				body["existing_id"] = ce.ExistingID
			}
			c.JSON(http.StatusConflict, body)
		case service.CandidateErrorCodeConfiguration:
			// 配置禁用/非法（如 discovery_v2 关闭时的检查入口）：503 = 服务当前不可用，
			// 不是请求错误也不是服务端 bug（design D9 可识别错误码集合）。
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": ce.Message, "code": ce.Code})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": ce.Message})
		}
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
}

// CreateCandidate POST /api/discovery/candidates — 手动新增原生 RSS 候选（入库 ≠ 订阅）。
func CreateCandidate(c *gin.Context) {
	var req service.CandidateCreateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body", "code": service.CandidateErrorCodeValidation})
		return
	}
	view, err := newCandidateCatalogService().CreateCandidate(c.Request.Context(), req)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view, "message": "candidate created (not subscribed)"})
}

// ListCandidates GET /api/discovery/candidates — 分页列表。
// 查询参数：page（默认 1）、page_size（默认 30、上限 100）、q（关键词 ≤500 runes，命中
// name/description/feed_url）、kind（rsshub|rss）、recommendation_enabled（true/false）。
func ListCandidates(c *gin.Context) {
	q := service.CandidateListQuery{
		Keyword: c.Query("q"),
		Kind:    strings.TrimSpace(c.Query("kind")),
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(service.CandidateListDefaultPageSize)))
	q.Page, q.PageSize = page, pageSize
	if v := strings.TrimSpace(c.Query("recommendation_enabled")); v != "" {
		enabled := v == "true" || v == "1"
		q.RecommendationEnabled = &enabled
	}
	result, err := newCandidateCatalogService().ListCandidates(c.Request.Context(), q)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// GetCandidate GET /api/discovery/candidates/:id — 单条（rsshub 附上游 Route）。
func GetCandidate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id", "code": service.CandidateErrorCodeValidation})
		return
	}
	view, err := newCandidateCatalogService().GetCandidate(c.Request.Context(), uint(id))
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// UpdateCandidate PATCH /api/discovery/candidates/:id — 编辑 manual 元数据 / 推荐启停。
// body 携带 revision（GET 返回的所见版本）做乐观锁；缺失 0 = 不校验版本。
func UpdateCandidate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id", "code": service.CandidateErrorCodeValidation})
		return
	}
	var req service.CandidateUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body", "code": service.CandidateErrorCodeValidation})
		return
	}
	view, err := newCandidateCatalogService().UpdateCandidate(c.Request.Context(), uint(id), req)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// ── 可用性检查 handler（improve-discovery-recommendations 3.3，design D7 / spec C6）──

// CheckCandidate POST /api/discovery/candidates/:id/check — 对候选实际端点执行一次同步检查。
// 返回新可用性状态 + last_checked_at + last_error_code + next_check_at（字段名对齐前端）。
// 错误映射：未授权私有来源 403 forbidden（零请求）、候选不存在 404 not_found、
// 同候选检查进行中 409 conflict；「检查结果为失败」不是接口失败——仍返回 200 + 状态与错误码。
// 请求体为空；抓取走 safefetch 默认 Options（10s 超时 / 2MiB / 3 次重定向 / 私网拒绝）。
func CheckCandidate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id", "code": service.CandidateErrorCodeValidation})
		return
	}
	view, err := newCandidateCheckService().CheckCandidate(c.Request.Context(), uint(id), safefetch.Options{})
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// ── 私网访问授权确认（Medium 7，design D9 / spec C5）──

// accessConfirmInput 确认请求体：confirm=false 显式拒绝转换（省略体 = 确认，即调用即确认）。
type accessConfirmInput struct {
	Confirm *bool `json:"confirm"`
}

// ConfirmCandidateAccess POST /api/discovery/candidates/:id/access-confirm — 用户显式确认
// 私有来源访问授权：private_pending → private_allowed。confirm=false → 403 forbidden（状态不变）；
// 非 private_pending → 409 conflict；不存在 → 404 not_found。确认后 CheckCandidate/accept
// 才会按该候选端点解析 IP 构造 AllowedIPs（不是全局关闭 SSRF）。
func ConfirmCandidateAccess(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id", "code": service.CandidateErrorCodeValidation})
		return
	}
	confirm := true
	if raw, rerr := c.GetRawData(); rerr == nil && len(strings.TrimSpace(string(raw))) > 0 {
		var req accessConfirmInput
		if jerr := json.Unmarshal(raw, &req); jerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body", "code": service.CandidateErrorCodeValidation})
			return
		}
		if req.Confirm != nil {
			confirm = *req.Confirm
		}
	}
	view, err := newCandidateCatalogService().ConfirmAccess(c.Request.Context(), uint(id), confirm)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// ── 目录导入导出（improve-discovery-recommendations 3.2，design D8 / spec C3+C4）──

// catalogImportConfirmInput 确认请求体：预览返回的 fingerprint / local_revision + 导入文件本体。
// import 与预览上传的文件同构（CatalogExport 形状），服务端重新解析校验，不信任客户端分类。
type catalogImportConfirmInput struct {
	Fingerprint   string                `json:"fingerprint"`
	LocalRevision uint64                `json:"local_revision"`
	Import        service.CatalogExport `json:"import"`
}

// ExportCandidates GET /api/discovery/candidates/export — 导出目录配置（默认安全导出）。
// 响应为附件下载（Content-Disposition）；data = {export: 文件本体, excluded_private,
// excluded_query, excluded_sensitive: 排除计数及原因（spec C4）。
// include_private=true 恒 400 validation（v1 不支持连私有导出，design D8）。
func ExportCandidates(c *gin.Context) {
	opts := service.CatalogExportOptions{}
	if v := strings.TrimSpace(c.Query("include_private")); v == "true" || v == "1" {
		opts.IncludePrivate = true
	}
	result, err := newCandidateCatalogService().ExportCatalog(c.Request.Context(), opts)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="syntopica-candidate-catalog.json"`)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// PreviewCatalogImport POST /api/discovery/candidates/import/preview — 预览导入文件。
// 请求体 = 导入文件 JSON 本体（≤2MiB，超限 400 validation；文件级问题：未知 format/
// version/非法 kind/坏 JSON/超 1000 条均 400 validation 零写入）。响应 data = 分类计数、
// 逐项明细、预览指纹与本地 revision（确认时回传）。预览零写库、零网络（spec C3）。
func PreviewCatalogImport(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "unable to read request body", "code": service.CandidateErrorCodeValidation})
		return
	}
	parsed, err := service.ParseCatalogImport(raw)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	preview, err := newCandidateCatalogService().PreviewCatalogImport(c.Request.Context(), parsed)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": preview})
}

// ConfirmCatalogImport POST /api/discovery/candidates/import/confirm — 确认应用导入。
// body：{fingerprint, local_revision, import:{...}}；只应用预览分类为 new 的条目，
// duplicate 复用、conflict 默认跳过保留本地，逐项返回结果（spec C3）。
// 指纹/revision 与预览不符 → 409 + code=stale_preview，必须重新预览；零写入。
func ConfirmCatalogImport(c *gin.Context) {
	var req catalogImportConfirmInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body", "code": service.CandidateErrorCodeValidation})
		return
	}
	fingerprint := strings.TrimSpace(req.Fingerprint)
	if fingerprint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "fingerprint is required (run preview first)", "code": service.CandidateErrorCodeValidation})
		return
	}
	// import 部分重新序列化后走同一 ParseCatalogImport：服务端重新校验，不信任客户端。
	raw, err := json.Marshal(req.Import)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body", "code": service.CandidateErrorCodeValidation})
		return
	}
	parsed, err := service.ParseCatalogImport(raw)
	if err != nil {
		writeCandidateError(c, err)
		return
	}
	result, err := newCandidateCatalogService().ApplyCatalogImport(c.Request.Context(), parsed, fingerprint, req.LocalRevision)
	if err != nil {
		if errors.Is(err, service.ErrStalePreview) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error(), "code": service.CandidateErrorCodeStalePreview})
			return
		}
		writeCandidateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
