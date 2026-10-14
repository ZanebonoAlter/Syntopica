package search

// add-article-search tasks 2.6 — GET /api/search 聚合端点编排与响应契约
// （design D6；spec「混合搜索聚合端点」）。
//
// 编排：关键词路（主力，硬错误→500）‖ 语义路（任何 error → semantic.
// skipped=true 静默降级，HTTP 200——红线 11「关键词路径永远在线」；
// ErrSemanticSkipped 哨兵与 SQL 失败同此处置）→ RRF 融合 → offset 分页
// → 窄投影 camelCase + hit_reasons + excerpt。
//
// 依赖注入：SemanticRunner / Expander / ExcerptFunc 均为窄接口注入点。
// ExcerptFunc 的生产实现在 reader/handler（buildExcerpt，article_excerpt.
// go）——本包 import 会成环（2.8 后 reader/handler 反向 import 本包），
// 故由 reader 装配层（routes.go）注入，缺省 nil 时 excerpt 置空。

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

// 分页参数边界（沿 articles 列表端点风格：非法数值硬 400，超上限按
// 100 钳制 + warn——test-cases「响应契约断言」）。
const (
	searchDefaultPerPage = 20
	searchMaxPerPage     = 100
)

// ExcerptFunc 导语构建器注入点：生产实现 = reader/handler.BuildArticleSearchExcerpt
// （≤200 字符纯文本，article-list-projection 契约）。
type ExcerptFunc func(description, content string, noFirecrawlBody bool) string

// SemanticRunner 语义路径执行接口（注入点供 handler 测试替身；
// 生产实现 *SemanticSearcher）。
type SemanticRunner interface {
	Search(ctx context.Context, query string, archived string) ([]SemanticHit, []MatchedTag, error)
}

// Service 搜索聚合编排器。字段均为依赖注入点（白盒测试可换）。
type Service struct {
	db          *gorm.DB
	prep        *QueryPreprocessor
	repo        *SearchRepository
	semantic    SemanticRunner
	expander    Expander     // nil → expand 路径恒降级
	expandCache *expandCache // nil → 不缓存
	excerpt     ExcerptFunc  // nil → excerpt 置空
}

// NewService builds the orchestration service over the given DB. dict 经
// DefaultDict() 单例共享（与列表端点过滤、tagmanagement 失效钩子同一份
// 缓存，一次 InvalidateDict 全消费方生效）。
func NewService(db *gorm.DB, semantic SemanticRunner, expander Expander, excerpt ExcerptFunc) *Service {
	return &Service{
		db:          db,
		prep:        NewQueryPreprocessor(db, DefaultDict()),
		repo:        NewSearchRepository(db),
		semantic:    semantic,
		expander:    expander,
		expandCache: newExpandCache(expandCacheTTL),
		excerpt:     excerpt,
	}
}

// expandRun 一次扩展路径执行的产物：有效扩词（响应回显）+ 扩展榜条目
// （type=expand，FuseInput.Expand）+ 行投影（分页结果组装用）。
type expandRun struct {
	words   []string
	entries []RankEntry
	rows    map[uint]ArticleSearchRow
}

// runExpand executes the LLM expand path (design D5): cache → LLM → per-word
// L1/L2 literal recall（SearchKeywordLiteral，spec：扩词只补跑字面层，
// 不跑 L3 trigram）。任何失败返回 error，调用方静默降级（expand.words
// 置空、扩展榜缺席，主结果不受影响）。
func (s *Service) runExpand(ctx context.Context, pq *PreparedQuery, archived string) (*expandRun, error) {
	if s.expander == nil {
		return nil, errors.New("search: no expander configured")
	}

	// 扩展路上限（review R-1）：airouter vendor 默认 TimeoutSeconds=600s，
	// 不设上限时 expand=on 的慢 LLM 会把主响应串行扣住最坏 10 分钟
	// （design D5「不阻塞首屏」）。超时走 LLM 失败同款降级（主结果正常，
	// 扩展区缺失）；10s 对齐 design「LLM 扩展延迟 1~3s」预期留足余量。
	ctx, cancel := context.WithTimeout(ctx, expandPathTimeout)
	defer cancel()

	words, cached := s.expandCache.get(pq.Raw)
	if !cached {
		var err error
		words, err = s.expander.ExpandQuery(ctx, pq.Raw)
		if err != nil {
			return nil, err
		}
		s.expandCache.set(pq.Raw, words)
	}
	if len(words) == 0 {
		return nil, errors.New("search: expander returned no words")
	}

	run := &expandRun{words: words, rows: make(map[uint]ArticleSearchRow)}
	merged := make(map[uint]*RankEntry)
	for _, word := range words {
		wpq, err := s.prep.Prepare(ctx, word)
		if err != nil {
			// 扩词本身空/纯空白：跳过该词，其余照常
			logging.Warnf("search: expand word %q unusable: %v", word, err)
			continue
		}
		hits, err := s.repo.SearchKeywordLiteral(ctx, wpq, archived)
		if err != nil {
			// 单词字面层失败只降级该词（红线 11 同源精神），不弃整路
			logging.Warnf("search: expand word %q literal recall failed: %v", word, err)
			continue
		}
		for _, h := range hits {
			run.rows[h.Article.ID] = h.Article
			score := KeywordScore(h.Hits)
			e := merged[h.Article.ID]
			if e == nil {
				e = &RankEntry{ArticleID: h.Article.ID}
				merged[h.Article.ID] = e
			}
			if score > e.Score {
				e.Score = score
			}
			e.Reasons = append(e.Reasons, HitReason{Type: ReasonExpand, Text: word, Score: score})
		}
	}
	run.entries = make([]RankEntry, 0, len(merged))
	for _, e := range merged {
		run.entries = append(run.entries, *e)
	}
	return run, nil
}

// semanticResult 语义路 goroutine 的回传载体。
type semanticResult struct {
	hits []SemanticHit
	tags []MatchedTag
	err  error
}

// expandResult 扩展路 goroutine 的回传载体（与语义路并行发车）。
type expandResult struct {
	run *expandRun
	err error
}

// HandleSearch is the GET /api/search gin handler（spec「混合搜索聚合端点」）。
// 参数：q（必填，空/纯空白→400）、archived（include|active-only，缺省
// include，非法回退默认不报错）、expand（off|on，缺省 off，非法回退 off）、
// page/per_page（≥1 整数，非法→400；per_page>100 钳 100+warn）。
func (s *Service) HandleSearch(c *gin.Context) {
	start := time.Now()

	page, perPage, ok := parseSearchPagination(c)
	if !ok {
		return // 已回 400
	}
	archived := parseArchivedParam(c.Query("archived"))
	expandOn := c.Query("expand") == "on"

	ctx := c.Request.Context()

	// 预处理（含空查询哨兵判定——handler 层先拦截纯空白 q，保证「不产生
	// 任何召回动作」；Prepare 内部对空串同样返回 ErrEmptyQuery）。
	if strings.TrimSpace(c.Query("q")) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "q is required"})
		return
	}
	pq, err := s.prep.Prepare(ctx, c.Query("q"))
	if err != nil {
		if errors.Is(err, ErrEmptyQuery) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "q is required"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// 关键词路（主力）与语义路（增强）并行：语义路内部自带 3s 超时，
	// 任何 error 都只降级为 skipped，不让端点报错（红线 11 / BE-2 契约）。
	semCh := make(chan semanticResult, 1)
	go func() {
		hits, tags, err := s.semantic.Search(ctx, pq.Raw, archived)
		semCh <- semanticResult{hits: hits, tags: tags, err: err}
	}()

	// LLM 扩展路与语义路同时发车（原串行：语义 join 后才跑扩词，慢 LLM
	// 时总延迟≈两者之和；并行后≈两者最大值——慢 embedding + 慢 chat
	// 各自封顶 3s/10s，不再叠加）。失败静默降级语义不变。
	var expandCh chan expandResult
	if expandOn {
		expandCh = make(chan expandResult, 1)
		go func() {
			run, err := s.runExpand(ctx, pq, archived)
			expandCh <- expandResult{run: run, err: err}
		}()
	}

	kwHits, err := s.repo.SearchKeyword(ctx, pq, archived)
	if err != nil {
		// 关键词路径 SQL 硬错误是真实故障，如实 500（区别于增强层降级）
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	sem := <-semCh
	semanticSkipped := sem.err != nil
	if sem.err != nil && !errors.Is(sem.err, ErrSemanticSkipped) {
		logging.Warnf("search: semantic path failed, degraded to keyword-only: %v", sem.err)
	}

	// LLM 扩展路（可选增强）：失败静默降级，expand.words 置空回显。
	var expandWords []string
	var expandEntries []RankEntry
	expandRows := map[uint]ArticleSearchRow{}
	if expandCh != nil {
		exp := <-expandCh
		if exp.err != nil {
			logging.Warnf("search: expand path failed, degraded: %v", exp.err)
			expandWords = []string{}
		} else {
			expandWords = exp.run.words
			expandEntries = exp.run.entries
			for id, row := range exp.run.rows {
				expandRows[id] = row
			}
		}
	}

	// RRF 融合 + offset 分页。
	fused := Fuse(FuseInput{
		Keyword:  KeywordRankEntries(kwHits),
		Semantic: SemanticRankEntries(sem.hits), // skipped 时为 nil，Fuse 容忍
		Expand:   expandEntries,
	})
	total := len(fused)
	offset := (page - 1) * perPage
	pageEnd := offset + perPage
	if offset > total {
		offset = total
	}
	if pageEnd > total {
		pageEnd = total
	}

	// 行投影池：三路命中行合并（任一路带回即用；窄投影字段一致）。
	rows := make(map[uint]ArticleSearchRow, total)
	for _, h := range kwHits {
		rows[h.Article.ID] = h.Article
	}
	for _, h := range sem.hits {
		if _, ok := rows[h.Article.ID]; !ok {
			rows[h.Article.ID] = h.Article
		}
	}
	for id, row := range expandRows {
		if _, ok := rows[id]; !ok {
			rows[id] = row
		}
	}

	results := make([]map[string]any, 0, pageEnd-offset)
	for _, fr := range fused[offset:pageEnd] {
		row, ok := rows[fr.ArticleID]
		if !ok {
			continue // 不可能：榜单条目均源自携带行投影的命中
		}
		results = append(results, s.searchResultItem(row, fr.HitReasons))
	}

	semanticBlock := gin.H{"skipped": semanticSkipped}
	if !semanticSkipped {
		tags := make([]gin.H, 0, len(sem.tags))
		for _, t := range sem.tags {
			tags = append(tags, gin.H{"label": t.Label, "score": t.Cosine})
		}
		// 零命中 = 空标签列表（区别于服务不可用的 skipped）
		semanticBlock["tags"] = tags
	}

	data := gin.H{
		"results":  results,
		"semantic": semanticBlock,
		"total":    total,
		"page":     page,
		"perPage":  perPage,
		"tookMs":   time.Since(start).Milliseconds(),
	}
	// expand 块仅 expand=on 时携带：成功回显词表，失败空词表（前端区分
	// 「扩展已开但失败」与「未开」；expand=off 整块缺席）。
	if expandOn {
		data["expand"] = gin.H{"words": expandWords}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// searchResultItem 组装单个结果项：窄投影镜像（articleListColumns 的
// camelCase 形态，article-list-projection 契约——正文类大字段零下发）+
// archived + hitReasons + excerpt（注入的 ExcerptFunc 构建，≤200 字符）。
func (s *Service) searchResultItem(row ArticleSearchRow, reasons []HitReason) map[string]any {
	var excerpt string
	if s.excerpt != nil {
		excerpt = s.excerpt(row.ExcerptDescription, row.ExcerptContent, row.NoFirecrawlBody == 1)
	}
	hitReasons := make([]HitReason, len(reasons))
	copy(hitReasons, reasons)
	return map[string]any{
		"id":                 row.ID,
		"feedId":             row.FeedID,
		"title":              row.Title,
		"link":               row.Link,
		"imageUrl":           row.ImageURL,
		"pubDate":            models.FormatDatetimeCSTPtr(row.PubDate),
		"author":             row.Author,
		"read":               row.Read,
		"favorite":           row.Favorite,
		"archived":           row.Archived,
		"summaryStatus":      row.SummaryStatus,
		"summaryGeneratedAt": models.FormatDatetimeCSTPtr(row.SummaryGeneratedAt),
		"firecrawlStatus":    row.FirecrawlStatus,
		"firecrawlError":     row.FirecrawlError,
		"firecrawlCrawledAt": models.FormatDatetimeCSTPtr(row.FirecrawlCrawledAt),
		"completionError":    row.CompletionError,
		"createdAt":          models.FormatDatetimeCST(row.CreatedAt),
		"excerpt":            excerpt,
		"hitReasons":         hitReasons,
	}
}

// parseSearchPagination 校验 page/per_page：缺省 1/20；非整数或 <1 → 400
// （spec「非法分页参数」：不执行搜索）；per_page>100 → 钳 100 + warn
// （沿列表端点风格）。
func parseSearchPagination(c *gin.Context) (page, perPage int, ok bool) {
	badRequest := func(msg string) bool {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": msg})
		return false
	}

	rawPage := c.DefaultQuery("page", "1")
	page, err := strconv.Atoi(rawPage)
	if err != nil || page < 1 {
		return 0, 0, badRequest("page must be an integer >= 1")
	}

	rawPerPage := c.DefaultQuery("per_page", strconv.Itoa(searchDefaultPerPage))
	perPage, err = strconv.Atoi(rawPerPage)
	if err != nil || perPage < 1 {
		return 0, 0, badRequest("per_page must be an integer >= 1")
	}
	if perPage > searchMaxPerPage {
		logging.Warnf("search: per_page=%d exceeds max=%d, clamped to %d", perPage, searchMaxPerPage, searchMaxPerPage)
		perPage = searchMaxPerPage
	}
	return page, perPage, true
}

// parseArchivedParam 归档口径：include（默认，含归档——搜索域专属契约，
// 与列表端点的默认排除互不影响）/ active-only；非法值静默回默认不报错
// （test-cases「响应契约断言」：枚举类回退默认，仅数值分页硬 400）。
func parseArchivedParam(raw string) string {
	if raw == ArchiveActiveOnly {
		return ArchiveActiveOnly
	}
	return ArchiveInclude
}

// ============================================================================
// 进程级默认 Service（路由注册用）
// ============================================================================

var (
	defaultServiceMu sync.RWMutex
	defaultService   *Service
)

// SetDefaultService installs the process-wide service used by the exported
// HandleSearch. Called from reader route assembly（routes.go），where
// handler.BuildArticleSearchExcerpt is importable without a cycle.
func SetDefaultService(s *Service) {
	defaultServiceMu.Lock()
	defaultService = s
	defaultServiceMu.Unlock()
}

// DefaultService returns the installed service, lazily building a fallback
// over database.DB when route assembly did not run（语义/扩展路因 nil
// Router 恒降级，关键词路照常——搜索永远在线）。
func DefaultService() *Service {
	defaultServiceMu.RLock()
	svc := defaultService
	defaultServiceMu.RUnlock()
	if svc != nil {
		return svc
	}
	fallback := NewService(
		database.DB,
		NewSemanticSearcher(database.DB, &AirouterEmbedder{}),
		&AirouterExpander{},
		nil,
	)
	SetDefaultService(fallback)
	return fallback
}

// HandleSearch is the route-registration entry（rg.GET("/search", ...)）：
// delegates to the process-wide default service.
func HandleSearch(c *gin.Context) {
	DefaultService().HandleSearch(c)
}
