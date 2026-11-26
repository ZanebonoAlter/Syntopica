<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: topic-graph, semantic-board, daily-report -->
<!-- doc-impact: flow, api -->

# 泳道编排新增事件线链（跨天同事件选材维度）

> **⚠️ 已废弃（2026-11-11）**：事件线设计经讨论与数据验证后决定放弃，本 change 未实施（0/24 任务）即归档。delta spec 未同步进主 spec，本文档仅作设计探索存档。

## Why

用户原话：「新建泳道时，我觉得**事件线**比现有的 embedding 关联线索更靠谱」。

用户已拍板 Q3 = 事件线作为**新增维度**：保留现有话题链，另加一条「事件线链」——同一事件跨天的分区直接串起来；建泳道时可按事件一次勾齐跨天材料，两条链并列供选。

两条硬证据支撑这个判断：

1. **候选链的 similarity 边量的是「两个分区像不像」，不是「是不是同一件事」。** `daily_report_section_relations` 的 similarity 边 distance = `daily_report_sections.embedding` 的余弦距离；该 embedding 是分区**内容向量**（section-content-embedding 口径：所聚标签的 label/description/代表文章摘录，无标签时退回线索标题、再退回 `cluster_label`）。它回答的是「这两坨材料讲的是不是相近的话题」，不是「这两天报道的是不是同一件事」。于是「同一件事被写成两种说法」必然连不上，而「两件事用了同一批词」反而连得上——用户说的「不靠谱」指向的正是这个错位。
2. **事件线身份走的是显式判定链，可靠性远高于 embedding。** 归属判定顺序固定：subject_key 预筛一票否决 → subject/object 严格相等 → 日期邻近 → 可信指纹（`fingerprint_trusted`）→ 合并闸门；LLM 不参与合并判定，向量相似度**只用于召回候选**（dist ≤0.5 top10），MUST NOT 单独作为合并判定依据（`flow/topic-graph.md` 红线 14、`flow/daily-report.md` §事件线归属与跨期去重 约束 42–45）。这条链判定的是「是不是同一件事」，正是选材时需要的那件事。

## What Changes

- **事件线链作为新增维度**，与现有话题链**并列**出现在编排态左侧候选栏：话题链仍按 candidate 话题分组、由原 identity/similarity 边连通切分；事件线链按**事件线存活 id** 分组，同一事件跨天的分区直接串起来，可一次勾齐。
- **两条链的分组键正交**：一条事件线链可横跨多个 candidate 话题；一条话题链里也可含多条事件线。事件线链是**辅助选材维度**，**不是候选话题资格判定**——不得因「同一事件」产生、合并或激活任何话题（候选生命周期、连续命中门禁、手动建话题直落 active 三条红线一律不动）。
- **事件线边的构造走既有数据链路**：section→threads→`story_id` 批量反查 + `merged_into` 一跳存活解析；不新增表列、不写关系表，查询期内存构造。
- **成链门槛与话题链同口径**：≥2 个分区且首末 UTC 日差 ≥3 天，中间允许断档。
- **回填依赖诚实降级**：板未回填事件身份（`story_id` 全 NULL）时该板**无事件线链**，界面明示「事件身份未回填」，MUST NOT 冒充「无候选」；部分回填时给出不完整提示。
- **相似推荐口径不变**：`useInlineCompose` 的「相似 section 推荐」仍按已选聚合向量质心做 cosine 召回，事件线**不进入**该召回集、不改其排序与阈值；两条线索在界面上分区并列、各自标注来源。理由见 design §D5。

## Capabilities

- Modified: `workshop-lane-compose`（`openspec/specs/workshop-lane-compose/spec.md`）
  - MODIFIED「候选话题侧栏」：左侧列表从单一话题链扩为话题链 + 事件线链两条并列维度。
  - MODIFIED「成员质心与关联推荐」：显式写死事件线不参与质心召回与相似推荐排序（保持既有优先级）。
  - MODIFIED「保存与只读边界」：显式写死事件线链不改变候选话题的命中计数与状态机。
  - ADDED「事件线链（跨天同事件选材维度）」：构造口径、成链门槛、正交性、降级与红线边界。

## Impact

- 后端：`backend-go/internal/topicgraph/repository/compose_chain.go`（新增事件线链纯函数）、`daily_report_manual_topic.go`（`GetComposeCandidates` 批量取 section→threads→story 并组装故事链）、`lane_snapshot_repository.go`（批量反查形态复用点）；`backend-go/internal/topicgraph/handler/daily_report_handler.go` 端点响应体扩展。**无 DDL、无迁移、无新表列**。
- 前端：`front/app/api/persistentTopics.ts`（响应类型）、`front/app/features/tags/composables/useLaneCompose.ts`（编排态并列两链的勾选态）、`front/app/features/tags/components/compose/LaneComposeCandidates.vue` 与 `LaneComposeCanvas.vue`（两链的呈现与区分）。沿用既有编排态布局，**不新建页面/路由/弹层**。
- 存储：零新增表列；事件线链在查询期内存构造，不落 `daily_report_section_relations`，不污染既有 section status 动态推导。
- 依赖：上游事件线能力 `add-event-story-heat`（`board_stories` + `daily_report_threads.story_id` + `cmd/backfill-stories` 回填）；无新增外部依赖。
- 性能：窗口上限 90 天；新增 2 次批量查询（threads + board_stories）+ 内存分组，不产生 N+1。
- 文档（实现阶段执行，本阶段不改）：`docs/reference/flow/topic-graph.md`、`docs/reference/flow/daily-report.md`、`docs/reference/flow/semantic-board.md`、`docs/reference/api/`。

## 部署后影响与需要的操作

本阶段只产出待评审制品，当前运行行为不变，无部署或数据操作。

后续实现部署后：新建泳道的编排态左侧会多出一组「事件线」候选，用户可按「同一事件跨天」一次勾齐材料，原话题链与相似推荐行为**保持不变**。**需要用户手动执行的操作**：事件线链依赖 `daily_report_threads.story_id` 已回填，未回填的版块**不会**出现事件线链（界面会明示「事件身份未回填」）；想让某板出现事件线链，需由运维按既有运维手册跑 `cmd/backfill-stories`（该命令属既有能力，本 change 不改其行为）。**旧数据降级**：已回填的板**无需任何数据迁移**即可受益（每次查询现算）；未回填的板维持现状，不报错、不冒充无候选；已有泳道、已有话题归属与既有关系表**完全不受影响**（本 change 不写关系表、不改话题状态机）。
