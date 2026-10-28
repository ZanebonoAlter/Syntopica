# Test Cases — add-intra-lane-theme-bucketing

> 复杂度：complex（分组器有阶段分支 + 校验修复 + 降级 + 事务回滚）。故事主线：**一条域型泳道的当天内容从「一坨」变成「2-6 个主题方向」**，且任何异常都退回现状、不丢事件。真实数据锚点见 `docs/research/lane-intra-bucketing/explore-findings.md`（section #7149：18 tags / 17 threads）。

## 用户故事

读者打开 2026-10-23 板块 3030 的日报，看到泳道「AGENT编程工具相关」下有 5~6 张子卡：小游戏/独立开发、Agent 工具与插件、Skill 与规范生态、编码 Agent 安全、推理基础设施——每张卡带方向名与当日章节标题，点开是 2~4 条事件线索。同一天里如果分桶不合意，用户点「重新分桶」，子卡重排而事件线索（含已确认的关键事件星标）一条不少。

## 主链路

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | 聚类线索数 17、tag 18 进入流水线 | 域型宽聚类被拆成多个主题 | 触发分桶，产出 2-6 条同 `cluster_index` section | service 单测（fake LLM） | `daily_report_bucket_test.go` |
| 2 | 聚类线索数 4、tag 5 | 未达阈值不触发分桶 | 不调 LLM，单 section（bucket_index=0/title NULL） | service 单测 | `daily_report_bucket_test.go` |
| 3 | 代码算 k=4，模型返回 6 个方向名 | 组数指令不被模型遵守时收敛 | 确定性合并到 4 组后再归属 | service 单测 | `daily_report_bucket_test.go` |
| 4 | 归属阶段漏掉 2 条线索 | 归属漏掉线索被修复归位 | 补入最近组，覆盖率 100%，不丢线索 | service 单测 | `daily_report_bucket_repair_test.go` |
| 5 | 归属结果含 1 条线索的组 | 单例组被合并 | 合并进最近组，最终每组 ≥2 | service 单测 | `daily_report_bucket_repair_test.go` |
| 6 | 修复后只剩 1 组 | 全组收敛为一组时回退 | 回退单 section，字段为默认值 | service 单测 | `daily_report_bucket_repair_test.go` |
| 7 | 分桶 LLM 超时 / 返回非法 JSON | 分桶失败不阻断流水线 | 记 warn，单 section，日报 status=done | service 单测（fake LLM） | `daily_report_bucket_test.go` |
| 8 | 拆成 3 条兄弟 section 后跑同日合并 | 分桶产物不参与同日合并 | 三条都在，不被 union 成一坨 | service 单测 | `daily_report_merge_test.go` |
| 9 | 3 条兄弟各自算 embedding | 分桶兄弟继承归属与来源 | lane_tier/topic/confidence 继承，distance 各自算 | service 单测 | `daily_report_bucket_test.go` |
| 10 | 日报落库 | 兄弟桶共享聚类序号 | 共享 cluster_index，bucket_index 0/1/2 | repository 集成（PG） | `daily_report_bucket_columns_test.go` |
| 11 | 未分桶聚类落库 | 未分桶 section 取默认桶序号 | bucket_index=0、bucket_title NULL | repository 集成（PG） | `daily_report_bucket_columns_test.go` |
| 12 | 生命周期更新 | 同日多条 section 只记一次命中 | hit_count +1、consecutive_hits +1 | repository 集成（PG） | `daily_report_assignment_test.go` |
| 13 | 质心重算 | 质心按 section 逐条计入 | 3 条兄弟各自进窗口 | repository 集成（PG） | `daily_report_centroid_test.go` |
| 14 | 调 rebucket，组内 1 条 18-tag section | 重新分桶替换一组 section | 返回 2-6 条同组 section，thread.section_id 迁移 | service（PG） | `daily_report_rebucket_test.go` |
| 15 | 组内某 thread 有 confirmed 注记 | 重新分桶保留关键事件注记 | 注记仍在，source_thread_id 不变，source_section_id 重指 | service（PG） | `daily_report_rebucket_test.go` |
| 16 | 内容确实只有一个方向 | 拆不动时返回单桶且不改数据 | 200 + buckets=1，库内无改动 | service（PG） | `daily_report_rebucket_test.go` |
| 17 | 归属 LLM 失败 | 失败整体回滚 | section 与 thread.section_id 保持原状 | service（PG） | `daily_report_rebucket_test.go` |
| 18 | 报告 status=generating | 拒绝非完成态日报 | 409，无写入 | handler 单测 | `daily_report_handler_test.go` |
| 19 | 泳道 A 6 天（某天 3 条）/ 泳道 B 3 天 | 活跃排序按天计 | A 在前且不因 3 条加权 | repository（PG） | `lane_snapshot_repository_test.go`（`TestLaneDynamicsSortsByDistinctDays`） |
| 20 | 泳道 A 1 天 4 条 / 泳道 B 2 天 1 条 | 同日多条只计一天 | B 在前 | repository（PG） | `lane_snapshot_repository_test.go`（`TestLaneDynamicsSortsByDistinctDays`） |
| 21 | 打开阅读视图 | UI 契约：子卡渲染桶徽标 | bucket_title 非空才渲染徽标；>1 子卡或带桶名时显示标题 | Vitest | `DailyReportTopicSection.test.ts` |
| 22 | 点「重新分桶」→ 成功 | UI 契约：重分桶成功后局部替换 | 组内 sections 替换，toast 出现，滚动不跳走 | Vitest | `useDailyReportReader.test.ts` |
| 23 | 组内 sections 乱序 | UI 契约：组内按 bucket_index 排序 | 按 bucket_index 升序，缺省保持稳定 | Vitest | `dailyReportMagazine.test.ts` |
| 24 | 目标组仅 4 条线索 | 未达阈值不改数据 | 200 + buckets=1，不调 LLM、不写库 | service（PG） | `daily_report_rebucket_test.go` |
| 25 | 目标组 `lane_tier=watch_sentence` | 拒绝 watch 物化组 | 400，无写入 | handler 单测 | `daily_report_handler_test.go` |
| 26 | 与同日重跑并发 | 与同日重跑并发时串行化 | 同键 advisory lock 串行，无幽灵 section | service（PG） | `daily_report_rebucket_test.go` |
| 27 | 组内有 watch 命中与页边注 | watch 命中与页边注跟随重派生 | 两表 section_id 重指，无悬空 | service（PG） | `daily_report_rebucket_test.go` |
| 28 | 端到端打开某板块日报并重分桶 | UI 验收 | 子卡重排 + toast，两档视口视觉正常 | agent-browser | `ui-verification/report.md` |

## 变体走查（五组固定清单）

- **输入**：空串/纯空白 → thread 标题为空时以摘要参与分组，摘要也为空则不计数（不计入 k 计算的分母口径 = 线索条数，空摘要线索仍可归属）；纯分隔符/特殊字符 → prompt 原样转义，不参与结构；单 token（只有 1 条线索）→ 阈值未达，不分桶；大小写/超长标题 → 标题按现有 56 字截断展示，分组用完整标题；`bucket_title` 超 12 字 → 后端按 prompt 约束截断到 12 字（超长仍照存，前端 ellipsis）。
- **前置**：空集（聚类无 thread）→ 不触发；单元素 → 不触发；重复 tag（同 tag 出现在多条 thread）→ `cluster_tag_ids` 取并集去重，不重复计数；越界引用（LLM 返回不存在的 line id）→ 视为未覆盖，走修复归位；部分满足（只有部分 thread 有 tag_ids）→ 并集按存在的取。
- **时间窗口**：k 计算与分桶只用**当次聚类**的线索，不跨天；同日重跑 → 旧 sections 整体替换（`SaveReport` 既有语义），分桶结果重算；跨窗口（历史日报）→ 不参与，靠「重新分桶」按需触发；`NormalizeReportDate` 归一化沿用既有逻辑。
- **幂等**：分桶对相同输入同 temp 下重复执行 → 归属逐条一致（实测 temp=0 两次一致），组名可能微差（已用「重新分桶」兜底；组名不参与任何跨天统计）；重试部分失败 → 单事务回滚，不留半成品；并发 → 重新分桶与日报重跑并发时，以报告状态闸与事务为准（不声称线程安全，冲突时报错回滚）。
- **可用性（UI 必检前四）**：误输入 → 「重新分桶」需二次确认（sm 弹层，文案说明会替换该天该泳道的分桶）；空态 → 拆不动时 toast 说明且子卡不变；错误态 → LLM/网络失败 toast + 按钮恢复；加载态 → 按钮置灰 + 原位 spinner，旧内容保留；超长文本 → 桶徽标 ellipsis + title 全文；重复提交 → 请求期间按钮禁用。

## 效果核对

- **效果依赖 LLM 行为，必须真数据核对**：
  - 触发原因：阈值 6 threads / 10 tags，实测近 14 天 8.2% 命中。
  - 方法：上线后跑 `select report_id, cluster_index, count(*), array_agg(bucket_title) from daily_report_sections where created_at > <上线时间> group by 1,2` 抽样 5 组，人工判定每个桶的方向是否成立（同向、不重叠、覆盖全部 tag）。
  - 量化结果：目标 ≥4/5 组人工判定「可接受」；覆盖 100%（SQL 校验 `sum(jsonb_array_length(cluster_tag_ids))` 与该聚类原 tag 数一致）。
  - 结论与偏差：若 <4/5，回退开关 `daily_report_intra_bucket_enabled=false` 并把失败样本补进 `docs/research/lane-intra-bucketing/`。
- **几何副作用核对**：抽样统计分桶后 section 的 `topic_match_distance` 与「section 到板块均值向量」距离之差是否较分桶前扩大（质心变纯的正向信号）。

## 白盒附加（分支表 / 边界值）

`BucketCluster` 主分支：

| 分支 | 条件 | 期望 | 覆盖 |
| --- | --- | --- | --- |
| B1 | `!enabled` | 直接返回单 section，不调 LLM | `TestPlanBucketCount`（开关关闭） |
| B2 | `lines < min_threads && tags < min_tags` | 不触发 | `TestPlanBucketCount`（4/5 边界） |
| B3 | `lines >= min_threads` 或 `tags >= min_tags` | 触发 | `TestPlanBucketCount`（6/10 边界） |
| B4 | k 计算 | `clamp(round(lines/2.5),2,6)`；lines=2→2、3→2、6→2、7→3、9→4、17→6、25→6 | b/w 边界值表 |
| B5 | phase1 返回组数 == k | 直接进入 phase2 | `TestBucketPhase1Parse` |
| B6 | phase1 返回组数 > k | 合并最小两组（按组内线索数，平局取后出现者） | `TestBucketPhase1Parse` |
| B7 | phase1 返回组数 < k | 尾部不补，按实际组数走（≥2 才继续） | `TestBucketPhase1Parse` |
| B8 | phase1 失败/非法 JSON | 返回 error → 上层降级单 section | `TestBucketClusterDegrade` |
| B9 | phase2 漏线索 | 并入语义最近组（无嵌入时按组大小最小者） | `TestRepairBuckets` |
| B10 | phase2 重复线索 | 保留首处 | `TestRepairBuckets` |
| B11 | phase2 单例组 | 合并进最近组 | `TestRepairBuckets` |
| B12 | 修复后 1 组 | 退化标记 → 单 section | `TestRepairBuckets` |
| B13 | 修复后 >6 组 | 合并至 6 组 | `TestRepairBuckets` |
| B14 | phase2 失败/非法 JSON | error → 降级 | `TestBucketClusterDegrade` |
| B15 | phase2 留下 0 条线索的空组 | 直接丢弃，不产 section，不计入组数上限 | `TestRepairBuckets`（空组分支） |
| B16 | phase1 某桶缺失当日章节标题 | 回退桶内首条 thread 标题 → 泳道 label（不得借用其他桶内容） | `TestBucketCluster` |

rebucket 分支：报告不存在 → 404；status≠done → 409；`lane_tier` 为 `watch_keyword`/`watch_sentence` → 400；section 不属于该报告 → 404；未达阈值 → 200/buckets=1 不改数据；新桶数 < 旧桶数 → 删除多余行（注记/watch 命中先重派生再删）；新桶数 > 旧桶数 → 补建新行；并发同日重跑 → 同键 advisory lock 串行；事务内任一步失败 → 全回滚。

派生字段核对（重新分桶后逐条断言）：`cluster_label`/`bucket_title`/`bucket_index`/`cluster_tag_ids`/`article_count`/`embedding`/`topic_match_distance`/`best_tier`/`avg_score`/`quality_breakdown` 重算；`topic_status_at_report` 沿用报告期快照；`topic_key_event_markers.source_section_id`、`topic_watch_hits.section_id`、`report_annotations.section_id` 均无悬空。

边界值：`lines = 0/1/2/5/6/7/16/17/18/30`；`tags = 0/1/9/10/11/25`；`k = 1（不触发）/2/6/7（裁剪到 6）`；`bucket_index` 0 与最大 k-1；`group 内 sections` 1 条（无拆）/2 条（最小拆分）/6 条（上限）。

不适用划除：并发安全 —— 本 change 不声称；跨时区分组 —— 分组只看聚类内容，不涉时间；国际化文案 —— 方向名与章节标题为中文，与现有 prompt 一致。

## 继承与调整（⓪ 改契约了吗 — 契约 M 反查）

| 旧 Scenario（main spec） | 处置 | 旧测试/资产 | 动作 |
| --- | --- | --- | --- |
| 日报数据模型 · 创建日报记录 / 唯一性 / 并发落库 / 日报关联昨日报告 | 语义不变 | `daily_report_topic_integration_test.go` | 保留，跑绿即可 |
| 日报数据模型 · 线程存储在独立表中 | 语义不变（新增 bucket 列不影响） | `daily_report_repository_test.go` | 保留；补列读写断言到 `daily_report_bucket_columns_test.go` |
| 日报生成编排流水线 · 完整流水线执行 | **契约变化**（新增分桶步） | `daily_report_orchestrator_test.go` | 更新步骤断言；新增 `daily_report_orchestrator_bucket_test.go` 覆盖「分桶成功 / 降级」两分支 |
| 日报生成编排流水线 · 物化失败不阻断 / 生成失败 | 语义不变 | `daily_report_watch_integration_test.go` / `daily_report_orchestrator_test.go` | 保留，跑绿即可 |
| section lane 归属标记 · section 记录 lane 来源 / 物化 section 记录物化来源 | 语义不变（新增兄弟继承约束） | `daily_report_cluster_test.go` / `daily_report_watch_test.go` | 保留；兄弟继承断言新增到 `daily_report_bucket_test.go` |
| DailyReportSection 强制归属 PersistentTopic · 11 条既有 Scenario | 语义不变（仅追加"同日可多条"补充段） | `daily_report_assignment_test.go`、`daily_report_lane_test.go` | 保留；新增「同日多条只记一次命中」「质心逐条计入」两条断言 |
| 展示范围与排序 · 活跃排序 | **契约变化**（section 条数 → 天数） | `lane_snapshot_repository_test.go`（`n` 排序断言） | 更新为按天断言；新增 `lane_dynamics_sort_test.go` |
| 展示范围与排序 · watch 标识 / 沉寂不展示 / 候选不进主区 | 语义不变 | `lane_dynamics_test.go` | 保留，跑绿即可 |
| 泳道卡片构成 · 单日多事件 | 语义不变（本就假设同日多 section） | `lane_snapshot_repository_test.go` | 保留，跑绿即可 |

---

## 实现核对（apply 阶段回填，2026-10-24）

### 白盒分支 → 用例

| 分支 | 用例 |
| --- | --- |
| B1 `!enabled` | `TestPlanBucketCount/kill_switch_off_keeps_a_single_section` |
| B2 阈值未达 | `TestPlanBucketCount/4_lines_5_tags...`、`/9_tags_and_3_lines...` |
| B3 阈值命中 | `.../6_lines_hits_the_thread_boundary_exactly`、`.../10_tags_hits_the_tag_boundary_exactly` |
| B4 k 计算 | `TestPlanBucketCount` 的 2/7/9/10/17/25 六档 + `single_line_cannot_be_split` |
| B5/B6/B7 phase1 ==k / >k / <k | `TestBucketPhase1Parse` 三个子用例（含 duplicate 合并、越界 line 忽略） |
| B8/B14 phase1/phase2 失败 | `TestBucketClusterDegrade/phase_1_error`、`/phase_2_error` |
| B9 未覆盖归位 | `TestRepairBuckets/uncovered_lines_join_the_smallest_group` |
| B10 重复保留首处 | `TestRepairBuckets/duplicate_line_keeps_the_first_occurrence`、`TestBucketPhase2Parse` |
| B11 单例合并 | `TestRepairBuckets/singleton_group_merges_into_the_smallest_other_group` |
| B12 收敛 1 组 | `TestRepairBuckets/degenerate_when_only_one_group_survives`、`TestBucketClusterDegrade/converged_to_one_group...` |
| B13 >6 组裁剪 | `TestRepairBuckets/more_than_six_groups_merge_down_to_six` |
| B15 空组丢弃 | `TestRepairBuckets/empty_group_is_dropped_and_produces_no_section` |
| B16 标题兜底链 | `TestBucketCluster`（桶内首条 thread 标题）、`TestBucketClusterFallsBackToLaneLabel`（泳道 label） |
| 前置：重复 tag 并集 / 无 tag 兜底 | `TestBucketClusterTagUnionAndFallback` |
| 输入：超长方向名照存 | `TestBucketTitlesAreNotTruncated` |

### rebucket 分支 → 用例

| 分支 | 用例 |
| --- | --- |
| 404 报告不存在 / section 不属于该报告 | `TestRebucketHandler/unknown_report_or_section_is_404` |
| 409 非完成态 | `TestRebucketHandler/rejects_a_non-completed_report_with_409`、`TestRebucketRejectsNonDone` |
| 400 watch 物化组 | `TestRebucketHandler/rejects_a_watch-materialized_group_with_400`、`TestRebucketRejectsWatchSection` |
| 未达阈值 → buckets=1 不改数据 | `TestRebucketThresholdGate` |
| 新桶数 > 旧桶数（补建行） | `TestRebucketReplacesGroupKeepingThreads`、`TestRebucketKeepsGroupHead` |
| 新桶数 < 旧桶数（先重派生再删行） | `TestRebucketShrinksGroupDeletingSurplus` |
| 派生字段重算 / 继承列与快照 | `TestRebucketDerivedFields`、`TestRebucketDerivedFieldsInheritAssignment` |
| watch 命中 / 页边注 / 关键事件注记重派生 | `TestRebucketDerivesWatchHitsAndAnnotations`、`TestRebucketShrinksGroupDeletingSurplus` |
| 失败整体回滚 | `TestRebucketRollbackOnFailure` |
| 并发同日重跑（同键 advisory lock） | `TestRebucketSerializedWithSaveReport` |
| 兄弟桶不参与同日合并 | `TestMergeRejectsSiblingBuckets` |
| 流水线成功拆分 / 失败降级 / 未达阈值不调 LLM | `TestOrchestratorBucketStep` |
| 每桶用自己的 tag 集算 embedding | `TestBucketSectionEmbeddingPerBucket` |
| 兄弟继承归属、各自算距离 | `TestBucketSiblingsInheritAssignment` |
| 泳道排序按天去重 | `TestLaneDynamicsSortsByDistinctDays`、`TestGetBoardLaneDynamics_FullMatrix`（原断言在新口径下保持不变） |
| 前端徽标 / 条件标题 / 按钮可见性 / 确认 / loading | `DailyReportTopicSection.test.ts` 新增 7 个用例 |
| 组内按 bucket_index 排序 | `dailyReportMagazine.test.ts` 新增 3 个用例 |
| 局部替换 / 失败不变 / 拆不动 / 重复点击 / 深链别名 | `useDailyReportReader.test.ts` 新增 4 个用例、`DailyReportTopicSection.test.ts` 别名用例 |
| API 契约 | `dailyReports.test.ts` 新增 2 个用例 |

### 五问句结论

- **⓪ 改契约了吗**：见上「继承与调整」表——所有旧 Scenario 逐行处置（语义不变者保留跑绿，契约变化者更新断言并新增用例）；`promptVersion` 旧断言按 R-1 从 `5.0` 改为 `6.0`（`TestPromptVersion_Is6`）。
- **① 节拍全吗**：新增 12 条 Scenario 全部有落点（上两表）；负向节拍（不丢线索、不跨聚类、不粘回、watch 组拒绝、非完成态拒绝）均有断言；外部依赖失败有答案（phase1/phase2/embedding/事务失败 → 降级或回滚）。
- **② 变体走查**：空串/纯空白 thread 标题 → 标题兜底链（泳道 label）已覆盖；单 token → 阈值不触发；超长方向名 → 照存 + 前端 ellipsis（`TestBucketTitlesAreNotTruncated`）；重复 tag → 并集去重；越界 line id → 视为未覆盖走修复；部分 thread 无 tag_ids → 桶并集为空时回退聚类 tags；时间窗口 → 分桶只用当次聚类线索（不跨天），历史日报靠 rebucket 按需触发；幂等 → 同输入同 temp 逐条一致（rebucket 失败整体回滚）；并发 → rebucket × 同日重跑同键串行（`TestRebucketSerializedWithSaveReport`，不声称通用线程安全）；可用性 → 二次确认 / 空态 toast / 错误 toast + 按钮恢复 / loading 置灰 / 重复提交忽略，均有 Vitest 用例。
- **③ 层选对了吗**：纯分组逻辑 → service 单测（fake LLM/embed）；SQL 与事务 → testcontainer PG（repository 禁 SQLite）；HTTP 门禁 → handler 单测（SQLite 跳过 advisory lock，仅门禁路径）；组件 → Vitest；端到端 → `ui-verification/report.md`（上线后按 ui-design 两档视口补录）。
- **④ 效果核对了吗**：依赖真库与 LLM 行为，**上线后执行**：`select report_id, cluster_index, count(*), array_agg(bucket_title) from daily_report_sections where created_at > <上线时间> group by 1,2` 抽 5 组人工判定（≥4/5 可接受、覆盖 100%），不达标回退 `daily_report_intra_bucket_enabled=false` 并把失败样本补进 `docs/research/lane-intra-bucketing/`。
- **⑤ 展示字段盘点了吗**：`bucket_title` / `bucket_index` 均有 spec 锚（`daily-report-system` 日报数据模型 + 泳道内主题分桶产出段），`section_count_14d` 语义变化（仅展示、排序改按天）见 `board-lane-dynamics` delta spec。
