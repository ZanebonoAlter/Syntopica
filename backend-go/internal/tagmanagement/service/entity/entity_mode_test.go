package entity

// 场景锚点（tasks.md 映射表）：
//   - 默认关闭态逐字不变
//   - 影子态写入限定三处
// 白盒 B6：TestEntityMode_OffIsNoop
//       B7：TestEntityMode_ShadowWritesOnlyThreePlaces

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"
)

func setMode(t *testing.T, mode string) {
	t.Helper()
	prev := viper.GetString("entity_subject.mode")
	viper.Set("entity_subject.mode", mode)
	t.Cleanup(func() { viper.Set("entity_subject.mode", prev) })
}

// 模式解析契约：空/未知 → off（未知值可见地回退）；active → 显式错误，
// 绝不静默按 shadow 执行。
func TestEntityMode_Validation(t *testing.T) {
	m, err := ModeFromValue("")
	require.NoError(t, err)
	assert.Equal(t, ModeOff, m)

	m, err = ModeFromValue("off")
	require.NoError(t, err)
	assert.Equal(t, ModeOff, m)

	m, err = ModeFromValue("shadow")
	require.NoError(t, err)
	assert.Equal(t, ModeShadow, m)

	m, err = ModeFromValue("active")
	assert.Equal(t, ModeActive, m)
	require.ErrorIs(t, err, ErrModeActiveNotImplemented, "active 显式拒绝")

	m, err = ModeFromValue("bogus")
	require.Error(t, err, "未知值可见报错，不静默 shadow")
	assert.Equal(t, ModeOff, m)
}

// 场景 + 白盒 B6：默认关闭态逐字不变。
// WHEN 系统以默认配置运行打标：零第二轮调用、零实体层写入、不写
// entity_subject 键——hook 在任何 DB 访问之前短路（off 态 repository.Repo
// 为 nil 也不会 panic，即零写入的结构性证明）。
func TestEntityMode_OffIsNoop(t *testing.T) {
	setMode(t, "off")
	assert.False(t, ShadowEnabledForExtraction())

	// repository.Repo 未初始化（nil）：off 短路发生在 DB 访问前 → 不 panic。
	// （该测试进程从不调 repository.InitRepository。）
	require.Nil(t, repository.Repo)
	assert.NotPanics(t, func() {
		core.EntitySubjectHook(context.Background(), core.TopicTag{
			Category:    "event",
			Fingerprint: &core.EventFingerprint{Subject: "特朗普", SubjectKey: "person:trump", Action: "宣布", Object: "政策"},
			SubjectDesc: "不应被使用的说明",
		}, &models.TopicTag{ID: 1}, 1, "ctx")
	})
}

// 场景 + 白盒 B7：影子态写入限定三处。
// WHEN 系统以 shadow 模式处理一批事件标签：实体层、metadata.entity_subject
// 惰性键有新增；event_fingerprint、标签业务字段不变；技术失败不阻断、
// 未归一记录原因。
func TestEntityMode_ShadowWritesOnlyThreePlaces(t *testing.T) {
	setMode(t, "shadow")
	repo, db := setupEntityDB(t)

	// 造一个已入库的 event 标签（带既有 event_fingerprint 与业务字段）。
	tag := &models.TopicTag{
		Slug: "trump-policy-event", Label: "特朗普宣布新政策", Category: "event",
		Status: "active", IsCanonical: true, Source: "ai",
		Metadata: models.MetadataMap{
			"event_fingerprint": map[string]any{
				"subject": "特朗普", "subject_key": "person:trump", "action": "宣布", "object": "政策",
			},
			"event_keywords": []string{"特朗普", "政策"},
		},
	}
	require.NoError(t, db.Create(tag).Error)

	// fake resolver：合法 none → 原子新建。
	embed := &fakeEmbed{byText: map[string][]float64{}}
	noneChat := &fakeChat{responses: []string{"none"}}
	fakeRS := NewResolver(repo, noneChat.chat, embed.embed, fastConfig())
	SetHookResolverForTest(func() *Resolver { return fakeRS })
	t.Cleanup(func() { SetHookResolverForTest(nil) })

	// repository.Repo 挂到测试库（hook 的 metadata 读-改-写走它）。
	repository.InitRepository(db)

	RunForTag(context.Background(), core.TopicTag{
		Category:    "event",
		Fingerprint: &core.EventFingerprint{Subject: "特朗普", SubjectKey: "person:trump", Action: "宣布", Object: "政策"},
		SubjectDesc: "美国第45任总统",
	}, tag, 42, "文章上下文…")

	// ① 实体层有新增（一个实体 + 一条提及证据 + aux 溯源为空）。
	var entities int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_registry").Scan(&entities).Error)
	assert.EqualValues(t, 1, entities)
	var mentions int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_mentions").Scan(&mentions).Error)
	assert.EqualValues(t, 1, mentions)

	// ② 惰性键 entity_subject 写入（仅影子态、无读路径）。
	var after models.TopicTag
	require.NoError(t, db.Select("metadata", "label", "category", "status", "source").Where("id = ?", tag.ID).First(&after).Error)
	es, ok := after.Metadata["entity_subject"].(map[string]any)
	require.True(t, ok, "影子态必须写 entity_subject 惰性键")
	assert.Equal(t, "特朗普", es["subject"])
	assert.Equal(t, "美国第45任总统", es["subject_desc"])
	assert.Equal(t, StatusCreated, es["status"])
	assert.Equal(t, "shadow", es["mode"])
	assert.NotZero(t, es["entity_id"])

	// ③ event_fingerprint / event_keywords / 业务字段逐字不变（先写者赢，
	// 单键合并不得动兄弟键）。
	fp, ok := after.Metadata["event_fingerprint"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "person:trump", fp["subject_key"])
	assert.Equal(t, "特朗普", fp["subject"])
	kw, ok := after.Metadata["event_keywords"].([]interface{})
	require.True(t, ok, "event_keywords 兄弟键必须在（jsonb 读回为 []interface{}）")
	assert.Equal(t, []interface{}{"特朗普", "政策"}, kw)
	assert.Equal(t, "特朗普宣布新政策", after.Label)
	assert.Equal(t, "event", after.Category)
	assert.Equal(t, "active", after.Status)

	// metadata 键集 = 旧键 + 新增 entity_subject，一个不多一个不少。
	keys := make([]string, 0)
	for k := range after.Metadata {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"event_fingerprint", "event_keywords", "entity_subject"}, keys)

	// 非影子前缀检查：shadow 模式下第二个标签（keyword 类）不触发任何写入。
	var mentionsBefore int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_mentions").Scan(&mentionsBefore).Error)
	RunForTag(context.Background(), core.TopicTag{
		Category: "keyword", Label: "PostgreSQL",
	}, tag, 42, "ctx")
	var mentionsAfter int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_mentions").Scan(&mentionsAfter).Error)
	assert.Equal(t, mentionsBefore, mentionsAfter, "keyword 标签不产生实体提及")
}

// active 模式在 hook 层显式报错路径：不写、不静默 shadow。
func TestEntityMode_ActiveHookRejects(t *testing.T) {
	setMode(t, "active")
	repo, db := setupEntityDB(t)
	repository.InitRepository(db)
	embed := &fakeEmbed{byText: map[string][]float64{}}
	noneChat := &fakeChat{responses: []string{"none"}}
	fakeRS := NewResolver(repo, noneChat.chat, embed.embed, fastConfig())
	SetHookResolverForTest(func() *Resolver { return fakeRS })
	t.Cleanup(func() { SetHookResolverForTest(nil) })

	tag := &models.TopicTag{ID: 1}
	assert.NotPanics(t, func() {
		RunForTag(context.Background(), core.TopicTag{
			Category:    "event",
			Fingerprint: &core.EventFingerprint{Subject: "特朗普", SubjectKey: "person:trump", Action: "宣布", Object: "政策"},
		}, tag, 1, "ctx")
	})
	var entities int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entity_registry").Scan(&entities).Error)
	assert.EqualValues(t, 0, entities, "active 显式拒绝：零写入")
}

// ShadowEnabledForExtraction mirrors the extraction-side shadow switch (the
// core package's own viper read) for cross-package assertions.
func ShadowEnabledForExtraction() bool { return core.EntitySubjectShadowEnabled() }
