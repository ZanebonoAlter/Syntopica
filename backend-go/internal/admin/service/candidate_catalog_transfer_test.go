package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 目录导入导出 S11/S12（feed-candidate-catalog spec：Portable Catalog Import and Export /
// Private Source and Export Safety）──
//
// sqlite 内存库复用 setupCandidateCatalogTestDB（FeedCandidate/Feed/RSSHubRoute/RouteParamOption）。
// 导入链路零网络：本文件全部用例不构造任何 HTTP 客户端；「零订阅」以 feeds 表行数断言。

// seedCandidate 落一条候选（测试便捷函数）。
func seedCandidate(t *testing.T, db *gorm.DB, cand models.FeedCandidate) models.FeedCandidate {
	t.Helper()
	require.NoError(t, db.Create(&cand).Error)
	return cand
}

func newRSSCandidate(stableKey, name, feedURL, accessScope string) models.FeedCandidate {
	enabled := true
	return models.FeedCandidate{
		StableKey:             stableKey,
		Kind:                  "rss",
		FeedURL:               &feedURL,
		CanonicalKey:          feedURL,
		ManualMetadata:        models.MetadataMap{ManualFieldName: name},
		RecommendationEnabled: &enabled,
		AccessScope:           accessScope,
		Revision:              1,
	}
}

// marshalImportFile 由条目构造导入文件 JSON。
func marshalImportFile(t *testing.T, entries []CatalogExportEntry) []byte {
	t.Helper()
	data, err := json.Marshal(CatalogExport{Format: CatalogTransferFormat, Version: CatalogTransferVersion, Entries: entries})
	require.NoError(t, err)
	return data
}

// rssEntryWithKey 构造带正确稳定键的 rss 导入条目（地址须可通过 NormalizeRSSURL：
// userinfo/私网凭据地址请手工构造——BuildRSSStableKey 经 NormalizeRSSURL 拒绝 userinfo）。
func rssEntryWithKey(t *testing.T, name, feedURL string) CatalogExportEntry {
	t.Helper()
	return CatalogExportEntry{StableKey: stableKeyForRSS(t, feedURL), Kind: "rss", Name: name, FeedURL: feedURL}
}

func stableKeyForRSS(t *testing.T, feedURL string) string {
	t.Helper()
	key, err := BuildRSSStableKey(feedURL)
	require.NoError(t, err)
	return key
}

// ── S12-1 默认安全导出：排除私有 scope / 带 query / 敏感复核不过的条目并计数 ──

func TestCatalogExportExcludesPrivateQueryAndSensitive(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	seedCandidate(t, db, newRSSCandidate("rss:pub1", "Public Feed", "https://pub.example.com/feed", "public"))
	seedCandidate(t, db, newRSSCandidate("rss:query1", "Token Feed", "https://pub.example.com/feed?token=abc", "public"))
	seedCandidate(t, db, newRSSCandidate("rss:priv1", "Private Feed", "https://pub.example.com/private/feed", "private_allowed"))
	// 敏感复核目标：scope 标错 public 的私网 IP 字面量与 userinfo 地址，凭据/私网永不落文件。
	seedCandidate(t, db, newRSSCandidate("rss:ip1", "LAN Feed", "https://10.0.0.5/feed", "public"))
	seedCandidate(t, db, newRSSCandidate("rss:cred1", "Cred Feed", "https://user:secret@pub.example.com/feed", "public"))

	result, err := svc.ExportCatalog(context.Background(), CatalogExportOptions{})
	require.NoError(t, err)

	require.Len(t, result.Export.Entries, 1)
	require.Equal(t, "rss:pub1", result.Export.Entries[0].StableKey)
	require.Equal(t, "Public Feed", result.Export.Entries[0].Name)
	require.Equal(t, "https://pub.example.com/feed", result.Export.Entries[0].FeedURL)
	require.Equal(t, CatalogTransferFormat, result.Export.Format)
	require.Equal(t, 1, result.Export.Version)

	require.Equal(t, 1, result.ExcludedPrivate)   // private_allowed scope
	require.Equal(t, 1, result.ExcludedQuery)     // 带 query（可能携带 token，保守排除）
	require.Equal(t, 2, result.ExcludedSensitive) // 私网 IP 字面量 + userinfo 复核排除

	// 文件内容不含任何排除条目的敏感信息。
	raw, err := json.Marshal(result.Export)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "10.0.0.5")
	require.NotContains(t, string(raw), "user:secret")
	require.NotContains(t, string(raw), "token=abc")
}

// ── S12-1 导出文件形状：不含订阅状态/向量/数据库 ID/访问授权字段 ──

func TestCatalogExportJSONShapeOmitsSensitiveFields(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	seedCandidate(t, db, newRSSCandidate("rss:shape", "Shape Feed", "https://shape.example.com/feed", "public"))
	route := models.RSSHubRoute{Namespace: "Test", Path: "Shape", Name: "Shape Route"}
	require.NoError(t, db.Create(&route).Error)
	enabled := false
	seedCandidate(t, db, models.FeedCandidate{
		StableKey:             "test/shape",
		Kind:                  "rsshub",
		RouteID:               &route.ID,
		ManualMetadata:        models.MetadataMap{ManualFieldDescription: "人工说明"},
		RecommendationEnabled: &enabled,
		AccessScope:           "public",
		Revision:              1,
	})

	result, err := svc.ExportCatalog(context.Background(), CatalogExportOptions{})
	require.NoError(t, err)
	require.Len(t, result.Export.Entries, 2)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(mustMarshal(t, result.Export), &doc))
	require.ElementsMatch(t, []string{"format", "version", "entries"}, keysOf(doc))

	for _, rawEntry := range result.Export.Entries {
		m := mustMarshal(t, rawEntry)
		var em map[string]any
		require.NoError(t, json.Unmarshal(m, &em))
		// 订阅状态、向量、数据库 ID、检查结果、访问授权、revision 一律不得出现。
		for _, banned := range []string{"id", "route_id", "subscribed", "embedding", "access_scope", "revision", "check", "created_at", "updated_at"} {
			require.NotContains(t, em, banned, "entry %s must not carry field %q", rawEntry.StableKey, banned)
		}
	}

	// rsshub 条目携带上游 ns/path 与人工说明，不携带 feed_url。
	rsshubEntry := result.Export.Entries[0] // stable_key 排序：rss:shape < test/shape? 比较 "rss:shape" vs "test/shape"：'r'<'t'，rss 在前
	require.Equal(t, "rss:shape", result.Export.Entries[0].StableKey)
	rsshubEntry = result.Export.Entries[1]
	require.Equal(t, "test/shape", rsshubEntry.StableKey)
	require.Equal(t, "Test", rsshubEntry.RouteNamespace)
	require.Equal(t, "Shape", rsshubEntry.RoutePath)
	require.Equal(t, "", rsshubEntry.FeedURL)
	require.Equal(t, "人工说明", rsshubEntry.ManualMetadata[ManualFieldDescription])
	require.NotNil(t, rsshubEntry.RecommendationEnabled)
	require.False(t, *rsshubEntry.RecommendationEnabled)
}

// ── 导出选项：第一版不支持连私有导出（design D8）──

func TestCatalogExportRejectsIncludePrivate(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	_, err := svc.ExportCatalog(context.Background(), CatalogExportOptions{IncludePrivate: true})
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeValidation, ce.Code)
	require.Contains(t, ce.Message, "not supported in v1")
}

// ── S11-3 文件级拒绝：未知 version/format、非法 kind、超限 → 整份拒绝零写入 ──

func TestParseCatalogImportRejectsFileLevel(t *testing.T) {
	okEntry := CatalogExportEntry{Kind: "rss", Name: "ok", FeedURL: "https://ok.example.com/feed"}
	okEntry.StableKey = stableKeyForRSS(t, okEntry.FeedURL)

	cases := []struct {
		name string
		data []byte
	}{
		{"unknown version v99", mustMarshal(t, map[string]any{"format": CatalogTransferFormat, "version": 99, "entries": []any{}})},
		{"unknown format", mustMarshal(t, map[string]any{"format": "some-other-catalog", "version": 1, "entries": []any{}})},
		{"not json", []byte("{{not-json")},
		{"illegal kind rejects whole file", mustMarshal(t, map[string]any{
			"format": CatalogTransferFormat, "version": 1,
			"entries": []any{
				map[string]any{"stable_key": "a/b", "kind": "rsshub", "route_namespace": "a", "route_path": "b"},
				map[string]any{"stable_key": "x", "kind": "atom", "name": "x", "feed_url": "https://x.example.com/feed"},
			},
		})},
		{"oversize file", append([]byte(`{"format":"`+CatalogTransferFormat+`","version":1,"pad":"`), strings.Repeat("a", int(CatalogImportMaxBytes))...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCatalogImport(tc.data)
			require.Error(t, err)
			var ce *CandidateError
			require.ErrorAs(t, err, &ce)
			require.Equal(t, CandidateErrorCodeValidation, ce.Code)
		})
	}

	// 超 1000 条整份拒绝。
	entries := make([]CatalogExportEntry, CatalogImportMaxEntries+1)
	for i := range entries {
		entries[i] = CatalogExportEntry{
			StableKey: fmt.Sprintf("bulk/%d", i), Kind: "rsshub",
			RouteNamespace: "bulk", RoutePath: fmt.Sprintf("%d", i),
		}
	}
	_, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeding the 1000 limit")

	// 合法文件正常解析（对照：不误伤）。
	parsed, err := ParseCatalogImport(marshalImportFile(t, []CatalogExportEntry{okEntry}))
	require.NoError(t, err)
	require.Len(t, parsed.Valid, 1)
	require.Empty(t, parsed.Invalid)
}

// ── S11-1 预览混合配置：新/重复/冲突/无效四分类 ──

func TestPreviewCatalogImportMixedClassification(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	// 本地已有：A（重复目标）、B（名称冲突目标）。
	seedCandidate(t, db, newRSSCandidate(stableKeyForRSS(t, "https://a.example.com/feed"), "A", "https://a.example.com/feed", "public"))
	seedCandidate(t, db, newRSSCandidate(stableKeyForRSS(t, "https://b.example.com/feed"), "本地名称B", "https://b.example.com/feed", "public"))

	entries := []CatalogExportEntry{
		rssEntryWithKey(t, "A", "https://a.example.com/feed"),                               // duplicate
		rssEntryWithKey(t, "导入名称B", "https://b.example.com/feed"),                           // conflict（名称不一致）
		rssEntryWithKey(t, "C", "https://c.example.com/feed"),                               // new
		{Kind: "rss", Name: "坏地址", FeedURL: "ftp://bad.example.com/feed"},                   // invalid（scheme，无 stable_key → 预览以 #3 占位）
		{StableKey: "new/route", Kind: "rsshub", RouteNamespace: "new", RoutePath: "route"}, // new（未解析 rsshub）
	}
	parsed, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)
	require.Len(t, parsed.Valid, 4)
	require.Len(t, parsed.Invalid, 1)

	preview, err := svc.PreviewCatalogImport(context.Background(), parsed)
	require.NoError(t, err)
	require.Equal(t, CatalogImportCounts{New: 2, Duplicate: 1, Conflict: 1, Invalid: 1}, preview.Counts)
	require.Len(t, preview.Items, 5)

	byKey := map[string]CatalogPreviewItem{}
	for _, it := range preview.Items {
		byKey[it.StableKey] = it
	}
	require.Equal(t, CatalogItemDuplicate, byKey[stableKeyForRSS(t, "https://a.example.com/feed")].Classification)
	require.Equal(t, CatalogItemConflict, byKey[stableKeyForRSS(t, "https://b.example.com/feed")].Classification)
	require.Contains(t, byKey[stableKeyForRSS(t, "https://b.example.com/feed")].Reason, "differs from the local effective name")
	require.Equal(t, CatalogItemNew, byKey[stableKeyForRSS(t, "https://c.example.com/feed")].Classification)
	require.Equal(t, CatalogItemNew, byKey["new/route"].Classification)
	require.Equal(t, CatalogItemInvalid, byKey["#3"].Classification)
	require.NotEmpty(t, byKey["#3"].Reason)

	// 指纹：64 位 hex、确定性（同份重复预览一致）。
	require.Len(t, preview.Fingerprint, 64)
	again, err := svc.PreviewCatalogImport(context.Background(), parsed)
	require.NoError(t, err)
	require.Equal(t, preview.Fingerprint, again.Fingerprint)
	require.Greater(t, preview.LocalRevision, uint64(0))
}

// ── S11-1 确认后只应用 new、订阅零变化（feeds 表无行）、导入零网络（结构性：无 HTTP 路径）──

func TestApplyCatalogImportAppliesOnlyNewNoSubscription(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	seedCandidate(t, db, newRSSCandidate(stableKeyForRSS(t, "https://a.example.com/feed"), "A", "https://a.example.com/feed", "public"))
	seedCandidate(t, db, newRSSCandidate(stableKeyForRSS(t, "https://b.example.com/feed"), "本地名称B", "https://b.example.com/feed", "public"))

	entries := []CatalogExportEntry{
		rssEntryWithKey(t, "A", "https://a.example.com/feed"),
		rssEntryWithKey(t, "导入名称B", "https://b.example.com/feed"),
		rssEntryWithKey(t, "C", "https://c.example.com/feed"),
		{Kind: "rss", Name: "坏地址", FeedURL: "ftp://bad.example.com/feed"},
	}
	parsed, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)
	preview, err := svc.PreviewCatalogImport(context.Background(), parsed)
	require.NoError(t, err)

	result, err := svc.ApplyCatalogImport(context.Background(), parsed, preview.Fingerprint, preview.LocalRevision)
	require.NoError(t, err)

	require.Equal(t, []string{stableKeyForRSS(t, "https://c.example.com/feed")}, result.Applied)
	require.Equal(t, []string{stableKeyForRSS(t, "https://a.example.com/feed")}, result.SkippedDuplicate)
	require.Len(t, result.SkippedConflict, 1)
	require.Equal(t, stableKeyForRSS(t, "https://b.example.com/feed"), result.SkippedConflict[0].StableKey)
	require.Empty(t, result.Failed)
	require.Len(t, result.Invalid, 1) // 解析阶段 invalid 回显

	// 库内：总 3 条（原 2 + 新 1）；冲突条目本地值保留；feeds 表零行（导入 ≠ 订阅）。
	var total, feeds int64
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&total).Error)
	require.NoError(t, db.Model(&models.Feed{}).Count(&feeds).Error)
	require.EqualValues(t, 3, total)
	require.EqualValues(t, 0, feeds)

	var stored models.FeedCandidate
	require.NoError(t, db.Where("stable_key = ?", stableKeyForRSS(t, "https://b.example.com/feed")).First(&stored).Error)
	require.Equal(t, "本地名称B", stored.ManualMetadata[ManualFieldName]) // 冲突不静默覆盖人工资料
	// 新变量查询：复用带主键的 dest 会让 GORM 拼上 AND id=<旧值> 误报 not found。
	var storedC models.FeedCandidate
	require.NoError(t, db.Where("stable_key = ?", stableKeyForRSS(t, "https://c.example.com/feed")).First(&storedC).Error)
	require.Equal(t, "public", storedC.AccessScope)
	require.EqualValues(t, 1, storedC.Revision)
}

// ── S11-2 部分写入失败后重试：只处理未成功项、无重复记录 ──
// sqlite 难以物理注入部分失败，采用 GORM Create 回调对指定 stable_key 注入错误
// （等价于单条事务失败，逐条独立小事务语义被真实执行）。

func TestApplyCatalogImportPartialFailureAndRetry(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	failKey := stableKeyForRSS(t, "https://fail.example.com/feed")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:injected_fail", func(tx *gorm.DB) {
		if c, ok := tx.Statement.Dest.(*models.FeedCandidate); ok && c != nil && c.StableKey == failKey {
			_ = tx.AddError(errors.New("injected write failure"))
		}
	}))

	entries := []CatalogExportEntry{
		rssEntryWithKey(t, "One", "https://one.example.com/feed"),
		rssEntryWithKey(t, "Two", "https://two.example.com/feed"),
		rssEntryWithKey(t, "Fail", "https://fail.example.com/feed"),
	}
	parsed, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)

	preview, err := svc.PreviewCatalogImport(context.Background(), parsed)
	require.NoError(t, err)
	result, err := svc.ApplyCatalogImport(context.Background(), parsed, preview.Fingerprint, preview.LocalRevision)
	require.NoError(t, err)
	require.Len(t, result.Applied, 2)
	require.Len(t, result.Failed, 1)
	require.Equal(t, failKey, result.Failed[0].StableKey)
	require.Contains(t, result.Failed[0].Reason, "injected write failure")

	// 重试：重新预览（新指纹/revision）→ 已成功的 2 条 duplicate，失败条目重新应用。
	parsed2, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)
	preview2, err := svc.PreviewCatalogImport(context.Background(), parsed2)
	require.NoError(t, err)
	require.Equal(t, CatalogImportCounts{New: 1, Duplicate: 2}, preview2.Counts)

	db.Callback().Create().Remove("test:injected_fail") // 故障修复
	result2, err := svc.ApplyCatalogImport(context.Background(), parsed2, preview2.Fingerprint, preview2.LocalRevision)
	require.NoError(t, err)
	require.Equal(t, []string{failKey}, result2.Applied)
	require.Len(t, result2.SkippedDuplicate, 2)

	var total int64
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&total).Error)
	require.EqualValues(t, 3, total) // 重试不产生重复记录
}

// ── S11-2 幂等：同份重复确认（重新预览后）第二次全 duplicate ──

func TestApplyCatalogImportIdempotentRetry(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	entries := []CatalogExportEntry{
		rssEntryWithKey(t, "One", "https://one.example.com/feed"),
		rssEntryWithKey(t, "Two", "https://two.example.com/feed"),
	}
	parsed, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)
	preview, err := svc.PreviewCatalogImport(context.Background(), parsed)
	require.NoError(t, err)
	result, err := svc.ApplyCatalogImport(context.Background(), parsed, preview.Fingerprint, preview.LocalRevision)
	require.NoError(t, err)
	require.Len(t, result.Applied, 2)

	// 同份重复 apply：先重新预览（指纹/revision 已变），再确认 → 全 duplicate、零新增。
	parsed2, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)
	preview2, err := svc.PreviewCatalogImport(context.Background(), parsed2)
	require.NoError(t, err)
	require.Equal(t, CatalogImportCounts{New: 0, Duplicate: 2}, preview2.Counts)
	result2, err := svc.ApplyCatalogImport(context.Background(), parsed2, preview2.Fingerprint, preview2.LocalRevision)
	require.NoError(t, err)
	require.Empty(t, result2.Applied)
	require.Len(t, result2.SkippedDuplicate, 2)

	var total int64
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&total).Error)
	require.EqualValues(t, 2, total)
}

// ── S11-4 确认期间目录被改：指纹不符 / revision 变化 → StalePreviewError 零写入 ──

func TestApplyCatalogImportFingerprintMismatch(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	// 先种一行：空库的 localCatalogRevision=0 恰为「跳过 revision 校验」哨兵，
	// 种子行使 revision 检验路径真正生效。
	seedCandidate(t, db, newRSSCandidate("rss:seed", "种子", "https://seed.example.com/feed", "public"))

	entries := []CatalogExportEntry{rssEntryWithKey(t, "One", "https://one.example.com/feed")}
	parsed, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)
	preview, err := svc.PreviewCatalogImport(context.Background(), parsed)
	require.NoError(t, err)
	require.Greater(t, preview.LocalRevision, uint64(0))

	// 指纹不符（伪造指纹）→ StalePreviewError 且零写入。
	_, err = svc.ApplyCatalogImport(context.Background(), parsed, "deadbeef", preview.LocalRevision)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStalePreview)
	var total int64
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&total).Error)
	require.EqualValues(t, 1, total) // 只有种子行

	// revision 变化（预览后本地被改）→ StalePreviewError。
	unrelated := newRSSCandidate("rss:unrelated", "无关", "https://unrelated.example.com/feed", "public")
	require.NoError(t, db.Create(&unrelated).Error)
	_, err = svc.ApplyCatalogImport(context.Background(), parsed, preview.Fingerprint, preview.LocalRevision)
	require.ErrorIs(t, err, ErrStalePreview)
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&total).Error)
	require.EqualValues(t, 2, total) // 种子 + 无关行，目标条目零写入

	// localRevision=0 降级路径：跳过 revision 校验，指纹仍兜底（无关行不影响分类，可应用）。
	result, err := svc.ApplyCatalogImport(context.Background(), parsed, preview.Fingerprint, 0)
	require.NoError(t, err)
	require.Len(t, result.Applied, 1)
}

// ── S11 白盒：字段限长逐条 invalid 不炸整份、文件内重复键、stable_key 不匹配、rsshub 携带 feed_url ──

func TestParseCatalogImportFieldLimitsAndEntryLevelInvalid(t *testing.T) {
	longName := strings.Repeat("名", 201)                              // name ≤200 runes
	longDesc := strings.Repeat("说", 4001)                             // description ≤4000 runes
	longURL := "https://long.example.com/" + strings.Repeat("a", 500) // >500 runes

	entries := []CatalogExportEntry{
		{Kind: "rss", Name: "好条目", FeedURL: "https://ok1.example.com/feed"},
		{Kind: "rss", Name: longName, FeedURL: "https://ok2.example.com/feed"},
		{Kind: "rss", Name: "超长说明", FeedURL: "https://ok3.example.com/feed", Description: longDesc},
		{Kind: "rss", Name: "超长地址", FeedURL: longURL},
		{StableKey: "mismatch/route", Kind: "rsshub", RouteNamespace: "mismatch", RoutePath: "other"},
		{StableKey: "withurl/route", Kind: "rsshub", RouteNamespace: "withurl", RoutePath: "route", FeedURL: "https://withurl.example.com/feed"},
		{Kind: "rss", Name: "重复", FeedURL: "https://dup.example.com/feed"},
		{Kind: "rss", Name: "重复", FeedURL: "https://dup.example.com/feed"}, // 同文件重复键
	}

	parsed, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)         // 文件级合法：单条问题不炸整份
	require.Len(t, parsed.Valid, 2) // 好条目 + 首个重复条目
	require.Len(t, parsed.Invalid, 6)

	reasons := map[string]string{}
	for _, inv := range parsed.Invalid {
		reasons[inv.StableKey] = inv.Reason
	}
	// 无显式 stable_key 的 invalid 条目以 #序号 占位（importStableKeyOrIndex）。
	require.Contains(t, reasons["#1"], "exceeds 200 runes")
	require.Contains(t, reasons["#2"], "exceeds 4000 runes")
	require.Contains(t, reasons["#3"], "exceeds 500 runes")
	require.Contains(t, reasons["mismatch/route"], "does not match route namespace/path")
	require.Contains(t, reasons["withurl/route"], "must not carry feed_url")
	require.Contains(t, reasons["#7"], "duplicate stable_key")
	// 文件内重复键的 invalid 条目与首个有效条目同键：占位 #7 仍指向文件序号，可定位。
}

// ── S12-2 导入内网/凭据地址：落 private_pending，保存目录不代表授权访问 ──

func TestApplyCatalogImportPrivatePending(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	entries := []CatalogExportEntry{
		rssEntryWithKey(t, "内网IP", "https://192.168.1.20/feed"), // 内网 IP 字面量（NormalizeRSSURL 放行，导入侧标 private）
		// userinfo 条目不能经 BuildRSSStableKey（NormalizeRSSURL 拒绝 userinfo）：
		// 留空 stable_key，由服务端按保留 userinfo 的规范化地址推导（parseImportRSSURL）。
		{Kind: "rss", Name: "带凭据", FeedURL: "https://user:secret@example.com/feed"},
		rssEntryWithKey(t, "带query公网", "https://q.example.com/feed?token=abc"), // 公网带 query：导入合法（导出才排除）
	}

	parsed, err := ParseCatalogImport(marshalImportFile(t, entries))
	require.NoError(t, err)
	require.Len(t, parsed.Valid, 3)
	require.Empty(t, parsed.Invalid)

	preview, err := svc.PreviewCatalogImport(context.Background(), parsed)
	require.NoError(t, err)
	result, err := svc.ApplyCatalogImport(context.Background(), parsed, preview.Fingerprint, preview.LocalRevision)
	require.NoError(t, err)
	require.Len(t, result.Applied, 3)
	require.Empty(t, result.Failed)

	scopes := map[string]string{}
	var rows []models.FeedCandidate
	require.NoError(t, db.Find(&rows).Error)
	for _, r := range rows {
		name, _ := r.ManualMetadata[ManualFieldName].(string)
		scopes[name] = r.AccessScope
	}
	require.Equal(t, "private_pending", scopes["内网IP"])
	require.Equal(t, "private_pending", scopes["带凭据"])
	require.Equal(t, "public", scopes["带query公网"]) // query 不是私网信号，导入侧合法

	// 这些条目后续导出必被排除：私网/凭据条目已落 private_pending 走 ExcludedPrivate 桶
	//（scope 检查先于敏感复核），带 query 条目走 ExcludedQuery 桶。
	exported, err := svc.ExportCatalog(context.Background(), CatalogExportOptions{})
	require.NoError(t, err)
	require.Empty(t, exported.Export.Entries)
	require.Equal(t, 2, exported.ExcludedPrivate)
	require.Equal(t, 1, exported.ExcludedQuery)
	require.Equal(t, 0, exported.ExcludedSensitive)
	rawExport, err := json.Marshal(exported.Export)
	require.NoError(t, err)
	require.NotContains(t, string(rawExport), "user:secret")
	require.NotContains(t, string(rawExport), "192.168.1.20")
}

// ── 辅助 ──

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
