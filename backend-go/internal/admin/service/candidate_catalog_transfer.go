package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── 候选目录导入导出（improve-discovery-recommendations，design D8 / spec C3+C4）──
//
// 导入导出是受控的配置操作，不是数据同步：
//   - 导出文件只携带可移植配置（stable_key/kind/路由标识或地址/人工字段/推荐开关），
//     永不含数据库 ID、向量、检查结果、订阅状态或访问授权（spec C4）；
//   - 默认导出仅 public 且无 query 的 rss 条目 + rsshub 条目；私有 scope、带 query、
//     或敏感复核（userinfo/内网 IP 字面量 host）不通过的条目整条排除并计数；
//   - 解析零网络、预览零写库、确认仅应用 new（duplicate 跳过、冲突默认保留本地），
//     逐条独立小事务 + stable_key 唯一冲突容错，同份重复确认幂等（全 duplicate）；
//   - 导入含 userinfo/内网地址的条目落库为 access_scope=private_pending：
//     保存目录不等于授权访问（spec C4「导入私网地址」）。

// 目录导入导出格式常量（design D8：第一版 JSON 格式）。
const (
	CatalogTransferFormat   = "syntopica-candidate-catalog"
	CatalogTransferVersion  = 1
	CatalogImportMaxBytes   = int64(2 << 20) // 2 MiB
	CatalogImportMaxEntries = 1000
)

// 预览分类（spec C3：预览新增、重复、冲突和无效项）。
const (
	CatalogItemNew       = "new"
	CatalogItemDuplicate = "duplicate"
	CatalogItemConflict  = "conflict"
	CatalogItemInvalid   = "invalid"
)

// ErrStalePreview 预览已过期：确认时的文件指纹或本地目录 revision 与预览时不一致，
// 必须重新预览（spec C3「确认期间记录变化返回需重新预览」）。handler 映射 409 + stale_preview。
var ErrStalePreview = errors.New("candidate: stale preview (file fingerprint or local catalog revision changed since preview)")

// CandidateErrorCodeStalePreview 预览脱节的独立可识别 code（design D9：错误响应统一可识别 code）；
// HTTP 状态归入 409 家族，但与条目级 conflict（existing_id）语义不同，不共用 code。
const CandidateErrorCodeStalePreview = "stale_preview"

// CatalogExportEntry 单条可移植目录条目：跨安装搬运的最小配置单元。
// 不含数据库 ID / 向量 / 检查结果 / 订阅状态 / 访问授权（spec C4）。
type CatalogExportEntry struct {
	StableKey             string             `json:"stable_key"`
	Kind                  string             `json:"kind"` // rsshub | rss
	Name                  string             `json:"name,omitempty"`
	Description           string             `json:"description,omitempty"`
	Language              string             `json:"language,omitempty"`
	Region                string             `json:"region,omitempty"`
	FeedURL               string             `json:"feed_url,omitempty"`        // kind=rss
	RouteNamespace        string             `json:"route_namespace,omitempty"` // kind=rsshub
	RoutePath             string             `json:"route_path,omitempty"`      // kind=rsshub
	RecommendationEnabled *bool              `json:"recommendation_enabled,omitempty"`
	ManualMetadata        models.MetadataMap `json:"manual_metadata,omitempty"`
}

// CatalogExport 导出文件整体（导入文件与它同构，v1）。
type CatalogExport struct {
	Format  string               `json:"format"`
	Version int                  `json:"version"`
	Entries []CatalogExportEntry `json:"entries"`
}

// CatalogExportOptions 导出选项。v1 不支持连私有导出（design D8）。
type CatalogExportOptions struct {
	IncludePrivate bool // v1 恒不支持：true 返回 validation 错误
}

// CatalogExportResult 导出产物 + 排除计数（spec C4「默认安全导出：排除并给出数量及原因」）。
type CatalogExportResult struct {
	Export            CatalogExport `json:"export"`
	ExcludedPrivate   int           `json:"excluded_private"`   // access_scope != public
	ExcludedQuery     int           `json:"excluded_query"`     // rss 地址带 query（可能携带 token，保守排除）
	ExcludedSensitive int           `json:"excluded_sensitive"` // 敏感复核：userinfo / 内网 IP 字面量 / 无法安全分类
}

// ExportCatalog 导出目录配置：默认仅 public 且无 query 的 rss 条目 + rsshub 条目（rsshub
// 无敏感地址概念，按 public 导）。每条导出前做一次敏感复核（userinfo / 内网 host 整条排除），
// 凭据与私有地址永不落文件。本方法零网络请求、零 DB 写入。
func (s *CandidateCatalogService) ExportCatalog(ctx context.Context, opts CatalogExportOptions) (*CatalogExportResult, error) {
	if opts.IncludePrivate {
		return nil, newCandidateValidationError(
			"exporting private entries is not supported in v1: private sources and credentials are never written to export files")
	}

	var cands []models.FeedCandidate
	if err := s.db.WithContext(ctx).Order("stable_key ASC").Find(&cands).Error; err != nil {
		return nil, err
	}
	views, err := s.buildViews(ctx, cands)
	if err != nil {
		return nil, err
	}

	result := &CatalogExportResult{Export: CatalogExport{
		Format:  CatalogTransferFormat,
		Version: CatalogTransferVersion,
		Entries: []CatalogExportEntry{},
	}}
	for i, c := range cands {
		v := views[i]
		if c.AccessScope != "public" {
			result.ExcludedPrivate++
			continue
		}
		entry := CatalogExportEntry{
			StableKey: c.StableKey,
			Kind:      c.Kind,
			Name:      v.Name, Description: v.Description,
			Language: v.Language, Region: v.Region,
			ManualMetadata: compactManualMetadata(c.ManualMetadata),
		}
		if c.RecommendationEnabled != nil {
			entry.RecommendationEnabled = c.RecommendationEnabled
		}
		switch c.Kind {
		case "rsshub":
			ns, path := splitRSSHubStableKey(c.StableKey)
			if v.Route != nil { // 已解析上游：优先上游原值（保留原始大小写）
				ns, path = v.Route.Namespace, v.Route.Path
			}
			if ns == "" || path == "" {
				// 无法还原路由标识的条目不能安全分类，保守排除（design D8）。
				result.ExcludedSensitive++
				continue
			}
			entry.RouteNamespace, entry.RoutePath = ns, path
		case "rss":
			if c.FeedURL == nil || *c.FeedURL == "" {
				result.ExcludedSensitive++ // rss 候选缺地址：数据异常，保守排除
				continue
			}
			feedURL := *c.FeedURL
			// 敏感复核：userinfo / 内网 IP 字面量 host 整条排除并计数（spec C4）。
			if urlIsSensitiveForExport(feedURL) {
				result.ExcludedSensitive++
				continue
			}
			// 带 query 的地址可能携带 token，默认保守排除（design D1/D8；
			// 不能假称能识别任意 token，可少导不可错导）。
			if strings.Contains(feedURL, "?") {
				result.ExcludedQuery++
				continue
			}
			entry.FeedURL = feedURL
		default:
			result.ExcludedSensitive++ // 未知 kind：不能安全分类，保守排除
			continue
		}
		result.Export.Entries = append(result.Export.Entries, entry)
	}
	return result, nil
}

// urlIsSensitiveForExport 敏感复核：URL 含 userinfo 或 host 为私网/保留段 IP 字面量 → 不可分享。
// 非 IP 主机名离线无法判定（导出零网络、零 DNS），按可分享处理——保守排除可少导，
// 不假称能识别任意 token（design D8）。无法解析的地址按不可分享处理。
func urlIsSensitiveForExport(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	if u.User != nil {
		return true
	}
	if host := u.Hostname(); host != "" {
		if ip := net.ParseIP(host); ip != nil && !safefetch.IsAllowed(ip, nil) {
			return true
		}
	}
	return false
}

// compactManualMetadata 只保留四个合法人工字段的非空字符串值，其余丢弃。
func compactManualMetadata(m models.MetadataMap) models.MetadataMap {
	if len(m) == 0 {
		return nil
	}
	out := models.MetadataMap{}
	for _, k := range []string{ManualFieldName, ManualFieldDescription, ManualFieldLanguage, ManualFieldRegion} {
		if s, ok := m[k].(string); ok && s != "" {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// splitRSSHubStableKey 由稳定键还原 namespace/path（小写）；无法还原返回空串。
func splitRSSHubStableKey(key string) (ns, path string) {
	i := strings.Index(key, "/")
	if i <= 0 || i >= len(key)-1 {
		return "", ""
	}
	return key[:i], key[i+1:]
}

// ── 导入：解析 ──

// catalogImportFile 导入文件 JSON 形状（与导出文件同构，v1）。
type catalogImportFile struct {
	Format  string               `json:"format"`
	Version int                  `json:"version"`
	Entries []CatalogExportEntry `json:"entries"`
}

// ParsedImportEntry 解析后的有效条目：字段校验与身份推导已完成。
type ParsedImportEntry struct {
	Index                 int // 文件内原始序号（预览按文件顺序展示）
	StableKey             string
	Kind                  string // rsshub | rss
	FeedURL               string // kind=rss：规范化地址（userinfo 若存在则原样保留）
	Private               bool   // userinfo / 内网 IP 字面量 → 落库 access_scope=private_pending
	Name                  string // 有效人工名称（预览展示；rsshub 可为空）
	Manual                models.MetadataMap
	RecommendationEnabled *bool
}

// CatalogItemFailure 逐条问题（invalid / failed / skipped_conflict 复用同一形状）。
type CatalogItemFailure struct {
	StableKey string `json:"stable_key"`
	Reason    string `json:"reason"`
}

// ParsedCatalogImport 解析产物：有效条目 + 无效条目（逐条原因，不炸整份）。
type ParsedCatalogImport struct {
	Format  string
	Version int
	Valid   []ParsedImportEntry
	Invalid []CatalogItemFailure
}

// ParseCatalogImport 解析并校验导入文件。文件级问题（超限 / 未知 format / version /
// 非法 kind / 坏 JSON）拒绝整份；单条字段问题（URL 规则、限长、身份不匹配、文件内重复键）
// 标 invalid 逐条原因。返回 parsed（valid + invalid），不做 DB 访问与网络请求。
func ParseCatalogImport(data []byte) (*ParsedCatalogImport, error) {
	if int64(len(data)) > CatalogImportMaxBytes {
		return nil, newCandidateValidationError(
			fmt.Sprintf("import file exceeds the %d byte (%d MiB) limit", CatalogImportMaxBytes, CatalogImportMaxBytes>>20))
	}
	var file catalogImportFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, newCandidateValidationError(fmt.Sprintf("import file is not valid JSON: %v", err))
	}
	if strings.TrimSpace(file.Format) != CatalogTransferFormat {
		return nil, newCandidateValidationError(
			fmt.Sprintf("unsupported import format %q: expected %q (whole file rejected)", file.Format, CatalogTransferFormat))
	}
	if file.Version != CatalogTransferVersion {
		return nil, newCandidateValidationError(
			fmt.Sprintf("unsupported import version %d: expected %d (incompatible version, nothing imported)", file.Version, CatalogTransferVersion))
	}
	if len(file.Entries) > CatalogImportMaxEntries {
		return nil, newCandidateValidationError(
			fmt.Sprintf("import file contains %d entries, exceeding the %d limit", len(file.Entries), CatalogImportMaxEntries))
	}
	// kind 非法拒整份（design D8「拒绝未知版本和非法类型」）：一条坏 kind 意味着文件
	// 来源不明，逐条放行会静默丢弃语义，整份拒绝让用户修正文件。
	for i, e := range file.Entries {
		if k := strings.TrimSpace(e.Kind); k != "rss" && k != "rsshub" {
			return nil, newCandidateValidationError(
				fmt.Sprintf("entry #%d has illegal kind %q: must be rss or rsshub (whole file rejected)", i, e.Kind))
		}
	}

	parsed := &ParsedCatalogImport{Format: file.Format, Version: file.Version}
	seenKeys := map[string]struct{}{}
	for i, e := range file.Entries {
		entry, reason := parseImportEntry(e)
		entry.Index = i
		if reason != "" {
			parsed.Invalid = append(parsed.Invalid, CatalogItemFailure{StableKey: importStableKeyOrIndex(e.StableKey, i), Reason: reason})
			continue
		}
		if _, dup := seenKeys[entry.StableKey]; dup {
			parsed.Invalid = append(parsed.Invalid, CatalogItemFailure{
				StableKey: importStableKeyOrIndex(e.StableKey, i),
				Reason:    fmt.Sprintf("duplicate stable_key %s within the same file", entry.StableKey),
			})
			continue
		}
		seenKeys[entry.StableKey] = struct{}{}
		parsed.Valid = append(parsed.Valid, entry)
	}
	return parsed, nil
}

// importStableKeyOrIndex 无效条目可能连稳定键都不可信，用文件内序号占位展示。
func importStableKeyOrIndex(key string, index int) string {
	if strings.TrimSpace(key) != "" {
		return key
	}
	return fmt.Sprintf("#%d", index)
}

// parseImportEntry 单条解析与校验：返回规范化条目或拒绝原因。
// 字段限长复用 ValidateManualField / RSSURLMaxRunes（与手动新增同一套规则，design D8）。
func parseImportEntry(e CatalogExportEntry) (ParsedImportEntry, string) {
	if strings.TrimSpace(e.StableKey) == "" && strings.TrimSpace(e.RouteNamespace) == "" && strings.TrimSpace(e.FeedURL) == "" {
		return ParsedImportEntry{}, "entry has no stable_key, route namespace/path or feed_url"
	}

	// 人工元数据：manual_metadata 优先，顶层 name/description/language/region 兜底。
	manual := models.MetadataMap{}
	for k, v := range e.ManualMetadata {
		if k != ManualFieldName && k != ManualFieldDescription && k != ManualFieldLanguage && k != ManualFieldRegion {
			return ParsedImportEntry{}, fmt.Sprintf("manual_metadata key %q is not one of the four allowed fields", k)
		}
		s, ok := v.(string)
		if !ok {
			return ParsedImportEntry{}, fmt.Sprintf("manual_metadata value for %q must be a string", k)
		}
		if s == "" {
			continue // 空串 = 清除覆盖语义，导入侧不写键
		}
		if err := ValidateManualField(k, s); err != nil {
			return ParsedImportEntry{}, err.Error()
		}
		manual[k] = s
	}
	for field, val := range map[string]string{
		ManualFieldDescription: e.Description,
		ManualFieldLanguage:    e.Language,
		ManualFieldRegion:      e.Region,
	} {
		if val == "" {
			continue
		}
		if _, exists := manual[field]; exists {
			continue // manual_metadata 优先于顶层字段
		}
		if err := ValidateManualField(field, val); err != nil {
			return ParsedImportEntry{}, err.Error()
		}
		manual[field] = val
	}
	name := ""
	if s, ok := manual[ManualFieldName].(string); ok {
		name = s
	}
	if name == "" && strings.TrimSpace(e.Name) != "" {
		if err := ValidateManualField(ManualFieldName, e.Name); err != nil {
			return ParsedImportEntry{}, err.Error()
		}
		name = e.Name
		manual[ManualFieldName] = e.Name
	}

	switch strings.TrimSpace(e.Kind) {
	case "rsshub":
		if strings.TrimSpace(e.FeedURL) != "" {
			return ParsedImportEntry{}, "rsshub entry must not carry feed_url (use route_namespace/route_path)"
		}
		key, err := BuildRSSHubStableKey(e.RouteNamespace, e.RoutePath)
		if err != nil {
			return ParsedImportEntry{}, err.Error()
		}
		if sk := strings.TrimSpace(e.StableKey); sk != "" && sk != key {
			return ParsedImportEntry{}, fmt.Sprintf("stable_key %q does not match route namespace/path (expected %q)", sk, key)
		}
		return ParsedImportEntry{StableKey: key, Kind: "rsshub", Name: name, Manual: manual, RecommendationEnabled: e.RecommendationEnabled}, ""
	case "rss":
		normalized, private, err := parseImportRSSURL(e.FeedURL)
		if err != nil {
			return ParsedImportEntry{}, err.Error()
		}
		sum := sha256.Sum256([]byte(normalized))
		key := "rss:" + hex.EncodeToString(sum[:])
		if sk := strings.TrimSpace(e.StableKey); sk != "" && sk != key {
			return ParsedImportEntry{}, fmt.Sprintf("stable_key %q does not match feed_url (expected %q)", sk, key)
		}
		if name == "" {
			return ParsedImportEntry{}, "name is required for rss entries (manual_metadata.name or top-level name)"
		}
		return ParsedImportEntry{
			StableKey: key, Kind: "rss", FeedURL: normalized, Private: private,
			Name: name, Manual: manual, RecommendationEnabled: e.RecommendationEnabled,
		}, ""
	}
	return ParsedImportEntry{}, fmt.Sprintf("kind %q must be rss or rsshub", e.Kind)
}

// parseImportRSSURL 导入条目地址解析：与 NormalizeRSSURL 同一套规则（http/https、host 必填、
// 端口合法且去默认端口、fragment 去除、host 小写、path/query 原样保留、≤500 runes），
// 唯一差异是**保留 userinfo**——含凭据地址不拒绝而是标记 private（spec C4「导入私网地址」：
// 记录标明访问需确认，保存目录不代表授权访问），内网/保留段 IP 字面量 host 同样标记 private。
// 无 userinfo 时输出与 NormalizeRSSURL 逐字节一致，稳定键与本地既有候选可比对。
func parseImportRSSURL(raw string) (normalized string, private bool, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false, errors.New("candidate: import entry feed_url must not be empty or blank")
	}
	if utf8.RuneCountInString(s) > RSSURLMaxRunes {
		return "", false, fmt.Errorf("candidate: import entry feed_url exceeds %d runes", RSSURLMaxRunes)
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false, fmt.Errorf("candidate: invalid import entry feed_url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false, fmt.Errorf("candidate: import entry feed_url scheme must be http or https, got %q", u.Scheme)
	}
	if u.User != nil {
		private = true // 凭据：不拒绝，落库 private_pending，永不进默认导出
	}
	hostname := u.Hostname()
	if hostname == "" {
		return "", false, errors.New("candidate: import entry feed_url must contain a host")
	}
	if ip := net.ParseIP(hostname); ip != nil && !safefetch.IsAllowed(ip, nil) {
		private = true // 内网/保留段 IP 字面量：不解析、不探测，落库 private_pending
	}
	port := u.Port()
	if port != "" {
		n, perr := strconv.Atoi(port)
		if perr != nil || n <= 0 || n > 65535 {
			return "", false, fmt.Errorf("candidate: import entry feed_url port %q is invalid", port)
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
	u.Fragment, u.RawFragment = "", ""
	return u.String(), private, nil
}

// ── 导入：预览 ──

// CatalogPreviewItem 预览逐项分类（含原因）。
type CatalogPreviewItem struct {
	StableKey      string `json:"stable_key"`
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	Classification string `json:"classification"` // new | duplicate | conflict | invalid
	Reason         string `json:"reason,omitempty"`
}

// CatalogImportCounts 预览分类计数。
type CatalogImportCounts struct {
	New       int `json:"new"`
	Duplicate int `json:"duplicate"`
	Conflict  int `json:"conflict"`
	Invalid   int `json:"invalid"`
}

// CatalogImportPreview 预览结果：分类计数、逐项明细、预览指纹与本地 revision
// （两者随确认请求回传做乐观校验，spec C3）。
type CatalogImportPreview struct {
	Counts        CatalogImportCounts  `json:"counts"`
	Items         []CatalogPreviewItem `json:"items"`
	Fingerprint   string               `json:"fingerprint"`
	LocalRevision uint64               `json:"local_revision"`
}

// importClassification 单条有效条目对库的当前分类（含冲突原因）。
type importClassification struct {
	Class  string
	Reason string
}

// PreviewCatalogImport 与库内 stable_key 比对分类 new/duplicate/conflict（invalid 来自解析
// 阶段），输出预览指纹与本地 revision。预览零 DB 写入、零网络请求（spec C3）。
// 分类规则：stable_key 已存在且 kind/名称/URL 与本地有效值一致 → duplicate；
// 已存在但不一致 → conflict（确认默认跳过保留本地）；不存在 → new。
func (s *CandidateCatalogService) PreviewCatalogImport(ctx context.Context, parsed *ParsedCatalogImport) (*CatalogImportPreview, error) {
	if parsed == nil {
		return nil, newCandidateValidationError("parsed import must not be nil")
	}
	preview := &CatalogImportPreview{Items: []CatalogPreviewItem{}}
	classifications, err := s.classifyImportEntries(ctx, parsed.Valid)
	if err != nil {
		return nil, err
	}

	type orderedItem struct {
		index int
		item  CatalogPreviewItem
	}
	ordered := make([]orderedItem, 0, len(parsed.Valid)+len(parsed.Invalid))
	for _, inv := range parsed.Invalid {
		ordered = append(ordered, orderedItem{index: -1, item: CatalogPreviewItem{
			StableKey: inv.StableKey, Classification: CatalogItemInvalid, Reason: inv.Reason,
		}})
		preview.Counts.Invalid++
	}
	for i, e := range parsed.Valid {
		cls := classifications[i]
		ordered = append(ordered, orderedItem{index: e.Index, item: CatalogPreviewItem{
			StableKey: e.StableKey, Kind: e.Kind, Name: e.Name, Classification: cls.Class, Reason: cls.Reason,
		}})
		switch cls.Class {
		case CatalogItemNew:
			preview.Counts.New++
		case CatalogItemDuplicate:
			preview.Counts.Duplicate++
		case CatalogItemConflict:
			preview.Counts.Conflict++
		}
	}
	// 按文件内原始顺序展示（invalid 条目无序号排最前）。
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].index < ordered[j].index })
	for _, o := range ordered {
		preview.Items = append(preview.Items, o.item)
	}

	rev, err := s.localCatalogRevision(ctx)
	if err != nil {
		return nil, err
	}
	preview.LocalRevision = rev
	preview.Fingerprint = computeImportFingerprint(parsed, classifications)
	return preview, nil
}

// classifyImportEntries 对全部有效条目做一次 IN 查询 + 一次 buildViews（禁 N+1）。
// 不回传本地或导入 URL 全文，防凭据经冲突原因泄露（spec C4 遮蔽）。
func (s *CandidateCatalogService) classifyImportEntries(ctx context.Context, entries []ParsedImportEntry) ([]importClassification, error) {
	out := make([]importClassification, len(entries))
	if len(entries) == 0 {
		return out, nil
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.StableKey)
	}
	var locals []models.FeedCandidate
	if err := s.db.WithContext(ctx).Where("stable_key IN ?", keys).Find(&locals).Error; err != nil {
		return nil, err
	}
	localByKey := map[string]models.FeedCandidate{}
	viewNameByKey := map[string]string{}
	if len(locals) > 0 {
		views, err := s.buildViews(ctx, locals)
		if err != nil {
			return nil, err
		}
		for i, lc := range locals {
			localByKey[lc.StableKey] = lc
			viewNameByKey[lc.StableKey] = views[i].Name
		}
	}

	for i, e := range entries {
		local, exists := localByKey[e.StableKey]
		if !exists {
			out[i] = importClassification{Class: CatalogItemNew}
			continue
		}
		if local.Kind != e.Kind {
			out[i] = importClassification{
				Class:  CatalogItemConflict,
				Reason: fmt.Sprintf("stable_key %s exists locally as kind %q while import says %q", e.StableKey, local.Kind, e.Kind),
			}
			continue
		}
		if e.Kind == "rss" {
			localURL := ""
			if local.FeedURL != nil {
				localURL = *local.FeedURL
			}
			if localURL != e.FeedURL {
				out[i] = importClassification{
					Class:  CatalogItemConflict,
					Reason: fmt.Sprintf("feed_url differs from the local candidate with stable_key %s (addresses are masked)", e.StableKey),
				}
				continue
			}
		}
		if e.Name != "" && e.Name != viewNameByKey[e.StableKey] {
			out[i] = importClassification{
				Class:  CatalogItemConflict,
				Reason: fmt.Sprintf("name %q differs from the local effective name %q", e.Name, viewNameByKey[e.StableKey]),
			}
			continue
		}
		out[i] = importClassification{Class: CatalogItemDuplicate}
	}
	return out, nil
}

// computeImportFingerprint 预览指纹：对（有效条目规范化内容 + 当前分类 + 无效条目原因）
// 按文件顺序做 sha256。覆盖文件内容与分类结果：确认时任一变化（文件被换 / 目录被改导致
// 分类翻转）都会使指纹失配 → ErrStalePreview，强制重新预览（spec C3）。
func computeImportFingerprint(parsed *ParsedCatalogImport, classifications []importClassification) string {
	h := sha256.New()
	// hash.Hash 的 Write 实现永不返回错误，显式丢弃满足 errcheck。
	for _, inv := range parsed.Invalid {
		_, _ = fmt.Fprintf(h, "invalid\x1f%s\x1f%s\x1e", inv.StableKey, inv.Reason)
	}
	for i, e := range parsed.Valid {
		enabled := "nil"
		if e.RecommendationEnabled != nil {
			enabled = strconv.FormatBool(*e.RecommendationEnabled)
		}
		// json.Marshal 对 map 按键排序，序列化确定。
		manualJSON, _ := json.Marshal(e.Manual)
		_, _ = fmt.Fprintf(h, "%d\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1e",
			e.Index, e.StableKey, e.Kind, e.FeedURL, string(manualJSON), enabled, classifications[i].Class)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// localCatalogRevision 目录级乐观锁 revision：COUNT×10^12 + SUM(revision)。
// 候选行创建（COUNT+1 且 SUM+1）与更新（revision 自增）都使该值严格递增；本切片目录
// 无删除路径，单调性成立（10^12 余量远超单次操作的 revision 增量）。
func (s *CandidateCatalogService) localCatalogRevision(ctx context.Context) (uint64, error) {
	var cnt int64
	if err := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).Count(&cnt).Error; err != nil {
		return 0, err
	}
	var sum int64
	if err := s.db.WithContext(ctx).Model(&models.FeedCandidate{}).
		Select("COALESCE(SUM(revision), 0)").Scan(&sum).Error; err != nil {
		return 0, err
	}
	if cnt < 0 || sum < 0 {
		return 0, fmt.Errorf("candidate: negative catalog revision aggregate (count=%d, sum=%d)", cnt, sum)
	}
	return uint64(cnt)*1_000_000_000_000 + uint64(sum), nil
}

// ── 导入：确认应用 ──

// CatalogApplyResult 确认后的逐项结果（spec C3：部分失败明确不是全部成功）。
type CatalogApplyResult struct {
	Applied          []string             `json:"applied"`           // 成功应用的 stable_key
	SkippedDuplicate []string             `json:"skipped_duplicate"` // 重复复用已有记录的 stable_key
	SkippedConflict  []CatalogItemFailure `json:"skipped_conflict"`  // 冲突默认保留本地
	Failed           []CatalogItemFailure `json:"failed"`            // 写入失败（含原因）
	Invalid          []CatalogItemFailure `json:"invalid"`           // 解析阶段 invalid 的回显
}

// ApplyCatalogImport 应用导入：仅应用 new（duplicate 跳过、conflict 默认跳过保留本地），
// 逐条独立小事务 + stable_key 唯一冲突 OnConflict Do Nothing 容错并发插入。
// 乐观校验：本地 revision ≠ localRevision 或重算指纹 ≠ previewFingerprint → ErrStalePreview
// 且零写入（localRevision=0 为跳过 revision 校验的降级路径：调用方未持有预览时的 revision，
// 指纹校验仍兜底；正常前端流程总是回传预览返回的 local_revision）。
// 本方法零网络请求、零 Feed 创建、零订阅（spec C1/C3）。
func (s *CandidateCatalogService) ApplyCatalogImport(ctx context.Context, parsed *ParsedCatalogImport, previewFingerprint string, localRevision uint64) (*CatalogApplyResult, error) {
	if parsed == nil {
		return nil, newCandidateValidationError("parsed import must not be nil")
	}
	if localRevision != 0 {
		rev, err := s.localCatalogRevision(ctx)
		if err != nil {
			return nil, err
		}
		if rev != localRevision {
			return nil, fmt.Errorf("%w: local catalog revision changed (preview had %d, now %d)", ErrStalePreview, localRevision, rev)
		}
	}
	classifications, err := s.classifyImportEntries(ctx, parsed.Valid)
	if err != nil {
		return nil, err
	}
	if fp := computeImportFingerprint(parsed, classifications); fp != previewFingerprint {
		return nil, fmt.Errorf("%w: preview fingerprint mismatch (file or catalog changed since preview)", ErrStalePreview)
	}

	result := &CatalogApplyResult{
		Applied:          []string{},
		SkippedDuplicate: []string{},
		SkippedConflict:  []CatalogItemFailure{},
		Failed:           []CatalogItemFailure{},
		Invalid:          append([]CatalogItemFailure{}, parsed.Invalid...),
	}
	for i, e := range parsed.Valid {
		switch classifications[i].Class {
		case CatalogItemDuplicate:
			result.SkippedDuplicate = append(result.SkippedDuplicate, e.StableKey)
			continue
		case CatalogItemConflict:
			result.SkippedConflict = append(result.SkippedConflict, CatalogItemFailure{StableKey: e.StableKey, Reason: classifications[i].Reason})
			continue
		}

		cand := buildImportCandidate(e)
		concurrentDup := false
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(cand)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				// 唯一冲突容错：预览后并发插入同 stable_key → 复查归 duplicate，不报错不重复。
				var existing models.FeedCandidate
				if rerr := tx.Where("stable_key = ?", e.StableKey).First(&existing).Error; rerr != nil {
					return fmt.Errorf("concurrent insert of stable_key %s could not be re-read: %w", e.StableKey, rerr)
				}
				concurrentDup = true
			}
			return nil
		})
		switch {
		case err != nil:
			result.Failed = append(result.Failed, CatalogItemFailure{StableKey: e.StableKey, Reason: err.Error()})
		case concurrentDup:
			result.SkippedDuplicate = append(result.SkippedDuplicate, e.StableKey)
		default:
			result.Applied = append(result.Applied, e.StableKey)
		}
	}
	return result, nil
}

// buildImportCandidate 由解析条目构造落库候选。rsshub 条目 RouteID 留空（未解析状态），
// 待目录同步按 namespace/path 绑定本地上游，不捏造技术参数（design D8）；
// 私有地址条目（userinfo / 内网 IP 字面量）落 access_scope=private_pending——
// 保存目录不等于授权访问，后续自动任务遵守授权门槛（spec C4）。
func buildImportCandidate(e ParsedImportEntry) *models.FeedCandidate {
	enabled := true
	if e.RecommendationEnabled != nil {
		enabled = *e.RecommendationEnabled
	}
	cand := &models.FeedCandidate{
		StableKey:             e.StableKey,
		Kind:                  e.Kind,
		ManualMetadata:        e.Manual,
		RecommendationEnabled: &enabled,
		AccessScope:           "public",
		Revision:              1,
	}
	if e.Private {
		cand.AccessScope = "private_pending"
	}
	if e.Kind == "rss" {
		feedURL := e.FeedURL
		cand.FeedURL = &feedURL
		cand.CanonicalKey = e.FeedURL
	}
	return cand
}
