
## boardhealth 代码现状与扩展点（实现直读）

实现 board-health-triage-advice 前必读的代码现状事实（2026-10-28 实查）：

**扩展点已就绪，改动面比想象小**：
1. `matchedRow`（metrics.go:42）已携带 `DirectionMismatch bool`，`loadMatchedRows` 的 SQL 已查 `direction_mismatch`——磁铁信号的分组依据零新增查询，直接在 `collectBoardStatistics` 已加载的 `matchedRows + tagEmbeddings` 内存数据上分组算两侧均值。
2. LLM 单调用结构（llm.go）：`BoardHealthLLM interface { ScoreBoardHealth(ctx, prompt) (*BoardHealthLLMResult, error) }`，`BoardHealthLLMResult{Score int, Comment string, Notify bool}`——只加 `Triage []TriageItem` 字段+解析校验；`boardHealthSystemPrompt` 常量已有「统计口径防误读」节（短实体/insufficient/P25 相对线），分诊判据节接在其后；`buildScorePrompt(board, metricsJSON, candidates)` 是 user prompt 组装点。
3. 快照模型（models/board_health.go）：`BoardHealthMetrics{Board, Labels []BoardHealthLabelMetrics}` jsonb serializer——Labels 加 `Magnet/Neighbors/DescFlags`、顶层加 `Triage`，零值 nil 自然兼容历史快照（无 DDL 无迁移）。`uncaptured_candidates` 是同模式先例（独立 jsonb 列）但本 change 不走新列。
4. 快照组装（run.go）：`CheckupBoard(ctx, db, llm, config, board, now)` = collectBoardStatistics → UncapturedCandidates → LLM → 降级（score/comment NULL 照常落库）→ INSERT。triage 挂进 metrics，降级路径 triage 随置空。`scorePromptCandidates=10` 是 prompt 截断先例（D6 护栏仿此）。
5. 摘除接口已存在：`DELETE /api/semantic-boards/:id/composition/:auxiliary_label_id`（router.go → board_crud_handler.go:155 removeBoardComposition）——前端直接调，后端零新增写路径。
6. 端点（board_health_handler.go）：`GET /api/semantic-boards/:id/health-checkup?trend=N`（N 白名单 1-90 默认 7）——响应是快照透传，模型加字段自然透出，handler 改动极小。
7. 前端链：`useBoardHealth.ts`（composable，单次请求）→ `api/normalizers/consoleRail.ts`（snake→camel 归一，RailHealthSnapshotView 类型）→ `components/console/RailHealthBrief.vue`（brief/full 双形态，352 行）+ `BoardHealthDetail.vue`（650 行）。分诊清单挂 full 形态 + Detail；测试文件 `components/BoardHealthPanel.test.ts` 已存在（未收编候选「一键收编」先例在此）。
8. 配置先例：`config.go` LoadConfig 从 ai_settings 读 `semantic_board_*` key——新 key `semantic_board_health_magnet_gap`(默认0.08)/`semantic_board_health_magnet_min_failed`(默认5) 仿此。
9. 最近邻查询：`semantic_labels.embedding` 是 vector(2560) 列，pgvector `<=>` 操作符可用（无 ivfflat 索引走顺序扫描，15k 行×每晚一轮可接受）；排除自身 + 限 `label_type='auxiliary' AND status='active'`。注意 metrics.go 既有教训：pgvector 列在 GORM Scan 里要显式 `gorm:"column:embedding"` 标签映射（2026-10-07 回归：全标签 insufficient）。
10. 测试基建：repo_pg_test.go 是 testcontainer PG 模式（最近邻 SQL 用例放这）；run_test.go 有桩 LLM 注入先例（checkupOneBoard 包级 seam）；metrics_test.go 纯函数单测。
11. 红线（flow/semantic-board 红线 20/22）：体检零写匹配表/零扩充建议；口径唯一实现于 boardhealth 包（前端/别包不得复制公式）；boardhealth 不得 import board 包（expand 消费 boardhealth，D7）——cosineSim 在 metrics.go 是本地复制品，别去 import board.CosineSimilarity。

**引用**：backend-go/internal/tagmanagement/service/boardhealth/metrics.go、backend-go/internal/tagmanagement/service/boardhealth/llm.go、backend-go/internal/tagmanagement/service/boardhealth/run.go、backend-go/internal/tagmanagement/models/board_health.go、backend-go/internal/tagmanagement/handler/board_health_handler.go、backend-go/internal/tagmanagement/handler/board_crud_handler.go:removeBoardComposition

<!-- pinned 2026-10-28T06:57:44Z -->

## 三病实证数据与效果核对锚点

三病实证数据（2026-10-28 本地库实查，效果核对 tasks 5.4 与提示词调优的直接参照）：

**病A 磁铁污染**：分版块 direction_mismatch 率——生成式AI与大模型厂商 0-0.5% / 开发者工具与开源生态 0-0.5% / 中国国内新闻 0-1.7% / 美国新闻 4-17% / **全球科技巨头动态 23-50%**（weighted 23.4%、max_sim 46.9%、hit_rate 50%，direct_hit 也 12.5% 失败=composition 本身污染）。磁铁失败挂载样本：「Claude Opus 5.0」「GPT-6.1 Sol」「MINIMAX」「Anthropic…」全被吸到全球科技巨头，但这些是人眼判断「也算合理」的多归属（identity 更像生成式AI）——**归因时要靠「主要流向」区分边界重叠 vs 真污染**（design D2 风险缓解）。真污染样本：「Redis 作者 antirez 推出 ds4」「大模型训练显卡选型」。

**病B 短名天花板**：identity×board cosine 实测——OpenAI→全球科技巨头 0.57 / MySQL→开发工具 0.45 / 腾讯 0.53 / 亚马逊 0.47 / TypeScript 0.55 / 台积电 0.58 / Claude 0.58 / Rust 0.59，全部「显然该挂」却过不了 0.6 绝对阈值；线上 direct_hit 仅 3.0% 失败靠 GapExemptMargin=0.10 豁免兜底。健康对照：伊朗→中东地区 0.922。**效果核对的硬指标：短名标签误判🔴数量必须=0**（tasks 5.4）。

**病C description 污染**：L3 storage embedding=embed("label: description")（auxiliary_label_service.go:315-321），全库 15374 个 aux 100% 带 desc（p50=7字）。三种形态实证（pgvector 最近邻）——漂移：「上海」desc="上海知名商业综合体"→最近邻「举办地点」0.796/「小商品城」0.709；窄化：「中国媒体」desc="《环球时报》"→最近邻「人民日报」0.854 全是具体媒体机构；「中国大陆」desc="全球主要销售区域"→「地区范围」「新兴市场」。显性事件词根仅 1.8%（273个，正则可抓），**隐性视角污染是大头要靠最近邻证据让 LLM 判**。板块 composition 内污染标签分布：中国国内新闻 8 个 / 中东 7 个 / 生成式AI 4 个 / 卫生健康 3 个。

**效果核对锚点版块与期望分诊**（tasks 5.4）：中国国内新闻→「中国大陆」🟡「中国媒体」🟡「上海」🟡「中国央行」「全国人大」🟢（短名）；全球科技巨头→OpenAI/腾讯/字节类 🟢；中东→「德黑兰」low_sim_ratio=0.79 但 desc="伊朗首都"是定义式→🟢 非🔴（体检 metrics 现有数据里德黑兰 0.79、巴基斯坦 0.86 low_sim_ratio 就是防误杀试金石）。

**语义 vs 关键字基线结论**（完整数据见 docs/research/board-matching-evaluation/explore-findings.md）：纯文本匹配复现现状挂载上限 46.6-50.3%，现状 53.4% 挂载零共同标签纯靠 embedding；朴素子串匹配短词陷阱（Allegiant Air→AI、PICO→Pi、奥迪工厂→美元）连方向校验拦不住——这是三病分诊存在的根本理由，向用户解释 change 价值时用。

<!-- pinned 2026-10-28T06:58:03Z -->

## 对话脉络与新会话上下文索引

对话脉络与新会话上下文索引（2026-10-28 探索会话产出）：

**用户原始问题演进**：①「板块语义匹配和关键字匹配效果差距」→ 库实测（match_reason 分布、分版块方向失败率、人眼抽样）；②澄清基线应为「拿标签直接全文匹配」→ /tmp/boardmatch 离线实验 6 变体（B1 同名/B2 aux文本交集/B3 子串/B4 bigram）；③引出「板块评分只给笼统建议」+ 用户补充「短标签 description 污染」→ description 污染机制实证（storage embedding="label: description"）；④三病鉴别诊断设计（磁铁/短名天花板/description 污染）→ 用户拍板先开第一个 change=board-health-triage-advice，规划 artifacts 全部完成并 validate 通过。

**详细调研档案**（新会话可直读）：
- docs/research/board-matching-evaluation/explore-findings.md——含 3 条 pin：①语义vs关键字实测（五级规则链、挂载分布、分版块质量）②朴素基线实验（6 变体数据、短词陷阱、B1 同名误伤 35.9%）③board_health_snapshots 现状。SQL 查询模式与最近邻验证 SQL 都在里面。
- /tmp/boardmatch/{analyze.py,sample.py}——离线实验脚本（临时目录，可能已清；数据导出 CSV 的 \copy 命令在对话中可重建）。

**后续 change 候选**（proposal Out of Scope 已声明，用户已知）：①aux-label-description-hygiene（提取提示词百科式约束+重写/重嵌/回填执行链，🟡建议的消费端）②direction-check-shortname-band（短名分层阈值，依赖本 change 攒「确认保留」数据）。

**实现阶段注意事项**（对话中确认的约定）：磁铁信号口径=「失败子集 vs 全体」贴合差分组（不做功劳归因，红线22口径）；LLM 单调用产四项不加第二次调用（design D3）；摘除后不触发即时重算（红线20，前端标「已摘除等待下轮」）；提示词证据截断护栏 60 标签（design D6，先例 scorePromptCandidates=10）；效果核对必查「短名误判🔴=0」。

<!-- pinned 2026-10-28T06:58:16Z -->

## 原型阶段核实：完整体检依据已有 1040px 单级工作面

2026-10-28 原型阶段从当前磁盘源码核实：L1 右轨 RailHealthBrief 的 brief「完整评语与依据」走 openDetail；现有 BoardWideWorkface.vue 以 AppDialog size=xl（1040px、92vw 上限）承接，tab=health 渲染 RailHealthBrief variant=full，tab=quality 等按 mode 复用 BoardHealthDetail.vue。WORKFACE_TABS 的既有顺序为 context/health/quality/composites/candidates/sources/settings，不能新增独立分诊页签或再套抽屉。分诊应插入 full 的评语后与 Detail 摘要/质量区，brief 保持轻结论。当前 BoardWideWorkface 明确组件不发请求、写动作由宿主注入；后续实现摘除须遵守前端组件禁直接调 API 的边界，通过既有 composable/宿主动作或 emit 接通（RailHealthBrief 的当前 full 调用尚未传 boardId，不要只在组件里拼 DELETE）。用户要求先看原型，ui-design 保留 minor/not-required 合法 marker，但正文明确「待用户确认，确认前不实施」；该 marker 不代表已批准。

**引用**：front/app/features/tags/components/console/BoardWideWorkface.vue、front/app/features/tags/components/console/RailHealthBrief.vue、front/app/features/tags/components/console/railContext.ts:WORKFACE_TABS、openspec/changes/board-health-triage-advice/ui-design.md

<!-- pinned 2026-10-28T08:47:14Z -->

## 描述修复闭环的真实缺口：缺少已有向量改写接口，回填任务进度仅内存

用户在 FB-2 后明确选择「把描述修复也做成闭环」，允许先调整/拆分范围再出原型。只读核实：board_crud_handler.go:158 auxiliary-labels 只有 GET列表/clusters、POST merge-alias/backfill-embeddings/gc/:id/disable；front/app/api/auxiliaryLabels.ts 同样只有列表/聚类/禁用/合并，尚无描述编辑或污染向量重建接口。AuxiliaryLabelService.BackfillMissingEmbeddings 只查 embedding 或 merge_embedding 为 NULL，backfillRow 只写NULL列，禁止拿该接口重写已有污染向量或先清空向量曲线复用。storage embedding = label + ': ' + description，merge_embedding=label-only，因此手动改描述应重建storage embedding但不改label/aliases/merge_embedding。aux标签是全局共享记录，确认页需说明会影响所有引用它的版块，而非仅当前版块。board.SemanticBoardBackfillService 已有 all/unassigned/board 三种模式，all从全部active topic_tags取样，board模式是已有匹配+关联候选并集；job存在进程内map，服务重启后不可查/不可恢复。handler.rematchAll仅枚举现有匹配表tagID，会漏未归属tag，不可直接作为完整修复范围。board.InvalidateMatchCache 必须在更新aux向量后调用（board缓存无TTL）。描述修复是用户显式确认的独立写链，不能放进boardhealth纯读采集/GET；应为任务持久化/幂等/冲突保护/失败重试建立独立契约，不能靠前端本地「已修复」冒充后端完成。CodeGraph本轮超时，以上定位由已知目录定向read/rg核实，未解锁或改动索引。

**引用**：backend-go/internal/tagmanagement/handler/board_crud_handler.go:registerSemanticBoardRoutes、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:BackfillMissingEmbeddings、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:backfillRow、backend-go/internal/tagmanagement/service/board/semantic_board_backfill.go:SemanticBoardBackfillService、backend-go/internal/tagmanagement/handler/board_match_handler.go:rematchAll、backend-go/internal/tagmanagement/service/board/semantic_board_cache.go:InvalidateMatchCache、front/app/api/auxiliaryLabels.ts

<!-- pinned 2026-10-28T13:50:41Z -->

## FB-2 已扩围为描述修复闭环：取代建议-only/零DDL假设，major 原型仍待批准

2026-10-28 用户否定 v1「堆卡片、后续引导不足」，在 ask_user_question 明确选择「把描述修复也做成闭环」。当前同一 change 已扩围，proposal=ui-impact major；ui-design=major/pending，原型在 ui-prototype/index.html（v2）。这只是同意扩围，不是用户批准原型，产品代码与真库/部署均未动。原先探索中「只复制、后端零新增写接口、零DDL」假设已失效：新增 specs/auxiliary-label-description-repair/spec.md 与 design D7，要求手动共享影响确认、storage向量生成后描述+embedding原子保存、merge_embedding/label/aliases不动、持久任务幂等/冲突保护、cache失效、含未归属者的全活跃tag匹配、部分失败只续匹配、旧体检/日报不重写、效果待下轮验证。体检采集/GET仍纯读，修复写链独立，不允许boardhealth import board。v1 tasks/test-cases/data-map已在头部标旧基线暂停：原型获批后必须刷新计划/Review节/测试映射和新修复故事，不能照旧复制-only任务开工。v2交互已从四组列表改成紧凑问题索引+单项处理；任务开始后将编辑区/证据/共享说明收成只读折叠摘要，首屏聚焦三阶段、失败恢复、下一项；窄屏有粘性主动作，keep/insufficient与统计按需展开。原型42项机械断言绿只是模拟证据，不是实际事务/任务后端已实现。

**引用**：openspec/changes/board-health-triage-advice/proposal.md、openspec/changes/board-health-triage-advice/ui-design.md、openspec/changes/board-health-triage-advice/specs/auxiliary-label-description-repair/spec.md、openspec/changes/board-health-triage-advice/design.md:D7、openspec/changes/board-health-triage-advice/tasks.md、openspec/changes/board-health-triage-advice/ui-prototype/index.html

<!-- pinned 2026-10-28T14:17:30Z -->

## B2已交付接缝与B3现成embedding适配

B2/任务2.2已由minimax-cn/MiniMax-M3.1-Flash:max完成（三HTTP资源与B1三P2专项PG证据），实际接缝以seam-b2-api-handoff.md为准，旧seam-b2-api-checkpoint.md仅历史现场。HTTP wire数字ID保持旧兼容，父线程已裁定前端API normalizer转字符串view，无需全局改AuxiliaryLabel.id旧DTO。新薄service.NewService(db)(*Service,error)显式DB；nil不落全局；无goroutine/不执行embedding。facade导出AuxRepairService/Context/JobView/CreateRequest与NewAuxRepairService，独立handler RegisterAuxiliaryDescriptionRepairRoutes(rg,svc)，尚未生产注册。B3可直接复用auxlabel.DefaultAuxiliaryLabelEmbedder(ctx,input,AuxiliaryLabelEmbeddingModeStorage)：auxiliary_label_service.go:1082，内部airouter.NewRouter().Embed，Operation=tagmanagement.auxlabel_embedding，metadata.operation=auxiliary_label_storage_embedding，返回(pgvector字符串,raw[]float64,error)，该函数本身不改DB/维度。新描述修复只需单次storage embedding，不动merge_embedding/alias/label，不调用EnsureVectorDimensionOnce/全库向量修正。B2完整源码已在/tmp/board-health-b2-review-bKcdnuTb做不可变只读快照供并行review，B3写者不得反写该快照。

**引用**：openspec/changes/board-health-triage-advice/seam-b2-api-handoff.md、backend-go/internal/tagmanagement/service/auxrepair/auxrepair.go、backend-go/internal/tagmanagement/service/service.go、backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go:DefaultAuxiliaryLabelEmbedder、openspec/changes/board-health-triage-advice/implementation-contract.md

<!-- pinned 2026-10-29T03:55:35Z -->

## B3已交付入口、B4真实匹配适配与E2E数据隔离红线

B3/2.3交付见seam-b3-atomic-handoff.md；B2三R修复证据seam-b2-review-fixes.md，账本8/29。auxrepair.NewService(db, opts...Option)兼容旧构造；ClaimJobForExecution与ExecuteEmbeddingPhase(ctx,jobID,owner)内部owner入口，embedding默认复用既有auxlabel storage helper；Repo CommitDescription锁job/aux原子保存desc/storage与description_committed=true、stage/status matching、私有CommittedVersion/At。保存前MarkRepair*写守卫description_committed=false，B4匹配阶段需单独的true+owner/lease守卫，不误复用保存前失败函数。CodeGraph实查board/semantic_board_matching.go:26 NewSemanticBoardMatchingService(db *gorm.DB)可直接传显式非nilDB，不必改全局Repo；:107 MatchTopicTag(ctx,tagID)([]SemanticBoardMatchResult,error)是真实持久写入口，合法空命中会调用replaceTopicTagBoardLabels清除旧挂载并nil err，属于成功执行，不等于报错/未调用。包级board cache共享，匹配与每次恢复/重试前必须InvalidateMatchCache。B4父线程已在contract/spec明确not_applicable、remaining口径：冻结total不减，NA不当成功，P+F+NA+remaining=total，只有无F/remaining才完成；data-map/test-cases/UI摘要需对应补齐。
安全关键：这个修复任务冻结并重算所在DB【全部】active tags，不是只动当前aux/board。即使E2E创建一个带测试前缀的aux，执行也会改真库全部tag匹配；不能用命名空间样本当作真库隔离，更不能把批准实现等同批准自动修复现有上海等真实标签。真实交互故事须在一次性PG及对应API执行写链，必要让静态:5100页面的测试浏览器API网络隔离到该测试API；LLM效果以真库只读抽取锚点在隔离实例评估，不向真库写测试任务/重算匹配。B2-fixed/B3源码快照/tmp/board-health-b3-review-NfMkzs2r供并行只读复审，写者不得反写。

**引用**：openspec/changes/board-health-triage-advice/seam-b3-atomic-handoff.md、openspec/changes/board-health-triage-advice/seam-b2-review-fixes.md、backend-go/internal/tagmanagement/service/auxrepair/embedding.go、backend-go/internal/tagmanagement/repository/auxiliary_description_repair_stage.go、backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:NewSemanticBoardMatchingService、backend-go/internal/tagmanagement/service/board/semantic_board_matching.go:MatchTopicTag、openspec/changes/board-health-triage-advice/specs/auxiliary-label-description-repair/spec.md

<!-- pinned 2026-10-29T04:49:34Z -->

## B4完成与B5调度/重试接缝

账本9/29，2.3 B3原四项审查修复已恢复勾，见seam-b3-review-fixes.md：SQL租约用clock_timestamp，取得job/aux锁后重检及最终job标记含owner+phase_token+实时lease并检查RowsAffected；阶段AcquireRepairPhase原子领token，已有token返回busy不改运行者状态；claim换owner/过期接管作废旧token。已生成后等待aux锁跨过期/label写后过期导致commit marker拒绝都有PG回归；失败→成功清error、新旧storage不同且committedVersion等PG回读令牌。
B4任务2.4交付seam-b4-matching-handoff.md。ExecuteMatchingPhase(ctx,jobID,owner)用同一阶段CAS/显式DB默认board.NewSemanticBoardMatchingService；构造无执行。ListRepairScopeItems只pending/failed；GetScopeTagState复用activeTopicTagsPredicate；RecordMatchingItemResult同短事务写item并从items聚合counters；FinishMatchingPhase成功completed或matching_failed，结束释放claim/phase（围栏不踩别人）。合法零命中是实际执行成功；失效删除合并NA不调引擎/不算成功/total不减；remaining服务端提供；成功/NA不重复，失败新实例仅补失败；真实引擎direct_hit持久用例已跑。匹配在途崩溃允许至少一次幂等重放，不宣称exactly-once外部调用。
B5尚缺dispatcher/heartbeat/restart/retry。父线程已在contract/spec定下：自动只领pending及失租未结束阶段；明确failed等待用户retry，不能自动无限烧AI；worker绑应用ctx而非HTTP。retry空body或{}，非法/非空参数400；running/completed返回202同任务no-op不改时间/计数，conflict/unavailable409重新核对；失败/失租才原任务入队，不修改expected_version/new_description，已commit只续匹配。B3-fixed/B4代码快照/tmp/board-health-b4-review-mh4ZMUCw供只读复审，后续写者不能反写。真实E2E仍需一次性PG隔离，不能用测试aux前缀避开全局tag重算。

**引用**：openspec/changes/board-health-triage-advice/seam-b3-review-fixes.md、openspec/changes/board-health-triage-advice/seam-b4-matching-handoff.md、backend-go/internal/tagmanagement/repository/auxiliary_description_repair_stage.go、backend-go/internal/tagmanagement/repository/auxiliary_description_repair_matching.go、backend-go/internal/tagmanagement/service/auxrepair/matching.go、openspec/changes/board-health-triage-advice/implementation-contract.md

<!-- pinned 2026-10-29T05:34:32Z -->

## 后端B6闭环与前端3.1接入边界

正式账本11/29，backend tasks2.1-2.6全部源码交付但未启动/部署。当前接缝seam-b6-wiring-handoff.md、B5四修复证据seam-b5-review-fixes.md；原B3四R和B4已由独立db933复审确认，B5四修+B6再审未最终放行。tagmanagement/wire.go提供InitDescriptionRepairChain(db)显式初始化和同实例Install/Getter/Start(ctx)/Stop；routes追加四资源；cmd/server/main.go在DB ready后SetupRoutes前Init；app/runtime.go应用ctx启动worker，HTTP关闭后cancel/join repair早于其他workers/DB关闭。NewService/NewExecutor/注册都不自启，只有显式Start。独立复审快照/tmp/board-health-b6-review-62WReNwQ，不反写。
生产后端故事已在一次性PG真注册router+app worker+stub embed+真实matcher完成；matching_failed→POSTretry→队列自动只续失败且不再embed亦有证据，旧auxiliary routes保留。四API与raw字段以implementation-contract和实际auxrepair.JobView为准；raw HTTP ID为数字，前端API归一为string视图，不全局改AuxiliaryLabel.id旧DTO。processed仅实际成功执行（合法零命中亦成功）；not_applicable跳过不冒成功，remaining服务端给；旧wire缺remaining保持未知不能填0；完成只表示更新范围结束，旧快照/日报不改。
前端第一小批只3.1 API/normalizers/composable，不改组件/原型/宿主，不起dev或部署。triage只消费同轮snapshot，无独立分诊请求，不在前端复制病型公式；旧/空/非法/重复/越界引用应安全降级不可执行，不伪造诊断。确认payload冻结后同键重试只能同body；请求失败不清已知task，不把网络超时视后台失败；poll按taskID有界/终态停/卸载及切label忽略迟到响应。major v2 user已批准，不重新brainstorm，不处理其他change的memory_hint；真E2E写链必须一次性PG隔离，不能用真库测试aux前缀。

**引用**：openspec/changes/board-health-triage-advice/seam-b6-wiring-handoff.md、openspec/changes/board-health-triage-advice/seam-b5-review-fixes.md、openspec/changes/board-health-triage-advice/implementation-contract.md、backend-go/internal/tagmanagement/service/auxrepair/auxrepair.go:JobView、backend-go/internal/tagmanagement/wire.go、backend-go/internal/app/runtime.go、front/AGENTS.md

<!-- pinned 2026-10-29T09:54:04Z -->

## 体检触发链只有两个入口（日报主轮尾步 + CLI），回填与公开手动端点不触发

【体检触发链（2026-11-04 核实）】

入口只有 2 处（全仓 grep RunCheckup / RunBoardHealthCheckupWrapped）：
1) backend-go/internal/admin/scheduler/job_daily_report.go:57 —— 日报 job 主轮尾步，调用点 :411（`runBoardHealthCheckupTailStep(baseCtx)`），前置条件 :396 `len(boardIDs)>0` 且 :410 `failedCount==0`（任一板块生成失败 → 整轮体检不跑）。
2) backend-go/cmd/admin/main.go:89 —— CLI `admin board-checkup`。

**不触发的路径（关键）**：
- 日报回填扫尾（backfill sweep，job_daily_report.go:427 起）：只有主轮调体检，回填不调。
- 公开手动端点 POST /api/daily-reports/generate（internal/topicgraph/handler/daily_report_handler.go:110 → generateSingleBoard:287 / generateAllBoards:324 / generateSelectedBoards:412）：**不调体检**（该 handler 包内 0 处 boardhealth 引用）。
- job_daily_report.go:408 注释说的「Manual TriggerNowWithDate flows through DailyReportJob」指 scheduler wrapper 的强制触发（internal/admin/scheduler/job_daily_report.go:597），**不是**上面那个 HTTP 端点 —— 注释有误导性。

预算：job_daily_report.go:54-57 自带 `context.WithTimeout(parent, 10*time.Minute)`；超时板块只报 WARN「checkup failed ... context deadline exceeded」（2026-10-30 实例，backend-go/logs/app-2026-11-03T14-01-39.129.log.gz）。

实际运行记录：
- 2026-10-31 21:00 ~ 2026-11-02 21:10：日报调度整段被停（149+151+11 条「Daily Report: skipped: model_unhealthy」）→ 这三晚既无日报也无体检。
- 2026-11-03 21:00：日报 job 跑通（scheduler_tasks id=4 daily_report：last_execution_time=2026-11-03 21:21:48，backfill_window 2026-10-27~11-02，backfilled_count=29）→ 主轮产 10 份 11-03 日报 → 体检跑一轮 21:05:02~21:12:51（12 板 = 12 快照）。
- 2026-11-03 21:16~21:21 回填补出 11-01/11-02 日报 —— 用户所谓「这两天有日报生成」正是这批回填产物，而回填不触发体检。
- 11-04 白天无新快照（board_health_snapshots max(created_at)=2026-11-03 21:12:51），当前 app.log 0 条 board-health 行。

**引用**：backend-go/internal/admin/scheduler/job_daily_report.go:54-70、backend-go/internal/admin/scheduler/job_daily_report.go:396-417、backend-go/internal/topicgraph/handler/daily_report_handler.go:110、backend-go/cmd/admin/main.go:78-92

<!-- pinned 2026-11-04T03:48:26Z -->

## 根因：validateTriage 硬要求 1:1 覆盖 → 45+ 构成标签的大板整轮降级（triage/score/评语全丢）

【根因：triage 覆盖硬校验把「LLM 漏裁决几个标签」判成整轮非法】

代码链：
- backend-go/internal/tagmanagement/service/boardhealth/llm.go:334-337 `validateTriage`：`if len(triage) != len(labels) { return fmt.Errorf("triage covers %d items but board has %d composition labels", ...) }` —— 要求 **1:1 全覆盖**，差一个就整轮非法。
- llm.go:213-215 `adjudicateBoardHealth`：validateTriage 失败 → 返回 error。
- backend-go/internal/tagmanagement/service/boardhealth/run.go:86-97 `CheckupBoard`：llmErr → 只记 WARN「[board-health] board %d LLM adjudication degraded (score=NULL, no retry)」，**score / comment / triage 全部丢弃**（metrics.Triage 只在 result!=nil 分支赋值）。
- 结果：快照落库 llm_score=NULL、llm_comment=NULL、metrics.triage=JSON null。

实测（2026-11-03 21:05~21:12 那一轮，12 板；ai_call_logs 与 board_health_snapshots 按时间戳配对）：
| board | 构成标签数 | 模型实际给出 aux_id 数 | 结果 |
|---|---|---|---|
|3122/2165/246030/2242/2128/50413|7/13/24/27/30/41|7/13/24/27/30/41|全 OK|
|1974|47|45|降级|
|2197|50|40|降级|
|15220|54|51|降级|
|2272|58|48|降级|
|1980|96|≥68(片段封顶)|降级|
|3030|98|≥72(片段封顶)|降级|

**不是 token 截断**：completion tokens 1050~6456 远低于 max_tokens 默认 16384（airouter/openai_compatible.go:391），response_snippet 尾部是完整的 `]` + ``` 围栏；是本地小模型 `Qwen3.5-4B-UD-Q8_K_XL.gguf` 在 ~45+ 标签规模**漏裁决**。
日志原文（2026-10-30T22:31~22:33，backend-go/logs/app-2026-11-03T14-01-39.129.log.gz）：board 1980「triage covers 95 items but board has 96 composition labels」、2128「29 vs 30」、2197「46 vs 50」、2272「51 vs 58」。

DB 侧对照（只读）：board_health_snapshots 2026-11-03 12 条中 6 条 llm_score IS NULL 且 metrics->'triage' 为 JSON null；全库 triage 分布 = array 15 / null 15 / 无该键 300（300 条是 triage 上线前的旧快照）。

修复方向（待实现）：覆盖不全 ≠ 非法输出。缺失标签补 category=insufficient（spec.md:84「证据不足…下轮再判」语义），保留 score/comment/已覆盖 triage；只有非法 category / 未知 aux_id / 重复 / evidence 空 / rewrite 缺 suggestion 才整轮降级。同步要改 spec.md:77「LLM 返回非法时 SHALL 降级」与 implementation-contract.md:30 的措辞（后者本就允许「整个裁决降级或合法性处理」二选一，只需显式化）。

**引用**：backend-go/internal/tagmanagement/service/boardhealth/llm.go:334-364、backend-go/internal/tagmanagement/service/boardhealth/llm.go:99-127、backend-go/internal/tagmanagement/service/boardhealth/run.go:77-113、backend-go/internal/platform/airouter/openai_compatible.go:391

<!-- pinned 2026-11-04T03:48:32Z -->

## 前端与静态托管不是问题；降级板用户看到「本轮未生成评分与分诊/待评分」

【前端/契约/静态托管（已排除「看的是旧前端」）】

- 静态托管产物 backend-go/frontend/（构建 2026-11-03 13:29）**含新 UI**：_nuxt/QDgtuQFw.js 里 triage ×7、建议摘除 ×1、建议重写 ×2、待核对 ×3；源码 board-health/* mtime 2026-10-29 22:25 < 产物时间 → 产物是新的，不是旧版。
- 接口契约一致，无字段名错位：GET /api/semantic-boards/:id/health-checkup（backend-go/internal/tagmanagement/handler/board_crud_handler.go:151）→ `{data:{latest:{metrics:{labels,triage}, llm_score, llm_comment, checked_at}, trend}}`（snake_case 不改）；前端 app/api/normalizers/boardHealthTriage.ts:128-131 归一：`if (!Array.isArray(raw)) return { available:false, items:[], dropped:0 }`。
- **降级板用户实际看到的文案**：
  - 体检工作面：app/features/tags/components/console/board-health/boardHealthWorkspace.ts:281 `if (snapshot.llm_score===null && snapshot.llm_comment===null) return 'degraded'` → :306-310 标题「**本轮未生成评分与分诊**」，正文「统计与信号已保留，但本轮没有评分与分诊；不能据此推断当前标签健康。」；triiageAvailable=false 且 llm_score 非 null 时则是 :301「历史快照 / 这份快照没有分诊字段」。
  - 右轨常驻卡：app/features/tags/components/console/RailHealthBrief.vue:62 徽章「**待评分**」，评语取 snapshot.comment（降级板为空 → 空白）。
- 实测 API（curl --noproxy '*'）：board 2128 → llm_score=82、triage 30/30（依据在）；board 246030 → llm_score=78、triage 24/24（依据在）；board 1974 → llm_score=null、llm_comment=null、metrics.triage=null、labels 47（无依据）。
- 结论：前端与契约无缺陷；用户看不到依据是**后端把整轮裁决作废**造成的，不是渲染问题。

【change 并入状态】git merge-base --is-ancestor 85206187 HEAD → true（commit 8520618799fe65a67b067f00f70e3973f32856b9，2026-11-03 15:56:26，49 files/10489 insertions）；工作树干净。迁移表 auxiliary_description_repair_jobs 已存在。
未完成任务：tasks.md 3.5（桌面/窄屏可达）、5.3（静态 5100 agent-browser 故事）、**5.4（LLM 效果核对 —— 安全触发体检并抽检、短名误判 remove=0、真污染有证据）**、7.5（doc-impact/check-standards）。5.4 正是本次缺陷的漏网点。

**引用**：front/app/api/normalizers/boardHealthTriage.ts:128、front/app/features/tags/components/console/board-health/boardHealthWorkspace.ts:263-315、front/app/features/tags/components/console/RailHealthBrief.vue:62、backend-go/internal/tagmanagement/handler/board_crud_handler.go:151

<!-- pinned 2026-11-04T03:48:38Z -->
