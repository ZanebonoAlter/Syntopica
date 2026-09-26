# Design: decouple-backend-domains

## Context

见 proposal.md（Why）。耦合现状量化数据：跨域 import 边共 9 条（生产文件 14 个 + 跨域测试文件 9 个，合计 23；2026-09-26 复测基线见 explore-findings）；真正的厚耦合在 `internal/models`（44 struct / 29 个消费包）、`admin` 大杂烩（25.7k 行 / 7+ 子域）、调度器框架错位（`admin/scheduler`）。本设计决定「怎么切、按什么顺序切、怎么防回潮」。

## Goals / Non-Goals

**Goals**
- 消除 `dataenrichment → admin` 反向依赖（调度器搬家）
- discovery 从 admin 拆为独立域（用户已决策）
- 单域独占模型下放所属域（保守：12 个真共享 + AI 系/Notification 留 models，见 D3）
- 跨域 import 收敛到 root 门面，depguard 编译期固化

**Non-Goals**
- 不做多 module 拆分（单人项目收益低）
- 不做事件总线/消费者侧接口重构（域间调用面已薄，P4 边仅收敛门面，不换通信范式）
- 不动 API 路径、DB schema、前端代码
- 不拆 admin 剩余部分（AI 运维/偏好画像/阅读行为/通知留在 admin，待后续 change）

## Decisions

### D1: 调度器搬家切法——框架走、job 留

`admin/scheduler` 的 `base.go`（JobFunc/JobResult/Interval）、`registry.go`、`pause.go`、`persistence.go` 迁 `internal/platform/scheduler`；`job_*.go`（具体任务编排，依赖各域业务）留在 `admin/scheduler` 只改 import。`SchedulerTask` 模型随框架迁 `platform/scheduler`（app 的 `resetStaleStates` 改 import platform，合法：app 是装配层）。`dataenrichment/scheduler_jobs.go`、platform 测试（analysispause/articlerefs）改引新路径。一次性切换（单 change 无需兼容 alias），`admin/scheduler` 不留转发层。

### D2: discovery 域的形状与边界

新域 `internal/discovery/{handler,service,repository}`，三件套自包含。迁移范围（自 admin）：
- **service**：`discovery_*`、`candidate_*`（access/catalog/check/embedding/identity）、`recommendation_*`、`availability.go`、`catalog_sync/extras`、`route_param_option_service.go`、`rsshub_config.go`、`lifecycle_config.go`
- **handler**：`discovery_handler.go`、`candidate_catalog_handler.go`、`candidate_check_handler.go`、`route_param_option_handler.go` 及配套测试
- **模型**：discovery_run.go / discovery_candidate.go / candidate_availability.go 全部 struct + RSSHubRoute/RouteParamOption/RouteEmbedding → `internal/discovery/models` 子包
- **路由**：`/api/discovery/*` 注册从 `admin/routes.go` 挪到 `discovery/routes.go`，路径字符串不变
- **job**：`job_discovery_v2.go`、`job_rsshub_catalog_sync.go`、`job_preference_profile_update.go` 留 admin/scheduler（它们是编排者），经 discovery root 门面调用
- **不随走**：`preference_profile_service.go` + `PreferenceVector`（偏好画像，非 discovery；留 admin）、`seed_policy*.go`、`discovery_v2_switch.go` 若被非 discovery 方消费则评估归属（实现时按实际引用面定，倾向 discovery）

### D3: 模型归属清单（保守版，每域建 `models/` 子包防循环）

域 root 包已有 wire/routes 等会 import 子包（如 tagmanagement root re-export service），模型若放域 root 会形成 root→service→root 循环。**决策：每域建 `internal/<domain>/models/` 子包**放域内独占模型，service/handler/repository 统一 import 它，`platform/database` AutoMigrate 补 import。

| 去向 | 模型 |
|---|---|
| `discovery/models` | DiscoveryRun, DiscoveryRunItem, DiscoveryInterestEntry, FeedCandidate, FeedRecommendation, CandidateAvailability, CandidateEmbedding, CandidatePreference, RSSHubRoute, RouteParamOption, RouteEmbedding |
| `tagmanagement/models` | BoardComposition, BoardUpgradeSuggestion, CompositeComponent, EmbeddingConfig, MergeReembeddingQueue, TagCategoryMeta, TagMergeSuggestion, TopicTagAnalysis, TopicTagBoardLabel, TopicTagEmbedding, TopicTagSemanticLabel |
| `reader/models` | FeedStats |
| `topicgraph/models` | TopicAnalysisCursor |
| `admin/models` | PreferenceVector |
| `platform/scheduler` | SchedulerTask |
| **留 `internal/models`（白名单）** | AISettings, Article, ArticleTopicTag, Category, EmbeddingQueue, Feed, FirecrawlJob, ReadingBehavior, SemanticLabel, TagJob, TopicTag, TopicTagRelation（12 真共享）+ **AIProvider, AIRoute, AIRouteProvider, AICallLog, AIEmbeddingCache, Notification**（platform/airouter 与 admin 双消费 / platform/notification 消费——grep 单域标记是因 platform 被过滤，实际跨层共享） |

白名单共 18 项，注释登记于 `.golangci.yml` depguard 配置旁（spec 要求）。

### D4: 门面推广——wire.go 模式复制到 reader/topicgraph/discovery

跨域调用点全部收敛到目标域 root 门面（re-export var/type）：
- **tagmanagement/wire.go 补**：`TagQueueStatusSnapshot`（原 handler 深路径，admin/handler/poll_handler 用）、`GetWatchedTagIDsExpanded`（原 service/watched，reader 用）、`FeedBoardHitStats` + `ParseWindow`（原 service/sourcestats，reader 用）
- **reader root 新建 wire**：admin 的 job_content_completion 等 reader 深路径调用点按需 re-export（实现时按实际调用清单补）
- **topicgraph root 补**：admin 的 daily_report 相关引用收敛（现 admin→topicgraph 仅 1 文件）
- **discovery root 新建 wire**：admin/scheduler 的 discovery job 调用点走它

### D5: depguard 规则设计

`backend-go/.golangci.yml` 新增 depguard 规则：业务域列表内，A 域生产文件 deny import `internal/<B域>/{,handler,service,repository,models}/...` 深路径（root 放行；`models/` 子包同属深路径——跨域引用域内独占模型类型必须经对方 root 门面 re-export，不得直接 import 对方 `models/`）；`app/` 与 `platform/` 前缀文件豁免该规则；测试文件经 `issues.exclude-rules`（path `_test\.go`）豁免。豁免登记：配置注释写 change 名 + 解除条件。规则先上线再迁移（迁移完成前允许带豁免注释的过渡项，归档前清零）。

### D6: harness 联动

`scripts/harness/change-scope.sh` 路径→命令映射补 `internal/discovery/`、`internal/platform/scheduler/` 等新路径；`.golangci.yml` 属 backend 影响面（lint 零欠账 spec 兼容）。

## Risks / Trade-offs

- [迁移中间态编译红] → 按域成批迁移，每批收口跑 `go build ./...`；批内顺序：先建目标包 → 移模型 → 移 service → 移 handler → 改消费方 import → build
- [GORM AutoMigrate 误动表] → 模型只挪包不改 struct 字段与表名 tag；迁移后启动验证 migrate 日志无 DDL 变更（幂等空跑）；不碰真库验证 DDL（testing.md 红线）
- [测试文件 import 面大] → 跨域测试依赖共 9 个测试文件（2026-09-26 复测），与生产同批修正；`reader/repository` 对 topicgraph/repository 的 blank import 随包路径更新
- [discovery 与 preference/seed 边界判断错] → D2 列了倾向，实现时以 grep 实际引用面为准；有争议文件在 tasks 标注「实现时定归属」
- [depguard 规则过严卡日常开发] → 先 deny 已消灭的深路径模式；新违规是本来就该走门面的信号，豁免注释通道保底

## Migration Plan

单 change 内分四批 commit（每批独立可 revert，无数据迁移）：
1. **Batch 1**：platform/scheduler 搬家（D1）+ change-scope.sh 更新
2. **Batch 2**：discovery 拆域（D2）+ discovery/models（D3 部分）
3. **Batch 3**：其余域 models 下放（D3 剩余）+ 门面收敛（D4）
4. **Batch 4**：depguard 上线（D5）+ 文档同步（backend.md / map.md / coupling-map.md）

回滚：任一批出问题 revert 对应 commit；结构无状态，无需数据回滚。

## Open Questions

- discovery_v2_switch 与 seed_policy 的归属（倾向 discovery，实现时按引用面定，不影响任务结构）
