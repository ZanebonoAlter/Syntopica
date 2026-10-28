## 1. 数据模型与配置

- [x] 1.1 `DailyReportSection` 增加 `bucket_title`（`gorm:"size:200"`，可空）与 `bucket_index`（`gorm:"not null;default:0"`）两列并补迁移；验证：`cd backend-go && go build ./...` 通过，且 `go test ./internal/topicgraph/repository -run TestDailyReportSectionBucketColumns` 通过（断言两列可读写、历史行默认 0/NULL）
- [x] 1.2 扩展 `PersistentTopicConfig` 读取 `daily_report_intra_bucket_enabled`(默认 true) / `daily_report_intra_bucket_min_threads`(6) / `daily_report_intra_bucket_min_tags`(10)；验证：`go test ./internal/topicgraph/repository -run TestPersistentTopicConfigBucketDefaults` 断言默认值与配置覆盖
- [x] 1.3 `SaveReport` 写入新列并保持同事务；验证：`go test ./internal/topicgraph/repository -run TestSaveReportPersistsBucketColumns` 通过

## 2. 分桶服务（两阶段 + 校验器）

- [x] 2.1 实现 `planBucketCount(lines, tags int, cfg) (k int, trigger bool)`，规则 `k = clamp(round(lines/2.5), 2, 6)`；验证：`go test ./internal/topicgraph/service -run TestPlanBucketCount` 覆盖 2/7/9/10/17/25 条线索、开关关闭与未达阈值分支
- [x] 2.2 实现 phase 1（定方向名 + 每组当日章节标题）prompt 与解析，operation=`daily_report.bucket_theme_names`；验证：`go test ./internal/topicgraph/service -run TestBucketPhase1Parse`（含组数多于 k 时合并最小两组、少于 k 时按实际组数继续）
- [x] 2.3 实现 phase 2（固定方向名逐条归属）prompt 与解析，operation=`daily_report.bucket_assign`，温度 0、禁增删改名；验证：`go test ./internal/topicgraph/service -run TestBucketPhase2Parse` 通过
- [x] 2.4 实现确定性校验器 `repairBuckets`：未覆盖并入最近组 / 重复保留首处 / 单例合并 / 空组丢弃 / 收敛后 1 组返回退化标记；验证：`go test ./internal/topicgraph/service -run TestRepairBuckets` 覆盖五类用例（含空组分支）
- [x] 2.5 实现 `BucketCluster(ctx, cluster, threads, cfg)` 组装：产出同 `cluster_index`、`bucket_index` 递增、`bucket_title`、`cluster_tag_ids` 为 tag 并集、`GroupName` 为该组当日章节标题（缺失时按**桶内** thread 标题 → 泳道 label 兜底）；失败返回 error 由调用方降级；验证：`go test ./internal/topicgraph/service -run TestBucketCluster`（含失败不产半成品、空组不产 section）

## 3. 流水线接入与护栏

- [x] 3.1 在 `GenerateDailyReport` 的 thread 生成之后、section embedding 生成之前接入分桶；分桶失败仅 warn 并保持单 section；验证：`go test ./internal/topicgraph/service -run TestOrchestratorBucketStep`（成功拆分 / 失败降级两分支），并在 `daily_report_orchestrator_test.go` 补 `prev_report_id` 链接断言（对齐「日报关联昨日报告」Scenario）
- [x] 3.2 分桶产出的每条 section 以自身桶内 tag 计算 `buildSectionEmbedText` 与 embedding（不复用整簇文本）；验证：`go test ./internal/topicgraph/service -run TestBucketSectionEmbeddingPerBucket`
- [x] 3.3 兄弟 section 继承 `lane_tier` / `persistent_topic_id` / `topic_match_confidence`，各自计算 `topic_match_distance`；验证：`go test ./internal/topicgraph/service -run TestBucketSiblingsInheritAssignment`
- [x] 3.4 同日合并护栏：`sameAnchorClass` 前增加「`cluster_index` 相同即拒绝」；`daily_report_section_merge_enabled` 默认保持 false；验证：`go test ./internal/topicgraph/service -run TestMergeRejectsSiblingBuckets`
- [x] 3.5 `promptVersion` 由 `"5.0"` 升至 `"6.0"` 并同步更新既有断言（`daily_report_llm_test.go:75` 现断言 5.0）；验证：`go test ./internal/topicgraph/service -run TestPromptVersion` 通过

## 4. 重新分桶 API

- [x] 4.1 实现组级重跑服务 `RebucketSectionGroup(ctx, reportID, sectionID)`：事务开头取 `dailyReportSaveXactKey(boardID, periodDate)` 的 `pg_advisory_xact_lock`，锁内复核报告 status 与目标 `lane_tier`；未达阈值直接返回 `buckets=1` 不改数据；验证：`go test ./internal/topicgraph/service -run "TestRebucket(ThresholdGate|RejectsNonDone|RejectsWatchSection)"`
- [x] 4.2 组内替换与派生：复用/补建/删除 section 行、thread 迁移 `section_id`、markers 按 `source_thread_id` 重派生 `source_section_id`、`topic_watch_hits` 同步迁移、`report_annotations` 按 `thread_id` 重派生、重算 `cluster_label`/`bucket_title`/`bucket_index`/`cluster_tag_ids`/`article_count`/`embedding`/`topic_match_distance`/`best_tier`/`avg_score`/`quality_breakdown` 并保留 `topic_status_at_report` 快照、重算质心、双向重建该组关系；验证：`go test ./internal/topicgraph/service -run TestRebucketReplacesGroupKeepingThreads` 与 `-run TestRebucketDerivedFields`
- [x] 4.3 失败路径：LLM 失败 / 解析失败 / 事务失败整体回滚；验证：`go test ./internal/topicgraph/service -run TestRebucketRollbackOnFailure`
- [x] 4.4 并发路径：rebucket 与同日重跑并发时不产生幽灵 section、不出现 UPDATE 静默 0 行；验证：`go test ./internal/topicgraph/service -run TestRebucketSerializedWithSaveReport`（同键 advisory lock 断言）
- [x] 4.5 注册路由 `POST /api/daily-reports/:id/rebucket`，请求体 `{section_id}`，响应返回该组最新 sections 与 `buckets`；验证：`go test ./internal/topicgraph/handler -run TestRebucketHandler`
- [x] 4.6 前端深链兼容：被删除桶行的旧锚点 `#report-section-<id>` 失效时回退到该组首条 section；验证：`go test ./internal/topicgraph/service -run TestRebucketKeepsGroupHead` + 前端 5.4 用例

## 5. 前端展示与动作

- [x] 5.1 `front/app/api/dailyReports.ts` 的 `DailyReportSection` 增加 `bucket_title?: string` / `bucket_index?: number`，新增 `rebucketSection(reportId, sectionId)`；验证：`cd front && pnpm test:unit app/api/dailyReports.test.ts --maxWorkers=2` 通过
- [x] 5.2 `DailyReportTopicSection.vue` 子卡头部渲染方向徽标（`bucket_title` 非空时）、标题渲染条件改为「组内 >1 section 或存在 bucket_title」、新增「重新分桶」动作（`lane_tier` 为 `watch_*` 时隐藏）与确认弹层；验证：`pnpm test:unit app/features/tags/components/daily-report/DailyReportTopicSection.test.ts --maxWorkers=2`（徽标渲染、条件标题、按钮可见性/隐藏、loading 置灰）
- [x] 5.3 `dailyReportMagazine.ts:groupSectionsByTopic` 组内按 `bucket_index` 升序稳定排序；验证：`pnpm test:unit app/features/tags/components/daily-report/dailyReportMagazine.test.ts --maxWorkers=2`
- [x] 5.4 `useDailyReportReader` 增加重分桶动作：成功局部替换该组 sections、失败回 idle 且原数据不变；验证：`pnpm test:unit app/features/tags/composables/useDailyReportReader.test.ts --maxWorkers=2`

## 6. 泳道动态排序口径

- [x] 6.1 `board-lane-dynamics` 聚合把排序键改为「近 14 天有锚定 section 的天数」（`len(a.dates)`），二次排序按最近命中日期、topic id；响应字段 `section_count_14d` 保留原值供展示；验证：`go test ./internal/topicgraph/repository -run TestLaneDynamicsSortsByDistinctDays`
- [x] 6.2 现有 `TestGetBoardLaneDynamics_FullMatrix` 的排序断言按新口径更新（活跃排序/同日多条/沉寂/候选/watch 标识同处覆盖）；验证：`go test ./internal/topicgraph/repository -run TestGetBoardLaneDynamics` 通过

## 7. 审查与处置

- [x] 7.1 派子线程 review 本 change（制品 + 实现 diff），意见按 High/Medium/Low 逐条处置并在 `review.md` 建账（格式：`### R-<N> <日期>【High|Medium|Low】<标题>` + 状态行）；验证：`bash scripts/harness/review-trace.sh openspec/changes/add-intra-lane-theme-bucketing` 退出码 0
- [x] 7.2 逐条复核首轮审查报告（1 H + 6 M + 3 L）的修复是否落地（promptVersion 6.0、rebucket 锁、牵连面、阈值口径、scenario 映射、review 任务行、watch 组拒绝、空组、Impact 落点、兜底链、useConfirm）；验证：人工核对 `review.md` 中每条均有「已修/不修+理由」

## 8. 测试

- [x] 8.1 后端影响包：`cd backend-go && go test ./internal/topicgraph/service ./internal/topicgraph/repository ./internal/topicgraph/handler` → PASS
- [x] 8.2 前端受影响用例：`cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportTopicSection.test.ts app/features/tags/components/daily-report/dailyReportMagazine.test.ts app/features/tags/composables/useDailyReportReader.test.ts app/api/dailyReports.test.ts --maxWorkers=2` → PASS
- [x] 8.3 `test-cases.md` 五问句与白盒附加表全部有结论（含 `⓪ 改契约了吗` 的继承与调整逐行处置）；验证：人工核对无空白项

## 9. 文档

<!-- doc-impact: flow, api, database, configuration -->

- [x] 9.1 `docs/reference/flow/daily-report.md`：链路设计补「泳道内主题分桶」（阈值/两阶段/校验/降级/rebucket），业务约束与不变量补红线（不得跨聚类、不得丢线索、失败退回单 section、兄弟不合并、watch 组不可重分桶）
- [x] 9.2 `docs/reference/flow/topic-graph.md`：补「同一话题同日多 section」的命中计数按日去重与质心按 section 计入
- [x] 9.3 `docs/reference/api/daily-reports.md`：新增 `POST /api/daily-reports/:id/rebucket` 契约（409/400/200+buckets=1 三态）与 `bucket_title` / `bucket_index` 字段
- [x] 9.4 `docs/reference/database/tables/`：补两列定义与默认值（section 所属表文档）
- [x] 9.5 `docs/reference/configuration.md`：登记 3 个新配置项与默认值
- [ ] 9.6 归档后按 `开发执行规范.md` §12.2 在 `flow/daily-report.md`「变更溯源」表补本 change 链接

## 10. 验证

- [x] 10.1 `cd backend-go && golangci-lint run ./...` → PASS
- [x] 10.2 `cd backend-go && go vet ./...` → PASS
- [x] 10.3 `cd backend-go && go test ./internal/topicgraph/service ./internal/topicgraph/repository ./internal/topicgraph/handler` → PASS
- [x] 10.4 `cd backend-go && go build ./...` → PASS
- [x] 10.5 `cd front && pnpm lint` → PASS
- [ ] 10.6 `cd front && pnpm exec nuxi typecheck` → PASS（2026-10-24 复核：本 change 触碰文件零 TS 错误；剩余 7 处均在 `app/features/tags/components/SignalReportView.test.ts`，属并发会话 `rework-workshop-into-radar-and-desk` 的进行中文件，已登记 test-patrol 台账，待该会话修复后复跑）
- [x] 10.7 `cd front && pnpm test:unit app/features/tags/components/daily-report/DailyReportTopicSection.test.ts app/features/tags/components/daily-report/dailyReportMagazine.test.ts app/features/tags/composables/useDailyReportReader.test.ts app/api/dailyReports.test.ts --maxWorkers=2` → PASS
- [x] 10.8 `cd front && pnpm build` → PASS
- [x] 10.9 真库抽查（只读）：`select report_id, cluster_index, count(*) from daily_report_sections group by 1,2 having count(*)>1` → 上线后新日报至少出现 1 个 `count(*)>1` 的组，且该组 `bucket_title` 全部非空（2026-10-24 21:40 完成：report 1234 board 3030 出现 6 桶组、`bucket_title` 全非空；thread 守恒 31/31、桶间 tag 零重叠、兄弟行继承一致、embedding 非空、`article_count`=去重并集；同批 3 份未拆分报告规模均未达 6 条阈值。详见 `ui-verification/report.md` A-5）
- [x] 10.10 `bash scripts/harness/scenario-trace.sh openspec/changes/add-intra-lane-theme-bucketing` → 退出码 0
- [x] 10.11 `bash scripts/harness/check-tasks-tail.sh openspec/changes/add-intra-lane-theme-bucketing` → 退出码 0
- [x] 10.12 `bash scripts/harness/doc-impact.sh verify openspec/changes/add-intra-lane-theme-bucketing` → 零失败
- [x] 10.13 `bash scripts/harness/check-standards.sh --change add-intra-lane-theme-bucketing` → A-D/F 段零失败

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 创建日报记录 | `backend-go/internal/topicgraph/repository/daily_report_topic_integration_test.go` |
| 日报记录唯一性 | `backend-go/internal/topicgraph/repository/daily_report_repository_concurrency_test.go` |
| 并发落库同板同日不炸唯一约束 | `backend-go/internal/topicgraph/repository/daily_report_repository_concurrency_test.go` |
| 日报关联昨日报告 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| 线程存储在独立表中 | `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` |
| 兄弟桶共享聚类序号 | `backend-go/internal/topicgraph/repository/daily_report_bucket_columns_test.go` |
| 未分桶 section 取默认桶序号 | `backend-go/internal/topicgraph/repository/daily_report_bucket_columns_test.go` |
| 完整流水线执行 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_bucket_test.go` |
| 物化失败不阻断流水线 | `backend-go/internal/topicgraph/service/daily_report_watch_integration_test.go` |
| 生成失败 | `backend-go/internal/topicgraph/service/daily_report_orchestrator_test.go` |
| 分桶失败不阻断流水线 | `backend-go/internal/topicgraph/service/daily_report_bucket_test.go` |
| section 记录 lane 来源 | `backend-go/internal/topicgraph/service/daily_report_cluster_test.go` |
| 物化 section 记录物化来源 | `backend-go/internal/topicgraph/service/daily_report_watch_test.go` |
| 分桶兄弟继承归属与来源 | `backend-go/internal/topicgraph/service/daily_report_bucket_test.go` |
| L2 prompt 不含历史 thread 文案 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L2 prompt 保留话题框架信号以区分近似话题 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| promptVersion 升级 | `backend-go/internal/topicgraph/service/daily_report_llm_test.go` |
| 域型宽聚类被拆成多个主题 | `backend-go/internal/topicgraph/service/daily_report_bucket_test.go` |
| 未达阈值不触发分桶 | `backend-go/internal/topicgraph/service/daily_report_bucket_test.go` |
| 归属漏掉线索被修复归位 | `backend-go/internal/topicgraph/service/daily_report_bucket_repair_test.go` |
| 单例组被合并 | `backend-go/internal/topicgraph/service/daily_report_bucket_repair_test.go` |
| 全组收敛为一组时回退 | `backend-go/internal/topicgraph/service/daily_report_bucket_repair_test.go` |
| 分桶产物不参与同日合并 | `backend-go/internal/topicgraph/service/daily_report_merge_test.go` |
| 组数指令不被模型遵守时收敛 | `backend-go/internal/topicgraph/service/daily_report_bucket_test.go` |
| 重新分桶替换一组 section | `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go` |
| 重新分桶保留关键事件注记 | `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go` |
| 拆不动时返回单桶且不改数据 | `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go` |
| 失败整体回滚 | `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go` |
| 拒绝非完成态日报 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 拒绝 watch 物化组 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 未达阈值不改数据 | `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go` |
| 与同日重跑并发时串行化 | `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go` |
| watch 命中与页边注跟随重派生 | `backend-go/internal/topicgraph/service/daily_report_rebucket_test.go` |
| L1 直挂命中（质心强匹配） | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| candidate 近距离降级 L2（观察期门禁） | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| 门禁开关关闭回退旧行为 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| vacuum active 不直挂（既有规则保持） | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L2 弱区 LLM 裁决留/换/新 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L2 裁决中观察中话题从严判断 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L2 keep 显式指定候选集内话题尊重归属 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L2 keep 集外/空 target 维持最近候选 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L3 新建候选（质心挂不上） | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| L2 LLM 指定 topic 超出候选集降级 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| embedding 为空标记未匹配 | `backend-go/internal/topicgraph/service/daily_report_lane_test.go` |
| 手动改写归属 | `backend-go/internal/topicgraph/repository/daily_report_assignment_test.go` |
| 同日多条 section 只记一次命中 | `backend-go/internal/topicgraph/repository/daily_report_assignment_test.go` |
| 质心按 section 逐条计入 | `backend-go/internal/topicgraph/repository/daily_report_centroid_test.go` |
| 活跃排序 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 同日多条只计一天 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| watch 标识 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 沉寂不展示 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
| 候选不进主区 | `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` |
