package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/datasources"
	"syntopica-backend/internal/datasources/wiring"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

// ── fred settings（研究数据源 FRED API key，界面可配 + 动态读）──
// 语义与 comtrade settings 逐条同款（add-fast-frequency-datasources spec
// 「FRED key 界面配置（同款动态链）」）：ai_settings 表 fred_config
// （api_key + enabled），优先级 界面 DB > env FRED_API_KEY > config.yaml
// fred.api_key；取数/目录 status 每次现读，界面改即时生效；设置 API 脱敏
// 回显，空 api_key 保存保留原值，enabled=false 跳过界面值走兜底。

// GetFredSettings GET /api/settings/fred — 读 FRED 配置（脱敏不回显完整 key）。
// 返回 {api_key_configured, api_key_hint(末4位), enabled}。
func GetFredSettings(c *gin.Context) {
	enabled := true
	apiKeyConfigured := false
	apiKeyHint := ""
	if cfg, _, err := aisettings.LoadFredConfig(); err == nil && cfg != nil {
		if v, ok := cfg["api_key"].(string); ok && strings.TrimSpace(v) != "" {
			apiKeyConfigured = true
			apiKeyHint = maskAPIKey(v)
		}
		if v, ok := cfg["enabled"].(bool); ok {
			enabled = v
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"api_key_configured": apiKeyConfigured,
			"api_key_hint":       apiKeyHint,
			"enabled":            enabled,
		},
	})
}

// saveFredSettingsRequest 保存 FRED 配置请求体（comtrade 同款语义：
// Enabled nil=保留现有值；APIKey 空串=保留原值，防表单回填空覆盖）。
type saveFredSettingsRequest struct {
	APIKey  string `json:"api_key"`
	Enabled *bool  `json:"enabled"`
}

// SaveFredSettings POST /api/settings/fred — 写 FRED 配置。界面改即时生效（动态读）。
func SaveFredSettings(c *gin.Context) {
	var req saveFredSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}

	existing, _, _ := aisettings.LoadFredConfig()
	if existing == nil {
		existing = map[string]interface{}{}
	}
	apiKey, _ := existing["api_key"].(string)
	enabled := true
	if v, ok := existing["enabled"].(bool); ok {
		enabled = v
	}
	if k := strings.TrimSpace(req.APIKey); k != "" {
		apiKey = k
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	configJSON := map[string]interface{}{
		"api_key": apiKey,
		"enabled": enabled,
	}
	if err := aisettings.SaveFredConfig(configJSON, "FRED API key (research data sources)"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败"})
		return
	}
	// 同步回写 data_sources 目录 status（live resolver 现算；回写失败不阻断
	// 保存——key 已落库，下次重启 Init 会按同一 resolver 重算）。
	if err := datasources.UpdateStatus(database.DB, "fred", wiring.DataSourcesKeyResolver()); err != nil {
		logging.Warnf("fred settings: refresh data_sources status: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
