# Review — add-ai-generated-article-detection

- 审查线程：change-review（minimax-cn/MiniMax-M3.1-Flash:max），2026-11-21
- 范围：制品（proposal/design D1-D14/specs×9/tasks/ui-design/explore-findings）+ 工作区实现 diff（前后端全量落点）+ 只读门禁复跑
- 总评：制品与实现逐条对得上，关键语义（失败不改写/幂等/阈值边界/人工主权/D14 命名/中性色三形态/窄投影/稳定分区）全部有真断言；2 High（开关链路死/并发覆盖）+ 6 Medium + 3 Low
- 核对通过项：列表窄投影只加 ai_detection_status；红线 45 稳定分区；capability 七类三处登记（后端枚举/并发/前端面板）；人工结论闸在 worker/服务/入队三路径有效；判定失败三路径零改写；队列幂等（唯一索引+ON CONFLICT+条件 UPDATE 乐观锁）；阈值 = 即疑似与在线调整；camelCase 映射只在 API 模块（唯一例外见 R-3）；中性色无警示无图标；后端 40+ 用例前端 143 用例真断言全绿

### R-1 2026-11-21【High】订阅源「AI 生成检测」开关后端静默丢弃，功能入口整链死路

- **状态**：已修复（收尾线程 2026-11-21 补 `UpdateFeedRequest.AIDetectionEnabled` + bodyMap presence 写入 + `TestUpdateFeedAIDetectionEnabled` 开关落库/不误覆盖两断言，主会话验收 diff）（已复核属实：`grep ai_detection internal/reader/handler/feed_handler.go` 零命中——`UpdateFeedRequest` 无字段、updates 白名单无分支；前端链路完整发载荷，PUT 返回 200 但字段被丢，开关弹回。修法：request 加 `AIDetectionEnabled *bool` + bodyMap 存在性写入（与 tagging_enabled 同款）+ handler 回读断言测试；违反 feed-settings-ui spec「切换失败 SHALL NOT 呈现为已保存态」）

### R-2 2026-11-21【High】判定结论整行 Save 落库，与摘要 worker 并发互覆（结论或摘要静默丢失且不重判）

- **状态**：已修复（`PersistAIDetectionResult` 改 `Model(&models.Article{}).Where("id = ?").Updates(...)` 只写 ai_detection_status/score/source/at 四列（含 detectedAt），不再整行 Save；新增 `TestPersistAIDetectionResultDoesNotClobberConcurrentWriter` 断言「加载快照 → 另写摘要/摘要状态/已读 → 判定落库」后四字段写入且摘要/状态/已读零回滚；`go test ./internal/reader/service -short -count=1` 绿）

### R-3 2026-11-21【Medium】camelCase 映射调用点在 feature composable 而非 API 模块（reading.md 约束 1 边界）

- **状态**：已修复（归一化收进 `front/app/api/articles.ts#getArticle`（用 `normalizeArticleDetection`），`useDailyReportReader.ts` 删掉 `as unknown as ArticleDetectionPayload` 与直接调用，只消费 camelCase 四字段；新增 `front/app/api/articles.test.ts` 断言 API 边界归一 + 非法枚举降级 + 失败透传）

### R-4 2026-11-21【Medium】日报后端 thread.articles 投影前端未消费，排序口径双份 + N+1

- **状态**：已修复（`DailyReportThread` 补 `articles?` 投影类型，`DailyReportTopicSection.vue` 直接按 `thread.articles` 渲染并删掉 per-article getArticle N+1 与前端二次重排（旧响应无该字段时回退标题缓存、不排序）；测试断言顺序跟随投影、不再 emit ensureArticles、成员与「N 篇」计数不变）

### R-5 2026-11-21【Medium】列表行角标在两行 clamp 下被整块裁掉（长标题不可见）

- **状态**：已修复（角标移出 `.row-title` 的 clamp 盒，挂在新 `.row-heading` 与 h3 同级、对齐首行；中性色契约不变（border-medium / text-secondary、无警示色无图标、几何同 .archived-badge）；测试改为结构断言「`.row-title` 内无角标 + `.row-heading > .suspect-badge`」并保留 clamp/CSS 锚）

### R-6 2026-11-21【Medium】显式重判无前端入口，误标后无法回到「由机器判」

- **状态**：已修复（`ArticleStatusMenu` 检测组补第三项「交由模型重新判定」→ emit `recheck-ai-detection`；`api/articles.ts` 加 `recheckAIDetection`（POST /articles/:id/ai-detection/recheck）；`useArticleContentView` 补 `manualRecheckLoading` pending 态与成功 toast（失败走菜单内联错误位）；菜单测试断言渲染/emit/pending 置灰，API 测试断言路径与载荷）

### R-7 2026-11-21【Medium】spec 文案残留「八条能力」（实现与测试已九条）

- **状态**：已修复（Scenario 改「九条能力均有供血说明」+ THEN 九条，>500 字符 Requirement 的关键分工清单移入新 Scenario「供血文案与后端实际调用方一致」，Requirement 正文 323 字符；`openspec validate add-ai-generated-article-detection --strict` 退出 0。附：`openspec/specs/ai-capability-routing/spec.md` 同名陈旧场景（八条）同步改一行——不改则 MODIFIED 块会被「omits scenario(s) the current spec still has」守卫判为丢弃场景，归档同样被拒）

### R-8 2026-11-21【Medium】红线登记文档 8.2-8.6 未落地（reading.md 约束 9 字面违反中）

- **状态**：已修复（8.1-8.7 全落地：ai-summary 七类红线 2、reading 约束 9 纳入 ai_detection_status、daily-report 新增 59/60/61、content-enrichment 新增 16、api 四份、database detection_jobs 表、standard 两份；主会话抽查命中）（收尾线程 A 正在执行 tasks 8.1-8.7——ai-summary.md 已于 12:40 更新为七类；reading.md/daily-report.md/content-enrichment.md/api/database 待 A 完成后复核消账）

### R-9 2026-11-21【Low】semanticBoards.ts 展开残留 snake_case 原键（双键并存）

- **状态**：已修复（新增 `normalizeBoardArticle`：先 `delete` 四个 `ai_detection_*` 原键再挂 camelCase 四字段，不再 `{...item, ...normalizeArticleDetection(item)}` 双键并存；组件层只见 camelCase）

### R-10 2026-11-21【Low】tasks.md 5.x/7.x/9.x 记账与实现状态脱节

- **状态**：已修复（2026-11-21 主会话回填 1.x/2.x/3.x/5.x/9.11 勾选，7.x/8.x/9.x 由收尾线程勾选带证据，现 50/52 勾，仅剩 7.3 冒烟与 8.8 归档后溯源）（5.1-5.7 实现已完成未勾选；7.x/9.x 由线程 A 执行中，完成后统一回填）

### R-11 2026-11-21【Low】人工标记置信分写界值、类名与 ui-design 建议名不一致

- **状态**：已采纳（界值 1.0/0.0 已在 tasks 4.7 登记为契约；类名差异按 human-feedback FB-1「原型不作 1:1 规格」不算缺陷，仅记录）

### R-12 2026-11-21【Low】实现拆散于已提交与未入库文件，当前 HEAD 不可构建

- **状态**：待处理（收口时把本 change 新增/修改文件一次性单独提交，确认 git stash 无残留；提交前需 HEAD 构建验证）

### R-13 2026-11-22【Medium】列表角标与标题争抢同一行宽度（52%），长标题可读性显著下降

- **状态**：已修复（方向 b：角标从 `.row-heading` flex 行移入 `.row-meta` 行尾（`flex:none` + `margin-left:auto`），标题拿到整列宽——浏览器实测 303px 行（内容列 165px）标题 165px 满宽（修复前 80px）、角标 77px 与收藏星/状态图标零重叠、有/无角标时 meta 19px 与行高 100px 完全一致；`.row-title` 两行 clamp 与中性色三形态不变，未判定不渲染；测试改为断言 `.row-title`/`.row-heading` 内零角标 + `.row-meta` 末元素是角标 + 落位 CSS 锚（`pnpm test:unit app/features/articles/components/ArticleCardView.test.ts --maxWorkers=2` 17/17 绿，`pnpm lint` / `pnpm exec nuxi typecheck` 退出 0））

### R-14 2026-11-22【Low】恢复 unknown 必须省略 source 字段，显式清空无路径

- **状态**：已修复（`buildManualAIDetectionUpdates` 放行 unknown + source 空串（与省略同义，走既有清空分支 score=0/source=""/at=nil），非空非 manual 仍 400、空串 + 非 unknown 仍 400（标记必须显式给 manual 或省略）；新增 `TestUpdateArticle_UnknownAcceptsEmptySource` 两断言用例，`go test ./internal/reader/handler -short -count=1` 绿）

### R-15 2026-11-22【Low】recheck 复用活跃作业时 attempt_count 只升不降，多次重判烧光重试预算

- **状态**：已修复（`applyDetectionJobUpdates` 在 `IsForceDetectionReason(req.Reason)` 时把 `attempt_count` 置 0，不依赖 reason 是否变化（连点仍重置）；新增 `TestEnqueueDetectionJobRecheckResetsAttemptBudget` 断言 4/5 → recheck → 0、普通入队保持 4、再重判仍归 0 且 `max_attempts` 不动；`go test ./internal/reader/repository -short -count=1` 绿）
