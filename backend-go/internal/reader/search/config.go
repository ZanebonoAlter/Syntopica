package search

// add-article-search tasks 2.2 — ai_settings 单键标量配置读取
// （spec「查询预处理四层管线」「语义召回约束与降级」三键）。
//
// 模式镜像 tagmanagement 的 LoadTagEdgeRetentionDays（edge_gc.go）：键缺失
// → 静默回默认（正常初始态）；空值/非数字/越界 → 回默认 + logging.Warnf
// （spec 要求非法值回退默认并记 warn）。IO 只在本文件，管线保持纯函数。

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// 搜索配置键与默认值（spec 与 design D2/D3）。
const (
	// TrigramThresholdKey L3 trigram 相似度阈值，默认 0.10。
	TrigramThresholdKey = "search_trigram_threshold"
	// SemanticMinCosineKey 语义路径 cosine 下限，默认 0.55。
	SemanticMinCosineKey = "search_semantic_min_cosine"
	// SemanticTopNKey 语义路径标签召回数上限，默认 8。
	SemanticTopNKey = "search_semantic_top_n"

	DefaultTrigramThreshold  = 0.10
	DefaultSemanticMinCosine = 0.55
	DefaultSemanticTopN      = 8
)

// SemanticConfig 语义路径双约束（cosine ≥ MinCosine ∧ 排名前 TopN）。
type SemanticConfig struct {
	MinCosine float64
	TopN      int
}

// LoadTrigramThreshold 读 search_trigram_threshold。合法域 [0,1]；键缺失/
// 空值/非数字/越界 → DefaultTrigramThreshold（非法值记 warn）。
func LoadTrigramThreshold(ctx context.Context, db *gorm.DB) float64 {
	if v, ok := readAISettingsFloat(ctx, db, TrigramThresholdKey, 0, 1); ok {
		return v
	}
	return DefaultTrigramThreshold
}

// LoadSemanticConfig 读语义双约束两键，各自独立回退默认。
func LoadSemanticConfig(ctx context.Context, db *gorm.DB) SemanticConfig {
	cfg := SemanticConfig{MinCosine: DefaultSemanticMinCosine, TopN: DefaultSemanticTopN}
	if v, ok := readAISettingsFloat(ctx, db, SemanticMinCosineKey, 0, 1); ok {
		cfg.MinCosine = v
	}
	if v, ok := readAISettingsInt(ctx, db, SemanticTopNKey, 1); ok {
		cfg.TopN = v
	}
	return cfg
}

// readAISettingsFloat 读一个 [min,max] 域内的浮点标量键。返回 ok=false 表示
// 应回退默认（调用方决定是否已告警——键缺失静默，其余已在内部 Warnf）。
func readAISettingsFloat(ctx context.Context, db *gorm.DB, key string, minValue, maxValue float64) (float64, bool) {
	if db == nil {
		logging.Warnf("search config %s: nil db; using default", key)
		return 0, false
	}
	raw, found, err := readAISettingsValue(ctx, db, key)
	if err != nil {
		logging.Warnf("search config %s: read failed (%v); using default", key, err)
		return 0, false
	}
	if !found {
		return 0, false // 键缺失：正常初始态，静默回默认
	}
	v, perr := strconv.ParseFloat(raw, 64)
	if perr != nil || v < minValue || v > maxValue {
		logging.Warnf("search config %s: invalid value %q; using default", key, raw)
		return 0, false
	}
	return v, true
}

// readAISettingsInt 读一个 ≥minValue 的整数标量键（同上回退语义）。
func readAISettingsInt(ctx context.Context, db *gorm.DB, key string, minValue int) (int, bool) {
	if db == nil {
		logging.Warnf("search config %s: nil db; using default", key)
		return 0, false
	}
	raw, found, err := readAISettingsValue(ctx, db, key)
	if err != nil {
		logging.Warnf("search config %s: read failed (%v); using default", key, err)
		return 0, false
	}
	if !found {
		return 0, false
	}
	v, perr := strconv.Atoi(raw)
	if perr != nil || v < minValue {
		logging.Warnf("search config %s: invalid value %q; using default", key, raw)
		return 0, false
	}
	return v, true
}

// readAISettingsValue 读 ai_settings 单键 Value（TrimSpace 后）；空值告警并
// 交调用方回默认（spec 把「空」列在非法值集合中）。
func readAISettingsValue(ctx context.Context, db *gorm.DB, key string) (string, bool, error) {
	var setting models.AISettings
	err := db.WithContext(ctx).Where("key = ?", key).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	raw := strings.TrimSpace(setting.Value)
	if raw == "" {
		logging.Warnf("search config %s: empty value; using default", key)
		return "", false, nil
	}
	return raw, true, nil
}
