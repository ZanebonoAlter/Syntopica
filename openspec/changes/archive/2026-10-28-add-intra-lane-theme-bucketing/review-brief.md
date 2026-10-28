# 审查 brief — add-intra-lane-theme-bucketing

> 只读审查。禁止修改任何文件。产出中文审查报告，严格按本文件末尾「输出要求」三段式。

## 背景（一句话）

域型泳道（如 `AGENT编程工具相关`）把当天全部线索塞进一条 section（实例：18 tags / 17 threads 平铺），本 change 在 lane 归属不变的前提下，对**单个聚类内部**做「主题方向分桶」，拆成 2-6 条同话题 section；并提供手动重新分桶。

## 必读文件（按序）

1. `openspec/changes/add-intra-lane-theme-bucketing/proposal.md`
2. `openspec/changes/add-intra-lane-theme-bucketing/design.md`（D1–D10）
3. `openspec/changes/add-intra-lane-theme-bucketing/specs/daily-report-system/spec.md`
4. `openspec/changes/add-intra-lane-theme-bucketing/specs/persistent-topic/spec.md`
5. `openspec/changes/add-intra-lane-theme-bucketing/specs/board-lane-dynamics/spec.md`
6. `openspec/changes/add-intra-lane-theme-bucketing/ui-design.md`
7. `openspec/changes/add-intra-lane-theme-bucketing/tasks.md`
8. `openspec/changes/add-intra-lane-theme-bucketing/test-cases.md`
9. `docs/research/lane-intra-bucketing/explore-findings.md`（取证与实验数据）

## 交叉核对用（只读，抽查即可）

- `backend-go/internal/topicgraph/service/daily_report_lane.go`（`assembleLaneGroups`）
- `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（`GenerateDailyReport` 步骤顺序）
- `backend-go/internal/topicgraph/service/daily_report_merge.go`（`sameAnchorClass`）
- `backend-go/internal/topicgraph/repository/daily_report_assignment.go`（`planLifecycle` / `assignAndUpdateTopics`）
- `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go`（`SectionCount14d` 排序）
- `docs/reference/flow/daily-report.md` §业务约束与不变量、`docs/reference/flow/topic-graph.md` §业务约束与不变量
- `docs/reference/开发执行规范.md` §11（归档门禁 / tasks 尾节要求）

## 审查维度（逐条给结论，不要泛泛而谈）

### A. 契约完整性
1. delta specs 是否覆盖 proposal「What Changes」的每一项？有无宣称改了但 spec 没写、或 spec 写了但 proposal 没提？
2. MODIFIED 的三个 requirement 是否有语义倒退（原文被无意削弱/删除）？重点看 `persistent-topic` 的 11 条既有 Scenario 与 `daily-report-system` 的流水线步骤文案。
3. 新 Scenario 是否都能被测试或人工验证？有没有「写了无法验证」的措辞。

### B. 与既有红线的一致性
4. 是否触碰 `topic-graph.md` 不变量：约束 1（planLifecycle 不得改状态）、约束 5（ClusterTags 与归属共享 `ListAnchorableTopicsByBoard`）、约束 6（快照字段同事务、不回填）。
5. 是否触碰 `daily-report.md` 不变量：section embedding 取内容化文本而非标题、L2 prompt 历史隔离、watch 物化 section 不参与合并、promptVersion 语义。
6. 分桶的 prompt 是否可能重新引入「历史叙事文案渗透」或「复述话题名」问题？

### C. 重新分桶（最高风险面）
7. `POST /api/daily-reports/:id/rebucket` 的整组替换是否遗漏牵连面？请自行列出受 section id 变化影响的表/接口，并与 spec 对照：`topic_key_event_markers`、`daily_report_section_relations`、`daily_report_threads`、section-timeline、关键事件推荐、泳道趋势/`topic_lane_snapshots`、前端缓存与深链（`#report-section-<id>`）。
8. 并发与幂等：日报重跑（`SaveReport` 整份删写）与 rebucket 并发时会发生什么？spec 只写了 409，是否足够？
9. 「拆不动返回 buckets=1 且不改数据」与「失败回滚」的边界是否清晰（哪些情况算失败、哪些算拆不动）？

### D. 阈值与质量
10. 阈值 6 threads / 10 tags 与 `k = clamp(round(lines/2.5),2,6)` 是硬编码常量还是配置？叙事型板块（1974，max 5 tags）是否确定不会误伤？
11. 校验器三条（覆盖 100% / 每组 ≥2 / 组数 ≤6）是否足以防止「分桶把日报变碎」？有没有需要补充的约束（比如单组线索上限、桶内 tag 数上限）？

### E. 测试与任务账
12. `test-cases.md` 五问句是否齐全（契约继承与调整 / 节拍覆盖 / 变体走查 / 层选择 / 效果核对）？白盒分支表与实现是否有对不上的分支？
13. `tasks.md` 的 Scenario→测试文件映射指向的是**尚不存在**的测试文件（实施时新建）。这对 `scenario-trace.sh` 归档对账是否合规？请在 `docs/reference/开发执行规范.md` §11.1 条件 4 里核实，并给出结论（合规 / 不合规 + 依据）。
14. 任务粒度是否可在一会话完成；每条任务是否有可执行验证。

### F. UI 声明是否站得住
15. `ui-impact: minor` 是否成立？「桶徽标 + 子卡标题条件 + 重新分桶按钮 + sm 确认弹层」是否已构成 major（新交互模式）？请给出判断依据（对照 `docs/reference/开发执行规范.md` 的 ui-impact 判据与 `ui-design.md` minor 四节要求）。

## 输出要求（三段式，中文）

### 一、总评
- 一句话结论（可通过 / 需修改后再实现 / 有阻断项）
- 3-5 条「这个 change 最站得住的地方」

### 二、问题清单
每条格式：
```
- [H|M|L] <标题>
  - 位置：<文件:行 或 章节名>
  - 问题：<具体描述，避免"建议完善"这类空话>
  - 依据：<引用 spec / 约束 / 代码事实>
  - 建议：<可执行的修改动作>
```
严重度：H = 会导致数据丢失/契约冲突/无法实现；M = 会返工或留下隐患；L = 打磨项。
按 H→M→L 排序，H 必须给全。

### 三、核实记录
- 逐条列出你实际读过/跑过的文件与命令（`read`/`grep` 的对象），以及「查了但没问题」的核对点（至少 5 条），便于复核。

---

# 第二轮：实现 diff 审查（2026-10-24）

> 首轮制品审查（R-1..R-11）已全部处置。本轮**新增**要求：核对实现是否真的落地了 spec，而不是只看制品。

## 实现落点（新增/改动文件）

后端新增：

- `backend-go/internal/topicgraph/service/daily_report_bucket.go`（两阶段分桶 + `planBucketCount` + `repairBuckets` + `BucketCluster`）
- `backend-go/internal/topicgraph/service/daily_report_rebucket.go`（`RebucketSectionGroup`：锁 → 重跑 → 计划 → 事务应用 → 质心刷新）
- `backend-go/internal/topicgraph/repository/daily_report_rebucket.go`（`LockDailyReportSave` / `LoadRebucketGroup` / `ApplyRebucketGroup`：组内 section 复用-补建-删除，thread 迁移，markers/hits/annotations 重派生，关系重建）
- `backend-go/internal/platform/database/postgres_migrations.go`（`intraLaneBucketColumnsMigration`，Version `20261024_0001`）

后端改动：

- `repository/daily_report_models.go`（`DailyReportSection.BucketTitle/BucketIndex`）
- `repository/daily_report_topic_repository.go`（3 个新配置项 + 读取）
- `service/daily_report_orchestrator.go`（`resolveSectionPlans` Step5.5 + 按桶组装 section/threadBatches）
- `service/daily_report_merge.go`（`ClusterIndex` 相同即拒绝合并）
- `service/daily_report_llm.go`（`promptVersion` 6.0）
- `repository/lane_snapshot_repository.go`（排序改按有锚定天数）
- `handler/daily_report_handler.go`（`POST /daily-reports/:id/rebucket`）

前端改动：`front/app/api/dailyReports.ts`、`app/features/tags/components/daily-report/DailyReportTopicSection.vue`、`dailyReportMagazine.ts`、`app/features/tags/composables/useDailyReportReader.ts`、`app/features/tags/components/BoardDailyReportTimeline.vue`

测试：`repository/daily_report_bucket_columns_test.go`、`service/daily_report_bucket_test.go`、`daily_report_bucket_repair_test.go`、`daily_report_orchestrator_bucket_test.go`、`daily_report_rebucket_test.go`、`repository/lane_snapshot_repository_test.go`（新增排序用例）、`handler/daily_report_handler_test.go`（rebucket 门禁）、前端 4 个 test 文件新增用例。

文档：`docs/reference/flow/daily-report.md`（约束 32/33）、`flow/topic-graph.md`（约束 11）、`api/daily-reports.md`、`database/tables/daily-report-watch.md`、`configuration.md`

## 本轮重点（实现层审计，逐条给结论）

1. `resolveSectionPlans` 与 spec「分桶在 thread 之后 / embedding 之前」是否一致？失败是否真的只降级不阻断？
2. `BucketCluster` 的产出字段与 spec 产出段逐条对照：`cluster_index` 共享、`bucket_index`、`bucket_title`、`cluster_tag_ids` 并集、`cluster_label` 兜底链（桶内 thread → 泳道 label，不借用其他桶）。
3. `repairBuckets` 五条规则顺序是否正确？「未覆盖并入最小组」是否与 spec「语义最近组」有偏差，偏差点是否可接受/需在 spec 补措辞？
4. `ApplyRebucketGroup` 的段落：锁内复核、thread 迁移行数校验、markers/hits/annotations 重派生顺序（是否在删多余 section 之前）、`RebucketApplySpec` 是否存在可绕过校验的调用路径。
5. 派生字段重算口径（`article_count` 用桶内 thread 的 distinct 文章、`best_tier/avg_score/quality_breakdown` 取自报告期 `quality_breakdown` 快照）是否与 spec「重算」一致？有无字段被漏掉（对照 spec 的派生字段清单）。
6. 并发：`LockDailyReportSave` 在 SQLite 下跳过（仅 handler 单测）是否会在生产造成缺口？`LoadRebucketGroupMeta` 在锁外的读是否会误判？
7. 前端：按钮可见性（`lane_tier` 白名单 vs spec「watch_* 隐藏」）、深链回退（`sectionAliases`）、局部替换是否正确；`bucket_index` 排序是否稳定。
8. 测试是否覆盖 spec 全部新增 Scenario（对照 `specs/*/spec.md` 的 Scenario 列表逐条点名缺口）。
9. 文档红线（约束 32/33）与实现是否一一对应，有无「文档比实现多写」的地方。
