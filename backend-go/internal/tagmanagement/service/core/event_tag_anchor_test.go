package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/tagmanagement/repository"
)

// ─── 纯函数层：年份提取 ───

func TestExtractEarliestYear(t *testing.T) {
	cases := []struct {
		name   string
		desc   string
		want   int
		wantOK bool
	}{
		{"单年份", "2024年4月发生的大规模袭击", 2024, true},
		{"多年份取最早", "2020年至2023年间多起事件", 2020, true},
		{"历史参照", "创2013年以来新高", 2013, true},
		{"无年份", "某国央行上调基准利率的政策事件", 0, false},
		{"空串", "", 0, false},
		{"纯空白", "   ", 0, false},
		{"1999不匹配（正则仅20xx）", "1999年的旧事", 0, false},
		{"版本号不匹配", "升级到v2024版本", 0, false},
		{"全角数字不匹配", "２０２４年", 0, false},
		{"长数字串不匹配", "编号12024的文件", 0, false},
		{"裸未来年份匹配", "展望2099", 2099, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExtractEarliestYear(tc.desc)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

// ─── 纯函数层：候选筛选边界 ───

func TestFilterEventAnchorCandidatesBoundaries(t *testing.T) {
	created := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	rows := []EventAnchorScanRow{
		{ID: 1, Label: "差1命中", Description: "2025年发生的事件", CreatedAt: created},
		{ID: 2, Label: "差0不命中", Description: "2026年发生的事件", CreatedAt: created},
		{ID: 3, Label: "无年份不命中", Description: "没有任何年份的描述", CreatedAt: created},
		{ID: 4, Label: "未来锚不命中", Description: "对2027年的展望", CreatedAt: created},
		{ID: 5, Label: "跨年表述取最早", Description: "2020年至2023年间索贿", CreatedAt: created},
	}

	got := FilterEventAnchorCandidates(rows)
	require.Len(t, got, 2)
	require.Equal(t, uint(1), got[0].ID)
	require.Equal(t, 2025, got[0].EarliestYear)
	require.Equal(t, uint(5), got[1].ID)
	require.Equal(t, 2020, got[1].EarliestYear)
}

func TestFilterEventAnchorCandidatesEmptyInput(t *testing.T) {
	require.Empty(t, FilterEventAnchorCandidates(nil))
}

// 幂等复扫：修复后（描述锚定到创建当年语境）同一标签不再命中。
func TestFilterEventAnchorCandidatesRescanAfterFix(t *testing.T) {
	row := EventAnchorScanRow{
		ID:          183730,
		Label:       "某事件",
		Description: "2024年4月发生的袭击事件",
		CreatedAt:   time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC),
	}
	require.NotEmpty(t, FilterEventAnchorCandidates([]EventAnchorScanRow{row}), "修复前应命中")

	row.Description = "发生于文章报道当期（2026年9月）的货币政策事件"
	require.Empty(t, FilterEventAnchorCandidates([]EventAnchorScanRow{row}), "修复后复扫不应命中")
}

// ─── 纯函数层：ID 白名单 ───

func TestSelectEventAnchorCandidatesByIDs(t *testing.T) {
	candidates := []EventAnchorCandidate{
		{EventAnchorScanRow: EventAnchorScanRow{ID: 1, Label: "a"}},
		{EventAnchorScanRow: EventAnchorScanRow{ID: 2, Label: "b"}},
		{EventAnchorScanRow: EventAnchorScanRow{ID: 3, Label: "c"}},
	}

	t.Run("命中且重复ID去重", func(t *testing.T) {
		selected, err := SelectEventAnchorCandidatesByIDs(candidates, []uint{2, 2, 1})
		require.NoError(t, err)
		require.Len(t, selected, 2)
		require.Equal(t, uint(2), selected[0].ID)
		require.Equal(t, uint(1), selected[1].ID)
	})

	t.Run("非候选ID拒绝", func(t *testing.T) {
		_, err := SelectEventAnchorCandidatesByIDs(candidates, []uint{2, 99})
		require.Error(t, err)
		require.Contains(t, err.Error(), "99")
	})

	t.Run("不存在ID拒绝", func(t *testing.T) {
		_, err := SelectEventAnchorCandidatesByIDs(candidates, []uint{42})
		require.Error(t, err)
		require.Contains(t, err.Error(), "42")
	})
}

// ─── DB 层（testcontainers PG，-short 跳过）───

// setupAnchorFixTestDB 复用 metadata_test 的测试库初始化（repository +
// database.DB + noop aux），并返回带时间戳的固定值。
func setupAnchorFixTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	setupTopicExtractionTestDB(t)
	return repository.Repo.DB()
}

// setupTaggingLLMRoute 按既有模式注册 CapabilityTopicTagging 路由指向 fake server。
func setupTaggingLLMRoute(t *testing.T, db *gorm.DB, serverURL string) {
	t.Helper()
	vendor := models.AIVendor{Name: "anchor-vendor", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: serverURL, APIKey: "token", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	model := models.AIModel{VendorID: vendor.ID, Name: "anchor-test-model", ModelKind: airouter.ModelKindLLM, Enabled: true, Status: airouter.ModelStatusOK}
	require.NoError(t, db.Create(&model).Error)
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityTopicTagging), Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: model.ID, Priority: 1, Enabled: true}).Error)
}

// createAnchorFixture 建 feed + 文章（发布日 2026-10-09）+ event 标签 + 首挂链接。
func createAnchorFixture(t *testing.T, db *gorm.DB, tagLabel, tagDescription string, tagCreatedAt time.Time) (tagID, articleID uint) {
	t.Helper()
	feed := models.Feed{Title: "fixture-feed", URL: "https://example.com/anchor-" + tagLabel}
	require.NoError(t, db.Create(&feed).Error)
	pubDate := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	article := models.Article{
		FeedID:           feed.ID,
		Title:            "美联储官员谈货币政策走向",
		Link:             "https://example.com/anchor/article-" + tagLabel,
		Description:      "多位官员就利率路径发表看法。",
		AIContentSummary: "多位美联储官员表示将依据通胀数据决定利率路径，市场预期年内维持当前水平。",
		PubDate:          &pubDate,
	}
	require.NoError(t, db.Create(&article).Error)
	tag := models.TopicTag{
		Slug:        "fixture-" + tagLabel,
		Label:       tagLabel,
		Category:    models.TagCategoryEvent,
		IsCanonical: true,
		Source:      "llm",
		Status:      "active",
		Description: tagDescription,
		CreatedAt:   tagCreatedAt,
	}
	require.NoError(t, db.Create(&tag).Error)
	link := models.ArticleTopicTag{
		ArticleID:  article.ID,
		TopicTagID: tag.ID,
		Source:     "llm",
		CreatedAt:  tagCreatedAt,
	}
	require.NoError(t, db.Create(&link).Error)
	return tag.ID, article.ID
}

func TestScanEventTagAnchorCandidates(t *testing.T) {
	db := setupAnchorFixTestDB(t)
	created := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)

	staleID, _ := createAnchorFixture(t, db, "病态事件标签", "2024年4月发生的袭击事件", created)

	healthyID, _ := createAnchorFixture(t, db, "健康事件标签", "发生于文章报道当期的货币政策事件", created)

	// keyword 类不扫
	keywordTag := models.TopicTag{Slug: "fixture-kw", Label: "关键词标签", Category: "keyword", Status: "active", Description: "2020年的旧描述", CreatedAt: created}
	require.NoError(t, db.Create(&keywordTag).Error)
	// merged 状态不扫
	mergedTag := models.TopicTag{Slug: "fixture-merged", Label: "已合并标签", Category: models.TagCategoryEvent, Status: "merged", Description: "2020年的旧描述", CreatedAt: created}
	require.NoError(t, db.Create(&mergedTag).Error)
	// 空描述不扫
	emptyTag := models.TopicTag{Slug: "fixture-empty", Label: "空描述标签", Category: models.TagCategoryEvent, Status: "active", Description: "  ", CreatedAt: created}
	require.NoError(t, db.Create(&emptyTag).Error)

	candidates, err := ScanEventTagAnchorCandidates()
	require.NoError(t, err)

	ids := make([]uint, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.ID)
	}
	require.Contains(t, ids, staleID)
	require.NotContains(t, ids, healthyID)
	require.NotContains(t, ids, keywordTag.ID)
	require.NotContains(t, ids, mergedTag.ID)
	require.NotContains(t, ids, emptyTag.ID)

	for _, c := range candidates {
		if c.ID == staleID {
			require.Equal(t, 2024, c.EarliestYear)
			require.Equal(t, "病态事件标签", c.Label)
		}
	}
}

func TestRegenerateEventTagDescriptionAnchorsFirstMountArticle(t *testing.T) {
	db := setupAnchorFixTestDB(t)

	var sawDateAnchor, sawTimeDiscipline bool
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "[日期: 2026-10-09]") {
			sawDateAnchor = true
		}
		if strings.Contains(string(body), "Time anchor discipline") {
			sawTimeDiscipline = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"description\":\"发生于文章报道当期（2026年10月）的货币政策事件\",\"keywords\":[\"美联储\",\"利率\",\"通胀\"]}"}}]}`))
	}))
	defer aiServer.Close()
	setupTaggingLLMRoute(t, db, aiServer.URL)

	tagID, _ := createAnchorFixture(t, db, "重生成事件标签", "2024年4月发生的袭击事件", time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC))

	newDesc, err := RegenerateEventTagDescription(tagID)
	require.NoError(t, err)
	require.Equal(t, "发生于文章报道当期（2026年10月）的货币政策事件", newDesc)
	require.True(t, sawDateAnchor, "重生成 prompt 应以首挂文章 [日期] 前缀为上下文")
	require.True(t, sawTimeDiscipline, "重生成 prompt 应包含时间锚纪律约束")

	var tag models.TopicTag
	require.NoError(t, db.Where("id = ?", tagID).First(&tag).Error)
	require.Equal(t, newDesc, tag.Description)
	require.Equal(t, []interface{}{"美联储", "利率", "通胀"}, tag.Metadata["event_keywords"])

	// 幂等：重复执行仍是单行 UPDATE，描述可再次覆盖，不产生重复副作用
	newDesc2, err := RegenerateEventTagDescription(tagID)
	require.NoError(t, err)
	require.Equal(t, newDesc, newDesc2)
	var count int64
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", tagID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	// 修复后复扫不再命中（描述锚定 2026 = 创建年 2026）
	candidates, err := ScanEventTagAnchorCandidates()
	require.NoError(t, err)
	for _, c := range candidates {
		require.NotEqual(t, tagID, c.ID, "修复后标签不应再出现在候选清单")
	}
}

func TestRegenerateEventTagDescriptionRejectsInvalidTargets(t *testing.T) {
	db := setupAnchorFixTestDB(t)
	created := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)

	// 非 event 类
	kwTag := models.TopicTag{Slug: "reject-kw", Label: "关键词标签", Category: "keyword", Status: "active", Description: "2020年旧描述", CreatedAt: created}
	require.NoError(t, db.Create(&kwTag).Error)
	_, err := RegenerateEventTagDescription(kwTag.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "category")

	// 不存在的标签
	_, err = RegenerateEventTagDescription(999999)
	require.Error(t, err)

	// 无首挂文章的 event 标签
	orphan := models.TopicTag{Slug: "reject-orphan", Label: "无文章事件", Category: models.TagCategoryEvent, Status: "active", Description: "2020年旧描述", CreatedAt: created}
	require.NoError(t, db.Create(&orphan).Error)
	_, err = RegenerateEventTagDescription(orphan.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no mounted article")

	// 非候选不触碰：以上失败路径过后，描述逐字未变（独立变量：GORM First 复用带主键的 struct 会叠加旧 ID 条件）
	var kwReloaded, orphanReloaded models.TopicTag
	require.NoError(t, db.Where("id = ?", kwTag.ID).First(&kwReloaded).Error)
	require.Equal(t, "2020年旧描述", kwReloaded.Description)
	require.NoError(t, db.Where("id = ?", orphan.ID).First(&orphanReloaded).Error)
	require.Equal(t, "2020年旧描述", orphanReloaded.Description)
}

func TestApplyEventTagAnchorFixBatchContinuesOnFailure(t *testing.T) {
	db := setupAnchorFixTestDB(t)
	created := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)

	failingID, _ := createAnchorFixture(t, db, "失败标签", "2021年发生的旧事件", created)
	okID, _ := createAnchorFixture(t, db, "成功标签", "2022年发生的旧事件", created)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "失败标签") {
			// 非法 JSON：三次重试全失败
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"description\":\"发生于文章报道当期（2026年10月）的事件\",\"keywords\":[\"主体\",\"动作\"]}"}}]}`))
	}))
	defer aiServer.Close()
	setupTaggingLLMRoute(t, db, aiServer.URL)

	candidates, err := ScanEventTagAnchorCandidates()
	require.NoError(t, err)

	results, err := ApplyEventTagAnchorFix(candidates, []uint{failingID, okID})
	require.NoError(t, err)
	require.Len(t, results, 2)

	byID := map[uint]EventAnchorFixResult{}
	for _, r := range results {
		byID[r.TagID] = r
	}
	require.Error(t, byID[failingID].Err, "LLM 失败的标签应记录错误")
	require.NotEmpty(t, byID[okID].NewDescription, "批处理不应因单标签失败中断")

	var failingReloaded, okReloaded models.TopicTag
	require.NoError(t, db.Where("id = ?", failingID).First(&failingReloaded).Error)
	require.Equal(t, "2021年发生的旧事件", failingReloaded.Description, "失败标签描述不得被改动")
	require.NoError(t, db.Where("id = ?", okID).First(&okReloaded).Error)
	require.Equal(t, "发生于文章报道当期（2026年10月）的事件", okReloaded.Description)

	// 含非候选 ID：整批拒绝，不触碰任何行
	_, err = ApplyEventTagAnchorFix(candidates, []uint{okID, 424242})
	require.Error(t, err)
	require.Contains(t, err.Error(), "424242")
	var okReloaded2 models.TopicTag
	require.NoError(t, db.Where("id = ?", okID).First(&okReloaded2).Error)
	require.Equal(t, "发生于文章报道当期（2026年10月）的事件", okReloaded2.Description)
}
