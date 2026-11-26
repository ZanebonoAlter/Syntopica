
## 话题泳道窄容器崩坏根因（容器宽 7 列网格 + span2 定位）

**崩坏不是「视口窄」，是「容器窄」——加媒体查询修不了。**

- 折线图本体是 `overflow-x:auto` + `min-width:49rem` + `repeat(7, minmax(7rem,1fr))`（MiniLifeline.vue:240/258/290），只会横滚，不会塌。
- 塌的是详情面板：外层网格 `repeat(7, minmax(0,1fr))`（:383-386，**无 min-width，随容器无限收缩**），面板内联定位 `gridColumn: ${selectedDayColumn} / span 2`（:159），列号 clamp(1..6)（:37-43）。再叠加 `.drm-history` `margin .75rem 1rem + padding 1rem`（DailyReportTopicSection.vue:1141-1146）和 `.drm-thread-row` `1fr 1fr + 1rem gap`（:967-969）→ 正文单列 ≈ `(容器×2/7 − 64 − 16)/2`。
- **命中的宽度区间**：日报三栏 `14rem | minmax(0,1fr) | clamp(15rem,17vw,17rem)`（DailyReportReader.vue:1101-1109），仅 ≤1100px 才收单栏（:1301-1311）。视口 1101–1400 → 正文列 700–900px、面板 200–300px；视口 721–1100 → 正文 690–1070px、面板 200–300px。**两段均 >720px**，而现有断点只有 `@media (max-width:720px)`（MiniLifeline.vue:456-475、DailyReportTopicSection.vue:1194-1200），永不命中。
- 硬约束：`standard/frontend/layout.md:169-173` 要求窄屏降级用 767.98px / `useIsNarrowViewport()`，且 ≥768px 布局交互零变化；但崩坏区在 ≥768px 的窄容器里 → 只能让面板脱离 7 列/span 2 定位、按容器宽整行重排。
- 零测试兜底：MiniLifeline 无自身 .test.ts，宿主测试 stub 掉它（DailyReportTopicSection.test.ts:25-28）→ 布局回归无人拦。替换时必须补布局/交互测试 + 1440×900 / 1920×1080 / 375×667 / 390×844 双视口证据。

**引用**：front/app/features/tags/components/daily-report/DailyReportMiniLifeline.vue:159、front/app/features/tags/components/daily-report/DailyReportMiniLifeline.vue:383、front/app/features/tags/components/daily-report/DailyReportMiniLifeline.vue:456、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:967、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:1141、front/app/features/tags/components/DailyReportReader.vue:1101、docs/reference/standard/frontend/layout.md:169

<!-- pinned 2026-11-18T05:59:09Z -->

## 线索跨窗口事件的三条数据通路与致命边界

「某持久话题 N 天内相关事件」的三条数据通路，与各缺的一角：

1. **日报快照（长期保留，最全，基座）**：`daily_report_threads.tag_ids`（JSONB 快照）+ `daily_reports.period_date` + `daily_report_sections.persistent_topic_id`（锚定快照）。日报重建只覆盖当日、不删历史 → 30 天窗口可按 topic_id join section 取 thread.tag_ids。模型：`daily_report_models.go:278`（TagIDs 第 284 行、Scope 按 event tag 数派生第 302-307 行）。
2. **人工关键事件注记（现成端点，但前端零调用）**：`topic_key_event_markers`（`key_event_markers.go:55`），字段 `persistent_topic_id` + `source_period_date` + `state` + 冻结快照 `title_snapshot/summary_snapshot/tag_ids_snapshot/article_ids_snapshot`，索引 `idx_topic_key_events_topic_state`。端点 `GET /daily-reports/topics/:id/key-events?days=N&state=confirmed`（`key_event_handler.go:36` + `dailyReports.ts:405`）。前端 `useTopicKeyEvents.ensureTopicKeyEvents(topicId, days)`（`useTopicKeyEvents.ts:144-170`，days=0 不限窗，只留 confirmed）**全仓无调用方，仅命中自身测试** —— 休眠基础设施可直接接 UI。
3. **related-events（不适合做主数据源）**：`GET /semantic-boards/:id/tags/:tagId/related-events`（`related_events_handler.go:36`），窗口吃 `topic_tags.created_at`（标签入库时间不是事件发生时间）+ 依赖手工 CLI 刷新的 `related_event_embeddings` 派生索引（无调度接线）→ 常态 not_ready/partial。只能做「点某条事件再看同类」的二级钻取。

**致命边界**：`article_topic_tags` 边按 `tag_edge_retention_days`（默认 7）GC，随后 `CleanupOrphanedTags` 把零边标签**硬删** → 30 天窗口不许用边做主基座；老标签会在 `thread.tag_ids` 里变成悬空 id，新查询必须容忍（禁内连接静默丢行）。

**SQL 骨架现成**：`ListTopicRecentBriefs`（`daily_report_repository.go:1559-1640`）已做 `persistent_topic_id IN (...) + period_date 窗口 + jsonb_array_elements_text WITH ORDINALITY 展开 join topic_tags`，但窗口写死 7 天、无 `category='event'` 过滤、只喂 prompt（`orchestrator.go:71-77`）→ 改造成 N 天 + event 过滤 + HTTP 出口即可，**另开端点，不动 prompt 注入路径**。

**lifeline 端点**：`/daily-reports/topics/:id/lifeline` 无天数限制、响应已带 confirmed `key_events`（`daily_report_repository.go:1220-1229`），但 `SectionTimelineNode` 无 tag_ids（:85-142），前端 `TopicLifelineData` 只声明 sections+relations（`useDailyReportReader.ts:14-17`）→ 数据在、UI 缺。

**引用**：backend-go/internal/topicgraph/repository/daily_report_models.go:278、backend-go/internal/topicgraph/repository/key_event_markers.go:55、backend-go/internal/topicgraph/handler/key_event_handler.go:36、backend-go/internal/tagmanagement/handler/related_events_handler.go:36、front/app/features/tags/composables/useTopicKeyEvents.ts:144、backend-go/internal/topicgraph/repository/daily_report_repository.go:1220

<!-- pinned 2026-11-18T05:59:09Z -->

## 趋势区与迷你泳道的重叠度：替代成立范围与必须接走的两项能力

**「已被矩阵泳道趋势替代」成立到哪一步：信息维度成立，钻取路径没被替代。**

- **同屏上下相邻**：趋势区（`DailyReportTopicSection.vue:366-374`）→ 当日 section 明细（:379）→ MiniLifeline（:512-628）；DOM 顺序断言在 `DailyReportTopicSection.test.ts:426-439`。
- **趋势区能力**：`LaneTrendOverview` 三档（14 天 / 月 / 年，默认 14 天 `:52`），逐日事件**默认收起**（`:54`）、单日超 5 条折叠（`:101-140`）。逐日事件是 thread 标题字符串，不是标签（`lane_snapshot_repository.go:29, 242-258, 595-610`）；窗口锚定板块最新 completed 日报日（`:30-32, 449-470`），与正在阅读的期次解耦（`useLaneTrendData.ts:9-16, 63-66`）。
- **MiniLifeline 能力**：近 7 天哪几天有节点 + 节点几条（`:120-140`）；identity 边跨天连续性可视化（`dailyReportMagazine.ts:313-331`，只取 `relation_type='identity'`，span>1 标 weak）；点日期就地钻取当日完整 section/thread（带 `fit_distance` 软降级 + `KeyEventMarkerActions`）。
- **结论**：「最近动态」这一维已被趋势区超额覆盖（14>7 天 + 每日细分）；**未替代的是 identity 连续性 + 就地钻取**——而钻取正是窄屏崩坏的载体。所以替换时必须把这两项能力显式接过去，否则是净损失。
- **lifeline 数据本身没有 7 天限制**：后端 `/daily-reports/topics/:id/lifeline` 返回该话题**全部** section（`daily_report_repository.go:1161-1181` + `daily_report_handler.go:744-767`），前端 `dayCount=7` 只是显示窗（`dailyReportMagazine.ts:292-334`）。

**可复用积木**：`LaneTrendOverview.vue`（三档切换 + 逐日折叠 + 四态降级）、`KeyEventMarkerActions.vue`、`DailyReportTopicSection.vue` 的 `selectLifelineDay/historicalSections/historicalPending`（`:177-207`）、`useDailyReportReader` 的 lifeline 缓存（`:55-66, 154-160`）、`createRequestCache/RequestCacheEntry`（`dailyReportMagazine.ts:60-66, 346-395`）。

**引用**：front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:366、front/app/features/tags/components/daily-report/LaneTrendOverview.vue:45、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:29、front/app/features/tags/components/daily-report/dailyReportMagazine.ts:313、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:177、backend-go/internal/topicgraph/repository/daily_report_repository.go:1161

<!-- pinned 2026-11-18T05:59:31Z -->

## 形态定稿：线索行内 chip（含被否决的 v1 与用户原话）

**形态定稿（用户已批准，2026-11-17 对话）：线索行内 chip + hover tooltip + 点击开文章。任何「最近 N 天事件列表/时间轴」的形态都已被否决，实现时不得回退。**

**否决记录（必须知道，否则会重走）**：v1 设计是「话题卡片底部新增『话题事件』区块，按天然日分组列出窗口内所有事件标签卡片」。用户在原型评审时原话：「不要堆列表吧，这可读性太差了，界面是给人用的，不是给agent看的」。核心理由：那是把数据摊开供检索，不是给人读；一屏三天的卡片墙扫不出重点。

**定稿交互三段**：
1. **悬停 / Tab 聚焦 chip** → tooltip：事件标签名 → description（2 行截断）→ 来源文章标题 → 关联度档位词与进度条 → 最近出现日 → 本话题出现次数。
2. **点击 chip** → 直接打开**站内阅读预览弹窗**（复用日报既有 `ArticlePreviewDialog` 出口），弹窗内保留「关联度」与「出现次数」作为阅读上下文。
3. **Esc / 点遮罩 / 点关闭** → 关弹窗，焦点回到触发 chip。

**版面契约**：
- 位置在每条当天线索（thread）**左列、标题+摘要之后**（用户截图红框处），不新增区块、不改两列结构。
- 最多渲染 3 个 chip，其余折叠为「还有 N 条」（展开后仍只是 chip，不跳转不弹层）。
- **无相关事件时整行不渲染**：不留 `min-height` 占位、不写「暂无相关」文案——没有相关是常态，不该每条线索都念一遍。
- chip 默认形态 = 标签（截断）；窄容器下缩短截断 + 换行，不横向溢出。图标形态只在原型里演示过，实现不做开关。

**为什么 chip 不显示关联度小数**：分数是启发式的不是概率，页面里的两位小数会被读成置信度。档位词（强关联 / 相关 / 弱关联）承载判断，小数只放在 tooltip 与弹窗里供核对。

**引用**：openspec/changes/replace-lane-lifeline-with-topic-events/ui-prototype/index.html、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:459、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:967

<!-- pinned 2026-11-18T08:00:00Z -->

## 关联度打分口径与「不接向量」的硬理由

**关联度用确定性打分，不接向量、不复用 related-events 的派生索引。**

```
recency = 1 - daysAgo / windowDays          // 0..1
volume  = min(1, occurrences / 3)           // 0..1，该标签在窗口内出现的次数
cooccur = min(1, sharedTags / 2)            // 0..1，与当前线索共享的 event 标签数
score   = 0.45 + 0.25*recency + 0.15*volume + 0.15*cooccur   // 0.45..1.0
confirmed 命中再 +0.05（封顶 1.0）
level   = score >= 0.80 ? "strong" : score >= 0.62 ? "related" : "weak"
```

**候选集**：窗口内（`[report_date-days, report_date-1]`，**不含当前期**）、同一 `persistent_topic_id` 的其它 section 的 thread，取 `tag_ids` 中 `category='event'` 且与当前 thread 的 event 标签**有交集**的标签。即「这条线之前也谈过这件事」。

**为什么不用向量**（写死，别在实现时改成调 embedding）：
- `related_event_embeddings` 只由 `cmd/related-events` 手工刷新（`cmd/related-events/main.go:167-169`），无调度接线 → 线上常态 `not_ready/partial`；
- 语义相关的「为什么相关」无法在 chip 这么小的位置解释；
- 本项目已有多处「派生索引常年空跑」的教训（related-events 端点至今无日报侧消费方）。

**已确认的取舍**：同义不同词（「显存调优」推不出「量化压缩」）会漏推 —— 记为 Non-Goal，将来若要做二级钻取再谈。

**常数标定依据**：0.45 是「同标签但很久以前、只出现一次」的地板分；权重让「最近」压过「次数多」，因为线索的时效性比篇数重要。实现后按真实分布观察再调，不落 `ai_settings`（不是 AI 参数）。

**引用**：backend-go/internal/tagmanagement/service/related/representation.go:47、backend-go/cmd/related-events/main.go:167、backend-go/internal/tagmanagement/handler/related_events_handler.go:36

<!-- pinned 2026-11-18T08:00:00Z -->

## 前端接入点与移除清单（含为什么整宽一行）

**前端接入点（精确到行，实现直接照做，不用再摸）**

1. **插入位置**：`front/app/features/tags/components/daily-report/DailyReportTopicSection.vue`，`.drm-thread-row` 闭合 `</div>`（约 `:471`）**之后**、`drm-articles`（`:472`）之前，作为 `.drm-thread` 内独立整宽一行：
   ```html
   <ThreadRelatedEvents
     :thread-id="thread.id" :topic-id="section.persistent_topic_id"
     :report-id="reportId" :days="relatedDays" :entry="threadRelationsEntry(thread.id)" />
   ```
   **为什么整宽而不是塞进左列**：`.drm-thread-row` 是 `grid-template-columns: minmax(0,1fr) minmax(0,1fr)`（`:964-969`），左列是同 grid 的 header、右列是 `KeyEventMarkerActions` 的多根 fragment（它自己用 `grid-column` 摆放 ke-detail/ke-actions）。再往这个 grid 里塞第三块会打乱自动摆放；放 row 之后整宽最稳，视觉上 chips 仍从左边起（短时只占左半）。

2. **删除块**：`DailyReportMiniLifeline`（import `:4`、挂载 `:512-628`）整段移除；连带退役 `selectedDayByTopic`、`selectLifelineDay`、`historicalSections`、`historicalPending`、`ensureLifeline`（`:177-207` 一带）与 `#details` 插槽——当日线索本来就在正文可见，不需要「点日期才知道」。

3. **`useDailyReportReader.ts`**：`TopicLifelineData`（`:14-17`）与 lifeline 缓存（`:55-66, 154-160`）在日报侧退役；`getTopicLifeline` API 方法**保留**（侦探墙/将来可能用），后端 lifeline 端点不动。

4. **新组件**：`ThreadRelatedEvents.vue`（chips + tooltip，样式复用 `drm-thread` 卡片 token 族）+ `useThreadRelatedEvents.ts`（模块级单例，key = `${reportId}:${days}`，形状照 `useTopicKeyEvents` 的 `status: idle|loading|success|error`）。

5. **档位状态**：放 `DailyReportTopicSection`（因为 cache key 含 days），7 天默认；切换只影响 chips，不影响正文。

**引用**：front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:471、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:512、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:964、front/app/features/tags/components/daily-report/DailyReportTopicSection.vue:177、front/app/features/tags/composables/useDailyReportReader.ts:14

<!-- pinned 2026-11-18T08:00:46Z -->

## 后端契约与查询路径（批量端点，避免 N+1）

**后端契约（实现直接照做）**

`GET /api/daily-reports/:reportId/thread-relations?days=7|30`

- `days` 只接受 7/30，其它回落 7 并在响应 `days` 回显实际值。
- 窗口 `[report_date-days, report_date-1]`：**不含当前期**（当天的线索同屏可见，不推自己）。
- `reportId` 不存在 → 404；非 completed 日报仍可读（只读快照）。

响应骨架：
```json
{ "report_id": 88, "days": 7, "window_start": "2026-11-11", "window_end": "2026-11-16",
  "status": "ready|partial|empty",
  "threads": { "<threadId>": [ { tag_id, label, description, score, level,
     occurrences, last_seen, confirmed,
     source_thread:{id,period_date,title}, source_article:{id,title,feed,published_at} } ] },
  "total": 6, "stale_candidates": 2 }
```

**查询路径（四步，全部只读）**：
1. 当前期 thread + section：`daily_report_sections`（按 report_id）→ `daily_report_threads`（按 section_id）；
2. 候选：同 `persistent_topic_id`、`board_daily_reports.period_date` 在窗口内的 section → 其 thread 的 `tag_ids`（jsonb，**必须先按 section 收敛再展开**，禁用全局 jsonb 扫描）；
3. 标签元数据：`WHERE id IN (...) AND category='event'` 一次批量（**LEFT JOIN 语义**，缺行即悬空）；
4. 来源文章：收集 `related_article_ids` 首篇 → 一次查 `articles`；confirmed：`topic_key_event_markers` 中 `state='confirmed'` 且 `tag_ids_snapshot` 含该 tag。

**失效候选**：label 缺失的 tag **不进入推荐位**（做不出 chip），但 `status` 置 `partial`、`stale_candidates` 计数如实返回 —— 与「完整性视图不得丢行」的区别：推荐位可以取舍，但不得伪装成 `ready`。

**SQL 骨架可参考但不照抄**：`ListTopicRecentBriefs`（`daily_report_repository.go:1559-1640`）已有 `persistent_topic_id IN (...) + period_date 窗口 + jsonb_array_elements_text WITH ORDINALITY 展开 join topic_tags` 的写法，但它是 7 天写死、无 `category='event'` 过滤、只喂 prompt —— **另开查询，不改它**（prompt 注入路径红线）。

**不做**：不新增索引；不给 `threads.tag_ids` 加 GIN（会鼓励全表 jsonb 扫描）；不写任何表。

**引用**：backend-go/internal/topicgraph/repository/daily_report_models.go:278、backend-go/internal/topicgraph/repository/key_event_markers.go:55、backend-go/internal/topicgraph/repository/daily_report_repository.go:1559

<!-- pinned 2026-11-18T08:00:46Z -->

## 选型实测：thread 向量 vs 事件标签向量（top-3 零重叠）

**选型实测（2026-11-17，用户质疑后重做，之前的数据作废）**：候选匹配用 **thread 向量**，不用事件标签向量。以下为可复现的实测依据。

**标签确实有向量（我最初漏查了）**：
- `topic_tags` 表没有 embedding 列，但向量在 `topic_tag_embeddings`（`embedding_type='event_keyword'`）：**8084 条 / 8084 个标签（1:1）**，活 event 标签 9142 的 **88.4%** 覆盖，有专用 HNSW 索引 `idx_topic_tag_embeddings_event_keyword_halfvec_hnsw`（halfvec(2560)）。
- 生成源：`core/embedding_queue.go:128` 按 `metadata.event_keywords` 文本生成；99.8% 的 event 标签带该字段。
- `related_event_embeddings`（840 条）是 related-events 专用派生表，**不是**标签主向量表，别混淆。
- thread 向量在主表 `daily_report_threads.embedding`，14340/15144 = **94.7%**，生成源是 thread **title**（`service/daily_report_thread_fit.go:35-60`，原本用于 fit_distance）。

**关键瓶颈不是覆盖率，是历史引用里标签行已被 GC 删掉**：近 30 天 `thread.tag_ids` 引用的 4173 个唯一标签 id，只有 1343 个还是 event 标签、其中 1181 个有向量 = **28.3%**。而历史 thread 有向量的比例是 100%（有 embedding 的那些）。

**top-3 对比实测**（topic 502，当期 2026-11-17 共 6 条线索，窗口 30 天）：
| 口径 | 历史候选池 | 平均 top-3 重合 | top1 一致 |
| --- | --- | --- | --- |
| thread 向量 | 235 条 | — | — |
| 标签·平均 | 44 条（19%） | **0.00/3** | **0/6** |
| 标签·最大 | 44 条 | **0.00/3** | **0/6** |

**典型反例**：
- #18669「企业级 Agent 落地需补齐托管与度量基建」：thread 0.890 →「企业级 Agent 平台与组织机制探索」（同一件事）；标签 0.754 →「Agent 协作架构向角色分工与路由拆分演进」（泛）。
- #18668「插件复用历史会话降低 AI 编程代理 token 消耗」：thread 0.854 →「本地记忆库打通多个 AI 编码 agent 的会话孤岛」（高度相关）；标签 0.888 →「编程 Agent 修复后仍持续执行多余步骤」（无关且分更高）。
- 标签 max 出现 **1.000** 推「Agent 客户端与浏览器操作能力外延」——不同标签共享同一段关键词向量导致满分，区分度归零。

**结论**：标签比的是「两个概念像不像」，thread 比的是「是不是同一条线的进展」；且标签聚合（平均/最大）必然丢信息。选 thread 向量的理由 = 候选池 5 倍 + 落点唯一 + chip 可读（标题 vs 概念短语），**不是**「相似度更准」（两者都是同领域召回）。复现脚本形状：当期 thread × 窗口 thread 全组合算 `1-(a<=>b)`；标签口径先展开 `jsonb_array_elements_text(tag_ids)` 再 `GROUP BY (cur_thread,hist_thread)` 取 avg/max。

**引用**：backend-go/internal/topicgraph/service/daily_report_thread_fit.go:35、backend-go/internal/tagmanagement/service/core/embedding_queue.go:128、backend-go/internal/tagmanagement/models/related_event_embedding.go:12

<!-- pinned 2026-11-18T14:29:34Z -->
