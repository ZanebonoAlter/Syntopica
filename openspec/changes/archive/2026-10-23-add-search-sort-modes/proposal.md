<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: reading -->

# Proposal: add-search-sort-modes

## Why

站内搜索当前唯一的排序是 RRF 融合相关度，时间完全不参与。真库实测（2026-10-23）：

- `q=英伟达` 近 3 天有 **118 篇**标题/描述命中，但首屏 50 条只有 8 条发布于 10-22、当天 **0 条**；全量 1147 条命中里大量 8~9 月老文章靠精确关键词压在最前。
- `q=芯片` 首屏全是 9 月的（最快的结果也要翻很多页）。
- 全库 47,701 篇中 45,668 篇（96%）已归档，而搜索默认含归档——老文章基数进一步放大「最新新闻看不到」的观感。

结论：**不是召回不到，也不是缺关键字匹配**（近 3 天 118 篇关键字命中真实存在），而是排序没有时间维度、用户也无法切换。搜索结果页因此只能当「找老文章」用，不能当「看实时新闻」用。

## What Changes

- `GET /api/search` 新增 `sort_by=date|relevance`（默认 `date`）：
  - `date`（默认）：按发布日（Asia/Shanghai，UTC+8）倒序分组；同一天内按现有融合相关度（RRF，含语义分量）倒序；日内再以精确发布时间倒序、文章 ID 升序兜底，保证跨页稳定。
  - `relevance`：保持现有 RRF 融合相关度排序（关键词 + 语义 + 扩展）。
  - 响应 `data` 顶层回显 `sortBy`（实际生效值）；非法值静默回退 `date`（与 `archived`/`expand` 枚举回退同款，不报错）。
- **BREAKING（仅默认顺序）**：未传 `sort_by` 的请求从「相关度优先」变为「时间优先」。结果集与 `total` 不变，只改变顺序；显式传 `sort_by=relevance` 可恢复旧顺序。
- 搜索页过滤行新增分段切换「**最新优先（默认）**｜相关度优先」；切换后按当前查询立即重查，加载更多沿用当前排序；首查加载中控件 disabled。
- 非目标：不加时间范围筛选（今天/近 3 天等）；不改召回路径、归档口径、语义降级与扩展区隔。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `article-search`（`openspec/specs/article-search/spec.md`）：
  - MODIFIED「混合搜索聚合端点」：新增 `sort_by` 参数与响应 `sortBy` 回显；默认排序改为时间优先。
  - ADDED「搜索结果排序模式」：日期分组口径、同日内相关度次序、时区日界、空发布时间兜底、稳定分页、非法值回退。
  - MODIFIED「搜索页入口与状态」：新增排序切换控件、默认值、切换即重查与加载更多排序延续。

## Impact

- **API**：`GET /api/search` 新增可选查询参数与响应字段（字段向后兼容；默认顺序为行为变更）。
- **代码**：`backend-go/internal/reader/search/`（handler 参数解析与排序装配、新 `sort.go` 纯函数 + 测试、handler 契约测试）；`front/app/api/search.ts`、`front/app/features/search/composables/useSearch.ts`、`front/app/pages/search.vue`（+ 测试）。
- **数据**：无迁移；只读 `articles.pub_date` / `articles.created_at`。
- **文档**：`docs/reference/api/search.md`、`docs/reference/flow/reading.md`（站内搜索节 + 业务约束 + 变更溯源）。
- **风险**：默认顺序变化可能让习惯旧顺序的用户觉得「最相关的没了」——由排序切换兜底；时间分组固定 UTC+8，与全站 CST 展示一致。
