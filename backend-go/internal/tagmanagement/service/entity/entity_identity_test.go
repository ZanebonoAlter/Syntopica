package entity

// 场景锚点（tasks.md 映射表）：
//   - 实体身份与名称证据按层持久化
//   - 同名不同实体可并存

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	repository "syntopica-backend/internal/tagmanagement/repository"
)

// 跨包集成测试约束③（standard/backend/testing.md）：本测试二进制的
// AutoMigrate 表集需要 entity 三表——tagging 根包的 init 注册无法在此
// import（wire.go blank import 本包会成环），按 semantic_board_upgrade_test
// 先例在 init 手动注册。
func init() {
	database.RegisterModels(tagmodels.EntitySubjectModels()...)
}

// setupEntityDB returns a repository bound to the golden-schema test DB.
func setupEntityDB(t *testing.T) (*repository.EntityRepository, *gorm.DB) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return repository.NewEntityRepository(db), db
}

func mustCreateAuxLabel(t *testing.T, db *gorm.DB, label string) uint {
	t.Helper()
	row := &models.SemanticLabel{Label: label, Slug: "aux-" + label, LabelType: "auxiliary", Status: "active", Source: "test"}
	require.NoError(t, db.Create(row).Error)
	return row.ID
}

// 场景：实体身份与名称证据按层持久化。
// WHEN 某提及被归一到实体 E：身份行持稳定 ID/entity_key/规范名/短说明且
// 规范名无唯一约束；提及原文留在证据层不覆盖规范字段；第一轮未产出短说明
// 时保持为空（不从 subject_key 回填、不编造）。
func TestEntityIdentity_PersistedInLayers(t *testing.T) {
	repo, db := setupEntityDB(t)
	ctx := context.Background()

	reg := &tagmodels.EntityRegistry{
		EntityKey:     "EAAAA0001",
		CanonicalName: "唐纳德·特朗普",
		StableDesc:    "美国第45任总统",
	}
	mention := &tagmodels.EntityMention{
		MentionText: "特朗普",
		MentionKey:  "特朗普",
	}
	require.NoError(t, repo.CreateEntityAtomic(ctx, reg, mention, nil))

	// 身份行：稳定 ID + 短不透明 key + 规范名 + 真实短说明。
	loaded, err := repo.FindEntityByID(ctx, reg.ID)
	require.NoError(t, err)
	assert.NotZero(t, loaded.ID)
	assert.Equal(t, "EAAAA0001", loaded.EntityKey)
	assert.Equal(t, "唐纳德·特朗普", loaded.CanonicalName)
	assert.Equal(t, "美国第45任总统", loaded.StableDesc)

	// 证据层：提及原文保留，不覆盖规范字段（分列存储）。
	var mentions []tagmodels.EntityMention
	require.NoError(t, db.Where("entity_id = ?", reg.ID).Find(&mentions).Error)
	require.Len(t, mentions, 1)
	assert.Equal(t, "特朗普", mentions[0].MentionText)
	assert.NotEqual(t, mentions[0].MentionText, loaded.CanonicalName, "原文证据与规范名分列")

	// 身份层无实体类型列（D1：类型不入身份层——模型猜类型会把漂移固化）。
	var typeCols []string
	require.NoError(t, db.Raw(`SELECT column_name FROM information_schema.columns
		WHERE table_name = 'entity_registry' AND column_name LIKE '%type%'`).Scan(&typeCols).Error)
	assert.Empty(t, typeCols, "entity_registry 不得有任何类型列")

	// 规范名无唯一约束（同名实体可并存的前提，下个用例正面验证）。
	var nameIdx []string
	require.NoError(t, db.Raw(`SELECT indexdef FROM pg_indexes
		WHERE tablename = 'entity_registry' AND indexdef ILIKE '%canonical_name%'`).Scan(&nameIdx).Error)
	for _, def := range nameIdx {
		assert.NotContains(t, def, "UNIQUE", "canonical_name 不允许唯一索引：%s", def)
	}

	// 短说明为空：保持为空，不从 subject_key 回填（场景 AND 断言）。
	reg2 := &tagmodels.EntityRegistry{EntityKey: "EAAAA0002", CanonicalName: "某公司"}
	mention2 := &tagmodels.EntityMention{MentionText: "某公司", MentionKey: "某公司"}
	require.NoError(t, repo.CreateEntityAtomic(ctx, reg2, mention2, nil))
	loaded2, err := repo.FindEntityByKey(ctx, "EAAAA0002")
	require.NoError(t, err)
	assert.Empty(t, loaded2.StableDesc, "未产出短说明时保持为空，不编造不回填")
}

// 场景：同名不同实体可并存。
// WHEN 两个真实不同的实体规范名相同：各持一个实体 ID，不因规范名相同
// 合并、也不被唯一约束拒绝；叫法层同名合法歧义，命中返回两个候选。
func TestEntityIdentity_SameNameDistinctEntitiesCoexist(t *testing.T) {
	repo, db := setupEntityDB(t)
	ctx := context.Background()

	mk := func(key, name string) *tagmodels.EntityRegistry {
		reg := &tagmodels.EntityRegistry{EntityKey: key, CanonicalName: name}
		m := &tagmodels.EntityMention{MentionText: name, MentionKey: name}
		require.NoError(t, repo.CreateEntityAtomic(ctx, reg, m, nil))
		return reg
	}
	a := mk("EBBBB0001", "苹果") // 公司还是水果？名字本身不判定
	b := mk("EBBBB0002", "苹果")

	assert.NotZero(t, a.ID)
	assert.NotZero(t, b.ID)
	assert.NotEqual(t, a.ID, b.ID, "同名不同实体各持独立 ID，不被合并")

	// 名称层证据：两个实体的叫法表里都有「苹果」——叫法歧义合法。
	ids, err := repo.FindEntityIDsByMentionKey(ctx, "苹果")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{a.ID, b.ID}, ids)

	// 第三个同名实体仍可登记（无唯一约束、无自动合并）。
	c := mk("EBBBB0003", "苹果")
	assert.NotEqual(t, a.ID, c.ID)
	var count int64
	require.NoError(t, db.Model(&tagmodels.EntityRegistry{}).Where("canonical_name = ?", "苹果").Count(&count).Error)
	assert.EqualValues(t, 3, count)
}
