# Tasks

> 本阶段仅完成制品，以下实现/验收任务**均未执行**。不修改实现或 `docs/reference/**`、不提交。执行前先评审 design.md、test-cases.md、ui-design.md。
> 本 change 的 spec 落点：`openspec/changes/lane-compose-story-chain/specs/workshop-lane-compose/spec.md`（MODIFIED 候选话题侧栏 / 成员质心与关联推荐 / 保存与只读边界 + ADDED 事件线链）。

## 1. 后端

- [ ] 1.1 在 `backend-go/internal/topicgraph/repository/compose_chain.go` 新增事件线链纯函数 `BuildComposeStoryChains` 与类型 `ComposeStoryChain`（分组键=存活事件线 id；成员=窗口内带该事件线身份且归属话题非 active 的分区；同一分区可入多条链；跨度用窗口内成员极差的 UTC 日差 `utcDayDiff`；门槛 ≥2 分区且跨 ≥3 天；坏引用/两层链跳过）。函数保持纯函数（无 DB、无时钟），验证：`cd backend-go && go test ./internal/topicgraph/repository -run 'StoryChain'`。
- [ ] 1.2 在 `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go` 的 `GetComposeCandidates` 内新增**两次批量查询**（线索身份：`section_id IN (...)` 按 500 分批，形态对齐 `lane_snapshot_repository.go:listLaneSectionThreadStats` 但只投影 `section_id/story_id/scope`；该板 `board_stories` 批量 `(id,title,merged_into)`）与内存一跳存活解析（语义等同 `story_repository.go:ResolveBoardStoryID`，两层链/环/坏引用跳过并 warn），组装 `story_chains` 并回填 `stories_backfilled` / `stories_partial`；**不写 `daily_report_section_relations`、不新增表列**。验证：`cd backend-go && go test ./internal/topicgraph/repository -run 'ComposeCandidates'` 且 `git diff --stat -- backend-go/internal/platform/database` 输出为空（证明零 DDL）。
- [ ] 1.3 在 `backend-go/internal/topicgraph/handler/daily_report_handler.go` 的 `composeCandidates` 里把新字段按既有 nil→[] 口径归一（`story_chains` 缺省给 `[]`，与 `sections/relations/candidate_chains` 同款），确保旧前端与新前端都能解析。验证：`cd backend-go && go build ./...` 且 `go test ./internal/topicgraph/handler` 通过。

## 2. 前端

- [ ] 2.1 在 `front/app/api/persistentTopics.ts` 增 `ComposeStoryChain` 接口与 `ComposeCandidatesData.storyChains / storiesBackfilled / storiesPartial`，并在 `getComposeCandidates` 的归一化里处理 `story_chains`（缺省降级为空数组，旧后端不炸）。验证：`cd front && pnpm test:unit app/api/persistentTopics.test.ts --maxWorkers=2`。
- [ ] 2.2 在 `front/app/features/tags/composables/useLaneCompose.ts` 增「话题链 / 事件线链」维度态与事件线链的组勾选/取消（按成员 ID 去重合并进既有 `selectedIds`；取消单链只撤销本链成员、保留其它链与人工补选；两条链共用既有三态与计数）。验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCompose.test.ts --maxWorkers=2`。
- [ ] 2.3 在 `front/app/features/tags/components/compose/LaneComposeCandidates.vue` 渲染两条并列维度：维度切换（话题链/事件线链）、事件线组标题取事件线标题（空标题降级「事件线 #ID」）+ 元信息「N 个分区 · 跨天 N 天 · 同一事件」+ **事件线专属图标 + 「事件线」文字标签**（不得只靠颜色）；未回填 → 维度禁用 + 「事件身份未回填，本板暂无事件线链」；部分回填 → 「部分事件未回填，事件线链不完整」；确无跨天事件 → 「本板暂无可串联的事件线」（**不得**写成未回填）。验证：`cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2`。
- [ ] 2.4 在 `front/app/features/tags/components/compose/LaneComposeCanvas.vue` 增事件线链的独立线型与节点角标，并在既有图例中增列一行（identity / similarity / 事件线各一行）；**不得**把事件线边塞进既有 `relations` 边集或改变 1-hop 相关节点计算。验证：`cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts --maxWorkers=2`。
- [ ] 2.5 **不改**相似推荐口径：编排态侧 `front/app/features/tags/composables/laneComposeLogic.ts` 的 `centroidRecommendations` 与 `front/app/features/tags/composables/useInlineCompose.ts` 的 `recommendations`（线程浏览器侧）**阈值、排序、分组、文案一字不改**，只补回归断言：事件线身份不进入召回集、不改质心。验证：`cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts app/features/tags/composables/useInlineCompose.test.ts --maxWorkers=2`（既有用例全绿 + 新增一条负向断言）。

## 3. Review

- [ ] 3.1 派只读子线程做制品与 diff 审查（重点：两条链正交性是否被实现误合并、事件线链是否越权写话题/关系表、存活 id 一跳解析的坏引用处理、降级文案三态是否混淆、相似推荐是否被污染），意见按 High/Medium/Low 处置后写入 `openspec/changes/lane-compose-story-chain/review.md`。验证：`bash scripts/harness/review-trace.sh openspec/changes/lane-compose-story-chain` 退出码 0。

## 4. 测试

- [ ] 4.1 按 `test-cases.md` 更新下表真实既有测试（先证明旧契约断言仍成立再补新断言）；仓储测试只用隔离 PostgreSQL / testcontainer，**不用 SQLite**、不碰真库写入。验证：`cd backend-go && go test ./internal/topicgraph/repository` 通过。
- [ ] 4.2 补白盒分支用例（design §D2–D4、test-cases §5.1）：一个分区同属多链、active 分区不入链、两层合并链跳过、坏引用跳过、跨度不足不成链、成员数不足不成链、未回填/部分回填标记。验证：`cd backend-go && go test ./internal/topicgraph/repository -run 'StoryChain|ComposeCandidates'` 全绿。
- [ ] 4.3 回归断言：既有话题链口径与相似推荐口径**一字未改**（旧用例全绿），事件线链只增维度不夺资格。验证：`cd front && pnpm test:unit app/features/tags/composables/useInlineCompose.test.ts app/features/tags/composables/useLaneCompose.test.ts --maxWorkers=2` 全绿。
- [ ] 4.4 人工：按 `test-cases.md` §4 效果核对，在静态 :5100 只读抽样量化「各板事件线覆盖率 / 事件线链条数 / 跨天跨度分布」，如实记录分母与结论，不写库、不重建报告。验证：结论写入 `openspec/changes/lane-compose-story-chain/ui-verification/report.md`。

## 5. 文档

<!-- doc-impact: flow, api -->

- [ ] 5.1 实现阶段更新 `docs/reference/flow/topic-graph.md`（两条链的正交性与「事件线链只是选材维度」）、`docs/reference/flow/daily-report.md`（事件线链的构造口径与存活 id 一跳解析）、`docs/reference/flow/semantic-board.md`（未回填/部分回填的降级文案三态），并说明**不改**事件线归属判定与候选生命周期。**当前制品任务不执行这些 reference 改动。**
- [ ] 5.2 实现阶段更新 `docs/reference/api/` 下编排态取数端点文档（新增 `story_chains` / `stories_backfilled` / `stories_partial` 字段与旧后端兼容口径），实施前先核对实际文档落点，不另建平行权威文档。
- [ ] 5.3 归档后按规范补 flow 域的永久 archive 变更溯源链接；完成报告写「部署后影响 / 需要的操作 / 旧数据降级」，明确**需运维跑 `cmd/backfill-stories` 才有事件线链**。

## 6. 验证

- [ ] 6.1 `openspec validate lane-compose-story-chain --strict` → 输出 valid；本阶段仅此制品校验，不声称实现测试通过。
- [ ] 6.2 `bash scripts/harness/scenario-trace.sh openspec/changes/lane-compose-story-chain` → 29 个 Scenario 映射齐全（退出码 0）。
- [ ] 6.3 `bash scripts/harness/check-tasks-tail.sh openspec/changes/lane-compose-story-chain` → 尾三节（测试/文档/验证）、doc-impact 标记与 review 任务行齐备（退出码 0）。
- [ ] 6.4 `bash scripts/harness/change-scope.sh` → 机械判定影响包；按结果只测试影响包，禁止顺手全量。
- [ ] 6.5 `cd backend-go && golangci-lint run ./... && go vet ./... && go test ./internal/topicgraph/repository ./internal/topicgraph/handler && go build ./...` → 影响包测试与构建通过（测试 PG 用一次性容器/库）。
- [ ] 6.6 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit app/api/persistentTopics.test.ts app/features/tags/composables/useLaneCompose.test.ts app/features/tags/composables/useInlineCompose.test.ts app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts --maxWorkers=2 && pnpm build` → 定点测试与构建通过，串行执行，不与测试/浏览器并行；参数不带 `--`。
- [ ] 6.7 `bash scripts/harness/doc-impact.sh verify && bash scripts/harness/check-standards.sh` → reference 文档对账及标准校验通过（归档前）；`git diff --cached --name-only` → 本 change 无暂存；`git diff --check -- openspec/changes/lane-compose-story-chain` → 无空白错误。
- [ ] 6.8 人工：按 `test-cases.md` §1 主链路走完「新建泳道 → 切到事件线维度 → 整组勾齐跨天材料 → 保存」，用 agent-browser 在静态 :5100 逐状态取证（未回填降级、部分回填提示、两条链的图标+文字标签可区分），证据建账 `ui-verification/report.md`；没有证据不得勾选。

| Scenario | 测试文件 |
|---|---|
| 跨期连接而非连续出现 | `backend-go/internal/topicgraph/repository/compose_chain_test.go` |
| 没连接或跨度不足 | `backend-go/internal/topicgraph/repository/compose_chain_test.go` |
| 采纳预填 | `front/app/features/tags/composables/useLaneCompose.test.ts`, `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 取消整组与半选 | `front/app/features/tags/composables/useLaneCompose.test.ts`, `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 人工名称不被覆盖 | `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 清空与取消本次 | `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 已有泳道显式参考 | `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 历史启用资格 | `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 两条链并列且互不干扰 | `front/app/features/tags/composables/useLaneCompose.test.ts`, `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 事件线维度不可用时诚实降级 | `front/app/features/tags/composables/useLaneCompose.test.ts`, `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 采纳后质量刷新 | `front/app/features/tags/composables/laneComposeLogic.test.ts`, `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 点击节点推荐关联 | `front/app/features/tags/composables/laneComposeLogic.test.ts`, `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 从推荐项加入 | `front/app/features/tags/composables/laneComposeLogic.test.ts`, `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 无关联 | `front/app/features/tags/composables/laneComposeLogic.test.ts` |
| 无距离数据 | `front/app/features/tags/composables/laneComposeLogic.test.ts` |
| 相近程度不等于已选 | `front/app/features/tags/composables/laneComposeLogic.test.ts` |
| 事件线不进入相似推荐召回 | `front/app/features/tags/composables/laneComposeLogic.test.ts`, `front/app/features/tags/composables/useLaneCompose.test.ts`, `front/app/features/tags/composables/useInlineCompose.test.ts` |
| 缺名称 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 保存成功 | `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 事件线链整组带入不改候选状态机 | `front/app/features/tags/composables/useLaneCompose.test.ts`, `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 同一事件跨天直连成链 | `backend-go/internal/topicgraph/repository/compose_chain_test.go`, `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 成员数或跨度不足不成链 | `backend-go/internal/topicgraph/repository/compose_chain_test.go`, `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 链键按存活事件线归一 | `backend-go/internal/topicgraph/repository/compose_chain_test.go`, `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 一个分区可同属多条事件线链 | `backend-go/internal/topicgraph/repository/compose_chain_test.go`, `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 事件线链只提供整组勾选 | `front/app/features/tags/composables/useLaneCompose.test.ts`, `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 事件线链不产生或激活话题 | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go`, `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 未回填板诚实降级 | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go`, `front/app/features/tags/composables/useLaneCompose.test.ts`, `front/app/features/tags/components/console/BoardComposeOverlay.test.ts` |
| 部分回填标记不完整 | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go`, `front/app/features/tags/composables/useLaneCompose.test.ts` |
| 事件线链不写入关系表 | `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` |
| 完整 UI 故事：两条链并列 → 按事件一次勾齐跨天材料 → 保存 | 人工：agent-browser 静态 :5100，证据写本 change `ui-verification/report.md` |
