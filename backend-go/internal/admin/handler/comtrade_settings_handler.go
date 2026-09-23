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

// ── comtrade settings（研究数据源 UN Comtrade 订阅 key，界面可配 + 动态读）──
// 语义对齐 bocha settings：ai_settings 表 comtrade_config（api_key + enabled），
// 取数器/目录 status 每次现读，界面改即时生效；env COMTRADE_API_KEY /
// config.yaml comtrade.api_key 仍作兜底（优先级：界面 DB > env > config.yaml）。

// GetComtradeSettings GET /api/settings/comtrade — 读 Comtrade 配置（脱敏不回显完整 key）。
// 返回 {api_key_configured, api_key_hint(末4位), enabled}。
func GetComtradeSettings(c *gin.Context) {
	enabled := true
	apiKeyConfigured := false
	apiKeyHint := ""
	if cfg, _, err := aisettings.LoadComtradeConfig(); err == nil && cfg != nil {
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

// saveComtradeSettingsRequest 保存 Comtrade 配置请求体。
// Enabled 用指针：nil（未传）=保留现有值；非 nil=覆盖。
// APIKey 空串=不改（保留原值）；非空才覆盖，防表单回填空覆盖掉已有 key。
type saveComtradeSettingsRequest struct {
	APIKey  string `json:"api_key"`
	Enabled *bool  `json:"enabled"`
}

// SaveComtradeSettings POST /api/settings/comtrade — 写 Comtrade 配置。界面改即时生效（动态读）。
func SaveComtradeSettings(c *gin.Context) {
	var req saveComtradeSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}

	// 读现有配置，作为「不改」语义的缺省源。
	existing, _, _ := aisettings.LoadComtradeConfig()
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
	if err := aisettings.SaveComtradeConfig(configJSON, "UN Comtrade subscription key (research data sources)"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "保存配置失败"})
		return
	}
	// 同步回写 data_sources 目录 status（live resolver 现算，UI 已配 key 即时
	// 生效不必重启；design §10.4 状态源统一）。回写失败不阻断保存——key 已
	// 落库，下次重启 Init 会按同一 resolver 重算。
	if err := datasources.UpdateStatus(database.DB, "un_comtrade", wiring.ComtradeKeyResolver()); err != nil {
		logging.Warnf("comtrade settings: refresh data_sources status: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
