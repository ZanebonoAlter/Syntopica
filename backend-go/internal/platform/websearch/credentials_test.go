package websearch

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
)

// setupCredentialsTestDB 装一份单连接内存 sqlite（不碰真库），并把包级 DB 访问
// 指针切到它；测试结束还原。
func setupCredentialsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:websearch-credentials-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AIVendor{}); err != nil {
		t.Fatalf("migrate ai_vendors: %v", err)
	}
	orig := credentialsDB
	credentialsDB = func() *gorm.DB { return db }
	t.Cleanup(func() { credentialsDB = orig })
	return db
}

// seedVendor 插入一条 vendor 并返回带 ID 的副本。
func seedVendor(t *testing.T, db *gorm.DB, v models.AIVendor) models.AIVendor {
	t.Helper()
	if err := db.Create(&v).Error; err != nil {
		t.Fatalf("seed vendor %q: %v", v.Name, err)
	}
	return v
}

// TestResolveCredentials_PicksEnabledDeepSeek 覆盖正常命中：跳过别的供应商，
// 命中 DeepSeek，并归一化 base_url（去尾斜杠）与 api_key（去空白）。
func TestResolveCredentials_PicksEnabledDeepSeek(t *testing.T) {
	db := setupCredentialsTestDB(t)
	seedVendor(t, db, models.AIVendor{
		Name: "LocalLLM", ProviderType: providerTypeOpenAICompatible,
		BaseURL: "http://localhost:11434/v1", APIKey: "local-key", Enabled: true,
	})
	seedVendor(t, db, models.AIVendor{
		Name: "DeepSeek", ProviderType: providerTypeOpenAICompatible,
		BaseURL: " https://api.deepseek.com/v1/ ", APIKey: "  sk-deepseek  ", Enabled: true,
	})

	creds, err := ResolveCredentials(context.Background())
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if creds.VendorName != "DeepSeek" {
		t.Fatalf("VendorName = %q", creds.VendorName)
	}
	if creds.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("BaseURL = %q (want trimmed trailing slash)", creds.BaseURL)
	}
	if creds.APIKey != "sk-deepseek" {
		t.Fatalf("APIKey = %q (want trimmed)", creds.APIKey)
	}
	if creds.VendorID == 0 {
		t.Fatal("VendorID must be set")
	}
}

// TestResolveCredentials_SkipsIneligible 覆盖逐条跳过条件：未启用 / 空 key /
// 纯空白 key / 非 DeepSeek 主机 / 仿冒域名 / provider_type 不匹配。
func TestResolveCredentials_SkipsIneligible(t *testing.T) {
	cases := []struct {
		name   string
		vendor models.AIVendor
	}{
		{"disabled", models.AIVendor{
			Name: "ds-disabled", ProviderType: providerTypeOpenAICompatible,
			BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-x", Enabled: false,
		}},
		{"empty key", models.AIVendor{
			Name: "ds-empty-key", ProviderType: providerTypeOpenAICompatible,
			BaseURL: "https://api.deepseek.com/v1", APIKey: "", Enabled: true,
		}},
		{"blank key", models.AIVendor{
			Name: "ds-blank-key", ProviderType: providerTypeOpenAICompatible,
			BaseURL: "https://api.deepseek.com/v1", APIKey: "   ", Enabled: true,
		}},
		{"other host", models.AIVendor{
			Name: "other-host", ProviderType: providerTypeOpenAICompatible,
			BaseURL: "https://api.openai.com/v1", APIKey: "sk-x", Enabled: true,
		}},
		{"lookalike host", models.AIVendor{
			Name: "lookalike", ProviderType: providerTypeOpenAICompatible,
			BaseURL: "https://api.deepseek.com.evil.example/v1", APIKey: "sk-x", Enabled: true,
		}},
		{"provider type mismatch", models.AIVendor{
			Name: "ollama-ds", ProviderType: "ollama",
			BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-x", Enabled: true,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupCredentialsTestDB(t)
			seedVendor(t, db, tc.vendor)

			_, err := ResolveCredentials(context.Background())
			if !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("want ErrNotConfigured, got %v", err)
			}
			if got := KindOf(err); got != KindNotConfigured {
				t.Fatalf("KindOf = %q, want %q", got, KindNotConfigured)
			}
		})
	}
}

// TestResolveCredentials_LeastIDWins 覆盖多条命中取 id 最小（确定性）。
func TestResolveCredentials_LeastIDWins(t *testing.T) {
	db := setupCredentialsTestDB(t)
	first := seedVendor(t, db, models.AIVendor{
		Name: "DeepSeek-A", ProviderType: providerTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", APIKey: "key-a", Enabled: true,
	})
	seedVendor(t, db, models.AIVendor{
		Name: "DeepSeek-B", ProviderType: providerTypeOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1", APIKey: "key-b", Enabled: true,
	})

	creds, err := ResolveCredentials(context.Background())
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if creds.VendorID != first.ID || creds.APIKey != "key-a" {
		t.Fatalf("want least-id vendor %d/key-a, got %d/%q", first.ID, creds.VendorID, creds.APIKey)
	}
}

// TestResolveCredentials_NotConfiguredOnEmptyTable 覆盖空表。
func TestResolveCredentials_NotConfiguredOnEmptyTable(t *testing.T) {
	setupCredentialsTestDB(t)

	_, err := ResolveCredentials(context.Background())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

// TestResolveCredentials_NilDB 覆盖 DB 未初始化（纯单测/早期启动）不 panic。
func TestResolveCredentials_NilDB(t *testing.T) {
	orig := credentialsDB
	credentialsDB = func() *gorm.DB { return nil }
	t.Cleanup(func() { credentialsDB = orig })

	if _, err := ResolveCredentials(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

// TestHostOf 固化 base_url 主机解析（带 scheme / 裸域名 / 带端口）。
func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"https://api.deepseek.com/v1":   "api.deepseek.com",
		" https://api.deepseek.com/v1/": "api.deepseek.com",
		"api.deepseek.com/v1":           "api.deepseek.com",
		"api.deepseek.com:443/v1":       "api.deepseek.com",
		"":                              "",
	}
	for in, want := range cases {
		if got := hostOf(in); got != want {
			t.Fatalf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
	if !isDeepSeekHost("https://API.DeepSeek.com/v1") {
		t.Fatal("host comparison must be case-insensitive")
	}
	if isDeepSeekHost("https://api.deepseek.com.evil.example/v1") {
		t.Fatal("lookalike host must not match")
	}
}

// TestProviderTypeConstantMatchesAirouter 是漂移守卫（review R-10）：本包刻意
// 不 import airouter（platform 内平级包互引只带来无谓耦合），改由测试断言字面
// 一致——注册表里的 vendor.provider_type 是 airouter 的常量值，两边漂移会让凭据
// 解析静默失效。
func TestProviderTypeConstantMatchesAirouter(t *testing.T) {
	require.Equal(t, airouter.ProviderTypeOpenAICompatible, providerTypeOpenAICompatible)
}
