# Design: add-search-sort-modes

## Context

现状见 proposal.md「Why」。技术现状：

- `backend-go/internal/reader/search/handler.go` 在 `HandleSearch` 里先调用 `Fuse`（`fusion.go`）得到纯 RRF 顺序，再按 offset 切片分页；行投影 `ArticleSearchRow` 已携带 `PubDate *time.Time` 与 `CreatedAt time.Time`。
- `Fuse` 的内部排序已被 `fusion_test.go` 锁定为「RRF 分 → 关键词分 → 语义分 → ID 升序」。
- 前端 `useSearch` 用 `keyOf(q)` 指纹（q + archived + expand）去重，`loadMore` 独立发请求。
- 搜索页过滤行已有 `AppToggle` 两个开关，无现成的排序分段组件；`BoardTimelinePanel.vue` 有一套可复用的分段按钮视觉模式。

## Goals / Non-Goals

**Goals**：默认让最新日期的命中先出现；同一天内仍让更相关的排前面；提供可显式切回旧顺序的开关；两种排序共享同一召回集与分页。

**Non-Goals**：不做时间范围筛选（今天/近 3 天）；不改关键词/语义/扩展三路召回；不改归档口径；不引入数据库索引或迁移。

## Decisions

### D1. API 参数命名与默认值

- **结论**：`GET /api/search` 新增 `sort_by=date|relevance`，缺省 `date`；响应 `data.sortBy` 回显生效值；非法值静默回退 `date`。
- **落点**：`backend-go/internal/reader/search/handler.go:parseSortByParam`
- **取舍**：选 `date` 而非 `time`，与既有 `GET /api/articles?sort_by=date|relevance` 词表一致；不做 400 硬错误，与 `archived`/`expand` 的枚举回退惯例一致（数值分页才硬 400）。

### D2. 日期分组时区与空值兜底

- **结论**：按 Asia/Shanghai（固定 UTC+8）的日历日分组；`pub_date` 为空回退 `created_at`；两者皆为零值时自然排最后。
- **落点**：`backend-go/internal/reader/search/sort.go:sortDayKey`
- **取舍**：不用 UTC 日界（会让每天 00:00~08:00 的新闻被算到前一天）；不做精确时间主排序（用户已确认同一天内要按相关度，精确时间会让第二顺位失效）。

### D3. 排序放在融合之后，不塞进 Fuse

- **结论**：`Fuse` 保持纯 RRF 职责；新增 `SortFused(fused, rows, mode)`，handler 在行投影池建好后、分页切片前调用。`relevance` 模式原样返回，`date` 模式复制后排序（不改入参）。
- **落点**：`backend-go/internal/reader/search/sort.go:SortFused`
- **取舍**：不把 PubDate 塞进 `FusedResult`/`RankEntry`（会污染融合输入、破坏既有 `fusion_test.go` 的纯函数测试），也不在 SQL 层排序（语义/扩展路是 Go 侧聚合，SQL 排序覆盖不到全集）。

### D4. 同一天内的第二顺位

- **结论**：同一天内按现有融合相关度降序：RRF 分数 → 关键词分 → 语义分 → 精确发布时间降序 → 文章 ID 升序。
- **落点**：tasks.md `1.1`
- **取舍**：不用纯语义分（关键词独占命中会全部垫底，语义 skipped 时直接退化）；不重新发明一套加权公式（用户已确认复用现有融合相关度，避免两套口径）。

### D5. 前端排序状态进查询指纹

- **结论**：`useSearch` 新增 `sortBy` ref（默认 `'date'`），加入 `keyOf` 指纹；`runSearch`/`loadMore` 都携带 `sortBy`；`setSortBy` 走与 `setIncludeArchived` 相同的 `requeryIfExecuted` 路径。
- **落点**：`front/app/features/search/composables/useSearch.ts:keyOf`
- **取舍**：不把排序只放在页面组件里（loadMore 会丢状态、切换后无法幂等判重）；不在响应里反向覆盖本地 `sortBy`（本地是用户意图，服务端只回显）。

### D6. UI 复用既有分段按钮形态

- **结论**：在搜索页过滤行内加「最新优先 / 相关度优先」双按钮分段控件，复用 `BoardTimelinePanel.vue` 的 `.tags-sort-toggle` 视觉与 `--color-accent*` token；loading 时 disabled；不新增基础组件。
- **落点**：`front/app/pages/search.vue`
- **取舍**：不抽 `AppSegmentedControl`（本 change 只有一处消费，抽取属额外公共 API 面）；不用 `<select>`（与同排 toggle 风格不一致，且移动端点击面积更小）。

### D7. 不做时间范围筛选

- **结论**：本 change 只做排序，不加今天/近 3 天/近 7 天过滤。
- **落点**：无代码落点：proposal.md「What Changes · 非目标」与 ui-design.md「入口与入口变更」，因为用户已明确选择只做排序、默认时间优先已足以让实时新闻进首屏。
- **取舍**：时间范围筛选会额外引入 API 参数、归档窗口口径、UI 状态与测试面，超出本次「看不到实时新闻」的最小修复范围。

## Risks / Trade-offs

- [默认顺序变化让老用户不适应] → 排序控件显式可见，且 `sort_by=relevance` 一键恢复旧顺序；proposal 标 BREAKING。
- [分页期间新文章入库导致跨页漂移] → 排序键全序确定（日 → RRF → 关键词 → 语义 → 精确时间 → ID），同一时刻请求稳定；数据变化导致的漂移与现有分页行为一致。
- [前端过滤器的排序状态与响应不一致] → 本地 `sortBy` 是唯一事实源，响应只回显不覆盖；非法值由后端回退 `date`，前端只会发送合法枚举。
- [扩展结果区隔与全局排序冲突] → expand=on 时扩展独占结果仍单独分区（既有 spec 要求）；时间排序在两个分区内部各自生效，默认 expand=off 不受影响。

## Migration Plan

无数据库迁移。部署即生效；回滚只需回退代码（API 参数缺省回到旧 RRF 顺序）。若需临时恢复旧默认，可让前端不传 `sort_by` 并让后端默认改回 `relevance`（本 change 不提供运行时开关）。

## Open Questions

无。
