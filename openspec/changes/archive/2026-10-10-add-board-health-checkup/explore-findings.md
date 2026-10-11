
## 体检 change 关键落点速查

add-board-health-checkup 关键落点（探索核实）：
1. 迁移注册：AutoMigrate via tagmanagement/wire.go init() database.RegisterModels(...)（tagmodels）+ testutil.go ~L477 镜像列表；版本化迁移在 internal/platform/database/postgres_migrations.go postgresMigrations() 尾部 append。
2. 体检包：internal/tagmanagement/service/boardhealth/（对标 sourcestats：口径唯一实现+只读）；cosine 用 board.CosineSimilarity（semantic_board_matching.go:724，已导出）；ParsePgVector 在 auxlabel 包。
3. 日报挂载点：job_daily_report.go DailyReportJob L213；成功裁决点 adjudicateDailyReportTerminal L294-296 调用后、resultData 构造前插体检尾步；手动路径 TriggerNowWithDate→executeWithDate→DailyReportJob(targetDate) 自动同路径（无需单独接入，L531）。
4. 扩充召回：semantic_board_expand.go recallExpandAuxCandidates L171（签名 (ctx, config, profile *boardExpandProfile, days) 返回 ([]expandAuxCandidate, map[uint]struct{}, error)）；已挂载滤 L176-186；相似路 days>0 批量过滤 labelIDsRecentlyActive；LLM 接口 SemanticBoardUpgradeLLM.SuggestSemanticBoardUpgrades(ctx, prompt, mode)，生产实现 default_llm.go（airouter.NewRouter().Chat, JSONMode, jsonutil.SanitizeLLMJSON）。
5. 通知：notification 包白名单设计（无通用 Create，导出面只有 DailyReportSuccess/DailyReportFailedSummary）；Notification model: Type(success|error)/Title/Summary/LinkType/LinkID；需新增 BoardHealth 落通知入口（LinkType=board, LinkID=board_id 字符串）。
6. ai_settings 模式：board 包 LoadUpgradeConfig L600 先默认后 Where key IN ? 覆盖（parseSemanticBoardUpgradeInt/Float 回退默认）。
7. 端点：routes.go RegisterSemanticBoardRoutes→board_crud_handler.go L117-155；只读端点样板 board_source_breakdown_handler.go（ParseWindow 白名单→400, ErrBoardNotFound→404）。
8. Embedding：tag identity = topic_tag_embeddings (topic_tag_id, embedding_type='identity')；board embedding = semantic_labels.embedding *string；TTSBL=topic_tag_semantic_labels（tag→aux 多对多）；TTBL=topic_tag_board_labels（tag→board 匹配行，PK tag_id+board_id，含 MatchReason/Downgraded/DirectionMismatch）；article_topic_tags（article↔tag）。

<!-- pinned 2026-10-02T14:17:10Z -->

## 体检评分 prompt 方法学锚（insufficient≠失效）

2026-10-03 用户反馈「评分低于预期」排查结论：
1. matched_tags 不是只看当天——loadMatchedRows 数的是 topic_tag_board_labels 全量现状（215=侧栏 216 一致）。
2. 真正砸分原因：单 aux 纯净样本占比天生极低（1.4%~6.8%，多数 tag 挂多 aux 是正常形态）→ labels 全部 insufficient → 旧 system prompt 把「大量构成标签证据不足」明确列为 <60 判据 → LLM 直落 35-45 且评语误读为「内容极度匮乏」。
3. 修复（llm.go，口径红线 22 不动）：system prompt 增「统计口径」节（insufficient=证据缺失≠标签失效/内容匮乏；single_aux_tag_ratio 低是数据形态）+ <60 判据改为真实病征（ghost 占比高/有样本 cosine 普遍<0.5/matched 萎缩/弱路径主导且 composite 近零）+ user prompt 加口径提醒行。
4. 真 LLM 复测（同快照重打分）：中东 35→58（ghost 25% 真病征保留）、生成式AI 45→65、俄乌 45→85。notify=true 但 score≥60 的由 shouldNotify 阈值闸门兜住不通知。
5. 重验方式：already_exists 早退路径不走体检尾步（failedCount==0 成功生成轮才跑），近几天都有日报无法重触发；当晚自然轮生效。曾用 LIVE_RESCORE=1 临时 go test（真库只读+真 LLM 不落库）验证后已删。

<!-- pinned 2026-10-03T14:57:14Z -->

## 体检口径修正实测数据：单 aux 口径失效证据 + 贴合度口径验证结果

背景：体检标签级统计旧口径（单 aux 纯净样本）真库实测结构性失效，改贴合度口径（构成标签 embedding × 全部挂载 tags identity embedding 的 cosine 分布）。

旧口径失效证据（2026-10-07 真库实测）：
- 全库 6874 个单 aux tags，仅 68 个（1%）匹配到任何版块。机制：匹配选择效应——多 aux tag 有多次 direct_hit 机会；单 aux tag 的唯一 aux 多为长尾（每个 aux 仅挂 2-3 个当天 tags，不在任何版块构成里）。
- 现挂载集单 aux 占比仅 2%-11%，且集中 2026-09-28 回填日带入的历史 tags（多为 2026-06-14 前后创建）；9-29 后每日新增单 aux 匹配行 ≈ 0（10-06 全天 695 条新行仅 2 条单 aux）。样本池永不增长。
- 板块 3030 实测：96 构成标签仅 6 个有分布（samples 合计 17），single_aux_ratio=0.086。
- 匹配管道为增量：embedding 队列驱动（文章入库→打标→算 embedding→立即 MatchTopicTag，semantic_board_matching.go:704 replaceTopicTagBoardLabels per-tag delete-then-insert），历史 tags 仅 9-28 回填一次（semantic_board_backfill）。topic_tag_board_labels 表共 2294 行（9-09~10-07 累积）。

新口径实测验证（pgvector SQL 全量模拟，五个板块）：
- 样本全覆盖：板块 3030 96 标签 × 444 样本/标签（identity 向量齐的挂载 tags）。
- 判别力：3030 cos_mean 极差 0.544~0.730，top=DeepSeek Harness/Codex/MCP/Vibe Coding（板块核心主题），bottom=kprobe/SQLite/Vue3/Spring Boot（外围技术栈）——排序语义人工可验证。五板块：96 标签极差 0.19、57 标签 0.21、36 标签 0.17、18 标签 0.08（小板块主题纯信号弱，可接受）、7 标签 0.32。
- 低分线必须改板块内相对线（cosine_mean 的 P25）：固定 0.5 线失效，tag×aux cosine 底噪高（板块内 min 0.51+），低分占比恒 0%。
- 外部基线重叠（板块外随机标签 mean 0.571 vs 板块内均值 0.653）可接受：体检是板块内互比排序，不做板块内外判别。
- 技术事实：embedding 列为 pgvector vector 类型（2560 维，aux/tag_identity/board 三类同维度同空间），SQL 可直接 `1-(a <=> b)` 算 cosine 验证；Go 侧经 GORM 扫 string 后 auxlabel.ParsePgVector。

实现改动落点（tasks §10）：
- computeLabelAttribution 重写：去 AuxCount/SingleAuxID 依赖，每构成 aux 对全部有 identity 向量的挂载 tags 算 cosine 分布；删 SingleAuxTagRatio（models/board_health.go:45 + 前端 BoardHealthPanel.vue:307 chip + DTO）；loadDirectionSimThreshold 引用移除（低分线改板块内 P25）；构成标签缺自身 embedding → 该标签 insufficient（不进 vector_missing）。
- 历史快照 labels 字段语义变化接受（趋势只画 llm_score；latest 永远最新口径）。

**引用**：backend-go/internal/tagmanagement/service/boardhealth/metrics.go:computeLabelAttribution、backend-go/internal/tagmanagement/models/board_health.go:SingleAuxTagRatio、front/app/features/tags/components/BoardHealthPanel.vue:307

<!-- pinned 2026-10-07T13:04:27Z -->
