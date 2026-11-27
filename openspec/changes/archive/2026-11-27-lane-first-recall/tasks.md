# 任务：lane-first-recall

## 1. 配置与迁移

- [x] 1.1 新增 ai_settings 播种迁移：`lane_recall_top_k`（默认 `8`）、`lane_recall_max_distance`（默认 `0.16`），沿用项目既有 seed 模式（`ON CONFLICT DO NOTHING`，不覆盖用户已改值）
- [x] 1.2 在 `internal/topicgraph/repository` 增加配置读取（缺失/非法回退默认，打点日志），与 `LoadPersistentTopicConfig` 同风格
- [x] 1.3 测试：配置缺失、非法字符串、越界值三种输入各自回退默认且不报错

## 2. 泳道 ANN 召回查询

- [x] 2.1 新增仓储方法 `RecallTagsByLane(laneID uint, day time.Time, topK int, maxDistance float64)`：返回该泳道当日 Top-K 且距离 < maxDistance 的 event 标签及其距离
- [x] 2.2 查询 MUST 用 `(embedding)::halfvec(2560) <=> $1::halfvec(2560)` 匹配 `idx_topic_tag_embeddings_semantic_halfvec_hnsw`，再以原列 float32 距离复核并截断（ANN 只是召回，裁决用精确距离）
- [x] 2.3 过滤条件：`topic_tags.status='active'`、`category='event'`、`embedding_type='semantic'`、当日有文章（`pub_date` 落在报告日整天）
- [x] 2.4 批量入口 `RecallTagsForBoard(boardID uint, day time.Time)`：一次取出该版块全部 active 泳道，逐泳道召回并做「同标签多泳道取最近」合并
- [x] 2.5 泳道 `embedding IS NULL` 或维度不符 → 跳过 + warn，不中断
- [x] 2.6 测试：Top-K 截断、距离上限截断、多泳道同标签取最近、空泳道返回空、泳道无向量跳过

## 3. 收集管线并入泳道召回

- [x] 3.1 `collectBoardTags` 在 fallback 之后并入泳道召回结果，返回额外的 `laneRecalled map[uint]bool` 标记集合
- [x] 3.2 泳道召回结果 MUST 走与版块通道相同的跨版块排他判定（`semantic_board_report_exclusive_gap`，gap>0 时生效）
- [x] 3.3 泳道召回 MUST NOT 写入 `topic_tag_board_labels`
- [x] 3.4 同一标签被两条通道同时召回时只保留一条（取版块通道的 `MatchReason`/`Score`，泳道证据作为附加标记）
- [x] 3.5 测试：并集去重、排他生效、挂载表零新增

## 4. 质量分级放行泳道证据

- [x] 4.1 `filterTagsByQuality` 增加 `laneRecalled map[uint]bool` 入参：命中者直接进 `kept`，不参与 `kept>=10` 二元开关
- [x] 4.2 未经泳道召回的其他 `weighted` 保持既有丢弃/保底行为不变
- [x] 4.3 测试：`kept>=10` 且有泳道证据 → 泳道标签保留、其余 weighted 丢弃；`kept<10` → 既有保底行为不变（零回归）

## 5. L3 污染防护

- [x] 5.1 L2 裁决为 `new` 时，属于 `laneRecalled` 的标签直接丢弃，不 append 到 `l3pool`
- [x] 5.2 非泳道召回的标签按既有逻辑进入 L3
- [x] 5.3 L2 调用失败的整批降 L3 降级行为保持不变
- [x] 5.4 测试：泳道标签被判 new → 不入 L3；版块标签被判 new → 照旧入 L3；L2 失败 → 全部入 L3 + warn
  - ⚠️ 第三分句不适用：代码里**不存在** L2 失败整批降 L3 的路径（`ClusterTagsLane` 返错、该版块该轮生成失败）。已用 `TestLaneRecallL2FailureKeepsExistingAbort` 钉住现状，冲突登记见 review.md R-1。

## 6. 端到端核对

- [x] 6.1 用 2026-11-26 board 15220 的真实当天标签池做一次离线核对（不写库）：泳道 2687 / 2688 应召回当日 16 条英伟达系标签中的 ≥13 条，且 4 条噪音泳道（以黎同类）保持空白
- [x] 6.2 记录核对结果到 review.md：召回条数、命中的质量分级身份（weighted/unmounted）、是否出现邻座误召回
- [x] 6.3 确认 `lane_recall_max_distance` 改动后行为随之变化（0.16 → 0.20 放行量应显著上升）

## 7. Review

- [x] 7.1 派子线程聚焦 review（`zai-coding-cn/glm-5.4:max`，change-review 档），产出 R-9~R-14；High 2 项已修（R-9 加 direction_mismatch 排除、R-10 改 spec 对齐真实行为），Medium 4 项已修/已补测试，处置记录见 review.md 文末表
  - 实现线程已自查并建账 8 条（review.md R-1~R-8）；**独立子线程 review 待主线程派发**。
- [x] 7.2 `bash scripts/harness/review-trace.sh lane-first-recall` 不报零痕迹

## 8. 测试

## 9. 文档

<!-- doc-impact: flow, api, database, configuration -->

- [x] 9.1 `docs/reference/flow/daily-report.md`「业务约束与不变量」新增红线：日报候选收集 SHALL 包含泳道召回通道；泳道证据替代版块统计证据；泳道召回的标签 L2 判 new 不得进 L3；当天无专属新闻的泳道不得硬拉邻座内容
- [x] 9.2 `docs/reference/flow/topic-graph.md` 泳道召回参数说明（`lane_recall_top_k` / `lane_recall_max_distance`，与 `persistent_topic_lane_l1_threshold` 同模式）
- [x] 9.3 `docs/reference/database/tables/daily-report-watch.md` 补 `ai_settings` 两个新 key 与「泳道召回不写挂载表」的口径说明
- [x] 9.3b `docs/reference/configuration.md` 的 `ai_settings` 键表补 `lane_recall_top_k` / `lane_recall_max_distance`（与 `persistent_topic_lane_*` 同节，避免配置注册表漂移）
- [x] 9.4 归档后按 §12 在 flow 文档补「变更溯源」链接

## 10. 验证

| Scenario | 测试文件 |
| --- | --- |
| 新事件标签经泳道通道进入候选集 | `backend-go/internal/topicgraph/service/daily_report_lane_recall_test.go` |
| 当天没有专属新闻的泳道保持空白 | `backend-go/internal/topicgraph/service/daily_report_lane_recall_test.go` |
| 召回参数可在线覆盖 | `backend-go/internal/topicgraph/repository/daily_report_lane_recall_test.go` |
| 泳道向量缺失时跳过该泳道 | `backend-go/internal/topicgraph/repository/daily_report_lane_recall_test.go` |
| 高质量标签充足时新事件仍能入场 | `backend-go/internal/topicgraph/service/daily_report_filter_test.go` |
| 泳道召回标签被 L2 判 new 时丢弃 | `backend-go/internal/topicgraph/service/daily_report_lane_gate_test.go` |
| L2 调用失败时该版块生成失败且不降级自建泳道 | `backend-go/internal/topicgraph/service/daily_report_lane_gate_test.go` |


- `cd backend-go && go build ./...` → PASS（编译通过）
- `cd backend-go && go vet ./...` → PASS
- `cd backend-go && go test ./internal/topicgraph/service -run TestLaneRecall` → PASS（泳道召回与质量分级放行单测全绿）
- `cd backend-go && go test ./internal/topicgraph/repository -run TestLaneRecall` → PASS（召回查询与参数回退单测全绿）
- `cd backend-go && go test ./internal/platform/database -run TestLaneRecallConfigMigration` → PASS（播种幂等 + 不覆盖用户已改值，testcontainers）
- `cd backend-go && go test ./internal/topicgraph/service ./internal/topicgraph/repository ./internal/platform/database`（全量、非 `-short`、真 testcontainers）→ PASS（168s / 372s / 358s，零回归）
- `cd backend-go && golangci-lint run ./...` → PASS（`0 issues`）
- `bash scripts/harness/scenario-trace.sh openspec/changes/lane-first-recall` → PASS（7 个 Scenario 全部映射到存在的测试文件）
- `bash scripts/harness/doc-impact.sh verify openspec/changes/lane-first-recall` → PASS（声明域与文档改动对账通过）
- `bash scripts/harness/check-standards.sh --change lane-first-recall` → PASS
- `bash scripts/harness/check-tasks-tail.sh openspec/changes/lane-first-recall` → PASS（尾三节结构合规）
- `openspec validate lane-first-recall` → PASS（`Change 'lane-first-recall' is valid`）
