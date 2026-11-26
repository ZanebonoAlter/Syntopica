package websearch

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// ── 凭据解析（design D2）──────────────────────────────────────────────────
//
// 搜索密钥复用 AI 供应商注册表（ai_vendors）：provider_type=openai_compatible
// 且 base_url 主机命中 api.deepseek.com 的已启用供应商，用它的 API Key 做
// Bearer 凭据。不新增独立配置键——两处存同一把 key 必然漂移；也不按 vendor
// name 匹配——name 是用户可改的自由文本，主机才稳定。
//
// 每次调用现读：AI 路由里换 key / 启停供应商即时生效，无缓存、无重启。

const (
	// deepSeekSearchHost 是唯一认可的搜索后端主机（DeepSeek 官方 API）。
	deepSeekSearchHost = "api.deepseek.com"
	// providerTypeOpenAICompatible 与 airouter.ProviderTypeOpenAICompatible 同值
	// （不 import airouter：platform 内平级包互引只会带来无谓耦合）。
	providerTypeOpenAICompatible = "openai_compatible"
)

// Credentials 是一次搜索/探测所需的凭据视图。VendorID/VendorName 供设置面板
// 展示「已关联哪个供应商」，不参与请求。
type Credentials struct {
	VendorID   uint
	VendorName string
	BaseURL    string
	APIKey     string
}

// credentialsDB is the swappable DB accessor (searchHTTPClient precedent):
// 同包测试替换为内存 sqlite，不碰真库；默认读全局 DB（生产装配）。
var credentialsDB = func() *gorm.DB { return database.DB }

// ResolveCredentials 现读 ai_vendors，返回可用于搜索的 DeepSeek 凭据。多条命中
// 取 id 最小者（确定性，避免同一账号多行时结果漂移）；无命中或密钥为空返回
// ErrNotConfigured（errors.Is 可判，调用方据此静默降级）。
func ResolveCredentials(ctx context.Context) (Credentials, error) {
	db := credentialsDB()
	if db == nil {
		return Credentials{}, ErrNotConfigured
	}

	var vendors []models.AIVendor
	err := db.WithContext(ctx).
		Where("enabled = ? AND provider_type = ? AND api_key <> ''", true, providerTypeOpenAICompatible).
		Order("id ASC").
		Find(&vendors).Error
	if err != nil {
		return Credentials{}, fmt.Errorf("websearch: load ai_vendors: %w", err)
	}

	for _, v := range vendors {
		if !isDeepSeekHost(v.BaseURL) || strings.TrimSpace(v.APIKey) == "" {
			continue
		}
		return Credentials{
			VendorID:   v.ID,
			VendorName: strings.TrimSpace(v.Name),
			BaseURL:    strings.TrimRight(strings.TrimSpace(v.BaseURL), "/"),
			APIKey:     strings.TrimSpace(v.APIKey),
		}, nil
	}
	return Credentials{}, ErrNotConfigured
}

// isDeepSeekHost 判定 base_url 的主机是否为 DeepSeek 官方域名。按主机而非
// 完整 URL 比较：base_url 的路径（/v1、/v1/）与是否带 scheme 都属用户可改的
// 细节，只有域名能标识后端身份。
func isDeepSeekHost(baseURL string) bool {
	return strings.EqualFold(hostOf(baseURL), deepSeekSearchHost)
}

// hostOf 从 base_url 取主机名：优先 url.Parse（带 scheme 的常规形态），解析
// 不出主机时按「剥 scheme → 剥路径 → 剥端口」兑底（裸域名 / host:port 形态，
// 同 airouter hostIsLocal 的处理）。
func hostOf(baseURL string) string {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	if idx := strings.Index(raw, "://"); idx >= 0 {
		raw = raw[idx+3:]
	}
	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}
	if h, _, err := net.SplitHostPort(raw); err == nil {
		return h
	}
	return raw
}
