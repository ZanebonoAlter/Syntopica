package database_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// setupDiscoveryRunModelsDB 照 internal/dataenrichment/model_test.go 的 sqlite 内存惯例，
// 仅 AutoMigrate 本切片的五个模型（improve-discovery-recommendations D2/D3/D6）。
// type:vector / type:jsonb 列与 PreferenceVector 同写法，sqlite 下可正常建表。
func setupDiscoveryRunModelsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.DiscoveryRun{},
		&models.DiscoveryRunItem{},
		&models.DiscoveryInterestEntry{},
		&models.CandidatePreference{},
		&models.CandidateEmbedding{},
	))
	return db
}

func discoveryRunModelTables() []string {
	return []string{
		"discovery_runs",
		"discovery_run_items",
		"discovery_interest_entries",
		"candidate_preferences",
		"candidate_embeddings",
	}
}

// TestDiscoveryRunModelsAutoMigrate：五表可建、可查询；vector 列按 column:embedding 落列名。
func TestDiscoveryRunModelsAutoMigrate(t *testing.T) {
	db := setupDiscoveryRunModelsDB(t)

	for _, table := range discoveryRunModelTables() {
		var count int64
		require.NoError(t, db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count).Error,
			"table %s should exist and be queryable", table)
	}

	// vector 列名必须落为 embedding（column tag 生效），发现链路 SQL 以此列名检索。
	for _, table := range []string{"discovery_interest_entries", "candidate_embeddings"} {
		rows, err := db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", table)).Rows()
		require.NoError(t, err)
		hasEmbedding := false
		for rows.Next() {
			var cid int
			var name, ctype string
			var notNull int
			var dflt any
			var pk int
			require.NoError(t, rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk))
			if name == "embedding" {
				hasEmbedding = true
			}
		}
		require.NoError(t, rows.Close())
		require.True(t, hasEmbedding, "%s should have embedding column", table)
	}
}

// TestDiscoveryRunModelsRoundTrip：字段完整落库回读（vector 字符串 / jsonb map / 可空指针）。
func TestDiscoveryRunModelsRoundTrip(t *testing.T) {
	db := setupDiscoveryRunModelsDB(t)
	now := time.Now().UTC().Truncate(time.Second)

	finished := now
	run := models.DiscoveryRun{
		RequestKey: "rk-roundtrip", Kind: "ask", Query: "科技新闻 RSS",
		Status: "succeeded", ErrorCode: "", StartedAt: now, FinishedAt: &finished,
	}
	require.NoError(t, db.Create(&run).Error)

	item := models.DiscoveryRunItem{
		RunID: run.ID, CandidateID: 7, Rank: 1,
		RecallSources:  models.MetadataMap{"board": []string{"hit"}, "seed": []string{"ask"}},
		ReasonSnapshot: "与查询高度相关的开发资讯源",
	}
	require.NoError(t, db.Create(&item).Error)

	entry := models.DiscoveryInterestEntry{
		RunID: &run.ID, QueryText: "科技新闻 RSS",
		EmbeddingVec: "[1,2,3]", Dimension: 3, Model: "test-embed",
	}
	require.NoError(t, db.Create(&entry).Error)

	pref := models.CandidatePreference{CandidateID: 7, ExcludedAt: &finished, SnoozedUntil: &finished}
	require.NoError(t, db.Create(&pref).Error)

	emb := models.CandidateEmbedding{
		CandidateID: 7, Model: "test-embed", Dimension: 3, TextHash: "hash-1",
		EmbeddingVec: "[1,2,3]",
	}
	require.NoError(t, db.Create(&emb).Error)

	var gotRun models.DiscoveryRun
	require.NoError(t, db.First(&gotRun, run.ID).Error)
	require.Equal(t, "rk-roundtrip", gotRun.RequestKey)
	require.Equal(t, "succeeded", gotRun.Status)
	require.NotNil(t, gotRun.FinishedAt)

	var gotItem models.DiscoveryRunItem
	require.NoError(t, db.First(&gotItem, item.ID).Error)
	require.Equal(t, run.ID, gotItem.RunID)
	require.Equal(t, uint(7), gotItem.CandidateID)
	require.Len(t, gotItem.RecallSources["board"], 1)
	require.Len(t, gotItem.RecallSources["seed"], 1)
	require.Equal(t, "与查询高度相关的开发资讯源", gotItem.ReasonSnapshot)

	var gotEntry models.DiscoveryInterestEntry
	require.NoError(t, db.First(&gotEntry, entry.ID).Error)
	require.NotNil(t, gotEntry.RunID)
	require.Equal(t, run.ID, *gotEntry.RunID)
	require.Equal(t, "[1,2,3]", gotEntry.EmbeddingVec)
	require.Equal(t, 3, gotEntry.Dimension)
	require.Equal(t, "test-embed", gotEntry.Model)
	require.Nil(t, gotEntry.BoardID)
	require.Equal(t, "active", gotEntry.Status)

	var gotPref models.CandidatePreference
	require.NoError(t, db.First(&gotPref, pref.ID).Error)
	require.Equal(t, uint(7), gotPref.CandidateID)
	require.NotNil(t, gotPref.ExcludedAt)
	require.NotNil(t, gotPref.SnoozedUntil)

	var gotEmb models.CandidateEmbedding
	require.NoError(t, db.First(&gotEmb, emb.ID).Error)
	require.Equal(t, "[1,2,3]", gotEmb.EmbeddingVec)
	require.Equal(t, "hash-1", gotEmb.TextHash)
}

// TestDiscoveryRunModelsUniqueConstraints：设计要求的唯一索引在 sqlite 下全部生效。
func TestDiscoveryRunModelsUniqueConstraints(t *testing.T) {
	t.Run("discovery_runs request_key 唯一（并发同 key 复用同运行，D2）", func(t *testing.T) {
		db := setupDiscoveryRunModelsDB(t)
		require.NoError(t, db.Create(&models.DiscoveryRun{RequestKey: "rk-dup", Kind: "ask", Status: "running", StartedAt: time.Now()}).Error)
		err := db.Create(&models.DiscoveryRun{RequestKey: "rk-dup", Kind: "refresh", Status: "running", StartedAt: time.Now()}).Error
		require.Error(t, err, "重复 request_key 必须被唯一索引拒绝")
	})

	t.Run("discovery_run_items run_id+candidate_id 联合唯一", func(t *testing.T) {
		db := setupDiscoveryRunModelsDB(t)
		require.NoError(t, db.Create(&models.DiscoveryRunItem{RunID: 1, CandidateID: 1, Rank: 1}).Error)
		require.NoError(t, db.Create(&models.DiscoveryRunItem{RunID: 1, CandidateID: 2, Rank: 2}).Error) // 同 run 不同候选合法
		require.NoError(t, db.Create(&models.DiscoveryRunItem{RunID: 2, CandidateID: 1, Rank: 1}).Error) // 同候选进不同 run 合法
		err := db.Create(&models.DiscoveryRunItem{RunID: 1, CandidateID: 1, Rank: 3}).Error
		require.Error(t, err, "同 run 同候选必须被联合唯一索引拒绝")
	})

	t.Run("discovery_interest_entries run_id 唯一（一 run 一种子，D3）", func(t *testing.T) {
		db := setupDiscoveryRunModelsDB(t)
		runA, runB := uint(1), uint(2)
		require.NoError(t, db.Create(&models.DiscoveryInterestEntry{RunID: &runA, QueryText: "体育", Dimension: 3}).Error)
		require.NoError(t, db.Create(&models.DiscoveryInterestEntry{RunID: &runB, QueryText: "软件", Dimension: 3}).Error) // 多主题各自独立记录
		err := db.Create(&models.DiscoveryInterestEntry{RunID: &runA, QueryText: "再次体育", Dimension: 3}).Error
		require.Error(t, err, "同 run 重复种子必须被唯一索引拒绝")
	})

	t.Run("discovery_interest_entries run_id 可空（legacy 迁移行 NULL 不参与唯一约束）", func(t *testing.T) {
		db := setupDiscoveryRunModelsDB(t)
		// 迁移回填的旧 seed 无原始 run，run_id=NULL；普通唯一索引允许多 NULL（PG/sqlite 均然），
		// 多条 legacy 行互不冲突。
		require.NoError(t, db.Create(&models.DiscoveryInterestEntry{QueryText: "legacy seed（无原始查询）", Status: "legacy_inactive", LegacyRef: "preference_vectors:1", Dimension: 3}).Error)
		require.NoError(t, db.Create(&models.DiscoveryInterestEntry{QueryText: "legacy seed（无原始查询）", Status: "legacy_inactive", LegacyRef: "preference_vectors:2", Dimension: 3}).Error)
		var count int64
		require.NoError(t, db.Model(&models.DiscoveryInterestEntry{}).Where("run_id IS NULL").Count(&count).Error)
		require.Equal(t, int64(2), count, "两条 NULL run_id legacy 行必须都能落库")
	})

	t.Run("candidate_preferences candidate_id 唯一（一候选一排除权威行，D2）", func(t *testing.T) {
		db := setupDiscoveryRunModelsDB(t)
		require.NoError(t, db.Create(&models.CandidatePreference{CandidateID: 5}).Error)
		err := db.Create(&models.CandidatePreference{CandidateID: 5}).Error
		require.Error(t, err, "重复 candidate_id 必须被唯一索引拒绝")
	})

	t.Run("candidate_embeddings candidate_id+model+dimension 联合唯一（D6）", func(t *testing.T) {
		db := setupDiscoveryRunModelsDB(t)
		require.NoError(t, db.Create(&models.CandidateEmbedding{CandidateID: 5, Model: "m1", Dimension: 1024, EmbeddingVec: "[1]", TextHash: "h1"}).Error)
		// 同候选不同维度可共存（模型切换期新旧向量并存，靠检索前校验阻断混算）
		require.NoError(t, db.Create(&models.CandidateEmbedding{CandidateID: 5, Model: "m1", Dimension: 2560, EmbeddingVec: "[2]", TextHash: "h2"}).Error)
		require.NoError(t, db.Create(&models.CandidateEmbedding{CandidateID: 5, Model: "m2", Dimension: 1024, EmbeddingVec: "[3]", TextHash: "h3"}).Error)
		err := db.Create(&models.CandidateEmbedding{CandidateID: 5, Model: "m1", Dimension: 1024, EmbeddingVec: "[4]", TextHash: "h4"}).Error
		require.Error(t, err, "同 candidate+model+dimension 必须被联合唯一索引拒绝")
	})
}
