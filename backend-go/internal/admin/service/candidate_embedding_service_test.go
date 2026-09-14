package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 候选有效介绍向量回补（improve-discovery-recommendations 4.6，design D6）──
//
// sqlite 内存库只迁移候选/路由/向量/设置四表；生成函数注入固定向量以断言
// 「未变复用、变了才重嵌、旧请求作废、失败保留旧向量、每批限 20」。

func setupCandidateEmbeddingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.FeedCandidate{}, &models.RSSHubRoute{}, &models.CandidateEmbedding{}, &models.AISettings{},
	))
	return db
}

// deterministicEmbedder 返回可计数的固定向量生成函数：每条调用记一次，向量按序号区分。
func deterministicEmbedder(calls *[]string) CandidateEmbedderFunc {
	return func(_ context.Context, text string) ([]float64, int, string, error) {
		*calls = append(*calls, text)
		return []float64{1, float64(len(*calls)), 0}, 3, "test-embed", nil
	}
}

// seedEmbeddingCandidate 建 ok 路由 + rsshub 候选（人工名称/说明非空，保证有效文本非空）。
func seedEmbeddingCandidate(t *testing.T, db *gorm.DB, key, name, desc string) *models.FeedCandidate {
	t.Helper()
	route := models.RSSHubRoute{
		Namespace: "ns", Path: "/" + key, Name: name, URL: "https://example.com/" + key,
		Description: desc, Parameters: "{}", Status: "ok",
	}
	require.NoError(t, db.Create(&route).Error)
	enabled := true
	cand := models.FeedCandidate{
		StableKey: "ns/" + key, Kind: "rsshub", RouteID: &route.ID,
		ManualMetadata:        models.MetadataMap{ManualFieldName: name},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	return &cand
}

func loadCandidateEmbedding(t *testing.T, db *gorm.DB, candidateID uint) (models.CandidateEmbedding, bool) {
	t.Helper()
	var row models.CandidateEmbedding
	err := db.Where("candidate_id = ?", candidateID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, false
	}
	require.NoError(t, err)
	return row, true
}

func countCandidateEmbeddings(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.CandidateEmbedding{}).Count(&n).Error)
	return n
}

// ── 文本拼装与清洗（纯逻辑）──

// TestEffectiveDescriptionTextSanitizesFormatting（D6：清除格式噪音但保留正文，提示块标记
// 不得挤掉内容说明）：Markdown 强调/链接/标题/列表/引用、HTML 注释与标签、VitePress 提示块
// 容器行全部剥掉，空白折叠为单空格，正文文字保留。
func TestEffectiveDescriptionTextSanitizesFormatting(t *testing.T) {
	text := EffectiveDescriptionText(
		"**Awesome** Feed",
		"https://example.com",
		"> 分类：科技\n\n::: tip 提示块标记\n:::",
		"# 标题\n\n这是**正文**说明，见 [文档](https://example.com/doc) 与 ![封面](x.png)。\n\n- 列表项一\n- 列表项二\n\n<!-- form: mono -->\n\n<p>HTML 段落</p>",
	)
	require.Equal(t, strings.Join([]string{
		"Awesome Feed",
		"https://example.com",
		"分类：科技",
		"标题 这是正文说明，见 文档 与 封面。 列表项一 列表项二 HTML 段落",
	}, "\n"), text)
	require.NotContains(t, text, "::: tip")
	require.NotContains(t, text, "form: mono")
	require.NotContains(t, text, "**")
	require.NotContains(t, text, "<p>")
}

// TestEffectiveDescriptionTextRuneLimits（D6：名称/网站各 80、分类等合计 80、说明 257，
// 含分隔符总量 ≤500 rune 以适配供应商 512 token 上限，按 rune 不按字节）：中文超长不被
// 切成乱码，各段独立限长。
func TestEffectiveDescriptionTextRuneLimits(t *testing.T) {
	name := strings.Repeat("名", 250)
	website := strings.Repeat("w", 250)
	classification := strings.Repeat("类", 250)
	desc := strings.Repeat("说", 1300)
	parts := strings.Split(EffectiveDescriptionText(name, website, classification, desc), "\n")
	require.Len(t, parts, 4)
	require.Len(t, []rune(parts[0]), candidateEmbeddingNameMaxRunes)
	require.Len(t, []rune(parts[1]), candidateEmbeddingWebsiteMaxRunes)
	require.Len(t, []rune(parts[2]), candidateEmbeddingClassificationMaxRunes)
	require.Len(t, []rune(parts[3]), candidateEmbeddingDescriptionMaxRunes)
	require.Equal(t, strings.Repeat("说", candidateEmbeddingDescriptionMaxRunes), parts[3])
	// 总预算硬约束：各段上限之和 + 3 个换行分隔符不得超过 500 rune（供应商 512 token）。
	require.LessOrEqual(t, candidateEmbeddingNameMaxRunes+candidateEmbeddingWebsiteMaxRunes+
		candidateEmbeddingClassificationMaxRunes+candidateEmbeddingDescriptionMaxRunes+3, 500)
}

// TestEffectiveDescriptionTextEmptyInputs（变体：空串/纯空白/全角空白）：全空返回空串
// （调用方跳过，不为无资料候选编造向量）；部分空只保留非空段。
func TestEffectiveDescriptionTextEmptyInputs(t *testing.T) {
	require.Equal(t, "", EffectiveDescriptionText("", "  ", "\t", "\u3000"))
	require.Equal(t, "只有名称", EffectiveDescriptionText("只有名称", "", "", ""))
}

// ── 增量回补 ──

// TestDirtyCandidateEmbeddingsReusesUnchangedVectors（spec「说明变化才重建」）：首轮生成后
// 未变候选不再生成（fetch 计数不涨），改动人工说明的候选重新生成，其余复用已有向量。
func TestDirtyCandidateEmbeddingsReusesUnchangedVectors(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	var calls []string
	svc := NewCandidateEmbeddingServiceWithEmbedder(db, deterministicEmbedder(&calls))
	ctx := context.Background()

	stable := seedEmbeddingCandidate(t, db, "stable", "稳定源", "原始说明")
	edited := seedEmbeddingCandidate(t, db, "edited", "待改源", "原始说明")

	summary, err := svc.DirtyCandidateEmbeddings(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 2, summary.Generated)
	require.Len(t, calls, 2)
	stableRowBefore, ok := loadCandidateEmbedding(t, db, stable.ID)
	require.True(t, ok)

	// 无变化 → 一条都不重嵌（复用已有向量）。
	calls = nil
	summary, err = svc.DirtyCandidateEmbeddings(ctx, 20)
	require.NoError(t, err)
	require.Zero(t, summary.Generated, "未变候选不得重复生成")
	require.Empty(t, calls)

	// 人工说明变化 → 只有该候选重嵌，且同一 (candidate,model,dimension) 覆盖不新增行。
	require.NoError(t, db.Model(&models.FeedCandidate{}).Where("id = ?", edited.ID).
		Updates(map[string]any{
			"manual_metadata": models.MetadataMap{ManualFieldName: "待改源", ManualFieldDescription: "新的说明"},
			"revision":        gorm.Expr("revision + 1"),
		}).Error)
	summary, err = svc.DirtyCandidateEmbeddings(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Generated)
	require.Len(t, calls, 1)
	require.Contains(t, calls[0], "新的说明")
	require.Equal(t, int64(2), countCandidateEmbeddings(t, db), "同 candidate+model+dimension 覆盖，不新增行")

	stableRowAfter, _ := loadCandidateEmbedding(t, db, stable.ID)
	require.Equal(t, stableRowBefore.TextHash, stableRowAfter.TextHash, "未变候选的向量行必须原样保留")
}

// TestDirtyCandidateEmbeddingsDiscardsResultWhenSourceChanged（D6：写入前复查候选 revision 与
// 指纹，旧请求作废）：生成期间候选资料被改，旧结果不得覆盖新资料，旧向量保持不动。
func TestDirtyCandidateEmbeddingsDiscardsResultWhenSourceChanged(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	ctx := context.Background()
	cand := seedEmbeddingCandidate(t, db, "race", "竞态源", "旧说明")
	// 预置一条「旧向量」行（指纹与当前文本不一致 → dirty）。
	oldText := "旧向量占位"
	oldRow := models.CandidateEmbedding{
		CandidateID: cand.ID, Model: "test-embed", Dimension: 3,
		TextHash: CandidateEmbeddingTextHash("别的文本" + oldText), EmbeddingVec: "[9,9,9]",
	}
	require.NoError(t, db.Create(&oldRow).Error)

	embedder := func(_ context.Context, _ string) ([]float64, int, string, error) {
		// 模拟生成期间用户改了资料（revision 与有效文本同时变化）。
		if err := db.Model(&models.FeedCandidate{}).Where("id = ?", cand.ID).
			Updates(map[string]any{
				"manual_metadata": models.MetadataMap{ManualFieldName: "竞态源", ManualFieldDescription: "生成期间改成的新说明"},
				"revision":        gorm.Expr("revision + 1"),
			}).Error; err != nil {
			return nil, 0, "", err
		}
		return []float64{1, 1, 1}, 3, "test-embed", nil
	}
	svc := NewCandidateEmbeddingServiceWithEmbedder(db, embedder)

	summary, err := svc.DirtyCandidateEmbeddings(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Stale, "revision/指纹变化 → 本次结果作废")
	require.Zero(t, summary.Generated)

	row, ok := loadCandidateEmbedding(t, db, cand.ID)
	require.True(t, ok)
	require.Equal(t, oldRow.TextHash, row.TextHash, "旧向量行不得被旧请求覆盖")
	require.Equal(t, "[9,9,9]", row.EmbeddingVec)
}

// TestDirtyCandidateEmbeddingsKeepsPreviousVectorOnFailure（spec「更新失败与模型切换」）：
// 生成失败保留兼容旧向量、候选保持 dirty；恢复后下一批重试成功并覆盖。
func TestDirtyCandidateEmbeddingsKeepsPreviousVectorOnFailure(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	ctx := context.Background()
	cand := seedEmbeddingCandidate(t, db, "flaky", "失败源", "原始说明")
	require.NoError(t, db.Create(&models.CandidateEmbedding{
		CandidateID: cand.ID, Model: "test-embed", Dimension: 3,
		TextHash: "stale-hash", EmbeddingVec: "[7,7,7]",
	}).Error)

	failing := true
	svc := NewCandidateEmbeddingServiceWithEmbedder(db, func(_ context.Context, _ string) ([]float64, int, string, error) {
		if failing {
			return nil, 0, "", errors.New("provider timeout")
		}
		return []float64{2, 2, 2}, 3, "test-embed", nil
	})

	summary, err := svc.DirtyCandidateEmbeddings(ctx, 20)
	require.NoError(t, err, "单条生成失败不得中断整批")
	require.Equal(t, 1, summary.Failed)
	row, _ := loadCandidateEmbedding(t, db, cand.ID)
	require.Equal(t, "stale-hash", row.TextHash)
	require.Equal(t, "[7,7,7]", row.EmbeddingVec, "失败保留旧向量")

	// 恢复后重试：指纹仍不匹配 → 下一批自动重嵌。
	failing = false
	summary, err = svc.DirtyCandidateEmbeddings(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Generated)
	row, _ = loadCandidateEmbedding(t, db, cand.ID)
	require.NotEqual(t, "stale-hash", row.TextHash)
	require.Equal(t, "[2,2,2]", row.EmbeddingVec)
}

// TestDirtyCandidateEmbeddingsBatchLimit（D9：初始全量回补限批默认 20）：25 条待回补候选
// 分两批处理（20 + 5），单批不得超限。
func TestDirtyCandidateEmbeddingsBatchLimit(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	var calls []string
	svc := NewCandidateEmbeddingServiceWithEmbedder(db, deterministicEmbedder(&calls))
	ctx := context.Background()
	for i := 0; i < 25; i++ {
		seedEmbeddingCandidate(t, db, fmt.Sprintf("b%02d", i), fmt.Sprintf("批量源%02d", i), "说明")
	}

	first, err := svc.DirtyCandidateEmbeddings(ctx, 0) // 0 → 默认 20
	require.NoError(t, err)
	require.Equal(t, CandidateEmbeddingBatchSizeDefault, first.Generated)
	require.Len(t, calls, CandidateEmbeddingBatchSizeDefault)

	calls = nil
	second, err := svc.DirtyCandidateEmbeddings(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, 5, second.Generated)
	require.Equal(t, int64(25), countCandidateEmbeddings(t, db))
}

// TestDirtyCandidateEmbeddingsSkipsGoneRoutes：已从上游目录消失（gone）的路由不再消耗
// embedding 额度（不生成向量行）。
func TestDirtyCandidateEmbeddingsSkipsGoneRoutes(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	var calls []string
	svc := NewCandidateEmbeddingServiceWithEmbedder(db, deterministicEmbedder(&calls))
	cand := seedEmbeddingCandidate(t, db, "gone", "消失源", "说明")
	require.NoError(t, db.Model(&models.RSSHubRoute{}).Where("id = ?", *cand.RouteID).
		Update("status", "gone").Error)

	summary, err := svc.DirtyCandidateEmbeddings(context.Background(), 20)
	require.NoError(t, err)
	require.Zero(t, summary.Generated)
	require.Empty(t, calls)
}

// TestDirtyCandidateEmbeddingsRejectedWhenV2Disabled（4.6 开关）：关闭 discovery_v2 后回补
// 入口直接返回 configuration 错误，不生成任何向量。
func TestDirtyCandidateEmbeddingsRejectedWhenV2Disabled(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	require.NoError(t, SaveDiscoveryV2Enabled(db, false))
	var calls []string
	svc := NewCandidateEmbeddingServiceWithEmbedder(db, deterministicEmbedder(&calls))
	seedEmbeddingCandidate(t, db, "off", "停用源", "说明")

	_, err := svc.DirtyCandidateEmbeddings(context.Background(), 20)
	require.Error(t, err)
	var ce *CandidateError
	require.True(t, errors.As(err, &ce))
	require.Equal(t, CandidateErrorCodeConfiguration, ce.Code)
	require.Empty(t, calls)
}

// TestLoadDiscoveryV2EnabledDefaults（开关读取语义）：缺省/空值/键缺失/非法值 → 启用；
// 裸布尔与 {"enabled":false} 均能关闭。
func TestLoadDiscoveryV2EnabledDefaults(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	require.True(t, LoadDiscoveryV2Enabled(db), "键缺失 → 默认启用")

	require.NoError(t, db.Create(&models.AISettings{Key: discoveryV2ConfigKey, Value: ""}).Error)
	require.True(t, LoadDiscoveryV2Enabled(db), "空值 → 默认启用")

	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", discoveryV2ConfigKey).
		Update("value", "not-json").Error)
	require.True(t, LoadDiscoveryV2Enabled(db), "非法值 → fail-open 启用")

	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", discoveryV2ConfigKey).
		Update("value", "false").Error)
	require.False(t, LoadDiscoveryV2Enabled(db), "裸布尔 false")

	require.NoError(t, SaveDiscoveryV2Enabled(db, false))
	require.False(t, LoadDiscoveryV2Enabled(db))
	require.NoError(t, SaveDiscoveryV2Enabled(db, true))
	require.True(t, LoadDiscoveryV2Enabled(db))
}

// TestMarkStaleRunningDiscoveryRuns（4.6 维护 job 的服务层）：超阈值 running 置 failed
// （error_code=stale_running + finished_at），未超阈值与已终态的行不受影响；重跑幂等。
func TestMarkStaleRunningDiscoveryRuns(t *testing.T) {
	db := setupCandidateEmbeddingTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.DiscoveryRun{}))
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	stale := models.DiscoveryRun{RequestKey: "rk-stale", Kind: "ask", Status: "running", StartedAt: now.Add(-2 * time.Hour)}
	fresh := models.DiscoveryRun{RequestKey: "rk-fresh", Kind: "refresh", Status: "running", StartedAt: now.Add(-time.Minute)}
	finished := now.Add(-3 * time.Hour)
	done := models.DiscoveryRun{RequestKey: "rk-done", Kind: "ask", Status: "succeeded", StartedAt: now.Add(-4 * time.Hour), FinishedAt: &finished}
	for _, run := range []*models.DiscoveryRun{&stale, &fresh, &done} {
		require.NoError(t, db.Create(run).Error)
	}

	marked, err := MarkStaleRunningDiscoveryRuns(ctx, db, now, 0) // 0 → 默认 1 小时
	require.NoError(t, err)
	require.EqualValues(t, 1, marked)

	var gotStale models.DiscoveryRun
	require.NoError(t, db.First(&gotStale, stale.ID).Error)
	require.Equal(t, DiscoveryRunStatusFailed, gotStale.Status)
	require.Equal(t, DiscoveryRunErrorStaleRunning, gotStale.ErrorCode)
	require.NotNil(t, gotStale.FinishedAt)

	var gotFresh models.DiscoveryRun
	require.NoError(t, db.First(&gotFresh, fresh.ID).Error)
	require.Equal(t, DiscoveryRunStatusRunning, gotFresh.Status, "未超阈值的运行不得误杀")

	var gotDone models.DiscoveryRun
	require.NoError(t, db.First(&gotDone, done.ID).Error)
	require.Equal(t, "succeeded", gotDone.Status)
	require.Empty(t, gotDone.ErrorCode)

	marked, err = MarkStaleRunningDiscoveryRuns(ctx, db, now, 0)
	require.NoError(t, err)
	require.EqualValues(t, 0, marked, "重跑幂等：已 failed 的行不再匹配")
}
