# Review：fix-radar-overview-false-zero

> 审查方式：派只读 change-review 档子线程按 `review-brief.md` 聚焦审查（tasks T2.1）——主审工作树未提交 diff（后端 `backend-go/internal/topicgraph/**` 7 文件 + 前端 `front/app/features/tags/**` 与 `front/app/api/**` 12 文件 + 文档 2 文件 + 配置 2 文件），对照 proposal / design D1–D8 / ui-design / delta spec（`specs/workshop-radar/spec.md`）/ tasks 逐项核查，重点 10 项（brief 全文）。
> 审查范围：本 change 辖区内的全部实现 diff + 制品 + 文档；跑只读验证（`go vet` / `golangci-lint`（topicgraph 包）/ `go test -p 1 ./internal/topicgraph/{repository,handler}/...` / 前端 typecheck / 5 个受影响测试文件 vitest / `doc-impact.sh verify`）。
> 未审：工作区内其它 active change 的脏改动本体（`tagmanagement/*`、`discovery/service/*`、`archive-time-chart-range` 的 BoardArchiveView 族与 `docs/reference/api/semantic-boards.md`、`unify-inner-scrollbar-style` 的 `.scroll-thin` 残留等）——只核了「本 change 是否回退/覆盖它们」（见验证记录第 7 条，无回退）。

## 发现

### R-1 2026-11-19【Medium】doc-impact 声明缺 `configuration`/`standard` 域 + tasks 里 verify 命令形态错误——归档门禁（T5.5）将被拦
- **状态**：已修复（`<!-- doc-impact: api, flow, configuration -->` + 新增 T4.3 configuration.md 条目；standard 域经 `doc-impact-excuse` 豁免，理由：theming.md/main.css 的 `.scroll-thin` 改动属已归档的 unify-inner-scrollbar-style，非本 change 辖区；T5.5 命令改为 `doc-impact.sh verify openspec/changes/fix-radar-overview-false-zero`。实测 verify → 通过）
- **位置**：`openspec/changes/fix-radar-overview-false-zero/tasks.md:47`（`<!-- doc-impact: api, flow -->`）；`tasks.md:58`（T5.5 命令写成 `doc-impact.sh verify --change fix-radar-overview-false-zero`）；证据改动 `backend-go/configs/config.yaml:14-19`、`backend-go/internal/platform/config/config.go:161-165`（连接池 25/40，D7 落地）；`docs/reference/configuration.md:106-115`（工作树已同步更新但未登记）；`docs/reference/standard/frontend/theming.md`（编辑落在本 change 的 edit.map 归属集合）。
- **问题**：实测 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-radar-overview-false-zero` → **2 FAIL**（归属轨命中：`疑似遗漏: 改了 configuration 相关代码未声明 configuration`、`疑似遗漏: 改了 standard 相关代码未声明 standard`）。本 change 确实改了配置域文件（config.yaml + config.go，且 configuration.md 已随之更新到位），但声明只写了 `api, flow`，文档节也没有 configuration.md 的 checkbox——**文档工作已做、登记欠账**，spec-gate ①/check-standards F 段会拦归档，T5.5「退出码 0」现在不成立。另：T5.5 写的 `verify --change <名>` 不是脚本用法（脚本只认 change 目录参数），照抄会得到「FAIL: change 目录无效或无 tasks.md」。
- **修复建议**：① tasks.md 头部注释改为 `<!-- doc-impact: api, flow, configuration -->`，文档节补一条 checkbox「T4.3 `docs/reference/configuration.md` — 连接池 25/40 调参说明（已随 D7 更新，补登记）」；② `standard` 域若确认 theming.md 是 `unify-inner-scrollbar-style` 的残留编辑被误归属，加 `<!-- doc-impact-excuse: standard=theming.md 为 unify-inner-scrollbar-style 的滚动条规范残留编辑，非本 change 辖区 -->` 豁免（或把该编辑收回正确 change）；③ T5.5 命令改成 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-radar-overview-false-zero`。

### R-2 2026-11-19【Medium】`CountLaneCluesByDate` 失败被吞成「全板 0 热」——与「未取到 ≠ 0」新契约残留一条「未知→0」旁路（既有行为，非本次引入）
- **状态**：已修复（`LaneDynamicsResponse` 新增 `CluesDegraded`，`computeBoardLaneDynamics` 线索聚合失败时置位；单版块端点行为不变，雷达 `radarRowFrom` 见 `CluesDegraded` 即走「未取到」——计数落 null + `data_unavailable=true`。新增用例 `TestBoardRadarCluesDegradedRowIsUnknownNotZero` 钉住）
- **位置**：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:692-696`（`clueByTopic, err := r.CountLaneCluesByDate(boardID, anchor); if err != nil { logging.Warnf(...); clueByTopic = map[uint]int{} }`）；下游 `board_radar_repository.go` 的 `radarRowFrom`（dynErr=nil 时行不标 `data_unavailable`、attention 照常输出）。
- **问题**：brief 重点第 1 项（「有没有任何路径把『未知』写成 0」）逐路径排查后，**唯一残余**是这条既有降级：锚点日线索聚合单查失败（非 ctx 取消的瞬时 DB 错误）时，该板所有泳道 lane_heat 记 0、attention 照常聚合（Σ0 或只剩 0）且 `data_unavailable=false`——前端会渲染成「关注强度 0.0」并进常温，正是本 change 要消灭的假 0 形态。说明：① 这是 lane-dynamics 单版块端点的**既有且有注释记载**的降级（「失败降级为 0 热并告警，不阻断响应」），非本次回归——批量路径与单版块路径共用该段，口径测试钉住的是「两条路径一致」而非「该失败路径诚实」；② ctx 取消场景已安全：预算到期后该查询与其后的 `GetLaneSnapshotsByTopicIDs` 会一起被取消，后者报错 → dynErr → 行正确标 `data_unavailable`，不会产出假 0。
- **修复建议**：不改统计口径的前提下收口：`computeBoardLaneDynamics` 里该查询失败时直接 `return nil, err`（让该板走既有「计算失败 → data_unavailable」路径），或在 `radarRowFrom` 增设「clue 计数降级过 → 至少 attention 置 null」的信号。若选择维持既有降级（单版块端点行为不变的理由成立），请在 design D2 或 flow 文档记一句豁免说明，避免与 delta spec「计算失败 SHALL 标 data_unavailable」字面冲突。

### R-3 2026-11-19【Low】「今日线索」合计剔除滞后版块：行为正确且有测试/验收记录，但 delta spec 与 flow 文档均未记（63 vs 66 差异无出处）
- **状态**：已修复（delta spec「滞后版块不占前排」补一句 + 新增 Scenario「滞后版块的旧日线索不进今日合计」；flow 文档 L0 段同步半句）
- **位置**：`front/app/features/tags/composables/useBoardRadarOverview.ts:121-131`（`todayClueTotal` 只遍历 `freshRows`，滞后版块的旧日线索不进合计）；行为证据：`useBoardRadarOverview.test.ts`「滞后行的旧日线索不进今日线索合计」、`ui-verification/report.md` A-2（API 合计 66、页面显示 63，差值 = 滞后版块 11-07 的 3 条）；未覆盖处：`specs/workshop-radar/spec.md` delta 两条 requirement（「降级不得呈现为业务结论」只写了未取到版块 → 未知，「滞后版块不占前排」只写分档/位次）与 `docs/reference/flow/semantic-board.md` L0 段均未提「今日线索合计只计锚点日 = 全局锚点日的版块」。
- **问题**：剔除本身是对的（滞后行 11-07 的线索不是「今日」的数，混入即是旧数据污染今日口径），也过了测试与真机验收；但这是用户可见的统计口径变化（报头 63 vs API 66），在 spec/flow 两层文档都没有出处——后续读者（或回归排查）容易把 3 条差额当 bug 报。
- **修复建议**：在 delta spec「滞后版块不占前排」requirement（或「降级不得呈现为业务结论」）补一句「当日线索合计 SHALL 只计锚点日与全局锚点日一致的版块（滞后版块的线索属于它自己的锚点日，SHALL NOT 计入今日合计）」，flow 文档 L0 段滞后沉底句顺势带半句；归档同步主 spec 时一并带上。

### R-4 2026-11-19【Low】防御性缺口：`attention=null` 且 `dataUnavailable=false` 的已知行会从四个分段全部漏出、从主列表消失（当前协议不可达）
- **状态**：已修复（采用建议②的邻居方案：`coldRows` 兜底收 `attention === null` 的已知行归常温，行内本身已全 null-safe 显示「—」，不再静默消失）
- **位置**：`front/app/features/tags/composables/useBoardRadarOverview.ts:92-97`——hot/warm/cold 三档都前置 `row.attention !== null` 滤掉 null，而该行又不是 `dataUnavailable`（不进 unavailableRows）、锚点日可能等于全局（不进 laggingRows）→ 四段并集（`storyRows`）不含它 → 视图不渲染该行。
- **问题**：触发条件在**当前后端协议下不可达**（attention=null ⟺ data_unavailable=true，两处赋值点一一对应；改前旧后端 attention 恒为数字），所以不是现存 bug；但 normalizer 注释承诺「缺字段与未取到都归一为 null，不造 0」，若未来某版本后端漏发/错发 `data_unavailable`（或只置 null 不置标记），这些行会**静默从页面消失**而不是显示「—」——比显示 0 更隐蔽。
- **修复建议**（二选一，一行改动）：① `coldRows` 分档把已知行兜底：`row.attention === null || row.attention < RADAR_WARM_ATTENTION`（null 已知行进常温，行内已全部 null-safe 显示「—」）；② normalizer 把「attention 为 null/缺字段」同时归一 `dataUnavailable=true`（与注释语义对齐，走未取到段呈现）。不阻断归档。

## 十项重点核查结果（brief 清单）

1. **未知写成 0 的路径**：后端逐分支核过（超时分支 `markRadarRowsUnavailable` 全置 nil、计算失败分支 `radarRowFrom` 全置 nil、brief 失败只置报告计数 nil 且不误伤其余）；前端 `numOrNull` 归一、无 `?? 0`/`|| 0` 残留（全量 grep，仅 `todayRead` 排序用 `?? -1` 作比较键，非展示）。唯一残余 = R-2（既有）。
2. **真实零值误伤**：无。`TestBoardRadarRealZeroRowKeepsZeroCounts` + handler 零值行用例钉住「0 计数 + data_unavailable=false + 14 档 0 轴」；前端「真实零值仍进常温」有专项用例。
3. **口径漂移**：批量路径与单版块路径共用同一份聚合（`radarRowFrom` + `computeBoardLaneDynamics`，非复制）；批量 anchor SQL `status='completed'` 过滤与 `GetBoardReportAnchor` 逐字一致（同表同过滤同「最新一行」语义）；`loadLatestReportBriefs` 不限 status 与 `GetLatestReportBrief` 一致，`(semantic_board_id, period_date)` 唯一索引（`idx_board_daily_reports_board_period`）保证 ROW_NUMBER 取舍确定。`board_radar_batch_test.go` 三条口径回归（板级三项 / 雷达行逐字段 / 端点 vs 单版块）钉住。连接池 25/40 不改任何查询语义。
4. **ctx 取消**：`withContext` 克隆让全部雷达路径查询挂请求 ctx；ctx 取消 → 批量取数失败 → `levelErr`/`briefErr` → 各板降级为「未取到」（null），无一处把 ctx 错误当「没数据 → 0」（ctx 取消导致的 clue 查询失败会被后续快照查询的取消错误兜住，见 R-2 说明②）。
5. **前端 null 安全**：`tierLabel(null)→null`、`attentionText`、`trendOf`、`isHot/isWarm`、Masthead `?? '—'`、Story 行内模板分支均已 null-safe；`toFixed` 只在非 null 分支。typecheck 全绿。残余 = R-4（防御性）。
6. **排序与序号**：四段接号（visible → cold[`rankOffset=visible.length`] → lagging[`+cold.length`] → unavailable[`+lagging.length`]）数学上连续不重号；视图渲染数组与 `storyRows` 构成一致（同一四段拼接）；后端排序只负责降序 + 未知沉底，前端重排为四段。真机 A-1 验证 11–12 常温折叠后接 13 滞后，序号连续。
7. **raw SQL 参数化**：两条新增批量 SQL（`loadBoardLevels` ①锚点、`loadLatestReportBriefs`）均 `?` 占位符 + 参数绑定（boardIDs / ReportStatusCompleted），无字符串拼接。
8. **SQLite 兼容**：`ROW_NUMBER()` 窗口函数两条 SQL 在内存 SQLite（glebarez/sqlite）下跑通——`go test ./internal/topicgraph/repository/...` 全绿（含批量口径回归与超时降级用例）。
9. **既有行为回归**：逐 hunk 对照，既有用例断言值零改动（只改指针解引用写法：12→*12、6.0→*6.0 等）；`data_delayed` 保留原义（超时专属），计算失败路径不标它（测试断言 `DataDelayed=false`）；`finalizeRadar` 日期回退语义只加了「存在未取到行时不回退」分支，真实空态回退今天行为不变（既有测试 `TestBoardRadarDateFallsBackToToday` 原样通过）。
10. **并发脏文件**：本 change 对 `BoardRadarStory.vue` / `BoardRadarView.vue` 的编辑为定点增量——`RowActionMenu` 行尾菜单、`BoardQuickEditDialog`/`BoardArchiveDialog`、`.scroll-thin`/`scrollbar-width` 移除等他人 hunk 均保留未动；`docs/reference/api/semantic-boards.md` 与 `semantic-board.md` 的 L1 归档段改动属 `archive-time-chart-range`，与本文 L0 段改动同文件共存、无覆盖回退。未发现本 change 回退他人改动。

## 验证记录（只读命令 + 受影响包测试）

1. `go vet ./internal/topicgraph/...` → 通过（0 问题）。
2. `golangci-lint run ./internal/topicgraph/...` → 0 issues。
3. `go test -p 1 ./internal/topicgraph/repository/... ./internal/topicgraph/handler/...` → 全绿（含新增 `board_radar_batch_test.go` 3 条、超时/计算失败/真实零值 3 条、handler wire shape 1 条）。
4. `go test -p 1 ./internal/topicgraph/...` 全量（T5.1 口径）：repository/handler 绿；**service 包因 Docker testcontainer 起不来挂**（「database system is ready to accept connections matched 1 times, expected 2 … context deadline exceeded」——环境问题，本 change 未改 service/ 任何文件，非本 change 红；台账登记交主线程）。
5. `pnpm exec nuxi typecheck` → 无类型错误（可空化全链路 null-safe 成立）。
6. `pnpm test:unit`（useBoardRadarOverview / BoardRadarView / BoardRadarStory / BoardRadarMasthead / boardRadar normalizer 五文件，`--maxWorkers=2`）→ 87 + 10 全绿。
7. `git status/diff` 三方核对（本 change 辖区 vs 其它 active change 脏文件 vs 未跟踪）→ 本 change 未回退他人改动（见上第 10 条）。
8. `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-radar-overview-false-zero` → **2 FAIL**（R-1）；`--change` 形态为脚本不支持的用法。

## 结论

十项重点全部核过：**核心契约（未取到 ≠ 0、真实零值不误伤、口径零漂移、ctx 取消安全、null 安全、接号、参数化、SQLite、断言值、脏文件）无 High 发现**；2 条 Medium（归档门禁欠账 R-1、既有假 0 旁路 R-2）+ 2 条 Low（口径出处 R-3、防御缺口 R-4）。R-1 必须在归档前处置（T5.5 现状不成立）；R-2 建议收口或留豁免说明；R-3/R-4 可随归档顺手清。

---

## 第二轮复审（2026-11-20，独立只读审查子线程）

> 审查方式：按 `review-brief.md` 十项重点对当前工作树全量复核（后端 topicgraph 5 文件 + handler 2 文件 + 前端 radar 链 8 文件 + 配置 2 + 文档 3），对照 proposal / design D1–D8 / ui-design / delta spec / tasks 逐条核；不依赖上轮结论，独立取证后与 R-1~R-4 对账。
> 上轮 R-1~R-4 均未处置（详见下方复核记录）；本轮新增 R-5~R-7。

### R-5 2026-11-20【Medium】日报 brief 取数失败路径：`report_date=""` 被前端渲染成「无日报日期 / 今天暂无日报」——「没取到」被当成「没有」的残留伪空态
- **状态**：已修复（`BoardRadarStory.vue` 新增 `reportUnknown` 判定——`reportArticleCount`/`reportEventCount` 为 null 时文案改「日报信息未取到」，`reportFallback` 同步；新增用例「日报信息没取到（brief 失败、泳道数据正常）：不说『无日报日期 / 今天暂无日报』」钉住）
- **位置**：后端 `backend-go/internal/topicgraph/repository/board_radar_repository.go:358-360`（`briefErr := batch.briefErr`，批量日报摘要单查失败时全体行的 brief=nil）与 `board_radar_repository.go:416-419`（`case briefErr != nil:` 只记日志——`ReportDate`/`ReportTitle` 留空串、`ReportArticle`/`ReportEvent` 留 nil，**不标 `data_unavailable`**）；前端 `front/app/features/tags/components/console/BoardRadarStory.vue:153`（`{{ row.reportDate ? shortDate(row.reportDate) : '无日报日期' }}`）与 `BoardRadarStory.vue:85,143-146`（`reportFallback` = reportDate 为空 → 「今天暂无日报」）。
- **问题**：delta spec 的 `data_unavailable` 只列了两条触发（端点超时未完成、泳道动态计算失败），但实现里存在**第三条降级路径**：`loadLatestReportBriefs` 批量查询失败（非 ctx 取消的瞬时 DB 错误，如连接抖动/重启窗口）→ `briefErr` 全体非 nil → 每行 `report_article_count`/`report_event_count` 为 null（诚实），但 `report_date` 是空串且 `data_unavailable=false` → 前端对非未取到行走正常分支，逐行渲染「无日报日期」+「今天暂无日报」——把「这次没取到日报信息」呈现成了「这个版块没有日报」的业务结论。这正是本 change 要消灭的形态（proposal What-Changes #6 明确「不再显示『无日报日期』这种伪空态」），只是触发面比超时窄。关注度/线索数不受影响（泳道数据独立取）。
- **建议改法**（一行级）：前端 `BoardRadarStory.vue` 在报告计数未知时改文案——`row.reportArticleCount === null`（且非 `dataUnavailable`）时首 span 渲染「日报数据未取到」、`reportFallback` 同步给「日报数据未取到」而非「今天暂无日报」；或后端在 `radarRowFrom` 的 `briefErr` 分支给行加局部未知标记。补一条 briefErr 注入的前端/后端用例（当前 `TestBoardRadarOverviewUnavailableRowWireShape` 只钉超时路径，briefErr 路径零覆盖）。

### R-6 2026-11-20【Low】partial 拐角：`visibleRows=0` 且部分未取到时，headline 仍以「0 个板块值得关注（M 个版块未取到）」开头——delta spec 场景的字面违反
- **状态**：已修复（`partial && visibleRows.length === 0` → headline 改「本次数据未取全，请重试」；新增用例断言不含「0 个板块值得关注」）
- **位置**：`front/app/features/tags/composables/useBoardRadarOverview.ts:158-160`（`unavailableCount > 0` 时 headline 模板直接拼 `${visibleRows.value.length} 个板块值得关注（…）`，未对 `visibleRows.length === 0` 设防）。
- **问题**：delta spec「部分版块未取到时不污染统计」要求报头标题「SHALL NOT 出现『0 个板块值得关注』这类由未取到数据推出的结论」。存在可达拐角：① 未取到行走**计算失败**路径且 brief 已取到（`ReportDate` 保留、且恰为全局最新），其余已知行锚点日全部落后 → 全局 `date` 由未取到行贡献、已知行全判滞后 → `visibleRows=0`，headline 输出「0 个板块值得关注（1 个版块未取到）」；② 更常见的弱化形态：已知行恰好全部低于关注阈值 + 部分未取到。两种形态里前半句「0 个板块值得关注」都部分源自未取到数据（未取到的版块本可能值得关注）。括号补语 + sub「本次数据未取全。」降低了误导，但与场景文本字面冲突。
- **建议改法**：`partial && visibleRows.length === 0` 时 headline 直接用「本次数据未取全，请重试」（与 allUnavailable 分支同文案或「本次数据未取全」），补一条用例。

### R-7 2026-11-20【Low】批量口径等价性测试缺 mixed-status 日报夹具——批量 anchor SQL 的 `status='completed'` 过滤漂移不会被钉住
- **状态**：已修复（新增 `TestBoardLevelBatchAnchorIgnoresUnfinishedReport`：造一份更新的 `generating` 日报，断言批量锚点 = completed 那份且与 `GetBoardReportAnchor` 一致、generating 不作锚点；同时钉住 brief 侧「不限 status」的不对称口径）
- **位置**：`backend-go/internal/topicgraph/repository/board_radar_batch_test.go:100-137`（`TestBoardLevelBatchMatchesSingleBoard` / `TestBoardRadarBatchMatchesSingleBoardRow` 的夹具 `seedRadarBatchBoard` → `seedTestReport` 恒 `status="completed"`，`daily_report_repository_test.go:393`）；对照单版块侧已有先例 `lane_snapshot_repository_test.go:239-251`（completed + generating 夹具钉 `GetBoardReportAnchor` 只认 completed）。
- **问题**：批量 vs 单版块的锚点等价断言在「存在**更新的 generating 日报**」数据上才有判别力——若未来批量 SQL 的 `status='completed'` 过滤被误删/误改，现有夹具下两条路径仍取到同一行、等价测试照绿。brief 批查（两路均不限 status）同理。当前两处 SQL 与单版块逐字一致（本轮已人工核对：`loadBoardLevels` ① 与 `GetBoardReportAnchor` 同表同过滤同「最新一行」；`loadLatestReportBriefs` 与 `GetLatestReportBrief` 均不限 status），纯属测试网眼问题。
- **建议改法**：在 `seedRadarBatchBoard`（或独立用例）补一份**更新的 generating 状态日报**，断言批量锚点 = completed 那份（与 `GetBoardReportAnchor` 一致）、brief 取最新那份（不限 status）。

## 上轮发现复核（截至 2026-11-20 10:30 工作树）

> **处置回填（2026-11-20 11:10）**：下列 R-1~R-4 的四条「未处置」记录是复审判定时的**历史快照**，四条均已在本轮处置完毕，逐条状态见上方各条目（R-1 已修复 / R-2 已修复 / R-3 已修复 / R-4 已修复）。`doc-impact.sh verify` 现为「通过（声明: api, flow, configuration 文件:3 个 启发式:ownership）」，归档门禁不再被拦。以下原文保留以便溯源。

- **R-1【Medium】未处置**：`tasks.md:47` 仍为 `<!-- doc-impact: api, flow -->`（缺 `configuration`），文档节无 T4.3；`tasks.md:58`（T5.5）仍写 `doc-impact.sh verify --change fix-radar-overview-false-zero`（脚本不支持的形态）。实测 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-radar-overview-false-zero` → 仍 **2 FAIL**（`疑似遗漏: 改了 configuration 相关代码未声明 configuration` / `…未声明 standard`）。归档门禁仍会被拦。
- **R-2【Medium】未处置**：`lane_snapshot_repository.go:700-703` 原样——`CountLaneCluesByDate` 失败吞错降级 0 热并告警（非 ctx 错误时该板 lane_heat 全 0、attention 照常输出且不标未取到）。仍为既有行为、批量/单版块两路共用；design/flow 仍无豁免说明。
- **R-3【Low】未处置**：「今日线索合计只计锚点日 = 全局锚点日的版块」（`useBoardRadarOverview.ts` freshRows 合计，实测 63 vs API 66）在 delta spec 与 `flow/semantic-board.md` L0 段仍无出处。
- **R-4【Low】未处置**：`attention=null` 且 `dataUnavailable=false` 的已知行仍会从四段全部漏出（hot/warm/cold 前置 `attention !== null`、不进 unavailable、可能不滞后）——当前协议不可达，防御缺口未收口（normalizer 注释承诺「缺字段归一 null」但不同时置 `dataUnavailable`）。

## 本轮验证记录（只读命令 + 受影响包测试）

1. `git status --short` + 逐文件 diff 全量过目：本 change 辖区（topicgraph repo 4 + handler 2 + config 2 + 前端 radar 链 8 + 文档 3 + 未跟踪 `board_radar_batch_test.go`/`BoardRadarStory.test.ts` 等）之外，他人 active change 的脏改动共存且**未被回退**（`BoardRadarStory.vue` 的 `RowActionMenu`/`editBoard`/`archiveBoard`、`BoardRadarView.vue` 的 `BoardQuickEditDialog`/`BoardArchiveDialog` 与 `scroll-thin`、`semantic-board.md`/`daily-reports.md` 的 console-rails-live-signals 段、`flow/daily-report.md` 派生关系行、theming.md/scrollbar 残留均在）。
2. `go vet ./internal/topicgraph/...` → 0 问题。
3. `go test -p 1 ./internal/topicgraph/... -count=1` → 首跑 service 包因 Docker testcontainer 启动超时 FAIL（环境抖动，机器高负载）；**复跑三包全绿**（repository 188.293s / handler 55.251s / service 52.257s）——T5.1 口径在本机当前负载下成立。
4. `pnpm test:unit`（useBoardRadarOverview / BoardRadarView / BoardRadarStory / BoardRadarMasthead / boardRadar normalizer 五文件，`--maxWorkers=2`）→ **97 passed**（含滞后沉底、未取到行、真实零值、四段接号用例）。
5. `pnpm exec nuxi typecheck` → EXIT=0、0 TS 错误（T5.2 口径成立）。
6. `openspec validate fix-radar-overview-false-zero --strict` → valid。
7. `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-radar-overview-false-zero` → 2 FAIL（R-1 未处置）。

## 本轮十项重点复核结论（与上轮一致的仅列增量证据）

1. **未知→0**：后端三分支（超时 `markRadarRowsUnavailable`、计算失败 `radarRowFrom`、brief 失败）计数全 nil；前端 radar 链无 `?? 0`/`|| 0` 业务兜底（仅 `degradedBoardCount`/`laggingBoardCount` 的右栏脚注计数 `?? 0`，数的是标记清单本身，非业务统计）。超时收口把「已算完但未出队」的行也标未取到——保守的**假未知**而非假 0，代码注释明示为可复现性取舍，可接受。新增残留 = R-5（未知→伪空态）；既有残留仍只有 R-2。
2. **真实零值**：`TestBoardRadarRealZeroRowKeepsZeroCounts` + handler `TestGetBoardRadarOverviewBoardWithoutDataZeroRow` + 前端「真实零值仍进常温」用例三层钉住。✓
3. **口径零漂移**：批量/单版块共用 `computeBoardLaneDynamics` + `radarRowFrom`（非复制）；两条批量 SQL 与单版块逐字一致（人工核对 + 三条等价测试）；缺口 = R-7（夹具）。
4. **ctx 取消**：`withContext` 克隆让全部雷达查询挂请求 ctx；取消 → 查询错 → dynErr → 未取到；`GetLaneSnapshotsByTopicIDs([])` 空参早退不查库（此时真无泳道，0 是真实零值）。无 ctx 错误被当「没数据→0」的路径。✓
5. **null 安全**：`tierLabel/attentionText/trendOf/isHot/isWarm` 与行内模板分支全 null-safe；`toFixed` 仅经 `attentionText` 非 null 分支；typecheck 0 错。残余 = R-4（防御性）。
6. **排序与序号**：四段接号（`BoardRadarView.vue:150,163,177,194`，`visible → cold[+V] → lagging[+V+C] → unavailable[+V+C+L]`）数学连续；`storyRows` 与视图渲染构成一致；测试断言 `['01','02','03','04']` 与「滞后在常温后、未取到最底」钉住。✓
7. **参数化**：`loadBoardLevels`/`loadLatestReportBriefs` 两条批量 SQL 均 `?` 占位符 + 参数绑定，无拼接。✓
8. **SQLite**：repository 包（内存 SQLite）全绿，`ROW_NUMBER()` 可用。✓
9. **既有断言**：逐 hunk 对照，既有用例断言值零改动（仅指针解引用写法）；`data_delayed` 保留超时原义（计算失败行断言 `DataDelayed=false`）；`finalizeRadar` 只加「存在未取到行不回退今天」分支，既有回退测试原样通过。✓
10. **并发脏文件**：未发现本 change 回退/覆盖他人改动（见验证记录 1）。✓

## 本轮结论

十项重点复核通过（无 High）；核心契约（未取到 ≠ 0、真实零值不误伤、口径零漂移、ctx 安全、null 安全、接号、参数化、SQLite、断言值、脏文件）在本轮独立取证下全部成立，后端三包 + 前端 97 用例 + typecheck + `openspec validate --strict` 全绿。新增 1 Medium（R-5 brief 失败伪空态）+ 2 Low（R-6 partial 拐角标题、R-7 等价测试夹具）；上轮 4 条（R-1~R-4）均未处置，其中 **R-1 仍阻断归档门禁（T5.5 不成立，doc-impact verify 2 FAIL）**，R-2/R-5 同属「未知被呈现为业务结论」家族建议一并收口。T5.1/T5.2/T5.3 的命令级验证本轮已代跑且成立（T5.4/T5.5/T5.6 未跑/未过，维持未勾选状态正确）。
