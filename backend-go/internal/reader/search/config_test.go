package search

// add-article-search tasks 2.2 — ai_settings 单键标量读取的函数单测。
// 覆盖 test-cases.md 白盒「阈值边界值表」前三行的配置侧：
//   - search_trigram_threshold 0.09/0.10/0.11 透传，非法（负数/非数字/>1/空）回退 0.10 + warn
//   - search_semantic_min_cosine 0.54/0.55/0.56 透传与非法回退
//   - search_semantic_top_n 第 8/9 名边界对应的配置透传与非法回退
// ai_settings 是简单 KV 读取（无 pg 方言语义分叉），按 testing.md
//「轻量 CRUD 可用内存 SQLite」；召回侧行为验证在 *_pg_test.go。

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

func setupConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 每测试独立命名内存库：cache=shared 的固定名会在同进程测试间串数据。
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:config-test-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AISettings{}))
	return db
}

func setConfig(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	require.NoError(t, db.Create(&models.AISettings{Key: key, Value: value}).Error)
}

func TestLoadTrigramThresholdPassThrough(t *testing.T) {
	db := setupConfigTestDB(t)
	ctx := context.Background()
	for _, v := range []string{"0.09", "0.10", "0.11", "0", "1"} {
		setConfig(t, db, TrigramThresholdKey, v)
		got := LoadTrigramThreshold(ctx, db)
		want := parseFloat(t, v)
		require.InDelta(t, want, got, 1e-9, "value %s 须原样透传", v)
		db.Where("key = ?", TrigramThresholdKey).Delete(&models.AISettings{})
	}
}

func TestLoadTrigramThresholdFallback(t *testing.T) {
	db := setupConfigTestDB(t)
	ctx := context.Background()

	// 键缺失：静默回默认（正常初始态，无 warn——不断言日志）。
	require.InDelta(t, DefaultTrigramThreshold, LoadTrigramThreshold(ctx, db), 1e-9)

	for _, v := range []string{"-0.5", "1.5", "abc", "", "   "} {
		var captured strings.Builder
		logging.SetWriters(&captured, &captured)
		db.Where("key = ?", TrigramThresholdKey).Delete(&models.AISettings{})
		setConfig(t, db, TrigramThresholdKey, v)
		require.InDelta(t, DefaultTrigramThreshold, LoadTrigramThreshold(ctx, db), 1e-9,
			"非法值 %q 须回退默认 0.10", v)
		logging.ResetWriters()
		require.Contains(t, captured.String(), TrigramThresholdKey, "非法值 %q 须记 warn", v)
	}
}

func TestLoadSemanticConfigPassThrough(t *testing.T) {
	db := setupConfigTestDB(t)
	ctx := context.Background()

	for _, v := range []string{"0.54", "0.55", "0.56"} {
		db.Where("key = ?", SemanticMinCosineKey).Delete(&models.AISettings{})
		setConfig(t, db, SemanticMinCosineKey, v)
		cfg := LoadSemanticConfig(ctx, db)
		require.InDelta(t, parseFloat(t, v), cfg.MinCosine, 1e-9, "min_cosine=%s 透传", v)
		require.Equal(t, DefaultSemanticTopN, cfg.TopN, "另一键缺失回默认")
	}
	db.Where("key = ?", SemanticMinCosineKey).Delete(&models.AISettings{})
	for _, v := range []string{"7", "8", "9", "1"} {
		db.Where("key = ?", SemanticTopNKey).Delete(&models.AISettings{})
		setConfig(t, db, SemanticTopNKey, v)
		cfg := LoadSemanticConfig(ctx, db)
		require.Equal(t, mustAtoi(t, v), cfg.TopN, "top_n=%s 透传", v)
		require.InDelta(t, DefaultSemanticMinCosine, cfg.MinCosine, 1e-9)
	}
}

func TestLoadSemanticConfigFallback(t *testing.T) {
	db := setupConfigTestDB(t)
	ctx := context.Background()

	require.Equal(t, SemanticConfig{MinCosine: DefaultSemanticMinCosine, TopN: DefaultSemanticTopN},
		LoadSemanticConfig(ctx, db), "双键缺失全默认")

	for _, v := range []string{"-1", "1.1", "xyz", ""} {
		var captured strings.Builder
		logging.SetWriters(&captured, &captured)
		db.Where("key = ?", SemanticMinCosineKey).Delete(&models.AISettings{})
		setConfig(t, db, SemanticMinCosineKey, v)
		cfg := LoadSemanticConfig(ctx, db)
		require.InDelta(t, DefaultSemanticMinCosine, cfg.MinCosine, 1e-9, "cosine 非法值 %q 回默认", v)
		logging.ResetWriters()
		require.Contains(t, captured.String(), SemanticMinCosineKey, "cosine 非法值 %q 须 warn", v)
	}
	for _, v := range []string{"0", "-3", "abc", ""} {
		var captured strings.Builder
		logging.SetWriters(&captured, &captured)
		db.Where("key = ?", SemanticTopNKey).Delete(&models.AISettings{})
		setConfig(t, db, SemanticTopNKey, v)
		cfg := LoadSemanticConfig(ctx, db)
		require.Equal(t, DefaultSemanticTopN, cfg.TopN, "top_n 非法值 %q 回默认", v)
		logging.ResetWriters()
		require.Contains(t, captured.String(), SemanticTopNKey, "top_n 非法值 %q 须 warn", v)
	}
}

func parseFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	require.NoError(t, err)
	return v
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	require.NoError(t, err)
	return v
}
