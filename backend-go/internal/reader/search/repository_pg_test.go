package search

// add-article-search tasks 2.3 — 关键词路径 repository 的 testcontainer PG
// 集成测试（DSN 红线：只连 testutil 的隔离容器，绝不连真库）。
//
// 覆盖 test-cases.md 故事 S1 步 2/3/6/7：
//   步 2 词典词重组「科技股AI泡沫」命中换序标题「AI 泡沫下的科技股定价逻辑」
//   步 3 trigram 召回「特斯拉电池自燃」→「特斯拉Model Y地下车库自燃」（分数区间 [0.1,0.3]）
//   步 6 默认含归档（archived 标记随行）
//   步 7 active-only 过滤
// 加变体「空库/无标签库」、通配符字面匹配（PG ESCAPE 子句）、
// 白盒「阈值边界值表」trigram 行（自校准阈值：t-ε 召回 / t 召回 / t+ε 不召回
// + 非法配置回退默认）。
//
// 黄金 schema 已含 pg_trgm 扩展与两列 GIN 索引（迁移 20261013_0001，
// tasks 1.1）。

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func setupSearchPG(t *testing.T) (*SearchRepository, *gorm.DB) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return NewSearchRepository(db), db
}

func seedFeedPG(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	feed := models.Feed{Title: "测试源", URL: fmt.Sprintf("https://example.com/%s", t.Name()), Icon: "rss"}
	require.NoError(t, db.Create(&feed).Error)
	return feed.ID
}

func seedArticlePG(t *testing.T, db *gorm.DB, feedID uint, title, description string, archived bool) uint {
	t.Helper()
	article := models.Article{
		FeedID:      feedID,
		Title:       title,
		Description: description,
		Link:        fmt.Sprintf("https://example.com/a/%s/%s", t.Name(), title),
		Archived:    archived,
	}
	require.NoError(t, db.Create(&article).Error)
	return article.ID
}

func seedTagPG(t *testing.T, db *gorm.DB, label string) uint {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: label, Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag.ID
}

func setSettingPG(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	db.Where("key = ?", key).Delete(&models.AISettings{})
	require.NoError(t, db.Create(&models.AISettings{Key: key, Value: value}).Error)
}

func findHitByID(hits []ArticleHit, id uint) *ArticleHit {
	for i := range hits {
		if hits[i].Article.ID == id {
			return &hits[i]
		}
	}
	return nil
}

// TestSearchKeywordDictRecompositionHitsReorderedTitle covers S1 步 2：
// 词重组查询经 L1 词典切词逐词命中换序标题，不因 L2 整串不匹配而漏。
func TestSearchKeywordDictRecompositionHitsReorderedTitle(t *testing.T) {
	repo, db := setupSearchPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	// 词典词标签（3 runes，词内无空白）。
	seedTagPG(t, db, "科技股")
	articleID := seedArticlePG(t, db, feedID, "AI 泡沫下的科技股定价逻辑", "科技股估值随利率波动", false)

	dict := NewDictCache(db)
	pre := NewQueryPreprocessor(db, dict)
	pq, err := pre.Prepare(ctx, "科技股AI泡沫")
	require.NoError(t, err)
	require.Equal(t, []string{"科技股"}, termTexts(pq.L1Words), "FMM 切出词典词，残段（AI泡沫）不产出")

	hits, err := repo.SearchKeyword(ctx, pq, ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, hits, 1, "换序标题经 L1 命中且唯一")

	hit := findHitByID(hits, articleID)
	require.NotNil(t, hit, "目标文章必须被召回")
	require.Equal(t, "AI 泡沫下的科技股定价逻辑", hit.Article.Title)

	// L1 命中明细存在（同文章可叠加 trigram 命中——各层独立召回属正常）。
	var dictHit *HitDetail
	for i := range hit.Hits {
		if hit.Hits[i].Level == LevelDictSingle {
			dictHit = &hit.Hits[i]
		}
		require.NotEqual(t, LevelWhole, hit.Hits[i].Level, "整串「科技股AI泡沫」不应子串命中换序标题")
	}
	require.NotNil(t, dictHit, "L1 词典词命中明细必须在")
	require.Equal(t, FieldTitle, dictHit.Field)
	require.Equal(t, "科技股", dictHit.Text)

	// 关键词榜内部分：dict-single × title = 0.6（白盒加权表口径）。
	require.InDelta(t, 0.6, KeywordScore(hit.Hits), 1e-9)
}

// TestSearchKeywordTrigramFallback covers S1 步 3：连续短语无词典词、不连续，
// 由 L3 trigram 兜底召回，分数区间 [0.1,0.3]（spec Scenario
// 「连续短语的 trigram 兜底」）。归档文章默认含入（步 6 同测）。
func TestSearchKeywordTrigramFallback(t *testing.T) {
	repo, db := setupSearchPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	// 无词典标签：L1 空切。
	archivedID := seedArticlePG(t, db, feedID, "特斯拉Model Y地下车库自燃", "", true)
	noiseID := seedArticlePG(t, db, feedID, "英超冬窗转会关闭", "", false)

	dict := NewDictCache(db)
	pre := NewQueryPreprocessor(db, dict)
	pq, err := pre.Prepare(ctx, "特斯拉电池自燃")
	require.NoError(t, err)
	require.Empty(t, pq.L1Words, "词典空 → L1 自动跳过（变体「无标签库」）")

	hits, err := repo.SearchKeyword(ctx, pq, ArchiveInclude)
	require.NoError(t, err)

	hit := findHitByID(hits, archivedID)
	require.NotNil(t, hit, "L3 必须召回换写标题的归档文章")
	require.True(t, hit.Article.Archived, "归档标记随行（S1 步 6）")
	require.Len(t, hit.Hits, 1, "仅 trigram 一路命中（整串/词典均不中）")
	require.Equal(t, LevelTrigram, hit.Hits[0].Level)
	require.Equal(t, FieldTitle, hit.Hits[0].Field)
	require.Equal(t, "特斯拉电池自燃", hit.Hits[0].Text)
	require.GreaterOrEqual(t, hit.Hits[0].TrigramScore, 0.1, "trigram 分数下界")
	require.LessOrEqual(t, hit.Hits[0].TrigramScore, 0.3, "trigram 分数上界")

	require.Nil(t, findHitByID(hits, noiseID), "无关文章不得混入")
}

// TestSearchKeywordArchiveFilter covers S1 步 6/7：默认 include 含归档、
// active-only 排除归档；LIKE 命中字段归属（title vs description）。
func TestSearchKeywordArchiveFilter(t *testing.T) {
	repo, db := setupSearchPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	activeID := seedArticlePG(t, db, feedID, "科技股集体高开", "", false)
	archivedID := seedArticlePG(t, db, feedID, "科技股泡沫争论再起", "", true)
	descHitID := seedArticlePG(t, db, feedID, "今日盘面综述", "科技股领涨，指数走高", false)

	pq, err := NewQueryPreprocessor(db, NewDictCache(db)).Prepare(ctx, "科技股")
	require.NoError(t, err)

	included, err := repo.SearchKeyword(ctx, pq, ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, included, 3, "默认含归档：三篇全回")
	require.NotNil(t, findHitByID(included, archivedID), "归档文章默认出现（步 6）")
	require.True(t, findHitByID(included, archivedID).Article.Archived)

	// 字段归属：标题命中 → title；仅描述命中 → description（同一文章可另叠
	// trigram 命中，各层独立，断言只锁定 whole 层的字段归属）。
	active := findHitByID(included, activeID)
	require.NotNil(t, requireHitByLevel(t, active.Hits, LevelWhole))
	require.Equal(t, FieldTitle, requireHitByLevel(t, active.Hits, LevelWhole).Field)
	descHit := findHitByID(included, descHitID)
	require.NotNil(t, descHit)
	wholeDesc := requireHitByLevel(t, descHit.Hits, LevelWhole)
	require.Equal(t, FieldDescription, wholeDesc.Field, "词只在 description → 归属 description")
	require.Equal(t, "科技股", wholeDesc.Text)

	activeOnly, err := repo.SearchKeyword(ctx, pq, ArchiveActiveOnly)
	require.NoError(t, err)
	require.Len(t, activeOnly, 2, "active-only：归档不出现（步 7）")
	require.Nil(t, findHitByID(activeOnly, archivedID))
}

// TestSearchKeywordTrigramThresholdBoundary covers 白盒「阈值边界值表」
// trigram 行。similarity 无法构造精确 0.10 的行，采用自校准：先实测分数 s，
// 再以 s-ε / s / s+ε 三档阈值断言召回边界（≥ 含等值），并验证非法配置
// 回退默认 0.10。
func TestSearchKeywordTrigramThresholdBoundary(t *testing.T) {
	repo, db := setupSearchPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)
	title := "特斯拉Model Y地下车库自燃"
	query := "特斯拉电池自燃"
	articleID := seedArticlePG(t, db, feedID, title, "", false)

	// 实测该对的 pg_trgm similarity（float4 精度内与查询期一致）。
	var measured float64
	require.NoError(t, db.Raw("SELECT similarity(?, ?)", query, title).Scan(&measured).Error)
	require.GreaterOrEqual(t, measured, 0.1, "种子对分数须落入可测区间")
	require.LessOrEqual(t, measured, 0.3)

	search := func() []ArticleHit {
		pq, err := NewQueryPreprocessor(db, NewDictCache(db)).Prepare(ctx, query)
		require.NoError(t, err)
		hits, err := repo.SearchKeyword(ctx, pq, ArchiveInclude)
		require.NoError(t, err)
		return hits
	}

	// 阈值 s-0.01：召回；阈值 s（等值）：召回（>= 含等）。
	setSettingPG(t, db, TrigramThresholdKey, strconv.FormatFloat(measured-0.01, 'f', -1, 64))
	require.NotNil(t, findHitByID(search(), articleID), "阈值低于分数 → 召回")
	setSettingPG(t, db, TrigramThresholdKey, strconv.FormatFloat(measured, 'f', -1, 64))
	require.NotNil(t, findHitByID(search(), articleID), "阈值等于分数 → 召回（>=）")

	// 阈值 s+0.01：不召回（分数越界）。
	setSettingPG(t, db, TrigramThresholdKey, strconv.FormatFloat(measured+0.01, 'f', -1, 64))
	require.Nil(t, findHitByID(search(), articleID), "阈值高于分数 → 不召回")

	// 非法配置（非数字）→ 回退默认 0.10：measured ≥ 0.1 → 仍召回。
	setSettingPG(t, db, TrigramThresholdKey, "abc")
	require.NotNil(t, findHitByID(search(), articleID), "非法配置回退默认 0.10 后按默认行为召回")
}

// TestSearchKeywordWildcardLiteralMatch covers 变体「纯通配符转义后按字面
// 匹配」的 PG 侧：含 % _ \ 的查询按字面命中，不做通配展开。
func TestSearchKeywordWildcardLiteralMatch(t *testing.T) {
	repo, db := setupSearchPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	targetID := seedArticlePG(t, db, feedID, "限时 50%_off 大促", "", false)
	// 若 % 被当通配符，这两篇（匹配任意串）也会混入。
	wildcardBaitID := seedArticlePG(t, db, feedID, "五折清仓", "", false)
	underscoreBaitID := seedArticlePG(t, db, feedID, "5xoff 秒杀", "", false)

	pq, err := NewQueryPreprocessor(db, NewDictCache(db)).Prepare(ctx, `50%_off`)
	require.NoError(t, err)

	hits, err := repo.SearchKeyword(ctx, pq, ArchiveInclude)
	require.NoError(t, err)

	// 目标文章有整串（whole）层字面命中（trigram 层可另叠，不在此断言）。
	target := findHitByID(hits, targetID)
	require.NotNil(t, target, "转义后字面命中目标")
	whole := requireHitByLevel(t, target.Hits, LevelWhole)
	require.Equal(t, `50%_off`, whole.Text)
	require.Equal(t, FieldTitle, whole.Field)

	// 饱餌文章不得有 whole 层命中（% _ 未被当通配符展开）；trigram 模糊
	// 层召回饱餌属各层独立召回的正常行为，不算通配符泄漏。
	for _, baitID := range []uint{wildcardBaitID, underscoreBaitID} {
		if bait := findHitByID(hits, baitID); bait != nil {
			require.Nil(t, hitByLevel(bait.Hits, LevelWhole),
				"饱餌 %d 只可能经 trigram 层进入，不得有整串字面命中", baitID)
		}
	}
}

// hitByLevel 返回首个指定层级的命中明细（无则 nil）。
func hitByLevel(details []HitDetail, level MatchLevel) *HitDetail {
	for i := range details {
		if details[i].Level == level {
			return &details[i]
		}
	}
	return nil
}

// requireHitByLevel 同 hitByLevel 但不存在即失败。
func requireHitByLevel(t *testing.T, details []HitDetail, level MatchLevel) *HitDetail {
	t.Helper()
	detail := hitByLevel(details, level)
	require.NotNil(t, detail, "缺少 %s 层命中明细：%v", level, details)
	return detail
}

// TestSearchKeywordEmptyDB covers 变体「前置：空库」：零文章零标签返回空
// 结果不报错，词典空 L1 自动跳过。
func TestSearchKeywordEmptyDB(t *testing.T) {
	repo, db := setupSearchPG(t)
	ctx := context.Background()

	dict := NewDictCache(db)
	pq, err := NewQueryPreprocessor(db, dict).Prepare(ctx, "特斯拉")
	require.NoError(t, err)
	require.Zero(t, dict.Size(), "空库词典为空")
	require.Empty(t, pq.L1Words)

	hits, err := repo.SearchKeyword(ctx, pq, ArchiveInclude)
	require.NoError(t, err, "空库不报错")
	require.Empty(t, hits)
}
