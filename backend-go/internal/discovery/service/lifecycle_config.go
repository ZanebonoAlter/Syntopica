package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 推荐生命周期配置持久化（design D5：推荐时效与用户选择）──
//
// ai_settings JSON blob：key=discovery_lifecycle，缺省字段回 D5 默认值；读取/保存
// 均 Validate，非法值返回 configuration 语义错误（run 层据此落
// error_code=configuration，用户动作端点直接拒绝，绝不静默回落默认值）。
// IO 只在本文件。

const lifecycleConfigKey = "discovery_lifecycle"

// lifecycleConfigDesc 落库描述字段（写入口 saveLifecycleConfig 的落库描述）。
const lifecycleConfigDesc = "推荐时效（TTL）与「暂时不看」冷却天数（design D5 两参数）" //nolint:unused // 服务端配置持久化写入口的落库描述，见 saveLifecycleConfig

// SnoozeDaysDefault 是「暂时不看」默认冷却天数（design D5：默认 30，与迁移回填
// 的 dismissed_at + interval '30 days' 一致）。
const SnoozeDaysDefault = 30

// 生命周期参数合法域（design D5：recommendation_ttl_days / snooze_days 均为 1–365 天）。
const (
	LifecycleMinDays = 1
	LifecycleMaxDays = 365
)

// DiscoveryLifecycleConfig 是推荐时效与冷却配置（design D5）。
type DiscoveryLifecycleConfig struct {
	RecommendationTTLDays int `json:"recommendation_ttl_days"`
	SnoozeDays            int `json:"snooze_days"`
}

// DefaultDiscoveryLifecycleConfig 返回 D5 默认值（TTL=14、冷却=30）。
func DefaultDiscoveryLifecycleConfig() DiscoveryLifecycleConfig {
	return DiscoveryLifecycleConfig{
		RecommendationTTLDays: RecommendationTTLDaysDefault,
		SnoozeDays:            SnoozeDaysDefault,
	}
}

// Validate 逐项校验，一次报出全部越界字段（带字段名与合法域）。
func (c DiscoveryLifecycleConfig) Validate() error {
	var errs []string
	if c.RecommendationTTLDays < LifecycleMinDays || c.RecommendationTTLDays > LifecycleMaxDays {
		errs = append(errs, fmt.Sprintf("recommendation_ttl_days=%d 超出合法域 [%d,%d]",
			c.RecommendationTTLDays, LifecycleMinDays, LifecycleMaxDays))
	}
	if c.SnoozeDays < LifecycleMinDays || c.SnoozeDays > LifecycleMaxDays {
		errs = append(errs, fmt.Sprintf("snooze_days=%d 超出合法域 [%d,%d]",
			c.SnoozeDays, LifecycleMinDays, LifecycleMaxDays))
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// EndOfSnooze 返回自 from 起按配置冷却天数计算的到期时刻。
func (c DiscoveryLifecycleConfig) EndOfSnooze(from time.Time) time.Time {
	return from.AddDate(0, 0, c.SnoozeDays)
}

// ExpiresAt 返回自 from 起按配置 TTL 计算的推荐到期时刻。
func (c DiscoveryLifecycleConfig) ExpiresAt(from time.Time) time.Time {
	return from.AddDate(0, 0, c.RecommendationTTLDays)
}

// loadLifecycleConfig 读 discovery_lifecycle：键缺失/空值 → 全默认；部分字段只覆盖
// 出现项，其余回默认；解析失败或非法值 → 错误（带字段名与合法域）。
func loadLifecycleConfig(db *gorm.DB) (DiscoveryLifecycleConfig, error) {
	cfg := DefaultDiscoveryLifecycleConfig()
	var settings models.AISettings
	err := db.Where("key = ?", lifecycleConfigKey).First(&settings).Error
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
		return cfg, fmt.Errorf("parse %s: %w", lifecycleConfigKey, err)
	}
	if v, ok := raw["recommendation_ttl_days"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.recommendation_ttl_days 类型非法", lifecycleConfigKey)
		}
		cfg.RecommendationTTLDays = int(f)
	}
	if v, ok := raw["snooze_days"]; ok {
		f, ok2 := v.(float64)
		if !ok2 {
			return cfg, fmt.Errorf("%s.snooze_days 类型非法", lifecycleConfigKey)
		}
		cfg.SnoozeDays = int(f)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", lifecycleConfigKey, err)
	}
	return cfg, nil
}

// saveLifecycleConfig 校验并写 discovery_lifecycle（镜像 seed_policy_config 的
// find-or-create upsert 形态）。本切片无生产调用方（前端第一版不另增设置面板，
// 配置由设置层/运维写入，读取侧已接线发布/列表/用户动作）；保留完整读写对供后续接线。
func saveLifecycleConfig(db *gorm.DB, cfg DiscoveryLifecycleConfig) error { //nolint:unused // 配置持久化写入口，测试覆盖 roundtrip；接线后移除
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("%s: %w", lifecycleConfigKey, err)
	}
	value, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var settings models.AISettings
	err = db.Where("key = ?", lifecycleConfigKey).First(&settings).Error
	if err == nil {
		settings.Value = string(value)
		settings.Description = lifecycleConfigDesc
		return db.Save(&settings).Error
	}
	if !isNotFound(err) {
		return err
	}
	return db.Create(&models.AISettings{
		Key: lifecycleConfigKey, Value: string(value), Description: lifecycleConfigDesc,
	}).Error
}
