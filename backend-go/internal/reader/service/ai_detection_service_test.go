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
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
)

// ============================================================================
// 判定服务包级测试（add-ai-generated-article-detection task 2.2 / 2.3 / 2.4）
// ============================================================================

// fakeAIDetectionRouter 是判定服务的 LLM 出口假实现：记录请求与本地失败审计。
type fakeAIDetectionRouter struct {
	content  string
	err      error
	requests []airouter.ChatRequest
	failures []airouter.LocalFailure
}

func (f *fakeAIDetectionRouter) Chat(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return &airouter.ChatResult{
		Content:      f.content,
		RouteName:    "default",
		ProviderName: "fake-vendor",
		ModelName:    "fake-model",
	}, nil
}

func (f *fakeAIDetectionRouter) LogLocalFailure(_ context.Context, req airouter.ChatRequest, failure airouter.LocalFailure) {
	f.failures = append(f.failures, failure)
}

// fakeAIDetectionProviderClient 是 airouter.ProviderClient 的假实现（审计测试走真实
// Router，只把 provider HTTP 层换掉）。
type fakeAIDetectionProviderClient struct {
	content string
	err     error
}

func (f *fakeAIDetectionProviderClient) Chat(_ context.Context, _ models.AIVendor, _ models.AIModel, _ string, _ airouter.ChatRequest) (*airouter.ChatResponse, []airouter.StructuredAttempt, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return &airouter.ChatResponse{Content: f.content}, nil, nil
}

func (f *fakeAIDetectionProviderClient) Embed(_ context.Context, _ models.AIVendor, _ models.AIModel, _ airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
	return nil, errors.New("embed not used")
}

// setupAIDetectionTestDB 装一份 SQLite 测试库（含判定用到的 ai_* 与 articles 表）。
func setupAIDetectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	database.DB = db
	repository.InitRepository(db)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{}, &models.Article{}, &models.TagJob{}, &models.DetectionJob{},
		&models.AISettings{}, &models.AIRoute{}, &models.AIVendor{}, &models.AIModel{},
		&models.AIRouteModel{}, &models.AICallLog{},
	))
	return db
}

// staticAIDetectionConfig 返回一个固定判定参数的 provider（绕开 DB）。
func staticAIDetectionConfig(threshold float64, head, tail int) AIDetectionConfigProvider {
	return func() (aisettings.AIDetectionConfig, error) {
		return aisettings.AIDetectionConfig{Threshold: threshold, SampleHead: head, SampleTail: tail}, nil
	}
}

// TestSelectAIDetectionInput 覆盖判定输入的四条取用路径（design D3）：
// 全文 → RSS 正文 → 订阅源摘要 → 全空。
func TestSelectAIDetectionInput(t *testing.T) {
	cases := []struct {
		name       string
		article    models.Article
		wantText   string
		wantSource string
	}{
		{
			name:       "有全文取全文",
			article:    models.Article{FirecrawlContent: "全文正文", Content: "RSS 正文", Description: "摘要"},
			wantText:   "全文正文",
			wantSource: AIDetectionInputFirecrawl,
		},
		{
			name:       "无全文回退 RSS 正文",
			article:    models.Article{FirecrawlContent: "   ", Content: "RSS 正文", Description: "摘要"},
			wantText:   "RSS 正文",
			wantSource: AIDetectionInputContent,
		},
		{
			name:       "正文也为空回退摘要",
			article:    models.Article{Content: "", Description: "订阅源摘要"},
			wantText:   "订阅源摘要",
			wantSource: AIDetectionInputDescription,
		},
		{
			name:       "全空无判定输入",
			article:    models.Article{},
			wantText:   "",
			wantSource: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, source := SelectAIDetectionInput(&tc.article)
			require.Equal(t, tc.wantText, text)
			require.Equal(t, tc.wantSource, source)
		})
	}
}

// TestBuildAIDetectionSample 覆盖长文本截断（首 3000 runes + 尾 2000 runes，按 rune 切）。
func TestBuildAIDetectionSample(t *testing.T) {
	long := strings.Repeat("甲", 8000)
	sample := BuildAIDetectionSample(long, 3000, 2000)
	require.Equal(t, 5000, len([]rune(sample)))
	require.Equal(t, strings.Repeat("甲", 3000), string([]rune(sample)[:3000]))
	require.Equal(t, strings.Repeat("甲", 2000), string([]rune(sample)[3000:]))

	short := strings.Repeat("乙", 100)
	require.Equal(t, short, BuildAIDetectionSample(short, 3000, 2000))
}

// TestParseAIDetectionScore 覆盖置信分解析的容错口径。
func TestParseAIDetectionScore(t *testing.T) {
	cases := []struct {
		content string
		want    float64
		wantOK  bool
	}{
		{content: "0.86", want: 0.86, wantOK: true},
		{content: "  0.42\n", want: 0.42, wantOK: true},
		{content: "置信分：0.86", want: 0.86, wantOK: true},
		{content: "0", want: 0, wantOK: true},
		{content: "1", want: 1, wantOK: true},
		{content: "1.5", wantOK: false},
		{content: "这不是一个分数", wantOK: false},
		{content: "", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.content, func(t *testing.T) {
			score, ok := ParseAIDetectionScore(tc.content)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.InDelta(t, tc.want, score, 1e-9)
			}
		})
	}
}

// TestDetectArticleThresholdBoundaries 覆盖阈值边界：达到阈值即疑似，低于阈值不标记。
func TestDetectArticleThresholdBoundaries(t *testing.T) {
	cases := []struct {
		name       string
		score      string
		threshold  float64
		wantStatus string
	}{
		{name: "超阈值标记疑似", score: "0.86", threshold: 0.7, wantStatus: AIDetectionStatusSuspect},
		{name: "低于阈值不标记", score: "0.42", threshold: 0.7, wantStatus: AIDetectionStatusNotAI},
		{name: "等于阈值即疑似", score: "0.7", threshold: 0.7, wantStatus: AIDetectionStatusSuspect},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := &fakeAIDetectionRouter{content: tc.score}
			svc := NewAIDetectionServiceWithRouter(router, staticAIDetectionConfig(tc.threshold, 3000, 2000))
			article := &models.Article{ID: 1, Title: "标题", Content: "正文内容"}

			result, err := svc.DetectArticle(context.Background(), article, false)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, result.Status)
			require.Equal(t, AIDetectionSourceLLM, result.Source)
			require.Equal(t, tc.threshold, result.Threshold)

			require.Len(t, router.requests, 1)
			require.Equal(t, AIDetectionOperation, router.requests[0].Operation)
			require.Equal(t, airouter.CapabilityAIDetection, router.requests[0].Capability)
		})
	}
}

// TestDetectArticleManualConclusionPrecedence 覆盖「人工结论优先」：已有人工结论时自动
// 判定跳过（除非 force），保证人工结论不被自动结论覆盖。
func TestDetectArticleManualConclusionPrecedence(t *testing.T) {
	router := &fakeAIDetectionRouter{content: "0.95"}
	svc := NewAIDetectionServiceWithRouter(router, staticAIDetectionConfig(0.7, 3000, 2000))

	manual := &models.Article{
		ID:                2,
		Content:           "正文",
		AIDetectionStatus: AIDetectionStatusNotAI,
		AIDetectionSource: AIDetectionSourceManual,
	}
	_, err := svc.DetectArticle(context.Background(), manual, false)
	require.ErrorIs(t, err, ErrAIDetectionManualOverride)
	require.Empty(t, router.requests, "人工结论优先时不得发起判定调用")

	forced, err := svc.DetectArticle(context.Background(), manual, true)
	require.NoError(t, err)
	require.Equal(t, AIDetectionStatusSuspect, forced.Status)
	require.Len(t, router.requests, 1)
}

// TestDetectArticleThresholdOnlineAdjustment 覆盖阈值在线调整：改 ai_settings 后立即
// 对后续判定生效（判定服务每次判定现读，不缓存）。
func TestDetectArticleThresholdOnlineAdjustment(t *testing.T) {
	db := setupAIDetectionTestDB(t)

	require.NoError(t, db.Create(&models.AISettings{
		Key:         "ai_detection_threshold",
		Value:       "0.9",
		Description: "疑似 AI 判定置信分阈值",
	}).Error)

	router := &fakeAIDetectionRouter{content: "0.86"}
	svc := NewAIDetectionServiceWithRouter(router, aisettings.LoadAIDetectionConfig)

	article := &models.Article{ID: 3, Title: "标题", Content: "正文内容"}
	result, err := svc.DetectArticle(context.Background(), article, false)
	require.NoError(t, err)
	require.Equal(t, AIDetectionStatusNotAI, result.Status, "阈值 0.9 时 0.86 不应判为疑似")

	require.NoError(t, db.Model(&models.AISettings{}).
		Where("key = ?", "ai_detection_threshold").
		Update("value", "0.7").Error)

	result, err = svc.DetectArticle(context.Background(), article, false)
	require.NoError(t, err)
	require.Equal(t, AIDetectionStatusSuspect, result.Status, "阈值在线调低后应立即按新值判定")
	require.Equal(t, 0.7, result.Threshold)
}

// TestDetectArticleAuditSuccessAndFailure 覆盖审计两条路径：成功与失败都写 ai_call_logs。
func TestDetectArticleAuditSuccessAndFailure(t *testing.T) {
	db := setupAIDetectionTestDB(t)

	router := airouter.NewRouterWithStore(airouter.NewStore(db))
	router.RegisterClient(airouter.ProviderTypeOpenAICompatible, &fakeAIDetectionProviderClient{content: "0.86"})

	vendor := models.AIVendor{Name: "detect-vendor", ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "https://detect.example/v1", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	model := models.AIModel{VendorID: vendor.ID, Name: "detect-model", ModelKind: airouter.ModelKindLLM,
		Enabled: true, Status: airouter.ModelStatusOK}
	require.NoError(t, db.Create(&model).Error)
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityAIDetection), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: model.ID, Priority: 1, Enabled: true}).Error)

	svc := NewAIDetectionServiceWithRouter(router, staticAIDetectionConfig(0.7, 3000, 2000))
	article := &models.Article{ID: 4, Title: "标题", Content: "正文内容"}
	result, err := svc.DetectArticle(context.Background(), article, false)
	require.NoError(t, err)
	require.Equal(t, AIDetectionStatusSuspect, result.Status)

	var successLog models.AICallLog
	require.NoError(t, db.Where("operation = ? AND success = ?", AIDetectionOperation, true).First(&successLog).Error)
	require.Equal(t, string(airouter.CapabilityAIDetection), successLog.Capability)
	require.Contains(t, successLog.Prompt, "正文内容")
	require.Equal(t, "0.86", successLog.ResponseSnippet)

	// 失败路径：能力未绑定模型（无 ai_detection 路由）→ 判定失败、记审计、文章判定状态保持未判定。
	failingDB := setupAIDetectionTestDB(t)
	failingRouter := airouter.NewRouterWithStore(airouter.NewStore(failingDB))
	failingRouter.RegisterClient(airouter.ProviderTypeOpenAICompatible, &fakeAIDetectionProviderClient{content: "0.86"})
	failingSvc := NewAIDetectionServiceWithRouter(failingRouter, staticAIDetectionConfig(0.7, 3000, 2000))
	failingArticle := &models.Article{ID: 5, Title: "标题", Content: "正文内容"}
	_, err = failingSvc.DetectArticle(context.Background(), failingArticle, false)
	require.Error(t, err)

	var failureLog models.AICallLog
	require.NoError(t, failingDB.Where("operation = ? AND success = ?", AIDetectionOperation, false).First(&failureLog).Error)
	require.Equal(t, "route_not_found", failureLog.ErrorCode)
	require.Contains(t, failureLog.Prompt, "正文内容")
	require.Empty(t, failingArticle.AIDetectionStatus,
		"判定失败不得改写文章判定状态")
}

// TestDetectArticleParseFailureKeepsUndecided 覆盖「置信分解析失败视为判定失败」：
// 模型回了非数字 → 判定失败并补一条失败审计，文章保持未判定。
func TestDetectArticleParseFailureKeepsUndecided(t *testing.T) {
	router := &fakeAIDetectionRouter{content: "这篇看起来像是 AI 写的"}
	svc := NewAIDetectionServiceWithRouter(router, staticAIDetectionConfig(0.7, 3000, 2000))
	article := &models.Article{ID: 6, Title: "标题", Content: "正文内容"}

	_, err := svc.DetectArticle(context.Background(), article, false)
	require.Error(t, err)
	require.Len(t, router.failures, 1)
	require.Equal(t, "parse_error", router.failures[0].Code)
	require.Empty(t, article.AIDetectionStatus, "判定失败不得改写文章判定状态")
	require.Zero(t, article.AIDetectionScore)
}

// TestPersistAIDetectionResult 覆盖判定结论落库（状态/置信分/来源/时间四字段齐写）。
func TestPersistAIDetectionResult(t *testing.T) {
	db := setupAIDetectionTestDB(t)

	feed := models.Feed{URL: "https://example.com/rss", Title: "源"}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{FeedID: feed.ID, Title: "标题", Content: "正文", Link: "https://example.com/a"}
	require.NoError(t, db.Create(&article).Error)

	result := &AIDetectionResult{
		Score:      0.88,
		Status:     AIDetectionStatusSuspect,
		Source:     AIDetectionSourceLLM,
		Threshold:  0.7,
		DetectedAt: time.Now().In(models.ShanghaiTZ),
	}
	require.NoError(t, PersistAIDetectionResult(&article, result))

	var stored models.Article
	require.NoError(t, db.First(&stored, article.ID).Error)
	require.Equal(t, AIDetectionStatusSuspect, stored.AIDetectionStatus)
	require.InDelta(t, 0.88, stored.AIDetectionScore, 1e-9)
	require.Equal(t, AIDetectionSourceLLM, stored.AIDetectionSource)
	require.NotNil(t, stored.AIDetectionAt)
}

// TestPersistAIDetectionResultDoesNotClobberConcurrentWriter 覆盖 review R-2：判定落库
// 只写四列，不得按「LLM 前加载的快照」整行回写，把另一写者（摘要补全 worker / 用户的
// 已读操作）在这期间写入的列静默回滚。
func TestPersistAIDetectionResultDoesNotClobberConcurrentWriter(t *testing.T) {
	db := setupAIDetectionTestDB(t)

	feed := models.Feed{URL: "https://example.com/rss3", Title: "源"}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{
		FeedID: feed.ID, Title: "标题", Content: "正文",
		Link: "https://example.com/c", SummaryStatus: "pending",
	}
	require.NoError(t, db.Create(&article).Error)

	// 判定 worker 先加载快照：此时摘要为空、未读。
	var snapshot models.Article
	require.NoError(t, db.First(&snapshot, article.ID).Error)

	// 另一写者随后写入摘要/状态，用户同时把文章标为已读。
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", snapshot.ID).Updates(map[string]any{
		"ai_content_summary": "并发写入的摘要",
		"summary_status":     "complete",
		"read":               true,
	}).Error)

	// 判定结论用旧快照落库（模拟 LLM 调用结束后的写回）。
	result := &AIDetectionResult{
		Score:      0.91,
		Status:     AIDetectionStatusSuspect,
		Source:     AIDetectionSourceLLM,
		Threshold:  0.7,
		DetectedAt: time.Now().In(models.ShanghaiTZ),
	}
	require.NoError(t, PersistAIDetectionResult(&snapshot, result))

	var stored models.Article
	require.NoError(t, db.First(&stored, snapshot.ID).Error)
	// 判定四列写入。
	require.Equal(t, AIDetectionStatusSuspect, stored.AIDetectionStatus)
	require.InDelta(t, 0.91, stored.AIDetectionScore, 1e-9)
	require.Equal(t, AIDetectionSourceLLM, stored.AIDetectionSource)
	require.NotNil(t, stored.AIDetectionAt)
	// 并发写者的列不被回滚。
	require.Equal(t, "并发写入的摘要", stored.AIContentSummary, "判定落库不得回滚摘要")
	require.Equal(t, "complete", stored.SummaryStatus, "判定落库不得回滚摘要状态")
	require.True(t, stored.Read, "判定落库不得回滚已读状态")
}

// TestNewArticleDefaultsToUndecided 覆盖存量文章语义：新文章判定状态默认「未判定」。
func TestNewArticleDefaultsToUndecided(t *testing.T) {
	db := setupAIDetectionTestDB(t)

	feed := models.Feed{URL: "https://example.com/rss2", Title: "源"}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{FeedID: feed.ID, Title: "标题", Link: "https://example.com/b"}
	require.NoError(t, db.Create(&article).Error)

	var stored models.Article
	require.NoError(t, db.First(&stored, article.ID).Error)
	require.Equal(t, AIDetectionStatusUnknown, stored.AIDetectionStatus)
	require.Zero(t, stored.AIDetectionScore)
	require.Empty(t, stored.AIDetectionSource)
	require.Nil(t, stored.AIDetectionAt)
}
