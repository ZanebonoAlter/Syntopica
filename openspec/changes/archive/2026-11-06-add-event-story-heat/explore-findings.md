
## AIHOT 参考实现：fact/story/signals 三层与确定性热度

来源 /tmp/aihot（KKKKhazix/AIHOT 开源引擎，Node24+TS+PG）。核心哲学：LLM 只做语义判断，代码做全部闸门与算术。

【三层身份】
- articles（报道）→ fact_articles(fact_id, article_id, role primary|report|mention, evidence, manual) → facts(id, public_id, story_id, title, subject, action, object, conditions, occurred_at, version, manual)
- facts → stories(id, public_id, title, status active|watching|settled, action, frame jsonb, first_report_at, latest_at, digest, latest, merged_into, version, origin model|replay|manual)
- story_aliases 存合并后旧 public_id；story_links 存 storyline/related
- 关键：一篇文章可挂多个 fact（综合稿 mention），PK 是 (fact_id, article_id)

【归属判定 events/relate.ts + group.ts】
- 召回：最近 14 天已归属 live fact 的报道，cos ≥ 0.6，top 10；无 pgvector，embeddings.vector real[] 线性扫
- 四分类：SAME_OCCURRENCE（同一次发生）/ SAME_STORY（同一事件进展）/ UNRELATED / ROUNDUP（综合稿只 mention）
- SAME_STORY 只挂 root fact（storyForDevelopment 过滤 storyRoot），防链条无限生长
- cos ≥ 0.85 直接合并；< 0.85 交 groupReview（建议另一个 provider）复核；story 合并阈值 STORY_REVIEW_MIN_CONFIDENCE=0.75
- 缺 decision 直接抛错（missing answers are failures, never assertions of unrelatedness）
- 综合稿/历史资料不建身份、不加热度

【热度 events/hot.ts】
- story_signals(story_id, article_id, participant_key, source_id, kind editorial|signal, observed_at) PK(story_id, article_id)
- heat = Σ over participant_key: power(0.5, (now - max(observed_at)) / 3600 / 24)，窗口 48h，半衰期 24h，每参与者只计一次
- participant_key CASE：signal_group_id → group:（媒体矩阵算1）；owner_entity_id → owner:（同公司多渠道算1）；DEV/HN 按作者；否则 source:id
- 上榜：参与者 ≥2 且 editorial 参与者 ≥1，最多 10 条
- 排除 participation_mode=isolated、撤回、standalone
- 落后源：>max(3×抓取间隔,90min) 未采集 → 标 incomplete，恢复后重算（公式无模型，重算一致）
- 三种热度分权：热榜半衰期求和 / 日报 (score??50)+5*log2(1+participants)+official5-followUp6 / 周月报按日报位置

【日报 reports/ 纯规则，零模型】
- 候选：已公开精选 + 非回填，按到达时间与发布时间较晚者归期，窗口出刊日前 24h
- 补漏：无精选但 ≥3 独立参与者 + 有官方帖
- 去重：读前 7 天日报已报 fact/article key（dailyMemory）
- 版面：12 正文 + 10 快讯 + 每源最多 2 条 + 跟进降级（当事方没新动作且 <4 源 → 快讯）
- 窗口内 0 条被判断 → 抛错告警，不出空刊
- 周月报才用模型写总述，grounded() 要求数字/拉丁专名必须出现在条目里，否则整段丢弃

**引用**：/tmp/aihot/database/migrations/0002_events_reports.sql、/tmp/aihot/packages/backend/src/events/relate.ts、/tmp/aihot/packages/backend/src/events/group.ts、/tmp/aihot/packages/backend/src/events/hot.ts、/tmp/aihot/packages/backend/src/reports/edition.ts、/tmp/aihot/packages/backend/src/editorial/analyze.ts

<!-- pinned 2026-11-05T11:42:25Z -->

## 数据库实测：标签资产与事件粒度（真实库 syntopica）

2026-11-05 对 docker syntopica-postgres（库 syntopica）只读实测，全部 SELECT，未改数据。

【topic_tags 分布】status=active
- event 10077，平均 label 20.3 字
- keyword 11410，平均 6.7 字
- person 330，平均 12.3 字，但约一半是完整事件句（「亚当·汉密尔顿击败罗杰·马歇尔」「拉夫罗夫出席 G20 外长会议引发争议」），且 thread tag_ids 中仅占 1.3% → 不可用于主语提取

【event 标签复用度】9863 个 event 标签有关联文章
- avg 1.03 篇 / p50 1 / p90 1 / max 6
- avg 1.01 天 / p50 1 / p90 1 / max 6
→ 一个 event 标签 ≈ 一篇报道，天然是「一次具体发生」的指纹，但跨文章/跨天不共享（同一事件的不同报道各有各的标签）

【event 标签样例（本身就是主谓宾句子）】
- 中国央行上调金融机构外汇存款准备金率 → event_keywords: 中国央行/外汇存款准备金率/上调/金融机构/货币政策
- 阳光电源股价因业绩反转预期及政策利好暴涨 → 阳光电源/股价暴涨/业绩反转/政策利好/光伏逆变器
- 字节扣子企业版分阶段下线 → 字节跳动/扣子/企业版/下线/产品整合

【metadata.event_keywords】10049/10077 = 99.7% 覆盖，固定 5 个词，含动作词（上调/暴涨/下线），是主谓宾拆词原料但无角色标注

【topic_tag_semantic_labels（auxiliary）】event 5273/10077 = 52.3%
- 数量分布：3 个 1032、4 个 1743、5 个 2353、6+ 少量
- 内容是实体锚点，几乎不含动作：「阳光电源股价暴涨」的 aux = A股/美国中期选举/储能/阳光电源/IRA法案（无动作无对象）→ 不能直接当主谓宾
- semantic_labels 表有 label_type（auxiliary 206840 / board 15 / composite 15）、embedding vector、ref_count

【daily_report_threads.tag_ids】JSONB 整数数组（如 [256484]、[256572,257002]），3 行 JSON null
- 最近 500 thread 的标签构成：event 623（82%）、keyword 131（17%）、person 10（1.3%）
- 一个 thread 的 tag_ids 常含 2–3 个 event 标签，且分属不同事件 → thread 当前是「叙事线索」，不等于单一事件

【每篇文章的 event 标签数】1 个 6265 篇、2 个 495、3 个 806、4 个 30、5 个 65 → 约 18% 是多事件/综合稿

【thread 跨天身份】DailyReportThread（daily_report_models.go:187-209）只有 SectionID，无 story/event/跨天字段；每天重新生成

【现成可用资产】thread.embedding 已有数据（daily_report_thread_fit.go:82-89 对标题 embedding）；pgvector <=> 已在 daily_report_matching.go:142 使用

**引用**：backend-go/internal/models/topic_graph.go、backend-go/internal/topicgraph/repository/daily_report_models.go、backend-go/internal/tagmanagement/service/core/extractor_section.go、backend-go/internal/topicgraph/service/daily_report_thread_fit.go、backend-go/internal/topicgraph/repository/daily_report_matching.go

<!-- pinned 2026-11-05T11:42:25Z -->

## Syntopica 现状：四个热度口径打架与三个结构缺陷

改前必须知道的四个口径打架 + 三个结构缺陷（代码位置已核）。

【四个热度口径】
1. 矩阵日格 = 当日该泳道各主题桶（section）thread 数的最大值（bucket max）
   - 前端界面自述 ObservationMatrix.vue:102「格值 = 当天最大主题桶的线索数（桶峰值）」
   - 泳道排序另用 DISTINCT 活跃天数（lane_snapshot_repository.go:501-505）
2. 演进节点 = 同日各桶 thread 数之和（lane_snapshot_repository.go 的 byDate 累加）
3. 雷达 attention = 版块当日所有泳道 section 数之和 ×2 + 近3日 + 净增（board_radar_repository.go:211-214）
   - 注意：buildRadarRow 里 counts[i]=len(day.Sections)，跨泳道 row.Days[i]+=counts[i]
4. 雷达「今天可能升温」= 前端算：todaySections≥3 且 todaySections/近3日均值 ≥2（useBoardRadarOverview.ts:58-65）

【自我矛盾（重要）】
lane_snapshot_repository.go:487-490 注释明确写「Sorting by section count is forbidden: intra-lane theme bucketing splits one day into several sections, so a section-count sort would reward content that happened to be split more」——但 board_radar_repository.go:211-214 的 attention 恰恰在按 section 数算。

【结构缺陷】
1. thread 无跨天身份：daily_report_models.go:187-209 只有 SectionID；每天重新生成
2. 跨天只有 section 相似度匹配（daily_report_matching.go:12-19 MatchPenalty=0.28、SplitGap=0.03、SkipDayThresh=0.20），连的是「相似日桶」不是「同一事件」；不存在「最近 N 期不重复」机制
3. 无参与者去重：同一 feed 刷 20 条 = 20；无 source tier/权重；reading_behaviors 不进热度且归档时被删
4. 热度不落任何历史表（topic_lane_snapshots 每泳道一行覆盖式；board_heat 无表）

【现有确定性正例（可参考实现风格）】
- signal_detect.go:37-38 score≥6 硬门槛 + schema 整数域校验
- boardhealth/run.go:404-406 分数范围 gate + triage 覆盖率降级
- daily_report_lane.go 阈值 0.18/0.30 先筛，LLM 只在代码给的 top-5 候选里选
- watch_adjudicate.go 召回层阈值保 recall，LLM 只做 precision pass

【调度】日报默认 21:00 / 兜底 23:30（admin/scheduler/job_daily_report.go:208-226），60s 复查等双队列清空

**引用**：backend-go/internal/topicgraph/repository/lane_snapshot_repository.go、backend-go/internal/topicgraph/repository/board_radar_repository.go、front/app/features/tags/components/console/ObservationMatrix.vue、front/app/features/tags/composables/useBoardRadarOverview.ts、backend-go/internal/topicgraph/repository/daily_report_matching.go

<!-- pinned 2026-11-05T11:42:43Z -->

## 设计决策：story/signals 模型、五层判定、热度公式、展示口径

本 change 的实现级设计决策（design.md + specs 已固化，实现时按此执行，改动需回写 design）。

【模型】
- 新增 board_stories(id, board_id NOT NULL, lane_id NULL 首次归属, title, subject, action, object, occurred_at, status active|settled|merged, first_seen_date, last_seen_date, thread_count, article_count, participant_count, embedding, merged_into, origin rule|model|manual)
- 新增 story_signals(story_id, article_id, feed_id, participant_key='feed:<id>', kind editorial|signal, observed_at=articles.pub_date, observed_at_source pub_date|fallback) PK(story_id, article_id)
- daily_report_threads 加 story_id NULL / scope single|composite / is_development bool
- merged_into 只允许一层，合并时直接指向最终存活者；读取一跳解析

【指纹】在 extractor_section.go 的融合提取调用内多输出 subject/action/object（occurred_at 可空），零新增调用；用 metadata.event_keywords 校验（subject 与 object 必须能在关键词里找到，允许包含+别名+lower/NFKC）；不通过标 fingerprint_trusted=false，标签照常入库
- 不用 auxiliary labels（52.3% 且不含动作）；不用 person 标签（质量差且仅占 1.3%）

【归属判定顺序（前层命中即终止）】
1. subject 不同 → 新建（一票否决，不被向量推翻）
2. subject+action+object 全同 → 同一次发生
3. subject+object 同、action 不同 → 同一 story + is_development=true；但反向动作表（发布 vs 召回/下架/否认）命中则跳过本层走 LLM
4. subject 同、object 不同 → 新建
5. 指纹不可信/冲突 → LLM 四分类 SAME_OCCURRENCE|SAME_STORY|UNRELATED|ROUNDUP，低置信/失败/ROUNDUP → 新建（保守）
- 向量只召回（14 天窗口、dist<=0.5、top 10），不参与判定
- scope=composite（event 标签≥2）不建身份、不计事件数

【热度】
- story_heat = Σ over participant 0.5^(Δt/24h)，窗口 48h，每 participant 只计一次
- lane_heat = Σ story_heat，半衰期 72h，窗口 14 天
- rule_version = heat-v1-48h-hl24h
- 热度只依赖 story_signals + board_stories + thread 归属 + articles.pub_date；禁止依赖 section 条数、thread 行数、桶数、任何 LLM 字段
- 不落历史快照表（本期读时算）

【展示口径】
- 矩阵日格数字 = 当日 story 数；颜色 = 当日最大 story 的 participant_count（4 档，必须有图例+数字 title/aria）
- 演进节点 = 当日 story 数
- 行序 = lane_heat 降序 → 事件数 → topic id
- 雷达关注度 = lane_heat（1 位小数）；阈值不可沿用 50/15，按全量历史 lane_heat 的 P80/P20 标定并记录分布
- 升温 = story_heat 环比 ≥2 且事件数 ≥3

【跨期去重（仅展示层）】
- thread.story_id 非空且该 story 最近 7 天进过正文：文章集合重叠≥0.6 且无新 feed → 重述降级快讯；重叠<0.6 或有新 feed 或 is_development → 进正文
- 降级不删 thread、不删 signals、不减热度

【回填】cmd/backfill-stories，按 board×date 升序、只处理 story_id IS NULL 的非综合稿 thread、--dry-run、版块级 advisory lock、人工指派不覆盖；未回填时按「非综合稿 thread 数」作事件上界并标「事件身份未回填」

【明确不做】不改 lane 三分桶 0.18/0.30、不改日报 section 生成、不改归档口径、不做 story 综述、不接社媒信号、不做热度历史曲线页面

**引用**：openspec/changes/add-event-story-heat/design.md、openspec/changes/add-event-story-heat/specs/event-story/spec.md、openspec/changes/add-event-story-heat/specs/story-heat-evidence/spec.md

<!-- pinned 2026-11-05T11:42:43Z -->

## 术语定稿：事件线 vs 事件、subject_key 三层归一化

术语定稿（2026-11-05 用户拍板，后续所有文档/界面/spec 以此为准）：

| 业务层 | 代码实体 | 定稿术语 |
|---|---|---|
| 版块 | semantic_boards | 版块 |
| 话题 | board_persistent_topics | 话题（泳道） |
| 事件线 | board_stories（新增） | 事件线 |
| 事件 | daily_report_threads | 事件（代码字段保留 thread） |
| 文章 | articles | 文章 |
| 日报分区 | daily_report_sections | 分区（展示容器，不属于身份层） |

三条硬约定：
1. 事件 ≠ 事件线：事件是某天的一次发生（关联多篇文章），事件线是多天同一事件的聚合。事件线对应参考实现的 story。
2. 分区不是身份：section 随 LLM 分桶 k 变化，任何身份判定/计数/热度都不得引用 section.id 或 bucket_title。
3. 代码标识符不改：daily_report_threads / story_id / board_stories / story_signals 保留英文，只统一文档、界面文案与 spec 措辞。不叫 story（与现有中文体系割裂），不叫「事件」（与 thread 撞名）。

事件线与现有体系的三处串联（不是平行体系）：
- 事件 → 事件线：daily_report_threads.story_id（可空，加一列）
- 事件线 → 话题：board_stories.lane_id（可空，仅首次归属，不参与判定）
- 事件线 → 热度：story_signals（从事件的 related_article_ids 展开）
展示维度（report → section → thread）与身份维度（board → story → thread）正交，不改 section 生成、话题分桶、日报版面、归档口径。

主体归一化定稿（关键设计决策，用户质疑 embedding 误匹配后确认）：
- LLM 在生成指纹的同一次调用内产出 subject_key（格式 类型:规范名，类型 person/company/org/product/other），由模型完成跨语言别名归并（Trump/川普/特朗普 → person:trump）
- 三层匹配：① subject_key 精确（lower+下划线）② subject_key embedding cos ≥ 0.92 ③ subject 字面归一（lower/NFKC/去空格/去公司集团后缀）
- embedding 阈值 0.92 远高于事件召回 0.5——实体误归并代价高于漏归并
- 不用 topic_tags.aliases：实测 person/event 覆盖 0%、keyword 7.3%，不可用
- 不建独立实体表：subject_key 存 board_stories，subject_embedding 可空，只为已建立的事件线算一次

设计图谱落盘：openspec/changes/add-event-story-heat/data-map.md（11 节：术语表 / 端到端数据流 / 两维度正交 / ER / 五层判定 / subject 归一化 / 热度 / 跨期去重 / 回填 / 三条降级 / 代码落点速查 / 评审清单）。实现时对着 data-map.md 写，图与 specs 冲突以 specs 为准。

**引用**：openspec/changes/add-event-story-heat/data-map.md、openspec/changes/add-event-story-heat/design.md、openspec/changes/add-event-story-heat/specs/event-story/spec.md

<!-- pinned 2026-11-05T12:11:58Z -->

## 派发模型与 UI 档位约定（2026-11-05 用户指令）

用户本 change 指定：原型子线程固定 `openai-codex/gpt-6.1-sol:medium`；实际实现子线程固定 `zai-coding-cn/glm-5.3:max`。已核实 `pi --list-models` 两个全称均存在（openai-codex/gpt-6.1-sol 500K；zai-coding-cn/glm-5.3 1M）。注意 fuzzy 名会落错供应商，派发必须写全称。UI 档位：proposal `<!-- ui-impact: minor -->`，ui-design.md 头部 `<!-- ui-approval: not-required -->` / `<!-- ui-prototype: none -->`，按 syntopica-ui 工作流 minor 档不需要 ui-prototype 目录，ui-design-gate 不阻断实现；若用户要求补原型，可在 change 下新增 ui-prototype/ 并先改 ui-design.md 的 Prototype 节。

**引用**：openspec/changes/add-event-story-heat/ui-design.md、openspec/changes/add-event-story-heat/proposal.md

<!-- pinned 2026-11-05T12:18:36Z -->

## 并发冲突预警：迁移与日报 handler 已被其他 active change 改动

开工前 concurrency-status 实测（2026-11-05T12:18Z，10 个 active change）：`backend-go/internal/platform/database/postgres_migrations.go` 已有 +131/-32 脏改动，归属 `unify-vector-column-dimension`（向量列维度迁移）；`backend-go/internal/topicgraph/handler/daily_report_handler.go` 已有 +40 脏改动，归属另一 active change（doc-impact 判为其他 change 桶）。本 change tasks 1.2 要往 postgres_migrations.go 追加 `board_stories`/`story_signals`/三列迁移，tasks 3.5/6.3 可能碰 daily report 相关文件。实现时 MUST 在文件末尾追加新 Migration（版本日期戳递增，如 20261105_0001 系列），禁止重排/回退现有迁移；禁止 `git checkout`/整体重写这两个文件。派发前再跑一次 `bash scripts/harness/concurrency-status.sh add-event-story-heat`，冲突文件按归属地图只改本 change 需要的段落。

**引用**：backend-go/internal/platform/database/postgres_migrations.go、backend-go/internal/topicgraph/handler/daily_report_handler.go

<!-- pinned 2026-11-05T12:18:36Z -->

## 指纹落点与写入链路：topic_tags.metadata（四条分支 + 异步 keywords）

指纹必须落到 `topic_tags.metadata`（JSONB）：`article_topic_tags` 无 metadata 列，只有 (article_id, topic_tag_id, score, source)。写入路径在 `backend-go/internal/tagmanagement/service/core/tagger.go`：① `findOrCreateTag` 有缓存命中早退（`GetTagCache().Get`）、exact 匹配、slug 兜底、新建四条分支，指纹要在四条分支都落 metadata（至少「缺失时补写」）；② 新建 event 标签后异步 `go generateTagDescription(...)` → `saveTagDescription(isEvent, tagID, desc, keywords)` 才写 `metadata['event_keywords']`（tagger.go:361-390），因此**提取时拿不到 event_keywords**，一致性校验（tasks 2.3）只能在归属判定时读 DB 执行；③ `saveTagDescription` 回写 metadata 时是「读-改-写」，别把指纹键覆盖掉。④ 同一 event 标签被多篇文章复用时 metadata 只有一个指纹（实测 avg 1.03 篇/标签，可接受，last-write-wins）；归属判定读指纹必须查 DB，不能信 TagCache（缓存对象可能无 metadata）。

**引用**：backend-go/internal/tagmanagement/service/core/tagger.go:59、backend-go/internal/tagmanagement/service/core/tagger.go:361、backend-go/internal/tagmanagement/service/core/types.go:20、backend-go/internal/models/topic_graph.go:92

<!-- pinned 2026-11-05T12:33:17Z -->

## 实现落差：无 groupReview 多 provider；daily_report_dedup.go 已存在

两个实现落差，批次 2/4 必须按仓库事实处置：① **design §5.4 的 groupReview 多 provider 复核在本仓库不存在**——`grep groupReview` 零命中；airouter 路由只按 `Capability` 选 route（`LoadRouteWithModels(req.Capability)`，`Operation` 仅作日志/元数据），`ChatRequest` 没有 provider 覆盖字段。事件线合并（tasks 3.3「双模型判定一致」）需降级为：单次 LLM 调用（Operation=`topicgraph.story_merge_review`、Capability=`topic_tagging`、temp≈0.1、要求 confidence ≥0.75），并按 design-audit 纪律在 review.md 记录这处偏离；不要伪造两家模型。② **`backend-go/internal/topicgraph/service/daily_report_dedup.go` 已存在**，但它是标签去重（`DedupRelevance`/`DeduplicateTags`，fix-digest-tag-leak），不是跨期事件去重；tasks 6.1 的「新增 daily_report_dedup.go」会撞名，应新建 `daily_report_story_dedup.go` + `daily_report_story_dedup_test.go`（或把跨期函数加进现有文件并明确命名），禁止覆盖现有标签去重实现。

**引用**：backend-go/internal/platform/airouter/router.go:210、backend-go/internal/platform/airouter/openai_compatible.go:45、backend-go/internal/topicgraph/service/daily_report_dedup.go、backend-go/internal/topicgraph/service/watch_adjudicate.go:107

<!-- pinned 2026-11-05T12:33:29Z -->

## 人工纠正标记缺口：daily_report_threads 需补 story_manual

spec `event-story` 「人工纠正优先」要求：人工拆分后事件 `story_id=NULL` **且记录人工标记**，后续回填/自动判定不得重新挂回；人工指派后跳过该事件。但 design §6.1 只给 `daily_report_threads` 设计了三列（story_id/scope/is_development），没有人工标记载体；仓库也不存在 design 引用的 `grouping_overrides` 表（grep 零命中）。**处置（迁移未部署，原地补）**：`daily_report_threads` 增加 `story_manual boolean NOT NULL DEFAULT false`。语义：`story_manual=true` 的事件是人工指派/人工拆分结果，自动归属与回填一律跳过（无论 story_id 是否为空）；人工指派=写 story_id + story_manual=true，人工拆分=story_id=NULL + story_manual=true。`board_stories.origin='manual'` 仅表示事件线本身人工创建/合并，不能替代事件级标记。实现 batch 2 的 3.4 按此列做跳过判定；本期不新增人工操作 API/UI（无对应任务），只保证数据模型与自动路径尊重该标记。

**引用**：openspec/changes/add-event-story-heat/specs/event-story/spec.md:119、openspec/changes/add-event-story-heat/design.md:225、backend-go/internal/topicgraph/repository/daily_report_models.go:187

<!-- pinned 2026-11-05T12:58:53Z -->

## 热度两级口径裁定：story 24h/48h，lane 72h/14d

热度两级公式的落地口径（spec 内部表述有歧义，按 design §6.2 固化，批次 3 照此实现）：① `story_heat`：窗口 48h、半衰期 24h，`per_participant = (story_id, participant_key) → MAX(observed_at)`，`heat = Σ 0.5^(Δt_h/24)`；② `lane_heat`：窗口 14 天、半衰期 72h，**同结构**——先按 `(story_id, participant_key)` 取窗内 MAX(observed_at)（同一 feed 在同一事件线只计一次；不同事件线分别计），按 `COALESCE(story.merged_into, story.id)` 归并到存活事件线，再按 `lane_id` 求和，`heat = Σ 0.5^(Δt_h/72)`。不要把 `lane_heat` 实现成「24h 半衰期的 story_heat 直接相加」（那样 72h 参数永远不生效）。`rule_version='heat-v1-48h-hl24h'` 写响应。~~排除证据：`articles.archived=true` 不计~~ → **【2026-11-06 订正，见 review.md R-7】该口径已证伪：`archived` 是保留期轮转标记（不是可见性/撤回标记），证据 MUST NOT 按 `archived` 过滤——红线见 `sourcestats` 包注释第 2 条。原实现因此把 86% 证据排除、热度全 0，已全数修复并重算。**

**引用**：openspec/changes/add-event-story-heat/specs/story-heat-evidence/spec.md:46、openspec/changes/add-event-story-heat/design.md:212、backend-go/internal/models/article.go:25

<!-- pinned 2026-11-05T13:05:33Z -->

## 指纹 metadata 契约与共享归一化落点

事件指纹的存储契约（批次 2a/2b 共用，勿各自发明）：写在 `topic_tags.metadata`，嵌套对象 `event_fingerprint = {subject, subject_key, action, object, occurred_at?}`；校验结果单独写平铺键 `fingerprint_trusted: bool`（tasks 2.3/2.4 的 grep 目标）。写入规则：① 只对 `category='event'` 写；keyword/person 不写；② 已有 `event_fingerprint` 且字段完整时保留旧值（先写者赢，避免同一标签被不同文章反复翻身份），缺失/不完整时才补写；③ 读改写必须保留 `event_keywords` 等既有键（`saveTagDescription` 也是读-改-写，别互相覆盖）；④ 归属判定读指纹走 DB，不信 TagCache。共享归一化放 `internal/platform/textutil`（叶子包，无内部依赖）：`NormalizeSubjectKey`（lower/trim/空白与连字符→下划线，保留 `类型:` 前缀）与 `NormalizeEntityText`（NFKC/lower/去空白/去「公司/集团/inc/ltd/corp」后缀）。`subject_key` 合法前缀 person/company/org/product/other；非法前缀按不可信处理但标签照常入库。

**引用**：backend-go/internal/platform/textutil/normalize.go、backend-go/internal/tagmanagement/service/core/types.go:55、backend-go/internal/tagmanagement/service/core/tagger.go:59、backend-go/internal/models/topic_graph.go:35

<!-- pinned 2026-11-05T13:07:57Z -->

## 矩阵/雷达响应字段契约（story_count/lane_heat/兼容字段）

批次 3 前后端字段契约（按 specs/workshop-board-console + ui-design 固化，避免实现时各叫各的）：后端 `LaneDynamicsTimelineDay` 增加 `story_count`（该泳道该日跨所有 section 的事件线数之和）、`max_participant_count`（该日最热事件线的 participant_count，未回填时按当日非综合稿 thread 的 DISTINCT feed_id 降级）、`unassigned_event_count`（该日 story_id IS NULL 的非综合稿事件数，用于 title/aria）。响应顶层增加 `stories_backfilled bool`（该版块是否已有任何 story_id）。`LaneDynamicsLane` 增加 `lane_heat float64`（72h/14d 口径，见热度 pin），行序改为 lane_heat 降序 → 事件线数降序 → topic id。雷达 wire 契约：保留 `today_sections`/`last3_sections`/`prev3_sections` 旧字段（section 口径，向后兼容），**新增** `today_stories`/`last3_stories`/`prev3_stories`（事件线数）与 `lane_heat`；`attention` 改为 `lane_heat` 一位小数；前端 normalizer 读新字段，标签改「当日事件线数」。矩阵前端断档判定仍用 day.sections.length（有 section=可点），格值用 storyCount，颜色用 maxParticipantCount，title/aria 同时给 storyCount/maxParticipantCount/unassignedEventCount。

**引用**：openspec/changes/add-event-story-heat/specs/workshop-board-console/spec.md、openspec/changes/add-event-story-heat/specs/workshop-radar/spec.md、backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:495、backend-go/internal/topicgraph/repository/board_radar_repository.go:190

<!-- pinned 2026-11-05T13:09:21Z -->

## 迁移 tableExists 守卫缺口与 testutil golden schema 顺序

批次 2a 期间发现并修复：迁移 20261105_0001 的 `daily_report_threads_story_idx` 原先无 tableExists 守卫，而 `daily_report_threads` 表只由 topicgraph 模型 AutoMigrate 注册创建（`internal/topicgraph/repository/daily_report_register_models.go` init()）。testutil golden schema 路径 = RunAutoMigrate（仅注册过的模型）→ RunMigrations；不 import topicgraph 的测试二进制（如 tagmanagement/service/core 的全部 testcontainer 用例）里该表不存在 → 建库即炸 `relation "daily_report_threads" does not exist`，殃及全仓 testcontainer 测试。已按同文件旧迁移惯例（postgres_migrations.go ~L653）给该索引补 tableExists 守卫；StoryHeat 迁移测试与 core 包全量测试均复验绿。教训：**任何引用 AutoMigrate-only 表的迁移语句必须包 tableExists 守卫**（后续批次 3.x/4.x 写迁移时同样适用）。另：指纹存储口径 = `topic_tags.metadata.event_fingerprint`（嵌套对象，键序 action/object/occurred_at/subject/subject_key），`fingerprint_trusted` 由 2.3（批次 2b）引入——tasks.md 2.4 的验证 grep（fingerprint_trusted）在 2.3 落地前必然不命中，属任务文本滞后于 design 的已知偏差。

<!-- pinned 2026-11-05T13:40:04Z -->

## 判定顺序裁定：subject 否决先于 untrusted→LLM

归属判定顺序裁定（2b 返工依据）：**subject 一票否决优先于「指纹不可信走 LLM」**。即使 `fingerprint_trusted=false` 或指纹不完整，只要新事件与候选双方 `subject_key` 非空且归一后不同，该候选必须被否决、不得进入 LLM 候选列表；所有候选都被 subject 否决时直接新建（origin=rule, reason=subject_veto, 不调 LLM）。原因：spec 把 subject 否决写成绝对规则（SHALL NOT 被向量/object 推翻），且误合并代价高于误拆分；若让 LLM 在明显不同主体的候选上判定，等于用模型覆盖第一层。空 subject_key 不参与等值/否决（keyless 候选才走 LLM）。2b 的 `DecideAssignment` 已按此返工，2c 接线时不得再加一层「untrusted 先送 LLM」的前置短路。

**引用**：openspec/changes/add-event-story-heat/specs/event-story/spec.md:73、backend-go/internal/topicgraph/service/story_assign.go

<!-- pinned 2026-11-05T14:03:44Z -->

## 2c 编排：计数重算/scope 单点/合并保护人工线程

2c 落库编排的三个必须处理点（否则矩阵/热度会读到脏计数）：① **事件线计数用重算而非自增**：`SaveReport` 会删除旧 threads 再插入（同 board 同日重跑/rebuild），自增 `thread_count`/`article_count` 会虚高。2c 实现 `RecomputeBoardStoryCounts(storyID)`——从 `daily_report_threads` 重新算 thread_count、article_count（related_article_ids 长度和）、first_seen_date=MIN(period_date)、last_seen_date=MAX(period_date)；attach/merge 后调用；`SaveReport` 删除旧 threads 前收集受影响的 story_id，commit 后重算。② **scope 推导放 `SaveThreads` 单点**：解析 `threads[i].TagIDs` → 查 `topic_tags.category`，event 数 ≥2 → `composite`，否则 `single`；查询失败降级 single 不阻断保存。这样 orchestrator/watch 物化/rebuild 所有调用者都覆盖。③ **合并不得移动人工线程**：`MergeBoardStories(from,into)` 只把 `story_manual=false` 的 threads 改指 into；manual threads 保持 `story_id=from`（读取经 merged_into 一跳解析到 into），避免自动合并覆盖人工指派。`merged_into` 只允许一层，合并前先解析 from/into 的最终存活者；目标仍是 merged 状态则拒绝（两层链）。

**引用**：backend-go/internal/topicgraph/repository/daily_report_repository.go:228、backend-go/internal/topicgraph/repository/daily_report_repository.go:591、openspec/changes/add-event-story-heat/specs/event-story/spec.md:119

<!-- pinned 2026-11-05T14:04:14Z -->

## 热度计算落点：索引查询 + Go 聚合（跨方言可测）

批次 3a 热度实现裁定：**用「索引查询 + Go 聚合」而不是 `power()/EXTRACT(EPOCH)` 的 PG 专用 SQL**。理由：① 单版块 14 天 signals 量级 ~10^3，Go 聚合毫秒级；② SQLite 单测可直接验证公式（不必全靠 testcontainer）；③ 避免 PG/SQLite 方言分叉。查询用 `story_signals` 的 `observed_at >= windowStart`（走 `story_signals_time_idx`）join `articles`（~~`archived=false`~~ → **订正：不加 archived 过滤，见 R-7**）join `board_stories`（拿 `merged_into`/`lane_id`），把 (story_id→survivor, lane_id, participant_key, observed_at) 拉回 Go：① 按 `(survivor_id, participant_key)` 取窗口内 MAX(observed_at)，再 `Σ math.Pow(0.5, hours/24)` 得 story_heat；② lane 级同结构用 72h、14d，按 survivor 的 `lane_id` 求和（不同 story 的同一 feed 分别计）。`rule_version='heat-v1-48h-hl24h'` 常量写响应。合并事件线的 signals 不迁移，靠 survivor 归并。

**引用**：openspec/changes/add-event-story-heat/design.md:212、openspec/changes/add-event-story-heat/specs/story-heat-evidence/spec.md:46

<!-- pinned 2026-11-05T14:09:01Z -->

## 跨期去重载体：display_tier + detached 判定

跨期去重（tasks 6.x）的必要载体与落点裁定：① 现有 `daily_report_threads` 没有「正文/快讯/未展示」字段，spec 要求「降级为快讯或丢弃，读取路径可区分」，因此 batch 4 需在 `20261105_0001` 迁移（仍未部署，可原地加）给 `daily_report_threads` 增加 `display_tier text NOT NULL DEFAULT 'body' CHECK IN ('body','flash','hidden')`，模型加 `DisplayTier string`，迁移测试断言默认 body/NOT NULL；② 本期选择「丢弃」语义：`GetReportByID` 的 `Sections.Threads` Preload 只加载 `display_tier='body'`，`flash/hidden` 不渲染（spec 允许「或丢弃」），字段保留供未来快讯 UI；矩阵/热度的读取路径**不得**过滤（身份与热度不受降级影响）；③ 去重判定在 `SaveReport + attributeStories` 之后以 detached 任务执行（`applyStoryDedup(reportID)`），因为 `story_id` 是归属判定后才有的，orchestrator 组装 section 时还没有身份；重述判定查同 story 最近 7 天 `display_tier='body'` 的 threads，按文章集合重叠 ≥0.6 且无新 feed 判重述（flash），否则进展（body）；`story_id=NULL` 或 composite 维持 body。这是对 tasks 6.3「在组装 section 前判定」的合理偏离，记入 review.md。

**引用**：openspec/changes/add-event-story-heat/specs/daily-report-system/spec.md:76、backend-go/internal/topicgraph/repository/daily_report_models.go:187、backend-go/internal/topicgraph/repository/daily_report_repository.go:400

<!-- pinned 2026-11-05T15:28:54Z -->

## archived 过滤导致热度证据被砍 86%（High）

**缺陷（High，实测发现）：`story_signals` 重建用 `articles.archived = false` 过滤，把 86% 真实证据排除了。**

## 事实
1. `models/article.go:20-25`：`Archived` = "aged out of the feed's active window (MaxArticles)"，是**保留期轮转标记**，不是「不可见/已撤回」。归档行保留全部文本字段（daily report threads 按 ID 引用），只从阅读列表/搜索/统计剔除。
2. `tagmanagement/service/sourcestats/sourcestats.go` 包注释第 2 条明确警告：**"window queries MUST NOT filter on `archived`; … High-volume feeds keep almost all their hits in the archived range (CleanupOldArticles), so filtering would invert the conclusion. Do not 'helpfully' add an archived clause."**
3. 生产库实测：`articles` 60,853 行 → `archived=true` 58,623（96%）；回填窗口 10-30~11-05 内 7,204 篇中 `archived=true` 6,224（86%）。
4. `story_signals_repository.go:recomputeSignalsForStoryWith` 的 `Where("id IN ? AND archived = ?", ids, false)` → 只有 4%~14% 证据落库。
5. 回填后实测：`board_stories=787`、`threads.story_id IS NOT NULL=790`，但 `story_signals` 仅 **141** 行、覆盖 141 条事件线（每线 1 条）→ `participant_count` / `story_heat` 被低估约 7 倍。

## 根因
design/review 的偏离 #3「仓库无 participation_mode/撤回列，排除证据只用 `articles.archived`」对 `archived` 语义判断错误。

## 修复方向
- 去掉 `recomputeSignalsForStoryWith` 的 `archived` 过滤（取全部关联文章）；「撤回/恶意」若确需排除，新增专用列，不得复用 `archived`。
- 修 design.md / review.md 的偏离描述 + `docs/reference/flow/daily-report.md` 相关表述。
- 补可重算 signals 的运维入口（CLI flag 或独立命令）：回填幂等会跳过已有 `story_id` 的线程，重跑回填**不会**重建 signals。

## 影响面
`story_signals` → `story_heat`（story 48h/24h、lane 14d/72h）→ 矩阵颜色分档 → 雷达 attention/排序/升温 → 5.3 标定的 P80/P20 分布。必须先修再重算，才能标定。

**引用**：backend-go/internal/topicgraph/repository/story_signals_repository.go:recomputeSignalsForStoryWith、backend-go/internal/models/article.go:Archived、backend-go/internal/tagmanagement/service/sourcestats/sourcestats.go:package-doc

<!-- pinned 2026-11-06T02:43:02Z -->

## 效果验收未通过：attach 0.4% 主因是 candidate_id 缺失（High）

**真库效果验收未通过：4 个缺陷（1 个 High 接口缺陷 + 3 个数据前提缺口），attach 率 0.4% 的主因已定位。**

## 缺陷一（High）：SAME_* 允许不返回 `candidate_id` → 405/408 次解析失败回退新建
- `story_assign_llm.go:47-55`：JSON schema 声明了 `candidate_id`，但 `required` 只含 `decision`/`confidence`。
- `story_assign_llm.go:139-150`：缺 ID 反序列化为 0 → 非有效候选 → 报错。
- `story_assign.go:278-280`：分类错误降级为「新建事件线」。
- 实测 713 次分类：SAME_OCCURRENCE 266（缺 ID 266）、SAME_STORY 142（缺 ID 139）、UNRELATED 296（缺 96）、ROUNDUP 9（缺 9）。**408 次声称相关里 405 次没有可挂载地址；仅 3 次带 ID，恰好等于实际 3 次 attach。**
- 这是 attach 率 0.4% 的**直接主因**，不是「数据多为独立事件」。

## 缺陷二（High）：降级路径的候选没有任何可判断内容
- `StoryCandidate` 不带标题/摘要；`story_assign_llm.go:113-117` 只渲染 id + 空的 subject/key/action/object + 日期。
- 模型明确在理由里承认「候选只有日期，缺失关键信息」（UNRELATED 296 次里大量此类）。
- 修法：候选带上 `title`/`summary` 或代表证据；同时**必须**表达「指纹缺失」而非空白。只修缺陷一而不修这条，会把盲判变成更多真实误合并。

## 缺陷三（数据前提）：历史 event 指纹全库为 0
- 全库 9,439 个 event 标签：`fingerprint_trusted=true` **0** 个、false 586 个、未记录 8,853 个；**全部缺 `metadata.event_fingerprint`**。
- 后果：五层指纹规则一条都没跑上，958 条线程全部走 LLM 降级路径。归因 10 组疑似拆分样本，主因 100% 是 `fingerprint_untrusted`（`recall_empty`/`subject_veto`/`object_veto`/`composite_skip` 均为 0）。

## 缺陷四（数据契约）：168 条实际综合稿的 `scope` 仍是 `single`
- 身份拦截正确（`scope=composite AND story_id IS NOT NULL` = 0；实际 ≥2 event 标签的 168 条也都没归属）。
- 但存储列 `scope` 未回填成 `composite`，违反「scope 由 tag_ids 中 event 标签数推导」的契约。需要在迁移/一次性脚本里修历史行。

## 效果指标（test-cases §4）
| # | 指标 | 实测 | 结论 |
|---|---|---|---|
| 1 | 指纹可信率 | 0 条可信 / 全部缺指纹 | 未通过 |
| 2 | 窗口归组率 | 790/790 = 100%（全库 5.94% 是存储 scope 粗算，不能冒充） | 窗口通过，但不代表归组正确 |
| 3 | **误合并率** | 全库仅 3 条多线程线，**2 条误合并 = 66.7%** | **未通过**（样本不足 50，但错误明确） |
| 4 | 误拆分率 | 定向 10 组疑似，非随机 50 组 | 无法测总体 |
| 5 | 综合稿拦截 | 归属数 0 ✅ / scope 契约 ✗ | 半通过 |
| 6 | 热度 vs 旧口径 | 排名变化：15220 5→2、1974 2→4、2272 4→6 | 记录完成，不等于正确性通过 |
| 7 | 阈值标定 | P20=0.701374154106、P80=3.325992212118、734 样本 | 通过（与 design 附录一致） |

## 结论
当前**不可归档**。最小返工路径：修缺陷一 + 缺陷二（同批）→ 清空本窗口 `story_id` 重跑归属 → 重测误合并率；缺陷三（指纹回填）与大范围 LLM 重打标成本高，建议登记为后续 change；缺陷四需一次性 scope 回填脚本。

**引用**：backend-go/internal/topicgraph/service/story_assign_llm.go:47-55、backend-go/internal/topicgraph/service/story_assign_llm.go:139-150、backend-go/internal/topicgraph/service/story_assign.go:278-280、backend-go/internal/topicgraph/service/story_assign_llm.go:113-117

<!-- pinned 2026-11-06T03:41:17Z -->

## JSON Schema 缺 Required 导致 qwen 约束解码死循环（诊断事实）

**JSON Schema 缺 `Required` 会让 qwen 的约束解码死循环，把 `maxTokens` 全部烧光（实测 5 次调用定量）。**

## 现象
`cmd/backfill-fingerprints` 调用 `CapabilityTopicTagging`（provider=qwen，model=Qwen3.5-4B）100% 失败：`parse_error: response does not contain extractable valid JSON`。
`ai_call_logs` 显示 `completion` 每次都打满（512 → 1024 → 2048 均打满）、`finish_reason=length`。把 prompt 从长段改成 8 行祈使句后**仍然打满**，说明不是 prompt 啰嗦。

## 受控实验（同 provider/model、`MaxTokens=1024`、`Temperature=0.2`、同一条输入）
| 变体 | completion | 结果 | `subject_key` |
|---|---:|---|---|
| V1 `JSONSchema: core.EventFingerprintSchema()`（**无 `Required`**） | 1024 打满 | 失败 | 模型在 `subject_key` 里循环生成 `product/other/other/other/...` 直到耗尽 |
| V2 同 schema + `Required: [subject, subject_key, action, object]` | **42** | 成功 | `"product"`（缺规范名，不合契约） |
| V3 V2 + `reason` 字段 | 54 | 成功 | `"product"`（同上） |
| V4 **只 `JSONMode:true`、不传 JSONSchema**，prompt 内给完整 JSON 示例 | **51** | 成功 | `"product:得帆apaas"`（**格式正确**） |
| V5 V1 + `Metadata.structured_stage` | 1024 打满 | 失败 | — |

## 结论与约束
1. **根因 = schema 缺 `Required`**（不是 prompt、不是 metadata、不是缺 reason 字段）。缺 Required 时约束解码器在 enum/自由字符串上不收敛，反复生成候选值直到撞上限。
2. **但补 `Required` 不够**：模型会把 `subject_key` 只填类型名（`product`），丢掉 `类型:规范名` 契约。**V4（只 JSONMode + prompt 示例）同时解决两个问题**，且 token 最省（51）。
3. **不能给 `core.EventFingerprintSchema()` 加 `Required`**：该 schema 被提取器 `sectionExtractionSchema()` 复用于**每个 tag 的 item**，加 Required 会导致 keyword/person 标签也被要求填 subject/action → 破坏既有打标。所以只在需要独立 schema 的调用侧处理（本项目选择直接不用 schema）。
4. **诊断方法论**：遇到「模型输出不了 JSON」先查 `ai_call_logs` 的 `finish_reason` 与 `token_usage.completion`，再看是否打满；`response_snippet` 目前只存错误、不存原始响应，需靠 `finish_reason` 间接判断（可改进项）。

## 同一模型的成功对照（供判断 token 量级）
- `topicgraph.story_assign_classify`：成功 2,140 次，completion avg **71** / max 156
- `tagmanagement.extractor_enhanced`：成功 1,255 次，completion avg **304** / max 2,002

**引用**：backend-go/internal/tagmanagement/service/core/extractor_section.go:71-79、backend-go/cmd/backfill-fingerprints/main.go、backend-go/internal/platform/airouter/router.go

<!-- pinned 2026-11-06T07:00:20Z -->
