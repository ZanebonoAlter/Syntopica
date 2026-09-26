package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── seed policy 配置持久化（照抄 internal/platform/aisettings 既有模式）──
//
// ai_settings 表 JSON blob：key=discovery_seed_policy，缺省字段回 D3 默认值；
// 读取/保存均 Validate，非法值返回 configuration 语义错误（run 层据此落
// error_code=configuration，零 provider 调用）。IO 只在本文件，seed_policy.go
// 保持纯函数。

const seedPolicyConfigKey = "discovery_seed_policy"

// seedPolicyConfigDesc 落库描述字段；写入口 saveSeedPolicyConfig 本切片无生产调用方
// （design D3：前端第一版不另增设置面板），供设置层/运维接线后使用。
const seedPolicyConfigDesc = "发现兴趣衰减与种子预算参数（design D3 七参数）" //nolint:unused // 服务端配置持久化写入口的落库描述，见 saveSeedPolicyConfig

// loadSeedPolicyConfig 读 discovery_seed_policy：键缺失/空值 → 全默认；
// 部分字段只覆盖出现项，其余回默认；解析失败或非法值 → 错误（带字段名与合法域）。
func loadSeedPolicyConfig(db *gorm.DB) (SeedPolicyConfig, error) {
	cfg := DefaultSeedPolicyConfig()
	var settings models.AISettings
	err := db.Where("key = ?", seedPolicyConfigKey).First(&settings).Error
	if err != nil {
		if isNotFound(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if strings.TrimSpace(settings.Value) == "" {
		return cfg, nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(settings.Value), &raw); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", seedPolicyConfigKey, err)
	}
	if v, ok := raw["interest_window_days"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.interest_window_days 类型非法", seedPolicyConfigKey)
		}
		cfg.WindowDays = int(f)
	}
	if v, ok := raw["interest_max_entries"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.interest_max_entries 类型非法", seedPolicyConfigKey)
		}
		cfg.MaxEntries = int(f)
	}
	if v, ok := raw["interest_half_life_days"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.interest_half_life_days 类型非法", seedPolicyConfigKey)
		}
		cfg.HalfLifeDays = int(f)
	}
	if v, ok := raw["behavior_maturity_articles"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.behavior_maturity_articles 类型非法", seedPolicyConfigKey)
		}
		cfg.MaturityArticles = int(f)
	}
	if v, ok := raw["seed_candidate_budget"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.seed_candidate_budget 类型非法", seedPolicyConfigKey)
		}
		cfg.SeedBudget = int(f)
	}
	if v, ok := raw["seed_match_similarity"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.seed_match_similarity 类型非法", seedPolicyConfigKey)
		}
		cfg.MatchSimilarity = f
	}
	if v, ok := raw["seed_match_margin"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.seed_match_margin 类型非法", seedPolicyConfigKey)
		}
		cfg.MatchMargin = f
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", seedPolicyConfigKey, err)
	}
	return cfg, nil
}

// saveSeedPolicyConfig 校验并写 discovery_seed_policy（镜像 aisettings
// saveConfigByKey 的 find-or-create upsert 形态；JSON 字段名与 design D3 表一致）。
// 本切片无生产调用方（design D3：前端第一版不另增设置面板，配置由设置层/运维
// 写入，读取侧已接线 StartAsk/Refresh/seedBatches）；保留完整读写对供后续接线。
func saveSeedPolicyConfig(db *gorm.DB, cfg SeedPolicyConfig) error { //nolint:unused // 配置持久化写入口，测试覆盖 roundtrip；接线后移除
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("%s: %w", seedPolicyConfigKey, err)
	}
	value, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var settings models.AISettings
	err = db.Where("key = ?", seedPolicyConfigKey).First(&settings).Error
	if err == nil {
		settings.Value = string(value)
		settings.Description = seedPolicyConfigDesc
		return db.Save(&settings).Error
	}
	if !isNotFound(err) {
		return err
	}
	return db.Create(&models.AISettings{
		Key: seedPolicyConfigKey, Value: string(value), Description: seedPolicyConfigDesc,
	}).Error
}
