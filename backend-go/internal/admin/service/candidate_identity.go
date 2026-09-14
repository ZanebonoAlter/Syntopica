package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"syntopica-backend/internal/models"
)

// ── improve-discovery-recommendations 候选身份纯函数（design D1）──
//
// 稳定身份规范：RSSHub 候选 = 小写 namespace/path（直接规范字符串，不哈希）；
// RSS 候选 = "rss:" + hex(sha256(规范化 URL))。规范化只处理 scheme/host 大小写、
// 默认端口与 fragment，保留 path 大小写、query 顺序与参数，不合并不同有效地址。
// 本文件为纯逻辑：禁止网络调用、DNS 解析、DB 访问与时间函数；稳定键不含数据库 ID、
// source 请求类型或时间。

// ManualMetadata 的合法键（人工覆盖只允许这四个字段，design D1）。
const (
	ManualFieldName        = "name"
	ManualFieldDescription = "description"
	ManualFieldLanguage    = "language"
	ManualFieldRegion      = "region"
)

// manualFieldMaxRunes 是人工字段按 rune 计数的长度上限（与 D8 导入限长一致）。
var manualFieldMaxRunes = map[string]int{
	ManualFieldName:        200,
	ManualFieldDescription: 4000,
	ManualFieldLanguage:    50,
	ManualFieldRegion:      50,
}

// RSSURLMaxRunes 是候选/订阅地址总长上限（rune 计数，与既有 Feed.URL varchar(500) 兼容，
// 不按字节截断中文）。
const RSSURLMaxRunes = 500

// BuildRSSHubStableKey 由 RSSHub namespace + 路由模板构造稳定键：
// 去首尾空白 → 整体小写 → 斜杠拼接。空（含纯空白）namespace 或 path 报错。
func BuildRSSHubStableKey(namespace, path string) (string, error) {
	ns := strings.TrimSpace(namespace)
	p := strings.TrimSpace(path)
	if ns == "" {
		return "", errors.New("candidate: rsshub namespace must not be empty")
	}
	if p == "" {
		return "", errors.New("candidate: rsshub path must not be empty")
	}
	return strings.ToLower(ns) + "/" + strings.ToLower(p), nil
}

// NormalizeRSSURL 规范化原生 RSS 地址（design D1），返回可直接落 FeedCandidate.FeedURL 的指针。
// 规则：仅 http/https；host 小写；http:80 / https:443 默认端口移除；fragment 移除；
// path 大小写与 query 顺序原样保留（非 ASCII path 按规范转义，转义前后身份一致）；
// 拒绝 userinfo、无 host、非法端口（含 >65535）；输入总长 ≤ RSSURLMaxRunes 个 rune
// （utf8.RuneCountInString 计数，不按字节截断中文）。
func NormalizeRSSURL(raw string) (*string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("candidate: feed url must not be empty or blank")
	}
	if utf8.RuneCountInString(s) > RSSURLMaxRunes {
		return nil, fmt.Errorf("candidate: feed url exceeds %d runes", RSSURLMaxRunes)
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("candidate: invalid feed url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("candidate: feed url scheme must be http or https, got %q", u.Scheme)
	}
	if u.User != nil {
		return nil, errors.New("candidate: feed url must not contain userinfo")
	}
	hostname := u.Hostname()
	if hostname == "" {
		return nil, errors.New("candidate: feed url must contain a host")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return nil, fmt.Errorf("candidate: feed url port %q is invalid", port)
		}
		if (u.Scheme == "http" && n == 80) || (u.Scheme == "https" && n == 443) {
			port = ""
		}
	}
	host := strings.ToLower(hostname)
	if strings.Contains(host, ":") { // IPv6 字面量补回方括号
		host = "[" + host + "]"
	}
	if port != "" {
		host = host + ":" + port
	}
	u.Host = host
	u.Fragment = ""
	u.RawFragment = ""
	normalized := u.String()
	return &normalized, nil
}

// BuildRSSStableKey 由原始地址构造原生 RSS 候选稳定键："rss:" + hex(sha256(规范化 URL))。
// 非法地址透传 NormalizeRSSURL 的错误。
func BuildRSSStableKey(rawURL string) (string, error) {
	normalized, err := NormalizeRSSURL(rawURL)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(*normalized))
	return "rss:" + hex.EncodeToString(sum[:]), nil
}

// ValidateManualField 校验单个人工元数据字段：长度按 rune 计数（name≤200 / description≤4000 /
// language、region≤50），name 额外要求非纯空白（空串视为空白拒绝；清除覆盖应直接删除键）。
// 错误信息带字段名，不拼接原始值、不做 HTML 转义。
func ValidateManualField(field, value string) error {
	max, ok := manualFieldMaxRunes[field]
	if !ok {
		return fmt.Errorf("candidate: unknown manual metadata field %q", field)
	}
	if field == ManualFieldName && strings.TrimSpace(value) == "" {
		return fmt.Errorf("candidate: manual field %q must not be blank", field)
	}
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("candidate: manual field %q exceeds %d runes", field, max)
	}
	return nil
}

// EffectiveMetadataValues 是候选对外展示的有效元数据（人工非空覆盖 → 上游 → 缺省空串，
// 两者皆缺不编造资料，design D1）。
type EffectiveMetadataValues struct {
	Name        string
	Description string
	Language    string
	Region      string
}

// EffectiveMetadata 计算有效展示字段：manual 中对应键存在且为非空字符串时覆盖上游值；
// 键存在但值为空串表示回退上游（清空覆盖）；键不存在或值非字符串时同样回退上游。
func EffectiveMetadata(manual models.MetadataMap, upstreamName, upstreamDescription, upstreamLanguage, upstreamRegion string) EffectiveMetadataValues {
	return EffectiveMetadataValues{
		Name:        effectiveValue(manual, ManualFieldName, upstreamName),
		Description: effectiveValue(manual, ManualFieldDescription, upstreamDescription),
		Language:    effectiveValue(manual, ManualFieldLanguage, upstreamLanguage),
		Region:      effectiveValue(manual, ManualFieldRegion, upstreamRegion),
	}
}

// effectiveValue 取单字段有效值：人工非空字符串优先，否则回退上游。
func effectiveValue(manual models.MetadataMap, key, upstream string) string {
	if manual == nil {
		return upstream
	}
	if v, ok := manual[key].(string); ok && v != "" {
		return v
	}
	return upstream
}
