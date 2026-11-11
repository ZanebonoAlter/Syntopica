package service

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingEmbed wraps an embed func with a call counter (the exact-tier
// "must not call embed" contract is asserted via the count).
type countingEmbed struct {
	calls int
	vec   []float64
	err   error
}

func (e *countingEmbed) fn(subjectKey string) ([]float64, error) {
	e.calls++
	if e.err != nil {
		return nil, e.err
	}
	if e.vec != nil {
		return e.vec, nil
	}
	return []float64{1, 0}, nil
}

// unit0p92 / justBelow compose with [1,0] to a cosine of exactly 0.92
// (0.92000000000000004 ≥ threshold) and 0.919 — verified deterministic in
// float64; see test-cases §5.2 boundary values.
var (
	unit0p92      = []float64{0.92, 0.39191835884530885}
	unitJustBelow = []float64{0.919, 0.39425753004857117}
)

func TestResolveSubjectKeyExact(t *testing.T) {
	embed := &countingEmbed{}
	candidates := []SubjectCandidate{
		{StoryID: 1, Subject: "字节跳动", SubjectKey: "company:tencent"},
		{StoryID: 2, Subject: "Donald Trump", SubjectKey: "Person:Trump"}, // case/spacing variants normalize equal
	}
	match := ResolveSubjectKey(SubjectInput{Subject: "特朗普", SubjectKey: "person:trump"}, candidates, embed.fn)
	assert.Equal(t, "person:trump", match.SubjectKey)
	assert.Equal(t, SubjectMethodExact, match.Method)
	assert.Equal(t, float64(1), match.Score)
	assert.Nil(t, match.Embedding, "exact hit must not consume an embedding call")
	assert.Equal(t, 0, embed.calls, "exact tier must not call embed")
}

func TestResolveSubjectKeyEmbeddingThreshold(t *testing.T) {
	t.Run("exactly 0.92 merges", func(t *testing.T) {
		embed := &countingEmbed{}
		candidates := []SubjectCandidate{
			{StoryID: 3, Subject: "特朗普", SubjectKey: "person:trump", SubjectEmbedding: unit0p92},
		}
		match := ResolveSubjectKey(SubjectInput{Subject: "特朗普"}, candidates, embed.fn)
		assert.Equal(t, SubjectMethodEmbedding, match.Method)
		assert.Equal(t, "person:trump", match.SubjectKey)
		assert.InDelta(t, 0.92, match.Score, 1e-9)
		assert.Equal(t, 1, embed.calls)
		assert.Equal(t, []float64{1, 0}, match.Embedding, "embedding tier echoes the input vector for 2c reuse")
	})

	t.Run("0.919 does not merge", func(t *testing.T) {
		embed := &countingEmbed{}
		candidates := []SubjectCandidate{
			{StoryID: 3, Subject: "字节跳动", SubjectKey: "company:bytedance", SubjectEmbedding: unitJustBelow},
		}
		match := ResolveSubjectKey(SubjectInput{Subject: "OpenAI"}, candidates, embed.fn)
		assert.Equal(t, SubjectMethodNew, match.Method)
		assert.Empty(t, match.SubjectKey)
		assert.Equal(t, float64(0), match.Score)
		assert.NotNil(t, match.Embedding, "embed was called — echo the vector even on a miss")
		assert.Equal(t, 1, embed.calls)
	})
}

func TestResolveSubjectKeyLiteral(t *testing.T) {
	embed := &countingEmbed{}
	// No keys overlap (exact/embedding miss), display names fold to the same
	// literal: full-width + spaces vs ASCII compact.
	candidates := []SubjectCandidate{
		{StoryID: 4, Subject: "SK海力士", SubjectKey: "company:sk_hynix"},
	}
	match := ResolveSubjectKey(SubjectInput{Subject: "ＳＫ 海力士"}, candidates, embed.fn)
	assert.Equal(t, SubjectMethodLiteral, match.Method)
	assert.Equal(t, "company:sk_hynix", match.SubjectKey)
	assert.Equal(t, float64(0), match.Score)
	assert.Equal(t, 0, embed.calls, "no candidate embedding means no embedding call")
	assert.Nil(t, match.Embedding)
}

func TestResolveSubjectKeyLiteralKeylessCandidate(t *testing.T) {
	// Keyless candidate matched literally → adopts the input's normalized key.
	embed := &countingEmbed{}
	candidates := []SubjectCandidate{{StoryID: 5, Subject: "Trump"}}
	match := ResolveSubjectKey(SubjectInput{Subject: "trump "}, candidates, embed.fn)
	assert.Equal(t, SubjectMethodLiteral, match.Method)
	assert.Empty(t, match.SubjectKey, "keyless input adopts nothing")
	assert.Equal(t, 0, embed.calls, "empty input key must skip the embedding tier entirely")
}

func TestResolveSubjectKeyNew(t *testing.T) {
	t.Run("all tiers miss", func(t *testing.T) {
		embed := &countingEmbed{}
		candidates := []SubjectCandidate{
			{StoryID: 6, Subject: "字节跳动", SubjectKey: "company:bytedance", SubjectEmbedding: unitJustBelow},
		}
		match := ResolveSubjectKey(SubjectInput{Subject: "特斯拉", SubjectKey: "company:tesla"}, candidates, embed.fn)
		assert.Equal(t, SubjectMethodNew, match.Method)
		assert.Equal(t, "company:tesla", match.SubjectKey)
		assert.Equal(t, float64(0), match.Score)
		assert.Nil(t, match.Embedding)
		assert.Zero(t, embed.calls, "conflicting explicit keys skip embedding")
	})

	t.Run("no candidates at all", func(t *testing.T) {
		embed := &countingEmbed{}
		match := ResolveSubjectKey(SubjectInput{Subject: "特朗普", SubjectKey: " person:donald trump "}, nil, embed.fn)
		assert.Equal(t, SubjectMethodNew, match.Method)
		assert.Equal(t, "person:donald_trump", match.SubjectKey)
		assert.Equal(t, 0, embed.calls, "nothing to match — no embed call")
	})
}

func TestResolveSubjectKeyCandidateEdgeCases(t *testing.T) {
	t.Run("candidate embedding empty is skipped", func(t *testing.T) {
		embed := &countingEmbed{}
		candidates := []SubjectCandidate{
			{StoryID: 7, Subject: "字节跳动", SubjectKey: "company:bytedance"}, // key, no embedding
		}
		match := ResolveSubjectKey(SubjectInput{Subject: "特斯拉", SubjectKey: "company:tesla"}, candidates, embed.fn)
		assert.Equal(t, SubjectMethodNew, match.Method)
		assert.Equal(t, 0, embed.calls)
	})

	t.Run("keyless candidate permits embedding fallback", func(t *testing.T) {
		embed := &countingEmbed{}
		candidates := []SubjectCandidate{
			{StoryID: 8, Subject: "字节跳动", SubjectEmbedding: unit0p92},
		}
		match := ResolveSubjectKey(SubjectInput{Subject: "特斯拉", SubjectKey: "company:tesla"}, candidates, embed.fn)
		assert.Equal(t, SubjectMethodEmbedding, match.Method)
		assert.Equal(t, "company:tesla", match.SubjectKey)
		assert.Equal(t, 1, embed.calls)
	})

	t.Run("embed failure degrades to literal/new", func(t *testing.T) {
		embed := &countingEmbed{err: errors.New("provider down")}
		candidates := []SubjectCandidate{
			{StoryID: 9, Subject: "特朗普", SubjectKey: "person:trump", SubjectEmbedding: unit0p92},
		}
		match := ResolveSubjectKey(SubjectInput{Subject: "特朗普"}, candidates, embed.fn)
		// Display names equal → literal tier still merges without the vector.
		assert.Equal(t, SubjectMethodLiteral, match.Method)
		assert.Equal(t, "person:trump", match.SubjectKey)
		assert.Nil(t, match.Embedding)
		assert.Equal(t, 1, embed.calls)
	})

	t.Run("dimension mismatch candidate is skipped, not fatal", func(t *testing.T) {
		embed := &countingEmbed{}
		candidates := []SubjectCandidate{
			{StoryID: 10, Subject: "字节跳动", SubjectKey: "company:bytedance", SubjectEmbedding: []float64{1, 0, 0}},
			{StoryID: 11, Subject: "特朗普", SubjectKey: "person:trump", SubjectEmbedding: unit0p92},
		}
		match := ResolveSubjectKey(SubjectInput{Subject: "特朗普"}, candidates, embed.fn)
		require.Equal(t, SubjectMethodEmbedding, match.Method)
		assert.Equal(t, "person:trump", match.SubjectKey)
	})
}

// ---- normalize-event-subject-entities：影子边界（event-story MODIFIED）----
// 既有 6 组测试（exact / embedding / literal / new / edge cases）全部照旧、
// 断言一字不改；以下只追加影子态边界用例。

// 场景：影子态不改变事件线身份判定。
// WHEN 某事件标签已在影子态被归一到某个全局实体 ID：事件线身份判定仍只
// 使用 subject_key 三层匹配与一票否决，判定路径不读取实体 ID、不放宽一票
// 否决；存量 subject_key 与事件线归属与上线前一致；人工归属不被归一结果
// 覆盖。
func TestResolveSubjectKey_ShadowEntityIDNotRead(t *testing.T) {
	// ① 结构层面就不存在实体 ID 通道：四个判定输入结构体任一新增
	// entity/aux/mention/registry 字段即红（编译期能过的接线，此断言必须红）。
	for _, typ := range []reflect.Type{
		reflect.TypeOf(SubjectInput{}),
		reflect.TypeOf(SubjectCandidate{}),
		reflect.TypeOf(AssignmentInput{}),
		reflect.TypeOf(StoryCandidate{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, bad := range []string{"entity", "aux", "mention", "registry"} {
				assert.NotContains(t, name, bad,
					"%s.%s：事件线判定输入不得携带实体层通道（D9：判定路径不读实体 ID）", typ.Name(), typ.Field(i).Name)
			}
		}
	}

	// ② 签名层面也不接收/不返回实体层类型：给判定函数加 EntityID 或
	// []EntityMention 之类参数即红。
	for _, fn := range []struct {
		label string
		fn    any
	}{
		{"ResolveSubjectKey", ResolveSubjectKey},
		{"subjectPreFilter", subjectPreFilter},
		{"DecideAssignment", DecideAssignment},
	} {
		ft := reflect.TypeOf(fn.fn)
		require.Equal(t, reflect.Func, ft.Kind())
		for i := 0; i < ft.NumIn(); i++ {
			assertNoEntityTypedChannel(t, fn.label+" 参数", ft.In(i))
		}
		for i := 0; i < ft.NumOut(); i++ {
			assertNoEntityTypedChannel(t, fn.label+" 返回值", ft.Out(i))
		}
	}

	// ③ 行为回归：exact 层命中不消耗 embedding，判定只看
	// subject_key / subject / embedding 三样输入（候选是否已被影子归一
	// 不进入这条路径的任何输入）。
	embed := &countingEmbed{}
	input := SubjectInput{Subject: "特朗普", SubjectKey: "person:trump"}
	candidates := []SubjectCandidate{
		{StoryID: 1, Subject: "特朗普", SubjectKey: "person:trump"}, // 假设它已被影子归一到 E1
		{StoryID: 2, Subject: "唐纳德·特朗普", SubjectKey: "person:donald_trump"},
	}
	matchA := ResolveSubjectKey(input, candidates, embed.fn)
	require.Equal(t, SubjectMethodExact, matchA.Method)
	assert.Equal(t, "person:trump", matchA.SubjectKey)
	assert.Equal(t, 0, embed.calls, "exact 层不触发 embedding——判定与实体层零耦合")

	// ④ 一票否决不被放宽：冲突显式键仍直接跳过 embedding 合并（既有
	// 行为，影子态不得改变）。
	conflict := ResolveSubjectKey(SubjectInput{Subject: "特斯拉", SubjectKey: "company:tesla"},
		[]SubjectCandidate{{StoryID: 3, Subject: "特斯拉", SubjectKey: "company:tsla"}},
		embed.fn)
	require.Equal(t, SubjectMethodNew, conflict.Method, "冲突键不得被合并（不放宽一票否决）")
	assert.Equal(t, 0, embed.calls, "冲突显式键跳过 embedding 调用")

	// ⑤ 三层不命中照旧新建（不因实体层存在而改走实体路径）。
	fresh := ResolveSubjectKey(SubjectInput{Subject: "全新主体", SubjectKey: "person:new"}, nil, embed.fn)
	require.Equal(t, SubjectMethodNew, fresh.Method)
	assert.Equal(t, "person:new", fresh.SubjectKey)
}

// assertNoEntityTypedChannel fails when a decision-path parameter or return
// type carries the entity layer (design D9: the path must not read entity IDs).
func assertNoEntityTypedChannel(t *testing.T, label string, typ reflect.Type) {
	t.Helper()
	name := strings.ToLower(typ.String())
	for _, bad := range []string{"entity", "auxlabel", "mention", "registry", "tagmodels"} {
		assert.NotContains(t, name, bad,
			"%s 类型 %s 携带实体层通道（D9：判定路径不读实体 ID）", label, typ.String())
	}
}

// 场景：事件线身份判定不读实体 ID（D9 + event-story MODIFIED）。
// 结构断言只能证明「输入结构体里没有实体 ID 字段」；本用例直接盯判定路径
// 的三个源文件——一旦有人把实体层（entity_registry / entity_mentions /
// entity_aux_links / metadata.entity_subject / tagmanagement 实体服务）接进
// subject_key 三层判定、subject 一票否决或归属判定，本测试立刻红。
//
// 为什么不是恒真句：断言的是「文件里不存在实体层引用」这个可被未来改写
// 打破的事实——先正向锤定三个入口仍在（防入口搬家后护栏静默空转），再反向
// 扫禁用符号。
func TestEventStoryDecisionPathDoesNotReadEntityIdentity(t *testing.T) {
	// 判定路径必须实际存在这些入口：文件改名 / 函数搬家时本护栏先红，
	// 而不是变成空转。
	anchors := map[string][]string{
		"story_subject_normalize.go": {"func ResolveSubjectKey(", "textutil.NormalizeSubjectKey"},
		"story_assign.go":            {"func subjectPreFilter(", "func DecideAssignment(", "textutil.NormalizeSubjectKey"},
		"story_attribute.go":         {"ResolveSubjectKey(SubjectInput{", "DecideAssignment("},
	}
	// 出现即红：任何一个都意味着判定路径读到了全局实体身份层。
	// 注意不能用裸 "entity"（NormalizeEntityText / matchEntityText 属既有
	// 字面归一，与实体身份层无关）。
	forbidden := []string{
		"entity_registry", "entity_mentions", "entity_aux_links",
		"entity_subject", "EntityRegistry", "EntityMention", "EntityAuxLink",
		"tagmanagement", "tagmodels", "service/entity",
	}
	for file, wanted := range anchors {
		src, err := os.ReadFile(file)
		require.NoError(t, err, "读取判定路径源文件 %s", file)
		body := string(src)
		for _, anchor := range wanted {
			require.Contains(t, body, anchor,
				"%s 缺少判定锚点 %q——入口搬家后本护栏必须同步，不能静默空转", file, anchor)
		}
		lower := strings.ToLower(body)
		for _, bad := range forbidden {
			assert.NotContains(t, lower, strings.ToLower(bad),
				"%s 出现实体层引用 %q：事件线判定路径不得读取实体 ID（D9）", file, bad)
		}
	}
}
