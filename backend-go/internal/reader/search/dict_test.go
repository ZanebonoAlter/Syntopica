package search

// add-article-search tasks 2.1 — 词典缓存与正向最大匹配的函数单测。
// 覆盖 test-cases.md 白盒「词典切词白盒」表全部分支 +「词典词长」边界
// （2/3/8/9 runes、词内空白）+ 失效重载（重载后新词可切）。
// 词典装载是简单 KV 读取（无 pg 方言语义分叉），按 testing.md「轻量 CRUD
// 可用内存 SQLite」用 glebarez sqlite；SQL 语义验证一律走 repository_pg_test。

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

func setupDictTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:dict-test-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TopicTag{}))
	return db
}

func seedDictLabel(t *testing.T, db *gorm.DB, label string) {
	t.Helper()
	require.NoError(t, db.Create(&models.TopicTag{Label: label, Slug: label, Category: "keyword", Status: "active"}).Error)
}

func wordset(words ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}

// TestSegmentForwardMaxMatch covers test-cases.md 白盒「词典切词白盒」行的
// 纯函数部分（词典任意词集，含 <3 rune 词以验证最长优先）。
func TestSegmentForwardMaxMatch(t *testing.T) {
	tests := []struct {
		name  string
		words map[string]struct{}
		query string
		want  []string
	}{
		{
			name:  "两词连续命中",
			words: wordset("人工智能", "大模型"),
			query: "人工智能大模型",
			want:  []string{"人工智能", "大模型"},
		},
		{
			name:  "最长优先：人工 vs 人工智能",
			words: wordset("人工", "人工智能"),
			query: "人工智能",
			want:  []string{"人工智能"},
		},
		{
			name:  "空串空切",
			words: wordset("人工智能"),
			query: "",
			want:  nil,
		},
		{
			name:  "纯空白空切",
			words: wordset("人工智能"),
			query: "  \t\u3000",
			want:  nil,
		},
		{
			name:  "纯英文词典外空切",
			words: wordset("人工智能"),
			query: "AI agent",
			want:  nil,
		},
		{
			name:  "词典外前后缀被消费不产出",
			words: wordset("人工智能"),
			query: "XX人工智能YY",
			want:  []string{"人工智能"},
		},
		{
			name:  "空词典",
			words: wordset(),
			query: "人工智能",
			want:  nil,
		},
		{
			name:  "词典词中段之后无词可续",
			words: wordset("科技股", "泡沫化"),
			query: "泡沫化尾部科技股",
			want:  []string{"泡沫化", "科技股"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SegmentForwardMaxMatch(tt.words, tt.query))
		})
	}
}

// TestDictWordBoundary covers test-cases.md 白盒「词典词长（topic_tags.label）」
// 行：2/3/8/9 runes 与词内空白（空格/全角空格/tab）判定。
func TestDictWordBoundary(t *testing.T) {
	tests := []struct {
		label string
		ok    bool
	}{
		{"人工", false},         // 2 runes
		{"人工智能", true},        // 3 runes（下界含）
		{"八个字符的标签测", true},    // 8 runes（上界含）
		{"九个字符的标签测试呀", false}, // 9 runes
		{"AI 泡沫", false},      // 词内半角空格
		{"AI\u3000泡沫", false}, // 词内全角空格
		{"标签\t内tab", false},   // 词内 tab
		{"  人工智能  ", true},    // 首尾空白剥离后 4 runes
		{"", false},           // 空
		{"   ", false},        // 纯空白
	}
	for _, tt := range tests {
		got, ok := dictWord(tt.label)
		require.Equal(t, tt.ok, ok, "label %q", tt.label)
		if ok {
			require.NotContains(t, got, " ", "首尾空白须剥离：%q", got)
		}
	}
}

// TestDictCacheLoadFilterReload covers 白盒「词典加载」行：3~8 字 label 全集
// 装载 + 标签写入后 InvalidateDict 失效重载（重载后新词可切）。
func TestDictCacheLoadFilterReload(t *testing.T) {
	db := setupDictTestDB(t)
	ctx := context.Background()

	// 词长/空白过滤全集：只有 3 与 8 rune 词入词典。
	seedDictLabel(t, db, "人工") // 2 runes → 不入
	seedDictLabel(t, db, "人工智能")
	seedDictLabel(t, db, "八个字符的标签测")   // 8 runes → 入
	seedDictLabel(t, db, "九个字符的标签测试呀") // 9 runes → 不入
	seedDictLabel(t, db, "AI 泡沫")      // 词内空白 → 不入

	cache := NewDictCache(db)
	require.Zero(t, cache.Size(), "懒加载：未 EnsureLoaded 前词典为空")
	require.NoError(t, cache.EnsureLoaded(ctx))
	require.Equal(t, 2, cache.Size())

	// 切词走缓存：新词（未失效前）切不中。
	require.Equal(t, []string{"人工智能"}, cache.Segment("XX人工智能YY"))

	// 标签写入方落新词 → 失效前缓存仍旧行为。
	seedDictLabel(t, db, "科技股")
	require.Equal(t, 2, cache.Size(), "失效前不重载")
	require.Nil(t, cache.Segment("科技股AI泡沫"), "失效前新词不可切")

	// 失效 → EnsureLoaded 走 stale-while-revalidate：立即返回旧词不阻塞
	// （打标活跃期 16.5K 标签重载 ~2s，同步重载违反 P50<1s）；
	// EnsureFresh 同步等待重载完成 → 新词可切（白盒判据）。
	cache.InvalidateDict()
	require.NoError(t, cache.EnsureLoaded(ctx))
	require.Equal(t, 2, cache.Size(), "stale-while-revalidate：后台重载完成前用旧词")
	require.Nil(t, cache.Segment("科技股AI泡沫"), "重载完成前新词仍不可切")
	require.NoError(t, cache.EnsureFresh(ctx))
	require.Equal(t, 3, cache.Size())
	require.Equal(t, []string{"科技股"}, cache.Segment("科技股AI泡沫"))
}

// TestDictCacheLoadErrorNotCached：装载失败不冻结为空——重试可恢复
// （临时 DB 故障不得让词典进程级永远为空）。用 nil db 触发装载错误。
func TestDictCacheLoadErrorNotCached(t *testing.T) {
	cache := NewDictCache(nil)
	require.Error(t, cache.EnsureLoaded(context.Background()))
	require.Zero(t, cache.Size())
	require.False(t, cache.segmentAvailable(), "失败后不得标记 loaded")
}

// segmentAvailable 白盒窥视 loaded 位（仅测试用）。
func (d *DictCache) segmentAvailable() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.loaded
}
