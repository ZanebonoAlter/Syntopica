package core

// speed-up-tag-embedding 任务 1.2/1.3（test-cases.md 步 2/3 + 白盒 typmod
// 失配回退）：两段式相似查询在真实 PG + pgvector 下——halfvec HNSW 索引召回
// + float32 精排 top10 与全量精确排序 overlap=10/10；EXPLAIN 确认候选池走
// Index Scan；无索引环境（索引未建）仍返回正确结果（慢但不错）。
//
// 索引建/删只发生在 testcontainer 隔离库内，测试结束 DROP 还原（🛑 不碰真库）。

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

const testVecDim = 2560

// createRecallIndexes 在测试容器内建三类 partial halfvec HNSW 索引（镜像
// scripts/db/tag-embedding-halfvec-hnsw-20261104.sql 的 DDL，typmod 必须带维度），
// 返回清理函数。
func createRecallIndexes(t *testing.T, db *gorm.DB, dim int) func() {
	t.Helper()
	names := []string{
		"idx_tte_semantic_halfvec_hnsw_test",
		"idx_tte_identity_halfvec_hnsw_test",
		"idx_tte_event_keyword_halfvec_hnsw_test",
	}
	ddls := []string{
		"semantic", "identity", "event_keyword",
	}
	for i, typ := range ddls {
		ddl := fmt.Sprintf(
			"CREATE INDEX %s ON topic_tag_embeddings USING hnsw ((embedding::halfvec(%d)) halfvec_cosine_ops) WHERE embedding_type = '%s' AND embedding IS NOT NULL",
			names[i], dim, typ)
		if err := db.Exec("SET max_parallel_maintenance_workers = 0").Error; err != nil {
			t.Fatalf("set parallel maintenance: %v", err)
		}
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create halfvec index for %s: %v", typ, err)
		}
	}
	return func() {
		for _, name := range names {
			_ = db.Exec("DROP INDEX IF EXISTS " + name).Error
		}
	}
}

func seedRecallVectors(t *testing.T, db *gorm.DB, n int) [][]float64 {
	t.Helper()
	// 生产库的 embedding 列是 vector(2560)（黄金 schema 为无 typmod 的 vector，
	// 会拖 planner 不走 HNSW）；对齐生产列形态再建索引测案才有代表性。
	// 只影响 testcontainer 隔离库，ResetTestData 会还原列维度。
	if err := db.Exec(fmt.Sprintf("ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector(%d)", testVecDim)).Error; err != nil {
		t.Fatalf("alter embedding column: %v", err)
	}
	rng := rand.New(rand.NewSource(42))
	vectors := make([][]float64, 0, n)
	for i := 0; i < n; i++ {
		tag := models.TopicTag{
			Label:    fmt.Sprintf("召回标签-%03d", i),
			Category: "keyword",
			Kind:     "topic",
			Slug:     Slugify(fmt.Sprintf("recall-tag-%03d", i)),
			Status:   "active",
		}
		if err := db.Create(&tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
		vec := make([]float64, testVecDim)
		var norm float64
		for d := range vec {
			// 拉大向量间距离差异，避免 top10 边界全是近平局（halfvec 舍入噪声）。
			vec[d] = rng.NormFloat64()
			norm += vec[d] * vec[d]
		}
		norm = math.Sqrt(norm)
		for d := range vec {
			vec[d] /= norm
		}
		vectors = append(vectors, vec)
		if err := db.Create(&tagmodels.TopicTagEmbedding{
			TopicTagID:    tag.ID,
			EmbeddingType: EmbeddingTypeSemantic,
			EmbeddingVec:  FloatsToPgVector(vec),
			Dimension:     testVecDim,
			Model:         "test-model",
			TextHash:      fmt.Sprintf("recall-hash-%03d", i),
		}).Error; err != nil {
			t.Fatalf("create embedding: %v", err)
		}
	}
	return vectors
}

// bruteForceTop10 在 Go 侧做全量 float32（近似 float64）余弦排序，作为基准。
func bruteForceTop10(t *testing.T, db *gorm.DB, probe []float64, limit int) []uint {
	t.Helper()
	type row struct {
		TagID uint
		Vec   string
	}
	var rows []row
	if err := db.Table("topic_tag_embeddings").
		Select("topic_tag_id AS tag_id, embedding::text AS vec").
		Where("embedding_type = ?", EmbeddingTypeSemantic).
		Scan(&rows).Error; err != nil {
		t.Fatalf("load all embeddings: %v", err)
	}
	type scored struct {
		id  uint
		sim float64
	}
	scoredRows := make([]scored, 0, len(rows))
	for _, r := range rows {
		floats := parsePgVector(t, r.Vec)
		sim := cosine(probe, floats)
		scoredRows = append(scoredRows, scored{r.TagID, sim})
	}
	// 简单选择排序取 top limit。
	for i := 0; i < limit && i < len(scoredRows); i++ {
		for j := i + 1; j < len(scoredRows); j++ {
			if scoredRows[j].sim > scoredRows[i].sim {
				scoredRows[i], scoredRows[j] = scoredRows[j], scoredRows[i]
			}
		}
	}
	out := make([]uint, 0, limit)
	for i := 0; i < limit && i < len(scoredRows); i++ {
		out = append(out, scoredRows[i].id)
	}
	return out
}

func parsePgVector(t *testing.T, s string) []float64 {
	t.Helper()
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	parts := strings.Split(s, ",")
	out := make([]float64, len(parts))
	var err error
	for i, p := range parts {
		if _, err = fmt.Sscanf(strings.TrimSpace(p), "%f", &out[i]); err != nil {
			t.Fatalf("parse vector component %q: %v", p, err)
		}
	}
	return out
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func TestFindSimilarByVectorRecallAgainstBruteForce(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	vectors := seedRecallVectors(t, db, 60)
	drop := createRecallIndexes(t, db, testVecDim)
	t.Cleanup(drop)

	svc := NewEmbeddingService()

	// 20 探针（含首尾），每探针 top10 overlap 必须 10/10。
	for p := 0; p < 20; p++ {
		idx := p * 3 % len(vectors)
		probe := vectors[idx]
		want := bruteForceTop10(t, db, probe, 10)
		got, err := svc.findSimilarByVector(FloatsToPgVector(probe), "keyword", EmbeddingTypeSemantic, 10, testVecDim)
		if err != nil {
			t.Fatalf("probe %d: findSimilarByVector: %v", p, err)
		}
		if len(got) != 10 {
			t.Fatalf("probe %d: got %d candidates, want 10", p, len(got))
		}
		gotSet := map[uint]bool{}
		for _, c := range got {
			gotSet[c.Tag.ID] = true
		}
		overlap := 0
		for _, id := range want {
			if gotSet[id] {
				overlap++
			}
		}
		if overlap < 10 {
			t.Fatalf("probe %d: overlap = %d/10 (want 10/10)", p, overlap)
		}
	}
}

func TestFindSimilarByVectorUsesIndexScan(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	seedRecallVectors(t, db, 20)
	drop := createRecallIndexes(t, db, testVecDim)
	t.Cleanup(drop)

	probe := make([]float64, testVecDim)
	for d := range probe {
		probe[d] = 0.01
	}
	pgVec := FloatsToPgVector(probe)

	// 候选池 SQL 与 findSimilarByVector 第一段同口径（typmod 必须带维度）。
	candSQL := fmt.Sprintf(`
		SELECT e.topic_tag_id AS tag_id
		FROM topic_tag_embeddings e
		JOIN topic_tags t ON t.id = e.topic_tag_id
		WHERE t.category = ?
		  AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
		  AND e.embedding IS NOT NULL
		  AND e.embedding_type = ?
		ORDER BY e.embedding::halfvec(%d) <=> ?::halfvec(%d)
		LIMIT ?`, testVecDim, testVecDim)

	var plan []string
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL hnsw.ef_search = 120").Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
			return err
		}
		// 小表上 btree+Sort 与 HNSW 成本平手（生产 2 万行 HNSW 必胜）；
		// 关掉 sort 通道逼出「有序索引路径」，验证 HNSW 索引能提供该排序。
		if err := tx.Exec("SET LOCAL enable_sort = off").Error; err != nil {
			return err
		}
		var planRows []string
		if err := tx.Raw("EXPLAIN "+candSQL, "keyword", EmbeddingTypeSemantic, pgVec, 50).Scan(&planRows).Error; err != nil {
			return err
		}
		plan = append(plan, planRows...)
		return nil
	})
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	joined := strings.Join(plan, "\n")
	if !strings.Contains(joined, "Index Scan") {
		t.Fatalf("candidate pool query did not use an index scan:\n%s", joined)
	}
	if !strings.Contains(joined, "halfvec_hnsw") {
		t.Logf("PLAN:\n%s", joined); t.Fatalf("plan does not mention the halfvec HNSW index")
	}
}

// 无索引环境（typmod 失配回退）：查询仍正确（seq scan 慢但不错）。
func TestFindSimilarByVectorWithoutIndexStillCorrect(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	vectors := seedRecallVectors(t, db, 15)
	// 不建索引。

	svc := NewEmbeddingService()
	probe := vectors[0]
	want := bruteForceTop10(t, db, probe, 10)
	got, err := svc.findSimilarByVector(FloatsToPgVector(probe), "keyword", EmbeddingTypeSemantic, 10, testVecDim)
	if err != nil {
		t.Fatalf("findSimilarByVector without index: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d candidates, want 10", len(got))
	}
	if got[0].Tag.ID != want[0] {
		t.Fatalf("top1 = %d, want %d (self-match)", got[0].Tag.ID, want[0])
	}
	gotSet := map[uint]bool{}
	for _, c := range got {
		gotSet[c.Tag.ID] = true
	}
	overlap := 0
	for _, id := range want {
		if gotSet[id] {
			overlap++
		}
	}
	if overlap < 10 {
		t.Fatalf("overlap = %d/10 without index (exact path must be identical)", overlap)
	}
}
