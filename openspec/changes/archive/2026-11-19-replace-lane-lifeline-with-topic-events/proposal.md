<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: daily-report, topic-graph, semantic-board -->

# Proposal: 删掉话题泳道折线，改为线索行内的「相关事件」推荐

## Why

日报 active 话题卡片展开后会挂一条 7 天折线（`DailyReportMiniLifeline.vue`：「话题泳道 · 最近 7 天 · 同话题连续性」），点日期展开当日详情面板。两个问题同时成立：

1. **窄容器下排版崩坏，加断点修不了**。折线轨道是横滚的还行，塌的是详情面板——它用**容器宽**的 `repeat(7, minmax(0,1fr))` 网格 + `gridColumn: N / span 2` 定位，再叠 `.drm-history` 的 margin/padding 与 `.drm-thread-row` 的 `1fr 1fr` 两列。日报三栏在视口 721–1400px 时正文列只有 690–900px，面板仅剩 200–300px，正文被挤成一字一行。现有断点只有 `@media (max-width:720px)`，**这个区间永远不命中**；而 `layout.md` 又要求 ≥768px 布局零变化，所以不能靠再加断点解决。
2. **信息维度已被矩阵泳道趋势区覆盖**。`LaneTrendOverview` 已给「近 14 天逐日线索标题」，比泳道的 7 天更细更长。泳道剩下的独有价值只有两项：identity 跨天连续性、点日期就地展开当日线索。

**新方案不是「换一种列表」，而是不占版面。** 用户指出：在每条当天线索下方那块空白里放「相关事件」的 chip，hover 出预览（标签描述 / 来源文章标题 / 关联度），点击直接打开文章。这样线索行保持原样，信息按需出现，不会再堆成列表墙。

数据侧的关键事实是：日报快照（`daily_report_sections.cluster_tag_ids` / `daily_report_threads.tag_ids`）长期保留、不随 7 天边 GC 消失，因此「这条线索过去 7/30 天还谈过什么」有稳定的数据基座。

## What Changes

- **移除 mini 生命线**（**BREAKING**）：日报话题卡片不再渲染折线、节点、贝塞尔连线、节点计数与「在侦探墙打开完整生命线」入口；`DailyReportMiniLifeline.vue` 退役删除。完整生命线从版块态势页的泳道进入。
- **新增「线索相关事件」读模型**：`GET /api/daily-reports/:reportId/thread-relations?days=7|30`，一次返回该期日报**所有**线索的相关推荐（按 `thread_id` 分组），避免每条线索一个请求。
  - 候选 = 窗口内（不含当前期）同话题其它线索，按 **thread 向量余弦相似度 ≥ 0.75** 筛选；
  - 每条推荐给出：候选线索的 title/summary/日期、相似度分数与档位、来源文章（id/title/feed/date）、是否已有 confirmed 注记；
  - 相似度**直接取余弦值**，不叠加时间衰减或篇数权重；不调 embedding 接口（`daily_report_threads.embedding` 已由日报生成流程写好）；
  - 当期线索缺向量的，跳过并计入 `skipped_no_embedding`，状态置 `partial` 如实标注。
- **新增线索行内 chips**：每条当日线索下方、摘要与动作按钮之间那行空白里，渲染最多 3 个 chip + 「还有 N 条」。**无相关事件时整行不渲染**（不留占位、不给「没有相关」提示——那是常态，不该每条都念一遍）。
- **hover / 键盘聚焦出 tooltip**：标签名 → description → 来源文章标题 → 关联度条与档位词 → 最近出现日 → 本话题出现过几次。
- **点击 chip 直接打开站内阅读预览弹窗**（复用日报既有能力与既有出口），弹窗内保留「关联度」与「本话题出现过 N 次」作为阅读上下文。
- **窄容器自适应**：chip 在窄宽度下缩短截断，不换行溢出；不再有任何按「容器宽 / 7 × 2」计算的布局。

## Non-Goals

- **不做「最近 N 天事件时间轴/列表」**：v1 设计（按天分组的事件卡片墙）已被用户否决——那是把数据摊开给人检索，不是给人读。本 change 只做行内推荐位。
- **不用事件标签做匹配**：实测「事件标签 id 精确匹配」有候选的线索仅 1.6%；「事件标签向量」的 top-3 与 thread 向量**零重叠**且候选池只有 19%（历史 `tag_ids` 所指的行 65% 已被孤儿清理硬删）。选型实测见 `design.md` D3。
- **不接 `related-events` 端点与派生索引**：`related_event_embeddings` 靠手工 CLI 刷新、无调度接线，常态 `not_ready/partial`；按标签的语义扩展留给将来的二级钻取。
- **不做跨线索归并**：一条推荐 = 一条候选线索，不合并、不聚类。
- **不改 `topic_key_event_markers` 的写入路径与状态机**：AI 仍只写 `suggested`，确认仍由人工触发；本 change 只读 confirmed 用于给 chip 加 ★。
- **不动 prompt 注入路径**：`ListTopicRecentBriefs` 的 7 天窗口与「每 section 取 5 个 label」口径不改，另开读路径。
- 不做事件染色、不做热度编码、不做事件图谱。
- 不改矩阵泳道趋势 / 雷达 / 雷达关注度的既有口径。

## Capabilities

### New Capabilities

- `thread-related-events`: 线索相关事件推荐读模型——`GET /api/daily-reports/:reportId/thread-relations` 的契约、候选集与关联度打分口径、跨线索合并与排序、失效候选的处置、只读边界。

### Modified Capabilities

- `daily-report-system`: 「话题卡片与 mini 生命线」整体改写为「话题卡片与线索相关事件 chip」——移除折线/节点/贝塞尔连线/节点计数/侦探墙入口，新增行内 chip、tooltip 预览、点击打开阅读预览、无相关时不渲染；相应 Scenario 改写。

## Impact

**后端**（`backend-go/`）
- 新增 `internal/topicgraph/handler/thread_relations_handler.go` + 路由 `/daily-reports/:id/thread-relations`。
- 新增 `internal/topicgraph/repository/thread_relations.go`：基座 `daily_report_sections`（`persistent_topic_id` 索引）→ `daily_report_threads`（`section_id` 索引）；候选按 `daily_report_threads.embedding` 余弦 ≥ 0.75 筛选（先按 section 收敛再算，不做全表向量扫描）；来源文章取 `related_article_ids` 首篇；confirmed 标记查 `topic_key_event_markers`。
- 无 schema 变更、无迁移、无回填、不加索引、不调 embedding。

**前端**（`front/app/`）
- 删除 `features/tags/components/daily-report/DailyReportMiniLifeline.vue`。
- 新增 `features/tags/components/daily-report/ThreadRelatedEvents.vue`（chips + tooltip）与 `features/tags/composables/useThreadRelatedEvents.ts`。
- 改 `DailyReportTopicSection.vue`（线索行插入 chip 行、移除 lifeline 挂载与 `#details` 生命周期）、`api/dailyReports.ts`（新响应类型）。
- `dailyReportMagazine.ts` 的 `buildLifelineWindow` / `buildBezierPath` 确认无消费方后退役。

**依赖与系统**：不新增依赖；不涉及调度、AI 调用、embedding、向量索引。
