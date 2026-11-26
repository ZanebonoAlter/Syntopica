package search

// add-article-search tasks 2.7 — LLM 扩展路径单测（内存 sqlite）。
// 覆盖 test-cases.md 白盒「LLM 扩词数」边界（0/4/5/8/9）、变体走查
// 「输入：非法 JSON/空数组/词数越界/含重复词」与「幂等：10min TTL 内
// 同 query 重复 expand 缓存命中零调用」（缓存命中断言在 handler_test.go
// 的 TestSearchHandlerExpandCacheHit；此处覆盖缓存本体与假时钟过期）。

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// fakeExpander LLM 扩展注入替身（handler_test.go 复用）。
type fakeExpander struct {
	words []string
	err   error
	calls int
}

func (f *fakeExpander) ExpandQuery(_ context.Context, _ string) ([]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.words, nil
}

// TestParseExpandWords covers 白盒「LLM 扩词数」边界与解析容错表。
func TestParseExpandWords(t *testing.T) {
	words := func(n int) string { // n 个不重复词的 JSON 数组
		parts := make([]string, n)
		for i := range parts {
			parts[i] = fmt.Sprintf("\"扩词%d号\"", i)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}

	tests := []struct {
		name    string
		content string
		want    []string
		wantErr bool
	}{
		{"两个有效词", `["电池安全","热失控"]`, []string{"电池安全", "热失控"}, false},
		{"词数4 仍执行", words(4), nil, false}, // 期望恰好 4 个，下方断言长度
		{"词数5 全用", words(5), nil, false},
		{"词数8 全用", words(8), nil, false},
		{"词数9 截前8", words(9), nil, false},
		{"含重复去重", `["电池","电池","热失控","电池"]`, []string{"电池", "热失控"}, false},
		{"大小写折叠去重", `["Battery","battery","Cell"]`, []string{"Battery", "Cell"}, false},
		{"空数组失败降级", `[]`, nil, true},
		{"全空串失败降级", `["", "  "]`, nil, true},
		{"非 JSON 失败降级", `抱歉我不能输出数组`, nil, true},
		// JSON 对象包数组：与「前后杂散文本」同源容错——截取括号平衡片段提取
		{"JSON 对象内数组提取", `{"words":["电池安全"]}`, []string{"电池安全"}, false},
		{"markdown 代码块容错", "```json\n[\"电池安全\"]\n```", []string{"电池安全"}, false},
		{"前后杂散文本容错", `好的，结果如下：["电池安全","热失控"] 请查收`, []string{"电池安全", "热失控"}, false},
		{"词项两侧空白剥离", `[" 电池安全 ","热失控"]`, []string{"电池安全", "热失控"}, false},
		{"空内容失败", ``, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExpandWords(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			if tt.want != nil {
				require.Equal(t, tt.want, got)
				return
			}
			// 词数边界：4/5/8 原样，9 截 8
			require.NotEmpty(t, got)
			require.LessOrEqual(t, len(got), expandMaxWords)
		})
	}

	// 词数边界的精确长度断言
	for n, want := range map[int]int{4: 4, 5: 5, 8: 8, 9: 8} {
		got, err := ParseExpandWords(words(n))
		require.NoError(t, err)
		require.Len(t, got, want, "输入 %d 词", n)
	}
}

// TestExpandCacheTTL covers TTL 缓存：命中、过期、副本隔离（假时钟）。
func TestExpandCacheTTL(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	cache := newExpandCache(10 * time.Minute)
	cache.now = func() time.Time { return now }

	require.False(t, func() bool { _, ok := cache.get("q"); return ok }(), "空缓存必 miss")

	cache.set("q", []string{"电池", "热失控"})
	got, ok := cache.get("q")
	require.True(t, ok)
	require.Equal(t, []string{"电池", "热失控"}, got)

	// 过期 → miss
	now = now.Add(10*time.Minute + time.Second)
	_, ok = cache.get("q")
	require.False(t, ok)

	// 重新写入后命中；返回副本，改动不污染缓存
	cache.set("q", []string{"电池"})
	got, ok = cache.get("q")
	require.True(t, ok)
	got[0] = "污染"
	again, _ := cache.get("q")
	require.Equal(t, "电池", again[0])

	// nil 容器：get/set 均 no-op 不 panic
	var nilCache *expandCache
	nilCache.set("q", []string{"x"})
	_, ok = nilCache.get("q")
	require.False(t, ok)
}

// setupExpandService 构造 sqlite + Service（扩展路白盒测试用）。
func setupExpandService(t *testing.T, expander Expander) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:expand-test-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.AISettings{}))
	database.DB = db
	return NewService(db, &handlerFakeSemantic{}, expander, nil)
}

// TestRunExpandLiteralRecall covers 扩词补跑 L1/L2 字面召回 → type=expand
// 榜条目与行投影产出；LLM 失败返回 error 由调用方降级。
func TestRunExpandLiteralRecall(t *testing.T) {
	expander := &fakeExpander{words: []string{"电池热失控", "无效词"}}
	svc := setupExpandService(t, expander)

	feed := models.Feed{Title: "源", URL: "https://example.com/expand"}
	require.NoError(t, database.DB.Create(&feed).Error)
	hit := models.Article{FeedID: feed.ID, Title: "电池热失控事故复盘", Link: "https://example.com/a1"}
	miss := models.Article{FeedID: feed.ID, Title: "无关文章", Link: "https://example.com/a2"}
	require.NoError(t, database.DB.Create(&hit).Error)
	require.NoError(t, database.DB.Create(&miss).Error)

	pq, err := svc.prep.Prepare(context.Background(), "电动车自燃")
	require.NoError(t, err)

	run, err := svc.runExpand(context.Background(), pq, ArchiveInclude)
	require.NoError(t, err)
	require.Equal(t, []string{"电池热失控", "无效词"}, run.words)
	require.Len(t, run.entries, 1)
	require.Equal(t, hit.ID, run.entries[0].ArticleID)
	require.Len(t, run.entries[0].Reasons, 1)
	require.Equal(t, ReasonExpand, run.entries[0].Reasons[0].Type)
	require.Equal(t, "电池热失控", run.entries[0].Reasons[0].Text)
	require.Positive(t, run.entries[0].Score)
	require.Contains(t, run.rows, hit.ID)
	require.Equal(t, "电池热失控事故复盘", run.rows[hit.ID].Title)

	// LLM 失败：error 交由调用方降级
	failed := &fakeExpander{err: fmt.Errorf("llm down")}
	svc2 := setupExpandService(t, failed)
	pq2, err := svc2.prep.Prepare(context.Background(), "电动车自燃")
	require.NoError(t, err)
	_, err = svc2.runExpand(context.Background(), pq2, ArchiveInclude)
	require.Error(t, err)

	// 未配置 expander（nil）：恒降级
	svc3 := setupExpandService(t, nil)
	pq3, err := svc3.prep.Prepare(context.Background(), "电动车自燃")
	require.NoError(t, err)
	_, err = svc3.runExpand(context.Background(), pq3, ArchiveInclude)
	require.Error(t, err)
}

// TestRunExpandCacheShortCircuit covers 缓存命中时零 LLM 调用 + 归档口径
// 透传（active-only 下归档行不进扩展榜）。
func TestRunExpandCacheShortCircuit(t *testing.T) {
	expander := &fakeExpander{words: []string{"电池热失控"}}
	svc := setupExpandService(t, expander)

	feed := models.Feed{Title: "源", URL: "https://example.com/expand2"}
	require.NoError(t, database.DB.Create(&feed).Error)
	active := models.Article{FeedID: feed.ID, Title: "电池热失控复盘", Link: "https://example.com/b1"}
	archived := models.Article{FeedID: feed.ID, Title: "电池热失控旧闻", Link: "https://example.com/b2", Archived: true}
	require.NoError(t, database.DB.Create(&active).Error)
	require.NoError(t, database.DB.Create(&archived).Error)

	ctx := context.Background()
	pq, err := svc.prep.Prepare(ctx, "电动车自燃")
	require.NoError(t, err)

	run1, err := svc.runExpand(ctx, pq, ArchiveInclude)
	require.NoError(t, err)
	require.Len(t, run1.entries, 2) // include：归档也在
	require.Equal(t, 1, expander.calls)

	// 第二次同 query：缓存命中，零 LLM 调用；active-only 重新执行字面召回
	run2, err := svc.runExpand(ctx, pq, ArchiveActiveOnly)
	require.NoError(t, err)
	require.Equal(t, 1, expander.calls, "TTL 内缓存命中：零重复 LLM 调用")
	require.Len(t, run2.entries, 1)
	require.Equal(t, active.ID, run2.entries[0].ArticleID)
}

// blockingExpander 模拟慢 LLM（review R-1）：阻塞到 ctx 取消才返回。
type blockingExpander struct{ calls int }

func (b *blockingExpander) ExpandQuery(ctx context.Context, _ string) ([]string, error) {
	b.calls++
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestRunExpandTimeoutBound 扩展路超时上限（review R-1）：慢 LLM 不得把主
// 响应扣住 vendor 默认 600s——runExpand 自带 WithTimeout，超时走 LLM 失败
// 同款降级（返回 error，调用方静默吸收主结果照常）。
func TestRunExpandTimeoutBound(t *testing.T) {
	orig := expandPathTimeout
	expandPathTimeout = 80 * time.Millisecond
	t.Cleanup(func() { expandPathTimeout = orig })

	svc := setupExpandService(t, &blockingExpander{})
	pq, err := svc.prep.Prepare(context.Background(), "电池热失控")
	require.NoError(t, err)

	start := time.Now()
	_, err = svc.runExpand(context.Background(), pq, ArchiveInclude)
	elapsed := time.Since(start)
	require.Error(t, err, "超时必须返回 error（降级信号）")
	require.Less(t, elapsed, 5*time.Second, "不得等 vendor 默认 600s")
}
