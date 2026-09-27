
## 泳道动态数据底座与方案乙细节

泳道动态底层数据事实（探索已核实）：

**泳道模型**：`board_persistent_topics`（status: candidate|active|archived；last_seen_date/hit_count/consecutive_hits），section 经 `daily_report_sections.persistent_topic_id` 锚定（LaneTier: l1_direct/l2_llm/l3_new/watch_keyword/watch_sentence），thread 叙事在 `daily_report_threads`（Title/Summary/RelatedArticleIDs jsonb）。watch = `board_topic_watches` 四类，keyword_topic/sentence_topic 是物化轨（每天追加真实 section）。

**lifeline 现状（topic_lifeline_context 表，dataenrichment/repository/models.go:66-91）**：UNIQUE(persistent_topic_id, granularity, period)；Content 是 LLM 三段式纯文本（【事件脉络】【关键动向】【涉及主体与领域】，非结构化）；**week 档定时已停用**（runtime.go:252-272 整块注释，生产仅 2 行，停用理由=近端记忆由 14 天 section 窗口承担+周舰队自愈突发风险）；month 档 = 每月1号03:30定时全量 + 分析时补全门 72h 重算（freshness_gate.go，限 40 次 LLM）+ 手动 regenerate——月中打开可能滞后近一个月；year 同理。
- 生成链：RefreshPeriod → refreshArchive（lifeline_context.go:203-237）→ ReadSections(topicID, from, to)（production_wiring.go:47-103，素材=区间内 sections 的「日期+cluster_label+前5条thread标题」纯文本，不含正文）→ summarizeArchive 单次 LLM（:288-306）
- situation_cards.go:54-79 LaneSituationCard（FactsDigest 120rune 压缩串/FactsSource 枚举），取材链 week→month→section指纹→description→none（:184-230）；recentLaneSections 查询（:243-257，现窗口 3 天，放宽到 14 天即可复用为「要点」数据源）；每板最多12卡
- **无 board 级批量端点**：现只有逐 topic 的 GET /api/persistent-topics/:topicId/enrichment/contexts（handler.go:124-131），前端 boardEnrichment.ts:864-905 消费；前端 daily-reports.ts:236 getTopicLifeline 是另一套（sections 时间线）勿混淆

**方案乙要点（用户已拍板）**：日报管线尾部（SaveReport 后）异步结算每板块活跃泳道「滚动14天一句话态势」（≤100字），输入=近14天 thread 标题列表小 prompt，新滚动快照表每泳道一行覆盖更新，失败不阻塞日报；不恢复 week 定时。要点=近14天 sections thread 标题机械聚合（零 LLM）。候选栏=达门槛可见 candidate（topics API 的 FilterVisibleTopics：hit_count≥UpgradeThreshold）只读列表。topic-landscape API 唯一消费方是 TopicLandscapePanel（删除安全）；「点卡片→话题总览focus」联动在 TagsPage.vue:87-94 handleLandscapeSelectTopic，新卡片保留此联动；空态「生成日报」入口（TopicLandscapePanel 内 WS 进度模式）也要保留到新视图。

<!-- pinned 2026-09-09T15:03:38Z -->
