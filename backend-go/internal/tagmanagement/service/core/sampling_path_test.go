package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

// capturingRouter 实现 tagChatRouter：捕获每次 Chat 调用全部消息内容后恒返错，
// 让 tagArticle 走 heuristic fallback 并把标签 persist 进测试 DB。
type capturingRouter struct {
	mu      sync.Mutex
	prompts []string
}

func (r *capturingRouter) Chat(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
	}
	r.prompts = append(r.prompts, b.String())
	return nil, errors.New("capture-only router")
}

// setupSamplingPathTest 与 setupAggregateTaggerTest 同模式：测试 DB + 全局恢复。
func setupSamplingPathTest(t *testing.T, router *capturingRouter) *gorm.DB {
	t.Helper()
	db := setupArticleTaggerTestDB(t)
	database.DB = db // needed by airouter.NewStore() via NewTagExtractor()
	GetTagCache().Clear()

	AuxServiceFactory = func(db *gorm.DB, embedder interface{}) AuxService {
		return &noopAuxService{}
	}
	tagExtractorFactory = func() *TagExtractor {
		return &TagExtractor{router: router}
	}
	t.Cleanup(func() {
		tagExtractorFactory = NewTagExtractor
	})
	return db
}

// 存量文章（ContentForm 零值）走 mono 路径，打标输入必须由同一采样函数构造：
// 4700 runes 叙事文按头[0:2000)/中[2350:3350)/尾[3700:4700) 采样，两个"洞"区
// 被跳过——若实现退化回旧掐头 4000 行为，洞甲必然进入 prompt，下面两条
// NotContains 就是本测试的关键证伪断言（long-form-sampled-tagging 2.4）。
func TestTagArticleLegacyMonoPathSamplesHeadMidTail(t *testing.T) {
	router := &capturingRouter{}
	db := setupSamplingPathTest(t, router)

	body := strings.Repeat("头", 2000) +
		strings.Repeat("洞甲", 175) + // 洞一 [2000:2350)
		strings.Repeat("中", 1000) + // 中窗 [2350:3350)
		strings.Repeat("洞乙", 175) + // 洞二 [3350:3700)
		strings.Repeat("尾", 1000) // 尾窗 [3700:4700)
	require.Equal(t, 4700, len([]rune(body)))

	feed := models.Feed{Title: "测试Feed", URL: "https://example.com/legacy-sampling"}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{
		FeedID:           feed.ID,
		Title:            "存量文章采样验证",
		FirecrawlContent: body, // ContentForm 留空零值 → mono 路径
	}
	require.NoError(t, db.Create(&article).Error)

	require.NoError(t, TagArticle(context.Background(), &article, "测试Feed", ""))

	require.NotEmpty(t, router.prompts)
	headMark := strings.Repeat("头", 50)
	var bodyPrompt string
	for _, p := range router.prompts { // 遍历查找更稳，不依赖调用次序
		if strings.Contains(p, headMark) {
			bodyPrompt = p
			break
		}
	}
	require.NotEmpty(t, bodyPrompt, "no captured prompt carries the sampled body")
	require.Contains(t, bodyPrompt, strings.Repeat("中", 50))
	require.Contains(t, bodyPrompt, strings.Repeat("尾", 50))
	require.Contains(t, bodyPrompt, ellipsisMark)
	require.NotContains(t, bodyPrompt, "洞甲")
	require.NotContains(t, bodyPrompt, "洞乙")
}
