## Context

上次变更 (`2026-05-23-semantic-label-board-system`) 建立了辅助标签→语义板块→叙事板的完整链路。后端匹配逻辑在 `semantic_board_matching.go`，叙事生成在 `narrative/board_narrative_generator.go`，前端板块管理在 `/tags` 页面，叙事展示在 `/topics` 的 NarrativePanel 中。

当前叙事板按 scope_type (global / feed_category) 分别生成，每个 SemanticBoard 在不同 scope 下产生多份叙事。前端 NarrativePanel 位于 /topics 页面的独立 tab 中，与 /tags 的板块管理完全分离。

核心文件引用:
- 匹配: `backend-go/internal/domain/tagging/semantic_board_matching.go`
- 匹配公式: `backend-go/internal/domain/tagging/semantic_board_matching.go` (`scoreSemanticBoardSimilarity`, `evaluateSemanticBoardMatches`)
- 升级建议: `backend-go/internal/domain/tagging/semantic_board_handler.go` (DTO: `semanticBoardUpgradeSuggestionDTO`)
- 叙事生成: `backend-go/internal/domain/narrative/board_narrative_generator.go` (prompt: `boardNarrativeSystemPrompt`)
- 叙事服务: `backend-go/internal/domain/narrative/service.go`
- 文章 API: `backend-go/internal/domain/article/handler.go` (`GetArticles`, 支持 concept_id/auxiliary_label_id/feed_id/start_date/end_date)
- 刷新: `backend-go/internal/domain/feed/handler.go` (`refreshAllFeedsWorker`), `front/app/features/shell/components/FeedLayoutShell.vue` (`onMounted`)
- WebSocket: `backend-go/internal/platform/ws/hub.go` (`Hub`, `BroadcastRaw`), 前端 composable 模式 (`useTagWebSocket.ts`, `useWebSocketRebuild.ts`)
- 定时任务: `backend-go/internal/jobs/` (`scheduler_tasks` 表, `narrative_summary` 任务)
- 日报新模块: `backend-go/internal/domain/daily_report/` (新建)
- 前端板块页: `front/app/features/tags/components/TagsPage.vue`
- 升级建议面板: `front/app/features/tags/components/UpgradeSuggestionPanel.vue`
- 叙事面板: `front/app/features/topic-graph/components/NarrativePanel.vue`
- 图谱页: `front/app/features/topic-graph/components/TopicGraphPage.vue` (activeTab='narrative')
- API client: `front/app/api/semanticBoards.ts`

## Goals / Non-Goals

**Goals:**
- 消除 max_sim 单因子匹配导致的跨域误挂
- 升级建议展示人类可读的标签名称和板块名称
- 板块详情页提供完整的交互闭环：composition → 叙事时间线 → 文章列表（含筛选）
- 叙事功能统一到 /tags 页面，取消 scope 分类，每个 board 每天一份叙事
- /topics 页面专注于图谱和热点分析
- Feed 刷新和页面加载体验优化（并行化）
- 匹配计算过程可视化，让用户理解"为什么这篇文章在这个板块"
- 日报生成异步执行，不阻塞 UI，提供实时 WebSocket 进度反馈
- 日报系统提供结构化的每日行业总结（今日重点、板块动态、聚类叙事线索），替代旧的叙事系统

**Non-Goals:**
- 不重写旧叙事生成的 LLM prompt 核心逻辑（已废弃的 scope 相关调整已完成；日报系统使用全新的 prompt 设计）
- 不做历史叙事数据的自动迁移（旧 scope 数据保留但新逻辑不再生成 scope 变体）
- 不改变辅助标签提取/入库流程
- 不改变 NarrativeBoard 数据模型核心字段（scope_type 保留但语义变更）
- 不做叙事内容的编辑/修正能力（后续迭代）
- 不重写 feed 刷新的核心逻辑（只改并发调度）
- 不改变匹配算法本身（只增强展示）
- 不迁移旧 NarrativeBoard/NarrativeSummary 的历史数据（保留只读）
- 日报系统不做跨板块关联分析（后续迭代）

## Decisions

### D1: max_sim 规则加 hits + hit_rate 双因子

**决策**: `max_sim ≥ direct_max_sim` 分支新增两个必要条件：`hits ≥ min(2, N)` 且 `hit_rate ≥ direct_max_sim_min_hit_rate`（默认 0.3）。N 为 tag 的辅助标签总数。当 N=1 时 min(2,1)=1，实质退化为原规则加强版。

**理由**: 当前 max_sim 只看单个最高相似度，不要求"有几个辅助标签支持这个匹配"。辅助标签数量固定在 1-5 个（keyword=1, event/person=3-5），hits≥2 在 N=5 时要求 40% 命中率，足以过滤跨域误匹配。

**边界行为**:
- N=1 (keyword 直入): hits≥1 + rate≥0.3 + sim≥0.8 → 与原规则基本一致
- N=2: hits≥2 + rate≥0.3 + sim≥0.8 → 两个辅助标签都需过阈值
- N=5: hits≥2 + rate≥0.3 + sim≥0.8 → 至少 2 个(40%)有信号

**备选方案**: 动态 hits 阈值 max(2, ceil(N×0.4)) → 被否决，因为 N 不会超过 5，动态阈值无额外收益反而增加复杂度。

### D2: 升级建议 DTO 新增 label 字段

**决策**: `semanticBoardUpgradeSuggestionDTO` 新增 `auxiliary_labels []struct{id, label}` 和 `target_board_label string`。保留原有 `auxiliary_label_ids` 和 `target_board_id` 用于执行操作。`suggestionsToDTO` 中从 DB 批量查询 semantic_labels 获取 label。

**理由**: 前端只需要展示用 label，执行操作仍需 ID。新增字段而非替换，保持向后兼容。

### D3: 新增独立的 Board 文章列表 API

**决策**: 新增 `GET /api/semantic-boards/:id/articles`，不修改通用 `GetArticles` handler。支持 feed_id/start_date/end_date/auxiliary_label_id/page/per_page 参数。每篇文章返回 feed_name（JOIN feeds）和 filtered_tags（通过 topic_tag_board_labels 过滤，只返回属于当前 board 的 event/person/keyword 标签）。

**理由**: 通用 `GetArticles` 是全局接口，改动影响面大。独立 API 可以定制返回结构（filtered_tags 按板过滤），隔离影响。

**filtered_tags 逻辑**: 
```
article → article_topic_tags → topic_tags
                                    ↕ topic_tag_board_labels (semantic_board_id = :id)
                                 只返回有 board 归属记录的 tags
```

### D4: 叙事生成取消 scope 分类

**决策**: 每个 SemanticBoard 每天只生成一份叙事，不再区分 global/category scope。叙事生成时收集该 board 下所有 event tags（不限 category），生成单一叙事集。NarrativeBoard.scope_type 保留字段但新数据统一使用 "board" 值。旧数据中的 global/feed_category 值保留不做迁移。

**理由**: 板块的叙事是主题维度的，不应该被 feed category 切割。取消 scope 简化了生成逻辑，也简化了前端展示。同一事件可能来自多个 feed/category，混合在一份叙事中更符合"叙事"本身的含义。

**影响**: `service.go` 中生成调度逻辑需去掉 scope 循环；废弃 `SaveNarrativesForBoard`（硬编码 feed_category），统一使用 `saveNarrativesWithBoard`（通用版）写入 scope_type="board"；`CollectSemanticBoardNarrativeInputs` 需移除 scopeType/categoryID 参数和 category JOIN，收集所有 event tags；`matchPreviousSemanticBoard` 仅按 semantic_board_id + 前一日日期匹配续接。叙事 generation 统一使用 `resolveGeneration(out, date)`（原 category 路径），不使用 `resolveGlobalGeneration`。

### D5: 叙事功能迁移到 /tags 页面

**决策**: 新增 `BoardNarrativeTimeline.vue` 组件嵌入 TagsPage 的 board 详情区域（composition 下方、文章列表上方）。每条叙事展示为"小文章卡片"：status 标签 + 日期 + 标题 + 摘要 + 关联标签 + 文章数。点击展开可查看关联文章列表（复用文章预览弹窗）。/topics 页面的 NarrativePanel 和叙事 tab 完全删除。

**注**: 此决策对应已完成的中间态（Group 5-8）。日报系统（D12-D17）将 `BoardNarrativeTimeline` 替换为 `BoardDailyReportTimeline`，TagsPage 改为 Tab 切换模式（板块内容 / 日报 / 文章）。

**理由**: 用户的认知模型是"选一个板块→看它的故事"，叙事和板块管理应该在同一个页面。/topics 专注于图谱可视化和热点分析。

**数据流（日报系统替代后）**:
```
选中 board → Tab 切换到"日报" → loadBoardDailyReports(boardId, days=7)
          → GET /api/semantic-boards/:id/daily-reports?days=7
          → 返回 [{date, title, summary, status, cluster_count, article_count}]
          → BoardDailyReportTimeline 渲染日报卡片列表
          → 点击卡片 → 展开：highlights + dynamics + 聚类叙事线索
```

### D6: Board 叙事时间线 API 设计

**决策**: `GET /api/semantic-boards/:id/narratives?days=7` 查询 narrative_summaries 表，按 semantic_board_id（通过 narrative_boards.semantic_board_id JOIN）+ period_date 倒序。返回每条叙事的 title/summary/status/related_tags（带 label）/related_article_ids（关联文章 ID 列表）/scope_type/article_count/date。

**理由**: 叙事数据已存在 narrative_summaries 表中，通过 board_id 关联即可查询。days 参数控制回溯天数，默认 7 天。

### D7: /topics 叙事 tab 删除策略

**决策**: 删除 TopicGraphPage.vue 中的 activeTab 状态、叙事 tab 按钮、NarrativePanel 组件引用。同时删除 NarrativePanel.vue 和 NarrativeBoardCanvas.vue 文件（它们仅被 TopicGraphPage 的叙事 tab 引用，删除 tab 后成为死代码）。

**理由**: 一次性清理比留死代码更干净。NarrativePanel.vue 和 NarrativeBoardCanvas.vue 的唯一引用方就是 TopicGraphPage 的叙事 tab，删除 tab 后这些文件不会被任何地方引用。

### D8: 文章匹配度展示

**决策**: Board 文章列表 API 的 `filtered_tags` 携带 `match_reason` 和 `score` 字段（来自 `topic_tag_board_labels` 表的已有数据）。前端在每个 tag chip 上用 tooltip 显示匹配信息："max_sim · 0.85" 或 "direct_hit · 1.00"。不占用常规布局空间，hover 即看。

**理由**: 用户需要理解"为什么这篇文章在这个板块"。数据已在 `topic_tag_board_labels` 中存储（Score + MatchReason），零额外存储成本。展示粒度为 per-tag（文章通过多个 tag 归属到 board，每个 tag 有独立匹配原因），最精确。

**match_reason 可视化**:
- `direct_hit` → 标签完全匹配板块辅助标签 → 显示「直接命中」
- `hit_rate` → 命中率超过阈值 → 显示「命中率 0.75」
- `max_sim` → 最高相似度超过阈值 → 显示「相似度 0.85」
- `weighted` → 加权综合分达标 → 显示「综合 0.62」

**数据流**:
```
board articles API 查询时:
  articles → article_topic_tags → topic_tags
                                    ↕ topic_tag_board_labels (WHERE semantic_board_id = :id)
  每个 filtered_tag 携带 topic_tag_board_labels.score + .match_reason
```

- **[旧叙事数据兼容]** 现有 narrative_summaries 中有 scope_type=global/feed_category 的数据 → 新 API 按 semantic_board_id 查询时旧数据仍可见，scope_type 字段值不同但不影响展示；新数据统一 scope_type="board"
- **[filtered_tags 性能]** 每篇文章的 tags 需通过 topic_tag_board_labels 过滤 → 采用批量预加载：收集当前页所有文章 IDs → 一次性查出 article_topic_tags + topic_tags + topic_tag_board_labels → 按 article_id 分组返回，避免 N+1 查询
- **[匹配规则收紧]** 新增双因子约束可能让部分原本能匹配的 tag 不再匹配 → 这些匹配本身就是误匹配（如"中国科技媒体"→"科技行业ETF"），收紧是预期行为；用户可通过回填重算验证
- **[叙事迁移体验断层]** /topics 叙事 tab 删除后，习惯从图谱页进入叙事的用户需要适应 → 可在图谱热点面板的事件点击后跳转到 /tags 对应 board（后续迭代）
- **[L4 叙事生成改造范围]** 取消 scope 需要改动 service.go 的调度循环和 board_narrative_generator.go 的保存逻辑 → 改动集中在 narrative 包内，不影响 tagger/matching 模块

### D9: Refresh-all 并发策略

**决策**: `refreshAllFeedsWorker` 改为 `sync.WaitGroup` + `chan struct{}(cap=3)` semaphore 并发。前端 `onMounted` 改为两波 `Promise.all`：第一波 `fetchFeeds()` + `loadWatchedTags()`（无依赖），第二波 `loadArticles()` + `fetchGlobalUnreadCount()`（可能依赖 feeds 列表）。

**理由**: 后端 refresh-all 已经返回 202 异步，前端通过轮询检查状态。并发改造主要缩短总刷新时间（5 feeds × 30s 串行 = 150s → 3 并发 ≈ 60s）。前端串行 await 是实打实的页面加载性能问题。

**边界行为**: 单 feed 刷新不受影响；semaphore=3 避免过多并发请求压垮上游 RSS 源。

### D10: 匹配得分展示方案

**决策**: 每个文章的 tag chip 用颜色区分匹配方式（direct_hit=绿/#22c55e, hit_rate=蓝/#3b82f6, max_sim=橙/#f59e0b, weighted=灰/#94a3b8），chip 右侧显示分数文字。文章行右侧 end 处额外显示该文章最强匹配的信息（匹配方式名称 + 最高分数）。数据来自 `topic_tag_board_labels` 的已有 `match_reason` 和 `score` 字段。

**匹配公式参考**:
- `direct_hit` (score=1.00): tag 的辅助标签 ID 与 board 的辅助标签 ID 有交集
- `hit_rate` (score=hits/N): tag 的 N 个辅助标签中 cosine_sim ≥ 0.72 的比例 > 0.5
- `max_sim` (score=max_cosine_sim): 所有 tag-board 辅助标签对的最高余弦相似度 ≥ 0.8，且 hits ≥ min(2,N) 且 hitRate ≥ 0.3
- `weighted` (score=0.6×maxSim+0.4×hitRate): 加权综合分 ≥ 0.6

**理由**: 用户需要直观理解匹配质量。颜色+分数是最快的信息传达方式。数据已在 topic_tag_board_labels 中，零额外存储成本。

### D11: 日报生成异步化 + WebSocket

**决策**: `POST /api/daily-reports/generate` 立即返回 `{job_id, status: "processing"}`。后台 goroutine 执行 `GenerateDailyReport` 流水线，通过已有 `ws.GetHub().BroadcastRaw()` 广播进度消息。前端新增 `useDailyReportProgress.ts` composable 监听 WS 事件，`NarrativeGenerateDialog.vue` 改为进度板模式（每板块一行：等待/生成中/完成+条数）。

旧的 `POST /api/narratives/boards/generate` 保留同步调用（向后兼容），不再主动使用。

**WS 消息格式**:
```json
{"type": "daily_report_progress", "job_id": "xxx", "board_id": 123, "board_name": "AI芯片", "status": "completed", "saved": 1, "progress": "2/5"}
{"type": "daily_report_done", "job_id": "xxx", "total_saved": 3, "total_boards": 5}
```

**理由**: 复用已有 ws.Hub + BroadcastRaw 基础设施，前端已有 3 个 composable 先例（useTagWebSocket, useWebSocketRebuild, useOrganizeWebSocket），改动集中且风险低。

### D12: 日报系统数据模型

**决策**: 新建 `board_daily_reports` 和 `daily_report_sections` 两张表，与旧 `narrative_boards`/`narrative_summaries` 分离。旧表保留只读，不再写入新数据。

```sql
board_daily_reports:
  id, semantic_board_id, period_date, title, summary,
  highlights JSON, -- [{title, reason, tag_ids[]}]
  dynamics TEXT,   -- 板块动态文本
  article_count INT, event_tag_count INT, cluster_count INT,
  status VARCHAR(20), -- generating/done/failed
  raw_clusters JSON,  -- 聚类结果(调试用)
  prev_report_id UINT NULL, -- 昨日日报ID
  generation_prompt_version VARCHAR(20),
  created_at, updated_at

daily_report_sections:
  id, report_id UINT,
  cluster_index INT, cluster_label VARCHAR(200),
  cluster_tag_ids JSON, -- [tag_id, ...]
  threads JSON, -- [{title, summary, status, related_tag_ids[], related_article_ids[], parent_thread_id}]
  article_count INT,
  created_at
```

**理由**: 独立模型避免旧系统数据格式冲突。BoardDailyReport 一行 = 一个板块一天的全部日报内容。DailyReportSection 按聚类拆分，便于前端分块展示和后续增量更新。

### D13: 日报生成流水线 - 去重策略

**决策**: 程序化精确去重（无 LLM），两层规则：
1. 如果两个 tag 关联的文章集合完全相同 → 合并（同一事件被重复提取）
2. 如果两个 tag 的 article_count=1 且关联同一篇文章 → 合并

**不用 embedding 去重的原因**: 实测发现 embedding 区分度不够——同一天由同一 LLM 生成的 tag description，embedding 趋同（"阿里云发布X" 和 "阿里云发布Y" 的 embedding 相似度达 1.0，但 X 和 Y 是完全不同的产品）。

**理由**: 精确去重规则简单可靠，能去掉 10-30% 的明显重复。复杂去重留给后续 LLM 分组步骤处理。

### D14: 日报生成流水线 - LLM 语义分组

**决策**: 去重后的 tags 用一次 LLM call 做语义分组。输入所有 tag 的 label + description，输出 `[{group_name, tag_ids[]}]`。

**prompt 约束**: 分组粒度为"同一核心事件"；每组 2-8 个 tag，超过 8 拆分；单个 tag 可自成一组；每组给简短中文名称。

**token 成本**: 73 tags × ~87 字 ≈ 4,000 tokens 输入 + ~500 tokens 输出 ≈ 4,500 tokens/次，可控。

**理由**: embedding 无法区分"同一事件的不同角度"和"不同事件"（实测验证），LLM 能理解语义差异。tag 数量不大（去重后 20-60 个），一次 call 成本低。

### D15: 日报生成流水线 - 分段并行生成

**决策**: 三类 LLM call 并行执行：
- **Call A (今日重点)**: 输入全部 tags(label+desc+article_count) + 昨日日报，输出 2-3 个重点+选择理由
- **Call B (板块动态)**: 输入全部 tags + 昨日日报，输出综合趋势描述
- **Call C×K (聚类叙事线索)**: 每个聚类一次 call，输入该聚类 tags + 关联文章(标题+压缩摘要) + 昨日匹配线索，输出 0-N 条线索(emerging/continuing/...)

**Call C 的文章摘要策略**: 优先用 `ai_content_summary`(平均 348 字)，否则截取 `description` 前 200 字。每个聚类 5-10 篇文章，约 1,150 字 ≈ 575 tokens/聚类。

**总成本**: ~15K-30K tokens/board/day，3+K 个 LLM call。

### D16: 叙事线索连续性匹配

**决策**: 同一 board 内，用 embedding + tag 交集双重策略匹配昨日线索：
1. 优先匹配：如果今天聚类与昨日线索有 tag ID 交集 → 续接
2. Fallback：取聚类内所有 tag 的 embedding 平均值 vs 昨日线索关联 tag 的 embedding 平均值，cosine_sim ≥ 0.7 → 续接
3. 无匹配 → 标记为 emerging

**理由**: embedding 单独不够可靠（D13 验证），但结合 tag 交集约束后，在同一个 board 的范围内足以区分"同一事件延续"和"新事件"。

### D18: hit_rate 样本量惩罚 + 混合打分（修复 1-aux 标签命中率为 1.0 的问题）

**决策**: `scoreSemanticBoardSimilarity` 返回的 `hitRate` 分母从 `tagAuxCount` 改为 `max(tagAuxCount, minEffectiveSample)`（默认 3）。`hit_rate` 规则通过后，score 不再等于 `hitRate`，改为混合打分 `score = α × maxSimilarity + (1-α) × adjustedHitRate`（默认 α=0.7，可配置）。

**理由**: 数据分析发现 55.3% 的标签只有 1 个辅助标签（共 2888 个）。对于 1-aux 标签，`hitRate = hits/tagAuxCount = 1/1 = 1.0`，只要单个 embedding 与某板块的某个辅助标签 cosine_sim ≥ 0.72 就直接满分匹配。这导致：
- 718 个 1-aux 标签通过 `hit_rate` 规则全部 score=1.0（无区分度）
- 93 个 1-aux 标签同时匹配 2+ 板块且全部 score=1.0（无法区分主次）
- 81.6% 的所有匹配 score=1.0（完全丧失排序意义）
- 匹配退化为关键字级别的最近邻搜索

`max_sim` 规则的 `min_hits` 保护无效——因为 `hit_rate` 规则优先级更高，1-aux 标签在 `hit_rate` 阶段就已满分通过，根本走不到 `max_sim`。

**分母惩罚的效果**（minEffectiveSample=3）：
- 1-aux 标签：hitRate = 1/3 = 0.333 < 0.5 门槛 → 被推出 `hit_rate` 规则
- 2-aux 标签：hitRate = 2/3 = 0.667 > 0.5 → 仍走 `hit_rate`，score 不再是 1.0
- ≥3-aux 标签：hitRate 计算不变（max(N,3) = N）

**混合打分的效果**（α=0.7）：
- 1-aux 退到 max_sim：score = maxSim（0.80~0.91 之间，不再是 1.0）
- 1-aux 退到 weighted：score ≈ 0.6×maxSim + 0.4×0.333 ≈ 0.55~0.62，门槛 0.6 过滤掉弱匹配
- multi-aux hit_rate：score = 0.7×maxSim + 0.3×hitRate，融合质量和密度

**新增配置参数**：
- `semantic_board_match_min_effective_sample`（默认 3，hitRate 分母下限）
- `semantic_board_match_hit_rate_sim_blend`（默认 0.7，hit_rate 规则中 maxSim 的权重 α）

**预期影响**：
- score=1.0 的匹配从 81.6% 降至 ~34%（仅 direct_hit）
- 1-aux 标签匹配数从 718 降至约 300-500（弱匹配被淘汰，强匹配保留但分数合理）
- 同匹配多板块的 1-aux 标签有了分数区分度（maxSim 不同 → score 不同）
- ≥3-aux 标签行为完全不变（分母不变，但 score 从纯 hitRate 改为混合打分）

**备选方案**:
1. 最低辅助标签门槛：1-aux 标签禁止走 hit_rate → 被否决，因为硬阈值，2-aux 仍有问题
2. 分母改用板块辅助标签数：被否决，因为板块大小差异大（1~33 个辅助标签），不公平

### D17: 定时任务复用

**决策**: 复用 `scheduler_tasks` 表的 `narrative_summary` 任务，改造为调用新的日报生成流程。`check_interval` 保持 86400s（每天一次）。执行时间为 UTC 时间，可通过 `user_preferences` 或 `ai_settings` 配置目标时间。

**理由**: 已有调度框架成熟稳定，无需新建。任务名可改为 `daily_report` 或保持 `narrative_summary` 向后兼容。

## Risks (新增)

- **[Refresh 并发安全]** 多个 goroutine 同时写 feed 状态 → 每个 goroutine 只操作自己的 feed_id，通过 WHERE feed_id=? 限定，无竞争
- **[LLM 分组不稳定]** LLM 分组结果可能因 temperature 或模型版本变化 → 使用 temperature=0.1 + 固定 JSON schema 约束；分组结果存入 raw_clusters 便于调试
- **[日报 token 成本]** 活跃日 93 个 tag 的板块可能消耗较多 token → 去重+摘要压缩后预估 30K tokens/board，在可接受范围
- **[旧叙事数据]** 旧的 NarrativeBoard/NarrativeSummary 数据保留但不再更新 → 前端迁移到新 API 后旧数据不可见，无需迁移
