# Review: add-search-sort-modes

审查方式：`change-review-glm` 子线程只读审查（制品 + scoped diff + 测试复跑），2026-10-23。
总评：排序核心、handler 接线、前端状态机与 spec 逐条对齐，无 High/Medium 问题，可归档。

### R-1 2026-10-23【Low】ui-design.md 禁用口径与实现不符（追加加载期间按钮并未 disabled）
- **状态**：已修复
- **证据**：`ui-design.md` 原文「`loading`（首查/追加）期间按钮 disabled」，实现 `search.vue` 的 `:disabled="loading"` 只覆盖首查（`loadingMore` 不禁用，点击会重查并经 seq 作废追加）。
- **处置**：改 ui-design 为「首查 loading 期间 disabled；追加加载期间可点击，点击会以新排序重查并作废进行中的追加（seq 丢弃旧响应）」，与实现/spec 对齐。

### R-2 2026-10-23【Low】handler 分页测试注释失实：「同分按 ID 兜底」实际由 RRF 名次差决定
- **状态**：已修复
- **证据**：`handler_test.go` 注释写「今天两篇同分（ID 升序兜底）」，但两篇关键词分相同时榜内名次不同（1/61 vs 1/62），handler 层实际由 RRF 分差定序；ID 兜底由 `sort_test.go` 纯函数层覆盖。
- **处置**：注释改为「今天两篇关键词分相同 → 榜内名次不同 → RRF 分差定序」。

### R-3 2026-10-23【Low】api 文档响应示例 snake/camel 混排（既有问题，本次新增行本身正确）
- **状态**：已修复
- **证据**：`docs/reference/api/search.md` 示例中 `per_page`/`took_ms` 与 wire 的 camelCase（`perPage`/`tookMs`/`sortBy`）混排。
- **处置**：顺手统一为 `perPage`/`tookMs`，与 `handler.go` 实际响应一致。

### R-4 2026-10-23【Low】requeryIfExecuted 在首查 loading 期间静默丢弃排序切换（可接受，与既有过滤器一致）
- **状态**：已修复（补注释，不改行为）
- **证据**：`useSearch.ts` 的 `requeryIfExecuted` 在 `loading` 时 return；`setSortBy` 先改本地态再调用，会出现「按钮态已变、结果仍旧排序」，直到再次 submit。
- **处置**：UI 层已用 `:disabled="loading"` 挡住该路径（组件测试锁定），与 `setIncludeArchived`/`setExpandEnabled` 既有语义一致；补注释说明「被跳过的变更由下次 submit 的指纹差异兜底」。

审毕无 High/Medium 发现（审查范围：sort.go/sort_test.go、handler.go/handler_test.go、api/search 前端归一层与 useSearch、search.vue 及测试、spec/design/tasks/test-cases/ui-design/api 与 flow 文档）。
