# Tasks

> 用例先行：各源解析器任务按「测试用例（翻译自 energy-mcp 口径）与实现交织」执行，fixture 从 `tools/energy-mcp/tests/` 样本与 `openspec/changes/archive/2026-09-19-connect-dsh-energy-data-sources/evidence/` 裁剪。测试只跑影响包（`bash scripts/harness/change-scope.sh` 判定）。

## 1. 域骨架与基础设施

- [x] 1.1 建 `internal/datasources/` 包骨架（`sources/`、`errors.go` typed error 三态 InvalidArgument/SourceUnavailable/SchemaChanged、`catalog.go` 四源目录常量）——验证：`go build ./internal/datasources/...` 通过，常量含 spec 目录全字段
- [x] 1.2 HTTP 预算层 `fetch.go`（host 白名单、有界重定向仅 https、45s/32MB 流式预算、HTML 检测）——验证：httptest 表驱动单测覆盖白名单拒绝/超预算终止/重定向边界/HTML 冒充（`go test ./internal/datasources`）
- [x] 1.3 TTL 缓存 `cache.go`（900s、错误不缓存、命中保留原 retrieved_at、驱逐接口）——验证：单测覆盖 TTL 过期/错误不进缓存/驱逐后重取（同包 go test）

## 2. 目录表与查询 API

- [x] 2.1 `data_sources` GORM model + AutoMigrate 注册 + 启动 upsert seed（以 code 幂等，静态属性快照 + 运行时状态字段）——验证：迁移后表存在四行 seed；重复启动不重复插入（单测或本地库人工验证）
- [x] 2.2 `config.go` 增 `Comtrade.APIKey`（`comtrade.api_key` + env `COMTRADE_API_KEY` 覆盖，照 BOCHA_API_KEY 风格）——验证：配置加载单测（默认空/env 覆盖/trim）
- [x] 2.3 handler `GET /api/datasources`（读表全量返回；Comtrade 无 key 时该行 disabled+原因）——验证：handler 单测（mock repo）覆盖四源返回与 disabled 分支

## 3. EIA WPSR 取数器（口径翻译自 energy-mcp）

- [x] 3.1 测试先行：`eia_test.go` 用例翻译（stocks 三类库存区分含 SPR/不含、supply 产量/进口/出口、en-dash 对→null、千分位逗号、M/D/YY 周结束日期、单位 MMbbl/Mb/d 固定、列漂移/重复列/未知标记→SCHEMA_CHANGED、成功结果带溯源元数据）——验证：用例文件就绪且红
- [x] 3.2 实现 `sources/eia.go`（CP1252 解码、按表头日期列索引定位当周/上周、B 区行分组前缀与脚注号剥离）——验证：`go test ./internal/datasources -run TestEIA` 全绿

## 4. JODI 取数器（口径翻译自 energy-mcp）

- [x] 4.1 测试先行：`jodi_test.go` 用例翻译（七列、三缺失标记 null+保留、unit 规则默认/拒绝库存+KBD/不换算、CONVBBL 不当流量值、month 缺省取最新期即使 null 不回退、本年 404 回退上一年一次且注明 year_used、参数校验 INVALID_ARGUMENT 不发网络请求）——验证：用例就绪且红
- [x] 4.2 实现 `sources/jodi.go`——验证：`go test ./internal/datasources -run TestJODI` 全绿

## 5. 世行 WDI 取数器（新源）

- [x] 5.1 测试先行+实现 `sources/wdi.go`（匿名 JSON API、指标+ISO3 多国+年份范围、非法参数本地校验不发请求、观测值含 null 期、lastupdated 透传）——验证：`go test ./internal/datasources -run TestWDI` 全绿（httptest mock 2026-09-19 实测响应形态）

## 6. UN Comtrade 取数器（新源，需 key）

- [x] 6.1 测试先行+实现 `sources/comtrade.go`（reporter/partner/HS/flow/period 参数、key 从 Config 注入、key 缺失→SourceUnavailable 且消息指向配置项、响应字段按官方文档建模 mock、限速/429 映射）——验证：`go test ./internal/datasources -run TestComtrade` 全绿（含 key 缺失路径真实测，无 key 可测）

## 7. probe 端点

- [x] 7.1 `POST /api/datasources/{code}/probe`（各源预定义最小参数集真实取数、返回摘要+耗时+retrieved_at、错误映射 400/502+error_code、结果不写目录状态）——验证：handler 单测（mock fetcher）覆盖成功/参数非法/上游失败三分支

## 8. Tool 适配与装配

- [x] 8.1 `datasources/tools.go`：四源产出 `dataenrichment/service.Tool`（map 参数→强类型转换、typed error→错误 JSON）；Registry 增最小动态注册口——验证：适配单测（参数转换/错误透传/名称与 InputSchema 无任意 URL 参数）
- [x] 8.2 `wire.go` 装配注入 + **现有工具面不变断言**——验证：dataenrichment 既有测试全绿（`go test ./internal/dataenrichment/...`），新增一条断言用例证明注入后探索/QA 的 allowedTools 清单不含数据源工具

## 9. 文档

<!-- doc-impact: flow, api, database, architecture, configuration -->

- [x] 9.1 `docs/reference/configuration.md` 增「研究数据源」节：四源口径速查（单位/频度/滞后/覆盖）、`COMTRADE_API_KEY` 配置、probe 用法、无 key 降级语义——验证：doc-impact verify 通过
- [x] 9.2 `tools/energy-mcp/README.md` 顶部加一行声明：已停止演进、产品口径以 `internal/datasources` 为准——验证：grep 命中
- [x] 9.3 `docs/reference/api/datasources.md` 新建（目录/probe 端点契约）+ `_index.md` 注册——验证：doc-impact verify
- [x] 9.4 `docs/reference/database/tables/research-data-sources.md` 新建（data_sources 表投影）+ `_index.md` 注册——验证：doc-impact verify
- [x] 9.5 `docs/reference/architecture/map.md` 增第 10 域行——验证：grep「研究数据源」
- [x] 9.6 `docs/reference/flow/research-data-sources.md` 新建（五位一体新域文档，含约束节与溯源表）——验证：check-standards 死链检查

## 10. 测试

- [x] 10.T1 单元全量（影响包）：`cd backend-go && go test -short ./internal/datasources/... ./internal/dataenrichment/ ./internal/dataenrichment/service/` → ok 全绿（2026-09-19 实测；cache/fetch/eia/jodi/wdi/comtrade/wiring/registry 共 40+ 用例）
- [x] 10.T2 真联网 smoke：`DATASOURCES_LIVE=1 COMTRADE_API_KEY=<key> go test ./internal/datasources/sources/ -run TestLiveSmoke -v` → 四源全 PASS（2026-09-19 实测 93.45s；EIA 1.3s / JODI 大文件 120s 预算内成功 / WDI 1.7s / Comtrade 2.2s 真实 key 取数非空）
- [x] 10.T3 lint/vet/build 全量：`golangci-lint run ./...`（0 issues）+ `go vet ./...` + `go build ./...`（2026-09-19 实测全绿）

## 11. 验证

| Scenario | 测试文件 |
| --- | --- |
| 目录查询四源 + disabled 语义 | internal/datasources/handler 测试 |
| EIA stocks/supply/漂移 | internal/datasources/eia_test.go |
| JODI 缺失/单位/404 回退 | internal/datasources/jodi_test.go |
| WDI 多国/非法参数/lastupdated | internal/datasources/wdi_test.go |
| Comtrade key 缺失显式报配置 | internal/datasources/comtrade_test.go |
| 三错误码可区分 + SCHEMA_CHANGED 驱逐缓存 | 各源测试 + cache_test.go |
| 预算/白名单/HTML 冒充 | internal/datasources/fetch_test.go |
| 缓存命中保留 retrieved_at | internal/datasources/cache_test.go |
| probe 成功/失败带原因 | internal/datasources/handler 测试 |
| 注入后现有工具面不变 | internal/dataenrichment/service 新增断言用例 |

- [x] 10.1 `bash scripts/harness/change-scope.sh` 判定影响包并只跑影响包测试；`golangci-lint run ./...` + `go vet ./...` + `go build ./...` 全绿
- [x] 10.2 真联网 smoke（opt-in `DATASOURCES_LIVE=1`）：EIA/JODI/WDI 匿名三源实测取数非空；Comtrade 仅 key 已配置时执行——验证：smoke 测试输出含各源 retrieved_at 与关键数值
- [x] 11.1 `openspec validate integrate-research-data-sources`（valid）+ `bash scripts/harness/doc-impact.sh verify openspec/changes/integrate-research-data-sources`（通过：声明 flow/api/database/architecture/configuration，文件 5 个）+ `bash scripts/harness/check-standards.sh --change integrate-research-data-sources`（本 change 相关全过：184/190；**E 段 6 个失败均为 2026-09-17 他人已归档 change 的存量溯源欠账**（dev-process-guard/fix-bulk-markall-all-scope/fix-injection-transition-fingerprint-wipe/harness-retro-loop/linux-native-dev-environment/per-session-constraint-binding），属他人 change 的文件、本 change 不修改，不宣称全局门禁全绿——与 configure-dsh-energy-research 先例同口径）——三条命令 2026-09-19 实测

## 12. 迭代：Comtrade key 界面配置（bocha 同款，2026-09-19 追加）

- [x] 12.1 `aisettings` 增 `comtrade_config` Load/Save 对（对齐 bocha_config 语义）
- [x] 12.2 admin 设置 API `GET/POST /api/settings/comtrade`（脱敏回显、空串不清 key、enabled 指针语义）+ wire/routes 注册
- [x] 12.3 key 解析动态化：`wiring.ComtradeKeyResolver()`（界面 DB > env > config.yaml），`sources.NewComtrade` 改收 provider、目录 List 现算 requires_key 源 status（免重启翻转）
- [x] 12.4 前端设置页「研究数据源」section：四源状态卡（频度/滞后/禁用原因/测试连通 probe）+ Comtrade key 管理（脱敏提示、留空不改），`SettingsWorkspace`/`pages/settings.vue`/`api/index.ts` 注册
- [x] 12.5 测试：settings roundtrip（脱敏/空串保留/禁用跳过）、resolver 链（DB 胜/禁用跳过/空白视为未配）、目录动态 status；全绿
- [x] 12.6 文档：configuration.md（三级优先级）、api/datasources.md（settings 端点+动态 status）、ui-design.md（minor 复用契约）、spec 增「Comtrade key 界面配置」Requirement
