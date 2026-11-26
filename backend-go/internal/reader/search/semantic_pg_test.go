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
// optimize-search-semantic-index tasks 4.1 增量（ANN 召回）：
//   - halfvec identity 部分 HNSW 索引存在且 indisvalid=true（dim=2560 生产维度）
//   - 同 tag 多 text_hash 行去重取 max（design D2 Go 侧聚合）
//   - 160 标签 top-8 与暴力精确 cosine top-8 对账（含直接调 matchTagsANN 的
//     白盒断言——确保用例真的走 ANN，而不是静默回退到旧全表扫描）
//
// 注意：黄金 schema 的 embedding 列是迁移 20260403_0003 的 staged 值 vector(4096)，
// 超过 halfvec 的 4000 维上限——索引建不了、ANN 路径不会触发。所以既有用例
// （黄金维度）实际覆盖的是 matchTagsExact 回退路径，ANN 三例自行降到生产
// 运行时维度 semanticTestVectorDim（2560）并跑生产迁移的 Up 建索引。
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
	"syntopica-backend/internal/platform/database"
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

// ─────────────────────────────────────────────────────────────────────────
// optimize-search-semantic-index tasks 4.1：ANN 候选召回（halfvec HNSW）
// ─────────────────────────────────────────────────────────────────────────

// semanticTestVectorDim 语义 ANN 用例的列维度：镜像生产运行时维度
// （embedding_config 的 embedding_dimension=2560）。黄金 schema 默认是 4096
// （staged cutover 值），超 halfvec 4000 上限，索引建不了也不会走 ANN。
const semanticTestVectorDim = 2560

// setupSemanticPGScaled 把 embedding 列降到生产运行时维度，并让生产迁移
// 20261023_0001 真正重跑一次建 halfvec 索引（测真实 DDL + 版本记账，不自造
// 索引）。SetupTestDB 的重置会把列还原成黄金维度，所以每个 ANN 用例自行降维
// + 建索引（空表 ALTER 瞬时完成）。
func setupSemanticPGScaled(t *testing.T) (*SemanticSearcher, *gorm.DB, *fakeEmbedder) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	goldenDim := goldenVectorDim(t, db)
	// 旧 float32 HNSW 索引（若有）依赖旧 typmod，且 2560 维超出其 2000 上限，
	// 先 drop 再 ALTER（与 ensureVectorDimension 的顺序一致）。
	require.NoError(t, db.Exec("DROP INDEX IF EXISTS idx_topic_tag_embeddings_embedding").Error)
	require.NoError(t, db.Exec(fmt.Sprintf(
		"ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector(%d)", semanticTestVectorDim)).Error)
	require.NoError(t, rerunSemanticHalfvecIndexMigration(db))
	// 共享测试库的下一次 SetupTestDB 会把列 ALTER 回黄金维度；halfvec 表达式
	// 索引依赖旧 typmod，必须在还原前 drop，否则 ALTER 会因维度不匹配失败。
	t.Cleanup(func() {
		_ = db.Exec("DROP INDEX IF EXISTS " + database.TopicTagIdentityHalfvecIndexName).Error
		_ = db.Exec(fmt.Sprintf(
			"ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector(%d)", goldenDim)).Error
	})
	embedder := &fakeEmbedder{vec: queryVec(semanticTestVectorDim)}
	return NewSemanticSearcher(db, embedder), db, embedder
}

// rerunSemanticHalfvecIndexMigration 让生产迁移 20261023_0001 重新执行一次：
// 黄金 schema 构建时列还是 vector(4096)（超 halfvec 上限），迁移按设计跳过并
// 记录了版本；删掉版本记录后跑 database.RunMigrations，走的就是部署首次启动
// 的同一条 in-tx 路径（含 SET LOCAL maintenance_work_mem）。
func rerunSemanticHalfvecIndexMigration(db *gorm.DB) error {
	if err := db.Exec("DELETE FROM schema_migrations WHERE version = '20261023_0001'").Error; err != nil {
		return err
	}
	return database.RunMigrations(db)
}

// TestSemanticSearchCandidateIndexExistsPG covers test-cases.md 主链路步 1：
// 迁移在运行时维度上生成 halfvec identity 部分 HNSW 索引且 indisvalid=true；
// 重复执行幂等（IF NOT EXISTS 不重复建）。
func TestSemanticSearchCandidateIndexExistsPG(t *testing.T) {
	_, db, _ := setupSemanticPGScaled(t)

	type indexState struct {
		Indisvalid bool   `gorm:"column:indisvalid"`
		Indexdef   string `gorm:"column:indexdef"`
	}
	readIndex := func() []indexState {
		var rows []indexState
		require.NoError(t, db.Raw(`
SELECT i.indisvalid AS indisvalid, pg_get_indexdef(i.indexrelid) AS indexdef
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
JOIN pg_class t ON t.oid = i.indrelid
WHERE c.relname = ? AND t.relname = 'topic_tag_embeddings'`,
			database.TopicTagIdentityHalfvecIndexName).Scan(&rows).Error)
		return rows
	}

	rows := readIndex()
	require.Len(t, rows, 1, "halfvec identity 索引必须存在且唯一")
	require.True(t, rows[0].Indisvalid, "索引必须 valid（INVALID 索引查询退化回 Seq Scan）")
	require.Contains(t, rows[0].Indexdef, "halfvec_cosine_ops", "必须是 halfvec cosine opclass")
	require.Contains(t, rows[0].Indexdef, fmt.Sprintf("halfvec(%d)", semanticTestVectorDim), "表达式维度跟随列维度")
	require.Contains(t, rows[0].Indexdef, "embedding_type", "必须是 identity 部分索引")
	require.Contains(t, rows[0].Indexdef, "identity", "谓词锁定 identity 行")

	// 幂等：再次执行迁移（如部署重试/重启）不重复建、不破坏已有索引。
	require.NoError(t, rerunSemanticHalfvecIndexMigration(db))
	require.Len(t, readIndex(), 1)
}

// TestSemanticSearchDedupesMultipleIdentityRowsPG covers test-cases.md 主链路
// 步 4：同一标签多条 identity 行（重嵌过渡期的不同 text_hash）只返回一条
// MatchedTag，分数取 max cosine（design D2 Go 侧聚合）。
func TestSemanticSearchDedupesMultipleIdentityRowsPG(t *testing.T) {
	s, db, _ := setupSemanticPGScaled(t)
	feedID := seedFeedPG(t, db)

	tagID := seedTagPG(t, db, "电动车")
	articleID := seedArticlePG(t, db, feedID, "电动车双哈希", "", false)
	linkTagToArticlePG(t, db, tagID, articleID)

	for i, c := range []float64{0.7, 0.9} {
		require.NoError(t, db.Exec(`
INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, embedding, dimension, model, text_hash, created_at, updated_at)
VALUES (?, 'identity', ?::vector, ?, 'fake-model', ?, NOW(), NOW())`,
			tagID, cosineVec(c, semanticTestVectorDim), semanticTestVectorDim,
			fmt.Sprintf("dedupe-hash-%d", i)).Error)
	}

	hits, tags, err := s.Search(context.Background(), "电动车", ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, tags, 1, "同 tag 多 text_hash 行只返回一条 MatchedTag")
	require.Equal(t, tagID, tags[0].TagID)
	require.InDelta(t, 0.9, tags[0].Cosine, 0.002, "去重取 max cosine")

	require.Len(t, hits, 1)
	require.InDelta(t, 0.9, hits[0].Cosine, 0.002, "文章语义分同样取 max")
	require.Len(t, hits[0].MatchedTags, 1, "命中标签不重复")
}

// TestSemanticSearchANNMatchesBruteForcePG covers test-cases.md 主链路步 2/5：
// 160 个标签（cosine 0.99→0.60 递减，全部达标）各挂一篇文章；ANN 只取
// LIMIT 128 候选，返回的 top-8 必须与暴力精确 cosine 排序的 top-8 一致
// （标签与分数都对账，容差 0.002）。
func TestSemanticSearchANNMatchesBruteForcePG(t *testing.T) {
	s, db, _ := setupSemanticPGScaled(t)
	ctx := context.Background()
	feedID := seedFeedPG(t, db)

	const tagCount = 160
	for i := 0; i < tagCount; i++ {
		c := 0.99 - float64(i)*(0.99-0.60)/float64(tagCount-1)
		tagID := seedTagWithEmbeddingPG(t, db, fmt.Sprintf("ANN标签%03d", i), c, "identity", i+1)
		articleID := seedArticlePG(t, db, feedID, fmt.Sprintf("ANN文章%03d", i), "", false)
		linkTagToArticlePG(t, db, tagID, articleID)
	}

	// 白盒断言：直接调 matchTagsANN，确认新路径真的可用（若 halfvec 表达式/
	// 索引有问题，这里会先红，而不是靠 Search 的静默回退把用例刷绿）。
	annTags, err := s.matchTagsANN(ctx, queryVec(semanticTestVectorDim),
		floatsToPgVector(queryVec(semanticTestVectorDim)),
		SemanticConfig{MinCosine: DefaultSemanticMinCosine, TopN: DefaultSemanticTopN})
	require.NoError(t, err, "ANN 候选 + 精确复算必须可用")
	require.Len(t, annTags, DefaultSemanticTopN)

	hits, tags, err := s.Search(ctx, "概念查询", ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, tags, DefaultSemanticTopN)
	require.Len(t, hits, DefaultSemanticTopN)

	// 暴力精确对账：原始 float32 cosine 全表排序（不经索引），取前 8。
	type bruteRow struct {
		Label  string  `gorm:"column:label"`
		Cosine float64 `gorm:"column:cosine"`
	}
	var brute []bruteRow
	require.NoError(t, db.Raw(`
SELECT t.label AS label, MAX(1 - (e.embedding <=> ?::vector)) AS cosine
FROM topic_tag_embeddings e
JOIN topic_tags t ON t.id = e.topic_tag_id
WHERE e.embedding_type = 'identity'
GROUP BY t.id, t.label
ORDER BY cosine DESC
LIMIT ?`, floatsToPgVector(queryVec(semanticTestVectorDim)), DefaultSemanticTopN).Scan(&brute).Error)
	require.Len(t, brute, DefaultSemanticTopN)

	for i := range brute {
		require.Equal(t, brute[i].Label, tags[i].Label, "第 %d 名标签与暴力精确结果一致", i+1)
		require.InDelta(t, brute[i].Cosine, tags[i].Cosine, 0.002, "第 %d 名分数与暴力精确结果一致", i+1)
	}
}
