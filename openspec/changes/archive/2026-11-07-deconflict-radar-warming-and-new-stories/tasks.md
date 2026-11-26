# 任务 — deconflict-radar-warming-and-new-stories

## 1. 用例先行

- [x] 1.1 `backend-go/internal/topicgraph/repository/board_radar_stories_test.go`：新增 `TestRadarStoryFeedNewStoriesExcludeTopStories`——造 6 条无基线事件线（热度递减），断言 `top_stories` 5 条、`new_stories` 只含第 6 条、`new_count == 1`。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run 'RadarStoryFeedNewStoriesExclude' -count=1`
- [x] 1.2 同文件：`TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered`——造 4 条无基线事件线且 Top 上限覆盖全部，断言 `new_stories` 为空、`new_count == 0`、`new_total_count == 4`（全被覆盖态三联，review R-1 补）。验证同上，`-run 'RadarStoryFeedNewStoriesSummary'`
- [x] 1.3 同文件：`TestRadarStoryFeedCrossDayStoryCount`——造 2 条有基线 + 3 条无基线，断言 `cross_day_story_count == 2`；再造全无基线场景断言 `== 0`。验证：`-run 'RadarStoryFeedCrossDayStoryCount'`
- [x] 1.4 同文件：`TestRadarStoryFeedNewCountNotAffectedByTrim`——造 8 条无基线线（Top 5 覆盖 5 条），断言 `new_count == 3` 而 `len(new_stories) == 3`、`new_total_count == 8`（全量不受截断影响；`count > len` 的截断态由 1.8 三联钉住）。验证：`-run 'RadarStoryFeedNewCountNotAffectedByTrim'`
- [x] 1.8 同文件：`TestRadarStoryFeedNewTotalCount`（review R-1）——三个子测断言 `new_total_count` / `new_count` / `len(new_stories)` 三联组合：截断态（10/5/3）、全被覆盖态（3/0/0）、无新起态（0/0/0）。验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run 'RadarStoryFeedNewTotalCount' -count=1`
- [x] 1.5 `front/app/features/tags/composables/useBoardRadarOverview.test.ts`：断言 `crossDayStoryCount` 三态（`0` / `>0` / `null`）与 `newCount` 透出。验证：`cd front && pnpm test:unit app/features/tags/composables/useBoardRadarOverview.test.ts --maxWorkers=2`
- [x] 1.6 `front/app/features/tags/components/console/BoardRadarView.test.ts`：断言三种空态文案互不相同、摘要行渲染、新起档不渲染条目。验证：`pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2`
- [x] 1.7 `front/app/api/normalizers/boardRadar.test.ts`：断言 `cross_day_story_count` 缺失归一为 `null`、`0` 归一为 `0`。验证：`pnpm test:unit app/api/normalizers/boardRadar.test.ts --maxWorkers=2`

## 2. 后端分档

- [x] 2.1 `backend-go/internal/topicgraph/repository/board_radar_repository.go`：`RadarStoryFeed` 新增 `CrossDayStoryCount int \`json:"cross_day_story_count"\``，注释写明「既有锚点日之前 3 日基线窗口内证据、又有锚点日当天证据的事件线数（全部候选，不受截断影响）」。
- [x] 2.2 同文件：`finalizeRadarStories` 在排序前统计 `HasBaseline` 数量写入 `feed.CrossDayStoryCount`。
- [x] 2.3 同文件：填完 `feed.TopStories` 后建 `topIDs` 集合，`newStories` 遍历加 `if _, ok := topIDs[item.StoryID]; ok { continue }`。
- [x] 2.4 同文件：改写 `WarmingCount / NewCount are the FULL counts` 注释，说明 `NewCount` 是「去 Top 后的新起全量」。
- [x] 2.5 验证：`cd backend-go && go test ./internal/topicgraph/repository/ -run 'Radar' -count=1 && go test ./internal/topicgraph/handler/ -run 'Radar' -count=1`
- [x] 2.6 同文件（review R-1）：`RadarStoryFeed` 新增 `NewTotalCount int \`json:"new_total_count"\``（去 Top 之前、截断之前的当日新起全量），`finalizeRadarStories` 在 `!HasBaseline` 分支、排除 `topIDs` 之前自增。
- [x] 2.7 `board_radar_overview_handler_test.go`：payload 结构体补 `new_total_count` 并在空版块用例断言随行同响（0 = 当日没有新起）。

## 3. 前端归一与派生

- [x] 3.1 `front/app/api/normalizers/boardRadar.ts`：`stories` 段归一新增 `cross_day_story_count` → `crossDayStoryCount: number | null`（缺失/非数字 → `null`，`0` → `0`）。
- [x] 3.2 `front/app/features/tags/composables/useBoardRadarOverview.ts`：透出 `crossDayStoryCount`、`newCount`；派生 `warmingEmptyKind: 'no-cross-day' | 'not-triggered' | 'unknown'`。
- [x] 3.3 验证：`cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts --maxWorkers=2`
- [x] 3.4 同两文件（review R-1）：归一并透出 `new_total_count` → `newStoryTotalCount`（缺失 → null，与 0 区分）。

## 4. 前端呈现

- [x] 4.1 `front/app/features/tags/components/console/BoardRadarReadingRail.vue`：把「今日新起」从 `.rd-warm-block` 内部移出，成为同级独立分档（复用 `.rd-warm-block` / `.rd-warm-sub` / `.rd-warm-why` / `.rd-read-meta`，不新增色值/断点）。
- [x] 4.2 同文件：升温空态按 `warmingEmptyKind` 三态渲染——`no-cross-day` → 「当前没有跨天延续的事件线。」+「跨天延续能力尚在建设中。」；`not-triggered` / `unknown` → 「暂未触发升温条件。」。
- [x] 4.3 同文件：新起档三态渲染（review R-1）——`newCount > newStories.length` 时列条目 + 摘要行「另有 N 条今日新起事件线（未在今日该读中列出）。」（N = 差值）；全被该读覆盖（`newStoryTotalCount > 0` 而 `newCount == 0`）时「N 条今日新起事件线均已列入「今日该读」。」；`new_total_count == 0` 时「今天没有新起的事件线。」（不静默消失；旧后端缺字段不出摘要）。
- [x] 4.4 同文件：改写两档脚注——升温档只讲 2 倍条件；新起档讲「近 3 日无基线，且未在今日该读中列出」。
- [x] 4.5 验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2 && pnpm exec nuxi typecheck && pnpm lint`

## 5. 测试

- [x] 5.1 `cd backend-go && go test ./internal/topicgraph/repository/ -run 'Radar' -count=1 && go test ./internal/topicgraph/handler/ -run 'Radar' -count=1`
- [x] 5.2 `cd front && pnpm test:unit app/api/normalizers/boardRadar.test.ts app/features/tags/composables/useBoardRadarOverview.test.ts app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2`
- [x] 5.3 先红后绿证据：把实现回退成旧行为，确认 1.1～1.7 全红；恢复后全绿，证据贴进报告。

## 6. 文档

<!-- doc-impact: api -->

- [x] 6.1 `docs/reference/api/daily-reports.md`：`stories` 段字段表新增 `cross_day_story_count`；改写 `new_count` 字段说明（去 Top 后的新起全量）；补「三档互斥」与「升温空态两成因」的降级说明；同步 JSON 示例。
- [x] 6.2 验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/deconflict-radar-warming-and-new-stories`

## 6b. Review

- [x] 6b.1 派只读子线程做 review（审查制品 + diff），重点：三档互斥的边界（升温档允许与该读重叠是否合理）、`new_count` 口径与列表是否一致、空态三态是否可能误断言、旧后端降级是否安全；发现记入 `review.md` 并逐条处置。（2026-11-07 独立只读 review 已完成：建账 5 条 R-1～R-5 全部处置完毕，见 review.md；R-1 修复后的复测见 ui-verification/report.md「复测」节）
- [x] 6b.2 修复 review.md R-1～R-5：后端新增 `new_total_count`、前端新起档三态渲染（先红后绿：后端编译红→断言红→绿，前端 7 红→83 绿）、proposal/design/spec/test-cases/api 文档口径同步更正（R-2/R-3/R-5）、composable 注释更正（R-4）。

## 7. 验证


- [x] 7.1 `cd backend-go && go build ./... && go vet ./... && golangci-lint run ./internal/topicgraph/... && gofmt -l internal/topicgraph/`
- [x] 7.2 `cd front && pnpm exec nuxi typecheck && pnpm lint`
- [x] 7.3 真接口核对：`curl -s 'http://127.0.0.1:5100/api/semantic-boards/overview'` 断言 `top_stories` 与 `new_stories` 无交集、`new_count` 与 `new_stories` 口径一致、`cross_day_story_count` 与真库有基线事件线数相符。
- [x] 7.4 UI 验收（静态 :5100）：三档视觉分离、三种空态文案、摘要行、390×844 无溢出。证据写 `ui-verification/report.md`。
- [x] 7.5 `bash scripts/harness/archive-readiness.sh deconflict-radar-warming-and-new-stories`（已跑三轮：①～④⑥⑦绿；⑤待 6b.1 review 建账 → 第三轮 R-1 修复复测后**六项全绿**，见 ui-verification/report.md「归档门禁」节）

| Scenario | 测试文件 |
| --- | --- |
| 今日该读按事件线跨版块混排 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 排序只按热度与来源数 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 今日按各版块自己的锚点日而非自然今天 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 锚点日取该版块自己的最新日报日而非泳道轴末日 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 滞后版块显示自己的日期 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 升温判定 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 分区多但事件少不升温 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 无升温 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 无跨天延续事件线时的空态 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 旧响应缺跨天线计数时不冒充无跨天线 | front/app/api/normalizers/boardRadar.test.ts |
| 今日新起单列且不显示倍率 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 今日新起与该读互斥 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 新起全被该读覆盖时说明已列入该读 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go front/app/features/tags/components/console/BoardRadarView.test.ts |
| 新起超出列出上限时附剩余计数摘要 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go front/app/features/tags/components/console/BoardRadarView.test.ts |
| 该读不按来源数硬过滤 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 未回填版块不冒充无事件 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 部分版块延迟时排序不完整明示 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 单版块事件线计算失败同样明示排序不完整 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
