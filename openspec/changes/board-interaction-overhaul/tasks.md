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

- [ ] 11.1 `TagsPage.vue`: 新增 `contentTab` ref（`'composition' | 'narratives' | 'articles'`），默认 `'composition'`，选中 board 时显示 Tab 栏（板块内容 / 叙事 / 文章），按 tab 切换显示对应面板
- [ ] 11.2 BoardCompositionPanel、BoardNarrativeTimeline、文章时间线各区域用 `v-if="contentTab === 'xxx'"` 控制显隐
- [ ] 11.3 Tab 栏样式：简洁横向 tab，和页面整体暗色风格一致，选中态用 accent 色
- [ ] 11.4 验证：`pnpm lint` + `vue-tsc --noEmit`

## 10. 整理叙事功能（按板块/日期触发）

- [x] 10.1 `service.go`: 新增 `GenerateAndSaveForBoard(semanticBoardID uint, date time.Time) (int, error)`，只收集指定 board 的 input，生成单板叙事
- [x] 10.2 `narrative_handler.go`: 新增 `POST /api/narratives/boards/generate` endpoint，参数 `{ date: string, board_id?: number }`，board_id 为空时生成全部
- [x] 10.3 `front/app/api/semanticBoards.ts`: 新增 `triggerNarrativeGeneration(params)` 方法
- [x] 10.4 `SemanticBoardList.vue`: 在“匹配参数”按钮下新增“整理叙事”按钮
- [x] 10.5 新增 `NarrativeGenerateDialog.vue`：日期选择器 + 板块下拉（可选全部），确认触发
- [x] 10.6 `TagsPage.vue`: 集成 NarrativeGenerateDialog，绑定按钮事件
- [x] 10.7 验证：`go build ./...` + `pnpm lint` + `vue-tsc --noEmit`

## 11. 集成验证

- [ ] 11.1 端到端：文章 → tag 提取 → 辅助标签 → board 匹配（验证 max_sim 双因子约束生效）
- [ ] 11.2 端到端：手动触发升级建议 → 确认执行 → 面板展示 label 而非 #id
- [ ] 11.3 端到端：选中 board → 叙事时间线展示 → 点击叙事展开文章 → 文章带 filtered_tags
- [ ] 11.4 端到端：Feed/时间筛选 board 文章列表
- [ ] 11.5 端到端：叙事生成 → 单 board 单日单叙事板（不按 category 拆分）→ /tags 页面查看
- [ ] 11.6 验证：`go build ./...` && `go test ./...` && `golangci-lint run ./...`
- [ ] 11.7 验证：`pnpm lint` && `pnpm exec nuxi typecheck` && `pnpm build`
