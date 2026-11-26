## Context

见 `proposal.md` · Why。设计前已完成的取证（记录在 `docs/research/lane-intra-bucketing/explore-findings.md`）：

- **结构**：`assembleLaneGroups`（`daily_report_lane.go`）用 `map[topicID]*acc` 聚合，一个话题当天恒定只产出一个 ClusterGroup → 一条 section。下游其实早就支持同话题多条：库内 86 个同 (topic, day) 多条实例（均来自手动编排与 watch 物化）、泳道布局按"单天最大节点数"撑高、阅读视图 `v-if="group.sections.length > 1"` 才渲染子标题。
- **几何**：L1 阈值 0.18 已失效（近 14 天 87%~100% 的 section 距离 <0.18）；质心 = 30 条 section 等权均值，实测到自身质心 0.098 vs 到板块全局均值 0.120（相关系数 0.685）。
- **实验**：单次自由分组 8 次运行中 2 次丢线索、组数 6→9 漂移；两阶段（固定组名 → 只做归属）5/5 次 100% 覆盖、temp=0 下重复运行逐条一致；`tags` 当输入过碎（9 桶 4 单例）。
- **分布**：触发阈值下近 14 天仅 51/621 条 section（8.2%，约 3.6 条/天）；集中在 3030（15）与 1980（18）；1974 等叙事板块单条 section 最大 5 个 tag，永不触发。

## Goals / Non-Goals

**Goals**
- 域型泳道的一天能在**不改归属语义**的前提下分出 2-6 个主题方向，读者一眼看出"这天这个领域在往哪几个方向使劲"。
- 分桶过程对数据零丢失：覆盖 100%、失败或不可拆时严格退回现状。
- 用户可对单日单泳道重分桶，且不丢关键事件注记。

**Non-Goals**
- 不做子桶的跨天身份（B 方案：`parent_topic_id` 两级话题树 / 主题词表归一）。
- 不改 lane 分桶（L1/L2/L3）、不改质心与吸尘器算法、不重设 0.18 / 0.30 阈值。
- 不改泳道视图（`BoardThreadBrowser.vue`）与页面结构；页面统筹优化另开 change。
- 不重跑历史日报：存量 section 保持 `bucket_index=0`、`bucket_title=NULL`。

## Decisions

### D1. 分桶发生在话题桶内部，产出多条同归属 section，不新建子话题实体

- **结论**: 在 lane 分桶之后、section 组装之前，对**单个聚类**（一条 `ClusterGroup`）内部拆桶；拆出的 k 条 section 共享 `persistent_topic_id` / `lane_tier` / `topic_match_confidence` 与 `cluster_index`，只用 `bucket_index` / `bucket_title` 区分。不引入 `parent_topic_id`、不建子话题、不动生命周期。
- **落点**: `backend-go/internal/topicgraph/service/daily_report_orchestrator.go:GenerateDailyReport`
- **取舍**: 否决"子话题树（B 方案）"——代价是表结构 + 质心 + 生命周期 + 全链路迁移，且会再造一批需要治理的持久实体；否决"纯前端分组"——不落库就没有重分桶、无法被泳道/趋势消费端识别。A 方案复用了全部既有渲染位与布局能力（86 个历史实例已验证多 section 同话题可用）。

### D2. 分桶输入是当日 thread 线索，不是原始 tag 列表

- **结论**: 两阶段都以该聚类当日的 thread（标题 + 摘要 + 关联 tag）为分组单位；`cluster_tag_ids` 由桶内 thread 的 tag 并集派生。
- **落点**: `backend-go/internal/topicgraph/service/daily_report_bucket.go`（实施时新增；现落在 `daily_report_llm.go:GenerateClusterThreads` 之后调用）
- **取舍**: 否决 tag 粒度输入——实测同一 section 用 tags 输入产出 9 桶、4 个单例桶，粒度太碎；thread 是已 fact-anchor 的短文本，token 省且判断质量更高。

### D3. 两阶段（代码定组数 → 定组名 → 只做归属），温度 0

- **结论**: 组数 k 由代码算（`clamp(round(线索数/2.5), 2, 6)`），phase 1 只产出 k 个方向名 + 每组当日章节标题，phase 2 用固定方向名做逐条归属、禁止增删改名合并拆分；两阶段温度均为 0。
- **落点**: 场景「组数指令不被模型遵守时收敛」（`specs/daily-report-system/spec.md`）
- **取舍**: 否决"一次自由分组"——实测 8 次运行 2 次静默丢线索、组数在 6~9 漂移、`-k` 指令形同虚设；否决"embedding 聚类直接分组"——当前向量空间对比度低（跨话题 section↔centroid 距离 0.08~0.18），不可作为主判据。

### D4. 触发由阈值决定，且阈值同时是"误伤保护"

- **结论**: 仅当聚类 thread 数 ≥ `daily_report_intra_bucket_min_threads`(默认 6) 或 tag 数 ≥ `daily_report_intra_bucket_min_tags`(默认 10) 时触发；全局开关 `daily_report_intra_bucket_enabled` 默认开。叙事型板块（单条 section ≤5 tags）永不触发。
- **落点**: `backend-go/internal/topicgraph/repository/daily_report_topic_repository.go:DefaultPersistentTopicConfig`（同处扩展配置读取）
- **取舍**: 否决"交给模型自己判断要不要分"——实测 4 条线索的 section 被硬切成 3 组且 2 个单例桶；否决"所有 section 都分"——72% 的 section 只有 1-2 条线索，分了只是制造碎卡片。

### D5. 校验器是确定性的，失败一律退回单 section

- **结论**: 归属结果必须满足 ①每条线索恰好一次 ②每组 ≥2 ③组数 ≤6。修复规则：未覆盖 → 并入语义最近组；重复 → 保留首处；单例 → 合并进最近组；收敛后仅 1 组 → 回退单 section。任一 LLM 阶段失败 / 解析失败 / 修复后仍不合法 → 记 warn 日志、保持单 section，日报不失败。
- **落点**: 场景「归属漏掉线索被修复归位」「单例组被合并」「全组收敛为一组时回退」「分桶失败不阻断流水线」
- **取舍**: 否决"信任模型输出的覆盖完整性"——实测两次漏掉 2-3 条线索且不报错；宁可退回现状也不让日报少事件。

### D6. 数据模型复用 `cluster_index` 做兄弟分组，只加两列且不回刷

- **结论**: 兄弟桶共享来源聚类的 `cluster_index`（该列本就无唯一约束），新增 `bucket_index`（默认 0）与 `bucket_title`（可空）。存量行保持 `bucket_index=0` / `bucket_title=NULL`，历史日报形态不变，不做回刷。
- **落点**: `backend-go/internal/topicgraph/repository/daily_report_models.go:DailyReportSection`
- **取舍**: 否决"新增 group_key UUID 列"——`cluster_index` 已能唯一标识"当日第几个聚类"，多一列只增加写入面；否决"回刷历史"——历史 section 的 thread 已被消费（注记、趋势），回刷收益低风险高。

### D7. 重新分桶按 (report_id, cluster_index) 整组替换，thread 复用不删

- **结论**: `POST /api/daily-reports/:id/rebucket` 以 `section_id` 定位桶组，重跑两阶段后替换该组 section：复用可复用的行、补建不足的桶行、删除多余的行；thread 行只迁移 `section_id`，绝不删除；`topic_key_event_markers.source_section_id` 按 `source_thread_id` 重新派生；重算该话题质心与该组 section 关系；全过程单事务，失败整体回滚；报告非 done 或目标 `lane_tier` 为 `watch_*` 返回 409/400；未达分桶阈值或收敛为 1 桶返回 200 + `buckets=1` 且不写库。
- **落点**: `backend-go/internal/topicgraph/handler/daily_report_handler.go:RegisterDailyReportRoutes`
- **取舍**: 否决"重新跑整份日报"——会把当日的 highlights 与所有其他 section 一起洗掉，且耗时与成本高一个量级；否决"删旧 section 建新 section"——删行会让已确认注记失去来源锚。

### D11. rebucket 与同日重跑共用 (board, date) 事务级 advisory lock

- **结论**: rebucket 事务开头 SHALL 先取与 `SaveReport` **同键**的 `pg_advisory_xact_lock(dailyReportSaveXactKey(boardID, periodDate))`，并在锁内复核报告 status 与目标组（status 检查不得留在锁外，避免 TOCTOU）。
- **落点**: `backend-go/internal/topicgraph/repository/daily_report_repository.go:dailyReportSaveXactKey`
- **取舍**: 否决"只靠 status=done 的 409 闸"——检查与写入之间存在窗口，同日重跑（`SaveReport` 同键加锁后整份删写 sections/threads）可能已把 thread 行删掉，rebucket 的 `UPDATE ... WHERE section_id=?` 会静默影响 0 行，或向已重建的 report 插入旧 `cluster_index` 的幽灵 section。

### D8. 分桶产物不参与同日相似合并

- **结论**: 同一 `cluster_index` 的兄弟 section 之间禁止合并；合并步骤在建边前按 `cluster_index` 过滤。`daily_report_section_merge_enabled` 保持默认关闭。
- **落点**: `backend-go/internal/topicgraph/service/daily_report_merge.go:sameAnchorClass`
- **取舍**: 否决"让合并自然处理"——`sameAnchorClass` 当前允许同话题合并，拆分产物会因内容贴近（距离 <0.20）被粘回一条，等于白拆。

### D9. 泳道动态排序改按天去重

- **结论**: `board-lane-dynamics` 的排序口径从"近 14 天锚定 section 数"改为"有锚定 section 的天数"，同日多条只计一次；二次排序按最近命中日期。
- **落点**: 场景「活跃排序按天计」（`specs/board-lane-dynamics/spec.md`）
- **取舍**: 否决"保持 section 条数排序"——拆分会让当天拆得多的泳道虚高，长期看排序会奖励"内容杂"的泳道，反向激励错误目标。

### D10. promptVersion 升 6.0，分桶 operation 纳入 AI 日志

- **结论**: `board_daily_reports.generation_prompt_version` 由 "5.0" 升至 "6.0"（"5.0" 已由 2026-08-29 `daily-report-section-title-decouple` 占用，同日两个新 prompt 若沿用会让两代语义共用一个版本号）；两个新 operation `daily_report.bucket_theme_names` / `daily_report.bucket_assign` 走 `CapabilityDigestPolish` 路由并写 `ai_call_logs`。
- **落点**: `backend-go/internal/topicgraph/service/daily_report_llm.go:promptVersion`
- **取舍**: 否决"沿用 5.0"——`ai_call_logs` 与版本审计将无法区分分桶前后；另：main spec「promptVersion 升级」Scenario 仍写 "4.0" 属既有漂移（5.0 落地时未同步），本 change 以 delta 一并修正为 "6.0"。

## Risks / Trade-offs

- **[模型漂移：同一份内容两次分桶结果不同]** → 两阶段 + temp=0 已把逐条差异压到 0~1 条；仍保留「重新分桶」入口作为人工纠偏；分桶失败不影响归属与日报完整性。
- **[拆分把质心窗口变密，质心更快漂]** → 拆分让每条 section 更纯（不再是多主题平均值），方向上是改善；上线后按 `docs/research/lane-intra-bucketing` 的度量复测「到自身质心 vs 到板块均值」的差值是否扩大。
- **[桶名逐日漂移，无法跨天统计]** → A 方案有意只做天级；数据（`bucket_title`）已落库，未来 B 阶段可离线挖掘高频方向再决定是否提升为持久主题。
- **[重分桶改变 section id，影响外部引用]** → thread 不删、注记按 thread 重派生；关系表按组重建；`section-timeline` 与关键事件注记以 thread 为锚，故不受影响。
- **[阈值拍在 6/10 上]** → 由配置项控制，可调；现值为实测 8.2% 触发率下最保守的选择（不会误伤 1974 这类叙事板块）。
- **[rebucket 事务横跨两次 LLM + 一次 embedding（秒级），期间持有 (board, date) advisory lock 与一个连接]** → 这是 D11 的直接代价：锁的作用域必须覆盖「读组 → 规划 → 写回」，否则无法排除同日重跑把 thread 换掉的窗口。正确性无虞（锁内复核 status / cluster_index / thread 归属，组变了返 `ErrRebucketStaleGroup` 让用户重试）；代价是同日重跑会等这几秒、且占用一个连接。后续若要缩短事务，可演化成「锁内读快照 → 锁外规划 → 新事务 Apply 复核」（`ErrRebucketStaleGroup` 已预留该路径）。
- **[rebucket 后 thread 的 `fit_distance` 沿用旧值]** → thread 行的 `fit_distance` 是「thread 标题 ↔ 所属 section 标题」的观测信号，rebucket 只重算 section 侧 embedding、不重算该观测值，故它会短暂反映旧整簇章节标题。spec 未把 `fit_distance` 列入重算清单（它是软降级观测项，不影响归属/展示结构）；如实记录为已知陈旧信号，下一次同日重跑自然刷新。

## Migration Plan

1. 加列（`bucket_title VARCHAR(200) NULL`、`bucket_index INT NOT NULL DEFAULT 0`），无数据回填。
2. 配置项默认值随代码下发（`daily_report_intra_bucket_enabled=true`、`min_threads=6`、`min_tags=10`），不写库也可用。
3. 上线后新生成的日报自动分桶；历史日报保持单 section 形态（阅读视图渲染条件已兼容）。
4. 回滚：把 `daily_report_intra_bucket_enabled` 置 false 即回到现状（新列留空不影响读取）；重新分桶接口可整体下线，不影响已落库数据。

## Open Questions

- 桶名的跨天归一（主题词表 / 聚类复用）留待 B 阶段；本 change 不做。
- 阈值是否应按泳道形态（域型 vs 叙事型）自适应，还是保持全局常量？先按全局常量上线，用 `bucket_title` 落库数据回看再定。
