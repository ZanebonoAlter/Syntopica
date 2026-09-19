package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

const (
	aggregateSection0Response = `{"tags":[` +
		`{"label":"AI缓存技术突破","category":"event","description":"AI缓存领域取得技术突破的新闻事件","auxiliary_labels":[{"label":"AI缓存","description":"人工智能缓存技术"},{"label":"缓存加速","description":"提升系统响应速度的技术"},{"label":"技术突破","description":"技术取得重大进展"}]},` +
		`{"label":"Kubernetes","category":"keyword","description":"开源容器编排系统"}]}`

	aggregateSection1Response = `{"tags":[` +
		`{"label":"Kubernetes","category":"keyword","description":"开源容器编排系统"},` +
		`{"label":"Sam Altman","category":"person","description":"","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"ChatGPT","description":"对话式AI产品"},{"label":"GPT-5","description":"大语言模型版本"}]}]}`

	aggregateSection2Response = `{"tags":[` +
		`{"label":"Rust语言","category":"keyword","description":"系统级编程语言"},` +
		`{"label":"Vite构建工具","category":"keyword","description":"前端构建工具"}]}`

	monoMergedResponse = `{"event_person_tags":[{"label":"OpenAI发布GPT-5","category":"event","auxiliary_labels":[{"label":"OpenAI","description":"人工智能研究公司"},{"label":"GPT-5","description":"大语言模型版本"},{"label":"模型发布","description":"产品发布行为"}]}],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库管理系统"},{"label":"LangChain","category":"keyword","description":"大模型应用开发框架"}]}`

	monoKeywordOnlyMergedResponse = `{"event_person_tags":[],"keyword_tags":[{"label":"PostgreSQL","category":"keyword","description":"开源关系型数据库管理系统"},{"label":"LangChain","category":"keyword","description":"大模型应用开发框架"}]}`
)

// aggregateMarkdownSummary builds a 4-column digest (intro + 3 body columns)
// that the splitter reduces to 3 sections.
func aggregateMarkdownSummary() string {
	return "# 科技周刊第100期\n\n" +
		"## 导读\n" + bodyOf("本期导读概述各栏目要点。", 350) + "\n\n" +
		"## AI缓存专题\n" + bodyOf("AI缓存技术取得新突破。", 350) + "\n\n" +
		"## 科技动态\n" + bodyOf("本周科技动态汇总。", 350) + "\n\n" +
		"## 言论\n" + bodyOf("开发者讨论提示词缓存。", 350)
}

func setupAggregateTaggerTest(t *testing.T, router *fakeTagChatRouter) *gorm.DB {
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

func createAggregateArticle(t *testing.T, db *gorm.DB, summary, contentForm string) *models.Article {
	t.Helper()
	feed := models.Feed{Title: "科技周刊", URL: "https://example.com/weekly"}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{
		FeedID:           feed.ID,
		Title:            "科技周刊第100期",
		AIContentSummary: summary,
		ContentForm:      contentForm,
	}
	require.NoError(t, db.Create(&article).Error)
	return &article
}

type articleTagScoreRow struct {
	Label string
	Score float64
}

// sectionAwareFakeRouter dispatches fake responses by metadata["section_index"],
// so each section can return its own tags regardless of call order.
type sectionAwareFakeRouter struct {
	responses []fakeTagChatResponse
}

func (r *sectionAwareFakeRouter) Chat(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	index, _ := req.Metadata["section_index"].(int)
	if index < 0 || index >= len(r.responses) {
		return nil, fmt.Errorf("no fake response for section %d", index)
	}
	resp := r.responses[index]
	if resp.err != nil {
		return nil, resp.err
	}
	return &airouter.ChatResult{Content: resp.content}, nil
}

func loadArticleTagScores(t *testing.T, db *gorm.DB, articleID uint) []articleTagScoreRow {
	t.Helper()
	var rows []articleTagScoreRow
	require.NoError(t, db.Model(&models.TopicTag{}).
		Select("topic_tags.label AS label, article_topic_tags.score AS score").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Where("article_topic_tags.article_id = ?", articleID).
		Order("article_topic_tags.id ASC").
		Scan(&rows).Error)
	return rows
}

func TestTagArticleAggregatePathDedupesAndTiersScores(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: aggregateSection0Response})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: aggregateSection1Response})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: aggregateSection2Response})
	db := setupAggregateTaggerTest(t, router)
	article := createAggregateArticle(t, db, aggregateMarkdownSummary(), "aggregate")

	require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

	// Intro column skipped → exactly 3 section calls, no mono extraction.
	require.Equal(t, 3, router.callCount("tag_extraction_section"))
	require.Equal(t, 0, router.callCount("tag_extraction_merged"))

	rows := loadArticleTagScores(t, db, article.ID)
	require.Len(t, rows, 5) // 6 candidates - 1 cross-section duplicate (Kubernetes)

	scoreByLabel := make(map[string]float64, len(rows))
	for _, r := range rows {
		scoreByLabel[r.Label] = r.Score
	}
	require.Equal(t, 0.9, scoreByLabel["AI缓存技术突破"])
	require.Equal(t, 0.9, scoreByLabel["Kubernetes"]) // first section wins → headline tier
	require.Equal(t, 0.7, scoreByLabel["Sam Altman"])
	require.Equal(t, 0.5, scoreByLabel["Rust语言"])
	require.Equal(t, 0.5, scoreByLabel["Vite构建工具"])

	// Event tags keep entering the embedding queue, same as the mono path.
	var eventTag models.TopicTag
	require.NoError(t, db.Where("label = ?", "AI缓存技术突破").First(&eventTag).Error)
	var queueCount int64
	require.NoError(t, db.Model(&models.EmbeddingQueue{}).Where("tag_id = ?", eventTag.ID).Count(&queueCount).Error)
	require.Equal(t, int64(1), queueCount)
}

func TestTagArticleAggregateSkipsFailedSection(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: aggregateSection0Response})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{err: errors.New("section down")})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{err: errors.New("section down")})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{err: errors.New("section down")})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: aggregateSection2Response})
	db := setupAggregateTaggerTest(t, router)
	article := createAggregateArticle(t, db, aggregateMarkdownSummary(), "aggregate")

	require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

	// 1 call for section 0 + 3 retries for section 1 + 1 call for section 2.
	require.Equal(t, 5, router.callCount("tag_extraction_section"))

	rows := loadArticleTagScores(t, db, article.ID)
	require.Len(t, rows, 4) // only sections 0 and 2 survive
	labels := make([]string, 0, len(rows))
	for _, r := range rows {
		labels = append(labels, r.Label)
	}
	require.ElementsMatch(t, []string{"AI缓存技术突破", "Kubernetes", "Rust语言", "Vite构建工具"}, labels)
}

func TestTagArticleMonoContentFormsKeepMonoPath(t *testing.T) {
	for _, form := range []string{"", "mono"} {
		t.Run("content_form="+form, func(t *testing.T) {
			router := newFakeTagChatRouter()
			router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: monoMergedResponse})
			db := setupAggregateTaggerTest(t, router)
			article := createAggregateArticle(t, db, aggregateMarkdownSummary(), form)

			require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

			require.Equal(t, 0, router.callCount("tag_extraction_section"))
			require.Equal(t, 1, router.callCount("tag_extraction_merged"))

			rows := loadArticleTagScores(t, db, article.ID)
			require.Len(t, rows, 3)
			for _, r := range rows {
				require.Equal(t, 0.7, r.Score) // mono path keeps the flat score
			}
		})
	}
}

func TestTagArticleAggregateWithoutSectionsFallsBackToMono(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: monoKeywordOnlyMergedResponse})
	db := setupAggregateTaggerTest(t, router)
	// No ## headings → splitter yields nothing → fall back to the mono path.
	article := createAggregateArticle(t, db, bodyOf("本期没有栏目结构的纯文本摘要。", 400), "aggregate")

	require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

	require.Equal(t, 0, router.callCount("tag_extraction_section"))
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))

	rows := loadArticleTagScores(t, db, article.ID)
	require.Len(t, rows, 2)
	labels := make([]string, 0, len(rows))
	for _, r := range rows {
		labels = append(labels, r.Label)
	}
	require.ElementsMatch(t, []string{"PostgreSQL", "LangChain"}, labels)
}

func TestTagAggregateArticleCapsAtFifteenTags(t *testing.T) {
	// Per-section responses: distinct labels so cross-section dedup never kicks in.
	sectionResponse := func(index int) string {
		var b strings.Builder
		b.WriteString(`{"tags":[`)
		for i := 0; i < 4; i++ { // maxSectionTags = 4 per section
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"label":"栏目` + string(rune('0'+index)) + "标签" + string(rune('A'+i)) + `","category":"keyword","description":"描述` + string(rune('A'+i)) + `"}`)
		}
		b.WriteString(`]}`)
		return b.String()
	}
	responses := make([]fakeTagChatResponse, 8)
	router := &sectionAwareFakeRouter{responses: responses}
	for i := range responses {
		router.responses[i] = fakeTagChatResponse{content: sectionResponse(i)}
	}

	var md strings.Builder
	md.WriteString("# 周刊\n")
	for i := 1; i <= 8; i++ {
		md.WriteString("\n## 栏目" + string(rune('0'+i)) + "\n" + bodyOf("栏目正文内容。", 350) + "\n")
	}
	article := &models.Article{AIContentSummary: md.String(), ContentForm: "aggregate"}

	tags, handled, err := tagAggregateArticle(context.Background(), article, ExtractionInput{Title: "周刊"}, &TagExtractor{router: router})

	require.NoError(t, err)
	require.True(t, handled)
	require.Len(t, tags, 15) // 8 sections × 4 tags = 32 candidates → capped at 15
	// Section order + in-section order is preserved: first 4 from section 0.
	for i := 0; i < 4; i++ {
		require.Equal(t, "栏目0标签"+string(rune('A'+i)), tags[i].Label)
	}
	// Score tiers follow original section position: 0.9 / 0.7 / ... / 0.5.
	require.Equal(t, 0.9, tags[0].Score)
	require.Equal(t, 0.7, tags[4].Score)
	require.Equal(t, 0.7, tags[8].Score)
	require.Equal(t, 0.7, tags[14].Score)
}

func TestTagAggregateArticleScoreTiersForOneAndTwoSections(t *testing.T) {
	sectionResponse := `{"tags":[{"label":"唯一标签","category":"keyword","description":"描述"}]}`

	t.Run("single section gets headline score", func(t *testing.T) {
		router := newFakeTagChatRouter()
		router.enqueue("tag_extraction_section", fakeTagChatResponse{content: sectionResponse})
		article := &models.Article{AIContentSummary: "## 唯一栏目\n" + bodyOf("正文。", 350)}

		tags, handled, err := tagAggregateArticle(context.Background(), article, ExtractionInput{Title: "标题"}, &TagExtractor{router: router})

		require.NoError(t, err)
		require.True(t, handled)
		require.Len(t, tags, 1)
		require.Equal(t, 0.9, tags[0].Score)
	})

	t.Run("two sections get first/last tiers", func(t *testing.T) {
		router := &sectionAwareFakeRouter{responses: []fakeTagChatResponse{
			{content: `{"tags":[{"label":"首栏目标签","category":"keyword","description":"描述"}]}`},
			{content: `{"tags":[{"label":"尾栏目标签","category":"keyword","description":"描述"}]}`},
		}}
		article := &models.Article{
			AIContentSummary: "## 栏目一\n" + bodyOf("正文一。", 350) + "\n\n## 栏目二\n" + bodyOf("正文二。", 350),
		}

		tags, handled, err := tagAggregateArticle(context.Background(), article, ExtractionInput{Title: "标题"}, &TagExtractor{router: router})

		require.NoError(t, err)
		require.True(t, handled)
		require.Len(t, tags, 2)
		require.Equal(t, "首栏目标签", tags[0].Label)
		require.Equal(t, 0.9, tags[0].Score)
		require.Equal(t, 0.5, tags[1].Score)
	})
}

func TestBuildArticleSummaryTruncatesAt4000Runes(t *testing.T) {
	article := models.Article{AIContentSummary: bodyOf("字", 5000)}

	summary := buildArticleSummary(article)

	require.Len(t, []rune(summary), 4000)
}

func TestLimitArticleTagsCapsAtSix(t *testing.T) {
	tags := make([]TopicTag, 8)
	for i := range tags {
		tags[i] = TopicTag{Label: string(rune('A' + i))}
	}

	limited := limitArticleTags(tags)

	require.Len(t, limited, 6)
}

// Aggregate articles must keep up to maxAggregateArticleTags (15) tags; the
// mono limitArticleTags cap (6) must not truncate the aggregate path.
func TestTagArticleAggregatePathExceedsMonoLimit(t *testing.T) {
	router := newFakeTagChatRouter()
	secTags := func(prefix string) string {
		return fmt.Sprintf(`{"tags":[%s]}`,
			`{"label":"`+prefix+`一","category":"keyword","description":"测试关键词一"},`+
				`{"label":"`+prefix+`二","category":"keyword","description":"测试关键词二"},`+
				`{"label":"`+prefix+`三","category":"keyword","description":"测试关键词三"},`+
				`{"label":"`+prefix+`四","category":"keyword","description":"测试关键词四"}`)
	}
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: secTags("专题")})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: secTags("动态")})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: secTags("工具")})
	db := setupAggregateTaggerTest(t, router)
	article := createAggregateArticle(t, db, aggregateMarkdownSummary(), "aggregate")

	require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

	rows := loadArticleTagScores(t, db, article.ID)
	require.Len(t, rows, 12) // 3 sections x 4 tags, all distinct — must NOT be capped at 6
}

func TestTagArticleAggregateAllSectionsFailedFallsBackToMono(t *testing.T) {
	router := newFakeTagChatRouter()
	// Every section call fails after retries (派早报 scenario).
	for i := 0; i < 9; i++ {
		router.enqueue("tag_extraction_section", fakeTagChatResponse{content: "not json"})
	}
	// Mono fallback: single merged call, keyword array carries the tags.
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: monoKeywordOnlyMergedResponse})
	db := setupAggregateTaggerTest(t, router)
	article := createAggregateArticle(t, db, aggregateMarkdownSummary(), "aggregate")

	require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

	require.Equal(t, 9, router.callCount("tag_extraction_section")) // 3 sections × 3 retries
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))

	rows := loadArticleTagScores(t, db, article.ID)
	require.Len(t, rows, 2)
	for _, r := range rows {
		require.Equal(t, 0.7, r.Score) // mono path flat score
	}
	labels := make([]string, 0, len(rows))
	for _, r := range rows {
		labels = append(labels, r.Label)
	}
	require.ElementsMatch(t, []string{"PostgreSQL", "LangChain"}, labels)
}

func TestTagArticleAggregateAllSectionsEmptyFallsBackToMono(t *testing.T) {
	router := newFakeTagChatRouter()
	// Sections parse fine but yield no candidates (408期 section-0 scenario).
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: `{"tags":[]}`})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: `{"tags":[]}`})
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: `{"tags":[]}`})
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: monoMergedResponse})
	db := setupAggregateTaggerTest(t, router)
	article := createAggregateArticle(t, db, aggregateMarkdownSummary(), "aggregate")

	require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

	require.Equal(t, 3, router.callCount("tag_extraction_section"))
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))

	rows := loadArticleTagScores(t, db, article.ID)
	require.NotEmpty(t, rows, "empty aggregate output must fall back to mono, not end with 0 tags")
}

func TestTagArticleAggregatePartialSuccessNoFallback(t *testing.T) {
	router := newFakeTagChatRouter()
	// Section 0 succeeds, sections 1-2 fail: partial output, no mono fallback.
	router.enqueue("tag_extraction_section", fakeTagChatResponse{content: aggregateSection0Response})
	for i := 0; i < 6; i++ {
		router.enqueue("tag_extraction_section", fakeTagChatResponse{content: "not json"})
	}
	db := setupAggregateTaggerTest(t, router)
	article := createAggregateArticle(t, db, aggregateMarkdownSummary(), "aggregate")

	require.NoError(t, TagArticle(context.Background(), article, "科技周刊", "科技"))

	require.Equal(t, 7, router.callCount("tag_extraction_section"))
	require.Equal(t, 0, router.callCount("tag_extraction_merged"), "partial aggregate output must NOT trigger mono fallback")
}
