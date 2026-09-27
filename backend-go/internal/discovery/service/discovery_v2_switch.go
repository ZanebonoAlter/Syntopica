package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── discovery v2 开关（improve-discovery-recommendations 4.6，design 迁移计划 5/6）──
//
// ai_settings key=discovery_v2 的 JSON blob 控制 v2 后台任务是否运行：
// 值为 {"enabled": true|false}（也接受裸布尔 true/false）。缺省、空值、键缺失、
// 读取失败一律按**启用**处理（fail-open）——本开关只用来在迁移/回滚期把 v2 后台
// 噪声关掉，误读成「禁用」会让首次全量回补悄悄停摆，风险高于误读成「启用」。
//
// 开关边界（回滚语义，design 迁移计划 6）：
//   - 管：候选可用性检查 job、候选向量回补 job、运行维护 job 的注册，以及
//     CheckCandidate / 回补服务的入口（关闭时直接返回 configuration 错误）；
//   - 不管：ask/refresh 推荐主链（引擎已整体切到 v2，没有旧引擎可回落）、
//     候选目录的增删改查与导入导出、推荐 accept 建源。
//     因此关闭开关 = 停后台任务 + 停检查端点，已发布的推荐与订阅不受影响；
//     真正回滚到旧推荐引擎必须另做 candidate_preferences → 旧路由资格的投影
//     （见 design 迁移计划 6），本开关不包含该投影。
const (
	discoveryV2ConfigKey  = "discovery_v2"
	discoveryV2ConfigDesc = "发现 v2 后台任务开关（false=停检查/回补/维护任务，推荐主链不受影响）"

	// CandidateErrorCodeConfiguration 是「配置禁用/非法」类错误的可识别 code
	// （design D9 错误码集合：validation/conflict/configuration/unavailable/forbidden）；
	// handler 将其映射为 503 Service Unavailable。
	CandidateErrorCodeConfiguration = "configuration"
)

// newCandidateConfigurationError 构造 configuration 类型错误（handler → 503）。
func newCandidateConfigurationError(msg string) error {
	return &CandidateError{Code: CandidateErrorCodeConfiguration, Message: msg}
}

// discoveryV2DisabledMessage 是开关关闭时的统一错误文案（服务入口返回 configuration 错误、
// job 入口返回良性跳过共用同一措辞，便于排查）。
const discoveryV2DisabledMessage = "discovery v2 background tasks are disabled (ai_settings.discovery_v2.enabled=false)"

// LoadDiscoveryV2Enabled 读 ai_settings.discovery_v2 的启用状态。
// 语义见文件头：缺省/读失败 → true（fail-open，记 warning）；值不可解析 → true + warning。
func LoadDiscoveryV2Enabled(db *gorm.DB) bool {
	if db == nil {
		return true
	}
	var settings models.AISettings
	// Find（而非 First）读缺省行：键缺失时不是错误，不必让 GORM 每小时打一条
	// 「record not found」噪声日志（本函数会被每个候选检查调用）。
	res := db.Where("key = ?", discoveryV2ConfigKey).Limit(1).Find(&settings)
	if res.Error != nil {
		// 含「表缺失」等环境问题：不阻断后台任务，按启用处理并留痕。
		logging.Warnf("read ai_settings.%s failed, treating discovery v2 as enabled: %v", discoveryV2ConfigKey, res.Error)
		return true
	}
	if res.RowsAffected == 0 {
		return true
	}
	raw := strings.TrimSpace(settings.Value)
	if raw == "" {
		return true
	}
	var asBool bool
	if err := json.Unmarshal([]byte(raw), &asBool); err == nil {
		return asBool
	}
	var asObject struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(raw), &asObject); err == nil && asObject.Enabled != nil {
		return *asObject.Enabled
	}
	logging.Warnf("ai_settings.%s value %q is not a boolean/{enabled:bool}, treating discovery v2 as enabled",
		discoveryV2ConfigKey, raw)
	return true
}

// SaveDiscoveryV2Enabled 写 ai_settings.discovery_v2（{"enabled": ...}），镜像
// seed_policy_config 的 find-or-create upsert 形态。生产接线由设置层负责；保留读写对
// 供运维脚本与测试使用。
func SaveDiscoveryV2Enabled(db *gorm.DB, enabled bool) error {
	if db == nil {
		return fmt.Errorf("%s: nil database", discoveryV2ConfigKey)
	}
	value, err := json.Marshal(map[string]bool{"enabled": enabled})
	if err != nil {
		return err
	}
	var settings models.AISettings
	res := db.Where("key = ?", discoveryV2ConfigKey).Limit(1).Find(&settings)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		settings.Value = string(value)
		settings.Description = discoveryV2ConfigDesc
		return db.Save(&settings).Error
	}
	return db.Create(&models.AISettings{
		Key: discoveryV2ConfigKey, Value: string(value), Description: discoveryV2ConfigDesc,
	}).Error
}
