# Tasks

## 1. 后端数据源域（internal/datasources/）

- [x] 1.1 catalog 扩三源常量：`yahoo_finance`/`fred`/`gdelt` 静态属性（覆盖/主题/频度 daily/典型滞后 T+0 与小时级/单位纪律/requires_key）+ HostAllowlist 三 host + 工具名 `yahoo_chart`/`fred_series`/`gdelt_doc`；启动 upsert seed 三行 ——验证：`go test ./internal/datasources/ -run Catalog` 绿，目录测试断言七源
- [x] 1.2 Yahoo 取数器：symbol 封闭枚举（首批 15 个，spec 列举清单为准；design 原写 17 系计数笔误）+ range 枚举（1mo/3mo/6mo/1y）发网前校验；`chart.result[0]` 对位解析 OHLC，null close 如实、结构不齐 SCHEMA_CHANGED；货币 meta.currency 透传；httptest 表驱动单测（合法/枚举外/结构漂移/null 点）——验证：`go test ./internal/datasources/ -run Yahoo` 绿
- [x] 1.3 FRED 取数器：series_id 格式校验 `^[A-Z0-9_]{1,64}$`；start/end ≤10 年窗口、默认最近 1 年；key 缺失发网前 SOURCE_UNAVAILABLE（配置缺失型）；units/frequency 透传、`.` → null；httptest 单测（key 缺失/合法序列/非法 id/窗口越界）——验证：`go test ./internal/datasources/ -run Fred` 绿
- [x] 1.4 GDELT 取数器：mode/timespan 枚举、query ≤120 字符 + 控制字符拒绝发网前校验；时间线点序列解析、零结果空序列；httptest 单测（合法/非法 mode/超长 query/空结果）——验证：`go test ./internal/datasources/ -run Gdelt` 绿
- [x] 1.5 host 白名单回归测试扩三条（真实 fetcher + catalog，不自带白名单绕过）——验证：`go test ./internal/datasources/ -run HostAllowlist` 绿
- [x] 1.6 probe 与 live smoke 扩源：probe 三新源自动可用；`DATASOURCES_LIVE=1` smoke 加 yahoo/gdelt（匿名），fred 仅 key 已配置时执行——验证：`DATASOURCES_LIVE=1 go test ./internal/datasources/ -run Live -count=1`（有 key 时全过，无 key 时 fred skip）

## 2. 研究白名单（internal/dataenrichment/）

- [x] 2.1 `signalResearchToolNames` 追加三工具名；`CheckCall` 拦截反馈文案从硬编码「四源」改为按白名单动态生成——验证：`go test ./internal/dataenrichment/ -run SignalResearch` 白名单断言七源、越权工具仍拦截
- [x] 2.2 实现前资产反查：`bash scripts/harness/test-assets.sh research-data-sources && bash scripts/harness/test-assets.sh board-signal-reports`，输出清单按 test-cases.md「继承与调整」表逐行处置（旧「四源/仅四源/无价格」断言更新）——验证：反查输出与 test-cases 表行数一致、无漏处置
- [x] 2.3 工具描述组装验证：三新源 description 由 catalog 派生进 `buildToolsDesc`（含能力边界与「非事实核查」声明）；空手源早停 per-source 行为在七源下不变——验证：`go test ./internal/dataenrichment/ -run SignalResearch` 相关用例绿

## 3. as-of 日度适配（internal/dataenrichment/service/signal_compose.go）

- [x] 3.1 「数据可用性」行各源最新期：日度源（yahoo/gdelt）输出 YYYY-MM-DD（取观测最大 date），月度/年度源保持原形态——验证：`go test ./internal/dataenrichment/ -run AsOf` 混合源用例绿
- [x] 3.2 `validateSignalFactsAsOf` 比对：数据可用性行含日度期值时按日粒度精确匹配，纯月度保持月粒度；token 正则不动（已支持双形态）——验证：`go test ./internal/dataenrichment/ -run ValidateSignalFactsAsOf` 日度/月度/混合/编造期值四类用例绿

## 4. FRED key 设置链与前端

- [x] 4.1 后端：`ai_settings` 增 `fred_config`（api_key+enabled），resolver 链 DB>env `FRED_API_KEY`>yaml `fred.api_key`；`POST/GET /api/settings/fred` 脱敏回显/空串保留（照 comtrade handler 模式）——验证：`go test ./internal/datasources/ -run FredConfig` + handler 测试绿
- [x] 4.2 前端：设置页「研究数据源」section 数据驱动七源卡 + FRED key 管理块（AppInput 脱敏回填/AppButton/enabled 开关，照 Comtrade 块）——验证：`pnpm test:unit settings --maxWorkers=2` 七源卡渲染 + fred 无 key disabled 用例绿
- [x] 4.3 信号报告视图零改动确认：as-of 行读已入库 payload（appendix 最大期），日度期值自然渲染——验证：`pnpm test:unit SignalReportView --maxWorkers=2` 既有用例不红

## 5. 测试

- [x] 5.1 影响包全绿：`cd backend-go && go test ./internal/datasources/ ./internal/dataenrichment/`；前端影响文件：`cd front && pnpm test:unit settings SignalReportView --maxWorkers=2`
- [x] 5.2 test-cases.md 主链路全节拍有落点、变体走查表填毕、继承与调整表处置完、效果核对记量化结果
- [x] 5.3 门禁：`golangci-lint run ./...` 与 `go vet ./...` 零新增；`pnpm lint` 零新增

## 6. 文档

<!-- doc-impact: data-enrichment, flow, api, database, configuration -->
<!-- 注：database 域为测试文件 setup 的 AutoMigrate 关键字命中，本 change 零表结构变更（data_sources 启动 upsert 零 DDL、ai_settings 仅新增 fred_config 值 key），database/ 文档无需更新；configuration 域已更新 configuration.md（FRED_API_KEY + 七源速查）；api 域已更新 datasources.md/board-signals.md；flow 域已更新 data-enrichment.md/research-data-sources.md -->

- [x] 6.1 `docs/reference/configuration.md`：「研究数据源」节扩三源口径速查（symbol 枚举/series_id 格式/mode 枚举/典型滞后/单位纪律）+ `FRED_API_KEY` 配置项 + probe 用法——验证：doc-impact verify 通过
- [x] 6.2 `docs/reference/flow/data-enrichment.md`：约束 31「四源工具面」表述更新为七源 + 约束 34 as-of 日度粒度补句 + 变更溯源行（归档时 §12）——验证：doc-impact verify 通过
- [x] 6.3 `docs/reference/api/` 数据源目录/设置端点如有清单页同步七源——验证：grep 一致性校验（`grep -rn '四源' docs/reference/ | grep -v 变更溯源` 无残留活文档表述）

## 7. 验证

- [x] 7.1 `cd backend-go && go build ./...` ——期望：编译通过零错误
- [x] 7.2 `cd backend-go && go test ./internal/datasources/ ./internal/dataenrichment/` ——期望：全绿（含新增 5 个测试文件）
- [x] 7.3 `cd backend-go && golangci-lint run ./... && go vet ./...` ——期望：零新增告警
- [x] 7.4 `cd front && pnpm lint && pnpm exec nuxi typecheck` ——期望：零新增错误
- [x] 7.5 `cd front && pnpm test:unit settings SignalReportView --maxWorkers=2` ——期望：全绿
- [x] 7.6 `curl -s localhost:5100/api/datasources | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('data',d)), [x['code'] for x in d.get('data',d)])"` ——期望：7 与七源 code 全列（起后端后）
- [x] 7.7 `openspec validate add-fast-frequency-datasources` ——期望：通过
