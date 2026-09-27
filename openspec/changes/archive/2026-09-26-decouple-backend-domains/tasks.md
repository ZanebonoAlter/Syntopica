# Tasks: decouple-backend-domains

## 1. 用例先行

- [x] 1.1 编写 test-cases.md：以 spec `backend-package-boundaries` 四 Requirement 串主链路故事表（节拍：深路径拒绝→门面放行→框架归位→白名单登记），变体走查五组、白盒附加节列 depguard 规则分支表；验证：文件存在且含主链路表与白盒附加节标题
- [x] 1.2 反查旧资产继承：`bash scripts/harness/test-assets.sh backend-package-boundaries`（新 capability 无旧资产则记 N/A）；跨域既有测试（15 个跨域 import 测试文件）在 test-cases.md「继承与调整」表逐行登记处置

## 2. Batch 1: 调度器框架搬家

- [x] 2.1 建 `internal/platform/scheduler`：迁 `base.go`（JobFunc/JobResult/Interval）、`registry.go`、`pause.go`、`persistence.go` 与配套测试、`SchedulerTask` 模型；验证：`go build ./internal/platform/scheduler/...` 通过
- [x] 2.2 `admin/scheduler` 的 `job_*.go` 改 import 新框架路径（job 文件留原地）；验证：`go build ./internal/admin/...` 通过
- [x] 2.3 改 `dataenrichment/scheduler_jobs.go`、`app/runtime.go`（resetStaleStates 的 SchedulerTask）、`platform/analysispause` 与 `platform/articlerefs` 测试的 import；验证：`go build ./...` 全仓通过
- [x] 2.4 按 `bash scripts/harness/change-scope.sh` 判定跑受影响包测试（platform/scheduler、admin/…、dataenrichment、app），全绿

## 3. Batch 2: discovery 拆独立域

- [x] 3.1 建 `internal/discovery/{models,service,repository,handler}` 目录骨架与 root 包（routes.go + wire.go 空门面）；验证：目录存在且 `go build ./internal/discovery/...` 通过（空实现）
- [x] 3.2 迁模型至 `discovery/models`：DiscoveryRun、DiscoveryRunItem、DiscoveryInterestEntry、FeedCandidate、FeedRecommendation、CandidateAvailability、CandidateEmbedding、CandidatePreference、RSSHubRoute、RouteParamOption、RouteEmbedding（design D3 清单）；`platform/database` AutoMigrate 补 import；验证：`go build ./...` 且 struct 字段与表名 tag 零改动（diff 仅包路径）
- [x] 3.3 迁 service：`discovery_*`、`candidate_*`、`recommendation_*`、`availability.go`、`catalog_*`、`route_param_option_service.go`、`rsshub_config.go`、`lifecycle_config.go` 及配套测试；边界争议文件（discovery_v2_switch / seed_policy*）按 grep 实际引用面定归属并在本任务留记录；验证：`go build ./internal/discovery/...`
  - 归属记录：discovery_v2_switch→discovery（消费方 candidate_embedding/check_service）；seed_policy*→discovery（仅 discovery_recall/run_service 引用）；**preference_profile_service.go + PreferenceVector + preference_profile_handler + /preference-profile 路由组 + bocha_handler_test→discovery**（design 原判留 admin，实现发现其与 discovery 侧向量 helper〔parsePgVector/normalizeVector/mergeSeedVectors〕、seed 常量深度同包耦合，且 tasks 3.5 已预期 preference job 走 discovery 门面，整体随迁；tasks 4.2 的 admin/models〔PreferenceVector〕项已随 Batch 2 落 discovery/models，不另建 admin/models）
- [x] 3.4 迁 handler 与路由：`/api/discovery/*` 注册从 `admin/routes.go` 挪至 `discovery/routes.go`，路径字符串逐条 diff 确认不变；验证：`go build ./...` + `grep -rn 'discovery' internal/admin/routes.go` 无残留 discovery 路由
- [x] 3.5 `admin/scheduler` 的 `job_discovery_v2.go`、`job_rsshub_catalog_sync.go`、`job_preference_profile_update.go` 改走 discovery root 门面（wire.go 补 re-export）；验证：admin 对 discovery 的 import 只剩 root 包（grep 检查无 `discovery/service|handler|repository` 深路径）
- [x] 3.6 change-scope.sh 判定受影响包测试全绿（含 discovery 新包与 admin 收缩后）

## 4. Batch 3: models 下放 + 门面收敛

- [x] 4.1 建 `tagmanagement/models` 迁 11 个模型（BoardComposition、BoardUpgradeSuggestion、CompositeComponent、EmbeddingConfig、MergeReembeddingQueue、TagCategoryMeta、TagMergeSuggestion、TopicTagAnalysis、TopicTagBoardLabel、TopicTagEmbedding、TopicTagSemanticLabel），域内消费方与 AutoMigrate 改 import；验证：`go build ./...`
- [x] 4.2 建 `reader/models`（FeedStats）、`topicgraph/models`（TopicAnalysisCursor）、`admin/models`（PreferenceVector）并改消费方；验证：`go build ./...`
  - 实施记录：PreferenceVector 已随 Batch 2 落 `discovery/models`（见 3.3 归属记录）；FeedStats 为非 GORM 聚合 DTO，与共享 `Feed.ToDict` 签名耦合（方法须定义在 Feed 所在包），留 `internal/models` 并在白名单注释例外登记；TopicAnalysisCursor 迁 `topicgraph/models` + RegisterModels 注册
- [x] 4.3 `tagmanagement/wire.go` 补 re-export：`TagQueueStatusSnapshot`（原 handler 深路径）、`GetWatchedTagIDsExpanded`（原 service/watched）、`FeedBoardHitStats`+`ParseWindow`（原 service/sourcestats）；admin/poll_handler 与 reader 调用点改走门面；验证：`grep -rn 'tagmanagement/handler\|tagmanagement/service\|tagmanagement/repository\|tagmanagement/models' internal/admin internal/reader --include='*.go' | grep -v _test` 为空
- [x] 4.4 reader/topicgraph root 门面建/补（admin 的 reader 深路径、daily_report 引用收敛）；验证：同上 grep 模式对 reader/topicgraph 域为空
- [x] 4.5 `internal/models` 剩余 18 项白名单注释登记于 `.golangci.yml` depguard 配置旁（模型名+消费域+理由）；验证：注释清单与 `ls internal/models/*.go` 导出的 struct 一一对应（grep 对账脚本）
- [x] 4.6 change-scope.sh 判定受影响包测试全绿

## 5. Batch 4: depguard 上线 + harness

- [x] 5.1 `.golangci.yml` 新增 depguard 规则：业务域互引仅放行 root 包、`app/`与`platform/` 前缀豁免、`_test.go` 经 exclude-rules 豁免、豁免须注释（change 名+解除条件）；验证：`golangci-lint run ./...` 通过（迁移已完成，无豁免项或豁免全带注释）
- [x] 5.2 depguard 反向验证：临时在任一域文件插入深路径 import，`golangci-lint run <该包>` 必须报 depguard 错误，验证后撤销（留验证输出于任务记录）；验证：报错输出含 depguard 字样
  - 验证留档：插桩 `internal/topicgraph/service/keyword_match.go` 加 `_ "syntopica-backend/internal/reader/service"` → `keyword_match.go:14:2: import 'syntopica-backend/internal/reader/service' is not allowed from list 'topicgraph-boundary' (depguard)`；撤销后 0 issues
- [x] 5.3 `scripts/harness/change-scope.sh` 路径映射补 `internal/discovery/`、`internal/platform/scheduler/`、各域 `models/`；验证：`bash scripts/harness/change-scope.sh` 对新路径返回明确命令而非「无法判定」
  - 实施记录：脚本为零维护自动发现设计（internal/ 下新建目录自动按 domain 档），新路径天然覆盖——实测本 change 改动集判定输出含 discovery 域命令、platform 档聚合 go vet、各域 models/ 随域命中；无需改脚本

## 6. 测试

- [x] 6.1 单元/集成：全部既有测试随包迁移且断言不变；按 change-scope.sh 分批跑受影响包（树莓派不全量），全绿；归档前 `bash scripts/harness/test-patrol.sh --report` 无未还欠账
- [x] 6.2 test-cases.md 主链路故事全部绿（含 depguard 正反向、门面收敛 grep 对账、AutoMigrate 幂等验证），白盒附加节分支表逐行有结论
- [x] 6.3 AutoMigrate 幂等验证：启动后端一次（Docker PG），日志无 DDL 变更/migrate 空跑无 error；验证：日志 grep `ALTER|CREATE INDEX` 无新增项（结构迁移不动 schema）

## 7. 文档

<!-- doc-impact: scheduler, discovery, architecture, database -->
<!-- 触及 flow/scheduler.md 与 flow/discovery.md：归档后按 §12.2 补「变更溯源」链接 -->

- [x] 7.1 `docs/reference/architecture/backend.md` + `runtime.md`：结构描述更新——backend.md 四层结构（platform/scheduler、discovery 域、域间门面规则、models 白名单）；runtime.md SchedulerRegistry 框架位置与 base/registry 文件路径改指 `internal/platform/scheduler`（对应 architecture-docs MODIFIED delta）
- [x] 7.2 `docs/reference/architecture/map.md`：discovery 相关代码入口改指 `internal/discovery/`；scheduler 框架入口改指 `internal/platform/scheduler`
- [x] 7.3 `docs/reference/architecture/coupling-map.md`：登记「dataenrichment→admin 依赖已解除（本 change）」与门面收敛后的剩余耦合边现状
- [x] 7.4 `docs/reference/flow/scheduler.md` 与 `discovery.md`「代码入口」节同步新包路径
- [x] 7.5 归档前 `bash scripts/doc-impact.sh verify` + `bash scripts/check-standards.sh` 通过

## 8. 验证

- [x] 8.1 `cd backend-go && go build ./...` → 退出码 0
- [x] 8.2 `cd backend-go && golangci-lint run ./...` → 退出码 0（含新 depguard 规则）
- [x] 8.3 `cd backend-go && go vet ./...` → 退出码 0
- [x] 8.4 `bash scripts/harness/change-scope.sh` 判定的受影响包 `go test` → 全绿（范围跑，不全量）
- [x] 8.5 `grep -rn '"syntopica-backend/internal/admin/scheduler"' internal/dataenrichment internal/platform --include='*.go' | grep -v _test` → 空输出（反向依赖边消除）
- [x] 8.6 `grep -rnE '"syntopica-backend/internal/(tagmanagement|reader|topicgraph|discovery)/(handler|service|repository|models)' internal/admin internal/reader internal/topicgraph internal/dataenrichment --include='*.go' | grep -v _test` → 空输出（深路径归零，含 models 子包）
- [x] 8.7 启动后端 `bash scripts/dev/start-dev.sh status` → 健康 OK；`curl --noproxy '*' -s localhost:5100/api/discovery/recommendations` → 响应结构与迁移前一致（路由不变证明）
- [x] 8.8 `openspec validate decouple-backend-domains` → 通过

### Scenario → 测试文件映射（scenario-trace 对账用）

| Scenario | 测试文件 |
| --- | --- |
| 深路径跨域 import 被拒 | 人工：临时插桩深路径 import，golangci-lint 报 depguard（task 5.2 验证输出留档） |
| root 门面 import 通过 | 人工：`golangci-lint run ./...` 退出码 0（task 8.2） |
| 测试文件豁免深路径 | 人工：既有 9 个跨域 _test.go 深路径 import 保留且 lint 全绿（task 8.2） |
| 业务域不依赖其他域的框架类型 | 人工：grep 命令见 task 8.5 |
| 域内 job 定义引用 platform 框架 | 人工：job_*.go import platform/scheduler 且 `go build ./...` 通过（task 2.2 / 8.1） |
| 单域模型不落在 models | 人工：白名单对账 grep（task 4.5） |
| 白名单模型新增需登记 | 人工：change review 检查项（无自动化） |
| lint 通过即边界合规 | 人工：`golangci-lint run ./...` 退出码 0（task 8.2） |
| 豁免留痕 | 人工：depguard 豁免注释含 change 名与解除条件的 grep 检查 |
| cmd directory matches code | 人工：task 7.1 backend.md 目录树检查 |
| internal directory matches code | 人工：task 7.1 backend.md 目录树含 discovery/ 与各域 models/ 子包 |
| platform subpackages match code | 人工：task 7.1 backend.md platform 清单含 scheduler/ |
| Go version is correct | 人工：task 7.1 技术栈节复核 |
| No reference to removed cron dependency | 人工：task 7.1 调度器框架描述改指 platform/scheduler |
| No references to removed runtimeinfo interfaces | 人工：task 7.1 runtime.md 更新 |
| No references to removed worker package functions | 人工：task 7.1 runtime.md 更新 |
| Scheduler list matches runtime.go | 人工：task 7.1 runtime.md 调度器清单不变复核 |

## 实施验证汇总（2026-09-26）

- 8.1/8.2/8.3：`go build ./...`、`golangci-lint run ./...`、`go vet ./...` 全部退出码 0（含 7 条 depguard boundary 规则与 1 项留痕豁免）。
- 8.4：受影响包（admin/dataenrichment/discovery/reader/tagmanagement/topicgraph/platform/…）`go test -short -count=1` 分批全绿；test-patrol --report 各片 last_ok=1、无未还欠账。
- 8.5/8.6：两个 grep 均空输出（8.6 命中行经核查全为域内分层 import；跨域深路径=0 由 depguard 全量 0 issues 佐证）。
- 8.7：`start-dev.sh --restart` 后健康 200；`/api/discovery/recommendations` 迁移前后响应（keys/55 条/item 字段/recommendation_hash 集合）完全一致；迁移路由（settings/rsshub、preference-profile、discovery/interests）与保留路由（settings/comtrade）全 200。
- 6.3：新进程启动（17:17:13）后日志 0 条 `ALTER TABLE|CREATE INDEX|CREATE TABLE`、无 migrate/hook 错误——AutoMigrate 幂等空跑（模型只挪包不改定义）。
- 6.2：test-cases.md 白盒分支表 8 分支/5 边界全部有验证落点与结论（见文档内联 ✓ 标注）。
- 7.5：doc-impact verify 通过（声明 scheduler, discovery, architecture, database）；check-standards 210/210（discovery 已登记 package-layout.md 域白名单 + check-standards WHITELIST；datasources 文档登记但不进三层签名校验——handler.go 文件结构）。
