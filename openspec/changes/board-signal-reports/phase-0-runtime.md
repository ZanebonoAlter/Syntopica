# 阶段0 白盒核定报告（phase-0-runtime）

> 2026-09-22，develop 主仓库直改（含其他活跃 change 脏改，未触碰）；所有定位基于**当前盘面**非 HEAD。核定时间点真实数据：现行 EIA table1 周结日 9/11/26。

## ① 接点清单（file:symbol + 对应 design 章节）

### 1. board 数据增强路由组挂载点与 202/409 互斥

| 事实 | 定位 |
| --- | --- |
| 路由挂载链 | `backend-go/internal/app/router.go:57`（`dataenrichment.RegisterRoutes(api)`）→ `backend-go/internal/dataenrichment/routes.go:8` → `handler/handler.go:110 (Handler).RegisterRoutes` |
| board 分析路由组 | `handler/handler.go:167`：`rg.Group("/semantic-boards/:id/enrichment/analysis")`，含 `POST /trigger`、`POST /investigations/trigger`、`GET /results[/:rid]`、relations 六路由、QA 三路由 |
| 轮询端点 | `handler/handler.go:121`：`GET /enrichment/analysis-status`（`?job_id=` 精确查 / `?scope=&id=` 恢复），`handler/board_enrichment_handler.go:272 getAnalysisStatus`（未知 job_id → 404） |
| 202/409 互斥实现 | **非 gin middleware**，是 runner 锁逻辑：`handler/analysis_runner.go:151 (*analysisRunner).Start`——同 `(scope,id)` 已有 running → `*RunningJobError`（携当前任务身份）→ handler 转 `respondErrorWithData(409, ..., runErr.Current)`；无冲突 → 后台 goroutine + `respondAccepted(202)`（`handler/handler.go:226 respondAccepted`） |
| 板块开关预检 | `handler/board_enrichment_handler.go:31 triggerBoardEnrichment`：先 `orchestrator.BoardEnrichmentEnabled` → 未开启 400（含 `containsNotEnabled` 判定）→ 再 `h.analysis.Start(AnalysisScopeBoard, boardID, AnalysisJobKindBoardBrief, analysisJobTimeout, fn)`，fn 调 `orchestrator.EnrichBoard(ctx, boardID)` 并返回 `output.Result.ID` |

对应 design §1（共享 board 级 202/409 互斥）、§7 新API（挂在同一路由组）。

### 2. 业务时区 / period 日历校验现状

| 事实 | 定位 |
| --- | --- |
| 业务时区 | `backend-go/internal/models/utils.go:7` `ShanghaiTZ = time.FixedZone("CST", 8*3600)`（reader/tagmanagement/admin scheduler 均用）；DB DSN `TimeZone=Asia/Shanghai`（`configs/config.yaml:9`）；dataenrichment 调度时段 `dataenrichment/scheduler_next_run.go`（`time.LoadLocation("Asia/Shanghai")`，失败回退 FixedZone） |
| period 半开区间 helper | `backend-go/internal/dataenrichment/service/period.go:75 ParsePeriodRange(period, granularity)` → `from, to, err`（半开 `[from,to)`）；week=ISO 周（Jan4 法则）、month=`2006-01`、year=`YYYY`（2000–2100 界） |
| 当前周期推导 | `service/period.go:56 PeriodForGranularity(t, granularity)`；消费方 `service/lifeline_context.go:125`、`service/freshness_gate.go:173` |
| ⚠️ 时区缺口 | `ParsePeriodRange` 的 month/year 边界目前用 **`time.UTC`** 构造（`period.go` month/year 分支），非业务时区——spec「服务端业务时区计算半开边界」需阶段1补时区感知 helper（新增或加 tz 参数），不能直接照抄现实现 |
| ⚠️ 未来周期校验缺口 | 现无「不晚于当前周期 → 400」的现成实现（datasources 域 JODI future-month 拒绝是类似先例：`sources/jodi.go:101`），阶段1 需新写 |

对应 design §2（周期校验/半开区间/cutoff）、spec「周期校验」Scenario。

### 3. topic_enrichment_result 模型 / kind 枚举 / sectors 现状

| 事实 | 定位 |
| --- | --- |
| 模型 | `backend-go/internal/dataenrichment/repository/models.go:87 TopicEnrichmentResult`——现有列：ID/PersistentTopicID*/SemanticBoardID*/AnalysisScope/ResultKind/ParentResultID*/QuestionKey*/EvolutionAssessment/**Sectors(jsonb)**/CausalChain/ToolCalls(jsonb)/InputSnapshot(jsonb)/SessionID/CreatedAt；**无 granularity/period/source_signal_id 列（本 change 新增，全部 nullable）** |
| kind 枚举（Go 常量） | `repository/models.go:79`：`topic_analysis | board_brief | board_investigation | legacy_board_analysis`（`EffectiveResultKind` 做迁移前兼容回读） |
| kind/shape DB CHECK | 迁移 `20260828_0001`：`backend-go/internal/platform/database/postgres_migrations.go:2190`——`chk_topic_enrichment_result_kind`（枚举 CHECK）+ `chk_topic_enrichment_result_parent_shape`（scope/owner 形状互斥 CHECK）+ 复合 FK/触发器（调查父必须同板块 board_brief）。**新增 signal_report 必须 DROP+re-ADD 这两条 CHECK 并加新 shape 分支**（board owner + semantic_board_id 非空 + parent/question_key 空 + granularity/period/source_signal_id 非空） |
| models 注册 | `repository/models.go` 底部 `init()` → `database.RegisterModels(...)`（AutoMigrate 路径） |
| sectors 载荷现状 | `json.RawMessage`；brief=`BoardSectors`（观察/关系/研究问题/不确定项，前端 `front/app/api/boardEnrichment.ts` 镜像类型）；无 schema_version 字段——design §7 的 `{schema_version:2, signal_snapshot, report, appendix, generation_meta}` 是全新 payload 形状，放现有 sectors 列 |

对应 design §7「数据库」、spec「成功快照与无评审输出」、约束#14（scope/owner 形状互斥 DB 强制）。

### 4. 内存 job 管理（kind/phase/outcome、30min、重启 404）

| 事实 | 定位 |
| --- | --- |
| runner | `backend-go/internal/dataenrichment/handler/analysis_runner.go`：`analysisRunner{mu, jobs(map[scope:id]), byID(map[jobID])}`；`Start/Status/StatusByJobID`；`safeRun` panic 兜底 |
| 现有字段 | `AnalysisStatus{JobID, JobKind, Scope, TargetID, Running, StartedAt, Finished, Error, ResultID}`——**无 phase/outcome/candidate_id/discovery_id/计数/error_stage 字段**（design §7 要求扩展：kind=`board_signal_discovery|board_signal_report`、phase=`prepare|detect|research|compose`、outcome=`discovered|no_signal|succeeded|failed`）；现 kind 常量仅 `topic_analysis|board_brief|board_investigation`（analysis_runner.go:40-44） |
| 30 分钟超时 | `handler/analysis_runner.go:31` `analysisJobTimeout = 30 * time.Minute`（`context.WithTimeout` 包裹 fn） |
| 重启 404 | 内存 map 进程重启即空 → `StatusByJobID` ok=false → handler 404；`jobs` 槽保留每 target 最近一个 job 供 `Status(scope,id)` 重进恢复；**注意 explore-findings 已记：job 回调无条件 `output.Result.ID`，零信号需显式无 result 防 nil** |

对应 design §1/§7「任务恢复与安全」、tasks 4.4、test-cases JB-1..4。

### 5. 增强 loop 基建（policy / 三防御 / allowedTools / airouter session）

| 事实 | 定位 |
| --- | --- |
| loop 核心（循环B 共享体） | `backend-go/internal/dataenrichment/service/orchestrator.go:576 runToolLoop(ctx, router, toolRegistry, capability, toolLoopParams)`；**policy 扩展点** `toolLoopPolicy{CheckCall/ObserveCall/CheckFinish}`（orchestrator.go:500-530）——新 signal_research 的 40 轮/预算/question 校验应挂此扩展点，不 fork 循环 |
| 三防御 | ① `/no_think` 前缀（runToolLoop 内 userMsg，~593 行）+ DB `enable_thinking=false` 主防线；② 工具结果 `ResultFull` 完整不截断（仅 `ResultPreview` 截断展示）；③ 去重 `orchestrator.go:1515 dedupKeyFor(tool,args)` + `seenCalls` map，重复直接拦截且不执行 |
| 轮数上限 | `orchestrator.go:441 maxAgentLoops = 6`（常量经 `toolLoopParams.maxLoops` 传入——新 research 传 40 即可，无需改常量） |
| allowedTools 机制 | runToolLoop 内 `allowedSet` guard：不在白名单 → 记 blocked（`tool_not_allowed`）不执行；现有白名单：`orchestrator.go:1530 explorationToolNames = [list_boards, list_lanes, get_lane_detail, web_search, fetch_page, search_internal_context]`；`buildAgentAllowedTools`（1532）拼 board 配置工具（财务类型已删、`ValidateSourceType` 全拒，实际追加为空）；调查链 `AllowedTools` 空=explorationToolNames（`board_investigation_research.go:647-649`）。**四源工具未进任何现有流程白名单（约束「工具面不变量」保持）** |
| 四源工具注册 | `backend-go/internal/datasources/wiring/tools.go:19 ResearchTools`（`eia_wpsr_table1/jodi_oil_primary/wb_wdi/un_comtrade_trade`）→ `dataenrichment/wire.go:101 toolRegistry.Register(wiring.BuildTools(...))`（已注册共享 registry、未授权）；registry 本体 `service/tool_registry.go:129/152 (Registry).Register` |
| airouter session | 每轮 `router.Chat(airouter.ChatRequest{SessionID, Operation, Capability...})`；session_id 生成：`service/board_analysis.go:16 generateBoardSessionID(boardID)`（`data_enrichment_board_{id}_{hex8}`）、`orchestrator.go:1488 generateSessionID(topicID)`、QA/关系链各有格式；`platform/airouter/router.go:172` 将 session_id 写入日志/span。新 discovery/research 各自独立 session_id（design §9） |
| 循环A/freshness | 循环A 自愈 `service/lifeline_context.go`（RefreshPeriod 按周期补建）；补全门 `service/freshness_gate.go:78 ensureLaneFreshness`（活跃泳道 month/year，72h 重算，限额 40 次 LLM）——design §2「freshness 保持原规则」即复用此处 |

对应 design §4（40轮/三防御/白名单）、§9（session/计数）、specs/research-data-sources「工具适配不改现有工具面」。

### 6. 四源适配器 / cache key / 404 与缺失处理 / JODI month

| 源 | 定位与现状 |
| --- | --- |
| EIA | `backend-go/internal/datasources/sources/eia.go`：URL 常量 `eiaWpsrURL = https://ir.eia.gov/wpsr/table1.csv`（302 → 同 host `/secure/...` CloudFront 签名 URL，`fetch.go` 有界重定向 MaxRedirects=3 同 host https）；`Fetch(ctx, section)`（stocks/supply 二选一）只产当周+上周两期；**cache key `eia:table1` 为无参常量**（eia.go:17）——加 weeks 后必须参数化；解析 `parseEiaTable1`：CP1252、`STUB_1`/MDY 日期列正则定位、未知标记/冲突重复行 → `SchemaChangedDetail` + `cache.Evict`；缺失标记 null+`missing_reason` 不转 0 |
| JODI | `sources/jodi.go`：`jodiURLTemplate = https://www.jodidata.org/_resources/files/downloads/oil-data/annual-csv/primary/primaryyear%d.csv`；`Fetch(ctx, geo, flow, unit, month)`（month=`YYYY-MM`）；`validateJodiArgs`（:66）：month 正则 + 年界 `jodiMinYear=2002..now.Year()` + 未来月拒绝；flow 映射 `jodiFlowMap`（production/imports/exports/closing_stocks，product 固定 CRUDEOIL）；`fetchYear`（:235）：无 month 本年 404 → 回退上一年**一次**（`year_strategy` 注记），显式 month 404 → 直接 SOURCE_UNAVAILABLE，双 404 → SOURCE_UNAVAILABLE；cache key `jodi:primary:%d`（按年文件，month 为取数后过滤） |
| WDI | `sources/wdi.go`：`https://api.worldbank.org/v2`；cache key `wdi:{indicator}:{codes}:{from}:{to}` |
| Comtrade | `sources/comtrade.go`：`https://comtradeapi.un.org/data/v1`；`Ocp-Apim-Subscription-Key` 头；cache key=完整 URL |
| 公共层 | `datasources/fetch.go`（FetchBudget 120s/32MB/3 跳/HTML 冒充检测）、`cache.go TTLCache`（900s，成功才 Put，Get 命中保留原 retrieved_at/sha256/last_modified）、`errors.go` 三态（InvalidArg/Unavailable(StatusCode)/SchemaChanged）；目录/probe：`catalog.go` + `wiring/register.go RegisterRoutes`（`GET /api/datasources` + `POST /{code}/probe`）；key 动态链 `wiring/register.go:92 ComtradeKeyResolver` |

对应 design §5「源能力和窗口」、specs/research-data-sources、约束（源封闭/口径/漂移/缓存）。

### 7. 前端定位（只报位置）

| 事实 | 定位 |
| --- | --- |
| 数据增强 API client | `front/app/api/boardEnrichment.ts`（类型 + 端点契约，`ResultKind`/`AnalysisJobKind` 镜像后端；新 signal API 类型按任务 5.1 新增，不复用旧 brief 返回类型）；client 底座 `front/app/api/client.ts` |
| 轮询 composable | `front/app/features/tags/composables/useBoardEnrichment.ts`（202/409 恢复 + 按 job_id 轮询状态机） |
| 工作台组件树 | `front/app/features/tags/components/BoardEnrichmentPanel.vue`（工作台容器：新闻背景折叠区 ~line 119、三 kind 分派 ~line 484）；子视图 `BoardBriefReport.vue`、`BoardInvestigationReport.vue`、`BoardAnalysisReport.vue`（legacy）、`BoardRelationPanel.vue`、`CausalAnalysisReport.vue`、`BoardTimelinePanel.vue`、`BoardThreadBrowser.vue` |
| 新闻背景折叠 | BoardEnrichmentPanel.vue 内部折叠态（循环A contexts 展示 + 叙事内联编辑）——design §8 要求保留 |
| AppPageShell reader | `front/app/components/ui/AppPageShell.vue` + `front/app/components/ui/layout-contract.ts`（`SHELL_MAX_WIDTH.reader = 760`） |
| 新组件落点（阶段3） | `front/app/features/tags/components/SignalCandidateList.vue / SignalReportList.vue / SignalReportView.vue`（tasks 5.2/5.3 已指明 + 测试文件同名 .test.ts） |

对应 design §8、ui-design.md Component Reuse、specs/data-enrichment「板块 tab 认知工作台」。

### 8. 迁移机制与隔离 PG 测试基建

| 事实 | 定位 |
| --- | --- |
| 建表/加列 | domain `repository` 包 `init()` → `database.RegisterModels`（`platform/database/migrator.go:57`）→ 启动 `RunAutoMigrate` |
| 版本化迁移 | **单文件** `backend-go/internal/platform/database/postgres_migrations.go`：`{Version: "YYYYMMDD_NNNN", Description, Up: func(db)}` 列表；`migrator.go:131 RunMigrations` → `runMigrationsList`（事务内执行、`schema_migrations` 台账、`withLockTimeout` DDL 护栏、`tableExists/columnIsNullable/ensureNotNullDefault` 幂等 helper）。CHECK/FK/回填/索引走这条路；新迁移文件就是往该文件追加条目 + 配套 `database/*_migration_test.go` |
| 现成迁移样板 | `20260828_0001`（result_kind：加列→校验脏行→回填→SET NOT NULL→CHECK→复合FK，`:2190-2300`）——阶段1 的 signal_report 迁移直接参照 |
| 隔离 PG 测试 | `backend-go/internal/platform/testutil/testutil.go`：`SetupTestDB(t)`（testcontainers-go，`pgImage = pgvector/pgvector:pg18-trixie`，进程级黄金 schema + `ResetTestData`）/ `ReimportTestDB` 逃生口；repository 包禁 SQLite（testing.md 红线）；迁移测试样板：`database/result_kind_migration_test.go` 等 |
| docker-compose.pg.yml | 容器 `syntopica-postgres`；端口 `${POSTGRES_PORT:-5432}:5432`；`POSTGRES_DB=syntopica / POSTGRES_USER=postgres / POSTGRES_PASSWORD=postgres`；`TZ/PGTZ=Asia/Shanghai`；数据卷 `./data`（**业务库，测试禁连**） |

对应 design §7「迁移需隔离PostgreSQL测试」、test-cases DB-1..5。

## ② EIA fixture 证据（URL / 参数 / 保存路径 / 可得窗口）

**核定结论：可核定，非阻塞。** 核定资源 = WPSR **归档版次 CSV**（官方 www.eia.gov 静态资源，无 key）。

- **URL 模板**：`https://www.eia.gov/petroleum/supply/weekly/archive/{YYYY}/{YYYY_MM_DD}/csv/table1.csv`（`YYYY_MM_DD` = 报告发布版次日，非周结日；周结日=文件内 MDY 日期列）
- **归档索引**：`https://www.eia.gov/petroleum/supply/weekly/archive/`——年版 **2011–2026**（实测 2011 与 2019 版次均有 `csv/table1.csv`；首页文本提及更早年 2000/2005 为旧报告页，未验 csv）
- **真实请求验证**（2026-09-22，外网直连）：
  - `.../archive/2026/2026_08_26/csv/table1.csv` → HTTP 200，6589 字节，周结 8/21/26+8/14/26，59 行，目标行标签与现行文件一致，8/21/26 值 718.636 与现行文件回溯值吻合
  - `.../archive/2019/2019_01_04/csv/table1.csv` → HTTP 200，6035 字节，周结 12/28/18，55 行（年代间总行数有差，标签定位解析不受影响）
  - 现行主文件 `https://ir.eia.gov/wpsr/table1.csv` → HTTP 302 同 host 签名跳转后 200（fetch.Get 有界重定向可处理，现状已在线上工作）
- **认证要求**：**无**。归档 URL 直接 GET 即可（对比：api.eia.gov/v2 实测 403 `API_KEY_MISSING`，注册入口 https://www.eia.gov/opendata/register.php——测试与运行时均无需走这条路）
- **结构**：与现行 table1.csv 同构（`STUB_1` + MDY 日期列 + `Crude Oil` / `Commercial (Excluding SPR)` / `Strategic Petroleum Reserve (SPR)` / `Crude Oil Supply` 分组 `Domestic Production/Imports/Exports`），现有 `parseEiaTable1` 无需改解析即可消费归档文件
- **weeks 1~12 可得窗口**：归档周版次连续（周三为主，假日周周四，实测 2026_09_10、2026_01_22 为周四版），12 周窗口远在覆盖内；**版次日映射不能纯「周三步进」**（假日跳变），实现建议：周三候选步进 + 版次 404 时探测相邻日，或按年版索引页发现版次列表（HTML 解析为次选）
- **fixture 保存**（真实字节，sha256）：
  - `backend-go/internal/datasources/sources/testdata/eia-wpsr-archive-2026-08-26-table1.csv`（`16193eb805aadd6358de5510125f00393f472d182854de8f2111158cdc369248`）
  - `backend-go/internal/datasources/sources/testdata/eia-wpsr-archive-2019-01-04-table1.csv`（`151e5b6224884393844df378880c6bf432109d80ed7bc0abb8e9cb6f1d96ef16`）
  - `backend-go/internal/datasources/sources/testdata/README-eia-wpsr-archive-fixture.md`（来源/结构/获取方式说明；惯例上 testdata 原先只有裸 CSV，特此补 README 防来源漂移）
- **备选资源（已核、未选）**：① EIA API v2 `petroleum/stocs/week/data`（JSON 全历史，需注册 key——选它=为 EIA 新增 key 面，与现行「EIA 匿名可用」冲突，需 controller 裁决，不推荐）；② dnav `hist_xls/*.xls`（无 key 全历史单文件，但旧版二进制 XLS 需引入新解析依赖，不推荐）

## ③ JODI month 兼容结论

**兼容，无冲突。** design/spec 要求「必须保留」的三行为全部有实现+有测试：

| 契约 | 实现 | 现有测试 |
| --- | --- | --- |
| 显式 `month=YYYY-MM` 历史单月 | `sources/jodi.go validateJodiArgs`（正则/2002..今年/未来月拒）+ `Fetch` 过滤 `r.period == month` | `TestJodiFetchExplicitMonth`（jodi_test.go:143） |
| 无参数取文件最新期，null 不回退旧值 | `Fetch` month=="" 分支取 max period；缺值 null+`missing_reason` | `TestJodiFetchLatestPeriodWithoutFallback` / `TestJodiFetchLatestNullNotSilentlyOlder` |
| 404 语义分链 | `fetchYear`：无 month 本年 404 → 回退上一年一次（`year_strategy` 注记）；显式 month 404 → SOURCE_UNAVAILABLE 不回退 | `TestJodiFetch404FallbackOnce`（含双 404 断言） |

附：缓存键按年文件（`jodi:primary:%d`），month/years 是取数后过滤维度，天然被覆盖；新增 `years` 只需与 `month` 互斥校验 + 多年文件循环（404 去重回退按 design §5）。测试现状与 tasks 3.1 的扩展点（`eia_test.go`/`jodi_test.go`）就绪。

## ④ 阻塞 / 冲突清单

**无 BLOCKER。** 以下为需阶段1实现注意的事实缺口与一处事实修正：

1. **（实现缺口，非阻塞）周期边界时区**：`service/period.go ParsePeriodRange` 现用 `time.UTC` 构造月/年边界；spec 要求业务时区半开边界 + 「不晚于当前周期」400——阶段1（task 2.2）需新增时区感知 helper，不得照抄现实现。
2. **（实现缺口，非阻塞）EIA 缓存键参数化**：现 `eia:table1` 为无参常量；weeks 参数化后必须入键（design §5「缓存键覆盖全部有效参数」），否则 12 周与 2 期响应会串缓存。
3. **（迁移注意，非阻塞）CHECK 扩展方式**：`chk_topic_enrichment_result_kind` 与 `chk_topic_enrichment_result_parent_shape` 是具名 CHECK，加 `signal_report` 必须 DROP+re-ADD（PostgreSQL CHECK 不支持增量），样板 `20260828_0001`；历史行回填后校验脏行的防御顺序照抄。
4. **（事实修正，不改变范围）**：explore-findings「样张内容核查」记录「当前四源不支持开工率」——核定发现官方 **`ir.eia.gov/wpsr/table9.csv` 在线可得，且含 Refiner Inputs and Utilization（炼厂开工）**。修正表述为：*当前适配器未接入* table9；首版仍按 design §5 不扩指标（不为此加适配器/表），未来如需开工率指标有真实官方资源可核。
5. **（可选裁决项）EIA API v2 路线**：如 controller 偏好 API v2 而非归档 CSV，需为 EIA 新增 API key 配置面（注册免费），并重跑 fixture 核定——与现行「除 Comtrade 外匿名可用」的 key 语义冲突。**推荐维持归档 CSV 路线**（无 key、解析器零改动、已留 fixture）。
6. **（实现缺口，非阻塞）AnalysisStatus 扩展**：现无 phase/outcome/candidate_id/discovery_id/计数/error_stage 字段（tasks 4.4 预期内）；另 explore-findings 已记 job 回调 `output.Result.ID` nil 风险——discovery 零信号必须显式无 result。

## ⑤ 对阶段 1/2/3 的实现定位建议（唯一写辖区）

- **阶段1（数据与源）**：`backend-go/internal/datasources/sources/eia.go`（weeks 参数 + 归档 URL 模板 + 缓存键参数化 + cutoff 语义注意）、`sources/jodi.go`（years 显式互斥）、对应 `eia_test.go`/`jodi_test.go` 扩展（fixture 已就位）；`internal/dataenrichment/repository/models.go` + `repository.go`（批次/候选新表 + 查询）；`internal/platform/database/postgres_migrations.go` 追加版本迁移（新表 + result 三 nullable 列 + CHECK 扩展 + 索引）+ `database/` 迁移测试；repository 测试 `signal_candidate_test.go`/`signal_report_test.go`（隔离 PG）。**不改 service/handler/前端。**
- **阶段2（编排与HTTP）**：`internal/dataenrichment/service/`（新文件 signal_material/signal_detect/signal_research/signal_calculation/signal_compose + policy 挂 `runToolLoop` 扩展点 + allowedTools 仅新 research 加四源）；`handler/analysis_runner.go`（phase/outcome/计数扩展）；`handler/handler.go`（新路由挂现有 board 组）+ 新 handler 文件；`wire.go` 装配；`internal/app/router.go` 不动（已挂 dataenrichment）。测试 `signal_*_test.go`。**不改阶段1的 datasources/迁移合同。**
- **阶段3（前端）**：`front/app/api/`（新 signal 契约模块）、`front/app/features/tags/components/Signal*.vue` + 测试、`BoardEnrichmentPanel.vue`（卸载旧产出/绑定入口、挂候选/报告列表、保留新闻背景折叠）；`AppPageShell reader` 仅复用。**不改后端。**
