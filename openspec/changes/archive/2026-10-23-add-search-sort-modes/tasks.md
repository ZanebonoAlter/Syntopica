# Tasks: add-search-sort-modes

## 1. 后端排序核心

- [x] 1.1 新增 `backend-go/internal/reader/search/sort.go`：`SortMode`（`date`/`relevance`）、`ParseSortMode`（非法回退 `date`）、`sortDayKey`（Asia/Shanghai UTC+8 日历日）、`effectiveTime`（`pub_date` 空回退 `created_at`）、`SortFused(fused, rows, mode)`（`relevance` 原样返回；`date` 复制后按 日倒序 → RRF 分 → 关键词分 → 语义分 → 精确时间倒序 → ID 升序排序）。验证：`cd backend-go && go test ./internal/reader/search -run TestSortFused -v` 通过
- [x] 1.2 新增 `sort_test.go` 白盒用例：日界（22 日 23:59 vs 23 日 00:01）、同日内高分在前、`pub_date=nil` 回退 `created_at`、`relevance` 模式不改变 RRF 顺序、同日同分按 ID 稳定。验证：`go test ./internal/reader/search -run TestSortFused -v` 全绿

## 2. API 接线与契约

- [x] 2.1 `handler.go` 增加 `parseSortByParam`（缺省/非法 → `date`），在行投影池建好后、offset 分页前调用 `SortFused`，响应 `data` 增加 `sortBy` 回显。验证：`go test ./internal/reader/search -run TestSearchHandler -v` 通过
- [x] 2.2 `handler_test.go` 扩展响应结构体 `searchResponse.Data.SortBy`，新增用例：默认 `date` 且最新日在前、`sort_by=relevance` 保持旧顺序、`sort_by=bogus` 200 回退 `date`、`per_page=1` 连续两页无重复/无遗漏。验证：`go test ./internal/reader/search -run 'TestSearchHandlerSort|TestSearchHandlerSortPagination' -v` 通过

## 3. 前端

- [x] 3.1 `front/app/api/search.ts` + `front/app/types/search.ts`：`SearchQueryParams.sortBy?: 'date' | 'relevance'` 映射到 `sort_by`；`normalizeSearchResponse` 解析 `sortBy`（缺省 `date`）。验证：`cd front && pnpm exec nuxi typecheck` 通过
- [x] 3.2 `front/app/features/search/composables/useSearch.ts`：新增 `sortBy` ref（默认 `date`）、`setSortBy`（经 `requeryIfExecuted`）、加入 `keyOf` 指纹、`runSearch`/`loadMore` 携带；`useSearch.test.ts` 补默认值/请求参数/切换重查/loadMore 延续/幂等指纹用例。验证：`pnpm test:unit app/features/search/composables/useSearch.test.ts --maxWorkers=2` 通过
- [x] 3.3 `front/app/pages/search.vue`：过滤行新增「最新优先 / 相关度优先」分段按钮（`data-test="search-sort-date"` / `"search-sort-relevance"`，active 用 semantic token），loading 时 disabled；`search.test.ts` 补默认高亮、点击切换请求 `sortBy:'relevance'`、loading disabled、empty/error 下控件仍在此四类断言。验证：`pnpm test:unit app/pages/search.test.ts --maxWorkers=2` 通过

## 4. 测试

- [x] 4.1 后端影响包：`cd backend-go && go test ./internal/reader/search`（含 sort/fusion/handler/repository 全部用例，确认既有 RRF 顺序用例不回归）
- [x] 4.2 前端影响文件：`cd front && pnpm test:unit app/features/search/composables/useSearch.test.ts app/pages/search.test.ts --maxWorkers=2`（参数不带 `--`）
- [x] 4.3 跨端排序效果核对：只读 `GET /api/search?q=英伟达&per_page=50`（默认）与 `&sort_by=relevance` 对比首屏发布日期分布，确认默认首屏含 10-22/10-23 文章且 `relevance` 仍为旧顺序；留痕本节。实测（2026-10-23 静态托管后）：默认 `sortBy=date` 首屏 50 条 = 10-23×19 + 10-22×31；`relevance` 首屏为 9 月旧文；`bogus` 回退 `date`。

## 5. 文档

<!-- doc-impact: api, flow -->

- [x] 5.1 `docs/reference/api/search.md`：参数表补 `sort_by`、JSON 示例补 `sortBy`、新增排序语义 bullet（date 日分组 / relevance RRF / 非法回退）。
- [x] 5.2 `docs/reference/flow/reading.md`：§站内搜索补排序说明；「业务约束与不变量」新增 #13（默认时间优先 + 日分组口径 + 两模式同结果集）；「变更溯源」表补 2026-10-23 行。
- [x] 5.3 更新/新建 `openspec/changes/add-search-sort-modes/ui-verification/report.md`：记录组件测试结论 + 1440×900/375×667 截图（editorial）+ dark 主题走查，未覆盖项写原因。

## 6. 验证

- [x] 6.1 `cd backend-go && golangci-lint run ./internal/reader/search/...`，期望 0 issues
- [x] 6.2 `cd backend-go && go vet ./internal/reader/search/...`，期望无输出
- [x] 6.3 `cd backend-go && go build ./...`，期望通过
- [x] 6.4 `cd front && pnpm lint`，期望通过（0 errors；8 个预存 warning 均在域外脏文件）
- [x] 6.5 `cd front && pnpm exec nuxi typecheck`，期望通过（本 change 文件 0 错误；全量仍剩 `front/app/features/tags/components/SignalReportView.test.ts` 7 个 TS 错误，属 redesign-enrichment-research-workbench 的未收口脏文件，已在 6.11 登记）
- [x] 6.6 `cd front && pnpm test:unit app/features/search/composables/useSearch.test.ts app/pages/search.test.ts --maxWorkers=2`，期望全绿（33/33）
- [x] 6.7 `cd front && pnpm build`，期望通过（deploy-frontend.sh 构建 + 铺盘 + 重启 + 健康检查成功）
- [x] 6.8 `bash scripts/harness/scenario-trace.sh openspec/changes/add-search-sort-modes`，期望退出码 0（15/15）
- [x] 6.9 `bash scripts/harness/check-tasks-tail.sh openspec/changes/add-search-sort-modes`，期望退出码 0
- [x] 6.10 `bash scripts/harness/archive-readiness.sh add-search-sort-modes`，期望全绿（2026-10-23 六项全绿后归档）
- [ ] 6.11 【域外阻塞登记】`front/app/features/tags/components/SignalReportView.test.ts` 的 7 个 TS 错误归属 redesign-enrichment-research-workbench（`git status` 为 M），本 change 不触碰；待该 change 收口后复跑全量 `nuxi typecheck`

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 关键词路径命中并返回解释 | backend-go/internal/reader/search/handler_test.go |
| 语义路径命中返回标签与分数 | backend-go/internal/reader/search/handler_test.go |
| 非法分页参数 | backend-go/internal/reader/search/handler_test.go |
| 未传排序参数默认时间优先 | backend-go/internal/reader/search/handler_test.go |
| 相关度排序模式 | backend-go/internal/reader/search/handler_test.go |
| 日界按 Asia/Shanghai 划分 | backend-go/internal/reader/search/sort_test.go |
| 同一天内按相关度排序 | backend-go/internal/reader/search/sort_test.go |
| 空发布时间回退创建时间 | backend-go/internal/reader/search/sort_test.go |
| 非法排序值回退默认 | backend-go/internal/reader/search/handler_test.go |
| 分页顺序稳定 | backend-go/internal/reader/search/handler_test.go backend-go/internal/reader/search/sort_test.go |
| 无结果引导扩展 | front/app/pages/search.test.ts |
| 语义降级提示 | front/app/pages/search.test.ts |
| 双主题渲染 | front/app/pages/search.test.ts |
| 默认最新优先 | front/app/pages/search.test.ts front/app/features/search/composables/useSearch.test.ts |
| 切换排序立即重查 | front/app/pages/search.test.ts front/app/features/search/composables/useSearch.test.ts |

## 7. Review

- [x] 7.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 `review.md` 建账
