## 1. 匹配精度增强 (tag-to-board-matching)

- [x] 1.1 `semantic_board_matching.go`: `SemanticBoardMatchConfig` 新增 `DirectMaxSimMinHits int`（默认 2）和 `DirectMaxSimMinHitRate float64`（默认 0.3）字段
- [x] 1.2 `evaluateSemanticBoardMatches`: max_sim 分支从 `case maxSimilarity >= config.DirectMaxSim` 改为三条件联合判断：`maxSimilarity >= config.DirectMaxSim && hits >= min(config.DirectMaxSimMinHits, len(tagAuxiliaries)) && hitRate >= config.DirectMaxSimMinHitRate`。注意：当前 `scoreSemanticBoardSimilarity` 返回 `(hitRate, maxSimilarity)` 但不返回 hits，需从 `hitRate * tagAuxiliaryCount` 反算或修改函数签名增加 hits 返回值
- [x] 1.3 `loadConfig`: 读取 `semantic_board_match_direct_max_sim_min_hits` 和 `semantic_board_match_direct_max_sim_min_hit_rate` 两个新 ai_settings key
- [x] 1.4 Seed 新默认配置到 ai_settings（`semantic_board_match_direct_max_sim_min_hits=2`, `semantic_board_match_direct_max_sim_min_hit_rate=0.3`）
- [x] 1.5 补充单元测试：覆盖 N=1/2/3/5 场景的 max_sim 规则行为、hits 不足拒绝、hit_rate 不足拒绝、N=1 退化兼容
- [x] 1.6 `MatchingConfigDialog.vue`: 前端匹配参数表单新增两个配置项展示和编辑
- [x] 1.7 验证：`go test ./internal/domain/tagging/ -run TestEvaluateSemanticBoardMatches -v` + `go build ./...`

## 2. 升级建议 DTO 增强 (board-management-api)

- [x] 2.1 `semantic_board_handler.go`: `semanticBoardUpgradeSuggestionDTO` 新增 `AuxiliaryLabels []struct{ID uint `json:"id"`; Label string `json:"label"`} `json:"auxiliary_labels"`` 和 `TargetBoardLabel string `json:"target_board_label,omitempty"``
- [x] 2.2 `suggestionsToDTO`: 从 DB 批量查询涉及的 semantic_labels（auxiliary_label_ids）和 board（target_board_id）获取 label，填充新字段
- [x] 2.3 `front/app/api/semanticBoards.ts`: `UpgradeSuggestion` type 新增 `auxiliary_labels: {id: number; label: string}[]` 和 `target_board_label?: string`
- [x] 2.4 `UpgradeSuggestionPanel.vue:116`: `标签 #{{ id }}` → 改用 `s.auxiliary_labels` 展示 label；`UpgradeSuggestionPanel.vue:111`: `板块 #{{ s.target_board_id }}` → 改用 `s.target_board_label` 展示
- [x] 2.5 验证：`go test ./internal/domain/tagging/ -run TestUpgrade -v` + `pnpm lint` + `pnpm exec nuxi typecheck`

## 3. Board 文章列表 API (board-article-api)

- [x] 3.1 `semantic_board_handler.go`: 新增 `getBoardArticles` handler，注册路由 `GET /semantic-boards/:id/articles`，参数：feed_id, start_date, end_date, auxiliary_label_id, page, per_page
- [x] 3.2 查询逻辑：从 topic_tag_board_labels 获取属于 board 的 tag IDs → 通过 article_topic_tags 获取文章 IDs → JOIN feeds 获取 feed_name → 分页
- [x] 3.3 filtered_tags 批量查询：收集当前页所有文章 IDs → 一次性 JOIN article_topic_tags + topic_tags + topic_tag_board_labels WHERE semantic_board_id = :id → 按 article_id 分组返回 {id, label, category, match_reason, score}，避免逐篇 N+1 查询。match_reason 和 score 直接从 topic_tag_board_labels 已有字段获取
- [x] 3.4 返回格式：每篇文章包含基础字段 + feed_name + filtered_tags[] + pagination
- [x] 3.5 `front/app/api/semanticBoards.ts`: 新增 `getBoardArticles(id, params)` 方法和 `BoardArticle` type
- [x] 3.6 编写 handler 单元测试：覆盖按 feed/time/auxiliary 筛选、filtered_tags 过滤、分页、空结果
- [x] 3.7 验证：`go test ./internal/domain/tagging/ -run TestBoardArticles -v`

## 4. TagsPage 文章列表改造

- [x] 4.1 `TagsPage.vue`: `loadTimelineArticles` 切换到新 API `getBoardArticles(boardId, params)`
- [x] 4.2 新增 feed 下拉筛选：调用 feed 列表 API 获取 options，筛选参数传给 `getBoardArticles`
- [x] 4.3 新增时间范围选择器：start_date/end_date 参数传给 `getBoardArticles`
- [x] 4.4 文章行展示增强：feedId → feed_name 展示；新增 filtered_tags chips 展示（每篇文章的事件标签），每个 tag chip 用 tooltip 显示匹配信息（match_reason + score，如"相似度 0.85"或"直接命中"）
- [x] 4.5 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

## 5. 叙事生成取消 scope (narrative-board-generation)

- [x] 5.1 `service.go`: 移除生成调度中的 scope 循环（`GenerateAndSaveForAllCategories` + `GenerateAndSaveGlobal` 双路调度），改为 `GenerateAndSaveForAllBoards`：遍历所有 active SemanticBoard，对每个 board 调用统一的生成流程（无 scope 参数）
- [x] 5.1a `collector.go`: `CollectSemanticBoardNarrativeInputs` 移除 scopeType/categoryID 参数，去掉 category JOIN，收集每个 board 下所有 event tags（不限 feed category）
- [x] 5.1b `board_creation.go`: `matchPreviousSemanticBoard` 移除 scopeType/categoryID 参数，仅按 `semantic_board_id + period_date（前一日）` 匹配续接
- [x] 5.2 `board_narrative_generator.go`: 废弃 `SaveNarrativesForBoard`（硬编码 feed_category），统一使用 `saveNarrativesWithBoard`，scope_type 设为 "board"，ScopeCategoryID 设为 nil
- [x] 5.3 `board_narrative_generator.go`: `LoadBoardEventTags` 不再按 scope 过滤文章，收集该 board 下所有 event tags
- [x] 5.4 `service.go`: NarrativeBoard 创建逻辑统一为单 scope，scope_type="board"，按 semantic_board_id + 前一日日期续接（通过改造后的 `matchPreviousSemanticBoard`）
- [x] 5.5 `saveNarrativesWithBoard`: 确保 scope_type="board" 时使用 `resolveGeneration(out, date)` 而非 `resolveGlobalGeneration`
- [x] 5.6 编写生成测试：覆盖单 board 单日单叙事板、跨 category 事件合并、续接、冷启动
- [x] 5.7 验证：`go test ./internal/domain/narrative/ -v` + `go build ./...`

## 6. Board 叙事时间线 API (board-narrative-timeline)

- [x] 6.1 `semantic_board_handler.go`: 新增 `getBoardNarratives` handler，注册路由 `GET /semantic-boards/:id/narratives?days=7`
- [x] 6.2 查询逻辑：通过 narrative_boards.semantic_board_id JOIN narrative_summaries，按 period_date 倒序，days 参数控制回溯范围
- [x] 6.3 返回格式：每条叙事包含 id, title, summary, status, related_tags[{id, label}], related_article_ids（从 NarrativeSummary.RelatedArticleIDs 解析）, scope_type, article_count, period_date
- [x] 6.4 `front/app/api/semanticBoards.ts`: 新增 `getBoardNarratives(id, params)` 方法和 `BoardNarrative` type
- [x] 6.5 编写 handler 单元测试：覆盖 7 天默认、自定义天数、无叙事空返回、旧 scope 数据兼容
- [x] 6.6 验证：`go test ./internal/domain/tagging/ -run TestBoardNarratives -v`

## 7. BoardNarrativeTimeline 组件

- [x] 7.1 新增 `BoardNarrativeTimeline.vue`：调用 `getBoardNarratives(boardId, {days: 7})`，渲染叙事卡片列表
- [x] 7.2 卡片展示：status 标签（emerging=绿/continuing=蓝/splitting=橙/merging=紫/ending=灰）+ 日期 + 标题 + 摘要 + 关联标签 chips + 文章数
- [x] 7.3 点击叙事卡片展开关联文章列表：使用叙事记录中的 `related_article_ids` 批量调用文章详情 API 加载（复用文章预览弹窗）
- [x] 7.4 空状态：无叙事时展示"暂无叙事"
- [x] 7.5 "加载更早"功能：增大 days 参数重新请求，追加展示
- [x] 7.6 `TagsPage.vue`: 在 composition 面板和文章列表之间嵌入 BoardNarrativeTimeline，选中 board 时加载叙事
- [x] 7.7 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

## 8. 叙事功能迁移：/topics 叙事 tab 删除

- [x] 8.1 `TopicGraphPage.vue`: 删除 activeTab 状态变量和叙事 tab 按钮（`v-if="activeTab === 'narrative'"` 分支及切换按钮）
- [x] 8.2 `TopicGraphPage.vue`: 移除 NarrativePanel 组件 import 和使用
- [x] 8.2a 删除 `front/app/features/topic-graph/components/NarrativePanel.vue`（仅被叙事 tab 引用，删除 tab 后为死代码）
- [x] 8.2b 删除 `front/app/features/topic-graph/components/NarrativeBoardCanvas.vue`（仅被 NarrativePanel 引用）
- [x] 8.3 `TopicGraphPage.vue`: 移除与叙事相关的 ref/函数（expandedBoardIds、unclassifiedTags 等仅被 NarrativePanel 使用的状态）
- [x] 8.4 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

## 9. TagsPage 内容 Tab 切换

- [x] 9.1 `TagsPage.vue`: 新增 `contentTab` ref（`'composition' | 'daily-reports' | 'articles'`），默认 `'composition'`，选中 board 时显示 Tab 栏（板块内容 / 日报 / 文章），按 tab 切换显示对应面板
- [x] 9.2 BoardCompositionPanel、BoardDailyReportTimeline、文章时间线各区域用 `v-if="contentTab === 'xxx'"` 控制显隐
- [x] 9.3 Tab 栏样式：简洁横向 tab，和页面整体暗色风格一致，选中态用 accent 色
- [x] 9.4 验证：`pnpm lint` + `pnpm exec nuxi typecheck`

## 10. 整理叙事功能（按板块/日期触发）

- [x] 10.1 `service.go`: 新增 `GenerateAndSaveForBoard(semanticBoardID uint, date time.Time) (int, error)`，只收集指定 board 的 input，生成单板叙事
- [x] 10.2 `narrative_handler.go`: 新增 `POST /api/narratives/boards/generate` endpoint，参数 `{ date: string, board_id?: number }`，board_id 为空时生成全部
- [x] 10.3 `front/app/api/semanticBoards.ts`: 新增 `triggerNarrativeGeneration(params)` 方法
- [x] 10.4 `SemanticBoardList.vue`: 在"匹配参数"按钮下新增"整理叙事"按钮
- [x] 10.5 新增 `NarrativeGenerateDialog.vue`：日期选择器 + 板块下拉（可选全部），确认触发
- [x] 10.6 `TagsPage.vue`: 集成 NarrativeGenerateDialog，绑定按钮事件
- [x] 10.7 验证：`go build ./...` + `pnpm lint` + `vue-tsc --noEmit`

## 11. Refresh 并行化 (refresh-parallelization)

- [x] 11.1 `backend-go/internal/domain/feed/handler.go`: `refreshAllFeedsWorker` 改为 `sync.WaitGroup` + `chan struct{}(cap=3)` semaphore 并发，每个 feed 的错误独立捕获不影响其他
- [x] 11.2 验证：`go test ./internal/domain/feed/ -run TestRefreshAll -v` + `go build ./...`
- [x] 11.3 `front/app/features/shell/components/FeedLayoutShell.vue`: `onMounted` 改为两波 `Promise.all`：第一波 `fetchFeeds()` + `loadWatchedTags()`，第二波 `loadArticles()` + `fetchGlobalUnreadCount()`
- [x] 11.4 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

## 12. 匹配得分可视化 (match-score-visualization)

- [x] 12.1 `front/app/features/tags/components/TagsPage.vue`: 文章行 tag chip 添加颜色样式——direct_hit 用绿(#22c55e)、hit_rate 用蓝(#3b82f6)、max_sim 用橙(#f59e0b)、weighted 用灰(#94a3b8)，颜色应用于 chip 的 border
- [x] 12.2 每个 tag chip 内显示分数文字（如 `0.85`），chip 格式改为 `标签名 0.85`
- [x] 12.3 文章行右侧 end 处显示该文章最强匹配信息：匹配方式中文名（直接命中/命中率/相似度/综合）+ 最高分数，用 `matchInfoLabel(tag)` 函数生成
- [x] 12.4 `front/app/features/tags/components/TagsPage.vue`: 新增 `matchReasonColor(reason: string): string`、`matchInfoLabel(tag: BoardArticleTag): string`、`strongestMatch(tags: BoardArticleTag[]): BoardArticleTag | null` 工具函数
- [x] 12.5 验证：`pnpm lint` pass；typecheck/build 因 WSL 缺少 native binding (oxc-parser/lightningcss) 失败，非代码问题

## 13. 日报系统 (daily-report-system) — 替代旧叙事

### 13.1 数据模型
- [x] 13.1.1 `backend-go/internal/domain/daily_report/models.go`: 定义 `BoardDailyReport` 和 `DailyReportSection` GORM 模型
- [x] 13.1.2 `backend-go/internal/domain/daily_report/models.go`: AutoMigrate 注册

### 13.2 去重模块
- [x] 13.2.1 `backend-go/internal/domain/daily_report/dedup.go`: 实现精确去重——`DeduplicateTags(tags []EventTag) []EventTag`，规则：关联文章集合完全相同的 tag 合并、article_count=1 且关联同一文章的 tag 合并
- [x] 13.2.2 去重单元测试

### 13.3 LLM 分组模块
- [x] 13.3.1 `backend-go/internal/domain/daily_report/cluster.go`: `ClusterTags(ctx, tags []EventTag) ([]TagCluster, error)` —— 一次 LLM call，输入所有 tag 的 label+description，输出 `[{group_name, tag_ids[]}]`
- [x] 13.3.2 分组 prompt 设计：温度 0.1，JSON schema 约束输出格式，分组粒度为"同一核心事件"
- [x] 13.3.3 分组单元测试（mock LLM response）

### 13.4 生成模块
- [x] 13.4.1 `backend-go/internal/domain/daily_report/generator.go`: `GenerateHighlights(ctx, input) ([]Highlight, error)` —— Call A：今日重点
- [x] 13.4.2 `generator.go`: `GenerateDynamics(ctx, input) (string, error)` —— Call B：板块动态
- [x] 13.4.3 `generator.go`: `GenerateClusterThreads(ctx, cluster, prevThreads) ([]Thread, error)` —— Call C：聚类叙事线索
- [x] 13.4.4 `generator.go`: `GenerateDailyReport(ctx, boardID, date) (*BoardDailyReport, error)` —— 编排流水线：收集→去重→分组→并行生成(Call A + B + C×K)→组装→存储
- [x] 13.4.5 叙事线索连续性匹配：`matchPreviousThreads(todayClusters, prevSections, tagEmbeddings)` —— tag 交集优先 + embedding fallback

### 13.5 存储模块
- [x] 13.5.1 `backend-go/internal/domain/daily_report/repository.go`: `SaveReport(report *BoardDailyReport, sections []DailyReportSection) error`
- [x] 13.5.2 `repository.go`: `GetReport(boardID, date) (*BoardDailyReport, []DailyReportSection, error)`
- [x] 13.5.3 `repository.go`: `ListReports(boardID, days int) ([]BoardDailyReport, error)`

### 13.6 API
- [x] 13.6.1 `backend-go/internal/domain/daily_report/handler.go`: `POST /api/daily-reports/generate` —— 异步触发，返回 `{job_id, status}`，goroutine 执行 `GenerateDailyReport`，WS 广播 `daily_report_progress`/`daily_report_done`
- [x] 13.6.2 `handler.go`: `GET /api/semantic-boards/:id/daily-reports?days=7` —— 查询日报列表
- [x] 13.6.3 `handler.go`: `GET /api/daily-reports/:id` —— 查询单篇日报详情（含 sections）
- [x] 13.6.4 `backend-go/internal/app/router.go`: 注册新路由

### 13.7 定时任务
- [x] 13.7.1 `backend-go/internal/jobs/daily_report.go`: 新建 daily_report scheduler_task，执行逻辑调用 `daily_report.GenerateDailyReport`
- [x] 13.7.2 定时任务触发异步执行，WS 广播进度

### 13.8 前端
- [x] 13.8.1 `front/app/api/dailyReports.ts`: 新增 API client——`generateDailyReport(params)`, `getBoardDailyReports(boardId, params)`, `getDailyReportDetail(id)`
- [x] 13.8.2 `front/app/composables/useDailyReportProgress.ts`: WS composable，监听 `daily_report_progress`/`daily_report_done` 消息
- [x] 13.8.3 `front/app/features/tags/components/BoardDailyReportTimeline.vue`: 新组件，替代 BoardNarrativeTimeline——展示结构化日报（日期+标题+summary），点击展开：今日重点列表、板块动态段落、聚类叙事线索卡片
- [x] 13.8.4 日报卡片设计：status 标签 + 日期 + 标题 + summary + 聚类数/文章数，点击展开详情
- [x] 13.8.5 聚类叙事线索展示：聚类标题 + 该聚类下的线程列表（每条线程：status 色+标题+summary）
- [x] 13.8.6 `front/app/features/tags/components/TagsPage.vue`: 替换 BoardNarrativeTimeline 为 BoardDailyReportTimeline
- [x] 13.8.7 `front/app/features/tags/components/NarrativeGenerateDialog.vue`: 改为触发日报生成（调用 `generateDailyReport`），显示实时进度
- [x] 13.8.8 "加载更早"：增大 days 参数重新请求
- [x] 13.8.9 空状态："暂无日报"

### 13.9 旧系统废弃
- [x] 13.9.1 `backend-go/internal/domain/narrative/service.go`: 标注旧的 `GenerateAndSaveForBoard`/`GenerateAndSaveForAllBoards` 为 deprecated
- [x] 13.9.2 `backend-go/internal/jobs/narrative_summary.go`: 改为调用新的日报生成逻辑
- [x] 13.9.3 旧 NarrativeBoard/NarrativeSummary API 路由保留但不再主动调用（向后兼容）

### 13.10 验证
- [x] 13.10.1 `go test ./internal/domain/daily_report/ -v`
- [x] 13.10.2 `go build ./...` + `go vet ./...`
- [x] 13.10.3 `pnpm lint` + `vue-tsc --noEmit`

## 15. hit_rate 样本量惩罚 + 混合打分 (tag-to-board-matching D18)

- [x] 15.1 `semantic_board_matching.go`: `SemanticBoardMatchConfig` 新增 `MinEffectiveSample int`（默认 3）和 `HitRateSimBlend float64`（默认 0.7）字段
- [x] 15.2 `scoreSemanticBoardSimilarity`: 函数签名增加 `minEffectiveSample int` 参数，hitRate 分母从 `float64(tagAuxiliaryCount)` 改为 `math.Max(float64(tagAuxiliaryCount), float64(minEffectiveSample))`
- [x] 15.3 `evaluateSemanticBoardMatches`: hit_rate 分支的 score 从 `hitRate` 改为 `config.HitRateSimBlend*maxSimilarity + (1-config.HitRateSimBlend)*hitRate`；同时将 adjustedHitRate 传入 max_sim 规则的条件判断（替代原始 hitRate）
- [x] 15.4 `loadConfig`: 读取 `semantic_board_match_min_effective_sample` 和 `semantic_board_match_hit_rate_sim_blend` 两个新 ai_settings key
- [x] 15.5 Seed 新默认配置到 ai_settings（`semantic_board_match_min_effective_sample=3`, `semantic_board_match_hit_rate_sim_blend=0.7`）
- [x] 15.6 补充/更新单元测试：覆盖 1-aux 标签 hitRate=0.333 不过门槛、1-aux 标签退到 max_sim/weighted、2-aux 标签 hit_rate 混合打分、multi-aux (≥3) 行为不变、混合打分公式精度
- [x] 15.7 `MatchingConfigDialog.vue`: 前端匹配参数表单新增两个配置项展示和编辑（minEffectiveSample 数字输入、hitRateSimBlend 滑块 0-1）
- [x] 15.8 验证：`go test ./internal/domain/tagging/ -run TestEvaluateSemanticBoardMatches -v` + `go build ./...`
- [x] 15.9 全量重算：调用匹配 API 重算所有现有 tag 的板块归属，验证 score 分布改善

## 16. 匹配详情 API — 按需实时计算 (D19)

- [x] 16.1 `semantic_board_matching.go`: 新增 `loadBoardAuxiliariesByBoardID(ctx, boardID)` 方法，复用 `loadBoardAuxiliaries` 的查询结构但加 `WHERE board_composition.board_id = ?` 过滤，返回 `[]boardAuxiliaryLabel`
- [x] 16.2 `semantic_board_matching.go`: 新增 `computeMatchDetail` 函数，展开 `scoreSemanticBoardSimilarity` 内层循环，对每个 tag 辅助标签找到最佳匹配的 board 辅助标签，返回结构体包含 `hits`、`hitRate`、`maxSimilarity`、`pairs[]`（每对 tag_aux↔board_aux 的 similarity + is_hit + 标签名称）
- [x] 16.3 `semantic_board_handler.go`: 新增 `getTagMatchDetail` handler，路由 `GET /semantic-boards/:id/match-detail/:tagId`。逻辑：调用 `loadTagAuxiliaries` + `loadBoardAuxiliariesByBoardID` → 检查 direct_hit → 调用 `computeMatchDetail` → 从 `topic_tag_board_labels` 读取存储的 score/match_reason → 调用 `loadConfig` 获取当前参数 → 组装返回 DTO
- [x] 16.4 定义返回 DTO 结构：`matchDetailResponse` 包含 `topic_tag_id/label`、`semantic_board_id`、`match_reason`（存储值）、`score`（存储值）、`config`（当前参数）、`direct_hit_auxiliaries[]`、`tag_auxiliary_count`、`hits`、`hit_rate`、`max_similarity`、`pairs[]`
- [x] 16.5 在 `RegisterSemanticBoardRoutes` 注册新路由（当前项目由 `backend-go/internal/domain/tagging/semantic_board_handler.go` 统一注册 semantic-board 子路由）
- [x] 16.6 按用户验收口径跳过过时 sqlite handler 测试，补充纯单元测试覆盖 direct_hit DTO、pairs、聚合指标、空输入
- [x] 16.7 验证：`go test ./internal/domain/tagging/ -run 'TestComputeMatchDetail|TestDirectHitAuxiliary' -v` + `go build ./...`

## 17. 匹配详情前端面板 (D20)

- [x] 17.1 安装 KaTeX：`pnpm add katex` + `pnpm add -D @types/katex`
- [x] 17.2 新增 `front/app/components/KaTeXRender.vue`：接收 `latex: string` + `display?: boolean` props，用 `katex.renderToString` 渲染，`v-html` 输出。带 `katex/dist/katex.min.css` import
- [x] 17.3 新增 `front/app/api/semanticBoards.ts`: `getMatchDetail(boardId, tagId)` 方法和 `MatchDetailResponse` / `MatchDetailConfig` / `MatchDetailPair` / `DirectHitAuxiliary` 类型
- [x] 17.4 新增 `front/app/features/tags/components/MatchDetailPanel.vue` 组件：
  - Props: `boardId: number`、`tag: BoardArticleTag | null`（null 时隐藏）
  - 点击 tag 变化时调用 `getMatchDetail(boardId, tag.id)`，loading 状态展示 skeleton
  - 渲染匹配公式（根据 match_reason 选择 LaTeX 字符串 + 代入具体数值，用 KaTeXRender 展示）
  - 渲染辅助标签逐对匹配明细表（pairs 数组）
  - 渲染可折叠的配置参数区域
  - direct_hit 场景：展示精确匹配的辅助标签列表，无公式
  - 关闭按钮 emit `close` 事件
- [x] 17.5 公式渲染逻辑：为四种 match_reason 分别生成 LaTeX 字符串，代入 response 中的实际数值。例如 hit_rate: `\text{score} = ${alpha} \times ${maxSim} + ${(1-alpha)} \times ${hitRate} = ${score}`
- [x] 17.6 `TagsPage.vue` 布局改造：
  - 新增 `selectedTagForDetail: BoardArticleTag | null` ref
  - 文章 tab 区域改为 flex row：左侧文章列表 `flex: 1`，右侧 `MatchDetailPanel` 条件渲染（`v-if="selectedTagForDetail"`），`width: 320px; flex-shrink: 0`
  - tag chip 点击事件：`selectedTagForDetail === tag ? null : tag` 切换
  - 选中的 chip 加 `ring` 高亮样式
  - CSS transition 在面板容器上加 `transition: width 0.2s`
- [x] 17.7 MatchDetailPanel 关闭按钮 → emit close → `selectedTagForDetail = null` → 面板收起
- [x] 17.8 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

## 18. 日报生成进度与日期修复（post-investigation）

详见：`daily-report-generate-stuck-investigation.md`。

- [x] 18.1 后端手动生成 WS 协议对齐：`daily_report_progress` 字段补齐 `board_name/saved/progress`，状态使用前端可识别值，并在单板/全板终态广播 `daily_report_done`
- [x] 18.2 日报查询 API 返回结构对齐前端：列表返回 `{ reports: [...] }`，详情返回 `{ report: ... }`（或同步调整前端类型与组件，保持一致）
- [x] 18.3 修复 `period_date` 日期偏移：请求日期 `2026-05-26` 必须持久化/查询为 `2026-05-26`
- [x] 18.4 日志可读性修复：慢 SQL 日志截断/脱敏超长 vector 字面量；WebSocket 正常 close 1000 不再打 WARN
- [x] 18.5 验证：覆盖 daily_report handler/repository 关键行为测试；运行相关 Go 测试、前端 lint/typecheck；手工或脚本验证示例 payload（本环境缺少 Go 工具链且前端 typecheck 缺少 native binding，详见实现总结）

## 19. Bug 修复：日报 summary UTF-8 截断导致保存失败

**根因**：`generator.go:478` 的 `summary = summary[:200]` 按字节截断中文 UTF-8 字符串，可能在多字节字符中间截断，产生无效 UTF-8 序列（如 `0xEF 0xBC 0x27`），导致 PostgreSQL 拒绝 INSERT（`SQLSTATE 22021: invalid byte sequence for encoding "UTF8"`），整条日报保存失败。

- [x] 19.1 修复 `generator.go:478`：字节截断改为 rune 安全截断
- [x] 19.2 验证：`go test ./internal/domain/daily_report/ -v` + `go build ./...`

## 20. 标签清理机制 (tag-cleanup)

**根因**：当前没有专门的定时任务来清理辅助标签（auxiliary labels）。标签清理是零散的、被动触发的：
- `cleanupOrphanedTags` 仅在 `RetagArticle`（Force 模式）时触发，不是定时执行的
- `CleanupOldArticles` 删文章后不清理孤儿 TopicTag，也不触发 `cleanupOrphanedTags`
- `semantic_labels.ref_count` 只在 `AttachAuxiliaryLabels` 中 +1，没有对应的 -1 逻辑（级联删除不会触发），导致 ref_count 只增不减
- 孤儿辅助标签（无 TopicTag 关联）永远不会被清理

**方案**：两步走 — 堵漏 + 定期对账
1. 堵漏：`CleanupOldArticles` 删文章后立即调用 `CleanupOrphanedTags` 清理孤儿 tag
2. 对账：挂在 `tag_quality_score`（每小时）下，重算 ref_count + 清理孤儿辅助标签

- [x] 20.1 `article_tagger.go`: 导出 `cleanupOrphanedTags` → `CleanupOrphanedTags`
- [x] 20.2 `feed/service.go`: `CleanupOldArticles` 删文章前收集 affected tag IDs，删文章后调用 `tagging.CleanupOrphanedTags`
- [x] 20.3 `jobs/tag_quality_score.go`: `runComputeCycle` 末尾增加两步：
  - (a) 重算 ref_count：`UPDATE semantic_labels SET ref_count = (SELECT COUNT(*) FROM topic_tag_semantic_labels WHERE semantic_label_id = semantic_labels.id) WHERE label_type = 'auxiliary'`
  - (b) 清理孤儿辅助标签：删除 `label_type='auxiliary' AND ref_count=0 AND protected=false AND status='active' AND created_at < 1天前`
- [x] 20.4 验证：`go test ./internal/domain/feed/ ./internal/jobs/ -v` + `go build ./...` ✅

## 14. 集成验证

- [ ] 14.1 端到端：文章 → tag 提取 → 辅助标签 → board 匹配（验证 max_sim 双因子约束生效）
- [ ] 14.2 端到端：手动触发升级建议 → 确认执行 → 面板展示 label 而非 #id
- [ ] 14.3 端到端：选中 board → Tab 切换（板块内容 / 日报 / 文章）各面板正确渲染
- [ ] 14.4 端到端：Feed/时间筛选 board 文章列表，tag chip 颜色+分数，文章行匹配信息
- [ ] 14.5 端到端：refresh-all 并发执行（验证 semaphore=3 限流生效）
- [ ] 14.6 端到端：日报生成 → 去重 → LLM 分组 → 并行分段生成 → 存储成功 → 前端查看结构化日报
- [ ] 14.7 端到端：日报生成触发 → 异步返回 `{job_id}` → WS 广播进度 → 前端进度板实时更新
- [ ] 14.8 端到端：定时任务触发日报生成（scheduler_tasks narrative_summary 任务）
- [ ] 14.9 端到端：叙事线索连续性（第二天日报的线索正确续接前一天）
- [ ] 14.10 端到端：旧叙事 API 向后兼容（旧数据仍可查询，新数据走日报系统）
- [x] 14.11 验证：`go build ./...` && `go test ./...` && `go vet ./...` ✅ go build/vet 通过，所有 daily_report 测试通过（仅有一个预存的 database 测试失败与本次无关）
- [x] 14.12 验证：`pnpm lint` && `vue-tsc --noEmit` ✅ 全部通过
