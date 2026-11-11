package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/tagmanagement/repository"
)

// setupFingerprintTestDB builds an isolated in-memory SQLite schema for the
// event-fingerprint persistence tests (same style as edge_gc_test.go: pure-SQL
// read-modify-write cases, no Postgres-specific SQL, runnable under -short).
func setupFingerprintTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:fingerprint-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.TopicTag{},
		&models.EmbeddingQueue{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repository.InitRepository(db)
	// airouter.NewStore()（事件描述异步 goroutine）经 database.DB 取库；
	// 指到本测试库可让缺表错误快速失败而不是打真网络。
	database.DB = db
	GetTagCache().Clear()
	t.Cleanup(func() {
		GetTagCache().Clear()
		if database.DB == db {
			database.DB = nil
		}
	})
	return db
}

func loadFingerprintMetadata(t *testing.T, db *gorm.DB, tagID uint) models.MetadataMap {
	t.Helper()
	var row models.TopicTag
	require.NoError(t, db.Select("metadata").Where("id = ?", tagID).First(&row).Error)
	return row.Metadata
}

func fingerprintOf(t *testing.T, meta models.MetadataMap) map[string]any {
	t.Helper()
	require.NotNil(t, meta)
	nested, ok := meta[eventFingerprintMetadataKey].(map[string]any)
	require.True(t, ok, "metadata must carry a nested event_fingerprint object, got %T", meta[eventFingerprintMetadataKey])
	return nested
}

func TestEventFingerprintComplete(t *testing.T) {
	require.False(t, (*EventFingerprint)(nil).Complete())
	require.False(t, (&EventFingerprint{Subject: "a", SubjectKey: "person:a", Action: "发布"}).Complete(), "missing object → incomplete")
	require.False(t, (&EventFingerprint{Subject: "a", Action: "发布", Object: "产品"}).Complete(), "missing subject_key → incomplete")
	// occurred_at 可空：四项非空即完整。
	require.True(t, (&EventFingerprint{Subject: "a", SubjectKey: "person:a", Action: "发布", Object: "产品"}).Complete())
}

func TestMergeFingerprintMetadataPureRules(t *testing.T) {
	complete := &EventFingerprint{Subject: "特朗普", SubjectKey: "person:trump", Action: "宣布", Object: "参选"}
	partial := &EventFingerprint{Subject: "特朗普", Action: "宣布"}

	t.Run("nil map + complete fp writes nested object with all fields", func(t *testing.T) {
		merged := mergeFingerprintMetadata(nil, complete)
		nested := fingerprintOf(t, merged)
		assert.Equal(t, "特朗普", nested["subject"])
		assert.Equal(t, "person:trump", nested["subject_key"])
		assert.Equal(t, "宣布", nested["action"])
		assert.Equal(t, "参选", nested["object"])
		assert.NotContains(t, nested, "occurred_at", "empty occurred_at omitted")
	})

	t.Run("occurred_at present is kept", func(t *testing.T) {
		withDate := &EventFingerprint{Subject: "s", SubjectKey: "person:s", Action: "a", Object: "o", OccurredAt: "2026-11-01"}
		merged := mergeFingerprintMetadata(models.MetadataMap{}, withDate)
		assert.Equal(t, "2026-11-01", fingerprintOf(t, merged)["occurred_at"])
	})

	t.Run("existing complete fingerprint wins（先写者赢，不翻转）", func(t *testing.T) {
		meta := models.MetadataMap{eventFingerprintMetadataKey: map[string]any{
			"subject": "字节跳动", "subject_key": "company:bytedance", "action": "发布", "object": "旧产品",
		}}
		merged := mergeFingerprintMetadata(meta, complete)
		assert.Equal(t, "company:bytedance", fingerprintOf(t, merged)["subject_key"], "first writer wins")
	})

	t.Run("existing incomplete fingerprint is replaced", func(t *testing.T) {
		meta := models.MetadataMap{eventFingerprintMetadataKey: map[string]any{"subject": "旧主体"}}
		merged := mergeFingerprintMetadata(meta, complete)
		assert.Equal(t, "person:trump", fingerprintOf(t, merged)["subject_key"])
	})

	t.Run("sibling keys survive", func(t *testing.T) {
		meta := models.MetadataMap{"event_keywords": []any{"阳光电源", "涨价"}}
		merged := mergeFingerprintMetadata(meta, complete)
		assert.Contains(t, merged, "event_keywords")
		assert.Contains(t, merged, eventFingerprintMetadataKey)
	})

	t.Run("nil or incomplete fp is a no-op", func(t *testing.T) {
		meta := models.MetadataMap{"event_keywords": []any{"k"}}
		assert.Equal(t, meta, mergeFingerprintMetadata(meta, nil))
		assert.Equal(t, meta, mergeFingerprintMetadata(meta, partial))
		assert.Nil(t, mergeFingerprintMetadata(nil, nil))
	})
}

func TestPersistEventFingerprintWriteThenKeep(t *testing.T) {
	db := setupFingerprintTestDB(t)

	tag := models.TopicTag{Slug: "event-fp-tag", Label: "某事件标签", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)

	first := &EventFingerprint{Subject: "阳光电源", SubjectKey: "company:sungrow", Action: "涨价", Object: "逆变器", OccurredAt: "2026-11-01"}
	require.NoError(t, persistEventFingerprint(&tag, first))

	// ① 新建 event 标签写入嵌套指纹。
	nested := fingerprintOf(t, loadFingerprintMetadata(t, db, tag.ID))
	assert.Equal(t, "阳光电源", nested["subject"])
	assert.Equal(t, "company:sungrow", nested["subject_key"])
	assert.Equal(t, "涨价", nested["action"])
	assert.Equal(t, "逆变器", nested["object"])
	assert.Equal(t, "2026-11-01", nested["occurred_at"])

	// ② 再写一次（不同指纹）：先写者赢，旧指纹不翻转。
	second := &EventFingerprint{Subject: "另一主体", SubjectKey: "company:other", Action: "下架", Object: "别的产品"}
	require.NoError(t, persistEventFingerprint(&tag, second))
	nested = fingerprintOf(t, loadFingerprintMetadata(t, db, tag.ID))
	assert.Equal(t, "company:sungrow", nested["subject_key"], "first fingerprint must win")
	// 传入对象的 Metadata 同步为 DB 真值（供缓存刷新）。
	assert.Equal(t, "company:sungrow", tag.Metadata[eventFingerprintMetadataKey].(map[string]any)["subject_key"])

	// ③ saveTagDescription 写 event_keywords 后指纹仍在（读-改-写共存）。
	require.NoError(t, saveTagDescription(true, tag.ID, "阳光电源涨价的事件描述", []string{"阳光电源", "涨价"}))
	meta := loadFingerprintMetadata(t, db, tag.ID)
	assert.Contains(t, meta, "event_keywords")
	nested = fingerprintOf(t, meta)
	assert.Equal(t, "company:sungrow", nested["subject_key"], "fingerprint must survive the description read-modify-write")

	// 不完整指纹 / nil：no-op，不报错也不写。
	bare := models.TopicTag{Slug: "event-fp-bare", Label: "无指纹事件", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&bare).Error)
	require.NoError(t, persistEventFingerprint(&bare, &EventFingerprint{Subject: "只有主体"}))
	require.NoError(t, persistEventFingerprint(&bare, nil))
	assert.NotContains(t, loadFingerprintMetadata(t, db, bare.ID), eventFingerprintMetadataKey)
}

func TestPersistEventFingerprintKeywordTagNoop(t *testing.T) {
	db := setupFingerprintTestDB(t)

	// ④ keyword 标签不写指纹：keyword 的 Fingerprint 恒为 nil（解析层不构造），
	// 即便误传完整指纹也不该在 keyword 标签上留下 event_fingerprint。
	keyword := models.TopicTag{Slug: "keyword-fp-tag", Label: "某关键词", Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&keyword).Error)

	require.NoError(t, persistEventFingerprint(&keyword, nil))
	assert.NotContains(t, loadFingerprintMetadata(t, db, keyword.ID), eventFingerprintMetadataKey)
}

// findOrCreateTag 四条返回路径（缓存命中 / exact / slug 兜底 / 新建）都要落
// 指纹。本用例走 新建 → exact → 缓存命中 三条（embedding TagMatch 的 slug 精确
// 命中在 SQLite 下即走 exact 分支），断言先写者赢 + 缓存 Metadata 同步。
func TestFindOrCreateTagPersistsFingerprintAcrossBranches(t *testing.T) {
	db := setupFingerprintTestDB(t)
	ctx := context.Background()

	first := &EventFingerprint{Subject: "字节跳动", SubjectKey: "company:bytedance", Action: "发布", Object: "AI视频模型"}
	second := &EventFingerprint{Subject: "Meta", SubjectKey: "company:meta", Action: "发布", Object: "AI视频模型"}
	third := &EventFingerprint{Subject: "谷歌", SubjectKey: "company:google", Action: "发布", Object: "AI视频模型"}

	// 新建分支：Create 后指纹落库。
	tag := TopicTag{
		Label: "字节跳动发布AI视频模型", Slug: Slugify("字节跳动发布AI视频模型"), Category: "event",
		Description: "字节跳动发布AI视频模型的事件", Fingerprint: first,
	}
	created, err := findOrCreateTag(ctx, tag, "llm", "文章上下文", 1)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	assert.Equal(t, "company:bytedance", fingerprintOf(t, loadFingerprintMetadata(t, db, created.ID))["subject_key"])

	// exact 分支（TagMatch slug 精确命中）：不同指纹不翻转。
	tag.Fingerprint = second
	matched, err := findOrCreateTag(ctx, tag, "llm", "文章上下文", 1)
	require.NoError(t, err)
	require.Equal(t, created.ID, matched.ID)
	assert.Equal(t, "company:bytedance", fingerprintOf(t, loadFingerprintMetadata(t, db, created.ID))["subject_key"],
		"exact branch must keep the first fingerprint")

	// 缓存命中分支：缓存对象可能带旧 metadata，落库仍读 DB 真值；返回对象同步最新值。
	tag.Fingerprint = third
	cached, err := findOrCreateTag(ctx, tag, "llm", "文章上下文", 1)
	require.NoError(t, err)
	require.Equal(t, created.ID, cached.ID)
	assert.Equal(t, "company:bytedance", fingerprintOf(t, loadFingerprintMetadata(t, db, created.ID))["subject_key"],
		"cache-hit branch must keep the first fingerprint")
}

// slug 兜底分支（embedding 服务不可用 → 退回 slug 精确查库）同样落指纹。
func TestFindOrCreateTagSlugFallbackPersistsFingerprint(t *testing.T) {
	db := setupFingerprintTestDB(t)
	ctx := context.Background()

	// 白盒：把 embedding 单例置空，命中 findOrCreateTag 的 es==nil 降级路径。
	savedES := embeddingService
	embeddingService = nil
	t.Cleanup(func() { embeddingService = savedES })

	existing := models.TopicTag{
		Slug: Slugify("某公司召回主力产品"), Label: "某公司召回主力产品", Category: "event", Status: "active",
	}
	require.NoError(t, db.Create(&existing).Error)

	tag := TopicTag{
		Label: "某公司召回主力产品", Slug: existing.Slug, Category: "event",
		Description: "某公司召回主力产品的事件",
		Fingerprint: &EventFingerprint{Subject: "某公司", SubjectKey: "company:acme", Action: "召回", Object: "主力产品"},
	}
	result, err := findOrCreateTag(ctx, tag, "llm", "文章上下文", 1)

	require.NoError(t, err)
	require.Equal(t, existing.ID, result.ID)
	assert.Equal(t, "company:acme", fingerprintOf(t, loadFingerprintMetadata(t, db, existing.ID))["subject_key"],
		"slug-fallback branch must persist the fingerprint")
}
