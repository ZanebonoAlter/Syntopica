package service

import (
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/aisettings"
)

// resolveRSSHubBaseURL 读 rsshub_config.rsshub_base_url；缺失/空回落 DefaultRSSHubBaseURL（design E）。
// catalog_sync 与 recommendation_service 共用：实例地址改一处，全链路生效。
func resolveRSSHubBaseURL(db *gorm.DB) string {
	cfg, _, err := aisettings.LoadRSSHubConfig()
	if err != nil {
		return DefaultRSSHubBaseURL
	}
	if u, ok := cfg["rsshub_base_url"].(string); ok {
		if trimmed := strings.TrimSpace(u); trimmed != "" {
			return trimmed
		}
	}
	return DefaultRSSHubBaseURL
}
