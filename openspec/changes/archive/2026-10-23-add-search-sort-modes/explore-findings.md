
## 搜索排序接线点：handler/fusion/rows

后端 GET /api/search 编排在 backend-go/internal/reader/search/handler.go `HandleSearch`：先 `Fuse(FuseInput{Keyword: KeywordRankEntries(kwHits), Semantic: SemanticRankEntries(sem.hits), Expand: expandEntries})`（fusion.go，RRF 排序已被 fusion_test.go 锁定），随后建 `rows map[uint]ArticleSearchRow`（kw/sem/expand 三来源合并），再 `total := len(fused)`、offset 切片、组装 results。ArticleSearchRow 已含 `PubDate *time.Time` 与 `CreatedAt time.Time`（repository.go）。排序必须在 rows 池建好之后、offset 切片之前插入 `SortFused(fused, rows, sortBy)`；响应 data 增加 `sortBy` 回显。不可改 Fuse 内部排序（既有测试与语义稳定性）。

**引用**：backend-go/internal/reader/search/handler.go:HandleSearch、backend-go/internal/reader/search/fusion.go:Fuse、backend-go/internal/reader/search/repository.go:ArticleSearchRow

<!-- pinned 2026-10-23T07:46:24Z -->

## 前端搜索排序状态接线点

前端搜索：`front/app/features/search/composables/useSearch.ts` 用 `keyOf(q)` = q + includeArchived + expandEnabled 做幂等指纹；`runSearch` 与 `loadMore` 各自构造 search 参数；`requeryIfExecuted` 是过滤器变更的统一重查入口（loading 时跳过）。`front/app/api/search.ts` 的 `SearchQueryParams`/`buildQueryParams` 映射到后端 snake_case（archived/expand/page/per_page），响应经 `normalizeSearchResponse` 归一。`front/app/pages/search.vue` 过滤行用 AppToggle 两个开关（含归档/LLM 扩展），无分段排序组件；`BoardTimelinePanel.vue` 有 `.tags-sort-toggle` 视觉可复用。测试：`front/app/features/search/composables/useSearch.test.ts`、`front/app/pages/search.test.ts`，fixture 分别叫 `response()`/`searchResponse()`。

**引用**：front/app/features/search/composables/useSearch.ts:keyOf、front/app/api/search.ts:normalizeSearchResponse、front/app/pages/search.vue

<!-- pinned 2026-10-23T07:46:24Z -->
