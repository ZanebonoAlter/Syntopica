## REMOVED Requirements

### Requirement: 主体归一化

**Reason**: 主体归一的三层匹配（`subject_key` 精确 → embedding 余弦 ≥ 0.92 → 字面归一）是为事件线身份判定服务的，判定入口随事件线退役一并停写。实测同一规范名对应 >1 个 `subject_key` 的漂移组达 138 组、类型前缀在 product / person / org / other 之间漂移，归一质量本身不可靠，继续保留会造成「规范仍在、判据已死」的悬空契约。
**Migration**: 事件标签的指纹与 `subject_key` 仍由打标链产出（见「事件指纹结构化产出」，本次不动），实体层影子模式仍读 `tag.Fingerprint.SubjectKey`，二者与事件线身份解耦。`board_stories.subject_key` 列与存量数据只读保留；新事件线不再建立，主体键不再用于任何在线判定。

### Requirement: 事件线数据模型

**Reason**: `board_stories` 事件线模型、`daily_report_threads.story_id` 归属列与 `merged_into` 合并链的在线写入路径整体退役：归属判定停写后不再产生新的事件线、不再挂载事件、不再合并。保留「新建/合并事件线」的 SHALL 契约会让实现与规范分叉。
**Migration**: 表与列全部保留且只读：`board_stories`、`story_signals`、`daily_report_threads.story_id` 不删表、不删列、不迁移数据，GORM 模型继续注册以维持 AutoMigrate 兜底。新生成与重建的日报 `story_id` 恒为 NULL、恒不出现在对外响应里。回滚只需把停写开关与读端口径改回去，不需要数据修复。

### Requirement: 事件归属判定顺序

**Reason**: 五层归属判定（`subject_key` 一票否决 → 精确相等 → 同次发生 → 阶段推进 → 合并闸门 → 模型四分类兜底）唯一的产出是事件线归属与合并，属已退役能力。指纹全同的跨天挂载本有通路，实际死在召回盲区与信任闸门，继续修判定顺序是在补三个各自独立的漏而非补主路。
**Migration**: 归属判定链整体停写（开关在日报链尾部的在线归属判定入口）。死代码 `SpawnAttributeStoriesForReport`（当前只有测试调用、无生产调用方）保留但不接线。历史归属结果保留在 `daily_report_threads.story_id`，只读。读者侧的「是不是同一件事」改由 `related-events` 的直接相关与日报当天的同事件聚簇承担。

### Requirement: 综合稿不建立事件身份

**Reason**: 该 Requirement 的主体是「综合稿永不建/挂事件线、不计 `thread_count`、不进热度证据」，其锚点是事件线模型。事件线退役后，「不建事件线」这半句失去对象。
**Migration**: 综合稿判定本身**不退役**，改由 `daily-report-system` 的「日报数据模型」承载：`scope` 仍由 `tag_ids` 中 event 标签数推导（≥2 = composite），且 composite 的 thread SHALL NOT 进入任何当天同事件聚簇、SHALL NOT 计入簇内文章数。`daily_report_threads.scope` 列与取值不变。

### Requirement: 人工纠正优先

**Reason**: 人工把事件指派到事件线、或把事件标记为「独立事件」，其全部作用对象是 `story_id` 与事件线合并链。归属判定停写后无新归属产生，人工纠正既无对象可纠正，也不再有任何自动流程会改写这些历史标记。
**Migration**: `daily_report_threads.story_manual` 列保留为只读历史列（人工指派 / 拆分过的行仍为 true），SHALL NOT 触发任何判定或写动作。人工纠正事件线的入口本就无独立 HTTP 端点（`topicgraph/handler/` 无 story 路由），本就不需要新增替代入口。

### Requirement: 历史回填与降级

**Reason**: 按版块 × 日期升序的存量回填命令是事件线身份的补齐手段；退役后回填出来的仍是「无人消费」的事件线，且回填会写 `board_stories` / `story_signals`——这正是「停写」必须堵死的旁路。读取路径的「事件身份未回填 → 保守降级」三级状态也随之整体失去消费方。
**Migration**: 回填 CLI 的写模式（`--reattribute` / `--restore` / `--rebuild-signals` / `--backfill-content-keys` / `--fill-story-embeddings` / `--fill-thread-embeddings`）改为直接拒绝并给出退役说明；只读 / 实验模式保留。`cmd/backfill-stories` 之外不新增任何写入口。读取路径不再有「未回填」三态：矩阵与雷达按线索数口径取数，无需事件身份。
