package airouter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
)

// ============================================================================
// ai_detection capability 测试（add-ai-generated-article-detection task 2.1）
// ============================================================================

// TestCapabilityAIDetectionIsIndependent 覆盖「新增第七类 capability 且不与打标共用
// 模型绑定与并发配额」：同一 capability 名字符串固定、默认并发独立、信号量按 capability
// 分桶（打标占满后判定仍能拿到自己的槽位）。
func TestCapabilityAIDetectionIsIndependent(t *testing.T) {
	require.Equal(t, "ai_detection", string(CapabilityAIDetection))

	detectionQuota, ok := defaultConcurrency[CapabilityAIDetection]
	require.True(t, ok, "ai_detection 必须有独立默认并发配额")
	require.Greater(t, detectionQuota, 0)
	require.NotEqual(t, defaultConcurrency[CapabilityTopicTagging], detectionQuota)

	router := NewRouterWithStore(nil)
	taggingSem := router.getSemaphore(CapabilityTopicTagging, nil)
	detectionSem := router.getSemaphore(CapabilityAIDetection, nil)
	require.True(t, taggingSem != detectionSem, "信号量必须按 capability 分桶")
	require.Equal(t, detectionQuota, cap(detectionSem))
	require.Equal(t, defaultConcurrency[CapabilityTopicTagging], cap(taggingSem))
}

// TestRouterAIDetectionUsesOwnRoute 覆盖「判定按 ai_detection 路由解析模型」：
// ai_detection 路由绑定的模型被调用，打标路由的绑定不参与。
func TestRouterAIDetectionUsesOwnRoute(t *testing.T) {
	db := setupAIRouterTestDB(t)
	store := NewStore(db)

	detectionRoute := models.AIRoute{Name: DefaultRouteName, Capability: string(CapabilityAIDetection), Enabled: true}
	require.NoError(t, db.Create(&detectionRoute).Error)
	fixture := seedChatBinding(t, db, &detectionRoute, "detect-vendor", "small-model", nil, nil, "", 1)

	router := NewRouterWithStore(store)
	router.RegisterClient(ProviderTypeOpenAICompatible, &fakeProviderClient{responses: map[string]fakeChatReply{
		"detect-vendor": {content: "0.86"},
	}})

	result, err := router.Chat(context.Background(), ChatRequest{
		Operation:  "tagmanagement.ai_detection",
		Capability: CapabilityAIDetection,
		Messages:   []Message{{Role: "user", Content: "样本正文"}},
	})
	require.NoError(t, err)
	require.Equal(t, "0.86", result.Content)
	require.Equal(t, fixture.vendor.Name, result.ProviderName)
	require.Equal(t, fixture.model.Name, result.ModelName)
	require.Equal(t, string(CapabilityAIDetection), string(CapabilityAIDetection))
}

// TestRouteConcurrencyOverrideAppliesToAIDetection 覆盖「默认值可被 ai_routes.max_concurrency
// 覆盖」（ai-summary 红线 3）：路由显式配了并发上限时以路由为准。
func TestRouteConcurrencyOverrideAppliesToAIDetection(t *testing.T) {
	router := NewRouterWithStore(nil)
	route := &models.AIRoute{MaxConcurrency: 7}
	require.Equal(t, 7, router.resolveConcurrency(CapabilityAIDetection, route))
	require.Equal(t, defaultConcurrency[CapabilityAIDetection], router.resolveConcurrency(CapabilityAIDetection, nil))
}

// TestLogLocalFailureWritesAuditRow 覆盖「能力未绑定模型 → 判定失败并记审计」：
// Router.Chat 早退不写行，由 LogLocalFailure 补写（成败都写审计红线）。
func TestLogLocalFailureWritesAuditRow(t *testing.T) {
	db := setupAIRouterTestDB(t)
	router := NewRouterWithStore(NewStore(db))

	req := ChatRequest{
		Operation:  "tagmanagement.ai_detection",
		Capability: CapabilityAIDetection,
		Messages:   []Message{{Role: "user", Content: "样本正文"}},
		Metadata:   map[string]any{"article_id": uint(42)},
	}
	router.LogLocalFailure(context.Background(), req, LocalFailure{
		Code:    "route_not_found",
		Message: "ai route not found",
	})

	var log models.AICallLog
	require.NoError(t, db.Where("operation = ?", req.Operation).First(&log).Error)
	require.False(t, log.Success)
	require.Equal(t, "route_not_found", log.ErrorCode)
	require.Equal(t, string(CapabilityAIDetection), log.Capability)
	require.Contains(t, log.Prompt, "样本正文")
	require.Contains(t, log.RequestMeta, "local_failure")
	require.Contains(t, log.RequestMeta, "article_id")
}

// TestLogLocalFailureTruncatesPayload 覆盖「审计日志必须做截断保护」：
// response_snippet 截 10000 runes，与 Chat 路径同款口径。
func TestLogLocalFailureTruncatesPayload(t *testing.T) {
	db := setupAIRouterTestDB(t)
	router := NewRouterWithStore(NewStore(db))

	longContent := make([]rune, 12000)
	for i := range longContent {
		longContent[i] = 'X'
	}
	req := ChatRequest{Operation: "tagmanagement.ai_detection", Capability: CapabilityAIDetection,
		Messages: []Message{{Role: "user", Content: "样本"}}}
	router.LogLocalFailure(context.Background(), req, LocalFailure{
		Code:            "parse_error",
		Message:         "cannot parse",
		ResponseSnippet: string(longContent),
	})

	var log models.AICallLog
	require.NoError(t, db.Where("operation = ?", req.Operation).First(&log).Error)
	require.Equal(t, maxResponseSnippet+3, len([]rune(log.ResponseSnippet)))
}
