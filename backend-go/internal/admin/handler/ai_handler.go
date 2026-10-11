package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/logging"
)

// ─── AI vendor / model / route / observability 管理端点（ai-provider-model-overhaul
// design D7：响应统一 {"success":true,"data":...}，错误 {"success":false,"error":...}）───

type UpsertVendorRequest struct {
	Name             string `json:"name"`
	PresetID         string `json:"preset_id"`
	ProviderType     string `json:"provider_type"`
	BaseURL          string `json:"base_url"`
	APIKey           string `json:"api_key"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	StartCommand     string `json:"start_command"`
	ThinkingDialect  string `json:"thinking_dialect"`
	StructuredOutput string `json:"structured_output"`
	Enabled          *bool  `json:"enabled"`
	// 仅更新路径：clear_api_key=true → key 置空；false 且 api_key 空串 → 沿用
	// 已存值（specs/ai-vendor-model「Vendor API Key 可空与显式清除」）。
	ClearAPIKey bool `json:"clear_api_key"`
	// clear_start_command 同理（start_command 只回 configured 布尔，前端无法
	// 回传原文，清除必须显式标记）。
	ClearStartCommand bool `json:"clear_start_command"`
}

type UpdateModelRequest struct {
	Enabled          *bool   `json:"enabled"`
	ModelKind        string  `json:"model_kind"`
	ThinkingLevel    *string `json:"thinking_level"` // 可空串=清除（继承全局默认）
	SupportsXhighMax *bool   `json:"supports_xhigh_max"`
}

type UpdateRouteBindingsRequest struct {
	Name           string `json:"name"`
	Enabled        *bool  `json:"enabled"`
	Description    string `json:"description"`
	MaxConcurrency int    `json:"max_concurrency"`
	Bindings       []struct {
		ModelID       uint   `json:"model_id"`
		ThinkingLevel string `json:"thinking_level"`
	} `json:"bindings"`
}

type TestVendorInlineRequest struct {
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	ProviderType   string `json:"provider_type"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// vendorPayload 脱敏视图：api_key/start_command 只回 configured 布尔
// （specs/ai-vendor-model「Vendor API Key 可空与显式清除」+ D7 详情脱敏规则）。
func vendorPayload(v models.AIVendor) gin.H {
	return gin.H{
		"id":                       v.ID,
		"name":                     v.Name,
		"preset_id":                v.PresetID,
		"provider_type":            v.ProviderType,
		"base_url":                 v.BaseURL,
		"timeout_seconds":          v.TimeoutSeconds,
		"thinking_dialect":         v.ThinkingDialect,
		"structured_output":        v.StructuredOutput,
		"enabled":                  v.Enabled,
		"api_key_configured":       strings.TrimSpace(v.APIKey) != "",
		"start_command_configured": strings.TrimSpace(v.StartCommand) != "",
	}
}

func ListAIVendors(c *gin.Context) {
	store := airouter.NewStore(repository.Repo.DB())
	vendors, err := store.ListVendors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// models_count：该 vendor 模型数；capabilities_count：其模型被绑定的去重
	// capability 数（前端扇出徽章数据源，W7/U2）。
	modelsCount := make(map[uint]int64)
	var counts []struct {
		VendorID uint  `gorm:"column:vendor_id"`
		N        int64 `gorm:"column:n"`
	}
	if err := repository.Repo.DB().Model(&models.AIModel{}).
		Select("vendor_id, count(*) AS n").Group("vendor_id").Scan(&counts).Error; err == nil {
		for _, row := range counts {
			modelsCount[row.VendorID] = row.N
		}
	}
	capabilitiesCount := make(map[uint]map[string]bool)
	if routes, rerr := store.ListRoutesWithModels(); rerr == nil {
		for _, route := range routes {
			for _, link := range route.RouteModels {
				vid := link.Model.VendorID
				if capabilitiesCount[vid] == nil {
					capabilitiesCount[vid] = make(map[string]bool)
				}
				capabilitiesCount[vid][route.Capability] = true
			}
		}
	}

	data := make([]gin.H, 0, len(vendors))
	for _, vendor := range vendors {
		item := vendorPayload(vendor)
		item["models_count"] = modelsCount[vendor.ID]
		item["capabilities_count"] = len(capabilitiesCount[vendor.ID])
		data = append(data, item)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// refreshVendorModelsFn 适配 store.RefreshVendorModels 的 fetch 闭包（探测即
// GET {base_url}/models 清单拉取，TestVendorConnection 同一语义）。
func refreshVendorModelsFn(ctx context.Context, vendor models.AIVendor) ([]string, error) {
	return airouter.TestVendorConnection(ctx, vendor)
}

func CreateAIVendor(c *gin.Context) {
	var req UpsertVendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	vendor := models.AIVendor{
		Name:             req.Name,
		PresetID:         req.PresetID,
		ProviderType:     req.ProviderType,
		BaseURL:          req.BaseURL,
		APIKey:           req.APIKey,
		TimeoutSeconds:   req.TimeoutSeconds,
		StartCommand:     req.StartCommand,
		ThinkingDialect:  req.ThinkingDialect,
		StructuredOutput: req.StructuredOutput,
		Enabled:          req.Enabled == nil || *req.Enabled,
	}
	store := airouter.NewStore(repository.Repo.DB())
	if err := store.CreateVendor(&vendor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// 保存后自动触发一次清单刷新（D7）。刷新失败不回滚 vendor 创建：
	// 200 仍 success=true，带 models_refreshed:false + refresh_error（W7/U2）。
	data := gin.H{"id": vendor.ID}
	result, refreshErr := store.RefreshVendorModels(c.Request.Context(), vendor.ID, refreshVendorModelsFn)
	if refreshErr != nil {
		logging.Warnf("ai-vendor: post-create refresh failed for vendor %q: %v", vendor.Name, refreshErr)
		data["models_refreshed"] = false
		data["refresh_error"] = refreshErr.Error()
	} else {
		data["models_refreshed"] = true
		data["added"] = len(result.Added)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func GetAIVendor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid vendor id"})
		return
	}

	var vendor models.AIVendor
	if err := repository.Repo.DB().First(&vendor, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "vendor not found"})
		return
	}

	var modelList []models.AIModel
	if err := repository.Repo.DB().Where("vendor_id = ?", vendor.ID).Order("name ASC").Find(&modelList).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	modelsData := make([]gin.H, 0, len(modelList))
	for _, m := range modelList {
		modelsData = append(modelsData, modelPayload(m, nil))
	}

	data := vendorPayload(vendor)
	data["models"] = modelsData
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func UpdateAIVendor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid vendor id"})
		return
	}

	var vendor models.AIVendor
	if err := repository.Repo.DB().First(&vendor, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "vendor not found"})
		return
	}

	var req UpsertVendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	vendor.Name = req.Name
	vendor.PresetID = req.PresetID
	vendor.ProviderType = req.ProviderType
	vendor.BaseURL = req.BaseURL
	vendor.TimeoutSeconds = req.TimeoutSeconds
	vendor.ThinkingDialect = req.ThinkingDialect
	vendor.StructuredOutput = req.StructuredOutput
	// start_command：清除标记优先；否则空串=沿用已存值（原文不回显，前端无法回传）
	if req.ClearStartCommand {
		vendor.StartCommand = ""
	} else if strings.TrimSpace(req.StartCommand) != "" {
		vendor.StartCommand = req.StartCommand
	}
	if req.Enabled != nil {
		vendor.Enabled = *req.Enabled
	}

	store := airouter.NewStore(repository.Repo.DB())
	if err := store.UpdateVendor(&vendor, req.ClearAPIKey); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": vendor.ID}})
}

func DeleteAIVendor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid vendor id"})
		return
	}

	// 级联删除（specs/ai-vendor-model「Vendor 删除级联」）：单事务解绑→删模型→
	// 删 vendor，不因仍被路由引用而拒绝；响应告知两个级联计数（W8-7）。
	deletedModels, detachedBindings, err := airouter.NewStore(repository.Repo.DB()).DeleteVendor(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("已删除 %d 个模型、解除 %d 条绑定", deletedModels, detachedBindings),
		"data": gin.H{
			"deleted_models":    deletedModels,
			"detached_bindings": detachedBindings,
		},
	})
}

func RefreshAIVendorModels(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid vendor id"})
		return
	}

	result, err := airouter.NewStore(repository.Repo.DB()).
		RefreshVendorModels(c.Request.Context(), uint(id), refreshVendorModelsFn)
	if err != nil {
		// 刷新失败（网络/超时/非 2xx/vendor 不存在）：库内清单保持刷新前状态
		// （W7-3），错误透传给前端 toast。
		logging.Warnf("ai-vendor: refresh failed for vendor %d: %v", id, err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
		return
	}

	added := result.Added
	if added == nil {
		added = []string{}
	}
	stale := result.Stale
	if stale == nil {
		stale = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"added":         added,
		"stale":         stale,
		"visible_count": len(result.Visible),
	}})
}

func TestAIVendor(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid vendor id"})
		return
	}

	var vendor models.AIVendor
	if err := repository.Repo.DB().First(&vendor, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "vendor not found"})
		return
	}

	ids, err := airouter.TestVendorConnection(c.Request.Context(), vendor)
	if err != nil {
		// 测试失败是业务结果而非服务端错误：200 + success=false，前端 toast 展示。
		logging.Warnf("ai-test: connection test failed for vendor %q: %v", vendor.BaseURL, err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	msg := "连接成功"
	if len(ids) > 0 {
		msg = fmt.Sprintf("连接成功，共 %d 个可用模型", len(ids))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": msg, "data": gin.H{"models": ids}})
}

// TestAIVendorInline 内联测试（D7 补充端点）：未保存表单两段式弹窗保存前测试，
// 不走库、不落库。
func TestAIVendorInline(c *gin.Context) {
	var req TestVendorInlineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "base_url is required"})
		return
	}

	vendor := models.AIVendor{
		BaseURL:        strings.TrimSpace(req.BaseURL),
		APIKey:         strings.TrimSpace(req.APIKey),
		ProviderType:   strings.TrimSpace(req.ProviderType),
		TimeoutSeconds: req.TimeoutSeconds,
	}
	ids, err := airouter.TestVendorConnection(c.Request.Context(), vendor)
	if err != nil {
		logging.Warnf("ai-test: inline connection test failed for %s: %v", vendor.BaseURL, err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}

	msg := "连接成功"
	if len(ids) > 0 {
		msg = fmt.Sprintf("连接成功，共 %d 个可用模型", len(ids))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": msg, "data": gin.H{"models": ids}})
}

// modelPayload 模型视图。带 vendor 时附 vendor 摘要与 usable 布尔
// （W9-7：stale/disabled 模型不出现在绑定候选——usable=false）。
func modelPayload(m models.AIModel, vendor *models.AIVendor) gin.H {
	item := gin.H{
		"id":                 m.ID,
		"vendor_id":          m.VendorID,
		"name":               m.Name,
		"model_kind":         m.ModelKind,
		"thinking_level":     m.ThinkingLevel,
		"supports_xhigh_max": m.SupportsXhighMax,
		"enabled":            m.Enabled,
		"status":             m.Status,
		"last_seen_at":       m.LastSeenAt,
	}
	if vendor != nil {
		item["vendor"] = gin.H{
			"id":            vendor.ID,
			"name":          vendor.Name,
			"provider_type": vendor.ProviderType,
			"base_url":      vendor.BaseURL,
		}
		item["usable"] = m.Enabled && m.Status == airouter.ModelStatusOK && vendor.Enabled
	}
	return item
}

func ListAIModels(c *gin.Context) {
	store := airouter.NewStore(repository.Repo.DB())
	modelList, err := store.ListModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	vendors, err := store.ListVendors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	vendorByID := make(map[uint]models.AIVendor, len(vendors))
	for _, v := range vendors {
		vendorByID[v.ID] = v
	}

	data := make([]gin.H, 0, len(modelList))
	for _, m := range modelList {
		vendor := vendorByID[m.VendorID]
		data = append(data, modelPayload(m, &vendor))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func UpdateAIModel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid model id"})
		return
	}

	var m models.AIModel
	if err := repository.Repo.DB().First(&m, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "model not found"})
		return
	}

	var req UpdateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if req.ModelKind != "" {
		m.ModelKind = req.ModelKind
	}
	if req.ThinkingLevel != nil { // 空串=清除（UpsertModel 归一为 NULL=继承全局默认档）
		m.ThinkingLevel = req.ThinkingLevel
	}
	if req.SupportsXhighMax != nil {
		m.SupportsXhighMax = *req.SupportsXhighMax
	}

	if err := airouter.NewStore(repository.Repo.DB()).UpsertModel(&m); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": modelPayload(m, nil)})
}

func ListAIRoutes(c *gin.Context) {
	routes, err := airouter.NewStore(repository.Repo.DB()).ListRoutesWithModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	data := make([]gin.H, 0, len(routes))
	for _, route := range routes {
		bindings := make([]gin.H, 0, len(route.RouteModels))
		for _, link := range route.RouteModels {
			bindings = append(bindings, gin.H{
				"id":             link.ID,
				"route_id":       link.RouteID,
				"model_id":       link.ModelID,
				"priority":       link.Priority,
				"enabled":        link.Enabled,
				"thinking_level": link.ThinkingLevel, // nil=继承模型默认档
				"model": gin.H{
					"id":                 link.Model.ID,
					"name":               link.Model.Name,
					"model_kind":         link.Model.ModelKind,
					"thinking_level":     link.Model.ThinkingLevel,
					"supports_xhigh_max": link.Model.SupportsXhighMax,
					"enabled":            link.Model.Enabled,
					"status":             link.Model.Status,
				},
				"vendor": gin.H{
					"id":   link.Model.Vendor.ID,
					"name": link.Model.Vendor.Name,
				},
			})
		}

		data = append(data, gin.H{
			"id":              route.ID,
			"name":            route.Name,
			"capability":      route.Capability,
			"enabled":         route.Enabled,
			"description":     route.Description,
			"max_concurrency": route.MaxConcurrency,
			"route_models":    bindings,
		})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func UpdateAIRoute(c *gin.Context) {
	capability := strings.TrimSpace(c.Param("capability"))
	if capability == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "capability is required"})
		return
	}

	var req UpdateRouteBindingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	route := &models.AIRoute{
		Capability:     capability,
		Name:           strings.TrimSpace(req.Name),
		Description:    strings.TrimSpace(req.Description),
		MaxConcurrency: req.MaxConcurrency,
		Enabled:        req.Enabled == nil || *req.Enabled,
	}
	if route.Name == "" {
		route.Name = airouter.DefaultRouteName
	}

	bindings := make([]airouter.RouteModelBindingInput, 0, len(req.Bindings))
	for _, b := range req.Bindings {
		bindings = append(bindings, airouter.RouteModelBindingInput{
			ModelID:       b.ModelID,
			ThinkingLevel: strings.TrimSpace(b.ThinkingLevel),
		})
	}

	// 校验与写入都在 store 层（W9）：类型约束/thinking_level 合法性的错误文案
	// 原样透传（400），前端 toast 直接展示。
	if err := airouter.NewStore(repository.Repo.DB()).UpsertRouteModels(route, bindings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "route updated"})
}

func QueryDegradations(c *gin.Context) {
	windowStr := c.Query("window")
	var window time.Duration
	switch windowStr {
	case "24h":
		window = 24 * time.Hour
	case "7d":
		window = 7 * 24 * time.Hour
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "window must be 24h or 7d"})
		return
	}

	report, err := airouter.NewStore(repository.Repo.DB()).
		QueryDegradations(window, c.Query("capability"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}

func GetSettings(c *gin.Context) {
	// 前端 §4 重写前的最小兼容：字段名沿用 provider_id/provider_name，值填
	// vendor/模型（ai-provider-model-overhaul D7 GetSettings 适配）。
	primary, vendor, _, err := airouter.NewRouter().ResolvePrimaryModel(airouter.CapabilitySummary)
	if err != nil {
		logging.Warnf("ai-settings: resolve primary model failed: %v", err)
	}

	data := gin.H{}

	if primary != nil && vendor != nil {
		data["provider_id"] = vendor.ID
		data["provider_name"] = vendor.Name
		data["base_url"] = vendor.BaseURL
		data["model"] = primary.Name
		data["api_key_configured"] = strings.TrimSpace(vendor.APIKey) != ""
	}

	var summarySetting models.AISettings
	if err := repository.Repo.DB().Where("key = ?", "summary_config").First(&summarySetting).Error; err == nil {
		var cfg map[string]interface{}
		if json.Unmarshal([]byte(summarySetting.Value), &cfg) == nil {
			if tr, ok := cfg["time_range"]; ok {
				if trNum, ok := tr.(float64); ok {
					data["time_range"] = int(trNum)
				}
			}
		}
	}

	var settings []models.AISettings
	repository.Repo.DB().Order("key ASC").Find(&settings)
	for _, s := range settings {
		if s.Key == "summary_config" || s.Key == "firecrawl_config" || s.Key == "bocha_config" {
			continue
		}
		data[s.Key] = s.Value
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

type SaveSettingsRequest struct {
	DailyReportTime string `json:"daily_report_time"`
}

func SaveSettings(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if val, ok := req["daily_report_time"]; ok {
		strVal, ok := val.(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "daily_report_time must be a string"})
			return
		}
		if err := aisettings.SaveDailyReportTimeConfig(strVal); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
