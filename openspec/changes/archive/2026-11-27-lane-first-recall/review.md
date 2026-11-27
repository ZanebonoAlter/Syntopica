# review：lane-first-recall

> 本文件承载两件事：task 6.2 要求的「离线核对结果留档」，以及 review 意见处置
> （`review-trace.sh` 的建账格式）。

## 离线核对（task 6.1 ~ 6.3）

**口径与方法**：真库 `syntopica` **只读**（全部为 `SELECT`，未执行任何 DDL/DML，未重建任何日报）。
用 SQL 复刻实现的判定链，逐条对齐代码：

1. stage 1 ANN：`ORDER BY e.embedding::halfvec(2560) <=> lane.vec::halfvec(2560)`，池 = `max(top_k*4, 16)` = 32；
2. stage 2 精确复核：同一池按原列 `e.embedding <=> lane.vec` 重排；
3. 判定：先剔 `dist >= lane_recall_max_distance`，再取最近 `top_k`；
4. 跨泳道合并：同一 (board, tag) 取最近泳道；排他：他板 `score >= 本板 score + 0.3` 时剔除。

日期窗 `[2026-11-26 00:00+08, 2026-11-27 00:00+08)`（与 `collectBoardTags` 的整日窗口同口径）；
当日 `status='active' AND category='event'` 且当天有文章的标签池 605 个；有向量可锚的 active 泳道 61 条（11 个版块）。

### 1. 版块 15220（事故版块）—— 英伟达系覆盖

泳道 2687「黄仁勋风波及英伟达股价、产品发布」与 2688「芯片与算力管制以及显卡相关」在 `top_k=8 / τ=0.16` 下的召回（按距离升序，已合并重复标签）：

| 标签 | 距离 | 版块通道身份 |
| --- | --- | --- |
| 万斯支持黄仁勋留任英伟达 CEO | 0.0761 | composite_hit（已在本板） |
| 万斯公开为黄仁勋辩护 | 0.0930 | weighted（**救回**） |
| 马斯克力挺黄仁勋回归英伟达 CEO 职务 | 0.0987 | composite_hit（已在本板） |
| 英伟达暂停向东南亚发货 | 0.1043 | max_sim（已在本板） |
| 黄仁勋复职后首次全员会 | 0.1128 | **未挂载**（**救回**） |
| 黄仁勋拒绝 2009 年教授请求 | 0.1164 | weighted（**救回**） |
| 英伟达暂停东南亚芯片交付 | 0.1181 | weighted（**救回**） |
| 英伟达暂停向东南亚四国发货 | 0.1195 | weighted（**救回**） |
| 司法部未对黄仁勋启动正式调查 | 0.1238 | weighted（**救回**） |
| 特朗普交易英伟达股票 | 0.1322 | weighted（**救回**） |
| 英伟达逼近死亡交叉 | 0.1323 | weighted（**救回**） |
| 英伟达成立国家安全与出口管制监督委员会 | 0.1450 | weighted（**救回**） |

结论：**12/16** 条当日「英伟达系」标签（英伟达 / 黄仁勋 / Blackwell GPU 家族，共 16 条）经泳道通道入场，其中 **9 条**是 11-26 当天被 `kept >= 10` 开关整批丢弃的 `weighted` 或从未挂载的标签 —— 正是事故形态（当日日报只收 10 篇）。同板另外两条泳道（503 / 32）另贡献 9 条（含 2 条被排他剔除），故该板当日共 20 条泳道召回、其中 2 条被跨版块排他拦下。

**与 task 6.1 期望值的偏差（1 条）**：tasks 写「应召回 ≥13 条」。实测 12 条，缺口是泳道 2687 的 rank 9 / 10——`黄仁勋邀请好友骑摩托横穿美国`(0.1425) 与 `Denny's 餐厅回应黄仁勋复职`(0.1541) 均 **在 τ 之内**，但被 `top_k=8` 截断；
`SpaceX下调Blackwell GPU租赁价格`(0.1609) 与 `标普全球评级将英伟达列入负面观察名单`(0.1836) 在 τ 之外（后者是 design D2 已明示接受的漏召回）。
如需覆盖 14/16，把 `lane_recall_top_k` 调到 10 即可（无需改代码）——**默认值保持 8**：design D2 已把 8 标定为召回量拐点（8→12 只多 22%，8→6 掉 15%），改默认属于标定决策，留给用户拍板。

### 2. 「当天没有专属新闻的泳道保持空白」（task 6.1 后半）

61 条有锚泳道中 **25 条**在 `K=8 / τ=0.16` 下召回 0 条，全部是「当天确实没有专属新闻」的形态：

| 泳道 | 板 | hit_count | rank1 距离 |
| --- | --- | --- | --- |
| C罗世界杯首秀数据低迷 | 3122 | 1 | 0.380 |
| 国务院推行工资集体协商制度 | 2272 | 1 | 0.323 |
| 青海海西州突发强震引发连锁关注 | 2272 | 3 | 0.299 |
| 中东冲突波及旅游业：Booking 发布谨慎指引 | 1974 | 1 | 0.281 |
| 美伊形式对市场影响 | 1974 | 42 | 0.282 |
| 协议落地引发能源市场剧烈震荡 | 1974 | 77 | 0.179 |
| 俄乌冲突前线战损与战术博弈 | 2165 | 126 | 0.163 |

以黎/海湾/美伊这一族（1974 / 2165 / 2197 / 2128 等）全部落在空白侧 —— design 明确点名的「被邻座内容污染」的泳道，τ=0.16 下不会硬拉内容凑数（对比：这些泳道此前正是靠 section 级 0.18 门槛把海湾/美伊/俄乌内容硬塞进来）。单条 hit 的新泳道同样空白，符合「空着是正确结果」。

### 3. `lane_recall_max_distance` 改动后行为随之变化（task 6.3）

同日同池，仅改 τ（K 恒 8）：

| τ | (board,tag) 合并召回 | 泳道-标签对 | 空白泳道 |
| --- | --- | --- | --- |
| 0.16 | **106** | 132 | 25 / 61 |
| 0.20 | **213** | 288 | 17 / 61 |

放行量 **2.0 倍**，且新增部分按 design D2 的判据是可预期的邻座内容（GTA6 / 仁王3 / 米哈游一类）。参数确实在线可控。

### 4. 召回标签的身份构成（τ=0.16，11 版块合计）

| 身份 | 召回 | 被排他剔除 | 实际入场 |
| --- | --- | --- | --- |
| quality（`direct_hit`/`hit_rate`/`max_sim`，本来就在 kept） | 27 | 0 | 27 |
| `composite_hit` | 4 | 0 | 4 |
| `weighted`（**救回**） | 38 | 0 | 38 |
| 未挂载（**救回**） | 37 | **12** | 25 |
| 合计 | 106 | 12 | 94 |

- 「救回」量 = 38 + 4 + 25 = 67 条 / 11 版块 ≈ **6 标签/版块/天**，与 design D2 预估的「≈5 标签/版块/天」同量级。
- 跨版块排他**真的在拦**：未挂载标签里有 12 条被他板更强挂载剔除（例：`用户购买 AI 模型服务后遭遇失联` 在他板 0.81、本板未挂载 → 剔除），说明 D6「排他同样作用于泳道召回结果」不是空条款。
- 其余 27 条 quality 标签不会改变既有日报内容（它们本来就在 `kept`），泳道通道对它们是空操作。

## 自审记录（实现线程自查；task 7.1 的独立子线程 review 留给主线程派发）

### R-1 2026-11-27【Medium】spec 的「L2 调用失败整批降 L3」与代码现实不符

- **状态**：待处理（需用户/主线程拍板：保持现状，还是实现降级）

spec Requirement 3 的第二个 Scenario 写「L2 LLM 调用失败 → 系统 SHALL 按既有行为整批降 L3 并告警，SHALL NOT 中断日报生成」，design D4 也把这条当作「不变」的既有行为。**代码里不存在这条降级**：`ClusterTagsLane` 在 `clusterChatFn` 返错时 `return nil, fmt.Errorf("l2 decide AI call failed: %w", err)`，错误一路冒到 `GenerateDailyReport`，该版块该轮生成失败（`daily-report.md` 约束 35 的「真失败」那一态）。本 change 未改这条路径，并用 `TestLaneRecallL2FailureKeepsExistingAbort` 把**现状**钉住。

为什么不顺手实现降级：(a) 那是既有管线语义变更，超出本 change 声明的「保持不变」；(b) 降级会把整批 L2 标签（含泳道召回标签）倒入 L3，直接违反同 change 的 D4 硬上界（「泳道召回只能喂已有泳道」）—— 两条要求互相矛盾，只能由用户裁。对应红线已按**代码现实**写进 `daily-report.md` 约束 72 末句。

### R-2 2026-11-27【Low】`composite_hit` 不在质量分级的 kept 集合里（既有问题，未在本 change 修）

- **状态**：已驳回（本 change 范围外，另开 change）

`filterTagsByQuality` 的 kept 只认 `direct_hit`/`hit_rate`/`max_sim`，`composite_hit` 落进 `weighted` —— 即「组合命中」（score 恒 1.0，最强证据）在 `kept >= 10` 的日子会被整批丢弃。11-26 事故同理（design 表里把 3 个 composite 算进「13 个高质量」，代码实际只算进 10 个）。
本 change 不修：`filterTagsByQuality` 的加权语义是 fix-digest-tag-leak 的口径，改动会波及所有版块；但泳道召回**顺带**把「被泳道证实的 composite 标签」救回来了（`laneRecalled` 标记 + laneKept）。建议另开 change 收敛 kept 集合。

### R-3 2026-11-27【Low】泳道召回标签的 `MatchReason` / `Score` 语义

- **状态**：已采纳

新增 `match_reason='lane_recall'`（不借用 `weighted`）：质量分级、`quality_breakdown` JSON、L3 闸门都需要把「版块统计猜测」与「泳道语义见证」分开；`tagging.MatchTier` 对未知 reason 返回 3（weighted 档），前端 `matchQuality.ts` 对未知 reason 回退中性色与原始字符串，故新增值不需要改前端（本 change ui-impact: none）。
`score` 记 `1 - 距离`（到见证泳道锚点的余弦相似度），**不是**版块匹配分——`match_reason` 已标明来源，`avg_score` 字段因此对纯泳道 section 会偏低（与 watch 物化 section 的 `AvgScore=0` 同属「如实低分」）。

### R-4 2026-11-27【Medium】30 条名额上限对泳道标签的处理（design 未覆盖）

- **状态**：已采纳

`filterTagsByQuality` 的 `> 30` 截断是版块统计证据的名额竞争（tier → score → 疑似 AI 占比）。泳道召回标签若参与该排序会被系统性截掉（`lane_recall` 落 tier 3、score 是相似度而非版块分），等于白救；若参与又会被怀疑拖大报告。
采取：laneKept **不计入** `kept >= 10` 阈值（否则会把开关顶过阈值、连带丢弃其余 weighted —— 违反「零回归」），也**不参与** 30 条截断，而是追加在 `kept` 之后。上界由 `lane_recall_top_k`（每条泳道每天最多 8 条）+ 泳道数自然约束，实测量级 +6 标签/版块/天。已在 `filterTagsByQuality` 注释与 `daily-report.md` 约束 71 写明。

### R-5 2026-11-27【High】写副作用排查（挂载表 / 话题表 / 关系表）

- **状态**：已修复（设计上避免）

`laneRecall` 全链路只读，理由与证据：
1. 召回查询只用 `SELECT`（`topic_tag_embeddings` / `topic_tags` / `article_topic_tags` / `articles` / `board_persistent_topics`），事务里只有 `SET LOCAL hnsw.ef_search`（会话内局部，随事务结束失效、不污染连接池）；
2. 排他判定**没有**复用 `tagging.SemanticBoardMatchingService.MatchTopicTag` —— 那条路径会 `replaceTopicTagBoardLabels` **写** `topic_tag_board_labels`（既有 fallback 通道正是靠它落挂载）。本 change 改为直接只读 `topic_tag_board_labels` 现有行做分差判定，泳道通道零写入；
3. 不写 `embedding` / `centroid` / `hit_count` / `status`，不动 `board_topic_relations`；
4. 手工核对：新代码里唯一的写操作调用是 `buildArticleContextForTag` 内的只读查询与 `applyAISuspectRatios` 的只读查询。

### R-6 2026-11-27【Medium】降级路径与顺序

- **状态**：已采纳

- `LoadLaneRecallConfig`：读表失败 / 缺行 / 非法值 → 各自回退默认 + `Warnf`，函数永不返错；`parseLaneRecallConfig` 纯函数，逐字段独立回退（一个 key 坏不影响另一个）。
- 泳道级失败隔离：向量缺失 / 非有限值 / > 4000 维（halfvec 上限）/ 单条 ANN 查询出错 → 只跳过该泳道 + `Warnf`，其余泳道与整份日报继续；`RecallTagsForBoard` 仅在**列泳道**这一步失败时返错，调用侧仍降级为「只用版块通道」。
- 排他查询失败 → 记 warn 并放行（宁可多收，不因排他探测失败丢掉召回；与主查询的排他条件不同源，后者在 SQL 里失败会直接报错）。
- 标签行加载不到 / 当天无文章 → 跳过该标签（不阻断）。
- 顺序：`collectBoardTags`（两通道并集）→ `DeduplicateTags` → `filterTagsByQuality` → `ClusterTagsLane`（L3 闸门）。排他判定在**并集入口**做（与版块通道同位置），L3 闸门在聚类内做，互不覆盖。

### R-7 2026-11-27【Low】索引命中与量纲

- **状态**：已采纳

查询表达式与索引定义逐字一致：索引 `((embedding)::halfvec(2560)) halfvec_cosine_ops WHERE embedding_type='semantic'`（`scripts/db/tag-embedding-halfvec-hnsw-20261104.sql`），查询 `ORDER BY e.embedding::halfvec(%d) <=> ?::halfvec(%d)`（dim 取自泳道锚向量长度）+ `e.embedding_type='semantic'` + `SET LOCAL hnsw.ef_search=200`（≥ 池 32，避免 pgvector 默认 40 之外的截断风险；池 < 40 时也无害）。静态守卫测试 `TestLaneRecallCandidateSQLUsesIndexedExpression` 钉住「不许裸 `::halfvec`」。
注意：该索引是**手建**（不进自动迁移链，见脚本头部说明），所以代码必须能在无索引时退化——不做 `EXPLAIN` 断言，退化路径只是慢（61 次 seq scan/天）。

### R-8 2026-11-27【Low】多泳道同标签的归属边界

- **状态**：已采纳

同一标签被多条泳道召回时只保留**最近**泳道作为见证（`mergeNearestLaneRecall`），候选集里仍只有一条标签行；最终归属仍由既有 L1/L2 分桶 + L2 裁决决定 —— 召回层不替归属层做决定（design 边界「MUST NOT 让一个标签同时进入多条泳道的候选」）。`lane_recalled` 标记只表达「这个标签的入场靠泳道证据」，不表达「它属于哪条泳道」。

---

## 独立子线程 review（task 7.1，2026-11-27）

> 只读审查，范围 = `review-brief.md` 第一节清单（迁移 / 召回查询 / 收集接线 / L3 防护 / 测试 / 三处文档），契约对 `spec.md`、设计对 `design.md`、任务对 `tasks.md`。上文「离线核对」与 R-1~R-8 是实现线程自审，本节意见从 **R-9** 起续编号（`review-trace.sh` 可计），不复述 brief。worktree 内其他会话的归档改动未审。

### 1. 结论

**有条件通过**。核心机制（ANN + 原列复核、两通道并集、泳道证据放行、L3 闸门、参数降级、零写挂载表）与 spec/design 一致且有测试钉住，board 通道零回归；但有两项须在归档前收敛：① 泳道通道**绕过 `direction_mismatch` 过滤**（真库 11-26 实测 9 条，触碰 semantic-board 红线 4 与 design D6「不变」）；② spec 的「L2 调用失败整批降 L3」Scenario 描述的是**不存在**的行为，测试钉的是反例，归档 sync 会把假 SHALL 写进主 spec。另：HNSW 索引在生产计划里**未命中**（走 Sort，功能与性能均无碍，但 design D5 / 注释 / 文档口径要改）。

### 2. 必须修（High）

### R-9 2026-11-27【High】泳道通道绕过 `direction_mismatch` 过滤（真库 11-26 命中 9 条）

- **状态**：待处理
- **位置**：`backend-go/internal/topicgraph/service/daily_report_orchestrator.go:910-958`（`laneRecallExclusivityAllowed` 的只读查询 `:934-935` 只取 `topic_tag_id, semantic_board_id, score`，不读 `direction_mismatch`）；对照主查询 `:625`（`NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)`）。
- **事实**：主查询把本板 `direction_mismatch=true` 的标签整体排除，这些标签因此不在 `existingReason` 里；泳道通道把它们当「版块通道没有的新标签」走 `pending`，排他判定只比 score，于是以 `match_reason='lane_recall'` 重新进入候选集。真库只读核对（`syntopica` 本地库，2026-11-26，board 15220，全部 `SELECT`）：当日 event 池 679 条，落在本板 active 泳道 τ=0.16 内 70 条，其中 **9 条在本板只有 mismatch 行**：`OpenAI 测试 Private Inference`(0.1119，1980:0.70 / 15220:0.70M)、`OpenAI 机器人团队重组`(0.1185)、`Anthropic 迁移 Messages API 至 Rust`(0.1198)、`Anthropic 推出 Claude Ship 测试版`(0.1218)、`OpenAI 撤销独立机器人团队`(0.1294)、`Claude Code 核心成员加盟 OpenAI`(0.1367)、`DeepSeek 替代 Luna 中转站`(0.1487)、`用户迁移编程工具`(0.1488)、`DeepSeek 开源 super-agent 原型`(0.1550)。与 1980 同分或分差 <0.3，排他 gap 不拦。
- **为什么是问题**：semantic-board 红线 4「mismatch 仍记录但不计日报且前端默认隐藏」被打破；design D6 表格写「`direction_mismatch` 过滤｜不变」，实现与之不符，spec 也没有声明豁免；自审 R-1~R-8 与「离线核对」都只按 composite/weighted/max_sim 看本板身份，漏了这一维。
- **怎么改**：二选一并落到 spec/design。(a) **默认建议**：`laneRecallExclusivityAllowed` 的同一条只读查询带回 `direction_mismatch`，`laneRecallExclusivityVerdict` 对「本板存在 mismatch 行且不存在非 mismatch 行」判 not allowed，补一条纯函数测试（含「mismatch 行 + 他板同分」形态）。(b) 若产品上认定「已成立泳道的延续」应压过版块级方向标记（AI 公司新闻落 15220 的 AI 泳道看起来是合理内容），则 spec Requirement 1 增 Scenario、design D6 改口径、`daily-report.md` 约束 71 补一句、semantic-board 红线 4 加例外——不能默认这样发生。

### R-10 2026-11-27【High】spec「L2 调用失败仍按既有行为降级」与实现相反，测试钉的是反例——归档前必须收敛 spec

- **状态**：待处理（与 R-1 同一事实，升级为 High：不收敛不能归档）
- **位置**：`openspec/changes/lane-first-recall/specs/lane-first-recall/spec.md` Requirement 3 末段 + 第二个 Scenario；`backend-go/internal/topicgraph/service/daily_report_lane.go:412-415`（`clusterChatFn` 返错 → `return nil, fmt.Errorf("l2 decide AI call failed: %w", err)`，无任何降级分支）；`daily_report_lane_gate_test.go:109-127`（`TestLaneRecallL2FailureKeepsExistingAbort` 断言 `require.Error` 且 `groups == nil`）。
- **事实**：独立复核成立——代码从未有「整批降 L3 + warn」路径，错误冒到 `GenerateDailyReport`（`daily_report_orchestrator.go:112-114`），该版块本轮生成失败（约束 35「真失败」态）。spec 两处把这当成「既有行为」并写 SHALL NOT 中断日报生成；`scenario-trace.sh` 7/7 通过只核对测试文件存在，对该 Scenario 是假阳性；`daily-report.md` 约束 72 末句已按代码现实写，与 spec 直接矛盾。
- **为什么是问题**：spec 是契约权威源，带着这条 sync 进主 spec 就是写入不存在的 SHALL；design D4「保持」的对象也不存在。
- **怎么改**：推荐**改 spec、代码不动**（符合 change「保持不变」意图）：删掉该 Scenario；Requirement 3 末段改为「L2 调用失败时沿用既有行为（`ClusterTagsLane` 返回错误、该版块本轮生成失败），本 change 不引入降级，泳道召回标签在该路径上不进入任何分组」；`tasks.md` 第 10 节映射表同步删行。若用户决定实现降级，另开 change——且降级路径仍应先过 `dropLaneRecalledTags` 再入 L3，D4 不构成阻碍（R-1 的理由 (b) 不成立）。

### 3. 建议修（Medium）

### R-11 2026-11-27【Medium】HNSW 索引在生产计划里未命中（三份 EXPLAIN 均走 Sort），design D5 / 注释 / watch 文档的「命中索引」表述不实

- **状态**：待处理
- **位置**：`backend-go/internal/topicgraph/repository/daily_report_lane_recall.go:343-352`（`recallLaneCandidates` 注释「hits idx_topic_tag_embeddings_semantic_halfvec_hnsw」）、`:60`（`laneRecallEfSearch = 200`）、`:378-383`（`SET LOCAL hnsw.ef_search`）；design D5「成本：61 条 active 泳道 × 1 次 ANN/天」；`docs/reference/database/tables/daily-report-watch.md:141`「查询 MUST 用 … 才能命中索引」。
- **事实**：真库只读 `EXPLAIN (COSTS OFF) EXECUTE`（pg18 + pgvector，索引存在；锚 = 2687 的 embedding 回退，2026-11-26 窗口，LIMIT 32）：custom plan、6 次执行后、`force_generic_plan` 三份计划全是 `Limit → Sort → Nested Loop(articles by idx_articles_feed_pub_date → article_topic_tags → topic_tag_embeddings by idx_topic_tag_embeddings_tag_type_hash → topic_tags pkey)`，`idx_topic_tag_embeddings_semantic_halfvec_hnsw` 零出现。原因：日期过滤 + EXISTS 让「先筛当天池（~680 行）再排序」比 ANN 扫描更便宜。pgx 语句缓存 5 次后转 generic plan，生产上绝大多数泳道查询走的就是这份 Sort 计划；`SET LOCAL hnsw.ef_search=200` 因此不生效。
- **影响**：功能正确且更准（Sort 用 halfvec 距离精确排序，无 ANN 近似），性能可接受（EXECUTE 即时返回）；问题在口径与成本模型。另 `top_k=64` 时 pool=256 > ef_search 200，若将来计划器切回索引会截断。
- **怎么改**：design D5 改为「表达式与索引兼容；当天池小，计划器现选 Sort；索引只在池变大时兜底」；watch 文档 :141 同改；`recallLaneCandidates` 注释同改；`laneRecallEfSearch` 改为 `max(200, pool)` 或连同 `SET LOCAL` 一起删（YAGNI）。

### R-12 2026-11-27【Medium】Scenario「泳道向量缺失时跳过该泳道」无行为级测试；「维度不符」靠 SQL 报错兜底而非预检

- **状态**：待处理
- **位置**：`backend-go/internal/topicgraph/repository/daily_report_lane_recall_test.go:181-200`（只测纯函数 `laneRecallAnchor` / `laneRecallAnchorUsable`）；`daily_report_lane_recall.go:293-345`（`RecallTagsForBoard` 的 `skipped++` `:318` 与 `failed++` `:328` 分支无任何测试调用）。
- **事实**：「NULL 向量泳道跳过 + 其余泳道继续 + 不返错」没有测试执行过；`laneRecallAnchorUsable` 只拦 0 维 / >4000 维 / 非有限值，「与标签向量空间维度不符」不在预检里——真库实测 `embedding::halfvec(1536)` 报 `expected 1536 dimensions, not 2560`，靠 `failed++` 兜底，日志是「recall failed, skipped」而非维度不符，且每板每天为该泳道浪费一次失败查询。
- **怎么改**：service 包补一条 testcontainers 用例（一个 NULL 向量泳道 + 一个正常泳道 → 只含正常泳道的召回、无 error）；`RecallTagsForBoard` 预检锚点维度与标签空间维度（与 `unifyVectorColumnDimensionMigration` 同一常量来源），不符即 skip + 明确 warn。tasks 2.6「泳道无向量跳过」的勾选需要这条证据。

### R-13 2026-11-27【Medium】`TestLaneRecallExclusivityVerdictGapZero` 对 gap<=0 的断言是自证式的，`laneRecallExclusivityAllowed` 的关闭分支从未被执行

- **状态**：待处理
- **位置**：`backend-go/internal/topicgraph/service/daily_report_lane_recall_test.go:141-158`；`daily_report_orchestrator.go:910-922`（`gap <= 0` 短路径）、`:960-978`（纯函数）。
- **事实**：`allowed := map[uint]bool{3: true}` 是测试自己造的字面量，随后 `assert.True(allowed[3])` / `assert.Len(allowed, len(hits))` 断言的是这个字面量；生产短路径零覆盖。且 `laneRecallExclusivityVerdict(…, 0)` 对所有标签返回 false（`x < 0` 恒假），与「gap=0 关闭排他」语义相反，只靠调用方短路径保护——是个脚枪。
- **怎么改**：给 `laneRecallExclusivityAllowed` 抽一个接收 `rows` 的纯函数入口（或注入查询函数），用真实调用断言 gap=0 时全部放行；`laneRecallExclusivityVerdict` 内部加 `if gap <= 0 { verdict[id] = true }` 让纯函数自洽。

### R-14 2026-11-27【Medium】Scenario「当天没有专属新闻的泳道保持空白」后半句（该泳道当日无 section）无断言

- **状态**：待处理
- **位置**：`repository/daily_report_lane_recall_test.go:105-116`（`TestLaneRecallDecisionEmptyLane` 只测 `recallDecision`）；`service/daily_report_lane_recall_test.go:104-113`（`TestLaneRecallEmptyLaneContributesNothing` 的输入是 `nil` / 空 hits，等于没测）。
- **事实**：spec 的 THEN 两半——不召回（有纯函数断言）+ 泳道无 section（无端到端断言）。逻辑上无标签 → `BucketTagsByCentroid` 不为该泳道产组，推断成立但未钉住；`TestLaneRecallChannelWritesNoMount` 只覆盖「有召回」一侧。
- **怎么改**：在 service 包 testcontainers 用例里加一个锚点正交的「远泳道」，断言 `collectBoardTags` 的 `laneRecalled` 为空且 `ClusterTagsLane` 输出里没有该泳道的组。

### 4. 可忽略（Low）

- `docs/reference/flow/topic-graph.md:96` 夹杂乱词「（blogs详见 …）」；同文「缺失/非法回退默认并告警」——缺 key 时 `parseLaneRecallConfig`（`daily_report_lane_recall.go:83-106`）不打 warn，只有非法值打，文档与实现差一词。
- `RecallTagsByLane`（`daily_report_lane_recall.go:264`）无调用方，与 `RecallTagsForBoard` 重复 anchor/skip 逻辑；tasks 2.1 要求存在，建议后者复用前者或删掉。
- `laneRecallExclusivityAllowed` 排他查询失败时 fail-open（`:938`「keeping recalled tags」），与主查询 fail-closed 不同源；自审 R-6 已记，建议 design D6 补一句。
- 前端 `front/app/utils/matchQuality.ts:27` 对 `lane_recall` 回退加权色 + 原始字符串；ui-impact: none 可接受，日报质量面板会显示英文 key，可选补中文标签。
- `tasks.md` 7.2 的命令写成 `review-trace.sh lane-first-recall`，脚本实际要求 change 目录路径（裸名会报「change 目录不存在」），照抄会误判。
- 自审 R-7 称索引「手建、不进自动迁移链」，但链里已有 `ensureTopicTagHalfvecIndexMigration()`（`postgres_migrations.go:2495`）；表述过时，结论（生产索引存在）不受影响。

### 5. 已验证通过

| # | 检查 | 结果 |
| --- | --- | --- |
| 1 | `cd backend-go && go build ./...` | PASS（22s，load 11） |
| 2 | `cd backend-go && go vet ./...` | PASS |
| 3 | `go test ./internal/topicgraph/repository -run 'TestLaneRecall' -count=1` | 11 个用例 PASS（4.5s） |
| 4 | `go test ./internal/topicgraph/service -run 'TestLaneRecall\|TestFilterTagsByQuality' -count=1` | 23 个用例 PASS（含 testcontainers `TestLaneRecallChannelWritesNoMount` 23.3s） |
| 5 | `go test ./internal/platform/database -run 'TestLaneRecall' -count=1` | PASS（18.2s，testcontainers：播种 / 幂等 / 不覆盖已改值） |
| 6 | `bash scripts/harness/scenario-trace.sh openspec/changes/lane-first-recall` | 7/7 映射（注意：对 R-10 那条是假阳性） |
| 7 | 迁移幂等与不覆盖 | `lane_recall_migration.go:48-52` `ON CONFLICT (key) DO NOTHING`；`models.AISettings.Key` 带 `unique`，冲突目标成立；版本 `20261126_0001` 在非测试迁移中唯一，登记在链尾；`tableExists` 守卫 |
| 8 | ANN 表达式 vs 索引 DDL | `scripts/db/tag-embedding-halfvec-hnsw-20261104.sql:26-28` 与 `laneRecallAnnSQLTemplate`（`daily_report_lane_recall.go:148-168`）逐字一致：`(embedding)::halfvec(dim) <=> ?::halfvec(dim)` + `embedding_type='semantic'` partial 条件；`TestLaneRecallCandidateSQLUsesIndexedExpression` 静态钉住 |
| 9 | 原列精确复核与截断 | stage 2 `laneRecallExactSQL` 用 `e.embedding <=> ?::vector` 重排整池；`recallDecision`（`:196-220`）按精确距离先剔 `>= maxDistance` 再取 topK，内部排序不改调用方切片 |
| 10 | 不写持久事实层 | orchestrator 新代码区（:572-1000）与 repository 新文件无 Create/Save/Update/Delete/Exec 写调用；`buildArticleContextForTag` 只 `Find`；`RecallTagsForBoard` 只 `SELECT` + 事务内 `SET LOCAL`；`TestLaneRecallChannelWritesNoMount` 断言挂载表零新增。版块通道 fallback 的现场匹配仍会写挂载——那是既有行为，不在本 change |
| 11 | 跨版块排他仍生效且走 Go 分支 | `laneRecallExclusivityAllowed` `gap <= 0` 为 Go 短路径，SQL（`:934-935`）不带 gap 参数；`laneRecallExclusivityVerdict` 的 `bestOther - own >= gap` 丢弃与主查询 NOT EXISTS `stronger.score >= score + gap` 等价；`TestLaneRecallExclusivityVerdict` 四种形态钉住 |
| 12 | 双通道合并与 `laneRecalled` 置位边界 | `laneRecallAdmission`（`:793-806`）只对「本板无行」与「本板非质量档」置位；质量档（direct_hit/hit_rate/max_sim）不置位、行与 MatchReason/Score 不动；`TestLaneRecallAdmissionDualChannel` 四形态钉住——版块质量标签不会让 `filterTagsByQuality` 判据失真 |
| 13 | 质量分级零回归 | `filterTagsByQuality`（`:1116-1157`）`laneKept` 不计入 `kept>=10`、不参与 30 截断排序，追加在 `kept` 之后；`laneRecalled` 为 nil 时与旧实现逐字等价；4 个既有测试仅改签名；`TestLaneRecallQualityGateThresholdNotInflated` / `NilMarkerIsLegacy` 钉住 |
| 14 | L3 防护边界 | `ClusterTagsLane`（`daily_report_lane.go:421-433`）L2 `new` + `laneRecalled` → 丢弃；非泳道 → 照旧 L3；L3 桶内泳道标签在 no-LLM fallback 分支前已 `dropLaneRecalledTags`；`laneConstraintOption` 合并两个 option；`TestLaneRecallL2NewDropsLaneTag` / `L3BucketDropsLaneTag` / `L3BucketKeepsUnmarkedTags` 钉住 |
| 15 | 参数与降级 | 缺失→默认；非法 / 越界（top_k 1..64、distance (0,1]）/ 空白→默认 + Warnf；`LoadLaneRecallConfig` 读表失败→默认不返错；单泳道 ANN 出错→`failed++` 跳过；列泳道失败→返错但 `collectLaneRecalledTags`（`:831-835`）降级为只用版块通道；`RecallTagsForBoard` 对非法 cfg 回退默认 |
| 16 | 泳道见证标签的归属路径 | 全部 active 泳道都在 `ListAnchorableTopicsByBoard` 内（`selectAnchorableTopics` 只对 candidate 做窗口/上限）；τ=0.16 < L1 0.18，见证标签常态直落 L1；`topicAnchorVec`（`daily_report_lane.go:116-124`）与 `laneRecallAnchor` 同为 centroid→embedding，召回锚与分桶锚同一点 |
| 17 | 下游兼容 | `match_reason` 无 DB CHECK 约束；`DeduplicateTags` 不读 MatchReason/Score；`quality_breakdown` 原样透传；`TagInput.Score` 只进 section `AvgScore` 与 30 截断排序（laneKept 不参与） |
| 18 | 文档红线表述 | `daily-report.md` 71/72 与 spec 语义一致、默认值写「默认」非硬约束、含「同一条排他判定」「不写挂载表」「取最近者」；watch 表文档、topic-graph 参数表、`configuration.md:268-269` 两行均在 |
| 19 | 自审 R-1 代码事实 | 独立复核成立：L2 失败无降级路径（见 R-10） |
| 20 | 本地库状态（只读） | `schema_migrations` 已含 `20261126_0001`，`ai_settings` 两行默认值已落地——说明未提交代码已被某会话启动并跑过迁移；不影响审查结论，提醒主线程这是 worktree 噪音之外的「状态已变」事实 |

### 6. 未验证项

- **真库端到端**：未用新代码对 2026-11-26 board 15220 跑 `GenerateDailyReport`（会写库，禁止）；上文「离线核对」是 SQL 复刻而非代码路径，本审查未复跑其 106/132 等数字，只独立核对了 direction_mismatch 维度（R-9）。
- **EXPLAIN 覆盖面**：只用 2687（embedding 回退锚）做 EXPLAIN；2688 等 centroid 锚未逐一跑。计划形状由日期过滤决定、与锚无关，结论可外推但不等于全量验证。
- **`golangci-lint run ./...`**：brief 第三节未列且机器 load 10+，未跑；自审记录称 `0 issues`，未复核。
- **前端渲染**：`SectionQualityExplore.vue` 对 `lane_recall` 的实际显示未在浏览器验证（ui-impact: none）。
- **半精度池 → float32 复核的边界漏召回率**：pool=top_k×4 的余量未在真实距离分布上量化（R-11 指出当前根本不走 ANN，故此项暂无实际影响）。
- **「维度不符」泳道**：只验证了 Postgres 会报错（`expected 1536 dimensions, not 2560`），未在 testcontainers 里跑通「报错 → 跳过 → 其余泳道继续」全链路（R-12）。

## 处置记录（2026-11-27，主线程）

| 编号 | 级别 | 处置 | 落地 |
| --- | --- | --- | --- |
| R-9 | High | **已修** | 两条召回 SQL 均加 `NOT EXISTS (... bl.semantic_board_id = ? AND COALESCE(bl.direction_mismatch,false))`；`boardID` 从 lane 带进入参。新增回归守卫 `TestLaneRecallCandidateSQLExcludesDirectionMismatch` |
| R-10 | High | **已修制品、代码不动** | spec 第 60 行与 Scenario 改为「`ClusterTagsLane` 返回错误、该版块该轮生成失败」，与 `daily_report_lane.go:414` 实测及 `daily-report.md` 约束 72 末句一致；design D4 同步并留痕「原稿描述有误」 |
| R-11 | Medium | **已修表述** | design D5、代码注释、`daily-report-watch.md` 统一改成「表达式与索引定义一致，但当日池千级时 EXPLAIN 走 Sort、索引不参与，`ef_search` 实际不生效」 |
| R-12 | Medium | **已补**（生产代码零改动） | `TestLaneRecallSkipsUnusableLanesKeepsOthers`：三条泳道（NULL / 2 维 vs 2560 / 健康）断言不返错、只召回健康泳道者；并分别钉住 NULL 走预检、维度不符走 SQL 报错后由 per-lane `failed++` 吞掉。**遗留**：维度不符仍是「报错兜底」而非预检，加预检时该断言会红，正好提醒同步 |
| R-13 | Medium | **已补** | 删掉自证式 `TestLaneRecallExclusivityVerdictGapZero`，换成 `...GapDisabled`（`Repo=nil` 下 gap=0/−0.3 仍全放行 → 证明短路径不查库）+ `...Boundary`（4 组落库边界：差==gap 剔、0.08<gap 留、本板未挂但他板 0.90 剔、仅本板挂 留）。**遗留**：`laneRecallExclusivityVerdict(…, 0)` 本身仍对全部返回 false，脚枪由调用方短路径保护，已在测试注释写明 |
| R-14 | Medium | **已补** | `TestLaneRecallEmptyLaneKeepsItsDayBlank`：正交锚（距离 1.0）饥饿泳道 + 命中泳道共用同一标签/文章，断言只召回 1 条且见证者是近泳道、`laneRecalled` 命中、`ClusterTagsLane` 只产出 1 个 group 且 `MatchedTopicID==近泳道`、远泳道无 group（当日无 section）、零 LLM 调用 |
| R-1~R-8 | — | 自审项，已在实现中处理 | 见各条正文 |

**对 R-9 数量的独立复核（主线程）**：审查报告写「真库 11-26 命中 9 条」，主线程用 `τ=0.16` 复算为 **1 条**（`OpenAI 撤销独立机器人团队` 0.158）；12 条 mismatch 标签中 τ=0.20 才会有 8 条。缺陷性质不变（红线 4 被绕过、必须修），但影响量级按默认阈值是 1 条/天，报告中的 9 条对应 0.20 档。
