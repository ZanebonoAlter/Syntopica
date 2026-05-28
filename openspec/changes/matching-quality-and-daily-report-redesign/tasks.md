## P1: 方向校验扩展

- [x] 1.1 `semantic_board_matching.go`: 将方向校验逻辑从 `case max_sim` 移到 switch 之后、`if matchReason != ""` 内，跳过 `direct_hit`
- [x] 1.2 `semantic_board_matching_test.go`: 补充 hit_rate + weighted 方向校验测试用例
- [x] 1.3 验证：`go test ./internal/domain/tagging -run TestEvaluateSemanticBoardMatches_DirectionCheck -v`

## P2: 文章排序优先级

- [x] 2.1 `semantic_board_handler.go`: `getBoardArticles` 新增 Go 端排序逻辑——遍历每篇文章的 `filtered_tags` 计算 best tier，然后 `sort.Slice` 按 `(tier ASC, score DESC, pub_date DESC)` 排序
- [x] 2.2 验证：`go build ./...`

## P3: 日报精简

### 3.1 collectBoardTags 携带匹配质量

- [x] 3.1.1 `generator.go`: `collectBoardTags` 查询新增 `tbl.match_reason`、`tbl.score` 字段（包括 fallback 路径）
- [x] 3.1.2 `daily_report/models.go`: `TagInput` 新增 `MatchReason string`、`Score float64` 字段
- [x] 3.1.3 验证：`go build ./...`

### 3.2 质量筛选层

- [x] 3.2.1 `generator.go`: `GenerateDailyReport` 在聚类前新增筛选步骤——过滤 direction_mismatch、过滤 weighted、<10 保底、>30 截断（对 fallback 标签同等规则）
- [x] 3.2.2 验证：`go test ./internal/domain/daily_report/...`

### 3.3 聚类数限制

- [x] 3.3.1 `cluster.go`: 修改 `clusterSystemPrompt` 构建逻辑，按标签数条件分支：≤15 不限、16-25 → "6-12 组"、>25 → "8-15 组"
- [x] 3.3.2 验证：`go build ./...`

### 3.4 去掉板块动态

- [x] 3.4.1 `generator.go`: 移除 `GenerateDynamics` 调用和并发逻辑（Call B），简化为只剩 Call A + Call C×K
- [x] 3.4.2 `generator.go`: `BoardDailyReport.Dynamics` 留空字符串，prompt version 升级 "2.0"
- [x] 3.4.3 验证：`go build ./...` + `go test ./internal/domain/daily_report/...`

### 3.5 聚类排序字段

- [x] 3.5.1 `daily_report/models.go`: `DailyReportSection` 新增 `BestTier int`、`AvgScore float64` 字段
- [x] 3.5.2 `generator.go`: 生成报告时计算每个 section 的 best_tier 和 avg_score 并写入
- [x] 3.5.3 验证：`go build ./...` + `go test ./internal/domain/daily_report/...`

## P4: 前端日报报纸布局

### 4.1 数据适配

- [x] 4.1.1 `BoardDailyReportTimeline.vue`: 处理 `dynamics` 为空时不渲染"板块动态"区块
- [x] 4.1.2 `BoardDailyReportTimeline.vue`: 聚类数据按 `best_tier + avg_score` 排序（使用后端返回的字段）
- [x] 4.1.3 `dailyReports.ts`: `DailyReportSection` TypeScript 类型新增 `best_tier` 和 `avg_score` 字段

### 4.2 长滚动布局 + 质量分区

- [x] 4.2.1 `BoardDailyReportTimeline.vue`: 纸张放大到 `min(1100px, 92vw)` × `92vh`
- [x] 4.2.2 `BoardDailyReportTimeline.vue`: 移除分页逻辑（pages/currentPage/翻页/页面切换动画），改为单页长滚动
- [x] 4.2.3 `BoardDailyReportTimeline.vue`: `qualityZones` computed——按 best_tier 分区：核心事件（Tier 0-1，双列）、相关事件（Tier 2，单列）、其他动态（Tier 3+，单列）
- [x] 4.2.4 `BoardDailyReportTimeline.vue`: 每个聚类卡片完整展示所有线索（title + summary + status），不截断
- [x] 4.2.5 `BoardDailyReportTimeline.vue`: 线索点击→`@floating-ui/vue` 文章浮窗，首批 5 篇并发加载，支持"加载更多"（分批 5 篇）
- [x] 4.2.6 `BoardDailyReportTimeline.vue`: emit `openArticle` → TagsPage 复用已有 `openArticlePreview`
- [x] 4.2.7 `TagsPage.vue`: 添加 `@open-article="openArticlePreview"` 事件监听
- [x] 4.2.8 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

## P5: 全量验证

- [x] 5.1 后端：`go build ./...` + `go vet ./...` + `go test ./internal/domain/tagging/... ./internal/domain/daily_report/...`
- [x] 5.2 前端：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`
- [ ] 5.3 端到端：调用 rematch-all → 生成日报 → 前端验证报纸布局 + 文章排序
