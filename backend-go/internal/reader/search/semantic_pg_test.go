package search

// add-article-search tasks 2.4 — 语义路径 testcontainer PG 集成测试
//（真 pgvector <=> 需真 PG；fake Embedder 注入固定查询向量）。
//
// 覆盖 test-cases.md 故事 S2 步 2/3 与白盒「阈值边界值表」cosine/top_n 边界：
//   - cosine 边界：min_cosine 0.54/0.55/0.56 三档对 ~0.55 分标签的取舍
//   - top_n 边界：第 8 名参与、第 9 名不参与（分数达标排名越界 → 不召回）
//   - embedding 失败/超时 → ErrSemanticSkipped（不报错）
//   - max cosine 聚合 + 命中标签全量 + 归档口径
//   - 无 identity 行 → 空结果而非 skipped
//
// 向量构造：黄金 schema 的 topic_tag_embeddings.embedding 列是固定维度
// vector(N)（迁移 ALTER，N 由 goldenVectorDim 实测），查询与种子向量同维。
// 查询向量取 [1,0,...,0]，目标 cosine c 的标签向量取
// [c, sqrt(1-c²), 0, ..., 0]——余弦几何与低维一致。边界值按 c=0.553
// （0.55 上方 +0.003）构造：pgvector 距离按 float4 精度计算，正落在阈值
// 上的等值向量会被舍入翻面，此偏置让三档判定稳定复现白盒表语义
// （0.54/0.55 参与、0.56 不参与）。

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// fakeEmbedder 注入替身：err 立即失败；block 挂起直到 ctx 超时。
type fakeEmbedder struct {
	vec   []float64
	err   error
	block bool
	calls int
}

func (f *fakeEmbedder) EmbedQuery(ctx context.Context, _ string) ([]float64, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.vec, nil
}

// goldenVectorDim 从黄金 schema 实测 embedding 列维度（vector(N) 的 typmod；
// 迁移 ALTER 到固定维，种子与查询向量必须同维）。
func goldenVectorDim(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var dim int
	require.NoError(t, db.Raw(`
SELECT a.atttypmod AS dim
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
WHERE c.relname = 'topic_tag_embeddings' AND a.attname = 'embedding'`).Scan(&dim).Error)
	require.Positive(t, dim)
	return dim
}

// cosineVec 返回与查询向量 [1,0,...,0] 夹角余弦恰为 c 的 dim 维向量：
// [c, sqrt(1-c²), 0, ..., 0]。边界值按 c=0.553（0.55 上方 +0.003）构造
// ——pgvector 距离按 float4 精度计算，正落在阈值上的等值向量会被舍入
// 翻面，此偏置让三档判定稳定复现白盒表语义（0.54/0.55 参与、0.56 不参与）。
func cosineVec(c float64, dim int) string {
	parts := make([]string, dim)
	for i := range parts {
		parts[i] = "0"
	}
	parts[0] = fmt.Sprintf("%f", c)
	parts[1] = fmt.Sprintf("%f", math.Sqrt(1-c*c))
	return "[" + strings.Join(parts, ",") + "]"
}

// queryVec 返回 dim 维查询向量 [1,0,...,0]。
func queryVec(dim int) []float64 {
	vec := make([]float64, dim)
	vec[0] = 1
	return vec
}

func setupSemanticPG(t *testing.T) (*SemanticSearcher, *gorm.DB, *fakeEmbedder) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	embedder := &fakeEmbedder{vec: queryVec(goldenVectorDim(t, db))}
	return NewSemanticSearcher(db, embedder), db, embedder
}

func seedTagWithEmbeddingPG(t *testing.T, db *gorm.DB, label string, c float64, embeddingType string, seq int) uint {
	t.Helper()
	tagID := seedTagPG(t, db, label)
	require.NoError(t, db.Exec(`
INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, embedding, dimension, model, text_hash, created_at, updated_at)
VALUES (?, ?, ?::vector, ?, 'fake-model', ?, NOW(), NOW())`,
		tagID, embeddingType, cosineVec(c, goldenVectorDim(t, db)), goldenVectorDim(t, db), fmt.Sprintf("hash-%s-%d", label, seq)).Error,
		"seed embedding for %s", label)
	return tagID
}

func linkTagToArticlePG(t *testing.T, db *gorm.DB, tagID, articleID uint) {
	t.Helper()
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: articleID, TopicTagID: tagID, Source: "llm"}).Error)
}

func tagCosineByLabel(tags []MatchedTag) map[string]float64 {
	out := make(map[string]float64, len(tags))
	for _, tag := range tags {
		out[tag.Label] = tag.Cosine
	}
	return out
}

// TestSemanticSearchCosineBoundary covers 白盒 cosine 0.54/0.55/0.56 行：
// ~0.553 分标签在阈值 0.54、0.55 时参与召回，0.56 时不参与（分数越界）。
func TestSemanticSearchCosineBoundary(t *testing.T) {
	s, db, _ := setupSemanticPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	highID := seedTagWithEmbeddingPG(t, db, "电动车", 0.9, "identity", 1)
	highArticle := seedArticlePG(t, db, feedID, "电动车行业年度盘点", "", false)
	linkTagToArticlePG(t, db, highID, highArticle)

	midID := seedTagWithEmbeddingPG(t, db, "电池安全", 0.553, "identity", 2)
	midArticle := seedArticlePG(t, db, feedID, "电池安全事件复盘", "", false)
	linkTagToArticlePG(t, db, midID, midArticle)

	search := func() ([]SemanticHit, []MatchedTag) {
		hits, tags, err := s.Search(ctx, "电动车电池安全", ArchiveInclude)
		require.NoError(t, err)
		return hits, tags
	}

	// 默认（无配置键）= 0.55：0.553 参与双标签。
	_, tags := search()
	require.Len(t, tags, 2, "默认阈值 0.55：0.553 分标签参与")
	require.InDelta(t, 0.553, tagCosineByLabel(tags)["电池安全"], 0.002)

	for _, threshold := range []string{"0.54", "0.55"} {
		setSettingPG(t, db, SemanticMinCosineKey, threshold)
		hits, tags := search()
		require.Len(t, tags, 2, "阈值 %s：0.553 分标签参与", threshold)
		require.Len(t, hits, 2)
		require.NotNil(t, findSemanticHit(hits, midArticle), "阈值 %s：mid 文章召回", threshold)
	}

	// 阈值 0.56：0.553 分数越界 → 标签与其文章均不参与。
	setSettingPG(t, db, SemanticMinCosineKey, "0.56")
	hits, tags := search()
	require.Len(t, tags, 1, "阈值 0.56：只剩高分标签")
	require.NotContains(t, tagCosineByLabel(tags), "电池安全")
	require.Nil(t, findSemanticHit(hits, midArticle), "分数越界标签的文章不召回")
}

// TestSemanticSearchTopNBoundary covers 白盒 top_n 第 8/9 名行：9 个分数
// 均达标（≥0.55）的标签，默认 top_n=8 只取前 8——第 9 名分数达标但排名
// 越界 → 不召回（spec「双重约束边界」）。
func TestSemanticSearchTopNBoundary(t *testing.T) {
	s, db, _ := setupSemanticPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	// 9 个标签 cosine 0.90 → 0.74（步长 0.02，全部 ≥ 0.55），各挂一篇文章。
	cosines := []float64{0.90, 0.88, 0.86, 0.84, 0.82, 0.80, 0.78, 0.76, 0.74}
	for i, c := range cosines {
		tagID := seedTagWithEmbeddingPG(t, db, fmt.Sprintf("标签%d", i+1), c, "identity", i+1)
		articleID := seedArticlePG(t, db, feedID, fmt.Sprintf("文章%d", i+1), "", false)
		linkTagToArticlePG(t, db, tagID, articleID)
	}

	hits, tags, err := s.Search(ctx, "概念查询", ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, tags, 8, "默认 top_n=8：第 9 名（0.74 分达标）排名越界不参与")
	require.Len(t, hits, 8)
	ninthArticle := findSemanticHit(hits, mustArticleIDByTitle(t, db, "文章9"))
	require.Nil(t, ninthArticle, "第 9 名标签的文章不召回")
	eighth := findSemanticHit(hits, mustArticleIDByTitle(t, db, "文章8"))
	require.NotNil(t, eighth, "第 8 名参与")

	// top_n=9：全部参与。
	setSettingPG(t, db, SemanticTopNKey, "9")
	hits, tags, err = s.Search(ctx, "概念查询", ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, tags, 9)
	require.Len(t, hits, 9)
}

// TestSemanticSearchSkippedOnEmbeddingFailure covers spec
// 「embedding 不可用降级」：Embed 失败 → ErrSemanticSkipped 哨兵、不报错。
func TestSemanticSearchSkippedOnEmbeddingFailure(t *testing.T) {
	s, _, embedder := setupSemanticPG(t)
	embedder.err = errors.New("provider down")

	hits, tags, err := s.Search(context.Background(), "任意查询", ArchiveInclude)
	require.ErrorIs(t, err, ErrSemanticSkipped)
	require.Nil(t, hits)
	require.Nil(t, tags)
	require.Equal(t, 1, embedder.calls)
}

// TestSemanticSearchTimeoutSkipped covers 白盒「语义超时 3s」：超时即弃语义
// 路（skipped）。默认 3s 超时在 Search 内部施加；测试白盒调短避免真等 3s。
func TestSemanticSearchTimeoutSkipped(t *testing.T) {
	s, _, embedder := setupSemanticPG(t)
	embedder.block = true
	s.timeout = 50 * time.Millisecond

	start := time.Now()
	hits, tags, err := s.Search(context.Background(), "慢查询", ArchiveInclude)
	require.ErrorIs(t, err, ErrSemanticSkipped, "超时 → skipped 哨兵（不报错弹给用户）")
	require.Nil(t, hits)
	require.Nil(t, tags)
	require.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)
	require.Less(t, time.Since(start), 3*time.Second, "不得拖满默认 3s 才返回")
}

// TestSemanticSearchMaxAggregationAndArchive covers design D3 max 聚合
// （文章语义分 = 命中标签 max cosine、全量标签进 hit_reasons 原料）与归档口径。
func TestSemanticSearchMaxAggregationAndArchive(t *testing.T) {
	s, db, _ := setupSemanticPG(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	strongTag := seedTagWithEmbeddingPG(t, db, "电动车", 0.9, "identity", 1)
	weakTag := seedTagWithEmbeddingPG(t, db, "充电桩", 0.7, "identity", 2)
	multiTagArticle := seedArticlePG(t, db, feedID, "双标签文章", "", false)
	linkTagToArticlePG(t, db, strongTag, multiTagArticle)
	linkTagToArticlePG(t, db, weakTag, multiTagArticle)

	archivedTag := seedTagWithEmbeddingPG(t, db, "储能", 0.8, "identity", 3)
	archivedArticle := seedArticlePG(t, db, feedID, "归档文章", "", true)
	linkTagToArticlePG(t, db, archivedTag, archivedArticle)

	hits, tags, err := s.Search(ctx, "概念查询", ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, tags, 3)

	multi := findSemanticHit(hits, multiTagArticle)
	require.NotNil(t, multi)
	require.InDelta(t, 0.9, multi.Cosine, 0.002, "文章语义分 = max cosine")
	require.Equal(t, "电动车", multi.Tag.Label, "最高分标签为主词条")
	require.Len(t, multi.MatchedTags, 2, "命中标签全量保留")

	archived := findSemanticHit(hits, archivedArticle)
	require.NotNil(t, archived, "默认含归档")
	require.True(t, archived.Article.Archived, "归档标记随行")
	require.Equal(t, "归档文章", archived.Article.Title, "行投影字段完整（Title 示样）")
	require.Equal(t, "储能", archived.Tag.Label, "命中标签主词条完整")

	hits, _, err = s.Search(ctx, "概念查询", ArchiveActiveOnly)
	require.NoError(t, err)
	require.Nil(t, findSemanticHit(hits, archivedArticle), "active-only：归档文章不召回")
	require.NotNil(t, findSemanticHit(hits, multiTagArticle))
}

// TestSemanticSearchNoIdentityRows covers 变体「topic_tag_embeddings 无
// identity 行」：零命中但不是 skipped（semantic 块为空标签列表）。
func TestSemanticSearchNoIdentityRows(t *testing.T) {
	s, db, _ := setupSemanticPG(t)
	feedID := seedFeedPG(t, db)

	// 只有 semantic 型向量与无向量标签：identity 路径空。
	seedTagWithEmbeddingPG(t, db, "只有semantic型", 0.99, "semantic", 1)
	seedTagPG(t, db, "没有向量")
	seedArticlePG(t, db, feedID, "无语义边的文章", "", false)

	hits, tags, err := s.Search(context.Background(), "概念查询", ArchiveInclude)
	require.NoError(t, err, "零命中不是错误")
	require.Empty(t, hits)
	require.Empty(t, tags)
	require.NotErrorIs(t, err, ErrSemanticSkipped)
}

func findSemanticHit(hits []SemanticHit, articleID uint) *SemanticHit {
	for i := range hits {
		if hits[i].Article.ID == articleID {
			return &hits[i]
		}
	}
	return nil
}

func mustArticleIDByTitle(t *testing.T, db *gorm.DB, title string) uint {
	t.Helper()
	var id uint
	require.NoError(t, db.Model(&models.Article{}).Where("title = ?", title).Pluck("id", &id).Error)
	require.NotZero(t, id, "article %s must exist", title)
	return id
}
