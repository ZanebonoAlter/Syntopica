package entity

// 场景锚点（tasks.md 映射表）：
//   - 辅助标签被清理后实体身份仍然有效
// 兼覆盖仓储层基础契约：幂等唯一索引、叫法反查、池载入。

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 场景：辅助标签被清理后实体身份仍然有效。
// WHEN 某实体溯源到的辅助标签被 GC 置 disabled 或被删除：
// 实体身份行仍然存在且 ID 不变；溯源行被移除（硬删 CASCADE）或失效
// （disabled 读时过滤），绝不级联删除实体。
func TestEntityRepository_AuxCleanupKeepsIdentity(t *testing.T) {
	repo, db := setupEntityDB(t)
	ctx := context.Background()

	liveAux := mustCreateAuxLabel(t, db, "美联储")
	gcAux := mustCreateAuxLabel(t, db, "联邦储备委员会")

	reg := &tagmodels.EntityRegistry{EntityKey: "ECCCC0001", CanonicalName: "美联储"}
	mention := &tagmodels.EntityMention{MentionText: "美联储", MentionKey: "美联储"}
	require.NoError(t, repo.CreateEntityAtomic(ctx, reg, mention, []uint{liveAux, gcAux}))

	// 基线：两条溯源行都是 active。
	n, err := repo.ActiveAuxLinkCount(ctx, reg.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	// 分支一：aux 被 GC 置 disabled（soft）→ 溯源行「失效」——读时过滤，
	// 实体身份不动。auxlabel 的 GC 不感知实体层（本 change 不改
	// auxiliary_label_service.go，失效语义在读取侧实现）。
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", gcAux).Update("status", "disabled").Error)
	n, err = repo.ActiveAuxLinkCount(ctx, reg.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "disabled aux 的溯源行失效（读时过滤）")
	still, err := repo.FindEntityByID(ctx, reg.ID)
	require.NoError(t, err)
	assert.Equal(t, reg.ID, still.ID, "disabled 不损毁实体身份，ID 不变")

	// 分支二：aux 行被硬删除 → 只有溯源行级联消失（FK 方向契约：
	// entity_aux_links.aux_label_id → semantic_labels ON DELETE CASCADE，
	// 永远没有从 aux 指向 entity_registry 的级联），实体身份行原样保留。
	require.NoError(t, db.Delete(&models.SemanticLabel{}, liveAux).Error)
	var linkRows int64
	require.NoError(t, db.Model(&tagmodels.EntityAuxLink{}).Where("entity_id = ?", reg.ID).Count(&linkRows).Error)
	assert.EqualValues(t, 1, linkRows, "硬删只掉该 aux 的溯源行（disabled 那条仍在库里但已失效）")

	still2, err := repo.FindEntityByID(ctx, reg.ID)
	require.NoError(t, err)
	assert.Equal(t, reg.ID, still2.ID, "实体身份行不受 aux 删除影响")
	assert.Equal(t, "ECCCC0001", still2.EntityKey)

	// 提及证据行同样保留（证据层独立于 aux 溯源层）。
	var mentionRows int64
	require.NoError(t, db.Model(&tagmodels.EntityMention{}).Where("entity_id = ?", reg.ID).Count(&mentionRows).Error)
	assert.EqualValues(t, 1, mentionRows)
}

// 幂等唯一索引契约：同 (source_tag_id, mention_key) 只允许一行——
// 「同来源同提及」的决策存储是数据库级幂等的兜底（重放语义由
// FindDecisionByIdempotency 在应用层先行，此为并发竞态护栏）。
func TestEntityRepository_IdempotencyUniqueIndex(t *testing.T) {
	repo, db := setupEntityDB(t)
	ctx := context.Background()

	tagID := uint(4242)
	reg := &tagmodels.EntityRegistry{EntityKey: "ECCCC0011", CanonicalName: "OpenAI"}
	m := &tagmodels.EntityMention{MentionText: "OpenAI", MentionKey: "openai", SourceTagID: &tagID}
	require.NoError(t, repo.CreateEntityAtomic(ctx, reg, m, nil))

	// 幂等查询命中已有决策。
	found, ok := repo.FindDecisionByIdempotency(ctx, &tagID, "openai")
	require.True(t, ok)
	assert.Equal(t, reg.ID, found.EntityID)

	// 数据库唯一索引拒绝第二行（并发竞态兜底）。
	tagIDCopy := tagID
	dup := &tagmodels.EntityMention{EntityID: reg.ID, MentionText: "OpenAI", MentionKey: "openai", SourceTagID: &tagIDCopy}
	err := db.Create(dup).Error
	require.Error(t, err, "幂等唯一索引必须拒绝同 (source_tag_id, mention_key) 第二行")
	assert.Contains(t, err.Error(), "uq_entity_mentions_source_key")

	// NULL source（评测合成来源）不参与唯一约束。
	nilA := &tagmodels.EntityMention{EntityID: reg.ID, MentionText: "OpenAI", MentionKey: "openai"}
	nilB := &tagmodels.EntityMention{EntityID: reg.ID, MentionText: "OpenAI", MentionKey: "openai"}
	require.NoError(t, db.Create(nilA).Error)
	require.NoError(t, db.Create(nilB).Error)

	// 不同来源同提及各自成行（跨来源不因文本焊死，D8）。
	otherTag := uint(9001)
	cross := &tagmodels.EntityMention{EntityID: reg.ID, MentionText: "OpenAI", MentionKey: "openai", SourceTagID: &otherTag}
	require.NoError(t, db.Create(cross).Error)

	// 空提及键不参与幂等（空/纯空白不归一）。
	_, ok = repo.FindDecisionByIdempotency(ctx, &tagID, "")
	assert.False(t, ok)
}

// 池载入契约：全量行可载入（内存余弦检索的输入），空池（冷启动）合法。
func TestEntityRepository_LoadEntityPool(t *testing.T) {
	repo, _ := setupEntityDB(t)
	ctx := context.Background()

	pool, err := repo.LoadEntityPool(ctx)
	require.NoError(t, err)
	require.NotNil(t, pool, "空池（冷启动）也合法")

	reg := &tagmodels.EntityRegistry{EntityKey: "ECCCC0021", CanonicalName: "NVIDIA"}
	m := &tagmodels.EntityMention{MentionText: "英伟达", MentionKey: "英伟达"}
	require.NoError(t, repo.CreateEntityAtomic(ctx, reg, m, nil))

	pool, err = repo.LoadEntityPool(ctx)
	require.NoError(t, err)
	found := false
	for _, r := range pool {
		if r.ID == reg.ID {
			found = true
		}
	}
	assert.True(t, found)
}
