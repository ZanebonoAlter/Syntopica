# Review：retire-story-adopt-event-metrics

> 审查方式：派只读 review 档子线程聚焦代码审查（tasks 6.1）——主审 `git diff 96f83315..a9be18b3`（= 后端 `0010aa50` + 前端收尾 `a9be18b3`，102 文件，+5269/−5933），对照 9 份 delta + proposal/design/tasks/test-cases 逐项核查。
> 审查范围：两 commit 的全部实现 diff + change 制品；重点 12 项（tasks 6.1 三处 + 实现线程自留 5 关注点 + 抽查 4 项）。
> 未审：基线 `96f83315`（存量收编提交，按任务书排除）；`cmd/calibrate-heat` 等未改动文件。

### R-1 2026-11-11【High】停写开关默认 true 与 spec/红线「新日报 story_id 恒 NULL」矛盾，部署清单漏了「关闭开关」动作
- **状态**：已修复（commit `e8aba010`）
- **处置**：用户 2026-11-11 拍板选「删开关改硬停写」（非 review 建议的文档级修复）：删除 `repository/story_attribution_settings.go` 与 `ai_settings.story_attribution_enabled`，`boundedReportAttribution` 及其在日报尾段的调用一并摘除，归属判定函数库与写端实现保留；测试改 `TestStoryAttributionHardStop`（硬停写下日报尾段恒零 story 写入）；proposal（What Changes + 部署节：回滚 = `git revert`）/ design D1/D2/Migration / `event-story` delta Migration / `flow/daily-report.md` 红线 43 / `flow/topic-graph.md` 红线 14 / `configuration.md` 同步删除或改写；矛盾面（制品说停写、默认行为继续写）随开关删除消失。
- **位置**：`specs/daily-report-system/spec.md`（ADDED「日报数据模型」：「新生成或重建的日报 SHALL 恒为 NULL，归属判定链 SHALL NOT 再写入该列」）；`docs/reference/flow/daily-report.md` 红线 42 同句式；`backend-go/internal/topicgraph/repository/story_attribution_settings.go:23`（`DefaultStoryAttributionEnabled = true`）；`backend-go/internal/topicgraph/service/daily_report_story_highlights_test.go:70`（开关开=照常进入归属判定并写 story_id）；`proposal.md`「部署后影响与需要的操作」（只列候选权重一项，无停写开关动作）。
- **问题**：实现按 design D2 默认开（灰度兼容，回滚=删键），开关开启期间 `boundedReportAttribution → AttachThreadToBoardStory` 会给**新日报**写 `story_id`（含 `is_development`）。而 delta 与红线 42 无条件断言「新日报恒 NULL、归属判定链 SHALL NOT 再写入」；proposal 的部署清单又没有把「关闭 `story_attribution_enabled`」列为上线后必做动作。三份制品拼起来的净效果：按文档原样部署且无人操作时，系统**继续写 story**（读端已全部退役，写入变无人消费的僵尸写入），归档进主 spec 的却是「已停写」契约——未来 Agent 与审计都会按「不再写入」理解，与默认行为直接冲突。
- **修复建议**（文档级三选一，建议 ①，需主会话拍板）：① proposal「部署后影响与需要的操作」补一条「上线后需将 `story_attribution_enabled` 置 false（design 上线顺序 ④）」，红线 42/delta 加「停写开关关闭后」限定语；② 仅把 delta/红线 42 的 SHALL 限定为开关关闭后的稳态；③ 改默认值为 false（放弃灰度，需用户确认）。至少消除「制品宣称停写、默认行为继续写」的矛盾再归档。

### R-2 2026-11-11【Medium】`SaveReport` 遗留派生计数重算未挂停写开关——独立验证属实，建议一并加开关
- **状态**：已修复（随 R-1 硬停写）
- **处置**：`SaveReport` 不再收集 `affectedStoryIDs`、不再在提交后重算 `story_signals` / `participant_count`；路径整体摘除（不是加开关），重跑上线前历史报也不得改写已退役的派生存量。新增 `TestDailyReportRerunKeepsRetiredStoryData`（隔离 PG：重跑同日报后证据行仍在、`participant_count` 保持原值）钉住该语义；tasks 5.3 注记同步。
- **位置**：`backend-go/internal/topicgraph/repository/daily_report_repository.go:403-411`（提交后对 `affectedStoryIDs` 循环 `RecomputeSignalsForStory` + `recomputeBoardStoryCountsWith`）；`story_signals_repository.go:49/73/94`（DELETE+INSERT `story_signals`、UPDATE `board_stories.participant_count`）。
- **问题**：tasks 5.3 的判定**核实为真**：全仓独立 grep 确认唯一非测试非开关的写命中就是此处（在线入口只有 `boundedReportAttribution`（开关拦）、`story_reattribute.go`/`story_backfill.go` 只被 CLI 调（写模式拒绝））。但它重跑「开关上线前的历史报」时会真实改写 `story_signals` 与 `board_stories` 计数，与 proposal「`board_stories` / `story_signals` …… 只是不再写入」字面冲突；虽无建线/挂线/合并能力、旧线读端已无消费方（影响不可见），仍是停写语义的一条旁路，且「旧数据只读保留」承诺被这一路径打破。
- **修复建议**：循环前置 `if !r.StoryAttributionEnabled() { 跳过并留日志 }`（一行 + 一个重跑历史报的测试）；若主会话选择不改代码，需在本条记录豁免理由（历史派生数据维护、无身份能力、无读消费方）后带说明归档。

### R-3 2026-11-11【Medium】新 AI operation 未登记 `ai-logging.md` 功能清单（MUST 级登记表）
- **状态**：已修复
- **处置**：`ai-logging.md` 「已接入 AI 调用的功能清单」补一行：功能=日报-同事件聚簇、operation=`topicgraph.daily_report.event_cluster.embedding`、经 airouter ✅、记 prompt N/A、记 token ✅、session_id N/A、状态=已接入（**不在 embedding 缓存白名单**，每次都是真实调用行；RequestMeta 带 board_id/report_id/batch_size）。
- **位置**：`docs/reference/standard/backend/ai-logging.md`「已接入 AI 调用的功能清单」表（规范 Scenario：新增 AI 调用 SHALL 新增一行）；新 operation `topicgraph.daily_report.event_cluster.embedding`（`backend-go/internal/topicgraph/service/daily_report_event_cluster.go:58`）。
- **问题**：本 change 新增了唯一的 AI 调用面（日报聚簇 embedding），六处文档更新（tasks 7.1，doc-impact 声明 flow/api/database/configuration 四轴）没有覆盖 `standard/backend/ai-logging.md`，清单表缺行。审计/排障按表索骥会找不到这个每日每板一次的 embedding 调用。
- **修复建议**：补一行：功能=日报-同事件聚簇、operation=`topicgraph.daily_report.event_cluster.embedding`、经 airouter ✅、记 prompt N/A（embedding 惯例，RequestMeta 带 board_id/report_id/batch_size）、记 token ✅、session_id N/A、状态=已接入（**不在 embedding 缓存白名单**，每次都是真实调用行）。

### R-4 2026-11-11【Medium】「不删代码」制品承诺与读端删除的事实偏差未留痕（解读本身可接受、回滚成立）
- **状态**：已修复
- **处置**：在 design D1 补「读端删除留痕」：承诺范围 = 实现库与写端，读端死代码（`board_radar_stories.go` + 其单测 + 2 个 story 身份集成测试）属「事件线清单段退场」的字段退场连带删除；Non-Goals 措辞收窄为「不删归属判定的实现库与写端实现（只停在线调用）」，proposal Impact 同步。回滚仍由 `git revert` 一并恢复。
- **位置**：删除件 `backend-go/internal/topicgraph/repository/board_radar_stories.go`（297 行）+ `board_radar_stories_test.go`（764 行）+ `report_story_highlights_integration_test.go` + `daily_report_story_highlights_integration_test.go`；对照 `design.md` Non-Goals「不删任何实现代码（只停写）」、proposal「story 停写（不删代码不删表）」。
- **问题**：实现把「不删代码」解读为「写端实现保留，读端死代码随字段退场删除」。独立验证：**写端完整保留**（`story_attribute.go`/`story_assign.go`/`story_reattribute.go`/`story_backfill.go`/`story_repository.go`/`story_signals_repository.go`/`story_subject_normalize.go` 全在，`story_attribution_enabled=true` 即恢复在线生产）；**回滚路径成立**（开关=写端恢复 + git revert=读端/前端口径恢复，与 design Migration/Rollback 文档一致）；读端删除与 delta「总览端点事件线清单段退场」一致、与「读端不再透出事件线概念」方向一致。解读可接受，但制品字面（Non-Goals「不删任何实现代码」）与事实矛盾且无解释，归档后对照会认为是越权删除。
- **修复建议**：在 design D1（或 proposal Impact）补一句：「读端死代码随字段退场删除（board_radar_stories.go 与 2 个 story 身份集成测试），写端实现与表全部保留」；test-cases 的「文件删除或改写并留痕」已覆盖测试侧。

### R-5 2026-11-11【Medium】聚簇主路径（真 embedding 向量聚簇）质量从未实测——建议归档后首个真实日报日补核
- **状态**：已采纳（不阻断归档，归档后补核）
- **处置**：归档后首个真实日报日核对一期 `cluster_mode=向量聚簇`（Tier 1）的簇质量：抽 2-3 簇人工判「是否值得一起看」+ 统计成簇率，回填 `test-cases.md` S2「效果核对」；观察窗口内的结论不反过来改本期契约。
- **位置**：`test-cases.md` S2「效果核对」（口径如实标注为降级链第 2 级「语义向量兜底」）；`daily_report_event_cluster.go`（主路径 Tier 1）。
- **问题**：5.2 的复算因「真库只读、避免写 ai_call_logs」走语义向量兜底（理由成立、口径标注诚实），但主路径（related-event-v1 表示 + 真 embedding + 0.86 单链）从未在真数据上跑过——Tier 1 的表示与 Tier 2 的 semantic 表示不同空间，Tier 2 的「方向正确」不能外推 Tier 1。上线后所有正常日报走的都是未实测的 Tier 1。
- **修复建议**：归档不阻断；在 tasks 或本 review 留「归档后首个真实日报日核对一期 `cluster_mode=向量聚簇` 的簇质量（抽 2-3 簇人工判『是否值得一起看』+ 成簇率），回填 test-cases S2」的跟进项。

### R-6 2026-11-11【Medium】聚簇覆盖率 17.6% → 多数日子「今日重点」候选退化为单例簇 Top 8——交用户拍板
- **状态**：已采纳（用户拍板维持 0.86 观察）
- **处置**：用户 2026-11-11 拍板**维持 `related_event_similarity_threshold` = 0.86**，下调到 0.80-0.84 的灰度方案不执行；归档后按 R-5 补核真实成簇率（观察 1-2 周）再评估是否重开此条，「宁可少聚不乱聚」为当前取向。
- **位置**：`test-cases.md` S2 效果核对（15220 板 17 标签 1 簇+14 单例；2197 板 0 簇）；`daily_report_event_cluster.go:RankEventClusterCandidates`（Top 8 按簇内文章数截断）。
- **问题**：Top-8 截断发生在 LLM 选择**之前**且纯按簇内文章数排序——低篇数高新闻价值的事件进不了候选白名单，LLM 再有选题判断也救不回池外的条目。「重点=LLM 语义选题」的语义仍在（未完全条数榜化），但候选池在无簇日子实质是「当天文章数 Top 8 标签」。这是产品口味而非缺陷。
- **修复建议**：交用户拍板：① 接受（0.86 沿用 related-events 同源阈值，「宁可少聚不乱聚」是既定取向）；② 灰度下调 `related_event_similarity_threshold`（在线可调）到 0.80-0.84 观察成簇率变化后再定。无论哪种，建议连续观察 1-2 周成簇率再定论（17.6% 是单日两板样本）。

### R-7 2026-11-11【Low】D16/红线 47/tasks 2.1 声称 embedding「成功另记 prompt 与 response_snippet」，与 airouter 实现不符
- **状态**：已修复
- **处置**：`design.md` D16、`flow/daily-report.md` 红线 47、`tasks.md` 2.1、`specs/related-events/spec.md` 四处措辞改为如实：「embedding 路径按仓内 N/A 惯例不记 prompt / response_snippet，token/latency 成败均记，Metadata 带 board_id/report_id/batch_size；截断保护只适用 chat 路径」。
- **位置**：`design.md` D16；`docs/reference/flow/daily-report.md` 红线 47；`tasks.md` 2.1；对照 `backend-go/internal/platform/airouter/router.go:760-785`（embedding LogCall 只有 operation/capability/token/latency/meta，无 Prompt/ResponseSnippet——chat 路径 277/351 才有）。
- **问题**：截断常量（10000/20000 runes）与成败双写都在（chat 路径），但 embedding 调用从不记 prompt/response_snippet——这与仓内清单表全部 embedding 行「记 prompt N/A」的惯例一致，实现无错；错的是本 change 制品把它写成 embedding 也记。照 D16 去审计 prompt 会找不到。
- **修复建议**：D16/红线 47/tasks 2.1 措辞改为「embedding 按仓内 N/A 惯例不记 prompt，RequestMeta 带 board_id/report_id/batch_size，token/latency 成败均记」。

### R-8 2026-11-11【Low】分支表 B「聚簇总预算超限（>500 截断并标注）」无测试覆盖
- **状态**：已修复
- **处置**：新增 `TestDailyReportEventClusterTruncatesOverBudget`（`backend-go/internal/topicgraph/service/daily_report_event_cluster_test.go`）：500 个高频标签（各 2 条线索）+ 1 个低频标签，断言 `Truncated=true`、`TagCount=500`、低频标签不进候选、`Mode=向量聚簇` 且函数返回结果而非错误（截断不阻断日报）；`test-cases.md` 分支表 B 该行标注落点。
- **位置**：`test-cases.md` 分支表 B「聚簇总预算超限」行；`daily_report_event_cluster.go:141-152`（截断逻辑）；`daily_report_event_cluster_test.go:227`（仅断言 `set.Truncated=false` 的非截断侧）。
- **修复建议**：补一个 >500 标签的截断单测（断言 Truncated=true、按命中数取前 N、截断只影响候选不阻断），或在此留痕接受纯代码审查结论（截断逻辑简单、同文件 BatchSplits 已验拆批纪律）。

### R-9 2026-11-11【Low】tasks 2.1 用例清单写「审计落库」，实际测试用 stub 绕过 airouter 未断言 ai_call_logs
- **状态**：已修复（措辞对齐，测试不改）
- **处置**：`tasks.md` 2.1 的用例清单删去「审计落库」，改为注记「审计保障在 airouter 共享测试（stub 绕过 airouter，本文件不重复断言 `ai_call_logs`）」；与 R-7 的措辞一并对齐。
- **位置**：`tasks.md` 2.1（用例清单含「审计落库」）；`daily_report_event_cluster_test.go`（`dailyReportClusterEmbed` stub，仅断言 operation 非空/批量/元数据）。
- **问题**：审计落库由 airouter 共享路径保证（`embed_cache_router_test.go:78/201/234` 已断言 embed 的 LogCall），stub 单测无法也无需重复验证；但任务文本的计划用例与实际覆盖面有出入，PASS 注记的用例列表已如实不含该项，属措辞残留。
- **修复建议**：无需改测试；在 R-3 补登记行时顺带把 tasks 2.1 该措辞与 R-7 一并对齐即可。

### R-10 2026-11-11【Low】前端旧响应 fallback 口径含综合稿（仅兼容路径，已注释声明）
- **状态**：已采纳
- **处置**：已带说明接受——仅旧后端（无 `clue_count`）时走 `threadCount ?? fallbackClueCount` 回退，注释已声明是旧响应兼容路径；新后端恒走权威 `clue_count`（排除综合稿）。前端本期不动（review 范围内不在 diff 中改动）。
- **位置**：`front/app/api/normalizers/laneDynamicsWindow.ts:165-180`（`dayCellMetrics` 旧响应回退 `threadCount ?? fallbackClueCount`，两者均含综合稿；权威 `clue_count` 排除综合稿）。
- **问题**：仅当旧后端（无 `clue_count`）时走 fallback，此时口径略偏大；注释已声明是旧响应兼容回退，新后端永远走权威值。风险极低。
- **修复建议**：接受留痕；若要绝对一致可在 fallback 注释中补「含综合稿，仅旧响应」字样。

---

## 逐项审查结论（tasks 6.1 重点 12 项，无发现的显式声明）

| # | 项 | 结论 |
| --- | --- | --- |
| 1 | 停写旁路写入口 | **独立验证属实**：全仓 grep `CreateBoardStory/AttachThreadToBoardStory/MergeBoardStories/RecomputeSignalsForStory/RecomputeBoardStoryCounts` 非测试命中仅 ① `boundedReportAttribution`（开关拦，三态已测）② `story_reattribute.go`/`story_backfill.go`（仅 CLI，写模式 9 种全拒绝、默认无参模式也拒绝、`--dry-run`/只读保留、`--restore` 连 dry-run 也拒）③ `daily_report_repository.go:406`（→R-2）。无第二在线入口。 |
| 2 | 前后端是否各写一份等价口径 | **审毕无发现**：热度/关注度/候选权重唯一实现在后端（`ComputeLaneHeat`/`ComputeBoardAttention`/`LoadLaneHeatCandidateWeight`，`lane_heat_repository.go` 单点）；前端 `boardRadarHelpers.trendOf` 消费后端 `last3/prev3_clues` 不重复聚合，attention 直接取后端值；矩阵日格以 `clue_count` 为权威、旧响应 fallback 有声明（→R-10）；后端锚点日热度（`CountLaneCluesByDate`）与时间线日格（`listLaneSectionThreads` Clues）两条聚合对锚点日等价（同 join、同 COALESCE 综合稿排除）。前端仅 MatchingConfigDialog 有展示默认 0.5（显示兜底，非计算）。 |
| 3 | 聚簇降级链口径标注是否如实 | **审毕无发现**：wire 三态 `vector/semantic_fallback/subject_approx` → `EventClusterModeLabel` 中文（向量聚簇/语义向量兜底/主体近似）→ `highlights[].cluster_mode` 落中文 → 前端「聚簇口径 X」原样展示、缺省省略；Tier 2 向量覆盖不全**强制**整级降级（不混口径），Tier 3 指纹缺失各自成簇不假合并；`TestDailyReportEventClusterDegradation` 断言三态各就位且「不得呈现为向量聚簇」。 |
| 4 | SaveReport 重算是否加开关 | →R-2（建议加；判定「自然 no-op」只对开关后新报成立，重跑历史报仍会改写派生计数）。 |
| 5 | 覆盖率 17.6% 退化风险 | →R-6（交用户拍板）。 |
| 6 | 主路径质量未实测 | →R-5（归档后补核，不阻断）。 |
| 7 | 新阈值安静日热点 0 触发 | **符合设计、留档即可**：P80=14.5 的分位语义本就允许单日 0 命中（2026-11-10 为安静日：0 热点/11 关注，D9 已如实记录「不是漏算」）；30 日窗复算 P80=14.0 口径稳定。是否接受「热点档可为空」属产品口味，与 R-6 一并交用户过目即可，无需改代码。 |
| 8 | 阶段 1 删读端死代码的偏差 | →R-4（解读可接受、回滚成立，但制品需补留痕）。 |
| 9 | ai_call_logs 审计完整性 | **核心合规**：operation 非空且与索引刷新 `tagmanagement.related_event.embedding` 区分（测试断言 NotEqual）；不在 `embeddingCacheOperations` 白名单（每次真实调用）；成败均 LogCall（airouter `embedViaBindings` 双分支）；截断常量 10000/20000 runes 在（chat 路径）。缺登记行→R-3；prompt 措辞→R-7。 |
| 10 | 前端「未知不造 0」 | **审毕无发现**：`boardRadar.ts` `todayClues/clueDays→numOrNull`、candidate `clueCount/laneHeat→null`；`dayCellMetrics` 全未知→`clueCount:null`（'?'）；`useBoardRadarOverview.todayClueTotal` 任一 null→null；Masthead `articleCount!==null` 才显示「N 篇来源」，`clusterMode/dataAsOf/researchAt` 缺省整体省略。 |
| 11 | 测试断言与分支表 A/B/C 对应 | **抽查 5 条全对应**（A 表 7 行 12 子例全覆盖含权重 0/1/坏值/综合稿/桶合并；B 表阈值三态/同分破序/批量拆分/3-feed 去重/无链接 feed 粒度/综合稿/三级降级/LLM 集外重试+合法空+空白标题全就位；C 表开关三态/CLI 9 写模式拒绝/dry-run 保留）。唯一缺口=预算超限截断行→R-8；「审计落库」措辞→R-9。 |
| 12 | 文档 7.1 六处与实现一致 | **抽查通过**：semantic-board 红线 28/29/30（线索合计/颜色退役/新阈值 14.5-2.3/top_lanes+partial 标记）与实现逐条对得上；configuration.md 两键（`lane_heat_candidate_weight` 域 [0,1] 默认 0.5、`story_attribution_enabled` 默认 true）与代码一致；daily-report 红线 42-49、database/api 未逐字核但抽读无冲突。红线 42 的「恒 NULL」矛盾归入 R-1（非文档错写，是与默认开关的口径矛盾）。 |

## 审查方核实记录（子线程实测）

| 检查 | 结果 |
| --- | --- |
| `openspec validate retire-story-adopt-event-metrics --strict` | valid |
| `git diff 96f83315..a9be18b3 --diff-filter=D --name-only` | 仅 4 个删除件（读端 2 + 集成测试 2），写端零删除 |
| grep 写路径调用点（`CreateBoardStory` 等 5 符号，非测试） | 在线=1（开关拦）/CLI=拒绝/遗留=SaveReport:406，与 tasks 5.3 判定一致 |
| `story_backfill.go`/`story_reattribute.go` 调用方 | 仅 `cmd/backfill-stories/main.go:319` 等 CLI 路径（写模式全拒） |
| `retiredWriteModes`（main.go:96-121） | 9 种写模式（含默认无参）拒绝；`--dry-run` 透传；`--restore` 即使 dry-run 也拒 |
| `DefaultStoryAttributionEnabled` | `true`（story_attribution_settings.go:23），开关开=照常写（TestStoryAttributionSwitch:70-74） |
| `SaveThreads` 插入收口 | story_id=nil/story_manual=false/display_tier=body（daily_report_repository.go:652-660）；is_development 唯一 true 写方=归属链 UPDATE（开关拦） |
| `ComputeLaneHeat`/`ComputeBoardAttention` 唯一性 | 全仓非测试调用点：lane_snapshot/board_radar 两 repository + handler 只读权重展示；前端零等价公式 |
| 聚簇三态标注链 | wire→`EventClusterModeLabel`→`EventHighlight.ClusterMode`（中文）→Masthead `聚簇口径 ${clusterMode}`，缺省省略 |
| `topicgraph.daily_report.event_cluster.embedding` 缓存白名单 | 不在 `embeddingCacheOperations`（router.go:446-451） |
| airouter embed 成败双写 | `embedViaBindings` 成功/失败双 LogCall（router.go:760/784）；Prompt/ResponseSnippet 仅 chat 路径记（→R-7） |
| `LoadAuxLabels`（representation.go 新增） | 只读导出，实体层零改动（entity-subject-resolution delta「一字不动」成立） |
| 分支表抽查 | A/B/C 抽 5 条全对应（详见上表 #11） |
| 文档抽查 | semantic-board 28/29/30 + configuration.md 两键与实现一致（详见 #12） |
