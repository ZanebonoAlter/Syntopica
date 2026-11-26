# Tasks: 删掉话题泳道折线，改为线索行内的「相关事件」推荐

> 依据：`proposal.md` / `design.md`（D1–D10）/ `specs/thread-related-events/spec.md` / `specs/daily-report-system/spec.md` / `ui-design.md`（已 approved）。
> **形态红线**：线索行内 chip + 悬停浮层 + 点击开文章。MUST NOT 实现成按天分组的事件列表/时间轴（v1 已否决）。
> 实现顺序：后端读模型 → HTTP 层 → 前端数据层 → 组件 → 移除旧视图 → 回归。

## 1. 后端读模型（repository）

- [x] 1.1 新建 `backend-go/internal/topicgraph/repository/thread_relations.go`：定义 `ThreadRelationsResponse{report_id, days, window_start, window_end, status, threads map[uint][]ThreadRelation, total, skipped_no_embedding}` 与条目 `ThreadRelation{thread_id, title, summary, period_date, score, level, confirmed, source_article}`。
- [x] 1.2 取当期线索：`daily_report_sections`（按 `report_id`）→ `daily_report_threads`（按 `section_id`），拿到每条 thread 的 `embedding`、`tag_ids` 与所属 `persistent_topic_id`。
- [x] 1.3 取候选：同 `persistent_topic_id`、`board_daily_reports.period_date` ∈ `[report_date-days, report_date-1]` 的 section → 其 thread（**要求 `embedding` 非空**）；先按 section 收敛再算，禁止全表向量扫描。
- [x] 1.4 余弦筛选（design D3）：`score = 1 - (current.embedding <=> candidate.embedding)`，**只保留 ≥ 0.75**；不叠加时间衰减、篇数权重或任何二次加权。
- [x] 1.5 档位（design D4）：`level = score >= 0.85 ? "strong" : "related"`；分数不取整。
- [x] 1.6 排序：`score` desc → `period_date` desc → `thread_id` asc；**返回全量**不截断。
- [x] 1.7 来源文章：取候选 thread 的 `related_article_ids` 首篇，一次批量查 `articles` 取 title/feed/published_at；查不到时 `source_article = nil`（不编造标题）。
- [x] 1.8 confirmed：候选 thread 的 event 标签在本话题存在 `state='confirmed'` 注记（`topic_key_event_markers.tag_ids_snapshot` 命中）时置 `true`。只读，不改注记。
- [x] 1.9 缺失向量与三态：当期 thread 无 embedding → 该线索不出现在 `threads`、计入 `skipped_no_embedding`；`skipped_no_embedding > 0` 时 `status=partial`；无任何推荐时 `status=empty` 且 `threads` 为空 map（JSON `{}` 非 `null`）。

## 2. 后端 HTTP 层

- [x] 2.1 新建 `backend-go/internal/topicgraph/handler/thread_relations_handler.go`：`GET /api/daily-reports/:id/thread-relations`，解析 `days`（只 7/30，其它回落 7 并回显实际值）。
- [x] 2.2 注册路由到既有 `/daily-reports` 路由组；`reportId` 不存在返回 404。
- [x] 2.3 只读约束：不写任何表/缓存/索引，不调模型，不计算 embedding，不触达 `ListTopicRecentBriefs` 的 prompt 路径。

## 3. 前端数据层

- [x] 3.1 `front/app/api/dailyReports.ts`：新增 `ThreadRelationsResponse` / `ThreadRelationItem` 类型与 `getThreadRelations(reportId, { days })`。
- [x] 3.2 新建 `front/app/features/tags/composables/useThreadRelatedEvents.ts`：模块级单例 + `Map<"reportId:days", entry>` 缓存，形状对齐 `useTopicKeyEvents`（`status: idle|loading|success|error`）；提供 `entry(reportId, days)` / `ensure(reportId, days)` / `retry`。
- [x] 3.3 并发同 key 只发一次；切档命中缓存不重取；错误可重试。

## 4. 前端组件

- [x] 4.1 新建 `front/app/features/tags/components/daily-report/ThreadRelatedEvents.vue`：最多 3 个 chip + 「还有 N 条」+ `7 天 / 30 天` segmented；**无数据时整行不渲染**（`v-if` 在外层，不留占位）。
- [x] 4.2 chip：`<button>`、`max-width: 15rem` 单行 ellipsis、`title` 保留全文、★ 用 `--color-accent` 描边区分。
- [x] 4.3 预览浮层：`position: fixed` + 视口边界翻转；内容四段（标签名+描述 / 来源文章标题 / 关联度档位+条+分数 / 最近出现+出现次数）；`focus` 同样触发；`Esc` 关闭；`aria-describedby` 关联。
- [x] 4.4 点击 chip → 调用既有站内阅读预览弹窗（传 `articleId` + `reportId` 上下文），弹窗内补充关联度与出现次数。
- [x] 4.5 `DailyReportTopicSection.vue`：在 `.drm-thread-row` 闭合之后（约 `:471`）插入 `<ThreadRelatedEvents>`；新增 `relatedDays` 状态与 `threadRelationsEntry(thread.id)`；**移除** `DailyReportMiniLifeline` 挂载（`:512-628`）、`selectedDayByTopic`、`selectLifelineDay`、`historicalSections`、`historicalPending`、`ensureLifeline` 与 `#details` 插槽。
- [x] 4.6 可访问性与动效：可见 focus、`prefers-reduced-motion` 关闭浮层过渡、触屏点击直达文章。

## 5. 移除旧视图与回归修复

- [x] 5.1 删除 `front/app/features/tags/components/daily-report/DailyReportMiniLifeline.vue`。
- [x] 5.2 grep 复核 `buildLifelineWindow` / `buildBezierPath` / `ensureLifeline` / `TopicLifelineData` 的消费方；确认无其它引用后删除函数与日报侧调用链；`getTopicLifeline` API 方法与后端 lifeline 端点保留。
- [x] 5.3 更新 `DailyReportTopicSection.test.ts`（stub 改为 `ThreadRelatedEvents`、DOM 顺序断言、无相关时不渲染）与 `useDailyReportReader.test.ts`（lifeline 用例退役）。

## 6. Review

- [x] 6.1 派 `change-review` 子线程聚焦审查：契约一致性（spec Scenario ↔ 代码）、`score=余弦` 与阈值 0.75/档位 0.85 是否逐字一致、缺向量是否计入 `skipped_no_embedding` 且 `status=partial`、confirmed 是否只读、「无相关不渲染」是否被 `min-height`/占位悄悄破功；意见按 High/Medium/Low 处置并落 `review.md` 建账。

## 7. 测试

- [x] 7.1 后端 repository：新建 `backend-go/internal/topicgraph/repository/thread_relations_pg_test.go`（testcontainer PG，禁 SQLite），覆盖阈值上下界（0.75/0.72）、档位边界（0.85）、排序稳定性、全量返回、来源文章缺失为 null、confirmed 只读判定、当期缺向量计入 skipped 且 status=partial、窗口不含当期、只读幂等。
- [x] 7.2 后端 handler：新建 `backend-go/internal/topicgraph/handler/thread_relations_handler_test.go`，覆盖 200 / `days=14` 回落 7 且回显 / 404 / 无推荐返回 `{}` 与 `status=empty`。
- [x] 7.3 前端组件：新建 `front/app/features/tags/components/daily-report/ThreadRelatedEvents.test.ts`，覆盖 3+「还有 N 条」、★、长标签 ellipsis + `title`、浮层四段内容、focus 触发、Esc、点击 emit 文章事件、无数据显示空（不渲染）。
- [x] 7.4 前端布局回归：新建 `front/app/features/tags/components/daily-report/DailyReportEvents.layout.test.ts`，断言 chip 行位于 `.drm-thread-row` 之后、不进入其两列 grid、不再存在 `gridColumn: N / span 2` 定位。
- [x] 7.5 前端数据层：新建 `front/app/features/tags/composables/useThreadRelatedEvents.test.ts`，覆盖 `(reportId, days)` 缓存、重复展开不重取、并发去重、错误重试、档位互不污染。
- [x] 7.6 回归：跑 `DailyReportTopicSection.test.ts`、`useDailyReportReader.test.ts`、`DailyReportReader.test.ts`。
- [x] 7.7 复杂档白盒附加：把变体走查（窗口边界、空候选、悬空、同标签多条来源、列表长度、超长标签）填进 `test-cases.md`。

## 8. 文档

<!-- doc-impact: flow, api, architecture -->

- [x] 8.1 `docs/reference/api/`：补 `GET /daily-reports/:id/thread-relations` 契约（参数、响应字段、关联度公式与档位、partial 语义、错误码）。
- [x] 8.2 `docs/reference/flow/daily-report.md`：更新阅读台链路（线索行 chip、浮层、点击开文章、7/30 档位）；在「业务约束与不变量」补红线（关联度确定性不打 embedding、悬空候选不进推荐位但 status 必须 partial、无相关不渲染、chip 不改线索行结构）。
- [x] 8.3 `docs/reference/flow/topic-graph.md`：补一条边界（线索相关推荐只读、不做语义归并、不复用 related-events 阈值）。
- [x] 8.4 `docs/reference/architecture/map.md`：补业务域 → 代码入口（新 handler / repository / 组件 / composable）。

## 9. 验证

每条为「可执行命令 + 期望结果」。

- [x] 9.1 `cd backend-go && go build ./...` → 退出码 0。
- [x] 9.2 `cd backend-go && go test ./internal/topicgraph/repository ./internal/topicgraph/handler` → 全绿。
- [x] 9.3 `cd backend-go && golangci-lint run ./internal/topicgraph/...` → 退出码 0、无新增告警。
- [x] 9.4 `cd front && pnpm test:unit app/features/tags/components/daily-report/ThreadRelatedEvents.test.ts app/features/tags/components/daily-report/DailyReportTopicSection.test.ts app/features/tags/components/daily-report/DailyReportEvents.layout.test.ts app/features/tags/composables/useThreadRelatedEvents.test.ts --maxWorkers=2` → 全绿（**路径相对 `front/`，不能再带 `front/` 前缀**；vitest include 是 `app/**`；参数不带 `--`）。
- [x] 9.5 `cd front && pnpm exec nuxi typecheck` → 退出码 0。
- [x] 9.6 `cd front && pnpm lint` → 退出码 0。
- [x] 9.7 `cd front && pnpm build` → 构建成功；随后 `bash scripts/dev/deploy-frontend.sh` 铺静态产物并健康检查通过。
- [x] 9.8 major UI 双层验收：agent-browser 覆盖 1440×900 与 1920×1080 + 900px 正文列 + 双主题，逐条对账 `ui-design.md`「Acceptance」A-1～A-9，结果写入 `ui-verification/report.md`（每条 `### A-<N> <场景>【pass|fail|未覆盖】` + 依据行，截图存 `ui-verification/`）。
- [x] 9.9 `bash scripts/harness/scenario-trace.sh openspec/changes/replace-lane-lifeline-with-topic-events` → 退出码 0（下表逐条对账）。
- [x] 9.10 `bash scripts/harness/check-tasks-tail.sh openspec/changes/replace-lane-lifeline-with-topic-events` → 退出码 0。
- [x] 9.11 `bash scripts/harness/doc-impact.sh verify openspec/changes/replace-lane-lifeline-with-topic-events` → 退出码 0。
- [x] 9.12 `bash scripts/harness/ui-verification-trace.sh openspec/changes/replace-lane-lifeline-with-topic-events` → 退出码 0。
- [x] 9.13 实现与已批准原型逐交互对账：`ui-design.md`「Interaction Contract」6 条主操作无多无少，「禁止项」5 条无越界。

### Scenario → 测试映射

| Scenario | 测试文件 |
| --- | --- |
| 查询 7 天窗口 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 非法天数回落并回显 | backend-go/internal/topicgraph/handler/thread_relations_handler_test.go |
| 日报不存在 | backend-go/internal/topicgraph/handler/thread_relations_handler_test.go |
| 无任何推荐 | backend-go/internal/topicgraph/handler/thread_relations_handler_test.go |
| 同话题高相似线索入选 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 低于阈值不入选 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 分数不做二次加权 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 不做语义归并 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 稳定排序 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 全部返回 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 已确认线索加星 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 只有 AI 建议不加星 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 来源文章不可用 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 当期线索无向量 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 全部线索缺向量时 empty 优先 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 历史线索无向量不进候选 | backend-go/internal/topicgraph/repository/thread_relations_pg_test.go |
| 读请求不产生写 | backend-go/internal/topicgraph/handler/thread_relations_handler_test.go |
| 展示日报卡片列表 | front/app/features/tags/components/DailyReportReader.test.ts |
| 打开全屏日报详情 | front/app/features/tags/components/DailyReportReader.test.ts |
| 宽屏双栏与窄屏降级 | front/app/features/tags/components/DailyReportReader.test.ts |
| 双主题渲染 | 人工：editorial/dark 各截一图核对 chip 与浮层可读性 |
| 按持久话题状态分区 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 线索行渲染相关事件 chip | front/app/features/tags/components/daily-report/ThreadRelatedEvents.test.ts |
| 无相关事件的线索不渲染 chip 行 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| chip 悬停出预览 | front/app/features/tags/components/daily-report/ThreadRelatedEvents.test.ts |
| 点击 chip 打开阅读预览 | front/app/features/tags/components/daily-report/ThreadRelatedEvents.test.ts |
| 7/30 档位切换与缓存 | front/app/features/tags/composables/useThreadRelatedEvents.test.ts |
| 推荐加载失败不阻断阅读 | front/app/features/tags/components/daily-report/ThreadRelatedEvents.test.ts |
| identity 连线跨越空白日期 | 人工：确认日报 DOM 中不存在 svg 连线（grep 组件模板） |
| 展开并缓存话题生命线 | front/app/features/tags/composables/useDailyReportReader.test.ts |
| lifeline 加载失败 | front/app/features/tags/composables/useDailyReportReader.test.ts |
| 进入完整话题生命线 | front/app/features/tags/components/daily-report/DailyReportEvents.layout.test.ts |
| 窄容器排版不崩坏 | front/app/features/tags/components/daily-report/DailyReportEvents.layout.test.ts |
| 展开 thread 文章并复用缓存 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 键盘关闭详情 | front/app/features/tags/components/DailyReportReader.test.ts |
| 空状态 | front/app/features/tags/components/DailyReportReader.test.ts |
| 加载超过 30 天 | front/app/features/tags/components/DailyReportReader.test.ts |
| 切换 board 重置状态 | front/app/features/tags/components/DailyReportReader.test.ts |
