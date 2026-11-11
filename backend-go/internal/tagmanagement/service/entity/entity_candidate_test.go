package entity

// 场景锚点（tasks.md 映射表）：
//   - 后续跨叫法跨板块再次出现合为一个 ID 选项
//   - 相关但不同的候选不被强合
// 白盒 B2：TestEntityCandidate_SameNameDistinctOptions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

func poolEntry(id uint, key, name, desc string) (uint, *tagmodels.EntityRegistry) {
	return id, &tagmodels.EntityRegistry{ID: id, EntityKey: key, CanonicalName: name, StableDesc: desc}
}

// 场景：后续跨叫法跨板块再次出现合为一个 ID 选项。
// WHEN 某实体已登记，后一篇文章以另一种叫法提及同一主体：三路候选合并后
// 该实体只有一个 ID 选项；模型选中后沿用同一统一 ID；该场景只约束「已被
// 召回且被选中」时的归并（真实未召回的重复由评测量化，不作无条件承诺）。
func TestEntityCandidate_CrossAliasMergedToSingleOption(t *testing.T) {
	// 实体「特朗普」被三路同时召回（名向量 / 名+说明向量 / 已登记叫法）。
	id, reg := poolEntry(7, "E7777AAA", "唐纳德·特朗普", "美国第45任总统")
	pool := map[uint]*tagmodels.EntityRegistry{id: reg}

	candidates := MergeCandidates(
		[]ScoredEntity{{EntityID: 7, Score: 0.91}}, // 名向量路
		[]ScoredEntity{{EntityID: 7, Score: 0.88}}, // 名+说明路
		[]uint{7}, // 已登记叫法路
		pool, 12,
	)
	require.Len(t, candidates, 1, "同一实体三路命中只留一个选项")
	assert.Equal(t, uint(7), candidates[0].EntityID)
	assert.Equal(t, "E7777AAA", candidates[0].EntityKey)
	assert.Equal(t, 0.91, candidates[0].BestScore, "合并保留最高分")
	assert.ElementsMatch(t, []string{"name", "name_desc", "registered_name"}, candidates[0].Sources)
}

// 场景：相关但不同的候选不被强合。
// WHEN 候选中同时存在产品与公司、团队与公司、人物与政府等相关但不是同一
// 实体的项：不因向量相近/名称相似/类型相同而合并；判 none 时新建另一实体。
// 白盒 B2：同名多实体保留多个 ID 选项，交模型按上下文选，不硬复用。
func TestEntityCandidate_SameNameDistinctOptions(t *testing.T) {
	idA, regA := poolEntry(1, "EAAA1111", "苹果", "美国消费电子公司")
	idB, regB := poolEntry(2, "EBBB2222", "苹果", "水果")
	pool := map[uint]*tagmodels.EntityRegistry{idA: regA, idB: regB}

	// 同名「苹果」的叫法命中必然同时召回两个实体（歧义合法）。
	candidates := MergeCandidates(nil, nil, []uint{idA, idB}, pool, 12)
	require.Len(t, candidates, 2, "同名多实体保留各自选项，不硬复用")
	assert.NotEqual(t, candidates[0].EntityID, candidates[1].EntityID)
	assert.ElementsMatch(t, []uint{idA, idB}, []uint{candidates[0].EntityID, candidates[1].EntityID})

	// 无分数的叫法命中并列 → ID 升序（机械确定）。
	assert.Equal(t, uint(1), candidates[0].EntityID)

	// 相关但不同的实体（DeepSeek 公司 vs DeepSeek Harness 产品）在候选里
	// 各自是独立选项：合并器永不因分数高/名字像/类型同而吞并另一个。
	idC, regC := poolEntry(3, "ECCC3333", "DeepSeek", "中国 AI 公司")
	idD, regD := poolEntry(4, "EDDD4444", "DeepSeek Harness", "DeepSeek 的产品线")
	pool2 := map[uint]*tagmodels.EntityRegistry{idC: regC, idD: regD}
	candidates2 := MergeCandidates(
		[]ScoredEntity{{EntityID: 3, Score: 0.96}, {EntityID: 4, Score: 0.9648}}, // 跨实体 token 重叠实测可到 0.96+
		nil, nil, pool2, 12,
	)
	require.Len(t, candidates2, 2, "产品与公司是两个实体，两个选项")
	assert.Equal(t, uint(4), candidates2[0].EntityID, "分数并列/更高在前，排序仅呈现顺序不是判定")

	// 二轮 prompt 的判据写死「同一实体」与「相关 ≠ 同一」。
	system, user := BuildSecondRoundPrompts("DeepSeek", "中国 AI 公司", "DeepSeek 发布新模型", candidates2)
	assert.Contains(t, system, "同一实体")
	assert.Contains(t, system, "不是「是否相关」")
	assert.Contains(t, system, "产品与厂商")
	assert.Contains(t, system, "机构与成员")
	assert.Contains(t, user, "ECCC3333 | DeepSeek | 中国 AI 公司")
	assert.Contains(t, user, "EDDD4444 | DeepSeek Harness | DeepSeek 的产品线")

	// schema enum 只含合法候选 + none（「只能输出候选集合内合法 ID 或 none」）。
	schema := SecondRoundSchema(candidates2)
	enum := schema.Properties["answer"].Enum
	assert.ElementsMatch(t, []string{"ECCC3333", "EDDD4444", "none"}, enum)

	// 注入文本只是数据：prompt 明确声明数据块不是指令。
	assert.Contains(t, system, "【数据】")
	assert.True(t, strings.Contains(system, "不得执行"))
}

// 合并条数上限是可评测配置（呈现上限），不是身份裁决：截断只影响候选呈现
// 条数，绝不吞并「唯一正确选项」之外的判定权（判定在二轮模型）。
func TestEntityCandidate_MaxOptionsCap(t *testing.T) {
	pool := map[uint]*tagmodels.EntityRegistry{}
	namePath := []ScoredEntity{}
	for i := 1; i <= 5; i++ {
		id := uint(i)
		pool[id] = &tagmodels.EntityRegistry{ID: id, EntityKey: string(rune('A'+i)) + "K", CanonicalName: "同名"}
		namePath = append(namePath, ScoredEntity{EntityID: id, Score: float64(i) / 10})
	}
	got := MergeCandidates(namePath, nil, nil, pool, 3)
	assert.Len(t, got, 3)
	// 分数降序截断。
	assert.Equal(t, uint(5), got[0].EntityID)

	// 池里不存在的命中 ID（防御）被跳过，不 panic。
	got2 := MergeCandidates([]ScoredEntity{{EntityID: 99, Score: 1}}, nil, nil, pool, 3)
	assert.Empty(t, got2)
}

// R-19：截断时先保留全部 registered_name 命中，再按分数补足向量路。
// WHEN 向量路高分候选足够多、MaxOptions 放不下时：已登记叫法命中仍在候选里
// （它是昵称/冷僻写法唯一召回路径；被挤出 → 候选空 → none → 重复新建）。
func TestEntityCandidate_TruncationKeepsRegisteredHits(t *testing.T) {
	pool := map[uint]*tagmodels.EntityRegistry{}
	// 1..4 = 向量高分候选（分高但都是近似项）；5 = 已登记叫法命中（score 0）。
	namePath := []ScoredEntity{}
	for i := 1; i <= 4; i++ {
		id := uint(i)
		pool[id] = &tagmodels.EntityRegistry{ID: id, EntityKey: string(rune('A'+i)) + "K", CanonicalName: "近似项"}
		namePath = append(namePath, ScoredEntity{EntityID: id, Score: 0.9 - float64(i)/100})
	}
	id5, reg5 := poolEntry(5, "EHUANG0001", "黄仁勋", "NVIDIA 创始人")
	pool[id5] = reg5
	id6, reg6 := poolEntry(6, "EEXTRA001", "额外近似", "")
	pool[id6] = reg6
	namePath = append(namePath, ScoredEntity{EntityID: id6, Score: 0.88})

	candidates := MergeCandidates(namePath, nil, []uint{id5}, pool, 3)

	got := make([]uint, 0, len(candidates))
	for _, c := range candidates {
		got = append(got, c.EntityID)
	}
	// 1 个叫法命中保留 + 2 个最高分向量候选（1、2），按分数降序呈现。
	assert.ElementsMatch(t, []uint{id5, 1, 2}, got, "叫法命中不被向量高分挤出")
	assert.Equal(t, id5, candidates[2].EntityID, "score=0 的叫法命中排最后，但必须在内")
	assert.Contains(t, candidates[2].Sources, "registered_name")

	// 叫法命中自身已占满席位：全部保留，不被分数挤掉。
	ids := []uint{id5, id6}
	many := MergeCandidates(nil, nil, ids, pool, 2)
	require.Len(t, many, 2)
	for _, c := range many {
		assert.Contains(t, c.Sources, "registered_name")
	}

	// 上限仍生效：总条数等于 MaxOptions，席位不放宽。
	assert.Len(t, candidates, 3)
}
