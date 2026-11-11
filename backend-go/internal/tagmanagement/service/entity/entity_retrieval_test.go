package entity

// 场景锚点（tasks.md 映射表）：
//   - 稳定短身份说明为空回退仅名检索
// 兼覆盖同空间守卫（D3）：跨 model/版本/维度不混用、不 fallback；名路径
// 始终存在；top-K 与并列按实体 ID 升序。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
	repository "syntopica-backend/internal/tagmanagement/repository"
)

func regWithVectors(id uint, key string, nameVec []float64, nameModel string, nameDim int, descVec []float64, descModel string, descDim int) tagmodels.EntityRegistry {
	r := tagmodels.EntityRegistry{ID: id, EntityKey: key, CanonicalName: key}
	if nameVec != nil {
		s := repository.EntityFloatsToPgVector(nameVec)
		r.NameEmbedding = &s
		r.NameEmbeddingModel = nameModel
		r.NameEmbeddingVersion = nameModel // 与 fakeEmbed 三元组一致（model==version）
		r.NameEmbeddingDim = nameDim
	}
	if descVec != nil {
		s := repository.EntityFloatsToPgVector(descVec)
		r.NameDescEmbedding = &s
		r.NameDescEmbeddingModel = descModel
		r.NameDescEmbeddingVersion = descModel
		r.NameDescEmbeddingDim = descDim
	}
	return r
}

// 场景：稳定短身份说明为空回退仅名检索。
// WHEN 第一轮未产出稳定短身份说明：该提及走「仅实体名」检索路径，不因缺
// 说明而跳过名路径或报错；标签照常入库（缺说明不丢弃）。
func TestEntityRetrieval_EmptyDescFallsBackToNameOnly(t *testing.T) {
	// 查询文本构造：desc 为空 → name+desc 路径文本为空 → 只走名路径。
	nameText, nameDescText := EmbedQueryTexts("特朗普", "")
	assert.Equal(t, "特朗普", nameText)
	assert.Empty(t, nameDescText, "短说明为空 → 无第二路查询文本（不隐形、不报错）")

	// 带说明时两路文本都存在且互不相同（两侧不同模板，防退化成事件检索）。
	nameText2, nameDescText2 := EmbedQueryTexts("特朗普", "美国第45任总统")
	assert.Equal(t, "特朗普", nameText2)
	assert.Equal(t, "特朗普 美国第45任总统", nameDescText2)
	assert.NotEqual(t, nameText2, nameDescText2)

	// 名路径始终可检索：池里有同空间实体即可命中（即使实体没有 desc 向量）。
	pool := []tagmodels.EntityRegistry{
		regWithVectors(1, "EA1", []float64{1, 0, 0}, "m1", 3, nil, "", 0),
	}
	q := QueryVector{Vec: []float64{1, 0, 0}, Triplet: SpaceTriplet{Model: "m1", Version: "m1"}}
	hits := SearchNamePath(pool, q, 10)
	require.Len(t, hits, 1)
	assert.Equal(t, uint(1), hits[0].EntityID)

	// 空说明不进第二路：NameDescPath 对该池返回空（无行有 desc 表示）。
	assert.Empty(t, SearchNameDescPath(pool, q, 10))
}

// 同空间守卫（D3）：两套表示各自只在同 (model, version, dim) 空间内比较；
// 跨空间行被跳过，绝无跨空间混用或 fallback；top-K 截断、并列按实体 ID 升序。
func TestEntityRetrieval_SameSpaceGuard(t *testing.T) {
	pool := []tagmodels.EntityRegistry{
		regWithVectors(1, "EA1", []float64{1, 0, 0}, "m1", 3, nil, "", 0),      // same space
		regWithVectors(2, "EA2", []float64{1, 0, 0}, "m2", 3, nil, "", 0),      // different model
		regWithVectors(3, "EA3", []float64{1, 0, 0, 0}, "m1", 4, nil, "", 0),   // different dim
		regWithVectors(4, "EA4", nil, "", 0, nil, "", 0),                       // no vector at all
		regWithVectors(5, "EA5", []float64{0.99, 0.1, 0}, "m1", 3, nil, "", 0), // same space, close
	}
	q := QueryVector{Vec: []float64{1, 0, 0}, Triplet: SpaceTriplet{Model: "m1", Version: "m1", Dim: 3}}

	hits := SearchNamePath(pool, q, 10)
	require.Len(t, hits, 2, "跨 model/跨 dim/无向量的行全部跳过")
	assert.Equal(t, uint(1), hits[0].EntityID)
	assert.Equal(t, uint(5), hits[1].EntityID)

	// top-K 截断。
	assert.Len(t, SearchNamePath(pool, q, 1), 1)

	// 并列分数按实体 ID 升序（机械确定）。
	tiePool := []tagmodels.EntityRegistry{
		regWithVectors(9, "EA9", []float64{1, 0, 0}, "m1", 3, nil, "", 0),
		regWithVectors(2, "EA2", []float64{1, 0, 0}, "m1", 3, nil, "", 0),
	}
	tieHits := SearchNamePath(tiePool, q, 10)
	require.Len(t, tieHits, 2)
	assert.Equal(t, uint(2), tieHits[0].EntityID, "同分 → ID 小者在前")
	assert.Equal(t, uint(9), tieHits[1].EntityID)

	// 第二路（name+desc）只在自身空间比较：名空间向量不参与。
	descPool := []tagmodels.EntityRegistry{
		regWithVectors(1, "EA1", []float64{1, 0, 0}, "m1", 3, []float64{1, 0, 0}, "m1", 3),
	}
	dq := QueryVector{Vec: []float64{1, 0, 0}, Triplet: SpaceTriplet{Model: "other", Version: "other"}}
	assert.Empty(t, SearchNameDescPath(descPool, dq, 10), "desc 空间不同的查询不命中（不 fallback 到名空间）")

	// 维度不匹配的比较直接报错（防线）——同空间过滤后不会走到。
	_, err := repository.CosineSimilarity([]float64{1, 0}, []float64{1, 0, 0})
	require.Error(t, err)
}

// 空/非法查询不 panic、返回空。
func TestEntityRetrieval_EmptyInputs(t *testing.T) {
	pool := []tagmodels.EntityRegistry{regWithVectors(1, "EA1", []float64{1, 0, 0}, "m1", 3, nil, "", 0)}
	assert.Empty(t, SearchNamePath(pool, QueryVector{}, 10))
	assert.Empty(t, SearchNamePath(pool, QueryVector{Vec: []float64{1, 0, 0}}, 0))
	assert.Empty(t, SearchNamePath(nil, QueryVector{Vec: []float64{1, 0, 0}}, 10))
}
