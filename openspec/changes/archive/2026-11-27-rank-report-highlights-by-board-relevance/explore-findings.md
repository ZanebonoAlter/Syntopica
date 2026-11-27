
## 日报今日重点（top3）选取与排序链路 <!-- pin:c6b8992c -->

## 现状一句话
今日重点 = LLM 从「当天同事件聚簇 Top8（簇内去重文章数降序）」里挑 ≤3 条写文案；展示序 = 服务端候选序（不是模型序）；highlight 上没有分数、没有阈值、没有「跟版块主题的相关度」。

## 生成链路（后端）
- 入口：日报落库尾部 `finalizeDailyReportHighlights`（`backend-go/internal/topicgraph/service/daily_report_story_highlights.go:186`）：SaveReport → 去重 no-op → 聚簇候选 → LLM → `FinalizeReportHighlights`。
- 候选：`RankEventClusterCandidates`（`backend-go/internal/topicgraph/repository/report_event_clusters.go:201-224`）排序 = 簇内去重文章数 DESC → 簇内 event 标签数 DESC → 代表标签 id ASC，截 Top 8（`maxEventClusterCandidates = 8`，`report_event_clusters.go:172`；调用点 `story_highlights.go:201`）。上游预算：当天 event 标签 > 500 按当天命中线程数 DESC、id ASC 截断（`daily_report_event_cluster.go:45/146-156`）。
- LLM：`story_highlights.go:31` system prompt + `:66-77` user = `json.Marshal(facts)`，facts 只有 `cluster_id / label / description / article_count / facts[{id,label,description}]`；`article_count` 只作上下文、禁止复述（`:37` 注释）。**prompt 不含版块名、当日主题、泳道/persistent topic 名、section 标题、bucket 标题**——模型看不到「本期主题」。旧标签回退 prompt 同样不含主题（`daily_report_llm.go:31` + `buildHighlightsPrompt:162`）。
- 输出：`parseClusterHighlightsResponse`（`story_highlights.go:148-156`）按簇归集后**遍历 candidates 顺序**输出，`if len(out) == 3 { break }`；注释 `:20`「Model selects and writes; the server's candidate order stays the display order (never the model's)」。模型可返回 1 条甚至 `[]`（合法降级不重试）；重复簇保留首条，集外 cluster_id / 越界 tag_ids / 空白文案判非法同参重试一次。标签回退路径次序 = LLM 数组原序。
- 落库：`FinalizeReportHighlights`（`backend-go/internal/topicgraph/repository/report_story_highlights.go:76-82`）单次条件 UPDATE 写 highlights/title/summary，**`title = highlights[0].Title`、`summary = highlights[0].Reason`**；无重点时 title=「日报 <date>」。

## 刊头渲染（前端）
- `DailyReportMasthead.vue:107` H1 = `boardTitle || report.title`；`:108` 副标题 = `report.title`（仅 `v-if="boardTitle"`）。`boardTitle` 是**静态版块名**（`DailyReportDeskShell.vue:79-81` → `DailyReportReader.vue:668-670` 用 `boardsApi.getBoards()` 的 label 查找），与当日内容零耦合。
- `:27` `highlights = report.highlights.slice(0,3)`；`:129-131` `v-for` + `N°{{index+1}}` → N°01/02/03 就是 highlights[0..2]，顺序照搬后端。
- 头条：`selectLeadStory`（`front/app/features/tags/components/daily-report/dailyReportMagazine.ts:238-246`）取 `highlights[0]`，空时回退 `sortDailyReportSections(sections)[0]`（`:43-51`：watch 板块末位 → `best_tier` 升序 → `avg_score` 降序）。
- 副作用：boardTitle 存在时同一条文字渲染 3 次（副标题 + 头条 H2 + N°01）。flow 文档 `docs/reference/flow/daily-report.md:217` 已写「消费方 MUST NOT 把 title/summary 与 highlights 并排展示」。

## 「中东地缘政治与美伊关系」期出现中美关税头条的直接原因
中美关税簇当天去重篇数最多 → 候选首位 → 模型选中 → 因展示序固定为候选序，必然是 highlights[0]，同时成为头条、N°01、副标题；刊头 H1 是静态版块名，所以看起来「稿子跑题」。该簇在正文归在「美•中•贸易战」话题（active，显示在「关心的话题」区）。

## 可用字段（过滤/二次排序能拿到的）
- `DailyReportHighlight`（`front/app/api/dailyReports.ts:13-27` / Go `daily_report_models.go:460-486`）：只有 `title / reason / tag_ids / cluster_id? / article_count? / cluster_mode? / data_as_of? / research_at?`。**没有 score、tier、section_id、thread_id、topic_id**。
- `DailyReportSection`（`dailyReports.ts:315-346` / Go `:196-250`）：`article_count / best_tier / avg_score / quality_breakdown / persistent_topic_id / topic_match_distance / topic_match_confidence / topic_status_at_report / lane_tier / cluster_label / cluster_tag_ids / bucket_title / bucket_index`。
- `avg_score` / `best_tier` 语义是**版块/泳道准入质量**不是新闻价值：`best_tier = min(tagging.MatchTier(MatchReason))`、`avg_score = mean(tag.Score)`（`daily_report_orchestrator.go:237-259`、`daily_report_rebucket.go:355-377`），Score 混用两套量纲（direct_hit=1.0 / max_sim / weighted，或泳道召回 `1-余弦距离`，`orchestrator.go:874-882`）；watch 物化 section 硬写 `best_tier=4, avg_score=0`（`watch_materialize_keyword.go:161-174`）。跨 section 直接比不严谨。
- highlight → section 反查：用 `highlight.tag_ids`（或 `cluster_id`）与 `section.cluster_tag_ids` / `thread.tag_ids` 求交（前端已有 `reportHasClusterThread` `dailyReportMagazine.ts:148-152`、`collectClusterThreads` `:176-210`）；映射 1:N（一个簇可跨多个 bucket/section），需定「取重叠最多/最高分」规则。
- 现存「相关度」只有 `section.topic_match_distance`（到 persistent topic 质心锚定距离，`daily_report_models.go:233-237`）和 `thread.fit_distance`（`daily_report_thread_fit.go:83`），都不是「贴版块主题」语义。**生成期不存在「本期主题」对象**，没有任何 highlight 对主题的余弦/标签重叠计算。

## 改动会碰到的规格
- `docs/reference/flow/daily-report.md`：节「今日重点的选题与热度（story highlights）」`:191`（正文 `:197-217`）；红线 46 `:473`（候选口径 + 输出顺序按服务端候选序而非模型序）、红线 47 `:474`（聚簇只用于当天重点）；代码入口清单 `:520`。
- `openspec/specs/daily-report-system/spec.md`：`:562 ### Requirement: 今日重点聚簇候选`（排序硬写：文章数 DESC → 标签数 DESC → 代表标签 id ASC，最多 8）+ 场景「候选按簇内文章数截断」；`:595 ### Requirement: 当天同事件聚簇先筛重点候选`（同口径 + 「同分与精度不反转」）。`:183 ### Requirement: 聚类排序字段` 是标签名额竞争排序，与 highlights 无关，别误改。
- `openspec/specs/daily-report-interface/spec.md`：`:236 ### 日报时间线组件`（`:249-250` masthead 展示真实标题、头条优先 highlights[0]）、`:474 ### 刊头「今日重点」空态可见性`（`:492` 最多 3 条）、`:494 ### 重点依据与正文定位`。
- daily-report 域红线 7：匹配与阈值默认参数必须可经 `ai_settings` 在线覆盖（若新增门槛/阈值须遵守）。

**引用**：backend-go/internal/topicgraph/service/daily_report_story_highlights.go:finalizeDailyReportHighlights、backend-go/internal/topicgraph/repository/report_event_clusters.go:RankEventClusterCandidates、backend-go/internal/topicgraph/repository/report_story_highlights.go:FinalizeReportHighlights、front/app/features/tags/components/daily-report/dailyReportMagazine.ts:selectLeadStory、front/app/features/tags/components/daily-report/DailyReportMasthead.vue、docs/reference/flow/daily-report.md:191、openspec/specs/daily-report-system/spec.md:562、openspec/specs/daily-report-interface/spec.md:236

<!-- pinned 2026-11-27T14:00:48Z -->

## 现成「版块关联度」信号盘点（lane_tier / match_reason / highlight 生成上下文） <!-- pin:93ee43f0 -->

## lane_tier（簇级，非 tag 级）
- 取值常量 `daily_report_lane.go:22-29`：l1_direct / l2_llm / l3_new + watch_keyword / watch_sentence；"" → 列 NULL（`clusterLaneToTier` :341-353）。
- 赋值仅一处：`daily_report_orchestrator.go:274` `LaneTier: clusterLaneToTier(cluster.Lane)`；lane 标签来源 `assembleLaneGroups`(:289-337, 组内至少一个 L1 tag → "l1"，否则 "l2")、L3 组建组即 "l3"(:401,:455-461)、L2 判 new/约束违例 → `downgradeLaneGroupToNew`(:924-936) 置 "l3"，违例重裁保留 "l2"(:765)。
- 判据 = 纯「tag embedding 到 topic 锚点（centroid，缺则首义）余弦距离分带」+ topic 状态门禁（`BucketTagsByCentroid` :126-206）：dist<L1(0.18) 且 !vacuum 且 (gate 关或 topic active) → L1；dist≤L2(0.30) → L2（带 top-K 候选，交 LLM keep/switch/new）；其余/无 embedding/无锚点 → L3。默认值 `repository/daily_report_topic_repository.go:88-100`（L1 .18 / L2 .30 / L2CandidateK 5 / CandidateL1GateEnabled true），ai_settings 可覆盖(:234-278)。**不是**「tag→topic 归属路径」；direct/llm/new 是距离分带的后果。红线 2 = `docs/reference/flow/daily-report.md:416`。
- watch 轨：`service/watch_materialize_keyword.go:161-176`（watch_keyword，BestTier=4/AvgScore=0/无 embedding/PersistentTopicID NULL）、`watch_materialize_sentence.go:298-311`（watch_sentence，confidence=manual，distance=0）。`watch_` 前缀判定 `repository/daily_report_rebucket.go:475`。
- 落库时机：`SaveReport` 事务内 `assignAndUpdateTopics` → `planTopicAssignments`（`repository/daily_report_assignment.go:143-190`）：l1/l2 且有 MatchedTopicID → anchor_hit + distance=section↔该 topic 质心；l3/new → auto_new + distance=**到最近邻锚点**（诊断量，非新话题质心）；无 embedding → unmatched/distance 0。

## topic_tag_board_labels（tag↔版块挂载）
- 写方只有 `tagmanagement/service/board/semantic_board_matching.go:834-847 replaceTopicTagBoardLabels`（delete+insert），入口 `MatchTopicTag`(:106-156)。触发：(a) event tag embedding 完成自动匹配 `tagmanagement/service/core/embedding_queue.go:500-509`；(b) 管理/回填接口 `handler/board_match_handler.go:692`、`service/board/semantic_board_backfill.go:327`、`service/auxrepair/matching.go:179`；(c) 日报 fallback 现场匹配未挂载标签（`orchestrator.go:690-752`，**会写挂载**）。
- 算法 `evaluateSemanticBoardMatches`(:424-509)：composite_hit（tag 组合标签 ∩ board_composition 组合）score=1.0 且免方向校验；direct_hit（aux 重叠 ≥2 且占比 ≥0.5）score=0.7；间接规则 `scoreSemanticBoardSimilarity`(:809-832)：per-tag-aux 取到 board-aux 的最大余弦 → hitRate=hits/max(|aux|,3)、maxSim=全局最大；判定顺序 hitRate>0.5 → score=0.7*maxSim+0.3*hitRate；否则 maxSim≥0.8 且 hits≥min(2,|aux|) 且 hitRate≥0.3 → score=maxSim；否则 weighted=0.6*maxSim+0.4*hitRate ≥0.6 → score=weighted。默认参数 :853-871；MaxBoards=3(:152-154)。方向校验/豁免 :410-423。
- `MatchTier`（`handler/board_match_handler.go:636-646`，`tagmanagement/wire.go:156` 再导出）：direct_hit→0、hit_rate→1、max_sim→2、其余（weighted **与 composite_hit**）→3；SQL 复刻 :612-614。
- section 汇总：`orchestrator.go:237-259` bestTier=min(MatchTier)/无 tag 为 4、avgScore=mean(Score)；rebucket 重算 `daily_report_rebucket.go:355-377`；watch 段硬写 4/0。量纲混用：lane 召回标签 MatchReason="lane_recall"、Score=1-到泳道锚点距离（`orchestrator.go:878-881`），仍落 tier 3。
- 版块侧输入：`board_composition(board_id, auxiliary_label_id)` **无权重列**（`tagmanagement/models/tag_models.go:86-131`）；board identity embedding = embed(label+". "+description)（`handler/board_crud_handler.go:267-274`），**aliases 不进匹配**（只在 boardhealth core-subject 用：`service/boardhealth/metrics.go:165`）。
- 日报入口（当天命中）：`collectBoardTags`（`orchestrator.go:595-643`）= 挂载 JOIN + topic_tags.status='active' + category='event' + 当日 pub_date + 跨板排他 `semantic_board_report_exclusive_gap`(:628-637)；第二条入口泳道召回 `collectLaneRecalledTags`(:828-897, 只读挂载 :935)；第三条 fallback(:690-752)。

## highlight 生成期上下文
- 顺序：`GenerateAndSaveReport`（`service/daily_report_watch.go:46-81`）SaveReport(:72) → finalizeDailyReportHighlights(:80)；sections 及其 lane_tier/topic_match_distance/topic_status_at_report 已在 SaveReport 事务内落库（`daily_report_repository.go:350`）。
- finalize 只拿到 report(ID/SemanticBoardID/PeriodDate/UpdatedAt) 与 `inputs.tags []TagInput(ID/Label/ArticleCount/MatchReason/Score)`、`inputs.clusters []ClusterGroup(GroupName/TagIDs/MatchedTopicID/Decision/TargetTopicID/Lane)`（`daily_report_story_highlights.go:172-176`，orchestrator.go:116-119 填充）。**report.Sections 未填充**（orchestrator.go:181-195 构造 report 时无 Sections），lane 距离在 assembleLaneGroups 被丢弃。
- 簇→section 无现成映射。轻量 reader 需新增（仿 `repository/report_event_clusters.go:41-58 ListReportEventThreads`）：SELECT id, cluster_index, bucket_index, cluster_tag_ids, lane_tier, persistent_topic_id, topic_match_distance, topic_match_confidence, topic_status_at_report, best_tier, avg_score, article_count FROM daily_report_sections WHERE report_id=?。不要用 `GetReportByID`（Preload threads + foldSectionArticleCopies + resolveReportHighlightClusters 会改写 report.Highlights）。
- 1:N 真相：intra-lane bucketing 会把一个 cluster 拆成同 cluster_index 的 2-6 个兄弟 section，**共享 head 的 lane_tier / persistent_topic_id / topic_match_confidence**，topic_match_distance 各按自身 embedding 重算到同一锚点，best_tier/avg_score/cluster_tag_ids 各自独立（`repository/daily_report_rebucket.go:196-232`）。tag 只落一个桶，所以 `cluster_id ∈ section.cluster_tag_ids` 仍命中唯一 section。
- 候选排序：`EventCluster`（`repository/report_event_clusters.go:176-194`）字段 ClusterID/TagIDs/Label/Description/ArticleCount/Facts/Mode/DataAsOf/ResearchAt；`RankEventClusterCandidates`(:196-225) 文章数 DESC → 标签数 DESC → ClusterID ASC，截 8（:172）。链：finalize(:186) → buildDailyReportEventClusters(`service/daily_report_event_cluster.go:102`) → Rank(:200) → GenerateClusterHighlights(:47) → parseClusterHighlightsResponse(:90，按候选遍历输出) → FinalizeReportHighlights(`repository/report_story_highlights.go:76`)。断言测试：`repository/report_story_highlights_test.go:16`、`repository/report_event_clusters_test.go:39`、`service/daily_report_event_cluster_composite_test.go:33`、`service/daily_report_highlights_cluster_test.go:36`（:63-68 展示序=候选序）。

**引用**：backend-go/internal/topicgraph/service/daily_report_lane.go:BucketTagsByCentroid、backend-go/internal/topicgraph/service/daily_report_lane.go:assembleLaneGroups、backend-go/internal/topicgraph/service/daily_report_orchestrator.go:collectBoardTags、backend-go/internal/topicgraph/service/daily_report_watch.go:GenerateAndSaveReport、backend-go/internal/topicgraph/repository/daily_report_assignment.go:planTopicAssignments、backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:evaluateSemanticBoardMatches、backend-go/internal/tagmanagement/handler/board_match_handler.go:MatchTier、backend-go/internal/topicgraph/service/daily_report_story_highlights.go:finalizeDailyReportHighlights

<!-- pinned 2026-11-27T14:13:13Z -->

## 版块关联度现成信号：lane_tier + topic_status_at_report（含 15 期回放） <!-- pin:49f0625b -->

## 结论：用 section 的 `lane_tier` + `topic_status_at_report` 当「版块关联度」，现成、无需新增向量计算
- `lane_tier`（`backend-go/internal/topicgraph/service/daily_report_lane.go:22-29`，`clusterLaneToTier` :341-353）取值：`l1_direct` / `l2_llm` / `l3_new` / `watch_keyword` / `watch_sentence`，空串写 NULL。**判据 = 到版块已成立泳道质心的余弦距离分带 + topic 状态门**（`BucketTagsByCentroid` :126-206）：`dist<0.18 && !vacuum && topic active` → L1；`dist≤0.30` → L2（top-K 候选交 LLM）；其余/无 embedding/无锚点 → L3。默认阈值 `.18/.30/K=5/gate=true` 在 `repository/daily_report_topic_repository.go:88-100`，`ai_settings` 可覆盖（:234-278）。direct/llm/new 是分带的后果不是输入。
- 落库：`SaveReport` 事务内 `assignAndUpdateTopics → planTopicAssignments`（`repository/daily_report_assignment.go:143-190`）把 lane 结果转成 `topic_match_confidence`（anchor_hit/auto_new/unmatched）+ `topic_match_distance` 写进 `daily_report_sections`。`l3_new` 的 distance 是到最近邻锚点的诊断量。人工 detach 保留 lane_tier。
- intra-lane bucketing 的兄弟 section 共享 head 的 `lane_tier / persistent_topic_id / confidence`（`daily_report_rebucket.go:196-232`），只有 `best_tier/avg_score/cluster_tag_ids` 分家 → 簇→section 用 `cluster_id ∈ section.cluster_tag_ids` 定位唯一桶即可（注意 `cluster_tag_ids` 是数字数组，jsonb `?` 文本比较对数字元素恒 false，要用 `jsonb_array_elements_text`）。

## 为什么不用其他量
- `avg_score`：量纲混用（标签↔版块 score 与 `1-余弦距离`），watch 段恒 0，跨 section 不可比。
- 标签↔版块 `topic_tag_board_labels.score/match_reason`：`weighted` 阈值 ≈ 0.6×maxSim（`tagmanagement/service/core/semantic_board_matching.go:834-847` 写入，入口 `MatchTopicTag` :106-156），是「至少一个辅助标签够近」的闸门，不是主题相关度。实测 board 1974：188 个绑定里 131 个是 weighted 0.60–0.73，贴题伊朗新闻与中美关税同档，无区分力。
- `MatchTier` 映射（`tagmanagement/handler/board_match_handler.go:636-646`）：direct_hit→0、hit_rate→1、max_sim→2、其余（weighted/composite_hit）→3。

## 文章进版块日报的入口（`collectBoardTags`，`service/daily_report_orchestrator.go:595-643`）
JOIN 挂载表 `topic_tag_board_labels` + `topic_tags.status='active' AND category='event'` + 当日 `pub_date` + 跨板排他 `semantic_board_report_exclusive_gap`（:628-637）。第二入口：泳道召回 `collectLaneRecalledTags`（:828-897，只读挂载表做排他 :935，不写）；第三入口：未挂载标签 `Limit(50)` 现场匹配 fallback（:690-752，会写挂载）。

## 重点生成时内存里有什么（`service/daily_report_story_highlights.go:172-176`）
- `report`（ID/SemanticBoardID/PeriodDate/UpdatedAt）+ `inputs.tags []TagInput`（MatchReason/Score/ArticleCount）+ `inputs.clusters []ClusterGroup`（Lane / MatchedTopicID / TagIDs；填充点 `orchestrator.go:116-119`）。**`report.Sections` 为空**（orchestrator 构造 report 时不带 Sections，:181-195），lane 距离在 `assembleLaneGroups` 里被丢弃。
- sections 已在 highlights 之前落库：`GenerateAndSaveReport`（`service/daily_report_watch.go:46-81`）先 `SaveReport`(:72) 再 `finalizeDailyReportHighlights`(:80)。
- 最少要加的查询：仿 `ListReportEventThreads`（`repository/report_event_clusters.go:41-58`）做轻量投影，如 `ListReportSectionSignals(reportID)`：`SELECT id, cluster_index, bucket_index, cluster_tag_ids, lane_tier, persistent_topic_id, topic_match_distance, topic_match_confidence, topic_status_at_report, best_tier, avg_score, article_count FROM daily_report_sections WHERE report_id=? ORDER BY cluster_index, bucket_index`。**不要用 `GetReportByID`**（Preload threads + fold 副本 + `resolveReportHighlightClusters` 会当场改写 `report.Highlights`）。
- 调用链：`finalizeDailyReportHighlights`(:186) → `buildDailyReportEventClusters`（`daily_report_event_cluster.go:102`）→ `RankEventClusterCandidates`(:200) → `GenerateClusterHighlights`(:47) → `parseClusterHighlightsResponse`(:90，按候选顺序输出并 break 3) → `FinalizeReportHighlights`（`repository/report_story_highlights.go:76`）。
- `EventCluster` 结构（`repository/report_event_clusters.go:176-194`）：`ClusterID / TagIDs / Label / Description / ArticleCount / Facts / Mode / DataAsOf / ResearchAt`——目前没有可挂 relevance 的字段。
- 前端：纯后端改顺序 = 前端零改动（`DailyReportMasthead.vue:27` 直接 `highlights.slice(0,3)` 按arrays序渲染；`front/app/api/dailyReports.ts:13-27` 是纯 TS interface 无运行时校验）。只有要展示新快照字段时才需加可选字段 + masthead `metaText`。

## 改排序会红的测试
- `repository/report_story_highlights_test.go:16 TestRankEventClusterCandidatesOrderAndLimit`（排序键 + Top8，注释明确禁止用 section 数/泳道热度替代篇数——改的是**输出序**不是候选序的话此测试可不动的可能性需确认）
- `repository/report_event_clusters_test.go:39 TestListReportEventThreadsIncludesComposite`（:100-105 大簇排首位）
- `service/daily_report_event_cluster_composite_test.go:33 TestDailyReportEventClusterCompositeBigThreadRanksFirst`
- `service/daily_report_highlights_cluster_test.go:36 TestHighlightsClusterPromptAndSnapshot`（:63-68「展示顺序 = 候选顺序」、:87 / :148 / :161）
- 规格：`openspec/specs/daily-report-system/spec.md:562/:595`；flow `docs/reference/flow/daily-report.md` 红线 46（:473）。

## 历史回放（只读 SELECT，board 1974「中东地缘政治与美伊关系」，2026-11-13 ~ 11-27 共 15 期）
按「簇成员标签所在 section 的 lane_tier 并集 / topic 状态并集」，低相关的头条/前三位：
- 11-27 #1 中美关税 = l2_llm/candidate（另两条 l1_direct/active）→ 应降到 #3
- 11-26 #1 美国对多国加征关税 = l2_llm/candidate(3 篇)；#3 商务部反对301关税 = l2_llm/candidate；#2 胡塞袭沙特 = l1_direct+watch_sentence/active → 胡塞应升 #1，两条关税仍占 #2/#3（模型挑了 3 条里 2 条关税——**只改顺序赶不走**）
- 11-23 #3 美国11月标普全球PMI初值 = l3_new/candidate（已末位）
- 11-19 #2 美国国安局IT系统故障 = l3_new/candidate → 应降到 #3；#3 国际能源署评估伊朗战争影响 = watch_sentence/active
- 11-14 #1 美国发布全国性航班停飞指令 = l3_new/candidate → 应降到 #3；#3 中国代表提出中东问题四点主张 = l1_direct+l3_new（跨两个 section，1:N）/active+candidate
- 11-13 #3 伊朗革命卫队称LNG船触雷爆炸 = l2_llm/active（贴题但走 LLM 档，1 篇，仍 #3）
- 11-22 两条 `topic_status_at_report` 为空（历史快照缺失）→ 规则须把缺失当中性不惩罚
- 其余 40+ 条均 l1_direct/active，顺序不受影响

## 待产品拍板
1. `watch_*` 命中的重点排哪（与正文一致排末位 / 一档之后二档之前 / 最前）。
2. 是否同时让 LLM 挑选时参考贴题度（否则 11-26 这类「3 条里 2 条跑题」只改序不消失）。

**引用**：backend-go/internal/topicgraph/service/daily_report_lane.go:22、backend-go/internal/topicgraph/service/daily_report_orchestrator.go:595、backend-go/internal/topicgraph/service/daily_report_story_highlights.go:172、backend-go/internal/topicgraph/repository/report_event_clusters.go:176、backend-go/internal/topicgraph/repository/report_story_highlights.go:76、backend-go/internal/topicgraph/service/daily_report_watch.go:46、backend-go/internal/tagmanagement/handler/board_match_handler.go:636

<!-- pinned 2026-11-27T14:15:32Z -->

## 方案调整：先筛后挑（候选池按贴题档）+ 15 期贴题板块数

## 用户追问后的方案调整（2026-11-27）：从「挑完再排」改为「先筛后挑 + 挑完再排」
用户原话：「为啥是模型先选，那选出来不就那3条么」——指出只改输出序赶不走跑题的（11-26 模型挑的 3 条里 2 条是关税）。拍板：**先筛后挑**；贴题池 = **近档 + 手动关注**；不足 3 个按「中档-在跟 → 中档-候选 → 远档」补位；挑完仍按贴题档排一遍兜底。

## 为什么现在是模型先挑（历史决策，别误删）
`openspec/changes/archive/2026-11-20-retire-story-adopt-event-metrics/design.md:110` D13「今日重点仍调 LLM，候选源换聚簇」：**否决「不用 LLM，直接按簇内文章数取前 3」——重点的「为什么值得看」是语义判断，纯排序会退化成条数榜**。所以新方案保留模型挑选，只是把贴题门槛放在挑选之前。同一 change 也定了「候选排序不用版块 section 数或泳道热度替代篇数」——新方案不违反：section 数/泳道热度仍不参与，贴题档是相关度不是热度，同档内仍按文章数。

## 近 15 期每日「贴题板块数」（只读 SELECT，board 1974，按 section 口径近似 cluster）
日期 | 近档 l1 | 手动关注 watch | 中档-在跟 | 中档-候选 | 远档 l3
11-27 | 3 | 0 | 0 | 1 | 3
11-26 | 2 | 1 | 1 | 1 | 2
11-25 | 4 | 1 | 0 | 0 | 5
11-24 | 4 | 0 | 0 | 0 | 0
11-23 | 5 | 0 | 0 | 1 | 3
11-22 | 5 | 0 | 0 | 0 | 1
11-21 | 4 | 0 | 1 | 0 | 0
11-20 | 3 | 0 | 0 | 0 | 3
11-19 | 4 | 1 | 0 | 0 | 3
11-18 | 4 | 0 | 0 | 3 | 3
11-17 | 3 | 0 | 0 | 0 | 0
11-16 | 4 | 0 | 0 | 0 | 4
11-15 | 4 | 0 | 0 | 1 | 1
11-14 | 3 | 0 | 0 | 0 | 4
11-13 | 2 | 0 | 1 | 0 | 0
结论：贴题板块（l1 + watch）≥3 的有 14/15 期；只有 11-13 是 2 个（中档-在跟那条「伊朗革命卫队称 LNG 船触雷爆炸」补位，结果与现状一致）。注意 section 数是 cluster 数的上界（同泳道分桶会把一个 cluster 拆成共享 cluster_index 的兄弟 section），且重点候选是 event 标签向量聚簇、与 section 不是同一套聚类——回放是近似。

## 11-26 当天板块明细（先筛后挑的效果依据）
0 | 伊朗在霍尔木兹海峡袭击违规船只并警告勿轻信美国 | l1_direct | active | 1 | [302619]
1 | 胡塞武装无人机袭击沙特最大炼油厂 | l1_direct | active | 1 | [302640]
2 | 以色列法官建议检方撤销对内塔尼亚胡4000案受贿诉讼 | l2_llm | active | 1 | [303446]
3 | 美方拟对14个经济体加征301关税，多方回应 | l2_llm | candidate | 5 | [302178, 302695, 302871, 303094, 303758]
4 | 美国以301条款调查为由拟对巴西飞机加税并削减糖配额 | l3_new | candidate | 1 | [302082]
5 | 库什纳称普京向美方保证无意在乌克兰使用核武器 | l3_new | candidate | 1 | [302119]
6 | 胡塞袭沙特炼油厂推升油价风险 | watch_sentence | active | 1 | [301867]
→ 贴题簇 3 个（2 个 l1 + 1 个 watch）≥3，两条关税簇都不进候选池。

## 统一的「贴题档」键（先筛与后排共用）
0 = `l1_direct`；1 = `watch_` 前缀；2 = `l2_llm` 且 `topic_status_at_report` 非 candidate（在跟或缺失）；3 = `l2_llm` 且 candidate；4 = `l3_new`；5 = 正文无含该簇标签的板块 / 板块无泳道档。
取法：以候选簇**完整成员标签**（并上代表标签）对照本期各板块 `cluster_tag_ids`（jsonb 数字数组，SQL 侧用 `jsonb_array_elements_text`，勿用 `?`），命中的板块里取档最小者；同档取非候选。

## 候选池规则
- 贴题簇（档 0–1）≥3 → 池子只含贴题簇，按既有候选序取至多 8。
- 贴题簇 1–2 个 → 池子 = 全部贴题簇 + 补位（其余聚簇按贴题档升序、同档按既有候选序）补到共 3 个。
- 贴题簇 0 个 → 维持现状（既有候选序前 8）——典型是新板子还没有已成立话题/关注，无从判贴题。
- 模型挑选、提示词、校验重试、快照全部不变；挑完按贴题档稳定排序兜底。
- 读板块信号失败 → 池子与展示序都退回现状 + Warn，不阻断。
- 候选池的选择器在 `RankEventClusterCandidates`（`repository/report_event_clusters.go:196-225`，唯一调用点 `service/daily_report_story_highlights.go:201`）之上包一层；既有候选序 comparator 保留不动（`TestRankEventClusterCandidatesOrderAndLimit` 预期不改）。

**引用**：backend-go/internal/topicgraph/service/daily_report_story_highlights.go:finalizeDailyReportHighlights、backend-go/internal/topicgraph/repository/report_event_clusters.go:RankEventClusterCandidates、openspec/changes/archive/2026-11-20-retire-story-adopt-event-metrics/design.md

<!-- pinned 2026-11-27T14:37:03Z -->
