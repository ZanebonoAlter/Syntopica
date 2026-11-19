# 语义版块流程（Semantic Board）

<!-- doc-impact-applies: backend-go/internal/tagmanagement/, backend-go/internal/topicgraph/, front/app/api/semanticBoards.ts, front/app/api/normalizers/boardRadar.ts, front/app/features/tags/, front/app/pages/tags/ | section=业务约束与不变量 -->
> 大功能：辅助标签与组合标签入库、SemanticBoard 匹配/升级/回填、版块治理面板。
> 跨端。互补：`flow/daily-report.md`、`flow/topic-graph.md`。

## 需求说明

SemanticBoard（语义版块）解决「把散装 event 标签组织成持久主题分区」的问题。event 标签每天产生数十上百个，用户无法在平铺列表里快速定位关心的领域。语义版块提供类似 BBS 论坛的持久概念版块（「AI 前沿」/「新能源」/「中美竞争」），让用户：

- **按版块浏览**：每个 section 通过辅助标签挂载到 1-3 个版块，形成折叠/钻取的分区阅读体验。
- **标签去重入库**：LLM 提取的辅助标签经 L1/L2/L3 三级去重，避免近义标签碎片化；组合标签（如「美债收益率」= 美国国债×收益率）按组件集合 L1 + 组合 embedding L2 两级去重。
- **版块自演进**：升级建议（board upgrade suggestion）按「方向 × 来源」四格生成——创建版块（标签簇→建新版）/ 版块扩充（锁定单版块→挂载候选）/ 创建组合（共现对→组合标签）/ 组合扩充（共现对→组合标签并挂载），每轮 LLM 只做一种判断（单一决策空间），用户确认执行或忽略；创建方向由定时任务自动涌现，扩充方向纯手动（必须选版块触发）。
- **粒度阶梯**：辅助标签（中性概念）→ 组合标签（指向性主题，匹配最强信号）→ 版块（持久分区）。组合标签是「中性概念无法表达方向」与「建版块太重」之间的中间粒度。
- **相关事件（related-events，enable-story-cross-day-continuation）**：读到一个 event 标签时，回答「同版块还有哪些事件值得一起看」。比较单位是 **event 标签自身的确定性文本**（名称 + 描述 + 当前有效辅助标签，版本 `related-event-v1`），不读 `subject_key`/实体 ID/指纹信任/thread 向量；只给**直接相关**，不合并标签、不做身份簇、不做阶段或发酵、不传递扩展。索引是独立的可丢弃派生缓存 `related_event_embeddings`，由显式刷新 CLI 生成，GET 只读（不调模型不写库）；阈值 `related_event_similarity_threshold`（默认 0.86，可在线调）与入板匹配参数相互独立。展示落在既有「归档文章 → event 标签卡 → 相关事件」按需展开，无新页面/新面板。

### 工坊视图入口（rework-workshop-into-radar-and-desk）

> **当前实现风险（2026-10-25 投诉复审，尚未修复）**：下述入口设计不代表旧能力已完整迁移。已确认候选版块的日报入口被 active 泳道门槛挡住、删除版块入口漏迁、站内正文阅读承载缩水；完整去向表、证据边界和恢复顺序见 [本 change 的独立 UI-review](../../../openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk/review.md)。本轮仅审查，无数据迁移或重生成要求。

```text
/tags                          L0 今日雷达（BoardRadarView）
  └ /tags/:boardId             L1 版块态势（BoardConsoleShell：矩阵 / 归档 / 编排 / 治理）
      └ /tags/:boardId/:date   L3 日报阅读台（DailyReportDeskShell 两栏：索引 + 正文）
```

- **L0 雷达**：单次 `GET /semantic-boards/overview` 取全部活跃版块；行按关注度降序；add-event-story-heat 起关注度 = `lane_heat` 量纲（版块当日各泳道 lane_heat 之和，保留一位小数，不再是 section 数加权），分档阈值临时 10/3（按全量 lane_heat 分布 P80/P20 标定后替换，不再沿用旧 section 计数量纲的 50/15）：≥10 带「热点」分档标记，≥3 记「关注」，<3 折叠进「常温」摘要；右栏只读且已收敛为**一块清单 + 行内徽标**（surface-story-events-in-radar 事件线化 → matrix-clue-fallback-and-rail-merge 单清单化）：「今日该读」= 跨版块全局 Top 8 事件线（原 5 条该读 + 3 条新起屏位合并），每条带版块名 + 来源数 + 热度 + 该版块自己的锚点日（来源数 = 独立参与者数，按内容身份去重——同 link 的多 feed 副本算 1 个来源，与 `board_stories.participant_count` 同源）；分档信息用**行内徽标**表达——`新起`（当日有证据 ∧ 近 3 日基线为 0，不显示倍率）/ `升温 N×`（有基线 ∧ 锚点日 story_heat ÷ 自身近 3 日基线 ≥2 ∧ 当日有报道证据），无徽标 = 普通当日事件线；独立的「今天可能升温」「今日新起」卡片、剩余计数摘要行与三态空文案已删除，只剩一行「跨天延续事件线建设中」脚注（仅 `cross_day_story_count == 0` 时出现，`> 0` 不出空态，缺字段不出脚注）；未回填版块不产条目并明示「N 个版块事件身份未回填，未纳入事件线排序」，端点超时降级时明示「本次事件线排序不完整」）;右栏底部另有「管线状态」块（`add-radar-pipeline-status` + matrix-clue-fallback-and-rail-merge 折叠）：**默认折叠成一行摘要**（状态词 · 日报 x/y · 队列 {标签队列文章数}[+{Embedding 队列标签数}]——两条队列量纲不同、不相加；异常词与计数在摘要行就可见），点击展开标签队列积压、日报生成时刻与兜底时刻、最新日报落点与当日日报状态，**异常态自动展开**（已过生成时刻未出日报 / 任一队列有失败任务 / 生成请求失败 / 队列活跃量 ≥100），异常消除回折叠；该块的**手动生成入口是右栏唯一的写动作**——队列未清空时禁用（手动生成不等队列，当日才完成打标的文章不会进入当日日报），清空后可按板块勾选生成（勾选 = 参与生成，默认全选；同日重生成整份替换，须显式警示）。总览报头「全局治理」承载既有全库动作，首次失败不把右栏空结果当业务结论。报头右上为操作区，依次是统计、返回主页、「全局治理」、新建版块；返回主页紧邻「全局治理」左侧且与其同档（min-height 2rem / 0.7rem 字 / border-medium / hover 变 accent），标题区不另挂返回入口（8F / FB-13）。`/tags?report=&annotation=` 旧深链进入 L0 后反查日报日期并 replace 到 L3 阅读台（8.4 物理清理前旧组件文件保留但不再从入口渲染）。**小矩阵逐格降级（matrix-clue-fallback-and-rail-merge）**：雷达行小矩阵格子值 = 该泳道该日事件线数 > 0 用事件线数，否则按**该日分区数**兜底（`story_counts[i] > 0 ? story_counts[i] : counts[i]`；`counts[i] = len(day.Sections)`——雷达行实际可得的口径就是分区数，控制台矩阵可得线索合计、那边用线索）；降级格 title/aria 明示「无事件线（按分区数 N 显示）」，SHALL NOT 静默冒充事件线数；事件线数为 0 且分区数也为 0 时不标降级（真断档），`story_counts` 缺省（旧后端）沿用整轴回退 `counts`。降级只影响展示，MUST NOT 进 `lane_heat`、行序或关注分档。**客户端边界（8A/R-18）**：后端 snake_case JSON 协议不变，仅在 Radar API 分支归一为 camelCase/字符串 ID 读模型（唯一实现在 `front/app/api/normalizers/boardRadar.ts`，`useBoardRadarApi().getOverview` 接线），总览簇组件（`components/console/BoardRadarView.vue` 壳 + `BoardRadarMasthead/Story/Cold/ReadingRail/Governance` + `boardRadarHelpers` + `composables/useBoardRadarOverview`）只消费归一后读模型；首次取数失败不展示业务空态/初始化 0 结论（R-17），有旧数据时刷新失败保留旧值。**版块编辑入口（lane-manual-correction-loop）**：雷达行尾「⋯」菜单（`console/BoardRadarStory.vue` + `console/RowActionMenu.vue`）给出「编辑版块」（轻编辑弹窗 `console/BoardQuickEditDialog.vue`，md 档 560px，字段与治理工作面「设置」页签同源 `console/boardEditFields.ts`）与「归档版块」（`console/BoardArchiveDialog.vue`，手打版块名确认）两项；菜单点击阻止冒泡、不触发行进版块态势页。
- **L1 版块态势**：`BoardConsoleShell` 三区壳（版块栏 / 主舞台 / 上下文右轨）；顶部视图 seg=矩阵·追踪｜文章归档｜研究工作台，另有唯一「打开日报 · MM/DD」、板块管理与新建泳道。**版块编辑入口（lane-manual-correction-loop）**：① L0 雷达行尾「⋯」→「编辑版块」轻编辑弹窗；② L1 顶栏「板块管理」下拉的「板块设置 / 删除」→ 治理工作面 `console/BoardWideWorkface.vue` 的「设置」页签（`console/BoardSettingsForm.vue`，名称/描述/排序/受保护 + 删除）；两块入口字段定义同源 `console/boardEditFields.ts`，旧 `BoardEditDialog.vue` / `BoardGovernanceDrawer.vue` / `BoardGovernanceRail.vue` 三套僵尸实现已删除（避免第四套并行）。矩阵/演进/编年共享 `?lane=` 与一份选中日；选中日点格后下方就地读，不再开 `SectionDrawer`。右轨顺序=版块体检（真实 `llm_score`，不是雷达 attention）→ 构成候选 → 话题追踪（管理仅 `boardId`）→ 来源构成（独立 7/30/90 滚动窗，不随时间视窗）；全库六项/新建版块唯一入口仍在 L0 总览，不在此重复。窄屏 `<768px` 不渲染矩阵/常驻右轨，改 `BoardNarrowSummary`（全部可选泳道 + 休眠折叠 + 下方同一编年 + 「本板块上下文」入口）；新建泳道提示需要宽屏/横屏。
- **L1 归档**：日期是主筛选（默认数据锚点当天，可切近 7/14/30 天与任意起止日期）；文章按日期平铺、行内显示全部命中标签，点文章抽屉看逐标签规则/分数/方向校验与是否进入日报；不按话题分组、只读。点命中标签 chip 开「标签卡」popover（`ArchiveMatchPopover.vue` tag 模式）：头区给「第 N 步 · 规则名 命中」与指标行「相似达标 x/N · R · Smax」，正文按「结论 → 判定过程 → 收编证据 → 描述 → 本文标签 → 参数」排列——判定过程（clarify-archive-match-rules）按 ⓪→⑤ 就地逐条列出已试过未通过的步（给真实差距）与命中的那一步（含问法原文与「这一步怎么算的」就地 KaTeX 公式，缺 match-detail 不渲染入口），命中步之后合并一排「未走到（第 N 步命中即停）」；两个口径显式分家：收编 M/N 标「相似 ≥ 阈值，非完全相同」（hits 口径），① 交集标「完全相同」（`direct_hit_auxiliaries` 口径）；规则名/问法/编号单一常量源 `archiveMatchFlow.ts RULE_COPY`（与工具行「匹配流程」弹窗、入板过程阶段名同源）；点「入板」chip 开入板过程 popover（article 模式，⓪①③④⑤五阶段分组 + 命中即停语义）。
- **L3 阅读台**：独立路由两栏（分区索引 + 正文），日期历史并入索引顶部下拉；删除最左历史栏与右栏演进/本期话题，页边注收为右下抽屉；正文独立滚动；文章抽屉 60vw，窄屏全屏。
- **热度/缓存口径（retire-story-adopt-event-metrics 起为线索数量纲；fix-lane-metric-provenance 收敛为单一口径）**：泳道行序 = `lane_heat` 降序 → 全窗线索合计（`thread_count_14d`）降序 → topic id 升序；矩阵日格 = 当日各主题桶**非综合稿**线索数之和（`clue_count`，同日多主题桶合并且不取桶峰值），演进/趋势节点与顶部时间视窗曲线同量纲；事件线量纲（`story_count` / `max_participant_count` / `hottest_story_*` / `stories_backfilled`）整体退役，`section` 数 / 桶数 / thread 总数 MUST NOT 参与热度与排序计算。旧口径（行序=source 全轴 thread 合计、矩阵格值=同日 bucket max、演进=同日 thread 合计、事件线数）已废止，详见泳道动态节尾的口径变更说明。前端 lane-dynamics 仅当前板块 4 档缓存（365 档与池共用一次请求），报告列表索引 1 份、报告详情 ≤14、文章详情 ≤20，失败不入成功缓存、当前展开对象不因 FIFO 逐出消失。

## 链路设计

### 辅助标签入库（L1/L2/L3）

```mermaid
flowchart TD
  TAG[tagging/extraction: LLM 提取 tag + 3-5 辅助标签] --> SVC[auxiliary_label_service.go]
  SVC --> P1["阶段一（无事务，慢 I/O 区）<br/>L1 内存预解析 + 批量 embedding（merge 批 1 次 + storage 批 1 次）+ L2 决策"]
  P1 --> L1{L1: slug/alias/normalizeKey 精确匹配}
  L1 -->|命中| R1[复用 ref_count++]
  L1 -->|未命中| L2{L2: embedding ≥ 0.95}
  L2 -->|命中| R2[小方加入 aliases ref_count++]
  L2 -->|未命中| L3[L3: 新建 semantic_label<br/>label_type=auxiliary + 双向量已备齐]
  R1 & R2 & L3 --> P2["阶段二（短事务，毫秒级纯写）<br/>alias UPDATE / INSERT / link OnConflict / ref_count++ → 提交后增量更新缓存"]
  P2 --> REL[topic_tag_semantic_labels 记录关联]
```

> **两阶段事务边界（fix-aux-label-idle-tx-cache，2026-10-06）**：历史实现在单事务内逐标签调外部 AI embedding API，是生产 `idle in transaction` 4–40s 的元凶（事务持 xmin 阻塞 vacuum，与 autovacuum 调优对抗）。现在 `AttachAuxiliaryLabels` 先在事务外完成全部慢 I/O（缓存加载、批量 embedding——`airouter.Embed` 原生 `[]string` 输入，N 个新标签恰好 2 次调用），再用一个短事务完成全部纯写；全有或全无原子性由阶段二保留（任一写失败整体回滚）。同模式已清理 `ConfirmSuggestion`（板向量/composite 短语 LLM 均事务前预计算，事务内 `ApplyCompositeLabel` 纯写）。规划与写入之间的 slug TOCTOU 与旧事务内「查后插入」一致，靠 `idx_semantic_labels_slug` 唯一索引兑底：冲突冒泡由打标 job 记 warn 跳过。批内重复 label（同 slug/normalizeKey）在阶段一去重共享首个决策；批内语义近似但文本不同的标签各自新建（旧串行路径靠缓存回灌 L2 归并，属罕见场景，后续手动合并治理）。PG 侧另有 `idle_in_transaction_session_timeout=60s` 防呆（`docker-compose.pg.yml`，recreate 生效）。

> **缓存失效增量维护（同 change）**：L2 去重依赖包级 `packageAuxLabelCache`（17.5k 行 merge 向量 ≈ 216MB 文本传输 + 解析，TTL 10 分钟），历史「一写全清」使批量打标每个新标签炸一次缓存再全量拉。现在 L3 新建直接把 merge 向量塞进 map + activeLabels append 轻量副本（列集合对齐、不含向量列）；disable/alias 合并源按行删除；alias 变更仍只清轻量的 activeLabels；时间戳不重置 → 批量打标零全量重拉，仅冷启动 1 次。handler 直改 status 的路径不经 service 失效，由 TTL 兑底（窗口与历史一致）。

> **L2 不会形成「合并黑洞」**：与主标签路径不同（`findOrCreateTag` 的 embedding 命中曾覆盖 label/slug → text_hash 变 → 重生成 embedding → 恶性循环，见 `v1.3.1/fix-tag-blackhole-embedding-match`），aux 的 L2 命中只 `addAlias`（append alias + ref_count++），**不改 Label、不重算 MergeEmbedding**（MergeEmbedding 仅 L3 新建时生成一次，之后恒定）。既有 aux 的「吸引力」= 固定 embedding 的 cosine，不随 alias 增多 / ref_count 升高而自我放大，无循环根因。阈值 `auxiliary_label_dedupe_sim` 可配（默认 0.95）。

> **空结果是合法结论，泛词进不了池**（fix-tagging-pollution，2026-09-25）：LLM 返回空 `keyword_tags` / 空双数组 / 零候选都是「宁缺毋滥」提示词下的合法输出——不再用规则词回填（旧回填把分类名「新闻」与 HTML 属性子串误命的「Coding」以 llm 来源写库），也**不降级 heuristic**（retire-heuristic-tag-fallback，2026-11-12 起规则词兜底打标整体退役——提取失败即失败：文章不写任何标签，打标任务标失败并进既有队列退避重试）；零候选文章保持无标签等待重打。**响应形态容错**（tolerate-schema-envelope-extraction-output，2026-11-12）：小模型在 JSON-schema 引导下可能把 **schema 骨架本身**当输出（数组塞在 `properties` 下，形如 `{"type":"object","properties":{...},"required":[...]}`），解析器 SHALL 识别该外壳并取 `properties` 内对象继续解析（判定须同时满足：顶层是对象 ∧ `properties` 是对象 ∧ 含 `type` 或 `required`；幂等；非外壳原样返回）；解包后目标键仍缺 SHALL 报解析错误触发重试，MUST NOT 当合法结果。**纪律：提取 prompt 的输出格式指令 MUST 与下发 schema 一致**——merged 路径指双数组 `event_person_tags`/`keyword_tags`，section 路径指单数组 `tags`，两边不可互相串用。入库前另有泛词黑名单（新闻/论坛/要闻/快讯/文章/内容/技术/发展，Slugify 后完整匹配）在 `persistArticleTags` 入口拦截，跨 feed reuse 复制路径在 siblingLinks 查询处同滤——任何来源的泛词标签不落库、不进辅助标签池。

### SemanticBoard 匹配（add-composite-labels 后五级优先）

```text
semantic_board_matching.go
  → 读取 tag 辅助标签 + tag 组合标签 + active Board composition（aux + composite）
  → composite_hit（组合交集 ≠ ∅，score=1.0，免方向校验）
  > direct_hit（aux 交集 ≥ min_overlap，score=direct_hit_score_factor 默认 0.7，强制方向校验）
  > hit_rate / max_sim / weighted（三间接规则，不变）
  → 写入 topic_tag_board_labels（最多 3 个 Board）
```

- **composite_hit**：tag 挂的组合标签（显式关联 ∪ **推导命中**：tag 挂齐某 active 组合的全部组件 aux 即视为挂该组合——确认组合后重算即生效的闭环来源）∩ board composition 挂的组合标签 ≠ ∅ 即命中，score 恒 1.0、`direction_mismatch=false`（组合命中即指向一致）；同 tag-board 同时满足单标签重叠时只记 composite_hit。组合 embedding 为 NULL（disabled/历史缺失）的组合不参与判定；全量组合组件集进匹配缓存（随 InvalidateBoardData 失效）。
- **direct_hit 降级**：单 aux 重叠是「话题域相同」的弱证据，score 从 1.0 降为 `semantic_board_match_direct_hit_score_factor`（默认 0.7，可配 1.0 恢复旧分数），且与间接规则一样强制方向校验（全库 gap 豁免+绝对+相对三层，见下方红线 4，前端默认隐藏）。0.7 让 direct_hit 仍强于典型 weighted 但弱于组合命中与高质量 hit_rate。
- 只挂组合（无 aux）的 board 不被 aux 数量预过滤拦下，仍可被 composite_hit 命中；board 组合数据进 board match cache（与 auxiliaries/embeddings 同失效语义，composition 变更即失效）。

### 组合标签（composite label，add-composite-labels）

```text
创建（手动 / 升级建议确认，同一服务）：
  composite_label_service.go CreateCompositeLabel
  → 组件校验（2-5 个不同 active aux，去重保序）
  → L1：组件 ID 集合与既有组合（含 disabled）完全一致 → 复用（ref_count++）
  → L2：组合 embedding cosine ≥ composite_label_dedupe_sim（默认 0.95，仅比 active）→ 只 addAlias（防黑洞纪律同 aux）
  → 均未命中：LLM 对「label. description」短语生成 embedding（禁组件向量合成）→ 建 semantic_label(label_type=composite) + composite_components（position 有序）

升级建议产线（compose 决策）：
  semantic_board_compose.go collectComposeCandidates
  → 同一文章内共现（CoTagWindowDays 窗口）的 aux 对/三元组，频次 ≥ semantic_board_upgrade_composite_min_cooccurrence（默认 10），组件 ref_count 达升级阈值
  → 三元组达标时吸收其子 pair；候选按频次降序限 top 20
  → 独立 LLM 裁决（mode=compose，坏 JSON/超时降级跳过本轮 compose 段）→ decision=compose 建议落库（evidence 带共现频次/窗口/代表事件标题）
  → 生命周期全复用：suggestion_hash 幂等 / dismiss 冷却 / 确认事务

确认执行（compose）：
  ConfirmSuggestion 同一事务内调 CreateCompositeLabel（含去重复用路径）+ MarkConfirmed；
  embedder 失败等任何错误整体回滚，建议保持 pending
```

冷启动序列：迁移 → 跑一轮 compose 建议 → 用户确认一批组合标签 → 一次性 `mode="all"` 匹配重算（direct_hit 存量降级重写、组合命中重算为 composite_hit）。

### 升级建议生命周期（split-board-upgrade-directions 四格矩阵）

> **split-board-upgrade-directions 变更**：生成入口从「discover_new/expand_existing 双模式（LLM 全决策空间混跑 + watch 观察池 + 高置信自动合并）」重构为「方向 × 来源四格矩阵 + 锁定单版块 + LLM 单一决策空间」。背景：旧模式 LLM 一次背三种决策，merge 目标产出不可信（实测 17 条全不在算法 shortlist、经常缺 target），前端被迫兜底成人工全量挑目标。watch 观察池与高置信自动合并退役；旧内存探索 UI（candidates/clusters/内存建议）退役，持久化建议为唯一数据源。

#### 四格矩阵与 LLM 决策空间

| 方向 | 来源 | LLM 只答 | 确认动作 |
| ------ | ------ | ------ | ------ |
| 创建版块 | 单标签 | 这个簇值得开新版块吗（create_new\|skip，prompt 带全量版块清单防重复，按簇质心相似度截断 top-60） | 建版块 + 挂 aux |
| 创建版块 | 组合标签 | 这对共现值得变组合吗（compose\|skip） | 建组合标签（去重复用） |
| 版块扩充 | 单标签 | 这个候选属于版块 B 吗（merge\|skip，二分类） | aux 挂进 B |
| 版块扩充 | 组合标签 | 值得组合且属于 B 吗（compose\|skip） | 建组合标签 + 同事务挂进 B |

- 单例簇（size=1）不产任何建议（不进 LLM、无观察池）；全部建议经 LLM 裁决（无合成旁路）；skip 不落库不返回。
- 扩充方向 target 由服务端注入（= 生成前锁定的版块），LLM 输出不含目标字段——从根上杜绝缺 target / off-target 兜底逻辑复活。
- 扩充候选召回（双路并集去重、排除已挂载/disabled、各路上限 40）：相似路（aux embedding 与版块 embedding 余弦距离 ≤ `semantic_board_expand_sim_distance` 默认 0.35）+ 共现路（与版块构成标签同文章共现 ≥ `semantic_board_expand_cooccurrence` 默认 3，窗口同 CoTagWindowDays）；组合路要求至少一组件 ∈ 召回集 ∪ 版块构成集。携带 days>0 时扩充各路收紧：相似路加近 days 天文章引用过滤（批量 EXISTS），共现/compose 路窗口取 min(days, CoTagWindowDays)。
- 版块画像 prompt：版块描述 + 构成标签（组合带标记）+ ≤8 条近期 section 标题（查询失败降级为名称+描述，不阻断）。
- days 时间窗四格恒携（expand-upgrade-days-window）：create×aux 按文章活动过滤候选；expand 相似路要求近 days 天文章中引用过、共现/compose 路窗口取 days 与 CoTagWindowDays 更严者；create×composite 携带但后端忽略；`0`=不过滤（与无时间窗现状一致）。

#### 建议状态机

```mermaid
stateDiagram-v2
  [*] --> pending: 生成入表（hash 幂等 + 冷却检查）
  pending --> confirmed: 用户确认执行（事务联动）
  pending --> dismissed: 用户忽略
  dismissed --> pending: 冷却期满，下一轮可重生
  confirmed --> [*]
```

> watch 状态已退役（split-board-upgrade-directions）：存量 pending watch 行由迁移 20260905_0002 一次性清理；decision 枚举保留 watch 值仅为存量行 DTO 兼容，生成侧不再产生。

#### 生成链路

```mermaid
flowchart TD
  TRIGGER[触发: 定时 06:30（仅创建方向两段）/ 手动四格] --> ROUTE{direction × source}
  ROUTE -->|create×aux| C1[聚类 → co-tag 事件<br/>+ 全量版块清单防重]
  ROUTE -->|create×composite| C2[co-tag 共现对<br/>≥10 且组件达标]
  ROUTE -->|expand×aux| E1[版块画像 + 双路召回<br/>相似 + 共现]
  ROUTE -->|expand×composite| E2[组合候选 ×<br/>版块相关性过滤]
  C1 & C2 & E1 & E2 --> LLM[LLM 单一决策空间裁决<br/>每轮只面对一种判断]
  LLM -->|create_new / merge / compose| PERSIST[InsertPending<br/>hash 幂等 + 冷却检查]
  LLM -->|skip| DROP[不落库]
  PERSIST --> EVI[快照 evidence<br/>cotag_events / source=expand 等]
```

#### 触发方式

- **定时**：scheduler `job_board_upgrade_suggest`，默认每日 06:30（`semantic_board_upgrade_suggest_time` 可配），**仅创建方向两段**（{create,aux} → {create,composite}），段失败仅记日志继续兄弟段不阻塞；版块扩充纯手动（必须选版块触发，spec：定时生成仅创建方向）。
- **手动**：前端生成入口（方向两选 → 来源两选 → 扩充时版块单选）→ `POST /api/semantic-boards/upgrade-suggestions/generate?direction=&source=&target_board_id=&days=` → `GenerateAndPersist`，返回 `{inserted, skipped, cooldown_blocked}`。参数校验：expand 缺 target / create 带 target / target 非活跃版块 / 旧 mode 参数 → 400。

#### 跨端协作

```mermaid
sequenceDiagram
  participant SCH as Scheduler 06:30
  participant FE as UpgradeSuggestionPanel
  participant BE as backend
  SCH->>BE: GenerateAndPersist({create,aux}) → ({create,composite})
  FE->>BE: POST /upgrade-suggestions/generate?direction=expand&source=aux&target_board_id=42
  FE->>BE: GET /upgrade-suggestions?status=pending
  BE-->>FE: 持久化建议列表（扩充建议 target 恒=锁定版块）
  alt 确认执行
    FE->>BE: POST /upgrade-execute {suggestion_id}
    BE->>BE: 事务: 写 board_composition + MarkConfirmed（compose 带 target 时同事务建组合+挂载）
    BE-->>FE: 成功，建议 → confirmed
  else 忽略
    FE->>BE: POST /upgrade-suggestions/:id/dismiss
    BE->>BE: status=dismissed（冷却期内同 hash 不重生）
  end
```

#### 前端面板分区（UpgradeSuggestionPanel）

- **生成入口（顶部）**：方向两选（创建版块 / 版块扩充）→ 来源两选（单标签 / 组合标签）→ 扩充时版块单选下拉（可搜索，仅活跃版块）+ days 下拉（四格恒显恒携，expand-upgrade-days-window）；未选版块时生成禁用；生成错误行内提示；空态区分「未生成过（引导）」与「本轮无建议（扩充方向附覆盖提示）」。
- **持久化建议列表（唯一数据源）**：决策过滤 tab（全部 / 合并 / 新建 / 组合，观察池 tab 已删）+ evidence 展示（泳道标题 / 共现事件 / 组合证据，缺 key 降级不渲染）+ per-row aux 勾选子集 + 确认执行（带 suggestion_id）/ dismiss。扩充建议卡片展示锁定版块徽标（「→ 美债」），merge 行确认按钮直接合并进锁定版块（无「合并到...」改目标下拉——目标不合适应 dismiss 后换版块重新生成）；compose 建议带 target 时按钮为「创建并挂载」。
- **旧内存探索区（candidates/clusters/内存建议/「获取 LLM 建议」）已整体退役**。

#### 配置项（ai_settings，均可缺省）

| key | 默认 | 说明 |
| ----- | ------ | ------ |
| `semantic_board_upgrade_suggest_time` | `06:30` | 定时生成触发时间点（仅创建方向） |
| `semantic_board_upgrade_suggestion_dismiss_cooldown_days` | `14` | dismissed 冷却期（期内同 hash 不重生） |
| `semantic_board_expand_sim_distance` | `0.35` | 扩充召回相似路阈值（与版块 embedding 余弦距离） |
| `semantic_board_expand_cooccurrence` | `3` | 扩充召回共现路阈值（与版块构成同文章共现篇数） |

> 已退役配置键（残留行无害，代码不再读取）：`semantic_board_upgrade_watch_gc_days`、`semantic_board_upgrade_merge_confidence_margin`。suggestion_hash 的 mode 维度值改为 `direction:source`（create:aux / create:composite / expand:aux / expand:composite），新旧建议 hash 空间天然隔离。

### SemanticBoard 管理 / 回填

```text
SemanticBoard 管理面板
  → 辅助标签入库: L1 slug匹配 → L2 embedding合并 → L3 新建
  → SemanticBoard 匹配: 三规则挂载 → topic_tag_board_labels
  → 升级建议: 见上「升级建议生命周期」（持久化 + 双签名 + 观察池 + 定时生成）
  → 辅助标签治理: 禁用、alias合并、composition移除、suggest-auxiliaries、clusters、gc
  → 回填: all / unassigned / board 三种模式 + backfill-embeddings + rematch-all
```

#### 版块运维端点（`board_crud_handler.go` / `board_match_handler.go` / `board_upgrade_handler.go`）

| 端点 | 业务用途 |
| ---- | -------- |
| `POST /api/semantic-boards/backfill` | 入队一个 backfill job（`SemanticBoardBackfillRequest`，all/unassigned/board 三模式），返回 job 对象 |
| `GET /api/semantic-boards/backfill/:id` | 查询 backfill job 状态/进度（前端 `BackfillProgress.vue` 轮询） |
| `POST /api/semantic-boards/backfill-embeddings` | 为 `embedding IS NULL` 的版块生成 embedding（一次性补齐；`board-direction-check` 引入版块向量后的回填入口） |
| `POST /api/semantic-boards/rematch-all` | 取所有已挂载 `topic_tag`，逐个重跑 `MatchTopicTag`，返回 `{success, failed, total}`（匹配阈值调整后全量重算） |
| `GET/PUT /api/semantic-boards/matching-config` | 读/写匹配阈值（`ai_settings` 中 13 个 `semantic_board_match_*` key；PUT 后调 `InvalidateMatchingConfigCache` 失效缓存）。前端 `MatchingConfigDialog.vue` |

#### 辅助标签治理与建议（`board_crud_handler.go` + `service/auxlabel/`）

| 端点 | 业务用途 |
| ---- | -------- |
| `GET /api/semantic-boards/suggest-auxiliaries?label=&description=` | 全局建议：embed 查询文本 → 与 active aux cosine 排序，分页返回候选 |
| `GET /api/semantic-boards/:id/suggest-auxiliaries` | 版块级建议：以版块 `label+description` 为查询，排除已在该版块 composition 里的 aux |
| `GET /api/auxiliary-labels/clusters` | 聚类：cosine 距离 < 0.2 的连通分量（size≥2），10 分钟缓存，`?refresh=true` 强制重算 |
| `POST /api/auxiliary-labels/gc` | GC 回收，`mode` ∈ `dry_run/disable/delete/recalculate`，可选 `grace_days` |
| `POST /api/auxiliary-labels/merge-alias` | alias 合并（source→target） |
| `POST /api/auxiliary-labels/:id/disable` | 禁用单个 aux |
| `GET/POST /api/semantic-boards/:id/composition`、`DELETE /:id/composition/:aux` | 版块 composition 增删查 |

前端治理 UI（`features/tags/components/`）：`AuxiliaryLabelPool.vue`（辅助标签池）、`AuxiliaryLabelPicker.vue`（选择器，双模式：版块推荐/手动指定）、治理工作面 `console/BoardWideWorkface.vue`（单级宽工作面，页签 构成/组合/候选/来源/设置——体检依据与处置 `console/board-health/BoardHealthWorkspace.vue`、构成明细 `console/BoardHealthDetail.vue`、来源 `BoardSourcePanel.vue`、设置 `console/BoardSettingsForm.vue`）、右轨体检摘要 `console/RailHealthBrief.vue`、`composables/useAuxiliaryLabels.ts`。

### 泳道动态（board-lane-dynamics，取代已退役的话题态势版图）

版块态势页 `/tags/:boardId` 当前是**观察窗 A 路线（名单稳定）**：独立 7/14/30/365 档只由档按钮/显式 URL `days` 改变，决定 A 档名单、前端 source 全轴行序（线索合计 DESC → 分区数 DESC → topicId）与休眠分组（≤2 分区，只展示不写 status）；自由 `start/end` 只裁切显示，恰 7/14/30/365 也不换档。365 池（B）在 A 数据到位且锚点已知即**预取**（顶部时间视窗背景曲线首屏即需整条 365 池曲线，不得只画当前档窗那几天、等用户拖出档窗才补历史；可见日跨出档窗只是复用已取到的池），B **只补 A 名单内同 topic ID 的历史内容**，不参与名单/排序/休眠/候选；池未取到/失败时历史日显示 `?`（不补 0），池提示与「重试历史池」无条件出现（A 侧照常可读、名单/排序不动）。

主舞台：矩阵行=泳道、列=可见日期；日格数字=**当日事件线数（story_count，同日多主题桶合并计数，不取桶峰值）**，无分区 `—` 禁用、有分区 0 事件 `0` 可开（零事件是合法结果）、历史未取到 `?` 不补 0，无行尾合计列；格颜色=当日最热事件线的独立参与者数（max_participant_count，按内容身份去重——同 link 的多 feed 副本算 1 个来源，与 `participant_count` 同源）分档，title/aria 同时给出事件线数/最热来源数/未归组事件数/主题桶数（颜色不是唯一信息载体），表头/footnote 有图例（「数字 = 当日事件线数；颜色 = 最热事件线的来源数」）与「事件身份未回填」降级说明。选中泳道演进节点=**当日事件线数（day story_count）**，点径随数值增大；>31 日切趋势图+折叠逐日索引。下方当日编年默认标题收束，单条就地展开真实 summary，相关文章按需，稳定 report/section/thread ID 关联；「打开所选日日报」是带完整日期的历史动作，顶部最新日报入口独立。接口契约见 `docs/reference/api/daily-reports.md` §`GET /semantic-boards/:id/lane-dynamics`。

> **口径变更（add-event-story-heat，2026-11-05 起）**：旧口径「日格 = 同日各主题桶 thread_count 的峰值（bucket max）」「演进节点 = 当日线索合计（day sum）」已被上述事件线口径**取代**——桶峰值随 LLM 分桶 `k` 漂移且同日多桶会重复抬高格值，不再是格值定义；事件线（`board_stories`）身份、归属判定与热度公式见 `flow/daily-report.md` §事件线归属与跨期去重。

- **当日其他话题**：矩阵保留折叠组，成员只来自所选日报，沿既有 sectionId/topicId 排除正式与休眠泳道；历史状态取 `topic_status_at_report`，不借当前候选资格。行高/标题/实底热度格/选中与泳道共用视觉，只绘所选日格。选中后，现有日期表头（窄屏复用同源日期条）用黄点标记同 `persistentTopicId` 的可见窗真实记录日，hover/focus 显示当日分区原题；同日多分区只一个点、仍定位原序首 section，不合并正文、不跨新旧 ID 自动补连。点黄点通过 `selectObservedDay` 原子切日期/report/section/topic，普通报日仍走原 `selectDayHeader`；换选更新、清选择/无 ID 清点。**已验证记录日**（黄点当日：观察带里同 ID 的真实分区记录，自带 report/section，证据强于报日索引）在报日索引缺该日（陈旧/不全）时不被选中日普通回落搬走；只保护当前这一个观察目标，普通选行/清场/切泳道/切板块/换日即失效，范围排出（轴外）照旧回落；目标实际无日报时保留日期与目标并显示诚实「所选日没有日报」态，不借旧正文、不退回别日。观察复用 days=0 单份板块缓存，失败/缺 ID 说明及重试留在阅读区，但不再重复日期药丸。正文复用泳道标题/分隔/线索展开，不产滚动态势；0 正式泳道未选其他时只保留选日/选话题方向提示，不画无效选泳道卡。此增量只读、无迁移/重生成/配置，旧无 ID 数据无关联点，缺分区如实提示。组 summary 的总数徽标 = 组内线索合计（= 各成员行所选日线索数之和，与矩阵主行格值同口径，**不是分区数**；分区数只在「N 个分区未入泳道」类说明文案里出现），折叠与展开态同数，非 ready 不冒 0。
- **展示范围/休眠**：A 档响应中的 active ∪ watch 关联泳道（`board_topic_watches.persistent_topic_id`）；行序按 lane_heat 降序（平手依次 story_count_14d 降序、topic id 升序），不按可见裁切重算；休眠（source 全窗分区 ≤2）仅折叠展示，不改生命周期；candidate 只进候选区/编排入口，需人工激活，不自动转正。
- **日期/锚点**：矩阵轴终点 = A 数据锚点（优先版块独立最新日报日期，0 active 泳道版块同样有；否则回退 A 最新有分区日），与后端 `lane-dynamics` 的 `[anchor-(days-1), anchor]` 对齐；今天无日报时列终点不为空白「今天」，另给「数据截止」提示。雷达每行 `anchor_date` 与全局 `date` 分别表达。
- **态势结算**（日结算异步，见 `flow/daily-report.md` §泳道态势结算）：每日日报生成后为活跃泳道滚动重算 ≤100 字态势句（输入=窗口内「日期+节标题+前3线索」，LLM），存 `topic_lane_snapshots`（每泳道一行覆盖更新）；失败不阻塞日报，下个日报日自愈；快照缺失时卡片「待结算」降级、时间线照常。
- **代码入口**：后端 `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go`（聚合 `GetBoardLaneDynamics`：当日线索合计（非综合稿）/主题桶数/lane_heat 排序 + 结算侧查询；**热度计算入口 = 同目录 `lane_heat_repository.go`**，`LaneHeatRuleVersion='heat-v2-lane-clues-1d'`（`ComputeLaneHeat` 唯一活实现：active 线索数 × 1.0 + candidate 线索数 × `lane_heat_candidate_weight`，只表达当天、不衰减不跨天；口径见本节上方「热度/缓存口径」与红线 29。旧 `story_heat.go` 的 `heat-v1-48h-hl24h` 已停写，见下「后端雷达清单」条）、`service/lane_snapshot.go`（结算管线，operation=`daily_report.lane_snapshot`）、`handler/lane_dynamics_handler.go`；前端观察窗/矩阵/演进/编年 `front/app/features/tags/components/console/{ObservationRangeControls,ObservationMatrix,ObservationLaneEvolution,LaneChronicle}.vue` 与 `composables/{useBoardConsole,useObservationWindow,useLaneDayReading,useOtherTopicObservation}.ts`、同目录组件 `DayOtherTopicsGroup.vue` / `ObservationMatrixDayHeader.vue` / `OtherTopicReading.vue`；API 边界 `front/app/api/laneDynamicsWindow.ts`、`front/app/api/normalizers/laneDynamicsWindow.ts`、`front/app/api/laneDayReading.ts`；旧 `lane-dynamics/LaneDynamicsPanel.vue`/`LaneDynamicsCard.vue` 文件保留但不再作为 `/tags/:boardId` 主路径渲染。

> 退役说明：原话题态势版图（分区卡片墙/活力顶栏/节奏气泡图/mini-lifeline/stance 五态派生）随 overview-lane-dynamics 整体移除（2026-09-10）；stance 派生与 ECharts 图表不再存在，历史设计见归档 change `2026-08-01-board-topic-landscape`。

> 变更溯源见本文件 [§变更溯源](#变更溯源)。

### 泳道工作面（unify-lane-workbench：三栏 / 节点卡显式写操作 / 模态本地时间窗）

版块态势页顶栏「新建泳道」与矩阵行入口打开的 `console/BoardComposeOverlay.vue` 同时承载两个模式；**修正模式（编辑既有泳道）为三栏工作面**：

```text
左栏 380px（已选侧）         中心画布（自适应）                右栏 300px（待选侧）
成员 / 更早成员 / 已移出      时间轴 + 节点散点 + 节点卡        关联推荐（估算接近度 + 「＋加入」）
首义范围（sticky 按钮）       点节点只 = 选中查看，不改成员    pending / 空态 / 错误态
```

- **左右不重复**：推荐只在右栏（`LaneRecommendationList.vue`），左栏（`LaneMemberList.vue`）收窄为成员/更早成员/本次已移出三组 + 首义（`LaneAnchorScope.vue`）；两侧各自独立纵向不均匀截断（块内滚动、操作按钮不被裁掉）。
- **显式卡片写入**：节点卡（跟随选中节点）上直接给「＋加入本泳道 / ⊘移出本泳道 / 加回来」，右栏「＋加入」与卡上按钮走**同一** `useLaneCorrection` 的 `addMember/removeMember/restoreMember`，一次显式动作 = 一次既有归属 PATCH；点画布本体/Enter 只改选中态（查看与选入分离）。写操作只写事实层：不动 `hit_count` / `consecutive_hits` / `status`（红线 20），移出不弹二次确认，首义仍只由左栏发起（次日生效）。
- **时间窗是模态本地态**：新建/修正各自持已提交范围 + 控件草稿 + 手势预览 + 请求序号，**不写 router/query**（矩阵深链 `days/start/end` 不被覆盖）；`preview` 不取数，`commit` 校验通过才取候选，失败恢复旧范围并保留草稿/旧清单；修正档 7/14/30/365（可编排上限 365，与后端 `ComposeWindowMaxDays` 同源）。
- **曲线口径（两条不混用）**：编辑曲线 = **本泳道逐日线索数**（`lane-dynamics?days=365` 目标 topicId）；新增曲线 = **全部可选线索数**（`compose-candidates?days=365&omit_embeddings=1` 后按日聚合非 active/无归属 section 的 `clue_count`，含无归属节点、不扣草稿已选）。两者都是 365 天历史参考，**不是**可编排范围；曲线未知不补 0（缺字段 / null = 未知，已知 0 照画）。轻量参数与窗口错误码见 [api/semantic-boards.md](../api/semantic-boards.md) §编排 / 修正工作面的候选读取。
- **修正质心含窗口外成员**：推荐质心 = 窗口内有效成员 + `early_members[].embedding`（后端全量成员口径），否则窗口内成员为 0 时质心为空、候选区全空；更早成员本身不上画布（只在左栏分组与节点卡可见）。

### 板块来源构成（add-source-board-hit-rate，当前右轨/单级工作面）

供血源视角的只读聚合（当前版块右轨简要 + 单级宽工作面全量；不随观察日期/档重算）：`GET /api/semantic-boards/:id/source-breakdown?window=7` 返回该版块窗口内的来源构成——`total_articles`（按文章去重）/ `source_count` / `sources[]`（`feed_id`、本版块篇数 `articles`、占比 `share`、该源窗口总量 `feed_articles`、该源整体命中率 `feed_hit_rate`）。与板块文章列表 `/:id/articles` 的口径差异：articles 是文章列表（分页/过滤、含匹配质量明细），source-breakdown 是按源聚合的命中构成——**含已归档、按文章去重（同版块多标签只计一次、同一文章命中多版块各计一次）、必限窗口**；不变量：`Σ sources[].articles == total_articles`。版块不存在或非 `label_type='board'` → 404；disabled 版块不是 404（按命中口径聚合为 0）。

```text
治理工作面（console/BoardWideWorkface.vue）页签流
  → 「体检」页签（console/board-health/BoardHealthWorkspace.vue：体检依据 + 分诊处置，吸收原构成管理）
  → 「来源」页签 → BoardSourcePanel.vue（窗口 7/30/90 + 汇总行 + 来源表，只读无写动作）
  → 选中版块变化时与 loadComposition 并行重取（请求竞态沿用既有面板惯例）
```

口径与源视角（`GET /api/feeds/board-hit-stats`，见 [reading.md](reading.md) §源视角观测）共用同一份聚合代码 `internal/tagmanagement/service/sourcestats/`；契约见 [semantic-boards.md](../api/semantic-boards.md)。处置入口不在面板内（仍在设置 → 订阅源）。

### 版块体检（add-board-health-checkup，每日纯读统计）

> 🗺 全链路图：[`diagrams/board-health-checkup.html`](diagrams/board-health-checkup.html)（双主题，含 2026-10-03 评分 prompt 方法学锚：insufficient ≠ 失效，<60 只看真实病征）

版块构成质量的自动反馈环：**三条日报产出路径都会触发体检**——① `DailyReportJob` 定时主轮成功裁决尾步（`failedCount==0`，同步执行，**零守卫**：每夜必须有当日依据）；② 回填补出至少一份日报之后（**异步 goroutine** + 6h 新鲜度守卫）；③ 手动生成端点 `POST /api/daily-reports/generate` 成功产出后（handler 内异步、scope 限本次板块，6h 守卫，panic 只记日志不污染日报 job）。体检对每个活跃版块算两级统计 + 事实画像 diff，LLM 单次合并产出评分/评语/打扰裁决，落 `board_health_snapshots` 快照（趋势可查，历史不覆盖），异常时经通知中心发 LinkType=board 通知（冷却默认 3 天）。**纯读硬边界：不写匹配表、不落任何扩充建议**（扩充建议仍必须用户手动触发，红线 10 不动）。

- **口径（唯一实现 `internal/tagmanagement/service/boardhealth/`，对标 sourcestats 纪律；2026-10-08 实测修正——初版「单 aux 纯净样本」口径因样本占比仅 2%-11% 且永不增长而废弃，实测数据见归档 change design D4）**：版块级（口径①全部匹配行）= matched_tags / reason_dist / ghost_matches（direction_mismatch 计数）/ vector_missing；标签级（口径②贴合度分布）= 每个构成标签 embedding × 该版块全部挂载 tags 的 identity embedding 现场重算 cosine（MUST NOT 按 tag 的 aux 挂载数量筛样本，不做功劳归因），分布 mean/median/p25/p75/low_sim_ratio（低分线=板块内各构成标签 cosine_mean 的 P25 相对线，自适应板块主题纯度），任一侧向量缺失跳过分布并计数（不按 0 计入），构成标签自身缺 embedding → 该标签 insufficient；samples < `board_health_min_sample_size`（默认 5）标 insufficient 分布置 null。
- **事实画像 diff（uncaptured_candidates）**：已挂到版块的 tags 所携带、但不在版块构成中的高频 aux（排除已挂载与 disabled），按 member_tags 降序存 top 50；只存候选不落建议。
- **member-derived 扩充主路（回退语义）**：手动扩充生成时候选来自最新快照的未收编清单前 `board_health_member_candidates_top`（默认 20，排除已挂载保险滤 + days>0 近期文章过滤）；无快照/清单空/滤后空 → 回退既有相似+共现双路（行为不变）。
- **LLM 降级**：评分单次调用（operation=`tagmanagement.board_health_checkup`，走 `topic_tagging` 能力路由）；坏 JSON/超时 → 统计照常落快照、score/comment 置 NULL，本轮跳过通知不重试。**分诊清单漏判不整轮作废**：模型合法但漏判部分构成标签（覆盖率 ≥ 50%）→ 缺失项补 ⚪ 证据不足、保留评分/评语/已给分诊；覆盖率 <50% 或结构性非法（闭集外 category / 未知或重复 aux_id / evidence 空 / rewrite 缺文案）才整轮降级（红线 27）。
- **触发面与预算（fix-board-health-triage-degrade）**：新鲜度守卫**只作用于回填与手动两条路径**——同板块最近已有**带评分**（`llm_score` 非空）的快照且距今不足 6 小时则整板跳过（不调 LLM、不写快照，只记 skip 计数）；**降级快照（无评分）不算新鲜**，否则降级过的板 6h 内连手动重跑都跑不了。主轮尾步是当夜首轮、**零守卫**——保证每天都有当日依据（白天手动跑过也不影响当晚主轮）。整轮预算 30 分钟（原 10 分钟），单板块另有 6 分钟子超时——慢板只降级自己，不吃掉后续板块预算。每板块每轮打一行日志（`labels/covered/filled/degraded/elapsed`，降级行也带真实 `covered`），降级行带 `reason`，成功路径也有日志（原先只有失败才打）。
- **配置（ai_settings，均可缺省）**：`board_health_min_sample_size`(5) / `board_health_alert_threshold`(60) / `board_health_notify_cooldown_days`(3) / `board_health_member_candidates_top`(20) / `board_health_magnet_gap`(0.08) / `board_health_magnet_min_failed`(5)（后两个是三病分诊的磁铁阈值，见下节）。
- **三病分诊与共享描述修复**：体检轮次在同一次纯读里多算三类确定性信号并把标签级分诊清单落进快照（`metrics.triage`），用户可据此逐项核对处置——详见下节「版块体检三病分诊与共享描述修复（board-health-triage-advice）」。
- **只读端点**：`GET /api/semantic-boards/:id/health-checkup?trend=N`（N 白名单 1-90 默认 7，非法 400）：latest + 趋势升序；版块不存在 404，disabled/非 board 返回空态（非 404，同红线 17 端点纪律）。
- **前端**：治理工作面「体检」页签 `console/board-health/BoardHealthWorkspace.vue`（原 `BoardHealthPanel.vue` 已随 rework-workshop-into-radar-and-desk 8.4 物理删除，其职能拆入本工作面）——评分徽章（好≥70/中 60-69/差<60）+ 评语 + 指标 chips + 质量表（行内移除，无快照「待体检」降级）+ 组合标签区 + 趋势 + 未收编候选（收编快捷动作 = 既有 addComposition 手动挂载，不走 LLM 建议管道）；右轨只透摘要 `console/RailHealthBrief.vue`。

```text
DailyReportJob 定时主轮成功（failedCount==0）
  → runBoardHealthCheckupTailStep（30min 整轮预算，panic 隔离，零守卫）
  → 回填阶段 backfilled>0 → runBoardHealthCheckupAfterBackfill（6h 新鲜度守卫，通常只花一次 SQL）
  → boardhealth.RunCheckup：遍历 active 版块 → 统计/画像 diff → LLM 评分/分诊 → normalizeTriage 补齐 → 快照落库 → 通知裁决（冷却）
  → 单版块失败/超时（6min 子超时）仅记日志继续兄弟版块

手动 POST /api/daily-reports/generate 成功产出
  → handler generate* 内异步 triggerBoardHealthCheckup（scope=本次板块，6h 守卫，30min 预算，panic 隔离）

手动扩充生成 expand×aux
  → recallExpandAuxCandidates → 最新快照 uncaptured_candidates 非空？主路 top N（保险滤 + days 过滤） ：回退相似+共现双路
```

### 版块体检三病分诊与共享描述修复（board-health-triage-advice）

体检从「一句笼统评语」升级为**可开药方的标签级分诊**：同一轮纯读里为每个构成标签算三类确定性信号，把证据喂给**同一次** LLM 调用，输出四类分诊清单（🔴 建议摘除 / 🟡 建议重写描述 / 🟢 保留澄清 / ⚪ 证据不足），随快照持久化（`metrics.triage`）。摘除与描述修复都**不在体检轮次内执行**：摘除走既有构成 DELETE 接口，描述修复走独立的持久任务链（人工确认后才创建）。

- **三病口径（唯一实现 `internal/tagmanagement/service/boardhealth/`，同轮同内存算，不重算匹配、不筛样本）**：
  - **磁铁失灵（磁铁信号 `magnet`）**：构成标签 embedding 对「`direction_mismatch=true` 的失败挂载子集」与「`direction_mismatch=false` 的健康挂载子集」的贴合度均值差（gap = failed_mean − good_mean，阈值 `>=`，可配 `board_health_magnet_gap` 默认 0.08）——**方向对齐「磁铁把跑题的挂载吸过来」**：只有失败侧贴合度**高于**健康侧才算嫌疑；旧口径（gap = all_mean − failed_mean，全体均值**含**失败行）与语义相反，2026-11-09 真库实证：失败挂载按定义就与本板方向不符、与核心主体天然不相似，旧口径于是「越是核心主体 gap 越大、越像污染源」（某中东板 47 个构成标签 41 个成立、伊朗 gap 0.124 全板靠前，信号失去区分度），已废止。失败侧样本数达 `board_health_magnet_min_failed`（默认 5）**且**健康侧样本达 `board_health_min_sample_size`（默认 5）才成立，**任一侧样本不足或构成标签自身向量缺失 → 标 `insufficient`，MUST NOT 按差值硬凑**。只做分组差统计，**不做「哪个标签吸引了哪个 tag」的功劳归因**（与红线 22 同纪律）。全体均值（`all_mean` ≡ `cosine_mean`）仍产出供历史对比展示，**MUST NOT 参与嫌疑判定**；磁铁信号携带口径标记 `caliber`（`healthy_subset` = 新口径 / 空 = 历史快照旧口径），消费方 MUST 按标记区分「旧口径快照」与「新快照但健康侧不可算」。
  - **核心主体标记（`core_subject`，不是第四种病，是防误杀标记）**：构成标签名（空白/大小写归一后）出现在版块 **description 或 aliases** 中 → 核心主体；**仅**出现在版块**标题**中时还须标签名 ≥ 3 字符（2 字短词可能是简称「美伊」也可能是泛化地域词「中东」，MUST NOT 据标题推断全名或泛化词）。判定 MUST 为纯字面匹配，MUST NOT 用向量相似度或语义推断。核心主体是版块自身要谈论的对象，**MUST NOT 作为摘除或重写对象**（规则层强制改判 keep + 证据标注「核心主体保护」）。
  - **描述污染（预筛 `desc_flags`）**：构成标签 description 命中固定文章式词根（`此次/本次/该事件/报道/针对/期间/近日`）→ 事件性嫌疑；description 为空不标记。词根集合是闭集常量，不从用户输入拼装。
  - **泛化漂移（最近邻 `neighbors`）**：构成标签在全库 **active 辅助标签**向量空间的 top-3 最近邻（排除自身）及其 cosine，作为「短概念标签最近邻是实例标签 = 语义窄化/漂移」的证据；自身向量缺失 → insufficient。
  - **不是第四种病**：「短名天花板」（短实体名 × 长版块名的 cosine 天然低，如 OpenAI 0.57、MySQL 0.45）是**防误杀判据**不是病症——分诊 MUST NOT 仅因 cosine 低归入摘除，评分层也不因此扣分。
  - **漏判补齐（不是三病之一，是输出契约）**：本轮模型合法但未覆盖的构成标签一律补 ⚪ 证据不足（证据写明「返回 N/M 条，下轮再判」），MUST NOT 补成 keep/remove/rewrite；覆盖率下限 50%，低于则整轮降级（红线 27）。
- **体检轮次纯读红线（红线 20 不动）**：体检只写 `board_health_snapshots`（含 `metrics.triage` 附属产物）与少量冷却内异常通知；MUST NOT 新增 `board_upgrade_suggestions` pending 行、MUST NOT 写 `topic_tag_board_labels` 匹配结果、MUST NOT 改任何 description/embedding。**分诊清单是建议不是执行**：🔴 摘除 MUST 经用户确认后调用既有 `DELETE /api/semantic-boards/:id/composition/:auxiliary_label_id`；🟡 重写 MUST 经用户另行核对共享影响并确认后才可创建修复任务。**人工修复与自动扩充的隔离边界不变**：版块扩充建议仍必须用户手动触发（红线 10），体检不自动挂载、不自动摘除、不自动重写。
- **组合标签在分诊中只读（仅摘除可处理）**：`metrics.labels[]` 逐条携带 `label_type`（`auxiliary` / `composite`；历史快照缺该字段 = 类型未知）。描述修复链全程只认 `label_type='auxiliary'`——**组合标签的 🟡 重写项只读展示**（证据照常展开、不隐藏、不当它健康，不提供修复入口、不拉必然 404 的修复上下文）；**🔴 摘除项不受类型限制**（走既有构成 DELETE 链，组合标签的构成关系同样可摘除）。类型未知按不可执行处理，MUST NOT 臆造类型。
- **共享描述修复的影响面（人工确认前必须声明）**：辅助标签是**全局共享**标签——改它的 description 会重算该标签的 storage 向量（`embed("label: description")`），并更新**全部活跃 topic_tags** 的版块匹配（冻结范围含未归属任何版块的标签），因此**可能改变其它版块的挂载归属**。确认前界面须声明：这是共享标签 / 直接引用它的版块都受影响 / 匹配更新覆盖全部活跃话题标签；用户取消则零写入。
- **任务状态机与恢复（持久任务，非内存 job）**：
  - 状态闭集 `pending → embedding → saving → matching → completed`，失败态 `embedding_failed` / `save_failed` / `matching_failed`，终态死路 `conflict`（来源版本已变）/ `unavailable`（标签非 active 或已删）；`stage` 取 `embedding|saving|matching`（`pending` 可空）。`description_committed` 标记「描述+向量是否已原子提交」。
  - **幂等键**：客户端每次确认生成唯一 `idempotency_key`；同键同载荷重发返回**同一任务**（不重复重嵌、不重复提交），同键不同载荷 → `409` 冲突（绝不默默返回旧任务当成功）。同标签存在未结束任务（含三种失败态）时新创建 → `409` 互斥。
  - **原子保存**：storage 向量在**事务外**生成，事务内锁行校验来源版本/类型/状态后**原子写 description+embedding+任务阶段**；失败不清空旧向量、不动 label/aliases/merge_embedding。
  - **重试语义**：三失败态等用户显式 `retry`（后台不自动无限重跑、不烧 AI 额度）；已提交描述的任务重试**只续匹配**，不重嵌、不重存、不改 expected_version/冻结范围；`conflict`/`unavailable` 不可原样重试（须重新核对）；运行中/已完成的重试是幂等 no-op。
  - **`not_applicable` 与 `remaining`**：`not_applicable` = 冻结范围内处理时已失效/删除/合并而**持久跳过**的项数（**不是成功**）；`remaining = total − processed − failed − not_applicable`；四类之和恒等于 `total`（`total` 是创建时冻结的范围数量，不因后续失效而缩小）。只有 `remaining=0` 且 `failed=0` 才算 `completed`，部分失败恒为 `matching_failed`。
  - **恢复**：worker 绑**应用生命周期**（关页面/取消请求不取消已确认任务）；租约+心跳用 DB 实时时钟（`clock_timestamp()`），失租未结束阶段由扫描器重新领取；保存后故障窗恢复**禁再重嵌/再重存**。
- **旧快照与日报不受影响**：修复**不改写**历史体检快照、既有日报正文与历史评分，也不自动追加体检轮次；「更新完成」的措辞只表示匹配已更新，**效果待下一轮体检验证**（不得把复制成功、已排队、描述已保存或部分完成说成已修好）。
- **端点**：`GET /api/auxiliary-labels/:id/description-repair-context`（纯读）、`POST /api/auxiliary-labels/:id/description-repairs`（202 建任务）、`GET /api/auxiliary-labels/:id/description-repairs/:job_id`（纯读）、`POST /api/auxiliary-labels/:id/description-repairs/:job_id/retry`（202）；契约见 [api/semantic-boards.md](../api/semantic-boards.md)「辅助标签描述修复」。
- **代码入口**：后端 `backend-go/internal/tagmanagement/service/auxrepair/`（`auxrepair.go` 读/建任务门面、`embedding.go` 事务外向量生成、`matching.go` 既有匹配引擎调用、`executor.go` claim/心跳/恢复、`retry.go` 显式重试）、`handler/auxiliary_description_repair_handler.go`（四资源，只绑参数/错误/DTO）、`repository/auxiliary_description_repair*.go`（持久化与冻结范围）、`models/auxiliary_description_repair.go`（任务/条目模型）；生产接线 `tagmanagement/wire.go`（`InitDescriptionRepairChain`）+ `tagmanagement/routes.go`（挂四个资源）+ `internal/app/runtime.go`（应用生命周期 ctx + 关停先 cancel 再 join worker）。前端 `front/app/api/normalizers/{boardHealthTriage,auxiliaryRepair}.ts`、`front/app/features/tags/components/console/board-health/`（问题索引 + 单项处理区 + 三阶段进度）。

```text
体检轮次（纯读）
  → boardhealth.RunCheckup：统计/画像 diff → 三病信号 → LLM 单调用（score/comment/notify/triage）
  → 快照落库（metrics 含 triage）→ 通知裁决（冷却）

用户核对问题项（工作面）
  ├─ 🔴 摘除 → 用户确认 → 既有 DELETE composition → 成功才标「已摘除」（历史匹配不自动回填）
  └─ 🟡 重写 → GET description-repair-context（实时描述/版本/直接引用版块/活跃标签数）
             → 用户核对并确认共享影响 → POST description-repairs（幂等键 + expected_version）→ 202
             → 后台：embedding（事务外）→ saving（事务内锁行原子提交）→ matching（既有引擎逐项）
             → 三阶段/计数可查；失败显式 retry（只续匹配）；完成 =「更新完成，效果待下一轮体检验证」
```

### 标签级 watched tags（区别于话题级 topic-watch）

用户可在标签管理里关注（watch）任意标签，用于按标签筛选文章/日报。这与 `flow/daily-report.md` / `flow/topic-graph.md` 的**话题级** topic-watch（watch 持久话题 → 日报评估命中）是两套独立机制：一个 watch 的是**标签**，一个 watch 的是**持久话题**。

路由（`tagmanagement/handler/watched_tags_handler.go` + `service/watched/watched_tags_service.go`，挂在 `/api/topic-tags` 下）：

| 端点 | 语义 |
| ---- | ---- |
| `GET /api/topic-tags/watched` | 列出所有 `is_watched && status=active` 的标签，附 abstract 元信息（`is_abstract` + `child_slugs`，查 `topic_tag_relations`） |
| `POST /api/topic-tags/:tag_id/watch` | 标记 `is_watched=true`、`watched_at=now` |
| `POST /api/topic-tags/:tag_id/unwatch` | 取消关注（`is_watched=false`、`watched_at=NULL`） |

`GetWatchedTagIDsExpanded` 递归展开被关注标签（含抽象标签）的全部子标签 ID，供下游按 watched 标签集合做文章/日报筛选时使用。前端：`api/watchedTags.ts`。

### 标签合并预览（merge-preview，流式扫描/评估）

> 该工作流替代了已废弃的「标签自动合并 scheduler」（见 `flow/scheduler.md`，旧 scheduler 已不存在）。合并不再由定时任务自动执行，而是由用户驱动的 scan → evaluate → 分组 → dismiss/merge 流水线完成。

路由（`tagmanagement/handler/tag_merge_preview_handler.go` + `service/merge/` + `service/core/`，挂在 `/api/topic-tags` 下）：

| 端点 | 语义 |
| ---- | ---- |
| `POST /api/topic-tags/merge-preview/scan` | 启动异步全量扫描（`StartFullScan` 单例锁，已在跑返回 409） |
| `GET /api/topic-tags/merge-preview/scan/stream` | SSE 流式推送扫描进度 |
| `POST /api/topic-tags/merge-preview/evaluate` | 启动异步 LLM 评估（`StartEvaluation`，对候选对逐个裁决） |
| `GET /api/topic-tags/merge-preview/evaluate/stream` | SSE 流式推送评估进度 |
| `GET /api/topic-tags/merge-preview` | 读 pending 的 `TagMergeSuggestion`，过滤 `should_merge=false`，按目标标签分组返回 |
| `GET /api/topic-tags/merge-preview/status` | 返回 scan/eval 是否在跑 |
| `POST /api/topic-tags/merge-preview/add-to-group` | 手动把标签加入某合并组（`source=manual`，`OnConflict DoNothing`） |
| `POST /api/topic-tags/merge-preview/dismiss` | 标记候选对 `status=dismissed` |
| `POST /api/topic-tags/merge-with-name` | 真正执行硬合并：事务内 `FOR UPDATE` 锁两端 → 可选 rename + slug 冲突检测 → `HardMergeTags` → 提交后 `EnqueueMergeReembedding` 入重算队列 → 相关 suggestion 标 `merged` |

工作流：**scan**（全量扫描候选对，SSE 进度）→ **evaluate**（LLM 评估每对，SSE 进度）→ **merge-preview** 列表（按 target 分组）→ 用户对每组 **dismiss** / **add-to-group** / **merge-with-name**。合并后源标签的文章/关系迁移到目标标签，并触发 `merge-reembedding` 队列重算 embedding（见下「队列与回填运维」）。

前端（`features/tags/components/`）：`TagMergePreview.vue`、`TagMergeGroup.vue`、`composables/useTagMergePreview.ts`、`api/tagMergePreview.ts`。后端 service：`service/merge/tag_merge_suggest.go`、`service/core/merge_suggestions.go`、`service/core/hard_merge.go`、`service/core/merge_reembedding_queue.go`。

### 版块治理面板数据流

```text
SemanticBoardPanel
  → loadBoards() → GET /api/semantic-boards
  → viewBoard(id) → GET /api/semantic-boards/:id
  → viewComposition(id) → GET /api/semantic-boards/:id/composition
  → viewUpgradeCandidates() → GET /api/semantic-boards/upgrade-candidate
  → upgradeSuggest() → POST /api/semantic-boards/upgrade-suggest（旧，兼容期）
  → getUpgradeSuggestions() → GET /api/semantic-boards/upgrade-suggestions（持久化列表）
  → generateUpgradeSuggestions() → POST /api/semantic-boards/upgrade-suggestions/generate
  → dismissUpgradeSuggestion(id) → POST /api/semantic-boards/upgrade-suggestions/:id/dismiss
  → upgradeExecute(suggestion_id) → POST /api/semantic-boards/upgrade-execute（联动 confirmed）
  → backfill() → POST /api/semantic-boards/backfill
```

### Event 标签延迟 embedding

```text
Event 标签延迟 embedding: 描述+关键词生成后入队
  → 多行 embedding (semantic + event_keyword)
```

Event 类标签不随入库立即向量化，而是等描述与关键词生成后才入队，产出 semantic 与 event_keyword 两路 embedding。

### 相关事件（related-events）

```mermaid
flowchart LR
  CLI["cmd/related-events --refresh --board N"] -->|"确定性文本（名称/描述/有效 aux）<br/>版本 related-event-v1"| ROUTE["airouter embedding 路由<br/>operation=tagmanagement.related_event.embedding"]
  ROUTE -->|"校验：维度/有限值/非零范数/空间一致"| CACHE[("related_event_embeddings<br/>(tag, 版本, space_key) → text_hash + vector")]
  CACHE -->|"同版块 · 近 30 天 · 精确 cosine"| API["GET /semantic-boards/:id/tags/:tagId/related-events"]
  API --> UI["ArchiveMatchPopover（event 卡）<br/>按需展开 → 名称/描述/相似度/来源预览"]
```

- **失效三轴**：文本签名（`text_hash` 与当前文本不一致即不参与）→ 表示版本（`representation_version`）→ 模型空间（`space_key = provider|model|gen`，同维不同模型不混算）。换模型权重需显式 `--generation` 并重建。
- **查询口径**：同版块、当前有效（`status='active'` 且入板非 `direction_mismatch`）的 event 标签，`created_at ∈ [as_of-30d, as_of]`（标签收录时间，不是事件发生时间）；阈值前不截断候选，阈值后按分数降序 + id 升序取 10；不传递。
- **覆盖状态**：`ready`（含合法空集）/ `partial`（部分候选未索引）/ `not_ready`（源未索引或无可用路由），响应带 `candidate_count`/`indexed_count`/`threshold`，界面按状态给不同文案——**缺索引不冒充空结果**。
- **只读边界**：GET 不改标签、辅助标签、文章、thread、事件线身份或热度；不写这张派生表。运维刷新见 [configuration.md](../configuration.md) §「相关事件索引」。

## 业务约束与不变量

> 本节是 constraint-injection extension 的注入数据源：apply 改 `internal/tagmanagement/` 或 `internal/topicgraph/` 代码前会自动注入 system prompt，必须遵守。

1. **辅助标签按三级去重：L1 slug/alias 精确匹配复用、L2 embedding 相似命中只加 alias、L3 未命中才新建**：L1 slug/alias 精确匹配复用（`ref_count++`）；L2 embedding ≥ `auxiliary_label_dedupe_sim`（默认 0.95）命中只 `addAlias`（append alias + `ref_count++`）；L3 未命中才新建 `label_type=auxiliary` 的 `semantic_label`。
2. **辅助标签 L2 命中只 addAlias，不得改 Label 或重算 MergeEmbedding，以免形成合并黑洞**：aux 的 L2 命中**只 `addAlias`，不改 Label、不重算 MergeEmbedding**。`MergeEmbedding` 仅 L3 新建时生成一次，之后恒定——既有 aux 的「吸引力」= 固定 embedding 的 cosine，不随 alias 增多 / ref_count 升高自我放大。（对照主标签 `findOrCreateTag` 的 embedding 黑洞教训。）（fix-alias-save-vector-wipe：alias 写路径已升级为代码级列级更新——`addAlias` 与手动 alias 合并 target 侧均 `Select("aliases")` 只写 aliases 列，向量列永不沾染；历史全量 `Save` 曾把缓存结构体的 nil 向量写 NULL 致 1505 个 active 死向量，存量由 `POST /api/auxiliary-labels/backfill-embeddings` 回填，见「队列与回填运维」。）**向量维度契约（unify-vector-column-dimension）**：`semantic_labels.embedding` / `merge_embedding` 的**列类型不承载维度**（无维度限定的 `vector`；迁移 `20261117_0001` 已把存量 `vector(2560)` 统一）——维度由 embedding 模型的实际输出决定，是运行时属性；L2 的 cosine 比较只在**同维同模型**内进行，跨维比较由 pgvector 在比较查询时报错暴露。
3. **tag 挂版块按 composite_hit→direct_hit→hit_rate→max_sim→weighted 优先级判定，同 board 组合与单标签重叠同时满足只记 composite_hit，单 tag 最多挂 MaxBoards（默认 3）个版块**：composite_hit（tag 组合 ∩ board 组合 ≠ ∅）score=1.0 且免方向校验；单标签 direct_hit 需交集 ≥ `direct_hit_min_overlap` **且交集数 / tag active 辅助标签总数 ≥ `direct_hit_min_ratio`（默认 0.5，0=门槛关闭）**才命中 score=`direct_hit_score_factor`（默认 0.7）；**交集数达标但占比不足时不得 direct_hit，退回相似度路径判定，且重叠标签不得再作为该版块的相似度证据（同一批标签不得既作直接命中证据又作相似度证据——自相似恒 1.0 会变相重新挂载）**；再后 hit_rate → max_sim → weighted（`semantic_board_matching.go`），单 tag 最多挂载 `MaxBoards`（默认 3）个版块写入 `topic_tag_board_labels`。（add-composite-labels：direct_hit 从 1.0 降级 + 免检特权取消；fix-board-match-cross-domain：补占比门槛拦蹭词挂载——2026-10-04 非农事故形态的源头防线。）
4. **除 composite_hit 外的匹配命中（含 direct_hit）必须校验 tag 与 board 向量 cosine，mismatch 仍记录但不计日报且前端默认隐藏**：除 composite_hit（组合命中即指向一致，天然免检）外所有匹配规则——含降级后的 direct_hit——命中后先判**全库 gap 豁免**：挂载版块 identity cosine 与该 tag 对**全部 active 版块**（不限本次匹配候选，垃圾单挂的真家常在候选外）identity cosine 最大值的差值 < `semantic_board_match_direction_gap_exempt_margin`（默认 0.10，≤0 回退并列最优口径：差值 < 1e-9 判等）时豁免双门——长事件 tag × 短板名 embedding 的 cosine 全挤窄带（实测 0.55-0.65）时全库第一名常被语义重叠的磁铁板（如中国国内新闻）抢走，真家排 2-5 名的窄带差距是排序噪声不是跑题；豁免受可选地板线 `semantic_board_match_direction_floor`（默认 0=关闭）压制：floor > 0 且 cos < floor 时豁免不生效、回退双门。未获豁免的挂载走**绝对+相对双门**：①绝对门 cosine(tag identity embedding, board embedding) 低于 `direction_sim_threshold`（默认 0.5）；②相对门 gap = bestCos − cos ≥ `semantic_board_match_direction_relative_margin`（默认 0.15，ai_settings 可配，0/负值=关闭只留绝对门），bestCos=该标签**同轮匹配上的全部版块**（含将被标 mismatch 的候选）中 identity cosine 最大者——单挂标签 gap≡0 恒过，相对门天然只作用于多挂标签；任一门触发即标 `direction_mismatch=true`——**仍记录但不计入日报、前端默认隐藏**；tag identity 或 board embedding 缺失时跳过对应门。（`board-direction-check` 引入绝对门；add-composite-labels 将 direct_hit 纳入强制校验；fix-digest-tag-leak 补相对门拦多挂串味——digest 案例正确归属 0.81 vs 串味 0.60，绝对阈值切不开、相对 gap 0.21 稳定拦截；fix-board-ghost-false-positive 加全库最优豁免层与地板线——绝对门 0.6 切在事件×类目天然 cosine 带 0.50-0.65 上致全库 23.5% 挂载被标、其中 55% 为全库最优冤案，改源头全链路受益；fix-board-direction-gap-exempt 将豁免从并列最优扩为 gap 豁免带——实证真库 ghost 553→229、日本新闻 25→1，磁铁板噪声压制下的真家挂载不再冤杀，默认 0.10 为实证甜点：0.08 美国残留多、0.15 漏放小 gap 真串味、纯 gap 门（去绝对门）会误标强关联不可行。）
5. **max_sim ≥ 0.8 直接挂载必须同时满足 hits ≥ min(2, N) 且 hit_rate ≥ 0.3**：max_sim ≥ 0.8 直接挂载需同时满足 `hits ≥ min(2, N)` 且 `hit_rate ≥ 0.3`，防止单标签高相似度跨域误匹配。（`board-interaction-overhaul` 引入。）
6. **升级建议按 suggestion_hash 幂等，同 hash 已有 pending 行则 skipped 不重复入库**：`ComputeSuggestionHash(mode, decision, targetBoardID, auxIDs)` 为 32-hex 指纹（mode 维度值 = `direction:source` 四格键）；同 hash 已有 pending 行则 `skipped`（幂等 no-op），不重复入库。
7. **被 dismiss 的升级建议在冷却期（默认 14 天）内同 hash 不得重新生成**：被 dismiss 的建议在 `semantic_board_upgrade_suggestion_dismiss_cooldown_days`（默认 14 天）内，同 hash 下一轮生成被 `CountDismissedInCooldown` 拦截（`cooldown_blocked`），期满才可重生。
8. **watch 观察池已退役：单例簇不产任何建议，不得再生成 decision=watch 建议或恢复观察池 GC**（split-board-upgrade-directions）：单标签簇（size=1）既不进 LLM 也不入观察池（等待未来成簇后参与）；生成侧不得产生 watch/高置信自动合并等合成建议（升级建议生成路径单一化）；存量 pending watch 行已由迁移 20260905_0002 一次性清理，decision 枚举保留 watch 值仅为存量行 DTO 兼容。
9. **升级确认执行必须在同一事务内写 board_composition 并 MarkConfirmed，失败整体回滚不留半状态**：确认执行在同一事务内写 `board_composition` + `MarkConfirmed(suggestion_id)`，建议 → confirmed；`board_composition` 写失败则整体回滚，不留半状态。
10. **job_board_upgrade_suggest 定时仅跑创建方向两段、段失败仅记日志继续兄弟段，扩充方向不得自动执行**（split-board-upgrade-directions）：定时任务顺序跑 {create,aux} + {create,composite}，单段失败仅记日志继续下一段，返回 nil error（不标 task failed、不阻塞兄弟 job）；版块扩充纯手动（必须选版块触发），定时任务不得对任何版块自动生成扩充建议。
11. **任何将 semantic_labels 置 disabled 的路径必须同事务清空 embedding 与 merge_embedding，行本体与 aliases 保留**：任何将 `semantic_labels.status` 置为 `disabled` 的路径（API 删除 board、`DisableAuxiliaryLabel`、alias 合并源标记、GC disable 模式、更新接口）MUST 同事务同步置 `embedding=NULL, merge_embedding=NULL`（行本体与 aliases 保留）；重新启用由 backfill / llm_extract 重算。存量 disabled 向量已一次性清理。
12. **删除 topic_tags 行时其向量必须经 DB 层 FK ON DELETE CASCADE 自动级联删除**：`topic_tag_embeddings.topic_tag_id` 有 DB 层 `FK ON DELETE CASCADE`（迁移 `20260820_0001`）——删 `topic_tags` 行时向量自动级联删除。历史孤儿（GORM 声明 CASCADE 但 DB 无约束期间残留的 25.6 万行）已清理；`hard_merge` 等显式删 embedding 的代码路径保持不变（幂等）。
13. **跨版块关系发现只引用现有版块，不得自动创建/合并/修改版块，不影响 tag 版块归属**（add-evidence-backed-cross-board-relations）：跨版块关系发现是「证据→目标」的外部检索能力，目标解析只**引用**现有版块（唯一高分才 resolved，歧义/无目标保持 unresolved），**不强制映射、不自动创建/合并/修改版块、不做 board×board 全量扫描**；tag→版块的语义归属（embedding 匹配四规则）不受关系发现影响。确认关系只注入简报背景字段，不改版块成员。落地点：`dataenrichment/service/relation_resolver.go`（只读解析）/ 约束详见 `flow/data-enrichment.md` 24-27。
14. **组合标签去重 canonical 化：L1 组件 ID 无序集合完全一致复用、L2 组合 embedding ≥ composite_label_dedupe_sim 只 addAlias，命中不得改 label/重算 embedding，均未命中才新建**（add-composite-labels）：L1 与全体组合（含 disabled）比组件 canonical ID 集合；L2 仅比 active（disabled 向量已置 NULL），命中只 `addAlias` + `ref_count++`（防黑洞纪律同红线 2）；新建必须 2-5 个不同 active aux 组件，embedding 由 LLM 对「label + description」短语生成。
15. **组合标签 embedding 禁止组件向量合成/平均，必须由 LLM 对组合短语生成；生成失败创建整体回滚**（add-composite-labels）：组件向量加权/平均 ≈ 主题域泛化向量，恰好丢掉组合的指向性——这是组合标签参与匹配的物理基础；embedder 失败时不得落半成品行。
16. **compose 建议确认必须在同一事务内创建组合标签（含去重复用路径，扩充方向另含挂载 board_composition）+ MarkConfirmed，失败整体回滚建议保持 pending；compose 候选频次未达 semantic_board_upgrade_composite_min_cooccurrence（默认 10）不得进入 LLM，LLM 失败不产半成品**（add-composite-labels + split-board-upgrade-directions）：候选收集限同一文章内共现（窗口同 CoTagWindowDays），组件 ref_count 达升级阈值；确认遇 L1/L2 去重命中按成功处理（目的已达成，复用既有组合）；扩充方向（建议带 target）确认在同事务内建组合 + 挂载，目标版块非活跃则确认失败整体回滚。LLM 失败语义按入口分层：手动单入口（create×composite）诚实报错，定时任务段失败仅记日志继续兄弟段（红线 10）。
17. **源/版块命中统计口径唯一实现于 sourcestats 包且两端点只读，消费方不得复制公式；版块不存在/非 board → 404，disabled 版块不是 404（聚合为 0）**（add-source-board-hit-rate）：`GET /api/semantic-boards/:id/source-breakdown`（与源视角 `GET /api/feeds/board-hit-stats`）的聚合口径唯一实现 `internal/tagmanagement/service/sourcestats/`——按文章去重、含已归档（不过滤 `archived`）、限窗口白名单 {7,30,90}（非法 400 不回退）、命中 = 标签经 `topic_tag_board_labels` 挂到 `label_type='board' AND status='active'`、未打标两分（pending/leased vs 其余）；端点只读（不写库、不触发打标/匹配）；不变量 `Σ sources[].articles == total_articles`。要改口径先改 spec（`openspec/specs/source-board-hit-rate/spec.md`）。
18. **mono 打标输入为预算 4000 runes 的分段采样（文集型按标题均匀采样/叙事型头中尾采样，计量前先剥 markdown 图片与链接噪声），超限不再掐头；采样逻辑唯一实现于 sampling_splitter.go，aggregate 路径的 splitSections 不受影响**（long-form-sampled-tagging）：`buildArticleSummary`（`service/core/article_tagger.go`）按 AIContentSummary → FirecrawlContent → Content → Description 选出打标正文后调 `sampleTaggingSummary`（`sampling_splitter.go`）——正文超 `maxSummaryRunesForTagging`（4000）预算时按标题切段：≤3 段视为叙事型走头中尾采样，>3 段为文集型按段均匀分配预算（句界回退窗口、`……` 省略标记拼接）；噪声剥离先于计量（图片语法整体删、行内链接只留 text，不占采样预算），无标题整篇聚合为单段即叙事型；aggregate 路径的栏目切片 `splitSections`（带 dropIntro/mergeShort/splitLong 后处理）职责不同、互不依赖、不受影响。
19. **打标提取的空结果是合法结论、提取失败也是合法结论：LLM 空 keyword 数组不触发规则词回填、成功调用零候选不降级、**提取调用失败（err != nil）同样不降级规则词——直接判失败**（retire-heuristic-tag-fallback，2026-11-12：规则词兜底打标整体退役，`extractor_heuristic.go` 整文件删除、`extractWithHeuristic` 与 `legacyExtractTopics` 两处兜底分支删除）；失败时文章不写任何标签，打标任务标 failed 并进既有队列退避重试（`failureBackoff`：1→2→4→8→16 分钟，`max_attempts=5` 时封顶实测 16 分钟、30 分钟档不可达），仍失败停在 failed 态待人工/批量 retag——**失败 SHALL NOT 被伪装成成功**（旧兜底吞掉错误致任务误标 completed、文章永不重打，正是 Coding/AI Agent 泛词刷版块与 event 标签断粮的病根）；零候选文章保持无标签等待重打；泛词黑名单（新闻/论坛/要闻/快讯/文章/内容/技术/发展，Slugify 后完整匹配）在 persistArticleTags 入口与跨 feed reuse siblingLinks 查询两处拦截，任何来源的泛词标签不得入库/进辅助标签池**（fix-tagging-pollution）：`extractor_enhanced.go` ExtractTags 对空数组只记观察信息（不再调 heuristicKeywordCandidates 回填，该函数已删除——旧回填把分类名「新闻」与凤凰网 HTML `decoding="async"` 属性子串误命的「Coding」以 llm 来源写库）；零候选原样返回（Tags=[]、err=nil）；`filterGenericLabels` 为 mono/aggregate 两路径汇聚拦截点（heuristic 路径已随兜底退役删除），reuse 路径（不经 persist 入口）在 siblingLinks 查询 join topic_tags 排除黑名单 slug（全黑名单 sibling → reuse 返回 false 落回 AI 提取）。
20. **体检轮次纯读不落任何扩充建议（pending 行零新增）、不写匹配结果表，自动化与扩充手动边界不变**（add-board-health-checkup）：体检挂日报成功尾步自动执行（单版块失败仅记日志继续兄弟版块，error/panic 不污染日报 job），但只写 `board_health_snapshots` + 少量冷却内异常通知；体检产出的未收编候选只是快照附属产物，扩充建议仍必须用户手动触发（红线 10 不动），体检轮次 MUST NOT 造成 `board_upgrade_suggestions` 新增 pending 行或 `topic_tag_board_labels` 任何写入。
21. **member-derived 扩充主路回退语义：最新快照未收编候选非空时候选 ONLY 来自快照清单 top N（排除已挂载保险滤 + days>0 近期文章过滤），无快照/清单空/滤后空必须回退既有相似+共现双路且行为不变**（add-board-health-checkup）：主路命中时不得触发全库相似/共现扫描；组合路（expand×composite）不受影响；suggestion 管道（hash 幂等/dismiss 冷却/LLM 裁决/逐条勾选）零改动，仅候选来源变化。
22. **体检统计口径唯一实现于 boardhealth 包（对标 sourcestats 纪律），标签级=构成标签 embedding × 全部挂载 tags identity embedding 的贴合度分布（现场 cosine 重算，不按 aux 挂载数量筛样本），样本不足标 insufficient 不硬凑，端点只读且 disabled 版块不是 404（空态）**（add-board-health-checkup）：`internal/tagmanagement/service/boardhealth/` 是两级指标/画像 diff/低分线（板块内 cosine_mean P25 相对线）/分布分位唯一实现，`GET /api/semantic-boards/:id/health-checkup?trend=N`（N 白名单 1-90 默认 7，非法 400）只读消费同一实现；标签级归因 MUST NOT 按 tag 的 aux 挂载数量筛选样本（不做功劳归因），规则分数 MUST NOT 作相似度统计输入，任一侧向量缺失跳过分布计入 vector_missing（不按 0 计入），LLM 坏 JSON/超时降级 score=NULL 不重试不丢统计。
23. **event 标签 description 时间锚以首挂文章日期为准：生成/重生成 MUST 以 articleContext 中的 `[日期: …]`（文章发布日期）为参照、MUST NOT 引入 Context 外时间信息（含模型先验知识中的历史事件时间），打标与描述生成的 prompt 示例 MUST 用时间中性/虚构事件（禁真实事件+具体历史年月组合）**（fix-event-tag-time-anchor）：`generateTagDescription` event 分支（`buildEventTagDescriptionPrompt`）与两个 extractor prompt 的正例均不得携带历史时间锚；存量修复走 `cmd/admin fix-event-tag-anchor`（dry-run 默认出清单+备份 CSV，`--apply --ids` 显式圈选，非候选 ID 整批拒绝不触碰，重生成复用首挂文章 articleContext 同一构造口径 `buildArticleContext`）。
24. **体检三病分诊信号口径唯一实现于 boardhealth 包（磁铁失灵/描述污染/泛化漂移），样本或向量不足 MUST 标 insufficient 不硬凑**（board-health-triage-advice + fix-board-health-magnet-signal）：磁铁信号 = 构成标签 embedding 对「`direction_mismatch=true` 失败挂载子集」与「`direction_mismatch=false` 健康挂载子集」的贴合均值差（gap = **failed_mean − good_mean** ≥ `board_health_magnet_gap` 默认 0.08，方向对齐「磁铁把跑题的挂载吸过来」；失败侧样本 ≥ `board_health_magnet_min_failed` 默认 5 **且**健康侧样本 ≥ `board_health_min_sample_size` 默认 5 才成立，只做分组差统计，MUST NOT 做「哪个标签吸引哪个 tag」的功劳归因，MUST NOT 用含失败行的全体均值 all_mean（≡ cosine_mean）参与判定，磁铁信号须携带口径标记 `caliber` 供消费方区分旧快照）；最近邻信号 = 全库 active 辅助标签 top-3（排除自身）；描述污染预筛 = 固定文章式词根（此次/本次/该事件/报道/针对/期间/近日）命中，description 为空不标记；任一侧样本不足或构成标签自身向量缺失 MUST 标 `insufficient`，MUST NOT 按差值硬凑；「短名天花板」（短实体名 cosine 天然低）是防误杀判据，MUST NOT 仅因 cosine 低判摘除或扣分。
25. **体检分诊清单只是建议不得自动执行，组合标签（composite）在分诊中只读仅摘除可处理**（board-health-triage-advice）：分诊四类 `remove|rewrite|keep|insufficient` 均为建议——remove MUST 经用户确认后走既有构成摘除接口，rewrite MUST 经用户另行核对共享影响并明确确认后才可进入独立描述修复任务；体检轮次/查询端点 MUST NOT 改 description/embedding，MUST NOT 写匹配结果表/升级建议表（红线 20 不动）；构成清单逐条带 `label_type`（auxiliary/composite，历史快照缺该字段按类型未知处理，MUST NOT 臆造类型），描述修复链只覆盖 auxiliary——composite 的 rewrite 项只读展示（证据照常展开、不隐藏、不当它健康、无修复入口），remove 项不受类型限制。
26. **共享描述修复 MUST 人工确认后在同一事务内原子保存 description+storage 向量，任务持久幂等、匹配范围冻结全部活跃 topic_tags，计数四类之和恒等于 total，旧快照与日报不改写**（board-health-triage-advice）：辅助标签是全局共享标签，改其 description 会重算 storage 向量并更新全部活跃 topic_tags 的版块匹配（可能改其它版块归属），确认前 MUST 声明共享影响、取消时零写入；向量生成 MUST 在事务外、事务内锁行校验来源版本/类型/状态后原子写 description+embedding+任务阶段，MUST NOT 清空旧向量绕道，MUST NOT 改 label/aliases/merge_embedding；任务持久化（同幂等键同载荷回同任务、同键异载荷 409、同标签未结束任务互斥、失败态等用户显式 retry 且只续匹配不重嵌），创建时冻结全部 active topic_tags（含未归属者），`total=processed+failed+not_applicable+remaining` 恒成立（`not_applicable` 是失效跳过项 NOT 成功，`total` 不缩小），部分失败 MUST NOT 标完成；修复 MUST NOT 改写旧体检快照/日报/历史评分、不自动追加体检，完成只表示匹配已更新、效果待下一轮体检验证。
27. **分诊清单漏判只能补 ⚪ 证据不足，不得作废整轮裁决，也不得补成可执行建议**（fix-board-health-triage-degrade）：LLM 合法但漏判部分构成标签（覆盖率 ≥ 50%）时 MUST 保留评分/评语/已给出的分诊项，缺失项逐条补 `insufficient`（证据含「本轮未覆盖 / 返回 N/M 条」），**MUST NOT 补成 keep/remove/rewrite**；只有结构性非法（category 闭集外、未知或重复 aux_id、evidence 空白、rewrite 缺 suggestion）或覆盖率 < 50% 才整轮降级（score/comment/triage 全空）并记录原因。触发面 MUST 覆盖日报主轮/回填/手动生成三条产出路径：主轮尾步零守卫（每夜必有当日依据），回填与手动两条走 6h 新鲜度守卫（窗口内整板跳过不调 LLM）且 MUST 异步执行（不阻塞 HTTP 响应与日报 job）；守卫 MUST 只把**带评分**的快照算新鲜（降级快照不挡重生）。整轮预算 30 分钟、单板块子超时 6 分钟（慢板不占兄弟板预算）；每板块每轮 MUST 留一行可检索日志（labels/covered/filled/degraded，降级行也带真实 covered），成功路径也要有日志。
28. **矩阵/演进口径必须用当天线索合计：矩阵日格 = 该泳道该日各主题桶非综合稿线索数之和（同日多桶合并计数，MUST NOT 取桶峰值）；演进节点与顶部时间视窗曲线与矩阵日格同一量纲；section 数、桶数、thread 条数、事件数 MUST NOT 参与任何热度或排序计算**（retire-story-adopt-event-metrics D4/D5）：事件线量纲（story_count / 最热事件线身份 / 未归组事件数）整体退役；矩阵日格 SHALL 按当日线索合计做**行内归一的连续深浅**（本行可见窗最大 = 最深、0 = 底色、线性插值，跨行 SHALL NOT 归一——冷泳道不被热门泳道压白），颜色只是冗余强化通道：格值仍由数值 + title/aria 承载全部信息（无颜色仍完全可用，`clueCount` 为 null 不着色也不造 0），矩阵 SHALL 保留「数字 = 当日线索合计」口径说明，行头/格子的可读文本 SHALL 给出线索合计与主题桶数；线索数未知（旧响应缺 `clue_count` 且各桶线索全未知）SHALL 显示 `?` 并注明「线索数未知」，MUST NOT 造 0、MUST NOT 拿分区数/事件数冒充；有分区且线索合计 0 SHALL 显示 0 且可进当日工作面，与断档 `—` 区分；`stories_backfilled` / `story_identity_state` 三态随事件线退役，不再有「事件身份未回填」footnote 与按事件数保守计算的降级层。事件线身份与热度公式见 `flow/daily-report.md`（该链路已停写，见其红线 42-47）。（fix-lane-metric-provenance 2026-11-16 口径收敛：① 「非综合稿线索数」= 该桶 thread 数减去综合稿（thread 的 `tag_ids` 中 event 标签数 ≥ 2，`daily_report_threads.scope='composite'`）条数，桶级综合稿数由 lane-dynamics 响应 `sections[].composite_count` 下发，前端桶线索数 = `thread_count − composite_count`（唯一公式，缺任一 → 未知）；② 格子的 title/aria SHALL NOT 再露出与格值不同口径的线索数（含综合稿的 thread 总数），综合稿只以差额形式说明（「另有 N 条综合稿未计入」）；③ 矩阵区 SHALL 常驻一行口径说明（数字 = 当日线索合计 / 综合稿不计入）；④ 前端回退路径（旧响应缺 `clue_count`）SHALL 回退**非综合稿**线索数合计，各桶不可数时显示 `?`，MUST NOT 拿含综合稿的 thread 总数冒充；⑤ `lane_heat` 的线索计数判据 `COALESCE(t.scope,'') <> 'composite'` 有意把「判不出来」的 thread 按非综合稿计入——`scope` 列 NOT NULL + CHECK，该分支在当前 schema 下不可达，是防御性宽口径，MUST NOT 收窄成排除 NULL（历史日热度会凭空归零）。）
29. **行序与雷达量纲固定：泳道行序 = 泳道当天热度（lane_heat）降序 → 全窗线索合计降序 → topic id 升序；雷达关注度 = 版块各泳道当天热度之和（候选泳道按 `lane_heat_candidate_weight` 加权，保留一位小数），MUST NOT 用 section 数 / 事件数 / 事件线数加权冒充；分档阈值 SHALL 按新关注度真实分布标定（P80/P20，当前 14.5 / 2.3，实测样本与日期见 change `retire-story-adopt-event-metrics` design D9），MUST NOT 沿用 lane_heat 旧值 3.3/0.7 或 section 量纲 50/15**（retire-story-adopt-event-metrics D6/D8/D10）：泳道当天热度 = active 当天线索数 × 1.0 + candidate 当天线索数 × 权重（默认 0.5、域 [0,1] 有限数），只表达当天、不衰减不跨天，口径版本 `heat-v2-lane-clues-1d`；候选权重只进热度，SHALL NOT 参与话题生命周期（命中计数 / 转正门槛 / 归档）。「今天可能升温」「今日新起」「升温 N×」徽标与跨天延续脚注随事件线热度环比整体退役；右栏 SHALL 为「当天线索最多的泳道」单清单（上限 8，服务端归并/排序/截断；每条带版块名 + 泳道名 + 当天线索数 + 该版块自己的锚点日），条目 SHALL NOT 提供 `?story=` 深链，当天线索为 0 的泳道不进清单。
30. **跨版块混排必须带诚实降级标记：各版块的「今日」= 该版块自己的锚点日（最新日报日），条目 SHALL 显式携带其锚点日，滞后版块 SHALL 显示自己的日期、SHALL NOT 冒充全局锚点日；**右栏清单排序 SHALL 分两级：先按锚点日新鲜度（条目锚点日 = 全局锚点日者优先，滞后条目整体沉底但仍进清单），同新鲜度内再按当天线索数降序——单级降序会把 4 天前的高线索量与今日数据摆在同一列，读者误读成今日热点**；滞后条目 SHALL 灰显并标「数据至 MM/DD」（用条目自带锚点日，不伪造），雷达列表的滞后版块 SHALL NOT 挂「热点 / 关注」分档徽标（阈值按今日分布标定，旧数据不可比），改标中性「数据至 MM/DD」；热度数值与线索数 SHALL 照实显示，沉底/弱化 MUST NOT 折算成时间衰减（`clue_count` 原值参与次级排序）；端点超时/泳道计算失败的版块 SHALL NOT 参与排序，响应 SHALL 标 `partial` + `degraded_board_ids`，界面 SHALL 明示「本次泳道排序不完整」；跨版块混排存在「滞后版块少一天累积」的系统性偏差，SHALL 视为已知口径如实呈现、SHALL NOT 以人为补偿伪造同刻可比性**（retire-story-adopt-event-metrics D10 + 2026-11-11 看版后修订 3.6）：事件线清单段的未回填 / 新起 / 升温标记随事件线退役（`unbackfilled_board_ids` 等不再下发），`top_lanes` 只保留 `partial` / `degraded_board_ids` / `lagging_board_ids`；清单为空 SHALL 渲染空态，MUST NOT 说成「今天没有事件」。

31. **辅助标签池只作实体归一的材料与叫法证据：auto alias 与相似度产物不构成同体证据，aux 被 GC/禁用只掉 `entity_aux_links` 溯源行、绝不损毁实体身份**（normalize-event-subject-entities）：实体身份层（`entity_registry`/`entity_mentions`/`entity_aux_links`）是独立登记表，**不复用** `semantic_labels` 当身份载体（其行会被 GC 置 disabled 并清空向量，拿它当身份会让 190k 历史行的 GC 直接改写身份）。`entity_aux_links` 唯一写入方是判据「本次标签的辅助标签列表中，规范化文本与本次提及文本**规范化相等**的那一行」——L2 相似度自动写入的 alias（如「英伟达GPU」挂到公司行）**不得**作为同体证据、不得单独成为候选；未关联的 aux 行只作资料。FK 方向契约：`entity_aux_links.aux_label_id → semantic_labels.id ON DELETE CASCADE`（aux 硬删只掉溯源行），GC 置 disabled 的溯源行读取时失效；永远没有从 aux 指向 `entity_registry` 的级联。既有 aux 存量向量模型来源不可证，不混入实体检索语料（两套检索表示各自同 model/版本/维度空间内比较，跨空间不混用不 fallback）。

32. **体检分诊 MUST NOT 把版块核心主体列为摘除或重写对象：核心主体由版块 description/aliases 命中判定，仅标题命中须标签名 ≥ 3 字符**（fix-board-health-magnet-signal）：核心主体（`core_subject`）= 构成标签名（空白/大小写归一后）出现在版块 description 或 aliases 任一文本中；**仅**出现在版块标题中时还须该标签名 ≥ 3 字符（2 字短词既可能是简称如「美伊」也可能是泛化地域词如「中东」，MUST NOT 据标题推断全名或泛化词）；判定 MUST 为纯字面匹配，MUST NOT 用向量相似度或语义推断。命中核心主体的标签被 LLM 判 remove/rewrite 时，规则层 MUST 在持久化前改判 keep 并在证据标注「核心主体保护」+ 保留原判，MUST NOT 以可执行建议呈现、MUST NOT 携带重写建议文案。**简称 ≠ 全名**：版块标题只用简称时，核心主体保护 MUST 依赖版块 description 明示主体。

33. **体检分诊清单在持久化前 MUST 过三道规则层闸门，闸门 MUST NOT 依赖 LLM 自觉**（fix-board-health-magnet-signal）：① 核心主体拦截（红线 32）；② 摘除证据校验——被判 remove 的标签其磁铁信号 MUST 成立（不成立或 insufficient 时 MUST 降级为 ⚪ 证据不足并注明原因，MUST NOT 以建议摘除呈现）；③ 可执行占比闸门——remove+rewrite 合计占分诊清单总数超过 `board_health_max_actionable_ratio`（默认 0.8）时 MUST 把全部可执行项降级为 ⚪ 证据不足并记录闸门结论（可执行条数/总数/占比/阈值/是否降级/原因），占比 MUST 取 **LLM 原始裁决**（前两道闸门之前），MUST NOT 取降级后状态。降级 MUST NOT 丢弃分诊证据本身、MUST NOT 改写评分与评语；被降级项 MUST NOT 出现在需核对索引、MUST NOT 提供摘除或重写入口。

34. **相关事件只做同版块直接相关、不改任何身份**（enable-story-cross-day-continuation / related-events）：输入是 event 标签的确定性文本（名称 + 描述 + 当前有效辅助标签，版本 `related-event-v1`），MUST NOT 读 `subject_key`、实体 ID、`fingerprint_trusted` 或 thread 向量作为前提；候选 = 同版块有效非 mismatch 的 event 标签 ∩ 近 30 天（`created_at`），阈值 `related_event_similarity_threshold`（默认 0.86，`[0,1]` 有限数，缺项/坏值回退默认）过滤后按分数降序 + id 升序取 ≤10，MUST NOT 在阈值前按无关 top-K 截断、MUST NOT 传递扩展；GET MUST NOT 调模型或写库（含派生缓存），索引只由显式刷新生成且失败不覆盖旧有效行；`ready`/`partial`/`not_ready` 覆盖状态 MUST 如实上报，缺索引 MUST NOT 伪装成空结果；该能力 MUST NOT 修改标签、辅助别名、文章、thread 或事件线身份与热度。
35. **版块 label 或 description 变更后 SHALL 在同一请求内失效 board embeddings 匹配缓存，且 SHALL NOT 自动重跑匹配**（lane-manual-correction-loop board-match-cache）：`PUT /api/semantic-boards/:id` 改 label 或 description 会重算版块向量（`embed("label. description")`），而 `loadBoardEmbeddings` 缓存无 TTL（只靠显式失效），故更新成功后必须立即调 `InvalidateMatchCache()` 清缓存——SHALL NOT 让方向校验继续用旧向量，SHALL NOT 依赖进程重启或 TTL 过期；失效后 SHALL NOT 自动重跑 `topic_tag_board_labels`（匹配表保持旧结果，重跑/回填是用户的显式动作）。

## 代码入口

- **后端辅助标签**：`backend-go/internal/tagmanagement/service/auxlabel/`（`auxiliary_label_service.go` L1/L2/L3 去重、`addAlias`、alias 合并、composition 移除、禁用；`composite_label_service.go` 组合标签创建/两级去重/禁用启用/列表）。
- **后端组合建议产线**：`backend-go/internal/tagmanagement/service/board/semantic_board_compose.go`（co-tag 共现候选收集 + compose prompt + 幂等过滤；`GenerateSuggestions` 内独立 LLM 轮，冷启动期聚类输入不足也照跑）。
- **后端版块匹配 / 升级 / 回填**：`backend-go/internal/tagmanagement/service/board/`（`semantic_board_matching.go` 五级优先规则 + 方向校验、`semantic_board_upgrade.go` 四格生成分发（GenerateSuggestions(UpgradeGenerateRequest)）+ compose 确认事务联动（扩充方向含挂载）、`semantic_board_expand.go` 扩充召回（相似+共现双路）+ 版块画像、`semantic_board_expand_prompt.go` 扩充二分类 prompt、`board_upgrade_suggestion_persist.go` `ComputeSuggestionHash` 幂等（mode=direction:source）+ `CountDismissedInCooldown` 冷却、`semantic_board_backfill.go` all/unassigned/board 三模式回填）。
- **后端版块 handler**：`backend-go/internal/tagmanagement/handler/`（`board_crud_handler.go` 版块 CRUD/运维端点/suggest-auxiliaries/clusters/gc、`board_match_handler.go` 匹配/rematch-all/matching-config（composite_hits 详情）、`board_upgrade_handler.go` 升级建议资源（含 compose 决策）/backfill job、`composite_label_handler.go` 组合标签 CRUD、`tag_management_handler.go`）。
- **后端标签关注 / 合并预览 / 队列 handler**：同目录下 `watched_tags_handler.go`（标签级 watched tags）、`tag_merge_preview_handler.go`（scan/evaluate SSE + dismiss/merge-with-name）、`tag_queue_handler.go`、`embedding_queue_handler.go`、`merge_reembedding_queue_handler.go`（见下「队列与回填运维」）。
- **后端 watched/merge service**：`service/watched/watched_tags_service.go`、`service/merge/tag_merge_suggest.go`、`service/core/{merge_suggestions,hard_merge,merge_reembedding_queue,person_metadata_backfill}.go`。
- **后端源/版块命中统计（sourcestats）**：`backend-go/internal/tagmanagement/service/sourcestats/`（口径唯一实现）、`handler/board_source_breakdown_handler.go`（版块视角端点）；源视角端点在 reader 域 `internal/reader/handler/feed_board_stats_handler.go`。
- **后端版块体检（boardhealth）**：`backend-go/internal/tagmanagement/service/boardhealth/`（统计口径/画像 diff/LLM 评分降级/通知冷却/RunCheckup 编排唯一实现；`metrics.go` 两级指标、`signals.go` 三病分诊信号（磁铁分组差/最近邻 top-3/描述词根预筛）、`diff.go` 未收编候选聚合、`llm.go` 合并评分 + 标签级分诊 prompt + `normalizeTriage` 覆盖补齐（`minTriageCoverage` 0.5）、`notify.go` 打扰裁决、`run.go` 轮次编排（`RunCheckupWithOptions` 板块 scope + 6h 新鲜度守卫；`perBoardTimeout` 6min 子超时；每板/整轮日志）+ 只读视图）、`handler/board_health_handler.go`（只读端点）；日报尾步挂载 `internal/admin/scheduler/job_daily_report.go`（`runPostReportBoardHealthCheckup` 30min 预算 + `runPostBackfillBoardHealthCheckup` 回填触发，经 tagmanagement root 门面 re-export），手动生成触发在 `internal/topicgraph/handler/daily_report_handler.go`（`triggerBoardHealthCheckup` seam，异步）；扩充召回主路 `service/board/semantic_board_expand.go`（recallMemberDerivedCandidates）；快照模型/迁移 `tagmanagement/models/board_health.go` + `database/postgres_migrations.go` 20261006_0001（FK CASCADE）。
- **后端共享描述修复（auxrepair）**：`backend-go/internal/tagmanagement/service/auxrepair/`（`auxrepair.go` 读/建任务门面与 DTO、`embedding.go` 事务外 storage 向量生成、`matching.go` 调用既有匹配引擎逐项推进、`executor.go` claim/心跳/失租恢复、`retry.go` 显式重试）、`handler/auxiliary_description_repair_handler.go`（四资源，只绑参数/错误/DTO，不写 SQL）、`repository/auxiliary_description_repair*.go`（任务/冻结范围持久化与版本校验）、`models/auxiliary_description_repair.go`（任务/条目模型）；生产接线 `tagmanagement/wire.go`（`InitDescriptionRepairChain`/`StartDescriptionRepairExecutor`）+ `tagmanagement/routes.go`（挂四个资源）+ `internal/app/runtime.go`（应用生命周期 ctx + 关停先 cancel 再 join worker）；表/迁移见 [database/tables/semantic-labels.md](../database/tables/semantic-labels.md) §5.6-§5.7。
- **后端相关事件（related-events，enable-story-cross-day-continuation）**：`backend-go/internal/tagmanagement/service/related/`（`representation.go` 确定性文本/版本/阈值/generation/空间解析唯一实现、`cache.go` 有效 event 标签与辅助标签投影 + 派生缓存读写 + 来源文章、`refresh.go` 显式索引刷新（批 ≤32、30s、串行、逐项校验、失败不覆盖）、`query.go` 同版块只读查询（窗口/阈值/稳定排序/覆盖状态）、`experiment.go` 只读历史试验与分层抽样）、`handler/related_events_handler.go`（只读端点）、`models/related_event_embedding.go`（派生缓存表）、`cmd/related-events/main.go`（`--refresh` / `--experiment` 两模式，明确分离）；阈值/刷新操作见 [configuration.md](../configuration.md) §「相关事件索引」。
- **前端相关事件**：`front/app/features/tags/components/console/ArchiveMatchPopover.vue`（event 标签卡「相关事件」按需展开：ready/partial/not_ready/错误四态 + 迟到响应丢弃 + 来源走既有 `ArticlePreviewModal`）、`front/app/features/tags/components/MatchingConfigDialog.vue`（相关事件相似度字段，`size="lg"`）、`front/app/api/semanticBoards.ts`（`getRelatedEvents` + `RelatedEventsResponse`）。
- **后端版块调度**：`backend-go/internal/admin/scheduler/job_board_upgrade_suggest.go`（定时 06:30，仅创建方向两段：{create,aux} → {create,composite}）。
- **后端版块时间线**：`backend-go/internal/topicgraph/`（`service/daily_report_*.go` 版块时间线、`handler/`）。
- **后端雷达清单（topicgraph 域，retire-story-adopt-event-metrics 后现行口径）**：`backend-go/internal/topicgraph/repository/board_radar_repository.go`（overview 聚合：顶层 `top_lanes` 段「当天线索最多的泳道」跨版块归并——排序「锚点日新鲜度 → 当日线索数降序 → 版块名/泳道名/id」，服务端截断 8 条（第 9 名不展示），当天 0 线索的泳道不进清单，滞后条目沉底但线索数原值参与次级排序，超时/失败版块不出条目并置 `partial`+`degraded_board_ids`）、`repository/lane_heat_repository.go`（**泳道当天热度唯一活实现**，`LaneHeatRuleVersion='heat-v2-lane-clues-1d'`，`ComputeLaneHeat` 只读派生、不落库）、`repository/lane_snapshot_repository.go`（`GetBoardLaneDynamics` 泳道窗口/桶线索/候选）；handler `backend-go/internal/topicgraph/handler/board_radar_handler.go`。**历史停写**：`board_radar_stories.go`（per-story 逐日热度与基线）已随事件线量纲退役删除；`repository/story_heat.go` 的 `HeatRuleVersion='heat-v1-48h-hl24h'`（`story_signals` 证据衰减）不再被生产路径调用（`ComputeBoardHeat` / `ComputeBoardStoryHeatByDay` 只剩测试与 `cmd/calibrate-heat` 标定在用），仅存历史常量与只读算法，文件内 `heatParticipantKey` 内容身份键仍被当天聚簇（`report_event_clusters.go`）复用。
- **前端雷达右栏（surface-story-events-in-radar）**：`front/app/api/normalizers/boardRadar.ts`（事件线清单段归一：`stories` 段缺省 = 旧后端 → 回退版块级列表）、`front/app/features/tags/composables/useBoardRadarOverview.ts`（`topStories`/`warmingCount`/`crossDayBuilding` 派生）、`front/app/features/tags/components/console/BoardRadarReadingRail.vue`（单清单「今日该读」+ 行内徽标 `新起`/`升温 N×` + 跨天线脚注 + 未回填/滞后/排序不完整提示）、`front/app/api/normalizers/laneDynamicsWindow.ts`（`dayCellMetrics.displayCount/degradedFromClue`：矩阵格与时间视窗曲线共用降级口径）、`front/app/features/tags/components/console/{observationLanes,laneConsoleView}.ts`（格值/曲线消费降级口径）、`BoardRadarStory.vue`（小矩阵逐格降级）、`BoardRadarStatusBlock.vue`（管线状态折叠状态机）。
- **前端**：`front/app/features/tags/components/UpgradeSuggestionPanel.vue`（升级建议面板：四格生成入口 + 版块单选 + 持久化建议列表（含 compose 卡片与「组合」过滤 tab）；旧内存探索区已退役）、`CompositeLabelPool.vue` + `CompositeLabelEditDialog.vue`（组合标签治理页，未选版块时「组合标签」tab）、`MatchDetailPanel.vue`（匹配详情，composite_hit 组合链展示）、`BoardSourcePanel.vue`（板块来源构成面板，挂在治理工作面「来源」页签，只读无写动作）、治理工作面 `console/BoardWideWorkface.vue`（页签 构成/组合/候选/来源/设置；构成明细 `console/BoardHealthDetail.vue` 承接原 BoardCompositionPanel 全部构成管理职能：picker 双模式添加/行内移除/组合创建挂载；体检依据 `console/board-health/BoardHealthWorkspace.vue` 承接评分/质量表/趋势/未收编候选收编）、`console/BoardSettingsForm.vue`（版块设置：名称/描述/排序/受保护）、`console/RailHealthBrief.vue`（右轨体检摘要）、`front/app/pages/tags/index.vue`（L0 今日雷达页）+ `front/app/pages/tags/[boardId]/index.vue`（L1 版块态势页）。

## 队列与回填运维

> settings 页 Queues section（`features/settings/components/SettingsSectionQueues.vue`）直接暴露下列三个队列给用户查看状态与 retry。

| 队列 / 回填 | 路由组 | 用途 |
| ----------- | ------ | ---- |
| tag-queue | `/api/tag-queue/{status,tasks,retry,retag-today}` | 文章打标签任务队列（`TagJob`）。`retag-today` 把今日文章批量重新入队（`force_retag=true`）；对应 `flow/reading.md` 的打标签时机。前端 `features/settings/components/TagQueuePanel.vue` |
| embedding/queue | `/api/embedding/queue/{status,tasks,retry}` | 标签/版块 embedding 生成队列（`EmbeddingQueueService`）。前端 `features/ai/components/EmbeddingQueuePanel.vue` |
| embedding/merge-reembedding | `/api/embedding/merge-reembedding/{status,tasks,retry}` | 标签合并后重算 embedding 的独立队列；`merge-with-name` 提交后由 `EnqueueMergeReembedding` 入队（`MergeReembeddingQueueService`） |
| person-metadata 回填 | `POST /api/embedding/queue/person-metadata/backfill` | 人物标签元数据回填（`service/core/person_metadata_backfill.go` 的 `BackfillPersonMetadata`） |
| 辅助标签死向量回填 | `POST /api/auxiliary-labels/backfill-embeddings` | 重建 active aux 标签缺失（NULL）的 embedding/merge_embedding（L3 同款生成规则，只补缺列、幂等重触发只处理残余、行级失败跳过）；同步查 pending 数、detached goroutine 异步跑、立即 202 `{pending:N}`，重触发至 pending=0 即完成。存量死向量源自己录：addAlias 曾用缓存结构体（不含向量列）全量 Save 把向量写 NULL（fix-alias-save-vector-wipe 修复 + 回填），完成后建议可选跑 `rematch-all` 让版块挂载重新竞争 |

handler 出处：`tagmanagement/handler/{tag_queue,embedding_queue,merge_reembedding_queue}_handler.go`。

## 变更溯源

| 日期 | 变更 | 摘要 | 归档位置 |
| ------ | ------ | ------ | ---------- |
| 2026-11-17 | unify-vector-column-dimension | `semantic_labels.embedding` / `merge_embedding` 去维度限定（迁移 20261117_0001）；辅助标签 L2 相似命中的向量维度不再由列类型约束，改由写入口按「随值留存维度」校验（不一致拒绝写入）；L2 命中只 addAlias、不改 Label / 不重算 MergeEmbedding 的红线未触及 | [archive/2026-11-17-unify-vector-column-dimension](../../../openspec/changes/archive/2026-11-17-unify-vector-column-dimension) |
| 2026-11-18 | simplify-workshop-matrix-copy | 矩阵首屏合并为一条格值图例，日期格式只在空输入或校验时提示；跨档窗才提示历史补充，治理规则移至详情与操作处。统计口径、交互与数据不变。 | [change](../../../openspec/changes/simplify-workshop-matrix-copy) |
| 2026-11-17 | unify-lane-workbench | 泳道修正工作面三栏化（左 380 已选 / 中画布 / 右 300 待选，推荐不再在左栏）+ 节点卡显式写操作（加入/移出/加回走同一 `useLaneCorrection`） + 时间窗归模态本地态（不写 router，preview 不取数） + 两条 365 曲线分口径（编辑=本泳道逐日线索数；新增=全部可选线索数，轻量 `omit_embeddings=1`）+ 两画布共享 `pointAppearance`/`relatedNodeIds`（接近度统一 1−距离）；后端只补 `section.clue_count` 运行时投影与 `early_members[].embedding`，无 DDL | [`openspec/changes/archive/2026-11-17-unify-lane-workbench`](../../../openspec/changes/archive/2026-11-17-unify-lane-workbench) |
| 2026-11-16 | fix-board-console-spec-residue | 主 spec `workshop-board-console` 的「矩阵视图」requirement 追上现行口径：格内数字由「当日事件线数」改为「当日非综合稿线索合计」、图例由「数字=事件线数 / 颜色=来源数分档」改为「数字=线索数 / 颜色=行内相对强度」，零线索可开的判定同步去事件线化。**纯 spec 文本债**（与同文件「热度口径」条对齐），代码/接口/数据/UI 零改动 | [`openspec/changes/archive/2026-11-16-fix-board-console-spec-residue`](../../../openspec/changes/archive/2026-11-16-fix-board-console-spec-residue) |
| 2026-11-14 | lane-manual-correction-loop | 泳道人工修正闭环（批 E1 文档收口）：① 修正过期组件指代——`BoardHealthPanel.vue` 已随 rework-workshop-into-radar-and-desk 8.4 物理删除，职能拆入治理工作面 `console/BoardWideWorkface.vue`（页签 构成/组合/候选/来源/设置）+ `console/board-health/BoardHealthWorkspace.vue` + `console/BoardHealthDetail.vue` + `console/RailHealthBrief.vue`；② 版块编辑入口现状 = L0 雷达行尾「⋯」（编辑/归档）+ L1 顶栏「板块管理 → 板块设置 / 删除」→ 治理工作面「设置」页签 `console/BoardSettingsForm.vue`，旧 `BoardEditDialog`/`BoardGovernanceDrawer`/`BoardGovernanceRail` 三套僵尸实现已删；③ 新增红线 35：版块 label/description 变更后同请求内失效 board embeddings 缓存（无 TTL），且不自动重跑匹配表 | [`openspec/changes/archive/2026-11-16-lane-manual-correction-loop`](../../../openspec/changes/archive/2026-11-16-lane-manual-correction-loop) |
| 2026-11-12 | retire-heuristic-tag-fallback | **删除 heuristic 规则词兜底打标**（用户报障「board 1980 满屏 Coding/AI Agent」的真病根）：删 `extractor_heuristic.go` 整文件（7 条硬编码规则 AI Agent/OpenAI/Anthropic/Multimodal/Coding/Infra/NVIDIA + 产品型号 token + feed 名 + 分类名，`Coding` 含子串 "code"）+ 提取层 `extractWithHeuristic` + 编排层 `legacyExtractTopics`；**失败语义改为「失败即失败」**——抽取失败不再写任何标签，任务标 failed 进既有队列退避重试（1/2/4/8/16 分钟，上限 5 次），仍失败停在 failed 态可批量 retag（旧兜底吞掉错误致任务误标 completed、文章永不重打，这才是泛词刷版块 + event 标签断粮的根因）。存量清理：删 1727 行 heuristic 文章关联 + 14 个零引用标签（FK CASCADE）+ 14 个 LLM 认领者改 source=llm（**按字面删 28 个会误删 OpenAI/NVIDIA 等真标签**）+ 悬挂引用 33 行 + 重算 1142 篇 tag_count；效果：heuristic 归零、**Coding 关联文章 722→1、AI Agent 85→16**（余为 LLM 真实产出），board 1980 挂载 Top20 全是真标签。**挂载行未解**——要彻底移除 Coding 需另开标签退役 change | [`openspec/changes/archive/2026-11-12-retire-heuristic-tag-fallback`](../../../openspec/changes/archive/2026-11-12-retire-heuristic-tag-fallback) |
| 2026-11-12 | tolerate-schema-envelope-extraction-output | 修复模型「schema 外壳」输出导致的打标解析失败：本地 4B 模型在 JSON-schema 引导下把 schema 骨架当输出（数组塞进 `properties`，11-12 占 82% 调用），解析器只认顶层键 → 解析失败 → 任务 failed。双管齐下：① **根因**——merged 路径 user prompt 末句仍写 merge-tag-branches 之前的旧单数组 `{"tags": [标签列表]}`，与 system prompt 正例/下发 schema 的双数组矛盾，改为指向双数组（section 路径 schema 本就是单数组，不动）；② **兜底**——新增 `unwrapSchemaEnvelope` 识别外壳取 `properties`，判定三条件（顶层对象 ∧ properties 是对象 ∧ 含 type/required）、幂等、非外壳原样返回；解包后缺目标键仍报错触发重试。接入 `unmarshalMergedExtraction` 与 `parseRawTagObjects`（section/legacy 同享）；标签语义零变化（aux/description/上限/黑名单/示例词护栏全照旧） | [`openspec/changes/archive/2026-11-12-tolerate-schema-envelope-extraction-output`](../../../openspec/changes/archive/2026-11-12-tolerate-schema-envelope-extraction-output) |
| 2026-11-12 | retire-downgraded-mount | 退役 `topic_tag_board_labels.downgraded` 降级标记（挂载行为零变化）：`min(2,N)` 放宽准入保留，单 aux 的 keyword 实体标签（Claude/英伟达/SGLang 等）照常挂版；删除 DDL 列（3627 行挂载零删除）、匹配层 `MatchResult.Downgraded` 与置位、归档弱证据口径、`MatchTier`（max_sim 统一 tier 2）、topicgraph 三处 DTO/SQL/`quality_breakdown` 键、前端「已降级 ↓」徽标与淡化色/「↓」后缀、match-detail 响应 `downgraded` 字段；真库实测关键标签挂载与分数完全未变（Claude 1.000/英伟达 1.000/Transformer 0.854/SGLang 0.808）；派生排序口径随之位移（版块文章质量序与分页、日报 top-30 截断集合、section best_tier 排序、归档行内标签次序）；连带修出 `cmd/dump-sanitizer` 的 stale 表条目（`analysis_methods`/`reference_roles` 已被迁移 DROP）与 demo seed 重新生成 | [`openspec/changes/archive/2026-11-12-retire-downgraded-mount`](../../../openspec/changes/archive/2026-11-12-retire-downgraded-mount) |
| 2026-11-11 | normalize-event-subject-entities | 实体身份层影子落地：辅助标签只作资料与叫法证据（相似度/auto alias 不构成同体证据），GC 置 disabled/删除只移除溯源行、不损毁实体身份；新增 `entity-subject-resolution` 能力（默认 off，影子态限定三处写入） | [`openspec/changes/archive/2026-11-11-normalize-event-subject-entities`](../../../openspec/changes/archive/2026-11-11-normalize-event-subject-entities) |
| 2026-11-11 | enable-story-cross-day-continuation | 新增 `related-events` 能力：event 标签按「名称+描述+当前 aux」的确定性文本（`related-event-v1`）做同版块直接相关推荐，阈值 `related_event_similarity_threshold` 默认 0.86（起点 0.82，S3 实测后由用户拍板上调）可在线调；独立可丢弃派生缓存 `related_event_embeddings`（`text_hash`/版本/`space_key` 三轴失效）+ 显式刷新 CLI（`--refresh` / 只读 `--experiment`）；GET 只读、覆盖状态 ready/partial/not_ready 如实上报；归档 event 标签卡按需展开、来源走既有阅读预览。AI 版块 1980 实测：782 标签/305,371 对，≥0.82 共 5,919 对，分层样本 50 对明确相关 34（68%，0.86+ 区间 12/12），0.82 偏宽松 → 用户拍板把默认值上调为 0.86 | [`openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation`](../../../openspec/changes/archive/2026-11-11-enable-story-cross-day-continuation) |
| 2026-11-10 | fix-board-health-magnet-signal | 体检磁铁信号口径反转 + 核心主体保护 + 分诊合理性闸门：磁铁信号由「failed 比全体低即嫌疑」（方向与语义相反，致核心主体被系统性点名——伊朗等 41/47 成立）改为「失败侧相对健康侧显著更高」；新增 `core_subject` 核心主体标记（description/aliases 命中，仅标题命中需标签名 ≥3 字符）与规则层三道闸门（核心主体拦截 / remove 必须磁铁成立 / 可执行占比 >0.8 全量降级）；提示词补四条硬约束；新增 `caliber` 口径标记区分旧快照；板块 1974 补定义式 description。效果：待核对 47 → 0、板分 58 → 88 | [`openspec/changes/archive/2026-11-10-fix-board-health-magnet-signal`](../../../openspec/changes/archive/2026-11-10-fix-board-health-magnet-signal) |
| 2026-11-06 | enable-unbound-jit-injection | 约束标签辖区补全：补后端漏网目录与前端业务区信号，修 5 条孤儿标签；未激活档编辑路径 JIT 注入见 change（本 flow 无业务行为变更） | [`openspec/changes/archive/2026-11-06-enable-unbound-jit-injection`](../../../openspec/changes/archive/2026-11-06-enable-unbound-jit-injection) |
| 2026-11-05 | speed-up-tag-embedding | 标签向量链路提速：`FindSimilarTags` 改两段式（halfvec HNSW 索引召回 + float32 精确重排，1366ms→9.9ms；cast 必须带 `::halfvec(2560)` typmod 否则不走索引，`hnsw.ef_search≥120`）；embedding 队列 worker 去 2s 轮询 + 4 并发 SKIP LOCKED 批领 + 同类型批量 ≤32 调 provider（缓存 key 按单条拆分，单条路径兼容）；失败自动重试（<3 次指数退避）+ 修 Stop/Start stopCh 竞态 + worker 真实 logger + status 增 `oldest_pending_age_seconds`/`throughput_per_min`；索引 SQL `scripts/db/tag-embedding-halfvec-hnsw-20261104.sql`（低负载窗口手动执行，docker-compose `shm_size: 1g` 为前置）。语义红线零改动（阈值/cosine 校验/三级去重不变） | [`openspec/changes/archive/2026-11-05-speed-up-tag-embedding`](../../../openspec/changes/archive/2026-11-05-speed-up-tag-embedding) |
| 2026-11-03 | fix-embedding-model-column-width | embedding 模型名列宽根治：`topic_tag_embeddings.model`（及 discovery 四张向量表）由 `VARCHAR(50)` 加宽到 `VARCHAR(100)`，与模型注册表 `ai_models.name` 对齐——本地 llama.cpp / GGUF 模型把模型文件绝对路径当名字注册（61 字符）超限 → Postgres `SQLSTATE 22001` → 标签向量落库被拒、embedding 队列批量失败（689 条 failed，且命中 embedding 缓存时返回的 `cached.Model` 同值故主路由是短名模型也挡不住）；建立不变式「存 embedding 模型名的列宽 MUST NOT 窄于 `ai_models.name`」（数据驱动测试守住，新增窄列即红）；连带修复本 change 自身引入的约束回归（gorm tag 变更触发 GORM AutoMigrate 对存量列发 ALTER → postgres driver 顺带 `DROP NOT NULL`，丢失 `20260723_0001` 固化的 `topic_tag_embeddings.model` NOT NULL，迁移 `20261103_0002` 幂等恢复） | [`openspec/changes/archive/2026-11-03-fix-embedding-model-column-width`](../../../openspec/changes/archive/2026-11-03-fix-embedding-model-column-width) |
| 2026-11-02 | fix-observation-pool-curve | 顶部时间视窗折线缩略图首屏只画当前档窗（14 天）、拖一下才出历史——根因：365 池加载被挂在「观察范围跨出档窗」触发线上，首屏池从不加载，档窗外全为未知日（`null` 不补 0）⇒ 曲线残缺。修复：池改为 A 数据到位且锚点已知即**预取**（顶部背景曲线首屏即需整条 365 池曲线；「按需拉池」只管历史日内容解析），365 档仍与池共用一次请求；池未取到/失败的提示条与「重试历史池」不再被「是否跨出档窗」门控；后台零改动（复用既有 `days=365` 读端点） | [`openspec/changes/archive/2026-11-02-fix-observation-pool-curve`](../../../openspec/changes/archive/2026-11-02-fix-observation-pool-curve) |
| 2026-10-24 | guard-tag-exemplar-leakage | 打标示例词泄漏护栏：两条 LLM 提取出口（mono 融合提取 / aggregate 栏目片）解析后过滤「prompt few-shot 示例词照抄」候选——label Slugify 归一命中示例词集合且模型可见输入无依据（大小写不敏感 Contains）即丢，auxiliary_labels 逐项同规则过滤；示例词集合与 prompt 同源常量 + 防漂移测试锁定；存量清理 CLI `cmd/admin tagguard-cleanup`（dry-run 默认 + --apply 事务删除 + tag_count 重算 + 孤儿清理，备份 CSV 留底）删 28 条误挂（PostgreSQL 26 + 某国央行上调基准利率 2 + kubernetes 1，PostgreSQL 标签挂载 36→11），416 期周刊不再误挂 | [`openspec/changes/archive/2026-10-28-guard-tag-exemplar-leakage`](../../../openspec/changes/archive/2026-10-28-guard-tag-exemplar-leakage) |
| 2026-10-24 | fix-board-health-llm-misread | 体检 LLM 评分判据三处校准（防系统性误读）：①贴合度分化双条件（相对垫底 low_sim_ratio 大面积偏高 且 多数达标标签 cosine_mean < 0.6，identity 事件句×短名词标签天然带 0.51-0.76）；②短实体标签（人名/地名/技术名词）cosine 低于泛概念属正常形态不扣分；③reason_dist 病征限双条件（弱路径畸高且 ghost 同时偏高，无组合挂载全单标签路径是常态）；实测假低分板全部回归（俄乌 55→92、日本 65→85、卫生 55→72 等） | [`openspec/changes/archive/2026-10-24-fix-board-health-llm-misread`](../../../openspec/changes/archive/2026-10-24-fix-board-health-llm-misread) |
| 2026-10-22 | fix-board-direction-gap-exempt | 方向校验豁免扩围：全库最优豁免（仅并列第一）升级为全库 gap 豁免带（挂载与全库最优差值 < `semantic_board_match_direction_gap_exempt_margin` 默认 0.10，≤0 回退旧口径）——窄带分布下磁铁板抢走全库第一名致真家排 2-5 名被冤杀（日本新闻 25 冤 22）；重算后全库 ghost 553→229、日本新闻 25→1、美国 216→48；红线 4 同步升级 | [`openspec/changes/archive/2026-10-24-fix-board-direction-gap-exempt`](../../../openspec/changes/archive/2026-10-24-fix-board-direction-gap-exempt) |
| 2026-10-15 | fix-board-ghost-false-positive | 方向校验加全库最优豁免层与地板线：绝对门 0.6 切在事件×类目天然 cosine 带 0.50-0.65 上致全库 23.5% 挂载被标、其中 55% 为全库最优冤案（豁免零冤案实证：重算后全库最优挂载 2313 行被标 0）；判定次序 composite_hit 免检 → 全库最优豁免（含并列）→ 绝对门 → 相对门；可选地板线 `semantic_board_match_direction_floor`（默认 0）压制豁免；改源头全链路受益（日报过滤/体检 ghost/前端展示零改动） | [`openspec/changes/archive/2026-10-24-fix-board-ghost-false-positive`](../../../openspec/changes/archive/2026-10-24-fix-board-ghost-false-positive) |
| 2026-10-13 | fix-digest-tag-leak | digest 串味根治双门：方向校验加相对门（gap = bestCos − cos ≥ `semantic_board_match_direction_relative_margin` 默认 0.15，bestCos=同轮全部匹配版块 identity cosine 最大者，composite_hit 豁免、direct_hit 同校验，红线 4 升级双门表述）+ 日报去重 tiebreak 升三级（文章数 > identity×版块 cosine > id，embedding 缺失回退 ID，红线 29）；margin=0.15 真库定值依据：gap≥0.15 抽样 18/18 全串味、0.10~0.15 混合法双归属不能低于 | [`openspec/changes/archive/2026-10-13-fix-digest-tag-leak`](../../../openspec/changes/archive/2026-10-13-fix-digest-tag-leak) |
| 2026-09-27 | fix-alias-save-vector-wipe | addAlias 清向量 bug 修复：L2 归并与手动 alias 合并改列级更新（Select("aliases")+struct，向量列永不沾染，红线 2 升级为代码+测试不变量）；新增死向量回填端点 `POST /api/auxiliary-labels/backfill-embeddings`（L3 同款生成、只补缺列、幂等、行级容错，202 异步）；真库存量 1505 个 active aux 死向量全部恢复（日本新闻版块构成 30 个 aux 复活）；附带修复 testutil 漏注册 discovery 模型的解耦回归（4421c068） | [`openspec/changes/archive/2026-09-27-fix-alias-save-vector-wipe`](../../../openspec/changes/archive/2026-09-27-fix-alias-save-vector-wipe) |
| 2026-09-25 | fix-tagging-pollution | 打标污染修复三件套：LLM 空 keyword 不再触发规则回填（判空=合法结论）、heuristic 兜底仅限提取调用失败、泛词标签黑名单拦截（新闻/论坛/要闻/快讯/文章/内容/技术/发展，persist 前丢弃 + reuse 复制排除）；存量清理（heuristic 484 行 + llm 泛词/Coding ≈800 行）+ retag 重打 586 篇全查零污染、近 3h 新增 llm=1679 行零命中 | [`openspec/changes/archive/2026-09-25-fix-tagging-pollution`](../../../openspec/changes/archive/2026-09-25-fix-tagging-pollution) |
| 2026-09-19 | long-form-sampled-tagging | mono 打标超长输入从「掫头 4000 runes」改为预算 4000 runes 分段采样：先剥 markdown 噪声（图片整删、链接保文字），文集型（标题段 >3）标题全保 + 正文均分段内句界截断（保底 120/段，段数超限退化标题清单），叙事型头中尾采样（2:1:1）；短文（≤4000）行为不变，新采样切分器 sampling_splitter.go 不动 aggregate 路径 | [`openspec/changes/archive/2026-09-19-long-form-sampled-tagging`](../../../openspec/changes/archive/2026-09-19-long-form-sampled-tagging) |
| 2026-09-19 | merge-tag-extraction-branches | mono 打标双分支提取（event/person + keyword 两次 LLM 调用）合并为单次调用双数组输出：两份系统提示合并去重、schema 双数组、数组级部分产出（空数组不触发 heuristic、仅保留另一侧），整体失败仍回退 heuristic；调用数 2→1、prefill 减半，去重优先级/上限/三级入库契约不变，aggregate 回落路径零改动 | [`openspec/changes/archive/2026-09-19-merge-tag-extraction-branches`](../../../openspec/changes/archive/2026-09-19-merge-tag-extraction-branches) |
| 2026-09-17 | fix-spa-nav-loading-ux | 叙事工坊（TagsPage）四个非默认 tab 面板改 defineAsyncComponent 懒加载 + 新增 PanelAsyncPlaceholder 占位（纯实现层性能优化，tab 入口与业务行为不变，主 chunk 瘦身）；同 change 另含全局导航加载反馈（>250ms 触发）与 pre-FCP 双主题背景色，契约见 spa-loading-ux spec 与 loading-experience.md | [`openspec/changes/archive/2026-09-17-fix-spa-nav-loading-ux`](../../../openspec/changes/archive/2026-09-17-fix-spa-nav-loading-ux) |
| 2026-09-17 | expand-upgrade-days-window | 升级建议「候选时间窗」days 从创建×单标签放开到全部四格：前端面板 days 下拉恒显、扩充方向请求携带 days；后端扩充召回时间窗——相似路按「近 N 天有文章引用标签」批量 EXISTS 过滤（days=0 跳过），共现路（扩充×单标签 + 扩充×组合 compose）cutoff 取 days 与全局 CoTagWindowDays 更严者（`effectiveCoTagWindowDays`，days 只收紧不放宽）；days=0 与现状逐字节一致；定时任务仅创建方向不受影响 | [`openspec/changes/archive/2026-09-17-expand-upgrade-days-window`](../../../openspec/changes/archive/2026-09-17-expand-upgrade-days-window) |
| 2026-09-05 | split-board-upgrade-directions | 升级建议生成重构为四格矩阵（方向 create/expand × 来源 aux/composite）：LLM 单一决策空间（每轮只做一种判断）、扩充锁定单版块（target 服务端注入）+ 双路召回（相似 + 共现，上限 40）+ 版块画像 prompt 二分类；watch 观察池与高置信自动合并退役（存量迁移 20260905_0002 清理）；定时任务收窄为创建方向两段，扩充纯手动；compose 确认支持「创建组合 + 同事务挂载版块」；旧内存探索 UI 与 upgrade-candidates 端点退役。归档前两批补修：①浮层样式误删事故（.usp-overlay 恢复 + 浮层展示锚测试 + 验收四维度规范）；②旧 discover_new 存量 150 条 pending 置 dismissed 留痕（迁移 20260907_0001）+ 已存在组合/已挂载组合防重复过滤（filterExistingComposeCandidates）；③聚簇阈值 0.35→0.25（贪心 average-link 传递混簇调研修调，见 scripts/research/candidate_freshness_probe.py） | [`openspec/changes/archive/2026-09-07-split-board-upgrade-directions`](../../../openspec/changes/archive/2026-09-07-split-board-upgrade-directions) |
| 2026-09-04 | add-composite-labels | 组合标签（composite label，指向性中间粒度）：semantic_labels 第三种 label_type + composite_components 组件表；匹配规则改五级优先（composite_hit 1.0 最强免方向校验 / direct_hit 降级 0.7 强制方向校验）；升级建议 compose 决策（co-tag 共现候选 → LLM 裁决，真实库通过率 75%）；组件齐全推导组合命中（确认→重算闭环）；治理 API + 版块上下文创建（本版块置顶/共现联动重排/创建即挂载）；真实库重算 composite_hit 44 行/direct_hit 342 行全降 0.7，过程修复 3 个链路缺口（composition 拒 composite、匹配缓存不失效、组合关联零写入） | [`openspec/changes/archive/2026-09-04-add-composite-labels`](../../../openspec/changes/archive/2026-09-04-add-composite-labels) |
| 2026-08-22 | analysis-remediation | 存储清理两不变量落地：disabled 标签向量置 NULL（四条禁用路径同步置 NULL，重启用由 llm_extract 重算）+ `topic_tag_embeddings` 孤儿一次性清理并加 DB 层 `FK ON DELETE CASCADE`（迁移 `20260820_0001`，与 GORM 声明对齐） | [`openspec/changes/archive/2026-08-22-analysis-remediation`](../../../openspec/changes/archive/2026-08-22-analysis-remediation) |
| 2026-08-02 | revamp-landscape-charts | 话题态势版图可视化改 ECharts：新增「话题节奏总览」气泡图（聚合全部话题节奏成主载体）；卡片节奏条改 ECharts 迷你柱图（柱高=数值），emerging 卡片去图；活力折线改面积图；引入 echarts 模块化按需引入 + `useEcharts` 封装 | [`openspec/changes/archive/2026-08-02-revamp-landscape-charts`](../../../openspec/changes/archive/2026-08-02-revamp-landscape-charts) |
| 2026-08-24 | restore-gorm-default-tags | 修复 a0b03bdc tag 剥离回归：TopicTag/TagMergeSuggestion.Status 恢复 default tag（GORM 零值显式 INSERT 病根）、SemanticLabel.ContextLayers 改 BeforeCreate 填默认（tag 语法不可表达 JSON 数组默认值）、迁移 constrain helper 尊重 notNull 参数（架空 bug）——版块/标签默认状态行为恢复 | [`openspec/changes/archive/2026-08-24-restore-gorm-default-tags`](../../../openspec/changes/archive/2026-08-24-restore-gorm-default-tags) |
| 2026-08-24 | retire-narrative-legacy | 叙事面板死路由 GET /semantic-boards/:id/narratives 下线；「板块」×29 修正为「版块」；叙事面板节改版块治理面板（NarrativePanel 已死块删除） | [`openspec/changes/archive/2026-08-24-retire-narrative-legacy`](../../../openspec/changes/archive/2026-08-24-retire-narrative-legacy) |
| 2026-08-01 | board-topic-landscape | 版块内容 tab 首屏「话题态势版图」：identity 轨态势派生（🌱emerging/🔴pending/🟢active/⏸️stalled/⬛archived + 🌀强吸引叠加）+ 分区卡片墙 + mini-lifeline + 活力顶栏；新增 `GET /semantic-boards/:id/topic-landscape` 聚合接口；禁 similarity 轨五态，可见口径保留 hit≥1（含 emerging 新苗头） | [`openspec/changes/archive/2026-08-01-board-topic-landscape`](../../../openspec/changes/archive/2026-08-01-board-topic-landscape) |
| 2026-09-10 | overview-lane-dynamics | 版块内容首屏「话题态势版图」退役，「泳道动态」接管：active∪watch 泳道卡（滚动14天态势句+发展时间线）+ 候选栏；新端点 `GET /semantic-boards/:id/lane-dynamics`；日报后异步结算 `topic_lane_snapshots`（详见 daily-report.md §泳道态势结算） | [`openspec/changes/archive/2026-09-11-overview-lane-dynamics`](../../../openspec/changes/archive/2026-09-11-overview-lane-dynamics) |
| 2026-07-23 | board-discovery-expansion | 升级建议持久化生命周期 + 双签名算法 + 观察池 watch + 定时 06:30 生成；`board_upgrade_suggestions` 表（suggestion_hash 幂等）；dismiss 冷却期 + watch GC；旧 upgrade-suggest 保留兼容期 | [`openspec/changes/archive/2026-07-23-board-discovery-expansion`](../../../openspec/changes/archive/2026-07-23-board-discovery-expansion) |
| 2026-05-29 | matching-quality-and-daily-report-redesign | hit_rate/weighted 加方向校验；文章按匹配质量排序；日报展示精简 | [`openspec/changes/archive/2026-05-29-matching-quality-and-daily-report-redesign`](../../../openspec/changes/archive/2026-05-29-matching-quality-and-daily-report-redesign) |
| 2026-05-29 | board-direction-check-and-board-editing | max_sim 方向性校验（direction_mismatch）；版块 embedding 生成 + 一次性 backfill；前端版块编辑 | [`openspec/changes/archive/2026-05-29-board-direction-check-and-board-editing`](../../../openspec/changes/archive/2026-05-29-board-direction-check-and-board-editing) |
| 2026-05-26 | board-interaction-overhaul | max_sim 双因子约束（hits ≥ min(2,N) + hit_rate ≥ 0.3）；升级建议 DTO 增强（label 替代 #id） | [`openspec/changes/archive/2026-05-26-board-interaction-overhaul`](../../../openspec/changes/archive/2026-05-26-board-interaction-overhaul) |
| 2026-05-10 | narrative-concept-boards | `board_concepts` 表，版块从「每日重建」变为跨日持久概念实体；LLM 扫描 + embedding 匹配的版块概念自动建议 | [`openspec/changes/archive/2026-05-10-narrative-concept-boards`](../../../openspec/changes/archive/2026-05-10-narrative-concept-boards) |
| 2026-09-04 | constraint-declaration-redline | 约束节红线句格式化：本域「业务约束与不变量」节每条约束改写为首行加粗自含红线句 + 细节跟后（语义不变），declaration 注入降为红线层（上线后实测 bytes 降约 60%），细节层经关键词/JIT 全节注入按需补全；本域为格式改写，无业务行为变更 | [`openspec/changes/archive/2026-09-04-constraint-declaration-redline`](../../../openspec/changes/archive/2026-09-04-constraint-declaration-redline) |
| 2026-09-05 | add-evidence-backed-cross-board-relations | 跨版块关系发现与版块语义归属正交：目标解析只引用现有版块（约束 13），不自动创建/合并/修改版块、不做 board×board 全量扫描、不强制映射；confirmed 关系只注入简报背景字段不改版块成员 | [`openspec/changes/archive/2026-09-05-add-evidence-backed-cross-board-relations`](../../../openspec/changes/archive/2026-09-05-add-evidence-backed-cross-board-relations) |
| 2026-09-11 | overview-lane-dynamics | 版块内容 tab 首屏「泳道动态」视图（泳道卡：滚动 14 天态势句 + 逐日发展时间线 + watch 角标 + 候选栏），取代话题态势版图（整体退役）；新端点 `GET /semantic-boards/:id/lane-dynamics` 单请求聚合；态势句随日报异步滚动结算（见 daily-report.md） | [`openspec/changes/archive/2026-09-11-overview-lane-dynamics`](../../../openspec/changes/archive/2026-09-11-overview-lane-dynamics) |
| 2026-09-18 | add-source-board-hit-rate | 板块来源构成：新增只读端点 `GET /semantic-boards/:id/source-breakdown`（供血源视角：本版块篇数/占比/该源自身命中率，`Σ sources == total_articles`）；「板块内容」tab 底部（不新增 tab）`BoardSourcePanel.vue` 来源面板；与源视角共用 `sourcestats` 口径唯一实现（按文章去重/含已归档/限窗口，红线 17） | [`openspec/changes/archive/2026-09-18-add-source-board-hit-rate`](../../../openspec/changes/archive/2026-09-18-add-source-board-hit-rate) |
| 2026-10-10 | add-board-health-checkup | 版块体检：挂日报成功尾步每日纯读统计（版块级指标 + 标签级贴合度分布，10-08 口径修正弃单 aux 纯净样本）+ 事实画像 diff 未收编候选 + LLM 合并评分/评语/打扰裁决 + `board_health_snapshots` 快照趋势 + 异常通知（冷却 3 天）+ 只读端点 + member-derived 扩充召回主路（无快照回退双路，红线 10/20/21/22）；前端体检治理面板 BoardHealthPanel 吸收构成管理（BoardCompositionPanel 退役）+ 低分警告条 | [archive/2026-10-10-add-board-health-checkup](../../../openspec/changes/archive/2026-10-10-add-board-health-checkup) |
| 2026-10-06 | fix-aux-label-idle-tx-cache | 辅助标签入库两阶段化：AI embedding 调用移出事务（N 新标签恰 2 次批量调用），短事务毫秒级纯写保留全有或全无原子性，idle in transaction 4–40s → 0；同模式清理 ConfirmSuggestion（`ConfirmSuggestion` 预计算 + `ApplyCompositeLabel` 纯写）；缓存失效增量维护仅 activeLabels 半边（mergeEmbeddings 原位写曾致 concurrent map fatal 两次宕机，FB-1 修订回退 TTL 重拉，见 callout 与 code-style.md 事务内慢 I/O 硬禁）；PG 防呆 `idle_in_transaction_session_timeout=60s`（recreate 生效） | [archive/2026-10-10-fix-aux-label-idle-tx-cache](../../../openspec/changes/archive/2026-10-10-fix-aux-label-idle-tx-cache) |
| 2026-10-04 | fix-board-match-cross-domain | direct_hit 占比门槛（交集达标且占比 ≥ 0.5 才命中，不足退回相似度且重叠标签不作相似度证据——拦蹭词挂载，约束 3）+ 匹配配置新 key `semantic_board_match_direct_hit_min_ratio`（matching-config 可调）；存量弱挂载不重算（用户决策），日报侧防线见 daily-report.md 排他约束 | [archive/2026-10-10-fix-board-match-cross-domain](../../../openspec/changes/archive/2026-10-10-fix-board-match-cross-domain) |
| 2026-10-09 | fix-event-tag-time-anchor | event 标签时间锚污染修复：三处 prompt 捒掉自带「2024年4月」历史时间锚的「伊朗袭击以色列」毒示例（generateTagDescription event 分支重构为 buildEventTagDescriptionPrompt + 两个 extractor 正例全换中性虚构事件「某国央行上调基准利率」），event 描述生成加时间锚纪律三句（锚定 Context `[日期: …]`/禁 Context 外时间/同名历史事件以 Context 为准）；存量还债 CLI `cmd/admin fix-event-tag-anchor`（dry-run 默认出清单+备份 CSV，`--apply --ids` 显式圈选非候选拒碰，重生成复用首挂文章 articleContext 同一口径 buildArticleContext）；真库全量扫候选 321，圈选 4 病态标签 apply 4/4 成功，复扫 321→317 全部不再命中（约束 23） | [archive/2026-10-10-fix-event-tag-time-anchor](../../../openspec/changes/archive/2026-10-10-fix-event-tag-time-anchor) |
| 2026-10-29 | board-health-triage-advice | 版块体检三病分诊与共享描述修复：体检同轮纯读算三病信号（磁铁失灵=失败子集 vs 全部挂载贴合均值差，阈值 `board_health_magnet_gap` 0.08 / `board_health_magnet_min_failed` 5；描述污染=文章式词根预筛；泛化漂移=全库 active aux top-3 最近邻）→ LLM 单调用输出标签级分诊清单（remove/rewrite/keep/insufficient）落 `metrics.triage`，摘除走既有构成 DELETE、rewrite 走独立人工确认修复链；新增持久修复任务链（原子保存 description+storage 向量、幂等键、冻结全部活跃 topic_tags 范围、四类计数、显式 retry 只续匹配），共享标签影响面（可能改其它版块归属）确认前声明；体检轮次仍纯读不落扩充建议/不写匹配表，旧快照与日报不改写、效果待下一轮体检（红线 24-26） | [archive/2026-11-06-board-health-triage-advice](../../../openspec/changes/archive/2026-11-06-board-health-triage-advice) |
| 2026-10-30 | fix-nested-dialog-layering | 治理工作面「组合」页签点「创建并挂载」弹窗被宿主遮罩盖住（嵌套浮层层级写死 60 < 宿主 AppDialog 1000，用户点了按钮完全看不到弹窗）：引入浮层层级契约（`Z_INDEX` 档位表 dialog 1000 / dialogNested 1100 / readingDialog 9100 / readingFullscreen 9200 / toast 9999 + `nestedOverlayZ(hostZ)` 跳过已占档位抬升），组合标签创建弹窗改走嵌套档 1100；契约写入 `standard/frontend/layout.md`（存量手写层级登记豁免，主重构时收敛）；零接口、零数据、零迁移，仅前端层级 | [archive/2026-10-30-fix-nested-dialog-layering](../../../openspec/changes/archive/2026-10-30-fix-nested-dialog-layering) |
| 2026-11-02 | rework-workshop-into-radar-and-desk | 工坊重构为雷达+版块态势台：`/tags` 首屏改今日雷达（单次聚合端点 overview），五 tab 退役（矩阵/归档视图/编排/阅读台承接，8.4 物理清理旧宿主），新建泳道编排工作面（服务端候选链口径+质心推荐+画布缩放平移），阅读台路由化 `/tags/:boardId/:date` + 旧版三栏恢复 + 页边注/收藏/Esc 语义回归 | [archive/2026-11-02-rework-workshop-into-radar-and-desk](../../../openspec/changes/archive/2026-11-02-rework-workshop-into-radar-and-desk) |
| 2026-11-03 | clarify-archive-match-rules | 归档标签卡 popover 判定过程重排：旧「一句长判定 + 跳转匹配流程弹窗」改为就地逐条判定（⓪→⑤ 已试过未通过步给真实差距 + 命中步含问法原文与「这一步怎么算的」就地 KaTeX + 命中步后合并「未走到」排）；头区补「第 N 步 · 规则名 命中」与指标行；两个口径显式分家（收编=相似 ≥ 阈值 hits 口径标「非完全相同」，① 交集=direct_hit_auxiliaries 完全相同口径）；规则名/问法/编号抽单一常量源 `archiveMatchFlow.ts RULE_COPY`（popover 判定过程、工具行「匹配流程」弹窗、入板阶段名同源）；spec 侧「入板过程 popover」requirement 以 REMOVED+ADDED 重建（更名「五阶段得分」，移除三个历史错位 Scenario，锚点并入事件卡 popover 新 Scenario）；纯前端呈现改版，后端/口径零改动 | [`openspec/changes/archive/2026-11-03-clarify-archive-match-rules`](../../../openspec/changes/archive/2026-11-03-clarify-archive-match-rules) |
| 2026-11-04 | fix-board-health-triage-degrade | 体检分诊漏判不再整轮作废：`validateTriage` → `normalizeTriage`，合法但漏判的构成标签补 ⚪ 证据不足（覆盖率下限 0.5，低于才降级；结构性非法仍整轮降级），保留评分/评语/已给分诊；体检触发面扩到三条日报产出路径（定时主轮尾步零守卫 / 回填补出日报 / 手动生成端点，后两者异步 + 6h 新鲜度守卫且只把带评分快照算新鲜）；整轮预算 10→30 分钟 + 单板 6 分钟子超时（慢板不再吃掉兄弟板预算）；每板块每轮一行可检索日志（labels/covered/filled/degraded/elapsed，成功也打）。实证：11-03 一轮 12 板 6 板因漏判整轮作废（47/50/54/58/96/98 标签板回 45/40/51/48/≥68/≥72 条）（红线 27） | [`openspec/changes/archive/2026-11-09-fix-board-health-triage-degrade`](../../../openspec/changes/archive/2026-11-09-fix-board-health-triage-degrade) |
| 2026-11-06 | add-event-story-heat | 事件线身份与热度证据分离：新增 `board_stories`（跨天事件线，带 `subject/subject_key/action/object/occurred_at` 指纹与 `participant_count`）与 `story_signals`（每篇证据按 `pub_date` 优先计时）；日报线程加 `story_id/scope/is_development/story_manual/display_tier` 五列。热度改为从 signals 派生（story 48h/24h、lane 14d/72h 独立衰减，`heat-v1-48h-hl24h`），矩阵格值=事件线数、颜色=最热事件线来源数（4 档）、行序=lane_heat→story_count→id、雷达关注度=Σ lane_heat（P80/P20 标定 3.3/0.7）。合并闸门最终收紧为「显式 subject_key 优先、对象严格相等、指纹不可信一律不合并、LLM 不参与合并」；新增 `cmd/backfill-stories`（含 `--rebuild-signals`）与 `cmd/backfill-fingerprints` 两个运维入口。 | [archive/2026-11-06-add-event-story-heat](../../../openspec/changes/archive/2026-11-06-add-event-story-heat) |
| 2026-11-07 | surface-story-events-in-radar | 雷达右栏事件线化：「今日该读」条目单位由版块日报标题改为**事件线**（跨版块全局 Top N，每条带版块名 + 来源数 + 热度 + 该版块锚点日）；「今天可能升温」判定下沉到**事件线自身**（锚点日 story_heat ÷ 自身近 3 日基线 ≥2 + 当日有报道证据），倍率挂事件线不挂版块，基线为 0 者单列「今日新起」且不显示倍率。**红线 29 由版块级改写为事件线级**（热度来源与 ≥2× 阈值不变，新增「当日有证据」下界 = 收紧）；红线 30 增补「各版块用自己的锚点日 + 条目显式带该日期 + 跨板排序系统性偏差如实呈现 + 超时降级明示排序不完整」。 | [archive/2026-11-07-surface-story-events-in-radar](../../../openspec/changes/archive/2026-11-07-surface-story-events-in-radar) |
| 2026-11-07 | deconflict-radar-warming-and-new-stories | 雷达右栏三档口径拆清：**「今日新起」与「今日该读」互斥**（新起只列未进 Top N 的新事件线，超出列出上限附剩余计数摘要，全被覆盖时说明均已在「今日该读」中，SHALL NOT 重复展示同一事件线）；**「今天可能升温」空态分两成因**——`cross_day_story_count == 0`（既无锚点日之前 3 日基线窗证据、又无锚点日当天证据的事件线）时 SHALL 明示「当前没有跨天延续的事件线」并说明能力建设中，仅存在跨天线但未达 2 倍时 SHALL 用「暂未触发升温条件」；判据 SHALL 由服务端下发，SHALL NOT 由前端凭条目反推；**「今日该读」SHALL NOT 按来源数设硬门槛**（热度已含来源权重）。「今日新起」由「今天可能升温」卡片内的嵌套子块提升为同级独立分档。 | [archive/2026-11-07-deconflict-radar-warming-and-new-stories](../../../openspec/changes/archive/2026-11-07-deconflict-radar-warming-and-new-stories) |
| 2026-11-09 | matrix-clue-fallback-and-rail-merge | 矩阵降级兼容 + 右栏合并：① 雷达小矩阵**逐格降级**（事件线 0 而分区数 > 0 时按分区数兜底，title 明示降级）；② 控制台矩阵格与顶部时间视窗曲线改用**同一条降级规则**（事件线 > 0 用事件线数，否则线索合计兜底；`dayCellMetrics.displayCount` 单一派生，曲线标注/轴单位同步改口径）；③ 右栏由四块收敛为**一块清单「今日该读」（上限 5→8）+ 行内徽标**（`新起`/`升温 N×`），删除「今天可能升温」「今日新起」独立卡片与三态摘要，跨天延续收敛为一行脚注，后端移除 `warming`/`new_stories`/`new_count`/`new_total_count` 字段（徽标字段随条目自带）；④ 管线状态块默认折叠成一行摘要、异常自动展开。红线 28/29 增展示降级与单清单徽标化条款（判定口径不变） | [archive/2026-11-09-matrix-clue-fallback-and-rail-merge](../../../openspec/changes/archive/2026-11-09-matrix-clue-fallback-and-rail-merge) |
| 2026-11-19 | docs-sync-audit-fixes | 文档治理审计清偿（docs-only）：热度计算入口订正——唯一活实现 = lane_heat_repository.go（heat-v2-lane-clues-1d），story_heat.go（heat-v1）已硬停写（两处「唯一实现」表述改写）；溯源表补 fix-board-console-spec-residue 行、unify-lane-workbench 日期 11-16→11-17、若干 archive 死链修正 | [archive/2026-11-19-docs-sync-audit-fixes](../../../openspec/changes/archive/2026-11-19-docs-sync-audit-fixes) |
