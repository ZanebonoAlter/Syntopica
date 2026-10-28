# Review 记录 — add-intra-lane-theme-bucketing

> 审查方：子线程 `change-review-glm`（zai-coding-cn/glm-5.3，thinking max），只读审查，brief 见 `review-brief.md`。
> 处置：全部条目在本轮修复，修复点已回写 proposal / design / specs / tasks / ui-design / test-cases。

### R-1 2026-10-24【High】promptVersion 升版目标与代码现状冲突

- **状态**：已修复（design D10 与 tasks 3.5 改为由 "5.0" 升至 "6.0"；`daily_report_llm_test.go` 断言同步；delta 新增 MODIFIED「日报聚类裁决 prompt 历史隔离」，把 main spec 里漂移的 "4.0" Scenario 修正为 "6.0" 并写明版本链）
- 依据：`daily_report_llm.go:16` 已是 `"5.0"`（2026-08-29 `daily-report-section-title-decouple` 占用）。

### R-2 2026-10-24【Medium】rebucket 与同日重跑并发无序列化设计

- **状态**：已修复（新增 design D11；spec「重新分桶 API」补 SHALL 取与 SaveReport 同键的 `pg_advisory_xact_lock`并在锁内复核 status，补 Scenario「与同日重跑并发时串行化」；tasks 4.1/4.4 落任务与验证）
- 依据：`daily_report_repository.go:219` SaveReport 已用 `dailyReportSaveXactKey` 加锁。

### R-3 2026-10-24【Medium】rebucket 牵连面清单不全

- **状态**：已修复（spec 补 `topic_watch_hits.section_id` 同步迁移/重建、`report_annotations.section_id` 按 thread 重派生、派生字段 `best_tier`/`avg_score`/`quality_breakdown`/`topic_status_at_report` 重算或沿用、跨天入边双向重建；ui-design 受影响状态补「深链失效」回退；tasks 4.2/4.6 落任务）
- 依据：`TopicWatchHit.SectionID` NOT NULL、`ReportAnnotation.SectionID` NOT NULL、`DailyReportTopicSection.vue:268` 深链。

### R-4 2026-10-24【Medium】「校验后仍为 1 组」同时出现在失败与成功例外里

- **状态**：已修复（spec 把「仍为 1 组」移出失败列表，明确为合法退化路径 200 + `buckets=1` 不写库；并写明 rebucket 同样执行触发阈值判断，未达阈值直接返回不调 LLM）
- 依据：原文自相矛盾；`test-cases.md` 白盒分支表按阈值处理。

### R-5 2026-10-24【Medium】scenario-trace 归档对账现状必红

- **状态**：已修复（映射表补 18 行继承 Scenario、修正「活跃排序」标题逐字匹配、`handler/lane_dynamics_test.go` 改指真实文件 `repository/lane_snapshot_repository_test.go`；「日报关联昨日报告」映射到 `service/daily_report_orchestrator_test.go` 并在 tasks 3.1 补该断言）
- 结果：`scenario-trace.sh` 现仅剩 18 条「实施时新建的测试文件尚不存在」，属 §11.1 条件 4 的归档时校验范围。

### R-6 2026-10-24【Medium】complex 档缺 review 任务行

- **状态**：已修复（tasks 新增「## 7. 审查与处置」两组任务，含 `review.md` 建账与首轮意见复核；`check-tasks-tail.sh` 已复跑退出码 0）

### R-7 2026-10-24【Medium】rebucket 未排除 watch 物化 section

- **状态**：已修复（spec 明确 `lane_tier` 为 `watch_*` 返回 400 并补 Scenario；tasks 4.1/4.5 落验证；前端按钮同步隐藏）
- 依据：物化 section 由关注轨自管，重分桶会与 watch-materialized-topic 语义冲突。

### R-8 2026-10-24【Low】校验器未覆盖「空组」

- **状态**：已修复（spec 校验与修复段补「空组直接丢弃、不产出 section」；test-cases 白盒补 B15）

### R-9 2026-10-24【Low】proposal Impact 的 rebuild_window 落点错误

- **状态**：已修复（改为「新增 rebucket 组级重建服务」，删掉 `daily_report_rebuild_window.go` 的错误指向）
- 依据：该文件只含重建窗口守卫，rebucket 基于已存 threads，无需窗口守卫。

### R-10 2026-10-24【Low】bucket 的 cluster_label 兜底链未定义

- **状态**：已修复（spec 产出段补「标题缺失时按该桶内线索兜底：桶内首条 thread 标题 → 泳道 label，不得借用其他桶内容」；test-cases 白盒补 B16）

### R-11 2026-10-24【Low】二次确认可复用全局 useConfirm

- **状态**：已修复（ui-design「复用组件与布局模式」改为优先复用 `front/app/composables/useConfirm.ts`）
- 依据：2026-09-23 change 已沉淀该封装。

---

> 第二轮：子线程 `change-review-glm`（zai-coding-cn/glm-5.3）实现 diff 审查，brief 见 `review-brief.md` 第二轮段。结论「需修改后再实现（无阻断项）」，4 M + 6 L 全部处置如下。

### R-12 2026-10-24【Medium】rebucket 响应缺 persistent_topic brief，前端兄弟桶会散成独立卡片

- **状态**：已修复（repository `AttachTopicBriefsToSections` + `ApplyRebucketGroup` 返回前调用 `attachBriefsToSections`；`useDailyReportReader.test.ts` 新增「替换后兄弟桶仍归入同一话题分组」用例断言 `groupSectionsByTopic` 仍以 topic 7 成组）
- 依据：GET 详情走 `AttachTopicBriefsToReport`，rebucket 原只 Preload Threads。

### R-13 2026-10-24【Medium】校验器「语义最近组」与实现「行数最小组」相反

- **状态**：已修复（实现改为 **tag 集合 Jaccard 重叠**作为无 embedding 时的确定性语义代理：未覆盖线索/单例组并入重叠度最高组，并列或无重叠才退回行数最小组；`TestRepairBuckets` 新增「语义最近优先于更小组」用例；delta spec 校验段与 `flow/daily-report.md` 约束 32 同步写明代理口径）
- 依据：spec 原文「并入与其语义最近的组」；无 embedding 时需要确定性代理。

### R-14 2026-10-24【Medium】兄弟桶命中计数 / 质心逐条计入 / rebucket 收敛为 1 组缺直接用例

- **状态**：已修复（新增 `TestPlanLifecycle_SameDayMultipleSectionsCountOnce`、`TestComputeTopicCentroid_CountsEachSiblingSection`、`TestRebucketConvergesToOneBucket`）；「分桶失败不阻断流水线」在日报级无独立断言一项不改代码：`resolveSectionPlans` 无 error 返回路径（分桶错误在 planner 内 warn 并落单 plan），orchestrator 结构上无法因此把 status 置 failed；已由 `TestOrchestratorBucketStep`（失败仍返 1 plan）覆盖。
- 依据：三处映射文件此前无 bucket 相关用例；`scenario-trace.sh` 只校验文件存在。

### R-15 2026-10-24【Medium】rebucket 的 `article_count` 口径未钉死

- **状态**：已修复（delta spec 重新分桶 API 段与 `api/daily-reports.md` 写明：按桶内线索 `related_article_ids` 去重并集重算，并说明与生成期「Σ tag 文章数」的差异来自「重分桶只持有 thread 事实、不重新选文」，属展示计数；`best_tier`/`avg_score`/`quality_breakdown` 明确取自报告期冻结快照按桶内 tag 子集重算）

### R-16 2026-10-24【Low】rebucket `buckets=1` 响应的 `threads` 为 null

- **状态**：已修复（`groupSectionsWithThreads` 在阈值/收敛两条 no-op 路径上挂上 threads 并归一 `[]`；新增 `TestRebucketNoOpResponseCarriesThreads`）

### R-17 2026-10-24【Low】分桶 prompt 的泳道名为空 / rebucket 用旧标题当泳道名

- **状态**：已修复（`resolveSectionPlans` 增加 `topicLabels` 入参，L1/L2 聚类用 `MatchedTopicID` 的 topic label 填充泳道名并作为标题兜底；rebucket 侧改取所属 topic label（`topicLabel`），不再复用旧 `cluster_label`）

### R-18 2026-10-24【Low】无 tag 桶回退整簇 tag 集超出 spec「并集」措辞

- **状态**：已修复（spec 产出段补「桶内线索均无 tag id 时 SHALL 回退为来源聚类的 tag 集（避免空壳 section）」，与 `TestBucketClusterTagUnionAndFallback` 对齐）

### R-19 2026-10-24【Low】「重新分桶」按钮白名单过严（隐藏了历史 section）

- **状态**：已修复（改为「非 `watch_*` 即开放」，与服务端门禁同口径；`ui-design.md` 同步修改；watch 组隐藏用例保留）

### R-20 2026-10-24【Low】test-cases 排序用例落点指向不存在的 `lane_dynamics_sort_test.go`

- **状态**：已修复（主链路 19/20 行改指 `lane_snapshot_repository_test.go` 的 `TestLaneDynamicsSortsByDistinctDays`）

### R-21 2026-10-24【Low】`TestRebucketSerializedWithSaveReport` 首条断言恒真

- **状态**：已修复（删除同表达式自比；保留「不同 board 不同键」与真实阻塞/释放断言）

### R-22 2026-10-24【Low】rebucket 长事务与 `fit_distance` 陈旧未记录

- **状态**：已修复（design「Risks / Trade-offs」补两条：长事务是 D11 串行化的直接代价 + 后续「锁内快照/锁外规划/新事务复核」演化路径；`fit_distance` 记为已知陈旧信号，spec 未列入重算清单，下次同日重跑自然刷新）

### R-23 2026-10-24【Low】tasks.md 勾选与实现不同步

- **状态**：已修复（§1-§9 全部按实际落地勾选，10.9 真库抽查保留未勾——需上线生成新日报后执行；10.10-10.13 门禁复跑通过后勾选）

### R-24 2026-10-24【Medium】方向徽标 `max-width: 12ch` 把 6/6 方向名整条截断

- **状态**：已修复（`max-width` 改为 `14em` + `min-width: 0`；新增源码守卫用例「徽标宽度上限用 em（汉字宽）而非 ch」；ui-design「Responsive/A11y」行改写为 em 口径并写明理由；真机两档视口复测 `clientWidth == scrollWidth`、0 截断，见 `ui-verification/report.md` A-4）
- 依据：`ch` 是「0」的数字宽（≈汉字一半），`12ch ≈ 83px` 只放得下 6 个汉字，而 prompt 允许方向名 12 字；验收首测实测 6 个徽标 `scrollWidth 102–123 > clientWidth 83`。这是「组件单测看不见、只有真机量测才暴露」的布局缺陷，已进验收证据链。
