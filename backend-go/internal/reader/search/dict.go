// Package search implements the add-article-search backend search pipeline
// (design D2–D5): the topic-tag label dictionary cache, the four-layer query
// preprocessor, the trigram keyword repository, the tag-vector semantic path
// and the RRF fusion ranking. HTTP handler and route wiring live in
// internal/reader/handler / internal/reader/routes.go (tasks 2.6–2.8) and are
// intentionally out of this package's scope.
package search

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// 词典词条长度边界（spec「查询预处理四层管线」：3~8 字、词内无空白，
// rune 计非字节）。
const (
	dictMinWordRunes = 3
	dictMaxWordRunes = 8
)

// DictCache 缓存标签概念词典（design D2）：topic_tags.label 中 3~8 字且词内
// 无空白的去重全集。启动懒加载——首次切词前 EnsureLoaded；标签写入方调用
// InvalidateDict 使缓存失效，下一次 EnsureLoaded 重新装载。
//
// 失效钩子接线：本包只暴露 InvalidateDict，不 import tagmanagement（避免
// reader/search → tagmanagement 深路径 import 环，depguard 边界同样禁止）。
// 由标签写入方（tagmanagement 域，tasks 2.8/后续）调用本钩子。
type DictCache struct {
	db *gorm.DB

	mu        sync.RWMutex
	words     map[string]struct{}
	maxLen    int    // 词典内最长词条 rune 数，匹配扫描上界
	loaded    bool   // false = 从未装载（首次装载阻塞）
	stale     bool   // true = 已失效，后台重载中或待触发（stale-while-revalidate）
	reloading bool   // 单飞：后台重载在途回收标志
	gen       uint64 // 失效代数（review R-2：重载窗口内的失效不得被完成时无条件清 stale 吞掉）
}

// NewDictCache builds a lazily-loaded dictionary over the given DB.
func NewDictCache(db *gorm.DB) *DictCache {
	return &DictCache{db: db}
}

// InvalidateDict marks the cache stale. 不清空当前词表：后续查询继续用旧词
// （stale-while-revalidate），首个到达的查询触发后台单飞重载，新词在重载
// 完成后的下一次查询生效（2026-10-14 真库实测：16.5K 标签 DISTINCT 加载
// ~2s，打标活跃期写入频繁，同步重载会把每次失效后自首个查询拖到 2s+，
// 违反 spec P50<1s；词典断档几秒不影响 L0/L2/L3 兑底）。
// 需要同步等待重载完成的调用方（测试/运维）用 EnsureFresh。
func (d *DictCache) InvalidateDict() {
	d.mu.Lock()
	d.stale = true
	d.gen++
	d.mu.Unlock()
}

// EnsureLoaded loads the dictionary on first use（首次阻塞——空词典会静默
// 降级 L1，首查必须等到词表就绪）；已装载后变为 stale-while-revalidate：
// 若已失效则触发一次后台单飞重载并立即返回旧词，不阻塞当前查询。
// 重载失败不清空旧词（沿用旧表），下个查询重试。并发安全。
func (d *DictCache) EnsureLoaded(ctx context.Context) error {
	d.mu.RLock()
	loaded, stale, reloading := d.loaded, d.stale, d.reloading
	d.mu.RUnlock()
	if loaded && (!stale || reloading) {
		return nil
	}
	if !loaded {
		// 首次装载：同步（磁盘上还没有任何词表可用）。
		return d.reload(ctx)
	}
	// stale：单飞触发后台重载，当前查询用旧词立即继续。
	d.mu.Lock()
	if d.reloading {
		d.mu.Unlock()
		return nil
	}
	d.reloading = true
	d.mu.Unlock()
	go func() {
		defer func() {
			d.mu.Lock()
			d.reloading = false
			d.mu.Unlock()
		}()
		_ = d.reload(context.WithoutCancel(ctx))
	}()
	return nil
}

// EnsureFresh 触发重载（如已失效）并阻塞至完成：测试与需要立即看到新词的
// 调试入口；生产查询路径用 EnsureLoaded。
func (d *DictCache) EnsureFresh(ctx context.Context) error {
	d.mu.RLock()
	loaded, stale := d.loaded, d.stale
	d.mu.RUnlock()
	if loaded && !stale {
		return nil
	}
	return d.reload(ctx)
}

// reload 从 topic_tags 重新装载词表并原子替换（含首次装载）。完成时仅在
// 装载期间无新失效（gen 未变）时清 stale——重载窗口内的 InvalidateDict
// 不被吞，下一个查询会再触发一轮重载（review R-2）。
func (d *DictCache) reload(ctx context.Context) error {
	if d.db == nil {
		return errors.New("search dict: nil db")
	}
	d.mu.RLock()
	genAtStart := d.gen
	d.mu.RUnlock()

	var labels []string
	if err := d.db.WithContext(ctx).Model(&models.TopicTag{}).Distinct().Pluck("label", &labels).Error; err != nil {
		return err
	}

	words := make(map[string]struct{}, len(labels))
	maxLen := 0
	for _, label := range labels {
		if w, ok := dictWord(label); ok {
			words[w] = struct{}{}
			if n := len([]rune(w)); n > maxLen {
				maxLen = n
			}
		}
	}

	d.mu.Lock()
	d.words = words
	d.maxLen = maxLen
	d.loaded = true
	if d.gen == genAtStart {
		d.stale = false
	}
	d.mu.Unlock()
	return nil
}

// dictWord validates one topic_tags.label for dictionary membership: trimmed,
// 3~8 runes, no whitespace inside the word（test-cases 白盒「词典词长」边界）.
func dictWord(label string) (string, bool) {
	w := strings.TrimSpace(label)
	if w == "" {
		return "", false
	}
	for _, r := range w {
		if unicode.IsSpace(r) {
			return "", false
		}
	}
	n := len([]rune(w))
	if n < dictMinWordRunes || n > dictMaxWordRunes {
		return "", false
	}
	return w, true
}

// Size returns the number of loaded dictionary words (0 before first load);
// observability/test helper.
func (d *DictCache) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.words)
}

// Segment runs forward maximum matching over the loaded dictionary. Returns
// nil when not loaded or nothing matches; unmatched characters are consumed
// without emitting（残段交 L2/L3 兜底）.
func (d *DictCache) Segment(query string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return SegmentForwardMaxMatch(d.words, query)
}

// SegmentForwardMaxMatch is the pure forward-maximum-match segmenter: at each
// position it takes the longest dictionary word starting there (consuming
// scan); on no match it skips one rune. Longest-first resolves prefix
// ambiguity（「人工」vs「人工智能」→ 取「人工智能」）. Accepts arbitrary word
// sets so unit tests can exercise words shorter than the 3-rune floor.
func SegmentForwardMaxMatch(words map[string]struct{}, query string) []string {
	if len(words) == 0 || query == "" {
		return nil
	}
	maxLen := 0
	for w := range words {
		if n := len([]rune(w)); n > maxLen {
			maxLen = n
		}
	}
	if maxLen == 0 {
		return nil
	}

	runes := []rune(query)
	var out []string
	i := 0
	for i < len(runes) {
		matched := ""
		for l := min(maxLen, len(runes)-i); l >= 1; l-- {
			cand := string(runes[i : i+l])
			if _, ok := words[cand]; ok {
				matched = cand // 最长优先：从候选上限往下探到首个命中
				break
			}
		}
		if matched == "" {
			i++
			continue
		}
		out = append(out, matched)
		i += len([]rune(matched))
	}
	return out
}
