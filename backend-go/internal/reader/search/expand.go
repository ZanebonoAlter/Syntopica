package search

// add-article-search tasks 2.7 — LLM 查询扩展路径（design D5）。
//
// expand=on 时查询串交 airouter chat（窄接口 Expander 注入，同 Embedder
// 模式——不 import 全局单例），prompt 约束输出 JSON 字符串数组 5~8 词；
// 解析容错：非 JSON / 空数组 / 去重后 0 词 → 失败降级（主结果正常，仅
// 扩展区缺失）；有效词 1~4 仍执行（有总比无好）；>8 截前 8；去重。
// 同查询扩展结果内存 TTL 缓存 10min（会话内重复搜索不重复调用 LLM）。
//
// 扩词的字面补跑（L1/L2 LIKE，spec「补跑 L1/L2 字面召回」）在
// handler.go 的 Service.runExpand 编排；本文件只负责「LLM 调用 + 解析 +
// 缓存」三件事。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"syntopica-backend/internal/platform/airouter"
)

// expandOperation 是 LLM 扩展的 airouter 业务操作名（AI 调用记录红线：
// operation 必填、经 Router.Chat 留痕）。扩展结果走内存 TTL 缓存（本
// 文件），不进 ai_embedding_cache——那是 embedding 专用表。
const expandOperation = "article_search_expand"

// 扩词数边界（design D5：prompt 要求 5~8；解析侧宽容 1~8，>8 截前 8）。
const (
	expandMaxWords      = 8
	expandCacheTTL      = 10 * time.Minute
	expandCacheMaxPrune = 128 // set 时顺带清理过期项的触发阈值（防无界增长）
)

// expandPathTimeout 扩展路（LLM 调用 + 补跑字面召回）总上限，review R-1：
// 不设上限时 vendor 默认 600s 会串行扣住主响应；超时走 LLM 失败同款降级。
// var 而非 const：测试可缩短（语义路 timeout 同款先例）。
var expandPathTimeout = 10 * time.Second

// expandSystemPrompt 约束 LLM 只输出 JSON 字符串数组。
const expandSystemPrompt = `你是站内文章搜索的查询扩展器。把用户查询扩展为 5~8 个用于文章标题/导语关键词匹配的搜索词。
要求：
- 输出 JSON 字符串数组，不要输出任何其他文本、解释或 markdown 代码块
- 每个词 2~12 个字符，保留查询的核心概念，包含同义词与更具体的表述
- 不要重复用户查询本身，不要输出空字符串
示例输入：电动车电池安全
示例输出：["电池安全","动力电池","热失控","电动车自燃","锂离子电池"]`

// Expander LLM 查询扩展的窄接口（依赖注入隔离 airouter，测试注入 mock）。
type Expander interface {
	ExpandQuery(ctx context.Context, query string) ([]string, error)
}

// AirouterExpander 把 airouter.Router.Chat 适配为 Expander（生产实现，
// 由调用方构造注入 Router）。
type AirouterExpander struct {
	Router *airouter.Router
}

// expandWordSchema 约束 chat 输出为字符串数组（JSONMode + schema 双保险，
// 解析侧仍有容错兜底）。
func expandWordSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type:  "array",
		Items: &airouter.JSONSchema{Type: "string"},
	}
}

// ExpandQuery asks the LLM to expand the query into 5~8 search words.
// Returns normalized words (trimmed, deduped, capped at 8); an empty result
// is reported as an error so callers degrade uniformly.
func (a *AirouterExpander) ExpandQuery(ctx context.Context, query string) ([]string, error) {
	if a == nil || a.Router == nil {
		return nil, errors.New("search: nil airouter router")
	}
	result, err := a.Router.Chat(ctx, airouter.ChatRequest{
		Operation:  expandOperation,
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: expandSystemPrompt},
			{Role: "user", Content: query},
		},
		JSONMode:   true,
		JSONSchema: expandWordSchema(),
		Metadata:   map[string]any{"query": query},
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("search: empty chat result")
	}
	return ParseExpandWords(result.Content)
}

// ParseExpandWords parses the LLM output into usable expand words:
// trim each entry, drop empties, dedupe (case-folded key), cap at 8.
// 非 JSON / 空数组 / 去重后 0 词 → 错误（调用方降级，spec「LLM 失败时主
// 结果正常返回」）。容错：容忍 markdown 代码块包裹与前后杂散文本
// （截取首个 '[' 到最后一个 ']' 的片段再解析）。
func ParseExpandWords(content string) ([]string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, errors.New("search: empty expand response")
	}
	trimmed = stripCodeFence(trimmed)

	var words []string
	if err := json.Unmarshal([]byte(trimmed), &words); err != nil {
		start := strings.Index(trimmed, "[")
		end := strings.LastIndex(trimmed, "]")
		if start < 0 || end <= start {
			return nil, fmt.Errorf("search: expand response is not a JSON array: %w", err)
		}
		words = nil
		if err2 := json.Unmarshal([]byte(trimmed[start:end+1]), &words); err2 != nil {
			return nil, fmt.Errorf("search: expand response is not a JSON array: %w", err2)
		}
	}

	seen := make(map[string]struct{}, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		key := strings.ToLower(w)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, w)
		if len(out) >= expandMaxWords {
			break
		}
	}
	if len(out) == 0 {
		return nil, errors.New("search: expand response yielded no valid words")
	}
	return out, nil
}

// stripCodeFence removes a single wrapping markdown code fence
// （```json ... ```）——JSONMode 关闭或弱模型常见的输出形态。
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		// 丢掉 ```json 的语言行
		s = s[i+1:]
	}
	return strings.TrimSuffix(strings.TrimSpace(s), "```")
}

// expandEntry TTL 缓存条目。
type expandEntry struct {
	words     []string
	expiresAt time.Time
}

// expandCache 同查询扩展结果的内存 TTL 缓存（design D5：10min）。简单
// map+mutex+时间戳，无需外部缓存；now 注入点供测试替换时钟。
type expandCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]expandEntry
}

func newExpandCache(ttl time.Duration) *expandCache {
	return &expandCache{
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[string]expandEntry),
	}
}

// get returns cached words for the query when fresh.
func (c *expandCache) get(query string) ([]string, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[query]
	if !ok || c.now().After(e.expiresAt) {
		return nil, false
	}
	// 返回副本：调用方/缓存互不影响
	out := make([]string, len(e.words))
	copy(out, e.words)
	return out, true
}

// set stores words for the query; opportunistically prunes expired entries
// once the map grows past expandCacheMaxPrune（防无界增长，失败结果不缓存
// ——下次重试 LLM）。
func (c *expandCache) set(query string, words []string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= expandCacheMaxPrune {
		now := c.now()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[query] = expandEntry{
		words:     append([]string(nil), words...),
		expiresAt: c.now().Add(c.ttl),
	}
}
