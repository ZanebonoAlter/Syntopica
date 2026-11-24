## 1. 新搜索入口（后端 platform 层）

- [x] 1.1 新建 `backend-go/internal/platform/websearch/` 包：`errors.go`（`ErrorKind` 枚举 + `Error` 结构体 + `Kind()` 方法）、`credentials.go`（`ResolveCredentials` 按 base_url 主机匹配 DeepSeek vendor，找不到返回 `ErrNotConfigured`）、`search.go`（`Search(ctx, query) ([]Result, error)`，POST `/v1/tools/search`，`count=10`、`need_content=false`，解析 `results[].title/url/summary/rerank_score/site_name/date/authority`，摘要截 400 runes）；验证：`go build ./internal/platform/websearch/` 通过
- [x] 1.2 新建 `websearch/health.go`：`ProbeHealth(ctx)` 复用 `Search`，固定探针词，映射四态 + `error_kind`；验证：`go vet ./internal/platform/websearch/` 通过
- [x] 1.3 写 `websearch/search_test.go`：用 `httptest.Server` 覆盖 200 有结果 / 200 空 / 401→auth / 402→quota / 403 余额文案→quota / 403 其他→auth / 429→rate_limited / 500→unavailable / 坏 JSON→unavailable / 空查询词拒绝 / 超时→timeout；验证：`go test ./internal/platform/websearch/` 全绿
- [x] 1.4 写 `websearch/credentials_test.go`：sqlite 内存库验证命中已启用 vendor、跳过未启用、跳过空 key、跳过非 deepseek 主机、多条取 id 最小、无命中返回 ErrNotConfigured；验证：同上
- [x] 1.5 写 `websearch/health_test.go`：四态各一例 + auth/quota 归 unreachable 并带 error_kind；验证：同上

## 2. 页边注链路切换（后端 topicgraph 层）

- [x] 2.1 `margin_notes_websearch.go`：`marginNoteWebSearchFn` 改调 `websearch.Search`（不再传 endpoint），预检由「endpoint 为空」改为 `errors.Is(err, websearch.ErrNotConfigured)`；`Result` 消费 `Score`；验证：`go build ./internal/topicgraph/...` 通过
- [x] 2.2 `margin_notes_websearch.go`：合并逻辑改为 URL trim 去重 + `sort.SliceStable` 按 Score 降序 + 截前 5；验证：`go vet` 通过
- [x] 2.3 `margin_notes_qa.go` 单 query 回退路径同步改调 `websearch.Search`；验证：`go build` 通过
- [x] 2.4 更新 `margin_notes_qa_test.go`：mock 的搜索函数签名跟随变更；新增「跨 query 合并按相关度重排」用例（Scenario 同名）与「摘要截 400」用例；验证：`go test ./internal/topicgraph/service/ -run 'MarginNote|TestMergeSearchResults|TestFilterWebSourcesAgainstCandidates|TestFilterCitedAgainstCandidates|TestParseMarginNoteQA'` 全绿（纯函数用例名不含 MarginNote）
- [x] 2.5 更新 `margin_notes_handler_test.go` 中涉及搜索状态的断言（若有）；验证：`go test ./internal/topicgraph/handler/ -run 'Annotation|MarginNote'` 全绿（用例名全部含 Annotation）

## 3. 设置 API 与配置清理（后端 discovery/admin 层）

- [x] 3.1 `discovery/routes.go`：移除 `/searxng`、`/searxng/health` 三条路由，新增 `GET /web-search`、`GET /web-search/health`（无 POST，凭据不可在面板编辑）；验证：`go build ./internal/discovery/...` 通过
- [x] 3.2 `discovery_handler.go`：删除 `GetSearxngSettings`/`SaveSearxngSettings`/`GetSearxngHealth`，新增 `GetWebSearchSettings`（返回 `{vendor_configured, vendor_name}`）与 `GetWebSearchHealth`（调 `websearch.ProbeHealth`）；验证：`go build` 通过
- [x] 3.3 删除 `backend-go/internal/platform/searxng/` 整个目录；删除 `aisettings/config_store.go` 中 `searxngConfigKey`、`LoadSearxngConfig`、`SaveSearxngConfig`；删除 `config/config.go` 中 `Searxng` 结构体字段、`SEARXNG_URL` 绑定与 yaml 读取；验证：`go build ./...` 通过，`rg -i searxng backend-go --type go` 实际余 4 处均为 `add-searxng-health-indicator` 归因注释（`margin_notes_qa.go:62` 折行 / `annotation_qas_web_search_status_migration_test.go:17` / `postgres_migrations.go:4170,4181`），按口径视为零残留
- [x] 3.4 删除 `discovery/handler/searxng_health_handler_test.go`，新建 `websearch_health_handler_test.go` 覆盖 `ok` / `degraded` / `unreachable+auth` / `unreachable+quota` / `disabled`；验证：`go test ./internal/discovery/handler/ -run WebSearch` 全绿
- [x] 3.5 `backend-go/internal/platform/database/annotation_qas_web_search_status_migration_test.go` 与 `margin_notes_web_sources_migration_test.go` 若引用 `searxng` 包则改 import；验证：`go test ./internal/platform/database/ -run 'MarginNote|WebSearchStatus'` 全绿（实施结论：零改动——两个迁移测试只有历史 change 名注释，无 searxng 包 import）
- [x] 3.6 `backend-go/internal/platform/config/config.go` 单测若覆盖 `searxng.endpoint` 加载，删除对应用例；验证：`go test ./internal/platform/config/` 全绿（实施结论：零改动——config 测试无 searxng 用例）
- [x] 3.7 `websearch` 包收尾（承接 review R-8 / R-10 / R-14）：注释里「沿用 searxng 取值」类悬空引用改为不带包名的措辞；`health.go` 的 `unreachableDetail` 在 `Unavailable` 默认分支改固定文案、响应体摘要只进日志（同步调整断言）；`credentials_test.go` 加 `require.Equal(airouter.ProviderTypeOpenAICompatible, providerTypeOpenAICompatible)` 漂移守卫（若 import airouter 触发 depguard 或初始化负担则汇报后放弃）；验证：`go test ./internal/platform/websearch/ -count=1` 全绿 + `golangci-lint run ./internal/platform/websearch/...` 0 issues
- [x] 3.8 `backend-go/.golangci.yml` 共享模型白名单注释补 `AIVendor` 一行（承接 review R-15；仅注释；该文件若已被其他 active change 弄脏则不改并在汇报中说明）；验证：`git diff --stat -- backend-go/.golangci.yml` 至多 1 行变更

## 4. 前端：API 层与设置面板

- [x] 4.1 新建 `front/app/api/webSearch.ts`（`useWebSearchApi`：`getSettings` / `getHealth`）与 `webSearch.test.ts`；删除 `front/app/api/searxng.ts` 及其测试；更新 `front/app/api/index.ts` 导出；验证：`pnpm test:unit webSearch.test.ts --maxWorkers=2` 全绿
- [x] 4.2 新建 `front/app/components/dialog/WebSearchConfigPanel.vue`：无表单，展示关联状态（`vendor_configured` + `vendor_name`）、空态指引文案、「测试连接」按钮、四态徽标（复用现有 `healthStatusMeta` 样式类）与 `error_kind` 针对性文案；验证：`pnpm exec nuxi typecheck` 通过
- [x] 4.3 新建 `WebSearchConfigPanel.test.ts`：四态徽标各一例、`vendor_configured=false` 空态、测试连接 loading 防重复、`error_kind=quota` 文案；验证：`pnpm test:unit WebSearchConfigPanel.test.ts --maxWorkers=2` 全绿
- [x] 4.4 `SettingsSectionDatasourcesNetwork.vue`：tabs 中 `searxng` 项替换为 `web-search`（label「网页搜索」）；删除 `SearxngConfigPanel.vue` 与其测试；更新 `SettingsSectionDatasourcesNetwork.test.ts` 的 tabs 断言；验证：`pnpm test:unit SettingsSectionDatasourcesNetwork.test.ts --maxWorkers=2` 全绿
- [x] 4.5 `SettingsWorkspace.vue`：两级旧深链重定向（旧 section 键 `searxng` → 本分区 `web-search` tab；`tab=searxng` → `tab=web-search`），分区描述文案「SearXNG」改「网页搜索」；更新 `SettingsWorkspace.test.ts` 追加旧深链重定向用例；验证：`pnpm test:unit SettingsWorkspace.test.ts --maxWorkers=2` 全绿
- [x] 4.6 `front/app/api/marginNotes.ts` 中若有 searxng 类型/函数引用则清理；更新 `marginNotes.test.ts`；验证：`pnpm test:unit marginNotes.test.ts --maxWorkers=2` 全绿（实施结论：无需改动——全仓无 searxng 类型 / 函数引用，仅历史 change 名注释）
- [x] 4.7 审查返修（R-20）：`WebSearchConfigPanel.vue` 与 `webSearch.ts` 注释里的分区名「AI 路由」→「AI 模型」（`section=ai-providers`）；`error_kind` 为 `auth` / `quota` 时在状态行旁补一行去向提示（纯文字，不新造跨分区跳转控件）；同步测试断言；验证：`pnpm test:unit WebSearchConfigPanel.test.ts --maxWorkers=2` 全绿
- [x] 4.8 审查返修（R-22）：`LEGACY_TAB_REDIRECT` 抽到 `front/app/features/settings/legacyRedirects.ts` 作单一来源，`SettingsWorkspace.vue` 与 `SettingsSectionDatasourcesNetwork.vue` 改为 import，删除两份字面拷贝；验证：`pnpm test:unit SettingsWorkspace.test.ts SettingsSectionDatasourcesNetwork.test.ts --maxWorkers=2` 全绿
- [x] 4.9 审查返修（R-23 / R-24）：`degraded` 色点改用 `var(--color-warning, #c98a2e)`；`vendor_configured=true` 且 `vendor_name` 为空时兜底显示「（未命名）」；验证：`pnpm test:unit WebSearchConfigPanel.test.ts --maxWorkers=2` 全绿
- [x] 4.10 审查返修（R-25 / R-28）：`WebSearchConfigPanel.test.ts` 把四态与 `error_kind` 的循环断言拆为独立用例，补 D6 守卫断言（面板不含 `input` / 开关组件 / `form`）；`SettingsWorkspace.test.ts` 加 `vi.mock('@iconify/vue')`（参照同目录 `SettingsSidebar.test.ts`）消除真实出网；验证：`pnpm test:unit WebSearchConfigPanel.test.ts SettingsWorkspace.test.ts --maxWorkers=2` 全绿

## 5. 前端：页边注卡三态标记

- [x] 5.1 `MarginNoteCard.vue`：在 `web_search_status==='empty'` 分支旁增加 `ok` 分支，按 `cited_web_sources.length` 渲染「已联网」（`data-testid="mn-web-ok"`）/「已联网 · 未采用」（`data-testid="mn-web-unused"`，title 提示）；复用现有弱化标记 class 族；验证：`pnpm exec nuxi typecheck` 通过
- [x] 5.2 `MarginNoteCard.test.ts`：新增 `ok+有引用`→`mn-web-ok`、`ok+无引用`→`mn-web-unused`、`empty`→`mn-web-search-empty`（现有）、`disabled`/NULL→无标记 四类断言；验证：`pnpm test:unit MarginNoteCard.test.ts --maxWorkers=2` 全绿

## 6. 文档

<!-- doc-impact: flow, api, architecture, configuration, database, deployment -->

触及 `flow/daily-report.md`：归档后按 §12.2 补「变更溯源」链接（见 6.3）。

- [x] 6.1 修订 `docs/reference/flow/daily-report.md` 约束 25 / 27 与代码入口（约 350、353、411、422 行）：SearXNG 措辞 → 统一网页搜索入口（`platform/websearch`，DeepSeek Search API）；约束 27 的渲染规则改为三态（`empty`→联网搜索无结果；`ok` 且有网络来源→已联网；`ok` 且无网络来源→已联网 · 未采用；`disabled`/NULL 不渲染）；落点 `platform/searxng/*` → `platform/websearch/*`、`SearxngConfigPanel.vue` → `WebSearchConfigPanel.vue`、路由 → `GET /api/settings/web-search/health`；约束 25 补「跨 query 按 rerank_score 重排、每条摘要截 400 runes」；红线句加粗格式与 change 归因写法遵循 `docs/reference/standard/shared/doc-authoring.md`；验证：`rg -n "searxng" docs/reference/flow/daily-report.md` 只剩变更溯源表的历史行
- [x] 6.2 修订其余文档的 searxng 段（落点清单见 explore-findings.md「红线文档落点」）：`docs/reference/api/discovery.md`（「SearXNG 配置与健康探测」一节 → 网页搜索：两个 GET 路由、请求/响应形状、四态 + `error_kind`）、`docs/reference/api/daily-reports.md`（约 36、1121、1137 行）、`docs/reference/configuration.md`（删 `SEARXNG_URL` 行）、`docs/reference/architecture/backend.md`（约 127、199 行）、`docs/reference/database/tables/margin-notes.md`（约 33 行措辞；34 行迁移归属 `add-searxng-health-indicator` 为历史事实保留）、`docs/reference/flow/data-enrichment.md`（约 284 行约束 10 中对 SearXNG 的举例措辞）、`docs/reference/flow/ai-summary.md`（仅历史行则不动）；验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/switch-margin-note-search-to-deepseek` 通过
- [x] 6.4 构建限载固化（用户 2026-11-24 指示「限制作为标准」）：`scripts/dev/deploy-frontend.sh` 的构建步包进 `run_guarded`（`nice -n 19` + `NODE_OPTIONS=--max-old-space-size=3584` + 每 20s 采样，`MemAvailable<350MB` 或 `load1>30` 才中止且只杀构建进程组）；`docs/reference/deployment.md` §本地裸跑静态托管 补「构建限载已内置」与手工构建口径；验证：`bash -n scripts/dev/deploy-frontend.sh` 通过、看门狗函数独立跑通（子进程 exit 0）、实际部署一次成功（2026-11-24 10:51~10:54）
- [x] 6.3 `docs/reference/flow/daily-report.md`「变更溯源」表追加本 change 行（**归档后**按 §12.2 执行，由主线程落行；归档位置列含 `<date>-switch-margin-note-search-to-deepseek`，链接相对路径 `../../../openspec/changes/archive/<date>-switch-margin-note-search-to-deepseek`）；验证：`bash scripts/harness/check-standards.sh` E 段零失败

## 7. 测试

- [x] 7.1 后端影响包：`cd backend-go && go test ./internal/platform/websearch/ ./internal/topicgraph/service/ ./internal/topicgraph/handler/ ./internal/discovery/handler/ ./internal/platform/aisettings/ ./internal/platform/config/ -count=1`（topicgraph service 包用 `-run 'MarginNote|TestMergeSearchResults|TestFilterWebSourcesAgainstCandidates|TestFilterCitedAgainstCandidates|TestParseMarginNoteQA'`——纯函数用例名不含 MarginNote，窄过滤会漏；topicgraph handler 包用 `-run 'Annotation|MarginNote'`（页边注用例名全部含 Annotation）；discovery handler 加 `-run WebSearch`）及 `go test ./internal/platform/database/ -run 'MarginNote|WebSearchStatus' -count=1`；`go vet` 与 `golangci-lint run` 仅跑上述包；验证：零失败
- [x] 7.2 前端影响文件：`cd front && pnpm test:unit MarginNoteCard.test.ts WebSearchConfigPanel.test.ts webSearch.test.ts SettingsSectionDatasourcesNetwork.test.ts SettingsWorkspace.test.ts marginNotes.test.ts --maxWorkers=2`；`pnpm exec eslint <本 change 改动的前端文件>`；`pnpm exec nuxi typecheck`；验证：零失败（辖区外文件的红登记为域外红，见 8.5）
- [x] 7.3 以 `bash scripts/harness/change-scope.sh` 判定影响包，若多于 7.1 / 7.2 所列则补跑；验证：判定输出与实际跑过的命令写进「验证」节

## 8. 验证

- [x] 8.1 `bash scripts/harness/check-standards.sh --change switch-margin-note-search-to-deepseek`：A-D / F / G 段零失败（E 段归档后校验）
- [x] 8.2 `bash scripts/harness/doc-impact.sh verify openspec/changes/switch-margin-note-search-to-deepseek` 零失败
- [x] 8.3 `bash scripts/harness/review-trace.sh openspec/changes/switch-margin-note-search-to-deepseek` 退出码 0，且 High 条目状态均非「待处理」
- [x] 8.4 API 主线实跑（需后端已重启到新代码，由主线程执行）：`GET /api/settings/web-search` 返回 `vendor_configured=true` 与 `vendor_name`；`GET /api/settings/web-search/health` 返回 `status=ok` 与耗时；`GET /api/settings/searxng` 返回 404；在日报批注处提问一次后 `annotation_qas.web_search_status` 落 `ok`/`empty` 之一，且 `ai_call_logs` 新增两行 operation（`daily_report.margin_note_search_query` / `daily_report.margin_note_qa`）；实际结果记录在本行下方
  - **实测（2026-11-24，重启后）**：`GET /api/settings/web-search` → `{vendor_configured:true, vendor_name:"DeepSeek"}`；`GET /api/settings/web-search/health` → `{status:"ok", latency_ms:440, detail:"搜索正常，返回 10 条结果"}`（SearXNG 同场景 4.03s）；旧路由 `GET|POST /api/settings/searxng`、`GET /api/settings/searxng/health` 三条均 404。**提问链路（20:02 模型主机恢复后补跑）**：`POST /api/annotations/26/questions` → `web_search_status=ok`、`cited_web_sources` 1 条（CRI 国际在线）、`cited_article_ids=[168556]`，落库 `annotation_qas.id=69`；`ai_call_logs` 两行成功（`margin_note_search_query` 489ms / `margin_note_qa` 1305ms）。浏览器复查卡片显示「已联网」+「网络来源」，截图 `ui-verification/margin-note-web-status.png`。
- [x] 8.5 域外红登记：7.1 / 7.2 撞见的非本 change 红测试用 `bash scripts/harness/test-patrol.sh --register <test_id> --context switch-margin-note-search-to-deepseek` 登记台账；验证：验证节写明「域外红已记账」或「无域外红」
- [x] 8.6 部署与走查（**需用户确认部署时点**——工作树含其他 active change 的未提交前端改动，静态部署会把它们一并构建上线；由主线程执行）：`bash scripts/dev/deploy-frontend.sh`；浏览器走查：设置 → 数据源与网络 → 「网页搜索」tab 存在且无「SearXNG」tab；旧深链 `?section=datasources-network&tab=searxng` 与 `?section=searxng` 均落到网页搜索；「测试连接」展示四态徽标；日报批注提问后卡片三态标记正确；走查记录写入 `ui-design.md` §6

### 实测结果（2026-11-23，树莓派本机，load < 5 串行执行）

| # | 命令 | 结果 |
| --- | --- | --- |
| 7.1 | `go test ./internal/platform/websearch/ -count=1` | ok 0.988s；顶层 22 选中 / 22 PASS |
| 7.1 | `go test ./internal/topicgraph/service/ -run '<宽模式>' -count=1` | ok；20 顶层选中 / 20 PASS（含 `TestMergeSearchResults_RerankAcrossQueries`、`TestAskMarginNote_WebSummaryEntersPromptUntruncated`、`TestAskMarginNote_WebSearchStatus_ThreeStates`） |
| 7.1 | `go test ./internal/topicgraph/handler/ -run 'Annotation\|MarginNote' -count=1` | ok；7 选中 / 7 PASS（`-run MarginNote` 会空跑，见 R-18） |
| 7.1 | `go test ./internal/discovery/handler/ -run WebSearch -count=1` | ok；5 选中 / 5 PASS |
| 7.1 | `go test ./internal/platform/aisettings/ ./internal/platform/config/ -count=1` | ok 9.367s / 0.009s |
| 7.1 | `go test ./internal/platform/database/ -run 'MarginNote\|WebSearchStatus' -count=1` | ok 38.7s；2 PASS |
| 7.1 | `go vet` + `golangci-lint run`（上述 6 包） | exit 0 / 0 issues |
| 7.2 | `pnpm test:unit <6 文件> --maxWorkers=2`（返修后复跑） | 8 files / 84 tests passed（含返修新增用例） |
| 7.2 | `pnpm exec eslint <本 change 前端文件>` | .ts 0 problems；4 个 .vue 报「no matching configuration」属仓库 eslint 不覆盖 .vue（存量，非本 change） |
| 7.2 | `pnpm exec nuxi typecheck` | exit 0（仅 node 26 `DEP0205` 提示） |
| 7.3 | `bash scripts/harness/change-scope.sh` | 工作树 1396 文件跨多个 active change 无法隔离；逐个核对辖区后判定无遗漏包（新增 `platform/websearch` 已跑、`internal/discovery` 无测试文件） |
| 8.1 | `bash scripts/harness/check-standards.sh --change switch-margin-note-search-to-deepseek` | 通过 325 / 失败 0（A–I 段；E 段归档后校验） |
| 8.2 | `bash scripts/harness/doc-impact.sh verify <change>` | 通过（声明 flow, api, architecture, configuration, database；文件 9 个） |
| 8.3 | `bash scripts/harness/review-trace.sh <change>` | 通过（34 条 R 条目；High 级 0 条，无「待处理」） |
| 8.5 | 域外红 | 无域外红（7.1 / 7.2 零失败，未触发 test-patrol 登记）；两条 gofmt 项归因：`internal/admin/handler/session_handler_test.go` 为 HEAD 存量未格式化、`job_firecrawl_detection_test.go` 为其他 change 的未跟踪新文件 |

注：8.1 / 8.2 / 8.3 与后端 7.1 的四条命令由主线程在 M6 汇报后**独立复跑**确认，非引用子线程结果。
