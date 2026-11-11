# Review 记录 — add-event-story-heat

> 审查执行：2026-11-06，只读审查子线程（模型 `zai-coding-cn/glm-5.3:max`，档 `reviewer`）。
> 审查范围：proposal/design/data-map/tasks/test-cases/ui-design/specs delta + 本 change 全部实现 diff + 迁移 `20261105_0001`。
> 说明：`change-review` 档当日本机 codemode 沙箱不可用（`Code mode is unavailable: the sandbox did not answer within 10000 ms`），改用内置 `reviewer` 档执行，能力等价（只读 + bash）。

### R-1 2026-11-06【Medium】反向动作表精确匹配漏复合动词短语，反向事件会被阶段规则误并为「进展」
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/story_assign.go`（`IsReverseAction` + `classifyStoryCandidate` 判定顺序）
- **事实**：`IsReverseAction` 对归一化动作做精确查表；但提取侧 `action` 是自由动词短语（「宣布召回」「正式下架」「公开否认」）。于是「发布」对「宣布召回」精确查表不中 → 落入阶段规则 → 判 `SAME_STORY` + `is_development=true`。
- **依据**：`event-story` spec「事件归属判定顺序」第 3 层明文——`action` 语义方向相反时 SHALL NOT 套用本层，须改走第 5 层。
- **影响**：误合并是本 change 声明的最高代价错误（主指标即误合并率）。
- **处置**：精确查表未命中后加包含式兜底（一侧含词表反向词、另一侧含其配对反向词即判反向 → `verdictUndecidable` → 走 LLM 保守路径）；补 4 条测试（含「发布」vs「发布产品」不得反向的既有断言防回归）。

### R-2 2026-11-06【Low】跨期去重历史窗口实际 8 个日历日，比 spec「最近 7 天」多一天
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/daily_report_story_dedup.go`（历史窗口 `since/until` 计算）
- **事实**：半开区间 `[day-7, day+1)` 含 day-7…day 共 8 天；spec 与函数注释均为「最近 7 天（含当天，不含当前 report）」。
- **影响**：day-7 进过正文的重述也会被降级 flash，降级口径激进一天；只少进正文、不碰热度，危害有限。
- **处置**：`since` 改为按窗口常量推导的 7 天起点；补 day-6 参与 / day-7 不参与的边界测试。

### R-3 2026-11-06【Low】雷达 top-3 泳道选取仍按 thread_count_14d，与事件线密度口径不一致
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/repository/board_radar_repository.go`（`buildRadarRow` 截前 3 的排序键）
- **事实**：小矩阵与 title 已换 `storyCounts`（事件线数），但「前 3 条泳道」筛选键仍是线索数。
- **依据**：`workshop-radar` spec 行构成「该版块前 3 条泳道的 14 天事件线密度」。
- **影响**：线索多但事件线少的泳道可能挤掉真正事件线密度高的泳道（不影响 Attention 排序，只影响展示哪 3 条）。
- **处置**：排序键改为该泳道事件线数之和降序；补「线索多事件少 vs 线索少事件多」的截断断言。

### R-4 2026-11-06【Medium】矩阵图例未列 4 档区间（e2e 发现，spec 明文要求）
- **状态**：已修复
- **位置**：`front/app/features/tags/components/console/ObservationMatrix.vue`、`observationLanes.ts`
- **事实**：`[data-testid="matrix-legend"]` 仅「颜色 = 最热事件线的来源数」，无 0 / 1-2 / 3-5 / 6+ 区间。
- **依据**：`workshop-board-console` spec「矩阵格颜色 … 分档 SHALL 有图例」+ `story-heat-evidence`「分档 SHALL 有图例且 SHALL 提供带数字的 title/aria」。
- **处置**：抽出 `PARTICIPANT_BUCKET_EDGES` / `PARTICIPANT_TIER_LABELS` 同源常量，格子分档与图例文案由同一处推导（避免两处魔法数字漂移）；图例渲染四档区间。
- **复验**：通过。图例原文「数字 = 当日事件线数；颜色 = 最热事件线的来源数（0 / 1-2 / 3-5 / 6+）· '—' 无分区 · '?' 未取到」；交叉校验 `is-p1` 样本落 1-2 档、`is-p2` 样本落 3-5 档，无错档（`ui-verification/report.md` §5）。

### R-5 2026-11-06【Medium】雷达「今日该读」事件数用错口径且未标注未回填降级
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/repository/board_radar_repository.go`、`front/app/api/normalizers/boardRadar.ts`、`front/app/features/tags/components/console/BoardRadarReadingRail.vue`
- **事实**：① 渲染的 `report_event_count`（日报 event 标签数，示例 14）不是事件线数；overview 响应里已有 `today_stories`（事件线数，降级态即非综合稿事件上界，示例 9）。② 雷达 overview 响应**没有** `stories_backfilled` 字段（该字段只存在于 lane-dynamics），导致降级标注无法由前端单独实现。
- **依据**：`workshop-radar` spec「『事件数』SHALL 为当日去重后的事件线数；未回填时 SHALL 按非综合稿事件数作上界并标注降级，MUST NOT 用 section 数代替」。
- **处置**：后端 `RadarBoardRow` 补 `stories_backfilled`（从已取到的 `dynamics` 赋值，零新查询）+ 仓储/handler 断言；前端 normalizer 补三态（true/false/null 未知），rail 数字改用 `today_stories`，仅在 `false` 时加「（未回填上界）」，字段缺失回落 `report_event_count` 且不标注。
- **复验**：通过。首条实测「11/05 · 14 篇文章 · **9 个事件（未回填上界）**」，其余版块同规律（`ui-verification/report.md` §5）。

### R-6 2026-11-06【Low】e2e「降级态矩阵颜色应全落最低档」为误报
- **状态**：误报丢弃
- **事实**：e2e 验收前提写「未回填时热度颜色全落最低档」，实测存在 `is-p2` 与「最热 4 个来源」。
- **依据**：`event-story` spec「未回填时 … 参与者按 feed 去重计算」——降级口径是**每条事件自身 distinct feed 取最大**（不是当天并集、也不是恒 0），颜色随样本自然分档。
- **处置**：非缺陷；已在 `ui-verification/report.md` 记录为误报并写明实际口径。

### R-7 2026-11-06【High】`archived` 语义判断错误，热度证据被砍 86%（真库实测发现）
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/repository/story_signals_repository.go`（signals 重建 + 参与者计数）、`story_heat.go`（热度读取）、`story_repository.go` 的 `ArticleFeeds`（共 4 处）
- **事实**：`articles.archived` 是**保留期轮转标记**（`models/article.go`："aged out of the feed's active window (MaxArticles)"），归档行保留全部文本字段，只从阅读列表/搜索/统计剔除。原实现把 `archived=false` 当作「可见证据」过滤，而生产库 60,853 篇文章中 58,623 篇（96%）为 `archived=true`；回填窗口 10-30~11-05 内 7,204 篇中 6,224 篇（86%）。
- **依据**：`tagmanagement/service/sourcestats/sourcestats.go` 包注释第 2 条为项目既有红线——window queries MUST NOT filter on `archived`；高频源几乎全部命中落在归档区，过滤会反转结论。
- **影响（实测）**：回填后 `board_stories=787`、已归属线程 790，但 `story_signals` 仅 **141** 行、`participant_count` max=1 / avg=0.179，`story_heat` 与 `lane_heat` 全 0 → 矩阵颜色分档、雷达 attention/排序/升温判定、5.3 阈值标定全部失真。
- **处置**：4 处过滤全部移除（注释引用 `sourcestats` 红线，并写明「将来排除撤回/恶意须新增专用列，不得复用 archived」）；补 `backfill-stories --rebuild-signals` 运维入口（幂等、按版块/日期限定、`--dry-run`、不改归属、复用 `RecomputeSignalsForStory`）；新增「归档文章仍是证据」的强断言测试。
- **重算验证**：`story_signals` 141 → **802**（覆盖 787 条线）、`participant_count` max 1 → **3** / avg 0.179 → **1.011**；再跑一次 802 → **802**（幂等）；`lane_heat` board 1974 变为 `[1.525, 1.271, 0.927, 0, 0, 0]`、雷达 `attention` max **18.4**、`today_heat` max **20.6**。
- **连带清理**：`docs/reference/flow/daily-report.md`、`docs/reference/database/tables/daily-report-watch.md`、`daily_report_story_dedup.go` 注释、`ArticleFeeds` 注释中「只算 archived=false」的失真表述同步订正。
- **遗留（不属本 change 辖区，仅登记）**：`repository/topic_watch_repository.go` 326 / 403 / 433 三处日期窗口扫描仍过滤 `archived`（影响关键词物化、事件标签命中、句子轨道文章池），建议后续 change 处理。

### R-8 2026-11-06【High】LLM 分类器未强制 `candidate_id`：405/408 次「模型说相关」却无地址可挂
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/story_assign_llm.go`（JSON schema `required` + 解析校验）、`story_assign.go`（降级分支）
- **事实**：schema 声明了 `candidate_id` 但 `required` 只有 `decision`/`confidence`；缺 ID 反序列化为 0 → 视为无效候选 → 报错 → 降级新建。实测 713 次分类中 408 次判 SAME_*，**405 次缺 ID**，仅 3 次可挂载，恰好等于实际 3 次 attach（attach 率 0.4%）。
- **影响**：几乎每条事件都新建线，事件线身份退化为「一线一事件」，热度/矩阵/雷达全部建立在错误的身份之上；这是效果验收第 3 项（误合并率）之外的另一主指标（误拆分）的直接成因。
- **处置**：schema `required` 加入 `candidate_id`；解析后做决策一致性校验（`SAME_*` 必须命中实际候选 ID，否则无效并走既有降级）；`UNRELATED`/`ROUNDUP` 允许 0；prompt 明示「SAME_* 必须给候选 ID」。**不采用「默认取第一个候选」这类兜底**（会制造误合并）。

### R-9 2026-11-06【High】降级分类的候选只有日期，模型无法判断（与 R-8 必须同修）
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/story_assign_llm.go`（prompt 渲染）、`story_repository.go`（候选查询列）
- **事实**：`StoryCandidate` 不含标题/摘要，prompt 里候选是「id + 空 subject/key/action/object + 日期」。模型在 UNRELATED 理由里明确写「候选只有日期，缺失关键信息」。
- **影响**：只修 R-8 会把「因解析失败被挡下的盲判」变成真实误合并（实测已有 2/3 误合并）。
- **处置**：候选增加代表证据（最近一条线程的 title/summary），prompt 渲染标题+摘要并在指纹缺失时显式标注「（指纹缺失：不可信）」而非空白；候选数 ≤10、摘要截断并加总长守卫。

### R-10 2026-11-06【Medium】历史 event 指纹全库为 0，五层规则一条未生效（转后续 change）
- **状态**：已驳回（转挂后续 change：历史指纹回填）
- **事实**：全库 9,439 个 event 标签中 `fingerprint_trusted=true` 为 **0**、false 586、未记录 8,853，`metadata.event_fingerprint` 全部缺失。故 958 条线程全部走 LLM 降级路径；10 组疑似拆分样本归因 `fingerprint_untrusted` 10/10，其余类别（recall_empty / subject_veto / object_veto / composite_skip）均为 0。
- **影响**：规则层（前四层）在历史数据上等同未启用；判定成本与误判风险全部压在 LLM 单点。
- **处置**：本 change 不承担历史数据重打标（需对 9,439 个 event 标签重跑 LLM 提取，成本高且属数据治理范畴）。已在 tasks/review 登记，另开 change 处理；新产生的事件标签会正常携带指纹。

### R-11 2026-11-06【Medium】168 条实际综合稿的 `scope` 仍为 `single`（转后续 change）
- **状态**：已驳回（转挂后续 change：scope 历史回填）
- **事实**：身份拦截正确（`scope='composite' AND story_id IS NOT NULL` 为 0；实际 ≥2 个 event 标签的 168 条也全部未归属），但存储列 `scope` 未回填成 `composite`，违反「scope 由 `tag_ids` 中 event 标签数推导」的契约。
- **影响**：读取侧用 `scope` 做过滤的地方对历史行判断失真；后续按 scope 统计会偏差。
- **处置**：需一次性回填脚本（按 `tag_ids` 中 `category='event'` 数量重算历史行 scope）。本 change 只保证新写入正确；回填另开 change 并需在低峰期执行。

### R-12 2026-11-06【High】LLM 阶段合并导致误合并 44%（主指标 ≤5% 未达标）
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/story_assign_llm.go`（决策枚举与 prompt）、`story_assign.go`（LLM 层处置）
- **事实**：判定链修复并重跑后，事件线 787 → 599、attach 率 0.38% → 24.18%，但多线程线从 3 条涨到 **134 条**（2 线程 95 / 3 线程 24 / 4 线程 12 / 5 线程 3）。全量人工复核：**确定误合并 59/134 = 44.03%**，存疑 38/134 = 28.36%，暂判正确 37/134 = 27.61%——即使存疑全部改判正确，也远超 spec 阈值 ≤5%。
- **归因**：134 条中 **125 条来自 LLM 路径**、9 条来自规则路径；LLM `confidence` 近似常数（57 次 0.9 / 9 次 0.8 / 7 次 0.95），**阈值调节无效**。历史 event 指纹全缺（见 R-10）→ 规则层失效 → 全部判定压在 LLM，而 `SAME_STORY`（阶段合并）在无可信指纹约束下把「同板块同主题」误判为「同一事件线的不同阶段」。
- **处置（用户裁定）**：LLM 层**只保留 `SAME_OCCURRENCE`**，schema `enum` 移除 `SAME_STORY`，prompt 明确「只有确认**同一次发生**才可合并；同主题不同次发生一律 UNRELATED」；`SAME_STORY` 若被模型幻觉返回则按**不合并**处理并记 warn（不得降级成 SAME_OCCURRENCE）。规则层前四层（含指纹可信时的 stage 规则）保持不变——阶段合并只能由可信指纹承担。
- **取舍说明**：这会降低 attach 率（从 24% 回落），但符合 design §9「误合并比误拆分代价高，主指标是误合并率」。指纹回填（R-10）完成后可再评估是否放开阶段合并。

### R-13 2026-11-06【High】显式 `subject_key` 被 embedding 层改写，违反 spec 一票否决
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/story_subject_normalize.go`（`ResolveSubjectKey`）
- **事实**：真库事件线 3047 把「官员披露对伊动武计划」与「特朗普称伊朗问题将很快结束」合并。两者 `subject_key` 分别是 `person:特朗普政府` 与 `person:美国总统特朗普`，**双方都非空且不同**。`ai_call_logs` 2692799 的 prompt 显示：进入分类前输入键已被改写成候选键 `person:特朗普政府`。
- **依据**：`event-story` spec「`subject_key` 不同 → SHALL 判为不同事件；该判定 SHALL NOT 被向量相似度或 `object` 相近推翻」。
- **处置**：改为**显式 key 优先**——双方 key 都非空时只允许确定性归一化后精确相等，不等即否决；embedding/字面层仅在一方缺 key 时兜底。跨语言别名归并能力保留在缺 key 场景。

### R-14 2026-11-06【High】对象「包含成立但严格不等」被下放 LLM，导致跨对象合并
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/story_assign.go`（`classifyStoryCandidate`）
- **事实**：`伊朗` 与 `伊朗问题将很快结束` 因双向包含被判「不确定」→ 交 LLM → LLM 判 `SAME_OCCURRENCE 0.9` 合并。
- **依据**：spec 第 4 层「`subject_key` 相同、`object` 不同 → 判为不同事件」。
- **处置**：对象严格不等 → 直接 `object_veto` 新建，**不再下放 LLM**；宽匹配只用于「判不同」，不用于「判相同」。

### R-15 2026-11-06【High】指纹不可信时 LLM 兜底合并，是残余误合并的唯一来源
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/service/story_assign.go`（合并前置闸门）
- **事实**：第 6 轮 6 条跨文线中 4 条误合并，**全部**发生于「一侧或双方指纹缺失/不可信 → 交 LLM」路径；指纹可信的 2 条全部判对。
- **处置**：加合并前置闸门——**输入指纹不可信或任一侧 `subject_key` 为空 → 一律新建，不调用 LLM**（记为 `merge_gate_untrusted`）；LLM 彻底退出合并路径，仅保留综述稿（ROUNDUP）判定。第 7 轮实测：跨文线 6 → 1，误合并 4/6 → **0/1**；按 spec 口径全部多线程线 **0/41 = 0%**。
- **取舍**：attach 率 7.85% → **5.82%**，事件线更接近「一事件一线」，跨天聚合减少。符合 design §9「误合并代价高于误拆分」。

### R-16 2026-11-06【Medium】效果指标口径错误：同一篇文章切出的多段线程被计入「误合并」分母
- **状态**：已采纳
- **事实**：初测把「一条事件线挂了多段线程」全部当作待检样本（59 条），其中 **51 条两段线程来自同一篇文章**——同文切分不可能构成「两件不同的事」，属正常结果。
- **处置**：口径改为分层统计——`same_article` 不计入误合并；真正检验归线的样本为 `diff_article`（第 7 轮 1 条）。同时保留 spec 原始口径（全部多线程线）作为对照：0/41。后续任何复测 MUST 先做此分层，否则会得出假高比例。
- **沉淀**：见 `docs/reference/flow/daily-report.md` 的验收口径说明。

### 记录在案的实现偏离（非审查问题，随 change 归档）
1. **无 groupReview 多 provider**：airouter 只按 Capability 路由，合并降级为单分类器 `confidence ≥ 0.75` 保守处理（对应 tasks 3.3）。
2. **跨期去重时序**：不在 orchestrator 组装 section 前判定，而在 `attributeStoriesSafe` 归属完成后 detached 执行（story_id 归属后才存在，对应 tasks 6.3）。
3. **「人工排除」无数据载体**：仓库无 `participation_mode` / 撤回列；「人工排除」能力本期未提供，仅 `story_manual` 覆盖人工指派/拆分。
   > **订正（2026-11-06，见 R-7）**：原表述为「排除证据只用 `articles.archived`」，该口径**已被证伪并推翻**——`archived` 是保留期轮转标记（`models/article.go`："aged out of the feed's active window"），不是可见性/撤回标记，不得用于排除证据。证据**不按 `archived` 过滤**（红线见 `sourcestats` 包注释第 2 条）。
4. **雷达阈值 10/3 为临时值**：待真库回填后按 `lane_heat` 的 P80/P20 标定（tasks 5.3），标定结果尚未回填到常量。
5. **未回填降级态**：本次只对真库执行 `--dry-run`（13 版块 / 73 日报 / 958 线程：790 将新建、168 跳过），未写库，故 `story_signals` 为空、`lane_heat` 全 0。
