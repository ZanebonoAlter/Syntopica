package search

// add-article-search tasks 2.2 — 四层管线函数单测。
// 覆盖 test-cases.md 变体走查「输入」组全部条目（空串/纯空白/通配符/超长）
// + 白盒「查询长度 199/200/201 runes」边界 + S1 步 5。
// 阈值 loader 注入替身，免 DB；词典用白盒预置。

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/logging"
)

// stubDict 白盒预置词典（绕过 DB 装载）。
func stubDict(words ...string) *DictCache {
	d := &DictCache{words: wordset(words...), loaded: true}
	for _, w := range words {
		if n := len([]rune(w)); n > d.maxLen {
			d.maxLen = n
		}
	}
	return d
}

func stubPreprocessor(dict *DictCache, threshold float64) *QueryPreprocessor {
	return &QueryPreprocessor{
		dict:                 dict,
		loadTrigramThreshold: func(context.Context) float64 { return threshold },
	}
}

// TestPrepareRejectsEmptyQuery covers spec「空与纯空白查询拒绝」：
// 空串/半角空白/全角 U+3000/tab 混合 → ErrEmptyQuery 哨兵（handler 转 400）。
func TestPrepareRejectsEmptyQuery(t *testing.T) {
	p := stubPreprocessor(nil, DefaultTrigramThreshold)
	for _, q := range []string{"", "   ", "\t", "\u3000", " \t\u3000 ", "\n\r"} {
		pq, err := p.Prepare(context.Background(), q)
		require.Nil(t, pq, "q=%q 不得产出 PreparedQuery", q)
		require.ErrorIs(t, err, ErrEmptyQuery, "q=%q", q)
	}
}

// TestPrepareEscapesLikeWildcards covers spec「特殊字符与超长查询」通配符半：
// % _ \ 被转义为字面匹配；test-cases 变体「纯通配符转义后按字面匹配」。
func TestPrepareEscapesLikeWildcards(t *testing.T) {
	p := stubPreprocessor(nil, DefaultTrigramThreshold)
	pq, err := p.Prepare(context.Background(), `50%_off\特价`)
	require.NoError(t, err)
	require.Equal(t, `50%_off\特价`, pq.Raw, "Raw 保留原文（trigram 输入/展示用）")
	// 单词查询：L0 与 L2 同词，两者转义形态一致。
	require.Len(t, pq.L0Terms, 1)
	require.Equal(t, `50\%\_off\\特价`, pq.L0Terms[0].Escaped)
	require.Equal(t, `50\%\_off\\特价`, escapeLikePattern(pq.Raw))
}

// TestPrepareTruncatesLongQuery covers 白盒「查询长度 199/200/201 runes」：
// 201 截为 200 + warn；200/199 不截断；rune 计（中文 1 rune）非字节。
func TestPrepareTruncatesLongQuery(t *testing.T) {
	p := stubPreprocessor(nil, DefaultTrigramThreshold)

	var captured strings.Builder
	logging.SetWriters(&captured, &captured)
	defer logging.ResetWriters()

	pq, err := p.Prepare(context.Background(), strings.Repeat("搜", 201))
	require.NoError(t, err, "超长截断不报错")
	require.Equal(t, 200, len([]rune(pq.Raw)), "201 runes 截为 200")
	require.Contains(t, captured.String(), "truncated", "截断须记 warn")

	pq, err = p.Prepare(context.Background(), strings.Repeat("搜", 200))
	require.NoError(t, err)
	require.Equal(t, 200, len([]rune(pq.Raw)), "200 runes 恰好不截断")

	pq, err = p.Prepare(context.Background(), strings.Repeat("搜", 199))
	require.NoError(t, err)
	require.Equal(t, 199, len([]rune(pq.Raw)), "199 runes 不截断")

	// rune 计非字节：200 个中文 = 600 UTF-8 字节，仍不截断。
	mixed := strings.Repeat("搜a", 100) // 200 runes / 300 bytes
	pq, err = p.Prepare(context.Background(), mixed)
	require.NoError(t, err)
	require.Equal(t, 200, len([]rune(pq.Raw)))
	require.Equal(t, mixed, pq.Raw)
}

// TestPrepareSplitsOnUnicodeWhitespace covers L0：全角空格 U+3000 与 tab 均
// 为分隔符（spec「词典词重组查询命中」前置拆分行为）。
func TestPrepareSplitsOnUnicodeWhitespace(t *testing.T) {
	p := stubPreprocessor(stubDict(), DefaultTrigramThreshold)
	pq, err := p.Prepare(context.Background(), "特斯拉\u3000召回\t事件")
	require.NoError(t, err)
	require.Equal(t, "特斯拉\u3000召回\t事件", pq.Raw, "Raw 保留整串（L2 输入）")
	require.Len(t, pq.L0Terms, 3)
	require.Equal(t, "特斯拉", pq.L0Terms[0].Text)
	require.Equal(t, "召回", pq.L0Terms[1].Text)
	require.Equal(t, "事件", pq.L0Terms[2].Text)
	require.Nil(t, pq.L1Words, "词典无命中 → L1 空切（纯英文同理）")
}

// TestPrepareDictSegmentation covers L1 切词在管线内的落点：
// 「XX人工智能YY」→ L1Words=[人工智能]，残段交 L2/L3（白盒词典切词表）。
func TestPrepareDictSegmentation(t *testing.T) {
	p := stubPreprocessor(stubDict("人工智能", "大模型"), DefaultTrigramThreshold)
	pq, err := p.Prepare(context.Background(), "XX人工智能YY大模型")
	require.NoError(t, err)
	require.Equal(t, []string{"人工智能", "大模型"}, termTexts(pq.L1Words))
	require.Equal(t, []string{"人工智能", "大模型"}, termEscaped(pq.L1Words))
}

// TestPrepareCarriesThreshold：阈值经 loader 注入透传到输出（供 repo L3）。
func TestPrepareCarriesThreshold(t *testing.T) {
	p := stubPreprocessor(nil, 0.11)
	pq, err := p.Prepare(context.Background(), "特斯拉")
	require.NoError(t, err)
	require.InDelta(t, 0.11, pq.TrigramThreshold, 1e-9)
}

// TestPrepareDictLoadFailureDegradesSilently：词典装载失败 → L1 空切、
// 不让 Prepare 整体报错（红线：关键词路径永远在线）。
func TestPrepareDictLoadFailureDegradesSilently(t *testing.T) {
	broken := NewDictCache(nil) // nil db → EnsureLoaded 必败
	p := &QueryPreprocessor{
		dict:                 broken,
		loadTrigramThreshold: func(context.Context) float64 { return DefaultTrigramThreshold },
	}
	pq, err := p.Prepare(context.Background(), "人工智能")
	require.NoError(t, err, "词典失败不得让预处理报错")
	require.Nil(t, pq.L1Words)
	require.Len(t, pq.L0Terms, 1, "L0/L2 照常")
	require.Equal(t, "人工智能", pq.Raw)
}

func termTexts(terms []MatchTerm) []string {
	out := make([]string, len(terms))
	for i, t := range terms {
		out[i] = t.Text
	}
	return out
}

func termEscaped(terms []MatchTerm) []string {
	out := make([]string, len(terms))
	for i, t := range terms {
		out[i] = t.Escaped
	}
	return out
}
