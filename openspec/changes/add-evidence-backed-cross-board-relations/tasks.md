## 1. 开工约束与数据基础

- [x] 1.1 执行 `bash scripts/doc-impact.sh suggest` 并把建议文档域与本文件文档声明对齐；验收：命令退出 0，差异域至少覆盖 data-enrichment、semantic-board、API、database、configuration，业务约束由 constraint-injection 自动注入。
- [x] 1.2 新增 relation run 与 relation lifecycle 模型、PostgreSQL 迁移/索引、稳定 suggestion hash 和 repository；验收：`cd backend-go && go test ./internal/dataenrichment/repository -run 'TestCrossBoardRelation'` 退出 0，并覆盖部分唯一索引、事务裁决、冷却和过期边界。
- [x] 1.3 扩展版块关系发现配置（自动开关默认 false、source/search/fetch/loop/timeout 预算、解析门槛与 margin、dismiss 冷却、确认关系 TTL），接入后端配置读写与前端类型；验收：配置 handler/service 测试证明旧版块缺字段时自动发现关闭且预算采用默认值。

## 2. 跨版块调查动态授权纵向切片

- [x] 2.1 实现 `search_internal_context` 紧凑混合检索，返回真实 board/lane ID、归属、标题和预算内摘要，不返回完整时间线/lifeline；验收：`go test ./internal/dataenrichment/service -run 'TestSearchInternalContext'` 退出 0，覆盖空库、归档对象、重复对象和超长概念。
- [x] 2.2 为共享工具循环增加可选的 `BeforeToolCall`/`AfterToolResult` 策略，实现每次调查独立的 `DynamicLaneGrantSet`，仅父简报和可信工具结果可授权；验收：`go test ./internal/dataenrichment/service -run 'TestDynamicGrant|TestRunToolLoop'` 退出 0，旧调用方测试保持通过。
- [x] 2.3 将 grant audit、跨版块 board/lane 归属写入调查不可变快照，并在综合 sanitize 阶段二次剔除未授权、已删除或归属漂移引用；验收：`go test ./internal/dataenrichment/service -run 'TestBoardInvestigationCrossBoard'` 退出 0，失败链路保持 0 半成品行且父数据未改写。
- [x] 2.4 扩展调查 API DTO 与 `BoardInvestigationReport`，明确展示跨版块引用的所属版块和证据用途；验收：对应 Go handler 测试与 `BoardInvestigationReport` 组件测试退出 0，旧报告缺 board ID 时仍可读取。

## 3. 证据优先的发现、解析与盲验纵向切片

- [x] 3.1 实现 source snapshot 校验与 Scout 阶段：服务端从 parent brief 重取 observation/question，生成预算化搜索计划，调用现有 raw Bocha/fetch 工具并保存 run tool calls/gaps；验收：`go test ./internal/dataenrichment/service -run 'TestRelationScout|TestRelationSource'` 退出 0，客户端伪造 source、博查失败和网页指令注入均不会生成 supported。
- [x] 3.2 实现 target concept 保守解析器：词法+现有向量候选、top-K、门槛与 top-1/top-2 margin、resolved/ambiguous/no_match 结果和版本化 mapping snapshot；验收：`go test ./internal/dataenrichment/service -run 'TestResolveTarget'` 退出 0，精确覆盖 threshold/margin 两端及稳定排序。
- [x] 3.3 实现独立 verifier operation/session、通用关系/结论枚举、支持/反证/替代解释输入和 evidence quote 保守核对；验收：`go test ./internal/dataenrichment/service -run 'TestRelationVerifier|TestRelationEvidence'` 退出 0，共同驱动材料不得被清洗为 causal，幽灵 quote 不计入证据。
- [x] 3.4 编排 Scout→Resolve→Verify→Persist：rejected 只留 run，ambiguous/no_match/insufficient 落 unresolved，resolved+supported 最多落 proposed；验收：`go test ./internal/dataenrichment/service -run 'TestRelationDiscoveryPipeline'` 退出 0，run 中途失败可审计且不产生半成品有效关系。

## 4. 手动任务、审阅与裁决纵向切片

- [x] 4.1 新增手动 trigger、job status、关系 list/detail/re-resolve API，复用版块异步任务的 202/409 和 job kind 轮询约定；验收：`go test ./internal/dataenrichment/handler -run 'TestRelationDiscovery'` 退出 0，覆盖 observation/question、跨 board 父简报、重复提交、过滤和追溯。
- [x] 4.2 新增 confirm/dismiss API 和 repository 状态机，确认前重验目标/证据/有效期，非法转换返回 409，dismiss 写理由并进入冷却；验收：handler + testcontainer repository 测试覆盖全部合法/非法转换和并发幂等。
- [x] 4.3 增加读取时即时过期过滤与维护任务批量 `confirmed → expired`；验收：repository 测试在 `expires_at` 前一刻、等于和后一刻分别得到有效、无效、无效结果，维护任务可重复运行。

## 5. 确认关系注入下一份简报纵向切片

- [x] 5.1 在 `EnrichBoard` 生成前读取 source/target 相关的有效 confirmed 关系，按 `quality_grade DESC, confirmed_at DESC, id ASC` 和条数/字符预算机械选取；验收：`go test ./internal/dataenrichment/service -run 'TestBoardBriefConfirmedRelations'` 退出 0，混合状态和预算边界选择稳定。
- [x] 5.2 将选中关系渲染为明确标注的外部背景，并冻结 relation IDs、证据 refs、截断数到 input snapshot；新增服务端机械 `cross_board_relations` 输出字段，不放宽原 `relationships` 的 active-lane 校验；验收：同一测试证明 tool calls 仍为空、web/fetch 调用为 0、旧简报不被状态变化改写。
- [x] 5.3 扩展前端 brief API 类型与报告展示，将本版块 relationships 和已确认跨版块关系分区并提供详情跳转；验收：组件测试覆盖无关系、有效关系、过期关系被后端排除及超长 claim 展示。

## 6. 关系建议前端纵向切片

- [x] 6.1 在版块增强 composable 增加关系任务轮询、列表/详情、confirm/dismiss/re-resolve 状态，并复用 board view epoch 守卫；验收：`useBoardRelations` 测试覆盖切版块迟到响应、重复提交、202/409、失败和空态。
- [x] 6.2 在 observation 和 research question 上增加“发现关联”动作，并实现关系建议列表/详情面板，分区展示 mapping、支持证据、反证、gap 和生命周期；验收：`BoardRelationPanel` 组件测试覆盖加载态、空态、错误态、空 dismiss 理由、超长文本和操作反馈。
- [ ] 6.3 用真实 Chrome 走通“从 observation 发起→轮询完成→查看外部证据→确认→生成下一份简报看到独立跨版块字段”的主链路；验收：按 `ui-verify` skill 使用 opencli 留存命令/结果，页面切换后无旧版块状态串台。
  - **豁免留痕（2026-09-04，用户批准归档）**：opencli 前端主链路未执行。当日完成替代验证——修复博查结构解析 bug（web_search.go `data.result`→`data.webPages.value`）与 run 状态写库 bug（relation_discovery.go []byte→string）后，以真实后端 + 真实博查 + 真实数据库跑通完整后端链路：POST relations/discover 202 → scout plan/extract（博查真实搜索结果）→ resolve 保守解析 → verify 反证搜索 + 盲验 → relation #1 落库（unresolved/common_driver，3 支持证据 + 2 反证真实 URL）→ run 终态 partial + tool_calls=8 审计 → GET relations API 返回数据。前端组件层已有 35 例单测覆盖（BoardRelationPanel 17 + useBoardRelations 18）。opencli 端到端留待后续 change/日常使用验证。

## 7. 自动发现纵向切片

- [x] 7.1 在新版块简报成功落库后 non-fatal enqueue 自动发现，严格检查按版块开关、全 sparse、预算为零和冲突任务；验收：`go test ./internal/dataenrichment/service -run 'TestAutoDiscovery'` 退出 0，任何 enqueue 失败均不回滚简报。
- [x] 7.2 按稳定 observation 顺序处理预算内 source，并在 run budget snapshot 记录 skipped、搜索/抓取/loop/timeout 消耗；验收：预算 0/1/上限/上限+1 测试退出 0，代码与测试不存在全量 board-pair 枚举。
- [x] 7.3 在版块配置 UI 增加自动发现开关与预算说明，默认关闭且明确“只生成建议、不自动确认”；验收：组件测试证明旧配置缺字段显示关闭，保存/重载不丢预算。

## 8. 测试

- [x] 8.1 按 `test-cases.md` 实现 resolver、evidence、verifier、动态授权、状态机、简报注入的白盒分支表和边界值测试；验收：表内所有测试用例名能在对应测试文件中检索到，未实现项不得勾选。
- [x] 8.2 使用 PostgreSQL testcontainer 实现迁移、部分唯一索引、并发幂等、事务回滚、状态过滤与过期测试，repository 禁用 SQLite；验收：`cd backend-go && go test ./internal/dataenrichment/repository` 退出 0。
- [x] 8.3 完成 handler/service 影响包回归和前端 composable/组件测试；验收：`cd backend-go && go test ./internal/dataenrichment/...` 退出 0，Windows cmd 下 `pnpm test:unit` 全部通过。
- [ ] 8.4 用真实博查与真实数据库完成 `test-cases.md` 三组跨域效果核对，记录 source 可追溯率、候选 resolved/unresolved/rejected 分布、引用核对率、重复率和调用预算；验收：把量化结果与“达标/上游瓶颈/需调整预期”结论回填 `test-cases.md`。
  - **豁免留痕（2026-09-04，用户批准归档）**：仅完成一组真实数据（日本新闻板块 o2 观察，run #3）：scout 4 条查询全部返回真实结果、extract 产出 ≥2 候选、引用可追溯率 100%（证据均带真实 URL+检索时间戳）、resolve 保守 no_match→unresolved、verify 完成率 1/2（候选2 撞 900s run 超时诚实降级 partial）、tool_calls=8 全部落库可审计。三组跨域量化回填未完成，留待后续 change 或日常使用积累。

## 9. 文档

<!-- doc-impact: flow, api, database, configuration, architecture, standard -->

- [x] 9.1 先阅读 `docs/reference/standard/shared/doc-authoring.md`，再更新 `docs/reference/flow/data-enrichment.md`：补充发现/解析/盲验/裁决/简报消费链路、失败降级和旧数据行为；验收：文档包含代码入口、业务约束与不变量、数据流和变更溯源占位。
- [x] 9.2 更新 `docs/reference/flow/semantic-board.md`：明确版块语义归属与跨版块关系是正交能力，目标解析不改变版块匹配/归属；验收：文档明确“不强制映射、不自动改版块、不全量两两扫描”。
- [x] 9.3 更新 `docs/reference/api/dataenrichment.md`、`docs/reference/api/semantic-boards.md` 及 `_index.md`（如新增注册点），记录 trigger/job/list/detail/confirm/dismiss/re-resolve、状态码和兼容字段；验收：grep 可找到全部新增路由和五种生命周期状态。
- [x] 9.4 更新 `docs/reference/database/DATABASE_FIELDS.md`、`DATA_LIFECYCLE.md`、`ER_DIAGRAM.md` 和数据库 `_index.md`，记录 run/relations 字段、索引、生命周期和保留策略；验收：表名、外键、JSONB 快照和过期规则与迁移一致。
- [x] 9.5 更新 `docs/reference/configuration.md`，记录自动开关默认关闭、各项预算、解析门槛/margin、冷却和 TTL；验收：默认值与代码配置测试逐项一致。

## 10. 验证

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 从观察手动发现 | `backend-go/internal/dataenrichment/handler/relation_discovery_handler_test.go` |
| 从研究问题手动发现 | `backend-go/internal/dataenrichment/handler/relation_discovery_handler_test.go` |
| 自动发现默认关闭 | `backend-go/internal/dataenrichment/service/relation_auto_trigger_test.go` |
| 自动发现遵守预算 | `backend-go/internal/dataenrichment/service/relation_auto_trigger_test.go` |
| 唯一目标解析成功 | `backend-go/internal/dataenrichment/service/relation_resolver_unit_test.go` |
| 多个候选无法消歧 | `backend-go/internal/dataenrichment/service/relation_resolver_unit_test.go` |
| 外部概念尚无内部目标 | `backend-go/internal/dataenrichment/service/relation_resolver_unit_test.go` |
| 支持证据和反证共同进入验证 | `backend-go/internal/dataenrichment/service/relation_verifier_test.go` |
| 共同驱动而非直接因果 | `backend-go/internal/dataenrichment/service/relation_verifier_test.go` |
| 证据不足 | `backend-go/internal/dataenrichment/service/relation_verifier_test.go` |
| 关系被反证 | `backend-go/internal/dataenrichment/service/relation_verifier_test.go` |
| 引用可在原文中核对 | `backend-go/internal/dataenrichment/service/relation_evidence_test.go` |
| 模型输出不存在的引用 | `backend-go/internal/dataenrichment/service/relation_evidence_test.go` |
| 博查不可用 | `backend-go/internal/dataenrichment/service/relation_evidence_test.go` |
| 验证通过仍不自动确认 | `backend-go/internal/dataenrichment/repository/cross_board_relation_test.go` |
| 用户确认建议 | `backend-go/internal/dataenrichment/repository/cross_board_relation_test.go` |
| 重复发现同一建议 | `backend-go/internal/dataenrichment/repository/cross_board_relation_test.go` |
| 驳回建议进入冷却 | `backend-go/internal/dataenrichment/repository/cross_board_relation_test.go` |
| 已确认关系过期 | `backend-go/internal/dataenrichment/repository/cross_board_relation_test.go` |
| 审阅待处理建议 | `front/app/features/tags/components/BoardRelationPanel.test.ts` |
| 追溯已确认关系 | `front/app/features/tags/components/BoardRelationPanel.test.ts` |
| 搜索返回的泳道获得授权 | `backend-go/internal/dataenrichment/service/board_investigation_dynamic_grant_test.go` |
| 猜测的泳道仍被拦截 | `backend-go/internal/dataenrichment/service/board_investigation_dynamic_grant_test.go` |
| 动态授权不跨会话泄漏 | `backend-go/internal/dataenrichment/service/board_investigation_dynamic_grant_test.go` |
| 搜索只暴露紧凑概要 | `backend-go/internal/dataenrichment/service/board_investigation_dynamic_grant_test.go` |
| 跨版块泳道进入调查快照 | `backend-go/internal/dataenrichment/service/board_investigation_cross_board_test.go` |
| 调查失败不改写父数据 | `backend-go/internal/dataenrichment/service/board_investigation_cross_board_test.go` |
| 调查引用其他版块泳道 | `backend-go/internal/dataenrichment/service/board_investigation_cross_board_test.go` |
| 父简报归属保持不变 | `backend-go/internal/dataenrichment/service/board_investigation_cross_board_test.go` |
| 未授权跨版块引用被剔除 | `backend-go/internal/dataenrichment/service/board_investigation_cross_board_test.go` |
| 新简报消费确认关系 | `backend-go/internal/dataenrichment/service/board_brief_cross_relation_test.go` |
| 未确认关系不进入简报 | `backend-go/internal/dataenrichment/service/board_brief_cross_relation_test.go` |
| 简报生成期间不联网 | `backend-go/internal/dataenrichment/service/board_brief_cross_relation_test.go` |
| 旧简报保持不可变 | `backend-go/internal/dataenrichment/service/board_brief_cross_relation_test.go` |
| 关系数量超过预算 | `backend-go/internal/dataenrichment/service/board_brief_cross_relation_test.go` |

- [x] 10.1 运行 `bash scripts/change-scope.sh --json`；期望：退出 0，并只列出本 change 实际影响的 Go 包与前端命令，若提示无法判定则先补路径映射再继续。
- [x] 10.2 运行 `cd backend-go && golangci-lint run ./...`；期望：退出 0、无 lint error。
- [x] 10.3 运行 `cd backend-go && go vet ./...`；期望：退出 0、无 vet error。
- [x] 10.4 运行 `cd backend-go && go test ./internal/dataenrichment/...`；期望：退出 0，目标域全部测试 PASS，DB 集成测试使用 PostgreSQL testcontainer。
- [x] 10.5 运行 `cd backend-go && go build ./...`；期望：退出 0、后端全部包构建成功。
- [x] 10.6 运行 `cd front && pnpm lint`；期望：退出 0、无 ESLint error。
- [x] 10.7 运行 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"`；期望：退出 0、无 TypeScript/Nuxt 类型错误。
- [x] 10.8 运行 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm test:unit 2>&1"`；期望：退出 0、全部前端单测 PASS。
- [x] 10.9 运行 `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"`；期望：退出 0、Nuxt production build 成功。
- [x] 10.10 运行 `openspec validate add-evidence-backed-cross-board-relations --strict`；期望：退出 0并显示 change valid。
- [x] 10.11 运行 `bash scripts/doc-impact.sh verify openspec/changes/add-evidence-backed-cross-board-relations`；期望：退出 0、声明文档域与实际 diff 对账通过。
- [x] 10.12 运行 `bash scripts/check-standards.sh`；期望：退出 0，standard/flow/API/database/configuration 一致性门禁全部通过。
